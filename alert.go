package markdown

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// AlertKind is the kind of a GitHub alert block: > [!NOTE] and so on.
type AlertKind int

// The five alert kinds GitHub draws.
const (
	AlertNote AlertKind = iota
	AlertTip
	AlertImportant
	AlertWarning
	AlertCaution
)

var alertNames = map[string]AlertKind{
	"NOTE": AlertNote, "TIP": AlertTip, "IMPORTANT": AlertImportant, "WARNING": AlertWarning, "CAUTION": AlertCaution,
}

// String is the alert's title, as GitHub shows it: "Note", "Tip"…
func (k AlertKind) String() string {
	return [...]string{"Note", "Tip", "Important", "Warning", "Caution"}[k]
}

// kindAlert is the parse tree node of an alert block.
var kindAlert = ast.NewNodeKind("Alert")

// alert is an alert block: a blockquote without its marker line.
type alert struct {
	ast.BaseBlock
	kind AlertKind
}

func (a *alert) Kind() ast.NodeKind { return kindAlert }

func (a *alert) Dump(src []byte, level int) {
	ast.DumpHelper(a, src, level, map[string]string{"Kind": a.kind.String()}, nil)
}

// alertMarker is an alert's first line. GitHub takes any case and
// trailing spaces, but nothing else on the line.
var alertMarker = regexp.MustCompile(`(?i)^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\][ \t]*\r?\n?$`)

// alertTransformer turns blockquotes that are GitHub alerts into alert
// nodes. As on GitHub (checked against its renderer): only top-level
// quotes, the marker alone on the first line, and some text after it;
// anything else stays a quote.
type alertTransformer struct{}

func (alertTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	for n := doc.FirstChild(); n != nil; {
		next := n.NextSibling()
		if q, ok := n.(*ast.Blockquote); ok {
			if a := toAlert(q, src); a != nil {
				doc.ReplaceChild(doc, q, a)
			}
		}
		n = next
	}
}

// toAlert returns the alert q is, or nil when it's a plain quote. It
// moves q's children into the alert, without the marker.
func toAlert(q *ast.Blockquote, src []byte) *alert {
	p, ok := q.FirstChild().(*ast.Paragraph)
	if !ok || p.Lines().Len() == 0 {
		return nil
	}
	first := p.Lines().At(0)
	m := alertMarker.FindSubmatch(first.Value(src))
	if m == nil {
		return nil
	}
	if p.Lines().Len() == 1 && p.NextSibling() == nil {
		return nil // nothing after the marker: GitHub shows a quote
	}
	a := &alert{kind: alertNames[strings.ToUpper(string(m[1]))]}
	if p.Lines().Len() == 1 {
		q.RemoveChild(q, p)
	} else {
		// Drop the inline nodes of the marker line: up to and including
		// the text that ends with its line break.
		for c := p.FirstChild(); c != nil; {
			next := c.NextSibling()
			t, isText := c.(*ast.Text)
			p.RemoveChild(p, c)
			if isText && (t.SoftLineBreak() || t.HardLineBreak()) {
				break
			}
			c = next
		}
		// And its source line, so the paragraph's lines match its text.
		rest := text.NewSegments()
		for i := 1; i < p.Lines().Len(); i++ {
			rest.Append(p.Lines().At(i))
		}
		p.SetLines(rest)
	}
	for c := q.FirstChild(); c != nil; {
		next := c.NextSibling()
		a.AppendChild(a, c)
		c = next
	}
	return a
}
