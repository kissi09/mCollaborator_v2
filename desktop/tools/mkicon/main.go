// Command mkicon builds the Windows application icon from the Cyberteq brand
// assets already in the web app.
//
// The two source images each fall short on their own. cyberteq-mark.png is the
// monogram at a usable 282px, but it is white on transparent - dropped straight
// into an .ico it is invisible against Explorer's white background. The
// apple-touch-icon is the finished artwork, dark tile and all, but only 180px,
// which is smaller than the 256px entry Windows wants for large-icon views.
//
// So the tile is redrawn here at 256px in the brand colour sampled from the
// apple-touch-icon, and the full-resolution monogram is composited onto it. The
// result is crisp at every size Windows asks for rather than upscaled from 180.
//
// Usage:
//
//	go run ./tools/mkicon -mark ../backend/static/images/cyberteq-mark.png \
//	    -reference ../backend/static/apple-touch-icon.png -out build/windows/icon.ico
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

// iconSizes are the entries Windows looks for: 256 for the extra-large view
// down to 16 for the title bar and taskbar.
var iconSizes = []int{256, 128, 64, 48, 32, 16}

// pngEntryMinSize is the size at or above which the entry is stored as PNG.
// Below it, Windows shell versions are happier with an uncompressed BMP, so the
// small sizes are written the old way.
const pngEntryMinSize = 64

// tileRadiusRatio is the corner rounding of the tile, as a fraction of its
// width. 0.2237 is the squircle ratio Apple's icon grid uses, which is what the
// reference artwork was drawn to.
const tileRadiusRatio = 0.2237

func main() {
	markPath := flag.String("mark", "", "path to the white monogram PNG")
	refPath := flag.String("reference", "", "path to the finished icon artwork, used for colour and proportions")
	outPath := flag.String("out", "icon.ico", "path to write the .ico to")
	pngOut := flag.String("png", "", "optional path to also write the 256px artwork as PNG")
	flag.Parse()

	// Progress goes to stdout: this is a build tool, and PowerShell treats a
	// native command writing to stderr as a failure regardless of exit code.
	log.SetOutput(os.Stdout)
	log.SetFlags(0)

	if *markPath == "" || *refPath == "" {
		log.Fatal("both -mark and -reference are required")
	}

	mark, err := loadPNG(*markPath)
	if err != nil {
		log.Fatalf("mark: %v", err)
	}
	ref, err := loadPNG(*refPath)
	if err != nil {
		log.Fatalf("reference: %v", err)
	}

	bg := tileColour(ref)
	inset := markInset(ref)
	log.Printf("tile colour #%02X%02X%02X, monogram occupies %.1f%% of the tile",
		bg.R, bg.G, bg.B, (1-2*inset)*100)

	master := composeTile(mark, bg, inset, 1024)

	if *pngOut != "" {
		if err := writePNG(*pngOut, resize(master, 256)); err != nil {
			log.Fatalf("write png: %v", err)
		}
		log.Printf("wrote %s", *pngOut)
	}

	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		log.Fatalf("create output directory: %v", err)
	}
	if err := writeICO(*outPath, master, iconSizes); err != nil {
		log.Fatalf("write ico: %v", err)
	}

	info, _ := os.Stat(*outPath)
	log.Printf("wrote %s (%d bytes, %d entries: %v)", *outPath, info.Size(), len(iconSizes), iconSizes)
}

// ---------------------------------------------------------------------------
// reading the brand assets
// ---------------------------------------------------------------------------

func loadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return img, nil
}

// tileColour reads the tile's fill out of the reference artwork. The centre of
// the monogram is background rather than ink - the C is a ring - so the sample
// is taken just inside the left edge, which is tile everywhere.
func tileColour(ref image.Image) color.RGBA {
	b := ref.Bounds()
	x := b.Min.X + b.Dx()/25
	y := b.Min.Y + b.Dy()/2
	r, g, bb, _ := ref.At(x, y).RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bb >> 8), 0xFF}
}

// markInset measures how much clear tile the reference leaves around its
// monogram, as a fraction of the tile width, so the rebuilt icon keeps the
// artwork's proportions instead of a guessed margin.
func markInset(ref image.Image) float64 {
	b := ref.Bounds()
	bg := tileColour(ref)
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X, b.Min.Y
	found := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bb, a := ref.At(x, y).RGBA()
			if a < 0x8000 {
				continue // outside the rounded tile
			}
			// "Ink" is anything meaningfully lighter than the tile.
			if int(r>>8)-int(bg.R) < 60 && int(g>>8)-int(bg.G) < 60 && int(bb>>8)-int(bg.B) < 60 {
				continue
			}
			found = true
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if !found {
		return 0.18
	}
	left := float64(minX-b.Min.X) / float64(b.Dx())
	right := float64(b.Max.X-1-maxX) / float64(b.Dx())
	top := float64(minY-b.Min.Y) / float64(b.Dy())
	bottom := float64(b.Max.Y-1-maxY) / float64(b.Dy())
	return (left + right + top + bottom) / 4
}

// ---------------------------------------------------------------------------
// drawing
// ---------------------------------------------------------------------------

// composeTile draws the rounded tile at size and composites the monogram onto
// it at the measured inset. It is rendered large and downsampled per entry,
// which is what keeps the small sizes clean.
func composeTile(mark image.Image, bg color.RGBA, inset float64, size int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	drawRoundedRect(dst, bg, float64(size)*tileRadiusRatio)

	side := int(math.Round(float64(size) * (1 - 2*inset)))
	scaled := resize(toRGBA(mark), side)
	offset := (size - side) / 2
	draw.Draw(dst, image.Rect(offset, offset, offset+side, offset+side),
		scaled, image.Point{}, draw.Over)
	return dst
}

// drawRoundedRect fills a rounded rectangle over the whole image, anti-aliasing
// the corners by supersampling so the curve does not stair-step.
func drawRoundedRect(dst *image.RGBA, fill color.RGBA, radius float64) {
	b := dst.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	const samples = 4
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			inside := 0
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					px := float64(x) + (float64(sx)+0.5)/samples
					py := float64(y) + (float64(sy)+0.5)/samples
					if insideRoundedRect(px, py, w, h, radius) {
						inside++
					}
				}
			}
			if inside == 0 {
				continue
			}
			a := uint8(255 * inside / (samples * samples))
			dst.SetRGBA(x, y, color.RGBA{fill.R, fill.G, fill.B, a})
		}
	}
}

func insideRoundedRect(x, y, w, h, r float64) bool {
	// Nearest point on the inner rectangle whose corners the radius rounds.
	cx := math.Min(math.Max(x, r), w-r)
	cy := math.Min(math.Max(y, r), h-r)
	if x >= r && x <= w-r || y >= r && y <= h-r {
		return x >= 0 && x <= w && y >= 0 && y <= h
	}
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= r*r
}

func toRGBA(src image.Image) *image.RGBA {
	if r, ok := src.(*image.RGBA); ok {
		return r
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

// resize does a box-filter downscale in premultiplied space. Every source pixel
// that falls in a destination pixel contributes to it, which is what avoids the
// speckling a nearest-neighbour shrink gives a thin monogram at 16px.
func resize(src *image.RGBA, size int) *image.RGBA {
	sb := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	xr := float64(sb.Dx()) / float64(size)
	yr := float64(sb.Dy()) / float64(size)

	for y := 0; y < size; y++ {
		y0 := sb.Min.Y + int(float64(y)*yr)
		y1 := sb.Min.Y + int(math.Ceil(float64(y+1)*yr))
		if y1 > sb.Max.Y {
			y1 = sb.Max.Y
		}
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < size; x++ {
			x0 := sb.Min.X + int(float64(x)*xr)
			x1 := sb.Min.X + int(math.Ceil(float64(x+1)*xr))
			if x1 > sb.Max.X {
				x1 = sb.Max.X
			}
			if x1 <= x0 {
				x1 = x0 + 1
			}

			var sr, sg, sb2, sa, n float64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					c := src.RGBAAt(xx, yy)
					a := float64(c.A) / 255
					// Source is straight alpha; premultiply before averaging.
					sr += float64(c.R) * a
					sg += float64(c.G) * a
					sb2 += float64(c.B) * a
					sa += float64(c.A)
					n++
				}
			}
			if n == 0 {
				continue
			}
			alpha := sa / n
			if alpha < 0.5 {
				continue
			}
			// Back to straight alpha.
			k := 255 / alpha
			dst.SetRGBA(x, y, color.RGBA{
				R: clamp8(sr / n * k),
				G: clamp8(sg / n * k),
				B: clamp8(sb2 / n * k),
				A: clamp8(alpha),
			})
		}
	}
	return dst
}

func clamp8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// ---------------------------------------------------------------------------
// ICO container
// ---------------------------------------------------------------------------

// writeICO assembles the multi-resolution .ico. Large entries are PNG, small
// entries are 32-bit BMP - the layout Windows itself ships its icons in.
func writeICO(path string, master *image.RGBA, sizes []int) error {
	type entry struct {
		size int
		data []byte
	}
	entries := make([]entry, 0, len(sizes))
	for _, size := range sizes {
		img := resize(master, size)
		var data []byte
		var err error
		if size >= pngEntryMinSize {
			data, err = encodePNG(img)
		} else {
			data, err = encodeBMP(img)
		}
		if err != nil {
			return fmt.Errorf("%dpx: %w", size, err)
		}
		entries = append(entries, entry{size, data})
	}

	var buf bytes.Buffer
	// ICONDIR: reserved, type 1 (icon), image count.
	binary.Write(&buf, binary.LittleEndian, uint16(0))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(len(entries)))

	offset := 6 + 16*len(entries)
	for _, e := range entries {
		// A 256px entry is recorded as 0: the field is one byte.
		dim := byte(e.size)
		if e.size >= 256 {
			dim = 0
		}
		buf.WriteByte(dim)                                  // width
		buf.WriteByte(dim)                                  // height
		buf.WriteByte(0)                                    // palette size, 0 for true colour
		buf.WriteByte(0)                                    // reserved
		binary.Write(&buf, binary.LittleEndian, uint16(1))  // colour planes
		binary.Write(&buf, binary.LittleEndian, uint16(32)) // bits per pixel
		binary.Write(&buf, binary.LittleEndian, uint32(len(e.data)))
		binary.Write(&buf, binary.LittleEndian, uint32(offset))
		offset += len(e.data)
	}
	for _, e := range entries {
		buf.Write(e.data)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func encodePNG(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// encodeBMP writes the DIB an .ico entry expects: a BITMAPINFOHEADER whose
// height covers both the colour rows and the AND mask, then bottom-up BGRA
// rows, then the mask itself. The mask is redundant for a 32-bit icon but
// Windows still expects the bytes to be there.
func encodeBMP(img *image.RGBA) ([]byte, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(40))    // header size
	binary.Write(&buf, binary.LittleEndian, int32(w))      // width
	binary.Write(&buf, binary.LittleEndian, int32(h*2))    // height, doubled for the mask
	binary.Write(&buf, binary.LittleEndian, uint16(1))     // planes
	binary.Write(&buf, binary.LittleEndian, uint16(32))    // bits per pixel
	binary.Write(&buf, binary.LittleEndian, uint32(0))     // BI_RGB, uncompressed
	binary.Write(&buf, binary.LittleEndian, uint32(w*h*4)) // image size
	binary.Write(&buf, binary.LittleEndian, int32(0))      // x pixels per metre
	binary.Write(&buf, binary.LittleEndian, int32(0))      // y pixels per metre
	binary.Write(&buf, binary.LittleEndian, uint32(0))     // palette entries used
	binary.Write(&buf, binary.LittleEndian, uint32(0))     // important palette entries

	for y := h - 1; y >= 0; y-- {
		for x := 0; x < w; x++ {
			c := img.RGBAAt(b.Min.X+x, b.Min.Y+y)
			buf.Write([]byte{c.B, c.G, c.R, c.A})
		}
	}

	// AND mask: 1 bit per pixel, rows padded to 4 bytes. All zero - the alpha
	// channel above already carries the transparency.
	maskRow := ((w + 31) / 32) * 4
	buf.Write(make([]byte, maskRow*h))

	return buf.Bytes(), nil
}
