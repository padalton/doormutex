package streamdeck

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"math"
	"time"
)

// Keys are drawn at 144 × 144 (the XL's and the @2x size); the software
// scales them for smaller decks. Text is left to the key's title, which
// the Stream Deck software renders in the user's font.
const size = 144

var (
	background = color.RGBA{0x12, 0x14, 0x18, 0xff}
	housing    = color.RGBA{0x2a, 0x2e, 0x36, 0xff}
	ring       = color.RGBA{0xe8, 0xea, 0xee, 0xff}
	lampLit    = map[Lamp]color.RGBA{
		Red: {0xff, 0x3b, 0x30, 0xff}, Yellow: {0xff, 0xc4, 0x00, 0xff}, Green: {0x2e, 0xd5, 0x73, 0xff},
		Off: {0x9a, 0xa0, 0xa8, 0xff},
	}
	tomato = color.RGBA{0xe5, 0x48, 0x2f, 0xff}
	leaf   = color.RGBA{0x3d, 0xa3, 0x5d, 0xff}
)

// dark is a lamp that's off: its colour, but dim, so the key still says
// which lamp it is.
func dark(c color.RGBA) color.RGBA {
	return color.RGBA{c.R / 5, c.G / 5, c.B / 5, 0xff}
}

type canvas struct{ *image.RGBA }

func newCanvas() canvas {
	c := canvas{image.NewRGBA(image.Rect(0, 0, size, size))}
	for i := 0; i < len(c.Pix); i += 4 {
		c.Pix[i], c.Pix[i+1], c.Pix[i+2], c.Pix[i+3] = background.R, background.G, background.B, 0xff
	}
	return c
}

// blend paints col over pixel (x, y) with coverage a (0..1).
func (c canvas) blend(x, y int, col color.RGBA, a float64) {
	if a <= 0 {
		return
	}
	if a > 1 {
		a = 1
	}
	o := c.PixOffset(x, y)
	mix := func(dst, src uint8) uint8 { return uint8(float64(dst)*(1-a) + float64(src)*a + 0.5) }
	c.Pix[o], c.Pix[o+1], c.Pix[o+2] = mix(c.Pix[o], col.R), mix(c.Pix[o+1], col.G), mix(c.Pix[o+2], col.B)
}

// shape fills everything where inside reports a signed distance < 0
// (negative inside), anti-aliased over one pixel.
func (c canvas) shape(col color.RGBA, dist func(x, y float64) float64) {
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			c.blend(x, y, col, 0.5-dist(float64(x)+0.5, float64(y)+0.5))
		}
	}
}

func (c canvas) disc(cx, cy, r float64, col color.RGBA) {
	c.shape(col, func(x, y float64) float64 { return math.Hypot(x-cx, y-cy) - r })
}

func (c canvas) circleLine(cx, cy, r, w float64, col color.RGBA) {
	c.shape(col, func(x, y float64) float64 { return math.Abs(math.Hypot(x-cx, y-cy)-r) - w/2 })
}

// roundRect fills a rectangle with rounded corners.
func (c canvas) roundRect(x0, y0, x1, y1, r float64, col color.RGBA) {
	c.shape(col, func(x, y float64) float64 {
		dx := math.Max(math.Max(x0+r-x, x-(x1-r)), 0)
		dy := math.Max(math.Max(y0+r-y, y-(y1-r)), 0)
		inside := math.Max(math.Max(x0-x, x-x1), math.Max(y0-y, y-y1))
		if dx > 0 && dy > 0 {
			return math.Hypot(dx, dy) - r
		}
		return inside
	})
}

// glow is a soft halo around a lit lamp.
func (c canvas) glow(cx, cy, r float64, col color.RGBA) {
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) - r
			if d > 0 && d < r*0.6 {
				c.blend(x, y, col, 0.35*(1-d/(r*0.6)))
			}
		}
	}
}

// pie fills the part of a disc from 12 o'clock clockwise up to frac.
func (c canvas) pie(cx, cy, r, frac float64, col color.RGBA) {
	c.shape(col, func(x, y float64) float64 {
		d := math.Hypot(x-cx, y-cy) - r
		a := math.Atan2(x-cx, cy-y) // 0 at the top, clockwise
		if a < 0 {
			a += 2 * math.Pi
		}
		if a > frac*2*math.Pi {
			return 1
		}
		return d
	})
}

func (c canvas) dataURL() string {
	var b bytes.Buffer
	_ = png.Encode(&b, c.RGBA)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

// lampImage is a lamp key: a big lamp, lit or dark; a ring marks the
// key whose status is set by hand.
func lampImage(lamp Lamp, lit, selected bool) canvas {
	c := newCanvas()
	col := lampLit[lamp]
	// Up a little: the title ("until 14:30") goes at the bottom.
	if lit {
		c.glow(72, 56, 40, col)
		c.disc(72, 56, 40, col)
	} else {
		c.disc(72, 56, 40, dark(col))
	}
	if selected {
		c.circleLine(72, 56, 50, 4, ring)
	}
	return c
}

// lightImage is a small traffic light with the current pattern, for the
// status and the auto key; a ring marks automatic mode on the auto key.
func lightImage(light string, now time.Time, offline, selected bool) canvas {
	c := newCanvas()
	// Leaves the bottom third for the title.
	if selected {
		c.roundRect(46, 2, 98, 108, 16, ring)
		c.roundRect(50, 6, 94, 104, 13, background)
	}
	c.roundRect(52, 8, 92, 102, 12, housing)
	for i, lamp := range []Lamp{Red, Yellow, Green} {
		cy := 25 + float64(i)*30
		col := lampLit[lamp]
		if !offline && litAt(light, lamp, now) {
			c.disc(72, cy, 11, col)
		} else {
			c.disc(72, cy, 11, dark(col))
		}
	}
	return c
}

// statusImage is the status key: the colour of what the light shows, dim
// enough for the title on top, with a bright band at the top.
func statusImage(light string, now time.Time, offline bool) canvas {
	c := newCanvas()
	col := housing
	if !offline {
		for _, lamp := range []Lamp{Red, Yellow, Green} {
			if litAt(light, lamp, now) {
				col = lampLit[lamp]
				break
			}
		}
	}
	tint := color.RGBA{col.R / 3, col.G / 3, col.B / 3, 0xff}
	c.roundRect(4, 4, 140, 140, 16, tint)
	c.roundRect(4, 4, 140, 22, 8, col)
	c.roundRect(4, 14, 140, 26, 0, tint)
	return c
}

// pomodoroImage is a tomato; while one runs, the part of the block that's
// left stays bright.
func pomodoroImage(running bool, work bool, frac float64) canvas {
	c := newCanvas()
	if !running {
		c.disc(72, 70, 44, tomato)
	} else {
		col := tomato
		if !work {
			col = lampLit[Green]
		}
		c.disc(72, 70, 44, dark(col))
		c.pie(72, 70, 44, frac, col)
	}
	c.roundRect(64, 18, 80, 30, 5, leaf)
	return c
}
