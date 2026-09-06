package main

import (
	"strconv"
	"strings"
	"testing"
)

// The strongest test available: render a report from a known engagement, then
// read it back. Anything the renderer writes, the extractor should recover.
func extractRendered(t *testing.T, config ReportConfig) []ExtractedFinding {
	t.Helper()
	out, _, err := mergeMCollaboratorDocx(config)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	found, notes, err := ExtractFromDOCX(out)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	for _, n := range notes {
		t.Logf("note: %s", n)
	}
	return found
}

func byTitle(findings []ExtractedFinding, title string) (ExtractedFinding, bool) {
	for _, f := range findings {
		if strings.EqualFold(strings.TrimSpace(f.Title), title) {
			return f, true
		}
	}
	return ExtractedFinding{}, false
}

func TestExtractReadsBackAGeneratedReport(t *testing.T) {
	config := rowsConfig()
	found := extractRendered(t, config)

	if len(found) != len(config.Findings) {
		var titles []string
		for _, f := range found {
			titles = append(titles, f.Title)
		}
		t.Fatalf("read %d findings out of a report with %d in it: %v", len(found), len(config.Findings), titles)
	}

	for _, want := range config.Findings {
		got, ok := byTitle(found, want.Title)
		if !ok {
			t.Errorf("finding %q was not read back", want.Title)
			continue
		}
		if got.Category != want.Area {
			t.Errorf("%s: area = %q, want %q (%s)", want.Title, got.Category, want.Area, got.Reason)
		}
		if got.Confidence != ConfHeading {
			t.Errorf("%s: confidence = %q, want %q — it sat under its own section heading", want.Title, got.Confidence, ConfHeading)
		}
		if got.Severity != want.Severity {
			t.Errorf("%s: severity = %q, want %q", want.Title, got.Severity, want.Severity)
		}
		if !strings.Contains(got.Description, strings.TrimSuffix(want.Description, ".")) {
			t.Errorf("%s: description = %q, want it to carry %q", want.Title, got.Description, want.Description)
		}
		if want.AffectedSystem != "" && got.AffectedSystem != want.AffectedSystem {
			t.Errorf("%s: affected = %q, want %q", want.Title, got.AffectedSystem, want.AffectedSystem)
		}
		if want.Impact != "" && got.Impact != want.Impact {
			t.Errorf("%s: impact = %q, want %q", want.Title, got.Impact, want.Impact)
		}
	}
}

// The recommendation comes back as the tester wrote it, without the vulnerability
// id the report prints in front of it.
func TestExtractStripsTheRecommendationID(t *testing.T) {
	found := extractRendered(t, rowsConfig())
	got, ok := byTitle(found, "Default SNMP community string")
	if !ok {
		t.Fatal("the CFG finding was not read back")
	}
	if strings.Contains(got.Remediation, "_REC") {
		t.Errorf("recommendation still carries its vulnerability id: %q", got.Remediation)
	}
	if !strings.HasPrefix(got.Remediation, "Set a unique community string") {
		t.Errorf("recommendation = %q, want it to start with the text that was written", got.Remediation)
	}
}

// A document with no section headings falls back to the finding's own wording,
// and says so.
func TestExtractGuessesTheAreaFromWording(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"Stored cross-site scripting in the profile page", "WPT"},
		{"Kerberoastable service account with an SPN", "ADT"},
		{"Default SNMP community string in the running config", "CFG"},
		{"WPA2 passphrase recovered from a captured handshake on the guest SSID", "WNA"},
		{"Flat network with no segmentation between VLANs", "NAR"},
		{"Public S3 bucket exposes customer statements", "IPTC"},
	}
	for _, tc := range cases {
		got, why := guessArea(tc.text)
		if got != tc.want {
			t.Errorf("guessArea(%q) = %q (%s), want %q", tc.text, got, why, tc.want)
		}
	}
}

// Wording that reads as two areas equally is left unplaced rather than filed
// under a coin toss - that is the whole point of the holding pen.
func TestAmbiguousWordingIsLeftUnplaced(t *testing.T) {
	f := ExtractedFinding{
		Title:       "Guest SSID reaches the internal file share",
		Description: "The wireless guest SSID routes onto the internal network and reaches a file share.",
	}
	placeArea(&f, "", "")
	if f.Category != "" {
		t.Errorf("area = %q, want it left empty when the wording reads two ways", f.Category)
	}
	if f.Confidence != ConfNone {
		t.Errorf("confidence = %q, want %q", f.Confidence, ConfNone)
	}
	if !strings.Contains(f.Reason, "reads as both") {
		t.Errorf("reason = %q, want it to say the wording reads two ways", f.Reason)
	}
}

func TestUnrecognisableWordingIsLeftUnplaced(t *testing.T) {
	f := ExtractedFinding{Title: "Outdated third party component", Description: "A library is behind."}
	placeArea(&f, "", "")
	if f.Category != "" || f.Confidence != ConfNone {
		t.Errorf("area = %q / %q, want it left for the reviewer", f.Category, f.Confidence)
	}
}

// The narrow keyword lists must not fire inside longer words.
func TestKeywordsMatchWholeWordsOnly(t *testing.T) {
	if got, _ := guessArea("Oracle database listener discloses its version"); got == "CFG" {
		t.Error("“acl” fired inside “Oracle”")
	}
	if got, _ := guessArea("The service did not respond to the probe"); got == "ADT" {
		t.Error("“spn” fired inside “respond”")
	}
}

// A PDF keeps the words but not the table. The row parser has to find the same
// findings out of the rows it does keep.
func TestExtractFromPDFText(t *testing.T) {
	text := strings.Join([]string{
		"3.3 Internal Penetration Testing",
		"3.3.1 SMB signing is not required on the domain controllers",
		"Description Rating",
		"Both domain controllers accept SMB sessions that are not signed. High",
		"Affected Hosts",
		"10.20.4.11, 10.20.4.12",
		"Recommendation",
		"ATB_REC1_IPT1 \u2013 Require SMB signing",
		"Require SMB signing through Group Policy.",
		"3.7 Configuration Files Review",
		"3.7.1 Default SNMP community string on the perimeter firewalls",
		"Description Rating",
		"All three appliances answer to the community string public. High",
		"Affected Device",
		"fw-hq-01",
		"Recommendation",
		"ATB_REC2_CFG1 \u2013 Replace the community strings",
		"Set a unique community string.",
	}, "\n")

	found, notes := ExtractFromPDFText(text)
	for _, n := range notes {
		t.Logf("note: %s", n)
	}
	if len(found) != 2 {
		t.Fatalf("read %d findings, want 2: %+v", len(found), found)
	}

	first := found[0]
	if first.Category != "IPT" || first.Confidence != ConfHeading {
		t.Errorf("first finding area = %q/%q, want IPT from its heading", first.Category, first.Confidence)
	}
	if first.Title != "SMB signing is not required on the domain controllers" {
		t.Errorf("first finding title = %q", first.Title)
	}
	if first.Severity != "high" {
		t.Errorf("first finding severity = %q, want high - it was on the end of the description row", first.Severity)
	}
	if first.AffectedSystem != "10.20.4.11, 10.20.4.12" {
		t.Errorf("first finding affected = %q", first.AffectedSystem)
	}
	if !strings.Contains(first.Description, "not signed") || strings.HasSuffix(first.Description, "High") {
		t.Errorf("first finding description = %q, want the rating split off the end", first.Description)
	}
	if strings.Contains(first.Remediation, "_REC") {
		t.Errorf("first finding recommendation still carries its id: %q", first.Remediation)
	}

	second := found[1]
	if second.Category != "CFG" {
		t.Errorf("second finding area = %q, want CFG", second.Category)
	}
	if !strings.Contains(second.Remediation, "unique community string") {
		t.Errorf("second finding recommendation = %q", second.Remediation)
	}
}

// A table of contents is a list of headings with nothing under them, and must
// not come back as a list of empty findings.
func TestPDFTableOfContentsIsNotFindings(t *testing.T) {
	text := strings.Join([]string{
		"Contents",
		"3.3 Internal Penetration Testing ................................ 19",
		"3.3.1 SMB signing is not required on the domain controllers ..... 19",
		"3.3.2 Unsupported Windows Server 2012 R2 hosts .................. 21",
		"3.7 Configuration Files Review .................................. 38",
	}, "\n")
	found, _ := ExtractFromPDFText(text)
	if len(found) != 0 {
		t.Errorf("the table of contents produced %d findings: %+v", len(found), found)
	}
}

// The vulnerability register's own column headings must not open a finding.
func TestPDFStrayLabelDoesNotStartAFinding(t *testing.T) {
	text := strings.Join([]string{
		"3.2 Vulnerability Register",
		"Vulnerability Exposure Criticality Recommendation",
		"SMB signing not required Internal High ATB_REC1_IPT1",
		"3.3 Internal Penetration Testing",
	}, "\n")
	found, _ := ExtractFromPDFText(text)
	if len(found) != 0 {
		t.Errorf("a column heading opened a finding: %+v", found)
	}
}

func TestPDFLabelRowParsing(t *testing.T) {
	if keys, ok := pdfLabelRow("Description Rating"); !ok || len(keys) != 2 || keys[0] != "description" || keys[1] != "rating" {
		t.Errorf("pdfLabelRow(Description Rating) = %v/%v", keys, ok)
	}
	if _, ok := pdfLabelRow("Description of the estate follows"); ok {
		t.Error("a sentence starting with a label word was taken as a label row")
	}
	if key, val, ok := pdfLabelledInline("Impact: Full account takeover."); !ok || key != "impact" || val != "Full account takeover." {
		t.Errorf("pdfLabelledInline = %q/%q/%v", key, val, ok)
	}
}

// A running footer repeats at the foot of every page and would otherwise be
// appended to whichever field was open when the page broke. A value that
// happens to repeat as often must survive, which is why position decides it.
func TestPDFFooterIsDroppedButRepeatedValuesSurvive(t *testing.T) {
	var rows []string
	for page := 1; page <= 3; page++ {
		rows = append(rows,
			// The footer sits at the top and bottom of every page; everything
			// between is the page's own content.
			"Cyberteq Falcon Ltd. VAPT Report Page "+strconv.Itoa(page),
			"3.3 Internal Penetration Testing",
			"3.3."+strconv.Itoa(page)+" Finding number "+strconv.Itoa(page),
			"Description Rating",
			"A weakness was found on host "+strconv.Itoa(page)+". High",
			"Impact",
			"It lets an attacker move sideways.",
			"CVSS Vector",
			"CVSS:3.1/AV:A/AC:L/PR:N/UI:R/S:U/C:H/I:H/A:H",
			"Attack Vector",
			"Adjacent Network",
			"Recommendation",
			"Fix it on host "+strconv.Itoa(page)+".",
			"Cyberteq Falcon Ltd. VAPT Report Page "+strconv.Itoa(page),
			"All rights reserved Ref: TEST-REP-001",
			"\f")
	}

	found, _ := ExtractFromPDFText(strings.Join(rows, "\n"))
	if len(found) != 3 {
		t.Fatalf("read %d findings, want 3: %+v", len(found), found)
	}
	for i, f := range found {
		if f.AttackVector != "Adjacent Network" {
			t.Errorf("finding %d: attack vector = %q - a value repeating as often as a footer was dropped with it", i, f.AttackVector)
		}
		if strings.Contains(f.AttackVector+f.Description, "Cyberteq Falcon") ||
			strings.Contains(f.AttackVector+f.Description, "All rights reserved") {
			t.Errorf("finding %d: the page furniture landed in a field: %+v", i, f)
		}
		if f.Severity != "high" {
			t.Errorf("finding %d: severity = %q, want high", i, f.Severity)
		}
	}
}

// Something that is not a Word file has to say so rather than come back empty.
func TestExtractRejectsGarbage(t *testing.T) {
	if _, _, err := ExtractFromDOCX([]byte("this is not a zip archive")); err == nil {
		t.Fatal("a non-docx was accepted")
	}
}

func TestBuildExtractResultCounts(t *testing.T) {
	res := BuildExtractResult("r.docx", "docx", []ExtractedFinding{
		{Category: "IPT"}, {Category: "IPT"}, {Category: "CFG"}, {Category: ""},
	}, nil)
	if res.Unplaced != 1 {
		t.Errorf("unplaced = %d, want 1", res.Unplaced)
	}
	if res.ByArea["IPT"] != 2 || res.ByArea["CFG"] != 1 {
		t.Errorf("by_area = %v", res.ByArea)
	}
}

// The CVSS base score is a published equation; these are the specification's own
// worked examples.
func TestCVSSBaseScore(t *testing.T) {
	cases := []struct {
		vector string
		want   float64
	}{
		{"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", 9.8},
		{"CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N", 6.1},
		{"CVSS:3.1/AV:A/AC:L/PR:N/UI:R/S:U/C:H/I:H/A:H", 8.0},
		{"CVSS:3.1/AV:N/AC:H/PR:N/UI:N/S:U/C:H/I:N/A:N", 5.9},
		{"CVSS:3.1/AV:L/AC:L/PR:L/UI:N/S:U/C:N/I:N/A:N", 0.0},
	}
	for _, tc := range cases {
		got, ok := cvssBaseScore(tc.vector)
		if !ok {
			t.Errorf("%s: could not be scored", tc.vector)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: score = %.1f, want %.1f", tc.vector, got, tc.want)
		}
	}
	if _, ok := cvssBaseScore("CVSS:3.1/AV:N/AC:L"); ok {
		t.Error("a partial vector was scored; it has no score")
	}
}

func TestSeverityFromVector(t *testing.T) {
	if got := severityFromVector("CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"); got != "critical" {
		t.Errorf("severity = %q, want critical", got)
	}
	if got := severityFromVector("CVSS:3.1/AV:N/AC:H/PR:N/UI:N/S:U/C:H/I:N/A:N"); got != "medium" {
		t.Errorf("severity = %q, want medium", got)
	}
}
