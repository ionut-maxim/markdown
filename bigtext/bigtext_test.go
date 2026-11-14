package bigtext

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/opentype"

	"github.com/ionut-maxim/markdown/kitty"
)

var white = color.RGBA{255, 255, 255, 255}

func TestScale(t *testing.T) {
	cases := []struct {
		cell   kitty.Cell
		cw, ch int
	}{
		{kitty.Cell{W: 10, H: 20}, 30, 60}, // ×3 to reach 48 pixels
		{kitty.Cell{W: 8, H: 16}, 24, 48},  // Rex's nominal cell
		{kitty.Cell{W: 25, H: 50}, 25, 50}, // big enough already
		{kitty.Cell{}, 30, 60},             // unknown: DefaultCell
	}
	for _, c := range cases {
		if cw, ch := Scale(c.cell); cw != c.cw || ch != c.ch {
			t.Errorf("Scale(%v) = %d,%d; want %d,%d", c.cell, cw, ch, c.cw, c.ch)
		}
	}
}

func TestHeading(t *testing.T) {
	cell := kitty.Cell{W: 8, H: 16}
	img, cols, rows := Heading("Release notes", 1, 60, white, Options{Cell: cell})
	if img == nil {
		t.Fatal("no image")
	}
	if rows != HeadingRows {
		t.Errorf("rows = %d, want %d", rows, HeadingRows)
	}
	cw, ch := Scale(cell)
	if b := img.Bounds(); b.Dx() != cols*cw || b.Dy() != rows*ch {
		t.Errorf("image %v for %d×%d cells of %d×%d", b, cols, rows, cw, ch)
	}
	if !drawn(img) {
		t.Error("image is empty")
	}
}

func TestHeadingLevels(t *testing.T) {
	_, title, _ := Heading("Same words", TitleLevel, 80, white, Options{})
	_, h1, _ := Heading("Same words", 1, 80, white, Options{})
	_, h2, _ := Heading("Same words", 2, 80, white, Options{})
	if !(title >= h1 && h1 > h2) {
		t.Errorf("widths: title %d, h1 %d, h2 %d; want title ≥ h1 > h2", title, h1, h2)
	}
}

func TestHeadingWraps(t *testing.T) {
	_, cols, rows := Heading("a heading long enough to need more than one line", 1, 20, white, Options{})
	if rows <= HeadingRows || rows%HeadingRows != 0 {
		t.Errorf("rows = %d, want several lines of %d", rows, HeadingRows)
	}
	if cols > 20 {
		t.Errorf("cols = %d, wider than 20", cols)
	}
}

func TestHeadingStaysText(t *testing.T) {
	if img, _, _ := Heading("   ", 1, 40, white, Options{}); img != nil {
		t.Error("blank text drew an image")
	}
	if img, _, _ := Heading("Supercalifragilisticexpialidocious", 1, 8, white, Options{}); img != nil {
		t.Error("a word wider than the width drew an image")
	}
}

func TestLoadFont(t *testing.T) {
	path := filepath.Join(t.TempDir(), "font.ttf")
	if err := os.WriteFile(path, jetbrainsMonoBold, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := LoadFont(path)
	if err != nil {
		t.Fatal(err)
	}
	if img, _, _ := Heading("Own font", 1, 40, white, Options{Font: f}); img == nil {
		t.Error("no image with a loaded font")
	}
	if _, err := LoadFont(filepath.Join(t.TempDir(), "missing.ttf")); err == nil {
		t.Error("want an error for a missing file")
	}
	bad := filepath.Join(t.TempDir(), "bad.ttf")
	_ = os.WriteFile(bad, []byte("not a font"), 0o600)
	if _, err := LoadFont(bad); err == nil {
		t.Error("want an error for a file that isn't a font")
	}
}

func TestHasGlyphs(t *testing.T) {
	face, err := opentype.NewFace(Font(), &opentype.FaceOptions{Size: 20, DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()
	if !HasGlyphs(face, "Done ✓") {
		t.Error("JetBrains Mono should draw plain text")
	}
	if HasGlyphs(face, "") { // a Nerd Font icon
		t.Error("JetBrains Mono has no Nerd Font icons")
	}
}

// drawn is true when some pixel isn't transparent.
func drawn(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
				return true
			}
		}
	}
	return false
}
