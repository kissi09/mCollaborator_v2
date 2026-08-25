package main

// docxmcollabpkg.go assembles the rendered mCollaborator report package: it
// walks the template's zip entries, hands the parts that carry content to the
// renderers in docxmcollab.go, drops the customer logo into the running header,
// and appends the proof-of-concept media parts.

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"
)

// docxPart is one entry of the .docx zip package while it is being rewritten.
type docxPart struct {
	Name   string
	Method uint16
	Data   []byte
}

// mergeMCollaboratorDocx renders the embedded template against config and
// returns a complete .docx.
func mergeMCollaboratorDocx(config ReportConfig) ([]byte, *reportNotes, error) {
	zr, err := zip.NewReader(bytes.NewReader(mcollabTemplateDocx), int64(len(mcollabTemplateDocx)))
	if err != nil {
		return nil, nil, fmt.Errorf("open template: %w", err)
	}

	// Read every part up front: the document body decides which relationships
	// and media parts the package needs, so nothing can be written until it has
	// been rendered.
	parts := make([]docxPart, 0, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, nil, fmt.Errorf("open %s: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", f.Name, err)
		}
		// .NET-produced packages can use backslash separators; normalize so the
		// part-name comparisons below match.
		parts = append(parts, docxPart{strings.ReplaceAll(f.Name, "\\", "/"), f.Method, data})
	}

	findings := buildNumberedFindings(config)
	pocs := &pocCollector{}
	notes := &reportNotes{}

	logo, err := renderHeaderLogo(config.CompanyLogo)
	if err != nil {
		log.Printf("report: ignoring client logo (%v)", err)
		notes.LogoError = err.Error()
		logo = nil
	}

	// Work out which media part is Cyberteq's own header logo, by looking at what
	// the headers that reserve a customer-logo slot display alongside it. Any
	// other header showing that same image is a running body header and should
	// carry the customer logo too.
	relsByPart := map[string]string{}
	houseLogoTargets := map[string]bool{}
	for _, p := range parts {
		if strings.HasSuffix(p.Name, ".rels") {
			relsByPart[p.Name] = string(p.Data)
		}
	}
	for _, p := range parts {
		if !isHeaderPart(p.Name) || !strings.Contains(string(p.Data), logoSlotMarker) {
			continue
		}
		rels := relsByPart["word/_rels/"+strings.TrimPrefix(p.Name, "word/")+".rels"]
		for _, target := range imageTargetsOf(rels) {
			houseLogoTargets[target] = true
		}
	}

	logoHeaders := map[string]bool{}
	for i := range parts {
		switch {
		case parts[i].Name == "word/document.xml":
			rendered, err := renderDocumentBody(string(parts[i].Data), config, findings, pocs, notes)
			if err != nil {
				return nil, nil, err
			}
			parts[i].Data = []byte(rendered)

		case parts[i].Name == "word/charts/chartEx1.xml":
			cats, vals := severityChartData(findings)
			parts[i].Data = []byte(renderChartData(string(parts[i].Data), cats, vals))

		case parts[i].Name == "word/charts/chartEx2.xml":
			cats, vals := areaChartData(config, findings)
			parts[i].Data = []byte(renderChartData(string(parts[i].Data), cats, vals))

		case parts[i].Name == "word/settings.xml":
			parts[i].Data = []byte(enableFieldUpdate(string(parts[i].Data)))

		case isHeaderPart(parts[i].Name):
			s := applyScalarPlaceholders(string(parts[i].Data), config)
			// The template reserves a customer-logo slot in the running headers
			// of the body sections but not in the one the Tools and Licenses and
			// Appendix pages use, so that section is given the same slot; a logo
			// that stops partway through the report reads as a mistake.
			if logo != nil && !strings.Contains(s, logoSlotMarker) &&
				headerCarriesHouseLogo(parts[i].Name, houseLogoTargets, relsByPart) {
				if withSlot, ok := addLogoBeforeHouseLogo(s, logo); ok {
					s = withSlot
					logoHeaders[parts[i].Name] = true
				}
			}
			if strings.Contains(s, logoSlotMarker) {
				if logo != nil {
					s = logo.replaceSlot(s)
					logoHeaders[parts[i].Name] = true
				} else {
					// No logo was uploaded, so the slot is removed rather than
					// left in place: it is an outlined box reading "Customer
					// logo, scale to height 0.5" - an instruction to whoever
					// fills the template in, which has no business printing on
					// every page of a delivered report.
					s = removeLogoSlot(s)
				}
			}
			parts[i].Data = []byte(s)

		case isFooterPart(parts[i].Name):
			s := healPlaceholderRuns(string(parts[i].Data))
			s = renderFooterText(s, config)
			s = applyScalarPlaceholders(s, config)
			// Last, because how much room the footer line needs depends on how
			// long this client name turned out to be.
			parts[i].Data = []byte(fitFooterSpacing(s))
		}
	}

	// Register the new media parts with the relationship files that reference
	// them: PoC screenshots hang off the document, the logo off every header
	// that carried the customer-logo slot.
	var extra []pocPart
	if len(logoHeaders) > 0 {
		extra = append(extra, pocPart{Name: logoMediaPart, RelID: logoRelID, Data: logo.PNG})
	}
	extra = append(extra, pocs.parts...)

	for i := range parts {
		if parts[i].Name == "word/_rels/document.xml.rels" && len(pocs.parts) > 0 {
			parts[i].Data = []byte(addImageRelationships(string(parts[i].Data), pocs.parts))
			continue
		}
		if header, ok := headerForRels(parts[i].Name); ok && logoHeaders[header] {
			parts[i].Data = []byte(addImageRelationships(string(parts[i].Data),
				[]pocPart{{Name: logoMediaPart, RelID: logoRelID}}))
		}
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range parts {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: p.Name, Method: p.Method})
		if err != nil {
			return nil, nil, err
		}
		if _, err := w.Write(p.Data); err != nil {
			return nil, nil, err
		}
	}
	for _, p := range extra {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: p.Name, Method: zip.Deflate})
		if err != nil {
			return nil, nil, err
		}
		if _, err := w.Write(p.Data); err != nil {
			return nil, nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), notes, nil
}

// renderDocumentBody applies every body-level transformation in an order that
// keeps each step working on a document the previous step already settled.
func renderDocumentBody(doc string, config ReportConfig, findings []numberedFinding, pocs *pocCollector, notes *reportNotes) (string, error) {
	// Word splits placeholder text across runs; heal that first so every later
	// step can match "[Company Name]" literally.
	doc = healPlaceholderRuns(doc)

	doc = fixClientCompanyBookmark(doc)
	doc = renderCoverTable(doc)
	doc = renderTableOfContents(doc, config)
	doc = renderScopeTable(doc, config)
	doc = renderMethodologySections(doc, config)
	doc = renderNamingConvention(doc, config)

	var err error
	doc, err = renderAreaSections(doc, config, findings, pocs, notes)
	if err != nil {
		return "", err
	}

	doc = renderVulnerabilityRegister(doc, findings)
	doc = renderToolsTable(doc, config)
	doc = renderAppendix(doc, config)

	doc = applySummaryPlaceholders(doc, findings)
	doc = applyScalarPlaceholders(doc, config)
	return doc, nil
}

func isHeaderPart(name string) bool {
	return strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml")
}

func isFooterPart(name string) bool {
	return strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml")
}

// headerForRels maps "word/_rels/header5.xml.rels" to "word/header5.xml".
func headerForRels(name string) (string, bool) {
	const prefix = "word/_rels/header"
	const suffix = ".xml.rels"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return "", false
	}
	n := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
	return "word/header" + n + ".xml", true
}

// addImageRelationships appends image relationship entries to a .rels part.
func addImageRelationships(rels string, images []pocPart) string {
	var b strings.Builder
	for _, p := range images {
		if strings.Contains(rels, `Id="`+p.RelID+`"`) {
			continue
		}
		b.WriteString(`<Relationship Id="` + p.RelID +
			`" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/` +
			strings.TrimPrefix(p.Name, "word/media/") + `"/>`)
	}
	if b.Len() == 0 {
		return rels
	}
	return strings.Replace(rels, "</Relationships>", b.String()+"</Relationships>", 1)
}

// ---------------------------------------------------------------------------
// customer logo
// ---------------------------------------------------------------------------

const (
	// logoSlotMarker is the prompt text inside the empty rectangle the template
	// reserves for the customer's logo in the running page header.
	logoSlotMarker = "Customer logo"
	logoMediaPart  = "word/media/customer_logo.png"
	logoRelID      = "rIdCustomerLogo"

	// logoSlotHeightEMU is the 0.5 inch height the template asks the logo to be
	// scaled to; logoSlotWidthEMU is the width of the reserved rectangle.
	logoSlotHeightEMU = 457200
	logoSlotWidthEMU  = 2442210
)

var altContentRe = regexp.MustCompile(`(?s)<mc:AlternateContent>.*?</mc:AlternateContent>`)

// headerLogo is an uploaded customer logo prepared for the header slot.
type headerLogo struct {
	PNG  []byte
	CX   int64 // rendered width in EMU
	CY   int64 // rendered height in EMU
	seen int
}

// renderHeaderLogo decodes the wizard's upload into a PNG part and works out the
// drawing size: the template asks for a 0.5 inch tall logo, so height is fixed
// and width follows the image's own aspect ratio (capped at the slot width).
// Returns nil when no logo was supplied.
func renderHeaderLogo(payload string) (*headerLogo, error) {
	if strings.TrimSpace(payload) == "" {
		return nil, nil
	}
	img, err := decodeUploadedImage(payload)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return nil, fmt.Errorf("logo image has empty bounds")
	}
	pngBytes, err := encodeImagePNG(img)
	if err != nil {
		return nil, err
	}

	cy := int64(logoSlotHeightEMU)
	cx := int64(float64(cy) * float64(b.Dx()) / float64(b.Dy()))
	if cx > logoSlotWidthEMU {
		cx = logoSlotWidthEMU
		cy = int64(float64(cx) * float64(b.Dy()) / float64(b.Dx()))
	}
	return &headerLogo{PNG: pngBytes, CX: cx, CY: cy}, nil
}

// replaceSlot swaps the header's placeholder rectangle for the uploaded logo.
func (l *headerLogo) replaceSlot(header string) string {
	done := false
	return altContentRe.ReplaceAllStringFunc(header, func(m string) string {
		if done || !strings.Contains(m, logoSlotMarker) {
			return m
		}
		done = true
		l.seen++
		return inlineImageRun(logoRelID, 4000+l.seen, l.CX, l.CY, "Customer Logo")
	})
}

// addLogoBeforeHouseLogo places the customer logo ahead of Cyberteq's own logo
// in a running header that has no reserved slot, mirroring how the headers that
// do reserve one lay the pair out: customer logo, two tabs, house logo.
func addLogoBeforeHouseLogo(header string, logo *headerLogo) (string, bool) {
	for _, r := range childElems(header, "w:r") {
		if !strings.Contains(header[r.Start:r.End], "<w:drawing>") {
			continue
		}
		logo.seen++
		run := inlineImageRun(logoRelID, 4000+logo.seen, logo.CX, logo.CY, "Customer Logo") +
			`<w:r><w:tab/><w:tab/></w:r>`
		return header[:r.Start] + run + header[r.Start:], true
	}
	return header, false
}

// headerCarriesHouseLogo reports whether a header displays the same Cyberteq
// logo image as the headers that reserve a customer-logo slot. It is how the
// running body headers are told apart from the cover's certification badges,
// which must not gain a customer logo.
func headerCarriesHouseLogo(headerName string, houseLogoTargets map[string]bool, relsByPart map[string]string) bool {
	rels, ok := relsByPart["word/_rels/"+strings.TrimPrefix(headerName, "word/")+".rels"]
	if !ok {
		return false
	}
	for target := range houseLogoTargets {
		if strings.Contains(rels, `Target="`+target+`"`) {
			return true
		}
	}
	return false
}

var relTargetRe = regexp.MustCompile(`Type="[^"]*/image"\s+Target="([^"]+)"`)

// imageTargetsOf lists the media parts a relationship file points at.
func imageTargetsOf(rels string) []string {
	var out []string
	for _, m := range relTargetRe.FindAllStringSubmatch(rels, -1) {
		out = append(out, m[1])
	}
	return out
}

// removeLogoSlot deletes the header's empty customer-logo placeholder, used when
// the wizard supplied no logo.
func removeLogoSlot(header string) string {
	done := false
	return altContentRe.ReplaceAllStringFunc(header, func(m string) string {
		if done || !strings.Contains(m, logoSlotMarker) {
			return m
		}
		done = true
		return ""
	})
}

// enableFieldUpdate asks Word to refresh field results on open so the table of
// contents matches the sections the report actually contains.
func enableFieldUpdate(settings string) string {
	if strings.Contains(settings, "<w:updateFields") {
		return settings
	}
	i := strings.Index(settings, "<w:settings")
	if i < 0 {
		return settings
	}
	gt := strings.Index(settings[i:], ">")
	if gt < 0 {
		return settings
	}
	gt += i + 1
	return settings[:gt] + `<w:updateFields w:val="true"/>` + settings[gt:]
}
