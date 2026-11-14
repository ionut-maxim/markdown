package markdown

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ionut-maxim/markdown/kitty"
)

// fakeFetch serves local images: the test card for testcard URLs, a
// small square for square ones, and an error for anything else. It
// counts its calls.
type fakeFetch struct{ calls atomic.Int32 }

func (f *fakeFetch) fetch(_ context.Context, url string) ([]byte, error) {
	f.calls.Add(1)
	switch {
	case strings.Contains(url, "testcard"):
		return os.ReadFile("kitty/testdata/testcard.png")
	case strings.Contains(url, "square"):
		img := image.NewRGBA(image.Rect(0, 0, 64, 64))
		img.Set(1, 1, color.White)
		var b bytes.Buffer
		err := png.Encode(&b, img)
		return b.Bytes(), err
	case strings.Contains(url, "garbage"):
		return []byte("not an image"), nil
	}
	return nil, errors.New("404 not found")
}

func imageRenderer(f *fakeFetch) *Renderer {
	return New(Options{Theme: testTheme(), Caps: Caps{Graphics: true, Cell: kitty.Cell{W: 8, H: 16}}, NewID: counter(), Fetch: f.fetch})
}

// runAll runs the jobs and hands their results to Loaded, returning what
// Loaded sent.
func runAll(r *Renderer, jobs []Job) string {
	var seq strings.Builder
	for _, j := range jobs {
		seq.WriteString(r.Loaded(j.Run(context.Background())))
	}
	return seq.String()
}

func TestImageLifecycle(t *testing.T) {
	f := &fakeFetch{}
	r := imageRenderer(f)
	d := r.Parse("Look: ![a shot](https://img.example/testcard.png) and more text.\n\n![again](https://img.example/testcard.png)")

	seq, jobs := r.Prepare(60, d)
	if len(jobs) != 1 || jobs[0].URL != "https://img.example/testcard.png" {
		t.Fatalf("want one job for the one URL, got %+v", jobs)
	}
	if seq != "" {
		t.Errorf("nothing to send before loading: %q", seq)
	}
	if out := ansi.Strip(r.Render(d, 60)); !strings.Contains(out, "▣ a shot (loading…)") {
		t.Errorf("while loading:\n%s", out)
	}

	seq = runAll(r, jobs)
	if strings.Count(seq, "a=p") != 1 || !strings.Contains(seq, "c=60") {
		t.Fatalf("Loaded should send the picture 60 columns wide: %q…", seq[:min(len(seq), 80)])
	}
	out := r.Render(d, 60)
	if strings.Contains(out, "\x1b_G") {
		t.Fatal("Render sent graphics")
	}
	p := ansi.Strip(out)
	if strings.Contains(p, "▣") || !strings.Contains(p, "Look:  and more text.") {
		t.Errorf("text should leave the pictures out:\n%s", p)
	}
	// 640×360 at 60 columns of 8×16 cells: 17 rows, drawn twice.
	if n := strings.Count(out, "\n") + 1; n != 1+17+1+17 {
		t.Errorf("want text, picture, blank, picture: %d lines", n)
	}

	if seq, jobs := r.Prepare(60, d); seq != "" || len(jobs) != 0 {
		t.Errorf("unchanged Prepare: %q, %d jobs", seq, len(jobs))
	}
	// A resize sends it again at the new size, without fetching.
	seq, jobs = r.Prepare(40, d)
	if len(jobs) != 0 || !strings.Contains(seq, "c=40") || f.calls.Load() != 1 {
		t.Errorf("resize: %d jobs, %d fetches, %q…", len(jobs), f.calls.Load(), seq[:min(len(seq), 60)])
	}
	// Forget, then Prepare: sent again from memory.
	r.Forget()
	if seq, jobs := r.Prepare(40, d); len(jobs) != 0 || !strings.Contains(seq, "a=p") || f.calls.Load() != 1 {
		t.Errorf("after Forget: %d jobs, %d fetches", len(jobs), f.calls.Load())
	}
	// The document left: the picture is deleted.
	if seq, _ := r.Prepare(40); strings.Count(seq, "a=d") != 1 {
		t.Errorf("dropping the document: %q", seq)
	}
	if hasPlaceholder(r.Render(d, 40)) {
		t.Error("placeholders for a dropped image")
	}
}

func TestImageErrors(t *testing.T) {
	r := imageRenderer(&fakeFetch{})
	d := r.Parse("![gone](https://img.example/missing.png) ![bad](https://img.example/garbage.png)")
	_, jobs := r.Prepare(60, d)
	if seq := runAll(r, jobs); seq != "" {
		t.Errorf("nothing to send for failures: %q", seq)
	}
	out := ansi.Strip(r.Render(d, 120))
	for _, want := range []string{"▣ gone (couldn't show it: 404 not found)", "▣ bad (couldn't show it: decode image:"} {
		if !strings.Contains(out, want) {
			t.Errorf("render lacks %q:\n%s", want, out)
		}
	}
	// A failed image isn't fetched again on every Prepare.
	if _, jobs := r.Prepare(60, d); len(jobs) != 0 {
		t.Errorf("retried: %d jobs", len(jobs))
	}
}

func TestImageLateResult(t *testing.T) {
	r := imageRenderer(&fakeFetch{})
	d := r.Parse("![](https://img.example/square.png)")
	_, jobs := r.Prepare(60, d)
	r.Prepare(60) // the document left before the image came
	if seq := r.Loaded(jobs[0].Run(context.Background())); seq != "" {
		t.Errorf("a result for an image no longer wanted sent %q", seq)
	}
	// Asked for again: the old result doesn't count for the new entry.
	_, jobs2 := r.Prepare(60, d)
	if seq := r.Loaded(jobs[0].Run(context.Background())); seq != "" {
		t.Errorf("stale result sent %q", seq)
	}
	if seq := runAll(r, jobs2); !strings.Contains(seq, "a=p") {
		t.Error("the new job should send it")
	}
}

func TestImageNeedsFetchAndGraphics(t *testing.T) {
	src := "![shot](https://img.example/testcard.png)"
	f := &fakeFetch{}
	for name, r := range map[string]*Renderer{
		"no fetch":    New(Options{Theme: testTheme(), Caps: Caps{Graphics: true, Cell: kitty.Cell{W: 8, H: 16}}}),
		"no graphics": New(Options{Theme: testTheme(), Fetch: f.fetch}),
	} {
		d := r.Parse(src)
		if _, jobs := r.Prepare(60, d); len(jobs) != 0 {
			t.Errorf("%s: %d jobs", name, len(jobs))
		}
		if out := ansi.Strip(r.Render(d, 60)); strings.TrimSpace(out) != "▣ shot" {
			t.Errorf("%s: want a plain link, got %q", name, out)
		}
	}
	if f.calls.Load() != 0 {
		t.Error("fetched without graphics")
	}
}

func TestImageInListFits(t *testing.T) {
	r := imageRenderer(&fakeFetch{})
	d := r.Parse("- item\n\n  > ![](https://img.example/testcard.png)")
	_, jobs := r.Prepare(40, d)
	runAll(r, jobs)
	out := r.Render(d, 40)
	if !hasPlaceholder(out) {
		t.Fatalf("no picture:\n%s", ansi.Strip(out))
	}
	for _, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w > 40 {
			t.Errorf("line %d wide", w)
		}
	}
}

func TestImageCloseDeletes(t *testing.T) {
	r := imageRenderer(&fakeFetch{})
	d := r.Parse("![](https://img.example/testcard.png) ![](https://img.example/square.png)")
	_, jobs := r.Prepare(60, d)
	runAll(r, jobs)
	if seq := r.Close(); strings.Count(seq, "a=d") != 2 {
		t.Errorf("Close: %q", seq)
	}
}

// TestImageJobsConcurrently runs the jobs on their own goroutines, as an
// app does, with Render on this one (run with -race).
func TestImageJobsConcurrently(t *testing.T) {
	r := imageRenderer(&fakeFetch{})
	d := r.Parse("![](https://img.example/testcard.png) ![](https://img.example/square.png) ![](https://img.example/missing.png)")
	_, jobs := r.Prepare(60, d)
	results := make(chan Result)
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func() { defer wg.Done(); results <- j.Run(context.Background()) }()
	}
	go func() { wg.Wait(); close(results) }()
	for res := range results {
		r.Loaded(res)
		r.Render(d, 60)
	}
	if strings.Count(ansi.Strip(r.Render(d, 60)), "couldn't show it") != 1 {
		t.Error("want the missing one noted")
	}
}
