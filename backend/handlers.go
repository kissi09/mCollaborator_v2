package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func HandleLogin(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ApiResponse{
				Error: &ApiError{Code: "INVALID_REQUEST", Message: "Invalid request body"},
			})
			return
		}
		user, err := store.Authenticate(req.Email, req.Password)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, ApiResponse{
				Error: &ApiError{Code: "AUTH_FAILED", Message: "Invalid email or password"},
			})
			return
		}
		tokenBytes := make([]byte, 32)
		rand.Read(tokenBytes)
		token := hex.EncodeToString(tokenBytes)
		store.CreateSession(user.ID, token, time.Now().Add(24*time.Hour))

		store.AddAuditLog(&AuditLog{OrgID: user.OrgID, ActorID: user.ID, Action: "user.login", Resource: "user", ResourceID: user.ID, IPAddress: r.RemoteAddr})

		writeJSON(w, http.StatusOK, ApiResponse{Data: LoginResponse{Token: token, User: *user}})
	}
}

func HandleLogout(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		token := ""
		if len(authHeader) > 7 {
			token = authHeader[7:]
		}
		if token != "" {
			store.DeleteSession(token)
		}
		writeJSON(w, http.StatusOK, ApiResponse{Data: map[string]string{"message": "Logged out"}})
	}
}

func HandleGetMe(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		writeJSON(w, http.StatusOK, ApiResponse{Data: user})
	}
}

func HandleListUsers(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		users := store.ListUsers(user.OrgID)
		writeJSON(w, http.StatusOK, ApiResponse{Data: users})
	}
}

type createEngagementInput struct {
	Name        string          `json:"name"`
	ClientName  string          `json:"client_name"`
	Status      string          `json:"status"`
	Methodology string          `json:"methodology"`
	Scope       EngagementScope `json:"scope"`
	Timeline    Timeline        `json:"timeline"`
	Team        []string        `json:"team"`
}

func HandleListEngagements(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		status := r.URL.Query().Get("status")
		engagements := store.ListEngagements(user.OrgID, status)
		writeJSON(w, http.StatusOK, ApiResponse{Data: engagements})
	}
}

func HandleGetEngagement(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		eng, err := store.GetEngagement(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, ApiResponse{Error: &ApiError{Code: "NOT_FOUND", Message: "Engagement not found"}})
			return
		}
		writeJSON(w, http.StatusOK, ApiResponse{Data: eng})
	}
}

func HandleCreateEngagement(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		var input createEngagementInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, ApiResponse{Error: &ApiError{Code: "INVALID_REQUEST", Message: "Invalid request body"}})
			return
		}
		eng := &Engagement{
			OrgID: user.OrgID, Name: input.Name, ClientName: input.ClientName,
			Status: input.Status, Methodology: input.Methodology,
			Scope: input.Scope, Timeline: input.Timeline,
			Team: input.Team, CreatedBy: user.ID,
		}
		store.CreateEngagement(eng)
		store.AddActivity(&ActivityItem{OrgID: user.OrgID, UserID: user.ID, UserName: user.Name, Action: "created_engagement", Detail: fmt.Sprintf("Created %s", eng.Name), EngagementID: eng.ID})
		store.AddAuditLog(&AuditLog{OrgID: user.OrgID, ActorID: user.ID, Action: "engagement.create", Resource: "engagement", ResourceID: eng.ID, IPAddress: r.RemoteAddr})
		writeJSON(w, http.StatusCreated, ApiResponse{Data: eng})
	}
}

func HandleUpdateEngagement(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		id := chi.URLParam(r, "id")
		eng, err := store.GetEngagement(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, ApiResponse{Error: &ApiError{Code: "NOT_FOUND", Message: "Engagement not found"}})
			return
		}
		var input createEngagementInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, ApiResponse{Error: &ApiError{Code: "INVALID_REQUEST", Message: "Invalid request body"}})
			return
		}
		eng.Name = input.Name
		eng.ClientName = input.ClientName
		eng.Status = input.Status
		eng.Scope = input.Scope
		eng.Timeline = input.Timeline
		eng.Team = input.Team
		store.UpdateEngagement(eng)
		store.AddAuditLog(&AuditLog{OrgID: user.OrgID, ActorID: user.ID, Action: "engagement.update", Resource: "engagement", ResourceID: eng.ID, IPAddress: r.RemoteAddr})
		writeJSON(w, http.StatusOK, ApiResponse{Data: eng})
	}
}

func HandleDeleteEngagement(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		store.DeleteEngagement(id)
		writeJSON(w, http.StatusOK, ApiResponse{Data: map[string]string{"message": "Deleted"}})
	}
}

func HandleListNodes(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		engID := chi.URLParam(r, "id")
		nodes := store.ListNodes(engID)
		writeJSON(w, http.StatusOK, ApiResponse{Data: nodes})
	}
}

func HandleCreateNode(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		engID := chi.URLParam(r, "id")
		var input struct {
			Target string `json:"target"`
			Type   string `json:"type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, ApiResponse{Error: &ApiError{Code: "INVALID_REQUEST", Message: "Invalid request"}})
			return
		}
		node := &Node{EngagementID: engID, Target: input.Target, Type: input.Type}
		store.CreateNode(node)
		writeJSON(w, http.StatusCreated, ApiResponse{Data: node})
	}
}

type createFindingInput struct {
	NodeID        string   `json:"node_id,omitempty"`
	Title         string   `json:"title"`
	CVE           string   `json:"cve,omitempty"`
	CWEs          []string `json:"cwes,omitempty"`
	MitreAttackIDs []string `json:"mitre_attack_ids,omitempty"`
	Severity      string   `json:"severity"`
	CVSSVector    string   `json:"cvss_vector,omitempty"`
	CVSSScore     float64  `json:"cvss_score"`
	Status        string   `json:"status"`
	Description   string   `json:"description"`
	POC           string   `json:"poc,omitempty"`
	Remediation   string   `json:"remediation,omitempty"`
	Impact        string   `json:"impact,omitempty"`
	Likelihood    string   `json:"likelihood,omitempty"`
	AssignedTo    string   `json:"assigned_to,omitempty"`
}

func HandleListFindings(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		engID := chi.URLParam(r, "id")
		nodeID := r.URL.Query().Get("node_id")
		severity := r.URL.Query().Get("severity")
		status := r.URL.Query().Get("status")
		findings := store.ListFindings(engID, nodeID, severity, status)
		writeJSON(w, http.StatusOK, ApiResponse{Data: findings})
	}
}

func HandleGetFinding(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		finding, err := store.GetFinding(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, ApiResponse{Error: &ApiError{Code: "NOT_FOUND", Message: "Finding not found"}})
			return
		}
		writeJSON(w, http.StatusOK, ApiResponse{Data: finding})
	}
}

func HandleCreateFinding(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		engID := chi.URLParam(r, "id")
		var input createFindingInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, ApiResponse{Error: &ApiError{Code: "INVALID_REQUEST", Message: "Invalid request"}})
			return
		}
		finding := &Finding{
			EngagementID: engID, NodeID: input.NodeID, Title: input.Title,
			CVE: input.CVE, CWEs: input.CWEs, MitreAttackIDs: input.MitreAttackIDs,
			Severity: input.Severity, CVSSVector: input.CVSSVector, CVSSScore: input.CVSSScore,
			Status: input.Status, Description: input.Description, POC: input.POC,
			Remediation: input.Remediation, Impact: input.Impact, Likelihood: input.Likelihood,
			AssignedTo: input.AssignedTo, CreatedBy: user.ID,
		}
		store.CreateFinding(finding)
		store.AddActivity(&ActivityItem{OrgID: user.OrgID, UserID: user.ID, UserName: user.Name, Action: "added_finding", Detail: fmt.Sprintf("Added %s finding: %s", finding.Severity, finding.Title), EngagementID: engID})
		store.AddAuditLog(&AuditLog{OrgID: user.OrgID, ActorID: user.ID, Action: "finding.create", Resource: "finding", ResourceID: finding.ID, IPAddress: r.RemoteAddr})
		writeJSON(w, http.StatusCreated, ApiResponse{Data: finding})
	}
}

func HandleUpdateFinding(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		id := chi.URLParam(r, "id")
		finding, err := store.GetFinding(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, ApiResponse{Error: &ApiError{Code: "NOT_FOUND", Message: "Finding not found"}})
			return
		}
		var input createFindingInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, ApiResponse{Error: &ApiError{Code: "INVALID_REQUEST", Message: "Invalid request"}})
			return
		}
		finding.NodeID = input.NodeID
		finding.Title = input.Title
		finding.CVE = input.CVE
		finding.CWEs = input.CWEs
		finding.MitreAttackIDs = input.MitreAttackIDs
		finding.Severity = input.Severity
		finding.CVSSVector = input.CVSSVector
		finding.CVSSScore = input.CVSSScore
		finding.Status = input.Status
		finding.Description = input.Description
		finding.POC = input.POC
		finding.Remediation = input.Remediation
		finding.Impact = input.Impact
		finding.Likelihood = input.Likelihood
		finding.AssignedTo = input.AssignedTo
		store.UpdateFinding(finding)
		store.AddAuditLog(&AuditLog{OrgID: user.OrgID, ActorID: user.ID, Action: "finding.update", Resource: "finding", ResourceID: finding.ID, IPAddress: r.RemoteAddr})
		writeJSON(w, http.StatusOK, ApiResponse{Data: finding})
	}
}

func HandleUpdateFindingStatus(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		id := chi.URLParam(r, "id")
		finding, err := store.GetFinding(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, ApiResponse{Error: &ApiError{Code: "NOT_FOUND", Message: "Finding not found"}})
			return
		}
		var input struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, ApiResponse{Error: &ApiError{Code: "INVALID_REQUEST", Message: "Invalid request"}})
			return
		}
		finding.Status = input.Status
		store.UpdateFinding(finding)
		store.AddActivity(&ActivityItem{OrgID: user.OrgID, UserID: user.ID, UserName: user.Name, Action: "updated_status", Detail: fmt.Sprintf("Updated %s status to %s", finding.Title, input.Status), EngagementID: finding.EngagementID})
		store.AddAuditLog(&AuditLog{OrgID: user.OrgID, ActorID: user.ID, Action: "finding.status_change", Resource: "finding", ResourceID: finding.ID, IPAddress: r.RemoteAddr, Diff: fmt.Sprintf(`{"new_status": "%s", "reason": "%s"}`, input.Status, input.Reason)})
		writeJSON(w, http.StatusOK, ApiResponse{Data: finding})
	}
}

func HandleAssignFinding(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		id := chi.URLParam(r, "id")
		var input struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, ApiResponse{Error: &ApiError{Code: "INVALID_REQUEST", Message: "Invalid request"}})
			return
		}
		if err := store.AssignFinding(id, input.UserID); err != nil {
			writeJSON(w, http.StatusNotFound, ApiResponse{Error: &ApiError{Code: "NOT_FOUND", Message: err.Error()}})
			return
		}
		finding, _ := store.GetFinding(id)
		store.AddAuditLog(&AuditLog{OrgID: user.OrgID, ActorID: user.ID, Action: "finding.assign", Resource: "finding", ResourceID: id, IPAddress: r.RemoteAddr})
		writeJSON(w, http.StatusOK, ApiResponse{Data: finding})
	}
}

func HandleDeleteFinding(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		store.DeleteFinding(id)
		writeJSON(w, http.StatusOK, ApiResponse{Data: map[string]string{"message": "Deleted"}})
	}
}

func HandleCreateEvidence(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		r.ParseMultipartForm(500 << 20)
		file, handler, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ApiResponse{Error: &ApiError{Code: "INVALID_REQUEST", Message: "File required"}})
			return
		}
		defer file.Close()
		findingID := r.FormValue("finding_id")
		engagementID := r.FormValue("engagement_id")
		tags := r.FormValue("tags")
		desc := r.FormValue("description")

		storageKey := fmt.Sprintf("uploads/%s_%s", uuid.New().String(), handler.Filename)
		ev := &Evidence{
			EngagementID: engagementID, FindingID: findingID, Filename: handler.Filename,
			StorageKey: storageKey, MimeType: handler.Header.Get("Content-Type"),
			SizeBytes: handler.Size, HashSHA256: fmt.Sprintf("%x", make([]byte, 32)),
			Tags: tags, Description: desc, UploadedBy: user.ID,
		}
		store.CreateEvidence(ev)
		store.AddActivity(&ActivityItem{OrgID: user.OrgID, UserID: user.ID, UserName: user.Name, Action: "uploaded_evidence", Detail: fmt.Sprintf("Uploaded %s", handler.Filename), EngagementID: engagementID})
		store.AddAuditLog(&AuditLog{OrgID: user.OrgID, ActorID: user.ID, Action: "evidence.upload", Resource: "evidence", ResourceID: ev.ID, IPAddress: r.RemoteAddr})
		writeJSON(w, http.StatusCreated, ApiResponse{Data: ev})
	}
}

func HandleListEvidence(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		engID := r.URL.Query().Get("engagement_id")
		findingID := r.URL.Query().Get("finding_id")
		evidence := store.ListEvidence(engID, findingID)
		writeJSON(w, http.StatusOK, ApiResponse{Data: evidence})
	}
}

func HandleGetEvidence(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		ev, err := store.GetEvidence(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, ApiResponse{Error: &ApiError{Code: "NOT_FOUND", Message: "Evidence not found"}})
			return
		}
		writeJSON(w, http.StatusOK, ApiResponse{Data: ev})
	}
}

func HandleDeleteEvidence(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		store.DeleteEvidence(id)
		writeJSON(w, http.StatusOK, ApiResponse{Data: map[string]string{"message": "Deleted"}})
	}
}

func HandleCreateReport(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		var input struct {
			EngagementID string `json:"engagement_id"`
			Title        string `json:"title"`
			Template     string `json:"template"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, ApiResponse{Error: &ApiError{Code: "INVALID_REQUEST", Message: "Invalid request"}})
			return
		}
		report := &Report{
			EngagementID: input.EngagementID, Title: input.Title,
			Template: input.Template, Classification: "Confidential",
			Blocks: []ReportBlock{}, CreatedBy: user.ID, Status: "draft",
		}
		store.CreateReport(report)
		store.AddAuditLog(&AuditLog{OrgID: user.OrgID, ActorID: user.ID, Action: "report.create", Resource: "report", ResourceID: report.ID, IPAddress: r.RemoteAddr})
		writeJSON(w, http.StatusCreated, ApiResponse{Data: report})
	}
}

func HandleListReports(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		engID := r.URL.Query().Get("engagement_id")
		reports := store.ListReports(engID)
		writeJSON(w, http.StatusOK, ApiResponse{Data: reports})
	}
}

func HandleGetReport(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		report, err := store.GetReport(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, ApiResponse{Error: &ApiError{Code: "NOT_FOUND", Message: "Report not found"}})
			return
		}
		writeJSON(w, http.StatusOK, ApiResponse{Data: report})
	}
}

func HandleExportReport(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		id := chi.URLParam(r, "id")
		report, err := store.GetReport(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, ApiResponse{Error: &ApiError{Code: "NOT_FOUND", Message: "Report not found"}})
			return
		}
		report.Status = "exported"
		store.UpdateReport(report)
		store.AddAuditLog(&AuditLog{OrgID: user.OrgID, ActorID: user.ID, Action: "report.export", Resource: "report", ResourceID: id, IPAddress: r.RemoteAddr})
		writeJSON(w, http.StatusOK, ApiResponse{Data: map[string]string{
			"job_id": uuid.New().String(), "status": "completed", "download_url": fmt.Sprintf("/api/v1/reports/%s/download", id),
		}})
	}
}

func HandleReportDownload(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		report, err := store.GetReport(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, ApiResponse{Error: &ApiError{Code: "NOT_FOUND", Message: "Report not found"}})
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.pdf", report.Title))
		fmt.Fprintf(w, "mCollaborator Report: %s\nEngagement: %s\nTemplate: %s\nBlocks: %d", report.Title, report.EngagementID, report.Template, len(report.Blocks))
	}
}

func HandleListActivities(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		activities := store.ListActivities(user.OrgID)
		writeJSON(w, http.StatusOK, ApiResponse{Data: activities})
	}
}

func HandleAnalyticsFindingsOverTime(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := store.FindingsOverTime("org_001")
		writeJSON(w, http.StatusOK, ApiResponse{Data: data})
	}
}

func HandleAnalyticsSeverityCount(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		data := store.FindingCountBySeverity(user.OrgID)
		writeJSON(w, http.StatusOK, ApiResponse{Data: data})
	}
}

func HandleAnalyticsStatusCount(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		data := store.FindingStatusCount(user.OrgID)
		writeJSON(w, http.StatusOK, ApiResponse{Data: data})
	}
}

func HandleAnalyticsTopAssets(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		data := store.TopVulnerableAssets(user.OrgID, 5)
		writeJSON(w, http.StatusOK, ApiResponse{Data: data})
	}
}

func HandleAnalyticsRecurringCWEs(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		data := store.RecurringCWEs(user.OrgID, 1)
		writeJSON(w, http.StatusOK, ApiResponse{Data: data})
	}
}

func HandleGlobalSeverity(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, ApiResponse{Data: store.ListAllFindingSeverities()})
	}
}

func HandleListComments(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		findingID := chi.URLParam(r, "id")
		comments := store.ListComments(findingID)
		writeJSON(w, http.StatusOK, ApiResponse{Data: comments})
	}
}

func HandleCreateComment(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*User)
		findingID := chi.URLParam(r, "id")
		var input struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, ApiResponse{Error: &ApiError{Code: "INVALID_REQUEST", Message: "Invalid request"}})
			return
		}
		comment := &Comment{FindingID: findingID, UserID: user.ID, UserName: user.Name, Body: input.Body}
		store.AddComment(comment)
		writeJSON(w, http.StatusCreated, ApiResponse{Data: comment})
	}
}
