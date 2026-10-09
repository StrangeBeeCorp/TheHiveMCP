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

	authContext := GetAuthContext(t)
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
		// No explicit assignee: under per-test-org isolation the admin is not a
		// member of the test's org, so assigning to it 4xxs. TheHive assigns the
		// creating (per-test) user automatically.
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
		Mandatory:   new(false),
	}
}
