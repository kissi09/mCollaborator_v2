package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
)

func sampleConfig() ReportConfig {
	return ReportConfig{
		CompanyName:     "Acme Test Corp",
		CompanyInitials: "ATC",
		EngagementName:  "VAPT Report",
		RefNumber:       "GH-REP-099-99999-01",
		ReportDate:      "12th August 2026",
		AssessmentStart: "1st August 2026",
		AssessmentEnd:   "10th August 2026",
		TesterName:      "Jane Analyst",
		ApproverName:    "John Approver",
		ApproverTitle:   "Head of Delivery",
		VersionLabel:    "1.0",
		Areas: []ReportArea{
			{Code: "WPT", Scope: "https://portal.acme.test (2 roles)"},
			{Code: "ASA", Scope: "/api/v1 REST API, 34 endpoints"},
			{Code: "NAR", Scope: "HQ and DR network topology"},
		},
		Tools: []string{"Burp Suite Pro", "Nmap", "Nessus", "Kali Linux"},
		Findings: []ReportFinding{
			{Title: "SQL Injection", Severity: "critical", Description: "desc-sqli", Recommendation: "Parameterise all queries.", Area: "WPT", AffectedSystem: "portal.acme.test", CVSSVector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"},
			{Title: "Reflected XSS", Severity: "high", Description: "desc-xss", Recommendation: "Encode output.", Area: "WPT"},
			{Title: "Missing rate limiting", Severity: "medium", Description: "desc-rate", Recommendation: "Throttle requests.", Area: "ASA"},
			{Title: "Flat network segmentation", Severity: "high", Description: "desc-seg", Recommendation: "Introduce VLANs.", Area: "NAR"},
			{Title: "Verbose banner", Severity: "low", Description: "desc-banner", Recommendation: "Suppress banners.", Area: "NAR"},
		},
	}
}

// readDocxParts renders the config and returns the package parts by name.
func readDocxParts(t *testing.T, config ReportConfig) map[string]string {
	t.Helper()
	out, _, err := mergeMCollaboratorDocx(config)
	if err != nil {
		t.Fatalf("mergeMCollaboratorDocx failed: %v", err)
	}
	if path := os.Getenv("DOCX_WRITE_SAMPLE"); path != "" {
		if err := os.WriteFile(path, out, 0644); err != nil {
			t.Fatalf("write sample: %v", err)
		}
	}
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("output is not a valid zip: %v", err)
	}
	parts := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		parts[strings.ReplaceAll(f.Name, "\\", "/")] = string(b)
	}
	return parts
}

// TestRenderedPartsAreWellFormedXML guards the string surgery: every edited part
// must still parse, otherwise Word refuses to open the report.
func TestRenderedPartsAreWellFormedXML(t *testing.T) {
	parts := readDocxParts(t, sampleConfig())
	for name, body := range parts {
		if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".rels") {
			continue
		}
		dec := xml.NewDecoder(strings.NewReader(body))
		for {
			_, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s is not well-formed XML: %v", name, err)
			}
		}
	}
}

func TestScalarPlaceholdersAreFilled(t *testing.T) {
	parts := readDocxParts(t, sampleConfig())
	doc := parts["word/document.xml"]

	for _, want := range []string{
		"Acme Test Corp", "ATC", "GH-REP-099-99999-01", "12th August 2026",
		"Jane Analyst", "John Approver", "Head of Delivery",
		"1st August 2026 to 10th August 2026",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("expected %q in document.xml", want)
		}
	}

	// No square-bracket placeholder may survive anywhere in the package.
	leftover := regexp.MustCompile(`\[(Company Name|Company Initials|Reference Number|Date|Tester Name|Approver|Role|Assessment Start|Recommendation Header|number|Number|total number of issues|vulnerability \d|test account|credentials|[A-Z]+ Vulnerability \d+)\]`)
	for _, name := range []string{"word/document.xml", "word/footer4.xml", "word/footer5.xml", "word/footer7.xml"} {
		if m := leftover.FindString(parts[name]); m != "" {
			t.Errorf("%s: unresolved placeholder %s", name, m)
		}
	}
}

// bodyTextWithoutTOC is the report's readable text minus the cached table of
// contents, which is a field Word rebuilds on open (settings.xml asks it to) and
// so still lists the template's original sections until then.
func bodyTextWithoutTOC(doc string) string {
	var b strings.Builder
	for _, c := range bodyChildren(doc) {
		if strings.HasPrefix(c.Style, "TOC") {
			continue
		}
		b.WriteString(c.Text)
		b.WriteString("\n")
	}
	return b.String()
}

func TestOnlySelectedAreasAreReported(t *testing.T) {
	parts := readDocxParts(t, sampleConfig())
	text := bodyTextWithoutTOC(parts["word/document.xml"])

	// Selected areas keep their naming-convention line and their section.
	for _, want := range []string{
		"WPT – Web Application Penetration Testing",
		"ASA - API Security Assessment",
		"NAR – Network Architecture Review",
		"Web Application Penetration Testing",
		"API Security Assessment",
		"Network Architecture Review",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("expected selected area entry %q to survive", want)
		}
	}
	// Unselected areas lose their naming-convention line, their scope row, their
	// chapter 3 section and their methodology subsection.
	for _, gone := range []string{
		"Internal Cloud Penetration Testing",
		"Wireless Network Penetration Testing",
		"Wireless Network Assessment",
		"Active Directory Testing",
		"Configuration Files Review",
		"External Penetration Testing",
	} {
		if strings.Contains(text, gone) {
			t.Errorf("unselected area %q should not appear anywhere in the report body", gone)
		}
	}
}

func TestChartsMatchTheFindings(t *testing.T) {
	parts := readDocxParts(t, sampleConfig())

	sev := parts["word/charts/chartEx1.xml"]
	// 1 critical, 2 high, 1 medium, 1 low, 0 informational.
	for _, want := range []string{
		`<cx:pt idx="0">1</cx:pt>`, `<cx:pt idx="1">2</cx:pt>`,
		`<cx:pt idx="2">1</cx:pt>`, `<cx:pt idx="3">1</cx:pt>`,
		`<cx:pt idx="4">0</cx:pt>`,
	} {
		if !strings.Contains(sev, want) {
			t.Errorf("severity chart missing %s", want)
		}
	}

	area := parts["word/charts/chartEx2.xml"]
	for _, want := range []string{"Web Apps", "APIs", "Network Architecture"} {
		if !strings.Contains(area, want) {
			t.Errorf("area chart missing category %q", want)
		}
	}
	for _, gone := range []string{"Wireless", "Config Review", "Internal Cloud"} {
		if strings.Contains(area, gone) {
			t.Errorf("area chart should not carry unselected category %q", gone)
		}
	}
	// WPT 2, ASA 1, NAR 2.
	if !strings.Contains(area, `<cx:pt idx="0">2</cx:pt>`) ||
		!strings.Contains(area, `<cx:pt idx="1">1</cx:pt>`) ||
		!strings.Contains(area, `<cx:pt idx="2">2</cx:pt>`) {
		t.Errorf("area chart values do not match the findings: %s", chartValues(area))
	}
}

func chartValues(chart string) string {
	re := regexp.MustCompile(`(?s)<cx:numDim type="val">.*?</cx:numDim>`)
	return re.FindString(chart)
}

func TestFindingContentAndVulnIDs(t *testing.T) {
	parts := readDocxParts(t, sampleConfig())
	text := elemText(parts["word/document.xml"])

	for _, want := range []string{
		"SQL Injection", "desc-sqli", "Parameterise all queries.",
		"Reflected XSS", "Missing rate limiting", "Flat network segmentation",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("expected finding content %q in the report", want)
		}
	}

	// Vulnerability ids are numbered per area and per report.
	for _, want := range []string{"ATC_REC1_WPT1", "ATC_REC2_WPT2", "ATC_REC3_ASA1", "ATC_REC4_NAR1", "ATC_REC5_NAR2"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected vulnerability id %q", want)
		}
	}
	if !strings.Contains(text, "5 issues") {
		t.Errorf("expected the executive summary to report 5 issues")
	}
}

func TestAppendixOmittedWithoutTestAccounts(t *testing.T) {
	parts := readDocxParts(t, sampleConfig())
	if strings.Contains(elemText(parts["word/document.xml"]), "The following test accounts were created") {
		t.Errorf("appendix should be dropped when no test accounts are supplied")
	}

	config := sampleConfig()
	config.TestAccountsCreated = []TestAccount{{Account: "pentest01", Credentials: "Sup3rSecret!"}}
	parts = readDocxParts(t, config)
	text := elemText(parts["word/document.xml"])
	if !strings.Contains(text, "pentest01") || !strings.Contains(text, "Sup3rSecret!") {
		t.Errorf("appendix should list the supplied test account")
	}
	if strings.Contains(text, "The following test accounts were unauthorizedly access") {
		t.Errorf("the pre-existing accounts table should be dropped when that list is empty")
	}
}

func TestToolsListIsReplaced(t *testing.T) {
	parts := readDocxParts(t, sampleConfig())
	text := elemText(parts["word/document.xml"])
	for _, want := range []string{"Burp Suite Pro", "Nmap", "Nessus", "Kali Linux"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected tool %q in the tools table", want)
		}
	}
	if strings.Contains(text, "AirCrack-Ng") {
		t.Errorf("template's example tools should be replaced by the supplied list")
	}
}

func TestLogoLandsInTheHeader(t *testing.T) {
	config := sampleConfig()
	config.CompanyLogo = tinyPNGDataURI()
	parts := readDocxParts(t, config)

	if _, ok := parts[logoMediaPart]; !ok {
		t.Fatalf("uploaded logo was not added to the package")
	}
	found := false
	for name, body := range parts {
		if isHeaderPart(name) && strings.Contains(body, logoRelID) {
			found = true
			if strings.Contains(elemText(body), logoSlotMarker) {
				t.Errorf("%s still shows the placeholder rectangle", name)
			}
			rels, ok := parts["word/_rels/"+strings.TrimPrefix(name, "word/")+".rels"]
			if !ok || !strings.Contains(rels, logoRelID) {
				t.Errorf("%s references the logo without a matching relationship", name)
			}
		}
	}
	if !found {
		t.Errorf("no header referenced the uploaded logo")
	}

	// The logo must run to the end of the report, including the header the Tools
	// and Licenses and Appendix pages use, which reserves no slot of its own.
	withLogo := 0
	for name, body := range parts {
		if isHeaderPart(name) && strings.Contains(body, logoRelID) {
			withLogo++
		}
	}
	if withLogo < 3 {
		t.Errorf("customer logo reached only %d running headers, want all 3", withLogo)
	}
}

// TestLogoLandsOnTheTitlePage pins the cover: the banner reserves a box for the
// customer's logo, and it has to float over the banner rather than join the text
// flow, which would push the title block onto a second page.
func TestLogoLandsOnTheTitlePage(t *testing.T) {
	config := sampleConfig()
	config.CompanyLogo = tinyPNGDataURI()
	parts := readDocxParts(t, config)

	doc := parts["word/document.xml"]
	i := strings.Index(doc, `name="Customer Logo"`)
	if i < 0 {
		t.Fatalf("the title page carries no customer logo")
	}
	drawing := doc[:i]
	anchor := strings.LastIndex(drawing, "<wp:anchor ")
	if anchor < 0 || strings.LastIndex(drawing, "<wp:inline ") > anchor {
		t.Errorf("the cover logo is in the text flow; it must be anchored to the page")
	}
	if !strings.Contains(doc[anchor:i], `relativeFrom="page"`) {
		t.Errorf("the cover logo is not positioned relative to the page")
	}
	if !strings.Contains(doc, `r:embed="`+logoRelID+`"`) {
		t.Errorf("the cover logo does not point at the uploaded image")
	}
	if rels := parts["word/_rels/document.xml.rels"]; !strings.Contains(rels, logoRelID) {
		t.Errorf("the document references the logo without a matching relationship")
	}

	// The banner box is 2.02in x 1.73in and the logo is a 4x2 PNG, so it fits
	// the full width of the box and must not have been stretched to its height.
	if !strings.Contains(doc[anchor:i], `cx="1849120" cy="924560"`) {
		t.Errorf("the cover logo was not scaled to fit the banner box; got %s",
			doc[anchor:i])
	}
}

// TestNoCoverLogoWhenNoLogoUploaded pins the other half: with nothing uploaded
// the cover prints exactly as the template draws it.
func TestNoCoverLogoWhenNoLogoUploaded(t *testing.T) {
	parts := readDocxParts(t, sampleConfig()) // sampleConfig uploads no logo
	if strings.Contains(parts["word/document.xml"], `name="Customer Logo"`) {
		t.Errorf("the title page shows a customer logo that was never uploaded")
	}
	if _, ok := parts[logoMediaPart]; ok {
		t.Errorf("an empty logo part was added to the package")
	}
}

// TestLandscapeSectionSurvives pins the page setup. The vulnerability register
// through the last assessment section is landscape in the template, and that
// section's properties also bind those pages to the header carrying the customer
// logo - so losing them while rebuilding chapter 3 costs orientation, header and
// logo at once.
func TestLandscapeSectionSurvives(t *testing.T) {
	parts := readDocxParts(t, sampleConfig())
	doc := parts["word/document.xml"]

	landscape := strings.Count(doc, `w:orient="landscape"`)
	if landscape != 1 {
		t.Errorf("expected exactly 1 landscape section, found %d", landscape)
	}
	if got, want := strings.Count(doc, "<w:sectPr"), 5; got != want {
		t.Errorf("expected %d section breaks (as in the template), found %d", want, got)
	}
	// The landscape section must still reference header6, the running header
	// that carries the customer-logo slot.
	sects := childElems(doc, "w:sectPr")
	var landscapeSect string
	for _, s := range sects {
		if strings.Contains(doc[s.Start:s.End], `w:orient="landscape"`) {
			landscapeSect = doc[s.Start:s.End]
		}
	}
	if landscapeSect == "" {
		t.Fatal("no landscape section found")
	}
	if !strings.Contains(landscapeSect, "<w:headerReference") {
		t.Errorf("landscape section lost its header reference: %s", landscapeSect)
	}
}

func TestScopeTableHeaderIsSplitIntoColumns(t *testing.T) {
	parts := readDocxParts(t, sampleConfig())
	doc := parts["word/document.xml"]
	if strings.Contains(elemText(doc), "Activity Details") {
		t.Errorf("scope header still crams both column names into one cell")
	}
	text := bodyTextWithoutTOC(doc)
	if !strings.Contains(text, "Activity") || !strings.Contains(text, "Details") {
		t.Errorf("scope header lost its column names")
	}
}

func TestFooterCompanyNameResolves(t *testing.T) {
	parts := readDocxParts(t, sampleConfig())

	// The cached field result carries the name for readers that never refresh.
	for _, name := range []string{"word/footer4.xml", "word/footer5.xml", "word/footer7.xml"} {
		if !strings.Contains(elemText(parts[name]), "Acme Test Corp") {
			t.Errorf("%s: footer is missing the company name", name)
		}
	}

	// And the bookmark the REF field points at now wraps the name itself, so a
	// field refresh keeps it rather than blanking it.
	doc := parts["word/document.xml"]
	i := strings.Index(doc, `w:name="ClientCompanyName"`)
	if i < 0 {
		t.Fatal("ClientCompanyName bookmark is missing")
	}
	start := strings.Index(doc[i:], "/>")
	if start < 0 {
		t.Fatal("malformed bookmarkStart")
	}
	rest := doc[i+start+2:]
	end := strings.Index(rest, "<w:bookmarkEnd")
	if end < 0 {
		t.Fatal("bookmarkEnd is missing")
	}
	if got := strings.TrimSpace(elemText(rest[:end])); got != "Acme Test Corp" {
		t.Errorf("ClientCompanyName bookmark wraps %q, want the company name", got)
	}
}

// TestFootersCarryNoPreviousClient guards against the template's leftovers: three
// of its footer parts hardcode an earlier engagement's reference number, two of
// them name that client, and both are visible in a delivered report.
func TestFootersCarryNoPreviousClient(t *testing.T) {
	parts := readDocxParts(t, sampleConfig())
	for name, body := range parts {
		if !isFooterPart(name) {
			continue
		}
		text := elemText(body)
		for _, stale := range []string{"MENA-035-22007-01", "GH-REP-035-26040-01", "APIC"} {
			if strings.Contains(text, stale) {
				t.Errorf("%s still carries the template's previous client detail %q", name, stale)
			}
		}
		if strings.Contains(text, "Ref:") && !strings.Contains(text, "GH-REP-099-99999-01") {
			t.Errorf("%s prints a reference that is not this engagement's: %q", name, text)
		}
	}
}

func TestCoverRulesRunFullWidth(t *testing.T) {
	parts := readDocxParts(t, sampleConfig())
	doc := parts["word/document.xml"]

	tbls := childElems(doc, "w:tbl")
	if len(tbls) == 0 {
		t.Fatal("no tables in the document")
	}
	tbl := doc[tbls[0].Start:tbls[0].End]
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
		t.Fatal("cover version table not found")
	}
	for ri := header + 1; ri < len(rows); ri++ {
		row := tbl[rows[ri].Start:rows[ri].End]
		if strings.TrimSpace(elemText(row)) == "" {
			continue // trailing spacer row stays borderless
		}
		for ci, c := range rowCells(row) {
			if !strings.Contains(row[c.Start:c.End], `<w:bottom w:val="single"`) {
				t.Errorf("cover table row %d cell %d has no bottom rule, so the line breaks", ri, ci)
			}
		}
	}
}

// TestWizardPayloadContract decodes the exact JSON body the report wizard posts
// to /reports/export. It fails if a field is renamed on either side, which would
// otherwise show up only as a silently blank placeholder in a delivered report.
func TestWizardPayloadContract(t *testing.T) {
	const body = `{
      "company_name": "BestPoint Savings & Loans",
      "company_initials": "BSL",
      "company_logo": "",
      "engagement_name": "VAPT Report",
      "ref_number": "GH-REP-035-26059-01",
      "report_date": "12th August 2026",
      "assessment_start": "17th June 2026",
      "assessment_end": "24th June 2026",
      "tester_name": "Jane Analyst",
      "approver_name": "Jamal Mekdachi",
      "approver_title": "VP, Operations",
      "version_label": "1.0",
      "areas": [
        {"code": "WPT", "scope": "https://portal.example.com (2 roles)"},
        {"code": "ASA", "scope": "/api/v1 REST API, 34 endpoints"}
      ],
      "sections": ["WPT", "ASA"],
      "out_of_scope": ["Physical security"],
      "findings": [
        {
          "title": "SQL Injection", "description": "d", "impact": "i",
          "cvss_vector": "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", "cvss_score": "9.8",
          "severity": "critical", "affected_system": "portal", "poc": "",
          "recommendation": "Parameterise queries.", "recommendation_header": "Parameterise queries",
          "attack_vector": "Network", "category": "web", "area": "WPT", "exposure": "Web",
          "vuln_id": "", "poc_evidence_ids": []
        }
      ],
      "tools": ["Burp Suite Pro", "Nmap"],
      "test_accounts_existing": [{"account": "svc_backup", "credentials": "Winter2026!"}],
      "test_accounts_created": [{"account": "pentest01", "credentials": "Sup3rSecret!"}],
      "sync_onedrive": true,
      "onedrive_folder": "VAPT Reports/BestPoint"
    }`

	var config ReportConfig
	if err := json.Unmarshal([]byte(body), &config); err != nil {
		t.Fatalf("wizard payload does not decode: %v", err)
	}

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"company name", config.CompanyName, "BestPoint Savings & Loans"},
		{"company initials", config.CompanyInitials, "BSL"},
		{"report date", config.ReportDate, "12th August 2026"},
		{"approver role", config.ApproverTitle, "VP, Operations"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %q want %q", c.name, c.got, c.want)
		}
	}
	if len(config.Areas) != 2 || config.Areas[0].Code != "WPT" || config.Areas[0].Scope == "" {
		t.Errorf("areas did not decode: %+v", config.Areas)
	}
	if len(config.Findings) != 1 || config.Findings[0].Area != "WPT" ||
		config.Findings[0].RecommendationHeader == "" || config.Findings[0].AttackVector == "" {
		t.Errorf("finding did not decode: %+v", config.Findings)
	}
	if len(config.TestAccountsExisting) != 1 || config.TestAccountsCreated[0].Account != "pentest01" {
		t.Errorf("test accounts did not decode: %+v %+v", config.TestAccountsExisting, config.TestAccountsCreated)
	}
	// The sync fields were renamed once already; unknown JSON keys decode
	// silently, so the contract has to be asserted rather than assumed.
	if !config.SyncOneDrive || config.OneDriveFolder != "VAPT Reports/BestPoint" {
		t.Errorf("OneDrive sync fields did not decode: %v %q", config.SyncOneDrive, config.OneDriveFolder)
	}

	parts := readDocxParts(t, config)
	text := bodyTextWithoutTOC(parts["word/document.xml"])
	for _, want := range []string{
		"BestPoint Savings & Loans", "BSL_REC1_WPT1", "Parameterise queries",
		"svc_backup", "pentest01", "API Security Assessment",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in the rendered report", want)
		}
	}
}

// TestLogoSlotRemovedWhenNoLogoUploaded pins the no-logo case: the template's
// slot is an outlined box instructing whoever fills it in, and leaving it in
// place prints that instruction on every page of the delivered report.
func TestLogoSlotRemovedWhenNoLogoUploaded(t *testing.T) {
	parts := readDocxParts(t, sampleConfig()) // sampleConfig uploads no logo
	for name, body := range parts {
		if !isHeaderPart(name) {
			continue
		}
		if strings.Contains(elemText(body), logoSlotMarker) {
			t.Errorf("%s still prints the empty customer-logo placeholder", name)
		}
	}
}

// tinyPNGDataURI is a 4x2 PNG, enough to exercise decode and header scaling.
func tinyPNGDataURI() string {
	return "data:image/png;base64," +
		"iVBORw0KGgoAAAANSUhEUgAAAAQAAAACCAIAAADwyuo0AAAAFUlEQVR4nGL5X8kAB0wMSAAQAAD//yTbAX8dRNStAAAAAElFTkSuQmCC"
}

// templateDocumentXML returns the untouched template's document.xml, for
// assertions that need a baseline of what the template already contains.
func templateDocumentXML(t *testing.T) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(mcollabTemplateDocx), int64(len(mcollabTemplateDocx)))
	if err != nil {
		t.Fatalf("template is not a valid zip: %v", err)
	}
	for _, f := range zr.File {
		if strings.ReplaceAll(f.Name, "\\", "/") != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open template document.xml: %v", err)
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read template document.xml: %v", err)
		}
		return string(b)
	}
	t.Fatal("template has no word/document.xml")
	return ""
}

// TestCriticalityIsColouredTextNotAFilledCell pins the register and the Rating
// row to colouring the word itself, in the exact colours the template's
// criticality-criteria legend fills its bands with (docs/colors.PNG).
//
// Those same hexes therefore appear as fills in the legend table the template
// ships, so a fill can only be blamed on the renderer if there are *more* of
// them in the output than the template already had.
func TestCriticalityIsColouredTextNotAFilledCell(t *testing.T) {
	parts := readDocxParts(t, sampleConfig())
	doc := parts["word/document.xml"]
	tmpl := templateDocumentXML(t)

	for _, sev := range []struct{ label, hex string }{
		{"Critical", "FF0000"},
		{"High", "F68831"},
		{"Medium", "FFC000"},
		{"Low", "92D050"},
	} {
		want := `<w:color w:val="` + sev.hex + `"/>`
		if !strings.Contains(doc, want) {
			t.Errorf("no %s text is set in %s", sev.label, sev.hex)
		}
		fill := `w:fill="` + sev.hex + `"`
		if got, base := strings.Count(doc, fill), strings.Count(tmpl, fill); got > base {
			t.Errorf("%d cells are filled %s against the template's %d; only the %s text should carry the colour",
				got, sev.hex, base, sev.label)
		}
	}
	// White-on-colour was the old badge treatment and has no place now that the
	// cell behind the word is the page.
	if strings.Contains(doc, `<w:b/><w:color w:val="FFFFFF"/>`) {
		t.Error("a criticality word is still being knocked out in white")
	}
}

// TestRecommendationBodyIsNotBold pins the elaboration under a finding's
// "<VulnID> - <Header>" line to plain text. The naming line stays bold; the
// paragraph explaining it does not. Bold has to be switched off explicitly -
// the cell's <w:cnfStyle> bolds any run that stays silent about it.
func TestRecommendationBodyIsNotBold(t *testing.T) {
	config := sampleConfig()
	config.Findings[0].Recommendation = "Parameterise every query and validate input."
	parts := readDocxParts(t, config)
	doc := parts["word/document.xml"]

	body := xmlEscape(config.Findings[0].Recommendation)
	i := strings.Index(doc, body)
	if i < 0 {
		t.Fatalf("recommendation body %q is not in the document", body)
	}
	run := doc[strings.LastIndex(doc[:i], "<w:r>"):i]
	if !strings.Contains(run, `<w:b w:val="0"/>`) {
		t.Errorf("recommendation body run does not switch bold off: %s", run)
	}
	if strings.Contains(run, `<w:b/>`) {
		t.Errorf("recommendation body run is still bold: %s", run)
	}

	// The naming line above it is still bold - it is the template's own run,
	// untouched.
	head := config.Findings[0].Title
	if !strings.Contains(doc, xmlEscape(head)) {
		t.Errorf("finding title %q is missing", head)
	}
}

// TestFooterItemsAreSpacedApart is the regression for a delivered report whose
// portrait footer read "Cyberteq Falcon Ltd.VAPT Report – <client>", the two
// items run together because the centred one was too wide for the page.
func TestFooterItemsAreSpacedApart(t *testing.T) {
	config := sampleConfig()
	config.CompanyName = "BestPoint Savings & Loans"
	parts := readDocxParts(t, config)

	checked := 0
	for name, body := range parts {
		if !isFooterPart(name) {
			continue
		}
		for _, p := range childElems(body, "w:p") {
			para := body[p.Start:p.End]
			stops := paraTabStops(para)
			segments := footerSegments(para)
			if len(stops) == 0 || len(segments) < 2 {
				continue
			}
			checked++
			if !footerLineFits(segments, stops, maxParaSz(para)) {
				t.Errorf("%s: footer line %q still crowds its items together",
					name, strings.TrimSpace(elemText(para)))
			}
		}
	}
	if checked == 0 {
		t.Fatal("no tabbed footer lines were examined - the test is not testing anything")
	}
}

// TestLandscapeFooterKeepsItsSize guards against over-correcting: the landscape
// page is wide enough for the template's own 11pt, and shrinking it there would
// leave the report set in two different footer sizes for no reason.
func TestLandscapeFooterKeepsItsSize(t *testing.T) {
	parts := readDocxParts(t, sampleConfig())
	footer, ok := parts["word/footer5.xml"]
	if !ok {
		t.Fatal("the landscape section's footer part is missing from the package")
	}
	if maxParaSz(footer) != 22 {
		t.Errorf("landscape footer was resized to %d half-points; it fits at 22",
			maxParaSz(footer))
	}
}

func TestReportBaseName(t *testing.T) {
	cases := []struct {
		name   string
		config ReportConfig
		want   string
	}{
		{
			"reference and company",
			ReportConfig{RefNumber: "GH-REP-035-26040-01", CompanyName: "Axxend Corporation"},
			"GH-REP-035-26040-01 - Axxend Corporation - VAPT",
		},
		{
			"an ampersand survives, a slash does not",
			ReportConfig{RefNumber: "GH-REP-1", CompanyName: "BestPoint Savings & Loans / Ghana"},
			"GH-REP-1 - BestPoint Savings & Loans Ghana - VAPT",
		},
		{
			"no reference yet",
			ReportConfig{CompanyName: "Acme Test Corp"},
			"Acme Test Corp - VAPT",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reportBaseName(tc.config); got != tc.want {
				t.Errorf("reportBaseName = %q, want %q", got, tc.want)
			}
		})
	}
}

// checklistStatuses returns every Test Type row of the rendered report as
// name -> status, across all the area checklists it still carries.
func checklistStatuses(t *testing.T, doc string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, tb := range childElems(doc, "w:tbl") {
		frag := doc[tb.Start:tb.End]
		if !isTestTypeTable(frag) {
			continue
		}
		for _, r := range parseTestTable(frag).rows {
			out[r.name] = strings.TrimSpace(elemText(frag[r.cell.Start:r.cell.End]))
		}
	}
	return out
}

// checklistConfig is one WPT/NAR engagement whose findings each point at a
// different row, plus one that points at none.
func checklistConfig() ReportConfig {
	config := sampleConfig()
	config.Areas = []ReportArea{{Code: "WPT", Scope: "portal"}, {Code: "NAR", Scope: "topology"}}
	config.Findings = []ReportFinding{
		{Title: "SQL Injection in login module", Severity: "critical", Description: "d", Recommendation: "Parameterise queries.", Area: "WPT"},
		{Title: "Reflected XSS in search parameter", Severity: "high", Description: "d", Recommendation: "Encode output.", Area: "WPT"},
		{Title: "Insecure direct object reference on invoices", Severity: "medium", Description: "d", Recommendation: "Check ownership.", Area: "WPT"},
		{Title: "Flat network segmentation between VLANs", Severity: "high", Description: "d", Recommendation: "Introduce VLANs.", Area: "NAR"},
	}
	return config
}

// TestChecklistCarriesNoTemplateStatuses is the regression for a delivered
// report that told the client "Audit SSL Services: Issues" about a network
// nobody had audited - the template's checklist shipped with a previous
// engagement's results filled in and nothing rewrote them. With no findings at
// all, every row must read Pass and none of the template's own answers survive.
func TestChecklistCarriesNoTemplateStatuses(t *testing.T) {
	config := checklistConfig()
	config.Findings = nil

	statuses := checklistStatuses(t, readDocxParts(t, config)["word/document.xml"])
	if len(statuses) == 0 {
		t.Fatal("the rendered report carries no Test Type checklist at all")
	}
	for name, status := range statuses {
		if status != statusPass {
			t.Errorf("%q reads %q in a report with no findings; nothing was derived, so it must read Pass",
				name, status)
		}
	}
}

// TestChecklistStatusesComeFromTheFindings pins the derivation, and pins it
// narrowly: one SQL injection must not also condemn the five other injection
// rows sitting beside it, and a reflected XSS must not report a stored one.
func TestChecklistStatusesComeFromTheFindings(t *testing.T) {
	statuses := checklistStatuses(t, readDocxParts(t, checklistConfig())["word/document.xml"])

	want := map[string]string{
		"Testing for SQL Injection":                  statusIssues,
		"Testing for Reflected Cross Site Scripting": statusIssues,
		"Segmentation":                               statusIssues,

		"Testing for NoSQL injection":                    statusPass,
		"Testing for LDAP Injection":                     statusPass,
		"Testing for XPath Injection":                    statusPass,
		"SQL Server Testing":                             statusPass,
		"Testing for Stored Cross Site Scripting":        statusPass,
		"Testing for Cross Site Request Forgery":         statusPass,
		"Secure network transmissions":                   statusPass,
		"Testing for Weak or unenforced username policy": statusPass,
	}
	for name, expect := range want {
		got, ok := statuses[name]
		if !ok {
			t.Errorf("row %q is missing from the rendered checklist", name)
			continue
		}
		if got != expect {
			t.Errorf("row %q reads %q, want %q", name, got, expect)
		}
	}
}

// TestUnmatchedFindingIsReported guards the one case the derivation cannot
// cover: a finding whose area has a checklist that names nothing like it. Those
// rows stay Pass, so the register would report a vulnerability the checklist
// silently does not - the export has to say so.
func TestUnmatchedFindingIsReported(t *testing.T) {
	_, notes, err := mergeMCollaboratorDocx(checklistConfig())
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	if len(notes.UnmatchedFindings) != 1 ||
		notes.UnmatchedFindings[0] != "Insecure direct object reference on invoices" {
		t.Errorf("unmatched findings = %v, want just the IDOR finding", notes.UnmatchedFindings)
	}
}

// TestChecklistMatchingIsPrecise exercises the matcher directly across the
// phrasings the checklists actually use, without rendering a whole report.
func TestChecklistMatchingIsPrecise(t *testing.T) {
	rows := []string{
		"Testing for SQL Injection",
		"Testing for NoSQL injection",
		"Testing for XPath Injection",
		"Testing for Reflected Cross Site Scripting",
		"Testing for Stored Cross Site Scripting",
		"Testing for Cross Site Request Forgery",
		"Test HTTP Strict Transport Security",
		"Test HTTP Methods",
		"Identify Rogue Access Points",
		"Test for Weak WPA2-PSK Passphrases (Dictionary/Brute-force)",
		"Audit Telnet Services",
		"Audit SMTP Services",
		"Network Sniffing",
	}
	parsed := parsedTestTable{}
	for _, name := range rows {
		parsed.rows = append(parsed.rows, testRow{name: name})
	}
	weight := testVocabulary(parsed)

	cases := []struct {
		finding string
		want    []string // the rows that must match, and only these
	}{
		{"SQL Injection in login module", []string{"Testing for SQL Injection"}},
		{"Reflected XSS in search parameter", []string{"Testing for Reflected Cross Site Scripting"}},
		{"CSRF token absent on password change", []string{"Testing for Cross Site Request Forgery"}},
		{"Missing HTTP Strict Transport Security header", []string{"Test HTTP Strict Transport Security"}},
		{"Rogue access point undetected by the WLAN controller", []string{"Identify Rogue Access Points"}},
		{"WPA2-PSK passphrase cracked offline", []string{"Test for Weak WPA2-PSK Passphrases (Dictionary/Brute-force)"}},
		{"Telnet enabled on network switches", []string{"Audit Telnet Services"}},
		{"Insecure direct object reference on invoices", nil},
	}
	for _, tc := range cases {
		t.Run(tc.finding, func(t *testing.T) {
			hay := findingHaystack(ReportFinding{Title: tc.finding})
			var got []string
			for _, r := range parsed.rows {
				if matchesTest(checklistTokens(r.name), weight, hay) {
					got = append(got, r.name)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("matched %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("matched %v, want %v", got, tc.want)
				}
			}
		})
	}
}
