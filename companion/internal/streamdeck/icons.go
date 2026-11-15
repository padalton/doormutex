package streamdeck

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"time"
)

// scaled shrinks a key picture to n × n (box filter; n divides nothing in
// particular, so it averages the covered source pixels).
func scaled(c canvas, n int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, n, n))
	f := float64(size) / float64(n)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			var r, g, b, cnt float64
			for sy := int(float64(y) * f); sy < int(float64(y+1)*f); sy++ {
				for sx := int(float64(x) * f); sx < int(float64(x+1)*f); sx++ {
					o := c.PixOffset(sx, sy)
					r, g, b, cnt = r+float64(c.Pix[o]), g+float64(c.Pix[o+1]), b+float64(c.Pix[o+2]), cnt+1
				}
			}
			o := out.PixOffset(x, y)
			out.Pix[o], out.Pix[o+1], out.Pix[o+2], out.Pix[o+3] = uint8(r/cnt+.5), uint8(g/cnt+.5), uint8(b/cnt+.5), 0xff
		}
	}
	return out
}

// WriteIcons writes the static pictures the manifest names into dir: the
// plugin and category icon, the action list icons and the keys' default
// pictures, each at 1x and @2x.
func WriteIcons(dir string) error {
	noon := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	pics := map[string]canvas{
		"plugin":   lightImage("green", noon, false, false),
		"lamp":     lampImage(Red, true, false),
		"auto":     lightImage("yellow", noon, false, true),
		"status":   statusImage("red", noon, false),
		"pomodoro": pomodoroImage(false, true, 0),
	}
	sizes := map[string][2]int{ // 1x and @2x
		"plugin": {28, 56}, "category": {28, 56}, "action": {20, 40}, "key": {72, 144},
	}
	write := func(name string, img image.Image) error {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if err := png.Encode(f, img); err != nil {
			_ = f.Close()
			return err
		}
		return f.Close()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, kind := range []string{"plugin", "category"} {
		s := sizes[kind]
		if err := write(kind+".png", scaled(pics["plugin"], s[0])); err != nil {
			return err
		}
		if err := write(kind+"@2x.png", scaled(pics["plugin"], s[1])); err != nil {
			return err
		}
	}
	for _, a := range []string{"lamp", "auto", "status", "pomodoro"} {
		for _, kind := range []string{"action", "key"} {
			s := sizes[kind]
			if err := write(a+"-"+kind+".png", scaled(pics[a], s[0])); err != nil {
				return err
			}
			if err := write(a+"-"+kind+"@2x.png", scaled(pics[a], s[1])); err != nil {
				return err
			}
		}
	}
	return nil
}
