package execute_automation

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
)

// errEmptyAnswer stands for the decode error of an answer with no body.
var errEmptyAnswer = errors.New("TheHive answered without a body")

// maxAnswerBytes bounds what is read of an answer to look for its ID.
const maxAnswerBytes = 1 << 20

// closeBody closes what is left of an API response once the handler is done.
func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

// answerBody reads the body of an answer the client already consumed (it puts
// the bytes back in resp.Body, so they can be read again).
func answerBody(resp *http.Response) []byte {
	if resp == nil || resp.Body == nil {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxAnswerBytes))

	return body
}

// isEmptyAnswer reports whether a 2xx answer carries nothing: no action or job
// came back to read, which is not a success (SIL-001).
func isEmptyAnswer(resp *http.Response) bool {
	return len(bytes.TrimSpace(answerBody(resp))) == 0
}

// createdID reads the "_id" of an answer the client could not decode. Only a
// string that looks like a TheHive ID is returned: anything else is entity
// text and stays out of the error (UNT-002).
func createdID(resp *http.Response) string {
	var answer struct {
		ID any `json:"_id"`
	}

	if json.Unmarshal(answerBody(resp), &answer) != nil {
		return ""
	}

	id, isString := answer.ID.(string)
	if !isString || !automationEntityIDPattern.MatchString(id) {
		return ""
	}

	return id
}

// acceptedButUnreadable is the error for a 2xx TheHive answered to a run that
// the client could not decode, or that came back empty. TheHive created the job
// or the action and the Cortex runs it: telling the model it failed would make
// it run it a second time. The decode error and the body stay out of the
// message: they can quote entity text, so they go to the log only. The one
// thing taken from the answer is a validated ID.
func acceptedButUnreadable(worker, entity, statusOperation string, err error, resp *http.Response) *tools.ToolError {
	slog.Warn("TheHive accepted a worker run the client could not read",
		"worker", worker, "status", resp.StatusCode, "error", err)

	toolErr := tools.NewToolErrorf("TheHive accepted the request to run the %s (HTTP %d) but its answer could not be read", worker, resp.StatusCode)

	hint := "The " + entity + " was created and runs: do not run it again. "
	if id := createdID(resp); id != "" {
		hint += "Its ID is " + id + ": read it with " + statusOperation + ", or with search-entities (entity-type " + entity + ")."
	} else {
		hint += "Read it with search-entities (entity-type " + entity + ", newest first), or with " + statusOperation + " once you have its ID."
	}

	return toolErr.Hint(hint)
}
