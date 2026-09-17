package emit

import (
	"slices"
	"testing"
)

// TestCommentLinesSplit covers a rule whose text spans several lines: a CEL
// expression written across two, or the source cel-go prints under a parse
// error. Emitted whole, everything after the first newline leaves the comment
// and lands in the generated module as code.
func TestCommentLinesSplit(t *testing.T) {
	got := CommentLines([]string{"cel[x]: this.a > 0 &&\n  this.b > 0", "plain"})
	want := []string{"cel[x]: this.a > 0 &&", "  this.b > 0", "plain"}
	if !slices.Equal(got, want) {
		t.Errorf("CommentLines() = %q, want %q", got, want)
	}
}
