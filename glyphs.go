package markdown

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	"github.com/ionut-maxim/markdown/bigtext"
	"github.com/ionut-maxim/markdown/internal/octicon"
	"github.com/ionut-maxim/markdown/kitty"
)

// Glyphs are small pictures that dress alerts as GitHub does: the alert's
// Octicon, and a tile of the left bar repeated on every row (a picture
// fills its cell exactly, where ▎ can leave seams between rows). They
// don't depend on the width, so a resize keeps them.

// glyphKey identifies a glyph picture: what it is and its colour.
type glyphKey struct {
	name  string // "bar", or an Octicon's name
	color [4]uint32
}

// alertOcticons are the icons GitHub draws for each kind.
var alertOcticons = [...]string{"info", "light-bulb", "report", "alert", "stop"}

// glyph sizes in cells.
const (
	iconCols = 2 // square in a 1:2 cell
	barCols  = 1
)

// alerts lists the kinds of the document's alerts, in order.
func (d *Doc) alerts() []AlertKind {
	var out []AlertKind
	for n := d.root.FirstChild(); n != nil; n = n.NextSibling() {
		if a, ok := n.(*alert); ok {
			out = append(out, a.kind)
		}
	}
	return out
}

// wantGlyphs lists the glyph pictures the documents need, in order.
func (r *Renderer) wantGlyphs(docs []*Doc) []glyphKey {
	if !r.opt.Caps.graphics() {
		return nil
	}
	seen := map[glyphKey]bool{}
	var out []glyphKey
	add := func(k glyphKey) {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for _, d := range docs {
		for _, k := range d.alerts() {
			c := colorKey(r.opt.Theme.alertColor(k))
			add(glyphKey{name: alertOcticons[k], color: c})
			add(glyphKey{name: "bar", color: c})
		}
	}
	return out
}

// drawGlyph draws a glyph picture at the scale bigtext uses for the
// terminal's cells, and says how many columns it takes.
func (r *Renderer) drawGlyph(k glyphKey, c color.Color) (image.Image, int) {
	cw, ch := bigtext.Scale(r.opt.Caps.Cell)
	if k.name == "bar" {
		// GitHub's border is a quarter of an em; ▎ is a quarter cell.
		img := image.NewRGBA(image.Rect(0, 0, cw, ch))
		draw.Draw(img, image.Rect(0, 0, max(1, int(math.Round(float64(cw)*0.25))), ch), image.NewUniform(c), image.Point{}, draw.Src)
		return img, barCols
	}
	// The icon as tall as capital letters are, centred in two cells.
	size := int(math.Round(float64(ch) * 0.6))
	icon, err := octicon.Draw(k.name, size, size, c)
	if err != nil {
		return nil, 0
	}
	img := image.NewRGBA(image.Rect(0, 0, iconCols*cw, ch))
	at := image.Pt((iconCols*cw-size)/2, (ch-size)/2)
	draw.Draw(img, image.Rectangle{Min: at, Max: at.Add(image.Pt(size, size))}, icon, image.Point{}, draw.Over)
	return img, iconCols
}

// sendGlyphs makes and sends the glyph pictures in want that aren't
// there yet.
func (r *Renderer) sendGlyphs(want []glyphKey, seq *strings.Builder) {
	for _, k := range want {
		if _, ok := r.glyphs[k]; ok {
			continue
		}
		p := &picture{id: r.opt.NewID()}
		img, cols := r.drawGlyph(k, glyphColor(k))
		if img != nil {
			if t, err := kitty.FromImage(p.id, img).TransmitSharp(cols, 1); err == nil {
				p.cols, p.rows = cols, 1
				seq.WriteString(t)
			}
		}
		r.glyphs[k] = p
		r.gen++
	}
}

// dropGlyphs forgets the glyph pictures not in want (all of them for
// nil) and returns the ids to delete.
func (r *Renderer) dropGlyphs(want []glyphKey) []int {
	keep := map[glyphKey]bool{}
	for _, k := range want {
		keep[k] = true
	}
	var gone []int
	for k, p := range r.glyphs {
		if !keep[k] {
			if p.cols > 0 {
				gone = append(gone, p.id)
			}
			delete(r.glyphs, k)
			r.gen++
		}
	}
	return gone
}

// glyphColor turns a key's colour back into a colour.
func glyphColor(k glyphKey) color.Color {
	return color.RGBA64{R: uint16(k.color[0]), G: uint16(k.color[1]), B: uint16(k.color[2]), A: uint16(k.color[3])}
}

// glyphCell is the placeholder cells for a glyph picture in colour c, or
// "" when there's none.
func (r *Renderer) glyphCell(name string, c color.Color) string {
	p, ok := r.glyphs[glyphKey{name: name, color: colorKey(c)}]
	if !ok || p.cols == 0 {
		return ""
	}
	return kitty.Placeholders(p.id, p.cols, 1)
}

// alertGlyphs is what the renderer asks for an alert: its icon and bar
// pictures as placeholder cells, "" for those it should draw as text.
func (r *Renderer) alertGlyphs(k AlertKind, c color.Color) (icon, bar string) {
	return r.glyphCell(alertOcticons[k], c), r.glyphCell("bar", c)
}
