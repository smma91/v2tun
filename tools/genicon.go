//go:build ignore

// genicon draws the app icon (a shield): the Windows .ico files, the tray icons
// for Linux (PNG) and the icon used by the UI and the Linux packages.
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

// halfWidth of the shield (in a 24x24 box) at height y, or -1 outside.
func halfWidth(y float64) float64 {
	switch {
	case y < 2 || y > 22:
		return -1
	case y < 5:
		return 8 * (y - 2) / 3
	case y < 11:
		return 8
	default:
		t := (y - 11) / 11
		return 8 * math.Sqrt(math.Max(0, 1-t*t))
	}
}

func inside(x, y, inset float64) bool {
	w := halfWidth(y)
	return w > 0 && math.Abs(x-12) <= w-inset
}

type palette struct{ top, bottom [3]float64 }

var (
	blue  = palette{[3]float64{59, 130, 246}, [3]float64{29, 78, 216}}
	green = palette{[3]float64{34, 197, 94}, [3]float64{21, 128, 61}}
	gray  = palette{[3]float64{148, 163, 184}, [3]float64{100, 116, 139}}
	cur   = blue
)

func draw(size int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 4 // supersampling
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var cover, inner float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+.5)/ss) * 24 / float64(size)
					y := (float64(py) + (float64(sy)+.5)/ss) * 24 / float64(size)
					if inside(x, y, 0) {
						cover++
					}
					// white "tunnel" arc inside the shield
					d := math.Hypot(x-12, y-12)
					if d > 3.2 && d < 5.0 && y > 9.5 && inside(x, y, 1.5) {
						inner++
					}
					if d < 1.7 {
						inner++
					}
				}
			}
			n := float64(ss * ss)
			if cover == 0 {
				continue
			}
			t := float64(py) / float64(size)
			r := cur.top[0] + (cur.bottom[0]-cur.top[0])*t
			g := cur.top[1] + (cur.bottom[1]-cur.top[1])*t
			b := cur.top[2] + (cur.bottom[2]-cur.top[2])*t
			k := inner / n
			r, g, b = r+(255-r)*k, g+(255-g)*k, b+(255-b)*k
			img.Set(px, py, color.NRGBA{uint8(r), uint8(g), uint8(b), uint8(255 * cover / n)})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func main() {
	writeICO("build/windows/app.ico", blue, []int{16, 24, 32, 48, 64, 256})
	writeICO("cmd/v2tun/assets/tray_on.ico", green, []int{16, 20, 24, 32, 48})
	writeICO("cmd/v2tun/assets/tray_off.ico", gray, []int{16, 20, 24, 32, 48})
	cur = green
	os.WriteFile("cmd/v2tun/assets/tray_on.png", draw(64), 0o644)
	cur = gray
	os.WriteFile("cmd/v2tun/assets/tray_off.png", draw(64), 0o644)
	cur = blue
	png.Encode(must(os.Create("cmd/v2tun/frontend/icon.png")), decode(draw(256)))
}

func writeICO(name string, p palette, sizes []int) {
	cur = p
	var imgs [][]byte
	for _, s := range sizes {
		imgs = append(imgs, draw(s))
	}
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		w := uint8(s)
		if s == 256 {
			w = 0
		}
		binary.Write(&out, binary.LittleEndian, struct {
			W, H, Colors, Reserved uint8
			Planes, BPP            uint16
			Size, Offset           uint32
		}{w, w, 0, 0, 1, 32, uint32(len(imgs[i])), uint32(offset)})
		offset += len(imgs[i])
	}
	for _, b := range imgs {
		out.Write(b)
	}
	os.WriteFile(name, out.Bytes(), 0o644)
}

func must(f *os.File, err error) *os.File {
	if err != nil {
		panic(err)
	}
	return f
}

func decode(b []byte) image.Image {
	i, _ := png.Decode(bytes.NewReader(b))
	return i
}
