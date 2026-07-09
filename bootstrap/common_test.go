package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateTheHiveConfig_OrganisationHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		organisation  string
		expectHeader  bool
		expectedValue string
	}{
		{
			name:          "Organisation set adds X-Organisation header",
			organisation:  testOrg,
			expectHeader:  true,
			expectedValue: testOrg,
		},
		{
			name:         "Empty organisation does not add X-Organisation header",
			organisation: "",
			expectHeader: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			creds := &TheHiveCredentials{
				URL: testHiveExampleURL,
				// #nosec G101 -- test fixture credential value, not a real secret
				APIKey:       "valid-api-key",
				Organisation: tt.organisation,
			}

			cfg, err := CreateTheHiveConfig(creds)
			require.NoError(t, err)

			val, exists := cfg.DefaultHeader["X-Organisation"]
			if tt.expectHeader {
				assert.True(t, exists, "X-Organisation header should be present")
				assert.Equal(t, tt.expectedValue, val)
			} else {
				assert.False(t, exists, "X-Organisation header should not be present when organisation is empty")
			}
		})
	}
}

func TestTheHiveCredentials_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		creds   TheHiveCredentials
		wantErr error
	}{
		{
			name: "Valid credentials with API key and no organisation",
			creds: TheHiveCredentials{
				URL:    testHiveExampleURL,
				APIKey: testValidKey,
			},
			wantErr: nil,
		},
		{
			name: "Valid credentials with API key and organisation",
			creds: TheHiveCredentials{
				URL:          testHiveExampleURL,
				APIKey:       testValidKey,
				Organisation: testOrg,
			},
			wantErr: nil,
		},
		{
			name: "Missing URL",
			creds: TheHiveCredentials{
				APIKey: testValidKey,
			},
			wantErr: ErrMissingHiveURL,
		},
		{
			name: "Missing authentication",
			creds: TheHiveCredentials{
				URL: testHiveExampleURL,
			},
			wantErr: ErrMissingAuthentication,
		},
		{
			name: "Dummy API key",
			creds: TheHiveCredentials{
				URL:    testHiveExampleURL,
				APIKey: dummyAPIKey,
			},
			wantErr: ErrInvalidAPIKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.creds.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
