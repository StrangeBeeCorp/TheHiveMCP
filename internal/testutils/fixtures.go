package testutils

import (
	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

func MockInputCase() *thehive.InputCreateCase {
	return &thehive.InputCreateCase{
		Title:       "Test Case",
		Description: "This is a test case",
		Severity:    thehive.PtrInt32(2),
		StartDate:   thehive.PtrInt64(1609459200),
		EndDate:     thehive.PtrInt64(1609545600),
		Tags:        []string{"test", "case"},
		Flag:        thehive.PtrBool(true),
		Tlp:         thehive.PtrInt32(2),
		Pap:         thehive.PtrInt32(2),
		Status:      thehive.PtrString("InProgress"),
		Summary:     thehive.PtrString("This is a summary"),
		Assignee:    thehive.PtrString("admin@thehive.local"),
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
				Description: thehive.PtrString("This is a test task"),
				Status:      thehive.PtrString("Waiting"),
				Flag:        thehive.PtrBool(true),
				StartDate:   thehive.PtrInt64(1609459200),
				EndDate:     thehive.PtrInt64(1609545600),
				Assignee:    thehive.PtrString("admin@thehive.local"),
			},
		},
	}
}

func MockInputAlert() *thehive.InputCreateAlert {
	return &thehive.InputCreateAlert{
		Title:       "Test Alert",
		Type:        "test",
		Description: "This is a test alert",
		Severity:    thehive.PtrInt32(2),
		Tags:        []string{"test"},
		Flag:        thehive.PtrBool(true),
		Tlp:         thehive.PtrInt32(2),
		Pap:         thehive.PtrInt32(2),
		Source:      "test",
		SourceRef:   "test",
		Summary:     thehive.PtrString("This is a summary"),
		Assignee:    thehive.PtrString("admin@thehive.local"),
	}
}

func MockInputUser() *thehive.InputCreateUser {
	return &thehive.InputCreateUser{
		Login:        "testuser",
		Name:         "Test User",
		Profile:      "admin",
		Email:        thehive.PtrString("testuser@thehive.local"),
		Password:     thehive.PtrString("password123"),
		Organisation: thehive.PtrString("test-org"),
	}
}

func MockInputOrganisation() *thehive.InputCreateOrganisation {
	return &thehive.InputCreateOrganisation{
		Name:           "Test Organisation",
		Description:    "This is a test organisation",
		TaskRule:       thehive.PtrString("BacklogTasks"),
		ObservableRule: thehive.PtrString("ObservableStrictTLP"),
		Locked:         thehive.PtrBool(false),
	}
}

func MockInputUserOrganisation() []thehive.InputUserOrganisation {
	return []thehive.InputUserOrganisation{
		{
			Organisation: "test-org",
			Profile:      "admin",
			Default:      thehive.PtrBool(true),
		},
	}
}

// MockInputCaseTemplate returns a valid InputCaseTemplate for testing.
func MockInputCaseTemplate() *thehive.InputCreateCaseTemplate {
	return &thehive.InputCreateCaseTemplate{
		Name:        "Test Template",
		Description: thehive.PtrString("A test case template"),
		Tags:        []string{"test", "template"},
		Flag:        thehive.PtrBool(false),
	}
}

// MockInputTask returns a valid InputTask for testing.
func MockInputTask() *thehive.InputCreateTask {
	return &thehive.InputCreateTask{
		Title:       "Test Task",
		Description: thehive.PtrString("This is a test task"),
		Status:      thehive.PtrString("Waiting"),
		Flag:        thehive.PtrBool(true),
		StartDate:   thehive.PtrInt64(1609459200),
		EndDate:     thehive.PtrInt64(1609545600),
		Assignee:    thehive.PtrString("admin@thehive.local"),
		Mandatory:   thehive.PtrBool(false),
	}
}

// MockInputAnalyzerJob returns a valid InputJob for testing Cortex analyzer functionality.
func MockInputAnalyzerJob() *thehive.InputJob {
	job := thehive.NewInputJob("file_hash", "cortex-1", "artifact-123")
	job.SetParameters(map[string]interface{}{
		"timeout": 300,
		"config": map[string]interface{}{
			"check_tlp": true,
		},
	})

	return job
}

// MockInputObservable returns a valid InputObservable for testing.
func MockInputObservable() *thehive.InputCreateObservable {
	observable := thehive.NewInputCreateObservable("domain")
	observable.SetData(thehive.StringAsInputObservableData(thehive.PtrString("example.com")))
	observable.SetMessage("Suspicious domain observed")
	observable.SetIoc(true)
	observable.SetTlp(2)
	observable.SetPap(1)
	observable.SetSighted(false)
	observable.SetTags([]string{"malware", "phishing"})

	return observable
}
