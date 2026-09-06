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
	)

	flush := func() {
		if pending != nil {
			out = append(out, *pending)
			pending = nil
		}
	}

	for _, c := range children {
		frag := doc[c.Start:c.End]

		if c.Tag == "w:p" {
			text := strings.TrimSpace(elemText(frag))
			if text == "" {
				continue
			}
			if isHeading(c.Style) {
				if code, label, ok := areaFromHeading(text); ok {
					flush()
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
		hasDesc := tableHasLabel(frag, "description")
		hasAffected := tableHasLabel(frag, "affected")
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

		flush()
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
	flush()

	if !sawTable {
		notes = append(notes, "No finding tables were recognised, so nothing could be read from this document. A report laid out as a table of Description, Rating and Recommendation rows reads best.")
	}
	return out, notes, nil
}

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
	return strings.TrimSpace(stripSectionNumber(s))
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
func findingFromTable(tbl string) ExtractedFinding {
	var f ExtractedFinding
	rows := tableRows(tbl)

	for ri := 0; ri+1 < len(rows); ri++ {
		row := tbl[rows[ri].Start:rows[ri].End]
		next := tbl[rows[ri+1].Start:rows[ri+1].End]
		nextCells := rowCells(next)
		if len(nextCells) == 0 {
			continue
		}
		for ci, c := range rowCells(row) {
			label := strings.ToLower(strings.TrimSpace(elemText(row[c.Start:c.End])))
			key, ok := detailLabels[label]
			if !ok {
				continue
			}
			target := ci
			if target >= len(nextCells) {
				target = 0
			}
			cell := next[nextCells[target].Start:nextCells[target].End]
			value := cellText(cell)
			if value == "" {
				continue
			}
			switch key {
			case "description":
				f.Description = value
			case "impact":
				f.Impact = value
			case "rating":
				f.Severity = normalizeSeverity(value)
			case "cvss":
				f.CVSSVector = value
			case "attackvector":
				f.AttackVector = value
			case "affected":
				f.AffectedSystem = value
			case "poc":
				f.POC = value
			case "recommendation":
				f.Remediation = recommendationFromCell(cell)
			}
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
	{"affected domain", "affected"},
	{"affected network", "affected"},
	{"affected ssids", "affected"},
	{"affected system", "affected"},
	{"poc", "poc"},
	{"proof of concept", "poc"},
	{"recommendation", "recommendation"},
	{"remediation", "recommendation"},
}

// ExtractFromPDFText reads findings out of a PDF's text.
//
// A PDF has no tables left in it, only lines, so this is looser than the DOCX
// path by nature: it walks the text watching for area headings and for the
// labelled lines a finding is made of, and starts a new finding whenever a
// second Description appears. Whatever it produces is reviewed before anything
// is saved, which is why a looser parse is worth having at all.
func ExtractFromPDFText(text string) ([]ExtractedFinding, []string) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")

	var (
		out          []ExtractedFinding
		notes        []string
		cur          *ExtractedFinding
		curKey       string
		headingArea  string
		headingLabel string
		lastLine     string
		page         = 1
	)

	flush := func() {
		if cur == nil {
			return
		}
		if strings.TrimSpace(cur.Title) == "" && strings.TrimSpace(cur.Description) == "" {
			cur = nil
			return
		}
		cur.Title = cleanTitle(cur.Title)
		cur.Remediation = stripRecommendationID(cur.Remediation)
		if cur.Severity == "" {
			cur.Severity = "info"
		}
		placeArea(cur, headingArea, headingLabel)
		out = append(out, *cur)
		cur = nil
	}

	appendTo := func(key, value string) {
		if cur == nil || value == "" {
			return
		}
		join := func(dst *string) {
			if *dst == "" {
				*dst = value
			} else {
				*dst += " " + value
			}
		}
		switch key {
		case "description":
			join(&cur.Description)
		case "impact":
			join(&cur.Impact)
		case "cvss":
			join(&cur.CVSSVector)
		case "attackvector":
			join(&cur.AttackVector)
		case "affected":
			join(&cur.AffectedSystem)
		case "poc":
			join(&cur.POC)
		case "recommendation":
			join(&cur.Remediation)
		case "rating":
			if cur.Severity == "" {
				cur.Severity = normalizeSeverity(value)
			}
		}
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "\f" || strings.Contains(raw, "\f") {
			page++
		}
		if line == "" {
			continue
		}

		if code, label, ok := areaFromHeading(line); ok {
			flush()
			headingArea, headingLabel = code, label
			lastLine = ""
			curKey = ""
			continue
		}

		if key, value, ok := pdfLabelled(line); ok {
			if key == "description" && cur != nil && cur.Description != "" {
				flush()
			}
			if cur == nil {
				cur = &ExtractedFinding{Ref: "p." + strconv.Itoa(page), Title: lastLine}
			}
			curKey = key
			appendTo(key, value)
			continue
		}

		if cur != nil && curKey != "" {
			appendTo(curKey, line)
			continue
		}
		lastLine = line
	}
	flush()

	if len(out) == 0 {
		notes = append(notes, "No findings were recognised in this PDF. PDFs keep the words but not the table they sat in, so a report whose findings are labelled Description, Rating and Recommendation reads best - the DOCX of the same report reads better still.")
	}
	return out, notes
}

// pdfLabelled splits "Affected Host: 10.0.0.1" into its field and value. It also
// accepts a bare label on its own line, which is how a table row usually lands.
func pdfLabelled(line string) (string, string, bool) {
	low := strings.ToLower(line)
	for _, l := range pdfLabels {
		if !strings.HasPrefix(low, l.prefix) {
			continue
		}
		rest := strings.TrimSpace(line[len(l.prefix):])
		rest = strings.TrimSpace(strings.TrimPrefix(rest, ":"))
		// "Description of the estate" is prose, not a Description row.
		if rest != "" && !strings.HasPrefix(strings.TrimSpace(line[len(l.prefix):]), ":") {
			return "", "", false
		}
		return l.key, rest, true
	}
	return "", "", false
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
