package main

// Closure-deck generation.
//
// The closing meeting is presented from a deck, and until now that deck was
// rebuilt by hand from the report every time. This renders it from the same
// findings the DOCX report is built from, against a template derived from the
// deck the team already presents (see tools/mkclosuretemplate).
//
// Two of the template's slides are repeated rather than filled in place:
//
//	slide 4   the issues table, four findings to a slide
//	slide 23  a vulnerability scenario, one per proof-of-concept screenshot
//
// Repeating a slide in a .pptx is not like repeating a row in a .docx. A slide
// is its own package part, and four other parts have to agree that it exists:
// the presentation's slide list, the presentation's relationships, the slide's
// own relationships, and the content-type table. Each clone below updates all
// four, and the two template slides are dropped once their clones are made.
//
// Screenshots come from the same place the report's do - the finding's evidence
// records, resolved by the export handler - so a finding's proof in the deck is
// the same file as its proof in the report and in the evidence vault, rather
// than a second copy that can drift.

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"regexp"
	"strings"
)

//go:embed templates/mcollaborator_closure_template.pptx
var closureTemplatePptx []byte

// The template parts the renderer repeats. mkclosuretemplate keeps the source
// deck's own part names, so these are the numbers from the original deck.
const (
	issuesTemplatePart   = "ppt/slides/slide4.xml"
	scenarioTemplatePart = "ppt/slides/slide23.xml"
)

// issuesPerSlide is how many findings the template's table holds. The table has
// exactly this many body rows; unused ones are removed.
const issuesPerSlide = 4

// closureDeck is the package being assembled.
type closureDeck struct {
	parts []*pptPart

	// nextSlideNum is the part number given to the next cloned slide. It starts
	// past every number the template uses so a clone can never collide with a
	// slide that is still in the package.
	nextSlideNum int
	nextRelID    int
	nextSldID    int
	nextMediaNum int
}

type pptPart struct {
	Name string
	Data []byte
}

// ---------------------------------------------------------------------------
// entry point
// ---------------------------------------------------------------------------

// buildClosureDeck renders the closing-meeting deck for an engagement.
func buildClosureDeck(config ReportConfig) ([]byte, *reportNotes, error) {
	notes := &reportNotes{}
	deck, err := openClosureTemplate()
	if err != nil {
		return nil, nil, err
	}

	findings := buildNumberedFindings(config)
	if len(findings) == 0 {
		return nil, nil, fmt.Errorf("the engagement has no findings to present")
	}

	issuesTmpl := deck.part(issuesTemplatePart)
	scenarioTmpl := deck.part(scenarioTemplatePart)
	summaryTmpl := deck.part(summaryTemplatePart)
	if issuesTmpl == nil || scenarioTmpl == nil || summaryTmpl == nil {
		return nil, nil, fmt.Errorf("the closure template is missing its repeated slides")
	}
	issuesRels := deck.part(relsNameFor(issuesTemplatePart))
	scenarioRels := deck.part(relsNameFor(scenarioTemplatePart))
	summaryRels := deck.part(relsNameFor(summaryTemplatePart))

	// Fill the placeholders that read the same on every slide.
	deck.fillFixedSlides(config)
	notes.LogoError = deck.addCustomerLogo(config.CompanyLogo)

	// The executive summary: one panel per assessment area, on the scope slide
	// and again over the headline findings, plus the two charts beside them.
	panels := buildAreaPanels(config, findings)
	deck.fillScopeSlide(config, panels, len(findings))
	deck.fillCharts(config, findings)

	var summarySlides []string
	for _, group := range chunkPanels(panels, summaryPanelsPerSlide) {
		body := renderSummarySlide(string(summaryTmpl.Data), group)
		summarySlides = append(summarySlides, deck.addSlide(body, summaryRels))
	}

	// The issues tables, in report order, four findings to a slide.
	groups := chunkFindings(findings, issuesPerSlide)
	var issueSlides []string
	for i, group := range groups {
		body := renderIssuesSlide(string(issuesTmpl.Data), group, i+1, len(groups))
		name := deck.addSlide(body, issuesRels)
		issueSlides = append(issueSlides, name)
	}

	// One scenario slide per screenshot, which is how the source deck reads: a
	// finding with two proofs gets two slides under the same heading.
	var scenarioSlides []string
	shots := 0
	for _, f := range findings {
		if len(f.POCImages) == 0 {
			notes.FindingsWithoutProof = append(notes.FindingsWithoutProof, strings.TrimSpace(f.Title))
			continue
		}
		part := 0
		for _, img := range f.POCImages {
			mediaName, cfg, err := deck.addMedia(img)
			if err != nil {
				// A single unreadable screenshot must not lose the whole deck.
				notes.FindingsWithoutProof = append(notes.FindingsWithoutProof,
					fmt.Sprintf("%s (screenshot %q could not be embedded: %v)",
						strings.TrimSpace(f.Title), img.Filename, err))
				continue
			}
			part++
			body := renderScenarioSlide(string(scenarioTmpl.Data), f, part)
			rels, relID := clonedRelsWithImage(string(scenarioRels.Data), mediaName)
			body = pointPictureAt(body, relID, cfg.Width, cfg.Height)
			scenarioSlides = append(scenarioSlides, deck.addSlideRaw(body, rels))
			shots++
		}
	}

	// Put the deck in order, then drop the three templates now that every clone
	// has been taken from them.
	if err := deck.orderSlides(summarySlides, issueSlides, scenarioSlides); err != nil {
		return nil, nil, err
	}
	deck.dropSlide(issuesTemplatePart)
	deck.dropSlide(scenarioTemplatePart)
	deck.dropSlide(summaryTemplatePart)

	out, err := deck.zip()
	if err != nil {
		return nil, nil, err
	}
	return out, notes, nil
}

// ---------------------------------------------------------------------------
// package handling
// ---------------------------------------------------------------------------

func openClosureTemplate() (*closureDeck, error) {
	zr, err := zip.NewReader(bytes.NewReader(closureTemplatePptx), int64(len(closureTemplatePptx)))
	if err != nil {
		return nil, fmt.Errorf("open closure template: %w", err)
	}
	deck := &closureDeck{nextSlideNum: 1000, nextRelID: 900, nextSldID: 900, nextMediaNum: 1}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		deck.parts = append(deck.parts, &pptPart{strings.ReplaceAll(f.Name, "\\", "/"), data})
	}
	return deck, nil
}

func (d *closureDeck) part(name string) *pptPart {
	for _, p := range d.parts {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func (d *closureDeck) add(name string, data []byte) {
	d.parts = append(d.parts, &pptPart{name, data})
}

func (d *closureDeck) zip() ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range d.parts {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: p.Name, Method: zip.Deflate})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(p.Data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func relsNameFor(slidePart string) string {
	i := strings.LastIndex(slidePart, "/")
	return slidePart[:i] + "/_rels" + slidePart[i:] + ".rels"
}

// ---------------------------------------------------------------------------
// cloning slides
// ---------------------------------------------------------------------------

// addSlide writes a new slide part with a copy of the template's relationships.
func (d *closureDeck) addSlide(body string, rels *pptPart) string {
	relsData := ""
	if rels != nil {
		relsData = string(rels.Data)
	}
	return d.addSlideRaw(body, relsData)
}

// addSlideRaw writes a new slide part and everything that has to know about it:
// its relationship part, its content type, a relationship from the
// presentation, and an entry in the presentation's slide list.
func (d *closureDeck) addSlideRaw(body, rels string) string {
	num := d.nextSlideNum
	d.nextSlideNum++
	name := fmt.Sprintf("ppt/slides/slide%d.xml", num)

	d.add(name, []byte(body))
	if rels != "" {
		d.add(relsNameFor(name), []byte(stripNotesSlideRel(rels)))
	}
	d.declareContentType(name)
	relID := d.linkFromPresentation(name)
	d.appendToSlideList(relID)
	return name
}

const slideContentType = "application/vnd.openxmlformats-officedocument.presentationml.slide+xml"

func (d *closureDeck) declareContentType(slidePart string) {
	types := d.part("[Content_Types].xml")
	if types == nil {
		return
	}
	entry := fmt.Sprintf(`<Override PartName="/%s" ContentType="%s"/>`, slidePart, slideContentType)
	body := string(types.Data)
	types.Data = []byte(strings.Replace(body, "</Types>", entry+"</Types>", 1))
}

const slideRelType = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide"

func (d *closureDeck) linkFromPresentation(slidePart string) string {
	rels := d.part("ppt/_rels/presentation.xml.rels")
	if rels == nil {
		return ""
	}
	id := fmt.Sprintf("rId%d", d.nextRelID)
	d.nextRelID++
	target := strings.TrimPrefix(slidePart, "ppt/")
	entry := fmt.Sprintf(`<Relationship Id="%s" Type="%s" Target="%s"/>`, id, slideRelType, target)
	body := string(rels.Data)
	rels.Data = []byte(strings.Replace(body, "</Relationships>", entry+"</Relationships>", 1))
	return id
}

func (d *closureDeck) appendToSlideList(relID string) {
	pres := d.part("ppt/presentation.xml")
	if pres == nil || relID == "" {
		return
	}
	id := d.nextSldID
	d.nextSldID++
	entry := fmt.Sprintf(`<p:sldId id="%d" r:id="%s"/>`, id, relID)
	body := string(pres.Data)
	pres.Data = []byte(strings.Replace(body, "</p:sldIdLst>", entry+"</p:sldIdLst>", 1))
}

var sldIdEntryRe = regexp.MustCompile(`<p:sldId id="\d+" r:id="(rId\d+)"\s*/>`)
var relEntryRe = regexp.MustCompile(`<Relationship [^>]*/>`)
var attrIDRe = regexp.MustCompile(`Id="([^"]+)"`)
var attrTargetRe = regexp.MustCompile(`Target="([^"]+)"`)

// orderSlides rewrites the slide list so the generated slides sit where the
// templates they came from used to be: the headline-findings slides after the
// scope slide, issues tables after those, scenarios next, closing slide last.
func (d *closureDeck) orderSlides(summarySlides, issueSlides, scenarioSlides []string) error {
	pres := d.part("ppt/presentation.xml")
	presRels := d.part("ppt/_rels/presentation.xml.rels")
	if pres == nil || presRels == nil {
		return fmt.Errorf("the closure template is missing its presentation part")
	}

	relOfSlide := map[string]string{}
	for _, m := range relEntryRe.FindAllString(string(presRels.Data), -1) {
		target := submatchOf(attrTargetRe, m)
		if strings.HasPrefix(target, "slides/slide") {
			relOfSlide["ppt/"+target] = submatchOf(attrIDRe, m)
		}
	}

	var order []string
	appendSlide := func(part string) {
		if id, ok := relOfSlide[part]; ok {
			order = append(order, id)
		}
	}
	for _, n := range []string{"ppt/slides/slide1.xml", "ppt/slides/slide2.xml"} {
		appendSlide(n)
	}
	for _, n := range summarySlides {
		appendSlide(n)
	}
	for _, n := range issueSlides {
		appendSlide(n)
	}
	for _, n := range scenarioSlides {
		appendSlide(n)
	}
	appendSlide("ppt/slides/slide49.xml")

	var list strings.Builder
	list.WriteString("<p:sldIdLst>")
	for i, id := range order {
		fmt.Fprintf(&list, `<p:sldId id="%d" r:id="%s"/>`, 256+i, id)
	}
	list.WriteString("</p:sldIdLst>")

	body := string(pres.Data)
	start := strings.Index(body, "<p:sldIdLst>")
	end := strings.Index(body, "</p:sldIdLst>")
	if start < 0 || end < 0 {
		return fmt.Errorf("the closure template has no slide list")
	}
	pres.Data = []byte(body[:start] + list.String() + body[end+len("</p:sldIdLst>"):])
	return nil
}

const notesSlideRelType = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesSlide"

var notesRelEntryRe = regexp.MustCompile(`<Relationship [^>]*/notesSlide"[^>]*/>`)

// stripNotesSlideRel drops the speaker-notes relationship from a cloned slide's
// relationships.
//
// A notes slide belongs to exactly one slide and holds a relationship pointing
// back at it. Copying the reference into every clone left several slides
// claiming the same notes page, and the page itself still pointing at the
// template slide that gets deleted once the clones are made - a dangling part
// reference, which is a file PowerPoint refuses to open rather than one it
// opens imperfectly. The notes were written about the template's example
// content anyway, so there is nothing to carry across.
func stripNotesSlideRel(rels string) string {
	return notesRelEntryRe.ReplaceAllString(rels, "")
}

// notesSlideOf returns the notes part a slide owns, or "".
func (d *closureDeck) notesSlideOf(slidePart string) string {
	rels := d.part(relsNameFor(slidePart))
	if rels == nil {
		return ""
	}
	m := notesRelEntryRe.FindString(string(rels.Data))
	if m == "" {
		return ""
	}
	target := submatchOf(attrTargetRe, m)
	return "ppt/" + strings.TrimPrefix(target, "../")
}

// dropSlide removes a slide and every reference to it. Used on the template
// slides once their clones exist, so the deck does not open on an empty
// placeholder table.
func (d *closureDeck) dropSlide(slidePart string) {
	presRels := d.part("ppt/_rels/presentation.xml.rels")
	pres := d.part("ppt/presentation.xml")
	types := d.part("[Content_Types].xml")

	// The slide's own notes page goes with it; left behind, it still points at
	// a slide that is no longer in the package.
	if notes := d.notesSlideOf(slidePart); notes != "" {
		d.dropPart(notes)
		d.dropPart(relsNameFor(notes))
		if types != nil {
			types.Data = []byte(regexp.MustCompile(`<Override PartName="/`+regexp.QuoteMeta(notes)+`"[^>]*/>`).
				ReplaceAllString(string(types.Data), ""))
		}
	}

	relID := ""
	if presRels != nil {
		body := relEntryRe.ReplaceAllStringFunc(string(presRels.Data), func(m string) string {
			if "ppt/"+submatchOf(attrTargetRe, m) != slidePart {
				return m
			}
			relID = submatchOf(attrIDRe, m)
			return ""
		})
		presRels.Data = []byte(body)
	}
	if pres != nil && relID != "" {
		body := sldIdEntryRe.ReplaceAllStringFunc(string(pres.Data), func(m string) string {
			if submatchOf(sldIdEntryRe, m) == relID {
				return ""
			}
			return m
		})
		pres.Data = []byte(body)
	}
	if types != nil {
		body := regexp.MustCompile(`<Override PartName="/`+regexp.QuoteMeta(slidePart)+`"[^>]*/>`).
			ReplaceAllString(string(types.Data), "")
		types.Data = []byte(body)
	}

	rels := relsNameFor(slidePart)
	kept := d.parts[:0]
	for _, p := range d.parts {
		if p.Name == slidePart || p.Name == rels {
			continue
		}
		kept = append(kept, p)
	}
	d.parts = kept
}

// dropPart removes one part from the package.
func (d *closureDeck) dropPart(name string) {
	kept := d.parts[:0]
	for _, p := range d.parts {
		if p.Name == name {
			continue
		}
		kept = append(kept, p)
	}
	d.parts = kept
}

func submatchOf(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}
