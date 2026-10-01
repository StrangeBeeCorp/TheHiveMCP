package permissions

import (
	"path/filepath"
	"strings"
	"testing"
)

// Regression: a misspelt key was dropped without error, and what it named fell
// back to its most permissive default. A typo in entity_permissions allowed
// every operation on every entity; a typo in filters removed the filters.
func TestParseYAMLRejectsUnknownKeys(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"misspelt entity_permissions": `version: "1.0"
permissions:
  tools:
    manage-entities:
      allowed: true
      entity_permission:
        case:
          delete: false
`,
		"misspelt filters": `version: "1.0"
permissions:
  tools:
    search-entities:
      allowed: true
      filter:
        _lte: {_field: "tlp", _value: 2}
`,
		"misspelt blocked": `version: "1.0"
permissions:
  responders:
    mode: "block_list"
    block: ["DeleteCase_1_0"]
`,
		"misspelt operation": `version: "1.0"
permissions:
  tools:
    manage-entities:
      allowed: true
      entity_permissions:
        case:
          delet: true
`,
		"misspelt section": `version: "1.0"
permission:
  tools: {}
`,
	}

	for name, doc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseYAML([]byte(doc))
			if err == nil || !strings.Contains(err.Error(), "not found") {
				t.Fatalf("ParseYAML() error = %v, want an unknown-field error", err)
			}
		})
	}
}

// An empty document still parses, so Validate can say what is missing.
func TestParseYAMLAcceptsAnEmptyDocument(t *testing.T) {
	t.Parallel()

	config, err := ParseYAML(nil)
	if err != nil {
		t.Fatalf("ParseYAML(nil) error = %v", err)
	}

	err = Validate(config)
	if err == nil || !strings.Contains(err.Error(), "version is required") {
		t.Fatalf("Validate() error = %v, want the missing version", err)
	}
}

// Strict parsing must not break a permissions file the project ships.
func TestShippedPermissionFilesParseStrictly(t *testing.T) {
	t.Parallel()

	_, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault() error = %v", err)
	}

	examples, err := filepath.Glob("../../docs/reference/permissions-examples/*.yaml")
	if err != nil || len(examples) == 0 {
		t.Fatalf("no shipped permission examples found (err = %v)", err)
	}

	for _, path := range append(examples, "../testutils/testdata/admin_permissions.yaml") {
		_, err := LoadFromFile(path)
		if err != nil {
			t.Errorf("LoadFromFile(%s) error = %v", path, err)
		}
	}
}
