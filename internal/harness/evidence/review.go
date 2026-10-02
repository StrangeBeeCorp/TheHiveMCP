package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Review is what the adversarial review (the review skill of sb-personal) records about the
// branch: its verdict, the commit it judged, and the base it compared that commit with.
type Review struct {
	Verdict string `json:"verdict"`
	Head    string `json:"verdict_head"`
	// Base is the ref the review compared with (origin/<the pull request's base>), and MergeBase
	// where the change it judged starts: MergeBase..Head is the diff it read.
	Base      string `json:"base"`
	MergeBase string `json:"merge_base"`
}

// Ready reports whether the review lets the change ship.
func (r Review) Ready() bool {
	return slices.Contains([]string{"ready", "ready_with_known_issues"}, r.Verdict)
}

// ReviewPath is where the review skill keeps its state, under the checkout's git directory.
const ReviewPath = "sb-review/state.json"

// ReadReview returns the review state of the checkout whose git directory is gitDir, or nil.
func ReadReview(gitDir string) (*Review, error) {
	return readReview(filepath.Join(gitDir, ReviewPath))
}

func readReview(p string) (*Review, error) {
	data, err := os.ReadFile(p) // #nosec G304 -- the review state of this checkout, or a copy the harness kept
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil //nolint:nilnil // no review is not an error
	}

	if err != nil {
		return nil, fmt.Errorf("read the review state: %w", err)
	}

	var r Review

	err = json.Unmarshal(data, &r)
	if err != nil {
		return nil, fmt.Errorf("decode the review state %s: %w", filepath.Base(p), err)
	}

	return &r, nil
}

// keptPrefix names, under Dir, the copy of a review a push kept: review-<the commit it judged>.json.
const keptPrefix = "review-"

// Keep records the review that let a commit ship. The review state holds one review, and moves on
// to the next branch reviewed in the checkout: the kept copy still covers this change once it is
// rebased onto a newer base (a stacked pull request whose base moved).
func Keep(root string, r Review) error {
	p := filepath.Join(root, Dir, keptPrefix+r.Head+".json")

	err := os.MkdirAll(filepath.Dir(p), dirPerm)
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(p), err)
	}

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the review: %w", err)
	}

	err = os.WriteFile(p, data, filePerm)
	if err != nil {
		return fmt.Errorf("keep the review of %s: %w", short(r.Head), err)
	}

	return nil
}

// Kept returns the reviews earlier pushes kept, in the order of their file names.
func Kept(root string) ([]Review, error) {
	entries, err := os.ReadDir(filepath.Join(root, Dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("list the kept reviews: %w", err)
	}

	var names []string

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), keptPrefix) && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}

	sort.Strings(names)

	reviews := make([]Review, 0, len(names))

	for _, name := range names {
		r, err := readReview(filepath.Join(root, Dir, name))
		if err != nil {
			return nil, err
		}

		if r != nil {
			reviews = append(reviews, *r)
		}
	}

	return reviews, nil
}
