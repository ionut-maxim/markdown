// Package markdown renders GitHub Flavored Markdown in the terminal.
//
// Headings are drawn bigger where the terminal can (kitty graphics),
// images and alert icons are drawn with kitty graphics, code is
// highlighted with Chroma, tables are grids, and every terminal gets a
// clean text fallback.
//
// A Renderer fits a Bubble Tea program: parse a document once, send what
// Prepare returns from Update, and call Render from View as often as you
// like, since it never writes to the terminal.
//
//	r := markdown.New(markdown.Options{Theme: markdown.DefaultTheme(dark), Caps: caps, Fetch: fetch})
//	doc := r.Parse(src)
//	seq, jobs := r.Prepare(width, doc) // in Update: send seq with tea.Raw, run jobs as commands
//	seq = r.Loaded(result)             // when a job is done: send seq
//	view := r.Render(doc, width)       // in View
//	seq = r.Close()                    // when the screen closes: send seq
//
// Images come only through the caller's Fetch: the library never goes on
// the network itself.
package markdown
