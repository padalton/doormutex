package streamdeck

import (
	"image"
	"image/draw"
	"image/png"
	"os"
	"testing"
	"time"
)

// TestMontage writes a preview of the key pictures when MONTAGE is set.
func TestMontage(t *testing.T) {
	path := os.Getenv("MONTAGE")
	if path == "" {
		t.Skip("set MONTAGE=file.png")
	}
	now := time.Now()
	pics := []canvas{
		lampImage(Red, true, true), lampImage(Yellow, false, false), lampImage(Green, false, false), lampImage(Off, false, false),
		statusImage("red", now, false), lightImage("green", now, false, true), pomodoroImage(true, true, 0.4), pomodoroImage(false, true, 0),
	}
	out := image.NewRGBA(image.Rect(0, 0, 4*(size+8), 2*(size+8)))
	for i, c := range pics {
		x, y := (i%4)*(size+8), (i/4)*(size+8)
		draw.Draw(out, image.Rect(x, y, x+size, y+size), c.RGBA, image.Point{}, draw.Src)
	}
	f, _ := os.Create(path)
	defer func() { _ = f.Close() }()
	_ = png.Encode(f, out)
}
