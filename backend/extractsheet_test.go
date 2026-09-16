package main

// The scanner-export import, against the shape a MUNIT export actually has.
//
// The sample this was written from is docs/vulnerability 38.csv, a real export
// and so not committed. TestExtractsTheRealMUNITExport reads it when it is
// there and skips when it is not; everything the format guarantees is pinned
// here against a fixture written to the same header row.

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// munitHeader is the export's header row, verbatim.
const munitHeader = "Attachments,Vulnerability_Title,Vulnerability_Description,Recommendation," +
	"Affected_System,System_IP,OS,Assignee,OWASP_Top_10_Category,Vulnerability_Rating,CVE,CVSS," +
	"Impact_Type,Technology,Vector,Actor,CIA_Damage,Risk_Value,Project_Key,Testers,Date_Started," +
	"Duration,Test_Type,Purchaser,Customer,Contact_Person,Technical_Contact,mUnit_ID," +
	"JIRA_Duplicate,JIRA_Duplicate_Status"

// munitRow writes one export row. Only the columns the importer reads are
// parameters; the rest are what the sample leaves in them.
func munitRow(title, description, recommendation, affected, ip, rating, cve, cvss, impact, technology string) string {
	q := func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
	return strings.Join([]string{
		"", q(title), q(description), q(recommendation), q(affected), q(ip), "Linux", "", "",
		rating, cve, cvss, q(impact), q(technology), "Internal network", "Unauthenticated user",
		"", "1.3370412", "", "", "", "", "", "", "", "", "", "", "No", "",
	}, ",")
}

func munitCSV(rows ...string) []byte {
	return []byte(munitHeader + "\n" + strings.Join(rows, "\n") + "\n")
}

func sheetFindings(t *testing.T, csv []byte, area string) ([]ExtractedFinding, []string) {
	t.Helper()
	rows, err := readSheetRows(csv, ".csv")
	if err != nil {
		t.Fatalf("readSheetRows: %v", err)
	}
	findings, notes, err := ExtractFromSheet(rows, area)
	if err != nil {
		t.Fatalf("ExtractFromSheet: %v", err)
	}
	return findings, notes
}

// TestSheetJoinsThePortToTheHost is the rule the whole feature was asked for:
// the port is not in a column of its own, it is the number beside the protocol
// in Technology, and Affected Host reads the two together.
func TestSheetJoinsThePortToTheHost(t *testing.T) {
	findings, _ := sheetFindings(t, munitCSV(
		munitRow("SSH Server CBC Mode Ciphers Enabled", "The SSH server supports CBC.",
			"Edit /etc/ssh/sshd_config.", "", "10.228.11.87", "Low", "", "3",
			"Internal operations impact", "SSH,22"),
	), "IPT")

	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(findings))
	}
	f := findings[0]
	if f.AffectedSystem != "10.228.11.87:22" {
		t.Errorf("affected host = %q, want %q", f.AffectedSystem, "10.228.11.87:22")
	}
	if f.Title != "SSH Server CBC Mode Ciphers Enabled" {
		t.Errorf("title = %q", f.Title)
	}
	if f.Description != "The SSH server supports CBC." {
		t.Errorf("description = %q", f.Description)
	}
	if f.Remediation != "Edit /etc/ssh/sshd_config." {
		t.Errorf("recommendation did not become the remediation: %q", f.Remediation)
	}
	if f.Impact != "Internal operations impact" {
		t.Errorf("impact = %q", f.Impact)
	}
	if f.Severity != "low" {
		t.Errorf("severity = %q, want low", f.Severity)
	}
	if f.CVSSScore != 3 {
		t.Errorf("cvss score = %v, want 3", f.CVSSScore)
	}
	if f.Category != "IPT" {
		t.Errorf("category = %q, want the area chosen at upload", f.Category)
	}
	if f.Confidence != ConfChosen {
		t.Errorf("confidence = %q, want %q", f.Confidence, ConfChosen)
	}
	// PoC is not in the export and is not required: a finding imported from a
	// scanner has no proof until someone attaches one.
	if f.POC != "" {
		t.Errorf("poc = %q, want empty", f.POC)
	}
}

// TestSheetMergesOneVulnerabilityAcrossHosts is the other half: a scanner
// repeats a vulnerability once per host, and the report wants one finding
// listing them all rather than one finding per host.
func TestSheetMergesOneVulnerabilityAcrossHosts(t *testing.T) {
	findings, notes := sheetFindings(t, munitCSV(
		munitRow("SSH Server CBC Mode Ciphers Enabled", "The SSH server supports CBC.",
			"Edit sshd_config.", "", "10.228.11.87", "Low", "", "3", "Internal operations impact", "SSH,22"),
		munitRow("SSH Server CBC Mode Ciphers Enabled", "", "", "", "10.228.11.90", "", "", "", "", "SSH,22"),
		munitRow("SSH Server CBC Mode Ciphers Enabled", "", "", "", "10.228.11.87", "", "", "", "", "SSH,22"),
		munitRow("Apache Tomcat Default Credentials", "Default manager credentials.",
			"Change them.", "", "10.228.11.90", "Critical", "CVE-2020-1938", "9.8",
			"Internal operations impact", "HTTPS,8443"),
	), "IPT")

	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2 - the three SSH rows are one vulnerability on two hosts", len(findings))
	}

	// Most severe first, so the board reads the way the report will.
	if findings[0].Title != "Apache Tomcat Default Credentials" {
		t.Errorf("findings are not most severe first: %q came top", findings[0].Title)
	}
	if got := findings[0].AffectedSystem; got != "10.228.11.90:8443" {
		t.Errorf("tomcat affected host = %q", got)
	}
	if got := findings[0].CVE; got != "CVE-2020-1938" {
		t.Errorf("cve = %q", got)
	}

	ssh := findings[1]
	// Two hosts, in the order the sheet listed them, and the repeat of the
	// first host does not print twice.
	if want := "10.228.11.87:22, 10.228.11.90:22"; ssh.AffectedSystem != want {
		t.Errorf("affected host = %q, want %q", ssh.AffectedSystem, want)
	}
	// The rows after the first are blank in every column but the host; the
	// finding keeps what the first row said rather than being emptied by them.
	if ssh.Description != "The SSH server supports CBC." {
		t.Errorf("a later row emptied the description: %q", ssh.Description)
	}
	if ssh.Severity != "low" {
		t.Errorf("a later row emptied the severity: %q", ssh.Severity)
	}

	if !strings.Contains(strings.Join(notes, " "), "merged") {
		t.Errorf("the notes do not say any rows were merged: %v", notes)
	}
}

// TestSheetHostWithoutAPort keeps a host with no port readable. "10.0.0.5:" is
// not a shorter way of saying the port is unknown.
func TestSheetHostWithoutAPort(t *testing.T) {
	findings, notes := sheetFindings(t, munitCSV(
		munitRow("Unsupported operating system", "Windows Server 2008 is out of support.",
			"Upgrade.", "", "10.228.11.99", "High", "", "", "Internal operations impact", "Windows"),
	), "EPT")

	if got := findings[0].AffectedSystem; got != "10.228.11.99" {
		t.Errorf("affected host = %q, want the bare host", got)
	}
	if findings[0].Category != "EPT" {
		t.Errorf("category = %q, want EPT", findings[0].Category)
	}
	_ = notes
}

// TestSheetAffectedHostIsTheAddress. The entry is the address and the port and
// nothing else, even where the export also names the system: the user asked for
// "10.228.11.87:22", not a hostname with the address after it.
func TestSheetAffectedHostIsTheAddress(t *testing.T) {
	findings, _ := sheetFindings(t, munitCSV(
		munitRow("Weak TLS ciphers", "TLS 1.0 is enabled.", "Disable it.",
			"portal.example.test", "10.228.11.87", "Medium", "", "5.3",
			"Internal operations impact", "TLS 1.2,443"),
	), "EPT")

	// The port is the last number in Technology, not the first - "TLS 1.2,443"
	// holds 1, 2 and 443.
	if got := findings[0].AffectedSystem; got != "10.228.11.87:443" {
		t.Errorf("affected host = %q, want %q", got, "10.228.11.87:443")
	}
	for _, c := range []string{"(", ")", "portal.example.test"} {
		if strings.Contains(findings[0].AffectedSystem, c) {
			t.Errorf("affected host carries %q: %q", c, findings[0].AffectedSystem)
		}
	}
}

// TestSheetFallsBackToTheNamedSystem. An export that names the system and
// leaves the address blank still has to produce a host, or the finding arrives
// with nothing under Affected Host at all.
func TestSheetFallsBackToTheNamedSystem(t *testing.T) {
	findings, _ := sheetFindings(t, munitCSV(
		munitRow("Weak TLS ciphers", "TLS 1.0 is enabled.", "Disable it.",
			"portal.example.test", "", "Medium", "", "5.3",
			"Internal operations impact", "HTTPS,443"),
	), "EPT")

	if got := findings[0].AffectedSystem; got != "portal.example.test:443" {
		t.Errorf("affected host = %q", got)
	}
}

// TestSheetWithoutATitleColumnIsRefused. A table with no vulnerability title is
// not a findings export, and reading it anyway names every finding after
// whatever the first column holds.
func TestSheetWithoutATitleColumnIsRefused(t *testing.T) {
	_, _, err := ExtractFromSheet([][]string{
		{"Host", "Port", "Notes"},
		{"10.0.0.1", "443", "something"},
	}, "IPT")
	if err == nil {
		t.Fatal("a sheet with no title column was accepted")
	}
	if !strings.Contains(err.Error(), "Host") {
		t.Errorf("the error does not quote the headers it found: %v", err)
	}
}

// TestSheetRefusesAnUnknownArea. The area is the whole point of the review
// board; a sheet filed under a code that is not an assessment area would print
// in no section of the report.
func TestSheetRefusesAnUnknownArea(t *testing.T) {
	rows, err := readSheetRows(munitCSV(munitRow("A finding", "d", "r", "", "10.0.0.1", "Low", "", "", "", "SSH,22")), ".csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ExtractFromSheet(rows, "NOPE"); err == nil {
		t.Error("a sheet was filed under an area that does not exist")
	}
}

// TestSheetNotesNameTheUnreadColumns. A tester who expected a column to arrive
// can see that its header is not one the importer knows.
func TestSheetNotesNameTheUnreadColumns(t *testing.T) {
	_, notes := sheetFindings(t, munitCSV(
		munitRow("A finding", "d", "r", "", "10.0.0.1", "Low", "", "", "i", "SSH,22"),
	), "IPT")
	joined := strings.Join(notes, " ")
	for _, want := range []string{"Columns not read", "OS", "Actor", "IPT"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the notes do not mention %q: %v", want, notes)
		}
	}
}

// ---------------------------------------------------------------------------
// the readers
// ---------------------------------------------------------------------------

// TestCSVReadsMultiLineCells. Every description in the sample runs to several
// lines inside one quoted cell.
func TestCSVReadsMultiLineCells(t *testing.T) {
	csv := []byte("Title,Description\n\"One\",\"line one\nline two\n\nline four\"\n")
	rows, err := readSheetRows(csv, ".csv")
	if err != nil {
		t.Fatalf("readSheetRows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 - the newlines inside the quoted cell broke the row up", len(rows))
	}
	if !strings.Contains(rows[1][1], "line four") {
		t.Errorf("the cell lost its later lines: %q", rows[1][1])
	}
}

// TestCSVReadsASemicolonExport. Excel on a European locale writes semicolons,
// and the whole file reads as one column without this.
func TestCSVReadsASemicolonExport(t *testing.T) {
	rows, err := readSheetRows([]byte("Vulnerability_Title;System_IP;Technology\nA finding;10.0.0.1;SSH,22\n"), ".csv")
	if err != nil {
		t.Fatalf("readSheetRows: %v", err)
	}
	if len(rows[0]) != 3 {
		t.Fatalf("the header read as %d columns, want 3: %q", len(rows[0]), rows[0])
	}
	// The port still comes out of a cell that itself contains a comma.
	findings, _, err := ExtractFromSheet(rows, "IPT")
	if err != nil {
		t.Fatal(err)
	}
	if got := findings[0].AffectedSystem; got != "10.0.0.1:22" {
		t.Errorf("affected host = %q", got)
	}
}

// TestCSVStripsTheByteOrderMark. Excel writes one in front of the header, and
// it otherwise becomes part of the first column's name.
func TestCSVStripsTheByteOrderMark(t *testing.T) {
	csv := append([]byte{0xEF, 0xBB, 0xBF}, []byte("Vulnerability_Title,System_IP\nA finding,10.0.0.1\n")...)
	rows, err := readSheetRows(csv, ".csv")
	if err != nil {
		t.Fatalf("readSheetRows: %v", err)
	}
	if _, _, err := ExtractFromSheet(rows, "IPT"); err != nil {
		t.Errorf("the byte-order mark hid the title column: %v", err)
	}
}

// buildXLSX writes a minimal workbook: a shared string table, one sheet, and a
// row whose second cell is missing entirely - which is how a real workbook
// stores a blank cell, and the trap the column-letter placement exists for.
func buildXLSX(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}

	add("[Content_Types].xml", `<?xml version="1.0"?><Types/>`)
	// The cover sheet first in tab order and first by part number - which is
	// what Excel writes when a sheet is inserted in front of another - so the
	// findings tab is found by what is in it, not by where it sits.
	add("xl/workbook.xml", `<?xml version="1.0"?><workbook><sheets>`+
		`<sheet name="Summary" sheetId="1" r:id="rId1"/>`+
		`<sheet name="Findings" sheetId="2" r:id="rId2"/>`+
		`</sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<?xml version="1.0"?><Relationships>`+
		`<Relationship Id="rId1" Target="worksheets/sheet1.xml"/>`+
		`<Relationship Id="rId2" Target="worksheets/sheet2.xml"/>`+
		`</Relationships>`)
	add("xl/worksheets/sheet1.xml", `<worksheet><sheetData><row r="1">`+
		`<c r="A1" t="inlineStr"><is><t>Summary tab - not the findings</t></is></c>`+
		`</row></sheetData></worksheet>`)

	// The string table, with one entry split into two runs the way a workbook
	// stores text that has a bold word in it.
	add("xl/sharedStrings.xml", `<?xml version="1.0"?><sst>`+
		`<si><t>Vulnerability_Title</t></si>`+
		`<si><t>System_IP</t></si>`+
		`<si><t>Technology</t></si>`+
		`<si><t>Vulnerability_Rating</t></si>`+
		`<si><r><t>SSH Server CBC </t></r><r><t>Mode Ciphers &amp; more</t></r></si>`+
		`<si><t>10.228.11.87</t></si>`+
		`<si><t>SSH,22</t></si>`+
		`<si><t>Low</t></si>`+
		`</sst>`)
	add("xl/worksheets/sheet2.xml", `<worksheet><sheetData>`+
		`<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c>`+
		`<c r="C1" t="s"><v>2</v></c><c r="D1" t="s"><v>3</v></c></row>`+
		// C2 is absent: the row jumps B2 -> D2, which without column-letter
		// placement shifts the rating left into the Technology column.
		`<row r="2"><c r="A2" t="s"><v>4</v></c><c r="B2" t="s"><v>5</v></c>`+
		`<c r="D2" t="s"><v>7</v></c></row>`+
		`<row r="3"><c r="A3" t="s"><v>4</v></c><c r="B3" t="s"><v>5</v></c>`+
		`<c r="C3" t="s"><v>6</v></c><c r="D3" t="s"><v>7</v></c></row>`+
		`<row r="4"/>`+
		`</sheetData></worksheet>`)

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestXLSXReadsTheWorkbooksFirstTab pins the three things about the format that
// a sheet reader has to get right: the tab order is not the part numbering, the
// text is in a shared table rather than in the cells, and a blank cell is
// missing rather than empty.
func TestXLSXReadsTheWorkbooksFirstTab(t *testing.T) {
	rows, err := readSheetRows(buildXLSX(t), ".xlsx")
	if err != nil {
		t.Fatalf("readSheetRows: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 - the trailing blank row is not a row", len(rows))
	}
	if rows[0][0] != "Vulnerability_Title" {
		t.Fatalf("read the wrong sheet: first header is %q", rows[0][0])
	}
	// The summary tab is first in tab order and lowest by part number, so the
	// findings tab is found by having a title column rather than by position.
	// Excel renumbers the parts when a sheet is inserted in front of another,
	// which is how a cover sheet ends up as both the first tab and sheet1.xml.
	if strings.Contains(strings.Join(rows[0], " "), "Summary tab") {
		t.Fatal("read the cover sheet rather than the findings tab")
	}
	// The split runs come back as one string, ampersand and all.
	if want := "SSH Server CBC Mode Ciphers & more"; rows[1][0] != want {
		t.Errorf("row 1 title = %q, want %q", rows[1][0], want)
	}
	// The missing cell holds its place, so the rating is still in column D.
	if len(rows[1]) != 4 || rows[1][2] != "" || rows[1][3] != "Low" {
		t.Errorf("the missing cell shifted the row: %q", rows[1])
	}

	findings, _, err := ExtractFromSheet(rows, "IPT")
	if err != nil {
		t.Fatalf("ExtractFromSheet: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1 - the two rows are one vulnerability", len(findings))
	}
	// One row had no Technology, so it contributes the bare host; the other
	// carries the port. Both are the same host, and it prints once either way.
	if got := findings[0].AffectedSystem; got != "10.228.11.87, 10.228.11.87:22" {
		t.Errorf("affected host = %q", got)
	}
}

// TestSniffNamesASpreadsheetInTheWrongFormat. The message has to say what to do,
// not what failed.
func TestSniffNamesASpreadsheetInTheWrongFormat(t *testing.T) {
	long := bytes.Repeat([]byte("x"), 200)
	for _, tc := range []struct {
		name string
		data []byte
		ext  string
		want string
	}{
		{"legacy xls", append(append([]byte{}, oleHeader...), long...), ".xlsx", "Save As"},
		{"workbook named csv", append([]byte("PK\x03\x04"), long...), ".csv", "plain text"},
		{"xlsx that is not a zip", append([]byte("Title,Host\n"), long...), ".xlsx", "zip container"},
		{"binary named csv", append([]byte("Title\x00Host"), long...), ".csv", "binary file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := sniffDocument(tc.data, tc.ext)
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("message does not mention %q: %v", tc.want, err)
			}
		})
	}

	// A small CSV is a legitimate file, where a document that size is a
	// truncated upload.
	if err := sniffDocument([]byte("Vulnerability_Title,System_IP\nA,10.0.0.1\n"), ".csv"); err != nil {
		t.Errorf("a short but complete CSV was refused: %v", err)
	}
}

// TestExtractsTheRealMUNITExport runs the sample the feature was built from,
// when it is on this machine. It is a real export and is not committed, so the
// test skips rather than fails where it is absent.
func TestExtractsTheRealMUNITExport(t *testing.T) {
	path := filepath.Join("..", "docs", "vulnerability 38.csv")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("the sample export is not on this machine: %v", err)
	}
	if err := sniffDocument(data, ".csv"); err != nil {
		t.Fatalf("the sample was refused before it was read: %v", err)
	}
	rows, err := readSheetRows(data, ".csv")
	if err != nil {
		t.Fatalf("readSheetRows: %v", err)
	}
	findings, notes, err := ExtractFromSheet(rows, "IPT")
	if err != nil {
		t.Fatalf("ExtractFromSheet: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("got %d findings from the sample, want 2", len(findings))
	}
	for _, f := range findings {
		if f.AffectedSystem != "10.228.11.87:22" {
			t.Errorf("%s: affected host = %q, want 10.228.11.87:22", f.Title, f.AffectedSystem)
		}
		if f.Severity != "low" {
			t.Errorf("%s: severity = %q", f.Title, f.Severity)
		}
		if f.Impact != "Internal operations impact" {
			t.Errorf("%s: impact = %q", f.Title, f.Impact)
		}
		if f.Category != "IPT" {
			t.Errorf("%s: category = %q", f.Title, f.Category)
		}
		if !strings.Contains(f.Description, "SSH") {
			t.Errorf("%s: description did not survive: %q", f.Title, f.Description)
		}
		if strings.TrimSpace(f.Remediation) == "" {
			t.Errorf("%s: no recommendation", f.Title)
		}
	}
	t.Logf("sample: %d findings, notes: %v", len(findings), notes)
}

// TestReflowUndoesTheScannersHardWrap. MUNIT wraps its prose at about eighty
// columns for a terminal; the report prints it in a cell several inches wide,
// where those breaks make a narrow ragged column instead of a paragraph.
func TestReflowUndoesTheScannersHardWrap(t *testing.T) {
	in := strings.Join([]string{
		"The instance of MongoDB running on the remote host is affected by MongoBleed, an unauthenticated unintialized heap",
		"memory leak vulnerability:",
		"",
		"  - Mismatched length fields in Zlib compressed protocol headers may allow a read of uninitialized heap memory by an",
		"    unauthenticated client. (CVE-2025-14847)",
		"",
		"",
		"A remote, unauthenticated attacker can exploit this issue, via a specially crafted request, to leak potentially",
		"sensitive server memory.",
		"MUNIT was able to exploit an uninitialized heap memory leak vulnerability by",
		"sending crafted requests and received the following leaked memory from the",
		"server:",
		"",
		"0x00:  33 44 38 33 42 35 31 65 70 68 65 6D 65 72 61 6C    3D83B51ephemeral",
		"0x10:  46 6F 72 54 65 73 74                               ForTest         ",
		"",
		"OS                                   : Ubuntu Linux 16.04",
		"Security End of Life                 : April 30, 2021",
		"",
	}, "\n")

	got := strings.Split(reflowScannerText(in), "\n")
	want := []string{
		"The instance of MongoDB running on the remote host is affected by MongoBleed, an unauthenticated unintialized heap memory leak vulnerability:",
		"",
		"  - Mismatched length fields in Zlib compressed protocol headers may allow a read of uninitialized heap memory by an unauthenticated client. (CVE-2025-14847)",
		"",
		"A remote, unauthenticated attacker can exploit this issue, via a specially crafted request, to leak potentially sensitive server memory. MUNIT was able to exploit an uninitialized heap memory leak vulnerability by sending crafted requests and received the following leaked memory from the server:",
		"",
		"0x00:  33 44 38 33 42 35 31 65 70 68 65 6D 65 72 61 6C    3D83B51ephemeral",
		"0x10:  46 6F 72 54 65 73 74                               ForTest",
		"",
		"OS                                   : Ubuntu Linux 16.04",
		"Security End of Life                 : April 30, 2021",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reflow produced %d lines, want %d", len(got), len(want))
		for i := 0; i < max(len(got), len(want)); i++ {
			g, w := "", ""
			if i < len(got) {
				g = got[i]
			}
			if i < len(want) {
				w = want[i]
			}
			if g != w {
				t.Errorf("line %d:\n got %q\nwant %q", i, g, w)
			}
		}
	}
}

// TestReflowKeepsASentencesDoubleSpace. A scanner's prose puts two spaces after
// a full stop, so "columns held apart by spaces" has to mean three or more -
// at two, every second line of prose was mistaken for a table and left wrapped.
func TestReflowKeepsASentencesDoubleSpace(t *testing.T) {
	in := "The SSH server is configured to support Cipher Block Chaining (CBC)\n" +
		"encryption.  This may allow an attacker to recover the plaintext message\n" +
		"from the ciphertext."
	want := "The SSH server is configured to support Cipher Block Chaining (CBC) " +
		"encryption.  This may allow an attacker to recover the plaintext message from the ciphertext."
	if got := reflowScannerText(in); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// TestReflowLeavesASingleParagraphAlone. Most descriptions are one line and
// must come through untouched.
func TestReflowLeavesASingleParagraphAlone(t *testing.T) {
	for _, s := range []string{
		"Unsigned SMB accepted on the domain controllers.",
		"",
		"   ",
	} {
		if got := reflowScannerText(s); got != strings.TrimSpace(s) {
			t.Errorf("reflow(%q) = %q", s, got)
		}
	}
}
