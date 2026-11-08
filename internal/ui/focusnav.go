package ui

type rect struct{ x, y, w, h float64 }

// navTolerance absorbs splitter handles and rounding: edges closer than
// this count as touching, and overlaps smaller than it don't count.
const navTolerance = 6

// neighbour picks the pane to move to from cur in direction (dx, dy), as
// tmux and vim do: only panes in the nearest column (or row) on that side
// that genuinely overlap cur are considered; the one spanning probe (the
// cursor's position across the direction of travel) wins, otherwise the one
// overlapping cur the most, then the top- or left-most. It returns an index
// into cands, or -1.
func neighbour(cur rect, probe float64, cands []rect, dx, dy int) int {
	// Work in (along, across) coordinates: along is the direction of travel.
	type span struct{ a0, a1, c0, c1 float64 }
	conv := func(r rect) span {
		s := span{r.x, r.x + r.w, r.y, r.y + r.h}
		if dy != 0 {
			s = span{r.y, r.y + r.h, r.x, r.x + r.w}
		}
		if dx < 0 || dy < 0 { // mirror so travel is always towards +along
			s.a0, s.a1 = -s.a1, -s.a0
		}
		return s
	}
	c := conv(cur)
	best, bestGap, bestOverlap, bestHit := -1, 0.0, 0.0, false
	for i, r := range cands {
		s := conv(r)
		gap := s.a0 - c.a1
		if gap < -navTolerance {
			continue // not on that side
		}
		overlap := min(s.c1, c.c1) - max(s.c0, c.c0)
		if overlap <= navTolerance {
			continue // only touching at a corner or edge
		}
		hit := probe >= s.c0 && probe < s.c1
		better := false
		switch {
		case best < 0:
			better = true
		case gap < bestGap-navTolerance:
			better = true // a nearer column always wins
		case gap > bestGap+navTolerance:
			better = false
		case hit != bestHit:
			better = hit
		case overlap != bestOverlap:
			better = overlap > bestOverlap
		default:
			better = s.c0 < conv(cands[best]).c0
		}
		if better {
			best, bestGap, bestOverlap, bestHit = i, gap, overlap, hit
		}
	}
	return best
}
