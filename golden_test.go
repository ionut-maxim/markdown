package markdown

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
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
				r := New(Options{Theme: DefaultTheme(true), Caps: c.caps, NewID: counter()})
				doc := r.Parse(string(src))
				var out strings.Builder
				for _, w := range goldenWidths {
					r.Prepare(w, doc)
					s := r.Render(doc, w)
					for i, l := range strings.Split(s, "\n") {
						if lw := ansi.StringWidth(l); lw > w {
							t.Errorf("width %d, line %d is %d wide: %q", w, i+1, lw, ansi.Strip(l))
						}
					}
					if strings.Contains(s, "\x1b_G") {
						t.Errorf("width %d: Render sent graphics", w)
					}
					fmt.Fprintf(&out, "=== width %d ===\n%s\n", w, s)
				}
				check(t, base+"."+c.name+".ansi", out.String())
				check(t, base+"."+c.name+".txt", ansi.Strip(out.String()))
			})
		}
	}
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
