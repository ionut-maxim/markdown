// Package bigtext draws headings bigger than a terminal cell, as images
// to show with kitty graphics, for terminals that can't size text
// themselves (Rex supports neither kitty's text sizing, OSC 66, nor
// double-height lines). JetBrains Mono Bold is embedded.
//
// Draw only once the terminal has reported its cell size (CSI 16 t), and
// pass it in Options. Some terminals report a nominal cell whatever the
// real one (Rex reports 8×16), so images are drawn at 48 pixels a cell
// or more and sent with kitty.Image.TransmitSharp: the terminal scales
// them into place and they stay sharp.
package bigtext

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"os"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/ionut-maxim/markdown/kitty"
)

// HeadingRows is how many cell rows one line of a big heading takes.
const HeadingRows = 2

// TitleLevel is the "heading level" of a page title, such as an issue's
// title above its description: drawn a little bigger than level 1.
const TitleLevel = 0

// minCellPixels is the least height, in pixels, a cell is drawn at.
const minCellPixels = 48

// jetbrainsMonoBold is JetBrains Mono Bold (SIL Open Font License 1.1,
// fonts/OFL.txt).
//
//go:embed fonts/JetBrainsMono-Bold.ttf
var jetbrainsMonoBold []byte

// Font is the embedded JetBrains Mono Bold, parsed once.
var Font = sync.OnceValue(func() *opentype.Font {
	f, err := opentype.Parse(jetbrainsMonoBold)
	if err != nil {
		panic("bigtext: embedded font: " + err.Error())
	}
	return f
})

// LoadFont reads a TTF or OTF file to draw with instead of the embedded
// font.
func LoadFont(path string) (*opentype.Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read font: %w", err)
	}
	f, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse font %s: %w", path, err)
	}
	return f, nil
}

// Options says how to draw.
type Options struct {
	// Cell is the terminal's cell size in pixels; zero means
	// kitty.DefaultCell.
	Cell kitty.Cell
	// Font draws the text; nil means the embedded JetBrains Mono Bold.
	Font *opentype.Font
}

func (o Options) font() *opentype.Font {
	if o.Font != nil {
		return o.Font
	}
	return Font()
}

// Scale is the size in pixels to draw one cell at: the cell size, scaled
// up by a whole factor to at least 48 pixels high.
func Scale(cell kitty.Cell) (cw, ch int) {
	cell = cell.OrDefault()
	k := max(1, (minCellPixels+cell.H-1)/cell.H)
	return cell.W * k, cell.H * k
}

// HasGlyphs is true when face draws every rune of s.
func HasGlyphs(face font.Face, s string) bool {
	for _, r := range s {
		if _, ok := face.GlyphAdvance(r); !ok {
			return false
		}
	}
	return true
}

// Heading draws text in c, wrapped to maxCols cells, each line
// HeadingRows cells tall. Level 2 is smaller than level 1, and TitleLevel
// a little bigger. It returns the image and its size in cells, or nil
// when the text can't be drawn this way (empty, or a word too long for
// the width): show it as text then.
func Heading(text string, level, maxCols int, c color.Color, opt Options) (img image.Image, cols, rows int) {
	cw, ch := Scale(opt.Cell)
	lineH := HeadingRows * ch
	size := float64(lineH) * 0.72
	switch level {
	case TitleLevel:
		size = float64(lineH) * 0.78
	case 2:
		size = float64(lineH) * 0.58
	}
	face, err := opentype.NewFace(opt.font(), &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, 0, 0
	}
	defer face.Close()
	maxPx := maxCols*cw - 4
	space := font.MeasureString(face, " ")
	var lines []string
	var cur string
	var curW fixed.Int26_6
	for _, w := range strings.Fields(text) {
		ww := font.MeasureString(face, w)
		if cur != "" && (curW+space+ww).Ceil() > maxPx {
			lines = append(lines, cur)
			cur, curW = "", 0
		}
		if cur != "" {
			cur += " "
			curW += space
		}
		cur += w
		curW += ww
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	widest := 0
	for _, l := range lines {
		widest = max(widest, font.MeasureString(face, l).Ceil())
	}
	if len(lines) == 0 || widest+4 > maxCols*cw {
		return nil, 0, 0
	}
	cols = (widest + 4 + cw - 1) / cw
	rgba := image.NewRGBA(image.Rect(0, 0, cols*cw, len(lines)*lineH))
	m := face.Metrics()
	for i, l := range lines {
		base := i*lineH + (lineH+m.Ascent.Ceil()-m.Descent.Ceil())/2
		d := font.Drawer{Dst: rgba, Src: image.NewUniform(c), Face: face, Dot: fixed.P(2, base)}
		d.DrawString(l)
	}
	return rgba, cols, len(lines) * HeadingRows
}
