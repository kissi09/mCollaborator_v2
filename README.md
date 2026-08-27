<p align="center">
  <img src="docs/logo.png" alt="mCollaborator" width="128">
</p>

<h1 align="center">mCollaborator</h1>

<p align="center">
  Cyberteq's penetration-testing workbench — engagements, findings and evidence
  in one place, and the client report generated from them.
</p>

---

## What it is

Security assessments produce two things: findings, and the documents that
communicate them. mCollaborator keeps both in one system. Analysts record
findings against the assets they tested and attach the evidence as they go; when
the engagement closes, the VAPT report and the closing-meeting deck are built
from that same data rather than retyped into a Word template.

The point is that the report is a **view of the engagement**, not a separate
document that has to be kept in step with it.

## How it works

```
  Engagement                          the client, the scope, the dates,
      |                               the assessment areas in scope
      +-- Nodes                       the assets under test
      |     +-- Findings              severity, CWE, description, remediation
      |           +-- Evidence        screenshots and files, uploaded as PoC
      |           +-- Comments        review conversation, and status changes
      |
      +-- Report  ------------------> DOCX   an embedded Word template with the
                                     |       engagement's values substituted in
                                     +-----> PDF    converted from that DOCX
                                     +-----> PPTX   the closing-meeting deck
                                     +-----> OneDrive (optional)
```

A **Go backend** (chi router, SQLite) holds the domain and serves a small
vanilla-JS single-page app. There is no frontend build step and no Node
toolchain — `main.go` embeds `static/` straight into the binary, along with the
Word and PowerPoint templates the generators fill in. One executable is the
whole application.

Report generation is template substitution rather than document assembly. The
DOCX generator opens the embedded Cyberteq template and replaces its
placeholders — `[Company Name]`, `[Reference Number]`, `[Date]`, the customer
logo slots on the title page and in the running header — then writes the
findings, evidence screenshots and appendices into the sections that expect
them. The layout is the template's, so generated reports match the house style
by construction.

PDF conversion tries Microsoft Word via COM first and falls back to headless
LibreOffice; if neither is present the export fails loudly rather than quietly
producing something that only looks like the real thing.

The closing-meeting deck is built the same way from
`mcollaborator_closure_template.pptx` — scope panels, severity charts and the
executive summary are all derived from the engagement's findings.

## Running it

There are two front ends over the same server, and they ship the same binary.

| | What it is | Where its data lives |
|---|---|---|
| **Web app** | `backend\mCollaborator.exe`, opened in a browser on `http://localhost:9900` | `data\` beside the executable |
| **Desktop app** | `desktop\dist\mCollaborator.exe`, a Wails window with no address bar, which runs the server as a hidden child process | `%APPDATA%\mCollaborator\` |

```powershell
.\backend\mCollaborator.exe          # then open http://localhost:9900
```

Sign in with the bootstrap admin `admin@cyberteq.com`; the seeded password is in
`backend/store.go` and should be changed on first login. Fresh installs start
empty apart from that account — no demo data is injected.

| Variable | Effect |
|---|---|
| `PORT` | Web app listen port (default `9900`) |
| `MCOLLABORATOR_DB_PATH` | SQLite file (default `data/mcollaborator.db`) |
| `OD_TENANT_ID`, `OD_CLIENT_ID`, `OD_CLIENT_SECRET`, `OD_USER`, `OD_FOLDER` | OneDrive sync, if reports should be pushed to the Cyberteq drive on export |

## Building

**Build everything with the script at the repository root:**

```powershell
.\build.ps1
```

This is not optional housekeeping. There are two independent binaries carrying
the same backend code — the web app and the server inside the desktop bundle —
and `desktop\build.ps1` rebuilds only the second. Rebuilding one and not the
other has already shipped a report missing its logo: the code was committed and
correct, but the binary rendering it was three days old, and nothing complains,
because a stale binary runs perfectly. It just renders the previous version.

`build.ps1` builds both and then SHA256-compares them. They are the same package
built with the same flags, so identical hashes are the proof the two halves
agree; a mismatch fails the build.

| Flag | Effect |
|---|---|
| `-WebOnly` | Just `backend\mCollaborator.exe` |
| `-DesktopOnly` | Just the desktop bundle |
| `-NoRestart` | Leave the web server stopped after the swap |
| `-NoInstaller` | Skip the NSIS installer |

Prerequisites are Go, plus the Wails CLI and NSIS for the desktop bundle — see
[`desktop/README.md`](desktop/README.md).

### Cloning: Git LFS is required

The shipping executables are tracked with **Git LFS**. Run `git lfs install`
before cloning, or `backend/*.exe` and `desktop/dist/*.exe` arrive as ~130-byte
pointer stubs instead of programs. GitHub showing them as 133 bytes is correct
LFS behaviour, not a broken upload.

## Layout

| Path | |
|---|---|
| `backend/` | The server: domain, HTTP API, and the DOCX/PDF/PPTX generators |
| `backend/static/` | The single-page app, embedded into the binary at build time |
| `backend/templates/` | The Word and PowerPoint templates reports are built from |
| `desktop/` | The Wails wrapper, its icon generator, and the installer build |
| `docs/` | Generated application documentation and the docgen that produces it |
| `build.ps1` | Builds both binaries and checks they agree |
