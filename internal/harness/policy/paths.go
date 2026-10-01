package policy

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
)

const (
	// BlockedFile is where an agent that stopped red too many times says why.
	BlockedFile = ".cache/harness/BLOCKED.md"
	// SealedDir holds the Tester's sealed tests: the policy refuses the Coder the commands that
	// read them before the end of the duel (see the read check of the Bash judge).
	SealedDir = ".claude/sealed/"
	// CanaryDir holds the known-bad patches the gates must catch; no implementing role reads them.
	CanaryDir  = "gates/canaries/"
	scratchDir = ".cache/tmp/"
)

// Class is a part of the repository only the main session changes directly.
type Class string

// The protected classes. Each one has a remedy an agent is told when it is denied.
const (
	HarnessPolicy Class = "harness policy"
	Gates         Class = "gates"
	Spec          Class = "spec"
	Baselines     Class = "baselines"
	Trust         Class = "trust"
)

var protected = []struct {
	pattern string
	class   Class
}{
	{"cmd/harness/**", HarnessPolicy},
	{"internal/harness/**", HarnessPolicy},
	{"cmd/surface/**", HarnessPolicy},
	{"cmd/analyzers/**", HarnessPolicy},
	{"internal/surface/**", HarnessPolicy},
	{"internal/analyzers/**", HarnessPolicy},
	{"bin/**", HarnessPolicy},
	{".githooks/**", HarnessPolicy},
	{".github/**", HarnessPolicy},
	{".claude/settings.json", HarnessPolicy},
	{".claude/agents/**", HarnessPolicy},
	{".claude/workflows/**", HarnessPolicy},
	{".claude/skills/**", HarnessPolicy},
	{"CLAUDE.md", HarnessPolicy},
	{"Makefile", HarnessPolicy},
	{"scripts/harness.mk", HarnessPolicy},
	{"scripts/lint.sh", HarnessPolicy},
	{"scripts/fmt.sh", HarnessPolicy},
	{"scripts/lint-makefile.sh", HarnessPolicy},
	{"gates/**", Gates},
	{".golangci.yml", Gates},
	{".markdownlint.jsonc", Gates},
	{".prettierrc.json", Gates},
	{".gitleaks.toml", Gates},
	{"checkmake.ini", Gates},
	{"spec/**", Spec},
	{"baselines/**", Baselines},
	{"internal/permissions/embedded/default_permissions.yaml", Trust},
}

var remedies = map[Class]string{
	HarnessPolicy: "The harness is fixed during a task: report the problem instead of changing it.",
	Gates:         "Gates and limits are fixed for implementers: make the code pass, or stop BLOCKED with the reason.",
	Spec:          "If the spec is wrong or missing, say so in your report; a person changes the spec.",
	Baselines:     "Regenerate a baseline with its make target (make help lists them), never by hand.",
	Trust:         "The default permission profile is read-only on purpose: report why it should change.",
}

// ClassOf is the protected class of a repository-relative path, if it has one.
func ClassOf(rel string) (Class, bool) {
	for _, p := range protected {
		if registry.Match(p.pattern, rel) {
			return p.class, true
		}
	}

	return "", false
}

// IsTest reports whether a repository-relative path is test code: the Tester's, never the Coder's.
func IsTest(rel string) bool {
	return strings.HasSuffix(rel, "_test.go") ||
		registry.Match("**/testdata/**", rel) ||
		strings.HasPrefix(rel, "internal/testutils/") ||
		strings.HasPrefix(rel, SealedDir)
}

// IsEnvFile reports whether a path names an environment file, which holds secrets. The template
// lists the keys and is the only one anyone reads.
func IsEnvFile(p string) bool {
	base := filepath.Base(p)
	return strings.HasPrefix(base, ".env") && base != ".env.template"
}

// Write decides whether the role may write the file at p (absolute, or relative to root).
func Write(role Role, root, p string) Decision {
	abs := absolute(root, p)
	if inGit(abs) {
		return deny("never write inside .git: git's own files change through git commands")
	}

	rel, inside := relative(root, abs)
	if !inside {
		if role == Main || temporary(abs) {
			return Allow
		}

		return deny("agents write inside their checkout, or in a temporary directory ($TMPDIR, %s)", scratchDir)
	}

	return writeInside(role, rel)
}

func writeInside(role Role, rel string) Decision {
	switch {
	case role == Main:
		return Allow
	case role.ReadOnly():
		return deny("the %s reads and reports: write your findings in your final answer", role)
	case rel == BlockedFile || strings.HasPrefix(rel, scratchDir):
		return Allow
	}

	if class, ok := ClassOf(rel); ok {
		return deny("%s is %s and the %s may not change it. %s", rel, class, role, remedies[class])
	}

	if role == Tester && !IsTest(rel) {
		return deny("the Tester writes only tests (*_test.go, testdata/, internal/testutils/, %s): "+
			"%s is code. Put the change the Coder should make in your questions", SealedDir, rel)
	}

	if role == Coder && IsTest(rel) {
		return deny("%s is a test, and tests are the Tester's: never weaken one. "+
			"If a test is wrong, say why in your questions to the Tester", rel)
	}

	return Allow
}

// hiddenFrom are the directories the role may not read: the canaries for an agent that writes,
// and the sealed tests too for the Coder.
func hiddenFrom(role Role) []string {
	switch {
	case role == Main || role.ReadOnly():
		return nil
	case role == Coder:
		return []string{CanaryDir, SealedDir}
	}

	return []string{CanaryDir}
}

func underAny(rel string, dirs []string) bool {
	return slices.ContainsFunc(dirs, func(d string) bool { return under(rel, d) })
}

// Read decides whether the role may read the file at p.
func Read(role Role, root, p string) Decision {
	if IsEnvFile(p) {
		return deny("environment files hold secrets and nobody reads them: the keys are listed in .env.template " +
			"and internal/types/constants.go")
	}

	rel, inside := relative(root, absolute(root, p))
	if !inside || role == Main || role.ReadOnly() {
		return Allow
	}

	if under(rel, CanaryDir) {
		return deny("the canaries are the known-bad patches the gates must catch: implement from the spec, not from them")
	}

	if role == Coder && under(rel, SealedDir) {
		return deny("the sealed tests stay hidden from the Coder until the end: make the visible tests pass")
	}

	return Allow
}

// under reports whether rel is the directory dir (given with its trailing slash) or inside it.
func under(rel, dir string) bool {
	return rel == strings.TrimSuffix(dir, "/") || strings.HasPrefix(rel, dir)
}

func absolute(root, p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, p[2:])
		}
	}

	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}

	return filepath.Clean(p)
}

// relative is p relative to root, slash-separated, and whether p is inside root.
func relative(root, abs string) (string, bool) {
	rel, err := filepath.Rel(filepath.Clean(root), abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}

	return filepath.ToSlash(rel), true
}

func inGit(abs string) bool {
	return slices.Contains(strings.Split(filepath.ToSlash(abs), "/"), ".git")
}

func temporary(abs string) bool {
	dirs := []string{"/tmp/", "/private/tmp/", "/var/folders/", "/private/var/folders/"}
	if t := os.Getenv("TMPDIR"); t != "" {
		dirs = append(dirs, filepath.Clean(t)+"/")
	}

	for _, d := range dirs {
		if strings.HasPrefix(abs, d) {
			return true
		}
	}

	return false
}
