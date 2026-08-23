package main

// Filling the closure deck's slides.
//
// The template carries bracketed placeholders in place of every engagement
// detail - [Company Name], [Issue], [Rating] and so on - which is what
// tools/mkclosuretemplate leaves behind. Everything here is either replacing one
// of those, removing a table row that has no finding to hold, or pointing a
// picture frame at a screenshot.

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"regexp"
	"strings"
)

var (
	aParaRe    = regexp.MustCompile(`(?s)<a:p>.*?</a:p>`)
	aRunRe     = regexp.MustCompile(`(?s)<a:r>.*?</a:r>`)
	aTextRe    = regexp.MustCompile(`(?s)<a:t>(.*?)</a:t>`)
	aRowRe     = regexp.MustCompile(`(?s)<a:tr .*?</a:tr>`)
	aCellRe    = regexp.MustCompile(`(?s)<a:tc>.*?</a:tc>`)
	blipRe     = regexp.MustCompile(`<a:blip r:embed="[^"]*"`)
	pngExtRe   = regexp.MustCompile(`(?i)\.(png|jpe?g|gif)$`)
	imageRelTy = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"
)

// ---------------------------------------------------------------------------
// placeholder text
// ---------------------------------------------------------------------------

// setPlaceholders replaces whole paragraphs whose text is one of the given
// placeholder tokens.
//
// Matching is on the paragraph's full text rather than run by run, because
// PowerPoint splits a phrase across runs freely - "[Company Name]" is often
// three of them - and a per-run search finds none of it.
func setPlaceholders(slide string, values map[string]string) string {
	return aParaRe.ReplaceAllStringFunc(slide, func(para string) string {
		text := strings.TrimSpace(aParaText(para))
		if text == "" {
			return para
		}
		if v, ok := values[text]; ok {
			return setAParaText(para, v)
		}
		// A placeholder can also sit inside a longer line, as the footer's
		// "[Company Name] - VAPT - Closing Meeting" does.
		replaced := text
		for token, v := range values {
			replaced = strings.ReplaceAll(replaced, token, v)
		}
		if replaced != text {
			return setAParaText(para, replaced)
		}
		return para
	})
}

func aParaText(para string) string {
	var b strings.Builder
	for _, m := range aTextRe.FindAllStringSubmatch(para, -1) {
		b.WriteString(m[1])
	}
	return b.String()
}

// setAParaText makes a paragraph read exactly text, keeping its first run so the
// styling survives and dropping the rest. Rebuilding a paragraph from scratch
// means reconstructing properties PowerPoint is unforgiving about.
func setAParaText(para, text string) string {
	runs := aRunRe.FindAllStringIndex(para, -1)
	if len(runs) == 0 {
		return para
	}
	first := para[runs[0][0]:runs[0][1]]
	if !aTextRe.MatchString(first) {
		done := false
		return aTextRe.ReplaceAllStringFunc(para, func(m string) string {
			if done {
				return "<a:t></a:t>"
			}
			done = true
			return "<a:t>" + xmlEscape(text) + "</a:t>"
		})
	}
	done := false
	retexted := aTextRe.ReplaceAllStringFunc(first, func(m string) string {
		if done {
			return ""
		}
		done = true
		return "<a:t>" + xmlEscape(text) + "</a:t>"
	})
	out := para
	for i := len(runs) - 1; i >= 1; i-- {
		out = out[:runs[i][0]] + out[runs[i][1]:]
	}
	return out[:runs[0][0]] + retexted + out[runs[0][1]:]
}

// ---------------------------------------------------------------------------
// the slides that appear once
// ---------------------------------------------------------------------------

// fillFixedSlides fills the title, the two executive-summary slides and the
// footers, which appear once each.
func (d *closureDeck) fillFixedSlides(config ReportConfig, findings []numberedFinding) {
	company := strings.TrimSpace(config.CompanyName)
	if company == "" {
		company = "Client"
	}
	values := map[string]string{
		"[Company Name]":  company,
		"[Date]":          strings.TrimSpace(config.ReportDate),
		"[Scope]":         scopeSummary(config),
		"[Affected Host]": scopeTargets(config),
		"[Finding]":       "",
		"[Target]":        "",
	}
	if values["[Date]"] == "" {
		delete(values, "[Date]")
	}

	// The headline findings on the second summary slide are the most severe
	// ones, which is what that slide is for.
	headlines := headlineFindings(findings)
	idx := 0

	for _, p := range d.parts {
		if !strings.HasPrefix(p.Name, "ppt/slides/slide") {
			continue
		}
		body := string(p.Data)
		// Headline findings are consumed in order across the summary slide.
		body = aParaRe.ReplaceAllStringFunc(body, func(para string) string {
			if strings.TrimSpace(aParaText(para)) != "[Finding]" {
				return para
			}
			text := ""
			if idx < len(headlines) {
				text = headlines[idx]
			}
			idx++
			return setAParaText(para, text)
		})
		p.Data = []byte(setPlaceholders(body, values))
	}
}

// scopeSummary describes what was in scope, one line, from the areas the
// engagement selected.
func scopeSummary(config ReportConfig) string {
	var names []string
	for _, code := range selectedAreaCodes(config) {
		if area, ok := areaByCode(code); ok {
			names = append(names, area.Label)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return strings.Join(names, ", ")
}

// scopeTargets lists what was actually tested - the scope text entered per area
// - for the summary slide's target column. The template's cells there came from
// the source deck's IP ranges, so leaving them unfilled ships a placeholder.
func scopeTargets(config ReportConfig) string {
	var out []string
	for _, a := range config.Areas {
		if s := strings.TrimSpace(a.Scope); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, ", ")
}

// headlineFindings are the most severe findings, most severe first, capped at
// what the summary slide has room for.
func headlineFindings(findings []numberedFinding) []string {
	sorted := make([]numberedFinding, len(findings))
	copy(sorted, findings)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && severityRank(sorted[j].Severity) < severityRank(sorted[j-1].Severity); j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	var out []string
	for _, f := range sorted {
		out = append(out, strings.TrimSpace(f.Title))
	}
	return out
}

// ---------------------------------------------------------------------------
// the issues table
// ---------------------------------------------------------------------------

// chunkFindings splits the findings into slide-sized groups.
//
// The split follows area first and severity second, then fills up to size, so a
// slide is titled "IPT Issues - Critical Level" rather than "IPT/WPT Issues -
// Critical/Medium/High Level". Grouping purely by count produces titles that
// name everything and say nothing, and wrap onto two lines doing it.
func chunkFindings(findings []numberedFinding, size int) [][]numberedFinding {
	type key struct{ area, severity string }
	var order []key
	buckets := map[key][]numberedFinding{}
	for _, f := range findings {
		k := key{f.Area.Code, severityDisplay(f.Severity)}
		if _, seen := buckets[k]; !seen {
			order = append(order, k)
		}
		buckets[k] = append(buckets[k], f)
	}

	var out [][]numberedFinding
	for _, k := range order {
		group := buckets[k]
		for i := 0; i < len(group); i += size {
			end := i + size
			if end > len(group) {
				end = len(group)
			}
			out = append(out, group[i:end])
		}
	}
	return out
}

// renderIssuesSlide fills one issues table and titles it for the findings it
// holds, then removes the rows it did not need.
func renderIssuesSlide(slide string, group []numberedFinding, index, total int) string {
	slide = setPlaceholders(slide, map[string]string{
		"[Area] Issues – [Severity] Level ([Index]/[Total])": issuesTitle(group, index, total),
	})

	tblStart := strings.Index(slide, "<a:tbl>")
	tblEnd := strings.Index(slide, "</a:tbl>")
	if tblStart < 0 || tblEnd < 0 {
		return slide
	}
	tblEnd += len("</a:tbl>")
	tbl := slide[tblStart:tblEnd]

	rows := aRowRe.FindAllStringIndex(tbl, -1)
	// Row 0 is the header; the rest are the findings' rows.
	var b strings.Builder
	prev := 0
	for ri, span := range rows {
		if ri == 0 {
			continue
		}
		row := tbl[span[0]:span[1]]
		b.WriteString(tbl[prev:span[0]])
		if fi := ri - 1; fi < len(group) {
			f := group[fi]
			b.WriteString(setPlaceholders(row, map[string]string{
				"[Issue]":          issueSummary(f),
				"[Rating]":         severityDisplay(f.Severity),
				"[Recommendation]": recommendationText(f),
			}))
		}
		// A row with no finding is dropped rather than left blank.
		prev = span[1]
	}
	b.WriteString(tbl[prev:])
	return slide[:tblStart] + b.String() + slide[tblEnd:]
}

// issuesTitle names the slide after the areas and severities it actually shows,
// the way the hand-built decks do: "IPT Issues - Critical/High Level (1/19)".
func issuesTitle(group []numberedFinding, index, total int) string {
	areas := uniqueInOrder(func() []string {
		var out []string
		for _, f := range group {
			out = append(out, f.Area.Code)
		}
		return out
	}())
	sevs := uniqueInOrder(func() []string {
		var out []string
		for _, f := range group {
			out = append(out, severityDisplay(f.Severity))
		}
		return out
	}())
	return fmt.Sprintf("%s Issues – %s Level (%d/%d)",
		strings.Join(areas, "/"), strings.Join(sevs, "/"), index, total)
}

func uniqueInOrder(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range items {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// issueSummary is what the issues table shows for a finding: its title, a
// sentence of description, and the host it was found on.
func issueSummary(f numberedFinding) string {
	parts := []string{strings.TrimSpace(f.Title) + ":"}
	if d := firstSentence(f.Description); d != "" {
		parts = append(parts, d)
	}
	if host := strings.TrimSpace(f.AffectedSystem); host != "" {
		parts = append(parts, "Affected Host: "+host)
	}
	return strings.Join(parts, " ")
}

func recommendationText(f numberedFinding) string {
	if r := strings.TrimSpace(f.RecommendationHeader); r != "" {
		return r
	}
	return truncateText(strings.TrimSpace(f.Recommendation), 160)
}

// firstSentence keeps a finding's description to the one line a slide can hold.
func firstSentence(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if s == "" {
		return ""
	}
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return truncateText(s, 220)
}

// ---------------------------------------------------------------------------
// the vulnerability scenario slides
// ---------------------------------------------------------------------------

// scenarioRows is what the details table beside the screenshot says, by
// position: the label in the first column, the finding's detail in the second.
//
// The cells are filled by position rather than by placeholder token. Every value
// cell in that table carries the same "[Issue]" token, so replacing by token put
// the finding's title into the label column and into Result as well.
func scenarioRows(f numberedFinding) [][2]string {
	result := strings.TrimSpace(f.Impact)
	if result == "" {
		result = firstSentence(f.Description)
	}
	return [][2]string{
		{"Vulnerability", strings.TrimSpace(f.Title)},
		{"Affected Host", strings.TrimSpace(f.AffectedSystem)},
		{"Result", result},
	}
}

// renderScenarioSlide fills one scenario slide for a finding. The screenshot is
// attached separately, by pointPictureAt.
func renderScenarioSlide(slide string, f numberedFinding) string {
	slide = setPlaceholders(slide, map[string]string{
		"Vulnerability Scenario – [Vulnerability ID]": "Vulnerability Scenario – " + f.VulnID,
	})
	return fillTableByPosition(slide, scenarioRows(f))
}

// fillTableByPosition writes label/value pairs into a table's body rows, after
// the header, leaving any row it has no pair for untouched.
func fillTableByPosition(slide string, rows [][2]string) string {
	tblStart := strings.Index(slide, "<a:tbl>")
	tblEnd := strings.Index(slide, "</a:tbl>")
	if tblStart < 0 || tblEnd < 0 {
		return slide
	}
	tblEnd += len("</a:tbl>")
	tbl := slide[tblStart:tblEnd]

	var b strings.Builder
	prev := 0
	for ri, span := range aRowRe.FindAllStringIndex(tbl, -1) {
		if ri == 0 || ri-1 >= len(rows) {
			continue // the header, or a row with nothing to say
		}
		row := tbl[span[0]:span[1]]
		cells := aCellRe.FindAllStringIndex(row, -1)
		pair := rows[ri-1]
		var rb strings.Builder
		rprev := 0
		for ci, cs := range cells {
			if ci > 1 {
				break
			}
			rb.WriteString(row[rprev:cs[0]])
			rb.WriteString(setCellParaText(row[cs[0]:cs[1]], pair[ci]))
			rprev = cs[1]
		}
		rb.WriteString(row[rprev:])
		b.WriteString(tbl[prev:span[0]])
		b.WriteString(rb.String())
		prev = span[1]
	}
	b.WriteString(tbl[prev:])
	return slide[:tblStart] + b.String() + slide[tblEnd:]
}

// setCellParaText makes a table cell read exactly text, in its first paragraph.
func setCellParaText(cell, text string) string {
	done := false
	return aParaRe.ReplaceAllStringFunc(cell, func(para string) string {
		if done || strings.TrimSpace(aParaText(para)) == "" {
			return para
		}
		done = true
		return setAParaText(para, text)
	})
}

// pointPictureAt repoints the slide's picture frame at a new relationship id.
func pointPictureAt(slide, relID string) string {
	if relID == "" {
		return slide
	}
	return blipRe.ReplaceAllString(slide, `<a:blip r:embed="`+relID+`"`)
}

// clonedRelsWithImage copies a slide's relationships and adds one for the
// screenshot, returning the new relationship file and the id to point at.
//
// The image relationship the template carries is left in place but unreferenced;
// dropping it would mean renumbering the rest, and an unused relationship is
// harmless where a mismatched one is not.
func clonedRelsWithImage(rels, mediaName string) (string, string) {
	id := fmt.Sprintf("rId%d", nextFreeRelNumber(rels))
	entry := fmt.Sprintf(`<Relationship Id="%s" Type="%s" Target="../media/%s"/>`,
		id, imageRelTy, mediaName)
	return strings.Replace(rels, "</Relationships>", entry+"</Relationships>", 1), id
}

var relNumRe = regexp.MustCompile(`Id="rId(\d+)"`)

func nextFreeRelNumber(rels string) int {
	max := 0
	for _, m := range relNumRe.FindAllStringSubmatch(rels, -1) {
		if n, err := parseInt(m[1]); err == nil && n > max {
			max = n
		}
	}
	return max + 1
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// ---------------------------------------------------------------------------
// screenshots
// ---------------------------------------------------------------------------

// addMedia puts a proof-of-concept screenshot into the package.
//
// The bytes are the evidence record's own, the same ones the DOCX report
// embeds, so the deck cannot show a different screenshot from the report for the
// same finding. They are decoded first only to reject anything that is not
// actually an image - PowerPoint reports the whole file as corrupt rather than
// skipping a picture it cannot read.
func (d *closureDeck) addMedia(img POCImage) (string, error) {
	if len(img.Data) == 0 {
		return "", fmt.Errorf("the evidence record holds no data")
	}
	if _, format, err := image.DecodeConfig(bytes.NewReader(img.Data)); err != nil {
		return "", fmt.Errorf("not a readable image: %w", err)
	} else if format == "" {
		return "", fmt.Errorf("unrecognised image format")
	}

	ext := strings.ToLower(pngExtRe.FindString(img.Filename))
	if ext == "" {
		ext = ".png"
	}
	name := fmt.Sprintf("poc%d%s", d.nextMediaNum, ext)
	d.nextMediaNum++
	d.add("ppt/media/"+name, img.Data)
	d.declareImageType(ext)
	return name, nil
}

// declareImageType makes sure the package advertises the extension. PNG and
// JPEG are already declared by the template; anything else has to be added or
// PowerPoint refuses the file.
func (d *closureDeck) declareImageType(ext string) {
	types := d.part("[Content_Types].xml")
	if types == nil {
		return
	}
	e := strings.TrimPrefix(ext, ".")
	body := string(types.Data)
	if strings.Contains(strings.ToLower(body), `extension="`+e+`"`) {
		return
	}
	mime := "image/" + e
	if e == "jpg" {
		mime = "image/jpeg"
	}
	entry := fmt.Sprintf(`<Default Extension="%s" ContentType="%s"/>`, e, mime)
	types.Data = []byte(strings.Replace(body, "<Types ", entry+"<Types ", 1))
	if !strings.Contains(string(types.Data), entry) {
		types.Data = []byte(strings.Replace(body, "</Types>", entry+"</Types>", 1))
	}
}
