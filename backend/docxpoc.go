package main

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
)

// pocPart is a media file that must be added to the merged DOCX, along with the
// relationship entry in the referencing part's .rels that points at it.
type pocPart struct {
	Name  string
	RelID string
	Data  []byte
}

// pocImagePartName returns the media part name for a PoC screenshot inside the
// merged DOCX. Names are scoped per finding index so repeated findings cannot
// collide with each other or with the template's own image1..image7 assets.
func pocImagePartName(findingIdx, imgIdx int) string {
	return fmt.Sprintf("word/media/poc_%d_%d.png", findingIdx, imgIdx)
}

// pocImageRelID returns a unique relationship id for a PoC screenshot. The
// template already uses rId1..rId33, so the "rIdPocN" space is collision-safe.
func pocImageRelID(findingIdx, imgIdx int) string {
	return fmt.Sprintf("rIdPoc%d_%d", findingIdx, imgIdx)
}

// preparePOCImage decodes the raw uploaded bytes (png/jpeg/gif) into a PNG part
// plus its natural pixel dimensions. PNG is used so the part is covered by the
// template's existing Content_Types png default and Word renders it unchanged.
func preparePOCImage(data []byte) (pngBytes []byte, w, h int, err error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("decode poc image: %w", err)
	}
	b := img.Bounds()
	w = b.Dx()
	h = b.Dy()
	if w <= 0 || h <= 0 {
		return nil, 0, 0, fmt.Errorf("poc image has empty bounds")
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, 0, 0, fmt.Errorf("encode poc png: %w", err)
	}
	return buf.Bytes(), w, h, nil
}

// pocDrawingXML builds an inline <w:drawing> run for one PoC screenshot, sized
// to fit the template's PoC table cell (capped at ~4.5in wide). cx/cy are in
// EMU (914400 per inch). It returns a single <w:r> run (no <w:p> wrapper) so it
// can drop into the template's raw {{FindingPOCImages}} paragraph slot, which
// carries its own centered paragraph properties.
func pocDrawingXML(relID string, docPrID int, pxW, pxH int) string {
	const maxWEmu = 4114800 // 4.5 inches
	w := int64(pxW)
	h := int64(pxH)
	emuW := w * 9525
	emuH := h * 9525
	if emuW > maxWEmu {
		ratio := float64(maxWEmu) / float64(emuW)
		emuW = maxWEmu
		emuH = int64(float64(emuH) * ratio)
	}
	if emuH < 1 {
		emuH = 1
	}
	return inlineImageRun(relID, docPrID, emuW, emuH, "PoC Screenshot")
}

// inlineImageRun builds one inline <w:drawing> run for an image already
// registered as a relationship. cx/cy are the rendered size in EMU (914400 per
// inch). The run carries no paragraph of its own so callers control placement.
func inlineImageRun(relID string, docPrID int, cx, cy int64, name string) string {
	return fmt.Sprintf(`<w:r><w:rPr><w:noProof/></w:rPr><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="%d" cy="%d"/><wp:effectExtent l="0" t="0" r="0" b="0"/><wp:docPr id="%d" name="%s"/><wp:cNvGraphicFramePr><a:graphicFrameLocks xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" noChangeAspect="1"/></wp:cNvGraphicFramePr><a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:nvPicPr><pic:cNvPr id="%d" name="%s"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`,
		cx, cy, docPrID, name, docPrID, name, relID, cx, cy)
}

// anchoredImageRun builds one floating <w:drawing> run for an image already
// registered as a relationship. x/y place its top left corner relative to the
// top left corner of the page and cx/cy are its rendered size, all in EMU.
//
// The drawing wraps nothing and is allowed to overlap, so it sits on top of
// whatever it covers without moving a line of the text it floats over.
// layoutInCell is off so that an anchor written into a table cell - the cover's
// banner is one - is placed against the page rather than clamped to the cell it
// happens to live in.
func anchoredImageRun(relID string, docPrID int, x, y, cx, cy int64, name string) string {
	return fmt.Sprintf(`<w:r><w:rPr><w:noProof/></w:rPr><w:drawing><wp:anchor distT="0" distB="0" distL="0" distR="0" simplePos="0" relativeHeight="251658240" behindDoc="0" locked="0" layoutInCell="1" allowOverlap="1"><wp:simplePos x="0" y="0"/><wp:positionH relativeFrom="page"><wp:posOffset>%d</wp:posOffset></wp:positionH><wp:positionV relativeFrom="page"><wp:posOffset>%d</wp:posOffset></wp:positionV><wp:extent cx="%d" cy="%d"/><wp:effectExtent l="0" t="0" r="0" b="0"/><wp:wrapNone/><wp:docPr id="%d" name="%s"/><wp:cNvGraphicFramePr><a:graphicFrameLocks xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" noChangeAspect="1"/></wp:cNvGraphicFramePr><a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:nvPicPr><pic:cNvPr id="%d" name="%s"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:anchor></w:drawing></w:r>`,
		x, y, cx, cy, docPrID, name, docPrID, name, relID, cx, cy)
}

// encodeImagePNG re-encodes a decoded image as PNG bytes so it is covered by the
// package's existing png content-type default.
func encodeImagePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}
