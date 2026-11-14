package markdown

import (
	"context"
	"image/color"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"golang.org/x/image/font/opentype"

	"github.com/ionut-maxim/markdown/bigtext"
	"github.com/ionut-maxim/markdown/kitty"
)

// Caps is what the terminal can do.
type Caps struct {
	// Graphics is true when the terminal draws kitty graphics with
	// Unicode placeholders (kitty.Query answers OK).
	Graphics bool
	// Cell is the terminal's cell size in pixels (CSI 16 t). Big
	// headings are drawn only once it's known.
	Cell kitty.Cell
	// TextSizing is true when the terminal draws big text itself with
	// kitty's OSC 66 (kitty.TextSizingQuery). Headings then use it rather
	// than pictures. Leave it off under Bubble Tea: its renderer drops
	// OSC 66 along with the text inside.
	TextSizing bool
	// Width is how the terminal measures text: ansi.WcWidth (the zero
	// value) counts each code point, as most terminals and Rex do;
	// ansi.GraphemeWidth counts grapheme clusters, for terminals that
	// report Unicode mode 2027 (kitty, Ghostty). Bubble Tea makes the same
	// choice; use the one it uses.
	Width ansi.Method
}

// graphics is true when pictures can be drawn: kitty graphics, and the
// cell size known.
func (c Caps) graphics() bool { return c.Graphics && c.Cell.W > 0 && c.Cell.H > 0 }

// pictures is true when headings are drawn as pictures: text sizing
// draws them bigger itself.
func (c Caps) pictures() bool { return c.graphics() && !c.TextSizing }

// Options configure a Renderer.
type Options struct {
	Theme Theme
	Caps  Caps
	// HeadingFont draws big headings; nil means bigtext's embedded
	// JetBrains Mono Bold.
	HeadingFont *opentype.Font
	// NewID gives kitty image ids; nil means kitty.NewID. Tests set it to
	// get the same output every run.
	NewID func() int
	// Fetch gets an image's bytes (PNG, JPEG, GIF or WebP) for Jobs to
	// decode, with whatever login or URL rules the app has. The library
	// never fetches anything itself; nil shows images as links.
	Fetch func(ctx context.Context, url string) ([]byte, error)
	// MaxImageRows is the tallest an image is drawn, in rows; 0 means 20.
	MaxImageRows int
}

// Renderer renders markdown documents and owns the pictures it sends to
// the terminal for them.
//
// Render has no side effects, so it's safe to call from a Bubble Tea
// View on every frame. Pictures are sent only by what Prepare returns,
// and deleted by what Close returns. A Renderer is not safe for use from
// more than one goroutine.
type Renderer struct {
	opt      Options
	md       goldmark.Markdown
	headings map[headingKey]*picture
	glyphs   map[glyphKey]*picture
	images   map[string]*imageEntry
	gen      int // bumped whenever what Render draws may change
}

// headingKey identifies a heading picture across Prepare calls.
type headingKey struct {
	level int
	text  string
	color [4]uint32
}

// picture is an image sent to the terminal.
type picture struct {
	id         int
	cols, rows int
	maxCols    int // the width it was made for
}

// New returns a Renderer.
func New(opt Options) *Renderer {
	if opt.NewID == nil {
		opt.NewID = kitty.NewID
	}
	return &Renderer{
		opt: opt,
		md: goldmark.New(goldmark.WithExtensions(extension.GFM),
			goldmark.WithParserOptions(parser.WithASTTransformers(util.Prioritized(alertTransformer{}, 1000)))),
		headings: map[headingKey]*picture{},
		glyphs:   map[glyphKey]*picture{},
		images:   map[string]*imageEntry{},
	}
}

// SetTheme changes the theme, for instance once the terminal reports its
// background. Call Prepare afterwards to redraw the pictures.
func (r *Renderer) SetTheme(th Theme) { r.opt.Theme = th; r.gen++ }

// SetCaps changes what the terminal can do, for instance once it has
// answered the queries. Call Prepare afterwards.
func (r *Renderer) SetCaps(c Caps) { r.opt.Caps = c; r.gen++ }

// Doc is a parsed markdown document. Parse it once and render it as often
// as needed; it keeps the last output for each width.
type Doc struct {
	src   []byte
	root  ast.Node
	gen   int
	cache map[int]string
}

// Parse parses markdown text.
func (r *Renderer) Parse(src string) *Doc {
	b := []byte(src)
	return &Doc{src: b, root: r.md.Parser().Parse(text.NewReader(b))}
}

// Image is a markdown image: ![Alt](URL).
type Image struct {
	Alt string
	URL string
}

// Images returns the document's images, in order.
func (d *Doc) Images() []Image {
	var out []Image
	_ = ast.Walk(d.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if im, ok := n.(*ast.Image); ok && entering {
			out = append(out, Image{Alt: plainText(im, d.src), URL: string(im.Destination)})
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return out
}

// headings lists the document's level 1 and 2 headings.
func (d *Doc) headings() []headingKey {
	var out []headingKey
	_ = ast.Walk(d.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if h, ok := n.(*ast.Heading); ok && entering {
			if h.Level <= 2 {
				out = append(out, headingKey{level: h.Level, text: plainText(h, d.src)})
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return out
}

func colorKey(c color.Color) [4]uint32 {
	if c == nil {
		return [4]uint32{}
	}
	r, g, b, a := c.RGBA()
	return [4]uint32{r, g, b, a}
}

// Prepare makes the pictures the documents need at width columns and
// returns the escape sequences that send them to the terminal, and that
// delete the pictures no longer needed. Pass every document on screen:
// pictures of documents left out are deleted. Call it from Update when
// the documents, the width, the theme or the caps change, and send what
// it returns (with tea.Raw in Bubble Tea). It returns "" when nothing
// changed.
//
// It also returns a Job for each image not fetched yet (only with
// Options.Fetch and kitty graphics). Run them off the UI goroutine and
// pass each Result to Loaded.
//
// Pictures belong to the screen they were sent on: the main screen and
// the alternate one keep their own, and entering the alternate screen or
// clearing it drops them. Send what Prepare returns once the program is
// on the screen it draws to (in Bubble Tea, after the first frame), and
// after a switch or a clear call Forget, then Prepare again.
func (r *Renderer) Prepare(width int, docs ...*Doc) (string, []Job) {
	width = max(width, minWidth)
	// The headings wanted, in document order so ids are given out in
	// the same order every time.
	want := map[headingKey]bool{}
	var order []headingKey
	if r.opt.Caps.pictures() {
		for _, d := range docs {
			for _, h := range d.headings() {
				h.color = colorKey(r.opt.Theme.headingColor(h.level))
				if !want[h] {
					want[h] = true
					order = append(order, h)
				}
			}
		}
	}
	var gone []int
	for k, p := range r.headings {
		if !want[k] || p.maxCols != width {
			if p.cols > 0 {
				gone = append(gone, p.id)
			}
			delete(r.headings, k)
			r.gen++
		}
	}
	glyphs := r.wantGlyphs(docs)
	gone = append(gone, r.dropGlyphs(glyphs)...)
	imgGone, imgSeq, jobs := r.prepareImages(width, docs)
	gone = append(gone, imgGone...)
	var seq strings.Builder
	seq.WriteString(deletes(gone))
	for _, k := range order {
		if _, ok := r.headings[k]; ok {
			continue
		}
		p := &picture{id: r.opt.NewID(), maxCols: width}
		img, cols, rows := bigtext.Heading(k.text, k.level, width, r.opt.Theme.headingColor(k.level),
			bigtext.Options{Cell: r.opt.Caps.Cell, Font: r.opt.HeadingFont})
		if img == nil {
			// Can't be drawn (a word too long): remember that, as a
			// picture of no size, so it isn't tried again.
			r.headings[k] = p
			continue
		}
		t, err := kitty.FromImage(p.id, img).TransmitSharp(cols, rows)
		if err != nil {
			r.headings[k] = p
			continue
		}
		p.cols, p.rows = cols, rows
		seq.WriteString(t)
		r.headings[k] = p
		r.gen++
	}
	r.sendGlyphs(glyphs, &seq)
	seq.WriteString(imgSeq)
	return seq.String(), jobs
}

// Close returns the escape sequences that delete every picture this
// Renderer sent. Send it when the screen closes. The Renderer can be
// used again; it draws text until the next Prepare.
func (r *Renderer) Close() string {
	var gone []int
	for k, p := range r.headings {
		if p.cols > 0 {
			gone = append(gone, p.id)
		}
		delete(r.headings, k)
	}
	gone = append(gone, r.dropGlyphs(nil)...)
	for url, e := range r.images {
		if e.cols > 0 {
			gone = append(gone, e.id)
		}
		delete(r.images, url)
	}
	r.gen++
	return deletes(gone)
}

// Forget forgets the pictures sent so far without deleting them, for
// when the terminal has dropped them itself (the screen switched or was
// cleared): the next Prepare sends everything again.
func (r *Renderer) Forget() {
	clear(r.headings)
	clear(r.glyphs)
	for _, e := range r.images {
		e.cols, e.rows = 0, 0 // decoded images are kept: no new fetch
	}
	r.gen++
}

// deletes is the sequences that delete the images ids, lowest id first.
func deletes(ids []int) string {
	slices.Sort(ids)
	var b strings.Builder
	for _, id := range ids {
		b.WriteString(kitty.Delete(id))
	}
	return b.String()
}

// minWidth is the narrowest column the renderer lays out for.
const minWidth = 16

// Render draws the document for a column of width cells. It sends
// nothing to the terminal: pictures that Prepare made are drawn as kitty
// placeholder cells, and everything else as styled text.
func (r *Renderer) Render(d *Doc, width int) string {
	width = max(width, minWidth)
	if d.gen != r.gen || d.cache == nil {
		d.gen, d.cache = r.gen, map[int]string{}
	}
	if s, ok := d.cache[width]; ok {
		return s
	}
	rr := &renderer{src: d.src, th: r.opt.Theme, picture: r.headingLines, alertGlyphs: r.alertGlyphs, image: r.imageView,
		textSizing: r.opt.Caps.TextSizing, m: r.opt.Caps.Width}
	s := strings.Join(rr.blocks(d.root, width, 0), "\n")
	d.cache[width] = s
	return s
}

// headingLines is placeholder lines for a heading's picture when there is
// one that fits width, else nil.
func (r *Renderer) headingLines(level int, text string, width int) []string {
	p, ok := r.headings[headingKey{level: level, text: text, color: colorKey(r.opt.Theme.headingColor(level))}]
	if !ok || p.cols == 0 || p.cols > width {
		return nil
	}
	return strings.Split(kitty.Placeholders(p.id, p.cols, p.rows), "\n")
}
