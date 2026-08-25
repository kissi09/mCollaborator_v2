package main

// The closure deck's executive summary - slides 2 and 3.
//
// Both slides were built for the engagement the source deck came from, and the
// template kept its furniture: slide 2's scope table is headed "Internal VAPT",
// "External VAPT", "Configuration Review" and "Web Application Testing", and
// slide 3 repeats four of those over the headline findings. Filling the
// bracketed placeholders between them left every heading naming an area this
// engagement may never have tested, with the same scope text repeated under all
// of them - which is what made the slide read as jumble when no scope was
// entered, and what made the findings sit under the wrong areas when it was.
//
// Both are rebuilt from the engagement instead: one panel per assessment area
// actually selected, headed with that area's own name and carrying only its own
// scope and its own findings. Panels the engagement does not fill are emptied
// rather than left showing the template's text, and when there are more areas
// than the template has panels, slide 3 repeats the way the issues table does.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	// summaryTemplatePart is slide 3, repeated once per group of areas.
	summaryTemplatePart = "ppt/slides/slide3.xml"
	scopeSlidePart      = "ppt/slides/slide2.xml"

	// headlinesPerPanel is how many findings one of slide 3's panels holds: the
	// template gives each of them two [Finding] lines.
	headlinesPerPanel = 2

	// summaryPanelsPerSlide is how many areas one headline slide shows.
	summaryPanelsPerSlide = 4
)

// areaPanel is one assessment area as the executive summary presents it.
type areaPanel struct {
	Heading   string   // the area's own name, e.g. "Web Application Penetration Testing"
	ScopeText []string // what was tested, one line per target
	Headlines []string // the area's findings, most severe first
}

// buildAreaPanels turns the engagement into one panel per selected area, in
// template order. An area with no scope entered still gets a panel: the client
// chose it, and a heading with nothing under it says "tested, nothing to list"
// where a missing heading says nothing at all.
func buildAreaPanels(config ReportConfig, findings []numberedFinding) []areaPanel {
	scopeOf := map[string]string{}
	for _, a := range config.Areas {
		scopeOf[strings.ToUpper(strings.TrimSpace(a.Code))] = a.Scope
	}

	byArea := map[string][]numberedFinding{}
	for _, f := range findings {
		byArea[f.Area.Code] = append(byArea[f.Area.Code], f)
	}

	var panels []areaPanel
	for _, code := range selectedAreaCodes(config) {
		area, ok := areaByCode(code)
		if !ok {
			continue
		}
		group := append([]numberedFinding(nil), byArea[code]...)
		sort.SliceStable(group, func(i, j int) bool {
			return severityRank(group[i].Severity) < severityRank(group[j].Severity)
		})
		var titles []string
		for _, f := range group {
			titles = append(titles, strings.TrimSpace(f.Title))
		}
		panels = append(panels, areaPanel{
			Heading:   area.Label,
			ScopeText: splitScopeLines(scopeOf[code]),
			Headlines: titles,
		})
	}
	return panels
}

// splitScopeLines breaks the scope a tester typed for one area into the lines
// the slide lists. Testers enter targets one per line or comma-separated, and
// both have to come out as separate lines rather than one run-on.
func splitScopeLines(scope string) []string {
	var out []string
	for _, line := range strings.FieldsFunc(scope, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ','
	}) {
		if s := strings.TrimSpace(line); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// chunkPanels splits the areas into slide-sized groups.
func chunkPanels(panels []areaPanel, size int) [][]areaPanel {
	var out [][]areaPanel
	for i := 0; i < len(panels); i += size {
		end := i + size
		if end > len(panels) {
			end = len(panels)
		}
		out = append(out, panels[i:end])
	}
	return out
}

// ---------------------------------------------------------------------------
// slide 2 - scope
// ---------------------------------------------------------------------------

// fillScopeSlide rewrites the scope table, one area per cell, and the bullets
// beneath it.
func (d *closureDeck) fillScopeSlide(config ReportConfig, panels []areaPanel) {
	p := d.part(scopeSlidePart)
	if p == nil {
		return
	}
	slide := setScopeTable(string(p.Data), panels)
	p.Data = []byte(setScopeBullets(slide, config))
}

var (
	aTcOpenRe  = regexp.MustCompile(`^<a:tc[^>]*>`)
	rowIDExtRe = regexp.MustCompile(`(?s)<a:extLst><a:ext uri="\{0D108BD9-81ED-4DB2-BD59-A6C34878D82A\}">.*?</a:extLst>`)
)

// setScopeTable fills the table's body cells with the areas, growing the table
// by cloning its last row when the engagement has more areas than the template
// has cells for.
func setScopeTable(slide string, panels []areaPanel) string {
	tblStart := strings.Index(slide, "<a:tbl>")
	tblEnd := strings.Index(slide, "</a:tbl>")
	if tblStart < 0 || tblEnd < 0 {
		return slide
	}
	tblEnd += len("</a:tbl>")
	tbl := slide[tblStart:tblEnd]

	for len(panelCells(tbl)) < len(panels) {
		grown, ok := cloneLastRow(tbl)
		if !ok {
			break
		}
		tbl = grown
	}

	// Back to front, so the offsets taken from the parse stay valid as the
	// fragment changes length.
	cells := panelCells(tbl)
	for i := len(cells) - 1; i >= 0; i-- {
		panel := areaPanel{}
		if i < len(panels) {
			panel = panels[i]
		}
		filled := setCellParagraphs(tbl[cells[i].Start:cells[i].End], panel.Heading, panel.ScopeText)
		tbl = tbl[:cells[i].Start] + filled + tbl[cells[i].End:]
	}
	return slide[:tblStart] + tbl + slide[tblEnd:]
}

// panelCells lists the table's fillable cells: everything below the "Scope"
// banner row that is not the hidden half of a merge.
func panelCells(tbl string) []span {
	var out []span
	for ri, r := range childElems(tbl, "a:tr") {
		if ri == 0 {
			continue
		}
		row := tbl[r.Start:r.End]
		for _, c := range childElems(row, "a:tc") {
			if open := aTcOpenRe.FindString(row[c.Start:c.End]); strings.Contains(open, "Merge=") {
				continue
			}
			out = append(out, span{r.Start + c.Start, r.Start + c.End})
		}
	}
	return out
}

// cloneLastRow appends a copy of the table's final row. The copy drops the row
// identifier PowerPoint stamps on a row, which has to stay unique.
func cloneLastRow(tbl string) (string, bool) {
	rows := childElems(tbl, "a:tr")
	if len(rows) < 2 {
		return tbl, false
	}
	last := rows[len(rows)-1]
	clone := rowIDExtRe.ReplaceAllString(tbl[last.Start:last.End], "")
	return tbl[:last.End] + clone + tbl[last.End:], true
}

// setCellParagraphs makes a cell read as a heading followed by one paragraph per
// line, reusing the template's own paragraphs so the fonts, sizes and colours
// survive: the first paragraph styles the heading, the second styles every line
// under it. A cell given an empty heading and no lines comes back blank, which
// is how a panel the engagement does not fill is cleared.
func setCellParagraphs(cell, heading string, lines []string) string {
	paras := childElems(cell, "a:p")
	if len(paras) == 0 {
		return cell
	}
	headTmpl := cell[paras[0].Start:paras[0].End]
	bodyTmpl := headTmpl
	if len(paras) > 1 {
		bodyTmpl = cell[paras[1].Start:paras[1].End]
	}

	var b strings.Builder
	b.WriteString(setAParaText(headTmpl, heading))
	for _, line := range lines {
		b.WriteString(setAParaText(bodyTmpl, line))
	}
	return cell[:paras[0].Start] + b.String() + cell[paras[len(paras)-1].End:]
}

// setScopeBullets fills the list under the table.
//
// The template's two bullets there are [Scope] placeholders, which the source
// deck used for the qualifications belonging to no single area. They are given
// the engagement's exclusions, labelled as exclusions: an unlabelled bullet on a
// slide headed "Scope" reads as something that *was* tested, which is the
// opposite of what an out-of-scope line means.
func setScopeBullets(slide string, config ReportConfig) string {
	var lines []string
	for _, s := range config.OutOfScope {
		if s = strings.TrimSpace(s); s != "" {
			lines = append(lines, "Out of scope - "+s)
		}
	}
	return replaceShapeParagraphs(slide, "[Scope]", lines)
}

// ---------------------------------------------------------------------------
// slide 3 - headline findings
// ---------------------------------------------------------------------------

// renderSummarySlide fills one headline slide with up to four areas.
func renderSummarySlide(slide string, panels []areaPanel) string {
	shapes := findingShapes(slide)

	// Rewrite from the last shape in the file backwards, so each edit only
	// moves text the loop has already passed. Which panel a shape gets comes
	// from its reading position, which is not its position in the file.
	for i := len(shapes) - 1; i >= 0; i-- {
		panel := areaPanel{}
		if shapes[i].reading < len(panels) {
			panel = panels[shapes[i].reading]
		}
		headlines := panel.Headlines
		if len(headlines) > headlinesPerPanel {
			headlines = headlines[:headlinesPerPanel]
		}
		sp := shapes[i].at
		slide = slide[:sp.Start] +
			setShapeParagraphs(slide[sp.Start:sp.End], panel.Heading, headlines) +
			slide[sp.End:]
	}
	return slide
}

var aOffRe = regexp.MustCompile(`<a:off x="(-?\d+)" y="(-?\d+)"/>`)

// panelShape is one of slide 3's text boxes: where it sits in the file, and
// where it comes in the order a reader meets them.
type panelShape struct {
	at      span
	reading int
}

// findingShapes locates slide 3's panels - the text boxes carrying a [Finding]
// placeholder - in file order, each tagged with its reading position: down the
// slide, then across. The template scatters them around the artwork, so the
// order they appear in the file is not the order they are read in, and filling
// them in file order would put the first area halfway down the slide.
func findingShapes(slide string) []panelShape {
	type placed struct {
		at   span
		x, y int
	}
	var found []placed
	for _, s := range childElems(slide, "p:sp") {
		frag := slide[s.Start:s.End]
		if !strings.Contains(frag, "[Finding]") {
			continue
		}
		p := placed{at: s}
		if m := aOffRe.FindStringSubmatch(frag); len(m) == 3 {
			p.x, _ = parseInt(m[1])
			p.y, _ = parseInt(m[2])
		}
		found = append(found, p)
	}

	byReading := make([]int, len(found))
	for i := range byReading {
		byReading[i] = i
	}
	sort.SliceStable(byReading, func(a, b int) bool {
		i, j := found[byReading[a]], found[byReading[b]]
		if i.y != j.y {
			return i.y < j.y
		}
		return i.x < j.x
	})

	out := make([]panelShape, len(found))
	for i := range found {
		out[i] = panelShape{at: found[i].at}
	}
	for rank, idx := range byReading {
		out[idx].reading = rank
	}
	return out
}

// setShapeParagraphs rewrites a text box to a heading plus one line each, then
// makes the result fit the box the template drew for it.
func setShapeParagraphs(shape, heading string, lines []string) string {
	body := strings.Index(shape, "<p:txBody>")
	if body < 0 {
		return shape
	}
	filled := setCellParagraphs(shape[body:], heading, lines)
	filled = leftAlign(filled)
	return shape[:body] + fitToBox(filled, shapeBox(shape), heading, lines)
}

var justifiedRe = regexp.MustCompile(` algn="just"`)

// leftAlign stops the panels justifying their text.
//
// The template justifies each finding line, which suited the source deck's
// two-word findings. A real finding title is a phrase, and justifying a
// two-line phrase in a three-inch box stretches the first line into
// "SQL     Injection     in     the" - which reads as a rendering fault rather
// than as emphasis.
func leftAlign(txBody string) string {
	return justifiedRe.ReplaceAllString(txBody, ` algn="l"`)
}

var (
	shapeExtRe = regexp.MustCompile(`<a:ext cx="(\d+)" cy="(\d+)"/>`)
	runSizeRe  = regexp.MustCompile(`(<a:rPr[^>]*?) sz="(\d+)"`)
)

// box is a shape's drawn size in EMU.
type box struct{ W, H int }

func shapeBox(shape string) box {
	m := shapeExtRe.FindStringSubmatch(shape)
	if len(m) != 3 {
		return box{}
	}
	w, _ := parseInt(m[1])
	h, _ := parseInt(m[2])
	return box{w, h}
}

// emuPerPoint converts EMU to points: 914400 EMU per inch, 72 points per inch.
const emuPerPoint = 12700

// fitToBox shrinks a panel's type until its text fits the space the template
// gave it.
//
// The panels are fixed boxes - the bottom-right one is barely an inch tall -
// and the template sized them for findings named in two or three words. A real
// title runs to a line and a half, so the last panel's second finding ran off
// the bottom of the slide. PowerPoint does not reflow an overflowing text box on
// its own, so the fit is worked out here and written into the runs.
//
// The estimate is deliberately rough: character widths vary, and the point is
// not to typeset the slide but to keep it on the slide. Nothing grows - a panel
// that already fits is left at the size the template set.
func fitToBox(txBody string, b box, heading string, lines []string) string {
	if b.H <= 0 || b.W <= 0 || len(lines) == 0 {
		return txBody
	}
	// The panels carry no size of their own - they inherit the presentation's
	// default text style - so there is nothing to scale and a size has to be
	// written in. defaultPanelSize is what that default renders at.
	base := firstRunSize(txBody)
	if base <= 0 {
		base = defaultPanelSize
	}

	size := base
	for range 6 {
		if estimateHeight(heading, lines, float64(size), b.W) <= float64(b.H)/emuPerPoint {
			break
		}
		size = size * 9 / 10
	}
	if size >= base {
		return txBody
	}
	// Below about two thirds the panel is smaller than the legend beside it and
	// stops reading as a heading with points under it.
	if min := base * 65 / 100; size < min {
		size = min
	}
	return setRunSize(txBody, size)
}

// defaultPanelSize is the size, in hundredths of a point, that slide 3's panels
// inherit from the presentation's default text style.
const defaultPanelSize = 1800

var rPrOpenRe = regexp.MustCompile(`<a:rPr\b`)

// setRunSize writes a type size onto every run of a text body, replacing the
// size where one is already set and adding it where the run inherited it.
func setRunSize(txBody string, size int) string {
	txBody = runSizeRe.ReplaceAllString(txBody, `${1}`)
	return rPrOpenRe.ReplaceAllString(txBody, fmt.Sprintf(`<a:rPr sz="%d"`, size))
}

// estimateHeight is the height in points that a heading and its lines need at
// the given type size in a box of the given width.
func estimateHeight(heading string, lines []string, sizeHundredths float64, widthEMU int) float64 {
	pt := sizeHundredths / 100
	if pt <= 0 {
		return 0
	}
	// A character averages about half its point size in width.
	perLine := float64(widthEMU) / emuPerPoint / (pt * 0.5)
	if perLine < 1 {
		perLine = 1
	}
	wrapped := func(s string) float64 {
		n := float64(len([]rune(s)))
		rows := 1.0
		for n > perLine {
			n -= perLine
			rows++
		}
		return rows
	}
	rows := wrapped(heading)
	for _, l := range lines {
		rows += wrapped(l)
	}
	// 1.2 for leading, plus a little space between paragraphs.
	return rows*pt*1.2 + float64(len(lines))*pt*0.3
}

// firstRunSize is the type size the template set for the panel, in hundredths
// of a point.
func firstRunSize(txBody string) int {
	m := runSizeRe.FindStringSubmatch(txBody)
	if len(m) != 3 {
		return 0
	}
	n, err := parseInt(m[2])
	if err != nil {
		return 0
	}
	return n
}

// replaceShapeParagraphs rewrites the paragraphs of whichever shape carries the
// given placeholder, to one paragraph per line. With no lines the shape keeps a
// single empty paragraph, so the placeholder never prints.
func replaceShapeParagraphs(slide, placeholder string, lines []string) string {
	for _, s := range childElems(slide, "p:sp") {
		frag := slide[s.Start:s.End]
		if !strings.Contains(frag, placeholder) {
			continue
		}
		body := strings.Index(frag, "<p:txBody>")
		if body < 0 {
			continue
		}
		paras := childElems(frag[body:], "a:p")
		if len(paras) == 0 {
			continue
		}
		tmpl := frag[body+paras[0].Start : body+paras[0].End]

		var b strings.Builder
		if len(lines) == 0 {
			b.WriteString(setAParaText(tmpl, ""))
		}
		for _, line := range lines {
			b.WriteString(setAParaText(tmpl, line))
		}
		rebuilt := frag[:body+paras[0].Start] + b.String() + frag[body+paras[len(paras)-1].End:]
		return slide[:s.Start] + rebuilt + slide[s.End:]
	}
	return slide
}
