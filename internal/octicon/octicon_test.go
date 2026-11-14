package octicon

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"testing"
)

func TestDrawAll(t *testing.T) {
	for _, name := range Names() {
		img, err := Draw(name, 48, 48, color.RGBA{0x4E, 0xA7, 0xFC, 0xFF})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// Some pixels filled, some not, and the centre column of an icon
		// with a hole (info's ring) has both.
		filled, empty := 0, 0
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if _, _, _, a := img.At(x, y).RGBA(); a > 0x8000 {
					filled++
				} else {
					empty++
				}
			}
		}
		if filled < 100 || empty < 100 {
			t.Errorf("%s: %d filled, %d empty pixels", name, filled, empty)
		}
	}
	if _, err := Draw("nope", 16, 16, color.Black); err == nil {
		t.Error("want an error for an unknown icon")
	}
}

func TestInfoHasAHole(t *testing.T) {
	img, _ := Draw("info", 160, 160, color.Black)
	// Between the outer ring and the "i": empty, so the inner circle was
	// cut out rather than filled.
	if _, _, _, a := img.At(80, 25).RGBA(); a != 0 {
		t.Errorf("inside the ring is filled (alpha %d)", a)
	}
	if _, _, _, a := img.At(80, 4).RGBA(); a == 0 {
		t.Error("the ring itself is empty")
	}
}

// TestSheet writes all icons side by side when OCTICON_SHEET names a
// file, for checking them by eye.
func TestSheet(t *testing.T) {
	path := os.Getenv("OCTICON_SHEET")
	if path == "" {
		t.Skip("set OCTICON_SHEET to write a sheet")
	}
	cols := []color.RGBA{{0x4E, 0xA7, 0xFC, 0xFF}, {0x4C, 0xB7, 0x82, 0xFF}, {0x8B, 0x93, 0xFF, 0xFF}, {0xF2, 0xC9, 0x4C, 0xFF}, {0xEB, 0x57, 0x57, 0xFF}}
	sheet := image.NewRGBA(image.Rect(0, 0, 5*112, 112))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.RGBA{0x1E, 0x1F, 0x2B, 0xFF}), image.Point{}, draw.Src)
	for i, name := range Names() {
		img, err := Draw(name, 96, 96, cols[i])
		if err != nil {
			t.Fatal(err)
		}
		draw.Draw(sheet, image.Rect(i*112+8, 8, i*112+104, 104), img, image.Point{}, draw.Over)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, sheet); err != nil {
		t.Fatal(err)
	}
}
