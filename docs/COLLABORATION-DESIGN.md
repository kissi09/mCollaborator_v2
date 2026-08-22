# mCollaborator — Shared Vulnerability Store & Real-Time Collaboration

Design draft. Covers (1) how SharePoint-backed access would work, and (2) alternatives
worth using for testing.

---

## 1. Where we are today

| Concern | Current state |
|---|---|
| Persistence | **None.** `store.go` is `map[string]*Finding` etc. behind a `sync.RWMutex`. Every restart re-runs `seed()` and all analyst work is lost. |
| Multi-user | Single process, single memory space. Two analysts only collaborate if they hit the *same* running binary. |
| "Real-time" | `startFindingsPolling()` polls `GET /engagements/{id}/findings/changes?since=<RFC3339>` every 15 s and raises a toast. |
| Concurrency control | Half-built. `UpdateFinding` **does** increment `Finding.Version`, and `UpdateFindingWithVersion` implements proper conflict detection — but no handler calls it, so every live write still goes through the last-write-wins path. |
| Evidence | Uploaded into process memory / local disk, not shared. |

The gap is not really "real-time" — it is that **there is no shared source of truth at
all**. Any option below has to solve persistence first; live updates are the layer on top.

---

## 2. Option A — SharePoint as the single point of access

### 2.1 Topology

```
   Analyst browser (SPA)
            │  REST + SSE
            ▼
   mCollaborator Go backend  ◄── the only thing holding Graph credentials
            │  Microsoft Graph (HTTPS)
            ▼
   SharePoint Online site  "Cyberteq VAPT"
      ├── List:    Engagements
      ├── List:    Findings          ← the vulnerability system of record
      ├── List:    Nodes (scope/targets)
      ├── List:    Activity
      ├── Library: Evidence          (PoC images, PCAPs, logs)
      └── Library: Reports           (generated DOCX/PDF)
```

The SPA never talks to SharePoint directly. The backend stays the single writer, which
keeps CVSS/severity validation, audit logging, and report generation in one place, and
avoids shipping Graph tokens to the browser.

### 2.2 Authentication

Two viable models:

**(a) App-only / client credentials — recommended to start.**
Register an Entra ID application, grant application permissions `Sites.Selected`, then
scope it to *only* the VAPT site via
`PATCH /sites/{siteId}/permissions`. The backend holds the client secret (or better, a
certificate) and acts as a service account.

- Simple; works for background sync and webhook renewal.
- Downside: SharePoint sees one identity, so item-level "modified by" is the app, not the
  analyst. Mitigate by writing a `ModifiedByUser` column ourselves from the session user.

**(b) Delegated / on-behalf-of.**
Analysts sign in with Entra ID (MSAL), the backend exchanges the token for a Graph token
per user. SharePoint permissions and audit trail then reflect the real person, and you
inherit Entra Conditional Access / MFA for free — which also replaces the current
bcrypt-in-memory login entirely.

- Correct end state, more moving parts (token cache, refresh, consent).

Practical path: build (a) for the sync engine, add (b) for user sign-in later.

### 2.3 Data mapping — `Findings` list

| Go field | SharePoint column | Type | Notes |
|---|---|---|---|
| `ID` | `FindingId` | Single line | Our UUID; indexed. Keep our ID as the key, not SharePoint's integer `Id`. |
| `EngagementID` | `EngagementId` | Single line (indexed) | Lookup column is tempting but hurts delta queries. |
| `Title` | `Title` | Single line | Built-in column. |
| `Severity` | `Severity` | Choice | critical/high/medium/low/info |
| `CVSSScore` | `CvssScore` | Number | |
| `CVSSVector` | `CvssVector` | Single line | |
| `CVE` | `Cve` | Single line | |
| `CWEs`, `MitreAttackIDs` | `Cwes`, `MitreIds` | Multi-line | JSON-encoded; SharePoint multi-value columns are painful via Graph. |
| `Status` | `Status` | Choice | draft/open/in_progress/… |
| `Description`, `POC`, `Remediation`, `Impact` | same names | Multi-line (plain text) | **See size limit below.** |
| `AssignedTo` | `AssignedTo` | Person or Single line | Person column if using delegated auth. |
| `Version` | `ItemVersion` | Number | Our optimistic-concurrency counter. |
| `CreatedAt`/`UpdatedAt` | built-in `Created`/`Modified` | DateTime | Use SharePoint's, don't duplicate. |

**Size limit that will bite us:** a multi-line text column tops out at 63,999 characters,
and the PoC field currently stores **base64 data-URI `<img>` tags** (`insertPocImage()` in
`pages.js`). A single screenshot blows past that instantly.

→ Required change: PoC images must move to the **Evidence document library**, with the PoC
text storing a reference (`![poc](evidence:{evidenceId})`) that the backend resolves when
rendering and when generating reports. This is worth doing regardless of which backend we
choose — it also shrinks the report payloads dramatically.

### 2.4 Reads, writes, and the sync engine

Keep the in-memory store as a **write-through cache**, not as the source of truth:

```
Read  →  serve from memory cache (fast, keeps current handlers unchanged)
Write →  PATCH Graph  →  on success, update cache  →  broadcast to SSE clients
Drift →  delta query every N seconds reconciles anything we missed
```

Change detection uses Graph delta tokens rather than our `?since=` timestamp:

```
GET /sites/{siteId}/lists/{listId}/items/delta
→ returns changed items + @odata.deltaLink
```

Persist the delta token so a backend restart resumes rather than re-reading the list.

**Optimistic concurrency.** Graph supports ETags on list items:

```go
req.Header.Set("If-Match", finding.ETag)   // 412 Precondition Failed = someone else won
```

On 412: re-fetch, and either merge field-wise or surface a conflict banner
("Sarah changed Severity while you were editing"). This is the piece that makes two
analysts on one finding actually safe, and it does not exist today.

### 2.5 Getting it to feel real-time

SharePoint cannot push to a browser. Two hops are needed:

1. **SharePoint → backend:** Graph change notifications (webhooks).
   `POST /subscriptions` with `resource: sites/{siteId}/lists/{listId}`, an HTTPS
   `notificationUrl`, and `expirationDateTime` **≤ 30 days for lists** — a renewal job is
   mandatory or collaboration silently dies. Notifications are "something changed", not
   the payload, so each one triggers a delta query.
   *Webhooks require a publicly reachable HTTPS endpoint* — for local testing use a
   tunnel (dev tunnels / ngrok) or fall back to short-interval polling.

2. **Backend → browser:** replace the 15 s poll with **Server-Sent Events**
   (`GET /api/v1/stream`). SSE is a good fit: one-way, works over plain HTTP, auto-reconnects,
   far less code than WebSockets. Events: `finding.created`, `finding.updated`,
   `finding.deleted`, `presence`.

Realistic end-to-end latency: **2–10 seconds** (Graph webhook delivery is not instant).
That is fine for "Sarah just added a finding". It is **not** enough for Google-Docs-style
co-typing in the finding editor — for that you would need field-level locking or a CRDT,
which is out of scope for a SharePoint-backed design.

### 2.6 Limits to design around

| Limit | Value | Impact |
|---|---|---|
| List view threshold | 5,000 items per query | Findings across many engagements will exceed this. Index `EngagementId` and always filter; never fetch a whole list. |
| Multi-line text | 63,999 chars | Forces PoC images out to the library (§2.3). |
| Graph throttling | ~2,000 req/min/app, HTTP 429 | Need `Retry-After` handling + exponential backoff. Bulk import must batch (`$batch`, 20 ops/request). |
| Webhook lifetime | ≤ 30 days (lists) | Background renewal job required. |
| Attachment/file size | 250 GB library limit, but >4 MB needs upload sessions | Evidence uploads need chunked `createUploadSession`. |
| Latency | 100–400 ms per Graph call | Never call Graph inside a request loop; the cache is not optional. |

### 2.7 Honest assessment

SharePoint's real win is **governance**: the data lives in the tenant, inherits retention,
DLP, eDiscovery and Entra access control, and clients/managers can look at findings in a
familiar UI without an mCollaborator account. For a security consultancy handling client
vulnerability data, that is a genuinely strong argument.

Its real cost is that **it is a document platform being used as a database**: 5,000-item
thresholds, 64 K text columns, throttling, no transactions, no joins, no referential
integrity, and webhook plumbing just to approximate change events. Expect the sync engine
to be the most bug-prone part of the codebase.

---

## 3. Alternatives — especially for testing

Ranked by how quickly they unblock multi-user testing.

### 3.1 SQLite — *recommended next step*
One file, zero infrastructure, `modernc.org/sqlite` needs no cgo. Directly replaces the
maps in `store.go` and instantly fixes "everything vanishes on restart". WAL mode handles
concurrent readers fine.
- **For:** an afternoon of work; keeps every handler signature; real transactions; trivial
  to reset (delete the file); testable in CI.
- **Against:** single-host only. Two analysts must reach one server — acceptable for testing.
- **Real-time:** keep the existing `?since=` poll, or upgrade it to SSE. No external dependency.

### 3.2 PostgreSQL (Supabase / Neon / plain Docker)
The natural production answer if SharePoint governance is not a hard requirement.
- **For:** proper concurrency, `LISTEN/NOTIFY` gives genuine sub-second push straight into
  SSE, row-level security, JSONB for the flexible fields, no 5,000-item nonsense.
- **Against:** infrastructure to host and back up; data leaves the M365 tenant.
- Supabase additionally gives realtime subscriptions and auth out of the box — the fastest
  route to a *truly* live multi-user demo.

### 3.3 Microsoft Lists / Dataverse
Lists is the same storage as §2 with a friendlier UI, so it inherits the same limits.
**Dataverse** is the more honest Microsoft answer: a real relational store, still inside
the tenant, with proper indexing and row counts, plus Power Automate integration.
- **For:** tenant governance without the document-platform constraints.
- **Against:** licensing cost; heavier API; more setup than SharePoint.

### 3.4 OneDrive/SharePoint file-as-database
Storing a `findings.json` in the existing OneDrive folder and syncing it.
- Mentioned only to rule it out: concurrent writes to one file produce OneDrive conflict
  copies (`findings-DESKTOP-ABC.json`) and silent data loss. **Do not use for shared state.**

### 3.5 Git-backed findings
Findings as Markdown/YAML in a repo, PRs for review.
- **For:** perfect audit trail, diffs, offline work, free.
- **Against:** merge conflicts pushed onto analysts; no live view; poor fit for the SPA.
- Genuinely useful as an **export/archive** format rather than the live store.

### 3.6 Comparison

| Option | Setup | Multi-user | True real-time | Tenant governance | Good for testing |
|---|---|---|---|---|---|
| In-memory (today) | none | ✗ | ✗ | ✗ | ✗ |
| **SQLite** | minutes | same host | via SSE poll | ✗ | **best** |
| Postgres/Supabase | hours | ✓ | ✓ (NOTIFY) | ✗ | very good |
| SharePoint + Graph | days | ✓ | ~2–10 s | ✓ | slow to iterate |
| Dataverse | days | ✓ | ~seconds | ✓ | moderate |
| Git-backed | hours | awkward | ✗ | partial | export only |

---

## 3.7 Dataverse: concrete implementation runbook

Dataverse is the strongest of the Microsoft options for this workload — it keeps the data
in the tenant like SharePoint, but it is a real relational store underneath, so most of
§2.6's limits simply do not apply.

### Why it fits better than SharePoint

| Constraint | SharePoint | Dataverse |
|---|---|---|
| Item ceiling per query | 5,000 view threshold | none (SQL-backed, paged) |
| Long text column | 63,999 chars | 1,048,576 chars |
| Binary evidence | separate document library + upload sessions | native **File** (128 MB) and **Image** columns on the row |
| Optimistic concurrency | ETag on list items | first-class `@odata.etag` + `If-Match` → 412 |
| Change feed | list webhooks, ≤30 day renewal | per-table **change tracking** with `@odata.deltaLink`, plus webhooks / Service Bus |
| Relationships | lookup columns, awkward via Graph | real lookups, `$expand` joins |

The File column is the clean answer to the base64-PoC problem in §2.3 — screenshots
attach to the finding row itself, no separate library plumbing.

### Step 1 — Get an environment (you, ~30 min)

The licensing question is the usual blocker, but there is a free path for exactly this:

- **Power Apps Developer Plan** — free, gives developer environments with full Dataverse.
  Sign up at `aka.ms/PowerAppsDevPlan` with the work account. Intended for dev/test, which
  is precisely this use case.
- Caveat: developer environments are not for production and can be reclaimed after a
  long idle period. Fine for testing, not a home for real client data.

Then: Power Platform Admin Center → New environment → type **Developer** → *Create a
database* → note the Environment URL (`https://<org>.crm<N>.dynamics.com`).

### Step 2 — App registration + application user (you, ~20 min)

This is the part people get wrong, so in order:

1. Entra ID → App registrations → New registration (single tenant).
2. Certificates & secrets → New client secret. Record it.
3. Record the **Tenant ID** and **Application (client) ID**.
4. **Power Platform Admin Center → your environment → Settings → Users + permissions →
   Application users → New app user** → pick the Entra app → assign a security role
   (*System Customizer* is enough for testing).

Step 4 is mandatory and easy to miss. Without it the app authenticates fine and then gets
**403 on every request**, which reads like a code bug and is not.

### Step 3 — Schema (either of us)

Create a publisher + solution first so tables get a prefix (`ct_`). Tables mirror
`models.go`:

| Go type | Dataverse table | Notes |
|---|---|---|
| `Engagement` | `ct_engagement` | choice cols for status/methodology |
| `Finding` | `ct_finding` | lookup → engagement; choice severity/status; **File** col for PoC |
| `Node` | `ct_node` | lookup → engagement |
| `Evidence` | `ct_evidence` | **File** column holds the bytes |
| `ActivityItem` | `ct_activity` | |

Enable **change tracking** on `ct_finding` and `ct_engagement` (table settings) or the
delta feed in step 5 will not work.

I can generate this as a solution definition / PAC CLI script; you run it against the env.

### Step 4 — Auth + client (me)

No official Go SDK, but the Web API is plain OData v4, so `net/http` is sufficient.

Client-credentials token:
```
POST https://login.microsoftonline.com/{tenant}/oauth2/v2.0/token
  grant_type=client_credentials
  client_id={appId}&client_secret={secret}
  scope=https://{org}.crm{N}.dynamics.com/.default
```
Then `Authorization: Bearer …` against
`https://{org}.crm{N}.dynamics.com/api/data/v9.2/ct_findings`.

Optimistic concurrency maps 1:1 onto the existing `UpdateFindingWithVersion` semantics:
```
PATCH /api/data/v9.2/ct_findings(<id>)
If-Match: W/"1234567"        → 412 Precondition Failed when someone else won
```

### Step 5 — Change feed → SSE (me)

```
GET /api/data/v9.2/ct_findings?$select=...
Prefer: odata.track-changes
→ body includes "@odata.deltaLink": ".../ct_findings?$deltatoken=..."
```
Poll the deltaLink on a short interval and fan changes out over SSE. **No public HTTPS
endpoint required**, which makes local testing far easier than SharePoint's webhooks.
Register real webhooks later if sub-second latency matters.

### Step 6 — Refactor (me)

`Store` is currently a concrete struct with 48 public methods. Nothing outside `store.go`
touches its fields — handlers only call methods — so extracting a `Store` interface is
mechanical rather than invasive, then `MemoryStore` and `DataverseStore` both satisfy it.

Note that six of those methods (`FindingsOverTime`, `FindingCountBySeverity`,
`FindingStatusCount`, `TopVulnerableAssets`, `RecurringCWEs`,
`ListAllFindingSeverities`) served the Insight dashboard that has since been removed.
Their routes still exist but no UI calls them, so a Dataverse implementation can stub them
initially and add `$apply` aggregations later.

### Effort estimate

| Task | Who | Estimate |
|---|---|---|
| Environment + app registration + app user | you | ~1 hour |
| Extract `Store` interface, keep memory impl | me | ~half a day |
| Schema / solution definition | me + you to apply | ~half a day |
| Dataverse client: auth, CRUD, OData mapping | me | 2–3 days |
| Change tracking → SSE | me | 1–2 days |
| Wire up 409 conflict handling end to end | me | ~half a day |
| Analytics via `$apply` (optional) | me | ~1 day |

### Honest caveat on "testing all the functionality"

Dataverse is the right call if it is the likely **production** target — building against it
now avoids migrating twice. But if the goal is genuinely to *exercise the app's features*,
it works against you: every test run then needs network, a valid token, and 100–300 ms per
API call, which makes the suite slower and flakier. SQLite tests the functionality better;
Dataverse tests the integration. Those are different goals, and it is worth being clear
which one is being bought.

A good compromise: extract the interface, keep `MemoryStore`/SQLite for feature and CI
testing, and add `DataverseStore` behind the same interface for integration testing.

## 4. Suggested phasing

1. **Persistence first — SQLite.** Swap the maps in `store.go`. Unblocks all multi-user
   testing and is throwaway-cheap if we later move to SharePoint.
2. **Move PoC images out of the text field** into the evidence store, with reference
   resolution at render/report time. Needed for *every* backend option, and it fixes the
   report payload bloat today.
3. **Finish optimistic concurrency.** The logic already exists in
   `UpdateFindingWithVersion`; wire `PUT /findings/{id}` to it, have the client send the
   version it read, return 409 on mismatch and show a conflict banner. This is what makes
   two analysts on one finding safe, independent of storage — and it is close to free.
4. **Replace polling with SSE.** Removes the 15 s lag and the toast-spam, and is the same
   browser-facing contract regardless of what sits behind it.
5. **Then decide storage.** If tenant governance is required → build the SharePoint sync
   engine per §2 behind the same `Store` interface. If not → Postgres.

Steps 1–4 are useful under every option, so none of that work is wasted by the step-5
decision.

## 5. Open questions

- Is keeping client vulnerability data inside the M365 tenant a hard compliance
  requirement, or a preference? This is the deciding factor for §2 vs §3.2.
- Should analysts sign in with Entra ID (replacing the current password store)? If yes,
  do it before building the SharePoint layer so the delegated-auth model is available.
- Expected concurrency: 2–3 analysts on an engagement, or 20? Below ~5, SQLite plus SSE is
  sufficient for a long time.
- Do clients need read access to findings directly in SharePoint? That is SharePoint's
  strongest differentiator — if nobody wants it, most of its cost buys nothing.
