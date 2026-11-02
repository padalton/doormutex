package eink

import (
	"image"

	"github.com/padalton/doormutex/hub/internal/status"
)

// symbol stands in for the colour, which e-ink can't show.
type symbol int

const (
	symDND     symbol = iota // no-entry sign
	symWorking               // clock
	symFree                  // tick
	symAway                  // crescent moon
	symFault                 // warning triangle
	symOffline               // warning triangle
)

func symbolFor(s status.Status) symbol {
	switch s {
	case status.DND:
		return symDND
	case status.Working:
		return symWorking
	case status.Free:
		return symFree
	default:
		return symAway
	}
}

// symbol draws s centred at (cx, cy) with radius r.
func (c *canvas) symbol(s symbol, cx, cy, r float64) {
	box := image.Rect(int(cx-r)-1, int(cy-r)-1, int(cx+r)+2, int(cy+r)+2)
	switch s {
	case symDND:
		c.fill(box, circle(cx, cy, r), true)
		c.fill(box, roundRect(cx-r*0.62, cy-r*0.17, cx+r*0.62, cy+r*0.17, 2), false)
	case symWorking:
		c.fill(box, ring(cx, cy, r, 4.5), true)
		c.fill(box, line(cx, cy, cx, cy-r*0.6, 4), true)
		c.fill(box, line(cx, cy, cx+r*0.45, cy+r*0.2, 4), true)
	case symFree:
		c.fill(box, ring(cx, cy, r, 4.5), true)
		c.fill(box, line(cx-r*0.45, cy+r*0.02, cx-r*0.12, cy+r*0.36, 5), true)
		c.fill(box, line(cx-r*0.12, cy+r*0.36, cx+r*0.48, cy-r*0.34, 5), true)
	case symAway:
		c.fill(box, andNot(circle(cx, cy, r*0.95), circle(cx+r*0.42, cy-r*0.32, r*0.78)), true)
	case symFault, symOffline:
		top, base := cy-r, cy+r*0.85
		c.fill(box, triangle(cx, top, cx-r, base, cx+r, base), true)
		c.fill(box, triangle(cx, top+8.5, cx-r+7.5, base-4.5, cx+r-7.5, base-4.5), false)
		c.fill(box, line(cx, cy-r*0.32, cx, cy+r*0.3, 4.5), true)
		c.fill(box, circle(cx, cy+r*0.58, 2.6), true)
	}
}
