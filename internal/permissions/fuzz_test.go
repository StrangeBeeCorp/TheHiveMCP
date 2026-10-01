package permissions

import (
	"testing"
)

// FuzzPermissionProfile: no profile, however malformed, makes parsing, validation or a permission
// check fail other than by an error, and a check gives the same answer twice. `go test` runs the
// seeds; the nightly fuzz gate searches for new inputs.
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
			allowed := config.IsToolAllowed(tool)
			if config.IsToolAllowed(tool) != allowed {
				t.Fatalf("IsToolAllowed(%q) changes its answer", tool)
			}

			op := config.IsEntityOperationAllowed(entity, operation)
			if config.IsEntityOperationAllowed(entity, operation) != op {
				t.Fatalf("IsEntityOperationAllowed(%q, %q) changes its answer", entity, operation)
			}

			_ = config.GetToolFilters(tool)
			_ = config.IsAnalyzerAllowed(tool, entity)
			_ = config.IsResponderAllowed(operation)
		}
	})
}
