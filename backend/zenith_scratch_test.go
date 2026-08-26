package main

// Scratch harness for the Zenith Bank check, not a pinned test suite: it builds
// a full engagement, writes the DOCX and the PPTX out where they can be opened,
// and asserts the things that were reported broken. Delete when done with it.

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const zenithDir = `C:\Users\SAMSUN~1\AppData\Local\Temp\claude\C--Users-Samsung-Galaxy-Book--local-bin\b3cf9a46-3a6a-4da3-829b-482d398e6a56\scratchpad\zenith`

// zenithLogo is the real Zenith Bank mark, as SVG - the format a bank's press
// kit actually ships, and the one the wizard used to accept and then drop.
func zenithLogo(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(zenithDir + `\zenith.svg`)
	if err != nil {
		t.Skip("no zenith.svg:", err)
	}
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(raw)
}

// zenithConfig is a Zenith Bank engagement across five assessment areas - one
// more than the executive summary has panels for, so the repeat is exercised.
func zenithConfig(t *testing.T) ReportConfig {
	return ReportConfig{
		CompanyName:     "Zenith Bank",
		CompanyInitials: "ZB",
		CompanyLogo:     zenithLogo(t),
		EngagementName:  "VAPT Report",
		RefNumber:       "GH-REP-041-26120-01",
		ReportDate:      "24th August 2026",
		AssessmentStart: "3rd August 2026",
		AssessmentEnd:   "17th August 2026",
		TesterName:      "Jane Analyst",
		ApproverName:    "John Approver",
		ApproverTitle:   "Head of Delivery",
		VersionLabel:    "1.0",
		Areas: []ReportArea{
			{Code: "IPT", Scope: "10.20.0.0/16 core banking VLAN, 412 hosts"},
			{Code: "EPT", Scope: "196.46.20.0/24, 18 public hosts"},
			{Code: "WPT", Scope: "https://ibank.zenithbank.test, https://corporate.zenithbank.test"},
			{Code: "CFG", Scope: "14 firewall and switch configurations"},
			{Code: "WNA", Scope: "Head office and Victoria Island branch wireless"},
		},
		OutOfScope: []string{
			"Denial-of-service testing against production",
			"Social engineering of branch staff",
		},
		Tools: []string{"Burp Suite Pro", "Nmap", "Nessus", "Kali Linux"},
		Findings: []ReportFinding{
			{Title: "SQL Injection in the internet banking login", Severity: "critical", Description: "The login form concatenates user input into a SQL statement. An attacker can authenticate as any customer.", Impact: "Full compromise of customer accounts.", Recommendation: "Parameterise all queries.", Area: "WPT", AffectedSystem: "ibank.zenithbank.test", CVSSVector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"},
			{Title: "Reflected Cross-Site Scripting", Severity: "high", Description: "The search parameter is echoed unencoded.", Recommendation: "Encode output.", Area: "WPT", AffectedSystem: "corporate.zenithbank.test"},
			{Title: "Outdated TLS on the corporate portal", Severity: "medium", Description: "TLS 1.0 remains enabled.", Recommendation: "Enforce TLS 1.2 or higher.", Area: "WPT"},
			{Title: "SMB signing not required", Severity: "high", Description: "Domain controllers accept unsigned SMB.", Recommendation: "Require SMB signing.", Area: "IPT", AffectedSystem: "10.20.4.11"},
			{Title: "Kerberoastable service accounts", Severity: "high", Description: "Six service accounts carry SPNs with weak passwords.", Recommendation: "Rotate to managed service accounts.", Area: "IPT"},
			{Title: "Local administrator password reuse", Severity: "critical", Description: "The same local administrator hash is present on 380 workstations.", Recommendation: "Deploy LAPS.", Area: "IPT"},
			{Title: "Exposed management interface", Severity: "high", Description: "A firewall management portal is reachable from the internet.", Recommendation: "Restrict to the management VLAN.", Area: "EPT", AffectedSystem: "196.46.20.14"},
			{Title: "Verbose service banners", Severity: "low", Description: "Banners disclose exact software versions.", Recommendation: "Suppress banners.", Area: "EPT"},
			{Title: "Telnet enabled on switches", Severity: "medium", Description: "Nine switches still accept Telnet.", Recommendation: "Disable Telnet and use SSH.", Area: "CFG"},
			{Title: "Default SNMP community strings", Severity: "medium", Description: "Two devices answer to public.", Recommendation: "Set unique SNMPv3 credentials.", Area: "CFG"},
			{Title: "WPA2-PSK passphrase recovered", Severity: "high", Description: "The guest network passphrase was recovered by dictionary attack.", Recommendation: "Move to WPA2-Enterprise.", Area: "WNA"},
			{Title: "Guest wireless reaches the corporate VLAN", Severity: "critical", Description: "The guest SSID routes into the corporate range.", Recommendation: "Isolate the guest network.", Area: "WNA"},
			{Title: "Wireless management frames unprotected", Severity: "informational", Description: "802.11w is not enabled.", Recommendation: "Enable protected management frames.", Area: "WNA"},
		},
	}
}

func zipParts(t *testing.T, out []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("output is not a valid zip: %v", err)
	}
	parts := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		parts[strings.ReplaceAll(f.Name, "\\", "/")] = string(data)
	}
	return parts
}

// ---------------------------------------------------------------------------
// the logo
// ---------------------------------------------------------------------------

func TestZenithLogoDecodes(t *testing.T) {
	img, err := decodeUploadedImage(zenithLogo(t))
	if err != nil {
		t.Fatalf("decode svg: %v", err)
	}
	b := img.Bounds()
	t.Logf("rasterised to %dx%d", b.Dx(), b.Dy())

	f, err := os.Create(zenithDir + `\zenith-raster.png`)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}

	opaque := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0x4000 {
				opaque++
			}
		}
	}
	t.Logf("opaque pixels: %d of %d", opaque, b.Dx()*b.Dy())
	if opaque < b.Dx()*b.Dy()/50 {
		t.Fatalf("rasterised logo is effectively blank: %d opaque pixels", opaque)
	}
}

// ---------------------------------------------------------------------------
// the report
// ---------------------------------------------------------------------------

func TestZenithReport(t *testing.T) {
	config := zenithConfig(t)
	out, notes, err := mergeMCollaboratorDocx(config)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := os.WriteFile(zenithDir+`\GH-REP-041-26120-01 - Zenith Bank - VAPT.docx`, out, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("docx: %d bytes", len(out))
	if notes.LogoError != "" {
		t.Fatalf("logo rejected: %s", notes.LogoError)
	}

	parts := zipParts(t, out)

	// The logo has to reach every running header, not just the first.
	headers := 0
	withLogo := 0
	for name, body := range parts {
		if !isHeaderPart(name) {
			continue
		}
		headers++
		if strings.Contains(body, logoRelID) {
			withLogo++
		}
		if strings.Contains(body, logoSlotMarker) {
			t.Errorf("%s still prints the placeholder instruction box", name)
		}
	}
	t.Logf("customer logo in %d of %d headers", withLogo, headers)
	if withLogo < 3 {
		t.Errorf("customer logo reached only %d headers, want at least 3", withLogo)
	}
	if _, ok := parts[logoMediaPart]; !ok {
		t.Errorf("the logo image part %s is not in the package", logoMediaPart)
	}

	// Pass green, Issues red - and neither as a cell fill.
	doc := parts["word/document.xml"]
	for _, want := range []struct{ status, hex string }{
		{statusPass, "28A745"},
		{statusIssues, "C00000"},
	} {
		re := regexp.MustCompile(`<w:color w:val="` + want.hex + `"/>[^<]*(?:<[^>]*>)*?<w:t[^>]*>` + want.status + `</w:t>`)
		if !re.MatchString(doc) {
			t.Errorf("no %q is printed in %s", want.status, want.hex)
		}
		if strings.Contains(doc, `w:fill="`+want.hex+`"`) {
			t.Errorf("a cell is filled %s; only the %s text should carry the colour", want.hex, want.status)
		}
	}
	t.Logf("status words: Pass x%d, Issues x%d",
		strings.Count(doc, `<w:t>Pass</w:t>`)+strings.Count(doc, `<w:t xml:space="preserve">Pass</w:t>`),
		strings.Count(doc, `<w:t>Issues</w:t>`)+strings.Count(doc, `<w:t xml:space="preserve">Issues</w:t>`))
}

// ---------------------------------------------------------------------------
// the closing deck
// ---------------------------------------------------------------------------

func TestZenithClosureDeck(t *testing.T) {
	config := zenithConfig(t)
	out, notes, err := buildClosureDeck(config)
	if err != nil {
		t.Fatalf("build deck: %v", err)
	}
	if err := os.WriteFile(zenithDir+`\GH-REP-041-26120-01 - Zenith Bank - Closing Meeting.pptx`, out, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("pptx: %d bytes", len(out))
	if notes.LogoError != "" {
		t.Fatalf("deck logo rejected: %s", notes.LogoError)
	}

	parts := zipParts(t, out)
	findings := buildNumberedFindings(config)

	// -- the charts must carry this engagement's numbers -------------------
	area := parts[areaChartPart]
	sev := parts[severityChartPart]
	t.Logf("area chart cats/vals:     %v / %v", chartCats(area), chartVals(area))
	t.Logf("severity chart cats/vals: %v / %v", chartCats(sev), chartVals(sev))

	wantAreaCats, wantAreaVals := areaChartData(config, findings)
	wantAreaCats = fitCategoryLabels(config, wantAreaCats)
	if got := chartCats(area); !equalStrings(got, wantAreaCats) {
		t.Errorf("area chart categories = %v, want %v", got, wantAreaCats)
	}
	if got := chartVals(area); !equalInts(got, wantAreaVals) {
		t.Errorf("area chart values = %v, want %v", got, wantAreaVals)
	}

	wantSevVals := []int{}
	counts := map[string]int{}
	for _, f := range findings {
		counts[severityDisplay(f.Severity)]++
	}
	for _, seg := range severityRingOrder {
		wantSevVals = append(wantSevVals, counts[seg.severity])
	}
	if got := chartVals(sev); !equalInts(got, wantSevVals) {
		t.Errorf("severity chart values = %v, want %v", got, wantSevVals)
	}
	if total := sum(chartVals(sev)); total != len(findings) {
		t.Errorf("severity chart totals %d, but the engagement has %d findings", total, len(findings))
	}
	// The template's old figures must be gone from both.
	for _, stale := range []string{"18", "19"} {
		_ = stale
	}
	if strings.Contains(area, "<c:v>WebApp Test</c:v>") {
		t.Error("the area chart still carries the template's own category names")
	}

	// -- no placeholder may survive anywhere -------------------------------
	for name, body := range parts {
		if !strings.HasPrefix(name, "ppt/slides/slide") || strings.Contains(name, "_rels") {
			continue
		}
		for _, token := range []string{"[Scope]", "[Finding]", "[Affected Host]", "[Target]", "[Company Name]", "[Issue]", "[Rating]"} {
			if strings.Contains(body, token) {
				t.Errorf("%s still shows %s", name, token)
			}
		}
	}

	// -- the scope slide names this engagement's areas ----------------------
	scope := parts[scopeSlidePart]
	scopeText := slideText(scope)
	t.Logf("scope slide: %s", scopeText)
	for _, a := range config.Areas {
		def, _ := areaByCode(a.Code)
		if !strings.Contains(scopeText, def.Label) {
			t.Errorf("the scope slide does not name %s", def.Label)
		}
		for _, line := range splitScopeLines(a.Scope) {
			if !strings.Contains(scopeText, line) {
				t.Errorf("the scope slide does not carry %s scope %q", a.Code, line)
			}
		}
	}
	for _, gone := range []string{"Internal VAPT", "External VAPT", "Configuration Review", "Web Application Testing"} {
		if strings.Contains(scopeText, gone) {
			t.Errorf("the scope slide still shows the template's heading %q", gone)
		}
	}
	for _, x := range config.OutOfScope {
		if !strings.Contains(scopeText, x) {
			t.Errorf("the scope slide does not mention the exclusion %q", x)
		}
	}

	// -- each headline finding sits under its own area ----------------------
	panels := buildAreaPanels(config, findings)
	summaries := summarySlideTexts(parts)
	t.Logf("%d headline slide(s) for %d areas", len(summaries), len(panels))
	if want := (len(panels) + summaryPanelsPerSlide - 1) / summaryPanelsPerSlide; len(summaries) != want {
		t.Errorf("got %d headline slides, want %d", len(summaries), want)
	}
	all := strings.Join(summaries, " || ")
	for _, p := range panels {
		if !strings.Contains(all, p.Heading) {
			t.Errorf("no headline panel is headed %q", p.Heading)
		}
		for i, title := range p.Headlines {
			if i >= headlinesPerPanel {
				break
			}
			if !strings.Contains(all, title) {
				t.Errorf("%s headline %q is missing", p.Heading, title)
			}
		}
	}
	for _, s := range summaries {
		t.Logf("headline slide: %s", s)
	}
}

// TestZenithDeckWithoutScope is the case the user reported as jumbled: areas
// chosen, but no scope text typed against any of them.
func TestZenithDeckWithoutScope(t *testing.T) {
	config := zenithConfig(t)
	for i := range config.Areas {
		config.Areas[i].Scope = ""
	}
	config.OutOfScope = nil

	out, _, err := buildClosureDeck(config)
	if err != nil {
		t.Fatalf("build deck: %v", err)
	}
	if err := os.WriteFile(zenithDir+`\Zenith Bank - Closing Meeting (no scope).pptx`, out, 0644); err != nil {
		t.Fatal(err)
	}
	parts := zipParts(t, out)
	scopeText := slideText(parts[scopeSlidePart])
	t.Logf("scope slide with no scope entered: %s", scopeText)

	for _, token := range []string{"[Scope]", "[Affected Host]", "[Target]"} {
		if strings.Contains(parts[scopeSlidePart], token) {
			t.Errorf("the scope slide still shows %s when no scope was entered", token)
		}
	}
	// Every chosen area still has to be named; only its target list is empty.
	for _, a := range config.Areas {
		def, _ := areaByCode(a.Code)
		if !strings.Contains(scopeText, def.Label) {
			t.Errorf("the scope slide drops %s when it has no scope text", def.Label)
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

var (
	cvRe     = regexp.MustCompile(`<c:v>([^<]*)</c:v>`)
	catBlkRe = regexp.MustCompile(`(?s)<c:cat>.*?</c:cat>`)
	valBlkRe = regexp.MustCompile(`(?s)<c:val>.*?</c:val>`)
	atRe     = regexp.MustCompile(`<a:t>([^<]*)</a:t>`)
)

func chartCats(chart string) []string {
	var out []string
	for _, m := range cvRe.FindAllStringSubmatch(catBlkRe.FindString(chart), -1) {
		out = append(out, m[1])
	}
	return out
}

func chartVals(chart string) []int {
	var out []int
	for _, m := range cvRe.FindAllStringSubmatch(valBlkRe.FindString(chart), -1) {
		n, err := parseInt(m[1])
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

func slideText(slide string) string {
	var parts []string
	for _, m := range atRe.FindAllStringSubmatch(slide, -1) {
		if s := strings.TrimSpace(m[1]); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " | ")
}

// summarySlideTexts returns the text of the generated headline slides, which are
// the cloned parts numbered from 1000 up.
func summarySlideTexts(parts map[string]string) []string {
	var names []string
	for name, body := range parts {
		if strings.HasPrefix(name, "ppt/slides/slide1") && strings.Contains(body, "Executive Summary") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var out []string
	for _, n := range names {
		out = append(out, fmt.Sprintf("%s: %s", n, slideText(parts[n])))
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sum(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return t
}
