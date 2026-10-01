package hooks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

// Check is the outcome of check-fast.
type Check struct {
	// Red is set when a gate failed.
	Red bool `json:"red"`
	// Incomplete is set when a gate could not run: a stated limit, not a failure of the agent's.
	Incomplete bool `json:"incomplete"`
	// Report says what failed, for the agent to act on.
	Report string `json:"report"`
}

// Checker runs check-fast. static restricts it to the gates that read files without building
// them: the Tester's tests fail on purpose, and may not compile until the Coder's turn.
type Checker func(ctx context.Context, static bool) (Check, error)

// Action is what the Stop hook tells Claude Code after check-fast.
type Action struct {
	// Block sends Text back to the agent, which keeps working; otherwise Text is a note.
	Block bool
	Text  string
}

// Decide applies the red-round rule. Green lets the agent stop. Red sends an enforced agent back
// to work, and after max red rounds it may stop only once it has written BLOCKED.md. The main
// session is told, never held back: a person is there.
func Decide(report string, red bool, rounds, maxRounds int, blockedWritten, enforce bool) Action {
	switch {
	case !red:
		return Action{}
	case !enforce:
		return Action{Text: "check-fast is red:\n" + report}
	case rounds < maxRounds:
		return Action{Block: true, Text: fmt.Sprintf("check-fast is red (round %d of %d). Fix what it reports before you stop; "+
			"never weaken a test or a gate:\n%s", rounds, maxRounds, report)}
	case blockedWritten:
		return Action{Text: fmt.Sprintf("stopped BLOCKED after %d red rounds: see %s", rounds, policy.BlockedFile)}
	}

	return Action{Block: true, Text: fmt.Sprintf("%d red rounds: stop trying. Write %s with what you tried, what still fails "+
		"(below) and what you need to go on, then stop.\n%s", rounds, policy.BlockedFile, report)}
}

func (a Action) output() Output {
	switch {
	case a.Text == "":
		return Output{}
	case a.Block:
		return block(a.Text)
	}

	return note(a.Text)
}

// Stopper is what the Stop hook needs from the harness.
type Stopper struct {
	Root      string
	MaxRounds int
	Check     Checker
	// Fingerprint identifies the state of the checkout (gitx.Fingerprint).
	Fingerprint func(ctx context.Context) (string, error)
}

// Stop runs check-fast when an agent stops (Stop, SubagentStop) and applies the red-round rule.
// A checkout that has not changed since the last run reuses its result, so stopping again without
// a change is another red round, not another wait.
func (s Stopper) Stop(ctx context.Context, in Input) Output {
	role := in.Role()
	if role.ReadOnly() {
		return Output{}
	}

	fp, err := s.Fingerprint(ctx)
	if err != nil {
		return note("harness: check-fast could not read the checkout: " + err.Error())
	}

	var out Output

	err = update(s.Root, func(st *State) error {
		c, err := cached(ctx, st, fp, role == policy.Tester, s.Check)
		if err != nil {
			return err
		}

		out = s.apply(st, in, c)

		return nil
	})
	if err != nil {
		return note("harness: check-fast could not run: " + err.Error())
	}

	return out
}

// apply counts the red round of the agent and decides.
func (s Stopper) apply(st *State, in Input, c Check) Output {
	actor := in.Actor()

	if !c.Red {
		delete(st.Rounds, actor)
		delete(st.RedSince, actor)
		// Green: whatever blocked the task is over.
		_ = os.Remove(filepath.Join(s.Root, policy.BlockedFile))

		if c.Incomplete {
			return note("check-fast could not run every gate, so this is not a pass:\n" + c.Report)
		}

		return Output{}
	}

	enforce := in.Role() != policy.Main
	rounds := 0

	if enforce {
		rounds = st.Rounds[actor] + 1
		st.Rounds[actor] = rounds

		if rounds == 1 {
			st.RedSince[actor] = time.Now()
		}
	}

	a := Decide(c.Report, true, rounds, s.MaxRounds, blockedSince(s.Root, st.RedSince[actor]), enforce)
	if enforce && !a.Block {
		delete(st.Rounds, actor)
		delete(st.RedSince, actor)
	}

	return a.output()
}

func cached(ctx context.Context, st *State, fp string, static bool, check Checker) (Check, error) {
	if st.Last != nil && st.Last.Fingerprint == fp && st.Last.Static == static {
		return st.Last.Check, nil
	}

	c, err := check(ctx, static)
	if err != nil {
		return Check{}, err
	}

	st.Last = &Last{Fingerprint: fp, Static: static, Check: c, At: time.Now()}

	return c, nil
}

// blockedSince reports whether BLOCKED.md was written during the current red streak: a report
// left by an earlier task or agent does not count.
func blockedSince(root string, since time.Time) bool {
	info, err := os.Stat(filepath.Join(root, policy.BlockedFile))
	return err == nil && info.Size() > 0 && !info.ModTime().Before(since)
}
