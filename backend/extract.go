package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Reading findings out of a finished report.
//
// A tester who has a report in hand should not have to retype it to get the
// findings into an engagement. Both a DOCX and a PDF are accepted, and both are
// read the same way as far as the format allows: locate the assessment-area
// sections, then the vulnerability blocks inside them, then the labelled rows
// of each block.
//
// Nothing here writes anything. It returns candidates for a person to check,
// because the one thing an extractor cannot do reliably is decide which area a
// finding belongs to when the document does not say. Every candidate carries
// how its area was decided so that judgement is quick to make - and a candidate
// with no area at all is returned with none rather than being filed under a
// guess.

// ExtractConfidence records how a candidate's area was arrived at.
type ExtractConfidence string

const (
	// ConfHeading: the finding sat under a recognised section heading. The
	// document said which area it belongs to.
	ConfHeading ExtractConfidence = "heading"
	// ConfKeywords: no heading applied, but the finding's own wording matched an
	// area's vocabulary strongly enough to suggest one.
	ConfKeywords ExtractConfidence = "keywords"
	// ConfNone: neither. The candidate comes back with no area.
	ConfNone ExtractConfidence = "none"
)

// ExtractedFinding is one candidate. The field names match createFindingInput so
// the reviewed set can be posted straight to the bulk endpoint.
type ExtractedFinding struct {
	Title          string  `json:"title"`
	Description    string  `json:"description"`
	Impact         string  `json:"impact"`
	Severity       string  `json:"severity"`
	CVSSVector     string  `json:"cvss_vector,omitempty"`
	CVSSScore      float64 `json:"cvss_score,omitempty"`
	AffectedSystem string  `json:"affected_system,omitempty"`
	AttackVector   string  `json:"attack_vector,omitempty"`
	POC            string  `json:"poc,omitempty"`
	Remediation    string  `json:"remediation,omitempty"`

	// Category is the assessment area, empty when nothing decided it.
	Category string `json:"category"`

	Confidence ExtractConfidence `json:"confidence"`
	Reason     string            `json:"reason"`
	Ref        string            `json:"ref,omitempty"`
}

// ExtractResult is what the endpoint returns.
type ExtractResult struct {
	Filename string             `json:"filename"`
	Kind     string             `json:"kind"`
	Findings []ExtractedFinding `json:"findings"`
	ByArea   map[string]int     `json:"by_area"`
	Unplaced int                `json:"unplaced"`
	Notes    []string           `json:"notes,omitempty"`
}

// ---------------------------------------------------------------------------
// deciding the area
// ---------------------------------------------------------------------------

// areaKeywords is the vocabulary each area is recognised by when no heading
// says. Terms are matched whole-word against the lower-cased title, description
// and affected system. They are deliberately narrow: a term that could belong to
// two areas is worse than no term, because a wrong area filed with confidence is
// harder to spot than one that asks.
var areaKeywords = map[string][]string{
	"IPT":  {"internal network", "domain controller", "smb", "ntlm", "lateral movement", "workstation", "file share", "netbios", "llmnr"},
	"EPT":  {"internet-facing", "internet facing", "public ip", "perimeter", "externally", "public-facing", "publicly accessible", "external host"},
	"IPTC": {"azure", "aws", "s3 bucket", "iam role", "cloud subscription", "storage account", "ec2", "gcp"},
	"WPT":  {"cross-site", "xss", "sql injection", "sqli", "csrf", "session cookie", "web application", "http header", "owasp", "endpoint", "login form"},
	"CFG":  {"configuration file", "running config", "snmp community", "firewall rule", "switch configuration", "router configuration", "banner", "nipper", "acl"},
	"ASA":  {"rest api", "api endpoint", "swagger", "openapi", "graphql", "api key", "bearer token"},
	"ADT":  {"active directory", "kerberos", "kerberoast", "spn", "group policy", "domain admin", "ldap", "asrep", "bloodhound"},
	"WNA":  {"wireless", "ssid", "wpa", "wep", "access point", "wifi", "wi-fi", "802.11", "evil twin", "deauthentication"},
	"NAR":  {"network architecture", "segmentation", "vlan", "topology", "network design", "out-of-band", "dmz", "broadcast domain"},
}

// guessArea reads an area out of a finding's own words. It returns the area only
// when one is clearly ahead: a tie means the wording does not settle it, and a
// finding with no area is a better outcome than one filed under a coin toss.
func guessArea(text string) (string, string) {
	low := strings.ToLower(text)
	type hit struct {
		code  string
		score int
		terms []string
	}
	var hits []hit
	for code, terms := range areaKeywords {
		h := hit{code: code}
		for _, t := range terms {
			if containsWord(low, t) {
				h.score++
				h.terms = append(h.terms, t)
			}
		}
		if h.score > 0 {
			sort.Strings(h.terms)
			hits = append(hits, h)
		}
	}
	if len(hits) == 0 {
		return "", ""
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].code < hits[j].code
	})
	if len(hits) > 1 && hits[0].score == hits[1].score {
		return "", fmt.Sprintf("reads as both %s and %s", hits[0].code, hits[1].code)
	}
	shown := hits[0].terms
	if len(shown) > 2 {
		shown = shown[:2]
	}
	return hits[0].code, "matched " + quoteList(shown)
}

func quoteList(items []string) string {
	for i, s := range items {
		items[i] = "“" + s + "”"
	}
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
	}
}

// containsWord matches a term on word boundaries so "acl" does not fire inside
// "oracle" and "spn" does not fire inside "respond".
func containsWord(hay, needle string) bool {
	from := 0
	for {
		i := strings.Index(hay[from:], needle)
		if i < 0 {
			return false
		}
		i += from
		before := i == 0 || !isWordByte(hay[i-1])
		afterIdx := i + len(needle)
		after := afterIdx >= len(hay) || !isWordByte(hay[afterIdx])
		if before && after {
			return true
		}
		from = i + 1
	}
}

func isWordByte(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// placeArea settles a candidate's area: the section heading it sat under when
// there was one, otherwise its own wording, otherwise nothing.
func placeArea(f *ExtractedFinding, headingArea, headingLabel string) {
	if headingArea != "" {
		f.Category = headingArea
		f.Confidence = ConfHeading
		f.Reason = "Under " + headingLabel
		return
	}
	code, why := guessArea(f.Title + " " + f.Description + " " + f.AffectedSystem + " " + f.Remediation)
	if code != "" {
		f.Category = code
		f.Confidence = ConfKeywords
		f.Reason = "No section heading — " + why
		return
	}
	f.Category = ""
	f.Confidence = ConfNone
	if why != "" {
		f.Reason = "No section heading and it " + why
		return
	}
	f.Reason = "No section heading and nothing in the wording to go on"
}

// ---------------------------------------------------------------------------
// severity
// ---------------------------------------------------------------------------

var severityWords = []struct {
	word string
	sev  string
}{
	{"critical", "critical"},
	{"high", "high"},
	{"medium", "medium"},
	{"moderate", "medium"},
	{"low", "low"},
	{"informational", "info"},
	{"info", "info"},
}

func normalizeSeverity(s string) string {
	low := strings.ToLower(strings.TrimSpace(s))
	for _, w := range severityWords {
		if containsWord(low, w.word) {
			return w.sev
		}
	}
	return ""
}

var cvssScoreRe = regexp.MustCompile(`\b(10(?:\.0)?|[0-9](?:\.[0-9])?)\b`)

// severityFromScore is the CVSS v3.1 band, used only when the document gives a
// score but no rating word.
func severityFromScore(score float64) string {
	switch {
	case score >= 9.0:
		return "critical"
	case score >= 7.0:
		return "high"
	case score >= 4.0:
		return "medium"
	case score > 0:
		return "low"
	}
	return ""
}

// ---------------------------------------------------------------------------
// DOCX
// ---------------------------------------------------------------------------

// ExtractFromDOCX reads findings out of a Word document.
//
// A report built from the mCollaborator template reads cleanly: chapter 3's
// headings name the areas, and each finding is a table of labelled rows this
// package already knows how to read (detailLabels). A document from elsewhere
// still works as long as it labels its rows recognisably; where it does not,
// the paragraph run before each table becomes the title and the area is guessed.
func ExtractFromDOCX(data []byte) ([]ExtractedFinding, []string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, fmt.Errorf("this is not a readable .docx file: %w", err)
	}
	var doc string
	for _, f := range zr.File {
		if strings.ReplaceAll(f.Name, "\\", "/") != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, nil, fmt.Errorf("could not open the document body: %w", err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("could not read the document body: %w", err)
		}
		doc = string(b)
		break
	}
	if doc == "" {
		return nil, nil, fmt.Errorf("the file has no word/document.xml - it may be an older .doc rather than a .docx")
	}

	doc = healPlaceholderRuns(doc)
	children := bodyChildren(doc)
	if len(children) == 0 {
		return nil, nil, fmt.Errorf("the document body could not be read")
	}

	var (
		out          []ExtractedFinding
		notes        []string
		headingArea  string
		headingLabel string
		lastPara     string
		paraSinceTbl bool
		sawTable     bool
		pending      *ExtractedFinding
		pocLines     []string
		recLines     []string
		section      string
		skipped      int
	)

	// flush finalises the finding being built. trailing, when given, is the
	// paragraph that named the *next* finding: nothing in the document closes a
	// recommendation run, so that title is sitting at the end of it.
	flush := func(trailing string) {
		if pending != nil {
			if trailing != "" {
				pocLines = dropTrailing(pocLines, trailing)
				recLines = dropTrailing(recLines, trailing)
			}
			if pending.POC == "" {
				pending.POC = strings.TrimSpace(strings.Join(pocLines, "\n"))
			}
			if pending.Remediation == "" {
				pending.Remediation = strings.TrimSpace(strings.Join(recLines, "\n"))
			}
			if isPlaceholderFinding(pending) {
				// A draft's unwritten finding: every field is XXXX. Importing it
				// would put an empty row in the engagement for someone to find
				// and delete.
				skipped++
			} else {
				out = append(out, *pending)
			}
			pending = nil
		}
		pocLines, recLines, section = nil, nil, ""
	}

	for _, c := range children {
		frag := doc[c.Start:c.End]

		if c.Tag == "w:p" {
			text := strings.TrimSpace(elemText(frag))
			if text == "" {
				continue
			}
			// A section title is not always a Heading style. This report names
			// its areas in bold numbered list paragraphs, and the app's own
			// template has been caught styling headings by hand too. An exact
			// match against an area's own name is decisive however the
			// paragraph is styled, so test every short one.
			if len(text) <= areaTitleMaxLen {
				if code, label, ok := areaFromHeading(text); ok {
					flush(text)
					headingArea, headingLabel = code, label
					lastPara = ""
					paraSinceTbl = true
					continue
				}
			}

			// "PoC:" and "Recommendations:" introduce runs of paragraphs that
			// belong to the finding above them but sit outside its table.
			if pending != nil {
				switch {
				case pocHeadingRe.MatchString(text):
					section = "poc"
					lastPara = ""
					paraSinceTbl = true
					continue
				case recHeadingRe.MatchString(text):
					section = "rec"
					lastPara = ""
					paraSinceTbl = true
					continue
				}
				switch section {
				case "poc":
					pocLines = append(pocLines, text)
				case "rec":
					recLines = append(recLines, text)
				}
			}

			if isHeading(c.Style) {
				if code, label, ok := areaFromHeading(text); ok {
					flush(text)
					headingArea, headingLabel = code, label
					lastPara = ""
					paraSinceTbl = true
					continue
				}
				// A heading that is not an area ends the previous area's run
				// only when it is at or above the level areas sit at.
				if headingLevel(c.Style) <= 2 {
					headingArea, headingLabel = "", ""
				}
			}
			lastPara = text
			paraSinceTbl = true
			continue
		}

		if c.Tag != "w:tbl" {
			continue
		}

		// A finding's table names what it is describing. The vulnerability
		// register also carries a Recommendation column, so recognising a table
		// by that alone would read the register as one more finding.
		hasDesc := extractTableHasLabel(frag, "description")
		hasAffected := extractTableHasLabel(frag, "affected")
		if !hasDesc && !hasAffected {
			continue
		}
		sawTable = true

		f := findingFromTable(frag)

		// NAR splits one finding across two tables - the description in the
		// first, the affected network and the recommendation in the second.
		// A table with no description of its own, arriving with no paragraph
		// since the last one, is the rest of the finding above it.
		if !hasDesc && pending != nil && !paraSinceTbl {
			mergeFinding(pending, f)
			lastPara = ""
			paraSinceTbl = false
			continue
		}

		flush(lastPara)
		if f.Title == "" {
			f.Title = lastPara
		}
		f.Title = cleanTitle(f.Title)
		if strings.TrimSpace(f.Title) == "" && strings.TrimSpace(f.Description) == "" {
			lastPara = ""
			paraSinceTbl = false
			continue
		}
		placeArea(&f, headingArea, headingLabel)
		pending = &f
		lastPara = ""
		paraSinceTbl = false
	}
	flush("")

	if skipped > 0 {
		notes = append(notes, fmt.Sprintf("%d finding%s in the document were left as XXXX placeholders and were not imported.", skipped, map[bool]string{true: "", false: "s"}[skipped == 1]))
	}

	if !sawTable {
		notes = append(notes, "No finding tables were recognised, so nothing could be read from this document. A report laid out as a table of Description, Rating and Recommendation rows reads best.")
	}
	return out, notes, nil
}

// areaTitleMaxLen keeps the whole-paragraph area test to things that could
// plausibly be a section title rather than a sentence that happens to end in
// an area's name.
const areaTitleMaxLen = 80

// areaFromHeading matches a heading against the assessment areas, tolerating a
// leading section number ("3.7 Configuration Files Review").
func areaFromHeading(text string) (string, string, bool) {
	trimmed := strings.TrimSpace(stripSectionNumber(text))
	for _, a := range reportAreas {
		for _, name := range []string{a.Heading, a.Label, a.ScopeRow} {
			if name != "" && strings.EqualFold(trimmed, name) {
				return a.Code, "heading “" + trimmed + "”", true
			}
		}
	}
	return "", "", false
}

var sectionNumRe = regexp.MustCompile(`^\s*\d+(\.\d+)*\.?\s+`)

func stripSectionNumber(s string) string { return sectionNumRe.ReplaceAllString(s, "") }

// cleanTitle drops the template's own vulnerability placeholder and any
// numbering in front of a finding's name.
func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	if vulnHeadingRe.MatchString(s) {
		return ""
	}
	s = strings.TrimSpace(stripSectionNumber(s))
	if trimmed := strings.TrimSpace(editorialSuffixRe.ReplaceAllString(s, "")); trimmed != "" {
		s = trimmed
	}
	return s
}

// mergeFinding folds a continuation table into the finding it belongs to,
// filling only what is still empty.
func mergeFinding(dst *ExtractedFinding, src ExtractedFinding) {
	fill := func(d *string, v string) {
		if *d == "" {
			*d = v
		}
	}
	fill(&dst.Description, src.Description)
	fill(&dst.Impact, src.Impact)
	fill(&dst.CVSSVector, src.CVSSVector)
	fill(&dst.AttackVector, src.AttackVector)
	fill(&dst.AffectedSystem, src.AffectedSystem)
	fill(&dst.POC, src.POC)
	fill(&dst.Remediation, src.Remediation)
	if dst.Severity == "" || dst.Severity == "info" {
		if src.Severity != "" && src.Severity != "info" {
			dst.Severity = src.Severity
		}
	}
}

// cellText is a cell's paragraphs, joined by newlines. elemText would run them
// together, which turns a two-paragraph proof of concept into one long line.
func cellText(cell string) string {
	var lines []string
	for _, p := range childElems(cell, "w:p") {
		if t := strings.TrimSpace(elemText(cell[p.Start:p.End])); t != "" {
			lines = append(lines, t)
		}
	}
	return strings.Join(lines, "\n")
}

// recommendationFromCell drops the "<VulnID> - <header>" line the report prints
// above the recommendation body. Where the two share one paragraph, the id is
// stripped off the front instead.
func recommendationFromCell(cell string) string {
	var lines []string
	for _, p := range childElems(cell, "w:p") {
		if t := strings.TrimSpace(elemText(cell[p.Start:p.End])); t != "" {
			lines = append(lines, t)
		}
	}
	if len(lines) > 1 && recIDRe.MatchString(lines[0]) {
		lines = lines[1:]
	}
	return stripRecommendationID(strings.Join(lines, "\n"))
}

// findingFromTable reads the labelled rows of one finding table. It is the
// inverse of fillDetailTable: the label row names the field, the row under it
// holds the value.
// Reading a report that was not written by this app.
//
// The renderer's own layout puts a row of labels above a row of values, and
// detailLabels is the exact vocabulary it prints. A report written by hand does
// neither: the ECG draft lays every finding out as a two-column table with the
// label beside its value, and calls the rows "Severity:", "Affected URL" and
// "CVSS 3.1" rather than "Rating", "Affected Hosts" and "CVSS Vector". Fifty-five
// findings were laid out that way and five were read - the five whose labels
// happened to carry no trailing colon.
//
// So the extractor keeps its own vocabulary, a superset of the renderer's.
// detailLabels stays exactly as it is: the renderer resolves real template
// cells through it and must not start matching things the template never
// prints.

// extractLabels is detailLabels plus the spellings found in reports written
// outside this app. Keys are normalised by normalizeLabel, so no entry here
// needs its own punctuation or casing variants.
var extractLabels = func() map[string]string {
	m := make(map[string]string, len(detailLabels)+24)
	for k, v := range detailLabels {
		m[normalizeLabel(k)] = v
	}
	for k, v := range map[string]string{
		"severity":                 "rating",
		"risk":                     "rating",
		"risk rating":              "rating",
		"risk level":               "rating",
		"cvss":                     "cvss",
		"cvss 3.1":                 "cvss",
		"cvss v3.1":                "cvss",
		"cvss 3.1 vector":          "cvss",
		"vector string":            "cvss",
		"affected url":             "affected",
		"affected urls":            "affected",
		"affected app":             "affected",
		"affected apps":            "affected",
		"affected app & endpoint":  "affected",
		"affected app & endpoints": "affected",
		"affected endpoints":       "affected",
		"affected system":          "affected",
		"affected systems":         "affected",
		"affected asset":           "affected",
		"affected assets":          "affected",
		"proof of concept":         "poc",
		"remediation":              "recommendation",
		"recommendations":          "recommendation",
		"mitigation":               "recommendation",
	} {
		m[k] = v
	}
	return m
}()

// normalizeLabel reduces a label cell to the form extractLabels is keyed by.
// A hand-written report is inconsistent about the trailing colon and the
// casing - "Severity:", "Severity" and "SEVERITY" are one label - and about
// the spaces around an ampersand.
func normalizeLabel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimRight(s, " 	:. ")
	return strings.Join(strings.Fields(s), " ")
}

// extractLabelKey resolves a cell's text to a finding field, or reports that
// the cell is not a label at all.
func extractLabelKey(text string) (string, bool) {
	key, ok := extractLabels[normalizeLabel(text)]
	return key, ok
}

// extractTableHasLabel is tableHasLabel over the wider vocabulary. The renderer
// keeps its own; this one decides whether a table is a finding worth reading.
func extractTableHasLabel(tbl, key string) bool {
	for _, r := range tableRows(tbl) {
		row := tbl[r.Start:r.End]
		for _, c := range rowCells(row) {
			if k, ok := extractLabelKey(elemText(row[c.Start:c.End])); ok && k == key {
				return true
			}
		}
	}
	return false
}

// A hand-written report marks its sections and its extra fields in ways the
// renderer never does, and all of them have to be read off the page rather than
// off a style.

// editorialSuffixRe strips the working notes a draft carries in its titles -
// "- Done", "– Done (Added to 1.9)", "– Dup". They are the author talking to
// themselves, not part of the finding's name.
var editorialSuffixRe = regexp.MustCompile(`(?i)\s*[-–—]\s*(done|dup(licate)?s?|pending|todo|wip|n/?a)\b.*$`)

// pocHeadingRe and recHeadingRe are the paragraphs that introduce a finding's
// proof and its fix. In this layout neither is a row of the table - they follow
// it as ordinary bold paragraphs, so a finding read from the table alone loses
// both.
var pocHeadingRe = regexp.MustCompile(`(?i)^\s*(poc|proof of concept)\s*:?\s*$`)
var recHeadingRe = regexp.MustCompile(`(?i)^\s*(recommendations?|remediations?|mitigations?)\s*:?\s*$`)

// draftPlaceholderRe matches the XXXX a draft leaves where a field is not written
// yet. Eight of the ECG draft's fifty-five finding tables were entirely this.
var draftPlaceholderRe = regexp.MustCompile(`^[\s.–-]*[xX]{2,}[\s.–-]*$`)

func isPlaceholderText(s string) bool { return draftPlaceholderRe.MatchString(strings.TrimSpace(s)) }

// isPlaceholderFinding reports a finding that carries no written content at
// all. Importing these would put empty rows in an engagement that a person then
// has to find and delete.
func isPlaceholderFinding(f *ExtractedFinding) bool {
	filled := 0
	for _, v := range []string{f.Title, f.Description, f.Impact, f.AffectedSystem, f.CVSSVector} {
		if strings.TrimSpace(v) != "" && !isPlaceholderText(v) {
			filled++
		}
	}
	return filled == 0
}

// dropTrailing removes a trailing paragraph equal to text. The paragraph that
// names the next finding sits inside the previous finding's recommendation run,
// because nothing in the document closes that run.
func dropTrailing(lines []string, text string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == strings.TrimSpace(text) {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func findingFromTable(tbl string) ExtractedFinding {
	var f ExtractedFinding
	rows := tableRows(tbl)

	// Label beside value, one row per field. This is how a report written
	// outside the app tends to be laid out, and it is unambiguous: a row whose
	// first cell is a label and whose second cell is not.
	for _, r := range rows {
		row := tbl[r.Start:r.End]
		cells := rowCells(row)
		if len(cells) < 2 {
			continue
		}
		key, ok := extractLabelKey(elemText(row[cells[0].Start:cells[0].End]))
		if !ok {
			continue
		}
		valueCell := row[cells[1].Start:cells[1].End]
		// The renderer's own layout puts labels side by side - "Description"
		// next to "Rating" - above the row holding their values. Reading that
		// horizontally would file one label as another's value.
		if _, alsoLabel := extractLabelKey(elemText(valueCell)); alsoLabel {
			continue
		}
		setFindingField(&f, key, cellText(valueCell), valueCell)
	}

	// Label row above value row, which is what this app's own template prints.
	for ri := 0; ri+1 < len(rows); ri++ {
		row := tbl[rows[ri].Start:rows[ri].End]
		next := tbl[rows[ri+1].Start:rows[ri+1].End]
		nextCells := rowCells(next)
		if len(nextCells) == 0 {
			continue
		}
		for ci, c := range rowCells(row) {
			key, ok := extractLabelKey(elemText(row[c.Start:c.End]))
			if !ok {
				continue
			}
			target := ci
			if target >= len(nextCells) {
				target = 0
			}
			cell := next[nextCells[target].Start:nextCells[target].End]
			setFindingField(&f, key, cellText(cell), cell)
		}
	}

	if f.Severity == "" && f.CVSSVector != "" {
		f.Severity = severityFromVector(f.CVSSVector)
	}
	if f.Severity == "" {
		f.Severity = "info"
	}
	return f
}

// setFindingField files one label's value. It never overwrites something
// already read, so whichever layout matched first wins and a stray second
// match cannot undo it.
func setFindingField(f *ExtractedFinding, key, value, cell string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	switch key {
	case "description":
		if f.Description == "" {
			f.Description = value
		}
	case "impact":
		if f.Impact == "" {
			f.Impact = value
		}
	case "rating":
		if f.Severity == "" {
			f.Severity = normalizeSeverity(value)
		}
		// A hand-written rating cell often carries the score with the word:
		// "High (7.5)", "Critical(9.5)", "HIGH 7.5". The score is worth keeping
		// and is not recorded anywhere else in these reports.
		if f.CVSSScore == 0 {
			if m := cvssScoreRe.FindString(value); m != "" {
				if n, err := strconv.ParseFloat(m, 64); err == nil {
					f.CVSSScore = n
				}
			}
		}
	case "cvss":
		if f.CVSSVector == "" {
			f.CVSSVector = value
		}
	case "attackvector":
		if f.AttackVector == "" {
			f.AttackVector = value
		}
	case "affected":
		if f.AffectedSystem == "" {
			f.AffectedSystem = value
		}
	case "poc":
		if f.POC == "" {
			f.POC = value
		}
	case "recommendation":
		if f.Remediation == "" {
			f.Remediation = recommendationFromCell(cell)
		}
	}
}

// recIDRe matches the "<Initials>_REC<n>_<AREA><n> - <header>" line the template
// prints above a recommendation body.
var recIDRe = regexp.MustCompile(`(?i)^[A-Z0-9]+_REC\s*\d+_[A-Z]+\s*\d+\s*[\x{2013}\x{2014}-]\s*`)

func stripRecommendationID(s string) string {
	s = strings.TrimSpace(s)
	if m := recIDRe.FindString(s); m != "" {
		return strings.TrimSpace(s[len(m):])
	}
	return s
}

// severityFromVector reads the CVSS base metrics far enough to band a finding
// when the document gave a vector but no rating.
func severityFromVector(vec string) string {
	if score, ok := cvssBaseScore(vec); ok {
		return severityFromScore(score)
	}
	return ""
}

// ---------------------------------------------------------------------------
// PDF
// ---------------------------------------------------------------------------

// pdfLabels maps a line's leading label to the field it fills. PDFs lose the
// table, so the labels arrive as run-on text and are matched by prefix.
var pdfLabels = []struct {
	prefix string
	key    string
}{
	{"description", "description"},
	{"impact", "impact"},
	{"rating", "rating"},
	{"severity", "rating"},
	{"criticality", "rating"},
	{"cvss vector string", "cvss"},
	{"cvss vector", "cvss"},
	{"cvss", "cvss"},
	{"attack vector", "attackvector"},
	{"affected hosts", "affected"},
	{"affected host", "affected"},
	{"affected application", "affected"},
	{"affected device", "affected"},
	{"affected endpoint", "affected"},
	{"affected domain", "affected"},
	{"affected network", "affected"},
	{"affected ssids", "affected"},
	{"affected system", "affected"},
	{"poc", "poc"},
	{"proof of concept", "poc"},
	{"recommendation", "recommendation"},
	{"remediation", "recommendation"},
}

// ExtractFromPDFText reads findings out of a PDF, given its pages as visual
// rows - one string per line as it appears on the page.
//
// A PDF has no tables left in it, only rows of text, so this is looser than the
// DOCX path by nature and says so. What makes it work at all on a report from
// this template is the numbering: a finding is a three-level heading ("3.3.2
// Unsupported Windows Server 2012 R2 hosts") under a two-level area heading,
// and the labelled rows follow it. A document from elsewhere still works as far
// as it labels its rows.
//
// Two rules keep the noise out. A finding only starts at a heading or a
// Description, never at any other label - the vulnerability register's own
// "Recommendation" column heading would otherwise open one. And a candidate is
// kept only if it has a title and something written under it, which is what
// drops the table of contents, whose entries are headings with nothing beneath.
func ExtractFromPDFText(text string) ([]ExtractedFinding, []string) {
	rows := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	repeated := repeatedRows(rows)

	var (
		out          []ExtractedFinding
		notes        []string
		cur          *ExtractedFinding
		curKey       string
		headingArea  string
		headingLabel string
		page         = 1
	)

	flush := func() {
		if cur == nil {
			return
		}
		// A heading with nothing under it is a table-of-contents entry.
		hasBody := strings.TrimSpace(cur.Description) != "" || strings.TrimSpace(cur.Remediation) != ""
		if strings.TrimSpace(cur.Title) != "" && hasBody {
			cur.Remediation = stripRecommendationID(cur.Remediation)
			if cur.Severity == "" {
				cur.Severity = severityFromVector(cur.CVSSVector)
			}
			if cur.Severity == "" {
				cur.Severity = "info"
			}
			placeArea(cur, headingArea, headingLabel)
			out = append(out, *cur)
		}
		cur = nil
		curKey = ""
	}

	appendTo := func(key, value string) {
		if cur == nil || value == "" {
			return
		}
		join := func(dst *string, sep string) {
			if *dst == "" {
				*dst = value
			} else {
				*dst += sep + value
			}
		}
		switch key {
		case "description":
			join(&cur.Description, " ")
		case "impact":
			join(&cur.Impact, " ")
		case "cvss":
			join(&cur.CVSSVector, " ")
		case "attackvector":
			join(&cur.AttackVector, " ")
		case "affected":
			join(&cur.AffectedSystem, " ")
		case "poc":
			join(&cur.POC, "\n")
		case "recommendation":
			// The "<id> - <header>" line repeats the sentence under it, so the
			// id is not all that has to go.
			if recIDRe.MatchString(value) {
				return
			}
			join(&cur.Remediation, "\n")
		case "rating":
			if cur.Severity == "" {
				cur.Severity = normalizeSeverity(value)
			}
		}
	}

	for _, raw := range rows {
		if strings.Contains(raw, "\f") {
			page++
		}
		line := stripDotLeaders(strings.TrimSpace(strings.ReplaceAll(raw, "\f", "")))
		if line == "" {
			continue
		}

		if code, label, ok := areaFromHeading(line); ok {
			flush()
			headingArea, headingLabel = code, label
			continue
		}

		if title, ok := findingHeading(line); ok {
			flush()
			cur = &ExtractedFinding{Ref: "p." + strconv.Itoa(page), Title: title}
			continue
		}

		keys, ok := pdfLabelRow(line)
		if ok {
			// "Description Rating" is one row with two labels over one row of
			// two values; the rating is the severity word on the end of it.
			if keys[0] == "description" && cur != nil && cur.Description != "" {
				flush()
			}
			if cur == nil {
				if keys[0] != "description" {
					continue // a stray column heading, not the start of a finding
				}
				cur = &ExtractedFinding{Ref: "p." + strconv.Itoa(page)}
			}
			curKey = keys[0]
			if len(keys) > 1 && keys[1] == "rating" {
				curKey = "description+rating"
			}
			continue
		}

		// The rating cell sits beside the description, so depending on how long
		// the description is it lands either as a row of its own or in the
		// middle of one. This is checked before the furniture filter because a
		// rating word repeats once per finding of that severity.
		if cur != nil && (curKey == "description+rating" || curKey == "rating") {
			if sev, ok := ratingWords[line]; ok {
				if cur.Severity == "" {
					cur.Severity = sev
				}
				continue
			}
		}

		// A running header or footer, which otherwise lands in whichever field
		// was open when the page broke.
		if repeated[furnitureKey(line)] {
			continue
		}

		if value, rest, ok := pdfLabelledInline(line); ok && cur != nil {
			appendTo(value, rest)
			curKey = value
			continue
		}

		if cur == nil || curKey == "" {
			continue
		}
		if curKey == "description+rating" {
			body, sev := line, ""
			if cur.Severity == "" {
				body, sev = splitRatingWord(line)
			}
			appendTo("description", body)
			if sev != "" {
				cur.Severity = sev
			}
			continue
		}
		appendTo(curKey, line)
	}
	flush()

	notes = append(notes, "Read from a PDF. A PDF keeps the words but not the table they sat in, so check the fields as well as the areas — the DOCX of the same report reads exactly.")
	if len(out) == 0 {
		notes = []string{"No findings were recognised in this PDF. If it is a scan the pages are images and there is nothing to read; otherwise import the DOCX of the same report, which reads exactly."}
	}
	return out, notes
}

// dotLeaderRe is a table-of-contents line's trailing dots and page number.
var dotLeaderRe = regexp.MustCompile(`\s*\.{3,}\s*\d*\s*$`)

func stripDotLeaders(s string) string { return strings.TrimSpace(dotLeaderRe.ReplaceAllString(s, "")) }

// findingHeadingRe is the template's vulnerability heading: a three-level
// section number and a name.
var findingHeadingRe = regexp.MustCompile(`^(\d+\.\d+\.\d+)\.?\s+(\S.*)$`)

func findingHeading(line string) (string, bool) {
	m := findingHeadingRe.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	title := strings.TrimSpace(m[2])
	if len(title) < 4 {
		return "", false
	}
	return title, true
}

// pdfLabelRow reads a row that is nothing but labels - "Recommendation" on its
// own, or "Description Rating" where two cells sat side by side.
func pdfLabelRow(line string) ([]string, bool) {
	low := strings.ToLower(strings.TrimSpace(line))
	if low == "" || len(low) > 40 {
		return nil, false
	}
	var keys []string
	rest := low
	for rest != "" {
		matched := false
		for _, l := range pdfLabels {
			if strings.HasPrefix(rest, l.prefix) {
				keys = append(keys, l.key)
				rest = strings.TrimSpace(rest[len(l.prefix):])
				matched = true
				break
			}
		}
		if !matched {
			return nil, false
		}
	}
	if len(keys) == 0 {
		return nil, false
	}
	return keys, true
}

// pdfLabelledInline reads "Affected Host: 10.0.0.1" - a label and its value on
// one line, which is how a document that is not built from tables writes them.
func pdfLabelledInline(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx <= 0 || idx > 40 {
		return "", "", false
	}
	head := strings.ToLower(strings.TrimSpace(line[:idx]))
	rest := strings.TrimSpace(line[idx+1:])
	for _, l := range pdfLabels {
		if head != l.prefix {
			continue
		}
		// A CVSS vector string starts "CVSS:3.1/..." - that colon belongs to
		// the value, and splitting on it would strip the head off the vector.
		if l.key == "cvss" && rest != "" && rest[0] >= '0' && rest[0] <= '9' {
			return "", "", false
		}
		return l.key, rest, true
	}
	return "", "", false
}

// repeatedRows are the running headers and footers: rows that repeat page after
// page at the top or the bottom of the page. Rebuilt rows carry them inline, so
// without this the footer lands in whichever field was open when the page broke.
//
// Repetition alone is not enough to call a row furniture - "Adjacent Network"
// is the attack vector of three findings in a row and repeats exactly like a
// footer does. What separates them is position: furniture sits at the edge of
// every page, and a value sits in the middle of one.
func repeatedRows(rows []string) map[string]bool {
	const (
		repeatsToBeFurniture = 3
		edgeRows             = 2 // how far in from the top and bottom to look
	)

	var pages [][]string
	page := []string{}
	for _, r := range rows {
		if strings.Contains(r, "\f") {
			pages = append(pages, page)
			page = []string{}
			continue
		}
		if line := strings.TrimSpace(r); line != "" {
			page = append(page, line)
		}
	}
	pages = append(pages, page)

	count := map[string]int{}
	for _, p := range pages {
		// A page with no middle has no furniture to tell from its content.
		if len(p) <= 2*edgeRows {
			continue
		}
		seen := map[string]bool{}
		for i, line := range p {
			atEdge := i < edgeRows || i >= len(p)-edgeRows
			if !atEdge || len(line) > 120 {
				continue
			}
			key := furnitureKey(line)
			if seen[key] {
				continue // once per page, however many times it appears on it
			}
			seen[key] = true
			count[key]++
		}
	}

	out := map[string]bool{}
	for key, n := range count {
		if n >= repeatsToBeFurniture {
			out[key] = true
		}
	}
	return out
}

// pageNumberRe is the page number a footer carries, which is what stops two
// occurrences of the same footer from looking the same.
var pageNumberRe = regexp.MustCompile(`(?i)\s*(page\s+)?\d+\s*$`)

func furnitureKey(line string) string {
	return strings.TrimSpace(pageNumberRe.ReplaceAllString(line, ""))
}

// ratingWords are the rating cell's exact wordings. The comparison is
// case-sensitive on purpose: prose says "a high risk", the rating cell says
// "High", and only the capitalised form is the cell.
var ratingWords = map[string]string{
	"Critical": "critical", "High": "high", "Medium": "medium",
	"Low": "low", "Informational": "info", "Info": "info",
}

// splitRatingWord pulls the rating out of a row that carried both a description
// and a rating. Rows are rebuilt from position, so the rating cell's word lands
// wherever that cell sat - often in the middle of the sentence beside it rather
// than on the end - and taking only a trailing word would miss it.
func splitRatingWord(line string) (string, string) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return line, ""
	}
	for i, f := range fields {
		sev, ok := ratingWords[strings.Trim(f, ".,;:")]
		if !ok {
			continue
		}
		rest := append(append([]string{}, fields[:i]...), fields[i+1:]...)
		return strings.TrimSpace(strings.Join(rest, " ")), sev
	}
	return line, ""
}

// ---------------------------------------------------------------------------
// assembling the result
// ---------------------------------------------------------------------------

// BuildExtractResult tallies the candidates for the review screen.
func BuildExtractResult(filename, kind string, findings []ExtractedFinding, notes []string) ExtractResult {
	byArea := map[string]int{}
	unplaced := 0
	for _, f := range findings {
		if f.Category == "" {
			unplaced++
			continue
		}
		byArea[f.Category]++
	}
	if findings == nil {
		findings = []ExtractedFinding{}
	}
	return ExtractResult{
		Filename: filename,
		Kind:     kind,
		Findings: findings,
		ByArea:   byArea,
		Unplaced: unplaced,
		Notes:    notes,
	}
}
