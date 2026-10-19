package testbook

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"testing"
)

// Colors returns a short book that exercises color: a heading, words and a
// code listing in colors, gray and near-black text that should take the
// reader's tint, white text on a dark box, a bright color photograph, a
// diagram on white paper and line art on a transparent background.
func Colors(t testing.TB) Book {
	t.Helper()
	return Book{
		Title: "Colors",
		Chapters: []string{
			`<h1 style="color:#B22222">Red heading</h1>
<p>Body text in plain black, which takes the reader's tint. Then
<span style="color:#1565C0">some words in blue</span> and
<em style="color:#2E7D32">a few in green</em>.</p>
<p style="color:#757575">A gray caption, also tinted.</p>
<h2 style="color:#1A1A2E">A near-black heading</h2>
<pre><span style="color:#0000FF">func</span> <span style="color:#795E26">main</span>() {
    <span style="color:#008000">// A comment in green.</span>
    fmt.Println(<span style="color:#A31515">"a string"</span>, <span style="color:#098658">42</span>)
}</pre>
<div style="background-color:#202020;padding:1em"><p style="color:#FFFFFF">White text on a dark box.</p></div>`,
			`<p>A photograph:</p><img src="photo.jpg" style="width:300pt"/>`,
			`<p>A diagram:</p><img src="diagram.png" style="width:300pt"/>
<p>Line art on a transparent background:</p><img src="lines.png" style="width:240pt"/>`,
		},
		Files: map[string][]byte{
			"photo.jpg":   JPEG(t, Photo()),
			"diagram.png": PNG(t, Diagram()),
			"lines.png":   PNG(t, LineArt()),
		},
	}
}

// Photo returns a bright landscape: a sky, a sun, green hills and a red
// house. Its mean luminance is above one half, like a sunny photograph, and
// it has hardly any white.
func Photo() image.Image {
	const w, h = 480, 320
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			fx, fy := float64(x)/w, float64(y)/h
			// Sky: light blue at the top, paler near the horizon.
			c := mix(color.RGBA{R: 96, G: 168, B: 232, A: 255}, color.RGBA{R: 176, G: 214, B: 236, A: 255}, fy/0.6)
			if d := math.Hypot(fx-0.78, fy-0.2); d < 0.07 {
				c = color.RGBA{R: 255, G: 214, B: 64, A: 255}
			}
			hill := 0.6 + 0.06*math.Sin(fx*9) + 0.03*math.Sin(fx*23+1)
			if fy > hill {
				c = mix(color.RGBA{R: 92, G: 170, B: 70, A: 255}, color.RGBA{R: 40, G: 110, B: 40, A: 255}, (fy-hill)/(1-hill))
			}
			if fx > 0.2 && fx < 0.36 && fy > 0.5 && fy < 0.7 {
				c = color.RGBA{R: 200, G: 40, B: 40, A: 255}
			}
			// Grain, so it compresses like a photograph.
			n := int(hash(x, y)%17) - 8
			img.SetRGBA(x, y, color.RGBA{R: clamp(int(c.R) + n), G: clamp(int(c.G) + n), B: clamp(int(c.B) + n), A: 255})
		}
	}
	return img
}

// Diagram returns line art on white paper: black boxes joined by an arrow,
// one of them filled with light red, and a blue rule.
func Diagram() image.Image {
	const w, h = 400, 240
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fill(img, img.Bounds(), color.RGBA{R: 255, G: 255, B: 255, A: 255})
	black := color.RGBA{A: 255}
	fill(img, image.Rect(40, 60, 160, 140), color.RGBA{R: 244, G: 204, B: 204, A: 255})
	frame(img, image.Rect(40, 60, 160, 140), 4, color.RGBA{R: 204, G: 0, B: 0, A: 255})
	frame(img, image.Rect(240, 60, 360, 140), 4, black)
	fill(img, image.Rect(160, 98, 232, 102), black)
	for i := range 12 { // arrow head
		fill(img, image.Rect(228+i/2, 88+i, 230+i/2, 112-i), black)
	}
	fill(img, image.Rect(40, 190, 360, 196), color.RGBA{R: 0, G: 64, B: 200, A: 255})
	return img
}

// LineArt returns black strokes on a transparent background: a frame and a
// diagonal, as an icon or a chart is often drawn.
func LineArt() image.Image {
	const w, h = 240, 160
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	black := color.NRGBA{A: 255}
	for _, r := range []image.Rectangle{
		image.Rect(10, 10, 230, 16), image.Rect(10, 144, 230, 150),
		image.Rect(10, 10, 16, 150), image.Rect(224, 10, 230, 150),
	} {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				img.SetNRGBA(x, y, black)
			}
		}
	}
	for x := 16; x < 224; x++ {
		y := 144 - (x-16)*128/208
		for dy := -3; dy <= 3; dy++ {
			img.SetNRGBA(x, y+dy, black)
		}
	}
	return img
}

// JPEG encodes img at a quality typical of books.
func JPEG(t testing.TB, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func mix(a, b color.RGBA, f float64) color.RGBA {
	f = max(0, min(1, f))
	lerp := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*f) }
	return color.RGBA{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B), A: 255}
}

func clamp(v int) uint8 {
	return uint8(max(0, min(255, v)))
}

func hash(x, y int) uint32 {
	h := uint32(x)*374761393 + uint32(y)*668265263
	h = (h ^ h>>13) * 1274126177
	return h ^ h>>16
}

func fill(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func frame(img *image.RGBA, r image.Rectangle, width int, c color.RGBA) {
	fill(img, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+width), c)
	fill(img, image.Rect(r.Min.X, r.Max.Y-width, r.Max.X, r.Max.Y), c)
	fill(img, image.Rect(r.Min.X, r.Min.Y, r.Min.X+width, r.Max.Y), c)
	fill(img, image.Rect(r.Max.X-width, r.Min.Y, r.Max.X, r.Max.Y), c)
}
