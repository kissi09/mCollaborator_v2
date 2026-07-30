package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Converting the merged DOCX with a real Word layout engine is the only way to
// get a PDF that matches the template exactly - every style, table, chart,
// header and page break is laid out by the same engine that authored the
// template. Redrawing the document with a PDF primitive library can only ever
// approximate it.
//
// Order of preference:
//  1. LibreOffice (soffice) - cross-platform, headless, no Office licence.
//  2. Microsoft Word via COM - Windows only, highest fidelity.
//
// Returns a descriptive error if no engine is available; the caller decides
// whether to fall back.
func convertDOCXToPDF(docxPath, pdfPath string) (engine string, err error) {
	if soffice := findSoffice(); soffice != "" {
		if err := convertWithSoffice(soffice, docxPath, pdfPath); err == nil {
			return "libreoffice", nil
		} else {
			lastErr := err
			if runtime.GOOS == "windows" {
				if err := convertWithWordCOM(docxPath, pdfPath); err == nil {
					return "word", nil
				}
			}
			return "", fmt.Errorf("libreoffice conversion failed: %w", lastErr)
		}
	}

	if runtime.GOOS == "windows" {
		if err := convertWithWordCOM(docxPath, pdfPath); err == nil {
			return "word", nil
		} else {
			return "", fmt.Errorf("word conversion failed: %w", err)
		}
	}

	return "", fmt.Errorf("no DOCX->PDF engine found (install LibreOffice, or Microsoft Word on Windows)")
}

// findSoffice locates a LibreOffice binary on PATH or in the usual install dirs.
func findSoffice() string {
	for _, name := range []string{"soffice", "libreoffice"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	candidates := []string{
		`C:\Program Files\LibreOffice\program\soffice.exe`,
		`C:\Program Files (x86)\LibreOffice\program\soffice.exe`,
		"/usr/bin/soffice",
		"/usr/bin/libreoffice",
		"/opt/libreoffice/program/soffice",
		"/Applications/LibreOffice.app/Contents/MacOS/soffice",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func convertWithSoffice(soffice, docxPath, pdfPath string) error {
	outDir := filepath.Dir(pdfPath)

	// A dedicated user profile avoids clashing with a desktop LibreOffice
	// session, which otherwise makes headless runs exit silently.
	profile, err := os.MkdirTemp("", "mcollab-lo-")
	if err != nil {
		return fmt.Errorf("create libreoffice profile dir: %w", err)
	}
	defer os.RemoveAll(profile)

	profileURL := "file:///" + filepath.ToSlash(profile)

	cmd := exec.Command(soffice,
		"--headless", "--norestore", "--nolockcheck", "--nodefault",
		"-env:UserInstallation="+profileURL,
		"--convert-to", "pdf:writer_pdf_Export",
		"--outdir", outDir,
		docxPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("soffice: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	// soffice names the output after the input basename; move it if needed.
	produced := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(docxPath), filepath.Ext(docxPath))+".pdf")
	if produced != pdfPath {
		if _, statErr := os.Stat(produced); statErr != nil {
			return fmt.Errorf("soffice produced no PDF: %s", strings.TrimSpace(string(out)))
		}
		os.Remove(pdfPath)
		if err := os.Rename(produced, pdfPath); err != nil {
			return fmt.Errorf("move converted pdf: %w", err)
		}
	}
	if fi, err := os.Stat(pdfPath); err != nil || fi.Size() == 0 {
		return fmt.Errorf("soffice produced an empty PDF")
	}
	return nil
}

// convertWithWordCOM drives Word through PowerShell. Word must be installed;
// this is the highest-fidelity path on Windows since it is the authoring engine.
func convertWithWordCOM(docxPath, pdfPath string) error {
	abcDocx, err := filepath.Abs(docxPath)
	if err != nil {
		return err
	}
	absPdf, err := filepath.Abs(pdfPath)
	if err != nil {
		return err
	}

	// wdExportFormatPDF = 17. Quotes are doubled for PowerShell single-quoted strings.
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$word = $null
$doc = $null
try {
  $word = New-Object -ComObject Word.Application
  $word.Visible = $false
  $word.DisplayAlerts = 0
  $doc = $word.Documents.Open('%s', [ref]$false, [ref]$true)
  $doc.ExportAsFixedFormat('%s', 17)
} finally {
  if ($doc)  { try { $doc.Close([ref]$false) } catch {} }
  if ($word) { try { $word.Quit() } catch {} }
}
`, strings.ReplaceAll(abcDocx, "'", "''"), strings.ReplaceAll(absPdf, "'", "''"))

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start powershell: %w", err)
	}
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("word export: %w", err)
		}
	case <-time.After(120 * time.Second):
		_ = cmd.Process.Kill()
		return fmt.Errorf("word export timed out")
	}

	if fi, err := os.Stat(absPdf); err != nil || fi.Size() == 0 {
		return fmt.Errorf("word produced no PDF")
	}
	return nil
}
