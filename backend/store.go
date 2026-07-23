package main

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
	"github.com/google/uuid"
)

type Store struct {
	mu          sync.RWMutex
	orgs        map[string]*Org
	users       map[string]*User
	usersByEmail map[string]*User
	engagements map[string]*Engagement
	nodes       map[string]*Node
	findings    map[string]*Finding
	evidence    map[string]*Evidence
	reports     map[string]*Report
	auditLog    []*AuditLog
	sessions    map[string]*Session
	comments    map[string]*Comment
	activities  []*ActivityItem
}

func NewStore() *Store {
	s := &Store{
		orgs:         make(map[string]*Org),
		users:        make(map[string]*User),
		usersByEmail: make(map[string]*User),
		engagements:  make(map[string]*Engagement),
		nodes:        make(map[string]*Node),
		findings:     make(map[string]*Finding),
		evidence:     make(map[string]*Evidence),
		reports:      make(map[string]*Report),
		auditLog:     make([]*AuditLog, 0),
		sessions:     make(map[string]*Session),
		comments:     make(map[string]*Comment),
		activities:   make([]*ActivityItem, 0),
	}
	s.seed()
	return s
}

func (s *Store) seed() {
	now := time.Now().UTC().Format(time.RFC3339)

	orgID := "org_001"
	s.orgs[orgID] = &Org{ID: orgID, Name: "Cyberteq Falcon", Slug: "cyberteq-falcon", CreatedAt: now, UpdatedAt: now}

	passwordHash := hashPassword("admin123")
	userID := "usr_001"
	s.users[userID] = &User{
		ID: userID, OrgID: orgID, Email: "admin@cyberteq.io", Name: "A. Jensen",
		PasswordHash: passwordHash, Role: "admin", MFAEnabled: false,
		Permissions: []string{"finding:write", "vault:read", "vault:write", "report:export", "admin:users", "engagements:write"},
		CreatedAt: now,
	}
	s.usersByEmail["admin@cyberteq.io"] = s.users[userID]

	userID2 := "usr_002"
	s.users[userID2] = &User{
		ID: userID2, OrgID: orgID, Email: "analyst@cyberteq.io", Name: "Sarah Jenkins",
		PasswordHash: passwordHash, Role: "analyst", MFAEnabled: false,
		Permissions: []string{"finding:write", "vault:read", "vault:write", "report:export"},
		CreatedAt: now,
	}
	s.usersByEmail["analyst@cyberteq.io"] = s.users[userID2]

	userID3 := "usr_003"
	s.users[userID3] = &User{
		ID: userID3, OrgID: orgID, Email: "client@acme.io", Name: "John Client",
		PasswordHash: passwordHash, Role: "client", MFAEnabled: false,
		Permissions: []string{"vault:read", "report:export"},
		CreatedAt: now,
	}
	s.usersByEmail["client@acme.io"] = s.users[userID3]

	engID := "eng_001"
	s.engagements[engID] = &Engagement{
		ID: engID, OrgID: orgID, Name: "Project Alpha - External Network",
		ClientName: "Apex Financial", Status: "in_progress", Methodology: "owasp",
		Scope: EngagementScope{
			Included: []string{"10.0.0.0/24", "api.apex.example.com"},
			Excluded: []string{"10.0.0.100"},
		},
		Timeline: Timeline{StartDate: "2025-10-01", EndDate: "2025-10-31"},
		Team: []string{userID, userID2}, CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
	}

	engID2 := "eng_002"
	s.engagements[engID2] = &Engagement{
		ID: engID2, OrgID: orgID, Name: "Internal API Security Review",
		ClientName: "Cyberteq Internal", Status: "review", Methodology: "ptes",
		Scope: EngagementScope{
			Included: []string{"api.internal.cyberteq.com/v2/*"},
		},
		Timeline: Timeline{StartDate: "2025-10-15", EndDate: "2025-11-15"},
		Team: []string{userID, userID2}, CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
	}

	engID3 := "eng_003"
	s.engagements[engID3] = &Engagement{
		ID: engID3, OrgID: orgID, Name: "Cloud Infrastructure Audit",
		ClientName: "Globex Corp", Status: "planning", Methodology: "nist",
		Scope: EngagementScope{
			Included: []string{"aws:prod/account-123456"},
		},
		Timeline: Timeline{StartDate: "2025-11-01", EndDate: "2025-12-15"},
		Team: []string{userID}, CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
	}

	engID4 := "eng_004"
	s.engagements[engID4] = &Engagement{
		ID: engID4, OrgID: orgID, Name: "Q3 Compliance Audit",
		ClientName: "FinCorp Ltd.", Status: "closed", Methodology: "custom",
		Scope: EngagementScope{
			Included: []string{"10.20.0.0/16"},
		},
		Timeline: Timeline{StartDate: "2025-07-01", EndDate: "2025-09-30"},
		Team: []string{userID2}, CreatedBy: userID2, CreatedAt: "2025-07-01T00:00:00Z", UpdatedAt: "2025-09-30T00:00:00Z",
	}

	engagements := []string{engID, engID2, engID3, engID4}

	for i, eid := range engagements {
		if i > 0 { continue }
		nodeID := fmt.Sprintf("node_%s_001", eid)
		s.nodes[nodeID] = &Node{ID: nodeID, EngagementID: eid, Target: "10.0.0.15", Type: "ip", CreatedAt: now}
		nodeID2 := fmt.Sprintf("node_%s_002", eid)
		s.nodes[nodeID2] = &Node{ID: nodeID2, EngagementID: eid, Target: "10.0.0.42", Type: "ip", CreatedAt: now}
		nodeID3 := fmt.Sprintf("node_%s_003", eid)
		s.nodes[nodeID3] = &Node{ID: nodeID3, EngagementID: eid, Target: "api.apex.example.com", Type: "domain", CreatedAt: now}

		f1ID := fmt.Sprintf("fid_%s_001", eid)
		s.findings[f1ID] = &Finding{
			ID: f1ID, EngagementID: eid, NodeID: fmt.Sprintf("node_%s_001", eid),
			Title: "SQL Injection in User Authentication Endpoint", CVE: "CVE-2023-28103",
			CWEs: []string{"CWE-89"}, Severity: "critical", CVSSVector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H",
			CVSSScore: 9.8, Status: "open",
			Description: "The application fails to sanitize user-supplied input in the username parameter of the login endpoint before incorporating it into a database query. This allows an unauthenticated, remote attacker to execute arbitrary SQL commands, potentially leading to unauthorized access, data exfiltration, or complete database compromise.",
			POC: "POST /v1/auth/login HTTP/1.1\nHost: api.example.com\nContent-Type: application/json\n\n{\n  \"username\": \"admin' OR '1'='1\",\n  \"password\": \"anything\"\n}",
			Remediation: "Use parameterized queries with prepared statements. Never concatenate user input directly into SQL queries. Implement strict input validation and use a WAF as a defense-in-depth measure.",
			Impact: "Complete database compromise, data exfiltration, unauthorized access.",
			Likelihood: "high", AssignedTo: userID, CreatedBy: userID, Version: 1, CreatedAt: now, UpdatedAt: now,
		}

		f2ID := fmt.Sprintf("fid_%s_002", eid)
		s.findings[f2ID] = &Finding{
			ID: f2ID, EngagementID: eid, NodeID: fmt.Sprintf("node_%s_001", eid),
			Title: "Remote Code Execution via File Upload", CVE: "CVE-2023-38204",
			CWEs: []string{"CWE-78"}, Severity: "critical", CVSSScore: 9.1,
			Status: "open", Description: "The file upload endpoint does not validate file contents, allowing an attacker to upload a malicious PHP file and execute arbitrary commands on the server.",
			POC: "POST /v1/media/upload HTTP/1.1\nContent-Type: multipart/form-data\n\nshell.php containing system($_GET['cmd'])",
			Remediation: "Validate file contents by magic bytes, not extension. Store uploads outside webroot. Use a sandboxed processing pipeline.",
			AssignedTo: userID2, CreatedBy: userID, Version: 1, CreatedAt: now, UpdatedAt: now,
		}

		f3ID := fmt.Sprintf("fid_%s_003", eid)
		s.findings[f3ID] = &Finding{
			ID: f3ID, EngagementID: eid, NodeID: fmt.Sprintf("node_%s_002", eid),
			Title: "Cross-Site Scripting (XSS) in Dashboard Search",
			CWEs: []string{"CWE-79"}, Severity: "high", CVSSScore: 8.5,
			Status: "open", Description: "The search parameter on the dashboard is reflected in the page without proper encoding, enabling stored XSS attacks.",
			Remediation: "Encode all user-supplied data before rendering. Implement Content Security Policy headers. Use DOMPurify on the frontend.",
			AssignedTo: userID2, CreatedBy: userID2, Version: 1, CreatedAt: now, UpdatedAt: now,
		}

		f4ID := fmt.Sprintf("fid_%s_004", eid)
		s.findings[f4ID] = &Finding{
			ID: f4ID, EngagementID: eid, NodeID: fmt.Sprintf("node_%s_001", eid),
			Title: "Insecure Direct Object Reference (IDOR) in User Profile",
			CWEs: []string{"CWE-639"}, Severity: "high", CVSSScore: 7.2,
			Status: "open", Description: "The /api/v1/users/profile/{id} endpoint does not verify that the requesting user owns the profile being accessed, allowing any authenticated user to view and modify other users' profiles.",
			Remediation: "Implement proper authorization checks. Use UUIDs instead of sequential IDs. Verify ownership before returning data.",
			AssignedTo: userID, CreatedBy: userID2, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
	}

	for _, eid := range engagements {
		if eid == engID2 {
			f5ID := fmt.Sprintf("fid_%s_001", eid)
			s.findings[f5ID] = &Finding{
				ID: f5ID, EngagementID: eid,
				Title: "Misconfigured S3 Bucket Allowing Public Access",
				CWEs: []string{"CWE-284"}, Severity: "critical", CVSSScore: 9.0,
				Status: "open", Description: "An S3 bucket containing sensitive customer data has public read access enabled due to a misconfigured bucket policy.",
				Remediation: "Block all public access at the bucket level. Use S3 Block Public Access features. Audit bucket policies regularly.",
				AssignedTo: userID, CreatedBy: userID, Version: 1, CreatedAt: now, UpdatedAt: now,
			}
			f6ID := fmt.Sprintf("fid_%s_002", eid)
			s.findings[f6ID] = &Finding{
				ID: f6ID, EngagementID: eid,
				Title: "Weak TLS Configuration on API Gateway",
				Severity: "medium", CVSSScore: 5.6,
				Status: "mitigated", Description: "The API gateway allows TLS 1.0 and 1.1 connections and uses weak cipher suites.",
				Remediation: "Disable TLS 1.0/1.1. Restrict cipher suites to TLS_AES_256_GCM_SHA384 and TLS_CHACHA20_POLY1305_SHA256.",
				AssignedTo: userID2, CreatedBy: userID2, Version: 1, CreatedAt: now, UpdatedAt: now,
			}
		}
	}

	s.evidence["ev_001"] = &Evidence{
		ID: "ev_001", EngagementID: engID, FindingID: "fid_eng_001_001",
		Filename: "auth_bypass_poc.png", StorageKey: "uploads/auth_bypass_poc.png",
		MimeType: "image/png", SizeBytes: 1245032,
		HashSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("mock"))),
		Tags: "#auth #poc #screenshot", Description: "Screenshot showing successful admin panel access without valid session token.",
		UploadedBy: userID2, CreatedAt: now,
	}

	s.evidence["ev_002"] = &Evidence{
		ID: "ev_002", EngagementID: engID, FindingID: "fid_eng_001_001",
		Filename: "nmap_scan_results.xml", StorageKey: "uploads/nmap_scan_results.xml",
		MimeType: "application/xml", SizeBytes: 45800000,
		HashSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("mock2"))),
		Tags: "#nmap #scan", Description: "Nmap scan results for target subnet 10.0.0.0/24.",
		UploadedBy: userID, CreatedAt: now,
	}

	s.evidence["ev_003"] = &Evidence{
		ID: "ev_003", EngagementID: engID,
		Filename: "network_capture_vlan2.pcap", StorageKey: "uploads/network_capture_vlan2.pcap",
		MimeType: "application/vnd.tcpdump.pcap", SizeBytes: 128400000,
		HashSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("mock3"))),
		Tags: "#pcap #wireshark", Description: "PCAP capture of VLAN 2 network traffic.",
		UploadedBy: "usr_002", CreatedAt: now,
	}

	s.reports["rpt_001"] = &Report{
		ID: "rpt_001", EngagementID: engID, Title: "Project Alpha - Final Assessment Report",
		Template: "executive", Classification: "Confidential - Client Eyes Only",
		Blocks: []ReportBlock{
			{Type: "heading", ID: "blk_1", Content: "Executive Summary", Order: 0},
			{Type: "text", ID: "blk_2", Content: "This report outlines the findings from the comprehensive penetration test conducted on Project Alpha infrastructure. The assessment revealed several critical vulnerabilities that require immediate remediation.", Order: 1},
			{Type: "finding", ID: "blk_3", Content: "fid_eng_001_001", Order: 2},
			{Type: "finding", ID: "blk_4", Content: "fid_eng_001_002", Order: 3},
		},
		CreatedBy: userID, Status: "draft", Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	s.activities = append(s.activities,
		&ActivityItem{ID: uuid.New().String(), OrgID: orgID, UserID: userID, UserName: "A. Jensen", Action: "created_engagement", Detail: "Created Project Alpha - External Network", EngagementID: engID, CreatedAt: now},
		&ActivityItem{ID: uuid.New().String(), OrgID: orgID, UserID: userID2, UserName: "Sarah Jenkins", Action: "added_finding", Detail: "Added Critical finding to Project Alpha", EngagementID: engID, CreatedAt: now},
		&ActivityItem{ID: uuid.New().String(), OrgID: orgID, UserID: userID, UserName: "A. Jensen", Action: "uploaded_evidence", Detail: "Uploaded auth_bypass_poc.png", EngagementID: engID, CreatedAt: now},
		&ActivityItem{ID: uuid.New().String(), OrgID: orgID, UserID: userID2, UserName: "Sarah Jenkins", Action: "updated_finding", Detail: "Updated status of Weak TLS to mitigated", EngagementID: engID, CreatedAt: now},
	)
}

func hashPassword(password string) string {
	h := sha256.Sum256([]byte("stitch_" + password + "_salt"))
	return fmt.Sprintf("%x", h)
}

// User operations
func (s *Store) Authenticate(email, password string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.usersByEmail[email]
	if !ok {
		return nil, fmt.Errorf("invalid credentials")
	}
	if user.PasswordHash != hashPassword(password) {
		return nil, fmt.Errorf("invalid credentials")
	}
	return user, nil
}

func (s *Store) GetUser(id string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[id]
	if !ok {
		return nil, fmt.Errorf("user not found")
	}
	return user, nil
}

func (s *Store) CreateSession(userID, token string, expiresAt time.Time) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := &Session{
		ID: uuid.New().String(), UserID: userID, Token: token,
		ExpiresAt: expiresAt, CreatedAt: time.Now(),
	}
	s.sessions[session.ID] = session
	return session
}

func (s *Store) GetSession(token string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, session := range s.sessions {
		if session.Token == token && time.Now().Before(session.ExpiresAt) {
			return session, nil
		}
	}
	return nil, fmt.Errorf("invalid or expired session")
}

func (s *Store) DeleteSession(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, session := range s.sessions {
		if session.Token == token {
			delete(s.sessions, id)
			return
		}
	}
}

func (s *Store) ListUsers(orgID string) []*User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*User
	for _, u := range s.users {
		if u.OrgID == orgID {
			result = append(result, u)
		}
	}
	return result
}

// Engagement operations
func (s *Store) ListEngagements(orgID, status string) []*Engagement {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Engagement
	for _, e := range s.engagements {
		if e.OrgID == orgID {
			if status == "" || e.Status == status {
				result = append(result, e)
			}
		}
	}
	return result
}

func (s *Store) GetEngagement(id string) (*Engagement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.engagements[id]
	if !ok {
		return nil, fmt.Errorf("engagement not found")
	}
	return e, nil
}

func (s *Store) CreateEngagement(e *Engagement) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.ID = uuid.New().String()
	e.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	e.UpdatedAt = e.CreatedAt
	s.engagements[e.ID] = e
}

func (s *Store) UpdateEngagement(e *Engagement) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.engagements[e.ID] = e
}

func (s *Store) DeleteEngagement(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.engagements, id)
}

// Node operations
func (s *Store) ListNodes(engagementID string) []*Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Node
	for _, n := range s.nodes {
		if n.EngagementID == engagementID {
			result = append(result, n)
		}
	}
	return result
}

func (s *Store) CreateNode(n *Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n.ID = uuid.New().String()
	n.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	s.nodes[n.ID] = n
}

func (s *Store) ListFindings(engagementID, nodeID, severity, status string) []*Finding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Finding
	for _, f := range s.findings {
		if f.EngagementID == engagementID {
			if nodeID != "" && f.NodeID != nodeID { continue }
			if severity != "" && f.Severity != severity { continue }
			if status != "" && f.Status != status { continue }
			result = append(result, f)
		}
	}
	return result
}

func (s *Store) GetFinding(id string) (*Finding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.findings[id]
	if !ok {
		return nil, fmt.Errorf("finding not found")
	}
	return f, nil
}

func (s *Store) CreateFinding(f *Finding) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f.ID = uuid.New().String()
	f.Version = 1
	now := time.Now().UTC().Format(time.RFC3339)
	f.CreatedAt = now
	f.UpdatedAt = now
	s.findings[f.ID] = f
}

func (s *Store) UpdateFinding(f *Finding) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f.Version++
	f.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.findings[f.ID] = f
}

func (s *Store) DeleteFinding(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.findings, id)
}

func (s *Store) AssignFinding(findingID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.findings[findingID]
	if !ok {
		return fmt.Errorf("finding not found")
	}
	f.AssignedTo = userID
	f.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// Evidence operations
func (s *Store) ListEvidence(engagementID, findingID string) []*Evidence {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Evidence
	for _, ev := range s.evidence {
		if engagementID != "" && ev.EngagementID != engagementID { continue }
		if findingID != "" && ev.FindingID != findingID { continue }
		result = append(result, ev)
	}
	return result
}

func (s *Store) GetEvidence(id string) (*Evidence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ev, ok := s.evidence[id]
	if !ok {
		return nil, fmt.Errorf("evidence not found")
	}
	return ev, nil
}

func (s *Store) CreateEvidence(ev *Evidence) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev.ID = uuid.New().String()
	ev.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	s.evidence[ev.ID] = ev
}

func (s *Store) DeleteEvidence(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.evidence, id)
}

// Report operations
func (s *Store) ListReports(engagementID string) []*Report {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Report
	for _, r := range s.reports {
		if r.EngagementID == engagementID {
			result = append(result, r)
		}
	}
	return result
}

func (s *Store) GetReport(id string) (*Report, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.reports[id]
	if !ok {
		return nil, fmt.Errorf("report not found")
	}
	return r, nil
}

func (s *Store) CreateReport(r *Report) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.ID = uuid.New().String()
	r.Version = 1
	now := time.Now().UTC().Format(time.RFC3339)
	r.CreatedAt = now
	r.UpdatedAt = now
	s.reports[r.ID] = r
}

func (s *Store) UpdateReport(r *Report) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.Version++
	r.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.reports[r.ID] = r
}

// Activity operations
func (s *Store) ListActivities(orgID string) []*ActivityItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*ActivityItem
	for i := len(s.activities) - 1; i >= 0; i-- {
		a := s.activities[i]
		if a.OrgID == orgID {
			result = append(result, a)
		}
	}
	if len(result) > 50 {
		result = result[:50]
	}
	return result
}

func (s *Store) AddActivity(a *ActivityItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a.ID = uuid.New().String()
	a.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	s.activities = append(s.activities, a)
}

// Audit log
func (s *Store) AddAuditLog(entry *AuditLog) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry.ID = uuid.New().String()
	entry.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	s.auditLog = append(s.auditLog, entry)
}

// Comments
func (s *Store) ListComments(findingID string) []*Comment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Comment
	for _, c := range s.comments {
		if c.FindingID == findingID {
			result = append(result, c)
		}
	}
	return result
}

func (s *Store) AddComment(c *Comment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.ID = uuid.New().String()
	c.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	s.comments[c.ID] = c
}

// Analytics helpers
func (s *Store) FindingCountBySeverity(orgID string) map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0, "info": 0}
	engMap := make(map[string]bool)
	for _, e := range s.engagements {
		if e.OrgID == orgID {
			engMap[e.ID] = true
		}
	}
	for _, f := range s.findings {
		if engMap[f.EngagementID] {
			result[f.Severity]++
		}
	}
	return result
}

func (s *Store) FindingStatusCount(orgID string) map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := map[string]int{}
	engMap := make(map[string]bool)
	for _, e := range s.engagements {
		if e.OrgID == orgID {
			engMap[e.ID] = true
		}
	}
	for _, f := range s.findings {
		if engMap[f.EngagementID] {
			result[f.Status]++
		}
	}
	return result
}

func (s *Store) TopVulnerableAssets(orgID string, limit int) []map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	assetCount := make(map[string]int)
	engMap := make(map[string]bool)
	for _, e := range s.engagements {
		if e.OrgID == orgID {
			engMap[e.ID] = true
		}
	}
	for _, f := range s.findings {
		if engMap[f.EngagementID] && f.NodeID != "" {
			assetCount[f.NodeID]++
		}
	}
	var result []map[string]interface{}
	for nodeID, count := range assetCount {
		if n, ok := s.nodes[nodeID]; ok {
			result = append(result, map[string]interface{}{
				"target": n.Target, "count": count,
			})
		}
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func (s *Store) RecurringCWEs(orgID string, minCount int) []map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cweCount := make(map[string]int)
	engMap := make(map[string]bool)
	for _, e := range s.engagements {
		if e.OrgID == orgID {
			engMap[e.ID] = true
		}
	}
	for _, f := range s.findings {
		if engMap[f.EngagementID] {
			for _, cwe := range f.CWEs {
				cweCount[cwe]++
			}
		}
	}
	var result []map[string]interface{}
	for cwe, count := range cweCount {
		if count >= minCount {
			result = append(result, map[string]interface{}{
				"cwe": cwe, "count": count,
			})
		}
	}
	return result
}

func (s *Store) FindingsOverTime(orgID string) []map[string]interface{} {
	return []map[string]interface{}{
		{"month": "2025-01", "critical": 2, "high": 5, "medium": 8, "low": 12},
		{"month": "2025-02", "critical": 1, "high": 3, "medium": 6, "low": 10},
		{"month": "2025-03", "critical": 4, "high": 7, "medium": 9, "low": 15},
		{"month": "2025-04", "critical": 3, "high": 6, "medium": 11, "low": 14},
		{"month": "2025-05", "critical": 5, "high": 8, "medium": 10, "low": 18},
		{"month": "2025-06", "critical": 2, "high": 4, "medium": 7, "low": 9},
	}
}

func (s *Store) ListAllFindingSeverities() map[string]int {
	result := map[string]int{}
	for _, f := range s.findings {
		result[f.Severity]++
	}
	return result
}
