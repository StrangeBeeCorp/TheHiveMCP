package manage

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/StrangeBeeCorp/thehive4go/thehive"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// observableMultiStatus is TheHive's 207 answer to an observable create in
// which at least one value failed: the observables it did create, and one
// entry per refused value ({type, message, object}). The client is generated
// for the 201 list only, so it fails to decode this body and reports an error
// for a create that may have succeeded in part.
type observableMultiStatus struct {
	Success []FilteredOutputObservable `json:"success"`
	Failure []map[string]any           `json:"failure"`
}

// errorTypePattern is the shape of a TheHive error type ("CreateError",
// "BadRequest"). Only a type of that shape is quoted in an error message, which
// reaches the model unwrapped.
var errorTypePattern = regexp.MustCompile(`^[A-Za-z]{1,64}$`)

// hintCreateError tells the model what TheHive's duplicate refusal means.
const hintCreateError = "A CreateError means the observable already exists in this case or alert: search its observables instead of creating it again"

func isSuccessStatus(resp *http.Response) bool {
	return resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300
}

// observableMultiStatusResult reads the body of a 207 from err. A create that
// made at least one observable is a success that lists the refused values; one
// that made none is an error.
func observableMultiStatusResult(err error) (EntityResult, error) {
	var (
		apiErr *thehive.GenericOpenAPIError
		status observableMultiStatus
	)

	if !errors.As(err, &apiErr) || json.Unmarshal(apiErr.Body(), &status) != nil {
		return EntityResult{}, tools.NewToolError("TheHive answered the observable create with a partial result this server could not read").Cause(err).
			Hint("Some values may have been created: search the parent's observables before creating them again")
	}

	if len(status.Success) == 0 {
		return EntityResult{}, tools.NewToolErrorf("TheHive created no observable: it refused all %d value(s)%s",
			len(status.Failure), failureTypesSuffix(status.Failure)).
			Hint(hintCreateError).
			Hint("Any other type means TheHive rejected the value itself: check it against dataType before retrying").
			Schema("observable", "create")
	}

	failures := make([]utils.UntrustedSubtree, len(status.Failure))
	for i, failure := range status.Failure {
		failures[i] = utils.UntrustedSubtree(failure)
	}

	return EntityResult{
		CreateObservableResult: newPartialCreateObservableResult(status.Success, failures),
	}, nil
}

// failureTypesSuffix lists the distinct error types of the refused values, as
// " (CreateError, BadRequest)", or nothing when none has a well-formed type.
func failureTypesSuffix(failures []map[string]any) string {
	var kinds []string

	for _, failure := range failures {
		kind, _ := failure["type"].(string)
		if errorTypePattern.MatchString(kind) && !slices.Contains(kinds, kind) {
			kinds = append(kinds, kind)
		}
	}

	if len(kinds) == 0 {
		return ""
	}

	return " (" + strings.Join(kinds, ", ") + ")"
}
