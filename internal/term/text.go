package term

import (
	"bytes"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Pos is an absolute position: Line is Buffer.Evicted + logical index, so
// positions stay valid while output scrolls.
type Pos struct {
	Line int64
	Col  int
}

// Less orders positions.
func (a Pos) Less(b Pos) bool {
	return a.Line < b.Line || (a.Line == b.Line && a.Col < b.Col)
}

// AbsToLogical converts an absolute line to a logical index (or -1).
func (b *Buffer) AbsToLogical(abs int64) int {
	i := abs - b.Evicted
	if i < 0 || i >= int64(b.count) {
		return -1
	}
	return int(i)
}

// ScreenTopAbs returns the absolute line number of screen row 0.
func (b *Buffer) ScreenTopAbs() int64 { return b.Evicted + int64(b.count-b.Rows) }

func appendLine(buf []byte, line []Cell, from, to int) []byte {
	for x := from; x < to && x < len(line); x++ {
		c := &line[x]
		switch {
		case c.Attr&(AttrWideCont|AttrWrapPad) != 0:
		case c.Ch == 0:
			buf = append(buf, ' ')
		case c.Attr&AttrCluster != 0:
			buf = append(buf, ClusterString(uint32(c.Ch))...)
		default:
			buf = utf8.AppendRune(buf, c.Ch)
		}
	}
	return buf
}

// TextRange extracts text between two absolute positions (inclusive of
// start, exclusive of end col). Soft-wrapped lines are joined; hard line
// ends become newlines. Rectangular selections use block mode.
func (b *Buffer) TextRange(start, end Pos, block bool) string {
	if end.Less(start) {
		start, end = end, start
	}
	var buf []byte
	for abs := start.Line; abs <= end.Line; abs++ {
		i := b.AbsToLogical(abs)
		if i < 0 {
			continue
		}
		line := b.LogicalLine(i)
		from, to := 0, b.Cols
		if block {
			from, to = min(start.Col, end.Col), max(start.Col, end.Col)
		} else {
			if abs == start.Line {
				from = start.Col
			}
			if abs == end.Line {
				to = end.Col
			}
		}
		mark := len(buf)
		buf = appendLine(buf, line, from, to)
		wrapped := b.Wrapped(i) && !block
		if !wrapped || abs == end.Line {
			buf = buf[:mark+len(bytes.TrimRight(buf[mark:], " "))]
		}
		if abs != end.Line && !wrapped {
			buf = append(buf, '\n')
		}
	}
	return string(buf)
}

func isWordRune(r rune, extra string) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(extra, r)
}

// WordAt returns the extent [start,end) of the word under p, following
// soft wraps. extra lists punctuation treated as part of words.
func (b *Buffer) WordAt(p Pos, extra string) (Pos, Pos) {
	i := b.AbsToLogical(p.Line)
	if i < 0 || p.Col < 0 || p.Col >= b.Cols {
		return p, p
	}
	runeAt := func(line []Cell, x int) rune {
		c := &line[x]
		if c.Attr&AttrWideCont != 0 && x > 0 {
			c = &line[x-1]
		}
		if c.Attr&AttrCluster != 0 {
			r, _ := utf8.DecodeRuneInString(ClusterString(uint32(c.Ch)))
			return r
		}
		return c.Ch
	}
	line := b.LogicalLine(i)
	if !isWordRune(runeAt(line, p.Col), extra) {
		return p, Pos{p.Line, p.Col + 1}
	}
	s, e := p, p
	// backwards
	for {
		if s.Col > 0 {
			if !isWordRune(runeAt(b.LogicalLine(b.AbsToLogical(s.Line)), s.Col-1), extra) {
				break
			}
			s.Col--
			continue
		}
		pi := b.AbsToLogical(s.Line - 1)
		if pi < 0 || !b.Wrapped(pi) || !isWordRune(runeAt(b.LogicalLine(pi), b.Cols-1), extra) {
			break
		}
		s = Pos{s.Line - 1, b.Cols - 1}
	}
	// forwards
	for {
		li := b.AbsToLogical(e.Line)
		if e.Col+1 < b.Cols {
			if !isWordRune(runeAt(b.LogicalLine(li), e.Col+1), extra) {
				break
			}
			e.Col++
			continue
		}
		ni := b.AbsToLogical(e.Line + 1)
		if !b.Wrapped(li) || ni < 0 || !isWordRune(runeAt(b.LogicalLine(ni), 0), extra) {
			break
		}
		e = Pos{e.Line + 1, 0}
	}
	return s, Pos{e.Line, e.Col + 1}
}

// LineExtent returns the full logical (soft-wrap joined) line around p.
func (b *Buffer) LineExtent(p Pos) (Pos, Pos) {
	s, e := p.Line, p.Line
	for {
		pi := b.AbsToLogical(s - 1)
		if pi < 0 || !b.Wrapped(pi) {
			break
		}
		s--
	}
	for {
		li := b.AbsToLogical(e)
		if li < 0 || !b.Wrapped(li) || b.AbsToLogical(e+1) < 0 {
			break
		}
		e++
	}
	return Pos{s, 0}, Pos{e, b.Cols}
}

// Match is a search hit spanning [Start, End).
type Match struct{ Start, End Pos }

// paragraph joins soft-wrapped lines starting at logical index i and
// returns the text, a mapping from byte offset to position and the index
// of the next paragraph. The text and positions are appended to text[:0]
// and pos[:0], so callers can reuse them.
func (b *Buffer) paragraph(i int, text []byte, pos []Pos) ([]byte, []Pos, int) {
	text, pos = text[:0], pos[:0]
	j := i
	for ; j < b.count; j++ {
		line := b.LogicalLine(j)
		abs := b.Evicted + int64(j)
		for x := range line {
			n := len(text)
			text = appendLine(text, line[x:x+1], 0, 1)
			for range len(text) - n {
				pos = append(pos, Pos{abs, x})
			}
		}
		if !b.Wrapped(j) {
			j++
			break
		}
	}
	pos = append(pos, Pos{b.Evicted + int64(j-1), b.Cols})
	return text, pos, j
}

// Search finds all case-insensitive (unless caseSensitive) occurrences of
// query in the buffer, oldest first.
func (b *Buffer) Search(query string, caseSensitive bool) []Match {
	if query == "" {
		return nil
	}
	if !caseSensitive {
		query = strings.ToLower(query)
	}
	q := []byte(query)
	var out []Match
	var text, lower []byte
	var pos, lowerPos []Pos
	for i := 0; i < b.count; {
		var next int
		text, pos, next = b.paragraph(i, text, pos)
		hay, hayPos := text, pos
		if !caseSensitive {
			lower = appendLower(lower[:0], text)
			hay = lower
			if len(lower) != len(text) {
				// Lowercasing changed the length (as for the Kelvin sign):
				// map each byte of the lowered text to its cell again.
				lower, lowerPos = appendLowerMapped(lower[:0], lowerPos[:0], text, pos)
				hay, hayPos = lower, lowerPos
			}
		}
		off := 0
		for {
			k := bytes.Index(hay[off:], q)
			if k < 0 {
				break
			}
			s := off + k
			e := s + len(query)
			endPos := hayPos[e-1]
			endPos.Col++
			out = append(out, Match{hayPos[s], endPos})
			off = e
		}
		i = next
	}
	return out
}

// appendLowerMapped appends s lower-cased to dst rune by rune, and to
// dstPos the position of each byte written: that of the byte of s it came
// from, from pos.
func appendLowerMapped(dst []byte, dstPos []Pos, s []byte, pos []Pos) ([]byte, []Pos) {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRune(s[i:])
		n := len(dst)
		dst = utf8.AppendRune(dst, unicode.ToLower(r))
		for range len(dst) - n {
			dstPos = append(dstPos, pos[i])
		}
		i += size
	}
	return dst, dstPos
}

// appendLower appends s lower-cased to dst, with an ASCII fast path.
func appendLower(dst, s []byte) []byte {
	for i, c := range s {
		if c >= utf8.RuneSelf {
			return append(dst, bytes.ToLower(s[i:])...)
		}
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		dst = append(dst, c)
	}
	return dst
}

var urlRe = regexp.MustCompile(`(?:https?|ftp|file)://[^\s<>"'\x60]+|mailto:[^\s<>"']+|www\.[^\s<>"']+`)

// URLAt returns a plain-text URL under p, with its extent.
func (b *Buffer) URLAt(p Pos) (string, Match, bool) {
	ls, _ := b.LineExtent(p)
	i := b.AbsToLogical(ls.Line)
	if i < 0 {
		return "", Match{}, false
	}
	t, pos, _ := b.paragraph(i, nil, nil)
	text := string(t)
	for _, m := range urlRe.FindAllStringIndex(text, -1) {
		s, e := m[0], m[1]
		// trim trailing punctuation that is rarely part of a URL
		for e > s && strings.ContainsRune(".,;:!?)]}'", rune(text[e-1])) {
			if text[e-1] == ')' && strings.Count(text[s:e], "(") >= strings.Count(text[s:e], ")") {
				break
			}
			e--
		}
		start, end := pos[s], pos[e-1]
		end.Col++
		if !p.Less(start) && p.Less(end) {
			u := text[s:e]
			if strings.HasPrefix(u, "www.") {
				u = "https://" + u
			}
			return u, Match{start, end}, true
		}
	}
	return "", Match{}, false
}

// LinkAt returns the OSC 8 hyperlink at screen-independent position p.
func (b *Buffer) LinkAt(p Pos) (Link, bool) {
	i := b.AbsToLogical(p.Line)
	if i < 0 || p.Col < 0 || p.Col >= b.Cols {
		return Link{}, false
	}
	c := b.LogicalLine(i)[p.Col]
	if c.Extra == 0 {
		return Link{}, false
	}
	id := ExtraOf(c.Extra).Link
	if id == 0 {
		return Link{}, false
	}
	return LinkOf(id), true
}
