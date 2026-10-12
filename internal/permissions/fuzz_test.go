package permissions

import (
	"testing"
)

// FuzzPermissionProfile: no profile, however malformed, makes parsing, validation or a permission
// check fail other than by an error; a check gives the same answer twice; a tool the profile does
// not allow is denied, and so is any entity operation when manage-entities is not allowed. `go
// test` runs the seeds; the nightly fuzz gate searches for new inputs.
func FuzzPermissionProfile(f *testing.F) {
	defaults, err := GetDefaultPermissions()
	if err != nil {
		f.Fatal(err)
	}

	f.Add(defaults, "search-entities", "case", "create")
	f.Add([]byte("version: 1\npermissions:\n  tools:\n    search-entities:\n      allowed: true\n"), "search-entities", "alert", "read")
	f.Add([]byte("permissions: [1, 2]\n"), "manage-entities", "task", "delete")
	f.Add([]byte("permissions:\n  analyzers:\n    allowed: [\"*\"]\n    denied: [\"Shodan_*\"]\n"), "execute-automation", "", "")
	f.Add([]byte("{{{"), "", "", "")

	f.Fuzz(func(t *testing.T, profile []byte, tool, entity, operation string) {
		config, err := ParseYAML(profile)
		if err != nil || config == nil {
			return
		}

		if Validate(config) != nil {
			return
		}

		for range 2 {
			checkProfile(t, config, tool, entity, operation)
		}
	})
}

// checkProfile holds a profile's answers to what it says: the same twice, a tool it does not allow
// denied, an entity operation denied while manage-entities is.
func checkProfile(t *testing.T, config *Config, tool, entity, operation string) {
	t.Helper()

	allowed := config.IsToolAllowed(tool)
	if config.IsToolAllowed(tool) != allowed {
		t.Fatalf("IsToolAllowed(%q) changes its answer", tool)
	}

	if granted := config.Permissions.Tools[tool]; allowed && !granted.Allowed {
		t.Fatalf("IsToolAllowed(%q) allows a tool the profile does not", tool)
	}

	op := config.IsEntityOperationAllowed(entity, operation)
	if config.IsEntityOperationAllowed(entity, operation) != op {
		t.Fatalf("IsEntityOperationAllowed(%q, %q) changes its answer", entity, operation)
	}

	if op && !config.IsToolAllowed(toolManageEntities) {
		t.Fatalf("IsEntityOperationAllowed(%q, %q) allows an operation while %s is not allowed", entity, operation, toolManageEntities)
	}

	_ = config.GetToolFilters(tool)
	_ = config.IsAnalyzerAllowed(tool, entity)
	_ = config.IsResponderAllowed(operation)
}
