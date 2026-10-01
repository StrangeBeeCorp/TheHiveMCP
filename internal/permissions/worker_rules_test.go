package permissions

import "testing"

// A worker is known by an ID and a name, and the rules may list either, so a
// check must look at both.
func TestWorkerRulesCheckEveryIdentifier(t *testing.T) {
	t.Parallel()

	const (
		workerID   = "d3e4f5"
		workerName = "DeleteCase_1_0"
	)

	tests := []struct {
		name  string
		rules AutomationPermissions
		ids   []string
		want  bool
	}{
		{"block list by name denies the ID", AutomationPermissions{Mode: modeBlockList, Blocked: []string{workerName}}, []string{workerID, workerName}, false},
		{"block list by ID denies the name", AutomationPermissions{Mode: modeBlockList, Blocked: []string{workerID}}, []string{workerID, workerName}, false},
		{"block list naming another worker allows", AutomationPermissions{Mode: modeBlockList, Blocked: []string{"Other_1_0"}}, []string{workerID, workerName}, true},
		{"allow list by name allows the ID", AutomationPermissions{Mode: modeAllowList, Allowed: []string{workerName}}, []string{workerID, workerName}, true},
		{"allow list by ID allows the name", AutomationPermissions{Mode: modeAllowList, Allowed: []string{workerID}}, []string{workerID, workerName}, true},
		{"allow list naming another worker denies", AutomationPermissions{Mode: modeAllowList, Allowed: []string{"Other_1_0"}}, []string{workerID, workerName}, false},
		{"wildcard allows", AutomationPermissions{Mode: modeAllowList, Allowed: []string{"*"}}, []string{workerID}, true},
		{"no identifier denies under a block list", AutomationPermissions{Mode: modeBlockList}, nil, false},
		{"empty identifiers deny under a wildcard", AutomationPermissions{Mode: modeAllowList, Allowed: []string{"*"}}, []string{"", ""}, false},
		{"an empty name is not matched", AutomationPermissions{Mode: modeAllowList, Allowed: []string{""}}, []string{workerID, ""}, false},
		{"unknown mode denies", AutomationPermissions{Mode: "other"}, []string{workerID}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := &Config{Permissions: Section{Analyzers: tt.rules, Responders: tt.rules}}

			if got := config.IsAnalyzerAllowed(tt.ids...); got != tt.want {
				t.Errorf("IsAnalyzerAllowed(%q) = %v, want %v", tt.ids, got, tt.want)
			}

			if got := config.IsResponderAllowed(tt.ids...); got != tt.want {
				t.Errorf("IsResponderAllowed(%q) = %v, want %v", tt.ids, got, tt.want)
			}
		})
	}
}

// When the rules name no worker, every worker gets the same answer and a
// caller may skip looking up the worker's other identifier.
func TestWorkerRulesNameWorkers(t *testing.T) {
	t.Parallel()

	const listed = "Mailer_1_0"

	tests := []struct {
		name  string
		rules AutomationPermissions
		want  bool
	}{
		{"allow list naming workers", AutomationPermissions{Mode: modeAllowList, Allowed: []string{listed}}, true},
		{"block list naming workers", AutomationPermissions{Mode: modeBlockList, Blocked: []string{"DeleteCase_1_0"}}, true},
		{"wildcard", AutomationPermissions{Mode: modeAllowList, Allowed: []string{listed, "*"}}, false},
		{"empty allow list", AutomationPermissions{Mode: modeAllowList}, false},
		{"empty block list", AutomationPermissions{Mode: modeBlockList}, false},
		{"unknown mode", AutomationPermissions{Mode: "other", Allowed: []string{listed}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := &Config{Permissions: Section{Analyzers: tt.rules, Responders: tt.rules}}

			if got := config.AnalyzerRulesNameWorkers(); got != tt.want {
				t.Errorf("AnalyzerRulesNameWorkers() = %v, want %v", got, tt.want)
			}

			if got := config.ResponderRulesNameWorkers(); got != tt.want {
				t.Errorf("ResponderRulesNameWorkers() = %v, want %v", got, tt.want)
			}
		})
	}

	var nilConfig *Config
	if nilConfig.AnalyzerRulesNameWorkers() || nilConfig.ResponderRulesNameWorkers() {
		t.Error("a nil config must not claim to name workers")
	}
}
