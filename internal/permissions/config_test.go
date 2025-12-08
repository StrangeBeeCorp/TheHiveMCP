package permissions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefault(t *testing.T) {
	config, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault() failed: %v", err)
	}

	if config.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", config.Version)
	}

	// Default should have read-only permissions
	if !config.IsToolAllowed("search-entities") {
		t.Error("Default should allow search-entities")
	}

	if config.IsToolAllowed("manage-entities") {
		t.Error("Default should not allow manage-entities")
	}

	if config.IsToolAllowed("execute-automation") {
		t.Error("Default should not allow execute-automation")
	}
}

func TestParseYAML(t *testing.T) {
	yamlData := []byte(`
version: "1.0"
permissions:
  tools:
    search-entities:
      allowed: true
    manage-entities:
      allowed: false
  analyzers:
    mode: "allow_list"
    allowed: ["Test1", "Test2"]
  responders:
    mode: "block_list"
    blocked: ["BadResponder"]
`)

	config, err := ParseYAML(yamlData)
	if err != nil {
		t.Fatalf("ParseYAML() failed: %v", err)
	}

	if config.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", config.Version)
	}

	if !config.IsToolAllowed("search-entities") {
		t.Error("search-entities should be allowed")
	}

	if config.IsToolAllowed("manage-entities") {
		t.Error("manage-entities should not be allowed")
	}

	if config.Permissions.Analyzers.Mode != "allow_list" {
		t.Errorf("Expected analyzers mode 'allow_list', got %s", config.Permissions.Analyzers.Mode)
	}

	if len(config.Permissions.Analyzers.Allowed) != 2 {
		t.Errorf("Expected 2 allowed analyzers, got %d", len(config.Permissions.Analyzers.Allowed))
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "permissions.yaml")

	configContent := []byte(`
version: "1.0"
permissions:
  tools:
    search-entities:
      allowed: true
    manage-entities:
      allowed: true
  analyzers:
    mode: "allow_list"
    allowed: ["*"]
  responders:
    mode: "allow_list"
    allowed: ["*"]
`)

	if err := os.WriteFile(configPath, configContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	config, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("LoadFromFile() failed: %v", err)
	}

	if !config.IsToolAllowed("search-entities") {
		t.Error("search-entities should be allowed")
	}

	if !config.IsToolAllowed("manage-entities") {
		t.Error("manage-entities should be allowed")
	}
}

func TestLoadFromFile_InvalidPath(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/path/permissions.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestParseYAML_InvalidYAML(t *testing.T) {
	invalidYAML := []byte(`
this is not: valid: yaml:
`)

	_, err := ParseYAML(invalidYAML)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}
