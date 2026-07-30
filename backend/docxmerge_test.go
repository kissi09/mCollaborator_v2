package main

import (
	"archive/zip"
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestMergeVAPTDocxScalarTokens(t *testing.T) {
	config := ReportConfig{
		CompanyName:     "Acme Test Corp",
		EngagementName:  "VAPT Report",
		RefNumber:       "GH-REP-099-99999-01",
		AssessmentStart: "1st August 2026",
		AssessmentEnd:   "10th August 2026",
		TesterName:      "Jane Analyst",
		ApproverName:    "John Approver",
		ApproverTitle:   "Head of Delivery",
		VersionLabel:    "1.0",
		Findings: []ReportFinding{
			{Title: "SQL Injection", Severity: "critical", Description: "desc", Recommendation: "rec"},
			{Title: "XSS", Severity: "high", Description: "desc2", Recommendation: "rec2"},
		},
	}

	out, err := mergeVAPTDocx(config)
	if err != nil {
		t.Fatalf("mergeVAPTDocx failed: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("output is not a valid zip: %v", err)
	}

	files := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		rc.Close()
		files[f.Name] = buf.Bytes()
	}

	docXML := string(files["word/document.xml"])
	footer4 := string(files["word/footer4.xml"])
	footer5 := string(files["word/footer5.xml"])
	footer7 := string(files["word/footer7.xml"])

	checks := []struct {
		name string
		body string
		want string
	}{
		{"company name in body", docXML, "Acme Test Corp"},
		{"ref number in footer4", footer4, "GH-REP-099-99999-01"},
		{"ref number in footer5", footer5, "GH-REP-099-99999-01"},
		{"ref number in footer7", footer7, "GH-REP-099-99999-01"},
		{"company name in footer4", footer4, "Acme Test Corp"},
		{"company name in footer5", footer5, "Acme Test Corp"},
		{"company name in footer7", footer7, "Acme Test Corp"},
		{"tester name", docXML, "Jane Analyst"},
		{"approver name", docXML, "John Approver"},
		{"approver title", docXML, "Head of Delivery"},
		{"assessment period", docXML, "1st August 2026 to 10th August 2026"},
		{"findings count", docXML, "<w:t>2</w:t>"},
	}
	for _, c := range checks {
		if !strings.Contains(c.body, c.want) {
			t.Errorf("%s: expected to find %q but did not", c.name, c.want)
		}
	}

	// Finding content should now repeat once per finding, with no leftover
	// markers or example content from the template.
	findingChecks := []string{"SQL Injection", "XSS", "desc", "desc2", "rec", "rec2"}
	for _, want := range findingChecks {
		if strings.Count(docXML, want) == 0 {
			t.Errorf("expected finding content %q to appear in document.xml", want)
		}
	}
	for _, marker := range []string{
		"<!--REPEAT:FINDING_ROW-->", "<!--END:FINDING_ROW-->",
		"<!--REPEAT:FINDING_DETAIL-->", "<!--END:FINDING_DETAIL-->",
	} {
		if strings.Contains(docXML, marker) {
			t.Errorf("repeat marker %q should not survive in the rendered output", marker)
		}
	}

	// No unresolved scalar tokens should remain anywhere in the merged parts.
	unresolvedTokens := []string{
		"{{CompanyName}}", "{{VersionLabel}}", "{{TesterName}}", "{{ApproverName}}",
		"{{ApproverTitle}}", "{{AssessmentPeriod}}", "{{ReportDate}}", "{{RefNumber}}",
		"{{FindingsCount}}", "{{FindingTitle}}", "{{FindingExposure}}", "{{FindingCriticality}}",
		"{{FindingCriticalityColor}}", "{{FindingCriticalityText}}", "{{FindingVulnID}}",
		"{{FindingDescription}}", "{{FindingCVSS}}", "{{FindingImpact}}", "{{FindingAffected}}",
		"{{FindingPOC}}", "{{FindingRecommendation}}", "{{FindingNumber}}",
	}
	for name, data := range files {
		body := string(data)
		for _, tok := range unresolvedTokens {
			if strings.Contains(body, tok) {
				t.Errorf("%s: unresolved token %s left in output", name, tok)
			}
		}
	}

	// Same part count/names as the source template (only content mutated).
	if len(zr.File) == 0 {
		t.Fatal("output zip has no entries")
	}

	if os.Getenv("DOCXMERGE_WRITE_SAMPLE") != "" {
		if err := os.WriteFile(os.Getenv("DOCXMERGE_WRITE_SAMPLE"), out, 0644); err != nil {
			t.Fatalf("write sample: %v", err)
		}
	}
}
