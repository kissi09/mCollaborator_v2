package main

import (
	"regexp"
	"strings"
	"testing"
)

// authorCells returns the text of each cell in the cover table's Authors
// column, top to bottom, with the name and the role separated by " / ".
func authorCells(t *testing.T, doc string) []string {
	t.Helper()

	children := bodyChildren(doc)
	tblIdx := -1
	for i, c := range children {
		if c.Tag == "w:tbl" {
			tblIdx = i
			break
		}
	}
	if tblIdx < 0 {
		t.Fatal("cover table not found")
	}
	tbl := doc[children[tblIdx].Start:children[tblIdx].End]

	rows := tableRows(tbl)
	header, col := -1, -1
	for ri, r := range rows {
		row := tbl[r.Start:r.End]
		for ci, c := range rowCells(row) {
			if strings.TrimSpace(elemText(row[c.Start:c.End])) == "Authors" {
				header, col = ri, ci
			}
		}
		if header >= 0 {
			break
		}
	}
	if header < 0 {
		t.Fatal("Authors column not found in the cover table")
	}

	var out []string
	for ri := header + 1; ri < len(rows); ri++ {
		row := tbl[rows[ri].Start:rows[ri].End]
		cells := rowCells(row)
		if col >= len(cells) {
			continue
		}
		cell := row[cells[col].Start:cells[col].End]
		var lines []string
		for _, p := range childElems(cell, "w:p") {
			if s := strings.TrimSpace(elemText(cell[p.Start:p.End])); s != "" {
				lines = append(lines, s)
			}
		}
		if len(lines) > 0 {
			out = append(out, strings.Join(lines, " / "))
		}
	}
	return out
}

// The template ships two author cells, both holding the same placeholder. The
// primary author belongs in the first and the second author in the one below.
func TestTwoAuthorsFillTheirOwnRows(t *testing.T) {
	config := sampleConfig()
	config.TesterName = "Emmanuel Addo Kissi"
	config.TesterTitle = "Lead Penetration Tester"
	config.SecondAuthorName = "Ama Mensah"
	config.SecondAuthorTitle = "Security Analyst"

	doc := readDocxParts(t, config)["word/document.xml"]
	got := authorCells(t, doc)

	want := []string{
		"Emmanuel Addo Kissi / Lead Penetration Tester",
		"Ama Mensah / Security Analyst",
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d author rows, got %d: %q", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("author row %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// A single author must appear once. Writing the placeholder everywhere it
// occurred used to print the same person in both rows.
func TestOneAuthorIsNotDuplicated(t *testing.T) {
	config := sampleConfig()
	config.TesterName = "Emmanuel Addo Kissi"
	config.TesterTitle = ""
	config.SecondAuthorName = ""

	doc := readDocxParts(t, config)["word/document.xml"]
	got := authorCells(t, doc)

	if len(got) != 1 {
		t.Fatalf("expected exactly one author row, got %d: %q", len(got), got)
	}
	// An author with no title of their own keeps the template's own wording.
	if want := "Emmanuel Addo Kissi / " + defaultAuthorTitle; got[0] != want {
		t.Errorf("author row = %q, want %q", got[0], want)
	}
	if n := strings.Count(doc, "Emmanuel Addo Kissi"); n != 1 {
		t.Errorf("the author's name appears %d times in the document, want 1", n)
	}
}

// The empty second row is removed, not left behind as a ruled blank line.
func TestSpareAuthorRowIsRemoved(t *testing.T) {
	config := sampleConfig()
	config.SecondAuthorName = ""

	doc := readDocxParts(t, config)["word/document.xml"]

	children := bodyChildren(doc)
	var tbl string
	for _, c := range children {
		if c.Tag == "w:tbl" {
			tbl = doc[c.Start:c.End]
			break
		}
	}
	rows := tableRows(tbl)
	header := -1
	for ri, r := range rows {
		text := elemText(tbl[r.Start:r.End])
		if strings.Contains(text, "Version") && strings.Contains(text, "Approver") {
			header = ri
			break
		}
	}
	if header < 0 {
		t.Fatal("cover header row not found")
	}
	for ri := header + 1; ri < len(rows); ri++ {
		row := tbl[rows[ri].Start:rows[ri].End]
		if len(rowCells(row)) < 6 {
			continue // the trailing full-width spacer row
		}
		if strings.TrimSpace(elemText(row)) == "" {
			t.Errorf("row %d under the cover header is a blank data row; it should have been removed", ri)
		}
	}
}

// Each area's finding block has its own set of labelled rows. The wizard offers
// the fields those labels ask for, so this pins the labels the template
// actually ships - if one changes, the editor has to change with it.
func TestSectionLayoutsCarryTheirOwnLabels(t *testing.T) {
	doc := templateDocumentXML(t)

	want := map[string][]string{
		"IPT":  {"Description", "Rating", "CVSS Vector", "Attack Vector", "Affected Hosts", "Recommendation"},
		"EPT":  {"Description", "Rating", "Impact", "CVSS Vector String", "Affected Host", "Recommendation"},
		"IPTC": {"Description", "Rating", "CVSS Vector", "Impact", "Affected Host", "POC", "Recommendation"},
		"WPT":  {"Description", "Rating", "CVSS Vector", "Impact", "Affected Application", "PoC", "Recommendation"},
		"CFG":  {"Description", "Rating", "Affected Device", "Recommendation"},
		"ADT":  {"Description", "Rating", "Attack Vector", "Affected Domain", "PoC", "Recommendation"},
		"NAR":  {"Description", "Rating", "Affected Network", "Recommendation"},
		"WNA":  {"Description", "Rating", "Attack Vector", "Affected SSIDs", "PoC", "Recommendation"},
	}

	children := bodyChildren(doc)
	marker := regexp.MustCompile(`^\[([A-Z]+) Vulnerability 1\]`)

	// Where each area's detail block starts, in document order.
	type mark struct {
		code  string
		start int
	}
	var marks []mark
	toolsAt := len(doc)
	for _, c := range children {
		text := strings.TrimSpace(elemText(doc[c.Start:c.End]))
		if m := marker.FindStringSubmatch(text); m != nil {
			marks = append(marks, mark{m[1], c.Start})
			continue
		}
		if strings.HasPrefix(text, "Tools and Licenses") && len(marks) > 0 && c.Start > marks[len(marks)-1].start {
			toolsAt = c.Start
			break
		}
	}
	if len(marks) != len(want) {
		t.Fatalf("found %d vulnerability blocks in the template, expected %d", len(marks), len(want))
	}

	for i, m := range marks {
		end := toolsAt
		if i+1 < len(marks) {
			end = marks[i+1].start
		}
		frag := doc[m.start:end]

		var labels []string
		seen := map[string]bool{}
		for _, tblSpan := range childElems("<w:body>"+frag+"</w:body>", "w:tbl") {
			wrapped := "<w:body>" + frag + "</w:body>"
			tbl := wrapped[tblSpan.Start:tblSpan.End]
			for _, r := range tableRows(tbl) {
				row := tbl[r.Start:r.End]
				for _, c := range rowCells(row) {
					text := strings.TrimSpace(elemText(row[c.Start:c.End]))
					if _, ok := detailLabels[strings.ToLower(text)]; ok && !seen[text] {
						seen[text] = true
						labels = append(labels, text)
					}
				}
			}
		}

		expect, ok := want[m.code]
		if !ok {
			t.Errorf("template carries an unexpected %s block", m.code)
			continue
		}
		if strings.Join(labels, "|") != strings.Join(expect, "|") {
			t.Errorf("%s layout = %v, want %v", m.code, labels, expect)
		}
	}
}
