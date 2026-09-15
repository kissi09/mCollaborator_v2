package main

import (
	"strings"
	"testing"
)

// Mobile Penetration Testing and Source Code Review have no block, no scope row
// and no naming convention line in the template. Everything that names an area
// has to be built for them, or they print a finding with nowhere to live.

func mobileSourceConfig() ReportConfig {
	config := sampleConfig()
	config.Areas = []ReportArea{
		{Code: "WPT", Scope: "https://portal.acme.test"},
		{Code: "MPT", Scope: "Acme Banking for Android 4.2"},
		{Code: "SCR", Scope: "payments-api repository"},
	}
	config.Findings = []ReportFinding{
		{Title: "Reflected XSS", Severity: "medium", Area: "WPT",
			Description: "q is echoed.", Impact: "Session theft.", CVSSVector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N",
			AffectedSystem: "portal.acme.test", POC: "GET /search?q=", Recommendation: "Encode on output."},
		{Title: "Session token stored in shared preferences", Severity: "high", Area: "MPT",
			Description: "The token is written unencrypted.", Impact: "Account takeover on a lost device.",
			CVSSVector: "CVSS:3.1/AV:P/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N",
			AffectedSystem: "com.acme.banking 4.2", POC: "adb pull shared_prefs", Recommendation: "Use the Android Keystore."},
		{Title: "Hardcoded database password", Severity: "critical", Area: "SCR",
			Description: "db.go carries the production password.", Impact: "Direct database access.",
			CVSSVector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H",
			AffectedSystem: "payments-api", POC: "config/db.go line 14", Recommendation: "Move secrets to a vault."},
	}
	return config
}

func TestMobileAndSourceCodeLayouts(t *testing.T) {
	doc := readDocxParts(t, mobileSourceConfig())["word/document.xml"]

	cases := []struct {
		title string
		want  []string
	}{
		{"Session token stored in shared preferences",
			[]string{"Description", "Rating", "Impact", "Affected Application", "PoC", "Recommendation"}},
		{"Hardcoded database password",
			[]string{"Description", "Rating", "Impact", "CVSS", "Affected Applications", "PoC", "Recommendation"}},
		// The web block both are built from is left exactly as it was.
		{"Reflected XSS",
			[]string{"Description", "Rating", "CVSS Vector", "Impact", "Affected Application", "PoC", "Recommendation"}},
	}
	for _, c := range cases {
		rows := findingRows(t, doc, c.title)
		if got := labelsOf(rows); strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s rows = %v, want %v", c.title, got, c.want)
		}
	}

	scr := findingRows(t, doc, "Hardcoded database password")
	for _, want := range [][2]string{
		{"CVSS", "CVSS:3.1/AV:N"}, {"Impact", "Direct database access"},
		{"Affected Applications", "payments-api"}, {"PoC", "config/db.go"},
	} {
		if !hasRow(scr, want[0], want[1]) {
			t.Errorf("SCR finding lost %s=%q: %v", want[0], want[1], scr)
		}
	}
	mpt := findingRows(t, doc, "Session token stored in shared preferences")
	if !hasRow(mpt, "Affected Application", "com.acme.banking") || !hasRow(mpt, "PoC", "adb pull") {
		t.Errorf("MPT finding lost a value: %v", mpt)
	}

	// Each prints under its own section heading, with its own vulnerability id.
	children := bodyChildren(doc)
	for _, h := range []string{"Mobile Penetration Testing", "Source Code Review"} {
		if findHeading(children, h) < 0 {
			t.Errorf("no %q section heading", h)
		}
	}
	for _, id := range []string{"_MPT1", "_SCR1"} {
		if !strings.Contains(elemText(doc), id) {
			t.Errorf("no vulnerability id ending %s", id)
		}
	}
}

func TestMobileAndSourceCodeAreNamedEverywhere(t *testing.T) {
	doc := readDocxParts(t, mobileSourceConfig())["word/document.xml"]
	children := bodyChildren(doc)

	// Scope table: one row each, in report order, with the scope typed.
	idx := findHeading(children, "Scope")
	var scope string
	for _, c := range children[idx:] {
		if c.Tag == "w:tbl" {
			scope = doc[c.Start:c.End]
			break
		}
	}
	var activities []string
	for _, r := range tableRows(scope)[1:] {
		row := scope[r.Start:r.End]
		cells := rowCells(row)
		activities = append(activities, strings.TrimSpace(elemText(row[cells[0].Start:cells[0].End]))+
			"="+strings.TrimSpace(elemText(row[cells[1].Start:cells[1].End])))
	}
	want := []string{
		"Web Application Testing=https://portal.acme.test",
		"Mobile Penetration Testing=Acme Banking for Android 4.2",
		"Source Code Review=payments-api repository",
	}
	if strings.Join(activities, "|") != strings.Join(want, "|") {
		t.Errorf("scope rows = %v, want %v", activities, want)
	}

	// Naming convention: a line each, after the web line, and nothing unselected.
	var lines []string
	start := findHeading(children, "Recommendations Naming Convention")
	for _, c := range children[start+1 : blockEnd(children, start, 2)] {
		text := strings.TrimSpace(elemText(doc[c.Start:c.End]))
		for _, a := range reportAreas {
			if strings.HasPrefix(text, a.Code+" ") {
				lines = append(lines, text)
			}
		}
	}
	wantLines := []string{
		"WPT – Web Application Penetration Testing",
		"MPT – Mobile Penetration Testing",
		"SCR – Source Code Review",
	}
	if strings.Join(lines, "|") != strings.Join(wantLines, "|") {
		t.Errorf("naming convention lines = %v, want %v", lines, wantLines)
	}
	// Only the code is bold, as on the template's own lines.
	for _, c := range children {
		para := doc[c.Start:c.End]
		text := strings.TrimSpace(elemText(para))
		if text != "WPT – Web Application Penetration Testing" && text != "MPT – Mobile Penetration Testing" {
			continue
		}
		var bold []bool
		for _, r := range childElems(para, "w:r") {
			if rPr, rt, ok := simpleRun(para[r.Start:r.End]); ok && strings.TrimSpace(rt) != "" {
				bold = append(bold, strings.Contains(rPr, "<w:b/>") || strings.Contains(rPr, `<w:b w:val="1"/>`))
			}
		}
		if len(bold) < 2 || !bold[0] || bold[len(bold)-1] {
			t.Errorf("%q: bold runs %v, want the code bold and the name not", text, bold)
		}
	}

	// Table of contents and register.
	all := elemText(doc)
	for _, want := range []string{"3.4Mobile Penetration Testing", "3.5Source Code Review"} {
		if !strings.Contains(all, want) {
			t.Errorf("contents lack %q", want)
		}
	}

	// Chapter 1 speaks about them.
	intro := paragraphStarting(t, doc, introOpening)
	for _, want := range []string{"mobile applications", "A review of the application source code"} {
		if !strings.Contains(intro, want) {
			t.Errorf("introduction lacks %q: %s", want, intro)
		}
	}
	risk := paragraphStarting(t, doc, "The critical issues identified")
	for _, want := range []string{"insecure data storage and communication in the mobile applications", "insecure coding practices in the application source code"} {
		if !strings.Contains(risk, want) {
			t.Errorf("risk overview lacks %q: %s", want, risk)
		}
	}
}

// A source-code-only engagement is a review: no scans, no exploitation, and no
// network devices in the attack narrative.
func TestSourceCodeOnlyReportIsAReview(t *testing.T) {
	config := mobileSourceConfig()
	config.Areas = []ReportArea{{Code: "SCR"}}
	config.Findings = config.Findings[2:]
	doc := readDocxParts(t, config)["word/document.xml"]
	all := strings.Join(paragraphTexts(doc), "\n")
	for _, wrong := range []string{"vulnerability scan was initiated", "network devices", "network design", "remotely for"} {
		if strings.Contains(all, wrong) {
			t.Errorf("an SCR-only report says %q", wrong)
		}
	}
	for _, want := range []string{"secure coding best practice", "insecure coding practices", "secure development processes", "flaws identified in the source code"} {
		if !strings.Contains(all, want) {
			t.Errorf("an SCR-only report lacks %q", want)
		}
	}
}

func TestImporterRecognisesMobileAndSourceCode(t *testing.T) {
	for text, want := range map[string]string{
		"Certificate pinning bypassed with Frida on the Android build": "MPT",
		"Hardcoded API secret found in the source code":                "SCR",
	} {
		if got, _ := guessArea(strings.ToLower(text)); got != want {
			t.Errorf("guessArea(%q) = %q, want %q", text, got, want)
		}
	}
}
