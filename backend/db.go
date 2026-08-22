package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// StoreDB is a thin SQLite-backed persistence layer for the in-memory store.
// The whole store snapshot is serialized as JSON into a single row after every
// mutating request and loaded back on startup. This keeps the existing Store
// API (all handlers unchanged) while giving us real durability — data survives
// a server restart, which is what the multi-pentester workflow requires.
type StoreDB struct {
	db   *sql.DB
	path string
	mu   sync.Mutex
}

// snapshotRow is the persisted shape of the entire Store.
type snapshotRow struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func openStoreDB(path string) (*StoreDB, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	// WAL mode for better concurrent read/write behaviour; busy_timeout so a
	// second process/connection waits instead of erroring immediately.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS store_snapshot (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		db.Close()
		return nil, err
	}

	return &StoreDB{db: db, path: path}, nil
}

func (d *StoreDB) Close() error {
	if d == nil || d.db == nil {
		return nil
	}
	return d.db.Close()
}

// Save serializes the given JSON bytes under the key "all".
func (d *StoreDB) Save(value []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec(
		`INSERT INTO store_snapshot (key, value, updated_at) VALUES ('all', ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		string(value), time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// Load reads the persisted snapshot, returning (nil, nil) if none exists yet.
func (d *StoreDB) Load() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var val string
	err := d.db.QueryRow(`SELECT value FROM store_snapshot WHERE key='all'`).Scan(&val)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []byte(val), nil
}

// storeSnapshot is the JSON shape of all Store maps/slices for persistence.
type storeSnapshot struct {
	Orgs          map[string]*Org           `json:"orgs"`
	Users         map[string]*userSnapshot  `json:"users"`
	UsersByEmail  map[string]*userSnapshot  `json:"users_by_email"`
	Engagements   map[string]*Engagement    `json:"engagements"`
	Nodes         map[string]*Node          `json:"nodes"`
	Findings      map[string]*Finding       `json:"findings"`
	Evidence      map[string]*Evidence      `json:"evidence"`
	Reports       map[string]*Report        `json:"reports"`
	ReportRecords map[string]*ReportRecord  `json:"report_records"`
	AuditLog      []*AuditLog               `json:"audit_log"`
	Sessions      map[string]*Session       `json:"sessions"`
	Comments      map[string]*Comment       `json:"comments"`
	Activities    []*ActivityItem           `json:"activities"`
}

// userSnapshot is the persisted form of a User. The live User struct marks
// PasswordHash as json:"-" so it never leaks through the API; here we use a
// dedicated type so the hash survives the round-trip to SQLite.
type userSnapshot struct {
	ID                 string    `json:"id"`
	OrgID              string    `json:"org_id"`
	Email              string    `json:"email"`
	Name               string    `json:"name"`
	Role               string    `json:"role"`
	PasswordHash       string    `json:"password_hash"`
	MFAEnabled         bool      `json:"mfa_enabled"`
	Permissions        []string  `json:"permissions"`
	CreatedAt          string    `json:"created_at"`
	PasswordExpiry     time.Time `json:"password_expiry"`
	MustChangePassword bool      `json:"must_change_password"`
}

func toUserSnapshot(u *User) *userSnapshot {
	return &userSnapshot{
		ID:                 u.ID,
		OrgID:              u.OrgID,
		Email:              u.Email,
		Name:               u.Name,
		Role:               u.Role,
		PasswordHash:       u.PasswordHash,
		MFAEnabled:         u.MFAEnabled,
		Permissions:        u.Permissions,
		CreatedAt:          u.CreatedAt,
		PasswordExpiry:     u.PasswordExpiry,
		MustChangePassword: u.MustChangePassword,
	}
}

func fromUserSnapshot(s *userSnapshot) *User {
	return &User{
		ID:                 s.ID,
		OrgID:              s.OrgID,
		Email:              s.Email,
		Name:               s.Name,
		Role:               s.Role,
		PasswordHash:       s.PasswordHash,
		MFAEnabled:         s.MFAEnabled,
		Permissions:        s.Permissions,
		CreatedAt:          s.CreatedAt,
		PasswordExpiry:     s.PasswordExpiry,
		MustChangePassword: s.MustChangePassword,
	}
}

// toSnapshot builds a snapshot copy of the store. Caller must hold no locks
// (we take the read lock internally).
func (s *Store) toSnapshot() *storeSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	users := make(map[string]*userSnapshot, len(s.users))
	usersByEmail := make(map[string]*userSnapshot, len(s.usersByEmail))
	for k, v := range s.users {
		users[k] = toUserSnapshot(v)
	}
	for k, v := range s.usersByEmail {
		usersByEmail[k] = toUserSnapshot(v)
	}
	return &storeSnapshot{
		Orgs:          s.orgs,
		Users:         users,
		UsersByEmail:  usersByEmail,
		Engagements:   s.engagements,
		Nodes:         s.nodes,
		Findings:      s.findings,
		Evidence:      s.evidence,
		Reports:       s.reports,
		ReportRecords: s.reportRecords,
		AuditLog:      s.auditLog,
		Sessions:      s.sessions,
		Comments:      s.comments,
		Activities:    s.activities,
	}
}

// fromSnapshot replaces the store contents with a loaded snapshot.
func (s *Store) fromSnapshot(snap *storeSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if snap.Orgs != nil {
		s.orgs = snap.Orgs
	}
	if snap.Users != nil {
		s.users = make(map[string]*User, len(snap.Users))
		for k, v := range snap.Users {
			s.users[k] = fromUserSnapshot(v)
		}
	}
	if snap.UsersByEmail != nil {
		s.usersByEmail = make(map[string]*User, len(snap.UsersByEmail))
		for k, v := range snap.UsersByEmail {
			s.usersByEmail[k] = fromUserSnapshot(v)
		}
	}
	if snap.Engagements != nil {
		s.engagements = snap.Engagements
	}
	if snap.Nodes != nil {
		s.nodes = snap.Nodes
	}
	if snap.Findings != nil {
		s.findings = snap.Findings
	}
	if snap.Evidence != nil {
		s.evidence = snap.Evidence
	}
	if snap.Reports != nil {
		s.reports = snap.Reports
	}
	if snap.ReportRecords != nil {
		s.reportRecords = snap.ReportRecords
	}
	if snap.AuditLog != nil {
		s.auditLog = snap.AuditLog
	}
	if snap.Sessions != nil {
		s.sessions = snap.Sessions
	}
	if snap.Comments != nil {
		s.comments = snap.Comments
	}
	if snap.Activities != nil {
		s.activities = snap.Activities
	}
}

// Persist writes the current store state to SQLite.
func (s *Store) Persist() error {
	if s.db == nil {
		return nil
	}
	snap := s.toSnapshot()
	data, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := s.db.Save(data); err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}
	return nil
}

// Load reads the persisted store state from SQLite, returning true if a
// snapshot existed and was applied.
func (s *Store) Load() (bool, error) {
	if s.db == nil {
		return false, nil
	}
	data, err := s.db.Load()
	if err != nil {
		return false, fmt.Errorf("load snapshot: %w", err)
	}
	if data == nil {
		return false, nil
	}
	var snap storeSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return false, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	s.fromSnapshot(&snap)
	s.backfillUserDefaults()
	return true, nil
}

// backfillUserDefaults repairs users persisted by older builds that predate
// fields like PasswordExpiry and role permissions. Without this, users created
// before those fields existed would show as instantly expired or have zero
// permissions.
func (s *Store) backfillUserDefaults() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.PasswordExpiry.IsZero() {
			u.PasswordExpiry = time.Now().AddDate(0, 3, 0)
		}
		if u.Role == "" {
			u.Role = "analyst"
		}
		if len(u.Permissions) == 0 {
			u.Permissions = rolePermissions(u.Role)
		}
	}
}

// persistMiddleware snapshots the store to SQLite after every mutating request
// (anything that is not a safe read method). Persistence is best-effort: a
// failure is logged, never fails the request.
func persistMiddleware(store *Store) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				return
			}
			if err := store.Persist(); err != nil {
				log.Printf("persist: %v", err)
			}
		})
	}
}

var _ = snapshotRow{}