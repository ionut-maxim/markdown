package main

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ionut-maxim/markdown"
	"github.com/ionut-maxim/markdown/teamd"
)

// pager shows one document, scrolled.
type pager struct {
	r    *markdown.Renderer
	p    *teamd.Pictures
	doc  *markdown.Doc
	dark bool

	w, h   int // the window
	narrow int // columns taken off the width with [
	top    int // the first line shown
	dragY  int // where a drag is, -1 when there's none

	text  bool          // the text fallback is forced
	saved markdown.Caps // the caps found, while text is forced
}

func newPager(src string, fetch func(context.Context, string) ([]byte, error)) *pager {
	r := markdown.New(markdown.Options{Theme: markdown.DefaultTheme(true), Fetch: fetch})
	return &pager{r: r, p: teamd.New(r, teamd.Options{}), doc: r.Parse(src), dark: true, dragY: -1}
}

func (m *pager) Init() tea.Cmd {
	return tea.Batch(m.p.Init(), tea.RequestBackgroundColor)
}

// width is the column the document is drawn in: the window less a
// margin each side, less what [ took off.
func (m *pager) width() int { return max(m.w-2-m.narrow, 16) }

// show prepares the pictures for the document as drawn now.
func (m *pager) show() tea.Cmd { return m.p.Show(m.width(), m.doc) }

func (m *pager) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.p.Update(msg)
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Every resize: Rex panes start at one width and get another
		// from the phone a moment later.
		m.w, m.h = msg.Width, msg.Height
		return m, tea.Batch(cmd, m.show())
	case tea.BackgroundColorMsg:
		if dark := msg.IsDark(); dark != m.dark {
			m.dark = dark
			m.r.SetTheme(markdown.DefaultTheme(dark))
			return m, tea.Batch(cmd, m.show())
		}
	case tea.MouseWheelMsg:
		switch msg.Mouse().Button {
		case tea.MouseWheelDown:
			m.scroll(3)
		case tea.MouseWheelUp:
			m.scroll(-3)
		}
	case tea.MouseClickMsg:
		m.dragY = msg.Mouse().Y
	case tea.MouseMotionMsg:
		if m.dragY >= 0 && msg.Mouse().Button == tea.MouseLeft {
			// Touch: the page follows the finger.
			y := msg.Mouse().Y
			m.scroll(m.dragY - y)
			m.dragY = y
		}
	case tea.MouseReleaseMsg:
		m.dragY = -1
	case tea.KeyPressMsg:
		return m, tea.Batch(cmd, m.key(msg.String()))
	}
	return m, cmd
}

func (m *pager) key(k string) tea.Cmd {
	page := max(m.h-2, 1)
	switch k {
	case "q", "ctrl+c", "esc":
		return tea.Sequence(m.p.Close(), tea.Quit)
	case "j", "down":
		m.scroll(1)
	case "k", "up":
		m.scroll(-1)
	case "space", "f", "pgdown":
		m.scroll(page)
	case "b", "pgup":
		m.scroll(-page)
	case "g", "home":
		m.top = 0
	case "G", "end":
		m.top = 1 << 30
	case "t":
		m.text = !m.text
		if m.text {
			m.saved = m.r.Caps()
			m.r.SetCaps(markdown.Caps{Width: m.saved.Width})
		} else {
			m.r.SetCaps(m.saved)
		}
		return m.show()
	case "d":
		m.dark = !m.dark
		m.r.SetTheme(markdown.DefaultTheme(m.dark))
		return m.show()
	case "[":
		if m.width() > 16 {
			m.narrow += 2
		}
		return m.show()
	case "]":
		m.narrow = max(m.narrow-2, 0)
		return m.show()
	case "r":
		return m.p.Reset()
	}
	return nil
}

func (m *pager) scroll(n int) { m.top = max(m.top+n, 0) }

func (m *pager) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	if m.w == 0 {
		return v
	}
	lines := strings.Split(m.r.Render(m.doc, m.width()), "\n")
	body := max(m.h-1, 1)
	m.top = min(m.top, max(len(lines)-body, 0))
	end := min(m.top+body, len(lines))
	var b strings.Builder
	for _, l := range lines[m.top:end] {
		b.WriteString(" " + l + "\n")
	}
	b.WriteString(strings.Repeat("\n", body-(end-m.top)))
	b.WriteString(m.status())
	v.SetContent(b.String())
	return v
}

// status is the bottom line: what the terminal can do, and the keys.
func (m *pager) status() string {
	c := m.r.Caps()
	if m.text {
		c = m.saved
	}
	var parts []string
	switch {
	case m.text:
		parts = append(parts, "text (forced)")
	case c.Graphics:
		parts = append(parts, "kitty graphics")
	case m.p.Ready():
		parts = append(parts, "no kitty graphics")
	default:
		parts = append(parts, "probing…")
	}
	if c.Cell.W > 0 {
		parts = append(parts, fmt.Sprintf("cell %d×%d", c.Cell.W, c.Cell.H))
	}
	if c.Width == ansi.GraphemeWidth {
		parts = append(parts, "graphemes")
	} else {
		parts = append(parts, "wcwidth")
	}
	theme := "dark"
	if !m.dark {
		theme = "light"
	}
	parts = append(parts, theme, fmt.Sprintf("%d cols", m.width()), "t text · d theme · [ ] width · q quit")
	th := markdown.DefaultTheme(m.dark)
	line := ansi.Truncate(" "+strings.Join(parts, " · "), m.w, "…")
	return lipgloss.NewStyle().Foreground(th.Muted).Render(line)
}
