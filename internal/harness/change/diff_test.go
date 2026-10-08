package change

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	aGoFile = "a.go"
	bGoFile = "b.go"
	movedGo = "moved.go"
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
diff --git a/my file.go b/my file.go
` + "--- a/my file.go\t\n+++ b/my file.go\t\n" + `@@ -1,0 +2 @@
+// a space
diff --git a/old.go b/moved.go
similarity index 100%
rename from old.go
rename to moved.go
diff --git a/was.go b/"edi\"ted.go"
similarity index 90%
rename from was.go
rename to "edi\"ted.go"
--- a/was.go
+++ "b/edi\"ted.go"
@@ -4,0 +5 @@
+// added
`

	d, renames := ParseDiff(patch)
	assert.Equal(t, Diff{
		aGoFile:      {{N: 3, Text: "\treturn 1"}, {N: 19, Text: "+++ not a header"}, {N: 20, Text: "second"}},
		"sp\tace.go": {{N: 1, Text: "package sp"}},
		"my file.go": {{N: 2, Text: "// a space"}},
		`edi"ted.go`: {{N: 5, Text: "// added"}},
	}, d, "a file moved as it was adds no line; a path with a space loses the tab git writes after it")
	assert.Equal(t, Renames{movedGo: "old.go", `edi"ted.go`: "was.go"}, renames)
}

func TestAHunkHeaderItCannotReadAddsNothing(t *testing.T) {
	t.Parallel()

	for _, header := range []string{"@@ -1 @@", "@@ -1 x1 @@", "@@ -1 +a,1 @@", "@@ -1 +1,b @@"} {
		start, count := hunk(header)
		assert.Zero(t, count, header)
		assert.Zero(t, start, header)
	}

	d, _ := ParseDiff("+++ b/a.go\n@@ -1 +a @@\n+x\n")
	assert.Empty(t, d)

	d, _ = ParseDiff("+++ c/a.go\n@@ -0,0 +1 @@\n+x\n")
	assert.Empty(t, d, "a header without b/ names no file")

	_, renames := ParseDiff("rename to b.go\n")
	assert.Empty(t, renames, "a rename names where it comes from first")
}
