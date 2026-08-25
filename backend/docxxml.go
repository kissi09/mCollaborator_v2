package main

// docxxml.go holds the low-level OOXML string surgery the mCollaborator report
// builder runs on word/document.xml. The template is edited as raw XML rather
// than through a document object model so that every style, chart, header and
// numbering definition Word wrote stays byte-identical - only the ranges we
// deliberately rewrite change.

import (
	"regexp"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// element scanning
// ---------------------------------------------------------------------------

// isTagBoundary reports whether the byte following a tag name terminates it.
func isTagBoundary(c byte) bool {
	return c == ' ' || c == '>' || c == '/' || c == '\t' || c == '\n' || c == '\r'
}

// findElem returns the offset of the next `<tag` opening tag at or after from,
// or -1. Matches only whole tag names, so "w:t" never matches "w:tbl".
func findElem(s string, from int, tag string) int {
	open := "<" + tag
	for i := from; ; {
		j := strings.Index(s[i:], open)
		if j < 0 {
			return -1
		}
		j += i
		if j+len(open) < len(s) && isTagBoundary(s[j+len(open)]) {
			return j
		}
		i = j + 1
	}
}

// lastElemStart returns the offset of the last `<tag` opening tag in s, or -1.
// Whole-name matching keeps "<w:r" from latching onto "<w:rPr".
func lastElemStart(s, tag string) int {
	open := "<" + tag
	for i := len(s); i > 0; {
		j := strings.LastIndex(s[:i], open)
		if j < 0 {
			return -1
		}
		if j+len(open) < len(s) && isTagBoundary(s[j+len(open)]) {
			return j
		}
		i = j
	}
	return -1
}

// elemEnd returns the offset just past the element that starts at start, which
// must point at the '<' of an opening `tag`. Self-closing tags are handled.
// Returns -1 when the document is malformed.
func elemEnd(s string, start int, tag string) int {
	gt := strings.Index(s[start:], ">")
	if gt < 0 {
		return -1
	}
	gt += start
	if s[gt-1] == '/' {
		return gt + 1
	}
	open := "<" + tag
	closeTag := "</" + tag + ">"
	depth := 1
	i := gt + 1
	for i < len(s) {
		nextClose := strings.Index(s[i:], closeTag)
		if nextClose < 0 {
			return -1
		}
		nextClose += i
		// Count nested opens that occur before this close.
		for k := i; k < nextClose; {
			n := strings.Index(s[k:nextClose], open)
			if n < 0 {
				break
			}
			n += k
			if n+len(open) < len(s) && isTagBoundary(s[n+len(open)]) {
				// Self-closing nested tags do not add depth.
				if e := strings.Index(s[n:], ">"); e > 0 && s[n+e-1] != '/' {
					depth++
				}
			}
			k = n + 1
		}
		depth--
		i = nextClose + len(closeTag)
		if depth == 0 {
			return i
		}
	}
	return -1
}

// span is a half-open [Start,End) byte range inside a document.
type span struct{ Start, End int }

// childElems returns the ranges of every outermost `tag` element inside s,
// without descending into an already-matched one.
func childElems(s, tag string) []span {
	var out []span
	for i := 0; ; {
		st := findElem(s, i, tag)
		if st < 0 {
			return out
		}
		en := elemEnd(s, st, tag)
		if en < 0 {
			return out
		}
		out = append(out, span{st, en})
		i = en
	}
}

// ---------------------------------------------------------------------------
// text extraction
// ---------------------------------------------------------------------------

var wtRe = regexp.MustCompile(`<w:t(?: [^>]*)?>([^<]*)</w:t>`)
var wtEmptyRe = regexp.MustCompile(`<w:t(?: [^>]*)?/>`)
var gridSpanRe = regexp.MustCompile(`<w:gridSpan [^>]*/>`)
var bottomBorderRe = regexp.MustCompile(`<w:bottom [^>]*/>`)

// elemText concatenates the visible text of an OOXML fragment.
func elemText(frag string) string {
	var b strings.Builder
	for _, m := range wtRe.FindAllStringSubmatch(frag, -1) {
		b.WriteString(xmlUnescape(m[1]))
	}
	return b.String()
}

var xmlUnescaper = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", string(rune(34)), "&apos;", string(rune(39)), "&amp;", "&")

func xmlUnescape(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	return xmlUnescaper.Replace(s)
}

var xmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func xmlEscape(s string) string { return xmlEscaper.Replace(s) }

// ---------------------------------------------------------------------------
// run normalization
// ---------------------------------------------------------------------------

var rPrRe = regexp.MustCompile(`(?s)^<w:rPr>.*?</w:rPr>`)

// simpleRun splits a `<w:r ...>...</w:r>` fragment into its run properties and
// its text. ok is false when the run holds anything other than rPr and w:t
// (drawings, field codes, breaks, footnote marks), because those must never be
// merged away.
func simpleRun(frag string) (rPr, text string, ok bool) {
	gt := strings.Index(frag, ">")
	if gt < 0 || !strings.HasSuffix(frag, "</w:r>") {
		return "", "", false
	}
	body := strings.TrimSuffix(frag[gt+1:], "</w:r>")
	if m := rPrRe.FindString(body); m != "" {
		rPr = m
		body = body[len(m):]
	}
	// Everything left must be w:t elements.
	rest := wtRe.ReplaceAllString(body, "")
	rest = wtEmptyRe.ReplaceAllString(rest, "")
	if strings.TrimSpace(rest) != "" {
		return "", "", false
	}
	return rPr, elemText(body), true
}

// placeholderRe matches the template's square-bracket placeholders. Word splits
// them across runs freely - "[WPT" and " Vulnerability 1]" are two runs with
// different run properties in the shipped template - so they have to be pulled
// back together before any literal match can find them.
var placeholderRe = regexp.MustCompile(`\[[^\[\]<>]{1,64}\]`)

// paragraphRun is one run of a paragraph together with the slice of the
// paragraph's concatenated text that it contributes.
type paragraphRun struct {
	span
	TextStart, TextEnd int
	RPr                string
	Simple             bool
}

// paragraphRuns returns every run of a paragraph and the paragraph's full text.
func paragraphRuns(para string) ([]paragraphRun, string) {
	var runs []paragraphRun
	var text strings.Builder
	for _, r := range childElems(para, "w:r") {
		rPr, t, ok := simpleRun(para[r.Start:r.End])
		start := text.Len()
		if ok {
			text.WriteString(t)
		}
		runs = append(runs, paragraphRun{r, start, text.Len(), rPr, ok})
	}
	return runs, text.String()
}

// healParagraphPlaceholders rewrites a paragraph so that each placeholder sits
// whole inside a single run, taking the run properties of the run the
// placeholder starts in. Text outside the placeholders keeps its own runs and
// formatting, so only what has to change does.
func healParagraphPlaceholders(para string) string {
	runs, text := paragraphRuns(para)
	if len(runs) < 2 || !strings.Contains(text, "[") {
		return para
	}
	matches := placeholderRe.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return para
	}

	// Apply back to front so earlier run offsets stay valid.
	for m := len(matches) - 1; m >= 0; m-- {
		lo, hi := matches[m][0], matches[m][1]
		first, last := -1, -1
		for i, r := range runs {
			if !r.Simple {
				continue
			}
			if r.TextEnd > lo && first == -1 {
				first = i
			}
			if r.TextStart < hi {
				last = i
			}
		}
		if first < 0 || last <= first {
			continue // already whole, or not found
		}
		spansOnlySimpleRuns := true
		for i := first; i <= last; i++ {
			if !runs[i].Simple {
				spansOnlySimpleRuns = false
				break
			}
		}
		if !spansOnlySimpleRuns {
			continue
		}
		prefix := text[runs[first].TextStart:lo]
		suffix := text[hi:runs[last].TextEnd]
		merged := `<w:r>` + runs[first].RPr + `<w:t xml:space="preserve">` +
			xmlEscape(prefix+text[lo:hi]+suffix) + `</w:t></w:r>`
		para = para[:runs[first].Start] + merged + para[runs[last].End:]
		runs, text = paragraphRuns(para)
	}
	return para
}

// healPlaceholderRuns applies healParagraphPlaceholders to every paragraph that
// carries a '[', leaving the rest of the package byte-identical to what Word
// produced.
func healPlaceholderRuns(s string) string {
	var b strings.Builder
	prev := 0
	for _, p := range childElems(s, "w:p") {
		frag := s[p.Start:p.End]
		if !strings.Contains(frag, "[") {
			continue
		}
		healed := healParagraphPlaceholders(frag)
		if healed == frag {
			continue
		}
		b.WriteString(s[prev:p.Start])
		b.WriteString(healed)
		prev = p.End
	}
	if prev == 0 {
		return s
	}
	b.WriteString(s[prev:])
	return b.String()
}

// ---------------------------------------------------------------------------
// paragraph / table editing
// ---------------------------------------------------------------------------

var pPrRe = regexp.MustCompile(`(?s)^<w:pPr>.*?</w:pPr>`)

// paraPPr returns the `<w:pPr>` block of a paragraph fragment, or "".
func paraPPr(para string) string {
	gt := strings.Index(para, ">")
	if gt < 0 {
		return ""
	}
	return pPrRe.FindString(para[gt+1:])
}

// paraFirstRPr returns the run properties of the paragraph's first run so that
// replacement text inherits the template's font, size and colour.
func paraFirstRPr(para string) string {
	for _, r := range childElems(para, "w:r") {
		if rPr, _, ok := simpleRun(para[r.Start:r.End]); ok {
			return rPr
		}
	}
	// Fall back to the paragraph mark's run properties.
	if pPr := paraPPr(para); pPr != "" {
		if m := rPrRe.FindString(strings.TrimPrefix(pPr, "<w:pPr>")); m != "" {
			return m
		}
	}
	return ""
}

// runsForText renders text as one or more runs, turning newlines into Word line
// breaks so multi-line finding fields keep their shape.
func runsForText(rPr, text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteString(`<w:r>` + rPr + `<w:br/></w:r>`)
		}
		if line == "" {
			continue
		}
		b.WriteString(`<w:r>` + rPr + `<w:t xml:space="preserve">` + xmlEscape(line) + `</w:t></w:r>`)
	}
	return b.String()
}

// setParaText rebuilds a paragraph so it contains exactly text, keeping the
// paragraph properties and borrowing the run properties already in place.
func setParaText(para, text string) string {
	gt := strings.Index(para, ">")
	if gt < 0 {
		return para
	}
	open := para[:gt+1]
	pPr := paraPPr(para)
	rPr := paraFirstRPr(para)
	return open + pPr + runsForText(rPr, text) + `</w:p>`
}

// textElems returns the ranges of every `<w:t>` element in a fragment, in
// document order.
func textElems(frag string) []span { return childElems(frag, "w:t") }

// nthText returns the content of the n-th (0-based) `<w:t>` in frag, or "".
func nthText(frag string, n int) string {
	els := textElems(frag)
	if n >= len(els) {
		return ""
	}
	return elemText(frag[els[n].Start:els[n].End])
}

// replaceNthText rewrites the content of the n-th (0-based) `<w:t>` in frag,
// leaving its run and its attributes alone. Used where a paragraph's meaning is
// positional, as in a table-of-contents entry: number, title, page number.
func replaceNthText(frag string, n int, value string) string {
	els := textElems(frag)
	if n >= len(els) {
		return frag
	}
	el := frag[els[n].Start:els[n].End]
	gt := strings.Index(el, ">")
	if gt < 0 || strings.HasSuffix(el, "/>") {
		return frag
	}
	rebuilt := el[:gt+1] + xmlEscape(value) + "</w:t>"
	return frag[:els[n].Start] + rebuilt + frag[els[n].End:]
}

// tableRows returns the row ranges of a `<w:tbl>` fragment.
func tableRows(tbl string) []span { return childElems(tbl, "w:tr") }

// rowCells returns the cell ranges of a `<w:tr>` fragment.
func rowCells(row string) []span { return childElems(row, "w:tc") }

// cellGridSpan sets how many grid columns a table cell occupies.
func cellGridSpan(cell string, span int) string {
	tag := `<w:gridSpan w:val="` + strconv.Itoa(span) + `"/>`
	if i := strings.Index(cell, "<w:tcPr>"); i >= 0 {
		end := strings.Index(cell, "</w:tcPr>")
		if end > i {
			inner := gridSpanRe.ReplaceAllString(cell[i+len("<w:tcPr>"):end], "")
			// gridSpan must follow tcW, which is always the first child Word writes.
			return cell[:i] + "<w:tcPr>" + inner + tag + cell[end:]
		}
	}
	gt := strings.Index(cell, ">")
	if gt < 0 {
		return cell
	}
	return cell[:gt+1] + "<w:tcPr>" + tag + "</w:tcPr>" + cell[gt+1:]
}

// setCellBottomBorder replaces a cell's bottom border, adding the border block
// when the cell has none.
func setCellBottomBorder(cell, border string) string {
	i := strings.Index(cell, "<w:tcBorders>")
	if i >= 0 {
		end := strings.Index(cell, "</w:tcBorders>")
		if end > i {
			inner := bottomBorderRe.ReplaceAllString(cell[i+len("<w:tcBorders>"):end], "")
			return cell[:i] + "<w:tcBorders>" + inner + border + cell[end:]
		}
	}
	if j := strings.Index(cell, "<w:tcPr>"); j >= 0 {
		return cell[:j+len("<w:tcPr>")] + "<w:tcBorders>" + border + "</w:tcBorders>" + cell[j+len("<w:tcPr>"):]
	}
	gt := strings.Index(cell, ">")
	if gt < 0 {
		return cell
	}
	return cell[:gt+1] + "<w:tcPr><w:tcBorders>" + border + "</w:tcBorders></w:tcPr>" + cell[gt+1:]
}

// setFirstEmptyParaText writes text into the first paragraph of frag that has no
// text of its own, and reports whether it found one. Template value cells are
// empty paragraphs sitting under a label row, and recommendation cells keep
// their heading paragraph followed by an empty body paragraph, so "first empty
// paragraph" addresses both without a per-table layout map.
func setFirstEmptyParaText(frag, text string) (string, bool) {
	for _, p := range childElems(frag, "w:p") {
		para := frag[p.Start:p.End]
		if strings.TrimSpace(elemText(para)) != "" {
			continue
		}
		return frag[:p.Start] + setParaText(para, text) + frag[p.End:], true
	}
	return frag, false
}

// ---------------------------------------------------------------------------
// run colour
// ---------------------------------------------------------------------------

var runColorRe = regexp.MustCompile(`<w:color [^>]*/>`)

// rPrColorFollowers are the CT_RPr children that the schema orders *after*
// <w:color>. A colour is inserted before the first of these that the run
// already carries, so the properties stay in schema order and Word does not
// offer to repair the file.
var rPrColorFollowers = []string{
	"<w:spacing", "<w:w ", "<w:w/", "<w:kern", "<w:position", "<w:sz",
	"<w:szCs", "<w:highlight", "<w:u ", "<w:u/", "<w:effect", "<w:bdr",
	"<w:shd", "<w:fitText", "<w:vertAlign", "<w:rtl", "<w:cs", "<w:em",
	"<w:lang", "<w:eastAsianLayout", "<w:specVanish", "<w:oMath",
}

// withRunColor returns rPr with the text colour set to hex, replacing any
// colour already there. An empty rPr becomes a minimal one, so callers can hand
// it straight to runsForText.
func withRunColor(rPr, hex string) string {
	color := `<w:color w:val="` + hex + `"/>`
	if strings.TrimSpace(rPr) == "" {
		return `<w:rPr>` + color + `</w:rPr>`
	}
	if runColorRe.MatchString(rPr) {
		return runColorRe.ReplaceAllString(rPr, color)
	}
	for _, tag := range rPrColorFollowers {
		if i := strings.Index(rPr, tag); i >= 0 {
			return rPr[:i] + color + rPr[i:]
		}
	}
	if i := strings.LastIndex(rPr, "</w:rPr>"); i >= 0 {
		return rPr[:i] + color + rPr[i:]
	}
	return rPr
}

// setParaTextColored rebuilds a paragraph so it reads exactly text, in the
// given colour, keeping every other run property the template gave it.
func setParaTextColored(para, text, hex string) string {
	gt := strings.Index(para, ">")
	if gt < 0 {
		return para
	}
	return para[:gt+1] + paraPPr(para) +
		runsForText(withRunColor(paraFirstRPr(para), hex), text) + `</w:p>`
}
