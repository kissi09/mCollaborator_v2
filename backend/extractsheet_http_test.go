package main

// The upload itself, end to end through the handler.
//
// The parser being right is not the same as the upload working: finding import
// never worked in the desktop shell for two days because the multipart body was
// lost on the way in, and the parser reported exactly what an empty document
// looks like. So the area field and the file are posted here the way the page
// posts them, through the real handler.

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// extractRequest posts a file to the extract endpoint the way the page does,
// and returns the status and the decoded body.
func extractRequest(t *testing.T, store *Store, engID, filename string, data []byte, area string) (int, ApiResponse) {
	t.Helper()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	// The file first and the area after it, which is the order the page
	// appends them in - a handler that read the field before the body would
	// pass a test that built the parts the other way round.
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if area != "" {
		if err := mw.WriteField("area", area); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/engagements/"+engID+"/findings/extract", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	// The handler reads the engagement id off the route, so the route context
	// has to carry it as chi would.
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", engID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	HandleExtractFindings(store)(rec, req)

	var out ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON (%d): %s", rec.Code, rec.Body.String())
	}
	return rec.Code, out
}

// testStore is an isolated store on a database of its own, so a test never
// touches the running app's.
func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	t.Setenv("MCOLLABORATOR_DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	store := NewStore()
	// Windows will not delete a file that is still open, and t.TempDir removes
	// its directory when the test ends - so the SQLite handle has to be let go
	// first or the test fails on the cleanup rather than on anything it checked.
	t.Cleanup(func() { store.db.Close() })
	eng := &Engagement{
		ID:        "eng-sheet-test",
		Name:      "Sheet import",
		Status:    "active",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	store.CreateEngagement(eng)
	return store, eng.ID
}

func TestExtractEndpointReadsASpreadsheet(t *testing.T) {
	store, engID := testStore(t)

	csv := munitCSV(
		munitRow("SSH Server CBC Mode Ciphers Enabled", "The SSH server supports CBC.",
			"Edit sshd_config.", "", "10.228.11.87", "Low", "", "3", "Internal operations impact", "SSH,22"),
		munitRow("SSH Server CBC Mode Ciphers Enabled", "", "", "", "10.228.11.90", "", "", "", "", "SSH,22"),
	)

	t.Run("with an area", func(t *testing.T) {
		code, res := extractRequest(t, store, engID, "vulnerability 38.csv", csv, "IPT")
		if code != http.StatusOK {
			t.Fatalf("status %d: %+v", code, res.Error)
		}
		raw, _ := json.Marshal(res.Data)
		var result ExtractResult
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		if result.Kind != "csv" {
			t.Errorf("kind = %q, want csv", result.Kind)
		}
		if len(result.Findings) != 1 {
			t.Fatalf("got %d findings, want 1", len(result.Findings))
		}
		if want := "10.228.11.87:22, 10.228.11.90:22"; result.Findings[0].AffectedSystem != want {
			t.Errorf("affected host = %q, want %q", result.Findings[0].AffectedSystem, want)
		}
		if result.Unplaced != 0 || result.ByArea["IPT"] != 1 {
			t.Errorf("by_area = %v, unplaced = %d", result.ByArea, result.Unplaced)
		}
	})

	t.Run("without an area", func(t *testing.T) {
		code, res := extractRequest(t, store, engID, "vulnerability 38.csv", csv, "")
		if code != http.StatusBadRequest {
			t.Fatalf("status %d, want 400 - a sheet with no area was accepted", code)
		}
		if res.Error == nil || res.Error.Code != "AREA_REQUIRED" {
			t.Errorf("error = %+v, want AREA_REQUIRED", res.Error)
		}
	})

	t.Run("with an area that is not one", func(t *testing.T) {
		code, res := extractRequest(t, store, engID, "scan.xlsx", csv, "NOPE")
		if code != http.StatusBadRequest || res.Error == nil || res.Error.Code != "AREA_REQUIRED" {
			t.Errorf("status %d, error %+v", code, res.Error)
		}
	})

	t.Run("a docx still needs no area", func(t *testing.T) {
		// Not a real .docx, so it is refused - but for being unreadable, not
		// for having no area, which is the distinction that matters here.
		code, res := extractRequest(t, store, engID, "report.docx",
			bytes.Repeat([]byte("not a zip"), 40), "")
		if code == http.StatusBadRequest && res.Error != nil && res.Error.Code == "AREA_REQUIRED" {
			t.Error("a .docx was made to pick an assessment area")
		}
	})

	t.Run("an xlsx that is really a csv is named", func(t *testing.T) {
		code, res := extractRequest(t, store, engID, "scan.xlsx", csv, "IPT")
		if code != http.StatusUnprocessableEntity {
			t.Fatalf("status %d, want 422", code)
		}
		if res.Error == nil || !strings.Contains(res.Error.Message, "zip container") {
			t.Errorf("message does not say what is wrong: %+v", res.Error)
		}
	})

	t.Run("an empty upload is still a transport fault", func(t *testing.T) {
		code, res := extractRequest(t, store, engID, "scan.csv", nil, "IPT")
		if code != http.StatusUnprocessableEntity {
			t.Fatalf("status %d, want 422", code)
		}
		if res.Error == nil || !strings.Contains(res.Error.Message, "fault in the upload") {
			t.Errorf("an empty sheet was not named as an upload fault: %+v", res.Error)
		}
	})

	t.Run("an xlsx workbook reads through the endpoint", func(t *testing.T) {
		code, res := extractRequest(t, store, engID, "scan.xlsx", buildXLSX(t), "EPT")
		if code != http.StatusOK {
			t.Fatalf("status %d: %+v", code, res.Error)
		}
		raw, _ := json.Marshal(res.Data)
		var result ExtractResult
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		if result.Kind != "xlsx" || len(result.Findings) != 1 {
			t.Fatalf("kind %q, %d findings", result.Kind, len(result.Findings))
		}
		if result.Findings[0].Category != "EPT" {
			t.Errorf("category = %q, want the area posted with the file", result.Findings[0].Category)
		}
	})
}
