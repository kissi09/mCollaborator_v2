package main

import (
	"strings"
	"testing"
)

// The same test the finding extractor gets, for the engagement rather than the
// findings: render a report from a known configuration, then read it back and
// check that the deck would be built from what the report actually says.
func TestClosureImportReadsBackAGeneratedReport(t *testing.T) {
	config := rowsConfig()
	docx, _, err := mergeMCollaboratorDocx(config)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	draft, err := closureDraftFromDOCX(docx)
	if err != nil {
		t.Fatalf("closure import failed: %v", err)
	}
	for _, n := range draft.Notes {
		t.Logf("note: %s", n)
	}

	got := draft.Config
	checks := []struct{ name, got, want string }{
		{"company", got.CompanyName, config.CompanyName},
		{"initials", got.CompanyInitials, config.CompanyInitials},
		{"reference", got.RefNumber, config.RefNumber},
		{"date", got.ReportDate, config.ReportDate},
		{"assessment start", got.AssessmentStart, config.AssessmentStart},
		{"assessment end", got.AssessmentEnd, config.AssessmentEnd},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}

	if len(draft.Missing) != 0 {
		t.Errorf("a report this app wrote left %v to be typed in by hand", draft.Missing)
	}

	// Every area, with the scope the report prints beside it, and in template
	// order however the document happened to be read.
	if len(got.Areas) != len(config.Areas) {
		t.Fatalf("read %d areas, want %d: %+v", len(got.Areas), len(config.Areas), got.Areas)
	}
	for i, want := range config.Areas {
		if got.Areas[i].Code != want.Code {
			t.Errorf("area %d = %q, want %q (areas must come back in template order)", i, got.Areas[i].Code, want.Code)
		}
		if got.Areas[i].Scope != want.Scope {
			t.Errorf("%s scope = %q, want %q", want.Code, got.Areas[i].Scope, want.Scope)
		}
	}

	// Every finding, in an area, so the deck has an issues slide for each.
	if len(got.Findings) != len(config.Findings) {
		t.Fatalf("read %d findings, want %d", len(got.Findings), len(config.Findings))
	}
	if draft.Unplaced != 0 {
		t.Errorf("%d findings came back with no area out of a report that prints them under headings", draft.Unplaced)
	}
	for _, want := range config.Findings {
		var found *ReportFinding
		for i := range got.Findings {
			if strings.EqualFold(got.Findings[i].Title, want.Title) {
				found = &got.Findings[i]
				break
			}
		}
		if found == nil {
			t.Errorf("finding %q was not read back", want.Title)
			continue
		}
		if found.Area != want.Area {
			t.Errorf("%s: area = %q, want %q", want.Title, found.Area, want.Area)
		}
		if found.Severity != want.Severity {
			t.Errorf("%s: severity = %q, want %q", want.Title, found.Severity, want.Severity)
		}
		if want.AffectedSystem != "" && found.AffectedSystem != want.AffectedSystem {
			t.Errorf("%s: affected host = %q, want %q", want.Title, found.AffectedSystem, want.AffectedSystem)
		}
		if found.Recommendation == "" && want.Recommendation != "" {
			t.Errorf("%s: no recommendation was read back", want.Title)
		}
	}
}

// The draft goes straight into the deck builder. This is the join between the
// two halves of the feature, and the one that silently produces an empty deck
// if the areas or the severities come back in a shape the builder does not
// recognise.
func TestClosureImportDraftBuildsADeck(t *testing.T) {
	docx, _, err := mergeMCollaboratorDocx(rowsConfig())
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	draft, err := closureDraftFromDOCX(docx)
	if err != nil {
		t.Fatalf("closure import failed: %v", err)
	}

	cfg := draft.Config
	normalizeReportAreas(&cfg)
	deck, notes, err := buildClosureDeck(cfg)
	if err != nil {
		t.Fatalf("the imported draft would not build a deck: %v", err)
	}
	if len(deck) == 0 {
		t.Fatal("the deck is empty")
	}
	// Nothing was uploaded, so no finding has a scenario slide - and the page
	// has to be able to say which ones, finding by finding.
	if len(notes.FindingsWithoutProof) != len(cfg.Findings) {
		t.Errorf("%d findings reported as having no proof, want all %d - "+
			"a report read off disk carries no screenshots",
			len(notes.FindingsWithoutProof), len(cfg.Findings))
	}
}

// A screenshot uploaded on the preview screen reaches the scenario slide,
// whether the browser sent a bare base64 payload or a full data URI.
func TestClosureImportAttachesUploadedScreenshots(t *testing.T) {
	cfg := ReportConfig{
		CompanyName: "Acme Test Corp",
		Areas:       []ReportArea{{Code: "WPT", Scope: "https://portal.acme.test"}},
		Findings: []ReportFinding{
			{Title: "Reflected XSS", Severity: "high", Area: "WPT", Description: "q is echoed.",
				POCUploads: []UploadedPOCImage{{Filename: "xss.png", Data: base64PNG(t)}}},
			{Title: "Missing security headers", Severity: "low", Area: "WPT", Description: "No CSP.",
				POCUploads: []UploadedPOCImage{{Filename: "headers.png", Data: "data:image/png;base64," + base64PNG(t)}}},
			{Title: "Verbose error page", Severity: "low", Area: "WPT", Description: "Stack traces.",
				POCUploads: []UploadedPOCImage{{Filename: "broken.png", Data: "not base64 at all"}}},
		},
	}

	bad := attachUploadedPOCImages(&cfg)
	if len(cfg.Findings[0].POCImages) != 1 || len(cfg.Findings[1].POCImages) != 1 {
		t.Fatalf("uploads did not become images: %d and %d",
			len(cfg.Findings[0].POCImages), len(cfg.Findings[1].POCImages))
	}
	if len(cfg.Findings[2].POCImages) != 0 {
		t.Error("an undecodable upload was embedded anyway")
	}
	if len(bad) != 1 || !strings.Contains(bad[0], "broken.png") {
		t.Errorf("the unreadable screenshot was not reported: %v", bad)
	}
	for i := range cfg.Findings {
		if cfg.Findings[i].POCUploads != nil {
			t.Errorf("%s: the base64 payload is still being carried after decoding", cfg.Findings[i].Title)
		}
	}

	_, notes, err := buildClosureDeck(cfg)
	if err != nil {
		t.Fatalf("deck build failed: %v", err)
	}
	// The two that decoded get a scenario slide; the third is named as having
	// no proof rather than quietly leaving the deck a slide short.
	if len(notes.FindingsWithoutProof) != 1 ||
		!strings.Contains(notes.FindingsWithoutProof[0], "Verbose error page") {
		t.Errorf("findings without proof = %v, want only the one whose upload failed", notes.FindingsWithoutProof)
	}
}

// A report that never went through this app - headings the extractor has to
// guess at, no naming convention list - still yields what the title slide
// needs, and says what it could not find rather than inventing it.
func TestClosureImportNamesWhatItCouldNotRead(t *testing.T) {
	meta := readReportMeta([]string{
		"Some Client Ltd - Vulnerability Assessment & Penetration Testing (VAPT)",
		"Prepared by the security team",
		"The assessment was carried out during the period from 3rd March 2026 to 14th March 2026.",
	})
	if meta.Company != "Some Client Ltd" {
		t.Errorf("company = %q, want it read off the cover title", meta.Company)
	}
	if meta.Start != "3rd March 2026" || meta.End != "14th March 2026" {
		t.Errorf("period = %q to %q, want 3rd March 2026 to 14th March 2026", meta.Start, meta.End)
	}
	if meta.Ref != "" || meta.Date != "" {
		t.Errorf("a reference (%q) and a date (%q) were invented out of a document that gives neither", meta.Ref, meta.Date)
	}

	cfg := ReportConfig{CompanyName: meta.Company, AssessmentStart: meta.Start, AssessmentEnd: meta.End}
	missing := missingDeckDetails(cfg)
	if len(missing) != 2 {
		t.Fatalf("missing = %v, want the report date and the assessment areas", missing)
	}
}

// The reference number and date are read from the cover table by position, not
// by shape: a version label like "1.0" sits in the same cell on some reports.
func TestClosureImportReadsTheCoverTableByPosition(t *testing.T) {
	items := []docxItem{{IsTable: true, Rows: [][]string{
		{"", "Version", "Date", "Authors", "Approver", ""},
		{"", "1.0", "4th April 2026", "Jane Analyst\nCybersecurity Expert", "John Approver\nVP, Operations", ""},
	}}}
	meta := &reportMeta{}
	meta.readCoverTable(items)
	if meta.Ref != "1.0" || meta.Date != "4th April 2026" {
		t.Errorf("reference = %q, date = %q; want them taken from under their own column headings", meta.Ref, meta.Date)
	}
}

// A PDF has no table cells. Every row of the cover table comes back as one line
// with the columns run together, and a reference number too wide for its column
// is broken across two of them. These lines are what the app's own report
// actually produces through GetTextByRow.
func TestClosureImportReadsAPDFCoverRow(t *testing.T) {
	meta := &reportMeta{}
	meta.readFlatCover([]string{
		"IT SECURITY CONSULTANCY",
		"Acme Test Bank – Vulnerability Assessment &",
		"Penetration Testing (VAPT)",
		"Version Date Authors Approver",
		"TEST-REP-001-00000-6th September 2026 Emmanuel Addo Kissi Jamal Mekdachi",
		"01",
		"Lead Penetration Tester VP, Operations",
	})
	if meta.Ref != "TEST-REP-001-00000-01" {
		t.Errorf("reference = %q, want the two halves of the wrapped column joined", meta.Ref)
	}
	if meta.Date != "6th September 2026" {
		t.Errorf("date = %q, want it picked out of the run-together row by its shape", meta.Date)
	}
}

// Where the cover gives nothing, the running footer prints the reference on
// every page.
func TestClosureImportFallsBackToTheFooterReference(t *testing.T) {
	meta := &reportMeta{}
	meta.readFlatCover([]string{
		"Cyberteq Falcon Ltd. VAPT Report – Acme Test Bank Page 6",
		"All rights reserved Ref: TEST-REP-001-00000-01",
	})
	if meta.Ref != "TEST-REP-001-00000-01" {
		t.Errorf("reference = %q, want it read out of the footer", meta.Ref)
	}
}

// The PDF's scope table: the activity and its detail on one row, wrapping onto
// the rows below, with the table's heading repeating and the running footer
// landing in the middle of it at a page break.
func TestClosureImportReadsAPDFScopeTable(t *testing.T) {
	meta := &reportMeta{}
	meta.readFlatScope([]string{
		// The table of contents carries a Scope heading of its own, with
		// section titles under it and no scope attached to any of them.
		"2.3 Scope ................................................................. 6",
		"2.4 Out of Scope ......................................................... 7",
		"3.3 Internal Penetration Testing ......................................... 14",
		"3.6 Configuration Files Review ........................................... 29",
		// The section itself.
		"2.3 Scope",
		"Security testing was conducted during the period from 17th August 2026 to 28th",
		"August 2026. The scope of the security testing is detailed hereunder:",
		"Activity Details",
		"Internal Penetration Testing 10.20.0.0/22 - 148 internal hosts",
		"across HQ and the DR site",
		"Cyberteq Falcon Ltd. VAPT Report – Acme Test Bank Page 6",
		"All rights reserved Ref: TEST-REP-001-00000-01",
		"Activity Details",
		"Configuration Files Review 3 Fortigate firewalls, 6 Cisco",
		"switches, 2 core routers",
		"The external activities were performed remotely.",
		"2.4 Out of Scope",
		"Denial of service testing",
	})

	want := map[string]string{
		"IPT": "10.20.0.0/22 - 148 internal hosts across HQ and the DR site",
		"CFG": "3 Fortigate firewalls, 6 Cisco switches, 2 core routers",
	}
	if len(meta.Areas) != len(want) {
		t.Fatalf("read %d areas, want %d: %+v", len(meta.Areas), len(want), meta.Areas)
	}
	for _, a := range meta.Areas {
		if a.Scope != want[a.Code] {
			t.Errorf("%s scope = %q, want %q", a.Code, a.Scope, want[a.Code])
		}
	}
	if meta.Areas[0].Code != "IPT" {
		t.Errorf("areas came back as %q first, want template order", meta.Areas[0].Code)
	}
}

// base64PNG is a 1x1 PNG, the smallest thing the deck will accept as a proof.
func base64PNG(t *testing.T) string {
	t.Helper()
	return "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
}
