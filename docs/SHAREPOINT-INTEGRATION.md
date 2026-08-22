# mCollaborator — SharePoint Online Integration
## Technical Design & Implementation Plan

**Status:** Draft for review
**Applies to:** `backend/` (Go, chi, embedded SPA) at commit `950d238`
**Related:** `COLLABORATION-DESIGN.md` (option analysis), `M365-TRIAL-SETUP.md` (tenant runbook)

---

# 1. System Overview

mCollaborator today has **no persistence**. `store.go` holds eleven `map[string]*T` fields
plus two append-only slices behind a single `sync.RWMutex`, seeded by `seed()` on every
process start. All analyst work
is lost on restart, and two analysts collaborate only if they happen to hit the same running
binary.

This document specifies how **SharePoint Online**, accessed through **Microsoft Graph**,
becomes the system of record for engagements, findings, evidence and reports — what has to
be built, and precisely **what access the application must be granted** to do it.

### Design Tenets

- **Backend is the only Graph client** — no Graph token ever reaches the browser. CVSS
  validation, RBAC, audit logging and report generation stay in one enforcement point.
- **Least privilege, provably** — the runtime identity holds `Sites.Selected` and is granted
  `write` on exactly one site. It cannot enumerate or touch any other site in the tenant.
- **SharePoint is the source of truth, memory is a cache** — reads are served from a
  write-through cache; every write goes to Graph first and is only cached on success.
- **Degrade, don't fail** — Graph is a remote dependency with throttling and multi-second
  webhook latency. Every path has a defined behaviour when Graph is slow, throttled, or down.
- **Governance is the reason we are here** — if the data does not end up inheriting tenant
  retention, DLP and eDiscovery, this integration has bought nothing that SQLite would not
  have bought more cheaply.

### Non-Goals

| Out of scope | Why |
|---|---|
| Google-Docs-style co-typing in the finding editor | Requires field-level locking or a CRDT. Graph webhook latency (2–10 s) makes this structurally impossible on SharePoint. |
| Browser talking to SharePoint directly | Would ship Graph tokens to the SPA and bypass all server-side validation. |
| Migrating user accounts / sessions to SharePoint | Sessions are ephemeral and high-churn. They stay in process memory (see §6.7). |
| Replacing the DOCX report engine | `report.go` is unchanged. Only the *destination* of the generated file moves (§8). |

---

# 2. High-Level Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│                          CLIENT LAYER                                │
│   Analyst browser — vanilla-JS SPA served from embed.FS              │
│   • REST  /api/v1/*        (bearer session token)                    │
│   • SSE   /api/v1/stream   (replaces the 15 s findings poll)         │
└───────────────────────────────┬──────────────────────────────────────┘
                                │  HTTPS
┌───────────────────────────────▼──────────────────────────────────────┐
│                    mCollaborator BACKEND (Go)                        │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │  chi router  →  authMiddleware  →  requirePermission            │  │
│  └───────────────────────────┬────────────────────────────────────┘  │
│  ┌───────────────────────────▼────────────────────────────────────┐  │
│  │                    Store  (interface)                          │  │
│  │   ┌──────────────┐   ┌───────────────────────────────────┐    │  │
│  │   │ MemoryStore  │   │        SharePointStore            │    │  │
│  │   │ (today, CI)  │   │  write-through cache + sync loop  │    │  │
│  │   └──────────────┘   └────────────────┬──────────────────┘    │  │
│  └────────────────────────────────────────┼─────────────────────-─┘  │
│  ┌──────────────────┐  ┌──────────────────▼──────────────────────┐  │
│  │  SSE Hub         │◄─┤  Sync Engine                            │  │
│  │  fan-out to      │  │  • delta poll (deltaLink persisted)     │  │
│  │  connected SPAs  │  │  • webhook receiver + renewal job       │  │
│  └──────────────────┘  │  • ETag / 412 conflict resolution       │  │
│                        └──────────────────┬──────────────────────┘  │
│  ┌────────────────────────────────────────▼──────────────────────┐  │
│  │  Graph Client — the ONLY holder of credentials                │  │
│  │  • MSAL client-credentials (cert preferred over secret)       │  │
│  │  • 429/Retry-After backoff, $batch, upload sessions           │  │
│  └────────────────────────────────────────┬──────────────────────┘  │
└───────────────────────────────────────────┼──────────────────────────┘
                                            │  Microsoft Graph v1.0
┌───────────────────────────────────────────▼──────────────────────────┐
│              SharePoint Online — site "VAPT" (one site only)         │
│   Lists:      Engagements · Findings · Nodes · Comments · Activity    │
│   Libraries:  Evidence (PoC, PCAP, logs) · Reports (DOCX/PDF)        │
│                                                                      │
│   Inherits: Entra ID access control · retention labels · DLP ·       │
│             eDiscovery · unified audit log · versioning              │
└──────────────────────────────────────────────────────────────────────┘
```

The SPA contract is unchanged apart from SSE. Every handler in `handlers.go` keeps its
signature; only what sits behind `Store` changes.

---

# 3. Component Selection

| Concern | Choice | Rationale |
|---|---|---|
| Graph transport | `net/http` + hand-rolled client | Graph is plain REST/OData. The official `microsoftgraph/msgraph-sdk-go` pulls a very large dependency tree (Kiota) for what amounts to eight endpoints. |
| Token acquisition | `github.com/AzureAD/microsoft-authentication-library-for-go` (MSAL Go) | Handles client-credentials + certificate auth, in-memory token cache, and refresh-before-expiry correctly. Do not hand-roll this. |
| Credential type | **Certificate** in production, client secret in the sandbox | Secrets expire silently and end up in env files. A cert can live in the OS store / Key Vault. Sandbox uses a secret for speed (see `M365-TRIAL-SETUP.md` §4). |
| Change detection | Graph **delta tokens**, webhooks as an accelerator | Delta is authoritative and needs no inbound network. Webhooks only shorten latency (§10). |
| Browser push | **Server-Sent Events** | One-way, works over plain HTTP/1.1, auto-reconnects, no handshake code. WebSockets buy nothing here. |
| Local cache | Existing in-memory maps, promoted to a write-through cache | Keeps p50 read latency at microseconds instead of 100–400 ms per Graph call. |

---

# 4. Access Model — What mCollaborator Needs

This is the section to hand to whoever approves the request. It is deliberately explicit
about what is asked for **and what is not**.

## 4.1 Identity summary

| Item | Value |
|---|---|
| Identity type | Entra ID **application** (daemon / service principal), single-tenant |
| Suggested name | `mCollaborator-Backend` |
| Redirect URI | None — non-interactive |
| Credential | X.509 certificate (prod) / client secret, 6-month expiry (sandbox) |
| Graph permission | **`Sites.Selected`** — application permission, admin consent required |
| Effective scope | Exactly one SharePoint site, granted `write` |
| Sign-in surface | None. The app never presents a UI and cannot be phished. |

## 4.2 The Graph permission: `Sites.Selected`

`Sites.Selected` is unusual and it is the whole argument for approvability: **granting it
confers no access to anything.** The application can authenticate successfully and still
read nothing. Access exists only for sites an administrator has explicitly authorised, one
site at a time, via a separate `POST /sites/{siteId}/permissions` call.

| Permission | Effective reach | Verdict |
|---|---|---|
| `Sites.ReadWrite.All` | Read/write **every site in the tenant** — HR, Finance, Legal, every Teams-backed site | **Do not request.** An admin should refuse this. |
| `Sites.Read.All` | Read every site in the tenant | Still tenant-wide. Refuse. |
| **`Sites.Selected`** | Nothing by default; per-site grants only | **Request this.** |

The site grant itself is the second half of the request:

```http
POST https://graph.microsoft.com/v1.0/sites/{siteId}/permissions
Content-Type: application/json

{
  "roles": ["write"],
  "grantedToIdentities": [
    { "application": { "id": "{client-id}", "displayName": "mCollaborator-Backend" } }
  ]
}
```

Available roles are `read`, `write`, `manage`, `fullcontrol`. **`write` is sufficient for
every runtime operation** in §4.4. `fullcontrol` is required only to modify site
permissions, which the application never does.

## 4.3 What the application explicitly does not need

Stating this up front shortens the security review:

| Not requested | Consequence of not having it |
|---|---|
| `Sites.ReadWrite.All` / `Sites.Read.All` | Cannot see any site other than the granted one. Intended. |
| `Files.ReadWrite.All` | Drive access is reached *through* the granted site, so this is redundant. |
| `User.Read.All` / `Directory.Read.All` | Analyst display names come from mCollaborator's own user table, not from Entra. |
| `Sites.FullControl.All` | The app cannot change site permissions, add site owners, or grant itself more access. |
| `Mail.Send` | No notification email from the service identity. |
| Any Exchange / Teams / OneDrive scope | Out of scope entirely. |

## 4.4 Least-privilege matrix — operation → required access

| mCollaborator operation | Graph call | Needs |
|---|---|---|
| List / read findings | `GET /sites/{s}/lists/{l}/items?$expand=fields` | site `read` |
| Create / update finding | `POST` / `PATCH .../items[/{id}/fields]` | site `write` |
| Delete finding | `DELETE .../items/{id}` | site `write` |
| Upload evidence ≤ 4 MB | `PUT /sites/{s}/drives/{d}/items/root:/{path}:/content` | site `write` |
| Upload evidence > 4 MB | `POST .../createUploadSession` then chunked `PUT` | site `write` |
| Download evidence | `GET /sites/{s}/drives/{d}/items/{id}/content` | site `read` |
| Publish generated report | `PUT` into the `Reports` library | site `write` |
| Change feed | `GET .../lists/{l}/items/delta` | site `read` |
| Webhook subscription | `POST /subscriptions` on `sites/{s}/lists/{l}` | site `read` on the resource — **verify, see §10.3** |
| Provision lists / columns | `POST /sites/{s}/lists` | Performed **once, by an admin**, out of band (§5.5). Not a runtime capability. |

The application therefore needs `write` on one site. Nothing above requires `manage` or
`fullcontrol`.

## 4.5 Phase 2 — delegated access (optional, recommended end state)

App-only auth means SharePoint records every change as being made by
`mCollaborator-Backend`, not by the analyst. Mitigation in phase 1 is a `ModifiedByUser`
column written by the backend from the session user (§6.2). The correct fix is delegated
auth:

| Item | Value |
|---|---|
| Flow | Authorization Code + PKCE (SPA sign-in) → **On-Behalf-Of** exchange in the backend |
| Delegated scopes | `Sites.Selected`, `User.Read`, `offline_access` |
| Gains | Real per-user `Modified By`; native SharePoint audit attribution; Entra Conditional Access and MFA inherited for free; **replaces the bcrypt-in-memory login in `store.go` entirely** |
| Costs | Token cache + refresh handling, consent flow, per-user throttling buckets, sessions become Entra-shaped |

Sequencing note: if delegated sign-in is going to happen at all, **do it before** the
SharePoint client is written. Retrofitting per-user tokens into a client built around a
single app identity means rewriting the client's auth layer and every call site.

## 4.6 Human access — SharePoint site permissions

Separate from the application identity, people need site access. This is what makes the
SharePoint option worth its cost.

| Group | SharePoint role | Who | Purpose |
|---|---|---|---|
| `VAPT Owners` | Full Control | Platform admin (1–2 people) | Provision lists, manage permissions, rotate the app grant |
| `VAPT Members` | Edit | Analysts | Direct list access for bulk edits and troubleshooting |
| `VAPT Visitors` | Read | Engagement managers, QA reviewers | Read findings in the SharePoint UI **without an mCollaborator account** |
| Per-engagement client access | Read, scoped to a filtered view or dedicated library folder | External reviewers (Entra B2B guests) | Optional. See the open question in §20. |

mCollaborator's own RBAC (`admin`, and the permission strings `finding:write`,
`vault:read`, `vault:write`, `report:export`, `engagements:write`, `admin:users` seeded in
`store.go`) is enforced in `requirePermission` and is **independent** of SharePoint
permissions. Under app-only auth these two models are not linked at all — a user who lacks
`finding:write` in mCollaborator is blocked by middleware before Graph is ever reached, but
if they have direct SharePoint Edit rights they can still change the list item. That gap is
inherent to phase 1 and closes with delegated auth. Keep `VAPT Members` small.

## 4.7 Credential handling

```
SHAREPOINT_TENANT_ID       Directory (tenant) ID
SHAREPOINT_CLIENT_ID       Application (client) ID
SHAREPOINT_CLIENT_SECRET   Secret VALUE (sandbox only — never the secret ID)
SHAREPOINT_CERT_PATH       PKCS#12 / PEM path (production, replaces the secret)
SHAREPOINT_SITE_ID         "host,siteCollectionId,webId" triple from §5.2
```

Rules:

- Environment or secret store only. Never in the working tree — `.gitignore` covers `.env`
  patterns, but the durable habit is that secrets never enter the repo directory at all.
- Secrets are **not** logged. `loggingMiddleware` logs method/path/status only; the Graph
  client must not log `Authorization` headers or token responses.
- Calendar the secret expiry. The failure mode is total and looks like an outage: every
  Graph call returns `401 invalid_client` at the same instant.
- Rotation is zero-downtime — Entra allows two active secrets/certs. Add the new one, deploy,
  then remove the old.

## 4.8 Conditional Access

Client-credentials flows are **not** subject to user-based Conditional Access, but they are
subject to **workload identity** CA policies where licensed. If the tenant enforces one:

- Location-based policies require the backend's egress IP to be allow-listed. Note this
  before deployment, not after the first 403.
- A policy blocking legacy/unapproved service principals will block the app until it is
  explicitly exempted.

Once phase 2 delegated auth is in, analyst sign-in *does* inherit user CA and MFA, which is
one of the strongest arguments for it.

---

# 5. SharePoint Information Architecture

## 5.1 Site topology

```
https://{tenant}.sharepoint.com/sites/VAPT
├── Lists
│   ├── Engagements    ← Engagement
│   ├── Findings       ← Finding      (the vulnerability system of record)
│   ├── Nodes          ← Node         (scope / targets)
│   ├── Comments       ← Comment
│   └── Activity       ← ActivityItem
└── Document libraries
    ├── Evidence       ← Evidence     (PoC images, PCAPs, logs)
    └── Reports        ← generated DOCX / PDF
```

One site, not one site per engagement. Per-engagement sites would multiply the app-grant
administration by the number of clients and make cross-engagement analytics
(`RecurringCWEs`, `TopVulnerableAssets`) impossible without fan-out queries.

## 5.2 Site and list identifiers

Resolve once at startup, cache for the process lifetime:

```http
GET /v1.0/sites/{tenant}.sharepoint.com:/sites/VAPT
→ id: "{tenant}.sharepoint.com,8f1c…,4a2b…"      ← SHAREPOINT_SITE_ID

GET /v1.0/sites/{siteId}/lists?$select=id,displayName
→ map displayName → list GUID
```

Resolve lists **by display name at boot**, not by hard-coded GUID — GUIDs differ between the
sandbox and the corporate tenant, and hard-coding them makes the §5.5 provisioning script
non-portable. Fail fast and loudly at startup if a required list is missing.

## 5.3 Internal names vs display names — a guaranteed bug

SharePoint stores a column's *internal* name separately from its display name, and the
internal name is what Graph's `fields` object uses. Two traps:

- A display name with a space becomes `Foo_x0020_Bar` internally. **Create every column with
  a space-free name.** All names in §6 comply.
- `Title` is a built-in required column. Reusing it is fine and intended for
  `Finding.Title`; creating a *second* column called `Title` silently produces `Title0`.

Verify after provisioning:

```http
GET /v1.0/sites/{siteId}/lists/{listId}/columns?$select=name,displayName
```

Assert that every name the Go code writes appears in the response. Do this in a startup
self-check — it converts a class of silent data-loss bugs into a boot failure.

## 5.4 Indexed columns

| List | Indexed columns | Why |
|---|---|---|
| Findings | `EngagementId`, `Status`, `Severity` | Every query filters on `EngagementId`. Without the index, the 5,000-item view threshold returns errors once findings accumulate across engagements. |
| Nodes | `EngagementId` | Same. |
| Comments | `FindingId` | Comment lookup is per-finding. |
| Activity | `EngagementId`, `Created` | The activity feed is a time-ordered tail. |

Indexes must be added **while the list is under 5,000 items** — SharePoint refuses to index
a list that has already crossed the threshold, which is a genuinely painful corner to get
stuck in. Add them at provisioning time, not when they start hurting.

## 5.5 Provisioning

Lists and columns are created **once, by an administrator**, using the PowerShell script in
`M365-TRIAL-SETUP.md` §7 running under an admin identity — not by the application. This
keeps `Sites.Selected`/`write` sufficient for the runtime identity and keeps schema changes
reviewable.

The script is the recovery path. If a column is added by hand in the portal and not added to
the script in the same commit, it does not exist as far as the next tenant is concerned.

---

# 6. Data Model Mapping

## 6.1 `Engagements` list ← `Engagement`

| Go field (`models.go`) | Column | Type | Notes |
|---|---|---|---|
| `ID` | `EngagementId` | Text, indexed | Our UUID is the key. SharePoint's integer `Id` is never used as a foreign key. |
| `OrgID` | `OrgId` | Text | |
| `Name` | `Title` | Text | Built-in column. |
| `ClientName` | `ClientName` | Text | |
| `Status` | `Status` | Choice | `planning`/`in_progress`/`review`/`closed` |
| `Methodology` | `Methodology` | Text | |
| `Scope` | `ScopeJson` | Multi-line | `EngagementScope` (included/excluded/RoE) JSON-encoded. |
| `Timeline` | `StartDate`, `EndDate`, `MilestonesJson` | DateTime ×2, Multi-line | Dates promoted to real columns so the SharePoint UI can sort and filter them. |
| `Team` | `TeamJson` | Multi-line | `[]string` of user IDs. |
| `CreatedBy` | `CreatedByUser` | Text | Our user ID, not the SharePoint author. |
| `CreatedAt` / `UpdatedAt` | built-in `Created` / `Modified` | DateTime | Use SharePoint's. Do not duplicate. |

## 6.2 `Findings` list ← `Finding`

| Go field | Column | Type | Notes |
|---|---|---|---|
| `ID` | `FindingId` | Text, indexed | |
| `EngagementID` | `EngagementId` | Text, **indexed** | Text, not a Lookup column — lookups complicate delta queries and add a join per read. |
| `NodeID` | `NodeId` | Text | |
| `Title` | `Title` | Text | |
| `CVE` | `Cve` | Text | |
| `CWEs`, `MitreAttackIDs` | `CwesJson`, `MitreIdsJson` | Multi-line | JSON arrays. SharePoint multi-value columns are painful through Graph and gain nothing. |
| `Severity` | `Severity` | Choice, indexed | `critical`/`high`/`medium`/`low`/`info` |
| `CVSSVector` | `CvssVector` | Text | |
| `CVSSScore` | `CvssScore` | Number | |
| `Status` | `Status` | Choice, indexed | `draft`/`open`/`in_progress`/`remediated`/`closed` |
| `Description`, `Remediation`, `Impact` | same names | Multi-line, plain text | **63,999 char cap** — see §7. |
| `POC` | `PocRef` | Multi-line | **Refactored.** Holds text plus `evidence:{id}` references, never base64. See §7. |
| `Likelihood` | `Likelihood` | Text | |
| `AssignedTo` | `AssignedTo` | Text (Person column in phase 2) | |
| `CreatedBy` | `CreatedByUser` | Text | |
| — | `ModifiedByUser` | Text | **New.** Written by the backend from the session user, because app-only auth makes SharePoint's `Modified By` always read `mCollaborator-Backend`. |
| `Version` | `ItemVersion` | Number | Our optimistic-concurrency counter, mirrored from `Finding.Version`. |
| `CreatedAt`/`UpdatedAt` | `Created`/`Modified` | DateTime | Built-in. |
| — | `@odata.etag` | (implicit) | Graph-supplied. Cached in memory, never a column. |

Note there are now **two** concurrency mechanisms: our `ItemVersion` (which the SPA sends
back and `UpdateFindingWithVersion` already understands) and the Graph ETag. Both are used —
`ItemVersion` catches conflicts between two mCollaborator users, the ETag additionally
catches edits made directly in the SharePoint UI. §9.3 covers the interaction.

## 6.3 `Nodes` list ← `Node`

| Go field | Column | Type |
|---|---|---|
| `ID` | `NodeId` | Text |
| `EngagementID` | `EngagementId` | Text, indexed |
| `Target` | `Title` | Text |
| `Type` | `NodeType` | Choice |
| `Metadata` | `Metadata` | Multi-line |

## 6.4 `Comments` list ← `Comment`

| Go field | Column | Type |
|---|---|---|
| `ID` | `CommentId` | Text |
| `FindingID` | `FindingId` | Text, indexed |
| `UserID` / `UserName` | `UserId` / `Title` | Text |
| `Body` | `Body` | Multi-line |

## 6.5 `Activity` list ← `ActivityItem`

| Go field | Column | Type |
|---|---|---|
| `ID` | `ActivityId` | Text |
| `OrgID` | `OrgId` | Text |
| `UserID` / `UserName` | `UserId` / `UserName` | Text |
| `Action` | `Title` | Text |
| `Detail` | `Detail` | Multi-line |
| `EngagementID` | `EngagementId` | Text, indexed |

Activity is append-only and the highest-volume list. It is a strong candidate for **not**
living in SharePoint at all — see the write-volume note in §15.

## 6.6 `Evidence` library ← `Evidence`

Evidence is a **document library**, so the row *is* the file. `Evidence` struct fields become
library columns on the driveItem's `listItem` facet:

| Go field | Mapping |
|---|---|
| `ID` | `EvidenceId` column |
| `EngagementID` / `FindingID` | `EngagementId` / `FindingId` columns, indexed |
| `Filename` | the file name |
| `StorageKey` | replaced by the Graph `driveItem.id` — store that instead |
| `MimeType`, `SizeBytes` | Graph supplies both on `driveItem`; do not duplicate as columns |
| `HashSHA256` | `driveItem.file.hashes` — see the bug note below |
| `Tags`, `Description` | `Tags` / `Description` columns |
| `UploadedBy` | `UploadedBy` column |

> **Existing bug this migration must fix.** `HandleCreateEvidence` in `handlers.go` opens the
> uploaded file and **never writes the bytes anywhere**. `StorageKey` is a fabricated
> `uploads/{uuid}_{filename}` string pointing at nothing, and `HashSHA256` is
> `fmt.Sprintf("%x", make([]byte, 32))` — sixty-four zeros for every record. Evidence upload
> is currently metadata-only. The SharePoint `Evidence` library will be the **first real
> storage** this feature has ever had, so implement a genuine SHA-256 over the stream during
> upload rather than porting the stub.

## 6.7 What does not move to SharePoint

| Entity | Stays where | Reason |
|---|---|---|
| `Session` | Process memory | High churn, seconds-to-hours lifetime, contains bearer tokens. Putting credentials in a document platform is indefensible. |
| `User`, `PasswordHash` | Local store | Replaced wholesale by Entra sign-in in phase 2 (§4.5); migrating it to SharePoint first is throwaway work. |
| `AuditLog` | Local store + SharePoint's own unified audit log | SharePoint already audits every item change tenant-side. Duplicating our `AuditLog` list adds write volume for a weaker copy. Keep the local log; cite the tenant audit log for compliance. |
| `Org` | Local store | One row. |
| `ReportRecord` | Derived from the `Reports` library | The library listing *is* the record (§8). |

---

# 7. The PoC / Evidence Refactor (prerequisite)

**This is a hard blocker, not a nice-to-have.**

`insertPocImage()` in `static/js/pages.js` reads a selected image with
`FileReader.readAsDataURL()` and splices a base64 `<img src="data:image/png;base64,…">` tag
directly into the `finding-poc` textarea. The client caps the file at 5 MB; base64 inflates
that by ~33 %, so a single screenshot produces roughly **6.8 million characters** in
`Finding.POC`.

A SharePoint multi-line text column holds **63,999 characters**. One screenshot exceeds the
column limit by about **106×**. The write fails; there is no partial-success path.

### Required change

```
Analyst attaches image
   → POST /api/v1/evidence/upload           (multipart, existing endpoint)
   → backend streams to Evidence library, computes SHA-256, returns evidence ID
   → editor inserts the marker:  ![poc](evidence:{evidenceId})
   → Finding.PocRef stores the marker text only  (tens of bytes)

Render (SPA)      → resolve marker → GET /api/v1/evidence/{id} → signed/proxied URL
Render (report)   → report.go resolves marker → embeds real bytes in the DOCX
```

### Why this is worth doing regardless of storage backend

- Findings JSON currently carries megabytes of base64 on every list response. `ListFindings`
  returns whole objects, so opening an engagement with twenty screenshotted findings ships
  well over a hundred megabytes to the browser.
- Report payloads shrink by the same factor.
- Evidence becomes deduplicable, hashable, and individually access-controlled.
- It is the only path to putting findings in SharePoint at all.

Do this in phase 0, before any Graph code is written. It is independently valuable and it
de-risks everything after it.

---

# 8. Reports Library

`report.go` writes generated documents to `filepath.Join("backend", "reports")` on local
disk and serves them from `/api/v1/reports/download/{type}/{name}`. Local disk means reports
vanish with the container and are invisible to anyone without an mCollaborator account.

### Target flow

```
POST /api/v1/reports/{id}/export
  → report.go generates DOCX (unchanged)
  → docx2pdf.go produces PDF via LibreOffice (unchanged)
  → NEW: upload both to the Reports library
        ≤ 4 MB : PUT  /sites/{s}/drives/{d}/items/root:/{engagement}/{name}.docx:/content
        > 4 MB : POST .../createUploadSession → chunked PUT (10 MiB chunks, multiple of 320 KiB)
  → response returns the SharePoint webUrl alongside the existing download path
```

Keep the local file as a cache for the immediate download, but treat the library copy as
authoritative. Managers and QA then read reports in SharePoint with no application account,
and the files inherit tenant retention and versioning.

> **Security issue to fix in the same change.** In `main.go`, `POST /reports/export` and
> `GET /reports/download/{type}/{name}` are registered **outside** the
> `r.Group(func(r chi.Router){ r.Use(authMiddleware(store)) })` block — both are reachable
> **unauthenticated**, and `HandleDownloadReport` serves by filename. Publishing client VAPT
> reports to SharePoint while an unauthenticated download route exists in front of them
> would be a serious regression. Move both routes inside the authenticated group and apply
> `requirePermission("report:export", …)` before this work ships.

---

# 9. Sync Engine

## 9.1 Write-through cache

```
Read   →  serve from in-memory cache                        (µs, handlers unchanged)
Write  →  PATCH/POST Graph  →  on 2xx update cache  →  publish SSE event
Miss   →  fetch from Graph, populate cache, serve
Drift  →  delta poll every N seconds reconciles anything missed
Boot   →  full page-through of each list, then persist deltaLink
```

The cache is not an optimisation, it is a requirement: Graph round-trips are 100–400 ms and
`handlers.go` calls store methods inside request handling. Serving reads from Graph directly
would make every page load feel broken.

Cache invalidation is driven by the delta feed, not by TTL. A TTL-based cache would either
be stale or hammer Graph.

## 9.2 Delta queries

```http
GET /v1.0/sites/{siteId}/lists/{listId}/items/delta?token=latest      ← first call, boot
GET /v1.0/sites/{siteId}/lists/{listId}/items/delta?token={deltaToken} ← subsequent
→ 200 { "value": [ …changed items… ], "@odata.deltaLink": "…?token=…" }
```

Persist the deltaLink (a small local file or the local DB) so a restart resumes from the
last known position instead of re-reading every list. Losing the token is recoverable — the
next call re-reads the list, which is slow but correct.

Delta replaces the current `?since=<RFC3339>` mechanism in
`HandleFindingsChanges`/`ListFindingsChanges`, which compares string timestamps and cannot
see deletions. Delta reports deletions explicitly.

## 9.3 Optimistic concurrency

The logic already exists and is unused. `UpdateFindingWithVersion` in `store.go:467`
implements correct version checking, but **no handler calls it** — `HandleUpdateFinding` at
`handlers.go:318` mutates the struct in place and calls `UpdateFinding`, which increments
`Version` unconditionally. Every concurrent edit today is last-write-wins, silently.

Target behaviour, layered:

```go
// Layer 1 — mCollaborator vs mCollaborator (works today, no SharePoint needed)
err := store.UpdateFindingWithVersion(id, mutate, input.Version)   // → 409 on mismatch

// Layer 2 — additionally catch edits made in the SharePoint UI
req.Header.Set("If-Match", cached.ETag)
// 412 Precondition Failed → re-fetch, then either field-merge or surface a conflict banner
```

On 412 the backend re-fetches the item, refreshes the cache, and returns 409 to the SPA with
the current server state so the UI can show *"Sarah changed Severity while you were
editing"* rather than silently discarding one analyst's work.

Wire layer 1 now, independently of SharePoint. It is close to free, it is what actually makes
two analysts on one finding safe, and it is required under every storage option.

## 9.4 Failure behaviour

| Graph state | Backend behaviour |
|---|---|
| Healthy | Normal write-through. |
| 429 throttled | Honour `Retry-After`, queue the write, return 202-style "saved, syncing" to the SPA. Never drop the write. |
| 5xx / timeout | Exponential backoff with jitter, cap 5 attempts, then park in a retry queue and raise an alert. |
| Sustained outage | **Read-only mode.** Serve reads from cache, reject writes with a clear banner. Do not accept writes you cannot durably store — that is how data is lost. |
| Token expired / 401 | Refresh once via MSAL; if it still fails, this is a credential expiry (§4.7) — alert loudly, it will not self-heal. |

---

# 10. Real-Time Layer

Latency budget end to end: **2–10 seconds**. Adequate for *"Sarah just added a finding"*;
not adequate for co-editing (§1 Non-Goals). Say this to stakeholders before building it, not
after the first demo.

## 10.1 SharePoint → backend

```http
POST /v1.0/subscriptions
{
  "changeType": "updated",
  "notificationUrl": "https://mcollab.example.com/api/v1/graph/notifications",
  "resource": "sites/{siteId}/lists/{listId}",
  "expirationDateTime": "2026-08-25T00:00:00Z",
  "clientState": "{random secret, echoed back and verified}"
}
```

Three non-optional details:

1. **Validation handshake.** On subscription creation Graph immediately `POST`s to
   `notificationUrl` with a `validationToken` query parameter. The endpoint must echo it back
   as `text/plain` within **10 seconds** or the subscription is never created. This endpoint
   must therefore be reachable and fast *before* the app has any subscription.
2. **Renewal is mandatory.** Maximum lifetime for SharePoint list subscriptions is **30
   days**. Without a renewal job, collaboration silently stops working a month after launch,
   in a way that produces no errors. Renew at 50 % of remaining lifetime and alert on failure.
3. **Notifications carry no payload.** They mean "something in this list changed". Each one
   triggers a delta query (§9.2). Debounce — a bulk import fires a burst.

## 10.2 Backend → browser

Replace the 15-second poll in `startFindingsPolling()` (`pages.js:1223`, currently also
toast-spamming on every change) with SSE:

```
GET /api/v1/stream            (authenticated, per-user)
event: finding.created   data: {"id":"…","engagement_id":"…"}
event: finding.updated   data: {"id":"…","engagement_id":"…","version":7}
event: finding.deleted   data: {"id":"…"}
event: presence          data: {"users":[…]}
: heartbeat every 25s     ← keeps proxies from closing the connection
```

Events carry IDs, not full objects — the SPA refetches what it needs. This keeps the event
stream small and avoids leaking findings to a user whose engagement filter excludes them.
Scope each connection to the engagements the user can see, server-side.

## 10.3 Webhooks are optional — start without them

Webhooks require a **publicly reachable HTTPS endpoint**, which is a deployment constraint,
a new inbound attack surface, and impossible on a laptop without a tunnel (dev tunnels /
ngrok).

Additionally, **whether `Sites.Selected` alone is sufficient to create list subscriptions
must be verified in the sandbox** — Graph's change-notification permission table has
historically been inconsistent for SharePoint list resources, and a design that assumes it
works may hit a 403 at integration time. Treat this as a spike, not an assumption.

Mitigation: build the delta poll first at a 5–10 second interval. That alone lands within the
2–10 s latency budget with **no inbound network exposure at all**, and it works identically
on a laptop and in production. Add webhooks later purely to reduce polling volume.

---

# 11. Backend Refactor Plan

## 11.1 Extract the `Store` interface

`Store` is a concrete struct with 48 public methods. Critically, **nothing outside `store.go`
touches its fields** — `handlers.go` and `report.go` only call methods. Extraction is
therefore mechanical:

```go
type Store interface {
    // engagements
    ListEngagements(orgID, status string) []*Engagement
    GetEngagement(id string) (*Engagement, error)
    CreateEngagement(e *Engagement) error
    // findings
    ListFindings(engagementID, nodeID, severity, status string) []*Finding
    UpdateFindingWithVersion(id string, fn func(*Finding) error, expectedVersion int) error
    // … 45 more
}

type MemoryStore struct { /* today's implementation, verbatim */ }
type SharePointStore struct {
    cache  *MemoryStore     // write-through cache
    graph  *GraphClient
    sync   *SyncEngine
}
```

Selected by config, so both survive:

```go
switch os.Getenv("STORE_BACKEND") {
case "sharepoint": store = NewSharePointStore(cfg)
default:           store = NewMemoryStore()      // CI, local dev, feature work
}
```

### Two signature changes required

The current methods `CreateEngagement`, `UpdateFinding`, `CreateEvidence`, `AddActivity` and
similar **return nothing**. A remote store can fail. They must return `error`, and every call
site in `handlers.go` must handle it. This is the single largest mechanical change in the
project and should land as its own commit against `MemoryStore` before any Graph code exists.

### Dead analytics methods

Six methods — `FindingsOverTime`, `FindingCountBySeverity`, `FindingStatusCount`,
`TopVulnerableAssets`, `RecurringCWEs`, `ListAllFindingSeverities` — served the Insight
dashboard removed in commit `950d238`. Their routes still exist in `main.go` but no UI calls
them. `SharePointStore` can stub them initially. Better: **delete the routes and the methods**
before the interface is extracted, so six fewer methods need a second implementation.

## 11.2 Package layout

```
backend/
├── main.go            store selection via STORE_BACKEND
├── store.go           Store interface + MemoryStore
├── sharepoint/
│   ├── client.go      auth (MSAL), request plumbing, 429/5xx retry
│   ├── lists.go       list item CRUD, field mapping, $batch
│   ├── drive.go       upload sessions, download, hashing
│   ├── delta.go       delta queries + token persistence
│   ├── subscribe.go   webhook create / renew / validate
│   └── store.go       SharePointStore implementing Store
└── sse/
    └── hub.go         connection registry + fan-out
```

---

# 12. Configuration

| Variable | Required | Purpose |
|---|---|---|
| `STORE_BACKEND` | no (`memory`) | `memory` \| `sharepoint` |
| `SHAREPOINT_TENANT_ID` | when sharepoint | Directory (tenant) ID |
| `SHAREPOINT_CLIENT_ID` | when sharepoint | Application (client) ID |
| `SHAREPOINT_CLIENT_SECRET` | sandbox | Secret **value** |
| `SHAREPOINT_CERT_PATH` | production | Certificate, replaces the secret |
| `SHAREPOINT_SITE_ID` | when sharepoint | `host,siteCollectionId,webId` |
| `SHAREPOINT_SYNC_INTERVAL` | no (`10s`) | Delta poll interval |
| `SHAREPOINT_WEBHOOK_URL` | no | Enables webhooks when set; delta-poll-only when absent |
| `PORT` | no (`9900`) | Existing |

Startup self-check, fail fast on any failure: acquire a token → `GET /sites/{siteId}` →
resolve all seven list/library IDs → verify every column internal name (§5.3). A misconfigured
deployment should refuse to start, not corrupt data at the first write.

---

# 13. Error Handling

| Condition | Graph signal | Handling |
|---|---|---|
| Throttled | `429` + `Retry-After` | Sleep exactly `Retry-After`, then retry. Never fixed-interval retry — it extends the throttle. Bulk operations use `$batch` (20 ops/request). |
| Service busy | `503` + `Retry-After` | Same path as 429. |
| Concurrency conflict | `412 Precondition Failed` | Re-fetch, refresh cache, return `409` + current state to the SPA (§9.3). |
| Site grant missing/wrong | `403` on a valid token | **The most common failure.** Token issues fine, every call 403s. Verify with `GET /sites/{siteId}/permissions`. Reads like a code bug; it is not. |
| Credential expired | `401 invalid_client` | Alert. Will not self-heal. |
| Secret ID used instead of secret value | `401 invalid_client` at boot | Classic. Called out in `M365-TRIAL-SETUP.md` troubleshooting. |
| View threshold exceeded | `400`, "The attempted operation is prohibited…" | A query is missing its `EngagementId` filter or the index (§5.4). |
| Text too long | `400` on write | The §7 refactor was skipped or a field grew past 63,999 chars. Validate length **before** the Graph call and return a clean 422. |

Retry policy: exponential backoff `1s → 2s → 4s → 8s → 16s` with ±25 % jitter, 5 attempts,
then park in the retry queue. `Retry-After` always overrides the computed delay. **Never
retry a `4xx` other than 429** — it will fail identically forever.

---

# 14. Security Considerations

| Area | Position |
|---|---|
| **Token isolation** | Graph tokens exist only in backend memory. The SPA holds an mCollaborator session token, unchanged. No Graph credential is ever serialised into a response. |
| **Least privilege** | `Sites.Selected` + one `write` grant (§4.2). The app is structurally incapable of reaching another site. |
| **Webhook endpoint** | New unauthenticated inbound surface. Mandatory controls: verify the `clientState` secret on every notification, treat the body as untrusted (it only signals "changed" — always re-read from Graph, never trust notification content), rate-limit the endpoint, and respond `202` within 3 s. |
| **Evidence handling** | Real SHA-256 computed during upload (fixing the §6.6 stub). Validate MIME type and extension server-side; the current 500 MB `ParseMultipartForm` limit permits a trivial memory-pressure DoS and should be tightened. |
| **Data residency** | Data lives in the tenant's SharePoint region and inherits retention labels, DLP policies, eDiscovery hold and the unified audit log. This is the entire justification for the integration. |
| **Audit attribution** | Under app-only auth every SharePoint change is attributed to the service principal. `ModifiedByUser` (§6.2) preserves real attribution in-app; full native attribution requires phase 2. **State this limitation to compliance reviewers explicitly — do not let them assume SharePoint's `Modified By` is the analyst.** |
| **Existing gaps to close first** | Three pre-existing issues become materially worse once real client data is persisted and shared: (a) `POST /reports/export` and `GET /reports/download/{type}/{name}` are unauthenticated (§8); (b) `corsMiddleware` sets `Access-Control-Allow-Origin: *` together with `Access-Control-Allow-Credentials: true`; (c) `rateLimitMiddleware` is a no-op passthrough that calls `next.ServeHTTP` and nothing else. Fix all three before go-live. |
| **Sandbox discipline** | The trial tenant in `M365-TRIAL-SETUP.md` is outside Cyberteq governance and expires in 30 days. **Synthetic data only. No real client findings, ever.** That constraint is what makes the sandbox usable without an approval process. |

---

# 15. Limits & Capacity

| Limit | Value | Design response |
|---|---|---|
| List view threshold | 5,000 items per query | Index `EngagementId`; always filter; never fetch a whole list; page with `$top` + `@odata.nextLink`. |
| List hard ceiling | 30 million items | Not a practical concern. |
| Multi-line text | 63,999 characters | Forces the §7 PoC refactor. Validate before write. |
| Simple file upload | 4 MB | Above that, `createUploadSession` with chunks that are multiples of 320 KiB (10 MiB is a good size). |
| Library capacity | 250 GB per file, effectively unbounded total | Fine for PCAPs. |
| Graph throttling | Per-app-per-tenant; treat the exact number as unknowable | Obey `Retry-After`, `$batch` bulk work, and keep the read cache so steady-state traffic is writes only. |
| Subscription lifetime | ≤ 30 days (lists) | Renewal job, alert on failure (§10.1). |
| Per-call latency | 100–400 ms | Never call Graph inside a loop over findings. Batch or cache. |

### Write-volume warning

`AddActivity` is called on essentially every mutation — finding create/update, evidence
upload, engagement change. At 20 findings per engagement across a handful of concurrent
engagements, the `Activity` list is the highest-write object in the system and consumes
throttling budget that findings need.

Recommendation: keep `Activity` **local** (or in the phase-0 SQLite store) and mirror only
engagement-level milestones to SharePoint. Nothing about the governance argument requires the
activity feed to be in the tenant.

---

# 16. Build Phases

### Phase 0 — Prerequisites (no SharePoint code)

| Task | Owner | Estimate |
|---|---|---|
| Move PoC images out of the text field to the evidence store (§7) | dev | 1–2 days |
| Implement real evidence persistence + SHA-256 (§6.6) | dev | ~half day |
| Wire `UpdateFindingWithVersion` into `PUT /findings/{id}`; SPA sends version; 409 + conflict banner (§9.3) | dev | ~half day |
| Fix unauthenticated report routes, CORS wildcard+credentials, no-op rate limiter (§14) | dev | ~half day |
| Delete the six dead analytics methods and their routes (§11.1) | dev | 1 hour |
| Extract the `Store` interface; add `error` returns; keep `MemoryStore` (§11.1) | dev | ~1 day |

**Every item here is valuable whether or not SharePoint is ever adopted.** None of it is
wasted by a later decision to use Postgres or Dataverse instead.

### Phase 1 — Tenant readiness

| Task | Owner | Estimate |
|---|---|---|
| Trial tenant, app registration, `Sites.Selected`, site grant | platform admin | ~1 hour (runbook: `M365-TRIAL-SETUP.md`) |
| Provision lists, libraries, columns, indexes via script (§5.5) | admin | ~1 hour |
| Verify token → `GET /sites/{siteId}` returns 200 | admin | 5 min |
| **Spike:** confirm `Sites.Selected` permits list subscriptions (§10.3) | dev | ~2 hours |

### Phase 2 — Graph client

| Task | Estimate |
|---|---|
| MSAL auth, retry/backoff, `$batch`, structured errors | 1–2 days |
| List item CRUD + field mapping + startup column self-check | 2–3 days |
| Drive: upload sessions, download, hashing | 1–2 days |
| `SharePointStore` implementing `Store`, write-through cache | 2 days |

### Phase 3 — Sync & real-time

| Task | Estimate |
|---|---|
| Delta queries + deltaLink persistence | 1–2 days |
| SSE hub + SPA migration off the 15 s poll | 1–2 days |
| ETag / 412 conflict path end to end | ~1 day |
| Webhooks + renewal job (**only if the phase-1 spike passes**) | 1–2 days |

### Phase 4 — Hardening

| Task | Estimate |
|---|---|
| Read-only degradation mode, retry queue | ~1 day |
| Metrics, alerting, operational runbook (§18) | ~1 day |
| Load test at realistic finding volume | ~1 day |

**Total: roughly 3–4 weeks of focused work**, of which phase 0 (~4 days) is unconditionally
useful and phases 2–3 (~2.5 weeks) are the SharePoint-specific bet.

---

# 17. Testing Strategy

| Layer | Approach |
|---|---|
| Unit | Field mapping (Go struct ↔ Graph `fields` JSON) round-trips, marker parsing for `evidence:{id}`, backoff/jitter calculation. No network. |
| Contract | Recorded Graph responses (golden JSON) replayed against a stub HTTP server. Covers 429, 412, 403, and the view-threshold 400 without needing a tenant. |
| Integration | Real sandbox tenant, marked `//go:build integration` so CI never depends on network or a live token. |
| Concurrency | Two clients editing one finding: assert 409 from the version check and 412 from the ETag path; assert no silent overwrite. |
| Volume | Seed 6,000 findings across engagements; assert every query still succeeds — this is the test that catches a missing index before production does. |
| Failure injection | Graph returning 429 / 503 / timeout; assert writes queue rather than drop, and that the read-only banner appears on sustained failure. |

**The default test suite must keep running against `MemoryStore`.** Routing feature tests
through Graph makes them slow (100–400 ms per call), flaky, and dependent on a tenant that
expires. Integration tests verify *the integration*; feature tests verify *the features*.
These are different goals and conflating them costs a working CI pipeline.

---

# 18. Operations

### Metrics

```
graph_requests_total{operation,status}
graph_request_duration_seconds{operation}     p50 / p95 / p99
graph_throttle_events_total
sync_delta_lag_seconds                        wall-clock since last successful delta
sync_queue_depth                              parked writes awaiting retry
subscription_expiry_seconds                   min across active subscriptions
sse_connections_active
```

### Alerts

| Alert | Condition | Severity |
|---|---|---|
| Subscription expiring | `subscription_expiry_seconds < 172800` (2 days) | **critical** — silent collaboration failure |
| Sync stalled | `sync_delta_lag_seconds > 300` | critical |
| Write queue growing | `sync_queue_depth > 50` for 5 min | warning |
| Sustained throttling | `graph_throttle_events_total` rate > 10/min | warning |
| Auth failure | any `401` after a successful start | **critical** — credential expiry |

### Runbook entries

- **"Nobody sees each other's changes"** → check `subscription_expiry_seconds` first; an
  unrenewed subscription is the single most likely cause and produces no errors.
- **"Everything 403s"** → the site grant, not the code. `GET /sites/{siteId}/permissions`.
- **"Everything 401s, worked yesterday"** → client secret expired. Rotate per §4.7.
- **"Saves are slow"** → check throttling metrics; check whether a bulk import is running.

---

# 19. Migration & Rollback

**Migration in.** There is no production data — the current store is seeded in-memory. The
first SharePoint deployment starts from an empty site plus the `seed()` fixtures, which is a
genuine one-time advantage: no data migration, no dual-write period, no backfill.

**Rollback.** `STORE_BACKEND=memory` reverts the application in one restart. Because
`SharePointStore` and `MemoryStore` sit behind the same interface, rollback is a config
change, not a deployment. Data written to SharePoint remains in SharePoint and is readable in
its UI regardless of which backend the app is running.

**Sandbox → corporate tenant.** Per `M365-TRIAL-SETUP.md` §10: the provisioning script
transfers, the app registration and site grant do not (a corporate admin must recreate both),
and the test data must not. The deliverable that makes that conversation short is a working
integration, a `Sites.Selected` request scoped to one site, and measured latency numbers —
not a proposal.

---

# 20. Open Questions

These change the design and should be answered before phase 2 starts.

1. **Is keeping client vulnerability data inside the M365 tenant a hard compliance
   requirement, or a preference?** This is the single deciding factor. If it is a preference,
   Postgres delivers real-time via `LISTEN/NOTIFY`, no 5,000-item threshold, and no sync
   engine — for a fraction of the effort in §16.
2. **Do managers or clients actually want to read findings in SharePoint directly?** This is
   SharePoint's strongest differentiator over every alternative. If nobody will use it, most
   of the cost in §16 buys governance alone.
3. **Should analysts sign in with Entra ID?** If yes, do it *before* the Graph client is
   written (§4.5) — retrofitting delegated auth means rewriting the client's auth layer.
4. **Expected concurrency: 2–3 analysts per engagement, or 20?** Below ~5, phase 0 plus
   SQLite plus SSE is sufficient for a long time and the rest of this document can wait.
5. **Who owns the tenant-side operational burden** — secret rotation, subscription renewal
   alerts, site permission reviews? These are ongoing obligations, not one-time setup.
6. **Does the tenant enforce workload-identity Conditional Access?** (§4.8) Affects
   deployment IP allow-listing.

---

# 21. Appendix — Graph Call Reference

```http
# Resolve site
GET  /v1.0/sites/{host}:/sites/VAPT

# Resolve lists
GET  /v1.0/sites/{siteId}/lists?$select=id,displayName

# Verify column internal names (startup self-check)
GET  /v1.0/sites/{siteId}/lists/{listId}/columns?$select=name,displayName

# Read findings for one engagement
GET  /v1.0/sites/{siteId}/lists/{listId}/items
       ?expand=fields&$filter=fields/EngagementId eq '{id}'&$top=200
     Prefer: HonorNonIndexedQueriesWarningMayFailRandomly   ← never rely on this; index instead

# Create finding
POST /v1.0/sites/{siteId}/lists/{listId}/items
     { "fields": { "Title": "...", "FindingId": "...", "Severity": "high", ... } }

# Update finding with concurrency check
PATCH /v1.0/sites/{siteId}/lists/{listId}/items/{itemId}/fields
      If-Match: {etag}          → 412 Precondition Failed on conflict

# Change feed
GET  /v1.0/sites/{siteId}/lists/{listId}/items/delta?token={deltaToken}

# Evidence upload, small
PUT  /v1.0/sites/{siteId}/drives/{driveId}/items/root:/{path}/{name}:/content

# Evidence upload, large
POST /v1.0/sites/{siteId}/drives/{driveId}/items/root:/{path}/{name}:/createUploadSession
PUT  {uploadUrl}   Content-Range: bytes 0-10485759/{total}

# Subscribe / renew
POST  /v1.0/subscriptions
PATCH /v1.0/subscriptions/{id}     { "expirationDateTime": "..." }

# Verify the site grant (first thing to check on any 403)
GET  /v1.0/sites/{siteId}/permissions
```
