package main

import (
	"bytes"
	"fmt"
)

// What a file actually is, as opposed to what its name claims.
//
// Word opens a legacy .doc, an .rtf and a saved web page happily whatever the
// file is called, so "it opens fine on my machine" is not evidence that a file
// is a .docx. When the extractor was handed one of those it reported the
// parser's own words - "zip: not a valid zip file" - which tells the tester
// nothing about what to do next. So does the truncated or empty upload that a
// transport fault produces. This names the difference.

// oleHeader is the OLE2 compound-document signature: a real .doc, .xls or .ppt.
var oleHeader = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

// pdfHeader must be at offset 0 for the PDF reader to accept the file, though
// some generators leave a byte-order mark or whitespace in front of it.
var pdfHeader = []byte("%PDF-")

// zipHeader opens every .docx, which is a zip container.
var zipHeader = []byte{'P', 'K', 0x03, 0x04}

// sniffDocument reports why data cannot be the format ext claims, or nil when
// the leading bytes are consistent with it. The message is written for the
// person holding the file, and says what to do rather than what failed.
func sniffDocument(data []byte, ext string) error {
	if len(data) == 0 {
		return fmt.Errorf("the upload arrived empty - the file was read but none of its content " +
			"reached the server, so there was nothing to import. This is a fault in the upload, " +
			"not in your document")
	}
	if len(data) < 64 {
		return fmt.Errorf("the upload arrived as only %d bytes, far too small to be a report - "+
			"the file content did not survive the upload. This is a fault in the upload, not in "+
			"your document", len(data))
	}
	if isAllZero(data[:min(len(data), 4096)]) {
		return fmt.Errorf("the upload arrived as %d bytes of zeros. That usually means the file "+
			"had not finished downloading from OneDrive (an 'online-only' file) or was locked by "+
			"another program. Open the file once, make sure it is available offline, and try again", len(data))
	}

	// Named as one thing, actually another. These are worth calling out by
	// name because each has a different fix.
	switch {
	case bytes.HasPrefix(data, oleHeader):
		return fmt.Errorf("this is a legacy Word .doc saved under a .docx name. Word opens it, but "+
			"it is not a .docx and cannot be read as one. Open it in Word and use File > Save As > "+
			"Word Document (.docx), then import that (received %d bytes)", len(data))
	case bytes.HasPrefix(data, []byte(`{\rtf`)):
		return fmt.Errorf("this is an RTF file under a %s name. Open it in Word and save it as a "+
			"Word Document (.docx) or a PDF, then import that", ext)
	case bytes.HasPrefix(bytes.TrimLeft(data[:min(len(data), 512)], " \t\r\n"), []byte("<")):
		return fmt.Errorf("this is an HTML or XML file under a %s name - a web page saved from a "+
			"browser, or Word's 'Web Page' format. Save it as a real .docx or PDF and try again", ext)
	}

	switch ext {
	case ".docx":
		if !bytes.HasPrefix(data, zipHeader) {
			return fmt.Errorf("this is not a .docx: a .docx is a zip container and must start with "+
				"%q, but this file starts with %s (%d bytes received)",
				"PK", describeHead(data), len(data))
		}
	case ".pdf":
		if !bytes.HasPrefix(data, pdfHeader) {
			if off := bytes.Index(data[:min(len(data), 1024)], pdfHeader); off > 0 {
				return fmt.Errorf("this PDF has %d stray bytes before its header, which the reader "+
					"will not accept. Re-save or re-export the PDF and try again", off)
			}
			return fmt.Errorf("this is not a PDF: it must start with %q, but this file starts with "+
				"%s (%d bytes received)", "%PDF-", describeHead(data), len(data))
		}
	}
	return nil
}

// trimPDFPreamble drops anything before the %PDF- header. Some exporters emit a
// byte-order mark or a blank line first; the file is otherwise a valid PDF and
// there is no reason to refuse it.
func trimPDFPreamble(data []byte) []byte {
	if bytes.HasPrefix(data, pdfHeader) {
		return data
	}
	if off := bytes.Index(data[:min(len(data), 1024)], pdfHeader); off > 0 {
		return data[off:]
	}
	return data
}

func isAllZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// describeHead renders the first bytes of a file readably, so a log line or an
// error message says what actually arrived.
func describeHead(data []byte) string {
	head := data[:min(len(data), 12)]
	printable := true
	for _, c := range head {
		if c < 0x20 || c > 0x7e {
			printable = false
			break
		}
	}
	if printable {
		return fmt.Sprintf("%q", string(head))
	}
	return fmt.Sprintf("bytes % x", head)
}
