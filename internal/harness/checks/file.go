package checks

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"slices"
	"strings"
)

// GoSource checks that every Go file of the tree parses and is gofmt-formatted. It needs no
// toolchain, so it runs on each edit; the fmt gate applies the full formatters later.
func GoSource(tree Tree) ([]Finding, error) {
	var out []Finding

	for _, f := range tree.Files() {
		if !strings.HasSuffix(f, ".go") {
			continue
		}

		content, err := tree.Read(f)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("go-source: %w", err)
		}

		out = append(out, goSource(f, content)...)
	}

	return out, nil
}

func goSource(f string, content []byte) []Finding {
	formatted, err := format.Source(content)
	if err != nil {
		return []Finding{{Path: f, Msg: "does not parse: " + err.Error()}}
	}

	if !bytes.Equal(formatted, content) {
		return []Finding{{Path: f, Msg: "gofmt would rewrite it: run make fmt"}}
	}

	return nil
}

// OneFile runs a built-in check on one edited file, for the edit cadence: what can be judged from
// that file alone, in well under a second. A file shorter than its cap is not reported here,
// because lowering a cap is not the editing agent's call.
func OneFile(builtin, rel string, content []byte, lim Limits) ([]Finding, error) {
	switch builtin {
	case "structure":
		var out []Finding

		top := strings.SplitN(rel, "/", 2)[0]
		if !slices.Contains(lim.Structure.Root, top) {
			out = append(out, Finding{Path: rel, Msg: fmt.Sprintf("top-level entry %q is not in structure.root of gates/limits.yaml: ask before adding one", top)})
		}

		out = append(out, cleanFindings(rel, content, lim.Structure)...)

		if !isBinary(content) && matchesFile(lim.Structure.CodeFiles, rel) {
			out = append(out, overFindings(rel, countLines(content), lim.Structure)...)
		}

		return out, nil
	case "public-safety":
		return PublicSafety(MemTree{rel: content}, lim.PublicSafety)
	case "go-source":
		if strings.HasSuffix(rel, ".go") {
			return goSource(rel, content), nil
		}

		return nil, nil
	}

	return nil, fmt.Errorf("builtin %q does not check single files", builtin)
}
