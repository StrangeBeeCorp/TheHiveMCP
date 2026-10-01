package checks

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gatesdoc"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
)

// CIWorkflow is the workflow that must run every gate whose cadence includes ci.
const CIWorkflow = ".github/workflows/ci.yml"

var (
	inlineCode = regexp.MustCompile("`([^`]+)`")
	lineSuffix = regexp.MustCompile(`:(\d+(-\d+)?|[A-Za-z_][A-Za-z0-9_.]*)$`) // a line, a range or a symbol
	gateFlag   = regexp.MustCompile(`--gate[ =]([A-Za-z0-9_,-]+)`)
	braces     = regexp.MustCompile(`\{([^{}]*)\}`)
)

// Docs checks that the documentation agents and people start from stays true: every repository
// path it names exists, docs/reference/gates.md matches the registry, and ci.yml runs every ci gate.
func Docs(root string, files []string, reg *registry.Registry) ([]Finding, error) {
	var out []Finding

	tops := topDirs(files)
	for _, f := range files {
		if !isEntryDoc(f) {
			continue
		}

		content, err := os.ReadFile(filepath.Join(root, f)) // #nosec G304 -- files come from git ls-files
		if os.IsNotExist(err) {
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("docs: %w", err)
		}

		out = append(out, missingPaths(root, f, content, tops, files)...)
	}

	page, err := os.ReadFile(filepath.Join(root, gatesdoc.Path)) // #nosec G304 -- a fixed path in the repository
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("docs: %w", err)
	}

	if string(page) != gatesdoc.Render(reg) {
		out = append(out, Finding{Path: gatesdoc.Path, Msg: "does not match gates/registry.yaml: run make harness-docs"})
	}

	ci, err := ciFindings(root, reg)
	if err != nil {
		return nil, err
	}

	return append(out, ci...), nil
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

func missingPaths(root, doc string, content []byte, tops map[string]bool, files []string) []Finding {
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
				if !exists(root, p, files) {
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

func exists(root, p string, files []string) bool {
	if strings.Contains(p, "*") {
		pat := strings.TrimSuffix(p, "/")
		for _, f := range files {
			if registry.Match(pat, f) || registry.Match(pat+"/**", f) {
				return true
			}
		}

		return false
	}

	_, err := os.Stat(filepath.Join(root, p)) // #nosec G703 -- only tells whether a path a document names exists

	return err == nil
}

// ciFindings checks that ci.yml runs every ci gate through bin/harness, and names no gate the
// registry does not have.
func ciFindings(root string, reg *registry.Registry) ([]Finding, error) {
	content, err := os.ReadFile(filepath.Join(root, CIWorkflow)) // #nosec G304 -- a fixed path in the repository
	if err != nil {
		return nil, fmt.Errorf("docs: %w", err)
	}

	named := map[string]bool{}

	for _, m := range gateFlag.FindAllStringSubmatch(string(content), -1) {
		for id := range strings.SplitSeq(m[1], ",") {
			named[id] = true
		}
	}

	var out []Finding

	for _, g := range reg.At("ci") {
		if !named[g.ID] {
			out = append(out, Finding{Path: CIWorkflow, Msg: fmt.Sprintf("does not run gate %q, whose cadence includes ci", g.ID)})
		}
	}

	ids := make([]string, 0, len(named))
	for id := range named {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	for _, id := range ids {
		if _, ok := reg.Gate(id); !ok {
			out = append(out, Finding{Path: CIWorkflow, Msg: fmt.Sprintf("runs gate %q, which gates/registry.yaml does not define", id)})
		}
	}

	return out, nil
}
