package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"

	"github.com/ionut-maxim/markdown"
	"github.com/ionut-maxim/markdown/kitty"
)

// printDoc prints the document straight to the terminal at its width,
// after asking it what it can do. Pictures stay after mdview exits, as
// printed text does.
func printDoc(src string, fetch func(context.Context, string) ([]byte, error)) error {
	width := 80
	var caps markdown.Caps
	if term.IsTerminal(os.Stdout.Fd()) && term.IsTerminal(os.Stdin.Fd()) {
		if w, _, err := term.GetSize(os.Stdout.Fd()); err == nil && w > 0 {
			width = w
		}
		var err error
		if caps, err = probe(); err != nil {
			return err
		}
	}
	r := markdown.New(markdown.Options{Theme: markdown.DefaultTheme(true), Caps: caps, Fetch: fetch})
	doc := r.Parse(src)
	seq, jobs := r.Prepare(width, doc)
	for _, j := range jobs {
		seq += r.Loaded(j.Run(context.Background()))
	}
	_, err := fmt.Fprint(os.Stdout, seq+r.Render(doc, width)+"\n")
	return err
}

// The answers probe reads.
var (
	cellReply  = regexp.MustCompile(`\x1b\[6;(\d+);(\d+)t`)
	cprReply   = regexp.MustCompile(`\x1b\[\d+;(\d+)R`)
	kittyReply = regexp.MustCompile(`\x1b_Gi=` + strconv.Itoa(kitty.QueryID) + `;([^\x1b]*)\x1b\\`)
	modeReply  = regexp.MustCompile(`\x1b\[\?2027;(\d)\$y`)
	da1Reply   = regexp.MustCompile(`\x1b\[\?[\d;]*c`)
)

// probe asks the terminal for its cell size, text sizing, Unicode mode
// and kitty graphics, and reads the answers until the primary device
// attributes, which every terminal sends last.
func probe() (markdown.Caps, error) {
	var caps markdown.Caps
	old, err := term.MakeRaw(os.Stdin.Fd())
	if err != nil {
		return caps, err
	}
	defer term.Restore(os.Stdin.Fd(), old) //nolint:errcheck

	// Text sizing draws test spaces at the start of the line: clear them.
	q := ansi.WindowOp(16) + kitty.TextSizingQuery + "\r" + ansi.EraseEntireLine +
		ansi.RequestModeUnicodeCore + kitty.Query
	if _, err := os.Stdout.WriteString(q); err != nil {
		return caps, err
	}
	got := make(chan []byte)
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				close(got)
				return
			}
			got <- append([]byte(nil), buf[:n]...)
		}
	}()
	var in []byte
	deadline := time.After(2 * time.Second)
read:
	for !da1Reply.Match(in) {
		select {
		case b, ok := <-got:
			if !ok {
				break read
			}
			in = append(in, b...)
		case <-deadline:
			break read
		}
	}

	if m := cellReply.FindSubmatch(in); m != nil {
		caps.Cell.H, _ = strconv.Atoi(string(m[1]))
		caps.Cell.W, _ = strconv.Atoi(string(m[2]))
	}
	if m := cprReply.FindAllSubmatch(in, 3); len(m) == 3 {
		col := func(i int) int { n, _ := strconv.Atoi(string(m[i][1])); return n }
		_, caps.TextSizing = kitty.TextSizing(col(0), col(1), col(2))
	}
	if m := kittyReply.FindSubmatch(in); m != nil {
		caps.Graphics = string(m[1]) == "OK"
	}
	// Only when it's on already: this mode doesn't turn it on.
	if m := modeReply.FindSubmatch(in); m != nil && (m[1][0] == '1' || m[1][0] == '3') {
		caps.Width = ansi.GraphemeWidth
	}
	return caps, nil
}
