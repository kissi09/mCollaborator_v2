# -*- coding: utf-8 -*-
"""Sections 1-9 of the mCollaborator documentation."""

PART_A = [

# ---------------------------------------------------------------- 1. Overview
("h2", "Overview"),
("p", "**mCollaborator** is Cyberteq Falcon's engagement collaboration and reporting "
      "platform. Analysts record findings and evidence against an engagement while the "
      "testing is still running, and the application assembles the client deliverables "
      "from that same record: the VAPT report as DOCX and PDF, and the closing-meeting "
      "deck as PPTX."),
("p", "It is a single Go binary. The web interface is embedded inside it, the database "
      "is a file beside it, and the Word and PowerPoint templates are compiled into it. "
      "There is nothing to deploy but the executable and its data directory. On Windows "
      "the same server can also be shipped inside a desktop window with its own icon and "
      "installer."),

("h3", "What the application produces"),
("table", (
    ["Deliverable", "Format", "Built from"],
    [
        ["VAPT report", "DOCX, and PDF when a Word engine is available",
         "The Cyberteq Word template merged with the engagement's details, selected "
         "findings and their evidence"],
        ["Closing-meeting deck", "PPTX",
         "The Cyberteq PowerPoint template, filled from the same findings and the same "
         "evidence files as the report"],
        ["Working record", "Held in the application",
         "Engagements, findings, evidence, comments, activity and an audit log"],
    ]), [1.4, 1.8, 3.3]),

("h3", "Design principles the code holds to"),
("bullets", [
    "**Template fidelity is the deliverable.** The client receives a document that must "
    "look like every other Cyberteq report. Layout, numbering, headers, footers and "
    "charts come from the template; the renderer fills them in and is not allowed to "
    "reinvent them.",
    "**No silent fallbacks.** When the PDF engine is missing, no PDF is produced and the "
    "wizard says why. A redrawn approximation shipped under the real export's name is "
    "worse than nothing, because it reads as the report until someone checks it.",
    "**One source of truth for proof.** A finding's screenshot in the report, in the deck "
    "and in the evidence vault is the same stored file, resolved once per export.",
    "**Failures reach the person, not the log.** A logo that will not decode, a finding "
    "the checklist could not account for, a OneDrive upload that did not happen - each "
    "comes back to the wizard as a warning beside the download link.",
    "**The desktop shell does not change the web app.** It runs the same server binary "
    "as a child process and proxies to it; the backend is not imported, forked or "
    "reshaped to fit it.",
]),

("h3", "Who uses it"),
("table", (
    ["Role", "Typical user", "What they do in the application"],
    [
        ["`admin`", "Team lead or platform owner",
         "Everything, plus user administration"],
        ["`analyst`", "Penetration tester",
         "Creates engagements, records findings, uploads evidence, exports reports and "
         "decks"],
        ["`project_manager`", "Delivery or account manager",
         "Reviews engagements, exports reports and prepares the closing deck; cannot "
         "edit findings or the evidence vault"],
    ]), [1.5, 1.7, 3.3]),
("p", "Closure Prep is deliberately open to every role: preparing a closing meeting is "
      "shared work between the analyst who found the issues and whoever presents them."),

("pagebreak",),

# ------------------------------------------------------------ 2. Architecture
("h2", "Architecture"),
("p", "One process serves the API and the interface. Everything else - the SQLite file, "
      "the uploaded evidence, the generated documents - lives in directories beside it."),

("h3", "Runtime topology"),
("code", """
  Browser tab                     Desktop window (Windows)
       |                                   |
       |  http://<host>:9900               |  wails://wails.localhost
       |                                   |  reverse proxy to 127.0.0.1:<random>
       +-----------------+-----------------+
                         |
                 mCollaborator server (Go, chi)
                         |
    +--------------------+---------------------+------------------+
    |                    |                     |                  |
 in-memory Store    report/deck            evidence files     SQLite snapshot
 (RWMutex)          renderers              data/uploads/      data/mcollaborator.db
                         |
              +----------+-----------+
              |                      |
      Word COM / LibreOffice   Microsoft Graph
      (DOCX -> PDF)            (optional OneDrive filing)
"""),

("h3", "Technology stack"),
("table", (
    ["Layer", "Choice", "Why it is that"],
    [
        ["Language", "Go 1.25",
         "One static binary, no runtime to install on the office machine"],
        ["HTTP router", "`github.com/go-chi/chi/v5`",
         "Standard `net/http` handlers with grouped middleware"],
        ["Database", "`modernc.org/sqlite`",
         "Pure-Go SQLite: no cgo, so the binary still cross-compiles and ships alone"],
        ["Passwords", "`golang.org/x/crypto/bcrypt`",
         "Cost-parameterised hashing; a legacy SHA-256 scheme is still accepted for "
         "accounts created before it"],
        ["Images", "`golang.org/x/image/webp`, `srwiley/oksvg`, `srwiley/rasterx`",
         "Client logos arrive as SVG or WEBP as often as PNG"],
        ["Front end", "Hand-written HTML, CSS and JavaScript",
         "No build step, no framework, no `node_modules`; the SPA is embedded with "
         "`go:embed`"],
        ["Desktop shell", "Wails v2 (WebView2 on Windows)",
         "A native window around the same server, in its own Go module"],
        ["Documents", "Direct OOXML manipulation",
         "The DOCX and PPTX packages are rewritten part by part, so the template's own "
         "layout survives untouched"],
    ]), [1.2, 2.1, 3.2]),

("h3", "Request lifecycle"),
("p", "Every request passes the same middleware chain, in this order, before it reaches "
      "a handler:"),
("table", (
    ["Middleware", "File", "What it does"],
    [
        ["`corsMiddleware`", "`middleware.go`",
         "Permissive CORS headers and a short-circuit for `OPTIONS`"],
        ["`loggingMiddleware`", "`middleware.go`",
         "One line per request: method, path, status, duration"],
        ["`rateLimitMiddleware`", "`middleware.go`",
         "A pass-through placeholder today - see Future Improvements"],
        ["`persistMiddleware`", "`db.go`",
         "After a mutating request completes, writes the whole store snapshot to SQLite"],
        ["`authMiddleware`", "`middleware.go`",
         "Applied to the protected route group only: resolves the bearer token to a "
         "session and puts the `*User` in the request context"],
        ["`requirePermission`", "`middleware.go`",
         "Wraps individual handlers; admins bypass, everyone else needs the named "
         "permission"],
    ]), [1.7, 1.2, 3.6]),

("h3", "Where the boundaries are"),
("bullets", [
    "**The store is the only state.** Handlers never touch SQLite directly; they mutate "
    "the in-memory store, and the snapshot is written for them.",
    "**The renderers are pure.** `mergeMCollaboratorDocx` and `buildClosureDeck` take a "
    "`ReportConfig` and return bytes. They do not read the database, the network or the "
    "clock, which is what makes the template test suite possible.",
    "**The desktop module is separate.** `desktop/` has its own `go.mod` and imports "
    "nothing from `backend/`.",
]),

("pagebreak",),

# -------------------------------------------------------- 3. Repository layout
("h2", "Repository Layout"),
("p", "The repository root holds three directories. The application folder is still "
      "named `stitch` from before the product was renamed; only the in-application names "
      "changed."),

("h3", "Backend source"),
("table", (
    ["File", "Lines", "Responsibility"],
    [
        ["`main.go`", "147", "Routing table, static file serving, server start-up"],
        ["`middleware.go`", "115", "CORS, logging, authentication, permission checks"],
        ["`handlers.go`", "795", "Every API handler except report and deck export"],
        ["`models.go`", "206", "The domain types and the API envelope"],
        ["`store.go`", "710", "In-memory store, sessions, seeding, audit and activity"],
        ["`db.go`", "318", "SQLite snapshot persistence and the persist middleware"],
        ["`report.go`", "539", "`ReportConfig`, export handlers, file naming, deck handler"],
        ["`docxmcollab.go`", "1769", "The DOCX renderer: areas, findings, tables, TOC, charts"],
        ["`docxmcollabpkg.go`", "510", "Package assembly: parts, relationships, customer logo"],
        ["`docxxml.go`", "494", "The OOXML helpers every renderer shares"],
        ["`docxtestcases.go`", "373", "Test-type checklist derivation (Pass / Issues)"],
        ["`docxfooter.go`", "320", "Footer tab-stop fitting"],
        ["`docxlogo.go`", "211", "Logo decoding: SVG rasterising, WEBP, scaling"],
        ["`docxpoc.go`", "106", "Screenshot parts and the inline / anchored drawing XML"],
        ["`docx2pdf.go`", "262", "Word COM and LibreOffice conversion"],
        ["`pptxclosure.go`", "460", "Closure deck package handling and slide cloning"],
        ["`pptxslides.go`", "615", "Slide filling: issues tables, scenarios, media, logo"],
        ["`pptxsummary.go`", "496", "Executive summary panels and their fitting"],
        ["`pptxcharts.go`", "196", "Rebuilding the deck's donut and bar charts"],
        ["`onedrive.go`", "267", "Microsoft Graph client-credential upload"],
    ]), [1.9, 0.7, 3.9]),

("h3", "Other directories"),
("table", (
    ["Path", "Contents"],
    [
        ["`backend/static/`",
         "The SPA: `index.html`, `js/api.js`, `js/state.js`, `js/pages.js`, `css/app.css` "
         "and the brand images. Embedded into the binary."],
        ["`backend/templates/`",
         "`mcollaborator_template.docx` and `mcollaborator_closure_template.pptx`, both "
         "embedded with `go:embed`, plus the superseded `vapt_report_template*.docx`"],
        ["`backend/tools/mkclosuretemplate/`",
         "The command that derived the PPTX template from a real closing deck; kept as "
         "the record of what was removed"],
        ["`backend/data/`",
         "Created at run time: `mcollaborator.db` and `uploads/`. Never committed."],
        ["`backend/reports/`",
         "Generated deliverables. Git-ignored - these carry client findings."],
        ["`desktop/`",
         "The Wails shell: `main.go`, `server.go`, `reports.go`, platform files, "
         "`build.ps1`, `wails.json`, and `dist/` with the shipping binaries"],
        ["`docs/`",
         "This document, the iteration notes, the collaboration and SharePoint design "
         "papers, and the template assets"],
    ]), [1.9, 4.6]),

("pagebreak",),

# ------------------------------------------- 4. Installation and configuration
("h2", "Installation and Configuration"),

("h3", "Prerequisites"),
("table", (
    ["Requirement", "Needed for", "Notes"],
    [
        ["Go 1.25 or newer", "Building the server and the shell", "No cgo toolchain required"],
        ["Microsoft Word", "PDF export on Windows",
         "Highest fidelity, and the only engine that also rebuilds the table of contents"],
        ["LibreOffice", "PDF export anywhere else",
         "`soffice` on `PATH` or in a standard install location"],
        ["Wails CLI v2", "Building the desktop app", "`go install github.com/wailsapp/wails/v2/cmd/wails@latest`"],
        ["NSIS", "Building the Windows installer", "Optional; `build.ps1` skips the installer without it"],
        ["Git LFS", "Cloning the repository", "The shipping `.exe` files are LFS objects"],
    ]), [1.6, 1.8, 3.1]),

("h3", "Building and running the server"),
("code", """
# from backend/
go build -o mCollaborator.exe .
go test ./...

# run (PORT defaults to 9900)
.\\mCollaborator.exe

# then open http://localhost:9900
"""),
("p", "First sign-in on a fresh database is `admin@cyberteq.com` / `admin123`. That is a "
      "bootstrap credential and the first thing to change. A fresh install seeds nothing "
      "else: no demo engagements, findings or users."),

("h3", "Environment variables"),
("table", (
    ["Variable", "Default", "Effect"],
    [
        ["`PORT`", "`9900`", "TCP port the server listens on"],
        ["`MCOLLABORATOR_DB_PATH`", "`data/mcollaborator.db`",
         "Location of the SQLite file; the directory is created if missing"],
        ["`STITCH_DB_PATH`", "-",
         "Honoured when the newer variable is unset, so a deployment predating the "
         "rename keeps its database instead of silently opening an empty one"],
        ["`OD_TENANT_ID`", "-", "Entra ID directory (tenant) ID for OneDrive filing"],
        ["`OD_CLIENT_ID`", "-", "Application (client) ID"],
        ["`OD_CLIENT_SECRET`", "-", "Client secret"],
        ["`OD_USER`", "-",
         "UPN of the OneDrive that receives the files, e.g. `reports@cyberteqfalcon.com`"],
        ["`OD_FOLDER`", "`VAPT Reports`", "Root folder inside that drive"],
    ]), [2.1, 1.5, 2.9]),
("p", "The front end has one tunable of its own: setting `window.FINDINGS_REFRESH_MS` "
      "before the application loads changes how often an open engagement polls for other "
      "analysts' changes. The default is 15,000 ms."),

("h3", "Runtime directories"),
("table", (
    ["Path", "Holds", "Back up?"],
    [
        ["`data/mcollaborator.db`", "Everything the application knows", "Yes"],
        ["`data/uploads/`", "Evidence files, named `<uuid>_<original name>`", "Yes"],
        ["`backend/reports/`", "Generated DOCX, PDF and PPTX deliverables",
         "Regenerable, but they are client documents - treat them as sensitive"],
        ["`%APPDATA%\\\\mCollaborator\\\\`",
         "Desktop shell logs, WebView2 cache and staged files opened from the app", "No"],
    ]), [2.2, 3.0, 1.3]),

("h3", "Deploying for a team on the office LAN"),
("bullets", [
    "Run the server on one Windows machine; analysts open `http://<server-ip>:9900`. That "
    "machine should be the one with Word installed, so PDF export works for everyone.",
    "Add an inbound Windows Firewall rule for TCP 9900 - without it, nothing outside the "
    "host can reach the server.",
    "Other analysts' changes appear within the polling window, 15 seconds by default.",
    "Everything lives in `data/`. Stopping the server, copying that directory and "
    "starting it again is a complete backup and restore.",
    "There is one writer: the SQLite connection pool is capped at a single connection and "
    "the whole store is written as one snapshot. Do not point two servers at one file.",
]),

("pagebreak",),

# --------------------------------------------------------- 5. Security model
("h2", "Security Model"),

("h3", "Authentication"),
("bullets", [
    "`POST /api/v1/auth/login` verifies the password and creates a session valid for "
    "**24 hours**. The token is returned to the browser and sent back as "
    "`Authorization: Bearer <token>`.",
    "Passwords are stored as **bcrypt** hashes at the library's default cost. A legacy "
    "SHA-256 scheme is still accepted on comparison so accounts created before bcrypt can "
    "still sign in; its salt prefix is a stored credential parameter and is deliberately "
    "not renamed.",
    "Passwords must be at least **8 characters**. Accounts carry a 90-day "
    "`password_expiry` and every non-admin account is created with "
    "`must_change_password` set - the interface will not let such a user leave the "
    "change-password screen until they set their own.",
    "`POST /api/v1/users/me/password` re-authenticates with the current password before "
    "changing it, clears the first-login flag and resets the expiry.",
]),

("h3", "Roles and permissions"),
("p", "A role is a fixed set of permissions, assigned when the account is created. "
      "Administrators bypass the permission check entirely."),
("table", (
    ["Permission", "Grants", "admin", "analyst", "project_manager"],
    [
        ["`engagements:write`", "Create, update and delete engagements and nodes", "Yes", "Yes", "No"],
        ["`finding:write`", "Create, edit, assign and bulk-import findings", "Yes", "Yes", "No"],
        ["`vault:read`", "Read evidence records", "Yes", "Yes", "No"],
        ["`vault:write`", "Upload and delete evidence", "Yes", "Yes", "No"],
        ["`report:export`", "Generate reports and closing decks", "Yes", "Yes", "Yes"],
        ["`admin:users`", "List, create and remove users", "Yes", "No", "No"],
        ["`admin:findings`", "Delete a finding outright", "Yes", "No", "No"],
    ]), [1.7, 2.4, 0.7, 0.8, 1.3]),

("h3", "Accountability"),
("bullets", [
    "**Audit log** - append-only records of `report.export`, `evidence.upload`, "
    "`user.password_change` and the other consequential actions, each with actor, "
    "resource, resource id and source address.",
    "**Activity feed** - the human-readable stream the dashboard shows: who uploaded "
    "what, who raised which finding, against which engagement.",
    "**Finding versions** - every finding carries a `version` that increments on update, "
    "which is what an optimistic-concurrency check would key on.",
]),

("h3", "Input handling"),
("bullets", [
    "The SPA escapes user-supplied text before putting it into markup and rejects obvious "
    "injection attempts in form fields.",
    "Report and evidence downloads reduce the requested name to a single path leaf before "
    "opening anything, so a name can never steer the read out of the reports directory.",
    "Generated file names are stripped of the characters Windows and Microsoft Graph "
    "reject, while keeping the spaces and ampersands a client's name legitimately "
    "contains.",
]),

("h3", "Known gaps"),
("p", "These are deliberate simplifications for a single-team, single-LAN deployment. "
      "They are the first things to revisit if the application is ever exposed more "
      "widely."),
("table", (
    ["Gap", "Detail"],
    [
        ["Export endpoints are unauthenticated",
         "`POST /reports/export`, `POST /reports/closure` and "
         "`GET /reports/download/{type}/{name}` are registered outside the authenticated "
         "group so the browser and the desktop window can fetch a document without "
         "attaching a token. Anyone who can reach the port can therefore generate and "
         "download reports."],
        ["CORS allows any origin",
         "`Access-Control-Allow-Origin: *` with credentials allowed. Acceptable on a "
         "closed LAN, not beyond it."],
        ["No rate limiting", "The middleware is a placeholder that calls the next handler."],
        ["Last write wins",
         "`UpdateFindingWithVersion` exists in the store but no handler routes through "
         "it, so two analysts editing one finding overwrite each other silently."],
        ["No transport encryption",
         "The server speaks plain HTTP. Put it behind a reverse proxy with TLS if it "
         "leaves the office network."],
    ]), [2.0, 4.5]),

("pagebreak",),

# ------------------------------------------------------------- 6. Data model
("h2", "Data Model"),
("p", "All types are defined in `models.go` and serialised as JSON on the wire and into "
      "the snapshot."),

("h3", "Entities"),
("table", (
    ["Entity", "Key fields", "Notes"],
    [
        ["`Org`", "id, name, slug", "One organisation, `org_001`, seeded on first run"],
        ["`User`", "id, email, name, role, permissions, password_expiry, must_change_password",
         "`PasswordHash` is never serialised to the API"],
        ["`Session`", "token, user_id, expires_at", "24-hour lifetime"],
        ["`Engagement`", "id, name, client_name, status, methodology, scope, timeline, team",
         "Status runs planning, in_progress, review, then closed or completed"],
        ["`Node`", "id, engagement_id, target, type", "A host, application or other target in scope"],
        ["`Finding`", "See the next table", "The unit the report is built from"],
        ["`Evidence`", "id, engagement_id, finding_id, filename, storage_key, mime_type, "
         "size_bytes, hash_sha256", "Bytes live on disk; the record lives in the store"],
        ["`Comment`", "id, finding_id, user_id, body", "Discussion on a finding"],
        ["`ActivityItem`", "action, detail, user_name, engagement_id", "The dashboard feed"],
        ["`AuditLog`", "actor_id, action, resource, resource_id, ip_address", "Append-only"],
        ["`Report` / `ReportRecord`", "id, engagement_id, title, template, format, file_path",
         "Report metadata held alongside the generated files"],
    ]), [1.5, 2.6, 2.4]),

("h3", "The finding"),
("table", (
    ["Field", "Meaning"],
    [
        ["`title`", "The vulnerability name, printed as the finding heading"],
        ["`severity`", "critical, high, medium, low or informational"],
        ["`cvss_vector`, `cvss_score`", "Printed in the finding detail table; the vector also supplies the attack vector when that field is blank"],
        ["`category` / `area`", "The assessment area the finding is reported under. `area` wins; `category` is the older, looser field and is mapped onto an area code"],
        ["`status`", "draft, open or in_progress"],
        ["`description`, `impact`, `likelihood`", "Body text of the finding"],
        ["`poc`", "Proof-of-concept narrative; screenshots come from `evidence_ids`"],
        ["`remediation`", "The recommendation; its first sentence becomes the short recommendation heading when none is given"],
        ["`cve`, `cwes`, `mitre_attack_ids`", "References"],
        ["`evidence_ids`", "Evidence records attached as proof"],
        ["`assigned_to`, `created_by`, `version`", "Ownership and change tracking"],
    ]), [2.0, 4.5]),

("h3", "Assessment areas"),
("p", "One list, defined in `docxmcollab.go`, ties an area to every place the template "
      "mentions it: the scope table, the naming convention list, its chapter 3 heading and "
      "its bar in the findings-by-area chart. Selecting areas in the wizard is therefore "
      "asked once and drives all four."),
("table", (
    ["Code", "Area", "Chapter 3 heading", "Chart label", "Exposure"],
    [
        ["IPT", "Internal Penetration Testing", "Internal Penetration Testing", "Internal", "Internal"],
        ["EPT", "External Penetration Testing", "External Penetration Testing", "External", "External"],
        ["IPTC", "Internal Cloud Penetration Testing", "Internal Cloud Penetration Testing", "Internal Cloud", "Internal Cloud"],
        ["WPT", "Web Application Penetration Testing", "Web Application Penetration Testing", "Web Apps", "Web"],
        ["CFG", "Configuration Files Review", "Configuration Files Review", "Config Review", "Configuration"],
        ["ASA", "API Security Assessment", "(no section in the template)", "APIs", "API"],
        ["ADT", "Active Directory Testing", "Active Directory Testing", "Active Directory", "Internal"],
        ["WNA", "Wireless Network Assessment", "Wireless Network Penetration Testing", "Wireless", "Wireless"],
        ["NAR", "Network Architecture Review", "Network Architecture Review", "Network Architecture", "Internal"],
    ]), [0.65, 1.75, 1.85, 1.0, 0.95]),

("h3", "Severity colours"),
("p", "The criticality word itself is coloured; the cell keeps the template's own "
      "background, so a rating reads as a coloured word rather than a block of colour "
      "with white text knocked out of it."),
("table", (
    ["Severity", "Hex", "Note"],
    [
        ["Critical", "C00000", "Deep red"],
        ["High", "FF7900", "Cyberteq orange"],
        ["Medium", "BF8F00", "Darker than the amber the criteria table fills a cell with, which is illegible as text on white"],
        ["Low", "28A745", "Green"],
        ["Informational", "2F6FED", "Blue"],
    ]), [1.3, 1.0, 4.2]),

("pagebreak",),

# ------------------------------------------------ 7. Persistence and storage
("h2", "Persistence and Storage"),

("h3", "How state is held"),
("p", "The store is a set of maps behind one `sync.RWMutex`. Durability is a snapshot: "
      "after any mutating request, `persistMiddleware` serialises the entire store to JSON "
      "and writes it into a single SQLite row. On start-up the snapshot is read back; only "
      "when there is none does the application seed the organisation and the bootstrap "
      "administrator."),
("table", (
    ["Aspect", "Detail"],
    [
        ["Table", "`store_snapshot(key TEXT PRIMARY KEY, value TEXT, updated_at TEXT)`, one row keyed `all`"],
        ["Journal mode", "WAL, with `busy_timeout=5000` and `synchronous=NORMAL`"],
        ["Connections", "Capped at one, because every write rewrites the same row"],
        ["Password hashes", "Persisted through a dedicated `userSnapshot` type - the live `User` marks the hash `json:\"-\"`, and a snapshot taken through that type would strip every password on restart"],
        ["Cost", "The whole store is rewritten per mutation. Comfortable for a team's engagements; not a design for tens of thousands of findings"],
    ]), [1.6, 4.9]),

("h3", "Evidence files"),
("bullets", [
    "Upload accepts a multipart form of up to 500 MB per request and reads the file fully "
    "into memory before storing it.",
    "The bytes are written to `data/uploads/<uuid>_<sanitised name>` and the record keeps "
    "the SHA-256 of the content, its size and its MIME type.",
    "`GET /api/v1/evidence/{id}/file` serves the bytes inline, which is what lets an "
    "image render in the finding view.",
    "The report and the deck read from these same files when they embed a screenshot; "
    "an evidence record whose file has gone missing is logged and skipped rather than "
    "failing the whole export.",
]),

("h3", "Backing up"),
("code", """
# stop the server first: the WAL should be checkpointed cleanly
Copy-Item -Recurse backend\\data  \\\\backup-server\\mcollaborator\\data-2026-08-26
"""),
("p", "Copying `data/` captures the database, its write-ahead log and every evidence "
      "file. Restoring is the reverse: put the directory back and start the server."),

("pagebreak",),

# ---------------------------------------------------- 8. HTTP API reference
("h2", "HTTP API Reference"),
("p", "Every endpoint lives under `/api/v1`. Responses are wrapped: a success carries "
      "`{\"data\": ...}` and a failure `{\"error\": {\"code\": ..., \"message\": ...}}`. "
      "Protected endpoints need `Authorization: Bearer <token>`; the permission column "
      "names what a non-admin must hold."),

("h3", "Authentication and users"),
("table", (
    ["Method and path", "Permission", "Purpose"],
    [
        ["`POST /auth/login`", "public", "Exchange email and password for a 24-hour token"],
        ["`POST /auth/logout`", "public", "Invalidate the current session"],
        ["`GET /users/me`", "any session", "The signed-in user, including permissions"],
        ["`POST /users/me/password`", "any session", "Change own password"],
        ["`GET /users`", "`admin:users`", "List the organisation's users"],
        ["`POST /users`", "`admin:users`", "Create a user with a role and initial password"],
        ["`DELETE /users/{id}`", "`admin:users`", "Remove a user"],
    ]), [2.4, 1.4, 2.7]),

("h3", "Engagements, nodes and findings"),
("table", (
    ["Method and path", "Permission", "Purpose"],
    [
        ["`GET /engagements`", "any session", "List engagements"],
        ["`POST /engagements`", "`engagements:write`", "Create an engagement"],
        ["`GET /engagements/{id}`", "any session", "One engagement"],
        ["`PUT /engagements/{id}`", "`engagements:write`", "Update"],
        ["`DELETE /engagements/{id}`", "`engagements:write`", "Delete"],
        ["`GET /engagements/{id}/nodes`", "any session", "Targets in scope"],
        ["`POST /engagements/{id}/nodes`", "`engagements:write`", "Add a target"],
        ["`GET /engagements/{id}/findings`", "any session", "Findings for the engagement"],
        ["`POST /engagements/{id}/findings`", "`finding:write`", "Raise a finding"],
        ["`POST /engagements/{id}/findings/bulk`", "`finding:write`", "Import an array of findings"],
        ["`GET /engagements/{id}/findings/changes`", "any session", "Poll for changes since a timestamp"],
        ["`GET /findings/{id}`", "any session", "One finding"],
        ["`PUT /findings/{id}`", "`finding:write`", "Update a finding"],
        ["`PATCH /findings/{id}/status`", "`finding:write`", "Change status only"],
        ["`POST /findings/{id}/assign`", "`finding:write`", "Assign to a user"],
        ["`DELETE /findings/{id}`", "`admin:findings`", "Delete a finding"],
        ["`GET /findings/{id}/comments`", "any session", "Comments on a finding"],
        ["`POST /findings/{id}/comments`", "any session", "Comment on a finding"],
    ]), [2.8, 1.4, 2.3]),

("h3", "Evidence"),
("table", (
    ["Method and path", "Permission", "Purpose"],
    [
        ["`GET /evidence`", "any session", "List evidence records"],
        ["`POST /evidence/upload`", "`vault:write`", "Upload a file as multipart form data"],
        ["`GET /evidence/{id}`", "any session", "One evidence record"],
        ["`GET /evidence/{id}/file`", "any session", "The stored bytes, served inline"],
        ["`DELETE /evidence/{id}`", "`vault:write`", "Delete a record and its file"],
    ]), [2.4, 1.4, 2.7]),

("h3", "Reports, analytics and health"),
("table", (
    ["Method and path", "Permission", "Purpose"],
    [
        ["`POST /reports/export`", "none - see Security Model", "Generate the DOCX and PDF from a `ReportConfig`"],
        ["`POST /reports/closure`", "none - see Security Model", "Generate the closing-meeting deck"],
        ["`GET /reports/download/{type}/{name}`", "none - see Security Model", "Download a generated `docx`, `pdf` or `pptx`"],
        ["`GET /reports`", "any session", "Report records"],
        ["`POST /reports`", "`report:export`", "Create a report record"],
        ["`GET /reports/{id}`", "any session", "One report record"],
        ["`POST /reports/{id}/export`", "`report:export`", "Export an existing report record"],
        ["`GET /activities`", "any session", "Activity feed"],
        ["`GET /analytics/findings-over-time`", "any session", "Findings raised per day"],
        ["`GET /analytics/severity-count`", "any session", "Count by severity"],
        ["`GET /analytics/status-count`", "any session", "Count by status"],
        ["`GET /analytics/top-assets`", "any session", "Most-affected targets"],
        ["`GET /analytics/recurring-cwes`", "any session", "Recurring weakness classes"],
        ["`GET /analytics/global-severity`", "any session", "Severity mix across engagements"],
        ["`GET /health`", "public", "`{\"status\":\"ok\",\"version\":\"1.0.0\"}`"],
    ]), [3.0, 1.5, 2.0]),
("p", "Anything not matching a route is served from the embedded SPA, and any unknown "
      "path falls back to `index.html` so client-side routes survive a refresh."),

("pagebreak",),

# ------------------------------------------------------- 9. Web application
("h2", "Web Application"),
("p", "The interface is three JavaScript files and a stylesheet, served from inside the "
      "binary. There is no framework and no build step: `pages.js` renders HTML strings, "
      "`state.js` owns routing and session state, and `api.js` is a thin fetch wrapper "
      "that attaches the bearer token."),

("h3", "Routes"),
("table", (
    ["Route", "Page", "Who sees the tab"],
    [
        ["`#/login`", "Sign-in", "Everyone, signed out"],
        ["`#/change-password`", "Forced password change", "First sign-in for non-admins"],
        ["`#/dashboard`", "Dashboard: counts, severity mix, recent activity", "Everyone"],
        ["`#/ledger/project`", "Project ledger: engagements and their findings", "Everyone"],
        ["`#/finding-editor`", "Finding editor", "admin, analyst"],
        ["`#/finding-detail`", "One finding with evidence and comments", "admin, analyst"],
        ["`#/evidence`", "Evidence vault", "admin, analyst"],
        ["`#/reports`", "Report generator wizard", "Everyone"],
        ["`#/closure-prep`", "Closing deck generator", "Everyone"],
        ["`#/admin/users`", "User management", "admin"],
        ["`#/command/engagements`, `#/command/feed`, `#/command/report-builder`",
         "Command views: active engagements, live vulnerability feed, report builder", "Direct link"],
    ]), [2.3, 2.8, 1.4]),

("h3", "Behaviour worth knowing"),
("bullets", [
    "**Themes.** Three: Cyberpunk, Ledger and Slate. The choice is kept in "
    "`localStorage` and applied as a `data-theme` attribute on the root element.",
    "**Session restore.** The token is read from `localStorage` on load and validated "
    "against `/users/me`; a rejected token logs the user out cleanly. Keys from before "
    "the rename are migrated rather than dropped, so nobody is signed out by an upgrade.",
    "**Live findings.** While an engagement is open the page polls "
    "`/findings/changes` every 15 seconds so another analyst's work appears without a "
    "refresh.",
    "**Bulk import.** The findings view accepts a pasted JSON array - `title`, `severity` "
    "and `description` required - and posts it to the bulk endpoint.",
    "**Role-aware navigation.** Tabs are built from the signed-in user's role, so a "
    "project manager never sees the Findings or Evidence tabs.",
]),

("h3", "The report wizard"),
("p", "Five steps, held in one client-side state object and sent as a single "
      "`ReportConfig` payload when Generate is pressed."),
("table", (
    ["Step", "Collects", "Where it lands in the report"],
    [
        ["1. Report details",
         "Client name and initials, report title, reference number, version label, report "
         "date, assessment start and end, tester, approver and role, client logo",
         "Cover page, running headers, footers, and the logo slots"],
        ["2. Areas and scope",
         "Which assessment areas were performed and the scope text for each, plus "
         "out-of-scope items and the tool list",
         "Scope table, naming convention, chapter 3 sections, findings-by-area chart"],
        ["3. Findings",
         "Which findings to report and the area each belongs to",
         "Chapter 3 detail, the vulnerability register and both charts"],
        ["4. Appendix",
         "Pre-existing and newly created test accounts",
         "The appendix - omitted entirely when both tables are empty"],
        ["5. Generate",
         "Export format, OneDrive sync and destination folder",
         "The export itself, plus the summary shown before it runs"],
    ]), [1.4, 2.6, 2.5]),
("p", "Closure Prep reads the same wizard state, which is why it asks you to complete the "
      "wizard first: the deck and the report are built from one set of answers so the two "
      "cannot disagree."),

("pagebreak",),
]
