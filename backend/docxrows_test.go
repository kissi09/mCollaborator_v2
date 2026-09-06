package main

import (
	"strings"
	"testing"
)

// findingRows returns the label=value pairs printed for one finding, in the
// order the report prints them.
func findingRows(t *testing.T, doc, title string) []string {
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
		t.Fatalf("finding %q not found in the report", title)
	}

	var out []string
	for i := start + 1; i < len(children); i++ {
		if children[i].Tag != "w:tbl" {
			// The heading of the next finding, or the next section.
			if text := strings.TrimSpace(elemText(doc[children[i].Start:children[i].End])); text != "" && len(out) > 0 {
				break
			}
			continue
		}
		tbl := doc[children[i].Start:children[i].End]
		if tableHasLabel(tbl, "description") == false && tableHasLabel(tbl, "recommendation") == false {
			break // the next section's test-type checklist
		}
		rows := tableRows(tbl)
		for ri := 0; ri+1 < len(rows); ri++ {
			row := tbl[rows[ri].Start:rows[ri].End]
			next := tbl[rows[ri+1].Start:rows[ri+1].End]
			nextCells := rowCells(next)
			for ci, c := range rowCells(row) {
				label := strings.TrimSpace(elemText(row[c.Start:c.End]))
				if _, ok := detailLabels[strings.ToLower(label)]; !ok {
					continue
				}
				target := ci
				if target >= len(nextCells) {
					target = 0
				}
				if target >= len(nextCells) {
					continue
				}
				value := strings.TrimSpace(elemText(next[nextCells[target].Start:nextCells[target].End]))
				out = append(out, label+"="+value)
			}
		}
	}
	return out
}

func rowsConfig() ReportConfig {
	config := sampleConfig()
	config.Areas = []ReportArea{
		{Code: "IPT", Scope: "10.0.4.0/24"},
		{Code: "EPT", Scope: "8 public IPs"},
		{Code: "WPT", Scope: "https://portal.acme.test"},
		{Code: "CFG", Scope: "4 firewall configs"},
		{Code: "ADT", Scope: "corp.acme.test"},
		{Code: "NAR", Scope: "HQ topology"},
	}
	config.Findings = []ReportFinding{
		{Title: "SMB signing not required", Severity: "high", Area: "IPT",
			Description: "Unsigned SMB accepted.", Impact: "Relay to domain admin.",
			AffectedSystem: "10.0.4.11", AttackVector: "Adjacent Network",
			POC: "ntlmrelayx transcript", Recommendation: "Require SMB signing."},
		{Title: "Exposed management interface", Severity: "high", Area: "IPT",
			Description: "No proof attached to this one.", Impact: "Console access.",
			AffectedSystem: "10.0.4.20", Recommendation: "Restrict to the management VLAN."},
		{Title: "Weak TLS ciphers", Severity: "medium", Area: "EPT",
			Description: "TLS 1.0 offered.", Impact: "Traffic interception.",
			AffectedSystem: "196.46.20.14", POC: "testssl.sh output",
			Recommendation: "Disable TLS 1.0 and 1.1."},
		{Title: "Default SNMP community string", Severity: "high", Area: "CFG",
			Description: "Answers to 'public'.", Impact: "Full config disclosure.",
			AffectedSystem: "fw-hq-01", Recommendation: "Set a unique community string."},
		{Title: "Kerberoastable service account", Severity: "medium", Area: "ADT",
			Description: "svc_sql has an SPN.", Impact: "Domain escalation.",
			AffectedSystem: "corp.acme.test", AttackVector: "Adjacent Network",
			POC: "GetUserSPNs.py", Recommendation: "Rotate the password."},
		{Title: "Flat network with no segmentation", Severity: "high", Area: "NAR",
			Description: "One broadcast domain.", Impact: "Lateral movement unimpeded.",
			AffectedSystem: "HQ core VLAN 1", Recommendation: "Introduce VLANs."},
		{Title: "Reflected XSS", Severity: "medium", Area: "WPT",
			Description: "q is echoed.", Impact: "Session theft.",
			AffectedSystem: "portal.acme.test", POC: "GET /search?q=",
			Recommendation: "Encode on output."},
	}
	return config
}

// Every section prints an Impact row, whether its template block shipped one or
// not: a finding has a business impact wherever it was found.
func TestEverySectionPrintsAnImpactRow(t *testing.T) {
	doc := readDocxParts(t, rowsConfig())["word/document.xml"]

	for title, want := range map[string]string{
		"SMB signing not required":          "Relay to domain admin.",
		"Weak TLS ciphers":                  "Traffic interception.",
		"Default SNMP community string":     "Full config disclosure.",
		"Kerberoastable service account":    "Domain escalation.",
		"Flat network with no segmentation": "Lateral movement unimpeded.",
		"Reflected XSS":                     "Session theft.",
	} {
		rows := findingRows(t, doc, title)
		if !hasRow(rows, "Impact", want) {
			t.Errorf("%s: no Impact row carrying %q; got %v", title, want, rows)
		}
	}
}

// IPT and EPT get a proof-of-concept row - but only when there is proof. A
// section that never shipped the row should not start printing an empty one.
func TestIPTAndEPTPrintAPoCOnlyWhenThereIsProof(t *testing.T) {
	doc := readDocxParts(t, rowsConfig())["word/document.xml"]

	proved := findingRows(t, doc, "SMB signing not required")
	if !hasRow(proved, "PoC", "ntlmrelayx transcript") {
		t.Errorf("an IPT finding with proof got no PoC row; rows: %v", proved)
	}

	external := findingRows(t, doc, "Weak TLS ciphers")
	if !hasRow(external, "PoC", "testssl.sh output") {
		t.Errorf("an EPT finding with proof got no PoC row; rows: %v", external)
	}

	bare := findingRows(t, doc, "Exposed management interface")
	for _, r := range bare {
		if strings.HasPrefix(strings.ToLower(r), "poc=") {
			t.Errorf("an IPT finding with no proof was given an empty PoC row: %v", bare)
		}
	}
}

// The added rows go where the template would have put them: Impact ahead of the
// rows that follow the description, the PoC immediately before the
// recommendation.
func TestAddedRowsKeepTheTemplateOrder(t *testing.T) {
	doc := readDocxParts(t, rowsConfig())["word/document.xml"]

	got := labelsOf(findingRows(t, doc, "SMB signing not required"))
	want := []string{"Description", "Rating", "Impact", "CVSS Vector", "Attack Vector", "Affected Hosts", "PoC", "Recommendation"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("IPT rows = %v, want %v", got, want)
	}

	got = labelsOf(findingRows(t, doc, "Default SNMP community string"))
	want = []string{"Description", "Rating", "Impact", "Affected Device", "Recommendation"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("CFG rows = %v, want %v", got, want)
	}

	got = labelsOf(findingRows(t, doc, "Flat network with no segmentation"))
	want = []string{"Description", "Rating", "Impact", "Affected Network", "Recommendation"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("NAR rows = %v, want %v", got, want)
	}

	// A layout that already prints both is left exactly as it was.
	got = labelsOf(findingRows(t, doc, "Reflected XSS"))
	want = []string{"Description", "Rating", "CVSS Vector", "Impact", "Affected Application", "PoC", "Recommendation"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("WPT rows = %v, want %v", got, want)
	}
}

func hasRow(rows []string, label, value string) bool {
	for _, r := range rows {
		if strings.HasPrefix(r, label+"=") && strings.Contains(r, value) {
			return true
		}
	}
	return false
}

func labelsOf(rows []string) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if i := strings.Index(r, "="); i >= 0 {
			out = append(out, r[:i])
		}
	}
	return out
}
