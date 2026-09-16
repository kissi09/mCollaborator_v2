package main

// Spreadsheets, read as rows of text.
//
// A scanner's export is not a report: it is a table, one row per vulnerability
// instance, and the extractor's document passes have nothing to work with. This
// turns a .csv or an .xlsx into the same [][]string either way, so the sheet
// extractor never has to know which arrived.
//
// The .xlsx reader is written here rather than pulled in, because a workbook is
// a zip of XML and this package already reads two other kinds - the helpers in
// docxxml.go do the parsing. What a full library buys is styles, dates,
// formulas and merged cells, none of which a scanner export uses: every cell in
// one is a string or a number.
//
// Two things about the format are not optional, though, and both are handled:
// a cell's text usually lives in a shared string table rather than in the cell,
// and a row omits its empty cells entirely - so a row's values have to be
// placed by the column letter in each cell's reference, not by counting.

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	// sheetPartMaxBytes caps a single part read out of a workbook. The whole
	// upload is already capped; this stops one crafted part inside it from
	// expanding without limit.
	sheetPartMaxBytes = 64 << 20

	// sheetMaxRows and sheetMaxCols are what a scanner export plausibly holds.
	// A sheet past either is not refused - it is read to the limit and the
	// caller says so - because a truncated import a tester can see beats a
	// refusal they cannot act on.
	sheetMaxRows = 20000
	sheetMaxCols = 512
)

// readSheetRows reads a spreadsheet upload into rows of cells. The first row is
// the header, exactly as the file has it.
func readSheetRows(data []byte, ext string) ([][]string, error) {
	switch strings.ToLower(ext) {
	case ".csv":
		return readCSVRows(data)
	case ".xlsx":
		return readXLSXRows(data)
	}
	return nil, fmt.Errorf("%s is not a spreadsheet this can read", ext)
}

// ---------------------------------------------------------------------------
// csv
// ---------------------------------------------------------------------------

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// readCSVRows reads a delimited text export.
//
// Quoted cells run to several lines in every export of this kind - a
// vulnerability description is a paragraph - and the standard reader handles
// that. Two things it does not do by default are handled here: Excel writes a
// byte-order mark in front of the header, which would otherwise become part of
// the first column's name, and a machine with a European locale exports
// semicolons rather than commas.
func readCSVRows(data []byte) ([][]string, error) {
	data = bytes.TrimPrefix(data, utf8BOM)

	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = delimiterOf(data)
	// A row is as long as it is. Exports append columns over time, and a sheet
	// whose last row is short is a sheet with a trailing blank, not a file to
	// refuse.
	r.FieldsPerRecord = -1
	// A stray quote inside an unquoted cell is common in recommendation text
	// pasted out of a terminal. Refusing the whole file over one is worse than
	// reading it: nothing is saved until the findings are reviewed on the
	// board, so a mangled cell is seen before it reaches the report.
	r.LazyQuotes = true

	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("this CSV could not be read as a table: %v", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("this CSV is empty - it has no header row")
	}
	return rows, nil
}

// delimiterOf picks the separator from the header line: whichever candidate
// appears most often in it. The header is the safest line to count on, being
// the one line of a scanner export with no free text in it.
func delimiterOf(data []byte) rune {
	head := data
	if i := bytes.IndexByte(head, '\n'); i >= 0 {
		head = head[:i]
	}
	best, bestN := ',', bytes.Count(head, []byte{','})
	for _, c := range []byte{';', '\t', '|'} {
		if n := bytes.Count(head, []byte{c}); n > bestN {
			best, bestN = rune(c), n
		}
	}
	return best
}

// ---------------------------------------------------------------------------
// xlsx
// ---------------------------------------------------------------------------

// readXLSXRows reads the workbook's findings sheet.
//
// Not simply the first sheet part: a workbook's tab order lives in workbook.xml
// and need not match the numbering of the parts, so a file whose export tab was
// added after a summary tab would be read from the wrong one - and read
// perfectly well, just from the wrong table.
//
// And not simply the first tab either. A tester who keeps a cover sheet in
// front of the export is not doing anything strange, and reading it produces
// either a refusal naming that sheet's headings or, worse, a findings list made
// out of a summary. So the tabs are read in order and the first one that has a
// vulnerability title column wins. When none of them has, the first tab is
// returned so the error the caller raises quotes headings the tester will
// recognise.
func readXLSXRows(data []byte) ([][]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("this .xlsx could not be opened as a workbook (%v). If it was "+
			"renamed from .xls, open it in Excel and use File > Save As > Excel Workbook (.xlsx)", err)
	}

	parts := map[string]string{}
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "xl/") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(rc, sheetPartMaxBytes))
		rc.Close()
		if err == nil {
			parts[f.Name] = string(b)
		}
	}

	names := sheetPartsInTabOrder(parts)
	if len(names) == 0 {
		return nil, fmt.Errorf("this .xlsx has no worksheet in it")
	}

	shared := sharedStrings(parts["xl/sharedStrings.xml"])
	var first [][]string
	for _, name := range names {
		rows := sheetRows(parts[name], shared)
		if len(rows) == 0 {
			continue
		}
		if first == nil {
			first = rows
		}
		if _, ok := sheetColumnMap(rows[0])["title"]; ok {
			return rows, nil
		}
	}
	if first == nil {
		return nil, fmt.Errorf("every sheet in this workbook is empty")
	}
	return first, nil
}

var (
	xlsxSheetRIDRe = regexp.MustCompile(`<sheet\b[^>]*\br:id="([^"]+)"`)
	xlsxRelIDRe    = regexp.MustCompile(`\bId="([^"]+)"`)
	xlsxRelTgtRe   = regexp.MustCompile(`\bTarget="([^"]+)"`)
	xlsxSheetNumRe = regexp.MustCompile(`^xl/worksheets/sheet(\d+)\.xml$`)
)

// sheetPartsInTabOrder names the worksheet parts in the order the tabs appear
// along the bottom of the window, which is the order workbook.xml lists them in
// and not necessarily the order their parts are numbered.
//
// When the workbook or its relationships cannot be read, the parts are returned
// in numeric order instead: sheets read from a guess still beat refusing the
// file.
func sheetPartsInTabOrder(parts map[string]string) []string {
	rels := parts["xl/_rels/workbook.xml.rels"]
	target := map[string]string{}
	for _, s := range childElems(rels, "Relationship") {
		frag := rels[s.Start:s.End]
		id := xlsxRelIDRe.FindStringSubmatch(frag)
		tgt := xlsxRelTgtRe.FindStringSubmatch(frag)
		if len(id) != 2 || len(tgt) != 2 {
			continue
		}
		t := strings.TrimPrefix(tgt[1], "/xl/")
		t = strings.TrimPrefix(t, "xl/")
		target[id[1]] = "xl/" + strings.TrimPrefix(t, "/")
	}

	var out []string
	seen := map[string]bool{}
	for _, m := range xlsxSheetRIDRe.FindAllStringSubmatch(parts["xl/workbook.xml"], -1) {
		name, ok := target[m[1]]
		if !ok || parts[name] == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	if len(out) > 0 {
		return out
	}

	for name := range parts {
		if xlsxSheetNumRe.MatchString(name) {
			out = append(out, name)
		}
	}
	sort.Slice(out, func(i, j int) bool { return sheetPartNum(out[i]) < sheetPartNum(out[j]) })
	return out
}

func sheetPartNum(name string) int {
	m := xlsxSheetNumRe.FindStringSubmatch(name)
	if len(m) != 2 {
		return 1 << 30
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 1 << 30
	}
	return n
}

var (
	xlsxTextRe = regexp.MustCompile(`<t(?: [^>]*)?>([^<]*)</t>`)
	xlsxPhonRe = regexp.MustCompile(`(?s)<rPh\b.*?</rPh>`)
	xlsxCellRe = regexp.MustCompile(`\br="([A-Z]+)\d+"`)
	xlsxTypeRe = regexp.MustCompile(`\bt="([^"]+)"`)
	xlsxValRe  = regexp.MustCompile(`(?s)<v(?: [^>]*)?>(.*?)</v>`)
)

// sharedStrings reads the workbook's string table. A cell's text is stored
// there once however many cells hold it, which for a scanner export means
// almost every description is a table entry rather than a cell value.
func sharedStrings(part string) []string {
	if part == "" {
		return nil
	}
	var out []string
	for _, s := range childElems(part, "si") {
		out = append(out, xlsxText(part[s.Start:s.End]))
	}
	return out
}

// xlsxText is the text of a string-table entry or an inline cell. An entry
// split into several runs - because a word in it is bold - is several <t>
// elements that have to be joined back into one string. Phonetic runs are
// dropped first: they carry a reading of the text, not the text.
func xlsxText(frag string) string {
	frag = xlsxPhonRe.ReplaceAllString(frag, "")
	var b strings.Builder
	for _, m := range xlsxTextRe.FindAllStringSubmatch(frag, -1) {
		b.WriteString(xmlUnescape(m[1]))
	}
	return b.String()
}

// sheetRows turns a worksheet part into rows of cells.
//
// A row lists only the cells that hold something, so a value is placed by the
// column letters in its reference rather than by its position among them: a
// sheet whose second column is blank on one row would otherwise shift every
// later value on that row one column to the left, which reads as data rather
// than as an error.
func sheetRows(part string, shared []string) [][]string {
	var out [][]string
	for _, rs := range childElems(part, "row") {
		if len(out) >= sheetMaxRows {
			break
		}
		row := part[rs.Start:rs.End]
		var cells []string
		for _, cs := range childElems(row, "c") {
			frag := row[cs.Start:cs.End]
			col := len(cells)
			if m := xlsxCellRe.FindStringSubmatch(frag); len(m) == 2 {
				col = columnIndex(m[1])
			}
			if col < 0 || col >= sheetMaxCols {
				continue
			}
			for len(cells) <= col {
				cells = append(cells, "")
			}
			cells[col] = cellValue(frag, shared)
		}
		out = append(out, cells)
	}

	// Trailing blank rows are what a sheet has below its data, not rows.
	for len(out) > 0 && isBlankRow(out[len(out)-1]) {
		out = out[:len(out)-1]
	}
	return out
}

// cellValue resolves one cell to text, whichever of the format's several ways
// of holding it this one uses.
func cellValue(frag string, shared []string) string {
	kind := ""
	if m := xlsxTypeRe.FindStringSubmatch(frag); len(m) == 2 {
		kind = m[1]
	}
	if kind == "inlineStr" {
		return xlsxText(frag)
	}
	m := xlsxValRe.FindStringSubmatch(frag)
	if len(m) != 2 {
		return ""
	}
	raw := xmlUnescape(strings.TrimSpace(m[1]))
	switch kind {
	case "s":
		i, err := strconv.Atoi(raw)
		if err != nil || i < 0 || i >= len(shared) {
			return ""
		}
		return shared[i]
	case "b":
		if raw == "1" {
			return "TRUE"
		}
		return "FALSE"
	}
	return raw
}

// columnIndex turns a column's letters into its position: A is 0, Z is 25, AA
// is 26.
func columnIndex(letters string) int {
	n := 0
	for _, c := range letters {
		if c < 'A' || c > 'Z' {
			return -1
		}
		n = n*26 + int(c-'A') + 1
	}
	return n - 1
}

func isBlankRow(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}
