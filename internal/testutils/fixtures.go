package testutils

import (
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"
)

// CreateCaseWithTLP creates a case with the given title and TLP via the raw
// TheHive API and asserts it succeeded. Shared by the manage and
// execute_automation scope-enforcement tests.
func CreateCaseWithTLP(t *testing.T, hiveClient *thehive.APIClient, title string, tlp int32) *thehive.OutputCase {
	t.Helper()

	authContext := GetAuthContext(NewHiveTestConfig())
	testCase := MockInputCase()
	testCase.Title = title
	testCase.Tlp = &tlp

	createdCase, httpResp, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	closeResponse(httpResp)
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	return createdCase
}

// MockInputCase returns a valid InputCreateCase (with a task and custom fields)
// for testing.
func MockInputCase() *thehive.InputCreateCase {
	return &thehive.InputCreateCase{
		Title:       "Test Case",
		Description: "This is a test case",
		Severity:    thehive.PtrInt32(2),
		StartDate:   thehive.PtrInt64(1609459200),
		EndDate:     thehive.PtrInt64(1609545600),
		Tags:        []string{testTag, "case"},
		Flag:        new(true),
		Tlp:         thehive.PtrInt32(2),
		Pap:         thehive.PtrInt32(2),
		Status:      new("InProgress"),
		Summary:     new("This is a summary"),
		Assignee:    new(DefaultAdminUser),
		CustomFields: &thehive.InputCreateAlertCustomFields{
			ArrayOfInputCustomFieldValue: &[]thehive.InputCustomFieldValue{
				{
					Name:  "custom_field_1",
					Value: "Custom Value 1",
				},
				{
					Name:  "custom_field_2",
					Value: 42,
				},
			},
		},
		Tasks: []thehive.InputCreateTask{
			{
				Title:       "Test Task",
				Description: new("This is a test task"),
				Status:      new("Waiting"),
				Flag:        new(true),
				StartDate:   thehive.PtrInt64(1609459200),
				EndDate:     thehive.PtrInt64(1609545600),
				Assignee:    new(DefaultAdminUser),
			},
		},
	}
}

// MockInputAlert returns a valid InputCreateAlert for testing.
func MockInputAlert() *thehive.InputCreateAlert {
	return &thehive.InputCreateAlert{
		Title:       "Test Alert",
		Type:        testTag,
		Description: "This is a test alert",
		Severity:    thehive.PtrInt32(2),
		Tags:        []string{testTag},
		Flag:        new(true),
		Tlp:         thehive.PtrInt32(2),
		Pap:         thehive.PtrInt32(2),
		Source:      testTag,
		SourceRef:   testTag,
		Summary:     new("This is a summary"),
		Assignee:    new(DefaultAdminUser),
	}
}

// MockInputUser returns a valid InputCreateUser for testing.
func MockInputUser() *thehive.InputCreateUser {
	return &thehive.InputCreateUser{
		Login:        "testuser",
		Name:         "Test User",
		Profile:      adminOrg,
		Email:        new("testuser@thehive.local"),
		Password:     new("password123"),
		Organisation: new("test-org"),
	}
}

// MockInputOrganisation returns a valid InputCreateOrganisation for testing.
func MockInputOrganisation() *thehive.InputCreateOrganisation {
	return &thehive.InputCreateOrganisation{
		Name:           "Test Organisation",
		Description:    "This is a test organisation",
		TaskRule:       new("BacklogTasks"),
		ObservableRule: new("ObservableStrictTLP"),
		Locked:         new(false),
	}
}

// MockInputUserOrganisation returns a valid slice of InputUserOrganisation for testing.
func MockInputUserOrganisation() []thehive.InputUserOrganisation {
	return []thehive.InputUserOrganisation{
		{
			Organisation: "test-org",
			Profile:      adminOrg,
			Default:      new(true),
		},
	}
}

// MockInputCaseTemplate returns a valid InputCaseTemplate for testing.
func MockInputCaseTemplate() *thehive.InputCreateCaseTemplate {
	return &thehive.InputCreateCaseTemplate{
		Name:        "Test Template",
		Description: new("A test case template"),
		Tags:        []string{testTag, "template"},
		Flag:        new(false),
	}
}

// MockInputTask returns a valid InputTask for testing.
func MockInputTask() *thehive.InputCreateTask {
	return &thehive.InputCreateTask{
		Title:       "Test Task",
		Description: new("This is a test task"),
		Status:      new("Waiting"),
		Flag:        new(true),
		StartDate:   thehive.PtrInt64(1609459200),
		EndDate:     thehive.PtrInt64(1609545600),
		Assignee:    new(DefaultAdminUser),
		Mandatory:   new(false),
	}
}

// MockInputAnalyzerJob returns a valid InputJob for testing Cortex analyzer functionality.
func MockInputAnalyzerJob() *thehive.InputJob {
	job := thehive.NewInputJob("file_hash", "cortex-1", "artifact-123")
	job.SetParameters(map[string]any{
		"timeout": 300,
		"config": map[string]any{
			"check_tlp": true,
		},
	})

	return job
}

// MockInputObservable returns a valid InputObservable for testing.
func MockInputObservable() *thehive.InputCreateObservable {
	observable := thehive.NewInputCreateObservable("domain")
	observable.SetData(thehive.StringAsInputObservableData(new("example.com")))
	observable.SetMessage("Suspicious domain observed")
	observable.SetIoc(true)
	observable.SetTlp(2)
	observable.SetPap(1)
	observable.SetSighted(false)
	observable.SetTags([]string{"malware", "phishing"})

	return observable
}
