package markdown

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"

	"github.com/yuin/goldmark/ast"
	mermaid "github.com/zkrebbekx/go-mermaid"
	"github.com/zkrebbekx/go-mermaid/raster"
)

// Diagrams: a fenced ```mermaid block is drawn as a picture, through the
// same path as images. Prepare hands out a Job that draws it (drawing
// takes tens of milliseconds, so it runs off the UI goroutine like a
// fetch), Loaded sends the picture, and Render draws its placeholders.
// Until then, and whenever it can't be drawn, the block shows its source
// as highlighted code.

// Diagram is what a Diagrammer is asked to draw.
type Diagram struct {
	// Source is the Mermaid source, without the fences.
	Source string
	// Theme is the renderer's theme: draw in its colours, on a
	// transparent background.
	Theme Theme
	// TextHeight is how tall a line of the diagram's text should be, in
	// pixels: the picture is shown at one terminal row per TextHeight
	// pixels, shrunk to fit.
	TextHeight int
}

// A Diagrammer draws Mermaid diagrams. The default, GoMermaid, draws them
// in Go; an app can bring its own (an external tool, a rendering
// service) with Options.Diagrams. Draw is called off the UI goroutine,
// perhaps from several goroutines at once.
type Diagrammer interface {
	Draw(ctx context.Context, d Diagram) (image.Image, error)
}

// NoDiagrams turns diagrams off: Mermaid blocks show their source.
var NoDiagrams Diagrammer = noDiagrams{}

type noDiagrams struct{}

func (noDiagrams) Draw(context.Context, Diagram) (image.Image, error) {
	return nil, fmt.Errorf("diagrams are off")
}

// GoMermaid draws diagrams with go-mermaid (github.com/zkrebbekx/go-mermaid),
// in pure Go: no browser, no Node, no network.
func GoMermaid() Diagrammer { return goMermaid{} }

type goMermaid struct{}

// mermaidFontSize is go-mermaid's default text size, in its pixels.
const mermaidFontSize = 14

func (goMermaid) Draw(ctx context.Context, d Diagram) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	th := d.Theme
	p := mermaid.Palette{
		Background: hex(th.Surface),
		NodeFill:   hex(th.Surface),
		NodeStroke: hex(th.Accent),
		Text:       hex(th.Text),
		Edge:       hex(th.Subtle),
	}
	// Themes are registered by name for every render in the process, so
	// the name says what's in it: two renderers with different themes
	// don't overwrite each other's.
	name := "markdown-" + strings.ReplaceAll(p.NodeFill+p.NodeStroke+p.Text+p.Edge, "#", "")
	// A line of text is about 1.25 font sizes tall.
	scale := float64(max(d.TextHeight, 1)) / (mermaidFontSize * 1.25)
	data, err := raster.PNG(d.Source, scale, mermaid.WithCustomTheme(name, p), mermaid.WithTransparentBackground())
	if err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(data))
}

// hex is c as #rrggbb.
func hex(c color.Color) string {
	if c == nil {
		return "#000000"
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

// isMermaid is true for a fenced code block in Mermaid.
func isMermaid(n *ast.FencedCodeBlock, src []byte) bool {
	return strings.EqualFold(string(n.Language(src)), "mermaid")
}

// codeSource is a code block's text.
func codeSource(n ast.Node, src []byte) string {
	var b strings.Builder
	ls := n.Lines()
	for i := 0; i < ls.Len(); i++ {
		seg := ls.At(i)
		b.Write(seg.Value(src))
	}
	return b.String()
}

// diagramKey is the images map key for a diagram: its source and the
// colours it's drawn in, so a theme change draws it again. It can't be
// mistaken for a URL.
func diagramKey(source string, th Theme) string {
	return "\x00mermaid\x00" + hex(th.Surface) + hex(th.Accent) + hex(th.Text) + hex(th.Subtle) + "\x00" + source
}

// diagrammer is the Diagrammer to use, nil when diagrams are off.
func (r *Renderer) diagrammer() Diagrammer {
	switch r.opt.Diagrams {
	case nil:
		return goMermaid{}
	case NoDiagrams:
		return nil
	}
	return r.opt.Diagrams
}
