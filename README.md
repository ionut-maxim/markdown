# markdown

A Go library that renders GitHub Flavored Markdown in the terminal. It uses the kitty protocols where the terminal has them: big headings, images and Mermaid diagrams. Every other terminal gets clean text.

```sh
go get github.com/ionut-maxim/markdown
```

> [!NOTE]
> This library is being built and has no release yet. The packages below are the plan for the first version (0.1.0).

## What it draws

- **Headings** at full size: drawn as images in terminals with kitty graphics, and as bold coloured text elsewhere. Programs that write straight to the terminal can use kitty's text sizing (OSC 66) instead; Bubble Tea v2 drops OSC 66 from its views, so Bubble Tea apps get the images.
- **Lists:** nested, ordered from any number, and task lists.
- **Tables:** column alignment, cells that wrap, and wide characters (CJK, emoji) measured correctly.
- **Code** highlighted with [Chroma](https://github.com/alecthomas/chroma).
- **GitHub alerts:** `> [!NOTE]`, `[!TIP]`, `[!IMPORTANT]`, `[!WARNING]` and `[!CAUTION]`.
- **Images and Mermaid diagrams** with kitty graphics, using Unicode placeholders so they scroll and clip with the text. Without kitty graphics they show as links and code.
- **Mermaid** drawn in pure Go with [go-mermaid](https://github.com/zkrebbekx/go-mermaid), in the theme's colours: no browser, no Node, no network. Apps can bring their own `Diagrammer`, or turn diagrams off with `NoDiagrams`.

Markdown is parsed with [goldmark](https://github.com/yuin/goldmark) and its GFM extensions.

## Packages

| Package | What it gives you |
|---|---|
| `markdown` | The renderer: parse once, render for a width, and send pictures only when they change. Made for Bubble Tea views that redraw often. |
| `kitty` | Kitty graphics with Unicode placeholders: send an image once, draw it as text cells. |
| `bigtext` | Headings drawn as images, for terminals with kitty graphics but no text sizing. |
| `teamd` | Helpers for Bubble Tea v2 apps: the renderer's work as `tea.Cmd` values. |
| `cmd/mdview` | A pager for checking the output by eye: `go run github.com/ionut-maxim/markdown/cmd/mdview file.md`. |

## Try it

```sh
go run github.com/ionut-maxim/markdown/cmd/mdview file.md
```

`mdview` is a pager built on `teamd`. Scroll with the arrow keys, `j`/`k`, space and `b`, the mouse wheel or a drag. Press `t` to force the text fallback, `d` to switch between dark and light, `[` and `]` to change the width, and `q` to quit. The bottom line shows what it detected about the terminal.

- Images load from paths next to the file. Web images are fetched only with `-fetch`.
- `-print` prints the file straight to the terminal instead of paging. In kitty, headings are then drawn with its text sizing (OSC 66), which Bubble Tea programs can't use.
- `testdata/sample/sample.md` shows everything the renderer draws.

## Licence

MIT, see `LICENSE`. JetBrains Mono is under the SIL Open Font License 1.1 (`bigtext/fonts/OFL.txt`).
