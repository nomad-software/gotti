package term

import "unicode/utf8"

// Parser states (after Paul Williams' VT500 state machine, trimmed to
// 7-bit controls and UTF-8 input).
type pstate uint8

const (
	sGround pstate = iota
	sEscape
	sEscapeInter
	sCSIEntry
	sCSIParam
	sCSIInter
	sCSIIgnore
	sOSC
	sDCSEntry
	sDCSParam
	sDCSInter
	sDCSPass
	sDCSIgnore
	sSOS // SOS/PM/APC: consumed and ignored
	sStringEsc
)

const (
	maxParams   = 32
	maxOSC      = 1 << 20 // OSC 52 payloads can be large
	maxDCS      = 1 << 16
	paramMaxVal = 1<<16 - 1
)

// Params holds CSI parameters. Sub-parameters (colon separated) are kept
// in sequence and flagged by Sub.
type Params struct {
	N   int
	V   [maxParams]int32 // -1 = omitted
	Sub [maxParams]bool  // true when this value followed a ':'
}

// Get returns parameter i (counting only top-level parameters), or def
// when it is missing or zero.
func (p *Params) Get(i int, def int) int {
	k := p.index(i)
	if k < 0 || p.V[k] <= 0 {
		return def
	}
	return int(p.V[k])
}

// Raw returns parameter i, or def when omitted (zero allowed).
func (p *Params) Raw(i int, def int) int {
	k := p.index(i)
	if k < 0 || p.V[k] < 0 {
		return def
	}
	return int(p.V[k])
}

// Count returns the number of top-level parameters.
func (p *Params) Count() int {
	n := 0
	for k := 0; k < p.N; k++ {
		if !p.Sub[k] {
			n++
		}
	}
	return n
}

func (p *Params) index(i int) int {
	for k := 0; k < p.N; k++ {
		if !p.Sub[k] {
			if i == 0 {
				return k
			}
			i--
		}
	}
	return -1
}

// parser is embedded in Terminal; its dispatch methods call straight into
// the terminal (no interface indirection on the hot path).
type parser struct {
	state   pstate
	ret     pstate // state to return to after ESC inside a string
	params  Params
	cur     int32 // current parameter being accumulated (-1 = none)
	nextSub bool  // the parameter being accumulated follows a ':'
	private byte  // '<' '=' '>' '?'
	inter   [2]byte
	ninter  int
	osc     []byte
	dcs     []byte
	dcsCmd  byte

	// UTF-8 decoding state for sequences split across reads.
	utf8buf [4]byte
	utf8n   int
	utf8len int
}

func (p *parser) resetSeq() {
	p.params.N = 0
	p.cur = -1
	p.nextSub = false
	p.private = 0
	p.ninter = 0
}

// Write feeds bytes from the PTY into the terminal. The caller must hold
// the terminal lock.
func (t *Terminal) Write(data []byte) {
	p := &t.parser
	i := 0
	for i < len(data) {
		if p.state == sGround {
			if p.utf8n > 0 {
				i = t.continueUTF8(data, i)
				continue
			}
			// ASCII fast path: bulk-print runs of printable bytes.
			j := i
			for j < len(data) && data[j] >= 0x20 && data[j] < 0x7f {
				j++
			}
			if j > i {
				t.printASCII(data[i:j])
				i = j
				continue
			}
			b := data[i]
			if b == 0x1b {
				t.step(b)
				i++
				continue
			}
			if b < 0x20 {
				t.execute(b)
				i++
				continue
			}
			if b == 0x7f {
				i++
				continue
			}
			// Non-ASCII: UTF-8 decode.
			if utf8.FullRune(data[i:]) {
				r, n := utf8.DecodeRune(data[i:])
				i += n
				t.printRune(r)
				continue
			}
			// Incomplete sequence at end of buffer, or invalid lead.
			need := utf8SeqLen(b)
			if need == 0 {
				t.printRune(utf8.RuneError)
				i++
				continue
			}
			p.utf8len = need
			p.utf8n = 0
			i = t.continueUTF8(data, i)
			continue
		}
		switch p.state {
		case sCSIEntry, sCSIParam:
			i = p.scanCSIParams(data, i)
		case sOSC:
			i = p.scanOSC(data, i)
		case sDCSPass:
			i = p.scanDCS(data, i)
		}
		if i < len(data) {
			t.step(data[i])
			i++
		}
	}
}

// scanCSIParams consumes the digits and separators of CSI parameters in a
// tight loop, returning the index of the first byte it did not consume,
// which step then handles. It is the bulk of most escape sequences.
func (p *parser) scanCSIParams(data []byte, i int) int {
	for ; i < len(data); i++ {
		switch b := data[i]; {
		case b >= '0' && b <= '9':
			if p.cur < 0 {
				p.cur = 0
			}
			if p.cur < paramMaxVal {
				p.cur = min(p.cur*10+int32(b-'0'), paramMaxVal)
			}
		case b == ';' || b == ':':
			p.pushParam()
			p.nextSub = b == ':'
		default:
			return i
		}
		p.state = sCSIParam
	}
	return i
}

// scanOSC appends a run of printable OSC payload bytes at once. Terminators
// and other controls are left to step.
func (p *parser) scanOSC(data []byte, i int) int {
	j := i
	for j < len(data) && data[j] >= 0x20 {
		j++
	}
	if room := maxOSC - len(p.osc); room > 0 {
		p.osc = append(p.osc, data[i:min(j, i+room)]...)
	}
	return j
}

// scanDCS appends DCS passthrough data up to the next ESC, CAN or SUB.
func (p *parser) scanDCS(data []byte, i int) int {
	j := i
	for j < len(data) {
		if b := data[j]; b == 0x1b || b == 0x18 || b == 0x1a {
			break
		}
		j++
	}
	if room := maxDCS - len(p.dcs); room > 0 {
		p.dcs = append(p.dcs, data[i:min(j, i+room)]...)
	}
	return j
}

func utf8SeqLen(b byte) int {
	switch {
	case b&0xe0 == 0xc0:
		return 2
	case b&0xf0 == 0xe0:
		return 3
	case b&0xf8 == 0xf0:
		return 4
	}
	return 0
}

// continueUTF8 accumulates a split UTF-8 sequence.
func (t *Terminal) continueUTF8(data []byte, i int) int {
	p := &t.parser
	for i < len(data) && p.utf8n < p.utf8len {
		b := data[i]
		if p.utf8n > 0 && b&0xc0 != 0x80 {
			// invalid continuation: emit replacement, reprocess byte
			p.utf8n = 0
			t.printRune(utf8.RuneError)
			return i
		}
		p.utf8buf[p.utf8n] = b
		p.utf8n++
		i++
	}
	if p.utf8n == p.utf8len {
		r, _ := utf8.DecodeRune(p.utf8buf[:p.utf8n])
		p.utf8n = 0
		t.printRune(r)
	}
	return i
}

// step advances the state machine by one byte outside the ground state.
func (t *Terminal) step(b byte) {
	p := &t.parser
	// Controls that act in any state.
	switch b {
	case 0x18, 0x1a: // CAN, SUB abort sequences
		p.osc = p.osc[:0]
		p.dcs = p.dcs[:0]
		p.state = sGround
		return
	case 0x1b:
		switch p.state {
		case sOSC, sDCSPass, sDCSIgnore, sSOS:
			p.ret = p.state
			p.state = sStringEsc
			return
		}
		p.state = sEscape
		p.resetSeq()
		return
	}

	switch p.state {
	case sEscape:
		switch {
		case b < 0x20:
			t.execute(b)
		case b >= 0x20 && b <= 0x2f:
			p.inter[0] = b
			p.ninter = 1
			p.state = sEscapeInter
		case b == '[':
			p.resetSeq()
			p.state = sCSIEntry
		case b == ']':
			p.osc = p.osc[:0]
			p.state = sOSC
		case b == 'P':
			p.resetSeq()
			p.dcs = p.dcs[:0]
			p.state = sDCSEntry
		case b == 'X' || b == '^' || b == '_':
			p.state = sSOS
		case b == 0x7f:
		default:
			p.state = sGround
			t.escDispatch(b)
		}
	case sEscapeInter:
		switch {
		case b < 0x20:
			t.execute(b)
		case b <= 0x2f:
			if p.ninter < len(p.inter) {
				p.inter[p.ninter] = b
				p.ninter++
			}
		case b == 0x7f:
		default:
			p.state = sGround
			t.escDispatch(b)
		}
	case sCSIEntry, sCSIParam:
		switch {
		case b < 0x20:
			t.execute(b)
		case b >= '0' && b <= '9':
			if p.cur < 0 {
				p.cur = 0
			}
			if p.cur < paramMaxVal {
				p.cur = min(p.cur*10+int32(b-'0'), paramMaxVal)
			}
			p.state = sCSIParam
		case b == ';' || b == ':':
			p.pushParam()
			p.nextSub = b == ':'
			p.state = sCSIParam
		case b >= '<' && b <= '?':
			if p.state == sCSIEntry {
				p.private = b
				p.state = sCSIParam
			} else {
				p.state = sCSIIgnore
			}
		case b >= 0x20 && b <= 0x2f:
			p.finishParam()
			p.inter[0] = b
			p.ninter = 1
			p.state = sCSIInter
		case b >= 0x40 && b <= 0x7e:
			p.finishParam()
			p.state = sGround
			t.csiDispatch(b)
		}
	case sCSIInter:
		switch {
		case b < 0x20:
			t.execute(b)
		case b <= 0x2f:
			if p.ninter < len(p.inter) {
				p.inter[p.ninter] = b
				p.ninter++
			}
		case b >= 0x30 && b <= 0x3f:
			p.state = sCSIIgnore
		case b >= 0x40 && b <= 0x7e:
			p.state = sGround
			t.csiDispatch(b)
		}
	case sCSIIgnore:
		if b < 0x20 {
			t.execute(b)
		} else if b >= 0x40 && b <= 0x7e {
			p.state = sGround
		}
	case sOSC:
		if b == 0x07 { // BEL terminator
			p.state = sGround
			t.oscBEL = true
			t.oscDispatch(p.osc)
			return
		}
		if b < 0x20 {
			return
		}
		if len(p.osc) < maxOSC {
			p.osc = append(p.osc, b)
		}
	case sStringEsc:
		// ESC inside a string: '\' completes ST, anything else aborts the
		// string and starts a new escape sequence.
		prev := p.ret
		if b == '\\' {
			p.state = sGround
			switch prev {
			case sOSC:
				t.oscDispatch(p.osc)
			case sDCSPass:
				t.dcsDispatch(p.dcsCmd, p.dcs)
			}
			return
		}
		p.state = sEscape
		p.resetSeq()
		t.step(b)
	case sDCSEntry, sDCSParam:
		switch {
		case b < 0x20:
		case b >= '0' && b <= '9':
			if p.cur < 0 {
				p.cur = 0
			}
			if p.cur < paramMaxVal {
				p.cur = min(p.cur*10+int32(b-'0'), paramMaxVal)
			}
			p.state = sDCSParam
		case b == ';' || b == ':':
			p.pushParam()
			p.state = sDCSParam
		case b >= '<' && b <= '?':
			if p.state == sDCSEntry {
				p.private = b
				p.state = sDCSParam
			} else {
				p.state = sDCSIgnore
			}
		case b >= 0x20 && b <= 0x2f:
			p.finishParam()
			p.inter[0] = b
			p.ninter = 1
			p.state = sDCSInter
		case b >= 0x40 && b <= 0x7e:
			p.finishParam()
			p.dcsCmd = b
			p.state = sDCSPass
		}
	case sDCSInter:
		switch {
		case b < 0x20:
		case b <= 0x2f:
			if p.ninter < len(p.inter) {
				p.inter[p.ninter] = b
				p.ninter++
			}
		case b >= 0x30 && b <= 0x3f:
			p.state = sDCSIgnore
		case b >= 0x40 && b <= 0x7e:
			p.dcsCmd = b
			p.state = sDCSPass
		}
	case sDCSPass:
		if len(p.dcs) < maxDCS {
			p.dcs = append(p.dcs, b)
		}
	case sDCSIgnore, sSOS:
		// swallow until ST
	}
}

func (p *parser) pushParam() {
	if p.params.N < maxParams {
		p.params.V[p.params.N] = p.cur
		p.params.Sub[p.params.N] = p.nextSub
		p.params.N++
	}
	p.cur = -1
	p.nextSub = false
}

func (p *parser) finishParam() {
	if p.cur >= 0 || p.params.N > 0 || p.nextSub {
		p.pushParam()
	}
}
