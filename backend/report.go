package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"html"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jung-kurt/gofpdf"
)

type ReportConfig struct {
	CompanyName     string          `json:"company_name"`      // Client company name (replaces "Company's Name")
	CompanyLogo     string          `json:"company_logo"`      // Client logo (base64) shown top-left in header
	EngagementName  string          `json:"engagement_name"`   // e.g. "VAPT Report"
	ClientEmail     string          `json:"client_email"`
	RefNumber       string          `json:"ref_number"`        // e.g. GH-REP-035-26059-01
	AssessmentStart string          `json:"assessment_start"`  // e.g. "17th June 2026"
	AssessmentEnd   string          `json:"assessment_end"`    // e.g. "24th June 2026"
	TesterName      string          `json:"tester_name"`
	ApproverName    string          `json:"approver_name"`
	ApproverTitle   string          `json:"approver_title"`
	VersionLabel    string          `json:"version_label"`
	Introduction    string          `json:"introduction"`      // Custom exec summary intro (optional)
	Scope           []string        `json:"scope"`
	OutOfScope      []string        `json:"out_of_scope"`
	Sections        []string        `json:"sections"`          // wpt, ept, ipt, nar
	TestCases       []TestCaseGroup `json:"test_cases"`        // Web app test case table
	Findings        []ReportFinding `json:"findings"`
	Tools           []string        `json:"tools"`
}

type TestCaseGroup struct {
	Group string         `json:"group"` // e.g. "Information Gathering"
	Cases []TestCaseItem `json:"cases"`
}

type TestCaseItem struct {
	Name   string `json:"name"`
	Status string `json:"status"` // Pass, Issues, N/A
}

type ReportFinding struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	Impact         string `json:"impact"`
	CVSSVector     string `json:"cvss_vector"`
	CVSSScore      string `json:"cvss_score"`
	Severity       string `json:"severity"` // critical, high, medium, low, informational
	AffectedSystem string `json:"affected_system"`
	POC            string `json:"poc"`
	Recommendation string `json:"recommendation"`
	Category       string `json:"category"`     // web, external, internal, architecture
	Exposure       string `json:"exposure"`     // Web, External, Internal, etc.
	VulnID         string `json:"vuln_id"`      // REC1_WPT1 etc. (auto-generated if empty)
}

type exportReportResponse struct {
	DOCXURL string `json:"docx_url"`
	PDFURL  string `json:"pdf_url"`
}

func ensureReportsDir() (string, error) {
	dir := filepath.Join("backend", "reports")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create reports directory: %w", err)
	}
	return dir, nil
}

func sanitizeFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, name)
	if len(name) > 80 {
		name = name[:80]
	}
	return name
}

func severityColor(sev string) (r, g, b int) {
	switch strings.ToLower(sev) {
	case "critical":
		return 220, 53, 69
	case "high":
		return 255, 152, 0
	case "medium":
		return 255, 193, 7
	case "low":
		return 40, 167, 69
	default:
		return 108, 117, 125
	}
}

func severityFillColor(sev string) (r, g, b int) {
	switch strings.ToLower(sev) {
	case "critical":
		return 248, 215, 218
	case "high":
		return 255, 238, 205
	case "medium":
		return 255, 243, 205
	case "low":
		return 212, 237, 218
	default:
		return 226, 227, 229
	}
}

func groupFindingsByCategory(findings []ReportFinding) map[string][]ReportFinding {
	grouped := make(map[string][]ReportFinding)
	for _, f := range findings {
		cat := f.Category
		if cat == "" {
			cat = "General"
		}
		grouped[cat] = append(grouped[cat], f)
	}
	return grouped
}

func countBySeverity(findings []ReportFinding) map[string]int {
	counts := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0, "info": 0}
	for _, f := range findings {
		sev := strings.ToLower(f.Severity)
		if _, ok := counts[sev]; ok {
			counts[sev]++
		}
	}
	return counts
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func HandleExportReport(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var config ReportConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			writeJSON(w, http.StatusBadRequest, ApiResponse{
				Error: &ApiError{Code: "INVALID_REQUEST", Message: "Invalid report configuration"},
			})
			return
		}

		if config.CompanyName == "" {
			config.CompanyName = "Security Assessment"
		}
		if config.EngagementName == "" {
			config.EngagementName = "Vulnerability Assessment Report"
		}

		reportsDir, err := ensureReportsDir()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ApiResponse{
				Error: &ApiError{Code: "INTERNAL_ERROR", Message: err.Error()},
			})
			return
		}

		timestamp := time.Now().Format("20060102_150405")
		baseName := sanitizeFilename(fmt.Sprintf("%s_%s_%s", config.CompanyName, config.EngagementName, timestamp))

		docxPath := filepath.Join(reportsDir, baseName+".docx")
		pdfPath := filepath.Join(reportsDir, baseName+".pdf")

		docxURL, err := GenerateDOCX(config, docxPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ApiResponse{
				Error: &ApiError{Code: "DOCX_GENERATION_FAILED", Message: fmt.Sprintf("Failed to generate DOCX: %s", err.Error())},
			})
			return
		}

		pdfURL, err := GeneratePDF(config, pdfPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ApiResponse{
				Error: &ApiError{Code: "PDF_GENERATION_FAILED", Message: fmt.Sprintf("Failed to generate PDF: %s", err.Error())},
			})
			return
		}

		// Best-effort audit log (may not have user if unauthenticated)
		if user, ok := r.Context().Value(userContextKey).(*User); ok && user != nil {
			store.AddAuditLog(&AuditLog{
				OrgID: user.OrgID, ActorID: user.ID,
				Action: "report.export", Resource: "report",
				ResourceID: baseName, IPAddress: r.RemoteAddr,
			})
		}

		writeJSON(w, http.StatusOK, ApiResponse{
			Data: exportReportResponse{
				DOCXURL: docxURL,
				PDFURL:  pdfURL,
			},
		})
	}
}

func HandleDownloadReport(w http.ResponseWriter, r *http.Request) {
	reportType := chi.URLParam(r, "type")
	reportName := chi.URLParam(r, "name")

	if reportType == "" || reportName == "" {
		http.Error(w, "Missing report type or name", http.StatusBadRequest)
		return
	}

	if reportType != "docx" && reportType != "pdf" {
		http.Error(w, "Invalid report type", http.StatusBadRequest)
		return
	}

	reportsDir, err := ensureReportsDir()
	if err != nil {
		http.Error(w, "Reports directory error", http.StatusInternalServerError)
		return
	}

	fileExt := "." + reportType
	if reportType == "docx" {
		fileExt = ".html"
	}
	filePath := filepath.Join(reportsDir, reportName+fileExt)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "Report not found", http.StatusNotFound)
		return
	}

	if reportType == "pdf" {
		w.Header().Set("Content-Type", "application/pdf")
	} else if reportType == "docx" {
		w.Header().Set("Content-Type", "text/html")
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.%s", reportName, reportType))
	http.ServeFile(w, r, filePath)
}

// ============================================================================
// Cyberteq VAPT report - shared helpers
// ============================================================================

const brandOrange = "#FF7900"

// severityDisplay normalizes a severity string to its canonical display label.
func severityDisplay(sev string) string {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return "Critical"
	case "high":
		return "High"
	case "medium":
		return "Medium"
	case "low":
		return "Low"
	case "info", "informational":
		return "Informational"
	default:
		if sev == "" {
			return "Informational"
		}
		return titleCase(strings.ToLower(sev))
	}
}

// vaptSeverityHex returns the brand hex color for a severity (for the DOCX badge).
func vaptSeverityHex(sev string) string {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return "#C00000"
	case "high":
		return "#FF7900"
	case "medium":
		return "#FFC000"
	case "low":
		return "#28A745"
	case "info", "informational":
		return "#2F6FED"
	default:
		return "#2F6FED"
	}
}

// vaptSeverityTextHex returns readable text color for a severity badge background.
func vaptSeverityTextHex(sev string) string {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "medium":
		return "#000000"
	default:
		return "#FFFFFF"
	}
}

// vulnID returns finding.VulnID or an auto-generated REC{i}_WPT{i}.
func vulnID(f ReportFinding, idx int) string {
	if strings.TrimSpace(f.VulnID) != "" {
		return f.VulnID
	}
	return fmt.Sprintf("REC%d_WPT%d", idx, idx)
}

// exposureOf returns finding.Exposure or the default "Web".
func exposureOf(f ReportFinding) string {
	if strings.TrimSpace(f.Exposure) != "" {
		return f.Exposure
	}
	return "Web"
}

// truncateText shortens a string to n runes, adding an ellipsis.
func truncateText(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "..."
}

// containsImage reports whether raw POC HTML contains an <img> tag.
func containsImage(s string) bool {
	return strings.Contains(strings.ToLower(s), "<img")
}

// vaptCountBySeverity counts findings under the five canonical VAPT buckets.
func vaptCountBySeverity(findings []ReportFinding) map[string]int {
	counts := map[string]int{"Critical": 0, "High": 0, "Medium": 0, "Low": 0, "Informational": 0}
	for _, f := range findings {
		counts[severityDisplay(f.Severity)]++
	}
	return counts
}

// defaultIntroduction builds the standard 1.1 Introduction text.
func defaultIntroduction(config ReportConfig) string {
	if strings.TrimSpace(config.Introduction) != "" {
		return config.Introduction
	}
	return fmt.Sprintf(
		"Cyberteq performed a security assessment on %s web applications. Vulnerability assessment and "+
			"penetration testing (VAPT) activities were conducted through a VPN to reach the internally hosted "+
			"applications. The security assessment was carried out during the period from %s to %s.",
		config.CompanyName, config.AssessmentStart, config.AssessmentEnd,
	)
}

var introBoilerplate = []string{
	"The main objective of this engagement was to identify security vulnerabilities within the in-scope " +
		"applications, evaluate their potential business impact, and provide actionable recommendations to " +
		"remediate the identified issues.",
	"During the reconnaissance phase, Cyberteq testers gathered information about the target applications " +
		"to understand the attack surface before performing automated and manual testing activities.",
	"This report details each of the findings identified during the assessment, providing a clear explanation " +
		"of the vulnerability, its impact on the business, and the recommendations required to remediate it.",
}

var scopeParagraph = "Security testing was conducted during the period from %s to %s."

var outOfScopeDefaults = [][2]string{
	{"Denial-of-Service (DoS) attacks", "Any testing intended to disrupt or degrade the availability of the target systems was excluded from the scope."},
	{"Client-side attacks", "Attacks that require social engineering or interaction with end-user devices were excluded from the scope."},
	{"Spoofing", "Email, caller-ID, or other spoofing-based attacks were excluded from the scope."},
}

var owaspMethodologyList = []string{
	"Injection",
	"Broken Authentication",
	"Sensitive Data Exposure",
	"Broken Access Control",
	"Security Misconfiguration",
	"Cross-Site Scripting (XSS)",
	"Insecure Deserialization",
	"Using Components with Known Vulnerabilities",
	"Insufficient Logging & Monitoring",
}

var ratingMetrics = []string{
	"Attack Vector - Reflects the context by which vulnerability exploitation is possible (Network, Adjacent, Local, Physical).",
	"Attack Complexity - Describes the conditions beyond the attacker's control that must exist to exploit the vulnerability.",
	"Privileges Required - Describes the level of privileges an attacker must possess before successfully exploiting the vulnerability.",
	"User Interaction - Captures the requirement for a human user, other than the attacker, to participate in the compromise.",
	"Authentication - The number of times an attacker must authenticate to a target to exploit the vulnerability.",
	"Exploitability - The likelihood of the vulnerability being attacked based on the current state of exploit techniques.",
	"Scope - Whether a vulnerability in one component impacts resources beyond its security scope.",
	"CIA - The impact to Confidentiality, Integrity, and Availability of the affected component.",
	"Asset Rating - The business value and criticality of the affected asset.",
	"Environment - Characteristics of the environment that may modify the base severity for the specific organization.",
}

var methodologyPhases = [][2]string{
	{"Information Gathering", "In this phase, Cyberteq testers collect as much information as possible about the target " +
		"environment through passive and active reconnaissance techniques to map the attack surface."},
	{"Automated Testing", "In this phase, Cyberteq testers use industry-standard automated tools to quickly identify " +
		"common and known vulnerabilities across the in-scope assets."},
	{"Manual Testing", "In this phase, Cyberteq testers manually verify and exploit the identified vulnerabilities and " +
		"discover complex logic flaws that automated tools cannot detect. Each finding is ranked as Informational, Low, " +
		"Medium, High, or Critical based on its risk."},
	{"Deliverables", "In this phase, Cyberteq consolidates all findings into a comprehensive report that includes an " +
		"executive summary, detailed technical findings, impact analysis, and remediation recommendations."},
}

var criticalityCriteria = [][3]string{
	{"Critical", "#C00000", "Business Critical - immediate action required."},
	{"High", "#FF7900", "Significant Security Threat - should be resolved quickly."},
	{"Medium", "#FFC000", "Area for Improvement - should be resolved."},
	{"Low", "#28A745", "Area for Improvement - low priority, resolve when convenient."},
	{"Informational", "#2F6FED", "Area for Improvement - promotes best practices."},
}

var namingConvention = [][2]string{
	{"REC", "Recommendation"},
	{"WPT", "Web Application Penetration Testing"},
	{"IPT", "Internal Penetration Testing"},
	{"EPT", "External Penetration Testing"},
	{"NAR", "Network Architecture Review"},
}

var defaultTools = []string{
	"Nessus Professional Feed",
	"Kali Linux",
	"Nmap",
	"FFuf",
	"Mantra",
	"SecretsFinder",
	"Acunetix",
	"Burpsuite Pro",
	"Curl",
}

// ============================================================================
// DOCX (Word-compatible HTML) generation
// ============================================================================

func GenerateDOCX(config ReportConfig, outputPath string) (string, error) {
	htmlPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".html"

	esc := html.EscapeString
	nowDate := strings.ToUpper(time.Now().Format("02-Jan-2006"))
	var sb strings.Builder

	// ---- Word HTML preamble ---------------------------------------------
	sb.WriteString("<html xmlns:o='urn:schemas-microsoft-com:office:office' ")
	sb.WriteString("xmlns:w='urn:schemas-microsoft-com:office:word' ")
	sb.WriteString("xmlns='http://www.w3.org/TR/REC-html40'>")
	sb.WriteString("<head><meta charset='utf-8'>")
	sb.WriteString("<title>" + esc(config.EngagementName) + " - " + esc(config.CompanyName) + "</title>")
	sb.WriteString("<style>")
	sb.WriteString("body{font-family:Calibri,Arial,sans-serif;color:#333;font-size:11pt;line-height:1.4;}")
	sb.WriteString("h1{color:" + brandOrange + ";font-size:20pt;margin-top:24px;}")
	sb.WriteString("h2{color:" + brandOrange + ";font-size:16pt;margin-top:18px;}")
	sb.WriteString("h3{color:" + brandOrange + ";font-size:13pt;margin-top:14px;}")
	sb.WriteString("h4{color:" + brandOrange + ";font-size:12pt;margin-top:12px;}")
	sb.WriteString("p{margin:6px 0;}")
	sb.WriteString("table{border-collapse:collapse;width:100%;margin:10px 0;}")
	sb.WriteString("th,td{border:1px solid #E0A96D;padding:6px 8px;text-align:left;vertical-align:top;font-size:10.5pt;}")
	sb.WriteString("th{background:" + brandOrange + ";color:#fff;font-weight:bold;}")
	sb.WriteString("td{background:#FDEEE0;}")
	sb.WriteString(".hdr{background:" + brandOrange + ";color:#fff;font-weight:bold;}")
	sb.WriteString(".badge{color:#fff;padding:2px 10px;border-radius:3px;font-weight:bold;display:inline-block;}")
	sb.WriteString(".pagebreak{page-break-before:always;}")
	sb.WriteString("ul{margin:6px 0 6px 18px;}")
	sb.WriteString("li{margin:3px 0;}")
	sb.WriteString(".toc td{background:#fff;border:none;padding:2px 8px;}")
	sb.WriteString(".poc{background:#f5f5f5;border:1px solid #ddd;padding:8px;font-family:Consolas,monospace;white-space:pre-wrap;}")
	sb.WriteString("</style></head><body>")

	// ---- COVER PAGE ------------------------------------------------------
	sb.WriteString("<div style='background:" + brandOrange + ";width:100%;height:120px;margin:0;padding:0;'>")
	if config.CompanyLogo != "" {
		sb.WriteString(fmt.Sprintf("<img src='data:image/png;base64,%s' style='max-height:90px;max-width:220px;margin:15px;'>", config.CompanyLogo))
	}
	sb.WriteString("</div>")
	sb.WriteString("<div style='margin-top:80px;'>")
	sb.WriteString("<h1 style='font-size:34pt;color:" + brandOrange + ";'>IT SECURITY CONSULTANCY</h1>")
	sb.WriteString(fmt.Sprintf("<p style='font-size:16pt;color:#444;'>%s &ndash; Vulnerability Assessment &amp; Penetration Testing (VAPT) POC</p>",
		esc(config.CompanyName)))
	sb.WriteString("</div>")

	// version table
	versionLabel := config.VersionLabel
	if strings.TrimSpace(versionLabel) == "" {
		versionLabel = "Details to be Provided"
	}
	sb.WriteString("<table style='margin-top:60px;'>")
	sb.WriteString("<tr><th>Version</th><th>Date</th><th>Authors</th><th>Approver</th></tr>")
	sb.WriteString("<tr>")
	sb.WriteString("<td>" + esc(versionLabel) + "</td>")
	sb.WriteString("<td>" + esc(nowDate) + "</td>")
	sb.WriteString("<td>" + esc(config.TesterName) + "<br>Cybersecurity Expert</td>")
	sb.WriteString("<td>" + esc(config.ApproverName) + "<br>" + esc(config.ApproverTitle) + "</td>")
	sb.WriteString("</tr></table>")

	// ---- TABLE OF CONTENTS ----------------------------------------------
	sb.WriteString("<div class='pagebreak'></div>")
	sb.WriteString("<h1>CONTENTS</h1>")
	tocEntries := []string{
		"1 Executive Summary",
		"&nbsp;&nbsp;&nbsp;1.1 Introduction",
		"&nbsp;&nbsp;&nbsp;1.2 Findings and Recommendations",
		"&nbsp;&nbsp;&nbsp;1.3 Risk Assessment and Impact on Business",
		"2 Methodology and Scope",
		"&nbsp;&nbsp;&nbsp;2.1 Cyberteq Security Assessment Methodology",
		"&nbsp;&nbsp;&nbsp;2.2 Cyberteq Vulnerability Rating Guide",
		"&nbsp;&nbsp;&nbsp;2.3 Scope",
		"&nbsp;&nbsp;&nbsp;2.4 Out of Scope",
		"&nbsp;&nbsp;&nbsp;2.5 Testing Methodologies",
		"&nbsp;&nbsp;&nbsp;2.6 Test Cases",
		"3 Findings and Recommendations",
		"&nbsp;&nbsp;&nbsp;3.1 Recommendations Naming Convention",
		"&nbsp;&nbsp;&nbsp;3.2 Vulnerability Register",
		"4 Tools and Licenses",
	}
	sb.WriteString("<table class='toc'>")
	for _, e := range tocEntries {
		sb.WriteString("<tr><td>" + e + "</td></tr>")
	}
	sb.WriteString("</table>")

	// ---- 1 EXECUTIVE SUMMARY --------------------------------------------
	sb.WriteString("<div class='pagebreak'></div>")
	sb.WriteString("<h1>1 Executive Summary</h1>")

	sb.WriteString("<h2>1.1 Introduction</h2>")
	sb.WriteString("<p>" + esc(defaultIntroduction(config)) + "</p>")
	for _, para := range introBoilerplate {
		sb.WriteString("<p>" + esc(para) + "</p>")
	}

	total := len(config.Findings)
	sb.WriteString("<h2>1.2 Findings and Recommendations</h2>")
	sb.WriteString(fmt.Sprintf("<p>During the security assessment of %s's infrastructure, %d issues of different "+
		"severity levels have been discovered, categorized, and reported upon. The following section summarizes the "+
		"distribution of the identified findings by their severity level.</p>", esc(config.CompanyName), total))

	sb.WriteString("<h3>1.2.1 Findings by Severity</h3>")
	counts := vaptCountBySeverity(config.Findings)
	sb.WriteString("<table><tr><th>Severity</th><th>Count</th></tr>")
	for _, sev := range []string{"Critical", "High", "Medium", "Low", "Informational"} {
		bg := vaptSeverityHex(sev)
		fg := vaptSeverityTextHex(sev)
		bar := strings.Repeat("&#9608;", counts[sev])
		sb.WriteString("<tr>")
		sb.WriteString(fmt.Sprintf("<td><span class='badge' style='background:%s;color:%s;'>%s</span></td>", bg, fg, sev))
		sb.WriteString(fmt.Sprintf("<td>%d &nbsp;<span style='color:%s;'>%s</span></td>", counts[sev], bg, bar))
		sb.WriteString("</tr>")
	}
	sb.WriteString("</table>")

	sb.WriteString("<h2>1.3 Risk Assessment and Impact on Business</h2>")
	sb.WriteString(fmt.Sprintf("<p>The vulnerabilities identified during this assessment pose varying levels of risk "+
		"to %s. If left unremediated, these findings could be leveraged by malicious actors to compromise the "+
		"confidentiality, integrity, and availability of the organization's information assets, potentially resulting "+
		"in financial loss, reputational damage, and regulatory non-compliance. Cyberteq recommends prioritizing "+
		"remediation efforts according to the criticality of each finding described in this report.</p>", esc(config.CompanyName)))

	// ---- 2 METHODOLOGY AND SCOPE ----------------------------------------
	sb.WriteString("<div class='pagebreak'></div>")
	sb.WriteString("<h1>2 Methodology and Scope</h1>")

	sb.WriteString("<h2>2.1 Cyberteq Security Assessment Methodology</h2>")
	sb.WriteString("<p>Cyberteq follows a structured, phased methodology to ensure comprehensive coverage during the " +
		"security assessment. The assessment is divided into the following four phases:</p>")
	for i, ph := range methodologyPhases {
		sb.WriteString(fmt.Sprintf("<h3>2.1.%d %s</h3>", i+1, esc(ph[0])))
		sb.WriteString("<p>" + esc(ph[1]) + "</p>")
	}

	sb.WriteString("<h2>2.2 Cyberteq Vulnerability Rating Guide</h2>")
	sb.WriteString("<p>Cyberteq rates the severity of each identified vulnerability using the Common Vulnerability " +
		"Scoring System (CVSS) version 3.1. The following metrics are considered when calculating the overall " +
		"severity of a finding:</p>")
	sb.WriteString("<ol>")
	for _, m := range ratingMetrics {
		sb.WriteString("<li>" + esc(m) + "</li>")
	}
	sb.WriteString("</ol>")

	sb.WriteString("<h2>2.3 Scope</h2>")
	sb.WriteString("<p>" + esc(fmt.Sprintf(scopeParagraph, config.AssessmentStart, config.AssessmentEnd)) + "</p>")
	sb.WriteString("<table><tr><th>Activity</th><th>Details</th></tr>")
	if len(config.Scope) > 0 {
		for _, s := range config.Scope {
			sb.WriteString("<tr><td>" + esc(s) + "</td><td>All activities were performed remotely.</td></tr>")
		}
	} else {
		sb.WriteString("<tr><td>Web Application Testing</td><td>All activities were performed remotely.</td></tr>")
	}
	sb.WriteString("</table>")

	sb.WriteString("<h2>2.4 Out of Scope</h2>")
	sb.WriteString("<ul>")
	if len(config.OutOfScope) > 0 {
		for _, s := range config.OutOfScope {
			sb.WriteString("<li>" + esc(s) + "</li>")
		}
	} else {
		for _, s := range outOfScopeDefaults {
			sb.WriteString("<li><strong>" + esc(s[0]) + ":</strong> " + esc(s[1]) + "</li>")
		}
	}
	sb.WriteString("</ul>")

	sb.WriteString("<h2>2.5 Testing Methodologies</h2>")
	sb.WriteString("<h3>2.5.1 Web Application Testing Methodology</h3>")
	sb.WriteString("<p>The web application testing was conducted in accordance with the OWASP Testing Guide, focusing " +
		"on the most critical web application security risks. The following categories were assessed:</p>")
	sb.WriteString("<ul>")
	for _, m := range owaspMethodologyList {
		sb.WriteString("<li>" + esc(m) + "</li>")
	}
	sb.WriteString("</ul>")

	sb.WriteString("<h2>2.6 Test Cases</h2>")
	sb.WriteString("<h3>2.6.1 Web Application Testing</h3>")
	if len(config.TestCases) > 0 {
		sb.WriteString("<table><tr><th>Test Type</th><th>Status</th></tr>")
		for _, grp := range config.TestCases {
			sb.WriteString(fmt.Sprintf("<tr><td colspan='2' style='background:#DDDDDD;color:#000;font-weight:bold;'>%s</td></tr>", esc(grp.Group)))
			for _, c := range grp.Cases {
				statusColor := "#6C757D"
				switch strings.ToLower(strings.TrimSpace(c.Status)) {
				case "pass":
					statusColor = "#28A745"
				case "issues":
					statusColor = "#DC3545"
				}
				sb.WriteString("<tr>")
				sb.WriteString("<td>" + esc(c.Name) + "</td>")
				sb.WriteString(fmt.Sprintf("<td><span style='color:%s;font-weight:bold;'>%s</span></td>", statusColor, esc(c.Status)))
				sb.WriteString("</tr>")
			}
		}
		sb.WriteString("</table>")
	}

	// ---- 3 FINDINGS AND RECOMMENDATIONS ---------------------------------
	sb.WriteString("<div class='pagebreak'></div>")
	sb.WriteString("<h1>3 Findings and Recommendations</h1>")
	sb.WriteString("<p>This section presents the findings identified during the security assessment along with their " +
		"associated risk criticality and recommended remediation actions. The criticality of each finding is " +
		"classified according to the following criteria:</p>")
	sb.WriteString("<table><tr><th>Criticality</th><th>Description</th></tr>")
	for _, c := range criticalityCriteria {
		fg := "#fff"
		if c[0] == "Medium" {
			fg = "#000"
		}
		sb.WriteString("<tr>")
		sb.WriteString(fmt.Sprintf("<td><span class='badge' style='background:%s;color:%s;'>%s</span></td>", c[1], fg, c[0]))
		sb.WriteString("<td>" + esc(c[2]) + "</td>")
		sb.WriteString("</tr>")
	}
	sb.WriteString("</table>")

	sb.WriteString("<h2>3.1 Recommendations Naming Convention</h2>")
	sb.WriteString("<table><tr><th>Abbreviation</th><th>Meaning</th></tr>")
	for _, n := range namingConvention {
		sb.WriteString("<tr><td>" + esc(n[0]) + "</td><td>" + esc(n[1]) + "</td></tr>")
	}
	sb.WriteString("</table>")
	sb.WriteString("<p>For example, <strong>REC3_WPT3</strong> refers to the 3rd recommendation for the 3rd finding " +
		"from web penetration testing.</p>")

	sb.WriteString("<h2>3.2 Vulnerability Register</h2>")
	sb.WriteString("<table>")
	sb.WriteString("<tr><th>Vulnerability</th><th>Exposure</th><th>Criticality</th><th>Vulnerability ID</th><th>Recommendation</th></tr>")
	for i, f := range config.Findings {
		id := vulnID(f, i+1)
		bg := vaptSeverityHex(f.Severity)
		fg := vaptSeverityTextHex(f.Severity)
		sb.WriteString("<tr>")
		sb.WriteString("<td>" + esc(f.Title) + "</td>")
		sb.WriteString("<td>" + esc(exposureOf(f)) + "</td>")
		sb.WriteString(fmt.Sprintf("<td><span class='badge' style='background:%s;color:%s;'>%s</span></td>", bg, fg, severityDisplay(f.Severity)))
		sb.WriteString("<td>" + esc(id) + "</td>")
		sb.WriteString("<td>" + esc(truncateText(f.Recommendation, 120)) + "</td>")
		sb.WriteString("</tr>")
	}
	sb.WriteString("</table>")

	// per-finding detail blocks
	for i, f := range config.Findings {
		id := vulnID(f, i+1)
		bg := vaptSeverityHex(f.Severity)
		fg := vaptSeverityTextHex(f.Severity)
		sb.WriteString(fmt.Sprintf("<h3>3.2.%d %s</h3>", i+1, esc(f.Title)))
		sb.WriteString("<table>")
		// Description header + row with rating
		sb.WriteString("<tr><td class='hdr' colspan='2'>Description</td></tr>")
		sb.WriteString("<tr>")
		sb.WriteString("<td style='width:75%;'>" + esc(f.Description) + "</td>")
		sb.WriteString(fmt.Sprintf("<td style='width:25%%;'>Rating: <span class='badge' style='background:%s;color:%s;'>%s</span></td>", bg, fg, severityDisplay(f.Severity)))
		sb.WriteString("</tr>")
		// CVSS Vector
		sb.WriteString("<tr><td class='hdr' colspan='2'>CVSS Vector String</td></tr>")
		sb.WriteString("<tr><td colspan='2'>" + esc(f.CVSSVector) + "</td></tr>")
		// Impact
		sb.WriteString("<tr><td class='hdr' colspan='2'>Impact</td></tr>")
		sb.WriteString("<tr><td colspan='2'>" + esc(f.Impact) + "</td></tr>")
		// Affected Application
		sb.WriteString("<tr><td class='hdr' colspan='2'>Affected Application</td></tr>")
		sb.WriteString("<tr><td colspan='2'>" + esc(f.AffectedSystem) + "</td></tr>")
		// PoC - raw HTML (may contain <img>)
		sb.WriteString("<tr><td class='hdr' colspan='2'>PoC</td></tr>")
		sb.WriteString("<tr><td colspan='2'><div class='poc'>" + f.POC + "</div></td></tr>")
		// Recommendation
		sb.WriteString("<tr><td class='hdr' colspan='2'>Recommendation</td></tr>")
		sb.WriteString("<tr><td colspan='2'>" + esc(id) + "&ndash; " + esc(f.Recommendation) + "</td></tr>")
		sb.WriteString("</table>")
	}

	// ---- 4 TOOLS AND LICENSES -------------------------------------------
	sb.WriteString("<div class='pagebreak'></div>")
	sb.WriteString("<h1>4 Tools and Licenses</h1>")
	sb.WriteString("<p>Cyberteq testers used below tools in the Vulnerability Assessment:</p>")
	sb.WriteString("<ul>")
	tools := config.Tools
	if len(tools) == 0 {
		tools = defaultTools
	}
	for _, t := range tools {
		sb.WriteString("<li>" + esc(t) + "</li>")
	}
	sb.WriteString("</ul>")

	// ---- Footer note -----------------------------------------------------
	sb.WriteString("<hr style='margin-top:40px;border:none;border-top:1px solid " + brandOrange + ";'>")
	sb.WriteString("<table style='margin-top:8px;'><tr>")
	sb.WriteString("<td style='background:#fff;border:none;'>Cyberteq Falcon Ltd.</td>")
	sb.WriteString("<td style='background:#fff;border:none;text-align:center;'>" + esc(config.EngagementName) + " &ndash; " + esc(config.CompanyName) + "</td>")
	sb.WriteString("<td style='background:#fff;border:none;text-align:right;'>Ref: " + esc(config.RefNumber) + "</td>")
	sb.WriteString("</tr><tr>")
	sb.WriteString("<td style='background:#fff;border:none;'>All rights reserved</td>")
	sb.WriteString("<td style='background:#fff;border:none;'></td>")
	sb.WriteString("<td style='background:#fff;border:none;'></td>")
	sb.WriteString("</tr></table>")

	sb.WriteString("</body></html>")

	if err := os.WriteFile(htmlPath, []byte(sb.String()), 0644); err != nil {
		return "", fmt.Errorf("failed to write HTML report: %w", err)
	}

	return "/api/v1/reports/download/docx/" + strings.TrimSuffix(filepath.Base(htmlPath), ".html"), nil
}

// ============================================================================
// PDF generation (gofpdf)
// ============================================================================

// vaptSeverityRGB returns the brand RGB color for a severity (for PDF).
func vaptSeverityRGB(sev string) (int, int, int) {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return 192, 0, 0
	case "high":
		return 255, 121, 0
	case "medium":
		return 255, 192, 0
	case "low":
		return 40, 167, 69
	case "info", "informational", "":
		return 47, 111, 237
	default:
		return 47, 111, 237
	}
}

// pdfSectionHeading writes an orange numbered section heading.
func pdfSectionHeading(pdf *gofpdf.Fpdf, size float64, text string) {
	pdf.SetFont("Helvetica", "B", size)
	pdf.SetTextColor(255, 121, 0)
	pdf.MultiCell(0, size*0.55, text, "", "L", false)
	pdf.Ln(2)
	pdf.SetTextColor(45, 55, 72)
}

// pdfParagraph writes a body paragraph.
func pdfParagraph(pdf *gofpdf.Fpdf, text string) {
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(45, 55, 72)
	pdf.MultiCell(0, 5.5, text, "", "L", false)
	pdf.Ln(2)
}

// pdfBullet writes a bulleted line.
func pdfBullet(pdf *gofpdf.Fpdf, text string) {
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(45, 55, 72)
	x := pdf.GetX()
	pdf.CellFormat(6, 5.5, "-", "", 0, "L", false, 0, "")
	pdf.SetX(x + 6)
	pdf.MultiCell(0, 5.5, text, "", "L", false)
}

// pdfOrangeHeaderRow writes a full-width orange header cell (stacked table style).
func pdfOrangeHeaderRow(pdf *gofpdf.Fpdf, w float64, text string) {
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetFillColor(255, 121, 0)
	pdf.SetTextColor(255, 255, 255)
	pdf.CellFormat(w, 7, text, "1", 1, "L", true, 0, "")
	pdf.SetTextColor(45, 55, 72)
}

// pdfBodyCell writes a light-orange body cell block using MultiCell.
func pdfBodyCell(pdf *gofpdf.Fpdf, w float64, text string) {
	if strings.TrimSpace(text) == "" {
		text = "-"
	}
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetFillColor(253, 238, 224)
	pdf.SetTextColor(45, 55, 72)
	pdf.MultiCell(w, 5.5, text, "1", "L", true)
}

// mcAt draws a bordered, filled MultiCell at position (x,y) and returns the
// Y coordinate at the end of the drawn block. It leaves the cursor at (x, endY)
// is NOT guaranteed - callers should reposition as needed. The current fill and
// text colors and font must be set by the caller beforehand.
func mcAt(pdf *gofpdf.Fpdf, w, lineH float64, text, border, align string, fill bool, x, y float64) float64 {
	if strings.TrimSpace(text) == "" {
		text = "-"
	}
	pdf.SetXY(x, y)
	pdf.MultiCell(w, lineH, text, border, align, fill)
	return pdf.GetY()
}

// GeneratePDF renders the full Cyberteq VAPT report structure.
func GeneratePDF(config ReportConfig, outputPath string) (string, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 25)
	pdf.SetMargins(20, 20, 20)

	nowDate := strings.ToUpper(time.Now().Format("02-Jan-2006"))

	// Footer (rendered on every page; cover page footer suppressed below).
	pdf.SetFooterFunc(func() {
		if pdf.PageNo() <= 1 {
			return
		}
		pdf.SetY(-18)
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(120, 120, 120)
		center := fmt.Sprintf("%s - %s", config.EngagementName, config.CompanyName)
		pageStr := fmt.Sprintf("Page %d", pdf.PageNo())
		// line 1
		pdf.CellFormat(57, 5, "Cyberteq Falcon Ltd.", "", 0, "L", false, 0, "")
		pdf.CellFormat(56, 5, center, "", 0, "C", false, 0, "")
		pdf.CellFormat(57, 5, pageStr, "", 1, "R", false, 0, "")
		// line 2
		pdf.CellFormat(57, 5, "All rights reserved", "", 0, "L", false, 0, "")
		pdf.CellFormat(56, 5, fmt.Sprintf("Ref: %s", config.RefNumber), "", 0, "C", false, 0, "")
		pdf.CellFormat(57, 5, "", "", 1, "R", false, 0, "")
	})

	// -----------------------------------------------------------------
	// COVER PAGE
	// -----------------------------------------------------------------
	pdf.AddPage()
	// orange banner
	pdf.SetFillColor(255, 121, 0)
	pdf.Rect(0, 0, 210, 45, "F")
	// client logo top-left
	if config.CompanyLogo != "" {
		logoBytes, err := base64.StdEncoding.DecodeString(config.CompanyLogo)
		if err == nil {
			img, format, derr := image.Decode(bytes.NewReader(logoBytes))
			if derr == nil {
				var buf bytes.Buffer
				imgType := "PNG"
				switch format {
				case "jpeg":
					_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95})
					imgType = "JPG"
				default:
					_ = png.Encode(&buf, img)
					imgType = "PNG"
				}
				name := "coverlogo"
				pdf.RegisterImageOptionsReader(name, gofpdf.ImageOptions{ImageType: imgType}, bytes.NewReader(buf.Bytes()))
				pdf.ImageOptions(name, 12, 8, 45, 0, false, gofpdf.ImageOptions{ImageType: imgType}, 0, "")
			}
		}
	}

	pdf.SetY(70)
	pdf.SetFont("Helvetica", "B", 30)
	pdf.SetTextColor(255, 121, 0)
	pdf.MultiCell(0, 13, "IT SECURITY CONSULTANCY", "", "L", false)
	pdf.Ln(4)
	pdf.SetFont("Helvetica", "", 15)
	pdf.SetTextColor(70, 70, 70)
	pdf.MultiCell(0, 8, fmt.Sprintf("%s - Vulnerability Assessment & Penetration Testing (VAPT) POC", config.CompanyName), "", "L", false)

	// version table
	versionLabel := config.VersionLabel
	if strings.TrimSpace(versionLabel) == "" {
		versionLabel = "Details to be Provided"
	}
	pdf.Ln(30)
	vColW := []float64{40, 35, 47.5, 47.5}
	// header
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetFillColor(255, 121, 0)
	pdf.SetTextColor(255, 255, 255)
	pdf.CellFormat(vColW[0], 8, "Version", "1", 0, "C", true, 0, "")
	pdf.CellFormat(vColW[1], 8, "Date", "1", 0, "C", true, 0, "")
	pdf.CellFormat(vColW[2], 8, "Authors", "1", 0, "C", true, 0, "")
	pdf.CellFormat(vColW[3], 8, "Approver", "1", 1, "C", true, 0, "")
	// body
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetFillColor(253, 238, 224)
	pdf.SetTextColor(45, 55, 72)
	y := pdf.GetY()
	x := pdf.GetX()
	rowH := 12.0
	mcAt(pdf, vColW[0], rowH/2, versionLabel, "1", "C", true, x, y)
	pdf.SetXY(x+vColW[0], y)
	mcAt(pdf, vColW[1], rowH/2, nowDate, "1", "C", true, x+vColW[0], y)
	pdf.SetXY(x+vColW[0]+vColW[1], y)
	mcAt(pdf, vColW[2], rowH/2, config.TesterName+"\nCybersecurity Expert", "1", "C", true, x+vColW[0]+vColW[1], y)
	pdf.SetXY(x+vColW[0]+vColW[1]+vColW[2], y)
	mcAt(pdf, vColW[3], rowH/2, config.ApproverName+"\n"+config.ApproverTitle, "1", "C", true, x+vColW[0]+vColW[1]+vColW[2], y)
	pdf.SetY(y + rowH)

	// -----------------------------------------------------------------
	// TABLE OF CONTENTS
	// -----------------------------------------------------------------
	pdf.AddPage()
	pdfSectionHeading(pdf, 20, "CONTENTS")
	tocEntries := []string{
		"1 Executive Summary",
		"    1.1 Introduction",
		"    1.2 Findings and Recommendations",
		"    1.3 Risk Assessment and Impact on Business",
		"2 Methodology and Scope",
		"    2.1 Cyberteq Security Assessment Methodology",
		"    2.2 Cyberteq Vulnerability Rating Guide",
		"    2.3 Scope",
		"    2.4 Out of Scope",
		"    2.5 Testing Methodologies",
		"    2.6 Test Cases",
		"3 Findings and Recommendations",
		"    3.1 Recommendations Naming Convention",
		"    3.2 Vulnerability Register",
		"4 Tools and Licenses",
	}
	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(45, 55, 72)
	for _, e := range tocEntries {
		pdf.CellFormat(0, 7, e, "", 1, "L", false, 0, "")
	}

	// -----------------------------------------------------------------
	// 1 EXECUTIVE SUMMARY
	// -----------------------------------------------------------------
	pdf.AddPage()
	pdfSectionHeading(pdf, 18, "1 Executive Summary")

	pdfSectionHeading(pdf, 14, "1.1 Introduction")
	pdfParagraph(pdf, defaultIntroduction(config))
	for _, para := range introBoilerplate {
		pdfParagraph(pdf, para)
	}

	total := len(config.Findings)
	pdfSectionHeading(pdf, 14, "1.2 Findings and Recommendations")
	pdfParagraph(pdf, fmt.Sprintf("During the security assessment of %s's infrastructure, %d issues of different "+
		"severity levels have been discovered, categorized, and reported upon. The following section summarizes the "+
		"distribution of the identified findings by their severity level.", config.CompanyName, total))

	pdfSectionHeading(pdf, 12, "1.2.1 Findings by Severity")
	counts := vaptCountBySeverity(config.Findings)
	// header
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetFillColor(255, 121, 0)
	pdf.SetTextColor(255, 255, 255)
	pdf.CellFormat(50, 8, "Severity", "1", 0, "C", true, 0, "")
	pdf.CellFormat(120, 8, "Count", "1", 1, "L", true, 0, "")
	for _, sev := range []string{"Critical", "High", "Medium", "Low", "Informational"} {
		sr, sg, sbb := vaptSeverityRGB(sev)
		pdf.SetFont("Helvetica", "B", 10)
		pdf.SetFillColor(sr, sg, sbb)
		if sev == "Medium" {
			pdf.SetTextColor(0, 0, 0)
		} else {
			pdf.SetTextColor(255, 255, 255)
		}
		pdf.CellFormat(50, 7, sev, "1", 0, "C", true, 0, "")
		// bar cell
		pdf.SetFillColor(253, 238, 224)
		pdf.SetTextColor(45, 55, 72)
		pdf.SetFont("Helvetica", "", 10)
		bar := strings.Repeat("|", counts[sev])
		pdf.CellFormat(120, 7, fmt.Sprintf("%d  %s", counts[sev], bar), "1", 1, "L", true, 0, "")
	}
	pdf.Ln(4)

	pdfSectionHeading(pdf, 14, "1.3 Risk Assessment and Impact on Business")
	pdfParagraph(pdf, fmt.Sprintf("The vulnerabilities identified during this assessment pose varying levels of risk "+
		"to %s. If left unremediated, these findings could be leveraged by malicious actors to compromise the "+
		"confidentiality, integrity, and availability of the organization's information assets, potentially resulting "+
		"in financial loss, reputational damage, and regulatory non-compliance. Cyberteq recommends prioritizing "+
		"remediation efforts according to the criticality of each finding described in this report.", config.CompanyName))

	// -----------------------------------------------------------------
	// 2 METHODOLOGY AND SCOPE
	// -----------------------------------------------------------------
	pdf.AddPage()
	pdfSectionHeading(pdf, 18, "2 Methodology and Scope")

	pdfSectionHeading(pdf, 14, "2.1 Cyberteq Security Assessment Methodology")
	pdfParagraph(pdf, "Cyberteq follows a structured, phased methodology to ensure comprehensive coverage during "+
		"the security assessment. The assessment is divided into the following four phases:")
	for i, ph := range methodologyPhases {
		pdfSectionHeading(pdf, 12, fmt.Sprintf("2.1.%d %s", i+1, ph[0]))
		pdfParagraph(pdf, ph[1])
	}

	pdfSectionHeading(pdf, 14, "2.2 Cyberteq Vulnerability Rating Guide")
	pdfParagraph(pdf, "Cyberteq rates the severity of each identified vulnerability using the Common Vulnerability "+
		"Scoring System (CVSS) version 3.1. The following metrics are considered when calculating the overall "+
		"severity of a finding:")
	for i, m := range ratingMetrics {
		pdfBullet(pdf, fmt.Sprintf("%d. %s", i+1, m))
	}
	pdf.Ln(2)

	pdfSectionHeading(pdf, 14, "2.3 Scope")
	pdfParagraph(pdf, fmt.Sprintf(scopeParagraph, config.AssessmentStart, config.AssessmentEnd))
	// scope table
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetFillColor(255, 121, 0)
	pdf.SetTextColor(255, 255, 255)
	pdf.CellFormat(70, 8, "Activity", "1", 0, "C", true, 0, "")
	pdf.CellFormat(100, 8, "Details", "1", 1, "C", true, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetFillColor(253, 238, 224)
	pdf.SetTextColor(45, 55, 72)
	scopeRows := config.Scope
	if len(scopeRows) == 0 {
		scopeRows = []string{"Web Application Testing"}
	}
	for _, s := range scopeRows {
		cy := pdf.GetY()
		cx := pdf.GetX()
		mcAt(pdf, 70, 6, s, "1", "L", true, cx, cy)
		pdf.SetXY(cx+70, cy)
		mcAt(pdf, 100, 6, "All activities were performed remotely.", "1", "L", true, cx+70, cy)
		pdf.SetY(cy + 6)
	}
	pdf.Ln(2)

	pdfSectionHeading(pdf, 14, "2.4 Out of Scope")
	if len(config.OutOfScope) > 0 {
		for _, s := range config.OutOfScope {
			pdfBullet(pdf, s)
		}
	} else {
		for _, s := range outOfScopeDefaults {
			pdfBullet(pdf, fmt.Sprintf("%s: %s", s[0], s[1]))
		}
	}
	pdf.Ln(2)

	pdfSectionHeading(pdf, 14, "2.5 Testing Methodologies")
	pdfSectionHeading(pdf, 12, "2.5.1 Web Application Testing Methodology")
	pdfParagraph(pdf, "The web application testing was conducted in accordance with the OWASP Testing Guide, focusing "+
		"on the most critical web application security risks. The following categories were assessed:")
	for _, m := range owaspMethodologyList {
		pdfBullet(pdf, m)
	}
	pdf.Ln(2)

	pdfSectionHeading(pdf, 14, "2.6 Test Cases")
	pdfSectionHeading(pdf, 12, "2.6.1 Web Application Testing")
	if len(config.TestCases) > 0 {
		// header
		pdf.SetFont("Helvetica", "B", 10)
		pdf.SetFillColor(255, 121, 0)
		pdf.SetTextColor(255, 255, 255)
		pdf.CellFormat(130, 8, "Test Type", "1", 0, "L", true, 0, "")
		pdf.CellFormat(40, 8, "Status", "1", 1, "C", true, 0, "")
		for _, grp := range config.TestCases {
			// group header row (gray)
			pdf.SetFont("Helvetica", "B", 10)
			pdf.SetFillColor(221, 221, 221)
			pdf.SetTextColor(0, 0, 0)
			pdf.CellFormat(170, 7, grp.Group, "1", 1, "L", true, 0, "")
			for _, c := range grp.Cases {
				pdf.SetFont("Helvetica", "", 10)
				pdf.SetFillColor(253, 238, 224)
				pdf.SetTextColor(45, 55, 72)
				cy := pdf.GetY()
				cx := pdf.GetX()
				mcAt(pdf, 130, 6, c.Name, "1", "L", true, cx, cy)
				endY := pdf.GetY()
				// status cell colored
				sr, sg, sbb := 108, 117, 125
				switch strings.ToLower(strings.TrimSpace(c.Status)) {
				case "pass":
					sr, sg, sbb = 40, 167, 69
				case "issues":
					sr, sg, sbb = 220, 53, 69
				}
				pdf.SetXY(cx+130, cy)
				pdf.SetTextColor(sr, sg, sbb)
				pdf.SetFont("Helvetica", "B", 10)
				pdf.CellFormat(40, endY-cy, c.Status, "1", 0, "C", true, 0, "")
				pdf.SetY(endY)
			}
		}
	}
	pdf.Ln(2)

	// -----------------------------------------------------------------
	// 3 FINDINGS AND RECOMMENDATIONS
	// -----------------------------------------------------------------
	pdf.AddPage()
	pdfSectionHeading(pdf, 18, "3 Findings and Recommendations")
	pdfParagraph(pdf, "This section presents the findings identified during the security assessment along with their "+
		"associated risk criticality and recommended remediation actions. The criticality of each finding is "+
		"classified according to the following criteria:")
	// criticality criteria table
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetFillColor(255, 121, 0)
	pdf.SetTextColor(255, 255, 255)
	pdf.CellFormat(45, 8, "Criticality", "1", 0, "C", true, 0, "")
	pdf.CellFormat(125, 8, "Description", "1", 1, "C", true, 0, "")
	for _, c := range criticalityCriteria {
		sr, sg, sbb := vaptSeverityRGB(c[0])
		pdf.SetFont("Helvetica", "B", 10)
		pdf.SetFillColor(sr, sg, sbb)
		if c[0] == "Medium" {
			pdf.SetTextColor(0, 0, 0)
		} else {
			pdf.SetTextColor(255, 255, 255)
		}
		cy := pdf.GetY()
		cx := pdf.GetX()
		pdf.CellFormat(45, 8, c[0], "1", 0, "C", true, 0, "")
		pdf.SetFillColor(253, 238, 224)
		pdf.SetTextColor(45, 55, 72)
		pdf.SetFont("Helvetica", "", 10)
		mcAt(pdf, 125, 8, c[2], "1", "L", true, cx+45, cy)
		pdf.SetY(cy + 8)
	}
	pdf.Ln(3)

	pdfSectionHeading(pdf, 14, "3.1 Recommendations Naming Convention")
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetFillColor(255, 121, 0)
	pdf.SetTextColor(255, 255, 255)
	pdf.CellFormat(45, 8, "Abbreviation", "1", 0, "C", true, 0, "")
	pdf.CellFormat(125, 8, "Meaning", "1", 1, "C", true, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetFillColor(253, 238, 224)
	pdf.SetTextColor(45, 55, 72)
	for _, n := range namingConvention {
		pdf.CellFormat(45, 7, n[0], "1", 0, "C", true, 0, "")
		pdf.CellFormat(125, 7, n[1], "1", 1, "L", true, 0, "")
	}
	pdf.Ln(2)
	pdfParagraph(pdf, "For example, REC3_WPT3 refers to the 3rd recommendation for the 3rd finding from web "+
		"penetration testing.")

	pdfSectionHeading(pdf, 14, "3.2 Vulnerability Register")
	// register table
	regCols := []float64{45, 22, 28, 30, 45}
	regHdr := []string{"Vulnerability", "Exposure", "Criticality", "Vulnerability ID", "Recommendation"}
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(255, 121, 0)
	pdf.SetTextColor(255, 255, 255)
	for i, h := range regHdr {
		nl := 0
		if i == len(regHdr)-1 {
			nl = 1
		}
		pdf.CellFormat(regCols[i], 8, h, "1", nl, "C", true, 0, "")
	}
	for i, f := range config.Findings {
		id := vulnID(f, i+1)
		cx := pdf.GetX()
		cy := pdf.GetY()
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetFillColor(253, 238, 224)
		pdf.SetTextColor(45, 55, 72)
		// vulnerability
		mcAt(pdf, regCols[0], 5, f.Title, "1", "L", true, cx, cy)
		h0 := pdf.GetY() - cy
		// exposure
		pdf.SetXY(cx+regCols[0], cy)
		mcAt(pdf, regCols[1], 5, exposureOf(f), "1", "L", true, cx+regCols[0], cy)
		h1 := pdf.GetY() - cy
		// criticality (colored)
		sr, sg, sbb := vaptSeverityRGB(f.Severity)
		pdf.SetXY(cx+regCols[0]+regCols[1], cy)
		pdf.SetFillColor(sr, sg, sbb)
		if severityDisplay(f.Severity) == "Medium" {
			pdf.SetTextColor(0, 0, 0)
		} else {
			pdf.SetTextColor(255, 255, 255)
		}
		pdf.SetFont("Helvetica", "B", 8)
		// determine row height as max of measured heights so far
		rh := h0
		if h1 > rh {
			rh = h1
		}
		pdf.CellFormat(regCols[2], rh, severityDisplay(f.Severity), "1", 0, "C", true, 0, "")
		// vuln id
		pdf.SetFillColor(253, 238, 224)
		pdf.SetTextColor(45, 55, 72)
		pdf.SetFont("Helvetica", "", 8)
		mcAt(pdf, regCols[3], 5, id, "1", "L", true, cx+regCols[0]+regCols[1]+regCols[2], cy)
		// recommendation
		pdf.SetXY(cx+regCols[0]+regCols[1]+regCols[2]+regCols[3], cy)
		mcAt(pdf, regCols[4], 5, truncateText(f.Recommendation, 80), "1", "L", true, cx+regCols[0]+regCols[1]+regCols[2]+regCols[3], cy)
		endY := pdf.GetY()
		if endY < cy+rh {
			endY = cy + rh
		}
		pdf.SetY(endY)
	}
	pdf.Ln(4)

	// per-finding detail blocks
	fullW := 170.0
	for i, f := range config.Findings {
		id := vulnID(f, i+1)
		pdfSectionHeading(pdf, 13, fmt.Sprintf("3.2.%d %s", i+1, f.Title))

		// Description header + two-column body (desc | rating)
		pdfOrangeHeaderRow(pdf, fullW, "Description")
		descW := 130.0
		rateW := 40.0
		cy := pdf.GetY()
		cx := pdf.GetX()
		pdf.SetFont("Helvetica", "", 10)
		pdf.SetFillColor(253, 238, 224)
		pdf.SetTextColor(45, 55, 72)
		descText := f.Description
		if strings.TrimSpace(descText) == "" {
			descText = "-"
		}
		mcAt(pdf, descW, 5.5, descText, "1", "L", true, cx, cy)
		descEnd := pdf.GetY()
		// rating cell
		sr, sg, sbb := vaptSeverityRGB(f.Severity)
		pdf.SetXY(cx+descW, cy)
		pdf.SetFillColor(sr, sg, sbb)
		if severityDisplay(f.Severity) == "Medium" {
			pdf.SetTextColor(0, 0, 0)
		} else {
			pdf.SetTextColor(255, 255, 255)
		}
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(rateW, descEnd-cy, "Rating: "+severityDisplay(f.Severity), "1", 0, "C", true, 0, "")
		pdf.SetY(descEnd)
		pdf.SetTextColor(45, 55, 72)

		// CVSS Vector String
		pdfOrangeHeaderRow(pdf, fullW, "CVSS Vector String")
		pdfBodyCell(pdf, fullW, f.CVSSVector)

		// Impact
		pdfOrangeHeaderRow(pdf, fullW, "Impact")
		pdfBodyCell(pdf, fullW, f.Impact)

		// Affected Application
		pdfOrangeHeaderRow(pdf, fullW, "Affected Application")
		pdfBodyCell(pdf, fullW, f.AffectedSystem)

		// PoC
		pdfOrangeHeaderRow(pdf, fullW, "PoC")
		pocText := f.POC
		if containsImage(pocText) {
			pocText = "[See PoC screenshot in DOCX version]"
		}
		pdfBodyCell(pdf, fullW, pocText)

		// Recommendation
		pdfOrangeHeaderRow(pdf, fullW, "Recommendation")
		pdfBodyCell(pdf, fullW, id+"- "+f.Recommendation)

		pdf.Ln(4)
	}

	// -----------------------------------------------------------------
	// 4 TOOLS AND LICENSES
	// -----------------------------------------------------------------
	pdf.AddPage()
	pdfSectionHeading(pdf, 18, "4 Tools and Licenses")
	pdfParagraph(pdf, "Cyberteq testers used below tools in the Vulnerability Assessment:")
	tools := config.Tools
	if len(tools) == 0 {
		tools = defaultTools
	}
	for _, t := range tools {
		pdfBullet(pdf, t)
	}

	if err := pdf.OutputFileAndClose(outputPath); err != nil {
		return "", fmt.Errorf("failed to save PDF: %w", err)
	}

	return "/api/v1/reports/download/pdf/" + strings.TrimSuffix(filepath.Base(outputPath), ".pdf"), nil
}
