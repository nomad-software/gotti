// termite-headless runs a program on a PTY through the termite emulator core
// and prints the resulting screen. It is used for testing real programs
// (vim, tmux, htop, vttest) without a GUI.
//
//	termite-headless -size 80x24 -script 'ls{enter}{wait 500}exit{enter}' -- bash
//
// Script tokens: {enter} {esc} {tab} {bs} {space} {up} {down} {left}
// {right} {wait N} (milliseconds) {ctrl X} {hex 1b5b41}; other text is sent
// literally.
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"termite/internal/pty"
	"termite/internal/term"
)

func main() {
	size := flag.String("size", "80x24", "COLSxROWS")
	script := flag.String("script", "", "input script")
	wait := flag.Duration("wait", 500*time.Millisecond, "settle time after script")
	cursor := flag.Bool("cursor", true, "mark the cursor with ▮")
	bench := flag.Bool("bench", false, "only report throughput and timing")
	flag.Parse()
	var cols, rows int
	if n, _ := fmt.Sscanf(*size, "%dx%d", &cols, &rows); n != 2 || cols < 2 || rows < 1 {
		fatal(fmt.Errorf("-size %q: want COLSxROWS, e.g. 80x24", *size))
	}
	tokens, err := parseScript(*script)
	if err != nil {
		fatal(err)
	}
	t := term.New(cols, rows, 10000)
	s, err := pty.Start(t, pty.Options{Command: flag.Args(), Cols: cols, Rows: rows})
	if err != nil {
		fatal(err)
	}
	exited := make(chan struct{})
	s.OnExit = func() { close(exited) }
	start := time.Now()
	s.Run()
	go runScript(s, tokens)
	if *bench {
		<-exited
		fmt.Printf("elapsed %v\n", time.Since(start))
		return
	}
	select {
	case <-exited:
	case <-time.After(scriptDuration(tokens) + *wait):
	}
	t.Mu.Lock()
	defer t.Mu.Unlock()
	b := t.Buf()
	cx, cy := t.CursorPos()
	fmt.Printf("┌%s┐ alt=%v cursor=%d,%d title=%q\n", strings.Repeat("─", cols), t.AltScreen(), cx, cy, t.Title)
	for y := 0; y < rows; y++ {
		line := b.Line(y)
		var sb strings.Builder
		for x := 0; x < cols; x++ {
			c := &line[x]
			if c.Attr&term.AttrWideCont != 0 {
				continue
			}
			if *cursor && x == cx && y == cy && t.CursorVisible {
				sb.WriteString("▮")
				if c.Attr&term.AttrWide != 0 {
					sb.WriteString(" ")
				}
				continue
			}
			if c.Ch == 0 {
				sb.WriteByte(' ')
			} else {
				sb.WriteString(c.Text())
			}
		}
		fmt.Printf("│%s│\n", sb.String())
	}
	fmt.Printf("└%s┘\n", strings.Repeat("─", cols))
	s.Close()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "termite-headless:", err)
	os.Exit(1)
}

type token struct {
	text string
	wait time.Duration
}

// keys are the named keys a script can send.
var keys = map[string]string{
	"enter": "\r", "esc": "\x1b", "tab": "\t", "bs": "\x7f", "space": " ",
	"up": "\x1b[A", "down": "\x1b[B", "right": "\x1b[C", "left": "\x1b[D",
}

func parseScript(s string) ([]token, error) {
	var out []token
	for len(s) > 0 {
		i := strings.IndexByte(s, '{')
		if i < 0 {
			out = append(out, token{text: s})
			break
		}
		if i > 0 {
			out = append(out, token{text: s[:i]})
		}
		j := strings.IndexByte(s[i:], '}')
		if j < 0 {
			out = append(out, token{text: s[i:]})
			break
		}
		cmd := strings.Fields(s[i+1 : i+j])
		s = s[i+j+1:]
		if len(cmd) == 0 {
			continue
		}
		arg := ""
		if len(cmd) > 1 {
			arg = cmd[1]
		}
		switch cmd[0] {
		case "wait":
			ms, err := strconv.Atoi(arg)
			if err != nil {
				return nil, fmt.Errorf("{wait %s}: want milliseconds", arg)
			}
			out = append(out, token{wait: time.Duration(ms) * time.Millisecond})
		case "ctrl":
			if len(arg) != 1 {
				return nil, fmt.Errorf("{ctrl %s}: want one key", arg)
			}
			out = append(out, token{text: string(rune(strings.ToUpper(arg)[0]) & 0x1f)})
		case "hex":
			b, err := hex.DecodeString(arg)
			if err != nil {
				return nil, fmt.Errorf("{hex %s}: %v", arg, err)
			}
			out = append(out, token{text: string(b)})
		default:
			k, ok := keys[cmd[0]]
			if !ok {
				return nil, fmt.Errorf("unknown script token {%s}", strings.Join(cmd, " "))
			}
			out = append(out, token{text: k})
		}
	}
	return out, nil
}

func scriptDuration(tokens []token) time.Duration {
	var d time.Duration
	for _, t := range tokens {
		d += t.wait + 20*time.Millisecond
	}
	return d + 300*time.Millisecond
}

func runScript(s *pty.Session, tokens []token) {
	time.Sleep(300 * time.Millisecond)
	for _, t := range tokens {
		if t.wait > 0 {
			time.Sleep(t.wait)
			continue
		}
		s.WriteString(t.text)
		time.Sleep(20 * time.Millisecond)
	}
}
