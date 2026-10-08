package change

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	aGoFile = "a.go"
	bGoFile = "b.go"
)

// The diff keeps each added line with its number in the new file, whatever the hunk header says:
// a missing count means one line, a hunk that only removes adds nothing, and a deleted file has no
// lines.
func TestTheDiffKeepsEachAddedLineWithItsNumber(t *testing.T) {
	t.Parallel()

	patch := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -3 +3 @@ func A() {
-	return 0
+	return 1
@@ -10,2 +10,0 @@
-x
-y
@@ -20,0 +19,2 @@
++++ not a header
+second
diff --git a/gone.go b/gone.go
--- a/gone.go
+++ /dev/null
@@ -1 +0,0 @@
-package gone
diff --git "a/sp\tace.go" "b/sp\tace.go"
--- "a/sp\tace.go"
+++ "b/sp\tace.go"
@@ -0,0 +1 @@
+package sp
`

	assert.Equal(t, Diff{
		aGoFile:      {{N: 3, Text: "\treturn 1"}, {N: 19, Text: "+++ not a header"}, {N: 20, Text: "second"}},
		"sp\tace.go": {{N: 1, Text: "package sp"}},
	}, ParseDiff(patch))
}

func TestAHunkHeaderItCannotReadAddsNothing(t *testing.T) {
	t.Parallel()

	for _, header := range []string{"@@ -1 @@", "@@ -1 x1 @@", "@@ -1 +a,1 @@", "@@ -1 +1,b @@"} {
		start, count := hunk(header)
		assert.Zero(t, count, header)
		assert.Zero(t, start, header)
	}

	assert.Empty(t, ParseDiff("+++ b/a.go\n@@ -1 +a @@\n+x\n"))
	assert.Empty(t, ParseDiff("+++ c/a.go\n@@ -0,0 +1 @@\n+x\n"), "a header without b/ names no file")
}
