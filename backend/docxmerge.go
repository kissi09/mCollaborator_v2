package main

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"
)

//go:embed templates/vapt_report_template.docx
var vaptTemplateDocx []byte

const (
	findingRowMarkerBegin    = "<!--REPEAT:FINDING_ROW-->"
	findingRowMarkerEnd      = "<!--END:FINDING_ROW-->"
	findingDetailMarkerBegin = "<!--REPEAT:FINDING_DETAIL-->"
	findingDetailMarkerEnd   = "<!--END:FINDING_DETAIL-->"
)

// xmlEscapeText escapes a plain string for safe placement inside OOXML text
// content (not attribute values). It also turns embedded newlines into real
// Word line breaks so multi-line findings fields render correctly.
func xmlEscapeText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, "\r")
	}
	return strings.Join(lines, `</w:t></w:r><w:r><w:br/><w:t xml:space="preserve">`)
}

// applyTokens replaces every {{Key}} occurrence in s with its escaped value.
func applyTokens(s string, tokens map[string]string) string {
	for k, v := range tokens {
		s = strings.ReplaceAll(s, "{{"+k+"}}", xmlEscapeText(v))
	}
	return s
}

// expandRepeatBlock finds a <!--REPEAT:NAME-->...<!--END:NAME--> region in s,
// renders it once per item (with that item's tokens substituted), and
// replaces the whole marked region (markers included) with the concatenated
// result. If the markers aren't present, s is returned unchanged - this lets
// the merge engine run against a template that hasn't had its repeating
// sections marked up yet, without failing the whole export.
func expandRepeatBlock(s, beginMarker, endMarker string, items []map[string]string) (string, error) {
	start := strings.Index(s, beginMarker)
	if start == -1 {
		return s, nil
	}
	end := strings.Index(s, endMarker)
	if end == -1 || end < start {
		return "", fmt.Errorf("found %s without matching %s", beginMarker, endMarker)
	}
	blockTemplate := s[start+len(beginMarker) : end]

	var out strings.Builder
	for _, item := range items {
		out.WriteString(applyTokens(blockTemplate, item))
	}

	return s[:start] + out.String() + s[end+len(endMarker):], nil
}

func severityHex(sev string) (fill, text string) {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return "C00000", "FFFFFF"
	case "high":
		return "FF7900", "FFFFFF"
	case "medium":
		return "FFC000", "000000"
	case "low":
		return "28A745", "FFFFFF"
	default:
		return "2F6FED", "FFFFFF"
	}
}

// buildFindingTokens produces the per-finding token maps used by both the
// vulnerability-register row block and the detailed 3.2.x block.
func buildFindingTokens(f ReportFinding, idx int) map[string]string {
	id := vulnID(f, idx+1)
	fill, text := severityHex(f.Severity)
	poc := f.POC
	if containsImage(poc) {
		poc = "See PoC screenshot in the separate evidence attachment."
	}
	return map[string]string{
		"FindingNumber":           strconv.Itoa(idx + 1),
		"FindingTitle":            f.Title,
		"FindingExposure":         exposureOf(f),
		"FindingCriticality":      severityDisplay(f.Severity),
		"FindingCriticalityColor": fill,
		"FindingCriticalityText":  text,
		"FindingVulnID":           id,
		"FindingDescription":      f.Description,
		"FindingCVSS":             f.CVSSVector,
		"FindingImpact":           f.Impact,
		"FindingAffected":         f.AffectedSystem,
		"FindingPOC":              poc,
		"FindingRecommendation":   f.Recommendation,
	}
}

// mergeVAPTDocx renders the embedded Word template against config, producing
// a byte-for-byte valid .docx (same styles/charts/headers as the original
// template - only the token text and repeated finding blocks are swapped).
func mergeVAPTDocx(config ReportConfig) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(vaptTemplateDocx), int64(len(vaptTemplateDocx)))
	if err != nil {
		return nil, fmt.Errorf("open template: %w", err)
	}

	scalarTokens := map[string]string{
		"CompanyName":      config.CompanyName,
		"VersionLabel":     config.VersionLabel,
		"TesterName":       config.TesterName,
		"ApproverName":     config.ApproverName,
		"ApproverTitle":    config.ApproverTitle,
		"AssessmentPeriod": fmt.Sprintf("%s to %s", config.AssessmentStart, config.AssessmentEnd),
		"ReportDate":       strings.ToUpper(time.Now().Format("02-Jan-2006")),
		"RefNumber":        config.RefNumber,
		"FindingsCount":    strconv.Itoa(len(config.Findings)),
	}
	if strings.TrimSpace(config.VersionLabel) == "" {
		scalarTokens["VersionLabel"] = "1.0"
	}

	rowItems := make([]map[string]string, len(config.Findings))
	detailItems := make([]map[string]string, len(config.Findings))
	for i, f := range config.Findings {
		t := buildFindingTokens(f, i)
		rowItems[i] = t
		detailItems[i] = t
	}

	// Swap the template's stale client logo for the uploaded one, if provided.
	// A bad upload must not sink the whole export, so failures fall back to the
	// template's own asset.
	clientLogo, err := renderClientLogoPart(config.CompanyLogo)
	if err != nil {
		log.Printf("report: ignoring client logo (%v)", err)
		clientLogo = nil
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.Name, err)
		}

		if f.Name == clientLogoPart && clientLogo != nil {
			data = clientLogo
		} else if f.Name == "word/document.xml" {
			s := string(data)
			s, err = expandRepeatBlock(s, findingRowMarkerBegin, findingRowMarkerEnd, rowItems)
			if err != nil {
				return nil, fmt.Errorf("finding row block: %w", err)
			}
			s, err = expandRepeatBlock(s, findingDetailMarkerBegin, findingDetailMarkerEnd, detailItems)
			if err != nil {
				return nil, fmt.Errorf("finding detail block: %w", err)
			}
			s = applyTokens(s, scalarTokens)
			data = []byte(s)
		} else if isMergeTargetPart(f.Name) {
			data = []byte(applyTokens(string(data), scalarTokens))
		}

		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: f.Method})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func isMergeTargetPart(name string) bool {
	switch name {
	case "word/footer4.xml", "word/footer5.xml", "word/footer7.xml":
		return true
	default:
		return false
	}
}
