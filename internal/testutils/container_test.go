package testutils

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The integration matrix runs TheHive 5.5 and 5.6, which disagree on how they
// reject a duplicate organisation. Matching only 5.6's 409 made the 5.5 leg
// fail whenever listOrganisation ran against a not-yet-refreshed index and the
// create that followed found the row already present.
func TestOrganisationAlreadyExists(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		status int
		body   string
		want   bool
	}{
		"5.6 answers conflict":       {http.StatusConflict, "", true},
		"5.5 answers bad request":    {http.StatusBadRequest, `{"type":"CreateError","message":"Organisation already exists"}`, true},
		"a genuine bad request":      {http.StatusBadRequest, `{"type":"BadRequest","message":"invalid organisation name"}`, false},
		"a server error stays fatal": {http.StatusInternalServerError, "boom", false},
		"created is not existence":   {http.StatusCreated, "", false},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.want, organisationAlreadyExists(testCase.status, testCase.body))
		})
	}
}
