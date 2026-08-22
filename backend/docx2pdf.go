package main

import (
	"fmt"
	"log"
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
//  1. Microsoft Word via COM - Windows only, highest fidelity. It is also the
//     only engine here that rebuilds the table of contents and the footer's
//     company-name reference, and it saves the refreshed result back into the
//     DOCX the user downloads.
//  2. LibreOffice (soffice) - cross-platform, headless, no Office licence.
//
// Returns a descriptive error if no engine is available; the caller decides
// whether to fall back.
func convertDOCXToPDF(docxPath, pdfPath string) (engine string, err error) {
	var wordErr error
	if runtime.GOOS == "windows" {
		if wordErr = convertWithWordCOM(docxPath, pdfPath); wordErr == nil {
			return "word", nil
		}
	}

	if soffice := findSoffice(); soffice != "" {
		sofficeErr := convertWithSoffice(soffice, docxPath, pdfPath)
		if sofficeErr == nil {
			return "libreoffice", nil
		}
		if wordErr != nil {
			return "", fmt.Errorf("word conversion failed (%v); libreoffice conversion failed: %w", wordErr, sofficeErr)
		}
		return "", fmt.Errorf("libreoffice conversion failed: %w", sofficeErr)
	}

	if wordErr != nil {
		return "", fmt.Errorf("word conversion failed: %w", wordErr)
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
	absDocx, err := filepath.Abs(docxPath)
	if err != nil {
		return err
	}
	absPdf, err := filepath.Abs(pdfPath)
	if err != nil {
		return err
	}

	// Word edits the document in place to resolve its fields, so it works on a
	// copy: if the conversion is killed part way through - a timeout, a Word
	// instance already wedged - the rendered DOCX we hand the user is still the
	// one the merge produced rather than a half-saved file.
	workPath := absDocx + ".word.docx"
	if err := copyFile(absDocx, workPath); err != nil {
		return fmt.Errorf("stage docx for word: %w", err)
	}
	defer os.Remove(workPath)

	// Word writes the refreshed document to a path of its own rather than back
	// over the file it has open: saving onto the open path fails silently here,
	// which left the delivered DOCX carrying the template's stale contents page
	// while the PDF had the right one.
	savedPath := absDocx + ".word.out.docx"
	defer os.Remove(savedPath)

	// The copy is opened for writing so the fields it carries can be resolved:
	// the table of contents still holds the template's cached entries and page
	// numbers, and the footers reference the client company name through a REF
	// field.
	//
	// Order and method here are not interchangeable. Exporting the PDF comes
	// first so that a problem writing the DOCX back can never cost us the PDF,
	// which is the harder artefact to reproduce. And the document is written
	// with SaveAs2 rather than Save: Save() blocks forever on this template
	// under automation, which is what silently pushed exports onto the fallback
	// renderer. Field updates are individually guarded so one stubborn field
	// cannot abort the whole conversion.
	//
	// wdExportFormatPDF = 17, wdFormatDocumentDefault = 16. Quotes are doubled
	// for PowerShell single-quoted strings.
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$word = $null
$doc = $null
try {
  $word = New-Object -ComObject Word.Application
  $word.Visible = $false
  $word.DisplayAlerts = 0
  $doc = $word.Documents.Open('%s', [ref]$false, [ref]$false)
  try { $doc.Fields.Update() | Out-Null } catch {}
  foreach ($sec in $doc.Sections) {
    foreach ($hf in $sec.Headers) { try { $hf.Range.Fields.Update() | Out-Null } catch {} }
    foreach ($hf in $sec.Footers) { try { $hf.Range.Fields.Update() | Out-Null } catch {} }
  }
  foreach ($toc in $doc.TablesOfContents) { try { $toc.Update() } catch {} }
  $doc.ExportAsFixedFormat('%s', 17)
  try { $doc.SaveAs2('%s', 16) } catch {}
} finally {
  if ($doc)  { try { $doc.Close([ref]$false) } catch {} }
  if ($word) { try { $word.Quit() } catch {} }
}
`,
		strings.ReplaceAll(workPath, "'", "''"),
		strings.ReplaceAll(absPdf, "'", "''"),
		strings.ReplaceAll(savedPath, "'", "''"))

	startedAt := time.Now()
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start powershell: %w", err)
	}
	go func() { done <- cmd.Wait() }()

	// Rebuilding the contents and every field on a report of this size takes
	// noticeably longer than a plain export, and Word's first COM start on a
	// cold machine is slow on its own.
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("word export: %w", err)
		}
	case <-time.After(300 * time.Second):
		_ = cmd.Process.Kill()
		// Killing PowerShell orphans the Word process it started, which then
		// sits invisibly holding the document open until the machine reboots.
		killOrphanedWord(startedAt)
		return fmt.Errorf("word export timed out")
	}

	if fi, err := os.Stat(absPdf); err != nil || fi.Size() == 0 {
		return fmt.Errorf("word produced no PDF")
	}
	// The refreshed document is promoted over the merged one so the DOCX
	// download carries the same resolved contents page as the PDF. If Word could
	// not write one, the PDF still stands and the merged DOCX asks Word to
	// refresh its fields on open, so this stays best effort.
	if fi, statErr := os.Stat(savedPath); statErr == nil && fi.Size() > 0 {
		if err := copyFile(savedPath, absDocx); err != nil {
			log.Printf("report: keeping the merged DOCX, could not promote word's copy: %v", err)
		}
	} else {
		log.Printf("report: keeping the merged DOCX, word wrote no refreshed copy")
	}
	return nil
}

// killOrphanedWord ends the invisible Word process our own timed-out conversion
// left behind. The filter is deliberately narrow - started no earlier than this
// conversion did, and with no main window - so it can only ever match an
// automation instance we spawned, never a Word the user has open.
func killOrphanedWord(startedAt time.Time) {
	script := fmt.Sprintf(`
$cutoff = [DateTime]::Parse('%s')
Get-Process WINWORD -ErrorAction SilentlyContinue |
  Where-Object { $_.MainWindowHandle -eq 0 -and $_.StartTime -ge $cutoff } |
  Stop-Process -Force -ErrorAction SilentlyContinue
`, startedAt.Add(-2*time.Second).Format("2006-01-02T15:04:05"))

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if err := cmd.Start(); err != nil {
		return
	}
	go func() {
		timer := time.AfterFunc(30*time.Second, func() { _ = cmd.Process.Kill() })
		defer timer.Stop()
		if err := cmd.Wait(); err != nil {
			log.Printf("report: could not clean up the orphaned word process: %v", err)
		}
	}()
}

// copyFile writes src over dst, creating it if needed.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
