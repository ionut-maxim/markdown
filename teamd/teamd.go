// Package teamd runs a markdown.Renderer in a Bubble Tea v2 program: it
// asks the terminal what it can do, sends pictures at the right moment,
// and turns image fetches and diagram drawing into commands.
//
// A screen adds a few lines:
//
//	p := teamd.New(r, teamd.Options{})
//
//	func (m model) Init() tea.Cmd { return m.p.Init() }
//
//	func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
//		cmd := m.p.Update(msg) // every message
//		if size, ok := msg.(tea.WindowSizeMsg); ok {
//			m.width = size.Width
//			cmd = tea.Batch(cmd, m.p.Show(m.width, m.doc))
//		}
//		…
//	}
//
//	func (m model) View() tea.View { … m.r.Render(m.doc, m.width) … }
//
// and sends p.Close() before it quits or the screen goes away.
package teamd

import (
	"context"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/ionut-maxim/markdown"
)

// SyncID is the kitty image id of the query Pictures sends to learn that
// the program's first frame is on the screen. Apps that probe kitty
// graphics themselves should use another id, and pass replies with this
// one on to Update.
const SyncID = 30

// syncQuery asks whether kitty graphics work, with id SyncID.
var syncQuery = ansi.KittyGraphics([]byte("AAAA"), "i=30", "s=1", "v=1", "a=q", "t=d", "f=24")

// Options configure Pictures.
type Options struct {
	// NoProbe leaves the terminal queries to the app: Pictures doesn't ask
	// for the cell size or take kitty graphics support from its own query,
	// and the app sets them with Renderer.SetCaps and calls Show. Use it
	// when the app already probes (rexkit's shell does), so the queries
	// aren't sent twice.
	NoProbe bool
	// Context is passed to jobs; nil means context.Background(). Cancel it
	// to stop fetches when the program ends.
	Context context.Context
}

// Pictures sends a Renderer's pictures from a Bubble Tea program.
//
// Pictures belong to the screen they were sent on, and Bubble Tea
// switches to the alternate screen only when it writes the first frame,
// after the first Update has run. So Pictures holds everything back until
// the terminal answers a query sent after that frame (kitty graphics
// terminals always answer it), then prepares what Show asked for.
type Pictures struct {
	r       *markdown.Renderer
	opt     Options
	width   int
	docs    []*markdown.Doc
	shown   bool // Show was called and Close wasn't since
	syncing bool // the frame query is out
	ready   bool // the frame query was answered
}

// New returns Pictures for r.
func New(r *markdown.Renderer, opt Options) *Pictures {
	if opt.Context == nil {
		opt.Context = context.Background()
	}
	return &Pictures{r: r, opt: opt}
}

// Init asks the terminal for its cell size, unless NoProbe is set.
func (p *Pictures) Init() tea.Cmd {
	if p.opt.NoProbe {
		return nil
	}
	return tea.Raw(ansi.WindowOp(16))
}

// loadedMsg is a finished job, for the Pictures that made it.
type loadedMsg struct {
	p   *Pictures
	res markdown.Result
}

// Update takes the terminal's answers and finished jobs. Pass it every
// message; it returns what to send, or nil.
func (p *Pictures) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if !p.ready && !p.syncing {
			// The first frame is written after this Update: the answer
			// comes back once it's on the screen.
			p.syncing = true
			return tea.Raw(syncQuery)
		}
	case uv.KittyGraphicsEvent:
		if msg.Options.ID != SyncID || p.ready {
			return nil
		}
		p.ready = true
		if !p.opt.NoProbe {
			c := p.r.Caps()
			c.Graphics = string(msg.Payload) == "OK"
			p.r.SetCaps(c)
		}
		return p.prepare()
	case uv.CellSizeEvent:
		if p.opt.NoProbe || msg.Width <= 0 || msg.Height <= 0 {
			return nil
		}
		c := p.r.Caps()
		c.Cell.W, c.Cell.H = msg.Width, msg.Height
		p.r.SetCaps(c)
		return p.prepare()
	case tea.ModeReportMsg:
		if msg.Mode != ansi.ModeUnicodeCore {
			return nil
		}
		// As Bubble Tea decides: a terminal that knows the mode gets it
		// turned on, and widths are then measured by grapheme.
		switch msg.Value {
		case ansi.ModeReset, ansi.ModeSet, ansi.ModePermanentlySet:
			c := p.r.Caps()
			if c.Width != ansi.GraphemeWidth {
				c.Width = ansi.GraphemeWidth
				p.r.SetCaps(c)
				return p.prepare()
			}
		}
	case loadedMsg:
		if msg.p == p {
			return raw(p.r.Loaded(msg.res))
		}
	}
	return nil
}

// Show sets the documents on screen and the width they're drawn at, and
// prepares their pictures once the first frame is out. Call it when
// either changes, and after Renderer.SetTheme or SetCaps; it sends
// nothing when nothing changed.
func (p *Pictures) Show(width int, docs ...*markdown.Doc) tea.Cmd {
	p.width, p.docs, p.shown = width, docs, true
	return p.prepare()
}

// Reset sends every picture again. Call it after the program enters or
// leaves the alternate screen, or clears the screen: the terminal drops
// pictures then.
func (p *Pictures) Reset() tea.Cmd {
	p.r.Forget()
	return p.prepare()
}

// Close deletes the pictures. Send it before quitting, or when the screen
// showing the documents goes away; Show brings them back.
func (p *Pictures) Close() tea.Cmd {
	p.shown, p.docs = false, nil
	return raw(p.r.Close())
}

// Ready is true once the first frame is out and pictures are being sent.
func (p *Pictures) Ready() bool { return p.ready }

// prepare sends what the documents need and starts their jobs, once
// there's somewhere to send it.
func (p *Pictures) prepare() tea.Cmd {
	if !p.ready || !p.shown {
		return nil
	}
	seq, jobs := p.r.Prepare(p.width, p.docs...)
	cmds := []tea.Cmd{raw(seq)}
	for _, j := range jobs {
		ctx := p.opt.Context
		cmds = append(cmds, func() tea.Msg { return loadedMsg{p: p, res: j.Run(ctx)} })
	}
	return tea.Batch(cmds...)
}

// raw is tea.Raw, or nil for nothing.
func raw(s string) tea.Cmd {
	if s == "" {
		return nil
	}
	return tea.Raw(s)
}
