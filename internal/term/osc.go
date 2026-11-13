package term

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func (t *Terminal) oscTerm() string {
	if t.oscBEL {
		return "\x07"
	}
	return "\x1b\\"
}

func (t *Terminal) oscDispatch(data []byte) {
	defer func() { t.oscBEL = false }()
	cmd, rest, _ := bytes.Cut(data, []byte{';'})
	n, err := strconv.Atoi(string(cmd))
	if err != nil {
		return
	}
	switch n {
	case 0, 2:
		t.Title = sanitizeTitle(string(rest))
	case 1:
		// icon name: ignored
	case 4:
		t.oscPalette(string(rest))
	case 7:
		t.oscCwd(string(rest))
	case 8:
		t.oscHyperlink(string(rest))
	case 10, 11, 12:
		t.oscDynamicColor(n, string(rest))
	case 52:
		t.oscClipboard(string(rest))
	case 104:
		if len(rest) == 0 {
			t.Palette = t.basePalette
		} else {
			for _, s := range strings.Split(string(rest), ";") {
				if i, err := strconv.Atoi(s); err == nil && i >= 0 && i < 256 {
					t.Palette[i] = t.basePalette[i]
				}
			}
		}
		t.paletteChanged()
	case 110:
		t.DefaultFg = t.baseFg
		t.paletteChanged()
	case 111:
		t.DefaultBg = t.baseBg
		t.paletteChanged()
	case 112:
		t.CursorColor = t.baseCursor
		t.paletteChanged()
	}
}

func sanitizeTitle(s string) string {
	if len(s) > 1024 {
		s = s[:1024]
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

func (t *Terminal) paletteChanged() {
	t.primary.MarkAllDirty()
	t.alt.MarkAllDirty()
}

func formatColor(c uint32) string {
	r, g, b := c>>16&0xff, c>>8&0xff, c&0xff
	return fmt.Sprintf("rgb:%02x%02x/%02x%02x/%02x%02x", r, r, g, g, b, b)
}

// ParseColor parses an X11 colour spec: rgb:r/g/b with 1 to 4 hex digits
// each, or #rgb, #rrggbb, #rrrgggbbb or #rrrrggggbbbb (the high digits
// of each count).
func ParseColor(s string) (uint32, bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "rgb:") {
		parts := strings.Split(s[4:], "/")
		if len(parts) != 3 {
			return 0, false
		}
		var out uint32
		for _, p := range parts {
			if len(p) == 0 || len(p) > 4 {
				return 0, false
			}
			v, err := strconv.ParseUint(p, 16, 32)
			if err != nil {
				return 0, false
			}
			// scale to 8 bits
			bits := uint(len(p) * 4)
			v8 := v * 255 / (1<<bits - 1)
			out = out<<8 | uint32(v8)
		}
		return out, true
	}
	if strings.HasPrefix(s, "#") {
		h := s[1:]
		switch len(h) {
		case 3:
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		case 6:
		case 9:
			h = h[0:2] + h[3:5] + h[6:8]
		case 12:
			h = h[0:2] + h[4:6] + h[8:10]
		default:
			return 0, false
		}
		v, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return 0, false
		}
		return uint32(v), true
	}
	return 0, false
}

func (t *Terminal) oscPalette(s string) {
	parts := strings.Split(s, ";")
	for i := 0; i+1 < len(parts); i += 2 {
		idx, err := strconv.Atoi(parts[i])
		if err != nil || idx < 0 || idx > 255 {
			continue
		}
		if parts[i+1] == "?" {
			t.reply("\x1b]4;" + strconv.Itoa(idx) + ";" + formatColor(t.Palette[idx]) + t.oscTerm())
			continue
		}
		if c, ok := ParseColor(parts[i+1]); ok {
			t.Palette[idx] = c
			t.paletteChanged()
		}
	}
}

func (t *Terminal) oscDynamicColor(n int, s string) {
	// Each further spec sets the next colour: OSC 10;fg;bg sets both.
	targets := [...]*uint32{&t.DefaultFg, &t.DefaultBg, &t.CursorColor}
	for _, spec := range strings.Split(s, ";") {
		if n > 12 {
			break
		}
		target := targets[n-10]
		if spec == "?" {
			t.reply("\x1b]" + strconv.Itoa(n) + ";" + formatColor(*target) + t.oscTerm())
		} else if c, ok := ParseColor(spec); ok {
			*target = c
			t.paletteChanged()
		}
		n++
	}
}

func (t *Terminal) oscCwd(s string) {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "file" {
		return
	}
	t.Cwd = u.Path
	t.events |= EvCwd
}

func (t *Terminal) oscHyperlink(s string) {
	params, uri, ok := strings.Cut(s, ";")
	if !ok {
		return
	}
	if uri == "" {
		t.link = 0
		t.updateExtra()
		return
	}
	id := ""
	for _, kv := range strings.Split(params, ":") {
		if v, ok := strings.CutPrefix(kv, "id="); ok {
			id = v
		}
	}
	if len(uri) > 4096 {
		uri = uri[:4096]
	}
	t.link = internLink(Link{ID: id, URI: uri})
	t.updateExtra()
}

func (t *Terminal) oscClipboard(s string) {
	target, data, ok := strings.Cut(s, ";")
	if !ok {
		return
	}
	primary := strings.ContainsAny(target, "ps") && !strings.Contains(target, "c")
	if data == "?" {
		if t.AllowClipboardRead {
			t.ClipboardQuery = true
			t.events |= EvClipboardQuery
		}
		return
	}
	dec, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return
	}
	t.ClipboardWrites = append(t.ClipboardWrites, ClipboardWrite{Primary: primary, Text: string(dec)})
	t.events |= EvClipboard
}

// ReplyClipboard answers a pending OSC 52 query.
func (t *Terminal) ReplyClipboard(text string) {
	t.ClipboardQuery = false
	t.reply("\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\x1b\\")
}

// dcsDispatch handles DECRQSS and XTGETTCAP; other DCS strings are ignored.
func (t *Terminal) dcsDispatch(cmd byte, data []byte) {
	p := &t.parser
	var inter byte
	if p.ninter > 0 {
		inter = p.inter[0]
	}
	switch {
	case inter == '$' && cmd == 'q': // DECRQSS
		switch string(data) {
		case "m":
			t.reply("\x1bP1$r" + t.sgrReport() + "m\x1b\\")
		case "r":
			t.reply("\x1bP1$r" + strconv.Itoa(t.top+1) + ";" + strconv.Itoa(t.bot+1) + "r\x1b\\")
		case " q":
			n := [...]int{CursorBlock: 2, CursorUnderline: 4, CursorBeam: 6}[t.CursorStyle]
			if t.CursorBlink {
				n--
			}
			t.reply("\x1bP1$r" + strconv.Itoa(n) + " q\x1b\\")
		default:
			t.reply("\x1bP0$r\x1b\\")
		}
	case inter == '+' && cmd == 'q': // XTGETTCAP
		var out []string
		for _, h := range strings.Split(string(data), ";") {
			name, err := hexDecode(h)
			if err != nil {
				continue
			}
			var val string
			switch name {
			case "TN", "name":
				val = "xterm-256color"
			case "Co", "colors":
				val = "256"
			case "RGB":
				val = "8/8/8"
			default:
				continue
			}
			out = append(out, h+"="+hexEncode(val))
		}
		if len(out) == 0 {
			t.reply("\x1bP0+r\x1b\\")
		} else {
			t.reply("\x1bP1+r" + strings.Join(out, ";") + "\x1b\\")
		}
	}
}

func hexDecode(s string) (string, error) {
	b, err := hex.DecodeString(s)
	return string(b), err
}

func hexEncode(s string) string { return strings.ToUpper(hex.EncodeToString([]byte(s))) }

// defaultPalette returns the xterm 256-colour palette.
func defaultPalette() [256]uint32 {
	return Palette256([16]uint32{
		0x000000, 0xcd0000, 0x00cd00, 0xcdcd00, 0x0000ee, 0xcd00cd, 0x00cdcd, 0xe5e5e5,
		0x7f7f7f, 0xff0000, 0x00ff00, 0xffff00, 0x5c5cff, 0xff00ff, 0x00ffff, 0xffffff,
	})
}

// Palette256 returns a full palette from 16 base colours, adding the
// standard 6×6×6 colour cube and grey ramp.
func Palette256(base [16]uint32) [256]uint32 {
	var p [256]uint32
	copy(p[:], base[:])
	steps := [6]uint32{0, 0x5f, 0x87, 0xaf, 0xd7, 0xff}
	for i := 0; i < 216; i++ {
		r, g, b := steps[i/36], steps[i/6%6], steps[i%6]
		p[16+i] = r<<16 | g<<8 | b
	}
	for i := 0; i < 24; i++ {
		v := uint32(8 + i*10)
		p[232+i] = v<<16 | v<<8 | v
	}
	return p
}
