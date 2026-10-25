package pages

import (
	"fmt"
	"math"
)

// waveBar is one bar of a drawn voice recording.
type waveBar struct {
	Height int    // px
	Color  string // "" for a bar not played yet
}

// brandStops are the logo's bar colours, orange to blue.
var brandStops = [][3]float64{
	{0xf3, 0x84, 0x0f}, {0xf4, 0x64, 0x22}, {0xe3, 0x1a, 0x5a},
	{0x9f, 0x15, 0x74}, {0x5a, 0x26, 0x92}, {0x14, 0x34, 0x95},
}

// wave draws n bars of a made-up voice (the same for a given seed), up to
// height px. The first played share of bars takes the logo's colours.
func wave(n, height, seed int, played float64) []waveBar {
	bars := make([]waveBar, n)
	s, prev := seed, 0.5
	for i := range bars {
		s = (s*9301 + 49297) % 233280
		v := 0.55*prev + 0.45*float64(s)/233280
		prev = v
		env := math.Pow(math.Sin(math.Pi*(float64(i)+0.5)/float64(n)), 0.45)
		gap := 1.0
		if i%11 == 7 || i%17 == 3 {
			gap = 0.35
		}
		bars[i].Height = max(3, int(float64(height)*env*v*gap*1.25))
		if t := float64(i) / float64(n); t < played {
			bars[i].Color = brandColor(t / played)
		}
	}
	return bars
}

// brandColor picks the colour at t (0 to 1) along the logo's bars.
func brandColor(t float64) string {
	p := min(t, 0.9999) * float64(len(brandStops)-1)
	i, f := int(p), p-math.Floor(p)
	a, b := brandStops[i], brandStops[i+1]
	return fmt.Sprintf("rgb(%d,%d,%d)", int(a[0]+(b[0]-a[0])*f), int(a[1]+(b[1]-a[1])*f), int(a[2]+(b[2]-a[2])*f))
}

func px(n int) string { return fmt.Sprintf("%dpx", n) }

func (b waveBar) style() string {
	s := fmt.Sprintf("height:%dpx", b.Height)
	if b.Color != "" {
		s += ";background:" + b.Color
	}
	return s
}

// voiceBar is one bar of the landing page's big voice wave: its height in
// percent of the wave, its logo colour (the played part always runs orange to
// blue), and whether it starts played.
type voiceBar struct {
	Pct   int
	Color string
	On    bool
}

// voiceWave draws n bars of a made-up voice (the page replaces their heights
// with the real recording's when the browser can read it).
func voiceWave(n, seed int, played float64) []voiceBar {
	bars := make([]voiceBar, n)
	for i, b := range wave(n, 100, seed, 0) {
		t := float64(i) / float64(n)
		bars[i] = voiceBar{min(b.Height, 100), brandColor(min(t/played, 1)), t < played}
	}
	return bars
}

func (b voiceBar) style() string {
	return fmt.Sprintf("height:%d%%;--c:%s", b.Pct, b.Color)
}
