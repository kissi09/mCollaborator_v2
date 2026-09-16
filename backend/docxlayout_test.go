package main

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The corrections applied to a section's block as it renders. Each of these was
// a fault a reader found in a draft report, and each is invisible in the source
// - the template looks fine until it is read on a page.

// renderedFindingTables returns the finding tables of one area's section in the
// rendered document, in order.
func renderedFindingTables(t *testing.T, doc, title string) []string {
	t.Helper()
	children := bodyChildren(doc)
	start := -1
	for i, c := range children {
		if strings.TrimSpace(elemText(doc[c.Start:c.End])) == title {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("finding %q not found", title)
	}
	var out []string
	for i := start + 1; i < len(children); i++ {
		c := children[i]
		if c.Tag == "w:tbl" {
			tbl := doc[c.Start:c.End]
			if !tableHasLabel(tbl, "description") && !tableHasLabel(tbl, "affected") &&
				!tableHasLabel(tbl, "recommendation") {
				break
			}
			out = append(out, tbl)
			continue
		}
		if strings.TrimSpace(elemText(doc[c.Start:c.End])) != "" {
			break
		}
	}
	return out
}

// labelRowOf returns the row carrying a field's label, and the row under it.
func labelRowOf(t *testing.T, tables []string, key string) (string, string) {
	t.Helper()
	for _, tbl := range tables {
		if l, v, ok := detailRowPair(tbl, key); ok {
			return l, v
		}
	}
	return "", ""
}

// The internal block splits its affected-hosts box into five columns nothing
// ever fills.
func TestAffectedRowIsOneBoxNotColumns(t *testing.T) {
	doc := readDocxParts(t, rowsConfig())["word/document.xml"]
	tables := renderedFindingTables(t, doc, "SMB signing not required")
	_, value := labelRowOf(t, tables, "affected")
	if value == "" {
		t.Fatal("the IPT finding has no affected row")
	}
	if n := len(rowCells(value)); n != 1 {
		t.Errorf("the affected hosts box is %d cells wide; it should be one box", n)
	}
	if !strings.Contains(value, "10.0.4.11") {
		t.Errorf("the affected hosts box lost its value: %q", strings.TrimSpace(elemText(value)))
	}
}

// A proof of concept added to a section that ships none has to look like one
// from a section that does, not like whatever row was cloned to make it.
func TestAddedPoCMatchesTheSectionThatShipsOne(t *testing.T) {
	doc := templateDocumentXML(t)
	templates, _, err := extractAreaTemplates(doc)
	if err != nil {
		t.Fatal(err)
	}
	lib := buildDetailRowLibrary(templates)
	if lib.pocLabel == "" || lib.pocValue == "" {
		t.Fatal("no proof-of-concept row was found to borrow")
	}

	rendered := readDocxParts(t, rowsConfig())["word/document.xml"]
	ipt := renderedFindingTables(t, rendered, "SMB signing not required")
	iptLabel, iptValue := labelRowOf(t, ipt, "poc")
	if iptLabel == "" {
		t.Fatal("the IPT finding has no proof-of-concept row")
	}
	wpt := renderedFindingTables(t, rendered, "Reflected XSS")
	wptLabel, _ := labelRowOf(t, wpt, "poc")
	if wptLabel == "" {
		t.Fatal("the WPT finding has no proof-of-concept row")
	}

	// Same alignment as the section the row was taken from.
	if got, want := paraAlignments(iptValue), paraAlignments(lib.pocValue); got != want {
		t.Errorf("the added PoC box is aligned %q, the one it was taken from is %q", got, want)
	}
	if n := len(rowCells(iptValue)); n != 1 {
		t.Errorf("the added PoC box is %d cells wide", n)
	}
}

func paraAlignments(row string) string {
	var out []string
	for _, c := range rowCells(row) {
		cell := row[c.Start:c.End]
		for _, p := range childElems(cell, "w:p") {
			m := regexp.MustCompile(`<w:jc w:val="([^"]*)"`).FindStringSubmatch(cell[p.Start:p.End])
			if m == nil {
				out = append(out, "-")
				continue
			}
			out = append(out, m[1])
		}
	}
	return strings.Join(out, ",")
}

// A value box is as tall as what is written in it. Two sections set a fixed
// height that left an inch and a half of white under two lines of text.
func TestValueBoxesFitTheirText(t *testing.T) {
	doc := readDocxParts(t, rowsConfig())["word/document.xml"]
	heights := regexp.MustCompile(`<w:trHeight[^>]*w:val="(\d+)"`)

	for _, title := range []string{
		"Flat network with no segmentation",
		"Kerberoastable service account",
		"SMB signing not required",
	} {
		for _, tbl := range renderedFindingTables(t, doc, title) {
			rows := tableRows(tbl)
			for ri := 0; ri+1 < len(rows); ri++ {
				label := strings.ToLower(strings.TrimSpace(elemText(tbl[rows[ri].Start:rows[ri].End])))
				if _, ok := detailLabels[label]; !ok {
					continue
				}
				value := tbl[rows[ri+1].Start:rows[ri+1].End]
				if m := heights.FindStringSubmatch(value); m != nil {
					n, _ := strconv.Atoi(m[1])
					t.Errorf("%s: the %s box is pinned to %d twips instead of fitting its text", title, label, n)
				}
			}
		}
	}
}

// Every label sits on the orange fill, so every label is printed white. Two
// sections never said so and came out black on orange.
func TestEveryFieldLabelIsWhite(t *testing.T) {
	doc := readDocxParts(t, rowsConfig())["word/document.xml"]
	for _, title := range []string{
		"Flat network with no segmentation",
		"Default SNMP community string",
		"SMB signing not required",
	} {
		for _, tbl := range renderedFindingTables(t, doc, title) {
			for _, r := range tableRows(tbl) {
				row := tbl[r.Start:r.End]
				for _, c := range rowCells(row) {
					cell := row[c.Start:c.End]
					label := strings.ToLower(strings.TrimSpace(elemText(cell)))
					if _, ok := detailLabels[label]; !ok {
						continue
					}
					for _, p := range childElems(cell, "w:p") {
						para := cell[p.Start:p.End]
						for _, rs := range childElems(para, "w:r") {
							run := para[rs.Start:rs.End]
							if strings.TrimSpace(elemText(run)) == "" {
								continue
							}
							if !strings.Contains(run, `<w:color w:val="FFFFFF"`) {
								t.Errorf("%s: the %q label is not printed white", title, label)
							}
						}
					}
				}
			}
		}
	}
}

// One section headed its recommendation in a different typeface at a different
// size from every other label in the report.
func TestLabelsCarryNoTypefaceOfTheirOwn(t *testing.T) {
	doc := readDocxParts(t, rowsConfig())["word/document.xml"]
	face := regexp.MustCompile(`<w:rFonts[^>]*w:ascii="([^"]*)"`)
	size := regexp.MustCompile(`<w:sz w:val="([^"]*)"`)

	for _, title := range []string{"Flat network with no segmentation", "Kerberoastable service account"} {
		for _, tbl := range renderedFindingTables(t, doc, title) {
			for _, r := range tableRows(tbl) {
				row := tbl[r.Start:r.End]
				for _, c := range rowCells(row) {
					cell := row[c.Start:c.End]
					label := strings.ToLower(strings.TrimSpace(elemText(cell)))
					if _, ok := detailLabels[label]; !ok {
						continue
					}
					if m := face.FindStringSubmatch(cell); m != nil {
						t.Errorf("%s: the %q label sets its own typeface %q", title, label, m[1])
					}
					if m := size.FindStringSubmatch(cell); m != nil {
						t.Errorf("%s: the %q label sets its own size %q", title, label, m[1])
					}
				}
			}
		}
	}
}

// Most blocks carry an empty band between the description and the fields under
// it - a row with nothing in it and nothing that fills it, which reads as a gap
// in the middle of the finding.
func TestNoEmptyBandsInsideAFindingTable(t *testing.T) {
	doc := readDocxParts(t, rowsConfig())["word/document.xml"]

	for _, title := range []string{
		"SMB signing not required",
		"Reflected XSS",
		"Kerberoastable service account",
		"Weak TLS ciphers",
		"Default SNMP community string",
		"Flat network with no segmentation",
	} {
		for _, tbl := range renderedFindingTables(t, doc, title) {
			rows := tableRows(tbl)
			isLabelRow := func(ri int) bool {
				row := tbl[rows[ri].Start:rows[ri].End]
				for _, c := range rowCells(row) {
					if _, ok := detailLabels[strings.ToLower(strings.TrimSpace(elemText(row[c.Start:c.End])))]; ok {
						return true
					}
				}
				return false
			}
			for ri := range rows {
				if isLabelRow(ri) || (ri > 0 && isLabelRow(ri-1)) {
					continue // a label, or the box belonging to one
				}
				if strings.TrimSpace(elemText(tbl[rows[ri].Start:rows[ri].End])) == "" {
					t.Errorf("%s: row %d is an empty band belonging to no field", title, ri)
				}
			}
		}
	}
}

// A finding is one table. Two tables cannot be made to touch - Word always
// leaves a gap between them - so a section that split its finding in half showed
// a band of white between the description and the fields under it however the
// paragraphs between were dealt with.
func TestAFindingIsOneTable(t *testing.T) {
	doc := readDocxParts(t, rowsConfig())["word/document.xml"]
	for _, title := range []string{
		"Flat network with no segmentation",
		"SMB signing not required",
		"Kerberoastable service account",
	} {
		if n := len(renderedFindingTables(t, doc, title)); n != 1 {
			t.Errorf("%s is printed as %d tables; a finding is one table", title, n)
		}
	}
}

// Every finding's title is styled the same way. Two of the blocks style theirs
// by hand rather than through the Heading style, and rewriting the title's runs
// then loses the formatting: the heading came out black beside the orange ones
// in every other section.
func TestEveryFindingTitleUsesTheSameHeadingStyle(t *testing.T) {
	doc := readDocxParts(t, rowsConfig())["word/document.xml"]
	children := bodyChildren(doc)

	styles := map[string][]string{}
	for _, title := range []string{
		"SMB signing not required",
		"Weak TLS ciphers",
		"Reflected XSS",
		"Default SNMP community string",
		"Kerberoastable service account",
		"Flat network with no segmentation",
	} {
		found := false
		for _, c := range children {
			if c.Tag != "w:p" || strings.TrimSpace(elemText(doc[c.Start:c.End])) != title {
				continue
			}
			styles[c.Style] = append(styles[c.Style], title)
			found = true
			break
		}
		if !found {
			t.Errorf("%q was not found", title)
		}
	}

	if len(styles) != 1 {
		for style, titles := range styles {
			t.Errorf("style %q: %v", style, titles)
		}
		t.Fatal("finding titles are not all in the same style")
	}
	for style := range styles {
		if !strings.Contains(style, "Heading") {
			t.Errorf("finding titles use %q, which is not a Heading style - they would not reach the navigation pane", style)
		}
	}
}

// Active Directory findings are reported by endpoint, and never had an attack
// vector to print.
func TestActiveDirectoryDropsTheAttackVector(t *testing.T) {
	doc := readDocxParts(t, rowsConfig())["word/document.xml"]
	tables := renderedFindingTables(t, doc, "Kerberoastable service account")

	if l, _ := labelRowOf(t, tables, "attackvector"); l != "" {
		t.Error("the Active Directory finding still prints an attack vector row")
	}
	label, value := labelRowOf(t, tables, "affected")
	if label == "" {
		t.Fatal("the Active Directory finding has no affected row")
	}
	if got := strings.TrimSpace(elemText(label)); got != "Affected Endpoint" {
		t.Errorf("the affected row is labelled %q, want %q", got, "Affected Endpoint")
	}
	if !strings.Contains(value, "corp.acme.test") {
		t.Errorf("the affected endpoint box lost its value: %q", strings.TrimSpace(elemText(value)))
	}
}

// An empty paragraph between the two tables of one finding opens a gap in the
// middle of it, and an empty heading puts a blank line in the navigation pane.
func TestNoBlankParagraphsInsideAFinding(t *testing.T) {
	doc := readDocxParts(t, rowsConfig())["word/document.xml"]
	children := bodyChildren(doc)

	start := -1
	for i, c := range children {
		if strings.TrimSpace(elemText(doc[c.Start:c.End])) == "Flat network with no segmentation" {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatal("the NAR finding was not found")
	}

	// From the heading to the last of this finding's tables, nothing empty.
	last := start
	for i := start + 1; i < len(children); i++ {
		if children[i].Tag == "w:tbl" {
			last = i
			continue
		}
		if strings.TrimSpace(elemText(doc[children[i].Start:children[i].End])) != "" {
			break
		}
	}
	for i := start + 1; i < last; i++ {
		c := children[i]
		if c.Tag != "w:p" {
			continue
		}
		if strings.TrimSpace(elemText(doc[c.Start:c.End])) == "" {
			t.Errorf("an empty paragraph sits inside the NAR finding, between its own tables")
		}
	}
}

// An empty heading anywhere in chapter 3 shows up as a blank line in the
// reader's navigation pane. So does an empty paragraph with no heading style
// but an outline level of its own, which is how the network architecture block
// wrote the spacer under every finding.
func TestNoEmptyHeadingsInTheFindings(t *testing.T) {
	doc := readDocxParts(t, narSpacingConfig())["word/document.xml"]
	children := bodyChildren(doc)

	inChapter := false
	for _, c := range children {
		el := doc[c.Start:c.End]
		text := strings.TrimSpace(elemText(el))
		if c.Tag != "w:p" {
			continue
		}
		if isHeading(c.Style) {
			if text == "Configuration Files Review" {
				inChapter = true
			}
			if strings.HasPrefix(text, "Tools and Licenses") {
				break
			}
		}
		if !inChapter || text != "" {
			continue
		}
		if isHeading(c.Style) {
			t.Errorf("an empty %s paragraph is in chapter 3; it prints as a blank line in the navigation pane", c.Style)
		}
		if outlineLvlRe.MatchString(el) {
			t.Errorf("an empty paragraph with an outline level is in chapter 3; it prints as a blank line in the navigation pane")
		}
	}
}

// narSpacingConfig has several findings in both review sections, which is what
// it takes to see the gap between one finding and the next.
func narSpacingConfig() ReportConfig {
	config := sampleConfig()
	config.Areas = []ReportArea{{Code: "CFG"}, {Code: "NAR"}}
	config.Findings = nil
	for _, f := range []struct{ title, area string }{
		{"Telnet enabled", "CFG"}, {"FTP enabled", "CFG"}, {"No login banner", "CFG"},
		{"Flat network", "NAR"}, {"No redundancy", "NAR"}, {"No DMZ", "NAR"},
	} {
		config.Findings = append(config.Findings, ReportFinding{
			Title: f.title, Severity: "high", Area: f.area, Description: "d", Impact: "i",
			AffectedSystem: "core", Recommendation: "Fix it.",
		})
	}
	return config
}

// The network architecture review lays its findings out exactly as the
// configuration review does: heading, table, one plain blank line - with no
// spacing of its own on either, and no blank line before the first finding.
func TestNetworkArchitectureSpacingMatchesConfigReview(t *testing.T) {
	doc := readDocxParts(t, narSpacingConfig())["word/document.xml"]
	children := bodyChildren(doc)

	shape := func(heading, next string) []string {
		start := findHeading(children, heading)
		end := findHeading(children, next)
		if start < 0 || end < 0 {
			t.Fatalf("section %q not found", heading)
		}
		var out []string
		for _, c := range children[start+1 : end] {
			el := doc[c.Start:c.End]
			switch {
			case strings.Contains(el, "<w:sectPr"):
				continue
			case c.Tag == "w:tbl" && tableHasLabel(el, "description"):
				out = append(out, "finding")
			case c.Tag == "w:tbl":
				out = append(out, "checklist")
			case c.Tag == "w:p" && strings.TrimSpace(elemText(el)) == "":
				out = append(out, "blank")
				if pPr := paraPPr(el); pPr != "" {
					t.Errorf("%s: a blank line between findings carries properties %s", heading, pPr)
				}
			case c.Tag == "w:p" && isHeading(c.Style):
				out = append(out, "heading")
				if paraSpacingRe.MatchString(paraPPr(el)) {
					t.Errorf("%s: finding heading %q sets its own spacing", heading, strings.TrimSpace(elemText(el)))
				}
			default:
				out = append(out, "text")
			}
		}
		return out
	}

	cfg := strings.Join(shape("Configuration Files Review", "Network Architecture Review"), " ")
	nar := strings.Join(shape("Network Architecture Review", "Tools and Licenses"), " ")
	if cfg != nar {
		t.Errorf("the two sections are laid out differently:\n CFG: %s\n NAR: %s", cfg, nar)
	}
}

// TestMultiLineCellIsParagraphsNotLineBreaks is the regression for a finding
// description that printed with its words stretched across the cell.
//
// The template's Normal style is justified, and Word justifies every line of a
// justified paragraph except the last. A scanner's description arrives
// hard-wrapped at about eighty columns; written as one paragraph with a line
// break at each wrap, every one of those lines was a line that is "not last",
// so Word spread each of them over the full width of the cell. A real MongoDB
// description came through as one paragraph of 26,327 characters with 350 line
// breaks in it, reading "memory        leak        vulnerability:".
func TestMultiLineCellIsParagraphsNotLineBreaks(t *testing.T) {
	config := rowsConfig()
	desc := "The instance of MongoDB running on the remote host is affected by MongoBleed, an\n" +
		"unauthenticated unintialized heap memory leak vulnerability:\n" +
		"\n" +
		"0x00:  33 44 38 33 42 35 31 65    3D83B51e\n" +
		"0x10:  46 6F 72 54 65 73 74       ForTest"
	config.Findings[0].Description = desc
	body := readDocxParts(t, config)["word/document.xml"]

	// Every line reached the document.
	for _, line := range strings.Split(desc, "\n") {
		if line == "" {
			continue
		}
		if !strings.Contains(body, xmlEscape(line)) {
			t.Errorf("the description lost the line %q", line)
		}
	}

	// And no paragraph carries the description with line breaks inside it.
	for _, p := range childElems(body, "w:p") {
		para := body[p.Start:p.End]
		if !strings.Contains(para, "affected by MongoBleed") {
			continue
		}
		if n := strings.Count(para, "<w:br/>"); n > 0 {
			t.Errorf("the description is one paragraph with %d line breaks; each line has to be "+
				"its own paragraph or Word justifies all but the last of them", n)
		}
		if text := elemText(para); strings.Contains(text, "0x00:") {
			t.Errorf("the whole description is still a single paragraph: %d characters", len(text))
		}
	}
}

// TestRegisterPhraseColumnsAreNotJustified. The register's Vulnerability and
// Recommendation columns are an inch and a half wide and carry a phrase. The
// template justifies them, and every line of a justified paragraph but the last
// is stretched to the margin - so a three-word title printed as
// "MongoDB            Unauthenticated". A narrow column has no room to
// distribute; the two phrase columns are left aligned instead.
//
// The code columns keep the template's own alignment: each holds one short
// token that never wraps, so nothing there is ever stretched.
func TestRegisterPhraseColumnsAreNotJustified(t *testing.T) {
	config := rowsConfig()
	config.Findings[0].Title = "MongoDB Unauthenticated Uninitialized Heap Memory Leak (MongoBleed)"
	doc := readDocxParts(t, config)["word/document.xml"]

	children := bodyChildren(doc)
	idx := findHeading(children, "Vulnerability Register")
	if idx < 0 {
		t.Fatal("no Vulnerability Register heading")
	}
	tbl := ""
	for i := idx + 1; i < len(children) && i < idx+6; i++ {
		if children[i].Tag == "w:tbl" {
			tbl = doc[children[i].Start:children[i].End]
			break
		}
	}
	if tbl == "" {
		t.Fatal("the register has no table")
	}

	rows := tableRows(tbl)
	if len(rows) < 2 {
		t.Fatalf("the register has %d rows", len(rows))
	}
	cells := rowCells(tbl[rows[1].Start:rows[1].End])
	if len(cells) <= registerRecCol {
		t.Fatalf("the register row has %d cells", len(cells))
	}
	body := tbl[rows[1].Start:rows[1].End]

	for _, col := range []int{registerVulnCol, registerRecCol} {
		cell := body[cells[col].Start:cells[col].End]
		if !strings.Contains(cell, `<w:jc w:val="left"/>`) {
			t.Errorf("register column %d is not left aligned: %s", col, elemText(cell))
		}
		if strings.Contains(cell, `<w:jc w:val="both"/>`) {
			t.Errorf("register column %d is still justified", col)
		}
		// The paragraph properties still come before the runs, or Word offers
		// to repair the document.
		if i, j := strings.Index(cell, "<w:pPr>"), strings.Index(cell, "<w:r>"); i >= 0 && j >= 0 && i > j {
			t.Errorf("register column %d has its pPr after its first run", col)
		}
	}

	// The exposure code column is untouched.
	code := body[cells[1].Start:cells[1].End]
	if strings.Contains(code, `<w:jc w:val="left"/>`) {
		t.Error("the exposure column was realigned; it holds one token and never wraps")
	}
}
