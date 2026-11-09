// termite-headless runs a program on a PTY through the termite emulator core
// and prints the resulting screen. It is used for testing real programs
// (vim, tmux, htop, vttest) without a GUI.
//
//	termite-headless -size 80x24 -script 'ls{enter}{wait 500}exit{enter}' -- bash
//
// Script tokens: {enter} {esc} {tab} {up} {down} {left} {right} {wait N}
// {ctrl X} {hex 1b5b41}; other text is sent literally.
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
	fmt.Sscanf(*size, "%dx%d", &cols, &rows)
	t := term.New(cols, rows, 10000)
	s, err := pty.Start(t, pty.Options{Command: flag.Args(), Cols: cols, Rows: rows})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exited := make(chan struct{})
	s.OnExit = func() { close(exited) }
	start := time.Now()
	s.Run()
	go runScript(s, *script)
	if *bench {
		<-exited
		fmt.Printf("elapsed %v\n", time.Since(start))
		return
	}
	select {
	case <-exited:
	case <-time.After(scriptDuration(*script) + *wait):
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

type token struct {
	text string
	wait time.Duration
}

func parseScript(s string) []token {
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
		keys := map[string]string{"enter": "\r", "esc": "\x1b", "tab": "\t", "up": "\x1b[A", "down": "\x1b[B",
			"right": "\x1b[C", "left": "\x1b[D", "bs": "\x7f", "space": " "}
		switch cmd[0] {
		case "wait":
			ms, _ := strconv.Atoi(cmd[1])
			out = append(out, token{wait: time.Duration(ms) * time.Millisecond})
		case "ctrl":
			out = append(out, token{text: string(rune(strings.ToUpper(cmd[1])[0]) & 0x1f)})
		case "hex":
			b, _ := hex.DecodeString(cmd[1])
			out = append(out, token{text: string(b)})
		default:
			out = append(out, token{text: keys[cmd[0]]})
		}
	}
	return out
}

func scriptDuration(s string) time.Duration {
	var d time.Duration
	for _, t := range parseScript(s) {
		d += t.wait + 20*time.Millisecond
	}
	return d + 300*time.Millisecond
}

func runScript(s *pty.Session, script string) {
	time.Sleep(300 * time.Millisecond)
	for _, t := range parseScript(script) {
		if t.wait > 0 {
			time.Sleep(t.wait)
			continue
		}
		s.WriteString(t.text)
		time.Sleep(20 * time.Millisecond)
	}
}
