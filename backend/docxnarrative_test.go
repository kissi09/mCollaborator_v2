package main

import (
	"strings"
	"testing"
)

// paragraphTexts is every top-level paragraph's text in the rendered document.
func paragraphTexts(doc string) []string {
	var out []string
	for _, c := range bodyChildren(doc) {
		if c.Tag == "w:p" {
			out = append(out, strings.TrimSpace(elemText(doc[c.Start:c.End])))
		}
	}
	return out
}

func paragraphStarting(t *testing.T, doc, opening string) string {
	t.Helper()
	for _, p := range paragraphTexts(doc) {
		if strings.HasPrefix(p, opening) {
			return p
		}
	}
	t.Fatalf("no paragraph starts %q", opening)
	return ""
}

// A configuration review and a network architecture review test nothing onsite
// or remotely and look at no external applications. The introduction and the
// summary said they did, because the template was written for a pentest.
func TestReviewOnlyReportDescribesAReview(t *testing.T) {
	doc := readDocxParts(t, narSpacingConfig())["word/document.xml"]
	all := strings.Join(paragraphTexts(doc), "\n")

	for _, wrong := range []string{
		"internal systems and external applications",
		"remotely for the external applications",
		"The external-facing assets",
		"minor TLS configuration issue",
		"exploitation attempts were undertaken",
		"vulnerability scan was initiated",
	} {
		if strings.Contains(all, wrong) {
			t.Errorf("a review-only report still says %q", wrong)
		}
	}

	intro := paragraphStarting(t, doc, introOpening)
	for _, want := range []string{"network device configurations and network architecture", "as a review of", "Acme"} {
		if !strings.Contains(intro, want) {
			t.Errorf("introduction lacks %q: %s", want, intro)
		}
	}
	if strings.Contains(intro, "[Company Name]") || strings.Contains(intro, "[Assessment") {
		t.Errorf("introduction kept a placeholder: %s", intro)
	}

	posture := paragraphStarting(t, doc, "The reviewed device configurations")
	if !strings.Contains(posture, "The network architecture") {
		t.Errorf("the posture paragraph does not cover the network architecture: %s", posture)
	}
}

// Section 1.3 was written about a web and fintech pentest. None of that may
// reach a review report, and every paragraph left must speak about its areas.
func TestRiskAssessmentFollowsTheAreas(t *testing.T) {
	doc := readDocxParts(t, narSpacingConfig())["word/document.xml"]
	all := strings.Join(paragraphTexts(doc), "\n")
	for _, wrong := range []string{
		"OWASP", "customer account takeover", "fraudulent financial activity",
		"unauthorized refunds", "move from public access", "not regularly updated with essential security patches",
		"The critical issues in the applications", "infrastructure and applications", "[vulnerability",
	} {
		if strings.Contains(all, wrong) {
			t.Errorf("a CFG/NAR report's risk section still says %q", wrong)
		}
	}
	for _, want := range []string{
		"insecure network device configurations, as well as weaknesses in network design and segmentation",
		"unauthorized administrative access to network devices and interception of management traffic, as well as an attack spreading",
		"This could result in",
		"that the network devices have not been hardened against a security baseline",
		"an attack spreading unhindered across the network",
		"take control of network devices",
		"Overall, the reviewed environment presents a high-risk attack surface",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("risk section lacks %q", want)
		}
	}
}

// "Not regularly patched" is said only when a named finding is about patching.
func TestRiskCauseMentionsPatchingOnlyForOutdatedSoftware(t *testing.T) {
	mk := func(title, area, sev string) numberedFinding {
		a, _ := areaByCode(area)
		return numberedFinding{ReportFinding: ReportFinding{Title: title, Severity: sev}, Area: a}
	}
	plain := riskCause([]numberedFinding{mk("Flat network", "NAR", "high")})
	if strings.Contains(plain, "patches") {
		t.Errorf("a design finding was blamed on patching: %s", plain)
	}
	eol := riskCause([]numberedFinding{
		mk("Telnet enabled", "CFG", "high"), mk("SNMP public", "CFG", "high"),
		mk("NX-OS 6.2(2) End of Life (EOL)", "CFG", "high"), mk("Flat network", "NAR", "high"),
	})
	for _, want := range []string{"NX-OS 6.2(2) End of Life (EOL) and Flat network",
		"that the systems are not regularly updated with essential security patches and that security was not fully considered"} {
		if !strings.Contains(eol, want) {
			t.Errorf("cause lacks %q: %s", want, eol)
		}
	}
}

// Low findings earn no attack narrative and no chaining claim; no findings at
// all leave only the overall verdict.
func TestRiskSectionDropsWhatTheFindingsDoNotSupport(t *testing.T) {
	a, _ := areaByCode("CFG")
	low := buildRiskSection([]string{"CFG"}, []numberedFinding{
		{ReportFinding: ReportFinding{Title: "No banner", Severity: "low"}, Area: a},
	})
	if low.Practical != "" || low.Chain != "" {
		t.Errorf("a single low finding kept practical=%q chain=%q", low.Practical, low.Chain)
	}
	if !strings.Contains(low.Overview, "limited risk") || !strings.Contains(low.Overall, "low-risk attack surface") {
		t.Errorf("low report overstated: %q / %q", low.Overview, low.Overall)
	}

	none := buildRiskSection([]string{"CFG"}, nil)
	if none.Overview != "" || none.Cause != "" || none.Impact != "" || none.Practical != "" || none.Chain != "" {
		t.Errorf("a report with no findings kept risk paragraphs: %+v", none)
	}
	if none.Overall == "" {
		t.Error("a report with no findings lost its overall verdict")
	}

	doc := readDocxParts(t, func() ReportConfig {
		c := narSpacingConfig()
		for i := range c.Findings {
			c.Findings[i].Severity = "low"
		}
		return c
	}())["word/document.xml"]
	all := strings.Join(paragraphTexts(doc), "\n")
	if strings.Contains(all, riskPracticalOpening) || strings.Contains(all, riskChainOpening) {
		t.Error("the removed paragraphs are still in the rendered report")
	}
}

// A pentest that tested both sides keeps the template's onsite/remote sentence.
func TestMixedEngagementKeepsOnsiteAndRemote(t *testing.T) {
	doc := readDocxParts(t, rowsConfig())["word/document.xml"]
	intro := paragraphStarting(t, doc, introOpening)
	for _, want := range []string{"both onsite for", "remotely for", "A review of"} {
		if !strings.Contains(intro, want) {
			t.Errorf("introduction lacks %q: %s", want, intro)
		}
	}
	scope := paragraphStarting(t, doc, scopeOpening)
	if !strings.Contains(scope, "mimicked the methods of malicious hackers") {
		t.Errorf("a tested engagement lost its penetration testing scope item: %s", scope)
	}
	// Testing was done, so the template's reconnaissance account stands.
	paragraphStarting(t, doc, reconOpening+", reconnaissance")
}

// Each area gets the posture sentence its worst finding earns.
func TestPostureFollowsEachAreasWorstFinding(t *testing.T) {
	worst := map[string]int{"EPT": 3, "NAR": 1, "CFG": 2}
	text, concern, sound := postureParagraph([]string{"EPT", "WPT", "CFG", "NAR"}, worst)
	if !concern || !sound {
		t.Errorf("concern=%v sound=%v, want both", concern, sound)
	}
	for _, want := range []string{
		areaNarratives["EPT"].Secure + " Only low-risk issues",
		areaNarratives["WPT"].Secure,
		"The reviewed device configurations are reasonably well secured",
		areaNarratives["NAR"].Concern,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("posture lacks %q:\n%s", want, text)
		}
	}
}

// The register's Exposure column prints the area's abbreviation, the same one
// its vulnerability id carries.
func TestRegisterExposureIsTheAreaCode(t *testing.T) {
	doc := readDocxParts(t, narSpacingConfig())["word/document.xml"]
	children := bodyChildren(doc)
	idx := findHeading(children, "Vulnerability Register")
	if idx < 0 {
		t.Fatal("no vulnerability register")
	}
	var tbl string
	for _, c := range children[idx:] {
		if c.Tag == "w:tbl" {
			tbl = doc[c.Start:c.End]
			break
		}
	}
	rows := tableRows(tbl)
	seen := map[string]bool{}
	for _, r := range rows[1:] {
		row := tbl[r.Start:r.End]
		cells := rowCells(row)
		exposure := strings.TrimSpace(elemText(row[cells[1].Start:cells[1].End]))
		id := strings.TrimSpace(elemText(row[cells[3].Start:cells[3].End]))
		if !strings.Contains(id, "_"+exposure) {
			t.Errorf("exposure %q does not match vulnerability id %q", exposure, id)
		}
		seen[exposure] = true
	}
	if !seen["CFG"] || !seen["NAR"] {
		t.Errorf("exposures printed: %v, want CFG and NAR", seen)
	}
}

// The register's recommendation is the first sentence, no longer than two lines
// of its column, and never ends in an ellipsis.
func TestRecommendationTitleIsShortAndWhole(t *testing.T) {
	cases := []struct{ body, want string }{
		{"Migrate SNMP monitoring to SNMPv3 using authentication and encryption. Remove v2c.",
			"Migrate SNMP monitoring to SNMPv3"},
		{"Upgrade to NX-OS 9.3(10) or later. Schedule a window.", "Upgrade to NX-OS 9.3(10) or later"},
		{"Implement TACACS+ or RADIUS for centralized device authentication, authorization, and accounting (AAA) across all devices.",
			"Implement TACACS+ or RADIUS"},
		{"Immediately disable Telnet on all devices.", "Immediately disable Telnet on all devices"},
		{"1. Disable FTP.\n2. Use SCP instead.", "Disable FTP"},
		{"Restrict access, e.g. with ACLs. Then audit.", "Restrict access, e.g. with ACLs"},
		{"Enforce TLS 1.2 or higher.", "Enforce TLS 1.2 or higher"},
	}
	for _, c := range cases {
		got := recommendationTitle(ReportFinding{Recommendation: c.body})
		if got != c.want {
			t.Errorf("recommendationTitle(%q) = %q, want %q", c.body, got, c.want)
		}
		if strings.Contains(got, "...") || strings.Contains(got, "…") {
			t.Errorf("%q ends in an ellipsis", got)
		}
		if !fitsLines(got, recTitleLineChars, recTitleMaxLines) {
			t.Errorf("%q runs past two lines", got)
		}
	}

	// A sentence with no pause at all is cut between words, never on a dangler.
	long := "Replace every legacy unmanaged access switch the branch offices still run today"
	got := shortRecommendation(long)
	if !fitsLines(got, recTitleLineChars, recTitleMaxLines) || strings.HasSuffix(got, " the") {
		t.Errorf("shortRecommendation(%q) = %q", long, got)
	}

	// A header the tester typed is theirs and is printed as typed.
	typed := "Harden SNMP, SSH and HTTPS management access on every core and distribution switch"
	if got := recommendationTitle(ReportFinding{RecommendationHeader: typed, Recommendation: "x"}); got != typed {
		t.Errorf("typed header changed to %q", got)
	}
}
