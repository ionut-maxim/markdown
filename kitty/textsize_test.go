package kitty

import (
	"strings"
	"testing"
)

func TestTextSize(t *testing.T) {
	cases := []struct {
		s    Size
		want string
	}{
		{Size{Scale: 2}, "\x1b]66;s=2;Hi\a"},
		{Size{Scale: 2, Num: 3, Den: 4, VAlign: 2}, "\x1b]66;s=2:n=3:d=4:v=2;Hi\a"},
		{Size{}, "\x1b]66;;Hi\a"},
		{Size{Scale: 9, Num: 5, Den: 4}, "\x1b]66;s=7;Hi\a"}, // capped, bad fraction dropped
	}
	for _, c := range cases {
		if got := TextSize("Hi", c.s); got != c.want {
			t.Errorf("TextSize(%+v) = %q, want %q", c.s, got, c.want)
		}
	}
}

func TestTextSizeSplitsLongText(t *testing.T) {
	text := strings.Repeat("é", 3000) // 6000 bytes
	got := TextSize(text, Size{Scale: 2})
	parts := strings.Split(strings.TrimSuffix(got, "\a"), "\a")
	if len(parts) != 2 {
		t.Fatalf("want 2 sequences, got %d", len(parts))
	}
	var joined strings.Builder
	for _, p := range parts {
		body := strings.TrimPrefix(p, "\x1b]66;s=2;")
		if len(body) > maxTextBytes {
			t.Errorf("sequence carries %d bytes", len(body))
		}
		joined.WriteString(body)
	}
	if joined.String() != text {
		t.Error("text split inside a character or lost")
	}
}

func TestTextSizing(t *testing.T) {
	cases := []struct {
		c0, c1, c2   int
		width, scale bool
	}{
		{1, 1, 1, false, false}, // ignored: no support
		{1, 3, 3, true, false},  // width only
		{1, 3, 5, true, true},   // both
		{1, 2, 3, false, false}, // spaces drawn as plain text
	}
	for _, c := range cases {
		w, s := TextSizing(c.c0, c.c1, c.c2)
		if w != c.width || s != c.scale {
			t.Errorf("TextSizing(%d,%d,%d) = %v,%v", c.c0, c.c1, c.c2, w, s)
		}
	}
	if strings.Count(TextSizingQuery, "\x1b[6n") != 3 || !strings.HasPrefix(TextSizingQuery, "\r") {
		t.Errorf("query = %q", TextSizingQuery)
	}
}
