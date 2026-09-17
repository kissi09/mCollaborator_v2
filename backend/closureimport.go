package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Closure prep from a finished report
// ---------------------------------------------------------------------------
//
// The closing deck is normally built from the report wizard, which still holds
// the engagement it just rendered. This path exists for the other case: the
// report is finished and out of the door - a DOCX or a PDF on disk, possibly
// written months ago or by someone else - and all that is wanted now is the
// deck for the closing meeting.
//
// Everything the deck needs is read back out of the document: the client's
// name, the reference number, the date, the assessment period, the areas that
// were tested with their scope, and every finding. The findings come from the
// same extractor the finding import uses, so a report this app wrote is read
// back exactly and a report written by hand is read as well as it can be.
//
// What cannot be read back are the screenshots. A PoC image in the report is a
// picture anchored in a table cell with nothing tying it to the finding around
// it, and in a PDF it is not a file at all. So the deck's scenario slides are
// left empty here, and the preview screen asks for each finding's proof to be
// uploaded from the report's evidence section. Nothing is guessed and no
// scenario slide is invented for a finding whose proof nobody supplied.

// ClosureDraft is what an uploaded report becomes: a report configuration ready
// for the closure deck, plus what had to be left for a person to settle.
type ClosureDraft struct {
	Filename string       `json:"filename"`
	Kind     string       `json:"kind"`
	Config   ReportConfig `json:"config"`

	// Unplaced is the number of findings the document did not place in an
	// assessment area. They are kept, not dropped: the preview holds them aside
	// and refuses to build until each has been given an area, because a finding
	// with no area appears on no issues slide.
	Unplaced int `json:"unplaced"`

	// Missing names the engagement details nothing in the document gave, so the
	// preview can ask for them rather than print a deck with blanks in it.
	Missing []string `json:"missing,omitempty"`

	Notes []string `json:"notes,omitempty"`
}

// HandleClosureImport reads a finished report into a closure draft.
//
// It saves nothing and creates no engagement. The draft goes back to the
// browser, is corrected on the preview screen, and comes back to
// /reports/closure as an ordinary deck request.
func HandleClosureImport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, extractMaxBytes+(1<<20))
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, ApiResponse{Error: &ApiError{
				Code: "INVALID_REQUEST", Message: "The upload could not be read. Reports over 25 MB are rejected."}})
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ApiResponse{Error: &ApiError{
				Code: "INVALID_REQUEST", Message: "No file was uploaded"}})
			return
		}
		defer file.Close()

		name := filepath.Base(header.Filename)
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".docx" && ext != ".pdf" {
			// Deliberately narrower than the finding import, which also takes a
			// scanner export. An export is a list of vulnerabilities and nothing
			// else: it names no client, no reference, no date and no assessment
			// area, which is most of what the deck's title and summary slides
			// are made of.
			writeJSON(w, http.StatusBadRequest, ApiResponse{Error: &ApiError{
				Code: "UNSUPPORTED_TYPE",
				Message: "Only a .docx or .pdf report can be turned into a closing deck. A .doc has to be saved as .docx first, " +
					"and a scanner export carries none of the engagement details the deck's title and summary slides need."}})
			return
		}

		data, err := io.ReadAll(io.LimitReader(file, extractMaxBytes+1))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ApiResponse{Error: &ApiError{
				Code: "INVALID_REQUEST", Message: "The file could not be read"}})
			return
		}
		if len(data) > extractMaxBytes {
			writeJSON(w, http.StatusBadRequest, ApiResponse{Error: &ApiError{
				Code: "TOO_LARGE", Message: "The report is larger than 25 MB"}})
			return
		}

		// Same reason the finding import logs it: an upload that loses its body
		// on the way looks exactly like a file in the wrong format once the
		// parser has had it, and only one of those is the tester's fault.
		log.Printf("closure import: %q ext=%s received=%d bytes declared=%s head=%s",
			name, ext, len(data), r.Header.Get("Content-Length"), describeHead(data))

		if err := sniffDocument(data, ext); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, ApiResponse{Error: &ApiError{
				Code: "UNREADABLE_DOCUMENT", Message: err.Error()}})
			return
		}

		var draft ClosureDraft
		if ext == ".docx" {
			draft, err = closureDraftFromDOCX(data)
		} else {
			draft, err = closureDraftFromPDF(data)
		}
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, ApiResponse{Error: &ApiError{
				Code: "UNREADABLE_DOCUMENT", Message: err.Error()}})
			return
		}
		if len(draft.Config.Findings) == 0 {
			writeJSON(w, http.StatusUnprocessableEntity, ApiResponse{Error: &ApiError{
				Code: "NO_FINDINGS",
				Message: "No findings could be read out of that report, and a closing deck with no issues in it is not worth presenting. " +
					"If this is a PDF, try the DOCX instead."}})
			return
		}

		draft.Filename = name
		log.Printf("closure import: %q read %d findings, %d unplaced, missing %v",
			name, len(draft.Config.Findings), draft.Unplaced, draft.Missing)
		writeJSON(w, http.StatusOK, ApiResponse{Data: draft})
	}
}

func closureDraftFromDOCX(data []byte) (ClosureDraft, error) {
	findings, notes, err := ExtractFromDOCX(data)
	if err != nil {
		return ClosureDraft{}, err
	}
	doc, err := docxDocumentXML(data)
	if err != nil {
		return ClosureDraft{}, err
	}
	items := docxBodyItems(doc)
	meta := readReportMeta(docLines(items))
	meta.readDOCXTables(items)
	return buildClosureDraft("docx", meta, findings, notes), nil
}

func closureDraftFromPDF(data []byte) (ClosureDraft, error) {
	text, err := pdfPlainText(trimPDFPreamble(data))
	if err != nil {
		return ClosureDraft{}, err
	}
	findings, notes := ExtractFromPDFText(text)

	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(strings.ReplaceAll(l, "\f", "")); t != "" {
			lines = append(lines, t)
		}
	}
	meta := readReportMeta(lines)
	meta.readFlatCover(lines)
	meta.readFlatScope(lines)
	return buildClosureDraft("pdf", meta, findings, notes), nil
}

// buildClosureDraft turns what was read into the configuration the deck builder
// takes, and says what is still missing.
func buildClosureDraft(kind string, meta *reportMeta, findings []ExtractedFinding, notes []string) ClosureDraft {
	draft := ClosureDraft{Kind: kind, Notes: notes}

	cfg := ReportConfig{
		CompanyName:     meta.Company,
		CompanyInitials: meta.Initials,
		EngagementName:  meta.Engagement,
		RefNumber:       meta.Ref,
		ReportDate:      meta.Date,
		AssessmentStart: meta.Start,
		AssessmentEnd:   meta.End,
		Areas:           meta.Areas,
	}
	if cfg.EngagementName == "" {
		cfg.EngagementName = "VAPT Report"
	}

	seen := map[string]bool{}
	for _, a := range cfg.Areas {
		seen[a.Code] = true
	}
	for _, f := range findings {
		rf := ReportFinding{
			Title:          strings.TrimSpace(f.Title),
			Description:    f.Description,
			Impact:         f.Impact,
			CVSSVector:     f.CVSSVector,
			Severity:       f.Severity,
			AffectedSystem: f.AffectedSystem,
			AttackVector:   f.AttackVector,
			POC:            f.POC,
			Recommendation: f.Remediation,
			Area:           strings.ToUpper(strings.TrimSpace(f.Category)),
		}
		if f.CVSSScore > 0 {
			rf.CVSSScore = strconv.FormatFloat(f.CVSSScore, 'f', -1, 64)
		}
		if rf.Area == "" {
			draft.Unplaced++
		} else if !seen[rf.Area] {
			// A finding under a section the scope table did not list still has
			// to appear on a slide. Its area joins the deck rather than the
			// finding being dropped for being somewhere unexpected.
			seen[rf.Area] = true
			cfg.Areas = append(cfg.Areas, ReportArea{Code: rf.Area})
		}
		cfg.Findings = append(cfg.Findings, rf)
	}
	cfg.Areas = areasInTemplateOrder(cfg.Areas)

	draft.Config = cfg
	draft.Missing = missingDeckDetails(cfg)
	return draft
}

// areasInTemplateOrder puts the areas back into the order the report prints
// them, whichever order they happened to be read in.
func areasInTemplateOrder(areas []ReportArea) []ReportArea {
	byCode := map[string]ReportArea{}
	for _, a := range areas {
		code := strings.ToUpper(strings.TrimSpace(a.Code))
		if _, ok := areaByCode(code); !ok {
			continue
		}
		// A scope read off the document wins over an empty one read elsewhere.
		if existing, ok := byCode[code]; ok && strings.TrimSpace(a.Scope) == "" {
			a.Scope = existing.Scope
		}
		a.Code = code
		byCode[code] = a
	}
	var out []ReportArea
	for _, def := range reportAreas {
		if a, ok := byCode[def.Code]; ok {
			out = append(out, a)
		}
	}
	return out
}

// missingDeckDetails names the title-slide and summary-slide values the report
// did not give up, in the words the preview puts on screen.
func missingDeckDetails(cfg ReportConfig) []string {
	var missing []string
	if strings.TrimSpace(cfg.CompanyName) == "" {
		missing = append(missing, "the client's name")
	}
	if strings.TrimSpace(cfg.ReportDate) == "" {
		missing = append(missing, "the report date")
	}
	if strings.TrimSpace(cfg.AssessmentStart) == "" || strings.TrimSpace(cfg.AssessmentEnd) == "" {
		missing = append(missing, "the assessment period")
	}
	if len(cfg.Areas) == 0 {
		missing = append(missing, "the assessment areas")
	}
	return missing
}

// ---------------------------------------------------------------------------
// reading the document
// ---------------------------------------------------------------------------

// docxItem is one top-level element of the body: a paragraph's text, or a
// table's rows. The order matters - the scope table is found by being the table
// that follows the Scope heading, not by what is in it.
type docxItem struct {
	IsTable bool
	Text    string
	Rows    [][]string
}

// docxDocumentXML pulls word/document.xml out of the package.
func docxDocumentXML(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("this is not a readable .docx file: %w", err)
	}
	for _, f := range zr.File {
		if strings.ReplaceAll(f.Name, "\\", "/") != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("could not open the document body: %w", err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return "", fmt.Errorf("could not read the document body: %w", err)
		}
		return healPlaceholderRuns(string(b)), nil
	}
	return "", fmt.Errorf("the file has no word/document.xml - it may be an older .doc rather than a .docx")
}

func docxBodyItems(doc string) []docxItem {
	var out []docxItem
	for _, child := range bodyChildren(doc) {
		frag := doc[child.Start:child.End]
		switch child.Tag {
		case "w:tbl":
			item := docxItem{IsTable: true}
			for _, r := range tableRows(frag) {
				row := frag[r.Start:r.End]
				var cells []string
				for _, c := range rowCells(row) {
					cells = append(cells, cellTextWithBreaks(row[c.Start:c.End]))
				}
				item.Rows = append(item.Rows, cells)
			}
			out = append(out, item)
		case "w:p":
			out = append(out, docxItem{Text: strings.TrimSpace(elemText(frag))})
		}
	}
	return out
}

var lineBreakRe = regexp.MustCompile(`<w:br(?: [^>]*)?/>`)

// cellTextWithBreaks is a cell's text with its line breaks kept as newlines.
//
// A scope cell listing three devices is usually one paragraph broken three
// times, not three paragraphs, and cellText alone runs those together into
// "Lilongwe DeviceHuawei Core SwitchCisco Core Switch" - which is then one
// long line on the deck's scope panel instead of three targets.
func cellTextWithBreaks(cell string) string {
	var lines []string
	for _, l := range strings.Split(cellText(lineBreakRe.ReplaceAllString(cell, "<w:t>\n</w:t>")), "\n") {
		if t := strings.TrimSpace(l); t != "" {
			lines = append(lines, t)
		}
	}
	return strings.Join(lines, "\n")
}

// docLines flattens the body into the lines the shared reader works on. A
// table's cells are read in place, because the cover page - reference number,
// date, the report's own title - is a table.
func docLines(items []docxItem) []string {
	var lines []string
	for _, it := range items {
		if !it.IsTable {
			if it.Text != "" {
				lines = append(lines, it.Text)
			}
			continue
		}
		for _, row := range it.Rows {
			for _, cell := range row {
				for _, l := range strings.Split(cell, "\n") {
					if t := strings.TrimSpace(l); t != "" {
						lines = append(lines, t)
					}
				}
			}
		}
	}
	return lines
}

// reportMeta is the engagement as the document describes it.
type reportMeta struct {
	Company    string
	Initials   string
	Engagement string
	Ref        string
	Date       string
	Start      string
	End        string
	Areas      []ReportArea
}

var (
	// The cover's reference number: GH-REP-047-3292129.
	refNumberRe = regexp.MustCompile(`^[A-Z]{2,5}-[A-Z]{2,5}-[0-9][0-9A-Z-]*$`)
	// "17th August 2026" and "August 17, 2026", the two ways the cover date and
	// the assessment period are written.
	longDateRe = regexp.MustCompile(`^\d{1,2}(?:st|nd|rd|th)?\s+[A-Za-z]{3,9}\s+\d{4}$`)
	usDateRe   = regexp.MustCompile(`^[A-Za-z]{3,9}\s+\d{1,2}(?:st|nd|rd|th)?,?\s+\d{4}$`)
	// "...during the period from 17th June 2026 to 27th June 2026."
	periodRe = regexp.MustCompile(`(?i)period\s+from\s+(\d{1,2}(?:st|nd|rd|th)?\s+[A-Za-z]{3,9}\s+\d{4}|[A-Za-z]{3,9}\s+\d{1,2}(?:st|nd|rd|th)?,?\s+\d{4})\s+(?:to|until|through|-|\x{2013}|\x{2014})\s+(\d{1,2}(?:st|nd|rd|th)?\s+[A-Za-z]{3,9}\s+\d{4}|[A-Za-z]{3,9}\s+\d{1,2}(?:st|nd|rd|th)?,?\s+\d{4})`)
	// The cover title: "<Client> - Vulnerability Assessment & Penetration Testing (VAPT)".
	coverTitleRe = regexp.MustCompile(`(?i)^(.{2,80}?)\s*[\x{2013}\x{2014}-]\s*(Vulnerability\s+Assessment.*|Penetration\s+Test.*|VAPT.*)$`)
	// The 3.1 heading, with or without the section number the PDF carries.
	namingHeadingRe = regexp.MustCompile(`(?i)^(?:\d+(?:\.\d+)*\.?\s+)?recommendations?\s+naming\s+convention$`)
	// One line of the 3.1 list: "CFG - Configuration Files Review".
	namingEntryRe = regexp.MustCompile(`^([A-Z][A-Z0-9]{1,5})\s*[\x{2013}\x{2014}-]\s*(\S.*)$`)
	// The 2.3 heading, likewise, and the 2.4 heading that closes the section.
	scopeHeadingRe      = regexp.MustCompile(`(?i)^(?:\d+(?:\.\d+)*\.?\s+)?scope$`)
	outOfScopeHeadingRe = regexp.MustCompile(`(?i)^(?:\d+(?:\.\d+)*\.?\s+)?out\s+of\s+scope$`)

	// A PDF has no tables, only rows, and these are the rows of the two tables
	// that matter: the cover's Version/Date row and the scope table's own
	// heading, which repeats at the top of every page it runs onto.
	coverHeaderRe    = regexp.MustCompile(`(?i)^version\s+date\s+authors?\s+approver`)
	scopeTableHeadRe = regexp.MustCompile(`(?i)^activity\s+details$`)
	anyDateRe        = regexp.MustCompile(`\d{1,2}(?:st|nd|rd|th)?\s+[A-Za-z]{3,9}\s+\d{4}|[A-Za-z]{3,9}\s+\d{1,2}(?:st|nd|rd|th)?,?\s+\d{4}`)
	// The running footer, which lands in the middle of a table that spans a
	// page break - and carries the reference number where the cover's own copy
	// of it was broken across two rows.
	pageFurnitureRe = regexp.MustCompile(`(?i)(all rights reserved|page\s+\d+\s*$)`)
	footerRefRe     = regexp.MustCompile(`(?i)\bref(?:erence)?\.?\s*:\s*([A-Z0-9][A-Z0-9-]{4,})`)
)

// readReportMeta reads the engagement details out of the document's lines. It
// is shared by the DOCX and the PDF paths: a PDF's rows and a DOCX's paragraphs
// carry the same sentences, and every rule here is a rule about the words.
//
// Nothing here guesses. A value that is not found is left empty and named in
// Missing, because a deck that prints the wrong client's name is worse than one
// that asks for it.
func readReportMeta(lines []string) *reportMeta {
	meta := &reportMeta{}
	meta.readNamingConvention(lines)

	// The cover title. Searched over the head of the document only: "X -
	// Vulnerability Assessment" turns up in prose further in.
	head := lines
	if len(head) > 40 {
		head = head[:40]
	}
	for _, l := range head {
		m := coverTitleRe.FindStringSubmatch(stripDotLeaders(l))
		if m == nil {
			continue
		}
		if meta.Company == "" {
			meta.Company = strings.TrimSpace(m[1])
		}
		meta.Engagement = strings.TrimSpace(m[2])
		break
	}

	// The reference number, and the date beside it in the cover table.
	for i, l := range head {
		if !refNumberRe.MatchString(l) {
			continue
		}
		meta.Ref = l
		for _, next := range head[i+1:] {
			if isLongDate(next) {
				meta.Date = next
				break
			}
		}
		break
	}
	if meta.Date == "" {
		for _, l := range head {
			if isLongDate(l) {
				meta.Date = l
				break
			}
		}
	}

	// The assessment period, out of the introduction or the scope sentence.
	for _, l := range lines {
		if m := periodRe.FindStringSubmatch(l); m != nil {
			meta.Start, meta.End = strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
			break
		}
	}
	return meta
}

func isLongDate(s string) bool {
	s = strings.TrimSpace(s)
	return longDateRe.MatchString(s) || usDateRe.MatchString(s)
}

// readNamingConvention reads section 3.1, which is the one place in the report
// where the client's initials, the client's name and every area tested are
// written down as a list:
//
//	GMP - PPPC Malawi
//	REC - Recommendation
//	CFG - Configuration Files Review
//	NAR - Network Architecture Review
//
// Every block in the document is read and the richest one wins, so the table of
// contents' own copy of the heading costs nothing.
func (m *reportMeta) readNamingConvention(lines []string) {
	best := -1
	for i, l := range lines {
		if !namingHeadingRe.MatchString(stripDotLeaders(l)) {
			continue
		}
		if n := countNamingAreas(lines, i); n > best {
			best = n
			m.applyNamingBlock(lines, i)
		}
	}
}

// namingBlockEntries lists the "CODE - value" lines that follow a 3.1 heading.
// The list is short and unbroken in every report; reading past a handful of
// lines that are not entries would start picking up the prose after it.
func namingBlockEntries(lines []string, heading int) [][2]string {
	var out [][2]string
	misses := 0
	for i := heading + 1; i < len(lines) && i < heading+25; i++ {
		m := namingEntryRe.FindStringSubmatch(strings.TrimSpace(lines[i]))
		if m == nil {
			// The intro sentence sits between the heading and the list, and the
			// example sentence closes it.
			if misses++; misses > 3 && len(out) > 0 {
				break
			}
			continue
		}
		misses = 0
		out = append(out, [2]string{strings.ToUpper(m[1]), strings.TrimSpace(m[2])})
	}
	return out
}

func countNamingAreas(lines []string, heading int) int {
	n := 0
	for _, e := range namingBlockEntries(lines, heading) {
		if _, ok := areaByCode(e[0]); ok {
			n++
		}
	}
	return n
}

func (m *reportMeta) applyNamingBlock(lines []string, heading int) {
	for _, e := range namingBlockEntries(lines, heading) {
		code, value := e[0], e[1]
		if area, ok := areaByCode(code); ok {
			m.addArea(area.Code, "")
			continue
		}
		if strings.EqualFold(code, "REC") || strings.EqualFold(value, "Recommendation") {
			continue
		}
		// Whatever is left is the client: "GMP - PPPC Malawi" is the only entry
		// in the list that is neither an area nor the recommendation marker.
		if m.Initials == "" {
			m.Initials, m.Company = code, value
		}
	}
}

func (m *reportMeta) addArea(code, scope string) {
	code = strings.ToUpper(strings.TrimSpace(code))
	for i, a := range m.Areas {
		if a.Code == code {
			if strings.TrimSpace(a.Scope) == "" {
				m.Areas[i].Scope = scope
			}
			return
		}
	}
	m.Areas = append(m.Areas, ReportArea{Code: code, Scope: scope})
}

// readDOCXTables takes what only a table can give: the scope of each area, and
// the cover's reference number and date where the prose reader could not tell
// them apart from the text around them.
func (m *reportMeta) readDOCXTables(items []docxItem) {
	m.readScopeTable(items)
	m.readCoverTable(items)
}

// readScopeTable reads the 2.3 table - Activity | Details - which is where an
// area's scope is written out.
func (m *reportMeta) readScopeTable(items []docxItem) {
	for i, it := range items {
		if it.IsTable || !scopeHeadingRe.MatchString(stripDotLeaders(it.Text)) {
			continue
		}
		// The table sits within a paragraph or two of its heading; anything
		// further away belongs to another section.
		for j := i + 1; j < len(items) && j <= i+3; j++ {
			if !items[j].IsTable {
				continue
			}
			for _, row := range items[j].Rows {
				if len(row) < 2 {
					continue
				}
				code, _, ok := areaFromHeading(row[0])
				if !ok {
					continue
				}
				m.addArea(code, strings.TrimSpace(row[1]))
			}
			return
		}
	}
}

// readCoverTable reads the reference number and date out of the cover's
// Version | Date | Authors | Approver table, where their position says what
// they are rather than their shape.
func (m *reportMeta) readCoverTable(items []docxItem) {
	for _, it := range items {
		if !it.IsTable {
			continue
		}
		for r, row := range it.Rows {
			ver, date := -1, -1
			for c, cell := range row {
				switch strings.ToLower(strings.TrimSpace(cell)) {
				case "version":
					ver = c
				case "date":
					date = c
				}
			}
			if ver < 0 || date < 0 || r+1 >= len(it.Rows) {
				continue
			}
			values := it.Rows[r+1]
			if ver < len(values) {
				if v := strings.TrimSpace(values[ver]); v != "" && m.Ref == "" {
					m.Ref = v
				}
			}
			if date < len(values) {
				if v := strings.TrimSpace(values[date]); v != "" {
					m.Date = v
				}
			}
			return
		}
	}
}

// readFlatCover is the PDF's stand-in for the cover table.
//
// The whole row comes back as one line with the columns run together -
// "TEST-REP-001-00000-6th September 2026 Emmanuel Addo Kissi Jamal Mekdachi" -
// and the reference number is broken across two of them where it was too long
// for its column. The date is the one column whose shape says what it is, so it
// is found first and the reference is whatever sits in front of it.
func (m *reportMeta) readFlatCover(lines []string) {
	head := lines
	if len(head) > 40 {
		head = head[:40]
	}
	for i, l := range head {
		if !coverHeaderRe.MatchString(strings.TrimSpace(l)) || i+1 >= len(head) {
			continue
		}
		row := strings.TrimSpace(head[i+1])
		loc := anyDateRe.FindStringIndex(row)
		if loc == nil {
			break
		}
		if m.Date == "" {
			m.Date = strings.TrimSpace(row[loc[0]:loc[1]])
		}
		ref := strings.TrimSpace(row[:loc[0]])
		// A reference that wrapped mid-column leaves its tail alone on the next
		// row: "TEST-REP-001-00000-" then "01".
		if strings.HasSuffix(ref, "-") && i+2 < len(head) {
			if tail := strings.TrimSpace(head[i+2]); len(tail) <= 8 && refTailRe.MatchString(tail) {
				ref += tail
			}
		}
		if m.Ref == "" {
			m.Ref = ref
		}
		break
	}
	if m.Ref == "" {
		// The running footer prints it on every page: "All rights reserved
		// Ref: TEST-REP-001-00000-01".
		for _, l := range lines {
			if match := footerRefRe.FindStringSubmatch(l); match != nil {
				m.Ref = strings.TrimSpace(match[1])
				break
			}
		}
	}
}

var refTailRe = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]*$`)

// readFlatScope is the PDF's stand-in for the scope table.
//
// Its rows read "Internal Penetration Testing 10.20.0.0/22 - 148 internal
// hosts" - the activity and its detail run together, wrapping onto the lines
// below. So an area is recognised by a row *starting* with its name, and what
// follows on that row and under it is its scope.
//
// Every "Scope" heading in the document is tried and the richest block wins,
// because the table of contents carries one too, and the entries beneath that
// one are section titles with no scope attached.
func (m *reportMeta) readFlatScope(lines []string) {
	best := map[string]string{}
	for i, l := range lines {
		if !scopeHeadingRe.MatchString(stripDotLeaders(l)) {
			continue
		}
		if block := flatScopeBlock(lines, i); countFilled(block) > countFilled(best) {
			best = block
		}
	}
	// In template order, so the deck's scope table reads the same way round
	// however the document was laid out.
	for _, def := range reportAreas {
		if scope, ok := best[def.Code]; ok {
			m.addArea(def.Code, scope)
		}
	}
}

func countFilled(block map[string]string) int {
	n := 0
	for _, v := range block {
		if strings.TrimSpace(v) != "" {
			n++
		}
	}
	return n
}

// flatScopeBlock reads the scope rows that follow one "Scope" heading.
func flatScopeBlock(lines []string, start int) map[string]string {
	out := map[string]string{}
	current := ""
	for j := start + 1; j < len(lines) && j <= start+60; j++ {
		row := strings.TrimSpace(lines[j])
		bare := stripDotLeaders(row)
		if outOfScopeHeadingRe.MatchString(bare) {
			break
		}
		// The table's own heading, repeated at each page break, and the running
		// footer that lands between two rows of it.
		if scopeTableHeadRe.MatchString(bare) || pageFurnitureRe.MatchString(row) {
			continue
		}
		if code, rest, ok := splitAreaPrefix(bare); ok {
			current = code
			if rest != "" || out[code] == "" {
				out[code] = rest
			}
			continue
		}
		if current == "" || row == "" {
			continue
		}
		// The prose that follows the table - "The external activities were
		// performed remotely." - is indistinguishable from a wrapped cell by
		// position alone, and it was being read as the last area's scope. A
		// scope cell names targets and does not end in a full stop; a sentence
		// does. The row is dropped rather than the block closed, so an area
		// listed after it is still read.
		if isProseSentence(row) {
			current = ""
			continue
		}
		// A scope cell too wide for its column, continued underneath.
		if out[current] == "" {
			out[current] = row
		} else {
			out[current] += " " + row
		}
	}
	return out
}

// splitAreaPrefix recognises a row that opens with an area's name and returns
// the area's code with the rest of the row. The longest name wins, so
// "Internal Cloud Penetration Testing" is never read as "Internal Penetration
// Testing" with a stray word in front of its scope.
func splitAreaPrefix(row string) (string, string, bool) {
	row = strings.TrimSpace(stripSectionNumber(row))
	low := strings.ToLower(row)
	bestCode, bestRest, bestLen := "", "", 0
	for _, a := range reportAreas {
		for _, name := range []string{a.Heading, a.Label, a.ScopeRow} {
			if name == "" || len(name) <= bestLen || !strings.HasPrefix(low, strings.ToLower(name)) {
				continue
			}
			rest := strings.TrimSpace(row[len(name):])
			// The name has to end the word, not start one: "Scope" must not
			// match the front of "Scoped systems".
			if rest != "" && !isRowBoundary(row[len(name)]) {
				continue
			}
			bestCode, bestRest, bestLen = a.Code, rest, len(name)
		}
	}
	return bestCode, bestRest, bestCode != ""
}

func isRowBoundary(c byte) bool {
	return c == ' ' || c == '\t' || c == ':' || c == '-'
}

// isProseSentence reports whether a row reads as a sentence rather than as the
// contents of a table cell.
func isProseSentence(row string) bool {
	return strings.HasSuffix(row, ".") && len(strings.Fields(row)) >= 5
}
