package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	_ "golang.org/x/image/webp"
)

// The VAPT template carries the previous client's logo as word/media/image1.png.
// Headers 5, 6 and 8 (the running page header) all reference that single part, so
// swapping its bytes drops the current customer's logo into the exact position,
// size and spacing the template already defines - no layout XML is touched.
const clientLogoPart = "word/media/image1.png"

// Natural pixel size of the template's client-logo asset. The header displays it
// at 1.41in x 0.38in (aspect 3.733), which matches these dimensions, so Word
// scales it 1:1. A replacement must keep the same canvas or Word stretches it.
const (
	clientLogoSlotW = 567
	clientLogoSlotH = 152
)

// svgRasterHeight is the height an SVG logo is rasterised at. The header prints
// the logo half an inch tall, so this is roughly eight times the resolution it
// is displayed at - enough that it stays sharp if someone zooms the PDF or
// reuses the header artwork at a larger size, without carrying a needless
// megabyte through the package.
const svgRasterHeight = 320

// decodeUploadedImage accepts either a bare base64 payload or a full
// "data:image/png;base64,...." URI as produced by the report wizard's FileReader.
//
// PNG, JPEG, GIF, WEBP and SVG are all accepted, because all of them are what a
// client sends when asked for their logo - a bank's press kit is usually SVG,
// and a logo saved off a web page is often WEBP. Neither decodes through the
// standard library, and until they did, the wizard accepted the upload, showed
// it in the preview (the browser can read both), and then produced a report with
// no logo in it at all and nothing on screen to say why.
func decodeUploadedImage(payload string) (image.Image, error) {
	raw, err := decodeUploadedBytes(payload)
	if err != nil {
		return nil, err
	}
	if isSVG(raw) {
		return rasterizeSVG(raw)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode logo image: %w", err)
	}
	return img, nil
}

// decodeUploadedBytes strips any data-URI wrapper and returns the raw file.
func decodeUploadedBytes(payload string) ([]byte, error) {
	s := strings.TrimSpace(payload)
	if i := strings.Index(s, ","); i >= 0 && strings.HasPrefix(s, "data:") {
		s = s[i+1:]
	}
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' {
			return -1
		}
		return r
	}, s)

	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("decode logo base64: %w", err)
	}
	return raw, nil
}

// isSVG reports whether the upload is SVG source rather than a raster image.
// The file may open with an XML declaration, a doctype or a comment before the
// root element, so the whole head of it is searched rather than just the start.
func isSVG(raw []byte) bool {
	head := raw
	if len(head) > 1024 {
		head = head[:1024]
	}
	return bytes.Contains(bytes.ToLower(head), []byte("<svg"))
}

// rasterizeSVG renders SVG source to a transparent RGBA image, scaled so its
// height is svgRasterHeight and its aspect ratio is the one the drawing
// declares.
func rasterizeSVG(raw []byte) (image.Image, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(raw), oksvg.WarnErrorMode)
	if err != nil {
		return nil, fmt.Errorf("read logo svg: %w", err)
	}
	vb := icon.ViewBox
	if vb.W <= 0 || vb.H <= 0 {
		return nil, fmt.Errorf("logo svg declares no size")
	}

	h := svgRasterHeight
	w := int(float64(h) * vb.W / vb.H)
	if w < 1 {
		w = 1
	}

	icon.SetTarget(0, 0, float64(w), float64(h))
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	scanner := rasterx.NewScannerGV(w, h, dst, dst.Bounds())
	icon.Draw(rasterx.NewDasher(w, h, scanner), 1.0)
	return dst, nil
}

// resizeBilinear scales src to exactly w x h using bilinear interpolation.
func resizeBilinear(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sb := src.Bounds()
	if w <= 0 || h <= 0 || sb.Dx() <= 0 || sb.Dy() <= 0 {
		return dst
	}
	xRatio := float64(sb.Dx()-1) / float64(w)
	yRatio := float64(sb.Dy()-1) / float64(h)

	for y := 0; y < h; y++ {
		sy := float64(y) * yRatio
		y0 := int(sy)
		yFrac := sy - float64(y0)
		for x := 0; x < w; x++ {
			sx := float64(x) * xRatio
			x0 := int(sx)
			xFrac := sx - float64(x0)

			r00, g00, b00, a00 := src.At(sb.Min.X+x0, sb.Min.Y+y0).RGBA()
			r10, g10, b10, a10 := src.At(sb.Min.X+x0+1, sb.Min.Y+y0).RGBA()
			r01, g01, b01, a01 := src.At(sb.Min.X+x0, sb.Min.Y+y0+1).RGBA()
			r11, g11, b11, a11 := src.At(sb.Min.X+x0+1, sb.Min.Y+y0+1).RGBA()

			blend := func(v00, v10, v01, v11 uint32) uint8 {
				top := float64(v00)*(1-xFrac) + float64(v10)*xFrac
				bot := float64(v01)*(1-xFrac) + float64(v11)*xFrac
				return uint8(uint32(top*(1-yFrac)+bot*yFrac) >> 8)
			}

			i := dst.PixOffset(x, y)
			dst.Pix[i+0] = blend(r00, r10, r01, r11)
			dst.Pix[i+1] = blend(g00, g10, g01, g11)
			dst.Pix[i+2] = blend(b00, b10, b01, b11)
			dst.Pix[i+3] = blend(a00, a10, a01, a11)
		}
	}
	return dst
}

// fitLogoToSlot letterboxes src onto a transparent slotW x slotH canvas,
// preserving aspect ratio and centring the result. Keeping the canvas identical
// to the template's asset means Word renders the header unchanged and the logo
// is never stretched, whatever shape the customer uploads.
func fitLogoToSlot(src image.Image, slotW, slotH int) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, slotW, slotH))
	draw.Draw(canvas, canvas.Bounds(), image.Transparent, image.Point{}, draw.Src)

	sb := src.Bounds()
	if sb.Dx() <= 0 || sb.Dy() <= 0 {
		return canvas
	}

	scale := float64(slotW) / float64(sb.Dx())
	if s := float64(slotH) / float64(sb.Dy()); s < scale {
		scale = s
	}
	w := int(float64(sb.Dx()) * scale)
	h := int(float64(sb.Dy()) * scale)
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}

	scaled := resizeBilinear(src, w, h)
	offset := image.Pt((slotW-w)/2, (slotH-h)/2)
	draw.Draw(canvas, image.Rect(offset.X, offset.Y, offset.X+w, offset.Y+h), scaled, image.Point{}, draw.Over)
	return canvas
}

// renderClientLogoPart turns the wizard's uploaded logo into PNG bytes sized for
// the template's client-logo slot. Returns nil (no error) when no logo was
// supplied, so the template's own asset is left in place.
func renderClientLogoPart(payload string) ([]byte, error) {
	if strings.TrimSpace(payload) == "" {
		return nil, nil
	}
	src, err := decodeUploadedImage(payload)
	if err != nil {
		return nil, err
	}
	fitted := fitLogoToSlot(src, clientLogoSlotW, clientLogoSlotH)

	var buf bytes.Buffer
	if err := png.Encode(&buf, fitted); err != nil {
		return nil, fmt.Errorf("encode logo png: %w", err)
	}
	return buf.Bytes(), nil
}
