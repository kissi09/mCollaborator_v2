package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type ReportConfig struct {
	CompanyName     string          `json:"company_name"`     // Client company name -> [Company Name]
	CompanyInitials string          `json:"company_initials"` // -> [Company Initials], derived when blank
	CompanyLogo     string          `json:"company_logo"`     // Client logo (base64) dropped into the header slot
	EngagementName  string          `json:"engagement_name"`  // e.g. "VAPT Report"
	ClientEmail     string          `json:"client_email"`
	RefNumber       string          `json:"ref_number"`       // e.g. GH-REP-035-26059-01 -> [Reference Number]
	ReportDate      string          `json:"report_date"`      // -> [Date] on the cover
	AssessmentStart string          `json:"assessment_start"` // e.g. "17th June 2026"
	AssessmentEnd   string          `json:"assessment_end"`   // e.g. "24th June 2026"
	TesterName      string          `json:"tester_name"`
	ApproverName    string          `json:"approver_name"`
	ApproverTitle   string          `json:"approver_title"` // -> [Role]
	VersionLabel    string          `json:"version_label"`
	Introduction    string          `json:"introduction"` // Custom exec summary intro (optional)
	Scope           []string        `json:"scope"`
	OutOfScope      []string        `json:"out_of_scope"`
	Areas           []ReportArea    `json:"areas"`    // Assessment areas in scope, each with its scope text
	Sections        []string        `json:"sections"` // Legacy area selection (wpt, ept, ipt, nar)
	Findings        []ReportFinding `json:"findings"`
	Tools           []string        `json:"tools"`

	// Appendix test accounts. When both lists are empty the appendix is dropped
	// from the report entirely rather than printed with blank credential rows.
	TestAccountsExisting []TestAccount `json:"test_accounts_existing"`
	TestAccountsCreated  []TestAccount `json:"test_accounts_created"`

	// SyncOneDrive pushes the generated DOCX and PDF into the Cyberteq OneDrive
	// once the report is built. OneDriveFolder is the destination folder inside
	// that drive; blank falls back to OD_FOLDER, then to "VAPT Reports".
	SyncOneDrive   bool   `json:"sync_onedrive"`
	OneDriveFolder string `json:"onedrive_folder"`
}

// ReportArea is one assessment area the engagement covered, e.g. WPT with the
// applications that were tested. Code matches an entry in reportAreas.
type ReportArea struct {
	Code  string `json:"code"`
	Scope string `json:"scope"`
}

// TestAccount is one appendix row tying an account to the credentials used.
type TestAccount struct {
	Account     string `json:"account"`
	Credentials string `json:"credentials"`
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
	Category       string `json:"category"` // web, external, internal, architecture
	Exposure       string `json:"exposure"` // Web, External, Internal, etc.
	VulnID         string `json:"vuln_id"`  // REC1_WPT1 etc. (auto-generated if empty)

	// Area is the assessment area this finding is reported under (IPT, EPT,
	// IPTC, WPT, CFG, ASA, ADT, WNA, NAR). It decides which chapter 3 section
	// the finding prints in and which bar of the findings-by-area chart it
	// counts towards. Falls back to Category for engagements predating areas.
	Area string `json:"area"`

	// AttackVector fills the template's "Attack Vector" cell; when empty it is
	// derived from the CVSS vector string.
	AttackVector string `json:"attack_vector"`

	// RecommendationHeader is the short title printed next to the vulnerability
	// id, e.g. "Enforce TLS 1.2 or higher". Derived from Recommendation when
	// left blank.
	RecommendationHeader string `json:"recommendation_header"`

	// POCEvidenceIDs are evidence records attached to the finding as PoC
	// screenshots. The export handler resolves them to raw bytes before the
	// DOCX/PDF renderers run.
	POCEvidenceIDs []string   `json:"poc_evidence_ids,omitempty"`
	POCImages      []POCImage `json:"-"`
}

// POCImage is a resolved PoC screenshot ready for embedding into a document.
type POCImage struct {
	Data     []byte
	Filename string
	MimeType string
}

type exportReportResponse struct {
	DOCXURL string `json:"docx_url"`
	PDFURL  string `json:"pdf_url,omitempty"`

	// PDFError explains why no PDF was produced. The DOCX is still returned:
	// it is the template-accurate artefact, and shipping an approximation of it
	// under the same name is worse than shipping nothing.
	PDFError string `json:"pdf_error,omitempty"`

	// UnmatchedFindings names findings the test-type checklist could not account
	// for. Those rows read Pass, so the register reports a vulnerability the
	// checklist does not - worth a person's eyes before the report goes out.
	UnmatchedFindings []string `json:"unmatched_findings,omitempty"`

	// OneDrive sync outcome: "ok", "failed" or "not_configured".
	ODStatus   string `json:"od_status,omitempty"`
	ODDOCXLink string `json:"od_docx_link,omitempty"`
	ODPDFLink  string `json:"od_pdf_link,omitempty"`
	ODFolder   string `json:"od_folder,omitempty"`
	ODError    string `json:"od_error,omitempty"`
}

func ensureReportsDir() (string, error) {
	dir := filepath.Join("backend", "reports")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create reports directory: %w", err)
	}
	return dir, nil
}

// reservedFilenameChars are the characters Windows forbids in a file name.
// Graph rejects the same set for a drive item.
const reservedFilenameChars = `<>:"/\|?*`

// sanitizeFilename strips the characters Windows and Graph will not accept in a
// file name, and collapses the whitespace that leaves behind.
//
// It deliberately keeps spaces, '&' and the other ordinary punctuation a client
// name contains: the report is named for a person to read, not for a shell.
func sanitizeFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(reservedFilenameChars, r) {
			return ' '
		}
		return r
	}, name)
	name = strings.Join(strings.Fields(name), " ")
	name = strings.Trim(name, " .")
	if len(name) > 120 {
		name = strings.TrimSpace(name[:120])
	}
	return name
}

// reportBaseName is the delivered file's name, without extension:
//
//	<Reference Number> - <Company Name> - VAPT
//
// Either half can be missing on a draft, so each is dropped rather than left as
// an empty gap, and a report with neither falls back to a timestamp so two
// unnamed drafts cannot overwrite each other.
func reportBaseName(config ReportConfig) string {
	var parts []string
	if ref := strings.TrimSpace(config.RefNumber); ref != "" {
		parts = append(parts, ref)
	}
	if company := strings.TrimSpace(config.CompanyName); company != "" {
		parts = append(parts, company)
	}
	if len(parts) == 0 {
		parts = append(parts, "Untitled "+time.Now().Format("20060102 150405"))
	}
	parts = append(parts, "VAPT")
	return sanitizeFilename(strings.Join(parts, " - "))
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
		normalizeReportAreas(&config)

		// Resolve PoC screenshot evidence IDs to raw bytes for embedding.
		// Missing/failed lookups are logged and skipped - the report must not
		// fail just because one attachment is unavailable.
		uploadsDir, uErr := ensureUploadsDir()
		for i := range config.Findings {
			ids := config.Findings[i].POCEvidenceIDs
			if len(ids) == 0 {
				continue
			}
			for _, id := range ids {
				ev, err := store.GetEvidence(id)
				if err != nil {
					log.Printf("report: skip PoC evidence %s (%v)", id, err)
					continue
				}
				if uErr != nil {
					log.Printf("report: uploads dir unavailable: %v", uErr)
					continue
				}
				filePath := filepath.Join(uploadsDir, filepath.Base(ev.StorageKey))
				data, err := os.ReadFile(filePath)
				if err != nil {
					log.Printf("report: skip PoC evidence %s file missing on disk: %v", id, err)
					continue
				}
				config.Findings[i].POCImages = append(config.Findings[i].POCImages, POCImage{
					Data: data, Filename: ev.Filename, MimeType: ev.MimeType,
				})
			}
		}

		reportsDir, err := ensureReportsDir()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ApiResponse{
				Error: &ApiError{Code: "INTERNAL_ERROR", Message: err.Error()},
			})
			return
		}

		// "<Reference Number> - <Company Name> - VAPT". Regenerating a report
		// for the same reference replaces it rather than leaving a pile of
		// timestamped near-duplicates for someone to pick the right one out of.
		baseName := reportBaseName(config)

		docxPath := filepath.Join(reportsDir, baseName+".docx")
		pdfPath := filepath.Join(reportsDir, baseName+".pdf")

		docxURL, notes, err := GenerateDOCX(config, docxPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ApiResponse{
				Error: &ApiError{Code: "DOCX_GENERATION_FAILED", Message: fmt.Sprintf("Failed to generate DOCX: %s", err.Error())},
			})
			return
		}

		// The PDF is laid out by a real Word engine so that it is the template
		// rather than an approximation of it. When no engine is installed there
		// is no PDF at all - see generatePDFFromDOCX.
		pdfURL, engine, pdfErr := generatePDFFromDOCX(docxPath, pdfPath)
		if pdfErr != nil {
			os.Remove(pdfPath)
			log.Printf("report: generated %s (DOCX only - no PDF: %v)", baseName, pdfErr)
		} else {
			log.Printf("report: generated %s (pdf engine: %s)", baseName, engine)
		}

		// Best-effort audit log (may not have user if unauthenticated)
		if user, ok := r.Context().Value(userContextKey).(*User); ok && user != nil {
			store.AddAuditLog(&AuditLog{
				OrgID: user.OrgID, ActorID: user.ID,
				Action: "report.export", Resource: "report",
				ResourceID: baseName, IPAddress: r.RemoteAddr,
			})
		}

		response := exportReportResponse{
			DOCXURL: docxURL,
			PDFURL:  pdfURL,
		}
		if pdfErr != nil {
			response.PDFError = pdfErr.Error()
		}
		if notes != nil && len(notes.UnmatchedFindings) > 0 {
			response.UnmatchedFindings = notes.UnmatchedFindings
			log.Printf("report: %s - %d finding(s) matched no test in their area's checklist: %s",
				baseName, len(notes.UnmatchedFindings), strings.Join(notes.UnmatchedFindings, "; "))
		}

		if config.SyncOneDrive {
			syncReportToOneDrive(r.Context(), config, docxPath, pdfPath, pdfErr == nil, &response)
		}

		writeJSON(w, http.StatusOK, ApiResponse{Data: response})
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

	// Report names are ordinary prose now - "GH-REP-035-26040-01 - Axxend
	// Corporation - VAPT" - so the name is taken as a leaf and nothing more,
	// rather than as a path the caller gets to steer.
	leaf := filepath.Base(filepath.Clean("/" + reportName))
	if leaf == "." || leaf == string(filepath.Separator) {
		http.Error(w, "Invalid report name", http.StatusBadRequest)
		return
	}

	filePath := filepath.Join(reportsDir, leaf+"."+reportType)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "Report not found", http.StatusNotFound)
		return
	}

	if reportType == "pdf" {
		w.Header().Set("Content-Type", "application/pdf")
	} else if reportType == "docx" {
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	}
	// Quoted, because the name has spaces in it.
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q", leaf+"."+reportType))
	http.ServeFile(w, r, filePath)
}

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

// ============================================================================
// DOCX (Word-compatible HTML) generation
// ============================================================================

// generatePDFFromDOCX converts the already-merged DOCX to PDF with a real Word
// layout engine, and reports which engine was used.
//
// There is deliberately no fallback renderer here. Redrawing the report with a
// PDF primitive library produces a document that shares nothing with the
// template but its words - no cover, no charts, no landscape section, no
// headers - and handing that over under the same filename as the real export is
// worse than handing over nothing: it looks like the report until someone
// reads it. When no engine is available the caller returns the DOCX on its own
// and says why the PDF is missing.
func generatePDFFromDOCX(docxPath, pdfPath string) (downloadURL, engine string, err error) {
	engine, err = convertDOCXToPDF(docxPath, pdfPath)
	if err != nil {
		return "", "", err
	}
	return reportDownloadURL("pdf", strings.TrimSuffix(filepath.Base(pdfPath), ".pdf")), engine, nil
}

// reportDownloadURL is the browser link to a generated report. Report names are
// prose - they carry spaces, and a client name can carry an '&' - so the name
// is escaped as the single path segment it is.
func reportDownloadURL(kind, baseName string) string {
	// PathEscape leaves the sub-delimiters alone, and a bare '&' in an href set
	// through innerHTML can start an HTML entity, so it is escaped too.
	return "/api/v1/reports/download/" + kind + "/" +
		strings.ReplaceAll(url.PathEscape(baseName), "&", "%26")
}

// GenerateDOCX merges the template and writes it out, returning the download
// URL and whatever the renderer could not settle on its own.
func GenerateDOCX(config ReportConfig, outputPath string) (string, *reportNotes, error) {
	docxBytes, notes, err := mergeMCollaboratorDocx(config)
	if err != nil {
		return "", nil, fmt.Errorf("failed to render DOCX from template: %w", err)
	}
	if err := os.WriteFile(outputPath, docxBytes, 0644); err != nil {
		return "", nil, fmt.Errorf("failed to write DOCX report: %w", err)
	}
	return reportDownloadURL("docx", strings.TrimSuffix(filepath.Base(outputPath), ".docx")), notes, nil
}

func mustReadFile(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return b
}
