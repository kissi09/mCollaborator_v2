package main

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type Store struct {
	mu           sync.RWMutex
	db           *StoreDB
	orgs         map[string]*Org
	users        map[string]*User
	usersByEmail map[string]*User
	engagements  map[string]*Engagement
	nodes        map[string]*Node
	findings     map[string]*Finding
	evidence     map[string]*Evidence
	reports      map[string]*Report
	reportRecords map[string]*ReportRecord
	auditLog     []*AuditLog
	sessions     map[string]*Session
	comments     map[string]*Comment
	activities   []*ActivityItem
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
		reportRecords: make(map[string]*ReportRecord),
		auditLog:      make([]*AuditLog, 0),
		sessions:     make(map[string]*Session),
		comments:     make(map[string]*Comment),
		activities:   make([]*ActivityItem, 0),
	}

	// Open the SQLite persistence layer. If a snapshot already exists, load it
	// (the data survives restarts); otherwise seed demo data and persist it.
	dbPath := os.Getenv("STITCH_DB_PATH")
	if dbPath == "" {
		dbPath = filepath.Join("data", "mcollaborator.db")
	}
	if db, err := openStoreDB(dbPath); err != nil {
		log.Printf("persistence disabled: %v", err)
	} else {
		s.db = db
		loaded, err := s.Load()
		if err != nil {
			log.Printf("persistence load failed: %v", err)
		}
		if !loaded {
			s.seed()
			if err := s.Persist(); err != nil {
				log.Printf("persistence seed write failed: %v", err)
			}
		}
	}

	return s
}

func (s *Store) seed() {
	now := time.Now().UTC().Format(time.RFC3339)

	// Fresh installs start empty except for the org and a single bootstrap admin.
	// Analysts, projects, findings, and evidence are created by the team during
	// real engagements — no demo data is injected.
	orgID := "org_001"
	s.orgs[orgID] = &Org{ID: orgID, Name: "Cyberteq Falcon", Slug: "cyberteq-falcon", CreatedAt: now, UpdatedAt: now}

	bcryptHash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	userID := "usr_001"
	s.users[userID] = &User{
		ID: userID, OrgID: orgID, Email: "admin@cyberteq.com", Name: "Admin User",
		PasswordHash: string(bcryptHash), Role: "admin", MFAEnabled: false,
		Permissions: []string{"finding:write", "vault:read", "vault:write", "report:export", "admin:users", "engagements:write"},
		CreatedAt:      now,
		PasswordExpiry: time.Now().AddDate(0, 3, 0),
	}
	s.usersByEmail["admin@cyberteq.com"] = s.users[userID]
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
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err == nil {
		return user, nil
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

func (s *Store) UpdateFindingWithVersion(id string, updateFn func(*Finding) error, expectedVersion int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.findings[id]
	if !ok {
		return fmt.Errorf("finding not found")
	}
	if f.Version != expectedVersion {
		return fmt.Errorf("conflict: finding version %d does not match expected version %d", f.Version, expectedVersion)
	}
	if err := updateFn(f); err != nil {
		return err
	}
	f.Version++
	f.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
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

func (s *Store) BulkCreateFindings(findings []*Finding) []*Finding {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, f := range findings {
		f.ID = uuid.New().String()
		f.Version = 1
		f.CreatedAt = now
		f.UpdatedAt = now
		s.findings[f.ID] = f
	}
	// Return a copy with the generated IDs
	result := make([]*Finding, len(findings))
	copy(result, findings)
	return result
}

func (s *Store) ListFindingsChanges(since string, engagementID string) []*Finding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Finding
	sinceTime, _ := time.Parse(time.RFC3339, since)
	if sinceTime.IsZero() {
		return result
	}
	for _, f := range s.findings {
		if f.EngagementID == engagementID {
			updatedAt, err := time.Parse(time.RFC3339, f.UpdatedAt)
			if err == nil && updatedAt.After(sinceTime) {
				result = append(result, f)
			}
		}
	}
	return result
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

// User management methods

func (s *Store) CreateUser(user *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.usersByEmail[user.Email]; exists {
		return fmt.Errorf("user with email %s already exists", user.Email)
	}
	user.ID = uuid.New().String()
	user.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	s.users[user.ID] = user
	s.usersByEmail[user.Email] = user
	return nil
}

func (s *Store) GetUserByEmail(email string) *User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.usersByEmail[email]
}

// UpdateUserPassword re-hashes a user's password, resets the 90-day expiry
// clock and clears the first-login "must change" flag. Used by the change
// password endpoint.
func (s *Store) UpdateUserPassword(userID, newHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return fmt.Errorf("user not found")
	}
	user.PasswordHash = newHash
	user.PasswordExpiry = time.Now().AddDate(0, 3, 0)
	user.MustChangePassword = false
	return nil
}

func (s *Store) ListAllUsers() []*User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*User
	for _, u := range s.users {
		result = append(result, u)
	}
	return result
}

func (s *Store) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[id]
	if !ok {
		return fmt.Errorf("user not found")
	}
	delete(s.users, id)
	delete(s.usersByEmail, user.Email)
	return nil
}

// Report record methods

func (s *Store) CreateReportRecord(rec *ReportRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec.ID = uuid.New().String()
	rec.CreatedAt = time.Now()
	s.reportRecords[rec.ID] = rec
	return nil
}

func (s *Store) GetReportRecord(id string) *ReportRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reportRecords[id]
}

func (s *Store) ListReportRecords() []*ReportRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*ReportRecord
	for _, r := range s.reportRecords {
		result = append(result, r)
	}
	return result
}
