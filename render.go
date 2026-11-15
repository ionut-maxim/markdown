package markdown

import (
	"image/color"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/util"

	"github.com/ionut-maxim/markdown/kitty"
)

// renderer draws one document for one width.
type renderer struct {
	src []byte
	th  Theme
	// picture returns placeholder lines for a heading drawn as an image
	// at most width cells wide, or nil to draw it as text.
	picture func(level int, text string, width int) []string
	// textSizing draws levels 1 and 2 with kitty's OSC 66.
	textSizing bool
	// m measures text the way the terminal does.
	m ansi.Method
	// alertGlyphs gives an alert's icon and bar as pictures, or "" to
	// draw them as text.
	alertGlyphs func(k AlertKind, c color.Color) (icon, bar string)
	// image gives an image's picture as placeholder lines if it fits
	// width, else nil and a note for its link.
	image func(url string, width int) (lines []string, note string)
	// drawn are the images of the paragraph being drawn that are shown
	// as pictures under it, so the text leaves them out.
	drawn map[*ast.Image]bool
	// diagram gives a Mermaid diagram's picture as placeholder lines if it
	// fits width, else nil and, if it can't be drawn, why.
	diagram func(src string, width int) (lines []string, note string)
}

func (r *renderer) fg(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

// blocks renders a container's children, a blank line between blocks
// (none between the items of a tight list's paragraphs).
func (r *renderer) blocks(n ast.Node, width, depth int) []string {
	var out []string
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		lines := r.block(c, width, depth)
		if len(lines) == 0 {
			continue
		}
		if len(out) > 0 && !tightSibling(c) {
			out = append(out, "")
		}
		out = append(out, lines...)
	}
	return out
}

// tightSibling is true for a block inside a tight list item, which follows
// the previous one without a blank line.
func tightSibling(n ast.Node) bool {
	if _, ok := n.Parent().(*ast.ListItem); ok {
		if l, ok := n.Parent().Parent().(*ast.List); ok {
			return l.IsTight
		}
	}
	return false
}

func (r *renderer) block(n ast.Node, width, depth int) []string {
	switch n := n.(type) {
	case *ast.Heading:
		return r.heading(n, width)
	case *ast.Paragraph, *ast.TextBlock:
		return r.paragraph(n, width)
	case *ast.List:
		return r.list(n, width, depth)
	case *ast.FencedCodeBlock:
		if isMermaid(n, r.src) && r.diagram != nil {
			lines, note := r.diagram(codeSource(n, r.src), width)
			if lines != nil {
				return lines
			}
			out := r.code(string(n.Language(r.src)), r.lines(n), width)
			if note != "" {
				out = append(out, r.fg(r.th.Muted).Render(r.m.Truncate(note, width, "…")))
			}
			return out
		}
		return r.code(string(n.Language(r.src)), r.lines(n), width)
	case *ast.CodeBlock:
		return r.code("", r.lines(n), width)
	case *ast.Blockquote:
		return barred(r.blocks(n, width-2, depth), r.fg(r.th.Accent2).Render(r.th.Quote))
	case *alert:
		return r.alert(n, width, depth)
	case *ast.ThematicBreak:
		return []string{r.fg(r.th.Rule).Render(strings.Repeat("─", width))}
	case *ast.HTMLBlock:
		var out []string
		for _, l := range r.lines(n) {
			out = append(out, r.fg(r.th.Muted).Render(r.m.Truncate(l, width, "…")))
		}
		return out
	case *extast.Table:
		return r.table(n, width)
	}
	// Anything else: its children.
	return r.blocks(n, width, depth)
}

func (r *renderer) lines(n ast.Node) []string {
	var out []string
	ls := n.Lines()
	for i := 0; i < ls.Len(); i++ {
		seg := ls.At(i)
		out = append(out, strings.TrimRight(string(seg.Value(r.src)), "\n"))
	}
	return out
}

// heading draws the text in the accent colour with a rule under it for
// levels 1 and 2. Levels 1 and 2 are drawn bigger, as kitty images, once
// Prepare has made them (Rex supports neither kitty's text sizing, OSC 66,
// nor VT100 double-height lines; tested on the iOS client).
func (r *renderer) heading(n *ast.Heading, width int) []string {
	st := inlineStyle{bold: true}
	switch n.Level {
	case 1:
		st.fg = r.th.Accent
	case 2:
		st.fg = r.th.Accent2
	case 3:
		st.fg = r.th.Text
	case 4, 5:
		st.fg = r.th.Subtle
	default:
		st.fg = r.th.Muted
	}
	var out []string
	switch {
	case r.textSizing && n.Level <= 2:
		out = r.sizedHeading(n.Level, plainText(n, r.src), st.fg, width)
	case r.picture != nil && n.Level <= 2:
		out = r.picture(n.Level, plainText(n, r.src), width)
	}
	if out == nil {
		out = r.wrap(r.inline(n, st), width)
	}
	switch n.Level {
	case 1:
		out = append(out, r.fg(r.th.Accent).Render(strings.Repeat("━", width)))
	case 2:
		out = append(out, r.fg(r.th.Rule).Render(strings.Repeat("─", width)))
	}
	return out
}

// alert draws a GitHub alert: a bar in the alert's colour, its icon and
// title, then its blocks. With kitty graphics the icon is the Octicon
// GitHub shows, and the bar a picture without seams.
func (r *renderer) alert(n *alert, width, depth int) []string {
	c := r.th.alertColor(n.kind)
	var icon, bar string
	if r.alertGlyphs != nil {
		icon, bar = r.alertGlyphs(n.kind, c)
	}
	style := lipgloss.NewStyle().Foreground(c).Bold(true)
	title := style.Render(r.th.alertIcon(n.kind) + " " + n.kind.String())
	if icon != "" {
		title = icon + " " + style.Render(n.kind.String())
	}
	if bar == "" {
		bar = r.fg(c).Render(r.th.Quote)
	}
	return barred(append([]string{title}, r.blocks(n, width-2, depth)...), bar)
}

// barred puts bar before each line; a blank line gets the bar alone.
func barred(lines []string, bar string) []string {
	for i, l := range lines {
		if l == "" {
			lines[i] = bar
		} else {
			lines[i] = bar + " " + l
		}
	}
	return lines
}

// headingSizes are the OSC 66 sizes of level 1 and 2 headings: both two
// rows high and two cells a character, level 2 drawn at three quarters
// inside them.
var headingSizes = map[int]kitty.Size{
	1: {Scale: 2},
	2: {Scale: 2, Num: 3, Den: 4, VAlign: 2},
}

// sizedHeading draws a heading with kitty's text sizing, wrapped at half
// the width. Each line takes two rows: the second stays empty, since the
// big characters cover it.
func (r *renderer) sizedHeading(level int, text string, fg color.Color, width int) []string {
	if text == "" {
		return nil
	}
	st := ansi.NewStyle().Bold().ForegroundColor(fg)
	var out []string
	for _, l := range strings.Split(r.m.Wrap(text, max(width/2, 1), " -"), "\n") {
		if l = strings.TrimRight(l, " "); l != "" {
			out = append(out, st.Styled(kitty.TextSize(l, headingSizes[level])), "")
		}
	}
	return out
}

// paragraph draws a paragraph's text, then the pictures of its images
// that have one, each under the text.
func (r *renderer) paragraph(n ast.Node, width int) []string {
	var pics [][]string
	if r.image != nil {
		_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
			im, ok := c.(*ast.Image)
			if !ok || !entering {
				return ast.WalkContinue, nil
			}
			if lines, _ := r.image(string(im.Destination), width); lines != nil {
				if r.drawn == nil {
					r.drawn = map[*ast.Image]bool{}
				}
				r.drawn[im] = true
				pics = append(pics, lines)
			}
			return ast.WalkSkipChildren, nil
		})
	}
	var out []string
	if text := r.inline(n, inlineStyle{}); strings.TrimSpace(ansi.Strip(text)) != "" {
		out = r.wrap(text, width)
	}
	for _, p := range pics {
		out = append(out, p...)
	}
	return out
}

// list draws items with a bullet by depth, a number, or a checkbox; the
// item's lines hang under the first.
func (r *renderer) list(n *ast.List, width, depth int) []string {
	var out []string
	num := n.Start
	if num == 0 {
		num = 1
	}
	// Ordered markers line up on the widest number.
	numW := len(strconv.Itoa(num + n.ChildCount() - 1))
	first := true
	for item := n.FirstChild(); item != nil; item = item.NextSibling() {
		var marker string
		if n.IsOrdered() {
			s := strconv.Itoa(num) + "."
			marker = r.fg(r.th.Subtle).Render(strings.Repeat(" ", numW+1-len(s)) + s)
			num++
		} else {
			marker = r.fg(r.th.Accent).Render(r.th.bullet(depth))
		}
		if box, ok := taskBox(item); ok {
			if box.IsChecked {
				marker = r.fg(r.th.Green).Render(r.th.CheckOn)
			} else {
				marker = r.fg(r.th.Muted).Render(r.th.CheckOff)
			}
		}
		pad := strings.Repeat(" ", r.m.StringWidth(marker)+1)
		body := r.blocks(item, width-len(pad), depth+1)
		if !first && !n.IsTight {
			out = append(out, "")
		}
		first = false
		for i, l := range body {
			switch {
			case i == 0:
				out = append(out, marker+" "+l)
			case l == "":
				out = append(out, "")
			default:
				out = append(out, pad+l)
			}
		}
		if len(body) == 0 {
			out = append(out, marker)
		}
	}
	return out
}

// taskBox finds a GFM task checkbox at the start of a list item.
func taskBox(item ast.Node) (*extast.TaskCheckBox, bool) {
	c := item.FirstChild()
	if c == nil || c.FirstChild() == nil {
		return nil, false
	}
	box, ok := c.FirstChild().(*extast.TaskCheckBox)
	return box, ok
}

// code draws a shaded panel with the language at the top right and the
// lines highlighted by Chroma. Long lines wrap.
func (r *renderer) code(lang string, lines []string, width int) []string {
	bg := r.th.Surface
	panel := lipgloss.NewStyle().Background(bg)
	inner := max(width-2, 8)
	var out []string
	label := ""
	if lang != "" {
		label = r.fg(r.th.Muted).Background(bg).Render(lang + " ")
	}
	out = append(out, panel.Render(strings.Repeat(" ", width-r.m.StringWidth(label)))+label)
	for _, l := range highlight(lang, strings.Join(lines, "\n"), r.th, bg) {
		for _, part := range strings.Split(r.m.Hardwrap(l, inner, true), "\n") {
			fill := inner - r.m.StringWidth(part)
			out = append(out, panel.Render(" ")+part+panel.Render(strings.Repeat(" ", max(fill, 0)+1)))
		}
	}
	out = append(out, panel.Render(strings.Repeat(" ", width)))
	return out
}

// highlight colours code by its language (guessed when unnamed), one
// string per line, on bg.
func highlight(lang, code string, th Theme, bg color.Color) []string {
	lexer := lexers.Get(lang)
	if lexer == nil && lang == "" {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)
	style := chromastyles.Get(th.codeStyle())
	it, err := lexer.Tokenise(nil, code)
	plain := lipgloss.NewStyle().Foreground(th.Text).Background(bg)
	if err != nil {
		var out []string
		for _, l := range strings.Split(code, "\n") {
			out = append(out, plain.Render(l))
		}
		return out
	}
	var out []string
	var line strings.Builder
	for _, tok := range it.Tokens() {
		e := style.Get(tok.Type)
		st := plain
		if e.Colour.IsSet() {
			st = st.Foreground(lipgloss.Color(e.Colour.String()))
		}
		if e.Bold == chroma.Yes {
			st = st.Bold(true)
		}
		if e.Italic == chroma.Yes {
			st = st.Italic(true)
		}
		parts := strings.Split(tok.Value, "\n")
		for i, p := range parts {
			if i > 0 {
				out = append(out, line.String())
				line.Reset()
			}
			if p != "" {
				line.WriteString(st.Render(strings.ReplaceAll(p, "\t", "    ")))
			}
		}
	}
	if line.Len() > 0 {
		out = append(out, line.String())
	}
	return out
}

// A table column shrinks to its longest word, but not below minCell
// cells, and a word longer than maxWord may be broken. When the columns
// don't fit even then, the table is drawn as cards.
const (
	minCell = 6
	maxWord = 16
)

// table draws a rounded grid. When it's too wide, columns shrink and
// their cells wrap; when even that doesn't fit, each row becomes a card
// ("header: value") instead.
func (r *renderer) table(n *extast.Table, width int) []string {
	var header []string
	var rows [][]string
	var aligns []extast.Alignment
	for row := n.FirstChild(); row != nil; row = row.NextSibling() {
		var cells []string
		for c := row.FirstChild(); c != nil; c = c.NextSibling() {
			st := inlineStyle{}
			if _, ok := row.(*extast.TableHeader); ok {
				st.bold, st.fg = true, r.th.Text
				if tc, ok := c.(*extast.TableCell); ok {
					aligns = append(aligns, tc.Alignment)
				}
			}
			cells = append(cells, r.inline(c, st))
		}
		if _, ok := row.(*extast.TableHeader); ok {
			header = cells
		} else {
			rows = append(rows, cells)
		}
	}
	cols := len(header)
	natural, least := make([]int, cols), make([]int, cols)
	for _, row := range append([][]string{header}, rows...) {
		for i := 0; i < cols && i < len(row); i++ {
			for _, l := range strings.Split(row[i], "\n") {
				natural[i] = max(natural[i], r.m.StringWidth(l))
			}
			for _, w := range strings.Fields(ansi.Strip(row[i])) {
				least[i] = max(least[i], min(r.m.StringWidth(w), maxWord))
			}
		}
	}
	for i := range least {
		least[i] = min(max(least[i], minCell), natural[i])
	}
	widths := fitColumns(natural, least, width-(1+3*cols))
	if widths == nil {
		return r.cards(header, rows, width)
	}

	border := r.fg(r.th.Rule)
	line := func(l, m, rr string) string {
		var b strings.Builder
		b.WriteString(l)
		for i, w := range widths {
			if i > 0 {
				b.WriteString(m)
			}
			b.WriteString(strings.Repeat("─", w+2))
		}
		return border.Render(b.String() + rr)
	}
	// rowLines draws one row, each cell wrapped to its column.
	rowLines := func(cells []string) []string {
		wrapped := make([][]string, cols)
		height := 1
		for i, w := range widths {
			if i < len(cells) {
				wrapped[i] = r.wrapN(cells[i], w)
			}
			height = max(height, len(wrapped[i]))
		}
		out := make([]string, height)
		for k := range height {
			var b strings.Builder
			b.WriteString(border.Render("│"))
			for i, w := range widths {
				c := ""
				if k < len(wrapped[i]) {
					c = wrapped[i][k]
				}
				pad := max(w-r.m.StringWidth(c), 0)
				switch {
				case i < len(aligns) && aligns[i] == extast.AlignRight:
					c = strings.Repeat(" ", pad) + c
				case i < len(aligns) && aligns[i] == extast.AlignCenter:
					c = strings.Repeat(" ", pad/2) + c + strings.Repeat(" ", pad-pad/2)
				default:
					c += strings.Repeat(" ", pad)
				}
				b.WriteString(" " + c + " " + border.Render("│"))
			}
			out[k] = b.String()
		}
		return out
	}
	body := make([][]string, len(rows))
	tall := false // a row wrapped: rule between rows so they're told apart
	for i, row := range rows {
		body[i] = rowLines(row)
		tall = tall || len(body[i]) > 1
	}
	out := []string{line("╭", "┬", "╮")}
	out = append(out, rowLines(header)...)
	out = append(out, line("├", "┼", "┤"))
	for i, b := range body {
		if i > 0 && tall {
			out = append(out, line("├", "┼", "┤"))
		}
		out = append(out, b...)
	}
	return append(out, line("╰", "┴", "╯"))
}

// fitColumns gives each column a width so they add up to at most avail
// cells: their natural widths when they fit, otherwise each gets its
// least width plus a share of the rest in proportion to how much more it
// would need. It returns nil when even the least widths don't fit.
func fitColumns(natural, least []int, avail int) []int {
	widths := slices.Clone(least)
	sum, need := 0, 0
	for i, w := range natural {
		sum += widths[i]
		need += w - widths[i]
	}
	if sum > avail {
		return nil
	}
	extra := avail - sum
	if extra >= need {
		return slices.Clone(natural)
	}
	// Each column's exact share is extra*more/need; whole cells first,
	// then what rounding left over to the largest remainders.
	given := 0
	rem := make([]int, len(widths))
	for i, w := range natural {
		more := w - widths[i]
		widths[i] += extra * more / need
		given += extra * more / need
		rem[i] = extra * more % need
	}
	order := make([]int, len(widths))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return rem[b] - rem[a] })
	for _, i := range order {
		if given == extra {
			break
		}
		if widths[i] < natural[i] {
			widths[i]++
			given++
		}
	}
	return widths
}

// cards draws each row as a card: its cells under their headers, with a
// bar on the left.
func (r *renderer) cards(header []string, rows [][]string, width int) []string {
	var out []string
	bar := r.fg(r.th.Accent2).Render(r.th.Quote) + " "
	for k, row := range rows {
		if k > 0 {
			out = append(out, "")
		}
		for i, c := range row {
			if i < len(header) {
				for _, l := range r.wrap(r.fg(r.th.Muted).Render(ansi.Strip(header[i])), width-2) {
					out = append(out, bar+l)
				}
			}
			for _, l := range r.wrap(c, width-4) {
				out = append(out, bar+"  "+l)
			}
		}
	}
	return out
}

// nbsp pads inline code: a space a line can't break at.
const nbsp = "\u00a0"

// closingPunct are the characters inline code isn't padded before.
const closingPunct = ".,;:!?)]}'\"…"

// inlineStyle is the style in effect for a run of inline text.
type inlineStyle struct {
	bold, italic, strike, underline bool
	code                            bool
	fg                              color.Color
}

func (r *renderer) render(s string, st inlineStyle) string {
	ls := lipgloss.NewStyle()
	if st.fg != nil {
		ls = ls.Foreground(st.fg)
	} else {
		ls = ls.Foreground(r.th.Text)
	}
	ls = ls.Bold(st.bold).Italic(st.italic).Strikethrough(st.strike).Underline(st.underline)
	if st.code {
		ls = ls.Background(r.th.SurfaceHi)
	}
	return ls.Render(s)
}

// inline renders a block's inline content.
func (r *renderer) inline(n ast.Node, st inlineStyle) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		b.WriteString(r.inlineNode(c, st))
	}
	return b.String()
}

func (r *renderer) inlineNode(n ast.Node, st inlineStyle) string {
	switch n := n.(type) {
	case *ast.Text:
		s := r.render(textValue(n, r.src), st)
		switch {
		case n.HardLineBreak():
			s += "\n"
		case n.SoftLineBreak():
			s += " "
		}
		return s
	case *ast.String:
		return r.render(string(n.Value), st)
	case *ast.CodeSpan:
		var b strings.Builder
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				b.Write(t.Segment.Value(r.src))
			}
		}
		cs := st
		cs.code, cs.bold = true, false
		if cs.fg == nil {
			cs.fg = r.th.Accent
		}
		// Padded like a chip, with no-break spaces so a line never breaks
		// inside it, and no pad before punctuation.
		right := nbsp
		if next, ok := n.NextSibling().(*ast.Text); ok {
			if v := textValue(next, r.src); v != "" && strings.ContainsRune(closingPunct, []rune(v)[0]) {
				right = ""
			}
		}
		return r.render(nbsp+b.String()+right, cs)
	case *ast.Emphasis:
		es := st
		if n.Level >= 2 {
			es.bold = true
		} else {
			es.italic = true
		}
		return r.inline(n, es)
	case *extast.Strikethrough:
		ss := st
		ss.strike = true
		ss.fg = r.th.Muted
		return r.inline(n, ss)
	case *ast.Link:
		ls := st
		ls.underline, ls.fg = true, r.th.Blue
		return ansi.SetHyperlink(string(n.Destination)) + r.inline(n, ls) + ansi.ResetHyperlink()
	case *ast.AutoLink:
		url := string(n.URL(r.src))
		ls := st
		ls.underline, ls.fg = true, r.th.Blue
		return ansi.SetHyperlink(url) + r.render(string(n.Label(r.src)), ls) + ansi.ResetHyperlink()
	case *ast.Image:
		if r.drawn[n] {
			return "" // drawn as a picture under the paragraph
		}
		// A link to the image, with why there's no picture yet.
		name := plainText(n, r.src)
		if name == "" {
			name = "image"
		}
		ls := st
		ls.underline, ls.fg = true, r.th.Blue
		link := ansi.SetHyperlink(string(n.Destination)) + r.render(r.th.ImageIcon+" "+name, ls) + ansi.ResetHyperlink()
		if r.image != nil {
			if _, note := r.image(string(n.Destination), 0); note != "" {
				ms := st
				ms.fg = r.th.Muted
				link += r.render(" ("+note+")", ms)
			}
		}
		return link
	case *ast.RawHTML:
		var b strings.Builder
		for i := 0; i < n.Segments.Len(); i++ {
			seg := n.Segments.At(i)
			b.Write(seg.Value(r.src))
		}
		ms := st
		ms.fg = r.th.Muted
		return r.render(b.String(), ms)
	case *extast.TaskCheckBox:
		return "" // drawn as the list marker
	}
	return r.inline(n, st)
}

// wrap breaks styled text into lines of width cells (8 at least),
// keeping hard breaks.
func (r *renderer) wrap(s string, width int) []string { return r.wrapN(s, max(width, 8)) }

// wrapN is wrap without the least width, for table cells.
func (r *renderer) wrapN(s string, width int) []string {
	var out []string
	for _, para := range strings.Split(strings.TrimRight(s, " "), "\n") {
		w := r.m.Wrap(para, max(width, 1), " -")
		for _, l := range strings.Split(w, "\n") {
			out = append(out, strings.TrimRight(l, " "))
		}
	}
	return out
}

// textValue is a text node's text with backslash escapes and entities
// resolved, as goldmark's HTML writer does: `\~7k` is `~7k`, `&amp;` is
// `&`. Raw text (code) keeps them.
func textValue(t *ast.Text, src []byte) string {
	v := t.Segment.Value(src)
	if t.IsRaw() {
		return string(v)
	}
	v = util.UnescapePunctuations(v)
	v = util.ResolveNumericReferences(v)
	v = util.ResolveEntityNames(v)
	return string(v)
}

// plainText is a node's text without markup.
func plainText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			b.WriteString(textValue(t, src))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(t.Value)
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(b.String())
}
