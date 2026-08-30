package main

// docxmcollab.go renders the mCollaborator VAPT template. Every placeholder the
// template carries in square brackets is filled from the wizard's answers, and
// every part of the document that has to track the engagement - the scope table,
// the recommendations naming convention, the two summary charts, the chapter 3
// assessment sections and the appendix - is rebuilt from the selected areas and
// the selected findings rather than left at the template's example values.

import (
	_ "embed"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed templates/mcollaborator_template.docx
var mcollabTemplateDocx []byte

// ---------------------------------------------------------------------------
// assessment areas
// ---------------------------------------------------------------------------

// areaDef ties one assessment area to the four places the template mentions it:
// the scope table, the naming convention list, its chapter 3 section heading and
// its bar in the findings-by-area chart.
type areaDef struct {
	Code       string // IPT, EPT, ... - also the vulnerability id infix
	Label      string // wizard label / naming convention description
	ScopeRow   string // activity name in the 2.3 Scope table
	Heading    string // chapter 3 heading, "" when the template has no block
	ChartLabel string // category label in the findings-by-area chart
	Exposure   string // vulnerability register "Exposure" column
}

// reportAreas is in template document order. The chapter 3 region is rebuilt in
// this order, so a selection always reads the same way round.
var reportAreas = []areaDef{
	{"IPT", "Internal Penetration Testing", "Internal Penetration Testing", "Internal Penetration Testing", "Internal", "Internal"},
	{"EPT", "External Penetration Testing", "External Penetration Testing", "External Penetration Testing", "External", "External"},
	{"IPTC", "Internal Cloud Penetration Testing", "Internal Cloud Penetration Testing", "Internal Cloud Penetration Testing", "Internal Cloud", "Internal Cloud"},
	{"WPT", "Web Application Penetration Testing", "Web Application Testing", "Web Application Penetration Testing", "Web Apps", "Web"},
	{"CFG", "Configuration Files Review", "Configuration Files Review", "Configuration Files Review", "Config Review", "Configuration"},
	{"ASA", "API Security Assessment", "API Security Assessment", "", "APIs", "API"},
	{"ADT", "Active Directory Testing", "Active Directory Testing", "Active Directory Testing", "Active Directory", "Internal"},
	{"WNA", "Wireless Network Assessment", "Wireless Network Assessment", "Wireless Network Penetration Testing", "Wireless", "Wireless"},
	{"NAR", "Network Architecture Review", "Network Architecture Review", "Network Architecture Review", "Network Architecture", "Internal"},
}

// severityHex returns the colour the criticality word itself is printed in.
//
// Only the text is coloured. The cell keeps whatever background the template
// gives it - which for both the vulnerability register and a finding's Rating
// row is none - so the criticality reads as a coloured word on the page rather
// than a solid block of colour with white text knocked out of it.
//
// The values are the exact fills the criticality-criteria table in the template
// uses for each band (docs/colors.PNG), so a severity reads as the same colour
// wherever it appears in the report. They are sampled from that legend, not
// picked for contrast on white - do not "improve" one of them in isolation.
func severityHex(sev string) string {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return "FF0000"
	case "high":
		return "F68831"
	case "medium":
		return "FFC000"
	case "low":
		return "92D050"
	default:
		return "00B0F0"
	}
}

func areaByCode(code string) (areaDef, bool) {
	for _, a := range reportAreas {
		if strings.EqualFold(a.Code, code) {
			return a, true
		}
	}
	return areaDef{}, false
}

// legacyCategoryArea maps the finding categories the app stored before areas
// existed onto an area code, so older engagements still land in a section.
var legacyCategoryArea = map[string]string{
	"web":          "WPT",
	"webapp":       "WPT",
	"external":     "EPT",
	"internal":     "IPT",
	"cloud":        "IPTC",
	"api":          "ASA",
	"config":       "CFG",
	"wireless":     "WNA",
	"ad":           "ADT",
	"architecture": "NAR",
	"network":      "NAR",
}

// areaCodeOf reads an area code out of a free-form field, accepting both the
// codes the finding editor now stores and the loose categories it stored before.
func areaCodeOf(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if area, ok := areaByCode(value); ok {
		return area.Code, true
	}
	code, ok := legacyCategoryArea[strings.ToLower(value)]
	return code, ok
}

// findingArea resolves the area a finding belongs to: its own area if it names
// one, else its category, else the first selected area so nothing is silently
// dropped from the report.
func findingArea(f ReportFinding, selected []string) string {
	if code, ok := areaCodeOf(f.Area); ok {
		return code
	}
	if code, ok := areaCodeOf(f.Category); ok {
		for _, s := range selected {
			if strings.EqualFold(s, code) {
				return code
			}
		}
	}
	if len(selected) > 0 {
		return strings.ToUpper(selected[0])
	}
	return "WPT"
}

// ---------------------------------------------------------------------------
// per-finding numbering
// ---------------------------------------------------------------------------

// numberedFinding is a finding once it knows where it sits in the report: which
// area section it prints under, its position within that section, and the
// recommendation number that its vulnerability id is built from.
type numberedFinding struct {
	ReportFinding
	Area     areaDef
	AreaIdx  int // 1-based position within the area
	RecIdx   int // 1-based recommendation number across the whole report
	VulnID   string
	RecTitle string
}

// severityRank orders findings most severe first for the register and the
// executive summary's "key areas of concern" sentence.
func severityRank(sev string) int {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	default:
		return 4
	}
}

// buildNumberedFindings assigns areas, per-area indexes and recommendation
// numbers. Findings are grouped by area in template order and, inside an area,
// sorted most severe first so the report reads top-down by risk.
func buildNumberedFindings(config ReportConfig) []numberedFinding {
	selected := selectedAreaCodes(config)
	byArea := map[string][]ReportFinding{}
	for _, f := range config.Findings {
		code := findingArea(f, selected)
		byArea[code] = append(byArea[code], f)
	}

	var out []numberedFinding
	rec := 0
	for _, code := range selected {
		area, ok := areaByCode(code)
		if !ok {
			continue
		}
		list := byArea[code]
		sort.SliceStable(list, func(i, j int) bool {
			return severityRank(list[i].Severity) < severityRank(list[j].Severity)
		})
		for i, f := range list {
			rec++
			nf := numberedFinding{ReportFinding: f, Area: area, AreaIdx: i + 1, RecIdx: rec}
			nf.VulnID = fmt.Sprintf("%s_REC%d_%s%d", initialsOrDash(config), nf.RecIdx, area.Code, nf.AreaIdx)
			nf.RecTitle = recommendationTitle(f)
			out = append(out, nf)
		}
	}
	return out
}

func initialsOrDash(config ReportConfig) string {
	if s := strings.TrimSpace(config.CompanyInitials); s != "" {
		return s
	}
	return companyInitialsFrom(config.CompanyName)
}

// companyInitialsFrom derives initials from the company name when the tester did
// not supply them, e.g. "BestPoint Savings & Loans" -> "BSL".
func companyInitialsFrom(name string) string {
	var b strings.Builder
	for _, word := range strings.Fields(name) {
		r := []rune(word)
		if len(r) == 0 || !isLetterRune(r[0]) {
			continue
		}
		b.WriteString(strings.ToUpper(string(r[0])))
	}
	if b.Len() == 0 {
		return "CLI"
	}
	return b.String()
}

func isLetterRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// recommendationTitle is the short header printed next to the vulnerability id.
// The tester can set it explicitly; otherwise the first sentence or line of the
// recommendation body stands in.
func recommendationTitle(f ReportFinding) string {
	if s := strings.TrimSpace(f.RecommendationHeader); s != "" {
		return s
	}
	body := strings.TrimSpace(f.Recommendation)
	if body == "" {
		return strings.TrimSpace(f.Title)
	}
	if i := strings.IndexAny(body, ".\n"); i > 0 {
		body = body[:i]
	}
	return truncateText(body, 90)
}

// ---------------------------------------------------------------------------
// document body navigation
// ---------------------------------------------------------------------------

var pStyleRe = regexp.MustCompile(`<w:pStyle w:val="([^"]*)"`)

// bodyChild is one top-level element of `<w:body>`.
type bodyChild struct {
	span
	Tag   string
	Style string
	Text  string
}

// bodyChildren lists the top-level elements of the document body with the style
// and text needed to find a section by its heading.
func bodyChildren(s string) []bodyChild {
	start := strings.Index(s, "<w:body>")
	if start < 0 {
		return nil
	}
	i := start + len("<w:body>")
	var out []bodyChild
	for i < len(s) {
		lt := strings.IndexByte(s[i:], '<')
		if lt < 0 {
			break
		}
		i += lt
		j := i + 1
		for j < len(s) && !isTagBoundary(s[j]) {
			j++
		}
		tag := s[i+1 : j]
		if tag == "/w:body" {
			break
		}
		end := elemEnd(s, i, tag)
		if end < 0 {
			break
		}
		frag := s[i:end]
		style := ""
		if m := pStyleRe.FindStringSubmatch(frag); m != nil {
			style = m[1]
		}
		out = append(out, bodyChild{span{i, end}, tag, style, elemText(frag)})
		i = end
	}
	return out
}

// sectPrCarriers returns a paragraph for every `<w:sectPr>` inside frag.
//
// Section properties are easy to lose and expensive to lose: they are stored on
// the last paragraph of the section they close, so deleting or rebuilding a
// stretch of the body silently merges that section into the next one. In this
// template that is what makes the vulnerability register through the wireless
// section print landscape, and what binds those pages to the header carrying
// the customer logo - so every removal re-emits the properties it swallowed.
func sectPrCarriers(frag string) string {
	var b strings.Builder
	for _, s := range childElems(frag, "w:sectPr") {
		b.WriteString(`<w:p><w:pPr>` + frag[s.Start:s.End] + `</w:pPr></w:p>`)
	}
	return b.String()
}

// deleteRange removes a body range, keeping any section properties it held.
func deleteRange(doc string, sp span) string {
	return doc[:sp.Start] + sectPrCarriers(doc[sp.Start:sp.End]) + doc[sp.End:]
}

// stripSectPr removes every `<w:sectPr>` from a fragment. Used on blocks that
// are about to be repeated, where a cloned section break would split the page
// setup once per copy.
func stripSectPr(frag string) string {
	for {
		s := childElems(frag, "w:sectPr")
		if len(s) == 0 {
			return frag
		}
		frag = frag[:s[0].Start] + frag[s[0].End:]
	}
}

func isHeading(style string) bool {
	return strings.HasPrefix(style, "Heading") || strings.HasPrefix(style, "StyleHeading")
}

func headingLevel(style string) int {
	switch {
	case strings.Contains(style, "Heading1"):
		return 1
	case strings.Contains(style, "Heading2"):
		return 2
	case strings.Contains(style, "Heading3"):
		return 3
	}
	return 0
}

// findHeading returns the index in children of the heading paragraph whose text
// matches want (trimmed, case-insensitive), or -1.
func findHeading(children []bodyChild, want string) int {
	return findHeadingFrom(children, want, 0)
}

// findHeadingFrom is findHeading starting the search at from, which matters for
// the handful of headings the template repeats (e.g. "Configuration File Review"
// appears under both Testing Methodologies and Test Cases).
func findHeadingFrom(children []bodyChild, want string, from int) int {
	for i := from; i < len(children); i++ {
		c := children[i]
		if c.Tag == "w:p" && isHeading(c.Style) && strings.EqualFold(strings.TrimSpace(c.Text), want) {
			return i
		}
	}
	return -1
}

// blockEnd returns the index of the first child after start that begins a new
// section at level <= level. Empty headings are treated as part of the current
// block, since the template leaves a few of them as spacers.
func blockEnd(children []bodyChild, start, level int) int {
	for i := start + 1; i < len(children); i++ {
		c := children[i]
		if c.Tag != "w:p" || !isHeading(c.Style) {
			continue
		}
		lv := headingLevel(c.Style)
		if lv > 0 && lv <= level && strings.TrimSpace(c.Text) != "" {
			return i
		}
	}
	return len(children)
}

// ---------------------------------------------------------------------------
// chapter 3 - assessment sections
// ---------------------------------------------------------------------------

var vulnHeadingRe = regexp.MustCompile(`\[[A-Z]+ Vulnerability \d+\]`)

// areaTemplate is one area's chapter 3 block split into the part that prints
// once (heading, intro, test-type table) and the part that repeats per finding
// (the vulnerability heading and its detail tables).
type areaTemplate struct {
	Prefix string
	Detail string
}

// extractAreaTemplates slices the chapter 3 region into per-area blocks. It
// returns the region's byte range so the caller can replace it wholesale.
func extractAreaTemplates(doc string) (map[string]areaTemplate, span, error) {
	children := bodyChildren(doc)
	first := findHeading(children, reportAreas[0].Heading)
	if first < 0 {
		return nil, span{}, fmt.Errorf("template is missing the %q section", reportAreas[0].Heading)
	}
	toolsIdx := findHeading(children, "Tools and Licenses")
	if toolsIdx < 0 {
		return nil, span{}, fmt.Errorf("template is missing the Tools and Licenses heading")
	}

	out := map[string]areaTemplate{}
	for _, area := range reportAreas {
		if area.Heading == "" {
			continue
		}
		idx := findHeading(children, area.Heading)
		if idx < 0 || idx >= toolsIdx {
			continue
		}
		end := blockEnd(children, idx, headingLevel(children[idx].Style))
		if end > toolsIdx {
			end = toolsIdx
		}
		blockStart := children[idx].Start
		blockEndOff := children[end-1].End

		// The detail unit starts at the "[XXX Vulnerability 1]" element and runs
		// to the end of the block.
		detailStart := blockEndOff
		for i := idx; i < end; i++ {
			if vulnHeadingRe.MatchString(children[i].Text) {
				detailStart = children[i].Start
				break
			}
		}
		// Section properties are stripped from the extracted blocks and re-emitted
		// once by renderAreaSections; leaving them in would clone a section break
		// into every repeated finding of the last area.
		out[area.Code] = areaTemplate{
			Prefix: stripSectPr(doc[blockStart:detailStart]),
			Detail: stripSectPr(doc[detailStart:blockEndOff]),
		}
	}
	if len(out) == 0 {
		return nil, span{}, fmt.Errorf("no assessment sections found in the template")
	}
	return out, span{children[first].Start, children[toolsIdx].Start}, nil
}

// normalizeReportAreas fills in the pieces an older or partial caller may leave
// out: areas derived from the legacy `sections` list, and the flat scope list
// the gofpdf fallback renderer still reads.
func normalizeReportAreas(config *ReportConfig) {
	if len(config.Areas) == 0 {
		for _, s := range config.Sections {
			if area, ok := areaByCode(s); ok {
				config.Areas = append(config.Areas, ReportArea{Code: area.Code})
			}
		}
	}
	if len(config.Scope) == 0 {
		for _, a := range config.Areas {
			area, ok := areaByCode(a.Code)
			if !ok {
				continue
			}
			line := area.Label
			if scope := strings.TrimSpace(a.Scope); scope != "" {
				line += ": " + scope
			}
			config.Scope = append(config.Scope, line)
		}
	}
}

// selectedAreaCodes returns the chosen area codes in template order, defaulting
// to the areas the findings themselves name when the wizard sent none.
func selectedAreaCodes(config ReportConfig) []string {
	chosen := map[string]bool{}
	for _, a := range config.Areas {
		if _, ok := areaByCode(a.Code); ok {
			chosen[strings.ToUpper(strings.TrimSpace(a.Code))] = true
		}
	}
	if len(chosen) == 0 {
		for _, s := range config.Sections {
			if area, ok := areaByCode(s); ok {
				chosen[area.Code] = true
			}
		}
	}
	if len(chosen) == 0 {
		for _, f := range config.Findings {
			if code, ok := areaCodeOf(f.Area); ok {
				chosen[code] = true
				continue
			}
			if code, ok := areaCodeOf(f.Category); ok {
				chosen[code] = true
			}
		}
	}
	if len(chosen) == 0 {
		chosen["WPT"] = true
	}
	var out []string
	for _, a := range reportAreas {
		if chosen[a.Code] {
			out = append(out, a.Code)
		}
	}
	return out
}

// renderAreaSections rebuilds the whole chapter 3 region so it contains exactly
// the selected areas, in template order, each carrying exactly its own findings.
func renderAreaSections(doc string, config ReportConfig, findings []numberedFinding, pocs *pocCollector, notes *reportNotes) (string, error) {
	templates, region, err := extractAreaTemplates(doc)
	if err != nil {
		return doc, err
	}

	byArea := map[string][]numberedFinding{}
	for _, f := range findings {
		byArea[f.Area.Code] = append(byArea[f.Area.Code], f)
	}

	var b strings.Builder
	for _, code := range selectedAreaCodes(config) {
		area, _ := areaByCode(code)
		tmpl, ok := templates[code]
		if !ok {
			// Areas the template has no block for (API Security Assessment)
			// borrow the web application block's finding layout under their own
			// heading, so they still print a real section.
			tmpl = synthesizeAreaTemplate(templates, area)
			if tmpl.Detail == "" {
				continue
			}
		}
		// The area block opens with its Test Type checklist. Its statuses are
		// rewritten from this area's findings before the placeholders are filled;
		// neither pass depends on the other, and doing it here is the one place
		// that knows which findings belong to which section.
		prefix := renderTestTypeTables(tmpl.Prefix, byArea[code], notes)
		b.WriteString(applyScalarPlaceholders(prefix, config))

		list := byArea[code]
		if len(list) == 0 {
			b.WriteString(noFindingsParagraph(area))
			continue
		}
		for _, f := range list {
			unit, err := renderFindingDetail(tmpl.Detail, f, config, pocs)
			if err != nil {
				return doc, err
			}
			b.WriteString(unit)
		}
	}

	// The chapter 3 region closes the landscape section that starts at the
	// vulnerability register, so its section properties have to survive the
	// rebuild - otherwise these pages fall back to the final portrait section
	// and pick up its header, losing the customer logo along the way.
	b.WriteString(sectPrCarriers(doc[region.Start:region.End]))

	return doc[:region.Start] + b.String() + doc[region.End:], nil
}

// synthesizeAreaTemplate builds a section for an area the template does not
// carry, by relabelling the web application block's heading and reusing its
// finding layout. The test-type table is dropped because it describes web tests.
func synthesizeAreaTemplate(templates map[string]areaTemplate, area areaDef) areaTemplate {
	src, ok := templates["WPT"]
	if !ok {
		return areaTemplate{}
	}
	// bodyChildren needs a body wrapper; keep slicing against the same string so
	// the offsets it returns stay valid.
	wrapped := "<w:body>" + src.Prefix + "</w:body>"
	children := bodyChildren(wrapped)
	if len(children) == 0 {
		return areaTemplate{}
	}
	heading := setParaText(wrapped[children[0].Start:children[0].End], area.Label)
	// The detail block needs no relabelling: renderFindingDetail rewrites the
	// vulnerability heading and the recommendation line from the finding itself.
	return areaTemplate{Prefix: heading, Detail: src.Detail}
}

func noFindingsParagraph(area areaDef) string {
	return `<w:p><w:r><w:t xml:space="preserve">No security issues were identified during the ` +
		xmlEscape(area.Label) + `.</w:t></w:r></w:p>`
}

// ---------------------------------------------------------------------------
// finding detail rendering
// ---------------------------------------------------------------------------

// detailLabels maps a template label cell to the finding field printed beneath
// it. The eight section layouts differ in which labels they use and in how many
// grid columns each spans, but they all follow "label row, then value row", so
// resolving by label keeps one routine correct for all of them.
var detailLabels = map[string]string{
	"description":          "description",
	"rating":               "rating",
	"cvss vector":          "cvss",
	"cvss vector string":   "cvss",
	"impact":               "impact",
	"attack vector":        "attackvector",
	"affected hosts":       "affected",
	"affected host":        "affected",
	"affected application": "affected",
	"affected device":      "affected",
	"affected domain":      "affected",
	"affected network":     "affected",
	"affected ssids":       "affected",
	"poc":                  "poc",
	"recommendation":       "recommendation",
}

var recLineRe = regexp.MustCompile(`\[Company Initials\]\s*_\s*REC\s*\[[Nn]umber\]\s*_\s*[A-Z]+\s*\d+\s*[^\[]*\[Recommendation Header\]`)

// rewriteParagraphs replaces the whole text of every paragraph the decide
// function claims. Used where a template line is assembled from several runs
// with different formatting and is being replaced end to end anyway.
func rewriteParagraphs(s string, decide func(text string) (string, bool)) string {
	var b strings.Builder
	prev := 0
	for _, p := range childElems(s, "w:p") {
		para := s[p.Start:p.End]
		text, ok := decide(elemText(para))
		if !ok {
			continue
		}
		b.WriteString(s[prev:p.Start])
		b.WriteString(setParaText(para, text))
		prev = p.End
	}
	if prev == 0 {
		return s
	}
	b.WriteString(s[prev:])
	return b.String()
}

// renderFindingDetail produces one finding's block: the vulnerability heading,
// the detail table(s) with every labelled cell filled, and the recommendation
// line rewritten to this finding's vulnerability id.
func renderFindingDetail(tmpl string, f numberedFinding, config ReportConfig, pocs *pocCollector) (string, error) {
	// The vulnerability heading and the recommendation naming line are rebuilt
	// whole: both are stitched together from several differently formatted runs
	// in the template, and both are entirely replaced by this finding's own
	// title and id, so there is nothing in them worth preserving run by run.
	s := rewriteParagraphs(tmpl, func(text string) (string, bool) {
		if vulnHeadingRe.MatchString(text) {
			return strings.TrimSpace(f.Title), true
		}
		if recLineRe.MatchString(text) {
			return f.VulnID + " – " + f.RecTitle, true
		}
		return "", false
	})

	values := map[string]string{
		"description":  strings.TrimSpace(f.Description),
		"rating":       severityDisplay(f.Severity),
		"cvss":         strings.TrimSpace(f.CVSSVector),
		"impact":       strings.TrimSpace(f.Impact),
		"attackvector": detailAttackVector(f.ReportFinding),
		"affected":     strings.TrimSpace(f.AffectedSystem),
		"poc":          pocText(f.ReportFinding),
	}
	if body := strings.TrimSpace(f.Recommendation); body != "" {
		values["recommendation"] = body
	}

	pocXML := pocs.drawingsFor(f.ReportFinding)

	// Fill every detail table in the block.
	var out strings.Builder
	prev := 0
	for _, t := range childElems(s, "w:tbl") {
		filled := fillDetailTable(s[t.Start:t.End], values, pocXML)
		out.WriteString(s[prev:t.Start])
		out.WriteString(filled)
		prev = t.End
	}
	out.WriteString(s[prev:])
	return out.String(), nil
}

// detailAttackVector prefers an explicit attack vector and otherwise reads the
// AV component out of the CVSS vector string so the cell is never left blank.
func detailAttackVector(f ReportFinding) string {
	if s := strings.TrimSpace(f.AttackVector); s != "" {
		return s
	}
	for _, part := range strings.Split(f.CVSSVector, "/") {
		if !strings.HasPrefix(part, "AV:") {
			continue
		}
		switch strings.TrimPrefix(part, "AV:") {
		case "N":
			return "Network"
		case "A":
			return "Adjacent Network"
		case "L":
			return "Local"
		case "P":
			return "Physical"
		}
	}
	return exposureOf(f)
}

// pocText is the written part of the proof of concept. Inline HTML images are
// dropped here because they are re-attached as real Word drawings.
func pocText(f ReportFinding) string {
	s := strings.TrimSpace(f.POC)
	if containsImage(s) {
		s = regexp.MustCompile(`(?is)<img[^>]*>`).ReplaceAllString(s, "")
		s = strings.TrimSpace(regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(s, " "))
	}
	return s
}

// fillDetailTable walks a detail table and writes each finding value into the
// row below its label.
func fillDetailTable(tbl string, values map[string]string, pocXML string) string {
	rows := tableRows(tbl)
	if len(rows) == 0 {
		return tbl
	}
	// Work back-to-front so earlier row offsets stay valid as we rewrite.
	type job struct {
		rowIdx, cellIdx int
		key             string
	}
	var jobs []job
	for ri := 0; ri < len(rows)-1; ri++ {
		cells := rowCells(tbl[rows[ri].Start:rows[ri].End])
		for ci, c := range cells {
			label := strings.ToLower(strings.TrimSpace(elemText(tbl[rows[ri].Start:rows[ri].End][c.Start:c.End])))
			key, ok := detailLabels[label]
			if !ok {
				continue
			}
			next := rowCells(tbl[rows[ri+1].Start:rows[ri+1].End])
			if len(next) == 0 {
				continue
			}
			target := ci
			if target >= len(next) {
				target = 0
			}
			jobs = append(jobs, job{ri + 1, target, key})
		}
	}

	sort.SliceStable(jobs, func(i, j int) bool {
		if jobs[i].rowIdx != jobs[j].rowIdx {
			return jobs[i].rowIdx > jobs[j].rowIdx
		}
		return jobs[i].cellIdx > jobs[j].cellIdx
	})

	for _, jb := range jobs {
		rowStart, rowEnd := rows[jb.rowIdx].Start, rows[jb.rowIdx].End
		row := tbl[rowStart:rowEnd]
		cells := rowCells(row)
		if jb.cellIdx >= len(cells) {
			continue
		}
		cell := row[cells[jb.cellIdx].Start:cells[jb.cellIdx].End]
		value := values[jb.key]

		switch jb.key {
		case "rating":
			if value == "" {
				continue
			}
			cell = setSeverityBadge(cell, value, severityHex(value))
		case "poc":
			cell = setPOCCell(cell, value, pocXML)
		case "recommendation":
			if value == "" {
				continue
			}
			// The recommendation cell holds two paragraphs: the
			// "<VulnID> - <Header>" naming line, which stays bold, and the
			// body elaborating on it, which does not. Bold has to be turned
			// off explicitly - the cell's <w:cnfStyle> bolds any run that
			// does not say otherwise.
			cell, _ = setFirstEmptyParaTextUnbolded(cell, value)
		default:
			if value == "" {
				continue
			}
			cell, _ = setFirstEmptyParaText(cell, value)
		}

		row = row[:cells[jb.cellIdx].Start] + cell + row[cells[jb.cellIdx].End:]
		tbl = tbl[:rowStart] + row + tbl[rowEnd:]
		rows = tableRows(tbl)
	}
	return tbl
}

// setSeverityBadge writes the criticality word into the cell, bold and in the
// colour for its severity. The cell background is left exactly as the template
// has it.
func setSeverityBadge(cell, label, textHex string) string {
	rPr := `<w:rPr><w:b/><w:color w:val="` + textHex + `"/></w:rPr>`
	for _, p := range childElems(cell, "w:p") {
		para := cell[p.Start:p.End]
		if strings.TrimSpace(elemText(para)) != "" {
			continue
		}
		gt := strings.Index(para, ">")
		if gt < 0 {
			continue
		}
		rebuilt := para[:gt+1] + paraPPr(para) +
			`<w:r>` + rPr + `<w:t xml:space="preserve">` + xmlEscape(label) + `</w:t></w:r></w:p>`
		return cell[:p.Start] + rebuilt + cell[p.End:]
	}
	return cell
}

// setPOCCell writes the proof-of-concept narrative and appends every attached
// screenshot as its own centred paragraph.
func setPOCCell(cell, text, drawings string) string {
	if text == "" && drawings == "" {
		return cell
	}
	if text != "" {
		cell, _ = setFirstEmptyParaText(cell, text)
	}
	if drawings == "" {
		return cell
	}
	end := strings.LastIndex(cell, "</w:tc>")
	if end < 0 {
		return cell
	}
	return cell[:end] + drawings + cell[end:]
}

// ---------------------------------------------------------------------------
// vulnerability register
// ---------------------------------------------------------------------------

// renderVulnerabilityRegister repeats the register's single template row once
// per finding, so the combined view lists exactly the reported vulnerabilities.
func renderVulnerabilityRegister(doc string, findings []numberedFinding) string {
	children := bodyChildren(doc)
	idx := findHeading(children, "Vulnerability Register")
	if idx < 0 {
		return doc
	}
	tblIdx := -1
	for i := idx + 1; i < len(children) && i < idx+6; i++ {
		if children[i].Tag == "w:tbl" {
			tblIdx = i
			break
		}
	}
	if tblIdx < 0 {
		return doc
	}
	tbl := doc[children[tblIdx].Start:children[tblIdx].End]
	rows := tableRows(tbl)
	if len(rows) < 2 {
		return doc
	}
	rowTmpl := tbl[rows[1].Start:rows[1].End]

	var b strings.Builder
	for _, f := range findings {
		row := rowTmpl
		cells := rowCells(row)
		values := []string{
			strings.TrimSpace(f.Title),
			f.Area.Exposure,
			severityDisplay(f.Severity),
			f.VulnID,
			f.RecTitle,
		}
		for ci := len(cells) - 1; ci >= 0; ci-- {
			if ci >= len(values) {
				continue
			}
			cell := row[cells[ci].Start:cells[ci].End]
			if ci == 2 {
				cell = setSeverityBadge(cell, values[ci], severityHex(f.Severity))
			} else {
				cell, _ = setFirstEmptyParaText(cell, values[ci])
			}
			row = row[:cells[ci].Start] + cell + row[cells[ci].End:]
		}
		b.WriteString(row)
	}

	newTbl := tbl[:rows[1].Start] + b.String() + tbl[rows[len(rows)-1].End:]
	return doc[:children[tblIdx].Start] + newTbl + doc[children[tblIdx].End:]
}

// ---------------------------------------------------------------------------
// scope table and naming convention
// ---------------------------------------------------------------------------

// renderScopeTable keeps only the activity rows for the selected areas and
// writes each area's scope detail into its Details cell, so the table describes
// this engagement rather than the template's full activity catalogue.
func renderScopeTable(doc string, config ReportConfig) string {
	children := bodyChildren(doc)
	idx := findHeading(children, "Scope")
	if idx < 0 {
		return doc
	}
	tblIdx := -1
	for i := idx + 1; i < len(children) && i < idx+6; i++ {
		if children[i].Tag == "w:tbl" {
			tblIdx = i
			break
		}
	}
	if tblIdx < 0 {
		return doc
	}

	details := map[string]string{}
	for _, a := range config.Areas {
		details[strings.ToUpper(strings.TrimSpace(a.Code))] = strings.TrimSpace(a.Scope)
	}
	keep := map[string]areaDef{}
	for _, code := range selectedAreaCodes(config) {
		if area, ok := areaByCode(code); ok {
			keep[strings.ToLower(area.ScopeRow)] = area
		}
	}

	tbl := doc[children[tblIdx].Start:children[tblIdx].End]
	rows := tableRows(tbl)
	var b strings.Builder
	for ri, r := range rows {
		row := tbl[r.Start:r.End]
		cells := rowCells(row)
		if ri == 0 {
			b.WriteString(scopeHeaderRow(row))
			continue
		}
		if len(cells) < 2 {
			b.WriteString(row)
			continue
		}
		label := strings.ToLower(strings.TrimSpace(elemText(row[cells[0].Start:cells[0].End])))
		area, ok := keep[label]
		if !ok {
			continue // activity not part of this engagement
		}
		if detail := details[area.Code]; detail != "" {
			cell := row[cells[1].Start:cells[1].End]
			cell, _ = setFirstEmptyParaText(cell, detail)
			row = row[:cells[1].Start] + cell + row[cells[1].End:]
		}
		b.WriteString(row)
	}

	newTbl := tbl[:rows[0].Start] + b.String() + tbl[rows[len(rows)-1].End:]
	return doc[:children[tblIdx].Start] + newTbl + doc[children[tblIdx].End:]
}

// scopeHeaderRow fixes the scope table's header, which ships with both column
// names crammed into the first cell ("Activity Details") and the second column
// left blank. The activity rows below merge their two remaining grid columns, so
// the Details header is merged to match and sits over the text it labels.
func scopeHeaderRow(row string) string {
	cells := rowCells(row)
	if len(cells) < 2 {
		return row
	}
	if strings.TrimSpace(elemText(row[cells[0].Start:cells[0].End])) != "Activity Details" {
		return row // already fixed, or a template we do not recognise
	}

	details := row[cells[1].Start:cells[1].End]
	details = setCellLines(details, []string{"Details"})
	if len(cells) > 2 {
		details = cellGridSpan(details, len(cells)-1)
	}

	// Rebuild back to front so the earlier cell offsets stay valid.
	if len(cells) > 2 {
		row = row[:cells[1].End] + row[cells[len(cells)-1].End:]
	}
	row = row[:cells[1].Start] + details + row[cells[1].End:]
	activity := setCellLines(row[cells[0].Start:cells[0].End], []string{"Activity"})
	return row[:cells[0].Start] + activity + row[cells[0].End:]
}

// renderNamingConvention drops the marker lines for areas that were not tested,
// so the convention lists only the codes that actually appear in the report.
func renderNamingConvention(doc string, config ReportConfig) string {
	children := bodyChildren(doc)
	idx := findHeading(children, "Recommendations Naming Convention")
	if idx < 0 {
		return doc
	}
	end := blockEnd(children, idx, headingLevel(children[idx].Style))

	keep := map[string]bool{}
	for _, code := range selectedAreaCodes(config) {
		keep[code] = true
	}

	var drop []span
	for i := idx + 1; i < end; i++ {
		c := children[i]
		if c.Tag != "w:p" {
			continue
		}
		text := strings.TrimSpace(c.Text)
		for _, area := range reportAreas {
			if keep[area.Code] {
				continue
			}
			// Lines read "WPT – Web Application Penetration Testing".
			if strings.HasPrefix(text, area.Code+" ") && strings.Contains(text, area.Label) {
				drop = append(drop, c.span)
				break
			}
		}
	}
	for i := len(drop) - 1; i >= 0; i-- {
		doc = deleteRange(doc, drop[i])
	}
	return doc
}

// ---------------------------------------------------------------------------
// cover page
// ---------------------------------------------------------------------------

// coverRuleBorder is the thin white rule the cover's version table draws under
// each of its rows, on the dark cover background.
const coverRuleBorder = `<w:bottom w:val="single" w:sz="4" w:space="0" w:color="FFFFFF" w:themeColor="background1"/>`

// renderCoverTable evens out the rules under the cover's Version / Date /
// Authors / Approver table. The template declares a bottom border on only some
// cells of the data rows, so the rules stop partway across the table; every cell
// in those rows gets the same rule here so the lines run its full width.
func renderCoverTable(doc string) string {
	children := bodyChildren(doc)
	tblIdx := -1
	for i, c := range children {
		if c.Tag == "w:tbl" {
			tblIdx = i
			break
		}
	}
	if tblIdx < 0 {
		return doc
	}
	tbl := doc[children[tblIdx].Start:children[tblIdx].End]

	rows := tableRows(tbl)
	header := -1
	for ri, r := range rows {
		text := elemText(tbl[r.Start:r.End])
		if strings.Contains(text, "Version") && strings.Contains(text, "Approver") {
			header = ri
			break
		}
	}
	if header < 0 {
		return doc
	}

	// Data rows only: the header keeps its own heavier rule, and the trailing
	// spacer row must stay borderless or the table gains a stray extra line.
	for ri := len(rows) - 1; ri > header; ri-- {
		row := tbl[rows[ri].Start:rows[ri].End]
		cells := rowCells(row)
		if len(cells) < len(rowCells(tbl[rows[header].Start:rows[header].End])) {
			continue
		}
		if strings.TrimSpace(elemText(row)) == "" {
			continue
		}
		for ci := len(cells) - 1; ci >= 0; ci-- {
			cell := setCellBottomBorder(row[cells[ci].Start:cells[ci].End], coverRuleBorder)
			row = row[:cells[ci].Start] + cell + row[cells[ci].End:]
		}
		tbl = tbl[:rows[ri].Start] + row + tbl[rows[ri].End:]
	}

	return doc[:children[tblIdx].Start] + tbl + doc[children[tblIdx].End:]
}

// ---------------------------------------------------------------------------
// footer company-name reference
// ---------------------------------------------------------------------------

var clientCompanyBookmarkRe = regexp.MustCompile(`<w:bookmarkStart w:id="(\d+)" w:name="ClientCompanyName"\s*/>`)

// fixClientCompanyBookmark moves the ClientCompanyName bookmark so it actually
// wraps the company name.
//
// The page footers print "VAPT Report – " followed by a REF field pointing at
// this bookmark, but the template anchors the bookmark just after the company
// name, over a single space. The cached field result reads correctly until Word
// refreshes fields, at which point the footer's company name disappears. Moving
// the bookmark onto the name makes the field resolve to the name for good.
func fixClientCompanyBookmark(doc string) string {
	m := clientCompanyBookmarkRe.FindStringSubmatchIndex(doc)
	if m == nil {
		return doc
	}
	id := doc[m[2]:m[3]]
	endTag := `<w:bookmarkEnd w:id="` + id + `"/>`
	endAt := strings.Index(doc, endTag)
	if endAt < 0 {
		return doc
	}

	// Drop the misplaced pair, back to front.
	if endAt > m[1] {
		doc = doc[:endAt] + doc[endAt+len(endTag):]
		doc = doc[:m[0]] + doc[m[1]:]
	} else {
		doc = doc[:m[0]] + doc[m[1]:]
		doc = doc[:endAt] + doc[endAt+len(endTag):]
	}

	// Re-anchor it around the run that holds the company name. Placeholder
	// healing has already pulled that name into a single run.
	for _, r := range childElems(doc, "w:r") {
		if strings.TrimSpace(elemText(doc[r.Start:r.End])) != "[Company Name]" {
			continue
		}
		start := `<w:bookmarkStart w:id="` + id + `" w:name="ClientCompanyName"/>`
		return doc[:r.Start] + start + doc[r.Start:r.End] + endTag + doc[r.End:]
	}
	return doc
}

// ---------------------------------------------------------------------------
// footers
// ---------------------------------------------------------------------------

var staleReportLineRe = regexp.MustCompile(`(?i)assessment report\s*[-–]\s*\S`)

// renderFooterText fixes the footer lines the template left pointing at whoever
// it was last used for. Three of the seven footer parts carry a previous
// engagement's reference number, and two of those also name that client, none of
// them behind a placeholder - so a delivered report would print another
// customer's details on the first page of the landscape section and on every
// page of the appendix.
func renderFooterText(footer string, config ReportConfig) string {
	company := strings.TrimSpace(config.CompanyName)
	if company != "" {
		// This line is a paragraph of its own in the footers that carry it.
		footer = rewriteParagraphs(footer, func(text string) (string, bool) {
			if staleReportLineRe.MatchString(strings.TrimSpace(text)) {
				return "VAPT Report – " + company, true
			}
			return "", false
		})
	}

	ref := strings.TrimSpace(config.RefNumber)
	if ref == "" {
		return footer
	}
	// The reference shares a paragraph with "All rights reserved" in one footer,
	// so it is rewritten run by run rather than a paragraph at a time. Working
	// back to front keeps the earlier element offsets valid.
	els := textElems(footer)
	for i := len(els) - 1; i >= 0; i-- {
		text := elemText(footer[els[i].Start:els[i].End])
		cut := strings.Index(text, "Ref:")
		if cut < 0 {
			continue
		}
		rest := text[cut+len("Ref:"):]
		if strings.TrimSpace(rest) != "" {
			footer = replaceNthText(footer, i, text[:cut]+"Ref: "+ref)
			continue
		}
		if i+1 >= len(els) {
			continue
		}
		// The number lives in the next run; supply the separating space unless
		// the marker run already ends with one.
		if rest == "" {
			footer = replaceNthText(footer, i+1, " "+ref)
		} else {
			footer = replaceNthText(footer, i+1, ref)
		}
	}
	return footer
}

// ---------------------------------------------------------------------------
// table of contents
// ---------------------------------------------------------------------------

var tocNumberRe = regexp.MustCompile(`^\d+(?:\.\d+)*`)

// stripFieldRuns removes the complete run sequence of the first field in frag,
// from the run holding fldChar "begin" to the run holding fldChar "end". Used to
// drop the PAGEREF from a contents entry that has no page to point at yet.
func stripFieldRuns(frag string) string {
	begin := strings.Index(frag, `<w:fldChar w:fldCharType="begin"/>`)
	end := strings.Index(frag, `<w:fldChar w:fldCharType="end"/>`)
	if begin < 0 || end < begin {
		return frag
	}
	start := lastElemStart(frag[:begin], "w:r")
	stop := strings.Index(frag[end:], "</w:r>")
	if start < 0 || stop < 0 {
		return frag
	}
	return frag[:start] + frag[end+stop+len("</w:r>"):]
}

// tocEntryTitle strips the leading section number from a contents entry.
func tocEntryTitle(text string) string {
	return strings.TrimSpace(tocNumberRe.ReplaceAllString(strings.TrimSpace(text), ""))
}

// renderTableOfContents rewrites the chapter 3 entries of the cached table of
// contents so they name exactly the sections the report contains. settings.xml
// asks Word to refresh the field on open, which restores the page numbers; this
// pass makes sure the contents never advertise a section that was left out even
// before that refresh happens.
func renderTableOfContents(doc string, config ReportConfig) string {
	children := bodyChildren(doc)

	// Every heading the template can print for an assessment area, so the
	// chapter 3 stretch of the contents can be recognised.
	areaTitles := map[string]bool{}
	for _, a := range reportAreas {
		if a.Heading != "" {
			areaTitles[strings.ToLower(a.Heading)] = true
		}
		areaTitles[strings.ToLower(a.Label)] = true
	}

	// A contents entry is three text runs: the section number, the title, and
	// the cached page number, so the title is read positionally rather than from
	// the paragraph's whole text.
	entryTitle := func(c bodyChild) string {
		return strings.TrimSpace(nthText(doc[c.Start:c.End], 1))
	}

	var entries []int
	for i, c := range children {
		if c.Tag != "w:p" || c.Style != "TOC2" {
			continue
		}
		if areaTitles[strings.ToLower(entryTitle(c))] {
			entries = append(entries, i)
		}
	}
	if len(entries) == 0 {
		return doc
	}
	// The area entries must be one contiguous run for a clean replacement.
	first, last := entries[0], entries[len(entries)-1]
	if last-first+1 != len(entries) {
		return doc
	}

	byTitle := map[string]string{}
	for _, i := range entries {
		byTitle[strings.ToLower(entryTitle(children[i]))] = doc[children[i].Start:children[i].End]
	}
	proto := doc[children[first].Start:children[first].End]

	// Chapter 3 opens with 3.1 Naming Convention and 3.2 Vulnerability Register.
	num := 2
	var b strings.Builder
	for _, code := range selectedAreaCodes(config) {
		area, ok := areaByCode(code)
		if !ok {
			continue
		}
		title := area.Heading
		if title == "" {
			title = area.Label
		}
		num++
		entry, reused := byTitle[strings.ToLower(title)]
		if !reused {
			// An area the template has no section for gets a fresh entry; its
			// page number arrives when the field is refreshed.
			entry = stripFieldRuns(proto)
		}
		entry = replaceNthText(entry, 0, fmt.Sprintf("3.%d", num))
		entry = replaceNthText(entry, 1, strings.TrimSpace(title))
		b.WriteString(entry)
	}

	return doc[:children[first].Start] + b.String() + doc[children[last].End:]
}

// ---------------------------------------------------------------------------
// methodology and test case subsections
// ---------------------------------------------------------------------------

// methodologySubsection ties one of the Heading3 blocks under "Testing
// Methodologies" and "Test Cases" to the assessment areas it describes. A block
// whose areas were all left out of the engagement is removed, so the report
// never explains a methodology it did not apply.
var methodologySubsections = []struct {
	Heading string
	Areas   []string
}{
	{"Internal/External Network VAPT Methodology", []string{"IPT", "EPT", "IPTC"}},
	{"Web Application Testing Methodology", []string{"WPT", "ASA"}},
	{"Configuration File Review", []string{"CFG"}},
	{"Wireless Network Assessment Methodology", []string{"WNA"}},
	{"Network Testing", []string{"IPT", "EPT", "IPTC"}},
	{"Web Application Testing", []string{"WPT", "ASA"}},
	{"Wireless Network Assessment", []string{"WNA"}},
}

// renderMethodologySections drops the methodology and test-case subsections for
// activities this engagement did not include.
func renderMethodologySections(doc string, config ReportConfig) string {
	selected := map[string]bool{}
	for _, code := range selectedAreaCodes(config) {
		selected[code] = true
	}

	children := bodyChildren(doc)
	start := findHeading(children, "Testing Methodologies")
	if start < 0 {
		return doc
	}
	end := findHeadingFrom(children, "Findings and Recommendations", start)
	if end < 0 {
		end = len(children)
	}

	wanted := map[string]bool{}
	for _, sub := range methodologySubsections {
		keep := false
		for _, code := range sub.Areas {
			if selected[code] {
				keep = true
				break
			}
		}
		wanted[strings.ToLower(sub.Heading)] = keep
	}

	var drop []span
	for i := start + 1; i < end; i++ {
		c := children[i]
		if c.Tag != "w:p" || headingLevel(c.Style) != 3 {
			continue
		}
		keep, known := wanted[strings.ToLower(strings.TrimSpace(c.Text))]
		if !known || keep {
			continue
		}
		stop := blockEnd(children, i, 3)
		if stop > end {
			stop = end
		}
		drop = append(drop, span{c.Start, children[stop-1].End})
	}
	for i := len(drop) - 1; i >= 0; i-- {
		doc = deleteRange(doc, drop[i])
	}

	return rewriteNamingExample(doc, config)
}

// rewriteNamingExample restates the worked example under the naming convention
// using an area that was actually tested - the template's own example cites
// external penetration testing, which is usually not in scope.
func rewriteNamingExample(doc string, config ReportConfig) string {
	codes := selectedAreaCodes(config)
	if len(codes) == 0 {
		return doc
	}
	area, ok := areaByCode(codes[0])
	if !ok {
		return doc
	}
	// The company placeholders are left in place; the scalar pass fills them
	// along with every other mention.
	example := "Example: The vulnerability labelled “[Company Initials]_REC3_" + area.Code + "3” " +
		"refers to the 3rd recommendation for [Company Name], addressing the 3rd finding from the " +
		strings.ToLower(area.Label) + "."

	return rewriteParagraphs(doc, func(text string) (string, bool) {
		if strings.HasPrefix(strings.TrimSpace(text), "Example: The vulnerability labelled") {
			return example, true
		}
		return "", false
	})
}

// ---------------------------------------------------------------------------
// tools and appendix
// ---------------------------------------------------------------------------

// renderToolsTable replaces the template's example tool list with the tools the
// tester actually used, split evenly over the table's two columns.
func renderToolsTable(doc string, config ReportConfig) string {
	tools := make([]string, 0, len(config.Tools))
	for _, t := range config.Tools {
		if t = strings.TrimSpace(t); t != "" {
			tools = append(tools, t)
		}
	}
	if len(tools) == 0 {
		return doc
	}
	children := bodyChildren(doc)
	idx := findHeading(children, "Tools and Licenses")
	if idx < 0 {
		return doc
	}
	tblIdx := -1
	for i := idx + 1; i < len(children) && i < idx+5; i++ {
		if children[i].Tag == "w:tbl" {
			tblIdx = i
			break
		}
	}
	if tblIdx < 0 {
		return doc
	}
	tbl := doc[children[tblIdx].Start:children[tblIdx].End]
	rows := tableRows(tbl)
	if len(rows) == 0 {
		return doc
	}
	row := tbl[rows[0].Start:rows[0].End]
	cells := rowCells(row)
	if len(cells) == 0 {
		return doc
	}
	half := (len(tools) + len(cells) - 1) / len(cells)
	for ci := len(cells) - 1; ci >= 0; ci-- {
		lo := ci * half
		hi := lo + half
		if lo > len(tools) {
			lo = len(tools)
		}
		if hi > len(tools) {
			hi = len(tools)
		}
		cell := row[cells[ci].Start:cells[ci].End]
		cell = setCellLines(cell, tools[lo:hi])
		row = row[:cells[ci].Start] + cell + row[cells[ci].End:]
	}
	newTbl := tbl[:rows[0].Start] + row + tbl[rows[0].End:]
	return doc[:children[tblIdx].Start] + newTbl + doc[children[tblIdx].End:]
}

// setCellLines rewrites a cell so it holds exactly one paragraph per line,
// cloning the first paragraph's formatting.
func setCellLines(cell string, lines []string) string {
	paras := childElems(cell, "w:p")
	if len(paras) == 0 {
		return cell
	}
	proto := cell[paras[0].Start:paras[0].End]
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(setParaText(proto, line))
	}
	if b.Len() == 0 {
		b.WriteString(setParaText(proto, ""))
	}
	return cell[:paras[0].Start] + b.String() + cell[paras[len(paras)-1].End:]
}

// renderAppendix fills the two test-account tables, and removes the appendix
// entirely when the tester supplied no accounts - an empty appendix of blank
// credential rows is worse than no appendix at all.
func renderAppendix(doc string, config ReportConfig) string {
	children := bodyChildren(doc)
	idx := findHeading(children, "Appendix")
	if idx < 0 {
		return doc
	}
	end := len(children)
	for i := idx + 1; i < len(children); i++ {
		if children[i].Tag == "w:sectPr" {
			end = i
			break
		}
	}

	existing := nonEmptyAccounts(config.TestAccountsExisting)
	created := nonEmptyAccounts(config.TestAccountsCreated)

	if len(existing) == 0 && len(created) == 0 {
		return deleteRange(doc, span{children[idx].Start, children[end-1].End})
	}

	// The appendix is [heading][intro 1][table 1][spacer][intro 2][table 2]...
	var tables []int
	for i := idx + 1; i < end; i++ {
		if children[i].Tag == "w:tbl" {
			tables = append(tables, i)
		}
	}
	if len(tables) < 2 {
		return doc
	}

	type edit struct {
		span
		body string // "" removes the range
	}
	var edits []edit

	// Second table (accounts created for the test) and its intro paragraph.
	if len(created) == 0 {
		edits = append(edits, edit{span{children[tables[1]-1].Start, children[tables[1]].End}, ""})
	} else {
		edits = append(edits, edit{children[tables[1]].span,
			fillAccountTable(doc[children[tables[1]].Start:children[tables[1]].End], created, true)})
	}

	// First table (pre-existing accounts) and its intro paragraph.
	if len(existing) == 0 {
		edits = append(edits, edit{span{children[tables[0]-1].Start, children[tables[0]].End}, ""})
	} else {
		edits = append(edits, edit{children[tables[0]].span,
			fillAccountTable(doc[children[tables[0]].Start:children[tables[0]].End], existing, false)})
	}

	for _, e := range edits {
		doc = doc[:e.Start] + e.body + sectPrCarriers(doc[e.Start:e.End]) + doc[e.End:]
	}
	return doc
}

func nonEmptyAccounts(accs []TestAccount) []TestAccount {
	var out []TestAccount
	for _, a := range accs {
		if strings.TrimSpace(a.Account) != "" || strings.TrimSpace(a.Credentials) != "" {
			out = append(out, a)
		}
	}
	return out
}

// fillAccountTable repeats the table's account row once per supplied account.
// hasHeader marks the layout whose first row is a header that must survive.
func fillAccountTable(tbl string, accounts []TestAccount, hasHeader bool) string {
	rows := tableRows(tbl)
	if len(rows) == 0 {
		return tbl
	}
	protoIdx := 0
	if hasHeader && len(rows) > 1 {
		protoIdx = 1
	}
	proto := tbl[rows[protoIdx].Start:rows[protoIdx].End]

	var b strings.Builder
	for _, acc := range accounts {
		row := proto
		cells := rowCells(row)
		values := []string{strings.TrimSpace(acc.Account), strings.TrimSpace(acc.Credentials)}
		for ci := len(cells) - 1; ci >= 0; ci-- {
			if ci >= len(values) {
				continue
			}
			cell := setCellLines(row[cells[ci].Start:cells[ci].End], []string{values[ci]})
			row = row[:cells[ci].Start] + cell + row[cells[ci].End:]
		}
		b.WriteString(row)
	}
	return tbl[:rows[protoIdx].Start] + b.String() + tbl[rows[len(rows)-1].End:]
}

// ---------------------------------------------------------------------------
// scalar placeholders
// ---------------------------------------------------------------------------

// scalarPlaceholders is the wizard-supplied text that appears verbatim wherever
// the template names it. Everything here is asked once in the wizard, so no
// value is collected twice.
func scalarPlaceholders(config ReportConfig) [][2]string {
	period := strings.TrimSpace(config.AssessmentStart)
	if end := strings.TrimSpace(config.AssessmentEnd); end != "" {
		if period != "" {
			period += " to " + end
		} else {
			period = end
		}
	}
	return [][2]string{
		{"[Company Initials]", initialsOrDash(config)},
		{"[Company Name]", config.CompanyName},
		{"[Reference Number]", config.RefNumber},
		{"[Assessment Start – Assessment End]", period},
		{"[Assessment Start - Assessment End]", period},
		{"[Tester Name]", config.TesterName},
		{"[Approver]", config.ApproverName},
		{"[Role]", config.ApproverTitle},
		{"[Date]", config.ReportDate},
	}
}

// applyScalarPlaceholders substitutes the engagement-wide placeholders in an XML
// fragment. Values are escaped because they are spliced into character data.
func applyScalarPlaceholders(s string, config ReportConfig) string {
	for _, kv := range scalarPlaceholders(config) {
		if !strings.Contains(s, kv[0]) {
			continue
		}
		s = strings.ReplaceAll(s, kv[0], xmlEscape(kv[1]))
	}
	return s
}

// applySummaryPlaceholders fills the executive-summary placeholders that are
// derived from the findings rather than typed by the tester: the issue count and
// the named example vulnerabilities.
func applySummaryPlaceholders(s string, findings []numberedFinding) string {
	sorted := make([]numberedFinding, len(findings))
	copy(sorted, findings)
	sort.SliceStable(sorted, func(i, j int) bool {
		return severityRank(sorted[i].Severity) < severityRank(sorted[j].Severity)
	})
	title := func(i int) string {
		if i < len(sorted) {
			return strings.TrimSpace(sorted[i].Title)
		}
		return ""
	}
	joinTitles := func(idx ...int) string {
		var parts []string
		for _, i := range idx {
			if t := title(i); t != "" {
				parts = append(parts, t)
			}
		}
		switch len(parts) {
		case 0:
			return "the issues listed below"
		case 1:
			return parts[0]
		default:
			return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
		}
	}

	repl := [][2]string{
		{"[total number of issues]", strconv.Itoa(len(findings)) + " issues"},
		{"[vulnerability 1, vulnerability 2]", joinTitles(0, 1)},
		{"[vulnerability 3 and vulnerability 4]", joinTitles(2, 3)},
		{"[vulnerability 5]", joinTitles(4)},
	}
	for _, kv := range repl {
		s = strings.ReplaceAll(s, kv[0], xmlEscape(kv[1]))
	}
	return s
}

// ---------------------------------------------------------------------------
// charts
// ---------------------------------------------------------------------------

var chartDimsRe = regexp.MustCompile(`(?s)<cx:strDim type="cat">.*?</cx:strDim><cx:numDim type="val">.*?</cx:numDim>`)

// renderChartData rewrites a chartEx part's cached category/value pairs. Word
// renders these charts from the cache (autoUpdate is off), so replacing the
// cache is what makes the bars match the report.
func renderChartData(chartXML string, cats []string, vals []int) string {
	if len(cats) == 0 {
		return chartXML
	}
	n := len(cats)
	var strPts, numPts strings.Builder
	for i, c := range cats {
		fmt.Fprintf(&strPts, `<cx:pt idx="%d">%s</cx:pt>`, i, xmlEscape(c))
		v := 0
		if i < len(vals) {
			v = vals[i]
		}
		fmt.Fprintf(&numPts, `<cx:pt idx="%d">%d</cx:pt>`, i, v)
	}
	block := fmt.Sprintf(
		`<cx:strDim type="cat"><cx:f>Sheet1!$A$2:$A$%d</cx:f><cx:lvl ptCount="%d">%s</cx:lvl></cx:strDim>`+
			`<cx:numDim type="val"><cx:f>Sheet1!$B$2:$B$%d</cx:f><cx:lvl ptCount="%d" formatCode="General">%s</cx:lvl></cx:numDim>`,
		n+1, n, strPts.String(), n+1, n, numPts.String())

	replaced := false
	return chartDimsRe.ReplaceAllStringFunc(chartXML, func(m string) string {
		if replaced {
			return m
		}
		replaced = true
		return block
	})
}

// severityChartData returns the five canonical severity buckets in the order the
// template's per-bar colours are defined, so Critical stays red and Low stays
// green however many findings land in each.
func severityChartData(findings []numberedFinding) ([]string, []int) {
	order := []string{"Critical", "High", "Medium", "Low", "Informational"}
	counts := map[string]int{}
	for _, f := range findings {
		counts[severityDisplay(f.Severity)]++
	}
	vals := make([]int, len(order))
	for i, k := range order {
		vals[i] = counts[k]
	}
	return order, vals
}

// areaChartData returns one bar per selected assessment area, carrying that
// area's real finding count.
func areaChartData(config ReportConfig, findings []numberedFinding) ([]string, []int) {
	counts := map[string]int{}
	for _, f := range findings {
		counts[f.Area.Code]++
	}
	var cats []string
	var vals []int
	for _, code := range selectedAreaCodes(config) {
		area, ok := areaByCode(code)
		if !ok {
			continue
		}
		cats = append(cats, area.ChartLabel)
		vals = append(vals, counts[code])
	}
	return cats, vals
}

// ---------------------------------------------------------------------------
// PoC screenshots
// ---------------------------------------------------------------------------

// pocCollector turns attached evidence into Word drawings and remembers the
// media parts and relationships the package must gain.
type pocCollector struct {
	parts []pocPart
	seq   int
}

func (p *pocCollector) drawingsFor(f ReportFinding) string {
	var b strings.Builder
	for _, img := range f.POCImages {
		pngBytes, w, h, err := preparePOCImage(img.Data)
		if err != nil {
			log.Printf("docx: skip PoC image %s (%v)", img.Filename, err)
			continue
		}
		p.seq++
		partName := fmt.Sprintf("word/media/poc_%d.png", p.seq)
		relID := fmt.Sprintf("rIdPoc%d", p.seq)
		p.parts = append(p.parts, pocPart{Name: partName, RelID: relID, Data: pngBytes})
		b.WriteString(`<w:p><w:pPr><w:jc w:val="center"/></w:pPr>`)
		b.WriteString(pocDrawingXML(relID, 3000+p.seq, w, h))
		b.WriteString(`</w:p>`)
	}
	return b.String()
}
