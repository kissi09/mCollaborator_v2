package main

import "strings"

// Cell shading, and why a finding's rows need it stated rather than left alone.
//
// The finding tables use the template's GridTable4-Accent61 style, whose
// band1Horz conditional format fills every odd row with FDE9D9. The rows a
// finding's values sit in are all flagged as that band, so Description, Impact,
// Affected Device and Recommendation all printed on peach. The client's own
// reports put the peach behind the description and its rating only; everything
// under it is white. The band is not ours to remove - it is the template's
// table style - so the cells that should be white say so themselves.
//
// The same table style is why the values print bold: its firstCol format bolds
// the first column, which is where a full-width value cell sits. That is
// switched off per run by withRunBoldOff, not here.

// cellFillWhite is the fill a value cell takes when it must not carry the
// table style's banding.
const cellFillWhite = "FFFFFF"

// tcPrAfterShd are the CT_TcPr children the schema orders *after* <w:shd>.
// Shading is inserted before the first of these the cell already carries so the
// properties stay in schema order and Word does not offer to repair the file.
var tcPrAfterShd = []string{
	"<w:noWrap", "<w:tcMar", "<w:textDirection", "<w:tcFitText",
	"<w:vAlign", "<w:hideMark",
}

// withCellFill returns the cell with its shading set to hex, replacing whatever
// shading it had. A cell with no <w:tcPr> gets one.
func withCellFill(cell, hex string) string {
	shd := `<w:shd w:val="clear" w:color="auto" w:fill="` + hex + `"/>`

	open := strings.Index(cell, "<w:tcPr")
	if open < 0 {
		// No properties at all: open a block right after the cell's own tag.
		gt := strings.Index(cell, ">")
		if gt < 0 {
			return cell
		}
		return cell[:gt+1] + "<w:tcPr>" + shd + "</w:tcPr>" + cell[gt+1:]
	}

	// An empty <w:tcPr/> carries nothing to order against.
	if end := strings.Index(cell[open:], ">"); end >= 0 && strings.HasSuffix(cell[open:open+end+1], "/>") {
		return cell[:open] + "<w:tcPr>" + shd + "</w:tcPr>" + cell[open+end+1:]
	}

	close := strings.Index(cell[open:], "</w:tcPr>")
	if close < 0 {
		return cell
	}
	close += open
	props := cell[open:close]

	if stripped, ok := removeElement(props, "<w:shd"); ok {
		props = stripped
	}

	at := len(props)
	for _, tag := range tcPrAfterShd {
		if i := strings.Index(props, tag); i >= 0 && i < at {
			at = i
		}
	}
	return cell[:open] + props[:at] + shd + props[at:] + cell[close:]
}

// removeElement drops the first self-closing or paired element named tag from
// frag, reporting whether one was there.
func removeElement(frag, tag string) (string, bool) {
	i := strings.Index(frag, tag)
	if i < 0 {
		return frag, false
	}
	rest := frag[i:]
	if end := strings.Index(rest, "/>"); end >= 0 {
		// Self-closing, provided no '>' closes the start tag first.
		if gt := strings.Index(rest, ">"); gt == end+1 {
			return frag[:i] + rest[end+2:], true
		}
	}
	name := strings.TrimPrefix(tag, "<")
	if end := strings.Index(rest, "</"+name+">"); end >= 0 {
		return frag[:i] + rest[end+len("</"+name+">"):], true
	}
	return frag, false
}
