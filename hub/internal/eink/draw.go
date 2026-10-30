package eink

import (
	"image"
	"image/color"
	"math"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Fonts: the Go fonts (BSD-3-Clause, © The Go Authors), embedded through
// golang.org/x/image – no system fonts, so every hub renders the same.
type weight int

const (
	regular weight = iota
	medium
	bold
)

var (
	fontsOnce sync.Once
	fonts     [3]*opentype.Font
)

func parseFonts() {
	for i, ttf := range [][]byte{goregular.TTF, gomedium.TTF, gobold.TTF} {
		f, err := opentype.Parse(ttf)
		if err != nil {
			panic("eink: embedded font: " + err.Error())
		}
		fonts[i] = f
	}
}

// canvas is an 8-bit grey working image; it is thresholded to black and
// white at the end. Faces are created per canvas because they are not
// safe for concurrent use.
type canvas struct {
	g     *image.Gray
	faces map[[2]int]font.Face
}

func newCanvas() *canvas {
	fontsOnce.Do(parseFonts)
	g := image.NewGray(image.Rect(0, 0, Width, Height))
	for i := range g.Pix {
		g.Pix[i] = 0xff
	}
	return &canvas{g: g, faces: map[[2]int]font.Face{}}
}

func (c *canvas) close() {
	for _, f := range c.faces {
		_ = f.Close()
	}
}

func (c *canvas) face(w weight, size int) font.Face {
	k := [2]int{int(w), size}
	if f, ok := c.faces[k]; ok {
		return f
	}
	f, err := opentype.NewFace(fonts[w], &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic("eink: font face: " + err.Error())
	}
	c.faces[k] = f
	return f
}

// width of s in pixels.
func width(f font.Face, s string) int { return font.MeasureString(f, s).Ceil() }

// text draws s with its baseline at y; x is the left edge.
func (c *canvas) text(f font.Face, x, y int, s string) {
	d := font.Drawer{Dst: c.g, Src: image.NewUniform(color.Gray{}), Face: f, Dot: fixed.P(x, y)}
	d.DrawString(s)
}

// lineHeight of a face (ascent + descent, rounded up).
func lineHeight(f font.Face) int {
	m := f.Metrics()
	return (m.Ascent + m.Descent).Ceil()
}

func ascent(f font.Face) int { return f.Metrics().Ascent.Ceil() }

// wrap breaks s into lines no wider than max pixels; words longer than a
// line are split.
func wrap(f font.Face, s string, max int) []string {
	var lines []string
	cur := ""
	for _, w := range strings.Fields(s) {
		for width(f, w) > max {
			// Split an over-long word at the last rune that fits.
			r := []rune(w)
			n := len(r) - 1
			for n > 1 && width(f, string(r[:n])) > max {
				n--
			}
			if cur != "" {
				lines = append(lines, cur)
				cur = ""
			}
			lines = append(lines, string(r[:n]))
			w = string(r[n:])
		}
		switch {
		case cur == "":
			cur = w
		case width(f, cur+" "+w) <= max:
			cur += " " + w
		default:
			lines = append(lines, cur)
			cur = w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// ellipsize shortens s with "…" until it fits max pixels.
func ellipsize(f font.Face, s string, max int) string {
	if width(f, s) <= max {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && width(f, string(r)+"…") > max {
		r = r[:len(r)-1]
	}
	return strings.TrimRight(string(r), " ") + "…"
}

// shape reports whether a point lies inside a figure.
type shape func(x, y float64) bool

// fill paints every pixel of the rectangle whose 4×4 sub-samples are at
// least half inside the shape – smooth curves without grey levels.
func (c *canvas) fill(r image.Rectangle, in shape, black bool) {
	v := uint8(0xff)
	if black {
		v = 0
	}
	r = r.Intersect(c.g.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			n := 0
			for sy := range 4 {
				for sx := range 4 {
					if in(float64(x)+(float64(sx)+0.5)/4, float64(y)+(float64(sy)+0.5)/4) {
						n++
					}
				}
			}
			if n >= 8 {
				c.g.SetGray(x, y, color.Gray{Y: v})
			}
		}
	}
}

func (c *canvas) rect(x0, y0, x1, y1 int, black bool) {
	c.fill(image.Rect(x0, y0, x1, y1), func(float64, float64) bool { return true }, black)
}

func circle(cx, cy, r float64) shape {
	return func(x, y float64) bool { return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r }
}

func ring(cx, cy, r, w float64) shape {
	return func(x, y float64) bool {
		d := math.Hypot(x-cx, y-cy)
		return d <= r && d >= r-w
	}
}

// line is a segment with round ends and width w.
func line(x0, y0, x1, y1, w float64) shape {
	dx, dy := x1-x0, y1-y0
	l2 := dx*dx + dy*dy
	return func(x, y float64) bool {
		t := ((x-x0)*dx + (y-y0)*dy) / l2
		t = math.Max(0, math.Min(1, t))
		px, py := x0+t*dx-x, y0+t*dy-y
		return px*px+py*py <= w*w/4
	}
}

func triangle(ax, ay, bx, by, cx, cy float64) shape {
	side := func(px, py, qx, qy, x, y float64) float64 { return (qx-px)*(y-py) - (qy-py)*(x-px) }
	return func(x, y float64) bool {
		d1, d2, d3 := side(ax, ay, bx, by, x, y), side(bx, by, cx, cy, x, y), side(cx, cy, ax, ay, x, y)
		neg := d1 < 0 || d2 < 0 || d3 < 0
		pos := d1 > 0 || d2 > 0 || d3 > 0
		return !(neg && pos)
	}
}

func roundRect(x0, y0, x1, y1, r float64) shape {
	return func(x, y float64) bool {
		cx := math.Max(x0+r, math.Min(x, x1-r))
		cy := math.Max(y0+r, math.Min(y, y1-r))
		return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r
	}
}

func andNot(a, b shape) shape { return func(x, y float64) bool { return a(x, y) && !b(x, y) } }

// bw thresholds the grey image: index 0 white, 1 black.
func (c *canvas) bw() *image.Paletted {
	p := image.NewPaletted(c.g.Rect, color.Palette{color.Gray{Y: 0xff}, color.Gray{}})
	for i, v := range c.g.Pix {
		if v < 0x80 {
			p.Pix[i] = 1
		}
	}
	return p
}
