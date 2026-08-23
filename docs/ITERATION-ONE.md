# mCollaborator — Iteration One

**Internal project documentation · Cyberteq Falcon**

mCollaborator is a single-tenant web application that gives a team of penetration
testers a shared working environment to run engagements, record findings, collect
evidence, collaborate on vulnerabilities, and produce Cyberteq-branded VAPT
(DOCX/PDF) deliverables. This document describes the application as it stands at
the end of Iteration One.

---

## 1. Overview

- **Purpose.** Replace siloed, per-tester spreadsheets/notes with one shared
  vulnerability store that multiple analysts can work against on the office LAN.
- **Current milestone.** Iteration One delivers a complete, usable vertical slice:
  authenticated SPA, engagement + finding lifecycle, evidence vault, analytics,
  multi-analyst change polling, a full VAPT report wizard, template-accurate
  DOCX/PDF export, and (new in this iteration) **SQLite-backed persistence** so
  analyst work survives a server restart.
- **Domain change.** The product uses the `cyberteq.com` domain everywhere
  (login, validation, branding). Earlier builds used `cyberteq.io`; this was
  migrated fully.
- **Not yet in scope (deferred).** OneDrive sync is built and wired into the
  wizard, but dormant until the `OD_*` credentials are set (see §10). WebSocket real-time
  updates were deliberately **not** implemented; 15 s polling meets the stated
  "2-minute refresh" requirement and keeps the deployment simple.

---

## 2. Architecture

```
Analyst browser (single-page app)
        │  REST + Bearer token
        ▼
Go backend (chi router)  :9900
   ├── auth / RBAC middleware
   ├── REST handlers  ──►  in-memory Store (map[string]*T + sync.RWMutex)
   │                              │
   │                              └── persistMiddleware ──► SQLite (store_snapshot row)
   ├── report export  ──►  DOCX template merge ──► Word/LibreOffice ──► PDF
   │                              └── (optional) Microsoft Graph ──► OneDrive
   └── static/ (embedded SPA)
```

- **Single process, single shared memory space.** All analysts hit the same
  running binary, so the in-memory store *is* the collaboration state.
- **Persistence.** Every non-GET request triggers a JSON snapshot of the whole
  store written to a single SQLite row (WAL mode). On boot, the snapshot is
  loaded back; otherwise demo data is seeded. This is the "shared source of
  truth" that multi-pentester workflows need.
- **Polling, not push.** The frontend polls
  `GET /api/v1/engagements/{id}/findings/changes?since=<RFC3339>` every 15 s and
  re-renders / raises a toast when other analysts change findings.

---

## 3. Tech stack

| Layer | Choice | Notes |
|---|---|---|
| Language | Go (go 1.25) | Module `github.com/cyberteq/mcollaborator` |
| Router | `go-chi/chi/v5` v5.1.0 | |
| Persistence | `modernc.org/sqlite` v1.56.0 | Pure-Go driver — no CGO/gcc needed |
| DOCX | `unidoc/unioffice` v1.39.0 + template merge engine | Real Word template, byte-swapped |
| DOCX→PDF | LibreOffice (`soffice`) or Microsoft Word (COM via PowerShell) | Highest-fidelity path |
| Password hashing | `golang.org/x/crypto/bcrypt` | |
| IDs | `google/uuid` | |
| Frontend | Vanilla JS SPA (no framework) | `state.js`, `pages.js`, `api.js` |
| Styling | Custom CSS with 3 themes | Cyberpunk (default), Ledger, Slate |

---

## 4. Repository layout

```
mCollaborator/
├── .gitignore
├── docs/
│   ├── ITERATION-ONE.md          ← this document
│   ├── COLLABORATION-DESIGN.md   ← design draft (SharePoint vs alternatives)
│   ├── M365-TRIAL-SETUP.md       ← how to stand up a test M365 tenant
│   └── SHAREPOINT-INTEGRATION.md/.docx
└── backend/
    ├── go.mod / go.sum
    ├── main.go          router, middleware chain, SPA embedding
    ├── models.go        data models + request/response types
    ├── store.go         Store struct, seed data, all store operations
    ├── db.go            SQLite persistence layer + persistMiddleware
    ├── handlers.go      HTTP handlers
    ├── middleware.go    CORS / logging / auth / RBAC / rate-limit
    ├── report.go        ReportConfig, export flow, report naming
    ├── onedrive.go      Microsoft Graph upload of the finished report
    ├── docxmerge.go     Word template merge engine (token + repeat blocks)
    ├── docxlogo.go      client-logo swap into template header slot
    ├── docxtestcases.go test-type checklist statuses derived from the findings
    ├── docxfooter.go    footer tab-stop fitting so items do not collide
    ├── docx2pdf.go      DOCX→PDF via LibreOffice / Word COM
    ├── docxmerge_test.go
    ├── sharepoint.go    Microsoft Graph client (currently inactive)
    ├── templates/vapt_report_template.docx
    ├── static/          embedded SPA (index.html, css/, js/, images/)
    │   └── js/  state.js (router+navbar), pages.js (pages & report wizard), api.js
    └── data/            SQLite DB created at runtime (mcollaborator.db)
```

---

## 5. Data model (models.go)

| Entity | Purpose | Key fields |
|---|---|---|
| `Org` | Tenant container | id, name, slug |
| `User` | Account | email, role (`admin`/`analyst`/`user`/`client`), bcrypt `PasswordHash`, permissions, password expiry |
| `Engagement` | A pentest project | client, status, methodology, scope (included/excluded/RoE), timeline, team |
| `Node` | In-scope target | target, type |
| `Finding` | A vulnerability | title, CVE, CWEs, MITRE ATT&CK, severity, CVSS vector/score, status, description, POC, remediation, impact, likelihood, assignee, version |
| `Evidence` | Uploaded proof | filename, storage key, mime, sha256, tags, description |
| `Report` | Draft report record | template, classification, blocks |
| `ReportRecord` | Export record | format, file path, created by |
| `AuditLog` | Immutable audit trail | actor, action, resource, diff, IP |
| `Session` | Bearer token | token, expiry (24 h) |
| `Comment` | Finding discussion | body, author |
| `ActivityItem` | Activity feed | action, detail |

> **Note on `json:"-"`:** `User.PasswordHash` is excluded from JSON so hashes
> never leak through the API. The persistence layer uses a dedicated
> `userSnapshot` DTO in `db.go` so the hash still survives the round-trip to
> SQLite — this was a bug found and fixed in Iteration One (see §12).

---

## 6. API surface (all under `/api/v1`)

**Auth (public):** `POST /auth/login`, `POST /auth/logout`

**Users (auth + RBAC):** `GET /users/me`, `GET /users`,
`POST /users` (`admin:users`), `DELETE /users/{id}` (`admin:users`)

**Engagements:** `GET|POST /engagements`, `GET|PUT|DELETE /engagements/{id}`,
`GET|POST /engagements/{id}/nodes` (writes require `engagements:write`)

**Findings:** `GET /engagements/{id}/findings`,
`POST /engagements/{id}/findings` (`finding:write`),
`POST /engagements/{id}/findings/bulk` (`finding:write`),
`GET /engagements/{id}/findings/changes?since=`,
`GET /findings/{id}`, `PUT /findings/{id}` (`finding:write`),
`PATCH /findings/{id}/status`, `POST /findings/{id}/assign`,
`DELETE /findings/{id}`, `GET|POST /findings/{id}/comments`

**Evidence:** `GET /evidence`, `POST /evidence/upload` (`vault:write`),
`GET|DELETE /evidence/{id}`

**Reports:** `GET /reports`, `POST /reports` (`report:export`),
`GET /reports/{id}`, `POST /reports/{id}/export` (`report:export`),
`GET /reports/{id}/download`

**Activity:** `GET /activities`

**Analytics:** `GET /analytics/findings-over-time`,
`GET /analytics/severity-count`, `GET /analytics/status-count`,
`GET /analytics/top-assets`, `GET /analytics/recurring-cwes`,
`GET /analytics/global-severity`

**Report generation (unauthenticated by design):**
`POST /reports/export`, `GET /reports/download/{type}/{name}`

**Other:** `GET /health` (version 1.0.0)

**Response envelope:** `{"data": ...}` or `{"error": {"code", "message"}}`.

---

## 7. Authentication & authorization

- **Login.** `HandleLogin` verifies email + password (`bcrypt` or legacy SHA-256
  fallback), issues a random 32-byte hex token stored as a `Session` (24 h TTL),
  and returns it as `Bearer` token.
- **Session check.** `authMiddleware` resolves the token to a user and injects it
  into the request context.
- **RBAC.** `requirePermission(perm, handler)` gates writes. `admin` bypasses
  all checks; other roles must carry the permission string (e.g. `finding:write`,
  `vault:write`, `report:export`, `engagements:write`, `admin:users`).
- **Demo credentials**
  - `admin@cyberteq.com` / `admin123` — admin, full permissions
  - `analyst@cyberteq.com` / `admin123` — analyst
  - `client@acme.io` / `admin123` — client (read-mostly)
- **Security notes.** SPA escape (`sanitizeInput`, `isXSSAttempt`), email
  format validation, password minimum length (8) for created users, CORS
  `Access-Control-Allow-Origin: *`, and a rate-limit middleware placeholder.

---

## 8. Frontend (SPA)

- **`state.js`** — global `MCOLLABORATOR` object, token/theme persistence, hash router,
  unified top **navbar** (Dashboard, Projects, Findings, Evidence, Reports,
  Users) plus theme switcher and logout. Routes: `/login`, `/dashboard`,
  `/ledger/project`, `/finding-editor`, `/evidence`, `/report-generator` (`/reports`),
  `/finding-detail`, `/command/engagements`, `/command/feed`,
  `/command/report-builder`, `/admin/users`.
- **`pages.js`** — all page renderers, the report wizard, and live-findings
  polling. Key pieces:
  - **Report wizard (4 steps).** Step 1: company/project meta, reference,
    version, assessment dates, tester/approver, scope / out-of-scope / tools,
    client logo upload. Step 2: report sections (WPT/EPT/IPT/NAR). Step 3:
    pick findings to include (checkbox list of live findings). Step 4: output
    format (DOCX/PDF), optional OneDrive sync + folder, generate.
  - **Findings polling.** `startFindingsPolling(engId)` polls
    `/findings/changes` on a configurable cadence. Default 15 s; set
    `window.FINDINGS_REFRESH_MS` before load to change (e.g. `120000` for 2 min).
  - **`generateReportDocument()`** builds the full `ReportConfig` payload
    (including `sync_onedrive`/`onedrive_folder` and full finding objects) and
    renders download links + OneDrive status in the preview.
- **`api.js`** — thin fetch wrapper with Bearer token injection and 401 → logout.

---

## 9. Report generation pipeline

The deliverable is a Cyberteq-branded VAPT report. Two paths exist:

### 9.1 Preferred: template-accurate DOCX + engine-converted PDF

1. `mergeVAPTDocx` (docxmerge.go) reads the embedded
   `templates/vapt_report_template.docx` as a ZIP and performs **string
   substitution** inside `word/document.xml` and the footers:
   - Scalar tokens `{{CompanyName}}`, `{{VersionLabel}}`, `{{TesterName}}`,
     `{{ApproverName}}`, `{{ApproverTitle}}`, `{{AssessmentPeriod}}`,
     `{{ReportDate}}`, `{{RefNumber}}`, `{{FindingsCount}}`.
   - Repeat blocks delimited by `<!--REPEAT:FINDING_ROW-->` /
     `<!--REPEAT:FINDING_DETAIL-->` markers, expanded once per finding with
     per-finding tokens (title, exposure, criticality + color, vuln ID,
     description, CVSS, impact, affected system, POC, recommendation).
   - Text is XML-escaped and newlines become real Word breaks.
2. `renderClientLogoPart` (docxlogo.go) letterboxes the uploaded client logo
   onto the exact 567×152 canvas the template's header expects and swaps
   `word/media/image1.png`, so the logo lands in the right place, sized and
   unstretched. Bad uploads fall back to the template asset.
3. `convertDOCXToPDF` (docx2pdf.go) converts the merged DOCX using, in order:
   LibreOffice headless (`soffice --convert-to pdf`), or Microsoft Word via
   COM/PowerShell (`ExportAsFixedFormat(..., 17)`). This preserves the exact
   template layout.
4. If no engine is installed there is no PDF. The export returns the DOCX on
   its own with `pdf_error` saying why. There is deliberately no fallback
   renderer: one existed, and when a Word timeout silently routed the export
   through it the delivered "PDF" was 16KB of hand-drawn text with none of the
   template. Engine choice is logged.

### 9.3 Test-type checklists (docxtestcases.go)

Each chapter 3 area section opens with a "Test Type / Status" table - 194 rows
across the seven areas that have one. The template ships those statuses filled
in from whatever engagement it was last used for, and for a long time nothing
rewrote them, so a delivered report asserted results about the client that were
really someone else's.

`renderTestTypeTables` now derives the Status column from the findings:

- **Issues** on a row the area's findings are about, **Pass** on the rest.
  Areas that were not in scope keep no table - `renderAreaSections` already
  dropped those.
- The checklist itself is **read out of the template** at render time, names and
  group headings alike. Nothing is copied into Go, so editing the checklist in
  the DOCX needs no code change and there is no second copy to fall out of date.
- A finding is tied to a row by word overlap, with each word weighted `1/(rows
  using it)` - measured off that table, so it re-weights itself if the checklist
  is re-worded. That weighting is what stops one SQL injection from also
  condemning the LDAP, XPath, XML, ORM and NoSQL injection rows beside it:
  "injection" appears in eleven rows and "xpath" in one.
- Tuned for **precision over recall**. A row wrongly marked Issues invents a
  vulnerability in the client's report; a row wrongly left Pass only leaves the
  default in place.
- A finding that matches nothing is reported: `unmatched_findings` on the export
  response, shown in the wizard and written to the server log. That is the one
  case where the register reports a vulnerability the checklist does not, and it
  is not allowed to pass quietly.

**Known limit.** With this rule "Pass" is the app's inference, not a tester's
assertion - a row nobody looked at still reads Pass. That was a deliberate
choice over a wizard step: zero extra clicks per report. If it ever needs to be
a human's word, the place to add it is a review step over the derived table
rather than a blank 194-row form.

### 9.4 Closure decks (pptxclosure.go, pptxslides.go)

The closing meeting is presented from a deck, and it used to be rebuilt by hand
from the report each time. `POST /api/v1/reports/closure` renders it from the
same payload the report export takes.

- **Template.** `templates/mcollaborator_closure_template.pptx`, derived by
  `tools/mkclosuretemplate` from the deck the team already presents. Six slides:
  title, two executive summaries, an issues table, a vulnerability scenario, and
  a closing slide. The last two are repeated.
- **Repeating a slide** is not like repeating a row in a DOCX. A slide is its own
  package part, and four other parts have to agree it exists: the slide list, the
  presentation relationships, the slide's own relationships and the content-type
  table. `addSlideRaw` updates all four; the two template slides are dropped once
  their clones are made, so no placeholder slide reaches the client.
- **Grouping.** Issues tables hold four findings, split by area then severity, so
  a slide is titled "IPT Issues - Critical Level (1/5)" rather than naming every
  area and severity it happens to contain.
- **Screenshots.** One scenario slide per screenshot, which is how the hand-built
  decks read. The images come from `resolvePOCImages`, the same function the DOCX
  report uses, so a finding's proof in the deck is the same file as its proof in
  the report and in the evidence vault rather than a second copy that can drift.
- **Findings without evidence** still appear in the issues tables but get no
  scenario slide, and the response names them in `findings_without_proof`. A
  closing meeting walks the scenario slides; a finding without one is a finding
  nobody will see demonstrated.
- **Access.** Open to every role, like the Closure Prep page it is reached from.

### 9.5 Report config

`ReportConfig` (report.go) carries company/logo/engagement meta, scope &
out-of-scope arrays, sections, findings, tools, and the optional OneDrive sync
folder. It no longer carries test-case groups: the checklist statuses are
derived (§9.3), so a field for them would only be somewhere else for them to go
stale. `HandleExportReport` writes files to
`backend/reports/` as `<Reference Number> - <Company Name> - VAPT.{docx,pdf}`,
returns `docx_url`/`pdf_url`, and, when `sync_onedrive` is on, reports
`od_status` (`ok` / `failed` / `not_configured`) with the uploaded links.

- **Known latency.** Word-engine conversion takes ~66 s; exports are slow but
  not failing. The frontend disables the Generate button and shows progress.

---

## 10. OneDrive sync

`onedrive.go` implements an OAuth2 **client-credentials** flow against Microsoft
Entra ID, caches the token until just before expiry, and uploads the generated
DOCX and PDF to `/users/{upn}/drive/root:/{folder}/{filename}:/content`,
returning each file's `webUrl`. Because a client-credential token has no signed-in
user behind it, the destination drive is named explicitly by UPN.

The DOCX is uploaded first and a failure there fails the sync; the PDF follows
when one was produced, and a PDF that fails to upload is reported as a partial
success rather than sinking a report that is already filed.

- **Configuration (env vars):** `OD_TENANT_ID`, `OD_CLIENT_ID`,
  `OD_CLIENT_SECRET`, `OD_USER` (the UPN whose OneDrive receives the files),
  `OD_FOLDER` (optional, defaults to `VAPT Reports`).
- **Destination.** The wizard's folder field wins; left blank, reports are filed
  under `<OD_FOLDER>/<Company Name>`, one folder per client.
- **Permission.** The app registration needs **Files.ReadWrite.All** as an
  *application* permission, with admin consent granted.
- **Wizard response.** `od_status` (`ok` / `failed` / `not_configured`),
  `od_docx_link`, `od_pdf_link`, `od_folder`, `od_error`. When the credentials
  are absent the response names the variables that still need setting rather
  than saying only "not configured".
- **History.** This replaced a SharePoint site upload (`sharepoint.go`), which
  could never be exercised: the tenant (`0507f324-…`) held only "Microsoft Entra
  ID free" and Graph rejected site access with *"Tenant does not have a SPO
  license."* A user's OneDrive for Business needs no SharePoint site. The design
  notes for the old integration are kept in `docs/SHAREPOINT-INTEGRATION.md`.
- **Decision.** The collaboration design doc proposed SharePoint as the source
  of truth. For Iteration One the user chose **Option 1: self-hosted SQLite
  persistence + polling** instead, because it is free, local, and sufficient for
  the office-LAN deployment. OneDrive sync is a delivery step for finished
  reports, not a datastore.

---

## 11. Persistence (db.go) — new in Iteration One

- **Mechanism.** `openStoreDB` opens a pure-Go SQLite DB (WAL journal mode,
  `busy_timeout(5000)`, `synchronous(NORMAL)`, `MaxOpenConns(1)`). One table,
  `store_snapshot(key, value, updated_at)`, holds a single row `key='all'` whose
  `value` is the entire store serialized as JSON (`storeSnapshot` struct).
- **Write path.** `persistMiddleware` runs *after* every non-GET/HEAD/OPTIONS
  request and calls `store.Persist()` (best-effort; failures are logged, never
  fail the request).
- **Load path.** `NewStore` (store.go) opens the DB; if a snapshot exists it is
  loaded via `Load()`/`fromSnapshot`, otherwise `seed()` runs and the demo data
  is persisted immediately.
- **Password hashes survive.** `userSnapshot` DTO carries `PasswordHash` with a
  real JSON tag, while the live `User` keeps `json:"-"` (see §5 note).
- **Files/locations.** Default DB path `backend/data/mcollaborator.db`
  (plus `-wal`/`-shm`). Override with env `MCOLLABORATOR_DB_PATH` (the older `STITCH_DB_PATH` is still
  read, so an existing deployment does not silently open an empty database). The `.gitignore`
  does **not** yet exclude the DB — add it if the data dir should not be
  committed.
- **Concurrency model.** Single server process; the DB is not shared across
  multiple running binaries. (That is acceptable for the LAN single-server
  deployment.)

---

## 12. Notable fixes & decisions this iteration

- **Login broke after restart.** Root cause: `User.PasswordHash` was
  `json:"-"`, so the SQLite snapshot stripped every hash. Fixed with the
  `userSnapshot` DTO in db.go. Login now survives restarts.
- **Domain migration.** `cyberteq.io` → `cyberteq.com` across store, seed,
  handlers, and frontend.
- **Navbar.** The sidebar was removed; a single responsive top navbar renders
  across all three themes.
- **Report wizard completeness.** All step-1 fields (incl. scope / out-of-scope
  / tools) are persisted in `reportWizardState`; findings are stored as full
  objects (`allFindings` + `toggleWizardFinding`); the generate payload sends
  everything, including the OneDrive sync fields.
- **Findings polling hardened.** `afterRenderProjectLedger` /
  `showBulkImportModal` now use `MCOLLABORATOR.currentEngagement?.id` instead of
  fragile URL parsing.
- **DOCX→PDF strategy.** A Word engine (Word COM / LibreOffice) is the only
  path. No engine means no PDF, said out loud - never a lookalike under the
  real export's name.

---

## 13. Build, run & test

```powershell
# Build (from backend/)
go build -o mCollaborator.exe .
go test ./...            # includes the template-render suite

# Run
.\mCollaborator.exe      # or: PORT=9900 .\mCollaborator.exe
# Open http://localhost:9900  → login as admin@cyberteq.com / admin123
```

Notes:
- `go.mod` lists `modernc.org/sqlite` under indirect — it is used directly
  (blank import in db.go); consider `go mod tidy` to clean it up.
- On this machine Word is installed, so PDF export uses the Word COM engine
  (~66 s) with no LibreOffice needed.
- `docxmerge_test.go` validates token substitution, repeat-block expansion,
  no leftover markers/tokens, and part-count preservation. Set
  `DOCXMERGE_WRITE_SAMPLE=path.docx` to dump a sample output.

---

## 14. Deployment — office LAN multi-pentester

- **Topology.** One Windows machine runs `mCollaborator.exe` on TCP 9900; analysts on
  the same LAN open `http://<server-ip>:9900`.
- **Data safety.** Restart-safe thanks to SQLite; all team changes live in one
  place and appear on others within the polling window (15 s by default).
- **To-do / known gaps**
  1. **Windows Firewall:** add an inbound rule for TCP 9900 (needs an elevated
     shell) so other machines can reach the server.
  2. **Polling cadence:** default 15 s already beats the 2-minute requirement;
     it is tunable via `window.FINDINGS_REFRESH_MS`.
  3. **Concurrency conflict detection** (`UpdateFindingWithVersion`) exists in
     the store but no handler routes through it yet — writes are last-write-wins.
  4. **Rate limiting** middleware is a pass-through placeholder.
  5. **Evidence** is metadata-only (real file storage on disk is not yet
     wired); `HashSHA256` is a placeholder of zero bytes.
  6. Rotate the Entra client secret shared in chat before any long-term SP use.

---

## 15. Roadmap (next iterations)

- Wire `UpdateFindingWithVersion` for optimistic-concurrency on shared findings.
- Persist evidence file bytes to disk (and record hashes for integrity).
- Real rate limiting per IP.
- Set the `OD_*` credentials to switch OneDrive sync on (docs in §10).
- Optional: multi-server mode with a shared DB / proper lock store.
