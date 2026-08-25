package main

// Test-type tables.
//
// Every chapter 3 area section opens with a "Test Type / Status" table: a
// checklist of named tests, grouped by phase, each marked Pass, Issues or N/A.
// The template ships those statuses filled in from whatever engagement it was
// last used for, and nothing rewrote them - so a delivered report asserted
// results for this client that were really someone else's. A report claiming
// "Audit SSL Services: Issues" about a network nobody audited is worse than a
// report that says nothing.
//
// The statuses are now derived from the findings. A group whose subject matter
// a finding names is marked Issues across its rows; every other row in a tested
// area reads Pass. Areas that were not in scope keep no table at all - that is
// renderAreaSections' doing, and it already worked.
//
// The checklist itself is never copied into Go. Both the test names and the
// group headings are read out of the template's own table at render time, so
// editing the checklist in the DOCX needs no code change and there is no second
// copy of the list here to fall out of date.

import (
	"strings"
)

// Statuses the template's checklist uses.
const (
	statusPass   = "Pass"
	statusIssues = "Issues"
)

// testTypeHeader is the first cell of the checklist's header row, and how the
// table is told apart from the other tables in an area's block.
const testTypeHeader = "test type"

// testRow is one checklist row: a named test and the cell its status prints in.
type testRow struct {
	name  string
	group int // index into the table's groups
	cell  span
	row   span
}

// testGroup is one shaded phase heading - "Authentication Testing",
// "Privilege Escalation" - and covers the rows that follow it.
type testGroup struct {
	name string
}

// parsedTestTable is a checklist table taken apart far enough to rewrite its
// Status column.
type parsedTestTable struct {
	groups []testGroup
	rows   []testRow
}

// isTestTypeTable reports whether a table fragment is a Test Type checklist.
func isTestTypeTable(tbl string) bool {
	rows := tableRows(tbl)
	if len(rows) < 2 {
		return false
	}
	cells := rowCells(tbl[rows[0].Start:rows[0].End])
	if len(cells) < 2 {
		return false
	}
	first := tbl[rows[0].Start:rows[0].End]
	return strings.EqualFold(
		strings.TrimSpace(elemText(first[cells[0].Start:cells[0].End])), testTypeHeader)
}

// parseTestTable splits a checklist into its groups and rows.
//
// The three row kinds are told apart by their status cell: the header row names
// the columns, a group heading leaves the status cell empty, and a test row
// fills it. That is the template's own structure rather than a guess about
// shading, so a re-styled table still parses.
func parseTestTable(tbl string) parsedTestTable {
	var out parsedTestTable
	group := -1
	for i, r := range tableRows(tbl) {
		row := tbl[r.Start:r.End]
		cells := rowCells(row)
		if len(cells) < 2 {
			continue
		}
		name := strings.TrimSpace(elemText(row[cells[0].Start:cells[0].End]))
		status := strings.TrimSpace(elemText(row[cells[1].Start:cells[1].End]))
		if i == 0 && strings.EqualFold(name, testTypeHeader) {
			continue
		}
		if name == "" {
			continue
		}
		if status == "" {
			out.groups = append(out.groups, testGroup{name: name})
			group = len(out.groups) - 1
			continue
		}
		out.rows = append(out.rows, testRow{
			name:  name,
			group: group,
			cell:  span{r.Start + cells[1].Start, r.Start + cells[1].End},
			row:   r,
		})
	}
	return out
}

// statusHex is the colour a checklist status is printed in: green for a test
// that passed, red for one the engagement raised a finding against. Only the
// word is coloured - the cell keeps the template's own fill, the same way the
// vulnerability register colours a criticality.
func statusHex(status string) string {
	if status == statusIssues {
		return "C00000"
	}
	return "28A745"
}

// setTestStatus writes status into a checklist row's status cell, keeping the
// cell's own paragraph and run formatting and colouring the word itself.
func setTestStatus(cell, status string) string {
	paras := childElems(cell, "w:p")
	if len(paras) == 0 {
		return cell
	}
	p := paras[0]
	return cell[:p.Start] + setParaTextColored(cell[p.Start:p.End], status, statusHex(status)) + cell[p.End:]
}

// ---------------------------------------------------------------------------
// matching findings to checklist rows
// ---------------------------------------------------------------------------

// checklistFiller are the words the checklist uses to phrase a test rather than
// to name one. "Testing for SQL Injection" and "Audit SMTP Services" are about
// SQL injection and SMTP; the rest is scaffolding, and matching on it would
// make every row in a table look like every other.
var checklistFiller = map[string]bool{
	"a": true, "all": true, "an": true, "analyse": true, "analysis": true,
	"analyze": true, "and": true, "any": true, "are": true, "as": true,
	"assess": true, "attempt": true, "audit": true, "be": true, "by": true,
	"check": true, "conduct": true, "ensure": true, "evaluate": true,
	"for": true, "from": true, "identify": true, "if": true, "in": true,
	"including": true, "is": true, "it": true, "not": true, "of": true,
	"on": true, "or": true, "other": true, "others": true, "perform": true,
	"review": true, "such": true, "test": true, "testing": true, "tests": true,
	"that": true, "the": true, "their": true, "this": true, "to": true,
	"verify": true, "via": true, "was": true, "were": true, "with": true,
}

// checklistSynonyms expands the shorthand a tester writes in a finding title
// into the words the checklist spells out. Both sides are expanded, so the
// template's own "SSL/TSL" typo lines up with a finding that says TLS.
var checklistSynonyms = map[string]string{
	"xss":     "cross site scripting",
	"csrf":    "cross site request forgery",
	"xsrf":    "cross site request forgery",
	"sqli":    "sql injection",
	"rce":     "remote code execution",
	"lfi":     "local file inclusion",
	"rfi":     "remote file inclusion",
	"hsts":    "http strict transport security",
	"cors":    "cross origin resource sharing",
	"mfa":     "multi factor authentication",
	"2fa":     "multi factor authentication",
	"tsl":     "tls",
	"ssl":     "tls",
	"privesc": "privilege escalation",
	"creds":   "credential",
	"cert":    "certificate",
	"config":  "configuration",
	"vuln":    "vulnerability",
	"ad":      "active directory",
	"dos":     "denial of service",
}

// singular folds the obvious English plural back to its stem, so a checklist
// row reading "Identify Rogue Access Points" meets a finding that says "rogue
// access point". Only the endings that are safe without a dictionary are
// touched; a word it cannot fold is left exactly as it is.
func singular(word string) string {
	switch {
	case len(word) > 4 && strings.HasSuffix(word, "ies"):
		return word[:len(word)-3] + "y"
	case len(word) > 4 && (strings.HasSuffix(word, "sses") ||
		strings.HasSuffix(word, "shes") || strings.HasSuffix(word, "ches") ||
		strings.HasSuffix(word, "xes") || strings.HasSuffix(word, "zes")):
		return word[:len(word)-2]
	case len(word) > 3 && strings.HasSuffix(word, "s") && !strings.HasSuffix(word, "ss"):
		return word[:len(word)-1]
	}
	return word
}

// checklistTokens reduces a phrase to the words worth matching on: lowercased,
// punctuation dropped, shorthand expanded, plurals folded, filler removed. The
// result is de-duplicated, so a phrase contributes each of its words once.
func checklistTokens(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		expanded := raw
		if syn, ok := checklistSynonyms[raw]; ok {
			expanded = syn
		}
		for _, tok := range strings.Fields(expanded) {
			tok = singular(tok)
			if len(tok) < 2 || checklistFiller[tok] || seen[tok] {
				continue
			}
			seen[tok] = true
			out = append(out, tok)
		}
	}
	return out
}

// testVocabulary weighs each word by how many rows of the same table use it.
//
// Counting matched words alone does not work: "injection" is half the words in
// "Testing for SQL Injection", so a SQL injection finding would also condemn the
// LDAP, XPath, XML, ORM and NoSQL injection rows sitting beside it. But
// "injection" appears in eleven rows of that table and "xpath" in one, so what
// separates those rows is exactly the word that recurs least. Weighing each word
// by 1/(rows using it) says so, and it is measured off the template every time -
// re-word the checklist and the weights follow.
func testVocabulary(p parsedTestTable) map[string]float64 {
	rows := map[string]int{}
	for _, r := range p.rows {
		// checklistTokens de-duplicates within a phrase, so this counts rows
		// rather than occurrences.
		for _, tok := range checklistTokens(r.name) {
			rows[tok]++
		}
	}
	weight := make(map[string]float64, len(rows))
	for tok, n := range rows {
		weight[tok] = 1 / float64(n)
	}
	return weight
}

// checklistMatchRatio is how much of a row's distinctive vocabulary a finding
// has to account for before that row is marked Issues.
//
// Tuned for precision over recall, because the two failures are not equal: a
// row wrongly marked Issues invents a vulnerability in the client's report,
// while a row wrongly left Pass only leaves the default in place. Where a
// finding matches nothing at all the export says so rather than letting the
// checklist quietly contradict the vulnerability register.
const checklistMatchRatio = 0.6

// checklistMatchOwnWords is the second way in, for rows phrased as a sentence.
// "Test for Weak WPA2-PSK Passphrases (Dictionary/Brute-force)" spends most of
// its words on how the test is run, so a finding naming the thing itself never
// reaches the ratio. Two words that belong to this row and no other is a strong
// enough signal on its own - one is not, or "access" alone would match half a
// wireless checklist.
const checklistMatchOwnWords = 2

// matchesTest reports whether a finding is about the test a row names.
func matchesTest(rowTokens []string, weight map[string]float64, findingTokens map[string]bool) bool {
	var total, hit float64
	ownWords := 0
	for _, tok := range rowTokens {
		w := weight[tok]
		total += w
		if !findingTokens[tok] {
			continue
		}
		hit += w
		if w == 1 {
			ownWords++
		}
	}
	if total == 0 {
		return false
	}
	return hit/total >= checklistMatchRatio || ownWords >= checklistMatchOwnWords
}

// findingHaystack is everything a finding says about itself, as a token set.
func findingHaystack(f ReportFinding) map[string]bool {
	set := map[string]bool{}
	for _, tok := range checklistTokens(
		f.Title + " " + f.RecommendationHeader + " " + f.Description + " " + f.Recommendation) {
		set[tok] = true
	}
	return set
}

// ---------------------------------------------------------------------------
// rendering
// ---------------------------------------------------------------------------

// reportNotes collects what the renderer could not settle on its own. It is
// surfaced on the export response rather than only logged: the whole reason
// these tables are being rewritten is that nobody noticed the template asserting
// results, and a note nobody reads would repeat the mistake in a quieter way.
type reportNotes struct {
	// FindingsWithoutProof names findings the closure deck could not illustrate,
	// because they carry no evidence screenshot or because one could not be read.
	// They still appear in the issues tables; they just get no scenario slide.
	FindingsWithoutProof []string

	// UnmatchedFindings names findings whose area has a checklist that none of
	// its rows could be tied to. Those rows stay Pass, so the register reports a
	// vulnerability the checklist does not account for.
	UnmatchedFindings []string

	// LogoError says why an uploaded customer logo did not make it into the
	// document. The render carries on without it - one missing logo is not worth
	// losing a report over - but it is reported rather than logged and dropped.
	// A logo that silently fails to appear looks exactly like one nobody
	// uploaded, which sends whoever built the report hunting through the
	// template for a fault that is in their image file.
	LogoError string
}

// renderTestTypeTables rewrites the Status column of every checklist in one
// area's block: Issues on the rows the area's findings are about, Pass on the
// rest. Rows are written back to front so the offsets taken from the parse stay
// valid as the fragment changes length.
func renderTestTypeTables(block string, findings []numberedFinding, notes *reportNotes) string {
	for _, tb := range childElems(block, "w:tbl") {
		tbl := block[tb.Start:tb.End]
		if !isTestTypeTable(tbl) {
			continue
		}
		parsed := parseTestTable(tbl)
		if len(parsed.rows) == 0 {
			continue
		}
		weight := testVocabulary(parsed)
		rowTokens := make([][]string, len(parsed.rows))
		for i, r := range parsed.rows {
			rowTokens[i] = checklistTokens(r.name)
		}

		issues := make([]bool, len(parsed.rows))
		for _, f := range findings {
			hay := findingHaystack(f.ReportFinding)
			hit := false
			for i := range parsed.rows {
				if matchesTest(rowTokens[i], weight, hay) {
					issues[i] = true
					hit = true
				}
			}
			if !hit && notes != nil {
				notes.UnmatchedFindings = append(notes.UnmatchedFindings, strings.TrimSpace(f.Title))
			}
		}

		for i := len(parsed.rows) - 1; i >= 0; i-- {
			status := statusPass
			if issues[i] {
				status = statusIssues
			}
			c := parsed.rows[i].cell
			tbl = tbl[:c.Start] + setTestStatus(tbl[c.Start:c.End], status) + tbl[c.End:]
		}
		// Only the first checklist in a block is the area's own; rewriting the
		// fragment invalidates the remaining offsets, so stop here.
		return block[:tb.Start] + tbl + block[tb.End:]
	}
	return block
}
