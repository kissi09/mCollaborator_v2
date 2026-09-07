package main

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// Reading a report that this app did not write.
//
// The extractor was built by reading back the app's own output, so it knew one
// layout: a row of labels above a row of values, spelled the way the renderer
// spells them. A real client draft (ECG, September 2026) is laid out the other
// way round - label beside value - calls the rows "Severity:", "Affected URL"
// and "CVSS 3.1", keeps its recommendations in paragraphs after the table
// rather than in it, and names its sections in bold list paragraphs that carry
// no Heading style. Fifty-five findings were laid out that way and five were
// read. These fixtures are that document's shape, reduced.

// docxWithBody wraps body XML in the smallest .docx the extractor will read:
// it only ever opens word/document.xml.
func docxWithBody(body string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		panic(err)
	}
	doc := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		body + `</w:body></w:document>`
	if _, err := w.Write([]byte(doc)); err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func para(text string) string {
	return `<w:p><w:r><w:t>` + text + `</w:t></w:r></w:p>`
}

// row builds a two-cell row: the label beside its value.
func row(label, value string) string {
	cell := func(t string) string {
		return `<w:tc><w:p><w:r><w:t>` + t + `</w:t></w:r></w:p></w:tc>`
	}
	return `<w:tr>` + cell(label) + cell(value) + `</w:tr>`
}

func table(rows ...string) string {
	return `<w:tbl>` + strings.Join(rows, "") + `</w:tbl>`
}

func TestExtractReadsAHandWrittenReport(t *testing.T) {
	body := para("Web Application Testing") +
		para("SQL Injection in the login form - Done") +
		table(
			row("Severity:", "High (7.5)"),
			row("Affected URL", "https://app.example.com/login"),
			row("Description:", "The login form concatenates its input into a SQL query."),
			row("Impact", "An attacker reads the whole customer table."),
			row("CVSS 3.1", "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"),
		) +
		para("PoC:") +
		para("' OR 1=1 --") +
		para("Recommendations:") +
		para("Use parameterised queries.") +
		para("Validate input server side.")

	findings, _, err := ExtractFromDOCX(docxWithBody(body))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	f := findings[0]

	// The title carries the author's working note, which is not its name.
	if f.Title != "SQL Injection in the login form" {
		t.Errorf("title = %q, want the editorial suffix stripped", f.Title)
	}
	// "Severity:" is the label and the value carries the score with the word.
	if f.Severity != "high" {
		t.Errorf("severity = %q, want high", f.Severity)
	}
	if f.CVSSScore != 7.5 {
		t.Errorf("cvss score = %v, want 7.5 read out of the rating cell", f.CVSSScore)
	}
	if !strings.HasPrefix(f.CVSSVector, "CVSS:3.1/") {
		t.Errorf("cvss vector = %q", f.CVSSVector)
	}
	if !strings.Contains(f.AffectedSystem, "app.example.com") {
		t.Errorf("affected = %q, want the Affected URL row", f.AffectedSystem)
	}
	if !strings.Contains(f.Description, "concatenates") {
		t.Errorf("description = %q", f.Description)
	}
	if !strings.Contains(f.Impact, "customer table") {
		t.Errorf("impact = %q", f.Impact)
	}
	// PoC and Recommendations are paragraphs after the table, not rows in it.
	if !strings.Contains(f.POC, "OR 1=1") {
		t.Errorf("poc = %q, want the paragraphs under PoC:", f.POC)
	}
	if !strings.Contains(f.Remediation, "parameterised") || !strings.Contains(f.Remediation, "Validate input") {
		t.Errorf("remediation = %q, want both paragraphs under Recommendations:", f.Remediation)
	}
	// The area comes from a paragraph carrying no Heading style at all.
	if f.Category != "WPT" {
		t.Errorf("category = %q, want WPT from the plain section paragraph", f.Category)
	}
	if f.Confidence != ConfHeading {
		t.Errorf("confidence = %q, want the heading route", f.Confidence)
	}
}

// A draft leaves unwritten findings as XXXX. Importing them would put empty
// rows in the engagement for somebody to hunt down and delete.
func TestExtractSkipsPlaceholderFindings(t *testing.T) {
	body := para("Real finding") +
		table(
			row("Severity:", "Low"),
			row("Description:", "Something genuinely wrong."),
			row("Impact", "Minor."),
		) +
		para("XXXX") +
		table(
			row("Severity:", "XXX"),
			row("Affected URL", "XXXX"),
			row("Description:", "XXXX"),
			row("Impact", "XXXX"),
		)

	findings, notes, err := ExtractFromDOCX(docxWithBody(body))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected the placeholder to be dropped, got %d findings: %+v", len(findings), findings)
	}
	if findings[0].Title != "Real finding" {
		t.Errorf("kept the wrong one: %q", findings[0].Title)
	}
	joined := strings.Join(notes, " ")
	if !strings.Contains(joined, "placeholder") {
		t.Errorf("expected a note about the skipped placeholder, got %q", joined)
	}
}

// The renderer's own layout must keep reading the way it did: labels side by
// side above their values. Reading that row horizontally would file one label
// as another's value.
func TestExtractStillReadsLabelRowAboveValueRow(t *testing.T) {
	cell := func(t string) string {
		return `<w:tc><w:p><w:r><w:t>` + t + `</w:t></w:r></w:p></w:tc>`
	}
	body := para("Weak ciphers offered") +
		`<w:tbl>` +
		`<w:tr>` + cell("Description") + cell("Rating") + `</w:tr>` +
		`<w:tr>` + cell("TLS 1.0 is still offered.") + cell("Medium") + `</w:tr>` +
		`</w:tbl>`

	findings, _, err := ExtractFromDOCX(docxWithBody(body))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if got := findings[0].Description; got != "TLS 1.0 is still offered." {
		t.Errorf("description = %q - a label was read as a value", got)
	}
	if got := findings[0].Severity; got != "medium" {
		t.Errorf("severity = %q, want medium", got)
	}
}

func TestNormalizeLabelIgnoresPunctuationAndCase(t *testing.T) {
	cases := map[string]string{
		"Severity:":                "rating",
		"SEVERITY":                 "rating",
		"Description:":             "description",
		"Affected URLs:":           "affected",
		"Affected app & endpoints": "affected",
		"CVSS 3.1":                 "cvss",
		"Vector String":            "cvss",
		"Recommendations":          "recommendation",
		"  Impact  ":               "impact",
		"Affected Hosts":           "affected",
	}
	for label, want := range cases {
		got, ok := extractLabelKey(label)
		if !ok || got != want {
			t.Errorf("%q resolved to (%q, %v), want %q", label, got, ok, want)
		}
	}
	if _, ok := extractLabelKey("The quick brown fox"); ok {
		t.Error("a sentence must not resolve to a label")
	}
}
