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
// reader's navigation pane.
func TestNoEmptyHeadingsInTheFindings(t *testing.T) {
	doc := readDocxParts(t, rowsConfig())["word/document.xml"]
	children := bodyChildren(doc)

	inChapter := false
	for _, c := range children {
		text := strings.TrimSpace(elemText(doc[c.Start:c.End]))
		if c.Tag == "w:p" && isHeading(c.Style) {
			if text == "Internal Penetration Testing" {
				inChapter = true
			}
			if strings.HasPrefix(text, "Tools and Licenses") {
				break
			}
			if inChapter && text == "" {
				t.Errorf("an empty %s paragraph is in chapter 3; it prints as a blank line in the navigation pane", c.Style)
			}
		}
	}
}
