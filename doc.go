// Package markdown renders GitHub Flavored Markdown in the terminal.
//
// Headings are drawn bigger where the terminal can (kitty graphics),
// code is highlighted with Chroma, tables are grids, and every terminal
// gets a clean text fallback.
//
// A Renderer fits a Bubble Tea program: parse a document once, send what
// Prepare returns from Update, and call Render from View as often as you
// like, since it never writes to the terminal.
//
//	r := markdown.New(markdown.Options{Theme: markdown.DefaultTheme(dark), Caps: caps})
//	doc := r.Parse(src)
//	cmd := tea.Raw(r.Prepare(width, doc)) // in Update: sends heading pictures
//	view := r.Render(doc, width)          // in View
//	cmd = tea.Raw(r.Close())              // when the screen closes
package markdown
