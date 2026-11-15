package teamd

import (
	"context"
	"fmt"
	"image"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"

	"github.com/ionut-maxim/markdown"
)

// run runs cmd the way Bubble Tea would, and returns what it sends to the
// terminal and the other messages it produces.
func run(cmd tea.Cmd) (string, []tea.Msg) {
	if cmd == nil {
		return "", nil
	}
	var out strings.Builder
	var msgs []tea.Msg
	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			s, m := run(c)
			out.WriteString(s)
			msgs = append(msgs, m...)
		}
	case tea.RawMsg:
		out.WriteString(fmt.Sprint(msg.Msg))
	default:
		msgs = append(msgs, msg)
	}
	return out.String(), msgs
}

// square draws every diagram as a 96×48 picture.
type square struct{}

func (square) Draw(context.Context, markdown.Diagram) (image.Image, error) {
	return image.NewRGBA(image.Rect(0, 0, 96, 48)), nil
}

const doc = "# Title\n\n```mermaid\nflowchart LR\n  A --> B\n```\n"

func newPictures(opt Options) (*Pictures, *markdown.Renderer, *markdown.Doc) {
	n := 0
	r := markdown.New(markdown.Options{Theme: markdown.DefaultTheme(true), Diagrams: square{},
		NewID: func() int { n++; return 100 + n }})
	return New(r, opt), r, r.Parse(doc)
}

func syncReply(payload string) tea.Msg {
	return uv.KittyGraphicsEvent{Options: kitty.Options{ID: SyncID}, Payload: []byte(payload)}
}

func TestProbeAndFirstFrame(t *testing.T) {
	p, r, d := newPictures(Options{})
	if s, _ := run(p.Init()); s != ansi.WindowOp(16) {
		t.Fatalf("Init should ask for the cell size, sent %q", s)
	}
	if s, _ := run(p.Update(uv.CellSizeEvent{Width: 10, Height: 20})); s != "" {
		t.Errorf("nothing to send before Show: %q", s)
	}

	// The first size: the app shows the document, and Pictures asks the
	// terminal when the frame is out instead of sending pictures.
	s, _ := run(p.Update(tea.WindowSizeMsg{Width: 60, Height: 20}))
	if s != syncQuery {
		t.Fatalf("want the frame query, sent %q", s)
	}
	if s, _ := run(p.Show(60, d)); s != "" {
		t.Fatalf("pictures held back until the frame is out, sent %q…", s[:min(len(s), 60)])
	}
	if s, _ := run(p.Update(tea.WindowSizeMsg{Width: 60, Height: 20})); s != "" {
		t.Errorf("the frame query goes once, sent %q", s)
	}

	// The answer: kitty graphics work, and the heading goes out with
	// the diagram's job.
	s, msgs := run(p.Update(syncReply("OK")))
	if !p.Ready() || !r.Caps().Graphics || r.Caps().Cell.W != 10 {
		t.Fatalf("caps after the answer: %+v", r.Caps())
	}
	if !strings.Contains(s, "i=102") || len(msgs) != 1 {
		t.Fatalf("want the heading sent and one job, got %q… and %d messages", s[:min(len(s), 60)], len(msgs))
	}
	s, _ = run(p.Update(msgs[0]))
	if !strings.Contains(s, "a=p,i=101") {
		t.Errorf("the finished job sends the diagram: %q…", s[:min(len(s), 80)])
	}
	if out := r.Render(d, 60); strings.Count(out, "\U0010EEEE") == 0 {
		t.Error("Render should draw the pictures")
	}

	// A narrower window: sent again, no new job.
	s, msgs = run(p.Show(30, d))
	if s == "" || len(msgs) != 0 {
		t.Errorf("at 30 columns: %d bytes, %d jobs", len(s), len(msgs))
	}
	// Reset: everything again, still no new job.
	s, msgs = run(p.Reset())
	if !strings.Contains(s, "a=p,i=101") || len(msgs) != 0 {
		t.Errorf("Reset: %q…, %d jobs", s[:min(len(s), 60)], len(msgs))
	}
	// Close deletes them, and nothing more is sent until Show.
	if s, _ := run(p.Close()); !strings.Contains(s, "a=d") {
		t.Errorf("Close: %q", s)
	}
	if s, _ := run(p.Update(uv.CellSizeEvent{Width: 12, Height: 24})); s != "" {
		t.Errorf("closed, yet sent %q", s)
	}
}

func TestNoKittyGraphics(t *testing.T) {
	p, r, d := newPictures(Options{})
	run(p.Update(uv.CellSizeEvent{Width: 10, Height: 20}))
	run(p.Update(tea.WindowSizeMsg{Width: 60, Height: 20}))
	run(p.Show(60, d))
	s, msgs := run(p.Update(syncReply("ENOTSUPPORTED")))
	if r.Caps().Graphics || s != "" || len(msgs) != 0 {
		t.Errorf("no graphics: caps %+v, sent %q, %d jobs", r.Caps(), s, len(msgs))
	}
}

func TestNoProbe(t *testing.T) {
	p, r, d := newPictures(Options{NoProbe: true})
	if p.Init() != nil {
		t.Error("NoProbe: Init sends nothing")
	}
	// The app's own probe found graphics and the cell size.
	r.SetCaps(markdown.Caps{Graphics: true, Cell: markdown.Caps{}.Cell})
	run(p.Update(uv.CellSizeEvent{Width: 10, Height: 20}))
	if r.Caps().Cell.W != 0 {
		t.Error("NoProbe: the cell size is the app's to set")
	}
	c := r.Caps()
	c.Cell.W, c.Cell.H = 10, 20
	r.SetCaps(c)
	run(p.Update(tea.WindowSizeMsg{Width: 60, Height: 20}))
	run(p.Show(60, d))
	// The frame answer only says the frame is out: graphics stay as set.
	s, _ := run(p.Update(syncReply("ENOTSUPPORTED")))
	if !r.Caps().Graphics || !strings.Contains(s, "i=102") {
		t.Errorf("NoProbe: caps %+v, sent %q…", r.Caps(), s[:min(len(s), 40)])
	}
}

func TestUnicodeMode(t *testing.T) {
	p, r, _ := newPictures(Options{})
	run(p.Update(tea.ModeReportMsg{Mode: ansi.ModeUnicodeCore, Value: ansi.ModeNotRecognized}))
	if r.Caps().Width != ansi.WcWidth {
		t.Error("not recognised: keep counting code points")
	}
	run(p.Update(tea.ModeReportMsg{Mode: ansi.ModeUnicodeCore, Value: ansi.ModeReset}))
	if r.Caps().Width != ansi.GraphemeWidth {
		t.Error("known to the terminal: Bubble Tea turns it on, so measure graphemes")
	}
}

func TestOtherPicturesJobs(t *testing.T) {
	a, _, _ := newPictures(Options{})
	b, _, _ := newPictures(Options{})
	if a.Update(loadedMsg{p: b}) != nil {
		t.Error("a job of other Pictures is left alone")
	}
}
