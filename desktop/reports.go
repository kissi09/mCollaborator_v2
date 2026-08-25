package main

// The one thing the window does that a browser tab does not have to.
//
// In a browser, a link to /api/v1/reports/download/... is enough: the server
// sends Content-Disposition: attachment and the browser saves the file. Inside
// WebView2 that link is a dead end. A target="_blank" anchor raises a
// NewWindowRequested event, Wails registers no handler for it, and the click is
// swallowed - no window, no download, and no request reaching the server at
// all. The desktop app would generate a report the user could never take
// delivery of.
//
// So the shell binds two methods into the page. The web app calls them when
// they are there and falls back to an ordinary download link when they are not,
// which keeps one codebase serving both shells.

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// reportFetchTimeout is loose because the file is read from the local server
// over loopback but can be a hundred-page PDF on a slow disk.
const reportFetchTimeout = 60 * time.Second

// App is the desktop shell's API, bound into the window as window.go.main.App.
type App struct {
	ctx     context.Context
	baseURL string
}

// startup captures the context the Wails dialogs need.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// SaveReportAs copies a generated document out of the server to wherever the
// user points the save dialog. It returns the path written, or an empty string
// if the dialog was cancelled - a cancel is not a failure and must not be
// reported as one.
func (a *App) SaveReportAs(reportURL, suggestedName string) (string, error) {
	data, name, err := a.fetchReport(reportURL, suggestedName)
	if err != nil {
		return "", err
	}

	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:                "Save report",
		DefaultFilename:      name,
		CanCreateDirectories: true,
		Filters:              saveFilters(name),
	})
	if err != nil {
		return "", fmt.Errorf("open the save dialog: %w", err)
	}
	if path == "" {
		return "", nil
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	log.Printf("saved %s (%d bytes)", path, len(data))
	return path, nil
}

// OpenReport writes the document beside the app's own data and hands it to
// whatever the machine opens that file type with.
//
// The copies are kept rather than cleaned up: the handler is still reading the
// file after this call returns, and a user who opened a report in Word will
// look for it again.
func (a *App) OpenReport(reportURL, suggestedName string) error {
	data, name, err := a.fetchReport(reportURL, suggestedName)
	if err != nil {
		return err
	}

	dir, err := openedReportsDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	log.Printf("opening %s", path)
	return openWithDefaultApp(path)
}

// fetchReport pulls the document from the supervised server.
func (a *App) fetchReport(reportURL, suggestedName string) ([]byte, string, error) {
	ref, err := url.Parse(reportURL)
	if err != nil {
		return nil, "", fmt.Errorf("parse %q: %w", reportURL, err)
	}
	// Only this app's own report endpoints are reachable through here. A bound
	// method that fetched and wrote out any URL handed to it would be a hole,
	// not a feature.
	if ref.IsAbs() || ref.Host != "" || !strings.HasPrefix(ref.Path, "/api/v1/reports/") {
		return nil, "", fmt.Errorf("refusing to fetch %q: not a report URL", reportURL)
	}
	base, err := url.Parse(a.baseURL)
	if err != nil {
		return nil, "", fmt.Errorf("parse server address %q: %w", a.baseURL, err)
	}

	client := &http.Client{Timeout: reportFetchTimeout}
	resp, err := client.Get(base.ResolveReference(ref).String())
	if err != nil {
		return nil, "", fmt.Errorf("ask the server for the report: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("the server answered %s for the report", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read the report: %w", err)
	}
	return data, safeFileName(suggestedName, ref.Path), nil
}

// safeFileName reduces the name the page suggested to a leaf the filesystem
// will accept. Report titles are prose - "GH-REP-041-26120-01 - Zenith Bank -
// VAPT.docx" - so spaces stay; separators and the characters Windows reserves
// do not.
func safeFileName(suggested, urlPath string) string {
	name := filepath.Base(strings.TrimSpace(suggested))
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 0x20 {
			return -1
		}
		return r
	}, name)
	name = strings.Trim(name, " .")

	if name == "" || name == "." {
		// Nothing usable was suggested, so fall back to the URL's own last two
		// segments: /reports/download/{kind}/{title}.
		parts := strings.Split(strings.Trim(urlPath, "/"), "/")
		if len(parts) >= 2 {
			if title, err := url.PathUnescape(parts[len(parts)-1]); err == nil {
				return title + "." + parts[len(parts)-2]
			}
		}
		return "report"
	}
	return name
}

// saveFilters names the file type in the save dialog, so the shell offers the
// right extension rather than "All files".
func saveFilters(name string) []wailsruntime.FileFilter {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".docx":
		return []wailsruntime.FileFilter{{DisplayName: "Word document (*.docx)", Pattern: "*.docx"}}
	case ".pdf":
		return []wailsruntime.FileFilter{{DisplayName: "PDF document (*.pdf)", Pattern: "*.pdf"}}
	case ".pptx":
		return []wailsruntime.FileFilter{{DisplayName: "PowerPoint presentation (*.pptx)", Pattern: "*.pptx"}}
	}
	return nil
}

// openedReportsDir is where documents opened from the window are staged.
func openedReportsDir() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "opened")
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", fmt.Errorf("prepare %s: %w", path, err)
	}
	return path, nil
}
