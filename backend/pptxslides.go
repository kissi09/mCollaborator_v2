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
		return setAFieldText(para, text)
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
	return out[:runs[0][0]] + stripHyperlink(retexted) + out[runs[0][1]:]
}

var (
	aFldRe    = regexp.MustCompile(`(?s)<a:fld [^>]*>.*?</a:fld>`)
	aFldRPrRe = regexp.MustCompile(`(?s)<a:rPr[^>]*(?:/>|>.*?</a:rPr>)`)
)

// setAFieldText fills a paragraph whose only content is a PowerPoint field.
//
// The template's date sits in an <a:fld type="datetime5">, which holds no
// <a:r> run at all, so the ordinary path left "[Date]" printing on every slide.
// The field is replaced by a plain run rather than re-texted in place, and
// deliberately so: a datetime field is refreshed by PowerPoint when the deck is
// opened, which would quietly relabel a report dated in August with whatever day
// the closing meeting happens to be held on.
func setAFieldText(para, text string) string {
	fld := aFldRe.FindStringIndex(para)
	if fld == nil {
		return para
	}
	rPr := aFldRPrRe.FindString(para[fld[0]:fld[1]])
	run := "<a:r>" + rPr + "<a:t>" + xmlEscape(text) + "</a:t></a:r>"
	return para[:fld[0]] + run + para[fld[1]:]
}

// ---------------------------------------------------------------------------
// styled runs
// ---------------------------------------------------------------------------

// A cell in the issues table is not one piece of text. The source deck writes
// the finding's title in bold, its description in plain, and labels the host
// with a bold "Affected Host:" before the plain address - three paragraphs of
// mixed weight in a single cell. Replacing the cell's [Issue] placeholder with
// one joined string inherited the placeholder's own bold and printed the whole
// lot bold, which is what made the tables read as a wall of heavy text.
//
// These build a paragraph run by run instead, cloning the properties off the
// template's own run so the font, size and language survive, and setting only
// the weight and colour that differ.

var (
	aRPrRe     = regexp.MustCompile(`(?s)<a:rPr[^>]*?(?:/>|>.*?</a:rPr>)`)
	aRPrOpenRe = regexp.MustCompile(`^<a:rPr[^>]*?/?>`)
	aRPrBoldRe = regexp.MustCompile(` b="[01]"`)
	aPPrRe     = regexp.MustCompile(`(?s)<a:pPr[^>]*?(?:/>|>.*?</a:pPr>)`)
	aSolidFill = regexp.MustCompile(`(?s)<a:solidFill>.*?</a:solidFill>`)
	aLnCloseRe = regexp.MustCompile(`(?s)<a:ln[^>]*?(?:/>|>.*?</a:ln>)`)
)

// styledRun is one run of a built paragraph: its text, whether it is bold, and
// an optional colour. A zero Hex leaves the template's colour alone.
type styledRun struct {
	Text string
	Bold bool
	Hex  string
}

// The deck's body font ships as two separate families rather than one family
// with a bold weight, so b="1" alone changes nothing: a run set in "Wavehaus
// 128 Bold" prints bold however its b attribute reads, and one set in "Wavehaus
// 66 Book" prints light. Weight is a choice of typeface here, and setting b
// without swapping the face is why the issues tables came out uniformly heavy.
const (
	deckBoldFace  = "Wavehaus 128 Bold"
	deckLightFace = "Wavehaus 66 Book"
)

var deckFaceRe = regexp.MustCompile(`typeface="(?:` +
	regexp.QuoteMeta(deckBoldFace) + `|` + regexp.QuoteMeta(deckLightFace) + `)"`)

// withRunBold returns rPr with b set explicitly and, where the run is set in
// one of the deck's two Wavehaus faces, switched to the face for that weight.
//
// Explicitly, not by omission: these runs sit in table cells whose placeholder
// was bold, and a run that says nothing about weight keeps whatever it
// inherits. A run in any other font is left to the b attribute alone.
func withRunBold(rPr string, bold bool) string {
	val := ` b="0"`
	face := deckLightFace
	if bold {
		val = ` b="1"`
		face = deckBoldFace
	}
	if strings.TrimSpace(rPr) == "" {
		return `<a:rPr lang="en-US"` + val + `/>`
	}
	open := aRPrOpenRe.FindString(rPr)
	if open == "" {
		return rPr
	}
	rest := deckFaceRe.ReplaceAllString(rPr[len(open):], `typeface="`+face+`"`)
	open = aRPrBoldRe.ReplaceAllString(open, "")
	const tag = "<a:rPr"
	return open[:len(tag)] + val + open[len(tag):] + rest
}

// withPptRunColor returns rPr with the text colour set to hex.
//
// DrawingML orders a run's fill after <a:ln> and before everything else, so a
// colour being added where there is none goes immediately after the line
// properties if the run has any, and otherwise straight after the opening tag.
func withPptRunColor(rPr, hex string) string {
	fill := `<a:solidFill><a:srgbClr val="` + hex + `"/></a:solidFill>`
	if strings.TrimSpace(rPr) == "" {
		return `<a:rPr lang="en-US">` + fill + `</a:rPr>`
	}
	if aSolidFill.MatchString(rPr) {
		return aSolidFill.ReplaceAllString(rPr, fill)
	}
	// A self-closing rPr has to be opened up before anything can go inside it.
	open := aRPrOpenRe.FindString(rPr)
	if strings.HasSuffix(open, "/>") {
		return open[:len(open)-2] + ">" + fill + "</a:rPr>"
	}
	rest := rPr[len(open):]
	if ln := aLnCloseRe.FindString(rest); ln != "" {
		i := strings.Index(rest, ln) + len(ln)
		return open + rest[:i] + fill + rest[i:]
	}
	return open + fill + rest
}

// buildAPara rebuilds a paragraph as the given runs, keeping the template
// paragraph's alignment and borrowing its first run's properties.
func buildAPara(tmpl string, runs []styledRun) string {
	pPr := aPPrRe.FindString(tmpl)
	base := ""
	if r := aRunRe.FindString(tmpl); r != "" {
		base = aRPrRe.FindString(r)
	}
	var b strings.Builder
	b.WriteString("<a:p>")
	b.WriteString(pPr)
	for _, r := range runs {
		rPr := withRunBold(base, r.Bold)
		if r.Hex != "" {
			rPr = withPptRunColor(rPr, r.Hex)
		}
		b.WriteString(stripHyperlink("<a:r>" + rPr + "<a:t>" + xmlEscape(r.Text) + "</a:t></a:r>"))
	}
	b.WriteString("</a:p>")
	return b.String()
}

// setCellParas makes a table cell read as exactly these paragraphs, styled off
// the paragraph the template left in it.
func setCellParas(cell string, paras [][]styledRun) string {
	existing := childElems(cell, "a:p")
	if len(existing) == 0 {
		return cell
	}
	tmpl := cell[existing[0].Start:existing[0].End]
	var b strings.Builder
	for _, runs := range paras {
		b.WriteString(buildAPara(tmpl, runs))
	}
	return cell[:existing[0].Start] + b.String() + cell[existing[len(existing)-1].End:]
}

// ---------------------------------------------------------------------------
// the slides that appear once
// ---------------------------------------------------------------------------

// fillFixedSlides fills the placeholders that mean the same thing wherever they
// appear - the client's name, the engagement date, the running footer.
//
// The executive summary's own placeholders are deliberately not handled here.
// [Scope], [Affected Host], [Target] and [Finding] each appear many times over
// on slides 2 and 3, once per area panel, and they only read correctly when each
// copy is filled from the area whose panel it sits in. Replacing them all with
// a single value is what put every area's scope under every area's heading, and
// what left the slide jumbled when there was no scope to put there at all. They
// are handled per panel in pptxsummary.go.
func (d *closureDeck) fillFixedSlides(config ReportConfig) {
	company := strings.TrimSpace(config.CompanyName)
	if company == "" {
		company = "Client"
	}
	values := map[string]string{
		"[Company Name]": company,
		"[Date]":         strings.TrimSpace(config.ReportDate),
	}
	if values["[Date]"] == "" {
		delete(values, "[Date]")
	}

	for _, p := range d.parts {
		if !strings.HasPrefix(p.Name, "ppt/slides/slide") {
			continue
		}
		p.Data = []byte(setPlaceholders(string(p.Data), values))
	}
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
			b.WriteString(setIssueRow(row, group[fi]))
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

// setIssueRow fills one row of an issues table: the finding on the left, its
// criticality in the middle, the recommendation on the right.
//
// The cells are filled by position rather than by placeholder token. Each needs
// its own weight and colour, which a token substitution cannot give it - the
// placeholder carries one set of run properties and every value replacing it
// came out looking the same.
func setIssueRow(row string, f numberedFinding) string {
	cells := childElems(row, "a:tc")
	var b strings.Builder
	prev := 0
	for ci, c := range cells {
		cell := row[c.Start:c.End]
		var filled string
		switch ci {
		case 0:
			filled = setCellParas(cell, issueParas(f))
		case 1:
			filled = setCellParas(cell, [][]styledRun{{{
				Text: deckSeverityLabel(f.Severity),
				Bold: true,
				Hex:  severityHex(f.Severity),
			}}})
		case 2:
			filled = setCellParas(cell, [][]styledRun{{{Text: recommendationText(f)}}})
		default:
			continue
		}
		b.WriteString(row[prev:c.Start])
		b.WriteString(filled)
		prev = c.End
	}
	b.WriteString(row[prev:])
	return b.String()
}

// deckSeverityLabel names a criticality for the issues table's Severity column.
//
// The column is a little over an inch wide, sized for "Critical". The report
// writes the band out in full, and "Informational" does not fit: it wrapped
// mid-word as "Informati / onal", which reads as a broken slide. The deck
// already has a short form for exactly this reason - both the doughnut's legend
// and the criticality key beside the ring say "Info" - so the column uses the
// name the rest of the deck uses. The slide title is prose with room for the
// full word and keeps it.
func deckSeverityLabel(sev string) string {
	label := severityDisplay(sev)
	if label == "Informational" {
		return "Info"
	}
	return label
}

// issueParas is what the issues table shows for a finding, laid out the way the
// source deck lays it out: the title in bold with a colon, the description
// under it in plain text, and the host on its own line behind a bold label.
func issueParas(f numberedFinding) [][]styledRun {
	paras := [][]styledRun{{{Text: strings.TrimSpace(f.Title) + ":", Bold: true}}}
	if d := firstSentence(f.Description); d != "" {
		paras = append(paras, []styledRun{{Text: d}})
	}
	if host := strings.TrimSpace(f.AffectedSystem); host != "" {
		paras = append(paras, []styledRun{
			{Text: "Affected Host: ", Bold: true},
			{Text: host},
		})
	}
	return paras
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
//
// part is which of the finding's screenshots this slide carries. A finding with
// more than one proof runs over several slides, and the source deck marks the
// later ones "(Cont'd)" rather than repeating the same heading verbatim - three
// identical titles in a row read as a duplicated slide.
func renderScenarioSlide(slide string, f numberedFinding, part int) string {
	title := "Vulnerability Scenario – " + f.VulnID
	if part > 1 {
		title += " (Cont’d)"
	}
	slide = setPlaceholders(slide, map[string]string{
		"Vulnerability Scenario – [Vulnerability ID]": title,
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

// pointPictureAt repoints the slide's picture frame at a new relationship id and
// reshapes the frame to the screenshot's own proportions.
//
// The template's frame is square, because the proof in the deck it came from
// was. Screenshots are not: a wide terminal capture dropped into it without
// rescaling is stretched to a square, which distorts exactly the text a client
// is being asked to read. The frame is fitted inside the square the template
// drew and centred there, so the evidence in the deck is the same picture the
// report shows rather than a squashed copy of it.
func pointPictureAt(slide, relID string, imgW, imgH int) string {
	if relID == "" {
		return slide
	}
	slide = blipRe.ReplaceAllString(slide, `<a:blip r:embed="`+relID+`"`)
	frame := pPicRe.FindStringIndex(slide)
	if frame == nil {
		return slide
	}
	return slide[:frame[0]] +
		fitLogoFrame(slide[frame[0]:frame[1]], relID, imgW, imgH) +
		slide[frame[1]:]
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
func (d *closureDeck) addMedia(img POCImage) (string, image.Config, error) {
	if len(img.Data) == 0 {
		return "", image.Config{}, fmt.Errorf("the evidence record holds no data")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(img.Data))
	if err != nil {
		return "", image.Config{}, fmt.Errorf("not a readable image: %w", err)
	}
	if format == "" {
		return "", image.Config{}, fmt.Errorf("unrecognised image format")
	}

	ext := strings.ToLower(pngExtRe.FindString(img.Filename))
	if ext == "" {
		ext = ".png"
	}
	name := fmt.Sprintf("poc%d%s", d.nextMediaNum, ext)
	d.nextMediaNum++
	d.add("ppt/media/"+name, img.Data)
	d.declareImageType(ext)
	return name, cfg, nil
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

// ---------------------------------------------------------------------------
// the customer's logo
// ---------------------------------------------------------------------------

const titleSlidePart = "ppt/slides/slide1.xml"

var (
	pPicRe   = regexp.MustCompile(`(?s)<p:pic>.*?</p:pic>`)
	aXfrmOff = regexp.MustCompile(`<a:off x="(-?\d+)" y="(-?\d+)"/>`)
	aXfrmExt = regexp.MustCompile(`<a:ext cx="(\d+)" cy="(\d+)"/>`)
	aSrcRect = regexp.MustCompile(`<a:srcRect[^>]*/>`)
	aBlipRe  = regexp.MustCompile(`<a:blip r:embed="[^"]*"`)
)

// addCustomerLogo drops the client's logo into the title slide's logo frame.
//
// The template's opening slide reserves the frame the source deck put its
// client's logo in, and mkclosuretemplate left a grey placeholder image in it -
// which is why an unfilled deck opens on what looks like a broken picture. The
// frame is filled rather than a new picture added beside it, so the logo lands
// exactly where the deck was designed for it.
//
// Three things about the frame have to be corrected as it is filled: it carries
// a crop cut for the previous client's artwork, its aspect ratio is that
// client's, and it is stretched to fill. The crop is removed and the frame
// resized to the uploaded logo's own proportions, centred in the space the
// template allotted, so nothing is squashed.
//
// With no logo uploaded the frame is deleted, on the same reasoning the report
// deletes its header placeholder: a grey "no image" box on the title slide of a
// document handed to a client is worse than nothing there at all.
//
// Returns the reason the logo could not be used, rather than nothing. The deck
// is presented with the client in the room, and their logo quietly missing from
// the opening slide is the kind of thing only noticed once it is on screen.
func (d *closureDeck) addCustomerLogo(payload string) string {
	slide := d.part(titleSlidePart)
	rels := d.part(relsNameFor(titleSlidePart))
	if slide == nil || rels == nil {
		return "the closure template has no title slide"
	}
	frame := pPicRe.FindStringIndex(string(slide.Data))
	if frame == nil {
		return "the closure template's title slide has no logo frame"
	}

	if strings.TrimSpace(payload) == "" {
		body := string(slide.Data)
		slide.Data = []byte(body[:frame[0]] + body[frame[1]:])
		return ""
	}

	img, err := decodeUploadedImage(payload)
	if err != nil {
		return err.Error()
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return "logo image has empty bounds"
	}
	pngBytes, err := encodeImagePNG(img)
	if err != nil {
		return err.Error()
	}

	name := fmt.Sprintf("customer_logo%d.png", d.nextMediaNum)
	d.nextMediaNum++
	d.add("ppt/media/"+name, pngBytes)
	d.declareImageType(".png")

	relID := fmt.Sprintf("rId%d", nextFreeRelNumber(string(rels.Data)))
	rels.Data = []byte(strings.Replace(string(rels.Data), "</Relationships>",
		fmt.Sprintf(`<Relationship Id="%s" Type="%s" Target="../media/%s"/>`, relID, imageRelTy, name)+
			"</Relationships>", 1))

	body := string(slide.Data)
	pic := fitLogoFrame(body[frame[0]:frame[1]], relID, b.Dx(), b.Dy())
	slide.Data = []byte(body[:frame[0]] + pic + body[frame[1]:])

	// The same logo goes in the middle of the executive summary's ring. It is
	// put on the summary template before that slide is cloned, so every
	// headline slide the engagement needs carries it without the picture being
	// added, and relationship-numbered, once per clone.
	d.logoInSummaryRing(name, b.Dx(), b.Dy())
	return ""
}

// logoInSummaryRing adds a relationship for the already-embedded logo to the
// summary template's own relationship part and drops the picture into the
// ring's empty centre.
func (d *closureDeck) logoInSummaryRing(mediaName string, imgW, imgH int) {
	slide := d.part(summaryTemplatePart)
	rels := d.part(relsNameFor(summaryTemplatePart))
	if slide == nil || rels == nil {
		return
	}
	relID := fmt.Sprintf("rId%d", nextFreeRelNumber(string(rels.Data)))
	rels.Data = []byte(strings.Replace(string(rels.Data), "</Relationships>",
		fmt.Sprintf(`<Relationship Id="%s" Type="%s" Target="../media/%s"/>`, relID, imageRelTy, mediaName)+
			"</Relationships>", 1))
	slide.Data = []byte(placeLogoInRing(string(slide.Data), relID, imgW, imgH))
}

// The executive summary's ring is drawn with a hole in the middle, and the
// source deck leaves it empty. These are that hole, measured off the rendered
// slide: centre and diameter in EMU, on a 13.333 x 7.5 inch slide.
const (
	ringHoleCX       = 6053088
	ringHoleCY       = 3348038
	ringHoleDiameter = 1466850

	// How much of the hole the logo is allowed to take, as a percentage. A mark
	// set edge to edge in a circle reads as clipped even when it is not, and a
	// client's logo is the last thing to crowd.
	ringLogoFillPct = 68
)

// placeLogoInRing drops the client's logo into the empty middle of the summary
// slide's ring, scaled to its own proportions and centred in the hole.
//
// The picture is appended to the slide's shape tree rather than filling a frame
// the template drew, because the template draws none there - the ring's centre
// is a gap in artwork, not a placeholder.
func placeLogoInRing(slide, relID string, imgW, imgH int) string {
	if relID == "" || imgW <= 0 || imgH <= 0 {
		return slide
	}
	end := strings.LastIndex(slide, "</p:spTree>")
	if end < 0 {
		return slide
	}

	boxed := ringHoleDiameter * ringLogoFillPct / 100
	scale := float64(boxed) / float64(imgW)
	if s := float64(boxed) / float64(imgH); s < scale {
		scale = s
	}
	w := int(float64(imgW) * scale)
	h := int(float64(imgH) * scale)

	pic := fmt.Sprintf(`<p:pic><p:nvPicPr>`+
		`<p:cNvPr id="%d" name="Customer Logo"/><p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr>`+
		`<p:nvPr/></p:nvPicPr>`+
		`<p:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></p:blipFill>`+
		`<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`+
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>`,
		ringLogoShapeID, relID,
		ringHoleCX-w/2, ringHoleCY-h/2, w, h)

	return slide[:end] + pic + slide[end:]
}

// ringLogoShapeID is above anything the template numbers, so the id stays
// unique however many shapes the slide already carries.
const ringLogoShapeID = 9001

// fitLogoFrame points a picture frame at relID and reshapes it to the image's
// own aspect ratio, centred inside the box the template drew.
func fitLogoFrame(pic, relID string, imgW, imgH int) string {
	pic = aBlipRe.ReplaceAllString(pic, `<a:blip r:embed="`+relID+`"`)
	// The crop belonged to the artwork that used to be here.
	pic = aSrcRect.ReplaceAllString(pic, "")

	off := aXfrmOff.FindStringSubmatch(pic)
	ext := aXfrmExt.FindStringSubmatch(pic)
	if len(off) != 3 || len(ext) != 3 {
		return pic
	}
	x, _ := parseInt(off[1])
	y, _ := parseInt(off[2])
	boxW, _ := parseInt(ext[1])
	boxH, _ := parseInt(ext[2])
	if boxW <= 0 || boxH <= 0 || imgW <= 0 || imgH <= 0 {
		return pic
	}

	scale := float64(boxW) / float64(imgW)
	if s := float64(boxH) / float64(imgH); s < scale {
		scale = s
	}
	w := int(float64(imgW) * scale)
	h := int(float64(imgH) * scale)

	pic = aXfrmOff.ReplaceAllString(pic,
		fmt.Sprintf(`<a:off x="%d" y="%d"/>`, x+(boxW-w)/2, y+(boxH-h)/2))
	return aXfrmExt.ReplaceAllString(pic, fmt.Sprintf(`<a:ext cx="%d" cy="%d"/>`, w, h))
}

var hlinkRe = regexp.MustCompile(`(?s)<a:hlinkClick[^>]*(?:/>|>.*?</a:hlinkClick>)`)

// stripHyperlink removes a link from a run whose text has just been replaced.
//
// Three cells of the template's scope table are hyperlinked, because in the
// source deck the tester had made the tested URLs clickable. Keeping the first
// run so the font survives kept the link with it, so this client's scope printed
// underlined and blue, pointing at the previous engagement's target.
func stripHyperlink(run string) string {
	if !hlinkRe.MatchString(run) {
		return run
	}
	// The underline goes with the link. PowerPoint underlines linked text by
	// setting u="sng" on the run beside the hlinkClick, so removing only the
	// link leaves the scope reading as a link that no longer goes anywhere.
	return underlineAttrRe.ReplaceAllString(hlinkRe.ReplaceAllString(run, ""), "")
}

var underlineAttrRe = regexp.MustCompile(` u="sng"`)
