// Package eink renders the light's 1.54″ e-ink sign: 200×200 pixels, black
// and white, readable right in front of the light. E-ink has no colour,
// so the status is a big word plus a symbol.
//
// The output is deterministic (embedded fonts, no grey levels), so a frame
// only changes when its content does – the device skips identical frames
// by their ETag and the panel isn't refreshed for nothing.
package eink

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/font"

	"github.com/padalton/doormutex/hub/internal/engine"
	"github.com/padalton/doormutex/hub/internal/status"
)

// Panel size in pixels (e.g. GDEY0154D67, SSD1681 controller).
const (
	Width  = 200
	Height = 200
)

// Battery is the device's charge, drawn in the footer when known.
type Battery struct {
	Percent  int // 0–100
	Charging bool
}

// Input is everything a frame shows.
type Input struct {
	State   engine.State
	Lang    string    // catalog language; unknown languages fall back to English
	Stamp   time.Time // "updated" time in the footer; zero hides it
	Battery *Battery  // nil hides it (the device may draw its own)
}

//go:embed i18n/*.json
var catalogFS embed.FS

var catalogs = func() map[string]map[string]string {
	out := map[string]map[string]string{}
	entries, err := catalogFS.ReadDir("i18n")
	if err != nil {
		panic(err)
	}
	for _, e := range entries {
		b, err := catalogFS.ReadFile("i18n/" + e.Name())
		if err != nil {
			panic(err)
		}
		m := map[string]string{}
		if err := json.Unmarshal(b, &m); err != nil {
			panic("eink: catalog " + e.Name() + ": " + err.Error())
		}
		out[strings.TrimSuffix(e.Name(), ".json")] = m
	}
	return out
}()

// Languages lists the available catalogs, sorted.
func Languages() []string { return slices.Sorted(maps.Keys(catalogs)) }

// Supported reports whether a catalog exists for lang.
func Supported(lang string) bool { _, ok := catalogs[lang]; return ok }

// tr looks a key up in lang, falling back to English, then to the key.
func tr(lang, key string, args ...string) string {
	s, ok := catalogs[lang][key]
	if !ok {
		if s, ok = catalogs["en"][key]; !ok {
			s = key
		}
	}
	for i := 0; i+1 < len(args); i += 2 {
		s = strings.ReplaceAll(s, "{"+args[i]+"}", args[i+1])
	}
	return s
}

// clock formats t as "14:30", with the weekday (or the date, more than six
// days ahead) when it isn't on the day of ref.
func clock(lang string, t, ref time.Time) string {
	hm := t.Format("15:04")
	if ref.IsZero() {
		return hm
	}
	t = t.In(ref.Location())
	y1, m1, d1 := t.Date()
	y2, m2, d2 := ref.Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return hm
	}
	if t.After(ref) && t.Sub(ref) < 6*24*time.Hour {
		return tr(lang, "day."+strconv.Itoa(int(t.Weekday()))) + " " + hm
	}
	return t.Format(tr(lang, "date")) + " " + hm
}

// Layout (pixels).
const (
	margin    = 8
	headerH   = 60 // symbol + status word
	footerTop = 182
)

// Render draws a frame for the state.
func Render(in Input) *image.Paletted {
	c := newCanvas()
	defer c.close()
	st, lang := in.State, in.Lang
	ref := st.Updated

	fault := st.Phase == status.PhaseFault
	if fault {
		c.header(symFault, tr(lang, "fault"))
	} else {
		c.header(symbolFor(st.Status), tr(lang, "status."+string(st.Status)))
	}
	y := headerH + 6
	full := Width - 2*margin

	if fault {
		f := c.face(regular, 18)
		for _, l := range wrap(f, tr(lang, "fault.text"), full) {
			c.text(f, margin, y+ascent(f), l)
			y += lineHeight(f) + 2
		}
		c.footer(lang, in.Stamp, in.Battery)
		return c.bw()
	}

	// Until when – as large as fits.
	until := tr(lang, "until.open")
	if st.Until != nil {
		until = tr(lang, "until", "time", clock(lang, *st.Until, ref))
	}
	y = c.line(bold, []int{24, 22, 20, 18}, until, y) + 2
	// Why – left out when there is a message: the person's words matter more.
	if st.Message == "" {
		reason := tr(lang, "reason."+st.Reason)
		if st.Reason == engine.ReasonCalendar && st.Detail != "" {
			reason = tr(lang, "kind."+st.Detail)
		}
		y = c.line(regular, []int{18, 16}, reason, y)
	}

	// Body: the message in a frame, then the next free time if it fits.
	bottom := footerTop - 4
	y += 6
	if st.Message != "" {
		y = c.message(st.Message, y, bottom)
	}
	if st.Status != status.Free && st.NextFree != nil {
		f := c.face(medium, 18)
		if y+lineHeight(f) <= bottom {
			c.line(medium, []int{18, 16}, tr(lang, "free_from", "time", clock(lang, st.NextFree.Start, ref)), y)
		}
	}

	c.footer(lang, in.Stamp, in.Battery)
	return c.bw()
}

// Offline is the frame the device shows when it loses the hub; it fetches
// it once (screen=offline) and keeps it in flash.
func Offline(lang string) *image.Paletted {
	c := newCanvas()
	defer c.close()
	c.header(symOffline, tr(lang, "offline"))
	f := c.face(regular, 18)
	y := headerH + 10
	for _, l := range wrap(f, tr(lang, "offline.text"), Width-2*margin) {
		c.text(f, margin, y+ascent(f), l)
		y += lineHeight(f) + 2
	}
	return c.bw()
}

// header draws the symbol at the left and the status word – as large as
// fits, on up to two lines – to its right.
func (c *canvas) header(sym symbol, word string) {
	c.symbol(sym, 30, 30, 23)
	x := 62
	w := Width - x - 4
	for size := 32; size >= 16; size -= 2 {
		f := c.face(bold, size)
		lines := wrap(f, word, w)
		lh := lineHeight(f)
		if len(lines) > 2 || len(lines)*lh > headerH {
			continue
		}
		if slices.ContainsFunc(lines, func(l string) bool { return width(f, l) > w }) {
			continue
		}
		top := (headerH - len(lines)*lh) / 2
		for i, l := range lines {
			c.text(f, x, top+i*lh+ascent(f), l)
		}
		break
	}
	c.rect(margin, headerH, Width-margin, headerH+2, true)
}

// line draws one line of text at the first size that fits the width
// (shortened with "…" at the smallest) and returns the y below it.
func (c *canvas) line(w weight, sizes []int, s string, y int) int {
	var f font.Face
	for _, size := range sizes {
		if f = c.face(w, size); width(f, s) <= Width-2*margin {
			break
		}
	}
	c.text(f, margin, y+ascent(f), ellipsize(f, s, Width-2*margin))
	return y + lineHeight(f)
}

// message draws the text in a rounded frame between y and bottom and
// returns the y below it. It shrinks the type before it cuts the text.
func (c *canvas) message(msg string, y, bottom int) int {
	const pad = 6
	inner := Width - 2*margin - 2*pad
	var (
		f     = c.face(medium, 18)
		lines []string
	)
	for _, size := range []int{22, 20, 18, 16} {
		f = c.face(medium, size)
		lines = wrap(f, msg, inner)
		if 2*pad+len(lines)*lineHeight(f) <= bottom-y {
			break
		}
	}
	lh := lineHeight(f)
	n := min(len(lines), (bottom-y-2*pad)/lh)
	if n <= 0 {
		return y
	}
	if n < len(lines) {
		lines[n-1] = ellipsize(f, strings.Join(lines[n-1:], " "), inner)
	}
	h := 2*pad + n*lh
	box := roundRect(margin, float64(y), Width-margin, float64(y+h), 6)
	c.fill(image.Rect(margin, y, Width-margin, y+h), box, true)
	c.fill(image.Rect(margin, y, Width-margin, y+h), roundRect(margin+2, float64(y+2), Width-margin-2, float64(y+h-2), 4), false)
	for i := range n {
		c.text(f, margin+pad, y+pad+i*lh+ascent(f), lines[i])
	}
	return y + h + 4
}

// footer: a thin rule, the time of the last change at the left and the
// battery at the right.
func (c *canvas) footer(lang string, stamp time.Time, b *Battery) {
	if stamp.IsZero() && b == nil {
		return
	}
	c.rect(margin, footerTop, Width-margin, footerTop+1, true)
	f := c.face(regular, 13)
	base := Height - 5
	if !stamp.IsZero() {
		c.text(f, margin, base, tr(lang, "updated", "time", stamp.Format("15:04")))
	}
	if b != nil {
		c.battery(f, base, *b)
	}
}

func (c *canvas) battery(f font.Face, base int, b Battery) {
	p := max(0, min(100, b.Percent))
	// Body 24×12 with a tip, right-aligned; the level fills it.
	x1 := Width - margin - 3
	x0 := x1 - 24
	y0 := base - 11
	y1 := base + 1
	c.rect(x0, y0, x1, y1, true)
	c.rect(x0+2, y0+2, x1-2, y1-2, false)
	c.rect(x1, y0+3, x1+3, y1-3, true)
	if fill := (x1 - x0 - 6) * p / 100; fill > 0 {
		c.rect(x0+3, y0+3, x0+3+fill, y1-3, true)
	}
	if b.Charging {
		// A bolt in white with a black outline, over the level.
		cx, cy := float64(x0+12), float64(y0+6)
		bolt := func(x, y float64) bool {
			return triangle(cx+2, cy-6, cx-4, cy+1, cx+1, cy+1)(x, y) || triangle(cx-1, cy-1, cx+4, cy-1, cx-2, cy+6)(x, y)
		}
		c.fill(image.Rect(x0, y0-2, x1, y1+2), bolt, false)
	}
	label := strconv.Itoa(p) + "%"
	c.text(f, x0-4-width(f, label), base, label)
}

// Pack converts a frame into the panel's memory layout: rows top to
// bottom, 8 pixels per byte, the leftmost pixel in the most significant
// bit, 1 = white and 0 = black (SSD1681 black/white RAM, command 0x24).
// A 200×200 frame is 5000 bytes.
func Pack(p *image.Paletted) []byte {
	w, h := p.Rect.Dx(), p.Rect.Dy()
	stride := (w + 7) / 8
	out := bytes.Repeat([]byte{0xff}, stride*h)
	for y := range h {
		for x := range w {
			if p.Pix[y*p.Stride+x] == 1 {
				out[y*stride+x/8] &^= 0x80 >> (x % 8)
			}
		}
	}
	return out
}

// PNG encodes a frame (1-bit palette).
func PNG(p *image.Paletted) []byte {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, p); err != nil {
		panic("eink: png: " + err.Error()) // only fails on writer errors
	}
	return buf.Bytes()
}

// Frame is a rendered sign in both formats.
type Frame struct {
	Image *image.Paletted
	Bits  []byte // Pack(Image)
	ETag  string // quoted strong ETag of the content
}

func newFrame(p *image.Paletted) Frame {
	bits := Pack(p)
	sum := sha256.Sum256(bits)
	return Frame{Image: p, Bits: bits, ETag: `"` + hex.EncodeToString(sum[:8]) + `"`}
}

// OfflineFrame is Offline as a Frame.
func OfflineFrame(lang string) Frame { return newFrame(Offline(lang)) }

// Display renders the frames of one profile and remembers since when the
// content is unchanged – that's the "updated" time in the footer, so the
// frame doesn't change with every evaluation. It is safe for concurrent use.
type Display struct {
	mu    sync.Mutex
	key   [32]byte
	since time.Time
}

// Frame renders the state; battery may be nil.
func (d *Display) Frame(st engine.State, lang string, b *Battery) Frame {
	if !Supported(lang) {
		lang = "en"
	}
	key := sha256.Sum256(Pack(Render(Input{State: st, Lang: lang})))
	now := st.Updated
	if now.IsZero() {
		now = time.Now()
	}
	d.mu.Lock()
	if d.since.IsZero() || key != d.key {
		d.key, d.since = key, now
	}
	stamp := d.since
	d.mu.Unlock()
	return newFrame(Render(Input{State: st, Lang: lang, Stamp: stamp, Battery: b}))
}
