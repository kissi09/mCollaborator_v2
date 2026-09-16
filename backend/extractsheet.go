package main

// Findings out of a scanner's spreadsheet.
//
// The document passes in extract.go read a written report: they look for
// headings, finding tables and labels, and they work out an area from what the
// document says around each finding. A MUNIT export says none of that. It is a
// table with one row per vulnerability instance and a fixed set of columns, and
// the area it came from is not in it at all - which is why the area is chosen
// at upload and every row takes it.
//
// Two things in the shape of the export decide the rest of this file.
//
// A vulnerability found on six hosts is six rows, identical but for the host.
// Six findings is not what the report wants - it wants one finding listing six
// affected hosts - so rows are grouped by title and their hosts collected.
//
// And the port is not in a column of its own. It is the number in Technology,
// beside the protocol: "SSH,22". The report's Affected Host reads host and port
// together - "10.228.11.87:22" - so the two are joined here, at the only point
// that knows they belong to the same row.

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// sheetColumns maps a header cell to the field it fills. The keys are header
// names with everything but letters and digits removed, so "Vulnerability_Title",
// "Vulnerability Title" and "vulnerability-title" are one key.
//
// The aliases are deliberately wider than MUNIT's own headers: the same export
// comes out of different versions with the column renamed, and a header this
// does not recognise is a column silently dropped.
var sheetColumns = map[string]string{
	"vulnerabilitytitle":       "title",
	"vulnerabilityname":        "title",
	"vulnerability":            "title",
	"title":                    "title",
	"name":                     "title",
	"vulnerabilitydescription": "description",
	"description":              "description",
	"details":                  "description",
	"recommendation":           "remediation",
	"recommendations":          "remediation",
	"remediation":              "remediation",
	"solution":                 "remediation",
	"fix":                      "remediation",
	"affectedsystem":           "affected",
	"affectedsystems":          "affected",
	"affectedhost":             "affected",
	"affectedhosts":            "affected",
	"hostname":                 "affected",
	"systemip":                 "ip",
	"ip":                       "ip",
	"ipaddress":                "ip",
	"host":                     "ip",
	"technology":               "technology",
	"service":                  "technology",
	"protocol":                 "technology",
	"port":                     "port",
	"vulnerabilityrating":      "severity",
	"severity":                 "severity",
	"rating":                   "severity",
	"risk":                     "severity",
	"risklevel":                "severity",
	"impacttype":               "impact",
	"impact":                   "impact",
	"cvss":                     "cvss",
	"cvssscore":                "cvss",
	"cvssbasescore":            "cvss",
	"basescore":                "cvss",
	"cve":                      "cve",
	"cveid":                    "cve",
	"poc":                      "poc",
	"proofofconcept":           "poc",
	"evidence":                 "poc",
}

var sheetHeaderStripRe = regexp.MustCompile(`[^a-z0-9]+`)

// normalizeSheetHeader reduces a header cell to its key.
func normalizeSheetHeader(s string) string {
	return sheetHeaderStripRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "")
}

// sheetColumnMap resolves the header row to field positions. The first column
// carrying a field wins, so a sheet with both "Host" and "System_IP" takes the
// one the export puts first rather than whichever the map happens to visit.
func sheetColumnMap(header []string) map[string]int {
	at := map[string]int{}
	for i, cell := range header {
		field, ok := sheetColumns[normalizeSheetHeader(cell)]
		if !ok {
			continue
		}
		if _, seen := at[field]; !seen {
			at[field] = i
		}
	}
	return at
}

// ExtractFromSheet reads a scanner export into candidate findings, all of them
// in the area the tester chose for the file.
//
// It returns an error only when the sheet is not one: a table with no title
// column is not a findings export, and reading it would produce a page of
// findings named after whatever its first column happens to hold.
func ExtractFromSheet(rows [][]string, areaCode string) ([]ExtractedFinding, []string, error) {
	if len(rows) < 2 {
		return nil, nil, fmt.Errorf("this sheet has a header row and nothing under it")
	}
	at := sheetColumnMap(rows[0])
	if _, ok := at["title"]; !ok {
		return nil, nil, fmt.Errorf("this sheet has no vulnerability title column - the headers read %s. "+
			"A findings export needs a column named Vulnerability_Title (or Title)", quoteHeaders(rows[0]))
	}

	area, ok := areaByCode(areaCode)
	if !ok {
		return nil, nil, fmt.Errorf("%q is not an assessment area", areaCode)
	}

	var (
		order   []string
		byTitle = map[string]*ExtractedFinding{}
		hosts   = map[string][]string{}
		seen    = map[string]map[string]bool{}
		skipped int
	)

	for _, row := range rows[1:] {
		cell := func(field string) string {
			i, ok := at[field]
			if !ok || i >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[i])
		}

		title := collapseSpaces(cell("title"))
		if title == "" {
			skipped++
			continue
		}
		key := strings.ToLower(title)

		f, ok := byTitle[key]
		if !ok {
			f = &ExtractedFinding{
				Title:      title,
				Category:   area.Code,
				Confidence: ConfChosen,
				Reason:     "From the assessment area you chose for this sheet (" + area.Code + ")",
			}
			byTitle[key] = f
			seen[key] = map[string]bool{}
			order = append(order, key)
		}

		// The first row of a group sets the finding's text; later rows only
		// fill what it left blank. Rows of one vulnerability differ in the host
		// they were found on, and where they differ in anything else it is
		// because one of them was filled in and the others were not.
		fillEmpty(&f.Description, cell("description"))
		fillEmpty(&f.Impact, cell("impact"))
		fillEmpty(&f.Remediation, cell("remediation"))
		fillEmpty(&f.POC, cell("poc"))
		fillEmpty(&f.CVE, cell("cve"))
		if f.Severity == "" {
			f.Severity = normalizeSeverity(cell("severity"))
		}
		if f.CVSSScore == 0 {
			if v, err := strconv.ParseFloat(cell("cvss"), 64); err == nil && v > 0 && v <= 10 {
				f.CVSSScore = v
			}
		}

		if host := sheetHost(cell("affected"), cell("ip"), cell("port"), cell("technology")); host != "" && !seen[key][host] {
			seen[key][host] = true
			hosts[key] = append(hosts[key], host)
		}
	}

	findings := make([]ExtractedFinding, 0, len(order))
	merged := 0
	for _, key := range order {
		f := byTitle[key]
		f.AffectedSystem = strings.Join(hosts[key], ", ")
		if len(hosts[key]) > 1 {
			merged++
		}
		findings = append(findings, *f)
	}

	// Most severe first, so the review board reads the way the report will.
	sort.SliceStable(findings, func(i, j int) bool {
		return severityRank(findings[i].Severity) < severityRank(findings[j].Severity)
	})

	return findings, sheetNotes(rows, at, findings, merged, skipped, area), nil
}

// fill writes a value into a field that has none. It is the sheet's own copy of
// the merge rule the document passes use.
func fillEmpty(dst *string, v string) {
	if *dst == "" && strings.TrimSpace(v) != "" {
		*dst = strings.TrimSpace(v)
	}
}

var (
	// sheetPortRe finds the numbers in a Technology cell. "SSH,22" gives 22;
	// "https (443/tcp)" gives 443 and 0 - which is why the last plausible port
	// wins rather than the first number present.
	sheetPortRe = regexp.MustCompile(`\d+`)
	spacesRe    = regexp.MustCompile(`\s+`)
)

// sheetHost builds one Affected Host entry from a row: the address, a colon,
// the port, and nothing else. "10.228.11.87:22".
//
// The address is System_IP. Affected_System is the fallback rather than the
// preference - it is empty in every export seen so far, and where an export
// does fill it the report still wants the address, plainly, not a name with the
// address after it in brackets.
//
// A row with no port contributes the bare address rather than one with an empty
// port stuck to it: "10.0.0.5:" is not a shorter way of saying the port is
// unknown, it is a typo.
func sheetHost(affected, ip, port, technology string) string {
	host := ip
	if host == "" {
		host = affected
	}
	host = collapseSpaces(host)
	if host == "" {
		return ""
	}
	if p := sheetPort(port, technology); p != "" {
		// An entry that already carries a port is left as it is: an export that
		// writes "10.0.0.5:443" in the host column and 443 in Technology would
		// otherwise come out as "10.0.0.5:443:443".
		if !strings.Contains(host, ":") {
			return host + ":" + p
		}
	}
	return host
}

// sheetPort is the port for a row: the Port column when the export has one, and
// otherwise the number sitting beside the protocol in Technology.
func sheetPort(port, technology string) string {
	if p, err := strconv.Atoi(strings.TrimSpace(port)); err == nil && p > 0 && p <= 65535 {
		return strconv.Itoa(p)
	}
	// The last number that could be a port. "TLS 1.2,443" holds 1, 2 and 443,
	// and only the last of them is the port.
	out := ""
	for _, m := range sheetPortRe.FindAllString(technology, -1) {
		if p, err := strconv.Atoi(m); err == nil && p > 0 && p <= 65535 {
			out = strconv.Itoa(p)
		}
	}
	return out
}

// sheetNotes says what the import did that the findings themselves do not show:
// what was dropped, what was merged, and which columns went unread.
func sheetNotes(rows [][]string, at map[string]int, findings []ExtractedFinding, merged, skipped int, area areaDef) []string {
	var notes []string

	notes = append(notes, fmt.Sprintf("%d row%s read into %d finding%s, all filed under %s (%s).",
		len(rows)-1, plural(len(rows)-1, "", "s"), len(findings), plural(len(findings), "", "s"),
		area.Code, area.Label))

	if merged > 0 {
		notes = append(notes, fmt.Sprintf("%d finding%s appeared on more than one host; their rows were "+
			"merged and every host listed under Affected Host.", merged, plural(merged, "", "s")))
	}
	if skipped > 0 {
		notes = append(notes, fmt.Sprintf("%d row%s had no vulnerability title and %s skipped.",
			skipped, plural(skipped, "", "s"), plural(skipped, "was", "were")))
	}
	if _, ok := at["technology"]; !ok {
		if _, hasPort := at["port"]; !hasPort {
			notes = append(notes, "This sheet has no Technology or Port column, so the affected hosts "+
				"carry no port.")
		}
	}
	if missing := missingSeverities(findings); missing > 0 {
		notes = append(notes, fmt.Sprintf("%d finding%s came through with no severity - set one on the "+
			"review board before importing.", missing, plural(missing, "", "s")))
	}

	// The columns nothing read. A tester who expected a field to arrive can see
	// immediately that its header is not one this recognises.
	var unread []string
	used := map[int]bool{}
	for _, i := range at {
		used[i] = true
	}
	for i, h := range rows[0] {
		if h = strings.TrimSpace(h); h != "" && !used[i] {
			unread = append(unread, h)
		}
	}
	if len(unread) > 0 {
		notes = append(notes, "Columns not read: "+strings.Join(unread, ", ")+".")
	}
	return notes
}

func missingSeverities(findings []ExtractedFinding) int {
	n := 0
	for _, f := range findings {
		if f.Severity == "" {
			n++
		}
	}
	return n
}

func collapseSpaces(s string) string {
	return strings.TrimSpace(spacesRe.ReplaceAllString(s, " "))
}

// quoteHeaders renders a header row for an error message, short enough to read.
func quoteHeaders(header []string) string {
	var named []string
	for _, h := range header {
		if h = strings.TrimSpace(h); h != "" {
			named = append(named, h)
		}
		if len(named) == 8 {
			named = append(named, "...")
			break
		}
	}
	if len(named) == 0 {
		return "(blank)"
	}
	return strings.Join(named, ", ")
}
