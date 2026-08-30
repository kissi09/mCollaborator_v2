package main

// The closure deck's two executive-summary charts.
//
// The template's charts carry the numbers of the engagement the source deck was
// built for - 18 internal findings, 10 critical, and so on - cached inside the
// chart parts. Nothing rewrote them, so every deck this app produced presented
// another client's figures under this client's name, beside a findings table
// that disagreed with them.
//
// Both are rewritten here from the same findings the report is built from:
//
//	chart1  "Discovered Security Vulnerabilities"  one bar per assessment area
//	chart2  "Issues by Severity"                   one ring segment per severity
//
// A .pptx chart keeps its data twice: a cached copy inside the chart part, which
// is what PowerPoint draws, and an embedded worksheet, which is what "Edit Data"
// opens. Only the cache is rewritten - the worksheet is a separate .xlsx part
// and rebuilding it would mean writing a spreadsheet to correct a picture nobody
// opens. Anyone who does click Edit Data sees the template's old numbers, which
// is why the caption below the chart is never sourced from there.

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	areaChartPart     = "ppt/charts/chart1.xml"
	severityChartPart = "ppt/charts/chart2.xml"
)

// severityRingOrder is the doughnut's five segments, in the order the template
// colours them: red, orange, amber, green, blue. The labels are the template's
// own short forms, so "Informational" counts land on the "Info" segment.
var severityRingOrder = []struct{ label, severity string }{
	{"Critical", "Critical"},
	{"High", "High"},
	{"Medium", "Medium"},
	{"Low", "Low"},
	{"Info", "Informational"},
}

// fillCharts rewrites both executive-summary charts from the engagement.
func (d *closureDeck) fillCharts(config ReportConfig, findings []numberedFinding) {
	cats, vals := areaChartData(config, findings)
	cats = fitCategoryLabels(config, cats)
	if len(cats) > 0 {
		if p := d.part(areaChartPart); p != nil {
			chart := setPptChartData(string(p.Data), cats, vals)
			chart = fitCategoryAxis(chart, len(cats))
			p.Data = []byte(fitBarGap(chart, len(cats)))
		}
	}

	counts := map[string]int{}
	for _, f := range findings {
		counts[severityDisplay(f.Severity)]++
	}
	ringCats := make([]string, len(severityRingOrder))
	ringVals := make([]int, len(severityRingOrder))
	for i, seg := range severityRingOrder {
		ringCats[i] = seg.label
		ringVals[i] = counts[seg.severity]
	}
	if p := d.part(severityChartPart); p != nil {
		p.Data = []byte(setPptChartData(string(p.Data), ringCats, ringVals))
	}
}

var (
	pptCatRe    = regexp.MustCompile(`(?s)<c:cat>.*?</c:cat>`)
	pptValRe    = regexp.MustCompile(`(?s)<c:val>.*?</c:val>`)
	pptDPtRe    = regexp.MustCompile(`(?s)<c:dPt>.*?</c:dPt>`)
	pptDPtIdxRe = regexp.MustCompile(`<c:idx val="(\d+)"/>`)
)

// setPptChartData rewrites a chart's cached categories and values.
//
// The <c:f> sheet references are rewritten alongside them so the cached range
// and the row count agree; PowerPoint reads the cache but re-derives the range
// when the chart is refreshed, and a stale range there truncates the series.
func setPptChartData(chart string, cats []string, vals []int) string {
	n := len(cats)
	if n == 0 {
		return chart
	}

	var strPts, numPts strings.Builder
	for i, c := range cats {
		fmt.Fprintf(&strPts, `<c:pt idx="%d"><c:v>%s</c:v></c:pt>`, i, xmlEscape(c))
		v := 0
		if i < len(vals) {
			v = vals[i]
		}
		fmt.Fprintf(&numPts, `<c:pt idx="%d"><c:v>%d</c:v></c:pt>`, i, v)
	}

	chart = replaceFirst(chart, pptCatRe, fmt.Sprintf(
		`<c:cat><c:strRef><c:f>Sheet1!$A$2:$A$%d</c:f>`+
			`<c:strCache><c:ptCount val="%d"/>%s</c:strCache></c:strRef></c:cat>`,
		n+1, n, strPts.String()))

	chart = replaceFirst(chart, pptValRe, fmt.Sprintf(
		`<c:val><c:numRef><c:f>Sheet1!$B$2:$B$%d</c:f>`+
			`<c:numCache><c:formatCode>General</c:formatCode><c:ptCount val="%d"/>%s`+
			`</c:numCache></c:numRef></c:val>`,
		n+1, n, numPts.String()))

	return hideZeroDataLabels(dropDataPointsBeyond(chart, n), vals)
}

// hideZeroDataLabels deletes the value label on any point worth nothing.
//
// A segment with no findings draws nothing, but its label is still placed at
// the angle the segment would have occupied - so an engagement with no Low
// findings printed a bare "0" sitting on the boundary between the two segments
// either side of it, reading as a count belonging to one of them.
//
// The per-point deletions go at the front of the series' own <c:dLbls>: the
// schema orders <c:dLbl> entries before the settings that follow, and a chart
// whose children are out of order is one PowerPoint offers to repair.
func hideZeroDataLabels(chart string, vals []int) string {
	var b strings.Builder
	for i, v := range vals {
		if v == 0 {
			fmt.Fprintf(&b, `<c:dLbl><c:idx val="%d"/><c:delete val="1"/></c:dLbl>`, i)
		}
	}
	if b.Len() == 0 {
		return chart
	}
	const open = "<c:dLbls>"
	i := strings.Index(chart, open)
	if i < 0 {
		return chart
	}
	i += len(open)
	return chart[:i] + b.String() + chart[i:]
}

// dropDataPointsBeyond removes per-point formatting for points the series no
// longer has. The bar chart's template carries <c:dPt> entries up to index 6
// from an engagement with more areas than most; left in place against a shorter
// series, PowerPoint reports the chart as corrupt.
func dropDataPointsBeyond(chart string, n int) string {
	return pptDPtRe.ReplaceAllStringFunc(chart, func(dpt string) string {
		m := pptDPtIdxRe.FindStringSubmatch(dpt)
		if len(m) < 2 {
			return dpt
		}
		idx, err := parseInt(m[1])
		if err != nil || idx < n {
			return dpt
		}
		return ""
	})
}

// replaceFirst swaps only the first match, leaving any later one alone. Both
// charts hold a single series, so a second match would be a template change
// worth noticing rather than one to quietly rewrite as well.
func replaceFirst(s string, re *regexp.Regexp, with string) string {
	done := false
	return re.ReplaceAllStringFunc(s, func(m string) string {
		if done {
			return m
		}
		done = true
		return with
	})
}

// axisTextRe is the category axis' default run properties, whose sz is in
// hundredths of a point.
var axisTextRe = regexp.MustCompile(`(<c:catAx>(?s:.*?)<c:txPr>(?s:.*?)<a:defRPr )sz="\d+"`)

// fitCategoryAxis shrinks the bar chart's category labels when there are more
// bars than the template was drawn for.
//
// The template has four categories in a narrow frame beside the doughnut, at
// 14pt. An engagement covering more areas than that runs the names into each
// other - "Internal External Web AppsConfig ReviewWireless" - which is unreadable
// and, worse, looks like one long label rather than five short ones.
func fitCategoryAxis(chart string, categories int) string {
	if categories <= 4 {
		return chart
	}
	// 14pt fits four; scale down with the count and stop at 9pt, below which
	// the labels are too small to read at the back of a meeting room.
	sz := 1400 * 4 / categories
	if sz < 900 {
		sz = 900
	}
	return axisTextRe.ReplaceAllString(chart, fmt.Sprintf(`${1}sz="%d"`, sz))
}

var gapWidthRe = regexp.MustCompile(`<c:gapWidth val="\d+"/>`)

// templateBarGap and templateBarCount are the gap the template sets and the
// number of bars it was set for. A bar chart divides its width evenly between
// its categories and then leaves gapWidth percent of a bar's width as space, so
// the same gap with fewer bars makes each one wider.
const (
	templateBarGap   = 37
	templateBarCount = 4
	maxBarGap        = 500
)

// fitBarGap keeps a bar the width it is in the template however many areas the
// engagement covers.
//
// The template's gap is tuned for four bars. An engagement scoped to one area
// drew that single bar across the entire plot - a block of colour four inches
// wide labelled "4", which reads as a chart that failed to render rather than
// as one finding count. The gap is widened to hold the bar at the width four of
// them would have had.
func fitBarGap(chart string, categories int) string {
	if categories <= 0 || categories >= templateBarCount {
		return chart
	}
	// bar = plot / (categories * (1 + gap/100)), solved for the template's own
	// bar width at templateBarCount categories.
	gap := (100+templateBarGap)*templateBarCount/categories - 100
	if gap > maxBarGap {
		gap = maxBarGap
	}
	return gapWidthRe.ReplaceAllString(chart, fmt.Sprintf(`<c:gapWidth val="%d"/>`, gap))
}

// fitCategoryLabels names the bars so they fit the frame the template gives
// them.
//
// The chart sits in a narrow column beside the doughnut, wide enough for four
// names. Past that the words collide - "Web AppsConfig ReviewWireless" - and
// neither shrinking the type nor angling it helps: at five bars each name has
// about four fifths of an inch, and "Configuration Files Review" does not fit in
// four fifths of an inch at any legible size.
//
// So a crowded chart is labelled with the area codes instead. They are the same
// codes the issue slides are titled with ("IPT Issues - Critical Level") and the
// same ones the vulnerability ids carry, so the deck is not introducing a new
// vocabulary to save space - it is using the one it already speaks.
func fitCategoryLabels(config ReportConfig, cats []string) []string {
	if len(cats) <= 4 {
		return cats
	}
	var codes []string
	for _, code := range selectedAreaCodes(config) {
		if area, ok := areaByCode(code); ok {
			codes = append(codes, area.Code)
		}
	}
	if len(codes) != len(cats) {
		return cats
	}
	return codes
}
