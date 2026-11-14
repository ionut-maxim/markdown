package markdown

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	xkitty "github.com/charmbracelet/x/ansi/kitty"

	"github.com/ionut-maxim/markdown/kitty"
)

// testTheme draws everything in one grey, so tests read plain text.
func testTheme() Theme {
	c := lipgloss.Color("#888888")
	return Theme{Dark: true, Text: c, Subtle: c, Muted: c, Accent: c, Accent2: c, Rule: c, Surface: c, SurfaceHi: c,
		Green: c, Red: c, Yellow: c, Blue: c, Bullets: []string{"•", "◦"}, CheckOn: "☑", CheckOff: "☐", Quote: "▎", ImageIcon: "▣"}
}

// counter gives ids 1, 2, 3…
func counter() func() int {
	n := 0
	return func() int { n++; return n }
}

func render(src string, width int, th Theme) string {
	r := New(Options{Theme: th})
	return r.Render(r.Parse(src), width)
}

func plain(src string, width int) string { return ansi.Strip(render(src, width, testTheme())) }

func TestRenderWraps(t *testing.T) {
	out := plain("# Title\n\nsome **bold** text that is long enough to wrap around at a narrow width", 30)
	if !strings.Contains(out, "Title") || strings.Contains(out, "#") || !strings.Contains(out, "bold") || !strings.Contains(out, "━━━") {
		t.Fatalf("render:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(line); w > 30 {
			t.Fatalf("line wider than 30 (%d): %q", w, line)
		}
	}
}

func TestRenderBlocks(t *testing.T) {
	src := "1. one\n2. two\n   - nested\n\n- [x] done\n- [ ] todo\n\n> quoted\n\n```go\nx := 1\n```\n\n---"
	out := plain(src, 40)
	for _, want := range []string{"1. one", "2. two", "   ◦ nested", "☑ done", "☐ todo", "▎ quoted", "go", "x := 1", strings.Repeat("─", 40)} {
		if !strings.Contains(out, want) {
			t.Errorf("render lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[x]") || strings.Contains(out, "```") {
		t.Errorf("markdown syntax left in:\n%s", out)
	}
}

func TestRenderHighlightsCode(t *testing.T) {
	out := render("```go\nfunc main() {}\n```", 40, DefaultTheme(true))
	// Keywords get their own colour: more than one foreground in the line.
	line := strings.Split(out, "\n")[1]
	if strings.Count(line, "38;2;") < 2 {
		t.Fatalf("no highlighting: %q", line)
	}
}

func TestCodeStyle(t *testing.T) {
	src := "```go\nfunc main() {}\n```"
	mocha := render(src, 40, DefaultTheme(true))
	th := DefaultTheme(true)
	th.CodeStyle = "monokai"
	if render(src, 40, th) == mocha {
		t.Error("CodeStyle didn't change the colours")
	}
	if render(src, 40, DefaultTheme(false)) == mocha {
		t.Error("a light theme should use a light code style")
	}
}

func TestRenderTables(t *testing.T) {
	src := "| Key | Action |\n|---|--:|\n| d | done |\n| a | answer |"
	out := plain(src, 40)
	if !strings.Contains(out, "╭") || !strings.Contains(out, "│ d   │   done │") {
		t.Fatalf("grid:\n%s", out)
	}
	// Too narrow for the grid: cards.
	out = plain("| A long header | Another long header |\n|---|---|\n| some value | other value |", 20)
	if strings.Contains(out, "╭") || !strings.Contains(out, "▎ A long header\n▎   some value") {
		t.Fatalf("cards:\n%s", out)
	}
}

func TestRenderLinks(t *testing.T) {
	out := render("see [docs](https://e.example/x)", 40, testTheme())
	if !strings.Contains(out, "\x1b]8;;https://e.example/x") || !strings.Contains(ansi.Strip(out), "see docs") {
		t.Fatalf("link: %q", out)
	}
}

func TestRenderEscapes(t *testing.T) {
	out := plain(`Food (\~7k/month now) and Gaming (\~1.5k/month now) \*not bold\* Tom &amp; Jerry &#169; `+"`a\\~b`", 80)
	for _, want := range []string{"Food (~7k/month now) and Gaming (~1.5k/month now)", "*not bold*", "Tom & Jerry ©", `a\~b`} {
		if !strings.Contains(out, want) {
			t.Errorf("render lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, `(\~`) {
		t.Errorf("escape left in:\n%s", out)
	}
}

func TestRenderImagesAsLinks(t *testing.T) {
	out := render("Look: ![a shot](https://img.example/a.png) and ![](https://img.example/b.png)", 60, testTheme())
	if !strings.Contains(out, "\x1b]8;;https://img.example/a.png") {
		t.Errorf("no link to the image: %q", out)
	}
	p := ansi.Strip(out)
	if !strings.Contains(p, "▣ a shot") || !strings.Contains(p, "▣ image") {
		t.Errorf("image links:\n%s", p)
	}
}

func TestImages(t *testing.T) {
	r := New(Options{})
	d := r.Parse("a ![x *y*](https://u/1) b ![](https://u/2 \"title\")\n\n```\n![not an image](https://u/3)\n```")
	im := d.Images()
	if len(im) != 2 || im[0] != (Image{Alt: "x y", URL: "https://u/1"}) || im[1] != (Image{URL: "https://u/2"}) {
		t.Fatalf("images = %+v", im)
	}
}

func TestBulletsFallback(t *testing.T) {
	th := testTheme()
	th.Bullets = nil
	if out := ansi.Strip(render("- a\n  - b", 30, th)); !strings.Contains(out, "• a") || !strings.Contains(out, "• b") {
		t.Errorf("no bullets in the theme:\n%s", out)
	}
}

// graphics is a Renderer for a terminal with kitty graphics and Rex's
// nominal cell size.
func graphics(th Theme) *Renderer {
	return New(Options{Theme: th, Caps: Caps{Graphics: true, Cell: kitty.Cell{W: 8, H: 16}}, NewID: counter()})
}

func hasPlaceholder(s string) bool { return strings.ContainsRune(s, xkitty.Placeholder) }

func TestHeadingPictures(t *testing.T) {
	r := graphics(testTheme())
	d := r.Parse("# Big title\n\ntext\n\n## Smaller\n\n### Stays text")

	if out := r.Render(d, 60); hasPlaceholder(out) {
		t.Fatal("pictures drawn before Prepare")
	}
	seq := r.Prepare(60, d)
	if strings.Count(seq, "a=p") != 2 {
		t.Fatalf("want 2 placements (levels 1 and 2), got %q", seq)
	}
	out := r.Render(d, 60)
	if !hasPlaceholder(out) {
		t.Fatal("no placeholders after Prepare")
	}
	if strings.Contains(out, "\x1b_G") {
		t.Fatal("Render sent graphics")
	}
	if p := ansi.Strip(out); strings.Contains(p, "Big title") || strings.Contains(p, "Smaller") || !strings.Contains(p, "Stays text") {
		t.Errorf("levels 1 and 2 should be pictures, 3 text:\n%s", p)
	}

	if again := r.Prepare(60, d); again != "" {
		t.Errorf("second Prepare sent %q", again)
	}

	// A new width: the old pictures go, new ones come.
	seq = r.Prepare(40, d)
	if strings.Count(seq, "a=d") != 2 || strings.Count(seq, "a=p") != 2 {
		t.Errorf("resize: %q", seq)
	}

	// Close deletes them, and headings are text again.
	seq = r.Close()
	if strings.Count(seq, "a=d") != 2 {
		t.Errorf("Close: %q", seq)
	}
	if hasPlaceholder(r.Render(d, 40)) {
		t.Error("placeholders after Close")
	}
	if r.Close() != "" {
		t.Error("second Close sent something")
	}
}

func TestHeadingPicturesNeedCaps(t *testing.T) {
	for name, caps := range map[string]Caps{
		"no graphics":  {Cell: kitty.Cell{W: 8, H: 16}},
		"cell unknown": {Graphics: true},
	} {
		r := New(Options{Theme: testTheme(), Caps: caps, NewID: counter()})
		d := r.Parse("# Title")
		if seq := r.Prepare(60, d); seq != "" {
			t.Errorf("%s: Prepare sent %q", name, seq)
		}
		if hasPlaceholder(r.Render(d, 60)) {
			t.Errorf("%s: placeholders", name)
		}
	}
}

func TestHeadingPicturesAcrossDocs(t *testing.T) {
	r := graphics(testTheme())
	a, b := r.Parse("# One"), r.Parse("# Two")
	r.Prepare(60, a, b)
	if !hasPlaceholder(r.Render(a, 60)) || !hasPlaceholder(r.Render(b, 60)) {
		t.Fatal("both documents should have their pictures")
	}
	// b left the screen: only its picture goes.
	if seq := r.Prepare(60, a); strings.Count(seq, "a=d") != 1 || strings.Contains(seq, "a=p") {
		t.Errorf("dropping a document: %q", seq)
	}
	if !hasPlaceholder(r.Render(a, 60)) || hasPlaceholder(r.Render(b, 60)) {
		t.Error("a keeps its picture, b loses it")
	}
}

func TestHeadingPictureTooWide(t *testing.T) {
	r := graphics(testTheme())
	d := r.Parse("# Pneumonoultramicroscopicsilicovolcanoconiosis")
	if seq := r.Prepare(16, d); seq != "" {
		t.Errorf("a word too wide for the width sent %q", seq)
	}
	if r.Prepare(16, d) != "" {
		t.Error("tried again")
	}
	if !strings.Contains(ansi.Strip(r.Render(d, 16)), "Pneumono") {
		t.Error("should stay text")
	}
	// Pictures are made for the full width; inside a quote there's less
	// room. Whatever is drawn there must still fit.
	for _, src := range []string{"> # Quoted heading that is long", "> # Quotedheadingthatislong", "- # Heading in a list item"} {
		r = graphics(testTheme())
		d = r.Parse(src)
		r.Prepare(30, d)
		for _, l := range strings.Split(r.Render(d, 30), "\n") {
			if w := ansi.StringWidth(l); w > 30 {
				t.Errorf("%q: line %d wide: %q", src, w, ansi.Strip(l))
			}
		}
	}
}

func TestSetThemeRedraws(t *testing.T) {
	r := graphics(DefaultTheme(true))
	d := r.Parse("# Title")
	r.Prepare(60, d)
	dark := r.Render(d, 60)
	r.SetTheme(DefaultTheme(false))
	seq := r.Prepare(60, d)
	if !strings.Contains(seq, "a=d") || !strings.Contains(seq, "a=p") {
		t.Errorf("new theme: %q", seq)
	}
	if r.Render(d, 60) == dark {
		t.Error("render didn't change with the theme")
	}
}

func TestSetCaps(t *testing.T) {
	r := New(Options{Theme: testTheme(), NewID: counter()})
	d := r.Parse("# Title")
	r.Prepare(60, d)
	text := r.Render(d, 60)
	r.SetCaps(Caps{Graphics: true, Cell: kitty.Cell{W: 8, H: 16}})
	if r.Prepare(60, d) == "" || r.Render(d, 60) == text {
		t.Error("caps reported later should bring pictures")
	}
	r.SetCaps(Caps{})
	if seq := r.Prepare(60, d); !strings.Contains(seq, "a=d") {
		t.Errorf("losing graphics should delete the pictures: %q", seq)
	}
}

func TestTextSizingHeadings(t *testing.T) {
	r := New(Options{Theme: testTheme(), Caps: Caps{Graphics: true, TextSizing: true, Cell: kitty.Cell{W: 8, H: 16}}, NewID: counter()})
	d := r.Parse("# Big\n\n## Smaller\n\n### Small")
	if seq := r.Prepare(60, d); seq != "" {
		t.Errorf("text sizing needs no pictures, Prepare sent %q", seq)
	}
	out := r.Render(d, 60)
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[0], "\x1b]66;s=2;Big\a") || lines[1] != "" {
		t.Errorf("level 1: %q then %q (the row under big text must stay empty)", lines[0], lines[1])
	}
	if !strings.Contains(out, "\x1b]66;s=2:n=3:d=4:v=2;Smaller\a") {
		t.Errorf("level 2 not sized: %q", out)
	}
	if strings.Contains(out, "66;s=2;Small\a") || !strings.Contains(ansi.Strip(out), "Small") {
		t.Errorf("level 3 should be plain text: %q", out)
	}
	if hasPlaceholder(out) {
		t.Error("placeholders with text sizing")
	}
}

func TestTextSizingWithoutGraphics(t *testing.T) {
	r := New(Options{Theme: testTheme(), Caps: Caps{TextSizing: true}})
	if out := r.Render(r.Parse("# Title"), 40); !strings.Contains(out, "\x1b]66;s=2;Title\a") {
		t.Errorf("text sizing doesn't need graphics: %q", out)
	}
}
