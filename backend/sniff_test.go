package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestSniffDocumentNamesWhatArrived(t *testing.T) {
	realDOCX := append([]byte{'P', 'K', 0x03, 0x04}, bytes.Repeat([]byte{'x'}, 200)...)
	realPDF := append([]byte("%PDF-1.7"), bytes.Repeat([]byte{'x'}, 200)...)
	legacyDOC := append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, bytes.Repeat([]byte{'x'}, 200)...)

	cases := []struct {
		name string
		data []byte
		ext  string
		want string // substring, empty means it must be accepted
	}{
		{"real docx", realDOCX, ".docx", ""},
		{"real pdf", realPDF, ".pdf", ""},
		{"empty upload", nil, ".docx", "arrived empty"},
		{"truncated upload", []byte("PK\x03\x04short"), ".docx", "only 9 bytes"},
		{"zeroed upload", make([]byte, 5000), ".pdf", "zeros"},
		{"legacy doc named docx", legacyDOC, ".docx", "legacy Word .doc"},
		{"rtf named docx", append([]byte(`{\rtf1\ansi`), bytes.Repeat([]byte{'x'}, 200)...), ".docx", "RTF"},
		{"html named docx", append([]byte("<html><body>hi"), bytes.Repeat([]byte{'x'}, 200)...), ".docx", "HTML or XML"},
		{"pdf named docx", realPDF, ".docx", "not a .docx"},
		{"docx named pdf", realDOCX, ".pdf", "not a PDF"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := sniffDocument(c.data, c.ext)
			if c.want == "" {
				if err != nil {
					t.Fatalf("expected acceptance, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected rejection mentioning %q, got nil", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("expected %q in error, got %v", c.want, err)
			}
		})
	}
}

// A PDF with a byte-order mark in front of its header is a valid PDF that the
// reader would otherwise refuse.
func TestTrimPDFPreamble(t *testing.T) {
	body := append([]byte("%PDF-1.7"), bytes.Repeat([]byte{'x'}, 100)...)
	withBOM := append([]byte{0xEF, 0xBB, 0xBF}, body...)
	if got := trimPDFPreamble(withBOM); !bytes.Equal(got, body) {
		t.Fatalf("preamble not trimmed: head %s", describeHead(got))
	}
	if got := trimPDFPreamble(body); !bytes.Equal(got, body) {
		t.Fatal("a clean PDF must be returned unchanged")
	}
}
