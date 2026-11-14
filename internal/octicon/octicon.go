// Package octicon draws the Octicons GitHub uses in its alerts, from
// their SVG paths, in any colour and size.
package octicon

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"

	"golang.org/x/image/vector"
)

// viewBox is the Octicons' coordinate space: 16×16.
const viewBox = 16

// Draw draws the named icon (info, light-bulb, report, alert, stop) in c,
// as large as fits a w×h image, centred.
func Draw(name string, w, h int, c color.Color) (image.Image, error) {
	d, ok := paths[name]
	if !ok {
		return nil, fmt.Errorf("octicon: no icon %q", name)
	}
	scale := math.Min(float64(w), float64(h)) / viewBox
	dx, dy := (float64(w)-viewBox*scale)/2, (float64(h)-viewBox*scale)/2
	r := vector.NewRasterizer(w, h)
	pen := &pen{r: r, scale: scale, dx: dx, dy: dy}
	if err := parsePath(d, pen); err != nil {
		return nil, err
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	r.Draw(dst, dst.Bounds(), image.NewUniform(c), image.Point{})
	return dst, nil
}

// Names are the icons there are.
func Names() []string { return []string{"info", "light-bulb", "report", "alert", "stop"} }

// pen draws path segments in icon coordinates onto the rasterizer.
type pen struct {
	r             *vector.Rasterizer
	scale, dx, dy float64
}

func (p *pen) pt(x, y float64) (float32, float32) {
	return float32(x*p.scale + p.dx), float32(y*p.scale + p.dy)
}

func (p *pen) moveTo(x, y float64) { p.r.MoveTo(p.pt(x, y)) }
func (p *pen) lineTo(x, y float64) { p.r.LineTo(p.pt(x, y)) }
func (p *pen) close()              { p.r.ClosePath() }
func (p *pen) cubeTo(x1, y1, x2, y2, x, y float64) {
	ax, ay := p.pt(x1, y1)
	bx, by := p.pt(x2, y2)
	cx, cy := p.pt(x, y)
	p.r.CubeTo(ax, ay, bx, by, cx, cy)
}

// parsePath reads SVG path data (M L H V C S A Z, absolute and relative)
// and draws it with p.
func parsePath(d string, p *pen) error {
	s := &scanner{s: d}
	var cmd byte
	var x, y, startX, startY, lastC2X, lastC2Y float64
	prevCubic := false
	for {
		s.skipSep()
		if s.done() {
			return nil
		}
		if c := s.s[s.i]; isCommand(c) {
			cmd = c
			s.i++
		} else if cmd == 0 {
			return fmt.Errorf("octicon: path starts with %q", c)
		}
		rel := cmd >= 'a'
		ox, oy := 0.0, 0.0
		if rel {
			ox, oy = x, y
		}
		cubic := false
		switch cmd | 0x20 { // lower case
		case 'm':
			nx, ny, err := s.pair()
			if err != nil {
				return err
			}
			x, y = ox+nx, oy+ny
			startX, startY = x, y
			p.moveTo(x, y)
			// Further pairs after M are line-tos.
			if rel {
				cmd = 'l'
			} else {
				cmd = 'L'
			}
		case 'l':
			nx, ny, err := s.pair()
			if err != nil {
				return err
			}
			x, y = ox+nx, oy+ny
			p.lineTo(x, y)
		case 'h':
			n, err := s.number()
			if err != nil {
				return err
			}
			x = ox + n
			p.lineTo(x, y)
		case 'v':
			n, err := s.number()
			if err != nil {
				return err
			}
			y = oy + n
			p.lineTo(x, y)
		case 'c', 's':
			var x1, y1 float64
			if cmd|0x20 == 'c' {
				a, b, err := s.pair()
				if err != nil {
					return err
				}
				x1, y1 = ox+a, oy+b
			} else if prevCubic {
				x1, y1 = 2*x-lastC2X, 2*y-lastC2Y // reflect the last control point
			} else {
				x1, y1 = x, y
			}
			a, b, err := s.pair()
			if err != nil {
				return err
			}
			x2, y2 := ox+a, oy+b
			a, b, err = s.pair()
			if err != nil {
				return err
			}
			nx, ny := ox+a, oy+b
			p.cubeTo(x1, y1, x2, y2, nx, ny)
			lastC2X, lastC2Y = x2, y2
			x, y = nx, ny
			cubic = true
		case 'a':
			rx, ry, err := s.pair()
			if err != nil {
				return err
			}
			rot, err := s.number()
			if err != nil {
				return err
			}
			large, err := s.flag()
			if err != nil {
				return err
			}
			sweep, err := s.flag()
			if err != nil {
				return err
			}
			a, b, err := s.pair()
			if err != nil {
				return err
			}
			nx, ny := ox+a, oy+b
			arc(p, x, y, rx, ry, rot, large, sweep, nx, ny)
			x, y = nx, ny
		case 'z':
			p.close()
			x, y = startX, startY
		default:
			return fmt.Errorf("octicon: unsupported path command %q", cmd)
		}
		prevCubic = cubic
	}
}

// arc draws an SVG elliptical arc from (x0, y0) to (x, y) as cubic
// curves, each spanning at most a quarter turn (SVG 1.1, appendix F.6).
func arc(p *pen, x0, y0, rx, ry, rotDeg float64, large, sweep bool, x, y float64) {
	if x0 == x && y0 == y {
		return
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	if rx == 0 || ry == 0 {
		p.lineTo(x, y)
		return
	}
	phi := rotDeg * math.Pi / 180
	cos, sin := math.Cos(phi), math.Sin(phi)
	// The midpoint, rotated into the ellipse's frame.
	dx, dy := (x0-x)/2, (y0-y)/2
	x1p, y1p := cos*dx+sin*dy, -sin*dx+cos*dy
	// Radii too small to reach: scale them up.
	if l := x1p*x1p/(rx*rx) + y1p*y1p/(ry*ry); l > 1 {
		rx, ry = rx*math.Sqrt(l), ry*math.Sqrt(l)
	}
	num := rx*rx*ry*ry - rx*rx*y1p*y1p - ry*ry*x1p*x1p
	den := rx*rx*y1p*y1p + ry*ry*x1p*x1p
	k := math.Sqrt(math.Max(num/den, 0))
	if large == sweep {
		k = -k
	}
	cxp, cyp := k*rx*y1p/ry, -k*ry*x1p/rx
	cx := cos*cxp - sin*cyp + (x0+x)/2
	cy := sin*cxp + cos*cyp + (y0+y)/2
	angle := func(ux, uy, vx, vy float64) float64 {
		return math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy)
	}
	t1 := angle(1, 0, (x1p-cxp)/rx, (y1p-cyp)/ry)
	dt := angle((x1p-cxp)/rx, (y1p-cyp)/ry, (-x1p-cxp)/rx, (-y1p-cyp)/ry)
	if !sweep && dt > 0 {
		dt -= 2 * math.Pi
	} else if sweep && dt < 0 {
		dt += 2 * math.Pi
	}
	n := int(math.Ceil(math.Abs(dt) / (math.Pi / 2)))
	step := dt / float64(n)
	kk := 4.0 / 3 * math.Tan(step/4)
	point := func(t float64) (float64, float64) {
		ex, ey := rx*math.Cos(t), ry*math.Sin(t)
		return cos*ex - sin*ey + cx, sin*ex + cos*ey + cy
	}
	deriv := func(t float64) (float64, float64) {
		ex, ey := -rx*math.Sin(t), ry*math.Cos(t)
		return cos*ex - sin*ey, sin*ex + cos*ey
	}
	t := t1
	for range n {
		ax, ay := point(t)
		adx, ady := deriv(t)
		bx, by := point(t + step)
		bdx, bdy := deriv(t + step)
		p.cubeTo(ax+kk*adx, ay+kk*ady, bx-kk*bdx, by-kk*bdy, bx, by)
		t += step
	}
}

func isCommand(c byte) bool {
	switch c | 0x20 {
	case 'm', 'l', 'h', 'v', 'c', 's', 'q', 't', 'a', 'z':
		return true
	}
	return false
}

// scanner reads numbers from SVG path data, which may run together:
// ".75.75" is two numbers, "1-2" too.
type scanner struct {
	s string
	i int
}

func (s *scanner) done() bool { return s.i >= len(s.s) }

func (s *scanner) skipSep() {
	for !s.done() && (s.s[s.i] == ' ' || s.s[s.i] == ',' || s.s[s.i] == '\n' || s.s[s.i] == '\t' || s.s[s.i] == '\r') {
		s.i++
	}
}

func (s *scanner) number() (float64, error) {
	s.skipSep()
	start := s.i
	if !s.done() && (s.s[s.i] == '-' || s.s[s.i] == '+') {
		s.i++
	}
	dot, exp := false, false
	for !s.done() {
		c := s.s[s.i]
		switch {
		case c >= '0' && c <= '9':
		case c == '.' && !dot && !exp:
			dot = true
		case (c == 'e' || c == 'E') && !exp:
			exp = true
			if s.i+1 < len(s.s) && (s.s[s.i+1] == '-' || s.s[s.i+1] == '+') {
				s.i++
			}
		default:
			goto end
		}
		s.i++
	}
end:
	if start == s.i {
		return 0, fmt.Errorf("octicon: number expected at %d", start)
	}
	return strconv.ParseFloat(s.s[start:s.i], 64)
}

func (s *scanner) pair() (float64, float64, error) {
	a, err := s.number()
	if err != nil {
		return 0, 0, err
	}
	b, err := s.number()
	return a, b, err
}

// flag reads an arc flag: a single 0 or 1, which may run into the next
// number.
func (s *scanner) flag() (bool, error) {
	s.skipSep()
	if s.done() || (s.s[s.i] != '0' && s.s[s.i] != '1') {
		return false, fmt.Errorf("octicon: arc flag expected at %d", s.i)
	}
	s.i++
	return s.s[s.i-1] == '1', nil
}
