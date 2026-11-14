package kitty

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Text sizing: kitty's OSC 66 draws text bigger than a cell (kitty 0.40
// and later). See https://sw.kovidgoyal.net/kitty/text-sizing-protocol/
//
// Bubble Tea v2 drops OSC 66 from a view, with the text inside it: its
// cell renderer keeps only hyperlinks among OSC sequences. Use text
// sizing only when writing straight to the terminal.

// Size is how big TextSize draws: Scale cells high and Scale cells wide
// per cell of text. A fraction Num/Den (Num < Den, both up to 15) draws
// the glyphs smaller inside the same cells, aligned by VAlign (0 top,
// 1 bottom, 2 centre).
type Size struct {
	Scale    int
	Num, Den int
	VAlign   int
}

// maxTextBytes is the most text one OSC 66 sequence may carry.
const maxTextBytes = 4096

// TextSize returns the OSC 66 sequences that draw text at size s. The
// text is plain: no escape codes or control characters. It takes
// Scale × its usual width in cells on the first row, and Scale rows: leave
// the cells below it empty (text written there is moved past it).
func TextSize(text string, s Size) string {
	var meta []string
	if s.Scale > 1 {
		meta = append(meta, "s="+strconv.Itoa(min(s.Scale, 7)))
	}
	if s.Den > 0 && s.Num < s.Den {
		meta = append(meta, "n="+strconv.Itoa(min(s.Num, 15)), "d="+strconv.Itoa(min(s.Den, 15)))
		if s.VAlign > 0 {
			meta = append(meta, "v="+strconv.Itoa(min(s.VAlign, 2)))
		}
	}
	head := "\x1b]66;" + strings.Join(meta, ":") + ";"
	var b strings.Builder
	for text != "" {
		n := len(text)
		if n > maxTextBytes {
			n = maxTextBytes
			for n > 0 && !utf8.RuneStart(text[n]) {
				n--
			}
		}
		b.WriteString(head + text[:n] + "\a")
		text = text[n:]
	}
	return b.String()
}

// TextSizingQuery asks whether the terminal supports text sizing. It
// writes a space two cells wide and a space two cells high, each followed
// by a cursor position request, so send it at the start of an empty line
// and clear the line (and the one below) afterwards. Pass the columns of
// the three reports to TextSizing.
const TextSizingQuery = "\r" + ansi.RequestCursorPositionReport +
	"\x1b]66;w=2; \a" + ansi.RequestCursorPositionReport +
	"\x1b]66;s=2; \a" + ansi.RequestCursorPositionReport

// TextSizing reads the cursor columns the terminal reported for
// TextSizingQuery: width is true when it can set the width of text, and
// scale when it can also draw text bigger.
func TextSizing(col0, col1, col2 int) (width, scale bool) {
	width = col1-col0 == 2
	return width, width && col2-col1 == 2
}
