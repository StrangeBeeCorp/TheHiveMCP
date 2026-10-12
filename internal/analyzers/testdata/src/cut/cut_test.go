package cut

import "testing"

// A test builds pages to check them: it is no result a model reads.
func TestAPartialPageInATest(t *testing.T) {
	_ = Page{Items: []int{1}}
}
