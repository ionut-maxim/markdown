package markdown

import (
	"context"
	"errors"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark/ast"

	"github.com/ionut-maxim/markdown/bigtext"
	"github.com/ionut-maxim/markdown/kitty"
)

// Images: the library never fetches anything itself. Prepare hands out a
// Job for each image it hasn't got, the app runs it off the UI goroutine
// (it calls Options.Fetch and decodes the bytes), and Loaded turns the
// Result into the sequence that sends the picture. Decoded images are
// kept, so a resize or Forget sends them again without fetching.

// defaultImageRows and defaultDiagramRows are how tall an image and a
// diagram may be when Options.MaxImageRows and MaxDiagramRows aren't set.
const (
	defaultImageRows   = 20
	defaultDiagramRows = 40
)

// Job fetches and decodes one image, or draws one diagram. Run it off
// the UI goroutine (in Bubble Tea, as a tea.Cmd) and pass its Result to
// Renderer.Loaded.
type Job struct {
	// URL is the image's address, as written in the markdown; "" for a
	// diagram.
	URL string
	run func(context.Context) Result
}

// Run fetches and decodes the image, or draws the diagram. It doesn't
// touch the Renderer, so it's safe on any goroutine.
func (j Job) Run(ctx context.Context) Result { return j.run(ctx) }

// Result is what a Job brings back, for Renderer.Loaded.
type Result struct {
	URL   string // the Job's URL
	Err   error  // why the image can't be shown, if it can't
	key   string
	entry *imageEntry
	img   *kitty.Image
}

// imageEntry is the state of one image URL or diagram.
type imageEntry struct {
	id         int
	img        *kitty.Image // nil while loading or after an error
	err        error
	loading    bool
	diagram    bool
	maxCols    int // the width to fit, from Prepare
	cols, rows int // the size sent to the terminal; 0 when not sent
}

// picturesWanted lists the images and diagrams in the documents, by key
// (the URL, or diagramKey), with the room each has: the width, less what
// lists and quotes around it take. One used in several places gets the
// narrowest. Diagrams are left out when theme is nil.
func picturesWanted(width int, docs []*Doc, images bool, theme *Theme) (room map[string]int, order []string, sources map[string]string) {
	room, sources = map[string]int{}, map[string]string{}
	for _, d := range docs {
		_ = ast.Walk(d.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			var key string
			switch n := n.(type) {
			case *ast.Image:
				if !images {
					return ast.WalkSkipChildren, nil
				}
				key = string(n.Destination)
			case *ast.FencedCodeBlock:
				if theme == nil || !isMermaid(n, d.src) {
					return ast.WalkContinue, nil
				}
				src := codeSource(n, d.src)
				key = diagramKey(src, *theme)
				sources[key] = src
			default:
				return ast.WalkContinue, nil
			}
			w := width
			for p := n.Parent(); p != nil; p = p.Parent() {
				switch p := p.(type) {
				case *ast.ListItem:
					w -= 2 // a bullet and a space
					if l, ok := p.Parent().(*ast.List); ok && l.IsOrdered() {
						w -= 2 // "10." is wider
					}
				case *ast.Blockquote, *alert:
					w -= 2
				}
			}
			if old, seen := room[key]; seen {
				room[key] = min(old, w)
			} else {
				room[key] = w
				order = append(order, key)
			}
			return ast.WalkSkipChildren, nil
		})
	}
	return room, order, sources
}

// prepareImages updates the images for the documents: new ones get a
// Job, loaded ones are sent again if their size changed, and ones no
// longer used are dropped. It returns ids to delete, sequences to send,
// and the jobs.
func (r *Renderer) prepareImages(width int, docs []*Doc) (gone []int, seq string, jobs []Job) {
	var room map[string]int
	var order []string
	var sources map[string]string
	if r.opt.Caps.graphics() {
		var th *Theme
		if r.diagrammer() != nil {
			th = &r.opt.Theme
		}
		room, order, sources = picturesWanted(width, docs, r.opt.Fetch != nil, th)
	}
	for url, e := range r.images {
		if _, ok := room[url]; !ok {
			if e.cols > 0 {
				gone = append(gone, e.id)
			}
			delete(r.images, url)
			r.gen++
		}
	}
	var b strings.Builder
	for _, key := range order {
		e, ok := r.images[key]
		if !ok {
			e = &imageEntry{id: r.opt.NewID(), loading: true}
			if src, ok := sources[key]; ok {
				e.diagram = true
				jobs = append(jobs, r.diagramJob(key, src, e))
			} else {
				jobs = append(jobs, r.job(key, e))
			}
			r.images[key] = e
			r.gen++
		}
		e.maxCols = max(room[key], 1)
		b.WriteString(r.place(e))
	}
	return gone, b.String(), jobs
}

// job fetches and decodes url for e.
func (r *Renderer) job(url string, e *imageEntry) Job {
	fetch, id := r.opt.Fetch, e.id
	return Job{URL: url, run: func(ctx context.Context) Result {
		data, err := fetch(ctx, url)
		if err != nil {
			return Result{URL: url, Err: err, key: url, entry: e}
		}
		img, err := kitty.Decode(id, data)
		return Result{URL: url, Err: err, key: url, entry: e, img: img}
	}}
}

// diagramJob draws the diagram src for e.
func (r *Renderer) diagramJob(key, src string, e *imageEntry) Job {
	dg, id := r.diagrammer(), e.id
	_, ch := bigtext.Scale(r.opt.Caps.Cell)
	d := Diagram{Source: src, Theme: r.opt.Theme, TextHeight: ch}
	return Job{run: func(ctx context.Context) Result {
		img, err := dg.Draw(ctx, d)
		if err != nil {
			return Result{Err: err, key: key, entry: e}
		}
		if img == nil || img.Bounds().Empty() {
			return Result{Err: errors.New("empty diagram"), key: key, entry: e}
		}
		return Result{key: key, entry: e, img: kitty.FromImage(id, img)}
	}}
}

// Loaded takes a Job's Result and returns the sequence that sends the
// picture, or "" when there's nothing to send (an error, which Render
// then shows, or an image no longer wanted). Call it on the UI goroutine.
func (r *Renderer) Loaded(res Result) string {
	e, ok := r.images[res.key]
	if !ok || e != res.entry {
		return "" // dropped, or asked for again since
	}
	e.loading = false
	r.gen++
	if res.Err != nil || res.img == nil {
		e.err = res.Err
		if e.err == nil {
			e.err = errors.New("no image")
		}
		return ""
	}
	e.img = res.img
	return r.place(e)
}

// place sends e's picture sized for its width, if that changed.
func (r *Renderer) place(e *imageEntry) string {
	if e.img == nil {
		return ""
	}
	b := e.img.Bounds
	// Pixels at bigtext's scale: sharp where the terminal reports a
	// nominal cell (Rex), without sending a screenshot at full size.
	cw, ch := bigtext.Scale(r.opt.Caps.Cell)
	scaled := kitty.Cell{W: cw, H: ch}
	rows, cell := r.opt.MaxImageRows, r.opt.Caps.Cell
	if rows <= 0 {
		rows = defaultImageRows
	}
	if e.diagram {
		// Drawn for cells of that size, so its text is as tall as a row.
		rows, cell = r.opt.MaxDiagramRows, scaled
		if rows <= 0 {
			rows = defaultDiagramRows
		}
	}
	cols, rows := kitty.Fit(b.Dx(), b.Dy(), e.maxCols, rows, cell)
	if cols == e.cols && rows == e.rows {
		return ""
	}
	seq, err := e.img.Transmit(cols, rows, scaled)
	if err != nil {
		e.err, e.img = err, nil
		e.cols, e.rows = 0, 0
		r.gen++
		return ""
	}
	e.cols, e.rows = cols, rows
	r.gen++
	return seq
}

// imageView is how the renderer draws an image: placeholder lines when
// its picture is sent and fits width, else nil and a note to put after
// its link ("loading…", or why it failed).
func (r *Renderer) imageView(url string, width int) (lines []string, note string) {
	e, ok := r.images[url]
	switch {
	case !ok:
		return nil, ""
	case e.cols > 0 && e.cols <= width:
		return strings.Split(kitty.Placeholders(e.id, e.cols, e.rows), "\n"), ""
	case e.loading:
		return nil, "loading…"
	case e.err != nil:
		if e.diagram {
			return nil, "couldn't draw it: " + oneLine(e.err.Error())
		}
		return nil, "couldn't show it: " + oneLine(e.err.Error())
	}
	return nil, ""
}

// diagramView is how the renderer draws a diagram: placeholder lines
// when its picture is sent and fits width, else nil and, when it can't be
// drawn, why.
func (r *Renderer) diagramView(src string, width int) (lines []string, note string) {
	lines, note = r.imageView(diagramKey(src, r.opt.Theme), width)
	if note != "" && !strings.HasPrefix(note, "couldn't") {
		note = "" // the source shows while it's drawn
	}
	return lines, note
}

// oneLine is s on one line, at most 60 cells.
func oneLine(s string) string {
	return ansi.Truncate(strings.Join(strings.Fields(s), " "), 60, "…")
}
