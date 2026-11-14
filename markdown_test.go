package markdown

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	xkitty "github.com/charmbracelet/x/ansi/kitty"
	"github.com/yuin/goldmark/ast"

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
	// Too wide: the cells wrap inside their columns, words kept whole,
	// with rules between rows that wrapped.
	out = plain("| A long header | Another long header |\n|---|---|\n| some value | other value |\n| x | y |", 20)
	for _, want := range []string{"│ A long │ Another │", "│ header │ long    │", "│ some   │ other   │", "├────────┼─────────┤\n│ x"} {
		if !strings.Contains(out, want) {
			t.Errorf("wrapped grid lacks %q:\n%s", want, out)
		}
	}
	// Words too long for even that: cards.
	out = plain("| Configuration | Documentation |\n|---|---|\n| some value | other value |", 20)
	if strings.Contains(out, "╭") || !strings.Contains(out, "▎ Configuration\n▎   some value") {
		t.Fatalf("cards:\n%s", out)
	}
}

func TestFitColumns(t *testing.T) {
	cases := []struct {
		natural, least []int
		avail          int
		want           []int
	}{
		{[]int{5, 10}, []int{5, 6}, 20, []int{5, 10}},        // fits as is
		{[]int{10, 30}, []int{6, 6}, 20, []int{7, 13}},       // shares by need: 4 and 24 of 8
		{[]int{10, 30}, []int{6, 16}, 22, []int{6, 16}},      // exactly the least
		{[]int{10, 30}, []int{6, 16}, 21, nil},               // not even the least: cards
		{[]int{8, 8, 8}, []int{6, 6, 6}, 20, []int{7, 7, 6}}, // rounding goes left to right
	}
	for _, c := range cases {
		got := fitColumns(c.natural, c.least, c.avail)
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("fitColumns(%v, %v, %d) = %v, want %v", c.natural, c.least, c.avail, got, c.want)
		}
	}
}

func TestWidthMethod(t *testing.T) {
	// Rex counts ❤️ as one cell (each code point), Ghostty as two (the
	// grapheme). Every line of the grid must be equally wide as the
	// terminal measures it, or the borders don't line up.
	src := "| A | B |\n|---|---|\n| ❤️ | x |"
	for m, want := range map[ansi.Method]int{ansi.WcWidth: 9, ansi.GraphemeWidth: 10} {
		r := New(Options{Theme: testTheme(), Caps: Caps{Width: m}})
		for _, l := range strings.Split(ansi.Strip(r.Render(r.Parse(src), 40)), "\n") {
			if w := m.StringWidth(l); w != want {
				t.Errorf("method %d: line %q is %d wide, want %d", m, l, w, want)
			}
		}
	}
}

func TestInlineCodePadding(t *testing.T) {
	out := plain("Run `go test`. Then `vet` it", 40)
	if !strings.Contains(out, "\u00a0go test.") || !strings.Contains(out, "\u00a0vet\u00a0 it") {
		t.Errorf("padding: %q", out)
	}
	// A line never breaks inside the padding.
	for _, l := range strings.Split(plain("aaaa bbbb cccc `code` dddd", 16), "\n") {
		if strings.HasSuffix(l, "\u00a0") && !strings.HasSuffix(l, "code\u00a0") {
			t.Errorf("broke inside the padding: %q", l)
		}
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
	seq := prep(r, 60, d)
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

	if again := prep(r, 60, d); again != "" {
		t.Errorf("second Prepare sent %q", again)
	}

	// A new width: the old pictures go, new ones come.
	seq = prep(r, 40, d)
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
		if seq := prep(r, 60, d); seq != "" {
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
	prep(r, 60, a, b)
	if !hasPlaceholder(r.Render(a, 60)) || !hasPlaceholder(r.Render(b, 60)) {
		t.Fatal("both documents should have their pictures")
	}
	// b left the screen: only its picture goes.
	if seq := prep(r, 60, a); strings.Count(seq, "a=d") != 1 || strings.Contains(seq, "a=p") {
		t.Errorf("dropping a document: %q", seq)
	}
	if !hasPlaceholder(r.Render(a, 60)) || hasPlaceholder(r.Render(b, 60)) {
		t.Error("a keeps its picture, b loses it")
	}
}

func TestHeadingPictureTooWide(t *testing.T) {
	r := graphics(testTheme())
	d := r.Parse("# Pneumonoultramicroscopicsilicovolcanoconiosis")
	if seq := prep(r, 16, d); seq != "" {
		t.Errorf("a word too wide for the width sent %q", seq)
	}
	if prep(r, 16, d) != "" {
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
		prep(r, 30, d)
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
	prep(r, 60, d)
	dark := r.Render(d, 60)
	r.SetTheme(DefaultTheme(false))
	seq := prep(r, 60, d)
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
	prep(r, 60, d)
	text := r.Render(d, 60)
	r.SetCaps(Caps{Graphics: true, Cell: kitty.Cell{W: 8, H: 16}})
	if prep(r, 60, d) == "" || r.Render(d, 60) == text {
		t.Error("caps reported later should bring pictures")
	}
	r.SetCaps(Caps{})
	if seq := prep(r, 60, d); !strings.Contains(seq, "a=d") {
		t.Errorf("losing graphics should delete the pictures: %q", seq)
	}
}

func TestTextSizingHeadings(t *testing.T) {
	r := New(Options{Theme: testTheme(), Caps: Caps{Graphics: true, TextSizing: true, Cell: kitty.Cell{W: 8, H: 16}}, NewID: counter()})
	d := r.Parse("# Big\n\n## Smaller\n\n### Small")
	if seq := prep(r, 60, d); seq != "" {
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

// TestAlertRules runs the cases checked against GitHub's own renderer
// (gh api markdown, mode gfm) on Nov 14, 2026.
func TestAlertRules(t *testing.T) {
	cases := []struct {
		name, src string
		kind      AlertKind
		alert     bool
	}{
		{"uppercase", "> [!NOTE]\n> text", AlertNote, true},
		{"lowercase", "> [!note]\n> text", AlertNote, true},
		{"mixed case", "> [!Tip]\n> text", AlertTip, true},
		{"trailing spaces", "> [!NOTE]   \n> text", AlertNote, true},
		{"blank line after marker", "> [!NOTE]\n>\n> text", AlertNote, true},
		{"lazy continuation", "> [!CAUTION]\ntext lazily continued", AlertCaution, true},
		{"indented quote", "   > [!IMPORTANT]\n   > text", AlertImportant, true},
		{"warning", "> [!WARNING]\n> text", AlertWarning, true},
		{"text on marker line", "> [!NOTE] text here", 0, false},
		{"text after marker, more lines", "> [!WARNING] text\n> more", 0, false},
		{"marker only", "> [!NOTE]", 0, false},
		{"unknown kind", "> [!DANGER]\n> text", 0, false},
		{"in a list", "- item\n\n  > [!NOTE]\n  > text", 0, false},
		{"nested in a quote", "> outer\n>\n> > [!NOTE]\n> > text", 0, false},
		{"marker on second line", "> text\n> [!NOTE]", 0, false},
		{"space inside", "> [! NOTE]\n> text", 0, false},
		{"emphasised marker", "> *[!NOTE]*\n> text", 0, false},
	}
	r := New(Options{Theme: testTheme()})
	for _, c := range cases {
		d := r.Parse(c.src)
		var found *alert
		_ = ast.Walk(d.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if a, ok := n.(*alert); ok && entering {
				found = a
			}
			return ast.WalkContinue, nil
		})
		switch {
		case c.alert && found == nil:
			t.Errorf("%s: want a %s alert, got a quote", c.name, c.kind)
		case c.alert && found.kind != c.kind:
			t.Errorf("%s: kind %s, want %s", c.name, found.kind, c.kind)
		case !c.alert && found != nil:
			t.Errorf("%s: want a quote, got a %s alert", c.name, found.kind)
		}
		if out := ansi.Strip(r.Render(d, 40)); c.alert && strings.Contains(strings.ToUpper(out), "[!") {
			t.Errorf("%s: marker left in:\n%s", c.name, out)
		}
	}
}

func TestAlertDrawing(t *testing.T) {
	out := plain("> [!WARNING]\n> Some **bold** text.\n>\n> Second paragraph.", 40)
	want := "▎ ⚠ Warning\n▎ Some bold text.\n▎\n▎ Second paragraph."
	if strings.TrimRight(out, " ") != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	th := testTheme()
	th.AlertIcons = map[AlertKind]string{AlertWarning: "W"}
	if out := ansi.Strip(render("> [!WARNING]\n> x\n\n> [!TIP]\n> y", 40, th)); !strings.Contains(out, "W Warning") || !strings.Contains(out, "✦ Tip") {
		t.Errorf("own icon, default for the rest:\n%s", out)
	}
}

func TestAlertPictures(t *testing.T) {
	r := graphics(DefaultTheme(true))
	d := r.Parse("> [!NOTE]\n> one\n\n> [!NOTE]\n> two\n\n> [!WARNING]\n> three\n\n> plain quote")
	if hasPlaceholder(r.Render(d, 40)) {
		t.Fatal("pictures before Prepare")
	}
	// Two kinds: an icon and a bar each; the second note reuses them.
	seq := prep(r, 40, d)
	if n := strings.Count(seq, "a=p"); n != 4 {
		t.Fatalf("want 4 pictures, got %d", n)
	}
	out := r.Render(d, 40)
	p := ansi.Strip(out)
	if strings.Contains(p, "ℹ") || strings.Contains(p, "⚠") || !strings.Contains(p, "Note") || !strings.Contains(p, "Warning") {
		t.Errorf("icons should be pictures, titles text:\n%s", p)
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(ansi.Strip(l), "plain quote") && (hasPlaceholder(l) || !strings.Contains(l, "▎")) {
			t.Errorf("plain quotes keep the text bar: %q", l)
		}
	}
	// A resize keeps them: they don't depend on the width.
	if seq := prep(r, 30, d); strings.Contains(seq, "a=p") || strings.Contains(seq, "a=d") {
		t.Errorf("resize resent glyphs: %q", seq)
	}
	// The warning left: its two go.
	if seq := prep(r, 30, r.Parse("> [!NOTE]\n> one")); strings.Count(seq, "a=d") != 2 {
		t.Errorf("dropping the warning: %q", seq)
	}
	if seq := r.Close(); strings.Count(seq, "a=d") != 2 {
		t.Errorf("Close: %q", seq)
	}
	if hasPlaceholder(r.Render(d, 40)) {
		t.Error("placeholders after Close")
	}
}

func TestAlertPicturesNeedGraphics(t *testing.T) {
	r := New(Options{Theme: testTheme(), Caps: Caps{Graphics: true}}) // cell size unknown
	d := r.Parse("> [!TIP]\n> x")
	if seq := prep(r, 40, d); seq != "" {
		t.Errorf("Prepare sent %q", seq)
	}
	if out := ansi.Strip(r.Render(d, 40)); !strings.Contains(out, "▎ ✦ Tip") {
		t.Errorf("text fallback:\n%s", out)
	}
	// Text sizing doesn't stop alert pictures.
	r = New(Options{Theme: testTheme(), Caps: Caps{Graphics: true, TextSizing: true, Cell: kitty.Cell{W: 8, H: 16}}, NewID: counter()})
	if seq := prep(r, 40, r.Parse("> [!TIP]\n> x")); strings.Count(seq, "a=p") != 2 {
		t.Errorf("text sizing: %q", seq)
	}
}

func TestForget(t *testing.T) {
	r := graphics(DefaultTheme(true))
	d := r.Parse("# Title\n\n> [!NOTE]\n> x")
	first := prep(r, 40, d)
	if prep(r, 40, d) != "" {
		t.Fatal("nothing should be resent")
	}
	r.Forget()
	if hasPlaceholder(r.Render(d, 40)) {
		t.Error("placeholders for forgotten pictures")
	}
	again := prep(r, 40, d)
	if strings.Contains(again, "a=d") || strings.Count(again, "a=p") != strings.Count(first, "a=p") {
		t.Errorf("after Forget: %d placements, %d deletes; want %d and none",
			strings.Count(again, "a=p"), strings.Count(again, "a=d"), strings.Count(first, "a=p"))
	}
	if r.Close() == "" {
		t.Error("Close should delete what was sent again")
	}
}

// prep is Prepare without the jobs.
func prep(r *Renderer, width int, docs ...*Doc) string {
	seq, _ := r.Prepare(width, docs...)
	return seq
}
