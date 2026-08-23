// Command mkclosuretemplate derives the closure-deck template from a real
// closing-meeting deck.
//
// The template's design is not invented here: it is the deck Cyberteq already
// presents, kept slide for slide so a generated deck looks like the ones the
// team has been building by hand. What this tool does is reduce that deck to its
// repeatable parts and take every trace of the client out of it.
//
// It keeps six slides:
//
//	1  title
//	2  executive summary - scope by assessment area
//	3  executive summary continued - headline findings, and the severity legend
//	4  issues table            <- repeated by the renderer, four findings a slide
//	23 vulnerability scenario  <- repeated by the renderer, one per finding
//	49 closing
//
// Slides 4 and 23 are the units the renderer clones. The rest are filled in
// place. Part names are deliberately left as they are rather than renumbered:
// PowerPoint addresses slides through relationships, not filenames, and
// renumbering is a large amount of risk for a tidier zip listing.
//
// Running it needs the source deck, which is client material and is not in this
// repository. The tool is kept anyway: it is the record of exactly what was
// removed, which matters more for a file derived from a real engagement than it
// would for an ordinary asset.
//
// Usage:
//
//	go run ./tools/mkclosuretemplate -in "VAPT - Closure Report.pptx" \
//	    -out templates/mcollaborator_closure_template.pptx
package main

import (
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// keptSlides are the source slide numbers that survive into the template, in
// presentation order.
var keptSlides = []int{1, 2, 3, 4, 23, 49}

func main() {
	in := flag.String("in", "", "path to the source closing-meeting deck")
	out := flag.String("out", "", "path to write the template to")
	flag.Parse()
	log.SetOutput(os.Stdout)
	log.SetFlags(0)

	if *in == "" || *out == "" {
		log.Fatal("both -in and -out are required")
	}

	parts, err := readParts(*in)
	if err != nil {
		log.Fatalf("read %s: %v", *in, err)
	}
	log.Printf("source deck: %d parts", len(parts))

	keep := map[int]bool{}
	for _, n := range keptSlides {
		keep[n] = true
	}

	parts, dropped := dropSlides(parts, keep)
	log.Printf("dropped %d slides, kept %v", dropped, keptSlides)

	scrubbed := scrubSlides(parts)
	log.Printf("scrubbed %d text runs of client detail", scrubbed)

	links := scrubExternalLinks(parts)
	log.Printf("rewrote %d external hyperlink targets", links)

	paras := scrubParagraphs(parts)
	log.Printf("replaced %d engagement-specific paragraphs with placeholders", paras)

	imgs, err := replaceSlideImagery(parts)
	if err != nil {
		log.Fatalf("replace slide imagery: %v", err)
	}
	log.Printf("replaced %d slide images with placeholders", imgs)

	parts, pruned := pruneOrphanMedia(parts)
	log.Printf("pruned %d unreferenced media files", pruned)

	if err := writeParts(*out, parts); err != nil {
		log.Fatalf("write %s: %v", *out, err)
	}
	info, _ := os.Stat(*out)
	log.Printf("wrote %s (%d parts, %d bytes)", *out, len(parts), info.Size())

	if remaining := auditClientTraces(parts); len(remaining) > 0 {
		log.Printf("WARNING: possible client detail still present:")
		for _, r := range remaining {
			log.Printf("  %s", r)
		}
		os.Exit(1)
	}
	log.Printf("audit clean: no client names, hosts or dates found")
}

// ---------------------------------------------------------------------------
// package plumbing
// ---------------------------------------------------------------------------

type part struct {
	Name string
	Data []byte
}

func readParts(path string) ([]*part, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, err
	}
	var parts []*part
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
		parts = append(parts, &part{strings.ReplaceAll(f.Name, "\\", "/"), data})
	}
	return parts, nil
}

func writeParts(path string, parts []*part) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range parts {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: p.Name, Method: zip.Deflate})
		if err != nil {
			return err
		}
		if _, err := w.Write(p.Data); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func find(parts []*part, name string) *part {
	for _, p := range parts {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// remove returns parts with everything matching pred taken out, and how many
// that was.
//
// It builds a fresh slice rather than compacting in place. Compacting leaves the
// caller holding a slice of the original length whose tail still points at parts
// that were supposed to be gone - which is how an earlier version of this tool
// wrote every client screenshot into the template as a duplicate zip entry and
// still reported them pruned.
func remove(parts []*part, pred func(string) bool) ([]*part, int) {
	kept := make([]*part, 0, len(parts))
	n := 0
	for _, p := range parts {
		if pred(p.Name) {
			n++
			continue
		}
		kept = append(kept, p)
	}
	return kept, n
}

// ---------------------------------------------------------------------------
// dropping slides
// ---------------------------------------------------------------------------

var (
	slideNameRe = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)
	sldIdRe     = regexp.MustCompile(`<p:sldId id="\d+" r:id="(rId\d+)"\s*/>`)
	relRe       = regexp.MustCompile(`<Relationship [^>]*/>`)
	relIDRe     = regexp.MustCompile(`Id="([^"]+)"`)
	relTargetRe = regexp.MustCompile(`Target="([^"]+)"`)
	overrideRe  = regexp.MustCompile(`<Override [^>]*/>`)
	partNameRe  = regexp.MustCompile(`PartName="([^"]+)"`)
)

// dropSlides removes the slides that are not kept, along with everything that
// points at them: the presentation's slide list, its relationships, the slides'
// own relationship parts and their content-type overrides.
func dropSlides(parts []*part, keep map[int]bool) ([]*part, int) {
	pres := find(parts, "ppt/presentation.xml")
	presRels := find(parts, "ppt/_rels/presentation.xml.rels")
	types := find(parts, "[Content_Types].xml")
	if pres == nil || presRels == nil || types == nil {
		log.Fatal("deck is missing presentation.xml, its rels, or [Content_Types].xml")
	}

	// Which relationship id points at which slide number.
	slideOfRel := map[string]int{}
	for _, m := range relRe.FindAllString(string(presRels.Data), -1) {
		id := submatch(relIDRe, m)
		target := submatch(relTargetRe, m)
		if n, ok := slideNumberOf("ppt/" + strings.TrimPrefix(target, "../")); ok {
			slideOfRel[id] = n
		}
	}

	// Prune the slide list, keeping the order the deck already had.
	var deadRels []string
	body := string(pres.Data)
	body = sldIdRe.ReplaceAllStringFunc(body, func(m string) string {
		id := submatch(sldIdRe, m)
		n, ok := slideOfRel[id]
		if !ok || keep[n] {
			return m
		}
		deadRels = append(deadRels, id)
		return ""
	})
	pres.Data = []byte(body)

	dead := map[string]bool{}
	for _, id := range deadRels {
		dead[id] = true
	}
	rels := string(presRels.Data)
	rels = relRe.ReplaceAllStringFunc(rels, func(m string) string {
		if dead[submatch(relIDRe, m)] {
			return ""
		}
		return m
	})
	presRels.Data = []byte(rels)

	// The slide parts themselves, and their own relationship files.
	parts, dropped := remove(parts, func(name string) bool {
		n, ok := slideNumberOf(name)
		return ok && !keep[n]
	})
	parts, _ = remove(parts, func(name string) bool {
		if !strings.HasPrefix(name, "ppt/slides/_rels/") {
			return false
		}
		n, ok := slideNumberOf("ppt/slides/" + strings.TrimSuffix(strings.TrimPrefix(name, "ppt/slides/_rels/"), ".rels"))
		return ok && !keep[n]
	})

	// Content types must not advertise parts that are gone.
	ct := string(types.Data)
	ct = overrideRe.ReplaceAllStringFunc(ct, func(m string) string {
		n, ok := slideNumberOf(strings.TrimPrefix(submatch(partNameRe, m), "/"))
		if ok && !keep[n] {
			return ""
		}
		return m
	})
	types.Data = []byte(ct)

	return parts, dropped
}

func slideNumberOf(name string) (int, bool) {
	m := slideNameRe.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

func submatch(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// ---------------------------------------------------------------------------
// pruning media
// ---------------------------------------------------------------------------

// pruneOrphanMedia deletes media parts nothing references any more. Dropping 43
// slides leaves most of the deck's 54 images behind, and they are screenshots of
// a real client's systems - they cannot be allowed to ride along invisibly
// inside the template.
func pruneOrphanMedia(parts []*part) ([]*part, int) {
	referenced := map[string]bool{}
	for _, p := range parts {
		if !strings.HasSuffix(p.Name, ".rels") {
			continue
		}
		for _, m := range relRe.FindAllString(string(p.Data), -1) {
			target := submatch(relTargetRe, m)
			if !strings.Contains(target, "media/") {
				continue
			}
			referenced["ppt/media/"+target[strings.LastIndex(target, "/")+1:]] = true
		}
	}
	return remove(parts, func(name string) bool {
		return strings.HasPrefix(name, "ppt/media/") && !referenced[name]
	})
}

// ---------------------------------------------------------------------------
// scrubbing
// ---------------------------------------------------------------------------

var (
	textRunRe = regexp.MustCompile(`(?s)<a:t>(.*?)</a:t>`)
	paraRe    = regexp.MustCompile(`(?s)<a:p>.*?</a:p>`)
	runRe     = regexp.MustCompile(`(?s)<a:r>.*?</a:r>`)
	rPrRe     = regexp.MustCompile(`(?s)<a:rPr[^>]*(?:/>|>.*?</a:rPr>)`)
)

// clientPatterns match text that belongs to the engagement the deck was built
// for, mapped to the placeholder that replaces it.
var clientPatterns = []struct {
	re   *regexp.Regexp
	with string
}{
	{regexp.MustCompile(`(?i)leasafric\s*ghana`), "[Company Name]"},
	{regexp.MustCompile(`(?i)leasafric`), "[Company Name]"},
	// Dates in the deck's own d-MMM-yy footer format.
	{regexp.MustCompile(`\b\d{1,2}-[A-Z][a-z]{2}-\d{2}\b`), "[Date]"},
	// IPv4 addresses and CIDR ranges, with or without a port.
	{regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(?:\s*/\s*\d{1,2})?(?:\s*\(\d+\))?`), "[Affected Host]"},
	// Hostnames under a real domain.
	{regexp.MustCompile(`https?://[^\s<]+`), "[Target]"},
	{regexp.MustCompile(`\b[a-z0-9.-]+\.(?:com|gh|net|org)(?:\.[a-z]{2})?\b`), "[Target]"},
}

// scrubSlides rewrites the visible text of every kept slide.
//
// Slides 4 and 23 are the repeatable ones, so their content cells are replaced
// wholesale with the placeholders the renderer fills. Everywhere else the client
// patterns are applied in place, which keeps the deck's own wording - headings,
// the severity legend, the standing narrative - intact.
func scrubSlides(parts []*part) int {
	count := 0
	for _, p := range parts {
		if !carriesVisibleText(p.Name) {
			continue
		}
		n, _ := slideNumberOf(p.Name)
		body := string(p.Data)
		switch n {
		case 4:
			body = placeholderTable(body, issuesPlaceholders)
		case 23:
			body = placeholderTable(body, scenarioPlaceholders)
		}
		body = textRunRe.ReplaceAllStringFunc(body, func(m string) string {
			inner := submatch(textRunRe, m)
			out := inner
			for _, cp := range clientPatterns {
				out = cp.re.ReplaceAllString(out, cp.with)
			}
			if out != inner {
				count++
			}
			return "<a:t>" + out + "</a:t>"
		})
		p.Data = []byte(body)
	}
	return count
}

// carriesVisibleText reports whether a part can hold text a reader will see.
//
// Slides are the obvious case, but the client name also sits in the layouts and
// masters: the deck's footer is a placeholder whose default text was typed once
// into the layout, and scrubbing only ppt/slides leaves it there. The audit
// found it, which is the whole reason the audit exists.
func carriesVisibleText(name string) bool {
	return strings.HasPrefix(name, "ppt/slides/slide") ||
		strings.HasPrefix(name, "ppt/slideLayouts/slideLayout") ||
		strings.HasPrefix(name, "ppt/slideMasters/slideMaster") ||
		strings.HasPrefix(name, "ppt/notesSlides/notesSlide")
}

// issuesPlaceholders is the text the renderer looks for in the issues table:
// a header row, then one row per finding.
var issuesPlaceholders = [][]string{
	{"Issues", "Severity", "Recommendation"},
	{"[Issue]", "[Rating]", "[Recommendation]"},
	{"[Issue]", "[Rating]", "[Recommendation]"},
	{"[Issue]", "[Rating]", "[Recommendation]"},
	{"[Issue]", "[Rating]", "[Recommendation]"},
}

// scenarioPlaceholders is the detail table beside a finding's screenshots.
var scenarioPlaceholders = [][]string{
	{"Details"},
	{"Vulnerability"},
	{"[Issue]"},
}

// placeholderTable replaces the text of each table cell with the placeholder
// for its position, leaving the table's own formatting untouched.
func placeholderTable(slide string, rows [][]string) string {
	tblStart := strings.Index(slide, "<a:tbl>")
	if tblStart < 0 {
		return slide
	}
	tblEnd := strings.Index(slide[tblStart:], "</a:tbl>")
	if tblEnd < 0 {
		return slide
	}
	tblEnd += tblStart + len("</a:tbl>")
	tbl := slide[tblStart:tblEnd]

	rowSpans := elementSpans(tbl, "a:tr")
	var b strings.Builder
	prev := 0
	for ri, rs := range rowSpans {
		if ri >= len(rows) {
			break
		}
		row := tbl[rs[0]:rs[1]]
		cellSpans := elementSpans(row, "a:tc")
		var rb strings.Builder
		rprev := 0
		for ci, cs := range cellSpans {
			if ci >= len(rows[ri]) {
				break
			}
			cell := row[cs[0]:cs[1]]
			rb.WriteString(row[rprev:cs[0]])
			rb.WriteString(setCellText(cell, rows[ri][ci]))
			rprev = cs[1]
		}
		rb.WriteString(row[rprev:])
		b.WriteString(tbl[prev:rs[0]])
		b.WriteString(rb.String())
		prev = rs[1]
	}
	b.WriteString(tbl[prev:])
	return slide[:tblStart] + b.String() + slide[tblEnd:]
}

// setCellText collapses a cell to a single paragraph holding text, keeping the
// run properties of whatever was there so the placeholder is styled like the
// content it stands in for.
func setCellText(cell, text string) string {
	paras := paraRe.FindAllStringIndex(cell, -1)
	if len(paras) == 0 {
		return cell
	}
	first := cell[paras[0][0]:paras[0][1]]
	rPr := ""
	if r := runRe.FindString(first); r != "" {
		rPr = rPrRe.FindString(r)
	}
	rebuilt := "<a:p><a:r>" + rPr + "<a:t>" + escape(text) + "</a:t></a:r></a:p>"
	// Everything after the first paragraph in the cell goes; a placeholder is
	// one line by definition.
	return cell[:paras[0][0]] + rebuilt + cell[paras[len(paras)-1][1]:]
}

func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// elementSpans returns the byte ranges of every outermost <tag> element,
// without descending into one already matched.
func elementSpans(s, tag string) [][2]int {
	open := "<" + tag
	closeTag := "</" + tag + ">"
	var out [][2]int
	for i := 0; ; {
		st := strings.Index(s[i:], open)
		if st < 0 {
			return out
		}
		st += i
		en := strings.Index(s[st:], closeTag)
		if en < 0 {
			return out
		}
		en += st + len(closeTag)
		out = append(out, [2]int{st, en})
		i = en
	}
}

// ---------------------------------------------------------------------------
// audit
// ---------------------------------------------------------------------------

// auditClientTraces re-reads the finished template looking for anything that
// still identifies the engagement. A template derived from a live deck is only
// safe to commit if this comes back empty, so a hit fails the build.
func auditClientTraces(parts []*part) []string {
	needles := []*regexp.Regexp{
		regexp.MustCompile(`(?i)leasafric`),
		regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`),
		regexp.MustCompile(`(?i)\b(?:expensepro|switchcarrentals)\b`),
	}
	var hits []string
	for _, p := range parts {
		if !strings.HasSuffix(p.Name, ".xml") && !strings.HasSuffix(p.Name, ".rels") {
			continue
		}
		body := string(p.Data)
		for _, re := range needles {
			for _, m := range re.FindAllString(body, 3) {
				hits = append(hits, fmt.Sprintf("%s: %q", p.Name, m))
			}
		}
	}
	sort.Strings(hits)
	return hits
}

// scrubExternalLinks rewrites the targets of external hyperlinks.
//
// The scope slide's application names were live links to the client's systems,
// and a URL in a relationship file is not text on a slide - the text scrub walks
// straight past it. The audit is what caught this; without it the template would
// have shipped with clickable links to a customer's estate.
func scrubExternalLinks(parts []*part) int {
	count := 0
	for _, p := range parts {
		if !strings.HasSuffix(p.Name, ".rels") {
			continue
		}
		body := relRe.ReplaceAllStringFunc(string(p.Data), func(m string) string {
			if !strings.Contains(m, `TargetMode="External"`) {
				return m
			}
			target := submatch(relTargetRe, m)
			if target == "" || strings.HasPrefix(target, placeholderLink) {
				return m
			}
			count++
			return strings.Replace(m, `Target="`+target+`"`, `Target="`+placeholderLink+`"`, 1)
		})
		p.Data = []byte(body)
	}
	return count
}

// placeholderLink stands in for every external hyperlink the source deck
// carried. It has to remain a valid URI or PowerPoint reports the file as
// corrupt.
const placeholderLink = "https://example.com/"

// ---------------------------------------------------------------------------
// placeholder imagery
// ---------------------------------------------------------------------------

// replaceSlideImagery swaps every image a kept slide points at for a generated
// placeholder of the same pixel size.
//
// Two of them survive the prune because the slides that use them survive: the
// client's logo on the title slide, and a proof-of-concept screenshot on the
// scenario slide. Both are the client's. Keeping the picture frames matters -
// they are the slots the renderer fills - but their contents cannot travel, so
// the frames stay and the pixels are replaced.
func replaceSlideImagery(parts []*part) (int, error) {
	targets := map[string]bool{}
	for _, p := range parts {
		if !strings.HasPrefix(p.Name, "ppt/slides/_rels/") {
			continue
		}
		for _, m := range relRe.FindAllString(string(p.Data), -1) {
			t := submatch(relTargetRe, m)
			if strings.Contains(t, "media/") {
				targets["ppt/media/"+t[strings.LastIndex(t, "/")+1:]] = true
			}
		}
	}

	count := 0
	for _, p := range parts {
		if !targets[p.Name] || !strings.HasSuffix(p.Name, ".png") {
			continue
		}
		img, err := png.Decode(bytes.NewReader(p.Data))
		if err != nil {
			return count, fmt.Errorf("%s: %w", p.Name, err)
		}
		b := img.Bounds()
		replacement, err := placeholderPNG(b.Dx(), b.Dy())
		if err != nil {
			return count, err
		}
		log.Printf("  replaced %s (%dx%d) with a placeholder", p.Name, b.Dx(), b.Dy())
		p.Data = replacement
		count++
	}
	return count, nil
}

// placeholderPNG draws a neutral image placeholder: a light panel, a dashed
// border and the usual picture glyph. No text, because drawing text would mean
// bundling a font for something every generated deck replaces anyway.
func placeholderPNG(w, h int) ([]byte, error) {
	if w < 8 || h < 8 {
		w, h = 320, 240
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	panel := color.RGBA{0xF1, 0xF3, 0xF6, 0xFF}
	edge := color.RGBA{0xB4, 0xBC, 0xC8, 0xFF}
	glyph := color.RGBA{0x9A, 0xA4, 0xB2, 0xFF}
	draw.Draw(img, img.Bounds(), &image.Uniform{panel}, image.Point{}, draw.Src)

	// Dashed border, two pixels thick.
	for x := 0; x < w; x++ {
		if (x/12)%2 == 1 {
			continue
		}
		for t := 0; t < 2; t++ {
			img.SetRGBA(x, t, edge)
			img.SetRGBA(x, h-1-t, edge)
		}
	}
	for y := 0; y < h; y++ {
		if (y/12)%2 == 1 {
			continue
		}
		for t := 0; t < 2; t++ {
			img.SetRGBA(t, y, edge)
			img.SetRGBA(w-1-t, y, edge)
		}
	}

	// A picture glyph: two hills and a sun, scaled to the frame.
	cx, cy := w/2, h/2
	s := min(w, h) / 4
	if s < 4 {
		return encodePNGBytes(img)
	}
	for x := cx - s; x <= cx+s; x++ {
		if x < 0 || x >= w {
			continue
		}
		// Two overlapping triangular hills.
		d1 := abs(x - (cx - s/2))
		d2 := abs(x - (cx + s/3))
		top := cy + s
		if v := cy + s - (s - d1); d1 < s && v < top {
			top = v
		}
		if v := cy + s - (2*s/3 - d2); d2 < 2*s/3 && v < top {
			top = v
		}
		for y := top; y <= cy+s; y++ {
			if y >= 0 && y < h {
				img.SetRGBA(x, y, glyph)
			}
		}
	}
	sunR := s / 4
	sunX, sunY := cx+s/2, cy-s/2
	for y := -sunR; y <= sunR; y++ {
		for x := -sunR; x <= sunR; x++ {
			if x*x+y*y > sunR*sunR {
				continue
			}
			px, py := sunX+x, sunY+y
			if px >= 0 && px < w && py >= 0 && py < h {
				img.SetRGBA(px, py, glyph)
			}
		}
	}
	return encodePNGBytes(img)
}

func encodePNGBytes(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// blanket paragraph scrub
// ---------------------------------------------------------------------------

// structuralText is the wording that belongs to the deck rather than to the
// engagement: headings, column labels and the severity legend. Everything else
// on a content slide is replaced.
//
// An allowlist is the right way round for this. A denylist has to anticipate
// every shape a client detail might take - an SSID, a firewall model, a finding
// title - and the ones it fails to anticipate ship. This way anything unfamiliar
// is replaced by default, and the cost of missing an entry is a placeholder
// where a heading should be, which is visible the moment the template is opened.
var structuralText = map[string]bool{}

func init() {
	for _, s := range []string{
		"executive summary", "(cont'd)", "(cont’d)", "scope",
		"executive summary (cont'd)", "executive summary (cont’d)",
		"issues", "severity", "recommendation", "details", "vulnerability", "result",
		"affected host", "affected hosts", "affected system", "affected application",
		"internal vapt", "external vapt", "wireless network assessment",
		"configuration review", "web application testing",
		"internal penetration testing", "ext. penetration testing",
		"external penetration testing", "config file review",
		"critical", "high", "medium", "low", "info", "informational",
		"immediate action required", "should be resolved quickly",
		"should be resolved", "will improve quality", "promotes best practices",
		"[company name] - vapt - closing meeting",
	} {
		structuralText[s] = true
	}
}

// slideTitles are the titles the renderer fills, written as placeholders.
var slideTitles = map[int]string{
	4:  "[Area] Issues – [Severity] Level ([Index]/[Total])",
	23: "Vulnerability Scenario – [Vulnerability ID]",
}

// contentSlides are the slides whose body text is engagement-specific.
var contentSlides = map[int]string{
	2:  "[Scope]",
	3:  "[Finding]",
	4:  "[Issue]",
	23: "[Issue]",
}

// scrubParagraphs replaces every paragraph that is not recognised structural
// wording with the slide's placeholder token, and rewrites the two repeated
// slides' titles.
func scrubParagraphs(parts []*part) int {
	count := 0
	for _, p := range parts {
		n, ok := slideNumberOf(p.Name)
		if !ok {
			continue
		}
		token, isContent := contentSlides[n]
		if !isContent {
			continue
		}
		title := slideTitles[n]
		titleDone := title == ""

		body := paraRe.ReplaceAllStringFunc(string(p.Data), func(para string) string {
			text := strings.TrimSpace(paraText(para))
			if text == "" {
				return para
			}
			key := strings.ToLower(strings.Join(strings.Fields(text), " "))
			if structuralText[key] || isPlaceholder(text) || isSlideNumberField(para) {
				return para
			}
			// The first non-structural paragraph on a repeated slide is its title.
			if !titleDone {
				titleDone = true
				count++
				return setParaText(para, title)
			}
			count++
			return setParaText(para, token)
		})
		p.Data = []byte(body)
	}
	return count
}

// placeholderOnlyRe matches text that is exactly one bracketed token.
//
// The brackets have to be the whole of it. Testing only that the text starts
// with "[" and ends with "]" also passes a whole sentence with a placeholder at
// each end, which is how "[Affected Host]-68, [Affected Host], 162.241.152 &
// [Affected Host]" survived the scrub and carried a real IP range with it.
var placeholderOnlyRe = regexp.MustCompile(`^\[[^\[\]]*\]$`)

// isPlaceholder reports whether a paragraph is already a placeholder, so the
// pass is idempotent and does not wrap "[Issue]" into "[Issue]" again.
func isPlaceholder(text string) bool {
	return placeholderOnlyRe.MatchString(strings.TrimSpace(text))
}

// isSlideNumberField spots the automatic slide-number placeholder, whose text is
// a field PowerPoint regenerates and which must be left alone.
func isSlideNumberField(para string) bool {
	return strings.Contains(para, `type="slidenum"`)
}

func paraText(para string) string {
	var b strings.Builder
	for _, m := range textRunRe.FindAllStringSubmatch(para, -1) {
		b.WriteString(m[1])
	}
	return b.String()
}

// setParaText makes a paragraph read exactly text.
//
// It keeps the paragraph's first run and rewrites the text inside it, then
// drops the remaining runs. Rebuilding the paragraph from scratch means
// reconstructing its properties, and getting that wrong produces a file
// PowerPoint refuses to open with no indication of which paragraph did it.
// Editing in place cannot go wrong that way.
func setParaText(para, text string) string {
	runs := runRe.FindAllStringIndex(para, -1)
	if len(runs) == 0 {
		return para
	}
	first := para[runs[0][0]:runs[0][1]]
	if !textRunRe.MatchString(first) {
		// The leading run carries no text of its own - a formatting-only run, or
		// a field. Rewriting runs would be guesswork, so every piece of text in
		// the paragraph is overwritten directly instead. Slower to read, but it
		// cannot leave part of a sentence behind, which is what matters when the
		// sentence is a client's IP range.
		firstText := true
		return textRunRe.ReplaceAllStringFunc(para, func(m string) string {
			if firstText {
				firstText = false
				return "<a:t>" + escape(text) + "</a:t>"
			}
			return "<a:t></a:t>"
		})
	}
	done := false
	retexted := textRunRe.ReplaceAllStringFunc(first, func(m string) string {
		if done {
			return ""
		}
		done = true
		return "<a:t>" + escape(text) + "</a:t>"
	})

	// Back to front, so the earlier offsets stay valid.
	out := para
	for i := len(runs) - 1; i >= 1; i-- {
		out = out[:runs[i][0]] + out[runs[i][1]:]
	}
	return out[:runs[0][0]] + retexted + out[runs[0][1]:]
}
