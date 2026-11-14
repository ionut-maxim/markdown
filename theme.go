package markdown

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme is how the renderer draws: the app's colours and glyphs.
type Theme struct {
	// Dark is true on a dark background. It picks the code style when
	// CodeStyle is empty.
	Dark bool

	// Text is body text; Subtle and Muted are fainter. Accent colours
	// level 1 headings, bullets and inline code; Accent2 level 2 headings
	// and the quote bar. Rule draws rules and table borders.
	Text, Subtle, Muted, Accent, Accent2, Rule color.Color
	// Surface is the code block's background, SurfaceHi inline code's.
	Surface, SurfaceHi color.Color
	// Green ticks done tasks and Blue colours links; Red and Yellow are
	// for alerts.
	Green, Red, Yellow, Blue color.Color

	Bullets           []string // by depth; the last repeats deeper down
	CheckOn, CheckOff string   // task list boxes
	Quote             string   // the bar before quoted lines
	ImageIcon         string   // before an image shown as a link

	// CodeStyle is the Chroma style for code blocks. Empty means
	// catppuccin-mocha on a dark background and catppuccin-latte on a
	// light one.
	CodeStyle string
}

// DefaultTheme is a theme for a dark or light background, with plain
// Unicode glyphs that need no Nerd Font.
func DefaultTheme(dark bool) Theme {
	c := lipgloss.Color
	th := Theme{
		Dark:    dark,
		Bullets: []string{"•", "◦", "▪"}, CheckOn: "☑", CheckOff: "☐", Quote: "▎", ImageIcon: "▣",
	}
	if dark {
		th.Text, th.Subtle, th.Muted = c("#E6E6EB"), c("#9BA0AB"), c("#5F6470")
		th.Accent, th.Accent2 = c("#8B93FF"), c("#E879B9")
		th.Surface, th.SurfaceHi = c("#262836"), c("#34374A")
		th.Green, th.Red, th.Yellow, th.Blue = c("#4CB782"), c("#EB5757"), c("#F2C94C"), c("#4EA7FC")
	} else {
		th.Text, th.Subtle, th.Muted = c("#1F2023"), c("#5E6169"), c("#9A9DA6")
		th.Accent, th.Accent2 = c("#5E6AD2"), c("#C2408C")
		th.Surface, th.SurfaceHi = c("#ECEDF5"), c("#DCDEEC")
		th.Green, th.Red, th.Yellow, th.Blue = c("#2E9A64"), c("#D93F3F"), c("#B98900"), c("#1F7AE0")
	}
	th.Rule = th.SurfaceHi
	return th
}

func (th Theme) bullet(depth int) string {
	if len(th.Bullets) == 0 {
		return "•"
	}
	return th.Bullets[min(depth, len(th.Bullets)-1)]
}

func (th Theme) codeStyle() string {
	switch {
	case th.CodeStyle != "":
		return th.CodeStyle
	case th.Dark:
		return "catppuccin-mocha"
	}
	return "catppuccin-latte"
}

// headingColor is the colour of a level 1 or 2 heading.
func (th Theme) headingColor(level int) color.Color {
	if level == 1 {
		return th.Accent
	}
	return th.Accent2
}
