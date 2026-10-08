package checks

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gatesdoc"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
	"gopkg.in/yaml.v3"
)

const (
	// CIWorkflow is the workflow that must run every gate whose cadence includes ci.
	CIWorkflow = ".github/workflows/ci.yml"
	// NightlyWorkflow runs every gate whose cadence includes nightly, then files their findings.
	NightlyWorkflow = ".github/workflows/nightly.yml"
	// harnessRun is how a workflow step runs gates.
	harnessRun = "bin/harness run"
)

var (
	inlineCode = regexp.MustCompile("`([^`]+)`")
	lineSuffix = regexp.MustCompile(`:(\d+(-\d+)?|[A-Za-z_][A-Za-z0-9_.]*)$`) // a line, a range or a symbol
	gateFlag   = regexp.MustCompile(`--gate[ =]([A-Za-z0-9_,-]+)`)
	braces     = regexp.MustCompile(`\{([^{}]*)\}`)
)

// Docs checks that the documentation agents and people start from stays true: every repository
// path it names exists, the pages keep their Diataxis folders, sections and links (DocsLayout),
// docs/reference/gates.md matches the registry, and ci.yml runs every ci gate.
func Docs(tree Tree, reg *registry.Registry) ([]Finding, error) {
	var out []Finding

	files := tree.Files()
	tops := topDirs(files)

	for _, f := range files {
		if !isEntryDoc(f) {
			continue
		}

		content, err := tree.Read(f)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("docs: %w", err)
		}

		out = append(out, missingPaths(tree, f, content, tops)...)
	}

	layout, err := DocsLayout(tree)
	if err != nil {
		return nil, err
	}

	out = append(out, layout...)

	page, err := tree.Read(gatesdoc.Path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("docs: %w", err)
	}

	if string(page) != gatesdoc.Render(reg) {
		out = append(out, Finding{Path: gatesdoc.Path, Msg: "does not match gates/registry.yaml: run make harness-docs"})
	}

	for _, w := range []struct{ path, cadence string }{{CIWorkflow, "ci"}, {NightlyWorkflow, "nightly"}} {
		found, err := workflowFindings(tree, reg, w.path, w.cadence)
		if err != nil {
			return nil, err
		}

		out = append(out, found...)
	}

	return out, nil
}

// isEntryDoc selects the pages an agent or a newcomer reads first.
func isEntryDoc(f string) bool {
	switch f {
	case "AGENTS.md", "README.md", "CLAUDE.md":
		return true
	}

	return strings.HasPrefix(f, ".claude/") && strings.HasSuffix(f, ".md")
}

// topDirs lists the top-level directories, so that only repository paths are checked.
func topDirs(files []string) map[string]bool {
	tops := map[string]bool{}

	for _, f := range files {
		if i := strings.IndexByte(f, '/'); i > 0 {
			tops[f[:i]] = true
		}
	}

	return tops
}

func missingPaths(tree Tree, doc string, content []byte, tops map[string]bool) []Finding {
	var out []Finding

	sc := bufio.NewScanner(bytes.NewReader(content))
	fenced := false

	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if t := strings.TrimSpace(text); strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fenced = !fenced
			continue
		}

		if fenced {
			continue
		}

		for _, m := range inlineCode.FindAllStringSubmatch(text, -1) {
			for _, p := range candidatePaths(m[1], tops) {
				if !exists(tree, p) {
					out = append(out, Finding{Path: doc, Line: line, Msg: fmt.Sprintf("names %q, which does not exist", p)})
				}
			}
		}
	}

	return out
}

// candidatePaths returns the repository paths an inline code span names, braces expanded; none
// when it is not a path (a command, a placeholder such as <tool>, a path outside the repository).
func candidatePaths(span string, tops map[string]bool) []string {
	span = lineSuffix.ReplaceAllString(strings.TrimRight(span, ".,;"), "")
	if strings.ContainsAny(span, " <>$()'\"=") {
		return nil
	}

	i := strings.IndexByte(span, '/')
	if i <= 0 || !tops[span[:i]] {
		return nil
	}

	return expand(span)
}

func expand(p string) []string {
	m := braces.FindStringSubmatchIndex(p)
	if m == nil {
		return []string{p}
	}

	var out []string
	for alt := range strings.SplitSeq(p[m[2]:m[3]], ",") {
		out = append(out, expand(p[:m[0]]+alt+p[m[1]:])...)
	}

	return out
}

func exists(tree Tree, p string) bool {
	if !strings.Contains(p, "*") {
		return tree.Exists(p)
	}

	pat := strings.TrimSuffix(p, "/")
	for _, f := range tree.Files() {
		if registry.Match(pat, f) || registry.Match(pat+"/**", f) {
			return true
		}
	}

	return false
}

// workflowFindings checks that a workflow runs every gate of its cadence through bin/harness, and
// names no gate the registry does not have. A cadence with no gate needs no workflow.
func workflowFindings(tree Tree, reg *registry.Registry, path, cadence string) ([]Finding, error) {
	gates := reg.At(cadence)

	content, err := tree.Read(path)
	if errors.Is(err, fs.ErrNotExist) && len(gates) == 0 {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("docs: %w", err)
	}

	named, err := ranGates(path, content)
	if err != nil {
		return nil, err
	}

	var out []Finding

	for _, g := range gates {
		if !named[g.ID] {
			out = append(out, Finding{Path: path, Msg: fmt.Sprintf("does not run gate %q, whose cadence includes %s", g.ID, cadence)})
		}
	}

	ids := make([]string, 0, len(named))
	for id := range named {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	for _, id := range ids {
		if _, ok := reg.Gate(id); !ok {
			out = append(out, Finding{Path: path, Msg: fmt.Sprintf("runs gate %q, which gates/registry.yaml does not define", id)})
		}
	}

	return out, nil
}

// workflow is the part of a GitHub workflow the docs gate reads: the scripts of each job's steps.
type workflow struct {
	Jobs map[string]struct {
		Steps []struct {
			Run string `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// ranGates lists the gates the workflow's steps run: the --gate values of the step scripts that
// call bin/harness run, comment lines left out. A YAML comment is no step, and a shell comment
// runs nothing.
func ranGates(path string, content []byte) (map[string]bool, error) {
	var wf workflow

	err := yaml.Unmarshal(content, &wf)
	if err != nil {
		return nil, fmt.Errorf("docs: read %s: %w", path, err)
	}

	named := map[string]bool{}

	for _, job := range wf.Jobs {
		for _, step := range job.Steps {
			if strings.Contains(step.Run, harnessRun) {
				addGates(named, step.Run)
			}
		}
	}

	return named, nil
}

// addGates adds the --gate values of a step script to named, its comment lines left out.
func addGates(named map[string]bool, script string) {
	for line := range strings.SplitSeq(script, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}

		for _, m := range gateFlag.FindAllStringSubmatch(line, -1) {
			for id := range strings.SplitSeq(m[1], ",") {
				named[id] = true
			}
		}
	}
}
