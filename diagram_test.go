package markdown

import (
	"context"
	"errors"
	"image"
	"image/color"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ionut-maxim/markdown/kitty"
)

// fakeDiagrams draws a w×h picture, fails for sources that say "broken",
// and counts its calls.
type fakeDiagrams struct {
	w, h  int
	calls atomic.Int32
	last  Diagram
}

func (f *fakeDiagrams) Draw(_ context.Context, d Diagram) (image.Image, error) {
	f.calls.Add(1)
	f.last = d
	if strings.Contains(d.Source, "broken") {
		return nil, errors.New("parse error")
	}
	return image.NewRGBA(image.Rect(0, 0, f.w, f.h)), nil
}

func diagramRenderer(dg Diagrammer, caps Caps) *Renderer {
	return New(Options{Theme: testTheme(), Caps: caps, NewID: counter(), Diagrams: dg})
}

var graphicsCaps = Caps{Graphics: true, Cell: kitty.Cell{W: 8, H: 16}}

const flowchart = "```mermaid\nflowchart LR\n    A --> B\n```\n"

func TestDiagramLifecycle(t *testing.T) {
	f := &fakeDiagrams{w: 480, h: 240}
	r := diagramRenderer(f, graphicsCaps)
	d := r.Parse("Before.\n\n" + flowchart + "\nAfter.")

	seq, jobs := r.Prepare(60, d)
	if len(jobs) != 1 || jobs[0].URL != "" || seq != "" {
		t.Fatalf("want one diagram job and nothing to send, got %q and %+v", seq, jobs)
	}
	if out := ansi.Strip(r.Render(d, 60)); !strings.Contains(out, "A --> B") || strings.Contains(out, "loading") {
		t.Errorf("while drawing, the source shows, without a note:\n%s", out)
	}

	seq = runAll(r, jobs)
	if f.last.Source != "flowchart LR\n    A --> B\n" || f.last.TextHeight != 48 {
		t.Errorf("asked to draw %+v", f.last)
	}
	// 480×240 at bigtext's 24×48 cell: 20×5, which fits.
	if strings.Count(seq, "a=p") != 1 || !strings.Contains(seq, "c=20,r=5") {
		t.Fatalf("Loaded should send the picture 20×5: %q…", seq[:min(len(seq), 120)])
	}
	out := r.Render(d, 60)
	if strings.Contains(out, "\x1b_G") || strings.Contains(ansi.Strip(out), "A --> B") {
		t.Fatalf("drawn: no graphics in Render, no source:\n%s", ansi.Strip(out))
	}
	if got := strings.Count(out, "\U0010EEEE"); got != 20*5 {
		t.Errorf("want 100 placeholder cells, got %d", got)
	}

	// Narrower: sent again smaller, without drawing again.
	seq, jobs = r.Prepare(16, d)
	if len(jobs) != 0 || !strings.Contains(seq, "c=16,r=4") {
		t.Errorf("at 16 columns: jobs %d, seq %q", len(jobs), seq)
	}
	// Same width again: nothing to do.
	if seq, jobs = r.Prepare(16, d); seq != "" || len(jobs) != 0 {
		t.Errorf("nothing changed, yet %q, %d jobs", seq, len(jobs))
	}

	// A theme change draws it again in the new colours and drops the old.
	th := testTheme()
	th.Accent = color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}
	r.SetTheme(th)
	seq, jobs = r.Prepare(16, d)
	if len(jobs) != 1 || !strings.Contains(seq, kitty.Delete(1)) {
		t.Errorf("after a theme change: %d jobs, seq %q", len(jobs), seq)
	}
	if runAll(r, jobs) == "" || f.calls.Load() != 2 {
		t.Errorf("drawn %d times", f.calls.Load())
	}

	// Close deletes it.
	if c := r.Close(); !strings.Contains(c, kitty.Delete(2)) {
		t.Errorf("Close: %q", c)
	}
}

func TestDiagramFallback(t *testing.T) {
	f := &fakeDiagrams{w: 100, h: 100}
	src := "```mermaid\nflowchart broken\n```\n"

	r := diagramRenderer(f, graphicsCaps)
	d := r.Parse(src)
	_, jobs := r.Prepare(60, d)
	if runAll(r, jobs) != "" {
		t.Error("nothing to send for a failed diagram")
	}
	if out := ansi.Strip(r.Render(d, 60)); !strings.Contains(out, "flowchart broken") || !strings.Contains(out, "couldn't draw it: parse error") {
		t.Errorf("failed diagram:\n%s", out)
	}
	if _, jobs := r.Prepare(60, d); len(jobs) != 0 {
		t.Error("a failed diagram isn't tried again")
	}

	for name, r := range map[string]*Renderer{
		"no graphics": diagramRenderer(f, Caps{}),
		"NoDiagrams":  diagramRenderer(NoDiagrams, graphicsCaps),
	} {
		d := r.Parse(flowchart)
		if _, jobs := r.Prepare(60, d); len(jobs) != 0 {
			t.Errorf("%s: want no jobs, got %d", name, len(jobs))
		}
		if out := ansi.Strip(r.Render(d, 60)); !strings.Contains(out, "A --> B") || strings.Contains(out, "couldn't") {
			t.Errorf("%s: want the source alone:\n%s", name, out)
		}
	}
}

func TestDiagramRows(t *testing.T) {
	tall := &fakeDiagrams{w: 240, h: 48 * 100}
	r := diagramRenderer(tall, graphicsCaps)
	d := r.Parse(flowchart)
	_, jobs := r.Prepare(60, d)
	if seq := runAll(r, jobs); !strings.Contains(seq, ",r=40") {
		t.Errorf("a tall diagram stops at 40 rows: %q…", seq[:min(len(seq), 120)])
	}

	r = New(Options{Theme: testTheme(), Caps: graphicsCaps, NewID: counter(), Diagrams: tall, MaxDiagramRows: 10})
	d = r.Parse(flowchart)
	_, jobs = r.Prepare(60, d)
	if seq := runAll(r, jobs); !strings.Contains(seq, ",r=10") {
		t.Errorf("MaxDiagramRows 10: %q…", seq[:min(len(seq), 120)])
	}
}

func TestGoMermaid(t *testing.T) {
	th := DefaultTheme(true)
	img, err := GoMermaid().Draw(context.Background(), Diagram{Source: "flowchart LR\n    A[Write] --> B[Merge]", Theme: th, TextHeight: 48})
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	// Two boxes side by side, drawn for 48-pixel rows: a few rows tall
	// and wider than tall.
	if b.Dy() < 2*48 || b.Dy() > 10*48 || b.Dx() <= b.Dy() {
		t.Errorf("odd size %v", b)
	}
	if _, _, _, a := img.At(b.Min.X, b.Min.Y).RGBA(); a != 0 {
		t.Error("the background should be transparent")
	}

	if _, err := GoMermaid().Draw(context.Background(), Diagram{Source: "flowchart LR\n    A -->", Theme: th, TextHeight: 48}); err == nil {
		t.Error("want an error for a broken diagram")
	}
}
