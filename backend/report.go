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
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jung-kurt/gofpdf"
)

type ReportConfig struct {
	CompanyName    string          `json:"company_name"`
	CompanyLogo    string          `json:"company_logo"`
	EngagementName string          `json:"engagement_name"`
	ClientEmail    string          `json:"client_email"`
	Scope          []string        `json:"scope"`
	Sections       []string        `json:"sections"`
	Findings       []ReportFinding `json:"findings"`
}

type ReportFinding struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	Impact         string `json:"impact"`
	CVSSVector     string `json:"cvss_vector"`
	CVSSScore      string `json:"cvss_score"`
	Severity       string `json:"severity"`
	AffectedSystem string `json:"affected_system"`
	POC            string `json:"poc"`
	Recommendation string `json:"recommendation"`
	Category       string `json:"category"`
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

func GenerateDOCX(config ReportConfig, outputPath string) (string, error) {
	htmlPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".html"

	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html><html><head><meta charset='utf-8'>")
	sb.WriteString("<style>")
	sb.WriteString("body{font-family:Calibri,Arial,sans-serif;margin:40px;}")
	sb.WriteString("h1{color:#1B2A4A;border-bottom:2px solid #FF7900;padding-bottom:8px;}")
	sb.WriteString("h2{color:#1B2A4A;margin-top:24px;}")
	sb.WriteString("h3{color:#4A5568;}")
	sb.WriteString("table{border-collapse:collapse;width:100%;margin:12px 0;}")
	sb.WriteString("th,td{border:1px solid #ddd;padding:8px;text-align:left;}")
	sb.WriteString("th{background:#1B2A4A;color:white;}")
	sb.WriteString(".critical{background:#f8d7da;color:#721c24;padding:2px 8px;border-radius:4px;}")
	sb.WriteString(".high{background:#fff3cd;color:#856404;padding:2px 8px;border-radius:4px;}")
	sb.WriteString(".medium{background:#fff3cd;color:#856404;padding:2px 8px;border-radius:4px;}")
	sb.WriteString(".low{background:#d4edda;color:#155724;padding:2px 8px;border-radius:4px;}")
	sb.WriteString(".confidential{text-align:center;color:#C53030;font-weight:bold;font-size:18px;margin:20px 0;}")
	sb.WriteString("</style></head><body>")

	sb.WriteString("<div style='text-align:center;margin-top:100px;'>")
	if config.CompanyLogo != "" {
		sb.WriteString(fmt.Sprintf("<img src='data:image/png;base64,%s' style='max-width:300px;'><br><br>", config.CompanyLogo))
	}
	sb.WriteString(fmt.Sprintf("<h1 style='font-size:36px;'>%s</h1>", html.EscapeString(config.CompanyName)))
	sb.WriteString(fmt.Sprintf("<h2 style='font-size:24px;color:#4A5568;'>%s</h2>", html.EscapeString(config.EngagementName)))
	sb.WriteString(fmt.Sprintf("<p style='color:#718096;'>%s</p>", time.Now().Format("January 2, 2006")))
	sb.WriteString("<div class='confidential'>CONFIDENTIAL</div>")
	sb.WriteString("</div>")

	sb.WriteString("<div style='page-break-before:always;'></div>")
	sb.WriteString("<h1>1. Executive Summary</h1>")
	counts := countBySeverity(config.Findings)
	total := len(config.Findings)
	sb.WriteString(fmt.Sprintf("<p>This report presents the findings of a security assessment conducted for %s. A total of %d findings were identified.</p>", html.EscapeString(config.CompanyName), total))
	sb.WriteString("<table><tr><th>Severity</th><th>Count</th></tr>")
	for _, sev := range []string{"critical", "high", "medium", "low"} {
		if counts[sev] > 0 {
			sb.WriteString(fmt.Sprintf("<tr><td><span class='%s'>%s</span></td><td>%d</td></tr>", sev, strings.Title(sev), counts[sev]))
		}
	}
	sb.WriteString("</table>")

	findingNum := 1
	for _, finding := range config.Findings {
		sb.WriteString(fmt.Sprintf("<h2>%d. %s</h2>", findingNum+2, html.EscapeString(finding.Title)))
		sb.WriteString(fmt.Sprintf("<p><strong>Severity:</strong> <span class='%s'>%s</span></p>", strings.ToLower(finding.Severity), strings.Title(finding.Severity)))
		if finding.CVSSVector != "" {
			sb.WriteString(fmt.Sprintf("<p><strong>CVSS:</strong> %s</p>", html.EscapeString(finding.CVSSVector)))
		}
		if finding.AffectedSystem != "" {
			sb.WriteString(fmt.Sprintf("<p><strong>Affected System:</strong> %s</p>", html.EscapeString(finding.AffectedSystem)))
		}
		sb.WriteString(fmt.Sprintf("<h3>Description</h3><p>%s</p>", html.EscapeString(finding.Description)))
		if finding.Impact != "" {
			sb.WriteString(fmt.Sprintf("<h3>Impact</h3><p>%s</p>", html.EscapeString(finding.Impact)))
		}
		if finding.POC != "" {
			sb.WriteString(fmt.Sprintf("<h3>Proof of Concept</h3><div>%s</div>", finding.POC))
		}
		if finding.Recommendation != "" {
			sb.WriteString(fmt.Sprintf("<h3>Recommendation</h3><p>%s</p>", html.EscapeString(finding.Recommendation)))
		}
		findingNum++
	}

	sb.WriteString("</body></html>")

	if err := os.WriteFile(htmlPath, []byte(sb.String()), 0644); err != nil {
		return "", fmt.Errorf("failed to write HTML report: %w", err)
	}

	return "/api/v1/reports/download/docx/" + strings.TrimSuffix(filepath.Base(htmlPath), ".html"), nil
}

func addPDFCoverPage(pdf *gofpdf.Fpdf, config ReportConfig) {
	pdf.AddPage()
	pdf.SetMargins(20, 20, 20)
	pdf.Ln(50)

	if config.CompanyLogo != "" {
		logoBytes, err := base64.StdEncoding.DecodeString(config.CompanyLogo)
		if err == nil {
			imgReader := bytes.NewReader(logoBytes)
			img, format, err := image.Decode(imgReader)
			if err == nil {
				tmpPath := filepath.Join(os.TempDir(), "report_logo_tmp.png")
				if format == "jpeg" {
					tmpPath = filepath.Join(os.TempDir(), "report_logo_tmp.jpg")
				}
				tmpFile, err := os.Create(tmpPath)
				if err == nil {
					if format == "jpeg" {
						jpeg.Encode(tmpFile, img.(*image.YCbCr), &jpeg.Options{Quality: 95})
					} else {
						png.Encode(tmpFile, img)
					}
					tmpFile.Close()
					defer os.Remove(tmpPath)
					pdf.Image(tmpPath, 75, 30, 60, 0, false, "", 0, "")
					pdf.Ln(70)
				}
			}
		}
	}

	pdf.SetFont("Helvetica", "B", 28)
	pdf.SetTextColor(27, 42, 74)
	pdf.CellFormat(0, 15, config.CompanyName, "", 1, "C", false, 0, "")
	pdf.Ln(5)

	pdf.SetFont("Helvetica", "", 18)
	pdf.SetTextColor(74, 85, 104)
	pdf.CellFormat(0, 12, config.EngagementName, "", 1, "C", false, 0, "")
	pdf.Ln(10)

	pdf.SetFont("Helvetica", "", 14)
	pdf.SetTextColor(113, 128, 150)
	pdf.CellFormat(0, 10, time.Now().Format("January 2, 2006"), "", 1, "C", false, 0, "")
	pdf.Ln(10)

	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetTextColor(197, 48, 48)
	pdf.CellFormat(0, 10, "CONFIDENTIAL", "", 1, "C", false, 0, "")

	if config.ClientEmail != "" {
		pdf.Ln(5)
		pdf.SetFont("Helvetica", "", 11)
		pdf.SetTextColor(113, 128, 150)
		pdf.CellFormat(0, 10, fmt.Sprintf("Prepared for: %s", config.ClientEmail), "", 1, "C", false, 0, "")
	}
}

func addPDFTableOfContents(pdf *gofpdf.Fpdf, config ReportConfig) {
	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 24)
	pdf.SetTextColor(27, 42, 74)
	pdf.CellFormat(0, 15, "Table of Contents", "", 1, "L", false, 0, "")
	pdf.Ln(5)

	pdf.SetDrawColor(27, 42, 74)
	pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
	pdf.Ln(5)

	entries := []string{
		"1. Executive Summary",
		"2. Scope",
	}
	for i, sec := range config.Sections {
		entries = append(entries, fmt.Sprintf("%d. %s Findings", i+3, sec))
	}
	nextNum := len(config.Sections) + 3
	entries = append(entries, fmt.Sprintf("%d. Risk Summary", nextNum))
	entries = append(entries, fmt.Sprintf("%d. Recommendations", nextNum+1))

	for _, entry := range entries {
		pdf.SetFont("Helvetica", "", 13)
		pdf.SetTextColor(45, 55, 72)
		pdf.CellFormat(0, 9, entry, "", 1, "L", false, 0, "")
	}
}

func addPDFExecutiveSummary(pdf *gofpdf.Fpdf, config ReportConfig) {
	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 24)
	pdf.SetTextColor(27, 42, 74)
	pdf.CellFormat(0, 15, "1. Executive Summary", "", 1, "L", false, 0, "")
	pdf.Ln(3)

	pdf.SetDrawColor(27, 42, 74)
	pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
	pdf.Ln(5)

	counts := countBySeverity(config.Findings)
	total := len(config.Findings)

	summaryText := fmt.Sprintf(
		"This report presents the findings of a security assessment conducted for %s. "+
			"The assessment evaluated the security posture of the organization's infrastructure "+
			"across the defined scope. A total of %d findings were identified during the assessment.",
		config.CompanyName, total,
	)

	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(45, 55, 72)
	pdf.MultiCell(0, 6, summaryText, "", "L", false)
	pdf.Ln(5)

	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetTextColor(45, 55, 72)
	pdf.CellFormat(0, 8, "Risk Breakdown:", "", 1, "L", false, 0, "")
	pdf.Ln(2)

	severityOrder := []string{"critical", "high", "medium", "low", "info"}
	for _, sev := range severityOrder {
		if counts[sev] > 0 {
			r, g, b := severityColor(sev)
			pdf.SetFont("Helvetica", "", 11)
			pdf.SetTextColor(r, g, b)
			label := titleCase(sev)
			if sev == "info" {
				label = "Informational"
			}
			pdf.CellFormat(10, 7, "", "", 0, "L", false, 0, "")
			pdf.CellFormat(0, 7, fmt.Sprintf("  %s: %d", label, counts[sev]), "", 1, "L", false, 0, "")
		}
	}

	if counts["critical"] > 0 || counts["high"] > 0 {
		pdf.Ln(5)
		pdf.SetFont("Helvetica", "B", 11)
		pdf.SetTextColor(197, 48, 48)
		alertText := fmt.Sprintf(
			"The assessment identified %d critical and %d high severity findings that require immediate attention.",
			counts["critical"], counts["high"],
		)
		pdf.MultiCell(0, 6, alertText, "", "L", false)
	}
}

func addPDFScopeSection(pdf *gofpdf.Fpdf, config ReportConfig) {
	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 24)
	pdf.SetTextColor(27, 42, 74)
	pdf.CellFormat(0, 15, "2. Scope", "", 1, "L", false, 0, "")
	pdf.Ln(3)

	pdf.SetDrawColor(27, 42, 74)
	pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
	pdf.Ln(5)

	if len(config.Scope) > 0 {
		pdf.SetFont("Helvetica", "B", 12)
		pdf.SetTextColor(45, 55, 72)
		pdf.CellFormat(0, 8, "In-Scope Targets:", "", 1, "L", false, 0, "")
		pdf.Ln(2)

		pdf.SetFont("Helvetica", "", 11)
		for _, item := range config.Scope {
			pdf.CellFormat(10, 7, "", "", 0, "L", false, 0, "")
			pdf.CellFormat(0, 7, fmt.Sprintf("  *  %s", item), "", 1, "L", false, 0, "")
		}
	}

	if len(config.Sections) > 0 {
		pdf.Ln(5)
		pdf.SetFont("Helvetica", "B", 12)
		pdf.SetTextColor(45, 55, 72)
		pdf.CellFormat(0, 8, "Assessment Types:", "", 1, "L", false, 0, "")
		pdf.Ln(2)

		pdf.SetFont("Helvetica", "", 11)
		for _, sec := range config.Sections {
			pdf.CellFormat(10, 7, "", "", 0, "L", false, 0, "")
			pdf.CellFormat(0, 7, fmt.Sprintf("  *  %s", sec), "", 1, "L", false, 0, "")
		}
	}
}

func addPDFFindingBlock(pdf *gofpdf.Fpdf, categoryNum, findingNum int, finding ReportFinding) {
	pdf.Ln(5)

	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetTextColor(45, 55, 72)
	pdf.CellFormat(0, 10, fmt.Sprintf("%d.%d %s", categoryNum, findingNum, finding.Title), "", 1, "L", false, 0, "")
	pdf.Ln(1)

	r, g, b := severityColor(finding.Severity)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(r, g, b)
	pdf.CellFormat(0, 7, fmt.Sprintf("Severity: %s", strings.ToUpper(finding.Severity)), "", 1, "L", false, 0, "")

	if finding.CVSSScore != "" {
		pdf.SetFont("Helvetica", "", 10)
		pdf.SetTextColor(74, 85, 104)
		pdf.CellFormat(0, 7, fmt.Sprintf("CVSS Score: %s", finding.CVSSScore), "", 1, "L", false, 0, "")
	}

	pdf.Ln(2)

	if finding.Description != "" {
		pdf.SetFont("Helvetica", "B", 11)
		pdf.SetTextColor(45, 55, 72)
		pdf.CellFormat(0, 7, "Description:", "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
		pdf.MultiCell(0, 5.5, finding.Description, "", "L", false)
		pdf.Ln(2)
	}

	if finding.Impact != "" {
		pdf.SetFont("Helvetica", "B", 11)
		pdf.SetTextColor(45, 55, 72)
		pdf.CellFormat(0, 7, "Impact:", "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
		pdf.MultiCell(0, 5.5, finding.Impact, "", "L", false)
		pdf.Ln(2)
	}

	if finding.CVSSVector != "" {
		pdf.SetFont("Helvetica", "B", 11)
		pdf.SetTextColor(45, 55, 72)
		pdf.CellFormat(0, 7, "CVSS Vector:", "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
		pdf.MultiCell(0, 5.5, finding.CVSSVector, "", "L", false)
		pdf.Ln(2)
	}

	if finding.AffectedSystem != "" {
		pdf.SetFont("Helvetica", "B", 11)
		pdf.SetTextColor(45, 55, 72)
		pdf.CellFormat(0, 7, "Affected System:", "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
		pdf.MultiCell(0, 5.5, finding.AffectedSystem, "", "L", false)
		pdf.Ln(2)
	}

	if finding.POC != "" {
		pdf.SetFont("Helvetica", "B", 11)
		pdf.SetTextColor(45, 55, 72)
		pdf.CellFormat(0, 7, "Proof of Concept:", "", 1, "L", false, 0, "")
		pdf.Ln(1)
		pdf.SetFillColor(240, 240, 240)
		pdf.SetFont("Courier", "", 9)
		pdf.SetTextColor(74, 85, 104)
		pdf.MultiCell(0, 5, finding.POC, "", "L", false)
		pdf.Ln(2)
	}

	if finding.Recommendation != "" {
		pdf.SetFont("Helvetica", "B", 11)
		pdf.SetTextColor(45, 55, 72)
		pdf.CellFormat(0, 7, "Recommendation:", "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
		pdf.MultiCell(0, 5.5, finding.Recommendation, "", "L", false)
		pdf.Ln(2)
	}

	pdf.SetDrawColor(200, 200, 200)
	pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
}

func addPDFFindingsSection(pdf *gofpdf.Fpdf, config ReportConfig, categoryStartNum int) {
	grouped := groupFindingsByCategory(config.Findings)
	categories := make([]string, 0, len(grouped))
	for cat := range grouped {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	for catIdx, category := range categories {
		pdf.AddPage()

		pdf.SetFont("Helvetica", "B", 24)
		pdf.SetTextColor(27, 42, 74)
		pdf.CellFormat(0, 15, fmt.Sprintf("%d. %s Findings", categoryStartNum+catIdx, category), "", 1, "L", false, 0, "")
		pdf.Ln(3)

		pdf.SetDrawColor(27, 42, 74)
		pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
		pdf.Ln(5)

		for findingNum, finding := range grouped[category] {
			addPDFFindingBlock(pdf, categoryStartNum+catIdx, findingNum+1, finding)
		}
	}
}

func addPDFRiskSummaryTable(pdf *gofpdf.Fpdf, config ReportConfig) {
	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 24)
	pdf.SetTextColor(27, 42, 74)
	pdf.CellFormat(0, 15, fmt.Sprintf("%d. Risk Summary", len(config.Sections)+3), "", 1, "L", false, 0, "")
	pdf.Ln(3)

	pdf.SetDrawColor(27, 42, 74)
	pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
	pdf.Ln(8)

	counts := countBySeverity(config.Findings)
	total := len(config.Findings)

	colW := []float64{60, 50, 50}
	tableW := float64(0)
	for _, w := range colW {
		tableW += w
	}
	startX := (210 - tableW) / 2

	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetFillColor(27, 42, 74)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetX(startX)
	pdf.CellFormat(colW[0], 10, "Severity", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colW[1], 10, "Count", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colW[2], 10, "Percentage", "1", 1, "C", true, 0, "")

	severityOrder := []string{"critical", "high", "medium", "low", "info"}
	for i, sev := range severityOrder {
		count := counts[sev]
		if count == 0 {
			continue
		}

		sr, sg, sb := severityColor(sev)
		fr, fg, fb := severityFillColor(sev)

		if i%2 == 0 {
			pdf.SetFillColor(fr, fg, fb)
		} else {
			pdf.SetFillColor(255, 255, 255)
		}

		pdf.SetFont("Helvetica", "B", 11)
		pdf.SetTextColor(sr, sg, sb)
		pdf.SetX(startX)
		pdf.CellFormat(colW[0], 10, strings.ToUpper(sev), "1", 0, "C", true, 0, "")

		pdf.SetFont("Helvetica", "", 11)
		pdf.SetTextColor(45, 55, 72)
		pdf.CellFormat(colW[1], 10, fmt.Sprintf("%d", count), "1", 0, "C", true, 0, "")

		pct := 0.0
		if total > 0 {
			pct = float64(count) / float64(total) * 100
		}
		pdf.CellFormat(colW[2], 10, fmt.Sprintf("%.1f%%", pct), "1", 1, "C", true, 0, "")
	}

	pdf.Ln(5)
	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetTextColor(45, 55, 72)
	pdf.CellFormat(0, 8, fmt.Sprintf("Total Findings: %d", total), "", 1, "L", false, 0, "")
}

func addPDFRecommendationsSection(pdf *gofpdf.Fpdf, config ReportConfig) {
	pdf.AddPage()

	recNum := len(config.Sections) + 4

	pdf.SetFont("Helvetica", "B", 24)
	pdf.SetTextColor(27, 42, 74)
	pdf.CellFormat(0, 15, fmt.Sprintf("%d. Recommendations", recNum), "", 1, "L", false, 0, "")
	pdf.Ln(3)

	pdf.SetDrawColor(27, 42, 74)
	pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
	pdf.Ln(5)

	generalRecs := []string{
		"Implement a comprehensive vulnerability management program with regular scanning cycles.",
		"Establish and enforce secure coding practices across all development teams.",
		"Deploy web application firewalls (WAF) to provide additional protection for internet-facing applications.",
		"Implement network segmentation to limit lateral movement in case of a breach.",
		"Conduct regular penetration testing and security assessments.",
		"Maintain an up-to-date asset inventory to ensure comprehensive coverage.",
		"Implement multi-factor authentication (MFA) for all administrative access.",
		"Establish an incident response plan and conduct regular tabletop exercises.",
	}

	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(45, 55, 72)

	for _, rec := range generalRecs {
		pdf.CellFormat(10, 7, "", "", 0, "L", false, 0, "")
		pdf.MultiCell(0, 6, fmt.Sprintf("*  %s", rec), "", "L", false)
		pdf.Ln(2)
	}

	pdf.Ln(10)
	pdf.SetFont("Helvetica", "I", 11)
	pdf.SetTextColor(113, 128, 150)
	pdf.CellFormat(0, 10, "--- End of Report ---", "", 1, "C", false, 0, "")
}

func GeneratePDF(config ReportConfig, outputPath string) (string, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 20)
	pdf.SetMargins(20, 20, 20)

	addPDFCoverPage(pdf, config)
	addPDFTableOfContents(pdf, config)
	addPDFExecutiveSummary(pdf, config)
	addPDFScopeSection(pdf, config)
	addPDFFindingsSection(pdf, config, 3)
	addPDFRiskSummaryTable(pdf, config)
	addPDFRecommendationsSection(pdf, config)

	if err := pdf.OutputFileAndClose(outputPath); err != nil {
		return "", fmt.Errorf("failed to save PDF: %w", err)
	}

	return "/api/v1/reports/download/pdf/" + strings.TrimSuffix(filepath.Base(outputPath), ".pdf"), nil
}
