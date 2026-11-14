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

// defaultImageRows is how tall an image may be when Options.MaxImageRows
// isn't set.
const defaultImageRows = 20

// Job fetches and decodes one image. Run it off the UI goroutine (in
// Bubble Tea, as a tea.Cmd) and pass its Result to Renderer.Loaded.
type Job struct {
	// URL is the image's address, as written in the markdown.
	URL string
	run func(context.Context) Result
}

// Run fetches and decodes the image. It doesn't touch the Renderer, so
// it's safe on any goroutine.
func (j Job) Run(ctx context.Context) Result { return j.run(ctx) }

// Result is what a Job brings back, for Renderer.Loaded.
type Result struct {
	URL   string
	Err   error // why the image can't be shown, if it can't
	entry *imageEntry
	img   *kitty.Image
}

// imageEntry is the state of one image URL.
type imageEntry struct {
	id         int
	img        *kitty.Image // nil while loading or after an error
	err        error
	loading    bool
	maxCols    int // the width to fit, from Prepare
	cols, rows int // the size sent to the terminal; 0 when not sent
}

// imageWidths is the room each image in the documents has: the width,
// less what lists and quotes around it take. An image used in several
// places gets the narrowest.
func imageWidths(width int, docs []*Doc) (map[string]int, []string) {
	room := map[string]int{}
	var order []string
	for _, d := range docs {
		_ = ast.Walk(d.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			im, ok := n.(*ast.Image)
			if !ok || !entering {
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
			url := string(im.Destination)
			if old, seen := room[url]; seen {
				room[url] = min(old, w)
			} else {
				room[url] = w
				order = append(order, url)
			}
			return ast.WalkSkipChildren, nil
		})
	}
	return room, order
}

// prepareImages updates the images for the documents: new ones get a
// Job, loaded ones are sent again if their size changed, and ones no
// longer used are dropped. It returns ids to delete, sequences to send,
// and the jobs.
func (r *Renderer) prepareImages(width int, docs []*Doc) (gone []int, seq string, jobs []Job) {
	var room map[string]int
	var order []string
	if r.opt.Fetch != nil && r.opt.Caps.graphics() {
		room, order = imageWidths(width, docs)
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
	for _, url := range order {
		e, ok := r.images[url]
		if !ok {
			e = &imageEntry{id: r.opt.NewID(), loading: true}
			r.images[url] = e
			jobs = append(jobs, r.job(url, e))
			r.gen++
		}
		e.maxCols = max(room[url], 1)
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
			return Result{URL: url, Err: err, entry: e}
		}
		img, err := kitty.Decode(id, data)
		return Result{URL: url, Err: err, entry: e, img: img}
	}}
}

// Loaded takes a Job's Result and returns the sequence that sends the
// picture, or "" when there's nothing to send (an error, which Render
// then shows, or an image no longer wanted). Call it on the UI goroutine.
func (r *Renderer) Loaded(res Result) string {
	e, ok := r.images[res.URL]
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
	rows := r.opt.MaxImageRows
	if rows <= 0 {
		rows = defaultImageRows
	}
	cols, rows := kitty.Fit(b.Dx(), b.Dy(), e.maxCols, rows, r.opt.Caps.Cell)
	if cols == e.cols && rows == e.rows {
		return ""
	}
	// Pixels at bigtext's scale: sharp where the terminal reports a
	// nominal cell (Rex), without sending a screenshot at full size.
	cw, ch := bigtext.Scale(r.opt.Caps.Cell)
	seq, err := e.img.Transmit(cols, rows, kitty.Cell{W: cw, H: ch})
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
		return nil, "couldn't show it: " + oneLine(e.err.Error())
	}
	return nil, ""
}

// oneLine is s on one line, at most 60 cells.
func oneLine(s string) string {
	return ansi.Truncate(strings.Join(strings.Fields(s), " "), 60, "…")
}
