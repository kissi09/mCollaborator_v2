package main

// Footer layout fitting.
//
// The template lays every page footer out as one tabbed line - the Cyberteq
// entity on the left, "VAPT Report – <client>" centred, the page number right -
// and each section's footer carries tab stops sized for that section's page.
//
// That works while the centred item is short. It is not: the client's name is
// in it, and on a portrait page the centred string is wide enough that its left
// half reaches back past where "Cyberteq Falcon Ltd." ends. Word cannot move a
// tab backwards, so it gives up on the stop and butts the two strings together,
// which is how a delivered report came to read
//
//	Cyberteq Falcon Ltd.VAPT Report – BestPoint Savings & Loans
//
// The landscape section never showed it because its page is 5,650 twips wider.
//
// fitFooterSpacing measures each footer line against its own tab stops and, when
// the line would collide, steps the type down until the items are properly
// spaced. Nothing else about the footer changes, and a line that already fits -
// every line of the landscape footer, for one - is left at the template's size.

import (
	"regexp"
	"strconv"
	"strings"
)

// minFooterGap is the clear space required between two footer items, in twips.
// 504 twips is 0.35" - enough that the items read as three separate things.
const minFooterGap = 504

// minFooterSz is how far the type may be stepped down, in half-points. Below
// 8pt the footer stops being readable and a cramped line is the lesser evil.
const minFooterSz = 16

// defaultTabStop is Word's implicit tab interval, used only when a tab runs off
// the end of the stops the paragraph declares.
const defaultTabStop = 720

// arialWidths holds Arial advance widths in 1/1000 em, the metrics Word lays
// footers out with: the Footer style inherits Normal, and Normal is Arial.
var arialWidths = map[rune]int{
	' ': 278, '!': 278, '"': 355, '#': 556, '$': 556, '%': 889, '&': 667, '\'': 191,
	'(': 333, ')': 333, '*': 389, '+': 584, ',': 278, '-': 333, '.': 278, '/': 278,
	'0': 556, '1': 556, '2': 556, '3': 556, '4': 556, '5': 556, '6': 556, '7': 556,
	'8': 556, '9': 556, ':': 278, ';': 278, '<': 584, '=': 584, '>': 584, '?': 556,
	'@': 1015,
	'A': 667, 'B': 667, 'C': 722, 'D': 722, 'E': 667, 'F': 611, 'G': 778, 'H': 722,
	'I': 278, 'J': 500, 'K': 667, 'L': 556, 'M': 833, 'N': 722, 'O': 778, 'P': 667,
	'Q': 778, 'R': 722, 'S': 667, 'T': 611, 'U': 722, 'V': 667, 'W': 944, 'X': 667,
	'Y': 667, 'Z': 611,
	'[': 278, '\\': 278, ']': 278, '^': 469, '_': 556, '`': 333,
	'a': 556, 'b': 556, 'c': 500, 'd': 556, 'e': 556, 'f': 278, 'g': 556, 'h': 556,
	'i': 222, 'j': 222, 'k': 500, 'l': 222, 'm': 833, 'n': 556, 'o': 556, 'p': 556,
	'q': 556, 'r': 333, 's': 500, 't': 278, 'u': 556, 'v': 500, 'w': 722, 'x': 500,
	'y': 500, 'z': 500,
	'{': 334, '|': 260, '}': 334, '~': 584,
	'–': 556, '—': 1000, '‘': 222, '’': 222, '“': 333, '”': 333, '…': 1000, '•': 350,
	' ': 278, '©': 737, '®': 737, // first key is U+00A0, a non-breaking space
}

// arialFallbackWidth is used for anything outside the table - close enough for
// a decision about whether two footer items collide.
const arialFallbackWidth = 556

// arialEmWidth returns the width of s in 1/1000 em at Arial's metrics.
func arialEmWidth(s string) int {
	total := 0
	for _, r := range s {
		if w, ok := arialWidths[r]; ok {
			total += w
			continue
		}
		total += arialFallbackWidth
	}
	return total
}

// twipsAt converts a 1/1000 em width to twips at a size given in half-points.
// A half-point is 10 twips, so one em at size sz is sz*10 twips.
func twipsAt(emWidth, sz int) int { return emWidth * sz / 100 }

// ---------------------------------------------------------------------------
// paragraph model
// ---------------------------------------------------------------------------

// footerPiece is one run's contribution to a footer line: its text width and
// the size that run is set in.
type footerPiece struct {
	em int
	sz int
}

// tabStop is one resolved stop from a paragraph's `<w:tabs>` block.
type tabStop struct {
	pos int
	val string // "left", "center", "right", ...
}

var (
	paraTabsBlockRe = regexp.MustCompile(`(?s)<w:tabs>.*?</w:tabs>`)
	tabStopRe       = regexp.MustCompile(`<w:tab ([^>]*?)/>`)
	attrValRe       = regexp.MustCompile(`w:val="([^"]*)"`)
	attrPosRe       = regexp.MustCompile(`w:pos="(-?\d+)"`)
	runTabRe        = regexp.MustCompile(`<w:tab\s*/>`)
	szRe            = regexp.MustCompile(`<w:sz w:val="(\d+)"\s*/>`)
	szCsRe          = regexp.MustCompile(`<w:szCs w:val="(\d+)"\s*/>`)
)

// paraTabStops resolves the effective stops of a footer paragraph. The template
// clears the two stops the Footer style declares and sets its own, so the
// "clear" entries have to be honoured rather than treated as more stops.
func paraTabStops(para string) []tabStop {
	pPr := paraPPr(para)
	block := paraTabsBlockRe.FindString(pPr)
	if block == "" {
		return nil
	}
	byPos := map[int]string{}
	var order []int
	for _, m := range tabStopRe.FindAllStringSubmatch(block, -1) {
		attrs := m[1]
		pm := attrPosRe.FindStringSubmatch(attrs)
		if pm == nil {
			continue
		}
		pos, err := strconv.Atoi(pm[1])
		if err != nil {
			continue
		}
		val := "left"
		if vm := attrValRe.FindStringSubmatch(attrs); vm != nil {
			val = vm[1]
		}
		if val == "clear" {
			delete(byPos, pos)
			continue
		}
		if _, seen := byPos[pos]; !seen {
			order = append(order, pos)
		}
		byPos[pos] = val
	}
	var stops []tabStop
	for _, pos := range order {
		if val, ok := byPos[pos]; ok {
			stops = append(stops, tabStop{pos, val})
		}
	}
	// Declared out of order in some parts; the layout walk needs them sorted.
	for i := 1; i < len(stops); i++ {
		for j := i; j > 0 && stops[j].pos < stops[j-1].pos; j-- {
			stops[j], stops[j-1] = stops[j-1], stops[j]
		}
	}
	return stops
}

// paraDefaultSz is the run size a run inherits when it sets none itself: the
// paragraph mark's size if it has one, else the Footer style's 8pt.
func paraDefaultSz(para string) int {
	if m := szRe.FindStringSubmatch(paraPPr(para)); m != nil {
		if sz, err := strconv.Atoi(m[1]); err == nil && sz > 0 {
			return sz
		}
	}
	return minFooterSz
}

// footerSegments splits a footer paragraph into the runs of text between its
// tabs. A run holding `<w:tab/>` ends the current segment.
func footerSegments(para string) [][]footerPiece {
	def := paraDefaultSz(para)
	segments := [][]footerPiece{{}}
	for _, r := range childElems(para, "w:r") {
		run := para[r.Start:r.End]
		sz := def
		if m := szRe.FindStringSubmatch(run); m != nil {
			if v, err := strconv.Atoi(m[1]); err == nil && v > 0 {
				sz = v
			}
		}
		if text := elemText(run); text != "" {
			segments[len(segments)-1] = append(segments[len(segments)-1],
				footerPiece{arialEmWidth(text), sz})
		}
		if runTabRe.MatchString(run) {
			segments = append(segments, []footerPiece{})
		}
	}
	return segments
}

// segmentWidth measures a segment in twips, with every run capped at maxSz.
func segmentWidth(seg []footerPiece, maxSz int) int {
	total := 0
	for _, p := range seg {
		sz := p.sz
		if sz > maxSz {
			sz = maxSz
		}
		total += twipsAt(p.em, sz)
	}
	return total
}

// nextStop returns the first tab stop past cursor, or an implicit left stop on
// the next default interval when the declared stops are exhausted.
func nextStop(stops []tabStop, cursor int) tabStop {
	for _, s := range stops {
		if s.pos > cursor {
			return s
		}
	}
	return tabStop{(cursor/defaultTabStop + 1) * defaultTabStop, "left"}
}

// footerLineFits reports whether the line lays out with at least minFooterGap
// between every pair of adjacent items when no run exceeds maxSz.
func footerLineFits(segments [][]footerPiece, stops []tabStop, maxSz int) bool {
	cursor := 0
	for i, seg := range segments {
		w := segmentWidth(seg, maxSz)
		if i == 0 {
			cursor = w
			continue
		}
		stop := nextStop(stops, cursor)
		start := stop.pos
		switch stop.val {
		case "center":
			start = stop.pos - w/2
		case "right", "end":
			start = stop.pos - w
		}
		if w > 0 && start-cursor < minFooterGap {
			return false
		}
		cursor = start + w
	}
	return true
}

// capParaSz rewrites every explicit run size in the paragraph that exceeds
// maxSz down to maxSz, leaving smaller runs alone.
func capParaSz(para string, maxSz int) string {
	shrink := func(re *regexp.Regexp, tag string) {
		para = re.ReplaceAllStringFunc(para, func(m string) string {
			sm := re.FindStringSubmatch(m)
			v, err := strconv.Atoi(sm[1])
			if err != nil || v <= maxSz {
				return m
			}
			return `<` + tag + ` w:val="` + strconv.Itoa(maxSz) + `"/>`
		})
	}
	shrink(szRe, "w:sz")
	shrink(szCsRe, "w:szCs")
	return para
}

// maxParaSz is the largest explicit run size in the paragraph.
func maxParaSz(para string) int {
	max := 0
	for _, m := range szRe.FindAllStringSubmatch(para, -1) {
		if v, err := strconv.Atoi(m[1]); err == nil && v > max {
			max = v
		}
	}
	return max
}

// fitFooterSpacing steps the type of any tabbed footer line down until its
// items clear each other, and returns the footer unchanged when they already do.
//
// It runs last, after the placeholders have been filled, because the width that
// decides the outcome is the width of the client's actual name.
func fitFooterSpacing(footer string) string {
	var b strings.Builder
	prev := 0
	changed := false
	for _, p := range childElems(footer, "w:p") {
		para := footer[p.Start:p.End]
		// Only tabbed lines can collide. The two table-based footer parts lay
		// their three items out in cells, which cannot overlap.
		if !runTabRe.MatchString(para) {
			continue
		}
		stops := paraTabStops(para)
		if len(stops) == 0 {
			continue
		}
		segments := footerSegments(para)
		if len(segments) < 2 {
			continue
		}
		startSz := maxParaSz(para)
		if startSz == 0 || footerLineFits(segments, stops, startSz) {
			continue
		}
		fitted := startSz
		for sz := startSz - 1; sz >= minFooterSz; sz-- {
			fitted = sz
			if footerLineFits(segments, stops, sz) {
				break
			}
		}
		b.WriteString(footer[prev:p.Start])
		b.WriteString(capParaSz(para, fitted))
		prev = p.End
		changed = true
	}
	if !changed {
		return footer
	}
	b.WriteString(footer[prev:])
	return b.String()
}
