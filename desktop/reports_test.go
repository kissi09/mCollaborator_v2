package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The window hands these methods a URL it read out of a JSON response. A bound
// method that fetched whatever it was given would let any page content the
// server ever renders pull a file onto the user's disk, so the guard is the
// part worth testing.
func TestFetchReportRejectsForeignURLs(t *testing.T) {
	app := &App{baseURL: "http://127.0.0.1:1"}

	for _, bad := range []string{
		"http://example.com/api/v1/reports/download/docx/x",
		"//example.com/api/v1/reports/download/docx/x",
		"/api/v1/evidence/1/file",
		"/etc/passwd",
	} {
		if _, _, err := app.fetchReport(bad, "x.docx"); err == nil {
			t.Errorf("fetchReport(%q) was allowed; it must be refused", bad)
		}
	}
}

func TestFetchReportReadsFromTheServer(t *testing.T) {
	const body = "PK\x03\x04 pretend docx"
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(body))
	}))
	defer srv.Close()

	app := &App{baseURL: srv.URL}
	data, name, err := app.fetchReport(
		"/api/v1/reports/download/docx/GH-REP-041%20-%20Zenith%20Bank%20-%20VAPT",
		"GH-REP-041 - Zenith Bank - VAPT.docx")
	if err != nil {
		t.Fatalf("fetchReport: %v", err)
	}
	if string(data) != body {
		t.Errorf("body = %q, want %q", data, body)
	}
	if name != "GH-REP-041 - Zenith Bank - VAPT.docx" {
		t.Errorf("name = %q", name)
	}
	if !strings.HasPrefix(gotPath, "/api/v1/reports/download/") {
		t.Errorf("server was asked for %q", gotPath)
	}
}

func TestFetchReportReportsAServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Report not found", http.StatusNotFound)
	}))
	defer srv.Close()

	app := &App{baseURL: srv.URL}
	if _, _, err := app.fetchReport("/api/v1/reports/download/pdf/missing", "missing.pdf"); err == nil {
		t.Fatal("a 404 from the server was reported as success")
	}
}

func TestSafeFileName(t *testing.T) {
	cases := []struct {
		suggested, urlPath, want string
	}{
		// The ordinary case: prose title, spaces kept.
		{"GH-REP-041 - Zenith Bank - VAPT.docx", "", "GH-REP-041 - Zenith Bank - VAPT.docx"},
		// A title cannot climb out of the directory the user picked.
		{"../../evil.docx", "", "evil.docx"},
		{`sub\dir\x.pdf`, "", "x.pdf"},
		// Characters Windows will not accept in a name.
		{`Zenith: Q3 "final"?.pptx`, "", "Zenith Q3 final.pptx"},
		// Nothing usable suggested, so the URL supplies the name.
		{"", "/api/v1/reports/download/pptx/Zenith%20Bank%20-%20Closing%20Meeting",
			"Zenith Bank - Closing Meeting.pptx"},
		{"", "", "report"},
	}
	for _, c := range cases {
		if got := safeFileName(c.suggested, c.urlPath); got != c.want {
			t.Errorf("safeFileName(%q, %q) = %q, want %q", c.suggested, c.urlPath, got, c.want)
		}
	}
}
