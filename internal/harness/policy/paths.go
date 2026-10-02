package policy

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

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

// ProtectedPatterns are the patterns of the protected classes, for .github/CODEOWNERS to match.
func ProtectedPatterns() []string {
	out := make([]string, 0, len(protected))
	for _, p := range protected {
		out = append(out, p.pattern)
	}

	return out
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

// lookAlike is why a path with a letter the filesystem reads as a plain one is refused.
const lookAlike = "%s holds a letter this filesystem reads as a plain one (ſ as s, the Kelvin sign as k, ß as ss): write the path in plain letters"

// foldsToASCII reports whether p holds a letter that a filesystem ignoring case reads as a plain
// one: a letter whose case folds to ASCII (ſ, the Kelvin sign), ß and ẞ, the Latin ligatures (ﬁ).
// The refusals compare names in ASCII case only, so such a path could name a guarded file.
func foldsToASCII(p string) bool {
	for _, r := range p {
		if r < utf8.RuneSelf {
			continue
		}

		if r == 'ß' || r == 'ẞ' || (r >= 0xFB00 && r <= 0xFB06) {
			return true
		}

		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			if f < utf8.RuneSelf {
				return true
			}
		}
	}

	return false
}

// classInAnyCase is the protected class of rel with its letters in any case: on a filesystem that
// ignores case, CLAUDE.MD is CLAUDE.md.
func classInAnyCase(rel string) (Class, bool) {
	if class, ok := ClassOf(rel); ok {
		return class, true
	}

	lower := strings.ToLower(rel)
	for _, p := range protected {
		if registry.Match(strings.ToLower(p.pattern), lower) {
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

// Write decides whether the role may write the file at p (absolute, or relative to root), judged
// where it lands: through the symlinks on its way, and with the names a refusal guards (.git, the
// records, the protected classes) in any case, since this host's filesystem ignores it.
func Write(role Role, root, p string) Decision {
	if foldsToASCII(p) {
		return deny(lookAlike, p)
	}

	root, abs := landing(root), landing(absolute(root, p))
	if inGit(abs) {
		return deny("never write inside .git: git's own files change through git commands")
	}

	if configFile(abs) {
		return deny("%s is git's or gh's configuration, which a person sets: an alias or a program named there runs "+
			"commands the policy never reads", p)
	}

	rel, inside := relative(root, abs)
	if (inside && record(strings.ToLower(rel))) || namesRecord(abs) {
		return deny("%s is the harness's own record (make pr's evidence, the hooks' state): only the harness writes it", rel)
	}

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

	if class, ok := classInAnyCase(rel); ok {
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

	rel, inside := relative(landing(root), landing(absolute(root, p)))
	if !inside || role == Main || role.ReadOnly() {
		return Allow
	}

	if foldsToASCII(rel) {
		return deny(lookAlike, rel)
	}

	if rel = strings.ToLower(rel); under(rel, strings.ToLower(CanaryDir)) {
		return deny("the canaries are the known-bad patches the gates must catch: implement from the spec, not from them")
	}

	if role == Coder && under(rel, strings.ToLower(SealedDir)) {
		return deny("the sealed tests stay hidden from the Coder until the end: make the visible tests pass")
	}

	return Allow
}

// record reports whether rel is a record only the harness writes: make pr's evidence, which a
// push trusts, and the hooks' state, which counts the red rounds.
func record(rel string) bool {
	return under(rel, ".cache/harness/evidence/") || rel == ".cache/harness/state.json"
}

// namesRecord reports whether a path, from wherever it comes, ends in a record of a harness
// checkout: .cache/harness/evidence/… or .cache/harness/state.json, in any case.
func namesRecord(abs string) bool {
	lower := strings.ToLower(filepath.ToSlash(abs))

	return strings.Contains(lower, "/.cache/harness/evidence/") || strings.HasSuffix(lower, "/.cache/harness/evidence") ||
		strings.HasSuffix(lower, "/.cache/harness/state.json")
}

// guardedPlaces are the places a link may not point at, into or above: where the harness keeps its
// records, and where git and gh read the configuration a person sets. A link to one of them, or
// to a directory above it, would let a later write in the same line reach it under a name the
// policy does not know yet.
func guardedPlaces(root string) []string {
	root = landing(root)
	places := []string{filepath.Join(root, ".cache", "harness", "evidence"), filepath.Join(root, ".cache", "harness", "state.json")}

	return append(places, configDirs()...)
}

// related reports whether a and b are the same place, or one is inside the other, in any case.
func related(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)

	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// holdsRecords reports whether rel is a directory a record lies under: the checkout, .cache or
// .cache/harness, in any case.
func holdsRecords(rel string) bool {
	rel = strings.ToLower(rel)

	return rel == "." || rel == "" || rel == ".cache" || rel == ".cache/harness"
}

// configFile reports whether abs is where git or gh reads the configuration a person sets:
// ~/.gitconfig and the files it includes beside it (~/.gitconfig-work), and git's and gh's
// directories under ~/.config, $XDG_CONFIG_HOME or $GH_CONFIG_DIR.
func configFile(abs string) bool {
	lower := strings.ToLower(filepath.ToSlash(abs))
	if strings.HasPrefix(filepath.Base(lower), ".gitconfig") || strings.Contains(lower, "/.config/git/") ||
		strings.Contains(lower, "/.config/gh/") {
		return true
	}

	return slices.ContainsFunc(configDirs(), func(d string) bool { return related(lower, d) && len(lower) >= len(d) })
}

// configDirs are git's and gh's configuration directories, where they land: under ~/.config,
// $XDG_CONFIG_HOME or $GH_CONFIG_DIR.
func configDirs() []string {
	var dirs []string

	home, err := os.UserHomeDir()
	if err == nil {
		dirs = append(dirs, filepath.Join(home, ".config", "git"), filepath.Join(home, ".config", "gh"))
	}

	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		dirs = append(dirs, filepath.Join(x, "git"), filepath.Join(x, "gh"))
	}

	if g := os.Getenv("GH_CONFIG_DIR"); g != "" {
		dirs = append(dirs, filepath.Clean(g))
	}

	for i, d := range dirs {
		dirs[i] = landing(d)
	}

	return dirs
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
	if err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
		return filepath.ToSlash(rel), true
	}

	// A filesystem that ignores case opens the checkout by any spelling of its path; the rest of
	// the path keeps its own.
	r, a := filepath.ToSlash(filepath.Clean(root)), filepath.ToSlash(abs)
	if len(a) < len(r) || !strings.EqualFold(a[:len(r)], r) || (len(a) > len(r) && a[len(r)] != '/') {
		return "", false
	}

	if rel = strings.TrimPrefix(a[len(r):], "/"); rel == "" {
		rel = "."
	}

	return rel, true
}

func inGit(abs string) bool {
	return slices.ContainsFunc(strings.Split(filepath.ToSlash(abs), "/"), func(part string) bool { return strings.EqualFold(part, ".git") })
}

// maxLinks bounds how many dangling symlinks landing follows by hand.
const maxLinks = 16

// landing is where a write or a read of p lands: the part of p that exists with its symlinks
// resolved, a dangling one followed by hand, then the rest of p.
func landing(p string) string {
	for range maxLinks {
		existing, rest := p, ""

		for {
			_, err := os.Lstat(existing) // #nosec G703 -- the policy looks where a path lands; it opens nothing
			if err == nil {
				break
			}

			parent := filepath.Dir(existing)
			if parent == existing {
				return p
			}

			rest, existing = filepath.Join(filepath.Base(existing), rest), parent
		}

		resolved, err := filepath.EvalSymlinks(existing)
		if err == nil {
			return filepath.Join(resolved, rest)
		}

		target, err := os.Readlink(existing)
		if err != nil {
			return p
		}

		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(existing), target)
		}

		p = filepath.Join(target, rest)
	}

	return p
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
