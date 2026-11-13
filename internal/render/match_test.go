package render

import (
	"testing"

	"termite/internal/term"
)

// TestOnlyCurrentMatch checks that only the current search match is shown,
// including one that wraps onto the next line.
func TestOnlyCurrentMatch(t *testing.T) {
	ms := []term.Match{
		{Start: term.Pos{Line: 1, Col: 3}, End: term.Pos{Line: 1, Col: 5}},
		{Start: term.Pos{Line: 1, Col: 8}, End: term.Pos{Line: 2, Col: 2}}, // wraps
		{Start: term.Pos{Line: 2, Col: 4}, End: term.Pos{Line: 2, Col: 5}},
	}
	for current := -1; current < len(ms); current++ {
		r := &Renderer{}
		r.view.Matches, r.view.Current = ms, current
		for abs := int64(0); abs <= 3; abs++ {
			for x := range 10 {
				want := current >= 0 && inMatch(ms[current], abs, x)
				if got := r.inCurrentMatch(abs, x); got != want {
					t.Errorf("current %d, line %d col %d: %v, want %v", current, abs, x, got, want)
				}
			}
		}
	}
}

// TestPlainRun checks that a ligature is only drawn over cells that need
// nothing drawn per cell: no cursor, no lines over or under them.
func TestPlainRun(t *testing.T) {
	arrow := func(attr term.Attr) []term.Cell {
		return []term.Cell{{Ch: '-', Attr: attr}, {Ch: '>', Attr: attr}, {Ch: ' '}}
	}
	r := &Renderer{}
	r.curX = 1
	if !r.plainRun(arrow(0), 0, 0, 2, false) {
		t.Error("a plain -> is not drawn as a ligature")
	}
	if r.plainRun(arrow(0), 0, 0, 2, true) {
		t.Error("the cursor on > is lost")
	}
	for _, attr := range []term.Attr{term.Attr(0).WithUnderline(term.ULSingle), term.AttrStrike, term.AttrOverline} {
		if r.plainRun(arrow(attr), 0, 0, 2, false) {
			t.Errorf("attribute %b drops its line", attr)
		}
	}
	r.view.HoverActive = true
	r.view.Hover = term.Match{Start: term.Pos{Line: 0, Col: 1}, End: term.Pos{Line: 0, Col: 2}}
	if r.plainRun(arrow(0), 0, 0, 2, false) {
		t.Error("a hovered link loses its underline")
	}
}
