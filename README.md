# markdown

A Go library that renders GitHub Flavored Markdown in the terminal. It uses the kitty protocols where the terminal has them: big headings, images and Mermaid diagrams. Every other terminal gets clean text.

```sh
go get github.com/ionut-maxim/markdown
```

> [!NOTE]
> This library is being built and has no release yet. The packages below are the plan for the first version (0.1.0).

## What it draws

- **Headings** at full size: kitty's text sizing protocol (OSC 66) where the terminal supports it, headings drawn as images where it only has kitty graphics, and bold coloured text elsewhere.
- **Lists:** nested, ordered from any number, and task lists.
- **Tables:** column alignment, cells that wrap, and wide characters (CJK, emoji) measured correctly.
- **Code** highlighted with [Chroma](https://github.com/alecthomas/chroma).
- **GitHub alerts:** `> [!NOTE]`, `[!TIP]`, `[!IMPORTANT]`, `[!WARNING]` and `[!CAUTION]`.
- **Images and Mermaid diagrams** with kitty graphics, using Unicode placeholders so they scroll and clip with the text. Without kitty graphics they show as links and code.

Markdown is parsed with [goldmark](https://github.com/yuin/goldmark) and its GFM extensions.

## Packages

| Package | What it gives you |
|---|---|
| `markdown` | The renderer: parse once, render for a width, and send pictures only when they change. Made for Bubble Tea views that redraw often. |
| `kitty` | Kitty graphics with Unicode placeholders: send an image once, draw it as text cells. |
| `bigtext` | Headings drawn as images, for terminals with kitty graphics but no text sizing. |
| `mermaid` | Mermaid diagrams drawn as images, without a browser or Node. |
| `teamd` | Helpers for Bubble Tea v2 apps: the renderer's work as `tea.Cmd` values. |
| `cmd/mdview` | A pager for checking the output by eye: `go run github.com/ionut-maxim/markdown/cmd/mdview file.md`. |

## Licence

MIT, see `LICENSE`.
