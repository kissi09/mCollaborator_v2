package main

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
)

//go:embed static
var staticFiles embed.FS

func main() {
	store := NewStore()
	port := os.Getenv("PORT")
	if port == "" {
		port = "9900"
	}

	r := chi.NewRouter()

	// Middleware
	r.Use(corsMiddleware)
	r.Use(loggingMiddleware)
	r.Use(rateLimitMiddleware)

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Public
		r.Post("/auth/login", HandleLogin(store))
		r.Post("/auth/logout", HandleLogout(store))

		r.Group(func(r chi.Router) {
			r.Use(authMiddleware(store))

			// User
			r.Get("/users/me", HandleGetMe(store))
			r.Get("/users", HandleListUsers(store))

			// Engagements
			r.Get("/engagements", HandleListEngagements(store))
			r.Post("/engagements", requirePermission("engagements:write", HandleCreateEngagement(store)))
			r.Get("/engagements/{id}", HandleGetEngagement(store))
			r.Put("/engagements/{id}", requirePermission("engagements:write", HandleUpdateEngagement(store)))
			r.Delete("/engagements/{id}", requirePermission("engagements:write", HandleDeleteEngagement(store)))
			r.Get("/engagements/{id}/nodes", HandleListNodes(store))
			r.Post("/engagements/{id}/nodes", requirePermission("engagements:write", HandleCreateNode(store)))
			r.Get("/engagements/{id}/findings", HandleListFindings(store))
			r.Post("/engagements/{id}/findings", requirePermission("finding:write", HandleCreateFinding(store)))

			// Findings
			r.Get("/findings/{id}", HandleGetFinding(store))
			r.Put("/findings/{id}", requirePermission("finding:write", HandleUpdateFinding(store)))
			r.Patch("/findings/{id}/status", requirePermission("finding:write", HandleUpdateFindingStatus(store)))
			r.Post("/findings/{id}/assign", requirePermission("finding:write", HandleAssignFinding(store)))
			r.Delete("/findings/{id}", requirePermission("finding:write", HandleDeleteFinding(store)))
			r.Get("/findings/{id}/comments", HandleListComments(store))
			r.Post("/findings/{id}/comments", HandleCreateComment(store))

			// Evidence
			r.Get("/evidence", HandleListEvidence(store))
			r.Post("/evidence/upload", requirePermission("vault:write", HandleCreateEvidence(store)))
			r.Get("/evidence/{id}", HandleGetEvidence(store))
			r.Delete("/evidence/{id}", requirePermission("vault:write", HandleDeleteEvidence(store)))

			// Reports
			r.Get("/reports", HandleListReports(store))
			r.Post("/reports", requirePermission("report:export", HandleCreateReport(store)))
			r.Get("/reports/{id}", HandleGetReport(store))
			r.Post("/reports/{id}/export", requirePermission("report:export", HandleExportReport(store)))
			r.Get("/reports/{id}/download", HandleReportDownload(store))

			// Activity
			r.Get("/activities", HandleListActivities(store))

			// Analytics
			r.Get("/analytics/findings-over-time", HandleAnalyticsFindingsOverTime(store))
			r.Get("/analytics/severity-count", HandleAnalyticsSeverityCount(store))
			r.Get("/analytics/status-count", HandleAnalyticsStatusCount(store))
			r.Get("/analytics/top-assets", HandleAnalyticsTopAssets(store))
			r.Get("/analytics/recurring-cwes", HandleAnalyticsRecurringCWEs(store))
			r.Get("/analytics/global-severity", HandleGlobalSeverity(store))
		})
	})

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","version":"1.0.0"}`))
	})

	// Static files (SPA)
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}

	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		name = strings.ReplaceAll(name, "\\", "/")

		f, err := staticFS.Open(name)
		if err != nil {
			// SPA fallback - serve index.html for client-side routes
			f, err = staticFiles.Open("static/index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			name = "index.html"
		}
		defer f.Close()
		stat, _ := f.Stat()
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		http.ServeContent(w, r, name, stat.ModTime(), f.(io.ReadSeeker))
	})

	addr := fmt.Sprintf(":%s", port)
	log.Printf("STITCH server starting on http://localhost%s", addr)
	log.Printf("Login credentials: admin@cyberteq.io / admin123")
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
