// mCollaborator — building a closing deck out of a finished report.
//
// Closure Prep's second entry point. A DOCX or PDF report is uploaded, the
// server reads the engagement and the findings back out of it, and what comes
// back lands here as a draft: not a deck, and not saved anywhere, but the slides
// the deck would be made of, laid out to be read and corrected.
//
// The screen exists because two things cannot be recovered from a finished
// document, and both of them matter:
//
//   - The screenshots. A proof in a report is a picture anchored in a table
//     cell with nothing tying it to the finding beside it, and in a PDF it is
//     not a file at all. Every scenario slide here therefore has an empty
//     picture frame until someone drops the image into it, taken from the
//     report's own evidence section. A finding whose frame stays empty gets no
//     scenario slide, exactly as it would from the wizard.
//   - Whatever the reader got wrong. A report written by hand puts its labels
//     in places the extractor has to work out, and an area it could not settle
//     is left blank rather than guessed at. Everything read is editable, and the
//     deck cannot be built while a finding still has no area, because a finding
//     with no area appears on no slide.
//
// The slides shown are the slides the deck builds: the grouping, the ordering,
// the vulnerability ids and the titles are the same rules as backend/pptx*.go.
// Where the two could drift, this file says which function it mirrors.

// closureDraft is the imported report: {filename, kind, config, unplaced,
// missing, notes}. config is a ReportConfig, ready to post to /reports/closure.
let closureDraft = null;

// closureDeckResult is the last /reports/closure response for this draft, kept
// so the download buttons survive a repaint.
let closureDeckResult = null;

const CLOSURE_EXTS = ['.docx', '.pdf'];
const CLOSURE_SHOT_EXTS = ['.png', '.jpg', '.jpeg', '.gif'];
const CLOSURE_SHOT_MAX_MB = 8;

// ---------------------------------------------------------------------------
// reading the report
// ---------------------------------------------------------------------------

async function handleClosureImportFile(files) {
  const file = files && files[0];
  if (!file) return;

  const name = file.name || '';
  const ext = name.slice(name.lastIndexOf('.')).toLowerCase();
  if (!CLOSURE_EXTS.includes(ext)) {
    paintClosureImportStatus(`<div class="import-error mt-3">Only a .docx or .pdf report can be turned into a
      closing deck. A .doc has to be saved as .docx first, and a scanner export carries none of the
      engagement details the title and summary slides need.</div>`);
    return;
  }
  if (file.size > IMPORT_MAX_MB * 1024 * 1024) {
    paintClosureImportStatus(`<div class="import-error mt-3">That report is ${formatMB(file.size)}.
      The limit is ${IMPORT_MAX_MB} MB.</div>`);
    return;
  }

  paintClosureImportStatus(`
    <div class="import-file mt-3">
      <span style="color:var(--primary);font-size:26px;line-height:1;">&#128196;</span>
      <div style="flex:1;min-width:0;">
        <div style="font-size:13px;font-weight:600;">${sanitizeInput(name)}</div>
        <div class="font-mono text-xs text-muted">${sanitizeInput(formatMB(file.size))}</div>
      </div>
      <span class="font-mono text-xs" style="color:var(--primary);">Reading&hellip;</span>
    </div>
    <div class="import-sweep mt-3"><i></i></div>`);

  const form = new FormData();
  form.append('file', file);
  try {
    const res = await api.upload('/reports/closure/import', form);
    closureDraft = res.data;
    closureDeckResult = null;
    prepareClosureDraft();
    MCOLLABORATOR.navigate('#/closure-preview');
  } catch (e) {
    paintClosureImportStatus(`<div class="import-error mt-3">${sanitizeInput(e.message || 'The report could not be read.')}</div>`);
  }
}

function paintClosureImportStatus(html) {
  const box = document.getElementById('closure-import-status');
  if (box) box.innerHTML = html;
}

// prepareClosureDraft gives every finding a handle of its own. The cards carry
// only this id, so a title full of quotes never has to survive a round trip
// through an inline onclick attribute.
function prepareClosureDraft() {
  const cfg = closureDraft.config = closureDraft.config || {};
  cfg.areas = cfg.areas || [];
  cfg.findings = (cfg.findings || []).map((f, i) => Object.assign({}, f, {
    _id: i + 1,
    poc_uploads: f.poc_uploads || []
  }));
}

function discardClosureDraft() {
  if (!confirm('Discard this report and everything corrected on this screen?')) return;
  closureDraft = null;
  closureDeckResult = null;
  MCOLLABORATOR.navigate('#/closure-prep');
}

// ---------------------------------------------------------------------------
// the deck, as this screen understands it
//
// Mirrors buildNumberedFindings and chunkFindings. If either changes, this has
// to change with it, or the preview stops being a preview.
// ---------------------------------------------------------------------------

const CLOSURE_SEVERITIES = ['critical', 'high', 'medium', 'low'];

function closureSeverityRank(sev) {
  const i = CLOSURE_SEVERITIES.indexOf((sev || '').trim().toLowerCase());
  return i < 0 ? 4 : i;
}

function closureSeverityLabel(sev) {
  switch ((sev || '').trim().toLowerCase()) {
    case 'critical': return 'Critical';
    case 'high': return 'High';
    case 'medium': return 'Medium';
    case 'low': return 'Low';
    default: return 'Informational';
  }
}

// The criticality palette is the report's own — severityHex in docxmcollab.go.
// Do not "improve" one of these in isolation: they are sampled from the
// template's criticality legend so a severity reads the same colour everywhere.
function closureSeverityHex(sev) {
  switch ((sev || '').trim().toLowerCase()) {
    case 'critical': return '#FF0000';
    case 'high': return '#F68831';
    case 'medium': return '#FFC000';
    case 'low': return '#92D050';
    default: return '#00B0F0';
  }
}

function closureAreaCodes() {
  const have = new Set((closureDraft.config.areas || []).map(a => (a.code || '').toUpperCase()));
  return REPORT_AREAS.map(a => a.code).filter(c => have.has(c));
}

function closureInitials() {
  const cfg = closureDraft.config;
  const given = (cfg.company_initials || '').trim();
  if (given) return given;
  return (cfg.company_name || '').split(/\s+/)
    .filter(w => /^[A-Za-z]/.test(w)).map(w => w[0].toUpperCase()).join('') || '—';
}

// closureNumbered is every placed finding in deck order: area by area in
// template order, and within an area by severity, numbered as it goes.
function closureNumbered() {
  const initials = closureInitials();
  const out = [];
  let rec = 0;
  closureAreaCodes().forEach(code => {
    const mine = closureDraft.config.findings
      .filter(f => (f.area || '').toUpperCase() === code)
      .sort((a, b) => closureSeverityRank(a.severity) - closureSeverityRank(b.severity));
    mine.forEach((f, i) => {
      rec++;
      out.push(Object.assign({}, f, {
        _area: code,
        _vulnId: `${initials}_REC${rec}_${code}${i + 1}`
      }));
    });
  });
  return out;
}

// closureIssueSlides groups the findings the way the issues slides are titled:
// one area and one severity to a slide, four findings at most.
function closureIssueSlides(numbered) {
  const order = [];
  const buckets = {};
  numbered.forEach(f => {
    const key = `${f._area}|${closureSeverityLabel(f.severity)}`;
    if (!buckets[key]) { buckets[key] = []; order.push(key); }
    buckets[key].push(f);
  });
  const out = [];
  order.forEach(key => {
    const group = buckets[key];
    for (let i = 0; i < group.length; i += 4) out.push(group.slice(i, i + 4));
  });
  return out;
}

// firstSentence keeps a description to the one line a slide can hold, the way
// the deck does.
function closureFirstSentence(s) {
  const text = (s || '').replace(/\s+/g, ' ').trim();
  if (!text) return '';
  const i = text.indexOf('. ');
  if (i > 0) return text.slice(0, i + 1);
  return text.length > 220 ? text.slice(0, 219) + '…' : text;
}

function closurePeriodLine() {
  const cfg = closureDraft.config;
  const start = (cfg.assessment_start || '').trim();
  const end = (cfg.assessment_end || '').trim();
  if (start && end) return `The project was executed during the period from ${start} to ${end}`;
  if (start) return `The project was executed during the period from ${start}`;
  if (end) return `The project was executed during the period ending ${end}`;
  return '';
}

// ---------------------------------------------------------------------------
// the preview
// ---------------------------------------------------------------------------

function renderClosurePreview() {
  if (!closureDraft) {
    return `
      <div class="card" style="padding:56px 24px;text-align:center;max-width:560px;margin:40px auto;">
        <h3 class="font-display font-bold mb-2">Nothing to preview</h3>
        <p class="text-sm text-muted mb-4">Upload a finished report in Closure Prep first.</p>
        <a class="btn btn-primary" href="#/closure-prep" style="text-decoration:none;">Back to Closure Prep</a>
      </div>`;
  }
  return `
    <div class="closure-preview" style="margin:-24px;">
      <div id="closure-preview-head"></div>
      <div class="closure-slides" id="closure-slides"></div>
    </div>`;
}

function afterRenderClosurePreview() {
  if (!closureDraft) return;
  paintClosurePreview();
}

function paintClosurePreview() {
  const head = document.getElementById('closure-preview-head');
  const body = document.getElementById('closure-slides');
  if (!body) return;

  const cfg = closureDraft.config;
  const unplaced = cfg.findings.filter(f => !(f.area || '').trim());
  const numbered = closureNumbered();
  const shots = numbered.reduce((n, f) => n + (f.poc_uploads || []).length, 0);
  const proven = numbered.filter(f => (f.poc_uploads || []).length).length;

  if (head) {
    head.innerHTML = `
      <div class="ws-topbar">
        <div style="min-width:0;">
          <h2 class="font-display font-bold mb-1" style="font-size:20px;letter-spacing:-0.2px;">Closing deck preview</h2>
          <div class="ws-meta">
            <span class="font-mono">${sanitizeInput(closureDraft.filename || '')}</span>
            <span class="ws-sep">|</span>
            <span>${cfg.findings.length} finding${cfg.findings.length === 1 ? '' : 's'} read</span>
            <span class="ws-sep">|</span>
            <span>${proven} with proof attached</span>
          </div>
        </div>
        <div class="flex items-center gap-3" style="flex-shrink:0;">
          <span class="review-flag ${unplaced.length ? 'warn' : 'ok'}">
            ${unplaced.length ? `${unplaced.length} still need an area` : 'Every finding has an area'}
          </span>
          <button class="btn btn-secondary" onclick="discardClosureDraft()">Discard</button>
          <button class="btn btn-primary" id="closure-build-btn"
            ${unplaced.length ? 'disabled' : 'onclick="generateClosureFromDraft()"'}>
            &#9889; Generate deck
          </button>
        </div>
      </div>`;
  }

  closureSlideNumber = 0;
  body.innerHTML = [
    closureNoticesHtml(unplaced, shots, numbered.length - proven),
    closureDeckResult ? closureDraftResultHtml() : '',
    closureTitleSlideHtml(),
    closureScopeSlideHtml(numbered.length),
    closureSummarySlidesHtml(numbered),
    closureIssueSlidesHtml(numbered),
    closureScenarioSlidesHtml(numbered)
  ].join('');
}

// closureNoticesHtml is everything the reader has to settle before the deck is
// worth building: findings with no area, and engagement details the document
// never gave.
function closureNoticesHtml(unplaced, shots, without) {
  const missing = closureDraft.missing || [];
  const notes = closureDraft.notes || [];

  const pen = unplaced.length ? `
    <div class="closure-notice warn">
      <div class="font-semibold mb-2">
        ${unplaced.length} finding${unplaced.length === 1 ? '' : 's'} could not be placed in an assessment area
      </div>
      <p class="text-sm text-muted mb-3" style="line-height:1.7;">
        The report did not say which assessment ${unplaced.length === 1 ? 'it belongs' : 'they belong'} to, and
        nothing here guesses. A finding with no area appears on no slide, so the deck will not build until
        each one is given one.
      </p>
      ${unplaced.map(f => `
        <div class="closure-pen-row">
          <span style="flex:1;min-width:0;">${sanitizeInput(f.title || '(untitled)')}</span>
          <select class="input" style="width:280px;" onchange="setClosureFinding(${f._id}, 'area', this.value)">
            <option value="">— choose an area —</option>
            ${REPORT_AREAS.map(a => `<option value="${a.code}">${sanitizeInput(a.label)} (${a.code})</option>`).join('')}
          </select>
        </div>`).join('')}
    </div>` : '';

  const gaps = missing.length ? `
    <div class="closure-notice">
      <div class="font-semibold mb-1">The report did not give ${sanitizeInput(missing.join(', '))}</div>
      <p class="text-sm text-muted" style="line-height:1.7;">
        Type ${missing.length === 1 ? 'it' : 'them'} into the slides below rather than letting the deck
        print a blank where the client's own details belong.
      </p>
    </div>` : '';

  const readNotes = notes.length ? `
    <div class="closure-notice">
      <div class="font-semibold mb-1">While reading the report</div>
      <ul class="text-sm text-muted" style="margin:6px 0 0 18px;line-height:1.7;">
        ${notes.map(n => `<li>${sanitizeInput(n)}</li>`).join('')}
      </ul>
    </div>` : '';

  const proof = `
    <div class="closure-notice">
      <div class="font-semibold mb-1">
        ${shots} screenshot${shots === 1 ? '' : 's'} attached &middot;
        ${without} finding${without === 1 ? '' : 's'} with none
      </div>
      <p class="text-sm text-muted" style="line-height:1.7;">
        Every finding appears in the issues tables whether or not it has a proof. A scenario slide is built
        only where a screenshot is attached &mdash; drop each one into the empty frame on its own slide below,
        taken from the report's evidence section. A finding with several proofs runs onto
        &ldquo;(Cont'd)&rdquo; slides, the way the reference deck does it.
      </p>
    </div>`;

  return `<div class="closure-notices">${pen}${gaps}${readNotes}${proof}</div>`;
}

function closureDraftResultHtml() {
  const data = closureDeckResult || {};
  const unproven = data.findings_without_proof || [];
  const imageErrors = data.image_errors || [];
  return `
    <div class="closure-notice ok">
      <div class="font-semibold mb-2">Closing deck ready</div>
      ${reportFileButtons('pptx', data.pptx_url, '&#128202;', 'PPTX')}
      ${imageErrors.length ? `
        <div class="text-sm" style="margin-top:12px;color:var(--warning);">
          &#9888; ${imageErrors.length} screenshot${imageErrors.length === 1 ? '' : 's'} could not be embedded:
          <ul style="margin:6px 0 0 18px;">${imageErrors.map(e => `<li>${sanitizeInput(e)}</li>`).join('')}</ul>
        </div>` : ''}
      ${unproven.length ? `
        <div class="text-sm" style="margin-top:12px;color:var(--warning);">
          &#9888; ${unproven.length} finding${unproven.length === 1 ? ' has' : 's have'} no screenshot, so
          ${unproven.length === 1 ? 'it got' : 'they got'} no scenario slide:
          <ul style="margin:6px 0 0 18px;">${unproven.map(f => `<li>${sanitizeInput(f)}</li>`).join('')}</ul>
        </div>` : ''}
    </div>`;
}

// ---------------------------------------------------------------------------
// the slides
// ---------------------------------------------------------------------------

// Slides are numbered as they are emitted, in deck order, so the number on a
// card is the number of the slide it becomes. paintClosurePreview resets it.
let closureSlideNumber = 0;

function closureSlideHtml(label, inner, extraClass) {
  closureSlideNumber++;
  return `
    <section class="closure-slide ${extraClass || ''}">
      <div class="closure-slide-tab"><span class="num">${closureSlideNumber}</span>${sanitizeInput(label)}</div>
      <div class="closure-canvas">${inner}</div>
    </section>`;
}

function closureTitleSlideHtml() {
  const cfg = closureDraft.config;
  const logo = cfg.company_logo
    ? `<img src="${cfg.company_logo}" alt="" class="closure-logo-img">`
    : `<span class="text-xs text-muted">No client logo</span>`;

  return closureSlideHtml('Title', `
    <div class="closure-title-slide">
      <div class="closure-logo" onclick="document.getElementById('closure-logo-input').click()" title="Upload the client's logo">
        ${logo}
        <input type="file" id="closure-logo-input" accept="image/*" style="display:none;"
          onchange="setClosureLogo(this.files)">
      </div>
      <input class="closure-edit closure-edit-title" value="${sanitizeInput(cfg.company_name || '')}"
        placeholder="The client's name" oninput="setClosureField('company_name', this.value)">
      <div class="closure-title-sub">Vulnerability Assessment &amp; Penetration Testing &mdash; Closing Meeting</div>
      <div class="closure-title-meta">
        <label>Date on the slide
          <input class="closure-edit" value="${sanitizeInput(cfg.report_date || '')}"
            placeholder="e.g. 17th August 2026" oninput="setClosureField('report_date', this.value)">
        </label>
        <label>Reference
          <input class="closure-edit" value="${sanitizeInput(cfg.ref_number || '')}"
            placeholder="e.g. GH-REP-047-3292129" oninput="setClosureField('ref_number', this.value)">
        </label>
      </div>
      <div class="closure-hint">
        The logo also fills the hole in the middle of the summary slide's ring. The reference names the
        file the deck is downloaded as.
      </div>
    </div>`);
}

function closureScopeSlideHtml(total) {
  const cfg = closureDraft.config;
  const rows = (cfg.areas || []).map((a, i) => {
    const def = REPORT_AREAS.find(x => x.code === (a.code || '').toUpperCase());
    return `
      <div class="closure-scope-cell">
        <div class="closure-scope-head">${sanitizeInput(def ? def.label : a.code || '')}</div>
        <textarea class="closure-edit closure-edit-area" rows="3"
          placeholder="${sanitizeInput(def ? def.hint : 'What was tested')}"
          oninput="setClosureAreaScope(${i}, this.value)">${sanitizeInput(a.scope || '')}</textarea>
      </div>`;
  }).join('');

  const period = closurePeriodLine();
  return closureSlideHtml('Executive Summary — scope', `
    <div class="closure-slide-title">Executive Summary</div>
    <div class="closure-scope-grid">${rows || '<div class="closure-empty">No assessment areas were read out of the report.</div>'}</div>
    <ul class="closure-bullets">
      <li>${period
        ? sanitizeInput(period)
        : '<span class="closure-missing">No assessment period — fill in the dates below</span>'}</li>
      <li>A total of ${total} ${total === 1 ? 'vulnerability has' : 'vulnerabilities have'} been discovered,
        analyzed, categorized and reported upon.</li>
    </ul>
    <div class="closure-title-meta">
      <label>Assessment start
        <input class="closure-edit" value="${sanitizeInput(cfg.assessment_start || '')}"
          placeholder="e.g. 17th June 2026" oninput="setClosureField('assessment_start', this.value)">
      </label>
      <label>Assessment end
        <input class="closure-edit" value="${sanitizeInput(cfg.assessment_end || '')}"
          placeholder="e.g. 27th June 2026" oninput="setClosureField('assessment_end', this.value)">
      </label>
    </div>`);
}

// The summary continuation slides carry four callouts around a donut chart, and
// the findings are shared out evenly between them. The exact share-out is the
// server's (buildAreaPanels), so this shows what goes on them area by area
// rather than pretending to know which callout each line lands in.
function closureSummarySlidesHtml(numbered) {
  const codes = closureAreaCodes();
  if (!codes.length) return '';
  const panels = codes.map(code => {
    const mine = numbered.filter(f => f._area === code);
    return `
      <div class="closure-callout">
        <div class="closure-callout-code">${code}</div>
        ${mine.slice(0, 3).map(f => `<div class="closure-callout-line">${sanitizeInput(closureClip(f.title, 60))}</div>`).join('')
          || '<div class="closure-callout-line closure-missing">No findings</div>'}
        ${mine.length > 3 ? `<div class="closure-callout-more">+${mine.length - 3} more, continued on the next callout</div>` : ''}
      </div>`;
  }).join('');

  const slides = Math.ceil(codes.length / 4) || 1;
  return closureSlideHtml(`Executive Summary — headline findings${slides > 1 ? ` (${slides} slides)` : ''}`, `
    <div class="closure-slide-title">Executive Summary cont'd</div>
    <div class="closure-callouts">${panels}</div>
    <div class="closure-hint">
      Four callouts to a slide, at most three findings each, spread evenly around the donut chart. The
      charts themselves are drawn from the findings below &mdash; there is nothing to correct here.
    </div>`);
}

function closureClip(s, n) {
  const text = (s || '').trim();
  if (text.length <= n) return text;
  const cut = text.slice(0, n);
  const space = cut.lastIndexOf(' ');
  return (space > 20 ? cut.slice(0, space) : cut) + '…';
}

function closureIssueSlidesHtml(numbered) {
  const groups = closureIssueSlides(numbered);
  if (!groups.length) {
    return closureSlideHtml('Issues', '<div class="closure-empty">No findings have an area yet.</div>');
  }
  return groups.map((group, i) => {
    const areas = [...new Set(group.map(f => f._area))].join('/');
    const sevs = [...new Set(group.map(f => closureSeverityLabel(f.severity)))].join('/');
    return closureSlideHtml(`${areas} issues`, `
      <div class="closure-slide-title">${sanitizeInput(areas)} Issues &ndash; ${sanitizeInput(sevs)} Level (${i + 1}/${groups.length})</div>
      <div class="closure-issues">
        <div class="closure-issues-head">
          <span>Vulnerability</span><span>Severity</span><span>Recommendation</span>
        </div>
        ${group.map(closureIssueRowHtml).join('')}
      </div>`);
  }).join('');
}

function closureIssueRowHtml(f) {
  const sev = (f.severity || 'informational').toLowerCase();
  return `
    <div class="closure-issue-row">
      <div>
        <input class="closure-edit closure-edit-strong" value="${sanitizeInput(f.title || '')}"
          placeholder="The vulnerability" oninput="setClosureFinding(${f._id}, 'title', this.value)">
        <textarea class="closure-edit" rows="2" placeholder="Description — the slide shows its first sentence"
          oninput="setClosureFinding(${f._id}, 'description', this.value)">${sanitizeInput(f.description || '')}</textarea>
        <div class="closure-affected">
          <span>Affected Host:</span>
          <input class="closure-edit" value="${sanitizeInput(f.affected_system || '')}"
            placeholder="host or URL" oninput="setClosureFinding(${f._id}, 'affected_system', this.value)">
        </div>
        <div class="closure-row-foot">
          <span class="font-mono">${sanitizeInput(f._vulnId)}</span>
          <select class="closure-edit closure-edit-inline" onchange="setClosureFinding(${f._id}, 'area', this.value)">
            ${REPORT_AREAS.map(a => `<option value="${a.code}" ${a.code === f._area ? 'selected' : ''}>${a.code}</option>`).join('')}
          </select>
        </div>
      </div>
      <div>
        <select class="closure-edit closure-edit-sev" style="color:${closureSeverityHex(sev)};"
          onchange="setClosureFinding(${f._id}, 'severity', this.value)">
          ${['critical', 'high', 'medium', 'low', 'informational'].map(s =>
            `<option value="${s}" ${s === sev || (s === 'informational' && sev === 'info') ? 'selected' : ''}>${closureSeverityLabel(s)}</option>`).join('')}
        </select>
      </div>
      <div>
        <textarea class="closure-edit" rows="4" placeholder="Recommendation"
          oninput="setClosureFinding(${f._id}, 'recommendation', this.value)">${sanitizeInput(f.recommendation || '')}</textarea>
      </div>
    </div>`;
}

// One scenario slide per screenshot, and one empty frame per finding that has
// none — an empty frame is a slide that will not be in the deck, and saying so
// on the slide itself is clearer than a list of names at the top.
function closureScenarioSlidesHtml(numbered) {
  if (!numbered.length) return '';
  return numbered.map(f => {
    const shots = f.poc_uploads || [];
    if (!shots.length) {
      return closureSlideHtml('Scenario — no proof yet', closureScenarioBody(f, 1, null, 0), 'is-empty');
    }
    return shots.map((shot, i) =>
      closureSlideHtml(`Scenario — ${f._vulnId}`, closureScenarioBody(f, i + 1, shot, i))).join('');
  }).join('');
}

function closureScenarioBody(f, part, shot, index) {
  const result = (f.impact || '').trim() || closureFirstSentence(f.description);
  const frame = shot ? `
    <div class="closure-shot">
      <img src="${shot.data.startsWith('data:') ? shot.data : `data:${shot.mime_type || 'image/png'};base64,${shot.data}`}" alt="">
      <button class="closure-shot-remove" onclick="removeClosureShot(${f._id}, ${index})" title="Remove this screenshot">&#10005;</button>
    </div>` : `
    <div class="closure-frame" onclick="document.getElementById('closure-shot-${f._id}').click()">
      <div style="font-size:26px;">&#128247;</div>
      <div class="font-semibold" style="font-size:13px;">Drop the screenshot here</div>
      <div class="text-xs text-muted">from the report's evidence section &middot; PNG, JPG or GIF</div>
      <div class="text-xs" style="color:var(--warning);margin-top:8px;">
        Until one is here, this finding gets no scenario slide.
      </div>
    </div>`;

  return `
    <div class="closure-slide-title">
      Vulnerability Scenario &ndash; ${sanitizeInput(f._vulnId)}${part > 1 ? " (Cont'd)" : ''}
    </div>
    <div class="closure-scenario">
      <div class="closure-scenario-rows">
        <div><span>Vulnerability</span><b>${sanitizeInput(f.title || '')}</b></div>
        <div><span>Affected Host</span><b>${sanitizeInput(f.affected_system || '')}</b></div>
        <div><span>Result</span><b>${sanitizeInput(result || '')}</b></div>
      </div>
      ${frame}
    </div>
    <div class="closure-shot-actions">
      <button class="btn btn-secondary btn-sm" onclick="document.getElementById('closure-shot-${f._id}').click()">
        ${shot ? '&#10133; Add another proof' : '&#11014; Upload the proof'}
      </button>
      <span class="text-xs text-muted">
        ${(f.poc_uploads || []).length} attached &middot; each extra one becomes a &ldquo;(Cont'd)&rdquo; slide
      </span>
      <input type="file" id="closure-shot-${f._id}" accept=".png,.jpg,.jpeg,.gif" multiple style="display:none;"
        onchange="addClosureShots(${f._id}, this.files)">
    </div>`;
}

// ---------------------------------------------------------------------------
// editing
// ---------------------------------------------------------------------------

// setClosureField edits an engagement detail. The slides are not repainted for
// a keystroke - that would take the cursor out of the field being typed in -
// so the derived lines that quote it are refreshed on the next full repaint.
function setClosureField(key, value) {
  if (!closureDraft) return;
  closureDraft.config[key] = value;
}

function setClosureAreaScope(index, value) {
  if (!closureDraft) return;
  const area = (closureDraft.config.areas || [])[index];
  if (area) area.scope = value;
}

function closureFindingById(id) {
  return (closureDraft?.config.findings || []).find(f => f._id === id);
}

// setClosureFinding edits one finding. Area and severity decide which slide the
// finding lands on and in what order, so those two repaint the deck; the text
// fields do not, because repainting mid-word takes the cursor with it.
function setClosureFinding(id, field, value) {
  const f = closureFindingById(id);
  if (!f) return;
  f[field] = value;
  if (field === 'area' || field === 'severity') paintClosurePreview();
}

async function setClosureLogo(files) {
  const file = files && files[0];
  if (!file) return;
  try {
    closureDraft.config.company_logo = await closureReadDataURL(file);
    paintClosurePreview();
  } catch (e) {
    showToast('That logo could not be read', 'error');
  }
}

// addClosureShots attaches screenshots to a finding.
//
// They are carried as base64 in the deck request rather than uploaded to the
// evidence vault first: this draft belongs to no engagement, so there is no
// vault record for them to be, and inventing one would put a stranger's
// screenshot in a client's evidence trail.
async function addClosureShots(id, files) {
  const f = closureFindingById(id);
  if (!f || !files || !files.length) return;

  for (const file of files) {
    const name = file.name || '';
    const ext = name.slice(name.lastIndexOf('.')).toLowerCase();
    if (!CLOSURE_SHOT_EXTS.includes(ext)) {
      showToast(`${name} is not a PNG, JPG or GIF — PowerPoint will not embed it`, 'error');
      continue;
    }
    if (file.size > CLOSURE_SHOT_MAX_MB * 1024 * 1024) {
      showToast(`${name} is ${formatMB(file.size)}. The limit is ${CLOSURE_SHOT_MAX_MB} MB a screenshot.`, 'error');
      continue;
    }
    try {
      f.poc_uploads.push({
        filename: name,
        mime_type: file.type || 'image/png',
        data: await closureReadDataURL(file)
      });
    } catch (e) {
      showToast(`${name} could not be read`, 'error');
    }
  }
  paintClosurePreview();
}

function removeClosureShot(id, index) {
  const f = closureFindingById(id);
  if (!f) return;
  f.poc_uploads.splice(index, 1);
  paintClosurePreview();
}

function closureReadDataURL(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

// ---------------------------------------------------------------------------
// building it
// ---------------------------------------------------------------------------

async function generateClosureFromDraft() {
  if (!closureDraft) return;
  const btn = document.getElementById('closure-build-btn');
  if (btn) btn.disabled = true;
  showToast('Building the closing deck…', 'info');
  try {
    const res = await api.post('/reports/closure', closureDeckPayload());
    closureDeckResult = res.data;
    paintClosurePreview();
    showToast('Closing deck ready', 'success');
    document.querySelector('.closure-notice.ok')?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  } catch (e) {
    showToast('Could not build the deck: ' + (e.message || 'Unknown error'), 'error');
  } finally {
    if (btn) btn.disabled = false;
  }
}

// closureDeckPayload is the draft as /reports/closure takes it: the same
// ReportConfig the wizard posts, with the screenshots carried inline.
function closureDeckPayload() {
  const cfg = closureDraft.config;
  return Object.assign({}, cfg, {
    sections: closureAreaCodes(),
    findings: cfg.findings
      .filter(f => (f.area || '').trim())
      .map(f => {
        const copy = Object.assign({}, f);
        delete copy._id;
        return copy;
      })
  });
}
