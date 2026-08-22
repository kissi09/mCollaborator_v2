package main

import "time"

type Org struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type User struct {
	ID                 string    `json:"id"`
	OrgID              string    `json:"org_id"`
	Email              string    `json:"email"`
	Name               string    `json:"name"`
	Role               string    `json:"role"`
	PasswordHash       string    `json:"-"`
	MFAEnabled         bool      `json:"mfa_enabled"`
	Permissions        []string  `json:"permissions"`
	CreatedAt          string    `json:"created_at"`
	PasswordExpiry     time.Time `json:"password_expiry"`
	MustChangePassword bool      `json:"must_change_password"`
}

type ReportRecord struct {
	ID           string    `json:"id"`
	EngagementID string    `json:"engagement_id"`
	Title        string    `json:"title"`
	Template     string    `json:"template"`
	Format       string    `json:"format"`
	FilePath     string    `json:"file_path"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
}

type Engagement struct {
	ID          string          `json:"id"`
	OrgID       string          `json:"org_id"`
	Name        string          `json:"name"`
	ClientName  string          `json:"client_name"`
	Status      string          `json:"status"`
	Methodology string          `json:"methodology"`
	Scope       EngagementScope `json:"scope"`
	Timeline    Timeline        `json:"timeline"`
	Team        []string        `json:"team"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

type EngagementScope struct {
	Included          []string `json:"included"`
	Excluded          []string `json:"excluded"`
	RulesOfEngagement string   `json:"rules_of_engagement,omitempty"`
}

type Timeline struct {
	StartDate  string      `json:"start_date"`
	EndDate    string      `json:"end_date"`
	Milestones []Milestone `json:"milestones,omitempty"`
}

type Milestone struct {
	Title     string `json:"title"`
	Due       string `json:"due"`
	Completed bool   `json:"completed"`
}

type Node struct {
	ID           string `json:"id"`
	EngagementID string `json:"engagement_id"`
	Target       string `json:"target"`
	Type         string `json:"type"`
	Metadata     string `json:"metadata,omitempty"`
	CreatedAt    string `json:"created_at"`
}

type Finding struct {
	ID             string   `json:"id"`
	EngagementID   string   `json:"engagement_id"`
	NodeID         string   `json:"node_id,omitempty"`
	Title          string   `json:"title"`
	CVE            string   `json:"cve,omitempty"`
	CWEs           []string `json:"cwes,omitempty"`
	MitreAttackIDs []string `json:"mitre_attack_ids,omitempty"`
	// Category is the assessment area this finding is reported under (IPT, EPT,
	// IPTC, WPT, CFG, ASA, ADT, WNA, NAR). It decides which chapter 3 section of
	// the report the finding prints in and which bar of the findings-by-area
	// chart it counts towards.
	Category    string   `json:"category,omitempty"`
	Severity    string   `json:"severity"`
	CVSSVector  string   `json:"cvss_vector,omitempty"`
	CVSSScore   float64  `json:"cvss_score"`
	Status      string   `json:"status"`
	Description string   `json:"description"`
	POC         string   `json:"poc,omitempty"`
	Remediation string   `json:"remediation,omitempty"`
	Impact      string   `json:"impact,omitempty"`
	Likelihood  string   `json:"likelihood,omitempty"`
	AssignedTo  string   `json:"assigned_to,omitempty"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"` // evidence records attached as PoC screenshots
	CreatedBy   string   `json:"created_by"`
	Version     int      `json:"version"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

type Evidence struct {
	ID             string `json:"id"`
	EngagementID   string `json:"engagement_id"`
	FindingID      string `json:"finding_id,omitempty"`
	Filename       string `json:"filename"`
	StorageKey     string `json:"storage_key"`
	MimeType       string `json:"mime_type"`
	SizeBytes      int64  `json:"size_bytes"`
	HashSHA256     string `json:"hash_sha256"`
	Tags           string `json:"tags,omitempty"`
	Description    string `json:"description,omitempty"`
	UploadedBy     string `json:"uploaded_by"`
	UploadedByName string `json:"uploaded_by_name,omitempty"`
	CreatedAt      string `json:"created_at"`
}

type Report struct {
	ID             string        `json:"id"`
	EngagementID   string        `json:"engagement_id"`
	Title          string        `json:"title"`
	Template       string        `json:"template"`
	Classification string        `json:"classification"`
	Blocks         []ReportBlock `json:"blocks"`
	CreatedBy      string        `json:"created_by"`
	Status         string        `json:"status"`
	Version        int           `json:"version"`
	CreatedAt      string        `json:"created_at"`
	UpdatedAt      string        `json:"updated_at"`
}

type ReportBlock struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Content string `json:"content"`
	Order   int    `json:"order"`
}

type AuditLog struct {
	ID         string `json:"id"`
	OrgID      string `json:"org_id"`
	ActorID    string `json:"actor_id"`
	Action     string `json:"action"`
	Resource   string `json:"resource"`
	ResourceID string `json:"resource_id"`
	Diff       string `json:"diff,omitempty"`
	IPAddress  string `json:"ip_address"`
	CreatedAt  string `json:"created_at"`
}

type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type Comment struct {
	ID        string `json:"id"`
	FindingID string `json:"finding_id"`
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type ActivityItem struct {
	ID           string `json:"id"`
	OrgID        string `json:"org_id"`
	UserID       string `json:"user_id"`
	UserName     string `json:"user_name"`
	Action       string `json:"action"`
	Detail       string `json:"detail"`
	EngagementID string `json:"engagement_id,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// Request/Response types
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type ApiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ApiResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error *ApiError   `json:"error,omitempty"`
}
