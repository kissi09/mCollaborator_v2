# -*- coding: utf-8 -*-
"""Sections 10-17 of the mCollaborator documentation."""

PART_B = [

# ------------------------------------------- 10. Report generation pipeline
("h2", "Report Generation Pipeline"),
("p", "This is the heart of the application. A `ReportConfig` goes in; a DOCX that is the "
      "Cyberteq template with this engagement's content in it comes out, followed by a PDF "
      "laid out by a real Word engine."),

("h3", "End to end"),
("table", (
    ["Step", "What happens"],
    [
        ["1. Decode", "`POST /reports/export` decodes the wizard's payload into `ReportConfig`"],
        ["2. Normalise areas", "Legacy section names are mapped onto area codes and the selection is put into template order"],
        ["3. Resolve proof", "Each finding's `poc_evidence_ids` are read from the evidence vault into raw image bytes"],
        ["4. Name the file", "`<Reference Number> - <Company Name> - VAPT`, sanitised; regenerating for the same reference replaces the file rather than leaving near-duplicates"],
        ["5. Merge", "`mergeMCollaboratorDocx` rewrites the template package part by part"],
        ["6. Write", "The DOCX is written to `backend/reports/`"],
        ["7. Convert", "Word COM, else LibreOffice, produces the PDF and refreshes the document's fields"],
        ["8. Record", "An audit entry is written when the caller is authenticated"],
        ["9. File", "When the wizard asked for it, both files are uploaded to OneDrive"],
        ["10. Answer", "Download URLs plus every warning worth a person's attention"],
    ]), [1.4, 5.1]),

("h3", "Inside the merge"),
("p", "The template is opened as a zip, every part is read into memory, and each is "
      "handed to whichever renderer owns it. The document body is rebuilt in a fixed "
      "order, each step working on a document the previous step has already settled:"),
("table", (
    ["Order", "Renderer", "What it does"],
    [
        ["1", "`healPlaceholderRuns`", "Word splits `[Company Name]` across runs; this rejoins them so later steps can match literally"],
        ["2", "`fixClientCompanyBookmark`", "Repairs the bookmark the footers' cross-reference points at"],
        ["3", "`renderCoverTable`", "Rules under the version, date, authors and approver rows, full width"],
        ["4", "`renderTableOfContents`", "Rebuilds the contents list to match the sections this report will actually contain"],
        ["5", "`renderScopeTable`", "Section 2.3: one row per selected area with its scope text"],
        ["6", "`renderMethodologySections`", "Keeps only the methodology and test-case blocks for the areas performed"],
        ["7", "`renderNamingConvention`", "The vulnerability id key, limited to the areas in scope"],
        ["8", "`renderAreaSections`", "Rebuilds chapter 3: one section per area, its checklist, and every finding's detail table"],
        ["9", "`renderVulnerabilityRegister`", "The landscape register: id, title, severity, exposure, affected system"],
        ["10", "`renderToolsTable`", "Replaces the template's example tools with the ones actually used"],
        ["11", "`renderAppendix`", "Test-account tables, or removal of the appendix when there are none"],
        ["12", "`applySummaryPlaceholders`", "Totals and severity counts in the executive summary"],
        ["13", "`applyScalarPlaceholders`", "Every remaining `[Company Name]`, `[Date]`, `[Reference Number]` and friend"],
        ["14", "`addCoverLogo`", "Floats the customer's logo over the title page banner"],
    ]), [0.7, 2.3, 3.5]),
("p", "Other parts are handled alongside the body: the two chart parts are rewritten from "
      "the findings, `settings.xml` is told to refresh fields on open, each header gets "
      "the customer logo and its scalar placeholders, and each footer is fitted (below). "
      "New media parts and their relationships are appended last, and the package is "
      "written back out."),

("h3", "Finding numbering"),
("p", "Findings are numbered `REC<n>_<AREA><m>`: a running number across the report, then "
      "the area code and the finding's index within that area - `REC1_IPT1`, `REC7_WPT3`. "
      "The same identifier is used in chapter 3, in the register and on the deck's "
      "scenario slides, so a finding can be followed from one document to the other."),

("h3", "Test-type checklists"),
("p", "Every chapter 3 area section opens with a Test Type / Status checklist. The "
      "template ships those statuses filled in from whatever engagement it was last used "
      "for, and nothing used to rewrite them - so a delivered report asserted results for "
      "one client that were really another's."),
("bullets", [
    "The statuses are now derived from the findings: a group whose subject a finding "
    "names is marked **Issues** across its rows; every other row in a tested area reads "
    "**Pass**. Areas that were not in scope keep no table at all.",
    "Matching is by weighted vocabulary rather than word count - the distinctive terms in "
    "a row (\"SQL\", \"SMTP\", \"Kerberos\") carry the decision, and the scaffolding words "
    "shared by twenty rows do not.",
    "It is tuned for precision. A finding the checklist cannot account for is **reported "
    "back to the wizard by name** rather than quietly marked Pass, because a report whose "
    "register lists a vulnerability its checklist denies is worse than one that asks a "
    "human to look.",
    "The checklist itself is never copied into Go. Both the test names and the group "
    "headings are read from the template's own table at render time, so editing the "
    "checklist in the DOCX needs no code change.",
]),

("h3", "Charts"),
("p", "The template carries two charts, and both would otherwise print the numbers of "
      "whatever engagement the template was made from. They are rewritten from this "
      "report's findings: `chartEx1` by severity, `chartEx2` by assessment area, using the "
      "area's chart label."),

("h3", "The customer logo"),
("table", (
    ["Placement", "Size", "Behaviour"],
    [
        ["Running page header", "0.5 in tall, up to 2.67 in wide",
         "Fills the slot the template reserves. Body sections that reserve no slot are "
         "given one, so the logo does not stop partway through the report"],
        ["Title page banner", "Fitted inside a 2.02 x 1.73 in box, centred",
         "A floating drawing anchored in the banner's own paragraph, printed on top of "
         "the orange artwork in the position the template's guide marks"],
        ["No logo uploaded", "-",
         "The placeholder box is removed rather than left in place; it is an instruction "
         "to whoever fills the template in and has no business printing in a delivered "
         "report"],
    ]), [1.7, 2.1, 2.7]),
("bullets", [
    "PNG, JPEG, GIF, **WEBP** and **SVG** are all accepted, because all of them are what "
    "a client sends when asked for their logo - a bank's press kit is usually SVG. SVG is "
    "rasterised at eight times its printed height so it stays sharp when the PDF is "
    "zoomed.",
    "A logo that will not decode does not vanish silently: the report is still produced, "
    "and the wizard shows a warning saying why the header carries Cyberteq's mark alone.",
    "A near-square logo comes out narrow in the header, which is correct but small beside "
    "Cyberteq's wide mark. The title page's box is far more generous, which is where such "
    "a logo reads properly.",
]),

("h3", "Footer fitting"),
("p", "Each footer is one tabbed line - the Cyberteq entity left, \"VAPT Report - "
      "<client>\" centred, the page number right - with tab stops sized for that section's "
      "page. A long client name makes the centred string reach back past the left item, "
      "and Word cannot move a tab backwards, so it gives up on the stop and butts the two "
      "strings together. `fitFooterSpacing` measures each line against its own stops and "
      "steps the type down, to a floor of 8 pt, only where a line would collide. Lines "
      "that already fit - the whole landscape footer, for one - keep the template's size."),

("h3", "DOCX to PDF"),
("bullets", [
    "**Microsoft Word via COM** is preferred on Windows. It is the highest fidelity and "
    "the only engine that also rebuilds the table of contents and the footer's "
    "cross-reference, saving the refreshed result back into the DOCX the user downloads.",
    "**LibreOffice** (`soffice`) is the cross-platform fallback, run headless against a "
    "throwaway profile so it does not clash with a desktop session.",
    "**There is no third option.** When neither engine is present the export returns the "
    "DOCX alone and says why there is no PDF.",
]),

("h3", "What the wizard gets back"),
("table", (
    ["Field", "Meaning"],
    [
        ["`docx_url`, `pdf_url`", "Download links for the generated documents"],
        ["`pdf_error`", "Why no PDF was produced; the DOCX is still valid"],
        ["`unmatched_findings`", "Findings no checklist row could account for"],
        ["`logo_error`", "Why the uploaded logo is not in the document"],
        ["`od_status`, `od_docx_link`, `od_pdf_link`, `od_folder`, `od_error`",
         "The OneDrive outcome: `ok`, `failed` or `not_configured`"],
    ]), [2.4, 4.1]),

("pagebreak",),

# ------------------------------------------------- 11. Closing-meeting deck
("h2", "Closing-Meeting Deck"),
("p", "The closing meeting is presented from a deck that used to be rebuilt by hand from "
      "the report every time. `POST /reports/closure` takes the same payload the report "
      "export takes and renders it, so the two documents cannot disagree about what was "
      "found."),

("h3", "The template"),
("p", "`tools/mkclosuretemplate` derived the template from a real closing deck, keeping "
      "six slides and removing every trace of the client. Part names were deliberately not "
      "renumbered: PowerPoint addresses slides through relationships, not file names."),
("table", (
    ["Slide", "Role", "Filled or repeated"],
    [
        ["1", "Title", "Filled in place, plus the customer logo"],
        ["2", "Executive summary - scope by assessment area", "Filled in place"],
        ["3", "Executive summary continued - headline findings and the severity legend", "Repeated, one per group of area panels"],
        ["4", "Issues table", "Repeated, four findings per slide"],
        ["23", "Vulnerability scenario", "Repeated, one per proof-of-concept screenshot"],
        ["49", "Closing", "Filled in place"],
    ]), [0.8, 3.2, 2.5]),

("h3", "Why cloning a slide is not like repeating a row"),
("p", "A slide is its own package part, and four other things have to agree that it "
      "exists: the presentation's slide list, the presentation's relationships, the "
      "slide's own relationships and the content-type table. Every clone updates all four, "
      "and the template slides are dropped once their clones have been taken."),

("h3", "Traps this renderer had to learn"),
("table", (
    ["Trap", "Symptom", "Handling"],
    [
        ["Charts carry the template's cached numbers",
         "A confident, entirely fictional donut and bar chart",
         "Both chart parts are rewritten from the findings"],
        ["Cloned slides inherit the speaker-notes relationship",
         "PowerPoint refuses to open the file - hard corruption, not a rendering glitch",
         "The notes relationship is stripped from every clone"],
        ["Table cells inherit hyperlinks",
         "A client's scope printed underlined and linked to the previous engagement's target",
         "Hyperlinks are removed from filled cells"],
        ["`[Date]` lives in a field with no run",
         "PowerPoint relabels the deck with the meeting date",
         "It is written as a plain run instead"],
        ["Placeholders filled in sequence",
         "Real data printed under headings for areas that were never tested",
         "One panel per selected area; unfilled panels blanked"],
    ]), [1.7, 2.4, 2.4]),

("h3", "What ends up in the deck"),
("bullets", [
    "**Every finding** appears in the issues tables, grouped by area and most severe "
    "first.",
    "**A finding gets a scenario slide only where a screenshot is attached to it**, and a "
    "finding with two screenshots gets two slides under the same heading. Findings without "
    "proof are named back to the user - a closing meeting walks through the scenario "
    "slides, and a finding without one is a finding nobody will see demonstrated.",
    "**Pass and Issues are coloured as text**, green `28A745` and red `C00000`, with no "
    "cell fill.",
    "**One unreadable screenshot never costs the deck.** It is reported alongside the "
    "findings without proof, and the rest of the deck is still produced.",
]),
("p", "Because three of these failures are invisible in the XML, the standing rule is to "
      "open a generated deck in PowerPoint and look at every slide before calling it "
      "done."),

("pagebreak",),

# ------------------------------------------------------- 12. OneDrive sync
("h2", "OneDrive Sync"),
("p", "The wizard's last step can push a finished report straight into the Cyberteq "
      "OneDrive for Business, so it lands where it is filed rather than in the server's "
      "reports directory."),

("h3", "How it authenticates"),
("bullets", [
    "OAuth2 **client credentials** against a Microsoft Entra ID app registration. There "
    "is no signed-in user in this flow, so the target drive is addressed explicitly by "
    "UPN - that is what `OD_USER` is for.",
    "The app registration needs the **`Files.ReadWrite.All` application permission** with "
    "admin consent granted.",
    "Tokens are cached in the process until shortly before they expire.",
]),

("h3", "Where files land"),
("p", "When the wizard leaves the folder blank, the destination is the configured root "
      "(`OD_FOLDER`, default `VAPT Reports`) with a subfolder named for the client, so a "
      "drive holding several engagements stays navigable. Folders are created as needed."),

("h3", "Outcomes"),
("table", (
    ["Status", "Meaning"],
    [
        ["`not_configured`", "The server has no `OD_*` credentials; the response names which are missing"],
        ["`ok`", "Both files uploaded - or the DOCX uploaded and the PDF did not, which is reported as a partial success with the reason"],
        ["`failed`", "The DOCX upload itself failed; the message carries the Graph error"],
    ]), [1.5, 5.0]),
("p", "A sync failure never fails the export. The documents are already on disk and their "
      "download links are returned either way."),

("pagebreak",),

# --------------------------------------------------- 13. Desktop application
("h2", "Desktop Application"),
("p", "On Windows the same server ships inside a native window with its own icon, its own "
      "taskbar entry and an installer. The web application inside it is unchanged."),

("h3", "How the shell works"),
("bullets", [
    "**The server is a child process, not a library.** The shell starts the real server "
    "binary on a private loopback port and reverse-proxies the window's requests to it. "
    "The desktop build and the plain server build are therefore the same program, tested "
    "the same way.",
    "**One window per machine.** A single-instance lock raises the existing window instead "
    "of starting a second server against the same SQLite file.",
    "**Its own data paths.** WebView2's cache, the shell log and the server log all live "
    "under `%APPDATA%\\\\mCollaborator\\\\`, so a failed launch leaves a trail even though "
    "there is no console behind a GUI build.",
    "**Start-up failures are shown, not logged.** If the server cannot start, the shell "
    "puts the reason in a dialog.",
]),

("h3", "Opening and saving reports"),
("p", "In a browser, a link to a generated document is enough: the server sends "
      "`Content-Disposition: attachment` and the browser saves the file. Inside WebView2 "
      "that link is a dead end - a `target=\"_blank\"` anchor raises a "
      "`NewWindowRequested` event, Wails registers no handler for it, and the click is "
      "swallowed. No window, no download, and no request reaching the server at all."),
("table", (
    ["Bound method", "What it does"],
    [
        ["`SaveReportAs`", "Fetches the document from the supervised server and opens a native Save dialog filtered to the file type, returning the path written so the app can say where it landed. A cancelled dialog is not a failure"],
        ["`OpenReport`", "Stages the file under `%APPDATA%\\\\mCollaborator\\\\opened\\\\` and hands it to whatever opens it - Word, PowerPoint, the PDF viewer"],
    ]), [1.6, 4.9]),
("p", "The page feature-detects the bound API and falls back to an ordinary download link "
      "when it is absent, so one codebase serves both shells. Anything added to the web "
      "app that opens a window, downloads a file or follows an external link must be "
      "tested in the installed executable, not only in a browser."),

("h3", "Building it"),
("code", """
# from desktop/
.\\build.ps1                 # server + icon + app + installer
.\\build.ps1 -SkipServer     # reuse the staged server binary
.\\build.ps1 -NoInstaller    # skip NSIS
"""),
("table", (
    ["Step", "Result"],
    [
        ["1. Build the backend", "Staged beside the shell as `mCollaborator-server.exe` - the same binary the server release ships"],
        ["2. Regenerate the icon", "The Cyberteq mark, because the default icon was rejected"],
        ["3. Build the Wails app", "`mCollaborator.exe`"],
        ["4. Package", "`mCollaborator-amd64-installer.exe` when NSIS is present"],
    ]), [2.0, 4.5]),
("p", "Everything lands in `desktop/dist/`, and nothing outside `desktop/` is written to. "
      "Those three executables are tracked with **Git LFS**: a clone without `git lfs "
      "install` gets 133-byte pointer stubs instead of programs. Linux and macOS builds "
      "are wanted but not yet produced."),

("pagebreak",),

# ---------------------------------------------------- 14. Testing and quality
("h2", "Testing and Quality"),
("p", "`go test ./...` in `backend/` runs the whole suite in a few seconds. It renders "
      "real documents from the embedded templates and inspects the resulting OOXML, which "
      "is why it can pin things that are otherwise only visible by opening the file."),

("h3", "What the suite pins"),
("table", (
    ["Area", "Tests"],
    [
        ["Package integrity", "Every rendered part is well-formed XML; the landscape section and its page setup survive the chapter 3 rebuild"],
        ["Content", "Scalar placeholders are filled; only selected areas are reported; findings and their vulnerability ids appear; the tools list replaces the template's examples; the appendix is omitted without test accounts"],
        ["Charts", "Both charts match the findings they were built from"],
        ["Checklists", "No template status survives; statuses come from the findings; an unmatched finding is reported; matching stays precise"],
        ["Cover and logo", "The logo reaches all three running headers and the title page; it is anchored, page-positioned and scaled to the banner box; nothing appears when no logo was uploaded"],
        ["Footers", "The client name resolves; no previous client's name survives; items stay spaced apart; the landscape footer keeps its size"],
        ["Closure deck", "Deck structure; it uses the engagement's own data; every screenshot is embedded; issues are grouped by area and severity"],
        ["Desktop shell", "The report fetcher rejects foreign URLs, reads from the supervised server, reports a server error, and file names are made safe"],
        ["Contract", "The wizard payload matches what the renderer expects"],
    ]), [1.7, 4.8]),

("h3", "Working practices"),
("bullets", [
    "**Render and look.** Several classes of defect - a corrupt deck, a chart with the "
    "wrong numbers, a logo on the wrong page - are invisible in the XML. Generating the "
    "document and opening it is part of finishing the change, not an optional extra.",
    "**Scratch harnesses are temporary.** A file like `zenith_scratch_test.go` exists to "
    "reproduce one reported problem against real client data. It hard-codes absolute paths, "
    "is not part of the suite's contract, and is deleted when the question it answers is "
    "settled.",
    "**Client documents never reach the repository.** Generated reports, decks and the "
    "live `data/` directory are all git-ignored, and the deck template was scrubbed of "
    "client material before being committed.",
]),

("pagebreak",),

# ------------------------------------------ 15. Operations and troubleshooting
("h2", "Operations and Troubleshooting"),

("h3", "Where to look"),
("table", (
    ["Log", "Location"],
    [
        ["Server request log", "Standard output of the server process; one line per request"],
        ["Desktop shell log", "`%APPDATA%\\\\mCollaborator\\\\mCollaborator-desktop.log`"],
        ["Supervised server log", "`%APPDATA%\\\\mCollaborator\\\\mCollaborator-server.log`"],
        ["Export diagnostics", "The server log records which PDF engine ran, unmatched findings, logo failures and OneDrive outcomes"],
    ]), [2.1, 4.4]),

("h3", "Common problems"),
("table", (
    ["Symptom", "Cause", "Fix"],
    [
        ["The report downloads as DOCX but there is no PDF",
         "No Word and no LibreOffice on the machine running the server",
         "Install one of them. The DOCX is the template-accurate document and is safe to send"],
        ["The client's logo is missing from the report",
         "The upload could not be decoded",
         "Read the warning the wizard shows; re-export with a PNG or SVG version of the mark"],
        ["PowerPoint refuses to open a generated deck",
         "A cloned slide pointing at a deleted notes part - the failure this renderer strips relationships to avoid",
         "Regenerate; if it recurs, compare the deck's relationships against the template"],
        ["Another analyst cannot reach the server",
         "No inbound firewall rule for TCP 9900",
         "Add the rule from an elevated shell on the server machine"],
        ["Changes from another analyst take a while to appear",
         "The findings poll runs every 15 seconds",
         "Set `window.FINDINGS_REFRESH_MS` lower before the app loads"],
        ["The cloned repository's executables are 133 bytes",
         "Git LFS is not installed, so the files are pointer stubs",
         "`git lfs install` then `git lfs pull`"],
        ["Two analysts' edits to one finding overwrite each other",
         "Writes are last-write-wins today",
         "Coordinate for now; see Future Improvements"],
        ["OneDrive sync says `not_configured`",
         "One or more `OD_*` variables are unset",
         "The response names exactly which; set them and restart the server"],
        ["The server will not start: address in use",
         "Another copy is running, or the port is taken",
         "Stop the other instance, or set `PORT`"],
    ]), [1.9, 2.1, 2.5]),

("pagebreak",),

# ---------------------------------------------------- 16. Future improvements
("h2", "Future Improvements"),
("p", "Ordered roughly by how much they would change the product, rather than by "
      "difficulty."),

("h3", "SharePoint Online as the system of record"),
("p", "A full technical design exists for moving engagements, findings, evidence and "
      "reports into **SharePoint Online**, reached through **Microsoft Graph**, with the "
      "Go backend as the only Graph client. It is a design, not an implementation: nothing "
      "in the shipping application talks to SharePoint today, and the SQLite snapshot "
      "described in this document is the system of record."),
("table", (
    ["Aspect of the design", "Summary"],
    [
        ["Why do it at all", "Engagement data would inherit the tenant's retention, DLP and eDiscovery. If it does not end up governed, the integration has bought nothing that SQLite has not bought more cheaply"],
        ["Access model", "The runtime identity holds `Sites.Selected` and is granted write on exactly one site; it cannot enumerate or touch any other site in the tenant"],
        ["Token handling", "No Graph token ever reaches the browser. CVSS validation, permission checks, audit logging and report generation stay in one enforcement point"],
        ["Data mapping", "Lists for engagements, findings, nodes, comments and activity; a document library for evidence"],
        ["Behaviour under failure", "SharePoint is the source of truth and memory is a write-through cache; every path has a defined behaviour when Graph is slow, throttled or down"],
        ["Explicit non-goal", "Google-Docs-style co-typing in the finding editor. Graph's webhook latency makes it structurally impossible"],
    ]), [2.1, 4.4]),
("p", "The papers are `docs/SHAREPOINT-INTEGRATION.md`, `docs/COLLABORATION-DESIGN.md` "
      "for the options that were weighed, and `docs/M365-TRIAL-SETUP.md` for the tenant "
      "runbook. Any Entra credentials quoted in them should be treated as burned and "
      "rotated before the work is picked up."),

("h3", "Correctness and safety"),
("bullets", [
    "**Optimistic concurrency on findings.** `UpdateFindingWithVersion` already exists in "
    "the store; routing the update handler through it turns two analysts' simultaneous "
    "edits into a visible conflict instead of a silent overwrite.",
    "**Authenticate the export endpoints.** They are open so the browser and the desktop "
    "window can fetch a document without a token. A short-lived signed download URL would "
    "close that without breaking either shell.",
    "**Real rate limiting**, per address, replacing the current placeholder.",
    "**TLS**, or a documented reverse proxy in front of the server, before it is reachable "
    "from anywhere but the office LAN.",
    "**Evidence integrity checks.** The SHA-256 is recorded on upload but never "
    "re-verified; a periodic check would catch a file altered or truncated on disk.",
]),

("h3", "Product"),
("bullets", [
    "**Live updates** over a websocket, replacing the 15-second poll.",
    "**Linux and macOS desktop builds** - the shell is cross-platform, only the Windows "
    "installer exists.",
    "**Retesting support**: mark a finding retested, carry the previous result into the "
    "next report and show what closed since the last engagement.",
    "**Report templates beyond VAPT** - the merge engine is not specific to this template, "
    "but the area definitions and the checklist logic currently are.",
    "**Multi-organisation support.** Everything is scoped to one seeded organisation "
    "today.",
    "**Continuous integration** running `go test ./...` and building both binaries on "
    "every push, so a broken renderer is caught before it reaches a delivery week.",
]),

("pagebreak",),

# ------------------------------------------------------------ 17. Glossary
("h2", "Glossary"),
("table", (
    ["Term", "Meaning"],
    [
        ["VAPT", "Vulnerability Assessment and Penetration Testing"],
        ["IPT / EPT / IPTC", "Internal, external and internal-cloud penetration testing"],
        ["WPT / ASA", "Web application penetration testing; API security assessment"],
        ["CFG / ADT / WNA / NAR", "Configuration files review; Active Directory testing; wireless network assessment; network architecture review"],
        ["PoC", "Proof of concept - the demonstration, and the screenshots attached to a finding"],
        ["CVSS", "Common Vulnerability Scoring System: the severity vector and score on a finding"],
        ["CWE", "Common Weakness Enumeration: the class of weakness a finding belongs to"],
        ["OOXML", "Office Open XML - the zip-of-XML format of DOCX and PPTX files"],
        ["EMU", "English Metric Unit, 914,400 per inch: the unit Office uses for drawing positions and sizes"],
        ["Part", "One file inside an OOXML package - a slide, a header, a chart, an image"],
        ["Relationship", "The reference that lets one part point at another; how PowerPoint and Word address slides, headers and images"],
        ["Wails", "The Go framework that wraps the web application in a native window"],
        ["WebView2", "The Microsoft Edge rendering component the Windows desktop window runs on"],
        ["Git LFS", "Large File Storage - how the shipping executables are stored in the repository"],
        ["Microsoft Graph", "The Microsoft 365 API used for OneDrive filing, and proposed for SharePoint"],
        ["Entra ID", "Microsoft's identity service, formerly Azure Active Directory; issues the tokens the OneDrive sync uses"],
    ]), [1.9, 4.6]),
]
