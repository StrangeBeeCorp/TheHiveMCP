package surface

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	dirPerm  = 0o750
	filePerm = 0o644
	// summaryFile holds the token counts the budgets are checked against.
	summaryFile = "tokens.json"
)

// Summary is the token cost of the surface.
type Summary struct {
	Tools          map[string]int `json:"tools"`
	ToolsTotal     int            `json:"tools_total"`
	PromptsTotal   int            `json:"prompts_total"`
	ResourcesTotal int            `json:"resources_total"`
}

// Summary counts the tokens of the snapshot.
func (s Snapshot) Summary() Summary {
	sum := Summary{Tools: map[string]int{}}

	for _, t := range s.Tools {
		sum.Tools[t.Name] = t.Tokens
		sum.ToolsTotal += t.Tokens
	}

	for _, p := range s.Prompts {
		sum.PromptsTotal += p.Tokens
	}

	for _, r := range s.Resources {
		sum.ResourcesTotal += r.Tokens
	}

	return sum
}

// Files renders the snapshot as the files of baselines/surface/, keyed by their path in it: one
// file per tool and per prompt, the resource list, and the token summary.
func (s Snapshot) Files() (map[string][]byte, error) {
	files := map[string][]byte{}

	for dir, entries := range map[string][]Tool{"tools": s.Tools, "prompts": s.Prompts} {
		for _, t := range entries {
			var b bytes.Buffer

			err := json.Indent(&b, t.JSON, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("indent %s %s: %w", dir, t.Name, err)
			}

			files[dir+"/"+t.Name+".json"] = append(b.Bytes(), '\n')
		}
	}

	for name, v := range map[string]any{"resources.json": s, summaryFile: s.Summary()} {
		data, err := pretty(v)
		if err != nil {
			return nil, err
		}

		files[name] = data
	}

	return files, nil
}

// Diff compares the files with those under dir and names each one that differs, is missing or is
// no longer produced.
func Diff(dir string, files map[string][]byte) ([]string, error) {
	committed, err := read(dir)
	if err != nil {
		return nil, err
	}

	var out []string

	for name, data := range files {
		old, ok := committed[name]

		switch {
		case !ok:
			out = append(out, name+": new, not in the snapshot")
		case !bytes.Equal(old, data):
			out = append(out, name+": changed")
		}
	}

	for name := range committed {
		if _, ok := files[name]; !ok {
			out = append(out, name+": in the snapshot, no longer advertised")
		}
	}

	sort.Strings(out)

	return out, nil
}

// Write makes dir hold exactly the files.
func Write(dir string, files map[string][]byte) error {
	committed, err := read(dir)
	if err != nil {
		return err
	}

	for name := range committed {
		if _, ok := files[name]; !ok {
			err = os.Remove(filepath.Join(dir, name))
			if err != nil {
				return fmt.Errorf("remove %s: %w", name, err)
			}
		}
	}

	for name, data := range files {
		p := filepath.Join(dir, name)

		err = os.MkdirAll(filepath.Dir(p), dirPerm)
		if err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(p), err)
		}

		err = os.WriteFile(p, data, filePerm)
		if err != nil {
			return fmt.Errorf("write %s: %w", p, err)
		}
	}

	return nil
}

func read(dir string) (map[string][]byte, error) {
	files := map[string][]byte{}

	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return fs.SkipAll
		}

		if err != nil || d.IsDir() {
			return err
		}

		data, err := os.ReadFile(p) // #nosec G304 G122 -- a file of the committed snapshot, in the checkout
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}

		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return fmt.Errorf("locate %s: %w", p, err)
		}

		files[filepath.ToSlash(rel)] = data

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}

	return files, nil
}

// compact encodes v as the server sends it, without escaping <, > and &.
func compact(v any) ([]byte, error) {
	var b bytes.Buffer

	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)

	err := enc.Encode(v)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}

	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

func pretty(v any) ([]byte, error) {
	var b bytes.Buffer

	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")

	err := enc.Encode(v)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}

	return b.Bytes(), nil
}

// Budgets bound what the surface may cost a model (gates/budgets.yaml).
type Budgets struct {
	// ToolsTotal bounds every tools/list entry together.
	ToolsTotal int `yaml:"tools_total"`
	// Tool bounds any one tool.
	Tool int `yaml:"tool"`
	// ResourcesTotal bounds the static resource texts together.
	ResourcesTotal int `yaml:"resources_total"`
	// PromptsTotal bounds every prompts/list entry together: 0 while the server has none, so the
	// first prompt comes with a reviewed budget.
	PromptsTotal int `yaml:"prompts_total"`
}

// LoadBudgets reads the surface section of gates/budgets.yaml, rejecting unknown keys.
func LoadBudgets(path string) (Budgets, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the repository's own budgets file
	if err != nil {
		return Budgets{}, fmt.Errorf("read budgets: %w", err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var file struct {
		Surface Budgets `yaml:"surface"`
	}

	err = dec.Decode(&file)
	if err != nil {
		return Budgets{}, fmt.Errorf("parse %s: %w", path, err)
	}

	b := file.Surface
	if b.ToolsTotal <= 0 || b.Tool <= 0 || b.ResourcesTotal <= 0 {
		return Budgets{}, fmt.Errorf("%s: surface needs tools_total, tool and resources_total", path)
	}

	return b, nil
}

// Over names each budget the summary exceeds.
func (b Budgets) Over(sum Summary) []string {
	var out []string

	if sum.ToolsTotal > b.ToolsTotal {
		out = append(out, fmt.Sprintf("the tools cost %d tokens together, over the budget of %d", sum.ToolsTotal, b.ToolsTotal))
	}

	names := make([]string, 0, len(sum.Tools))
	for name := range sum.Tools {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		if n := sum.Tools[name]; n > b.Tool {
			out = append(out, fmt.Sprintf("%s costs %d tokens, over the per-tool budget of %d", name, n, b.Tool))
		}
	}

	if sum.ResourcesTotal > b.ResourcesTotal {
		out = append(out, fmt.Sprintf("the static resources cost %d tokens together, over the budget of %d",
			sum.ResourcesTotal, b.ResourcesTotal))
	}

	if sum.PromptsTotal > b.PromptsTotal {
		out = append(out, fmt.Sprintf("the prompts cost %d tokens together, over the budget of %d", sum.PromptsTotal, b.PromptsTotal))
	}

	return out
}

// Describe is a one-line account of a summary, for logs.
func (sum Summary) Describe() string {
	parts := make([]string, 0, len(sum.Tools))
	for name, n := range sum.Tools {
		parts = append(parts, fmt.Sprintf("%s %d", name, n))
	}

	sort.Strings(parts)

	return fmt.Sprintf("tools %d tokens (%s), prompts %d tokens, static resources %d tokens", sum.ToolsTotal, strings.Join(parts, ", "),
		sum.PromptsTotal, sum.ResourcesTotal)
}
