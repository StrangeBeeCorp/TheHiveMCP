package policy

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/shell"
)

// copyOverRecords refuses a directory copy, a move or an extraction into a directory that holds the
// harness's records: it could drop a forged evidence file where make pr's goes. Into .cache or
// .cache/harness, any is refused; into the checkout itself, an extraction (what an archive holds
// is not known) and a copy or move of a source that carries a .cache. A recursive delete or
// chmod writes no file there.
func (j *judge) copyOverRecords(name string, args []string, target string) Decision {
	archive := name == cmdTar || name == cmdUnzip
	whole := archive || slices.Contains([]string{"mv", "rsync", "ditto"}, name) || (name == "cp" && recursive(args))

	if !whole || !filepath.IsAbs(j.cwd) {
		return Allow
	}

	rel, inside := j.landed(filepath.Clean(j.abs(target)))

	switch {
	case !inside || !holdsRecords(rel):
		return Allow
	case archive || (rel != "." && rel != ""):
		return deny("%s %s writes a whole tree into %s, which holds the harness's records: copy the files you need one by one", name, target, rel)
	}

	for _, src := range operandsBut(args, target) {
		if j.carriesCache(src) {
			return deny("%s %s brings a .cache into the checkout, where the harness keeps its records: copy the files you need one by one", name, src)
		}
	}

	return Allow
}

// operandsBut are a command's operands other than target.
func operandsBut(args []string, target string) []string {
	var out []string

	for _, a := range args {
		if !strings.HasPrefix(a, "-") && a != target {
			out = append(out, a)
		}
	}

	return out
}

// carriesCache reports whether a source of a copy may bring a .cache with it: it is one, holds one,
// or is computed or a pattern whose matches do.
func (j *judge) carriesCache(src string) bool {
	if computed(src) {
		return true
	}

	paths := []string{j.abs(src)}
	if strings.ContainsAny(src, "*?[") {
		matches, _ := filepath.Glob(strings.ReplaceAll(j.abs(src), "[!", "[^"))
		paths = append(paths, matches...)
	}

	return slices.ContainsFunc(paths, func(p string) bool {
		if strings.EqualFold(filepath.Base(filepath.Clean(p)), ".cache") {
			return true
		}

		_, err := os.Lstat(filepath.Join(p, ".cache"))

		return err == nil
	})
}

// recursive reports whether cp's options copy a whole tree: -r, -R or -a, alone or in a cluster
// (-rf, -Rp), or their long forms.
func recursive(args []string) bool {
	return slices.ContainsFunc(args, func(a string) bool {
		if a == "--recursive" || a == "--archive" {
			return true
		}

		return len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.ContainsAny(a[1:], "rRa")
	})
}

// inPlace reports whether a read command is used to write: sed -i or w, sort -o, uniq's or xxd's
// second file, yq -i.
func inPlace(name string, args []string) bool {
	return len(readerWrites(name, args)) > 0
}

// readerWrites are the files a read command writes besides what it prints.
func readerWrites(name string, args []string) []string {
	switch name {
	case cmdSed:
		call := parseSed(args)
		if call.inPlace {
			return append(call.files, call.writes...)
		}

		return call.writes
	case cmdSort:
		return flagValues(args, "kStT", "o", "--output")
	case "uniq":
		return operands(args, "fsw", 1)
	case "xxd":
		return operands(args, "cglosn", 1)
	case "yq":
		return yqWrites(args)
	case cmdAwk:
		return awkWrites(args)
	}

	return nil
}

// operands are a command's operands from the n-th on, once its options are set aside: valued are
// its short options that take a value.
func operands(args []string, valued string, n int) []string {
	var out []string

	for i := 0; i < len(args); i++ {
		a := args[i]

		switch {
		case a == "--":
			out = append(out, args[i+1:]...)
			i = len(args)
		case len(a) > 1 && a[0] == '-' && a[1] != '-':
			if strings.IndexByte(valued, a[len(a)-1]) >= 0 {
				i++
			}
		case !strings.HasPrefix(a, "-") || a == "-":
			out = append(out, a)
		}
	}

	if len(out) <= n {
		return nil
	}

	return out[n:]
}

// yqWrites are the files yq -i rewrites: its operands after the expression. yq -s names the files
// it writes as it runs, which the policy cannot read.
func yqWrites(args []string) []string {
	if flagValues(args, "opI", "s", "--split-exp") != nil {
		return []string{shell.Computed + "yq-split"}
	}

	inplace := slices.ContainsFunc(args, func(a string) bool {
		return a == "--inplace" || (len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.Contains(cutAtValued(a[1:], "opIs"), "i"))
	})
	if !inplace {
		return nil
	}

	return operands(args, "opI", 1)
}

// extracts reports whether tar or unzip writes the archive's files: unzip unless it only lists or
// tests, tar with x (or --extract, --get) in its mode.
func extracts(name string, args []string) bool {
	if name == cmdUnzip {
		// -l, -t, -v and -Z list or test, -p and -c print, -z shows the comment.
		return !slices.ContainsFunc(args, func(a string) bool {
			return len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.ContainsAny(cutAtValued(a[1:], "dPx"), "ltvZpcz")
		})
	}

	extracting := false

	for i, a := range args {
		switch {
		case a == "--to-stdout":
			return false
		case a == "--extract" || a == optGet:
			extracting = true
		case strings.HasPrefix(a, "--"):
		case strings.HasPrefix(a, "-") || i == 0:
			cluster := cutAtValued(strings.TrimPrefix(a, "-"), "fbCFgHIKLNTVX")
			if strings.Contains(cluster, "O") {
				return false
			}

			extracting = extracting || strings.Contains(cluster, "x")
		}
	}

	return extracting
}

// moverValued are, per command that copies or moves, its short options that take a value.
var moverValued = map[string]string{"cp": "S", "mv": "S", cmdInstall: "mogS", "ln": "S"}

// targets are the files a writing command names.
func targets(name string, args []string) []string {
	var positional []string

	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
		}
	}

	switch name {
	case "tee", "rm", "rmdir", "touch", "chmod", "chown", "truncate", "mkdir", "unlink", "shred":
		return positional
	case "cp", "mv", cmdInstall, "ln", "rsync", "ditto":
		return moverTargets(name, args, positional)
	case "dd":
		for _, a := range args {
			if out, ok := strings.CutPrefix(a, "of="); ok {
				return []string{out}
			}
		}
	}

	if w := readerWrites(name, args); w != nil {
		return w
	}

	return outputs(name, args)
}

// moverTargets are where a copy, a move or a link lands: the -t directory, else every operand of ln
// (as before) and the last operand of the others.
func moverTargets(name string, args, positional []string) []string {
	if valued, ok := moverValued[name]; ok {
		if dir := flagValues(args, valued, "t", "--target-directory"); dir != nil {
			return dir
		}
	}

	if name == "ln" || len(positional) == 0 {
		return positional
	}

	return positional[len(positional)-1:]
}

// curlValued and wgetValued are the short options of curl and wget that take a value.
const (
	curlValued = "AbcCdDeEFHKmoPQrtTuUwxXyYz"
	wgetValued = "aABDeiIloOPQRtTUwX"
)

// outputs are what the commands that write besides their main job name: the files curl and wget
// save, and the directory tar or unzip extracts into.
func outputs(name string, args []string) []string {
	switch name {
	case "curl":
		files := flagValues(args, curlValued, "oDc", "--output", "--dump-header", "--cookie-jar", "--trace",
			"--trace-ascii", "--stderr", "--etag-save", "--output-dir")
		if remoteNamed(args) {
			files = append(files, urlNames(args, firstOf(flagValues(args, curlValued, "", "--output-dir")))...)
		}

		return files
	case "wget":
		files := flagValues(args, wgetValued, "Ooa", "--output-document", "--output-file", "--append-output")
		if flagValues(args, wgetValued, "O", "--output-document") == nil {
			files = append(files, urlNames(args, firstOf(flagValues(args, wgetValued, "P", "--directory-prefix")))...)
		}

		return files
	case cmdTar:
		if dir := flagValues(args, "fbFgHIKLNTVX", "C", "--directory"); dir != nil {
			return dir
		}
	case cmdUnzip:
		if dir := flagValues(args, "P", "d"); dir != nil {
			return dir
		}
	}

	if (name == cmdTar || name == cmdUnzip) && extracts(name, args) {
		return []string{"."}
	}

	return nil
}

// remoteNamed reports whether curl saves what it fetches under the name the URL ends with: -O,
// --remote-name or --remote-name-all.
func remoteNamed(args []string) bool {
	return slices.ContainsFunc(args, func(a string) bool {
		if a == "--remote-name" || a == "--remote-name-all" {
			return true
		}

		return len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.ContainsRune(cutAtValued(a[1:], curlValued), 'O')
	})
}

// urlNames are the files a download names after its URLs: the last part of each URL's path, in
// dir when there is one.
func urlNames(args []string, dir string) []string {
	var out []string

	for _, a := range args {
		_, rest, ok := strings.Cut(a, "://")
		if !ok {
			continue
		}

		rest, _, _ = strings.Cut(rest, "?")

		name := "index.html"
		if _, p, ok := strings.Cut(rest, "/"); ok && path.Base(p) != "." && path.Base(p) != "/" && !strings.HasSuffix(p, "/") {
			name = path.Base(p)
		}

		out = append(out, filepath.Join(dir, name))
	}

	return out
}

func firstOf(values []string) string {
	if len(values) == 0 {
		return ""
	}

	return values[0]
}

// cutAtValued is a cluster of short options up to the first that takes a value: what follows it is
// that value, not more options.
func cutAtValued(cluster, valued string) string {
	for k := range len(cluster) {
		if strings.IndexByte(valued, cluster[k]) >= 0 {
			return cluster[:k+1]
		}
	}

	return cluster
}

// flagValues are the values of a command's options shorts and longs, given as -o FILE, -oFILE, at
// the end of a cluster (-sSo FILE) or inside one (-sSoFILE), --output FILE or --output=FILE, or a
// long option cut short as getopt accepts it (--out FILE).
// valued are the command's other short options that take a value: in a cluster, what follows one
// is its value, not more options.
func flagValues(args []string, valued, shorts string, longs ...string) []string {
	var out []string

	for i := 0; i < len(args); i++ {
		a := args[i]

		var (
			value string
			named bool
		)

		switch {
		case a == "--":
			return out
		case strings.HasPrefix(a, "--"):
			name, v, attached := strings.Cut(a, "=")
			named = slices.ContainsFunc(longs, func(l string) bool { return abbreviates(name, l, len("--o")) })

			if named && !attached && i+1 < len(args) {
				i++
				v = args[i]
			}

			value = v
		case len(a) > 1 && a[0] == '-':
			i, value, named = clusterValue(args, i, valued, shorts)
		}

		if named {
			out = append(out, value)
		}
	}

	return out
}

// clusterValue reads the short options of args[i]: the value of the last one, attached or the next
// word, and whether it is one of shorts. It returns the index of the last word it read.
func clusterValue(args []string, i int, valued, shorts string) (int, string, bool) {
	a := args[i]
	cluster := cutAtValued(a[1:], valued+shorts)
	last := cluster[len(cluster)-1]

	value := a[1+len(cluster):]
	if value == "" && strings.IndexByte(valued+shorts, last) >= 0 && i+1 < len(args) {
		i++
		value = args[i]
	}

	return i, value, strings.IndexByte(shorts, last) >= 0 && value != ""
}
