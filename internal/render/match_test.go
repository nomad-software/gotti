package render

import (
	"testing"

	"termite/internal/term"
)

// TestMatchCursor checks the forward-only cursor against a linear scan.
func TestMatchCursor(t *testing.T) {
	ms := []term.Match{
		{Start: term.Pos{Line: 1, Col: 3}, End: term.Pos{Line: 1, Col: 5}},
		{Start: term.Pos{Line: 1, Col: 8}, End: term.Pos{Line: 2, Col: 2}}, // spans a wrap
		{Start: term.Pos{Line: 2, Col: 4}, End: term.Pos{Line: 2, Col: 5}},
		{Start: term.Pos{Line: 4, Col: 0}, End: term.Pos{Line: 4, Col: 1}},
	}
	for abs := int64(0); abs <= 5; abs++ {
		mc := newMatchCursor(ms, abs)
		for x := 0; x < 10; x++ {
			want := -1
			for i, m := range ms {
				if inMatch(m, abs, x) {
					want = i
				}
			}
			if got := mc.at(x); got != want {
				t.Errorf("line %d col %d: got %d, want %d", abs, x, got, want)
			}
		}
	}
}
