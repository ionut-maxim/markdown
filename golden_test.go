package markdown

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ionut-maxim/markdown/kitty"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata/golden")

// goldenWidths are the column widths every input is rendered at: a folded
// phone, a regular pane and a wide one.
var goldenWidths = []int{30, 60, 100}

// goldenCaps are the terminals every input is rendered for.
var goldenCaps = []struct {
	name string
	caps Caps
}{
	{"text", Caps{}},
	{"graphics", Caps{Graphics: true, Cell: kitty.Cell{W: 8, H: 16}}},
	{"textsizing", Caps{Graphics: true, TextSizing: true, Cell: kitty.Cell{W: 8, H: 16}, Width: ansi.GraphemeWidth}},
}

// TestGolden renders each testdata/golden/*.md for every terminal and
// width, and compares it with NAME.CAPS.ansi (the exact bytes) and
// NAME.CAPS.txt (the same without escape codes, easier to read in a
// diff). Run `go test -update` to rewrite them, then read the diff.
func TestGolden(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("testdata", "golden", "*.md"))
	if err != nil || len(inputs) == 0 {
		t.Fatalf("no inputs: %v", err)
	}
	for _, in := range inputs {
		src, err := os.ReadFile(in)
		if err != nil {
			t.Fatal(err)
		}
		base := strings.TrimSuffix(in, ".md")
		for _, c := range goldenCaps {
			t.Run(filepath.Base(base)+"/"+c.name, func(t *testing.T) {
				r := New(Options{Theme: DefaultTheme(true), Caps: c.caps, NewID: counter(), Fetch: goldenFetch, Diagrams: goldenDiagrams{}})
				doc := r.Parse(string(src))
				var out strings.Builder
				for _, w := range goldenWidths {
					_, jobs := r.Prepare(w, doc)
					runAll(r, jobs)
					s := r.Render(doc, w)
					for i, l := range strings.Split(s, "\n") {
						if lw := cellWidth(l, c.caps.Width); lw > w {
							t.Errorf("width %d, line %d is %d wide: %q", w, i+1, lw, ansi.Strip(l))
						}
					}
					if strings.Contains(s, "\x1b_G") {
						t.Errorf("width %d: Render sent graphics", w)
					}
					fmt.Fprintf(&out, "=== width %d ===\n%s\n", w, s)
				}
				check(t, base+"."+c.name+".ansi", out.String())
				check(t, base+"."+c.name+".txt", ansi.Strip(showSized(out.String())))
			})
		}
	}
}

// osc66 matches a kitty text sizing sequence: its metadata and text.
var osc66 = regexp.MustCompile("\x1b\\]66;([^;]*);([^\a\x1b]*)(?:\a|\x1b\\\\)")

// cellWidth is how many cells a line takes, measured with m, counting
// text drawn with OSC 66 at its scale (StringWidth counts it as nothing).
func cellWidth(line string, m ansi.Method) int {
	w := 0
	for _, sz := range osc66.FindAllStringSubmatch(line, -1) {
		scale := 1
		for _, kv := range strings.Split(sz[1], ":") {
			if v, ok := strings.CutPrefix(kv, "s="); ok {
				scale, _ = strconv.Atoi(v)
			}
		}
		w += scale * m.StringWidth(sz[2])
	}
	return w + m.StringWidth(osc66.ReplaceAllString(line, ""))
}

// showSized writes OSC 66 text as ⟦meta|text⟧, so the plain golden files
// show it.
func showSized(s string) string { return osc66.ReplaceAllString(s, "⟦$1|$2⟧") }

// goldenFetch serves the test card for any .png and fails for anything
// else, so the image golden files show both.
func goldenFetch(_ context.Context, url string) ([]byte, error) {
	if strings.HasSuffix(url, ".png") {
		return os.ReadFile("kitty/testdata/testcard.png")
	}
	return nil, errors.New("404 not found")
}

// goldenDiagrams draws every diagram as a 400×200 picture, two rows of
// text tall per 100 pixels at TextHeight 50, and fails for sources that
// say "broken": the golden files then show pictures and the fallback
// without depending on go-mermaid's drawing.
type goldenDiagrams struct{}

func (goldenDiagrams) Draw(_ context.Context, d Diagram) (image.Image, error) {
	if strings.Contains(d.Source, "broken") {
		return nil, errors.New("mermaid: parse error: line 1: unexpected token")
	}
	return image.NewRGBA(image.Rect(0, 0, 400, 200)), nil
}

func check(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if string(want) != got {
		t.Errorf("%s differs from the output (go test -update rewrites it):\n%s", path, firstDiff(string(want), got))
	}
}

// firstDiff shows the first line that differs.
func firstDiff(want, got string) string {
	wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < max(len(wl), len(gl)); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			return fmt.Sprintf("line %d\nwant %q\ngot  %q", i+1, w, g)
		}
	}
	return "(same lines, different ends)"
}
