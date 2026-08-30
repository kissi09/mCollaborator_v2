package main

// What the closure deck has to look like, as against what it has to contain.
//
// Every test here is a regression for something a reader saw on screen and the
// package-level tests could not: a table printed entirely in bold, a scope
// table holding a band of empty cells, a ring with three of its four panels
// blank, evidence stretched out of shape. They are checked against the
// engagement's own reference deck - docs/VAPT - Closure Report.pptx - which is
// what the generated deck is meant to be indistinguishable from.

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// singleAreaConfig is an engagement scoped to one area with more findings than
// one panel of the executive summary shows - the shape that left the deck's
// scope table half empty and its ring lopsided.
func singleAreaConfig(t *testing.T) ReportConfig {
	t.Helper()
	config := closureConfig(t)
	config.CompanyName = "MTN Ghana"
	config.CompanyInitials = "MTN"
	config.AssessmentStart = "1st August 2026"
	config.AssessmentEnd = "10th August 2026"
	config.Areas = []ReportArea{{Code: "WPT", Scope: "https://hr.mtn.com\nhttps://portal.mtn.com"}}
	for i := range config.Findings {
		config.Findings[i].Area = "WPT"
	}
	config.Findings = append(config.Findings, ReportFinding{
		Title: "Missing HTTP security headers", Severity: "medium", Area: "WPT",
		Description: "No HSTS, CSP or X-Frame-Options.", Recommendation: "Add them.",
		AffectedSystem: "portal.mtn.com",
	})
	return config
}

// issueCellOf returns one cell of the first issues table's first data row.
func issueCellOf(t *testing.T, parts map[string]string, cell int) string {
	t.Helper()
	for name, body := range parts {
		if !strings.HasPrefix(name, "ppt/slides/slide") || !strings.Contains(body, ">Issues<") {
			continue
		}
		start := strings.Index(body, "<a:tbl>")
		end := strings.Index(body, "</a:tbl>")
		if start < 0 || end < 0 {
			continue
		}
		tbl := body[start:end]
		rows := childElems(tbl, "a:tr")
		if len(rows) < 2 {
			continue
		}
		row := tbl[rows[1].Start:rows[1].End]
		cells := childElems(row, "a:tc")
		if cell >= len(cells) {
			continue
		}
		return row[cells[cell].Start:cells[cell].End]
	}
	t.Fatal("the deck has no issues table with a data row")
	return ""
}

// allSlideText joins every slide in the package, for checks that only care that
// something reached a slide somewhere.
func allSlideText(parts map[string]string) string {
	var b strings.Builder
	for name, body := range parts {
		if strings.HasPrefix(name, "ppt/slides/slide") {
			b.WriteString(body)
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// the issues tables
// ---------------------------------------------------------------------------

// TestIssuesCellBoldsOnlyTheTitleAndTheHostLabel is the regression for tables
// that printed as a solid block of bold. The source deck bolds the finding's
// title and the words "Affected Host:", and nothing else in the cell.
//
// The deck's body font ships as two families rather than one with a bold
// weight, so the check is on the typeface as much as on b: a run left in
// "Wavehaus 128 Bold" prints bold however its b attribute reads, which is why
// setting b alone changed nothing on screen.
func TestIssuesCellBoldsOnlyTheTitleAndTheHostLabel(t *testing.T) {
	deck, _, err := buildClosureDeck(closureConfig(t))
	if err != nil {
		t.Fatalf("buildClosureDeck: %v", err)
	}
	cell := issueCellOf(t, deckParts(t, deck), 0)

	paras := childElems(cell, "a:p")
	if len(paras) < 3 {
		t.Fatalf("the issue cell has %d paragraphs; want the title, the description and the host on lines of their own", len(paras))
	}

	for _, w := range []struct {
		para, run int
		bold      bool
		what      string
	}{
		{0, 0, true, "the finding's title"},
		{1, 0, false, "the description"},
		{2, 0, true, `the "Affected Host:" label`},
		{2, 1, false, "the host itself"},
	} {
		para := cell[paras[w.para].Start:paras[w.para].End]
		runs := childElems(para, "a:r")
		if w.run >= len(runs) {
			t.Errorf("paragraph %d has no run %d, so %s is missing", w.para, w.run, w.what)
			continue
		}
		run := para[runs[w.run].Start:runs[w.run].End]
		wantB, wantFace := ` b="0"`, deckLightFace
		if w.bold {
			wantB, wantFace = ` b="1"`, deckBoldFace
		}
		if !strings.Contains(run, wantB) {
			t.Errorf("%s does not carry%s: %s", w.what, wantB, run)
		}
		if strings.Contains(run, "typeface=") && !strings.Contains(run, `typeface="`+wantFace+`"`) {
			t.Errorf("%s is not set in %q: %s", w.what, wantFace, run)
		}
	}
}

// TestSeverityCellUsesTheReportsColours ties the deck's criticality colours to
// the report's. They are the same five bands off the same legend, and a deck
// disagreeing with the document it summarises is the version the client
// notices.
func TestSeverityCellUsesTheReportsColours(t *testing.T) {
	deck, _, err := buildClosureDeck(closureConfig(t))
	if err != nil {
		t.Fatalf("buildClosureDeck: %v", err)
	}
	parts := deckParts(t, deck)

	cell := issueCellOf(t, parts, 1)
	if !strings.Contains(cell, `<a:srgbClr val="`+severityHex("critical")+`"/>`) {
		t.Errorf("the criticality is not printed in %s: %s", severityHex("critical"), cell)
	}
	if !strings.Contains(cell, ` b="1"`) {
		t.Errorf("the criticality is not bold: %s", cell)
	}

	// Every severity the engagement carries has to reach a slide in its own
	// colour, not just whichever one happens to come first.
	slides := allSlideText(parts)
	for _, sev := range []string{"critical", "low"} {
		if want := `<a:srgbClr val="` + severityHex(sev) + `"/>`; !strings.Contains(slides, want) {
			t.Errorf("no %s finding is coloured %s anywhere in the deck", sev, severityHex(sev))
		}
	}
}

// ---------------------------------------------------------------------------
// the executive summary
// ---------------------------------------------------------------------------

// TestScopeTableLeavesNoEmptyRows is the regression for a scope table that kept
// the template's four rows however few areas were tested, printing a band of
// empty cells under the single line of scope an engagement had.
func TestScopeTableLeavesNoEmptyRows(t *testing.T) {
	deck, _, err := buildClosureDeck(singleAreaConfig(t))
	if err != nil {
		t.Fatalf("buildClosureDeck: %v", err)
	}
	slide := deckParts(t, deck)["ppt/slides/slide2.xml"]
	tbl := slide[strings.Index(slide, "<a:tbl>"):strings.Index(slide, "</a:tbl>")]

	rows := childElems(tbl, "a:tr")
	if len(rows) != 2 {
		t.Errorf("the scope table has %d rows for one area; want the banner and one row", len(rows))
	}
	for ri := 1; ri < len(rows); ri++ {
		row := tbl[rows[ri].Start:rows[ri].End]
		if strings.TrimSpace(aParaText(row)) == "" {
			t.Errorf("row %d of the scope table is empty and should have been dropped", ri)
		}
		// The single area reads across the table rather than leaving the
		// right-hand column blank beside it.
		if !strings.Contains(row, `gridSpan="2"`) || !strings.Contains(row, `hMerge="1"`) {
			t.Errorf("row %d does not span the table width", ri)
		}
	}
}

// TestScopeBulletsSayThePeriodAndTheTotal fills the two bullets the source deck
// carries under the scope table. They were left as [Scope] tokens and then
// blanked, which is what left the bottom half of the slide empty.
func TestScopeBulletsSayThePeriodAndTheTotal(t *testing.T) {
	config := singleAreaConfig(t)
	deck, _, err := buildClosureDeck(config)
	if err != nil {
		t.Fatalf("buildClosureDeck: %v", err)
	}
	slide := deckParts(t, deck)["ppt/slides/slide2.xml"]

	for _, want := range []string{
		"from " + config.AssessmentStart + " to " + config.AssessmentEnd,
		fmt.Sprintf("A total of %d vulnerabilities", len(config.Findings)),
	} {
		if !strings.Contains(slide, xmlEscape(want)) {
			t.Errorf("the scope slide does not say %q", want)
		}
	}
	if strings.Contains(slide, "[Scope]") {
		t.Error("a [Scope] placeholder is still printing on the scope slide")
	}
}

// TestOneAreaSpreadsAroundTheRing is the regression for the executive summary's
// second slide with a single area in scope: one panel filled, three left empty,
// and every finding piled into one corner of a ring drawn to be surrounded.
func TestOneAreaSpreadsAroundTheRing(t *testing.T) {
	config := singleAreaConfig(t)
	panels := buildAreaPanels(config, buildNumberedFindings(config))
	if len(panels) != 1 {
		t.Fatalf("expected one area panel, got %d", len(panels))
	}

	spread := spreadPanels(panels, 4)
	if len(spread) < 2 {
		t.Fatalf("%d findings in one area filled %d panel(s); the spare slots were left empty",
			len(config.Findings), len(spread))
	}
	if spread[0].Heading != panels[0].Heading {
		t.Errorf("the first panel lost its heading: %q", spread[0].Heading)
	}
	for i, p := range spread[1:] {
		if p.Heading != "" {
			t.Errorf("continuation panel %d repeats the heading %q; it is one area's list, headed once", i+1, p.Heading)
		}
	}

	// Nothing is dropped on the way: every finding still reaches a panel.
	shown := 0
	for _, p := range spread {
		shown += len(p.Headlines)
	}
	if shown != len(config.Findings) {
		t.Errorf("%d of %d findings reached a panel", shown, len(config.Findings))
	}

	// An engagement that fills the slots on its own is left exactly as it is.
	four := []areaPanel{
		{Heading: "A", Headlines: []string{"1", "2", "3"}},
		{Heading: "B"}, {Heading: "C"}, {Heading: "D"},
	}
	got := spreadPanels(four, 4)
	if len(got) != 4 || got[0].Heading != "A" || got[1].Heading != "B" {
		t.Errorf("four areas were rearranged: %+v", got)
	}
}

// TestSummaryPanelsReadDownTheLeftColumnFirst pins the order the ring's panels
// are filled in. Sorting them by height alone interleaves the panel on the
// ring's left with the three stacked down its right, which put the second area
// above the first - and, once one area could span two panels, printed the
// continuation of a list above the heading it continued.
func TestSummaryPanelsReadDownTheLeftColumnFirst(t *testing.T) {
	deck, err := openClosureTemplate()
	if err != nil {
		t.Fatalf("openClosureTemplate: %v", err)
	}
	part := deck.part(summaryTemplatePart)
	if part == nil {
		t.Fatal("the closure template has no summary slide")
	}
	slide := string(part.Data)

	shapes := findingShapes(slide)
	if len(shapes) < 4 {
		t.Fatalf("the summary template has %d panels; expected four around the ring", len(shapes))
	}
	xOf := map[int]int{}
	for _, s := range shapes {
		if m := aOffRe.FindStringSubmatch(slide[s.at.Start:s.at.End]); len(m) == 3 {
			x, _ := parseInt(m[1])
			xOf[s.reading] = x
		}
	}
	if xOf[0] >= xOf[1] {
		t.Errorf("the first panel sits at x=%d, right of the second at x=%d; the left of the ring reads first",
			xOf[0], xOf[1])
	}
}

// TestCustomerLogoSitsInTheRing puts the client's mark in the hole the ring is
// drawn around, which the template leaves empty.
func TestCustomerLogoSitsInTheRing(t *testing.T) {
	config := singleAreaConfig(t)
	config.CompanyLogo = tinyPNGDataURI()
	deck, notes, err := buildClosureDeck(config)
	if err != nil {
		t.Fatalf("buildClosureDeck: %v", err)
	}
	if notes.LogoError != "" {
		t.Fatalf("the logo was rejected: %s", notes.LogoError)
	}

	var found bool
	for name, body := range deckParts(t, deck) {
		if !strings.HasPrefix(name, "ppt/slides/slide") || !strings.Contains(body, "Customer Logo") {
			continue
		}
		var pic string
		for _, p := range pPicRe.FindAllString(body, -1) {
			if strings.Contains(p, "Customer Logo") {
				pic = p
				break
			}
		}
		if pic == "" {
			t.Errorf("%s names a customer logo but has no picture for it", name)
			continue
		}
		off := aXfrmOff.FindStringSubmatch(pic)
		ext := aXfrmExt.FindStringSubmatch(pic)
		if len(off) != 3 || len(ext) != 3 {
			t.Errorf("%s: the ring logo has no position", name)
			continue
		}
		x, _ := parseInt(off[1])
		y, _ := parseInt(off[2])
		w, _ := parseInt(ext[1])
		h, _ := parseInt(ext[2])

		// Centred on the hole, and inside it.
		if cx := x + w/2; abs(cx-ringHoleCX) > ringHoleDiameter/20 {
			t.Errorf("%s: the logo is centred at x=%d, not on the ring's %d", name, cx, ringHoleCX)
		}
		if cy := y + h/2; abs(cy-ringHoleCY) > ringHoleDiameter/20 {
			t.Errorf("%s: the logo is centred at y=%d, not on the ring's %d", name, cy, ringHoleCY)
		}
		if w > ringHoleDiameter || h > ringHoleDiameter {
			t.Errorf("%s: the logo is %dx%d in a hole %d across", name, w, h, ringHoleDiameter)
		}
		// The frame keeps the 4x2 sample's own proportions, to the rounding.
		if h == 0 || abs(w-2*h) > 2 {
			t.Errorf("%s: a 4x2 logo sits in a %dx%d frame; it has been stretched", name, w, h)
		}
		found = true
	}
	if !found {
		t.Error("no summary slide carries the customer logo in the ring")
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ---------------------------------------------------------------------------
// the vulnerability scenarios
// ---------------------------------------------------------------------------

var scenarioTitleRe = regexp.MustCompile(`<a:t>(Vulnerability Scenario[^<]*)</a:t>`)

// TestExtraScreenshotsAreMarkedContd stops a finding with several proofs
// producing slide after slide under an identical heading, which reads as the
// same slide pasted in twice.
func TestExtraScreenshotsAreMarkedContd(t *testing.T) {
	deck, _, err := buildClosureDeck(closureConfig(t))
	if err != nil {
		t.Fatalf("buildClosureDeck: %v", err)
	}

	var titles []string
	for name, body := range deckParts(t, deck) {
		if !strings.HasPrefix(name, "ppt/slides/slide") {
			continue
		}
		for _, m := range scenarioTitleRe.FindAllStringSubmatch(body, -1) {
			titles = append(titles, m[1])
		}
	}

	seen := map[string]bool{}
	contd := 0
	for _, title := range titles {
		if seen[title] {
			t.Errorf("two scenario slides are both titled %q", title)
		}
		seen[title] = true
		if strings.Contains(title, "Cont") {
			contd++
		}
	}
	if contd == 0 {
		t.Errorf("the finding with two screenshots produced no continuation slide; titles were %q", titles)
	}
}

// TestScreenshotKeepsItsShape is the regression for evidence squashed into the
// template's square frame. The deck shows the same picture the report does, and
// a wide terminal capture stretched to a square is not it.
func TestScreenshotKeepsItsShape(t *testing.T) {
	// The sample screenshot is 4x2, so its frame has to come out twice as wide
	// as it is tall whatever shape the template drew.
	deck, _, err := buildClosureDeck(closureConfig(t))
	if err != nil {
		t.Fatalf("buildClosureDeck: %v", err)
	}

	checked := 0
	for name, body := range deckParts(t, deck) {
		if !strings.HasPrefix(name, "ppt/slides/slide") || !strings.Contains(body, "Vulnerability Scenario") {
			continue
		}
		pic := pPicRe.FindString(body)
		if pic == "" {
			continue
		}
		m := aXfrmExt.FindStringSubmatch(pic)
		if len(m) != 3 {
			t.Errorf("%s: the picture frame has no size", name)
			continue
		}
		w, _ := parseInt(m[1])
		h, _ := parseInt(m[2])
		if h == 0 || abs(w-2*h) > 2 {
			t.Errorf("%s: a 4x2 screenshot sits in a %dx%d frame; it has been stretched", name, w, h)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no scenario slide carried a picture")
	}
}

// ---------------------------------------------------------------------------
// the charts
// ---------------------------------------------------------------------------

// TestEmptySeveritySegmentHasNoLabel keeps a severity nobody found from
// printing a bare "0" on the doughnut, where it lands between the two segments
// either side of the gap and reads as belonging to one of them.
func TestEmptySeveritySegmentHasNoLabel(t *testing.T) {
	deck, _, err := buildClosureDeck(singleAreaConfig(t))
	if err != nil {
		t.Fatalf("buildClosureDeck: %v", err)
	}
	chart := deckParts(t, deck)[severityChartPart]
	if chart == "" {
		t.Fatal("the deck has no severity chart")
	}
	// The engagement has no Informational findings, and Info is the last of the
	// five segments.
	if !strings.Contains(chart, `<c:dLbl><c:idx val="4"/><c:delete val="1"/></c:dLbl>`) {
		t.Error("the empty severity segment still carries its zero label")
	}
	if strings.Contains(chart, `<c:idx val="0"/><c:delete val="1"/>`) {
		t.Error("a segment that has findings had its label deleted")
	}
}

// TestOneBarIsNotDrawnAcrossThePlot holds a bar at the width the template gives
// it. The gap is a percentage of a bar, so the template's setting for four bars
// drew a single one across the whole chart - a block of colour four inches wide
// that reads as a rendering fault, not as a count.
func TestOneBarIsNotDrawnAcrossThePlot(t *testing.T) {
	deck, _, err := buildClosureDeck(singleAreaConfig(t))
	if err != nil {
		t.Fatalf("buildClosureDeck: %v", err)
	}
	chart := deckParts(t, deck)[areaChartPart]
	if chart == "" {
		t.Fatal("the deck has no area chart")
	}
	m := gapWidthRe.FindString(chart)
	if m == "" {
		t.Fatal("the area chart has no gap width")
	}
	gap, _ := parseInt(strings.TrimSuffix(strings.TrimPrefix(m, `<c:gapWidth val="`), `"/>`))
	if gap <= templateBarGap {
		t.Errorf("one bar is drawn with the template's %d%% gap; it fills the plot", gap)
	}

	// Four areas leave the template's own setting alone.
	if got := fitBarGap(`<c:gapWidth val="37"/>`, 4); got != `<c:gapWidth val="37"/>` {
		t.Errorf("a full chart had its gap changed to %q", got)
	}
}

// TestInformationalFitsTheSeverityColumn is the regression for a criticality
// that wrapped mid-word. The Severity column is sized for "Critical", and
// "Informational" broke across two lines as "Informati / onal".
func TestInformationalFitsTheSeverityColumn(t *testing.T) {
	if got := deckSeverityLabel("informational"); got != "Info" {
		t.Errorf("the deck labels an informational finding %q; the column fits %q", got, "Info")
	}
	for _, sev := range []string{"critical", "high", "medium", "low"} {
		if got, want := deckSeverityLabel(sev), severityDisplay(sev); got != want {
			t.Errorf("%s was shortened to %q; only Informational needs it", sev, got)
		}
	}
}
