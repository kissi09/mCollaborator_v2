package main

// OneDrive sync.
//
// The report wizard's last step can push the generated report straight into the
// Cyberteq OneDrive for Business, so a finished report lands where it is filed
// rather than in the server's reports directory.
//
// Authentication is the OAuth2 client-credential flow against a Microsoft Entra
// ID app registration, and the upload goes to a named user's drive through
// Microsoft Graph. Because there is no signed-in user in this flow, the target
// drive is addressed explicitly by UPN - that is what OD_USER is for.
//
// Environment:
//
//	OD_TENANT_ID      Azure Directory (tenant) ID
//	OD_CLIENT_ID      Application (client) ID
//	OD_CLIENT_SECRET  Client secret from Certificates & secrets
//	OD_USER           UPN of the OneDrive to upload into,
//	                  e.g. "you@cyberteqfalcon.com"
//	OD_FOLDER         Optional. Default destination folder inside that drive.
//	                  Defaults to "VAPT Reports".
//
// The app registration needs the Files.ReadWrite.All *application* permission
// with admin consent granted.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// OneDriveConfig holds the Entra ID app registration credentials and the drive
// to upload into.
type OneDriveConfig struct {
	TenantID     string
	ClientID     string
	ClientSecret string
	User         string // UPN whose OneDrive receives the files
	Folder       string // default destination folder
}

type oneDrive struct {
	cfg      OneDriveConfig
	token    string
	expires  time.Time
	tokenMu  sync.Mutex
	apiBase  string
	loginURL string
	client   *http.Client
}

// defaultOneDriveFolder is where reports are filed when the wizard leaves the
// folder blank and OD_FOLDER is unset.
const defaultOneDriveFolder = "VAPT Reports"

// newOneDrive builds a client from the environment. It returns nil when the
// integration is not configured, which the caller reports to the wizard as
// "not configured" rather than as a failure.
func newOneDrive() *oneDrive {
	cfg := OneDriveConfig{
		TenantID:     os.Getenv("OD_TENANT_ID"),
		ClientID:     os.Getenv("OD_CLIENT_ID"),
		ClientSecret: os.Getenv("OD_CLIENT_SECRET"),
		User:         strings.TrimSpace(os.Getenv("OD_USER")),
		Folder:       strings.TrimSpace(os.Getenv("OD_FOLDER")),
	}
	if cfg.TenantID == "" || cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.User == "" {
		return nil
	}
	if cfg.Folder == "" {
		cfg.Folder = defaultOneDriveFolder
	}
	return &oneDrive{
		cfg:      cfg,
		apiBase:  "https://graph.microsoft.com/v1.0",
		loginURL: fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", cfg.TenantID),
		client:   &http.Client{Timeout: 120 * time.Second},
	}
}

// missingOneDriveEnv names the variables that still need setting, so the wizard
// can say which one is missing instead of "not configured".
func missingOneDriveEnv() []string {
	var missing []string
	for _, name := range []string{"OD_TENANT_ID", "OD_CLIENT_ID", "OD_CLIENT_SECRET", "OD_USER"} {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

// acquireToken returns a valid Bearer token, reusing the cached one until just
// before it expires.
func (d *oneDrive) acquireToken(ctx context.Context) (string, error) {
	d.tokenMu.Lock()
	defer d.tokenMu.Unlock()

	if d.token != "" && time.Now().Before(d.expires.Add(-2*time.Minute)) {
		return d.token, nil
	}

	form := url.Values{}
	form.Set("client_id", d.cfg.ClientID)
	form.Set("client_secret", d.cfg.ClientSecret)
	form.Set("scope", "https://graph.microsoft.com/.default")
	form.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.loginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token failed (%d): %s", resp.StatusCode, string(body))
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("empty access token in response")
	}

	d.token = tr.AccessToken
	d.expires = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return d.token, nil
}

// drivePath builds the Graph item path for one file, escaping each path segment
// on its own so folder separators survive but spaces and '&' in a client name
// do not break the URL.
func (d *oneDrive) drivePath(folder, filename string) string {
	var segs []string
	for _, part := range strings.Split(strings.Trim(strings.TrimSpace(folder), "/"), "/") {
		if part = strings.TrimSpace(part); part != "" {
			segs = append(segs, url.PathEscape(part))
		}
	}
	segs = append(segs, url.PathEscape(filename))
	return strings.Join(segs, "/")
}

// Upload puts one file into the configured OneDrive, creating any missing
// parent folders, and returns the webUrl the file can be opened at.
//
// This uses the simple upload endpoint, which Graph caps at 250MB - well above
// anything this report generator produces.
func (d *oneDrive) Upload(ctx context.Context, folder, filename string, content []byte) (string, error) {
	token, err := d.acquireToken(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(folder) == "" {
		folder = d.cfg.Folder
	}

	uploadURL := fmt.Sprintf("%s/users/%s/drive/root:/%s:/content",
		d.apiBase, url.PathEscape(d.cfg.User), d.drivePath(folder, filename))

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(content))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload failed (%d): %s", resp.StatusCode, string(body))
	}

	var item driveItemResponse
	if err := json.Unmarshal(body, &item); err != nil {
		return "", fmt.Errorf("parse drive item: %w", err)
	}
	return item.WebURL, nil
}

// syncReportToOneDrive uploads the generated report and records the outcome on
// the wizard's response.
//
// The DOCX is the deliverable of record - it is the merged template - so it is
// uploaded first and a failure there fails the sync. The PDF is uploaded when
// one was produced; a report that only made it to DOCX still syncs.
func syncReportToOneDrive(ctx context.Context, config ReportConfig, docxPath, pdfPath string, havePDF bool, out *exportReportResponse) {
	drive := newOneDrive()
	if drive == nil {
		out.ODStatus = "not_configured"
		out.ODError = "OneDrive is not set up on the server: " +
			strings.Join(missingOneDriveEnv(), ", ") + " not set"
		return
	}

	folder := strings.TrimSpace(config.OneDriveFolder)
	if folder == "" {
		// One folder per client under the configured root, so a drive holding
		// several engagements stays navigable.
		folder = drive.cfg.Folder
		if company := sanitizeFilename(config.CompanyName); company != "" {
			folder += "/" + company
		}
	}
	out.ODFolder = folder

	docxLink, err := drive.Upload(ctx, folder, filepath.Base(docxPath), mustReadFile(docxPath))
	if err != nil {
		out.ODStatus = "failed"
		out.ODError = err.Error()
		log.Printf("onedrive: upload of %s failed: %v", filepath.Base(docxPath), err)
		return
	}
	out.ODDOCXLink = docxLink

	if havePDF {
		pdfLink, err := drive.Upload(ctx, folder, filepath.Base(pdfPath), mustReadFile(pdfPath))
		if err != nil {
			// The DOCX is already filed, so this is a partial success and is
			// reported as one rather than as an outright failure.
			out.ODStatus = "ok"
			out.ODError = "the DOCX was uploaded but the PDF was not: " + err.Error()
			log.Printf("onedrive: upload of %s failed: %v", filepath.Base(pdfPath), err)
			return
		}
		out.ODPDFLink = pdfLink
	}

	out.ODStatus = "ok"
	log.Printf("onedrive: filed %q in %s", filepath.Base(docxPath), folder)
}

// --- response models -------------------------------------------------------

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type driveItemResponse struct {
	ID     string `json:"id"`
	WebURL string `json:"webUrl"`
}
