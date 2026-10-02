package shell_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/shell"
)

// The words most lines here are made of.
const (
	git  = "git"
	push = "push"
)

func words(line string) [][]string {
	var out [][]string

	for _, c := range shell.Parse(line).Commands {
		if len(c.Words) > 0 {
			out = append(out, c.Words)
		}
	}

	return out
}

func TestOperatorsQuotesAndKeywords(t *testing.T) {
	t.Parallel()

	assert.Equal(t, [][]string{
		{"cd", "x"}, {git, "commit", "-m", "a && b"}, {"tee", "out"}, {"echo", "q;r"},
	}, words(`cd x && FOO=1 git commit -m 'a && b' | tee out; echo "q;r"`))
	assert.Equal(t, [][]string{{git, "update-ref", "a", "b"}, {"true"}}, words("if git update-ref a b; then true; fi"))
	assert.Equal(t, [][]string{{git, push}}, words("{ git push; }"))
	assert.Equal(t, [][]string{{"rm", "${f}"}}, words("for f in a b; do rm $f; done"))
	assert.Equal(t, [][]string{{git, push}}, words(`g"i"t pu\sh`), "quotes and escapes are removed")
	assert.Equal(t, [][]string{{git, push}}, words("f() { git push; }; f")[:1], "a function body runs its commands")
}

func TestAssignmentsAreRecorded(t *testing.T) {
	t.Parallel()

	p := shell.Parse("GIT_CONFIG_COUNT=1 git commit")
	require.Len(t, p.Commands, 1)
	assert.Equal(t, []string{"GIT_CONFIG_COUNT=1"}, p.Commands[0].Assigns)

	p = shell.Parse("export GIT_DIR=/x; git log")
	assert.Equal(t, []string{"GIT_DIR=/x"}, p.Commands[0].Assigns, "export assigns too")

	p = shell.Parse(`export ID; declare -x "$v" X=1`)
	require.Len(t, p.Commands, 2)
	assert.Equal(t, []string{"ID"}, p.Commands[0].Assigns, "a bare name exports without assigning")
	assert.Equal(t, []string{"-x", "${v}", "X=1"}, p.Commands[1].Assigns, "an option, and a name computed at run time")
}

func TestSubstitutionsAreCommandsOfTheirOwn(t *testing.T) {
	t.Parallel()

	assert.Contains(t, words(`echo "$(git update-ref a b)"`), []string{git, "update-ref", "a", "b"})
	assert.Contains(t, words("echo `git push`"), []string{git, push})
	assert.Contains(t, words("cat <(git push)"), []string{git, push})
	assert.Contains(t, words("cat <<EOF\n$(git push)\nEOF"), []string{git, push}, "an unquoted here-document runs its substitutions")
}

func TestQuotedHereDocumentsAreData(t *testing.T) {
	t.Parallel()

	p := shell.Parse("cat > msg.txt <<'EOF'\ngit push --force\nrm -rf gates\nEOF\ngit commit -F msg.txt")

	assert.Equal(t, [][]string{{"cat"}, {git, "commit", "-F", "msg.txt"}}, words("cat > msg.txt <<'EOF'\ngit push --force\nEOF\ngit commit -F msg.txt"))
	assert.Equal(t, []string{"msg.txt"}, p.Commands[0].Writes)
}

func TestComputedCommandsAreOpaque(t *testing.T) {
	t.Parallel()

	assert.NotEmpty(t, shell.Parse("$(echo git) push").Opaque)
	assert.NotEmpty(t, shell.Parse("${G} push").Opaque)
	assert.NotEmpty(t, shell.Parse(`$'\x67it' push`).Opaque, "ANSI-C escapes are not decoded")
	assert.NotEmpty(t, shell.Parse("/usr/bin/gi? push").Opaque, "a glob in the command name")
	assert.NotEmpty(t, shell.Parse("echo 'open").Opaque, "a line that does not parse")
	assert.Empty(t, shell.Parse("git log --oneline -5").Opaque)
}

func TestBracesAreExpanded(t *testing.T) {
	t.Parallel()

	assert.Equal(t, [][]string{{git, push, "puh"}}, words("git pu{s,}h"))
}

func TestRedirectionsNameWhatTheyWrite(t *testing.T) {
	t.Parallel()

	p := shell.Parse("grep x f 2>/dev/null >out.txt; cmd &> log 2>&1; cat < in > .git/refs/heads/main; echo >&file")

	var writes []string
	for _, c := range p.Commands {
		writes = append(writes, c.Writes...)
	}

	assert.Equal(t, []string{"/dev/null", "out.txt", "log", ".git/refs/heads/main", "file"}, writes)
	assert.Equal(t, []string{"grep", "x", "f"}, p.Commands[0].Words)
	assert.Equal(t, []string{"cat"}, p.Commands[2].Words, "an input redirection is not a word")
}

func TestAPlainVariableKeepsItsName(t *testing.T) {
	t.Parallel()

	line := shell.Parse(`grep FAIL "$TMPDIR/it.log" ${HOME}/x $1 ${X:-y} $(pwd)`)
	require.Len(t, line.Commands, 2, "the substitution is a command of its own")
	assert.Equal(t, []string{"grep", "FAIL", "${TMPDIR}/it.log", "${HOME}/x", "$", "$", "$"}, line.Commands[0].Words)
	assert.Equal(t, []bool{true, true, false, false, false, false, false}, line.Commands[0].Exact)
}

func TestRedirectionsNameWhatTheyRead(t *testing.T) {
	t.Parallel()

	line := shell.Parse("sort < in.txt > out.txt; cat <> both.txt")
	require.Len(t, line.Commands, 2)
	assert.Equal(t, []string{"in.txt"}, line.Commands[0].Reads)
	assert.Equal(t, []string{"out.txt"}, line.Commands[0].Writes)
	assert.Equal(t, []string{"both.txt"}, line.Commands[1].Reads)
}

func TestPipesNameTheirReader(t *testing.T) {
	t.Parallel()

	p := shell.Parse("make test-integration 2>&1 | tail -20")
	require.Len(t, p.Commands, 2)
	assert.Equal(t, []string{"tail", "-20"}, p.Commands[0].Into)

	p = shell.Parse("a | b | c")
	assert.Equal(t, []string{"b"}, p.Commands[0].Into)
	assert.Equal(t, []string{"c"}, p.Commands[1].Into)
}

func TestAChildShellsCommandsAreMarked(t *testing.T) {
	t.Parallel()

	child := map[string]bool{}
	for _, c := range shell.Parse(`(cd a); cd b | sort; echo "$(cd c)"; cd d & cd e; f() { cd g; }`).Commands {
		child[strings.Join(c.Words, " ")] = c.Child
	}

	assert.Equal(t, map[string]bool{
		"cd a": true, "cd b": true, "sort": true, "echo $": false, "cd c": true, "cd d": true, "cd e": false, "cd g": false,
	}, child, "a subshell, a pipeline, a substitution and the background are child shells; a function body is not")
}
