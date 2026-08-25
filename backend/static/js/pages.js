// ============================================================
// mCollaborator — All Page Render Functions
// ============================================================

// -------- LOGIN --------
function renderLogin() {
  return `
    <div class="login-card">
      <div class="flex items-center gap-3 mb-4">
        <img src="/images/cyberteq-mark.png" alt="" class="brand-mark brand-mark-lg">
        <div>
          <h1 style="margin:0;">mCollaborator</h1>
          <p style="margin:0;">By Cyberteq Falcon</p>
        </div>
      </div>
      <p>Sign in to your workspace.</p>
      <form onsubmit="handleLogin(event)">
        <label for="email">Email</label>
        <input type="email" id="email" class="input" placeholder="you@example.com" autocomplete="username" required>
        <label for="password">Password</label>
        <input type="password" id="password" class="input" placeholder="••••••••" autocomplete="current-password" required>
        <button type="submit" class="btn btn-primary">Sign In</button>
      </form>
    </div>
  `;
}

async function handleLogin(e) {
  e.preventDefault();
  const email = document.getElementById('email').value;
  const password = document.getElementById('password').value;
  if (isXSSAttempt(email) || isXSSAttempt(password)) {
    showToast('Invalid input detected', 'error');
    return;
  }
  if (!validateEmail(email)) {
    showToast('Enter a valid email address', 'error');
    return;
  }
  const safeEmail = sanitizeInput(email);
  try {
    const res = await api.post('/auth/login', { email: safeEmail, password });
    MCOLLABORATOR.saveToken(res.data.token);
    MCOLLABORATOR.user = res.data.user;
    window.location.hash = MCOLLABORATOR.user.must_change_password ? '#/change-password' : '#/dashboard';
  } catch (err) {
    showToast(err.message, 'error');
  }
}

// -------- CHANGE PASSWORD (first-login gate) --------
function renderChangePassword() {
  return `
    <div class="login-card">
      <div class="flex items-center gap-3 mb-4">
        <img src="/images/cyberteq-mark.png" alt="" class="brand-mark brand-mark-lg">
        <div>
          <h1 style="margin:0;">Set New Password</h1>
          <p style="margin:0;">${sanitizeInput(MCOLLABORATOR.user?.name || '')}</p>
        </div>
      </div>
      <p>You must set a new password before continuing. It stays valid for 90 days.</p>
      <form onsubmit="handleChangePassword(event)">
        <label for="cp-current">Current Password</label>
        <input type="password" id="cp-current" class="input" autocomplete="current-password" required>
        <label for="cp-new">New Password</label>
        <input type="password" id="cp-new" class="input" minlength="${MIN_PASSWORD_LENGTH}" autocomplete="new-password" required>
        <label for="cp-new2">Confirm New Password</label>
        <input type="password" id="cp-new2" class="input" minlength="${MIN_PASSWORD_LENGTH}" autocomplete="new-password" required>
        <button type="submit" class="btn btn-primary">Update Password</button>
      </form>
    </div>
  `;
}

async function handleChangePassword(e) {
  e.preventDefault();
  const current = document.getElementById('cp-current').value;
  const next = document.getElementById('cp-new').value;
  const confirm = document.getElementById('cp-new2').value;
  if (next.length < MIN_PASSWORD_LENGTH) {
    showToast(`Password must be at least ${MIN_PASSWORD_LENGTH} characters`, 'error');
    return;
  }
  if (next !== confirm) {
    showToast('Passwords do not match', 'error');
    return;
  }
  try {
    await api.post('/users/me/password', { current_password: current, new_password: next });
    MCOLLABORATOR.user.must_change_password = false;
    showToast('Password updated', 'success');
    MCOLLABORATOR.navigate('#/dashboard');
  } catch (err) {
    showToast(err.message, 'error');
  }
}

// -------- CLOSURE PREP --------
// Placeholder workspace: project managers get a "work in progress" notice,
// admins get an empty section. The real closure workflow lands here next.
// -------- CLOSURE PREP --------
// The closing meeting is presented from a deck built out of the same findings
// the report is built from. This page generates it.
//
// It reuses the report wizard's state rather than asking for the engagement
// details a second time: a closing deck that disagreed with the report about the
// client name, the date or which findings were included would be worse than no
// deck at all. The page therefore only offers to build once the wizard has been
// filled in, and says so plainly when it has not.
function renderClosurePrep() {
  const s = reportWizardState;
  const ready = (s.companyName || '').trim() !== '' && s.findings.length > 0;
  const withProof = s.findings.filter(f => (f.evidence_ids || []).length > 0).length;

  return `
    <div>
      <div class="flex items-center justify-between mb-6">
        <h2 class="font-display font-bold" style="font-size:22px;">Closure Prep</h2>
        ${ready ? '<span class="status-pill open">Ready</span>' : ''}
      </div>

      ${!ready ? `
        <div class="card p-6" style="text-align:center;padding:64px 24px;">
          <div style="font-size:40px;margin-bottom:12px;">&#128202;</div>
          <h3 class="font-display font-bold mb-2">Fill in the report wizard first</h3>
          <p class="text-sm text-muted" style="max-width:520px;margin:0 auto 20px;">
            The closing deck is built from the same engagement details and findings as the
            report, so that the two cannot disagree. Complete the wizard, then come back.
          </p>
          <a class="btn btn-primary" href="#/reports" style="text-decoration:none;">Open the report wizard</a>
        </div>`
      : `
        <div class="card p-4 mb-4">
          <h4 class="font-semibold mb-2">What the deck will contain</h4>
          <div style="font-size:13px;color:var(--text-muted);">
            <div class="mb-1"><strong>Company:</strong> ${sanitizeInput(s.companyName)}</div>
            <div class="mb-1"><strong>Reference:</strong> ${sanitizeInput(s.refNumber) || 'Not specified'}</div>
            <div class="mb-1"><strong>Findings:</strong> ${s.findings.length} across ${selectedAreaCodes().length} area(s)</div>
            <div class="mb-1"><strong>Vulnerability scenarios:</strong> ${withProof} finding(s) have evidence attached</div>
            <div class="text-xs mt-3">
              Every finding appears in the issues tables. A finding gets a scenario slide only
              where a screenshot is attached to it in the Evidence vault &mdash; the deck shows the
              same image the report does, not a second copy.
            </div>
          </div>
        </div>

        <div id="closure-result" style="border:1px solid var(--border);border-radius:var(--radius);padding:32px;background:var(--bg);min-height:160px;">
          <div style="text-align:center;color:var(--muted);padding:28px 0;">
            <div style="font-size:32px;margin-bottom:8px;">&#127909;</div>
            <p>Generate the closing meeting deck from this engagement.</p>
          </div>
        </div>

        <div style="display:flex;justify-content:flex-end;margin-top:20px;">
          <button class="btn btn-primary" onclick="generateClosureDeck()">&#9889; Generate Closing Deck</button>
        </div>`
      }
    </div>
  `;
}

// ---------------------------------------------------------------------------
// Getting a generated document out of the app.
//
// This has to work in two shells. In a browser tab an <a download> is all it
// takes. Inside the desktop window it is not: WebView2 turns target="_blank"
// into a new-window request that Wails does not handle, so the click is
// swallowed and no request ever reaches the server - the app appears to build a
// report you cannot have. The desktop shell binds SaveReportAs and OpenReport
// for exactly that, and they are preferred whenever they are present.
// ---------------------------------------------------------------------------

// Download URLs of the documents generated in this session, by kind. The
// buttons carry only the kind, so a report title full of spaces and quotes
// never has to survive a round trip through an inline onclick attribute.
const generatedReportFiles = { docx: '', pdf: '', pptx: '' };

// desktopShell is the shell's bound Go API, present only inside the .exe.
function desktopShell() {
  return (window.go && window.go.main && window.go.main.App) || null;
}

// reportFileName is what to save the document as: the URL's last segment is
// the report title, and the kind is its extension.
function reportFileName(kind) {
  const leaf = (generatedReportFiles[kind] || '').split('/').pop() || 'report';
  let name = leaf;
  try { name = decodeURIComponent(leaf); } catch (e) { /* keep the raw leaf */ }
  return `${name}.${kind}`;
}

// reportFileButtons records the download URL and renders the pair of buttons
// that act on it.
function reportFileButtons(kind, url, icon, label) {
  generatedReportFiles[kind] = url;
  const style = 'text-decoration:none;margin:0 8px 8px 0;';
  return `
          <button class="btn btn-primary" style="${style}" onclick="openGeneratedReport('${kind}')">${icon} Open ${label}</button>
          <button class="btn btn-secondary" style="${style}" onclick="downloadGeneratedReport('${kind}')">&#11015; Download ${label}</button>`;
}

// downloadGeneratedReport puts the document on disk.
async function downloadGeneratedReport(kind) {
  const url = generatedReportFiles[kind];
  if (!url) return;
  const name = reportFileName(kind);
  const shell = desktopShell();

  if (shell && shell.SaveReportAs) {
    try {
      const path = await shell.SaveReportAs(url, name);
      // An empty path is the save dialog being cancelled, which is not news.
      if (path) showToast(`Saved to ${path}`, 'success');
    } catch (e) {
      showToast(`Could not save the ${kind.toUpperCase()}: ${e}`, 'error');
    }
    return;
  }

  const link = document.createElement('a');
  link.href = url;
  link.download = name;
  document.body.appendChild(link);
  link.click();
  link.remove();
}

// openGeneratedReport hands the document to whatever the machine opens it
// with - Word, PowerPoint, the PDF viewer.
async function openGeneratedReport(kind) {
  const url = generatedReportFiles[kind];
  if (!url) return;
  const shell = desktopShell();

  if (shell && shell.OpenReport) {
    try {
      await shell.OpenReport(url, reportFileName(kind));
    } catch (e) {
      showToast(`Could not open the ${kind.toUpperCase()}: ${e}`, 'error');
    }
    return;
  }
  window.open(url, '_blank');
}

// generateClosureDeck posts the wizard's engagement to the closure endpoint and
// shows the result, including which findings will have no scenario slide.
async function generateClosureDeck() {
  const btn = document.querySelector('[onclick="generateClosureDeck()"]');
  if (btn) btn.disabled = true;
  showToast('Building the closing deck...', 'info');
  try {
    const res = await api.post('/reports/closure', reportWizardPayload());
    const url = res.data?.pptx_url;
    const unproven = res.data?.findings_without_proof || [];
    const deckLogoError = res.data?.logo_error;

    const box = document.getElementById('closure-result');
    if (box) {
      const logoWarning = deckLogoError ? `
        <div class="text-sm" style="margin-top:16px;color:var(--warning);text-align:left;">
          &#9888; The client logo is not on the title slide: ${sanitizeInput(deckLogoError)}
        </div>` : '';

      const warning = unproven.length ? `
        <div class="text-sm" style="margin-top:16px;color:var(--warning);text-align:left;">
          &#9888; ${unproven.length} finding${unproven.length === 1 ? ' has' : 's have'} no evidence attached, so
          ${unproven.length === 1 ? 'it gets' : 'they get'} no scenario slide:
          <ul style="margin:6px 0 0 18px;">${unproven.map(f => `<li>${sanitizeInput(f)}</li>`).join('')}</ul>
          <span class="text-xs">Attach a screenshot in the Evidence vault and generate again.</span>
        </div>` : '';
      box.innerHTML = `
        <div style="text-align:center;padding:24px 0;">
          <div style="font-size:32px;margin-bottom:8px;">&#9989;</div>
          <p class="font-semibold mb-4">Closing deck ready</p>
          ${reportFileButtons('pptx', url, '&#128202;', 'PPTX')}
          ${warning}
          ${logoWarning}
        </div>`;
    }
    showToast('Closing deck ready', 'success');
  } catch (e) {
    showToast('Could not build the deck: ' + (e.message || 'Unknown error'), 'error');
  } finally {
    if (btn) btn.disabled = false;
  }
}

// -------- LEDGER: DASHBOARD --------
function renderLedgerDashboard() {
  return `
    <div class="dashboard-grid">
      <div class="flex flex-col gap-4" style="grid-column:1/3;">
        <div class="flex items-center justify-between border-b pb-2">
          <h3 class="font-display font-bold">Active Projects</h3>
          <div class="flex items-center gap-3">
            <span class="text-sm text-muted" id="ledger-active-count">—</span>
            ${isAdmin() ? `<button class="btn btn-primary btn-sm" onclick="showNewEngagementModal()">+ New Project</button>` : ''}
          </div>
        </div>
        <div id="ledger-project-list">
          <div class="skeleton" style="height:80px;margin-bottom:8px;"></div>
          <div class="skeleton" style="height:80px;margin-bottom:8px;"></div>
          <div class="skeleton" style="height:80px;"></div>
        </div>

        <div class="flex items-center justify-between border-b pb-2 mt-4">
          <h3 class="font-display font-bold">Recently Completed</h3>
          <span class="text-sm text-muted" id="ledger-completed-count">Last 90 days</span>
        </div>
        <div id="ledger-completed-list">
          <div class="skeleton" style="height:56px;margin-bottom:8px;"></div>
        </div>
      </div>
      <div class="flex flex-col gap-4">
        <div class="flex items-center justify-between border-b pb-2">
          <h3 class="font-display font-bold">Recent Activity</h3>
        </div>
        <div class="card p-4" id="ledger-activity">
          <div class="skeleton" style="height:40px;margin-bottom:8px;"></div>
          <div class="skeleton" style="height:40px;"></div>
        </div>
      </div>
    </div>
  `;
}

// A project counts as completed once its status is closed/completed.
function isCompletedEngagement(e) {
  return e.status === 'closed' || e.status === 'completed';
}

// Completion date: prefer the scheduled end date, fall back to last update.
function engagementCompletedAt(e) {
  return e.timeline?.end_date || e.updated_at || '';
}

function completedWithinDays(e, days) {
  const raw = engagementCompletedAt(e);
  if (!raw) return false;
  const when = new Date(raw);
  if (isNaN(when)) return false;
  const cutoff = new Date();
  cutoff.setDate(cutoff.getDate() - days);
  return when >= cutoff && when <= new Date();
}

async function afterRenderLedgerDashboard() {
  try {
    const [engRes, actRes] = await Promise.all([
      api.get('/engagements'),
      api.get('/activities')
    ]);
    const engagements = engRes.data || [];
    const activities = actRes.data || [];

    const active = engagements.filter(e => !isCompletedEngagement(e));
    const completed = engagements
      .filter(e => isCompletedEngagement(e) && completedWithinDays(e, 90))
      .sort((a, b) => new Date(engagementCompletedAt(b)) - new Date(engagementCompletedAt(a)));

    document.getElementById('ledger-active-count').textContent =
      `${active.length} Ongoing`;
    document.getElementById('ledger-completed-count').textContent =
      `${completed.length} in the last 90 days`;

    document.getElementById('ledger-project-list').innerHTML = active.length ? active.map(e => `
      <a class="flex items-start gap-4 p-4 card mb-3" style="cursor:pointer;" onclick="MCOLLABORATOR.navigate('#/ledger/project',{engagement:${JSON.stringify(e).replace(/"/g,'&quot;')}})">
        <div class="flex-1">
          <div class="flex items-center gap-3 mb-1">
            <h4 class="font-display font-bold" style="font-size:15px;">${e.name}</h4>
            <span class="status-pill ${e.status}">${e.status.replace('_',' ')}</span>
          </div>
          <p class="text-sm text-muted mb-3">Client: ${e.client_name || 'N/A'} | ${(e.scope?.included||[]).length} targets</p>
          <div class="flex items-center gap-4">
            <div class="flex-1" style="max-width:300px;">
              <div style="height:4px;background:var(--border);border-radius:2px;overflow:hidden;">
                <div style="height:100%;width:${Math.floor(Math.random()*80+20)}%;background:var(--primary);"></div>
              </div>
            </div>
          </div>
        </div>
        <span style="color:var(--muted);">→</span>
      </a>
    `).join('') : '<p class="text-sm text-muted">No active projects.</p>';

    document.getElementById('ledger-completed-list').innerHTML = completed.length ? completed.map(e => `
      <a class="flex items-start gap-4 p-4 card mb-3" style="cursor:pointer;opacity:0.85;" onclick="MCOLLABORATOR.navigate('#/ledger/project',{engagement:${JSON.stringify(e).replace(/"/g,'&quot;')}})">
        <div class="flex-1">
          <div class="flex items-center gap-3 mb-1">
            <h4 class="font-display font-bold" style="font-size:15px;">${e.name}</h4>
            <span class="status-pill ${e.status}">${e.status.replace('_',' ')}</span>
          </div>
          <p class="text-sm text-muted mb-3">Client: ${e.client_name || 'N/A'} | ${(e.scope?.included||[]).length} targets</p>
          <div class="flex items-center gap-4">
            <span class="text-xs text-muted font-mono">Completed ${timeAgo(engagementCompletedAt(e))}</span>
          </div>
        </div>
        <span style="color:var(--muted);">→</span>
      </a>
    `).join('') : '<p class="text-sm text-muted">No projects completed in the last 90 days.</p>';

    document.getElementById('ledger-activity').innerHTML = activities.map(a => `
      <div class="flex gap-3 mb-4" style="font-size:13px;">
        <div style="width:6px;height:6px;border-radius:50%;background:var(--primary);margin-top:6px;flex-shrink:0;"></div>
        <div>
          <span class="font-semibold">${a.user_name}</span> ${a.detail || a.action}
          <div class="text-xs text-muted">${timeAgo(a.created_at)}</div>
        </div>
      </div>
    `).join('');
  } catch (e) {
    document.getElementById('ledger-project-list').innerHTML = '<p class="text-muted">Failed to load projects.</p>';
    const done = document.getElementById('ledger-completed-list');
    if (done) done.innerHTML = '<p class="text-muted">Failed to load projects.</p>';
  }
}

// -------- LEDGER: PROJECT LEDGER --------
function renderProjectLedger() {
  return `
    <div class="split-pane" style="height:calc(100vh - 64px - 48px);margin:-24px;">
      <div class="pane pane-left p-4">
        <h4 class="font-display font-bold text-sm mb-4" style="text-transform:uppercase;letter-spacing:0.5px;color:var(--muted);">Scope & Targets</h4>
        <div id="project-nodes">
          <div class="skeleton" style="height:32px;margin-bottom:8px;"></div>
          <div class="skeleton" style="height:32px;"></div>
        </div>
      </div>
      <div class="pane pane-center p-4">
        <div class="flex items-center justify-between mb-4">
          <h4 class="font-display font-bold">Findings</h4>
          ${canWriteFindings() ? `
            <button class="btn btn-sm btn-ghost" onclick="showBulkImportModal()">+ Bulk Import</button>
            <button class="btn btn-primary" onclick="MCOLLABORATOR.currentFinding=null;MCOLLABORATOR.navigate('#/finding-editor')">+ New Finding</button>` : ''}
        </div>
        <div id="project-findings">
          <div class="skeleton" style="height:60px;margin-bottom:8px;"></div>
          <div class="skeleton" style="height:60px;"></div>
        </div>
      </div>
      <div class="pane pane-right p-4">
        <h4 class="font-display font-bold text-sm mb-4" style="text-transform:uppercase;letter-spacing:0.5px;color:var(--muted);">Project Details</h4>
        <div id="project-meta">
          <div class="skeleton" style="height:100px;"></div>
        </div>
      </div>
    </div>
  `;
}

async function afterRenderProjectLedger() {
  const engId = MCOLLABORATOR.currentEngagement?.id;
  if (!engId) {
    ['project-nodes','project-findings','project-meta'].forEach(id => {
      const el = document.getElementById(id);
      if (el) el.innerHTML = '<p class="text-muted text-sm">No engagement selected.</p>';
    });
    return;
  }
  try {
    const engId = MCOLLABORATOR.currentEngagement.id;
    const [nodesRes, findingsRes, engRes] = await Promise.all([
      api.get(`/engagements/${engId}/nodes`),
      api.get(`/engagements/${engId}/findings`),
      api.get(`/engagements/${engId}`)
    ]);
    const nodes = nodesRes.data || [];
    const findings = findingsRes.data || [];
    const eng = engRes.data || MCOLLABORATOR.currentEngagement || {};

    // The scope pane prefers persisted Node records, but falls back to the
    // engagement's scope list so targets entered at creation always show up.
    const scopeTargets = nodes.length
      ? nodes.map(n => `<div class="flex items-center gap-2 px-2 py-2" style="cursor:pointer;border-radius:var(--radius);font-family:var(--font-mono);font-size:13px;hover:background:var(--surface-hover);" onmouseover="this.style.background='var(--surface-hover)'" onmouseout="this.style.background=''">
        <span style="font-size:14px;color:${MCOLLABORATOR.theme==='cyberpunk'?'var(--primary)':MCOLLABORATOR.theme==='ledger'?'var(--primary)':'var(--muted)'};">●</span>
        ${sanitizeInput(n.target)}
        <span class="text-xs text-muted">(${sanitizeInput(n.type || '')})</span>
      </div>`)
      : (eng.scope?.included || []).map(t => `<div class="flex items-center gap-2 px-2 py-2" style="font-family:var(--font-mono);font-size:13px;">
        <span style="font-size:14px;color:var(--muted);">●</span>
        ${sanitizeInput(t)}
      </div>`);

    document.getElementById('project-nodes').innerHTML =
      scopeTargets.length ? scopeTargets.join('') : '<p class="text-sm text-muted">No scope defined.</p>';

    document.getElementById('project-findings').innerHTML = findings.map(f => `
      <div class="flex items-start gap-3 p-3 card mb-2" style="cursor:pointer;" onclick="MCOLLABORATOR.navigate('#/finding-detail',{finding:${JSON.stringify(f).replace(/"/g,'&quot;')}})">
        <div class="badge-severity ${f.severity}" style="flex-shrink:0;width:48px;text-align:center;">${f.cvss_score||''}</div>
        <div class="flex-1">
          <div class="flex items-center justify-between">
            <h5 class="font-semibold" style="font-size:13px;">${f.title}</h5>
            <span class="status-pill ${f.status}" style="font-size:10px;height:20px;">${f.status}</span>
          </div>
          <div class="flex items-center gap-3 text-xs text-muted mt-1">
            <span>${f.created_by ? 'Analyst' : 'System'}</span>
            <span>${timeAgo(f.created_at)}</span>
            ${f.cve ? `<span class="font-mono" style="font-size:11px;">${f.cve}</span>` : ''}
          </div>
        </div>
      </div>
    `).join('');
    
    document.getElementById('project-meta').innerHTML = `
      <dl style="font-size:13px;">
        <dt style="color:var(--muted);font-size:11px;text-transform:uppercase;letter-spacing:0.5px;">Client</dt>
        <dd class="font-semibold mb-3">${eng.client_name || 'N/A'}</dd>
        <div style="display:grid;grid-template-columns:1fr 1fr;gap:8px;margin-bottom:12px;">
          <div>
            <dt style="color:var(--muted);font-size:11px;text-transform:uppercase;">Start</dt>
            <dd class="font-mono" style="font-size:12px;">${eng.timeline?.start_date || 'N/A'}</dd>
          </div>
          <div>
            <dt style="color:var(--muted);font-size:11px;text-transform:uppercase;">End</dt>
            <dd class="font-mono" style="font-size:12px;">${eng.timeline?.end_date || 'N/A'}</dd>
          </div>
        </div>
        <dt style="color:var(--muted);font-size:11px;text-transform:uppercase;">Methodology</dt>
        <dd class="text-sm mb-3">${(eng.methodology || 'N/A').toUpperCase()}</dd>
        <dt style="color:var(--muted);font-size:11px;text-transform:uppercase;">Status</dt>
        <dd><span class="status-pill ${eng.status}">${eng.status || 'N/A'}</span></dd>
      </dl>
      ${isAdmin() && !(eng.status === 'closed' || eng.status === 'completed') ? `
        <button class="btn btn-primary btn-sm w-full mt-4" onclick="markEngagementCompleted('${eng.id}')">Mark Completed</button>` : ''}
    `;
  } catch (e) {
    ['project-nodes','project-findings','project-meta'].forEach(id => {
      const el = document.getElementById(id);
      if (el) el.innerHTML = '<p class="text-muted text-sm">Failed to load.</p>';
    });
  }
    if (engId) startFindingsPolling(engId);
}

// Transition an active project to "completed" so it moves off the active list
// and into the Recently Completed section. Admin-only.
async function markEngagementCompleted(engId) {
  if (!isAdmin()) { showToast('Only admins can close projects', 'error'); return; }
  if (!confirm('Mark this project as completed?')) return;
  try {
    const res = await api.get(`/engagements/${engId}`);
    const eng = res.data || {};
    await api.put(`/engagements/${engId}`, {
      name: eng.name,
      client_name: eng.client_name,
      status: 'completed',
      methodology: eng.methodology,
      scope: eng.scope,
      timeline: eng.timeline,
      team: eng.team
    });
    showToast('Project marked as completed', 'success');
    MCOLLABORATOR.currentEngagement = null;
    MCOLLABORATOR.navigate('#/dashboard');
  } catch (e) {
    showToast(e.message, 'error');
  }
}

// -------- mCollaborator: FINDING EDITOR --------
function renderFindingEditor() {
  return `
    <div class="editor-pane" style="height:calc(100vh - 64px - 48px);margin:-24px;">
      <div class="editor-left">
        <div class="flex items-center justify-between px-6 py-3 border-b" style="background:var(--surface);">
          <span class="text-xs font-mono text-muted">Finding Details</span>
        </div>
        <div style="padding:16px;overflow-y:auto;flex:1;">
          <div style="margin-bottom:16px;">
            <label style="font-size:12px;font-weight:600;display:block;margin-bottom:4px;color:var(--muted);">Title</label>
            <input class="input w-full" id="finding-title" value="SQL Injection in Login Module" placeholder="Finding title...">
          </div>
          <div style="margin-bottom:16px;">
            <label style="font-size:12px;font-weight:600;display:block;margin-bottom:4px;color:var(--muted);">Description</label>
            <textarea class="input w-full" id="finding-description" rows="4" placeholder="Describe the vulnerability..." style="resize:vertical;font-family:var(--font-mono);font-size:12px;">The authentication endpoint is vulnerable to SQL injection. User input from the username parameter is directly concatenated into the SQL query without proper sanitization.</textarea>
          </div>
          <div style="margin-bottom:16px;">
            <label style="font-size:12px;font-weight:600;display:block;margin-bottom:4px;color:var(--muted);">Impact</label>
            <textarea class="input w-full" id="finding-impact" rows="3" placeholder="Business & technical impact..." style="resize:vertical;font-family:var(--font-mono);font-size:12px;">An attacker can bypass authentication, extract sensitive user data, and potentially gain unauthorized access to the entire application database.</textarea>
          </div>
          <div style="margin-bottom:16px;">
            <label style="font-size:12px;font-weight:600;display:block;margin-bottom:4px;color:var(--muted);">CVSS Vector String</label>
            <input class="input w-full" id="finding-cvss-vector" value="CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H" placeholder="CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H" style="font-family:var(--font-mono);font-size:12px;">
          </div>
          <div style="margin-bottom:16px;">
            <label style="font-size:12px;font-weight:600;display:block;margin-bottom:4px;color:var(--muted);">Affected System</label>
            <input class="input w-full" id="finding-affected" value="Login Module - /v1/auth/login" placeholder="Target system / endpoint..." style="font-family:var(--font-mono);font-size:12px;">
          </div>
          <div style="margin-bottom:16px;">
            <label style="font-size:12px;font-weight:600;display:block;margin-bottom:4px;color:var(--muted);">Proof of Concept (POC)</label>
            <div style="position:relative;">
              <textarea class="input w-full" id="finding-poc" rows="4" placeholder="Detailed steps to reproduce..." style="resize:vertical;font-family:var(--font-mono);font-size:12px;">POST /v1/auth/login
{"username": "admin' OR '1'='1", "password": "test"}

Response: HTTP 200 with admin session token</textarea>
              <div style="position:absolute;bottom:8px;right:8px;display:flex;gap:6px;">
                <button type="button" onclick="document.getElementById('poc-image-input').click()" style="background:var(--surface-hover);color:var(--text);border:1px solid var(--border);border-radius:4px;padding:4px 10px;font-size:11px;font-weight:600;cursor:pointer;">+ Upload to Evidence</button>
                <button type="button" onclick="showEvidencePickerForPoc()" style="background:var(--primary);color:var(--bg);border:none;border-radius:4px;padding:4px 10px;font-size:11px;font-weight:600;cursor:pointer;">+ From Evidence</button>
              </div>
              <input type="file" id="poc-image-input" accept="image/*" style="display:none;" onchange="insertPocImage(this)">
            </div>
            <div id="finding-poc-evidence" style="margin-top:8px;display:flex;flex-wrap:wrap;gap:8px;"></div>
            <input type="hidden" id="finding-evidence-ids" value="">
          </div>
          <div style="margin-bottom:16px;">
            <label style="font-size:12px;font-weight:600;display:block;margin-bottom:4px;color:var(--muted);">Recommendation</label>
            <textarea class="input w-full" id="finding-recommendation" rows="3" placeholder="Remediation steps..." style="resize:vertical;font-family:var(--font-mono);font-size:12px;">Use parameterized queries with prepared statements. Implement input validation and apply the principle of least privilege to database accounts.</textarea>
          </div>
        </div>
      </div>
      <div class="editor-right">
        <div class="flex items-center justify-between px-6 py-3 border-b">
          <span class="text-xs font-mono text-muted">Metadata</span>
        </div>
        <div style="padding:16px;border-bottom:1px solid var(--border);background:var(--bg);">
          <div style="display:grid;grid-template-columns:1fr 1fr;gap:8px;">
            <div>
              <label style="font-size:11px;color:var(--muted);font-weight:600;">Severity</label>
              <select class="input w-full" id="finding-severity" style="padding:4px 8px;">
                <option value="critical">Critical</option>
                <option value="high" selected>High</option>
                <option value="medium">Medium</option>
                <option value="low">Low</option>
                <option value="info">Info</option>
              </select>
            </div>
            <div style="grid-column:1/3;">
              <label style="font-size:11px;color:var(--muted);font-weight:600;">Category (Area of Assessment)</label>
              <select class="input w-full" id="finding-category" style="padding:4px 8px;">
                ${REPORT_AREAS.map(a => `<option value="${a.code}">${a.label}</option>`).join('')}
              </select>
            </div>
            <div>
              <label style="font-size:11px;color:var(--muted);font-weight:600;">Status</label>
              <select class="input w-full" id="finding-status" style="padding:4px 8px;">
                <option value="draft">Draft</option>
                <option value="open" selected>Open</option>
                <option value="in_progress">In Progress</option>
              </select>
            </div>
            <div>
              <label style="font-size:11px;color:var(--muted);font-weight:600;">CVSS Score</label>
              <input type="number" class="input w-full" id="finding-cvss" value="8.5" step="0.1" style="padding:4px 8px;">
            </div>
            <div>
              <label style="font-size:11px;color:var(--muted);font-weight:600;">CVE</label>
              <input class="input w-full" id="finding-cve" placeholder="CVE-YYYY-NNNNN" style="padding:4px 8px;">
            </div>
          </div>
        </div>
        <div class="editor-preview" id="markdown-preview">
          <h1>SQL Injection in Login Module</h1>
          <h2>Description</h2>
          <p>The authentication endpoint is vulnerable to SQL injection...</p>
        </div>
        <div style="padding:12px 24px;border-top:1px solid var(--border);display:flex;gap:8px;justify-content:flex-end;background:var(--surface);">
          <button class="btn btn-secondary" onclick="MCOLLABORATOR.navigate('#/ledger/project')">Discard</button>
          <button class="btn btn-primary" onclick="saveFindingFromEditor()">Publish to Ledger</button>
        </div>
      </div>
    </div>
  `;
}

// Save the finding editor form to the current engagement via the API.
async function saveFindingFromEditor() {
  const engId = MCOLLABORATOR.currentEngagement?.id;
  if (!engId) {
    showToast('No engagement selected. Open a project first.', 'error');
    return;
  }
  const title = document.getElementById('finding-title')?.value || '';
  if (!title.trim()) {
    showToast('Finding title is required', 'error');
    return;
  }
  const payload = {
    title: title,
    description: document.getElementById('finding-description')?.value || '',
    impact: document.getElementById('finding-impact')?.value || '',
    cvss_vector: document.getElementById('finding-cvss-vector')?.value || '',
    cvss_score: parseFloat(document.getElementById('finding-cvss')?.value) || 0,
    cve: document.getElementById('finding-cve')?.value || '',
    severity: document.getElementById('finding-severity')?.value || 'info',
    // The category is the assessment area the finding is reported under, so it
    // decides its section, its vulnerability id and its bar in the
    // findings-by-area chart when the report is generated.
    category: document.getElementById('finding-category')?.value || '',
    status: document.getElementById('finding-status')?.value || 'open',
    poc: document.getElementById('finding-poc')?.value || '',
    remediation: document.getElementById('finding-recommendation')?.value || '',
    evidence_ids: getPocEvidenceIds()
  };
  try {
    const isEdit = !!MCOLLABORATOR.currentFinding?.id;
    let res;
    if (isEdit) {
      res = await api.put(`/findings/${MCOLLABORATOR.currentFinding.id}`, payload);
    } else {
      res = await api.post(`/engagements/${engId}/findings`, payload);
    }
    showToast('Finding published to ledger', 'success');
    MCOLLABORATOR.currentFinding = res.data;
    MCOLLABORATOR.navigate('#/ledger/project');
  } catch (e) {
    showToast('Failed to save finding: ' + e.message, 'error');
  }
}

// -------- LEDGER: EVIDENCE VAULT --------
function renderEvidenceVault() {
  return `
    <div>
      <div class="dropzone mb-6" onclick="document.getElementById('file-input').click()">
        <div style="font-size:32px;margin-bottom:8px;">☁️</div>
        <p class="font-display font-bold" style="font-size:16px;">Drop PCAP, logs, or images here</p>
        <p class="text-sm text-muted">or click to browse. Max 500MB.</p>
        <input type="file" id="file-input" style="display:none;" multiple onchange="handleUpload(this)">
      </div>
      <div class="card" id="evidence-list">
        <div class="flex items-center px-4 py-3 border-b" style="background:var(--bg);font-size:11px;font-weight:700;color:var(--muted);text-transform:uppercase;font-family:var(--font-mono);">
          <div style="flex:2;">Filename</div>
          <div style="flex:1;">Uploaded By</div>
          <div style="flex:1;">Date</div>
          <div style="flex:0 0 80px;text-align:right;">Size</div>
        </div>
        <div id="evidence-rows">
          <div class="skeleton" style="height:48px;"></div>
          <div class="skeleton" style="height:48px;"></div>
        </div>
      </div>
    </div>
  `;
}

async function afterRenderEvidenceVault() {
  try {
    const engId = MCOLLABORATOR.currentEngagement?.id;
    const res = await api.get(engId ? `/evidence?engagement_id=${encodeURIComponent(engId)}` : '/evidence');
    const items = res.data || [];
    const rows = document.getElementById('evidence-rows');
    if (items.length === 0) {
      rows.innerHTML = '<div class="p-4 text-sm text-muted text-center">No evidence uploaded.</div>';
      return;
    }
    rows.innerHTML = items.map(ev => `
      <div class="flex items-center px-4 py-3 border-b" style="cursor:pointer;" onclick="window.open('/api/v1/evidence/${ev.id}/file','_blank')" onmouseover="this.style.background='var(--surface-hover)'" onmouseout="this.style.background=''">
        <div style="flex:2;display:flex;align-items:center;gap:8px;">
          ${ev.mime_type?.includes('image')
            ? `<img src="/api/v1/evidence/${ev.id}/file" style="width:36px;height:36px;object-fit:cover;border-radius:4px;" onerror="this.outerHTML='<span style=font-size:18px;>📄</span>'">`
            : `<span style="font-size:18px;">${ev.mime_type?.includes('pcap') ? '📡' : '📄'}</span>`}
          <span class="font-mono" style="font-size:13px;">${ev.filename}</span>
          ${ev.finding_id ? `<span class="status-pill in_progress" style="font-size:9px;">attached</span>` : ''}
        </div>
        <div style="flex:1;font-size:13px;">${sanitizeInput(ev.uploaded_by_name || ev.uploaded_by || 'Unknown')}</div>
        <div style="flex:1;font-size:12px;font-family:var(--font-mono);">${timeAgo(ev.created_at)}</div>
        <div style="flex:0 0 80px;text-align:right;font-size:12px;font-family:var(--font-mono);">${formatBytes(ev.size_bytes)}</div>
      </div>
    `).join('');
  } catch (e) {
    document.getElementById('evidence-rows').innerHTML = '<div class="p-4 text-sm text-muted">Failed to load.</div>';
  }
}

async function handleUpload(input) {
  const files = input.files;
  if (!files.length) return;
  const engId = MCOLLABORATOR.currentEngagement?.id;
  if (!engId) { showToast('No engagement selected. Open a project first.', 'error'); return; }
  for (const file of files) {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('engagement_id', engId);
    formData.append('tags', '#upload');
    try {
      await api.upload('/evidence/upload', formData);
      showToast(`Uploaded ${file.name}`, 'success');
    } catch (e) {
      showToast(`Failed to upload ${file.name}`, 'error');
    }
  }
  MCOLLABORATOR.render();
}

// -------- LEDGER: REPORT GENERATOR WIZARD --------
// The wizard collects every placeholder the mCollaborator template carries, and
// each one exactly once: what the tester types here is what the report prints.
// Everything the findings already imply — issue counts, the severity and
// area charts, vulnerability ids, the naming convention list — is derived on the
// server and is never asked for again.
const REPORT_AREAS = [
  { code: 'IPT',  label: 'Internal Penetration Testing',      hint: 'e.g. 10.0.0.0/24 — 42 internal hosts' },
  { code: 'EPT',  label: 'External Penetration Testing',      hint: 'e.g. 8 public IPs, 3 external domains' },
  { code: 'IPTC', label: 'Internal Cloud Penetration Testing', hint: 'e.g. Azure subscription prod-01' },
  { code: 'WPT',  label: 'Web Application Penetration Testing', hint: 'e.g. https://portal.example.com (2 roles)' },
  { code: 'CFG',  label: 'Configuration Files Review',        hint: 'e.g. 4 firewall + 6 switch configs' },
  { code: 'ASA',  label: 'API Security Assessment',           hint: 'e.g. /api/v1 REST API, 34 endpoints' },
  { code: 'ADT',  label: 'Active Directory Testing',          hint: 'e.g. corp.example.com forest' },
  { code: 'WNA',  label: 'Wireless Network Assessment',       hint: 'e.g. 3 SSIDs across HQ' },
  { code: 'NAR',  label: 'Network Architecture Review',       hint: 'e.g. HQ and DR network topology' }
];

const REPORT_WIZARD_STEPS = 5;

function emptyReportWizardState() {
  return {
    step: 1,
    companyName: '', companyInitials: '', logo: '',
    projectName: 'VAPT Report', refNumber: '', versionLabel: 'Details to be Provided',
    reportDate: '', assessmentStart: '', assessmentEnd: '',
    testerName: '', approverName: '', approverTitle: '',
    areas: {},              // code -> scope text, presence means "selected"
    outOfScope: '', tools: '',
    findings: [], allFindings: [],
    findingAreas: {},       // finding id -> area code
    accountsExisting: [{ account: '', credentials: '' }],
    accountsCreated: [{ account: '', credentials: '' }],
    odSync: false, odFolder: '', format: 'docx'
  };
}

let reportWizardState = emptyReportWizardState();

function renderReportGenerator() {
  reportWizardState = emptyReportWizardState();
  const steps = [];
  for (let s = 1; s <= REPORT_WIZARD_STEPS; s++) {
    steps.push(`
      <div style="width:32px;height:32px;border-radius:50%;display:flex;align-items:center;justify-content:center;font-size:13px;font-weight:700;
        background:${s === 1 ? 'var(--primary)' : 'var(--surface-hover)'};color:${s === 1 ? 'var(--bg)' : 'var(--muted)'};"
        id="wizard-step-${s}">${s}</div>
      ${s < REPORT_WIZARD_STEPS ? '<div style="width:20px;height:2px;background:var(--border);"></div>' : ''}
    `);
  }
  return `
    <div style="max-width:960px;margin:0 auto;">
      <div class="flex items-center justify-between mb-6">
        <h2 class="font-display font-bold" style="font-size:22px;">Report Generation Wizard</h2>
        <div class="flex items-center gap-2">${steps.join('')}</div>
      </div>
      <div class="card" style="padding:32px;" id="report-wizard-content">
        ${renderReportStep1()}
      </div>
    </div>
  `;
}

function wizardField(label, id, stateKey, placeholder) {
  return `
    <div>
      <label style="font-size:13px;font-weight:600;display:block;margin-bottom:6px;">${label}</label>
      <input class="input w-full" id="${id}" placeholder="${placeholder}"
             value="${sanitizeInput(reportWizardState[stateKey] || '')}"
             oninput="reportWizardState.${stateKey}=this.value">
    </div>`;
}

// Step 1 — every cover page, header and footer placeholder, asked once.
function renderReportStep1() {
  return `
    <h3 class="font-display font-bold mb-2" style="font-size:18px;">Step 1: Report Details</h3>
    <p class="text-sm text-muted mb-6">These fill the cover page, the running headers and the footers.</p>
    <div style="display:grid;grid-template-columns:1fr 1fr;gap:16px;margin-bottom:20px;">
      ${wizardField('Client Company Name <span style="color:var(--critical)">*</span>', 'wizard-company-name', 'companyName', 'e.g. BestPoint Savings &amp; Loans')}
      ${wizardField('Company Initials', 'wizard-company-initials', 'companyInitials', 'e.g. BSL — derived from the name if blank')}
      ${wizardField('Report Title', 'wizard-project-name', 'projectName', 'VAPT Report')}
      ${wizardField('Reference Number', 'wizard-ref', 'refNumber', 'e.g. GH-REP-035-26059-01')}
      ${wizardField('Version Label', 'wizard-version', 'versionLabel', 'e.g. 1.0')}
      ${wizardField('Report Date', 'wizard-report-date', 'reportDate', 'e.g. 12th August 2026')}
      ${wizardField('Assessment Start', 'wizard-start', 'assessmentStart', 'e.g. 17th June 2026')}
      ${wizardField('Assessment End', 'wizard-end', 'assessmentEnd', 'e.g. 24th June 2026')}
      ${wizardField('Tester Name (Author)', 'wizard-tester', 'testerName', 'Name of Tester')}
      ${wizardField('Approver Name', 'wizard-approver', 'approverName', 'e.g. Jamal Mekdachi')}
      ${wizardField('Approver Role', 'wizard-approver-title', 'approverTitle', 'e.g. VP, Operations')}
    </div>
    <div style="margin-bottom:20px;">
      <label style="font-size:13px;font-weight:600;display:block;margin-bottom:6px;">Client Logo</label>
      <div class="flex items-center gap-4">
        <div style="width:120px;height:60px;border:2px dashed var(--border);border-radius:var(--radius);display:flex;align-items:center;justify-content:center;overflow:hidden;" id="wizard-logo-preview">
          ${reportWizardState.logo ? `<img src="${reportWizardState.logo}" style="width:100%;height:100%;object-fit:contain;">` : '<span style="color:var(--muted);font-size:24px;">+</span>'}
        </div>
        <div>
          <button class="btn btn-secondary" onclick="document.getElementById('wizard-logo-input').click()">Upload Logo</button>
          <input type="file" id="wizard-logo-input" accept="image/*" style="display:none;" onchange="uploadReportLogo(this)">
          <div class="text-xs text-muted mt-2">PNG, JPG, GIF, WEBP or SVG up to 2MB. Dropped into the customer-logo slot in the page header and scaled to 0.5in tall, and onto the closing deck's title slide.</div>
        </div>
      </div>
    </div>
    <div style="display:flex;justify-content:flex-end;margin-top:24px;">
      <button class="btn btn-primary" onclick="reportWizardNext()">Next Step →</button>
    </div>
  `;
}

// Step 2 — the areas that were assessed. This one selection drives the 2.3 Scope
// table, the naming convention list, the chapter 3 sections and the
// findings-by-area chart, so it is never asked for a second time.
function renderReportStep2() {
  return `
    <h3 class="font-display font-bold mb-2" style="font-size:18px;">Step 2: Scope</h3>
    <p class="text-sm text-muted mb-6">
      Tick only the activities this engagement covered and describe what each one covered.
      The Scope table, the Recommendations Naming Convention, the report sections and the
      findings-by-area chart will contain these and nothing else.
    </p>
    <div class="flex flex-col gap-2 mb-6">
      ${REPORT_AREAS.map(a => {
        const on = Object.prototype.hasOwnProperty.call(reportWizardState.areas, a.code);
        return `
        <div class="card" style="padding:12px 16px;${on ? 'border-color:var(--primary);' : ''}" id="area-card-${a.code}">
          <label class="flex items-center gap-3" style="cursor:pointer;">
            <input type="checkbox" ${on ? 'checked' : ''} onchange="toggleReportArea('${a.code}', this.checked)">
            <span style="font-size:14px;font-weight:600;">${a.label}</span>
            <span class="text-xs font-mono text-muted">${a.code}</span>
          </label>
          <div id="area-scope-${a.code}" style="margin-top:10px;display:${on ? 'block' : 'none'};">
            <textarea class="input w-full" rows="2" placeholder="${a.hint}"
                      style="resize:vertical;font-size:13px;"
                      oninput="reportWizardState.areas['${a.code}']=this.value">${sanitizeInput(reportWizardState.areas[a.code] || '')}</textarea>
          </div>
        </div>`;
      }).join('')}
    </div>
    <div style="margin-bottom:20px;">
      <label style="font-size:13px;font-weight:600;display:block;margin-bottom:6px;">Out of Scope (optional)</label>
      <textarea class="input w-full" id="wizard-out-of-scope" rows="2" placeholder="Anything excluded beyond the template's standard exclusions"
                style="resize:vertical;font-size:13px;" oninput="reportWizardState.outOfScope=this.value">${sanitizeInput(reportWizardState.outOfScope)}</textarea>
    </div>
    <div style="margin-bottom:20px;">
      <label style="font-size:13px;font-weight:600;display:block;margin-bottom:6px;">Tools Used</label>
      <textarea class="input w-full" id="wizard-tools" rows="3" placeholder="One tool per line (e.g. Burp Suite Pro, Nmap, Nessus)"
                style="resize:vertical;font-size:13px;" oninput="reportWizardState.tools=this.value">${sanitizeInput(reportWizardState.tools)}</textarea>
      <div class="text-xs text-muted mt-2">Leave blank to keep the template's standard tool list.</div>
    </div>
    <div style="display:flex;justify-content:space-between;margin-top:24px;">
      <button class="btn btn-secondary" onclick="reportWizardBack()">← Back</button>
      <button class="btn btn-primary" onclick="reportWizardNext()">Next Step →</button>
    </div>
  `;
}

function toggleReportArea(code, on) {
  if (on) {
    if (!Object.prototype.hasOwnProperty.call(reportWizardState.areas, code)) reportWizardState.areas[code] = '';
  } else {
    delete reportWizardState.areas[code];
    // Findings parked in a dropped area fall back to the first remaining one.
    Object.keys(reportWizardState.findingAreas).forEach(id => {
      if (reportWizardState.findingAreas[id] === code) delete reportWizardState.findingAreas[id];
    });
  }
  const box = document.getElementById(`area-scope-${code}`);
  if (box) box.style.display = on ? 'block' : 'none';
  const card = document.getElementById(`area-card-${code}`);
  if (card) card.style.borderColor = on ? 'var(--primary)' : 'var(--border)';
}

function selectedAreaCodes() {
  return REPORT_AREAS.filter(a => Object.prototype.hasOwnProperty.call(reportWizardState.areas, a.code)).map(a => a.code);
}

// Step 3 — which findings go in, and which area each one is reported under. The
// area decides its section, its vulnerability id and its bar in the chart.
function renderReportStep3() {
  return `
    <h3 class="font-display font-bold mb-2" style="font-size:18px;">Step 3: Findings</h3>
    <p class="text-sm text-muted mb-4">
      Choose the findings to report and confirm the area each belongs to. Severity counts and the
      per-area totals in the charts are taken straight from this selection.
    </p>
    <div id="wizard-findings-list" style="max-height:420px;overflow-y:auto;">
      <div class="text-muted text-sm p-4">Loading findings...</div>
    </div>
    <div id="wizard-findings-summary" class="text-sm text-muted mt-4"></div>
    <div style="display:flex;justify-content:space-between;margin-top:24px;">
      <button class="btn btn-secondary" onclick="reportWizardBack()">← Back</button>
      <button class="btn btn-primary" onclick="reportWizardNext()">Next Step →</button>
    </div>
  `;
}

// Step 4 — the appendix. Leaving both tables empty removes the appendix from the
// report rather than printing blank credential rows.
function renderReportStep4() {
  return `
    <h3 class="font-display font-bold mb-2" style="font-size:18px;">Step 4: Appendix — Test Accounts</h3>
    <p class="text-sm text-muted mb-6">
      Optional. Leave both tables empty and the Appendix is left out of the report entirely.
    </p>
    <div class="card p-4 mb-4">
      <h4 class="font-semibold mb-1" style="font-size:14px;">Pre-existing accounts accessed during testing</h4>
      <p class="text-xs text-muted mb-3">Accounts that already existed and were reached during the engagement.</p>
      <div id="accounts-existing">${renderAccountRows('accountsExisting')}</div>
      <button class="btn btn-secondary mt-2" onclick="addAccountRow('accountsExisting')">+ Add row</button>
    </div>
    <div class="card p-4 mb-4">
      <h4 class="font-semibold mb-1" style="font-size:14px;">Accounts created for the test</h4>
      <p class="text-xs text-muted mb-3">Accounts the testers provisioned themselves.</p>
      <div id="accounts-created">${renderAccountRows('accountsCreated')}</div>
      <button class="btn btn-secondary mt-2" onclick="addAccountRow('accountsCreated')">+ Add row</button>
    </div>
    <div style="display:flex;justify-content:space-between;margin-top:24px;">
      <button class="btn btn-secondary" onclick="reportWizardBack()">← Back</button>
      <button class="btn btn-primary" onclick="reportWizardNext()">Next Step →</button>
    </div>
  `;
}

function renderAccountRows(key) {
  return reportWizardState[key].map((row, i) => `
    <div class="flex gap-2 mb-2" style="align-items:center;">
      <input class="input" style="flex:1;" placeholder="Test account" value="${sanitizeInput(row.account)}"
             oninput="reportWizardState.${key}[${i}].account=this.value">
      <input class="input" style="flex:1;" placeholder="Credentials" value="${sanitizeInput(row.credentials)}"
             oninput="reportWizardState.${key}[${i}].credentials=this.value">
      <button class="btn btn-secondary" style="padding:6px 10px;" onclick="removeAccountRow('${key}', ${i})">✕</button>
    </div>
  `).join('');
}

function addAccountRow(key) {
  reportWizardState[key].push({ account: '', credentials: '' });
  const host = document.getElementById(key === 'accountsExisting' ? 'accounts-existing' : 'accounts-created');
  if (host) host.innerHTML = renderAccountRows(key);
}

function removeAccountRow(key, idx) {
  reportWizardState[key].splice(idx, 1);
  if (reportWizardState[key].length === 0) reportWizardState[key].push({ account: '', credentials: '' });
  const host = document.getElementById(key === 'accountsExisting' ? 'accounts-existing' : 'accounts-created');
  if (host) host.innerHTML = renderAccountRows(key);
}

// Step 5 — review what the report will actually contain, then generate.
function renderReportStep5() {
  const sevCounts = { critical: 0, high: 0, medium: 0, low: 0, informational: 0 };
  reportWizardState.findings.forEach(f => {
    const s = (f.severity || 'informational').toLowerCase();
    const key = s === 'info' ? 'informational' : s;
    if (key in sevCounts) sevCounts[key]++;
  });
  const areaCounts = {};
  reportWizardState.findings.forEach(f => {
    const code = reportWizardState.findingAreas[f.id] || selectedAreaCodes()[0] || 'WPT';
    areaCounts[code] = (areaCounts[code] || 0) + 1;
  });

  const sevRow = Object.entries(sevCounts).map(([k, v]) =>
    `<span class="badge-severity ${k === 'informational' ? 'info' : k}" style="margin-right:6px;">${k} ${v}</span>`).join('');
  const areaRows = selectedAreaCodes().map(code => {
    const area = REPORT_AREAS.find(a => a.code === code);
    return `<div class="flex justify-between" style="font-size:13px;padding:3px 0;">
              <span>${area.label}</span><span class="font-mono">${areaCounts[code] || 0}</span>
            </div>`;
  }).join('') || '<div class="text-sm text-muted">No areas selected.</div>';

  return `
    <h3 class="font-display font-bold mb-6" style="font-size:18px;">Step 5: Generate Report</h3>
    <div class="flex flex-col gap-4 mb-6">
      <div class="card p-4">
        <h4 class="font-semibold mb-2">What the report will contain</h4>
        <div style="font-size:13px;color:var(--text-muted);">
          <div class="mb-1"><strong>Company:</strong> ${sanitizeInput(reportWizardState.companyName) || 'Not specified'}</div>
          <div class="mb-1"><strong>Reference:</strong> ${sanitizeInput(reportWizardState.refNumber) || 'Not specified'}</div>
          <div class="mb-3"><strong>Total issues:</strong> ${reportWizardState.findings.length}</div>
          <div class="mb-2"><strong>Findings by severity</strong><br>${sevRow}</div>
          <div class="mb-1"><strong>Findings by area of assessment</strong></div>
          ${areaRows}
          <div class="mt-3"><strong>Appendix:</strong> ${hasTestAccounts() ? 'included' : 'omitted (no test accounts entered)'}</div>
        </div>
      </div>
      <div class="card p-4">
        <label style="font-size:13px;font-weight:600;display:block;margin-bottom:8px;">Export Format</label>
        <div class="flex gap-3">
          <label class="flex items-center gap-2 p-3 card" style="cursor:pointer;flex:1;${reportWizardState.format === 'docx' ? 'border-color:var(--primary);' : ''}">
            <input type="radio" name="report-format" value="docx" ${reportWizardState.format === 'docx' ? 'checked' : ''} onchange="reportWizardState.format='docx'">
            <span style="font-size:14px;">📄 DOCX</span>
          </label>
          <label class="flex items-center gap-2 p-3 card" style="cursor:pointer;flex:1;${reportWizardState.format === 'pdf' ? 'border-color:var(--primary);' : ''}">
            <input type="radio" name="report-format" value="pdf" ${reportWizardState.format === 'pdf' ? 'checked' : ''} onchange="reportWizardState.format='pdf'">
            <span style="font-size:14px;">📕 PDF</span>
          </label>
        </div>
      </div>
      <div class="card p-4">
        <label class="flex items-center gap-3" style="cursor:pointer;">
          <input type="checkbox" ${reportWizardState.odSync ? 'checked' : ''} onchange="reportWizardState.odSync=this.checked;document.getElementById('wizard-od-folder-wrap').style.display=this.checked?'block':'none'">
          <span style="font-size:13px;font-weight:600;">☁️ Sync to OneDrive</span>
        </label>
        <div id="wizard-od-folder-wrap" style="margin-top:10px;display:${reportWizardState.odSync ? 'block' : 'none'};">
          <label style="font-size:12px;font-weight:600;display:block;margin-bottom:6px;">OneDrive Folder (optional)</label>
          <input class="input w-full" id="wizard-od-folder" placeholder="VAPT Reports/${sanitizeInput(reportWizardState.companyName) || 'Acme Corp'}" value="${sanitizeInput(reportWizardState.odFolder)}" oninput="reportWizardState.odFolder=this.value">
          <div class="text-xs text-muted mt-2">
            Uploads the DOCX and the PDF to the Cyberteq OneDrive. Leave blank to file them under
            <strong>VAPT Reports/${sanitizeInput(reportWizardState.companyName) || '&lt;company&gt;'}</strong>.
            Requires OD_TENANT_ID, OD_CLIENT_ID, OD_CLIENT_SECRET and OD_USER on the server.
          </div>
        </div>
      </div>
      <div id="report-preview" style="border:1px solid var(--border);border-radius:var(--radius);padding:32px;background:var(--bg);min-height:200px;">
        <div style="text-align:center;color:var(--muted);padding:40px 0;">
          <div style="font-size:32px;margin-bottom:8px;">📋</div>
          <p>Click "Generate Report" to create your document.</p>
        </div>
      </div>
    </div>
    <div style="display:flex;justify-content:space-between;margin-top:24px;">
      <button class="btn btn-secondary" onclick="reportWizardBack()">← Back</button>
      <button class="btn btn-primary" onclick="generateReportDocument()">⚡ Generate Report</button>
    </div>
  `;
}

function hasTestAccounts() {
  const filled = rows => rows.some(r => (r.account || '').trim() || (r.credentials || '').trim());
  return filled(reportWizardState.accountsExisting) || filled(reportWizardState.accountsCreated);
}

function reportWizardNext() {
  if (reportWizardState.step === 2 && selectedAreaCodes().length === 0) {
    showToast('Select at least one assessment area', 'error');
    return;
  }
  if (reportWizardState.step < REPORT_WIZARD_STEPS) reportWizardState.step++;
  renderReportWizardStep();
}

function reportWizardBack() {
  if (reportWizardState.step > 1) reportWizardState.step--;
  renderReportWizardStep();
}

function renderReportWizardStep() {
  const container = document.getElementById('report-wizard-content');
  if (!container) return;
  const stepFns = { 1: renderReportStep1, 2: renderReportStep2, 3: renderReportStep3, 4: renderReportStep4, 5: renderReportStep5 };
  container.innerHTML = stepFns[reportWizardState.step]();
  for (let s = 1; s <= REPORT_WIZARD_STEPS; s++) {
    const el = document.getElementById(`wizard-step-${s}`);
    if (el) {
      el.style.background = s <= reportWizardState.step ? 'var(--primary)' : 'var(--surface-hover)';
      el.style.color = s <= reportWizardState.step ? 'var(--bg)' : 'var(--muted)';
    }
  }
  if (reportWizardState.step === 3) loadWizardFindings();
}

// LEGACY_CATEGORY_AREA mirrors the server-side mapping so a finding recorded
// before areas existed still opens on a sensible area.
const LEGACY_CATEGORY_AREA = {
  web: 'WPT', webapp: 'WPT', external: 'EPT', internal: 'IPT', cloud: 'IPTC',
  api: 'ASA', config: 'CFG', wireless: 'WNA', ad: 'ADT', architecture: 'NAR', network: 'NAR'
};

// normalizeAreaCode accepts either an area code as the finding editor now stores
// it, or one of the loose category labels it stored before, and returns the area
// code — or '' when it recognises neither.
function normalizeAreaCode(value) {
  const raw = (value || '').trim();
  if (!raw) return '';
  const upper = raw.toUpperCase();
  if (REPORT_AREAS.some(a => a.code === upper)) return upper;
  return LEGACY_CATEGORY_AREA[raw.toLowerCase()] || '';
}

function defaultAreaFor(finding) {
  const codes = selectedAreaCodes();
  const guess = normalizeAreaCode(finding.category);
  if (guess && codes.includes(guess)) return guess;
  return codes[0] || 'WPT';
}

function loadWizardFindings() {
  const engId = MCOLLABORATOR.currentEngagement?.id;
  const container = document.getElementById('wizard-findings-list');
  if (!container) return;
  if (!engId) {
    container.innerHTML = '<p class="text-sm text-muted">No engagement selected. Open a project first.</p>';
    return;
  }
  const codes = selectedAreaCodes();
  api.get(`/engagements/${engId}/findings`).then(res => {
    const findings = res.data || [];
    reportWizardState.allFindings = findings;
    if (findings.length === 0) {
      container.innerHTML = '<p class="text-muted text-sm p-4">No findings available.</p>';
      return;
    }
    const grouped = {};
    findings.forEach(f => {
      const cat = f.severity || 'other';
      if (!grouped[cat]) grouped[cat] = [];
      grouped[cat].push(f);
    });
    container.innerHTML = Object.entries(grouped).map(([cat, items]) => `
      <div class="mb-4">
        <div class="text-xs font-mono text-muted uppercase mb-2" style="padding:4px 0;border-bottom:1px solid var(--border);">${cat} (${items.length})</div>
        ${items.map(f => {
          const included = reportWizardState.findings.some(x => x.id === f.id);
          const area = reportWizardState.findingAreas[f.id] || defaultAreaFor(f);
          return `
          <div class="flex items-center gap-3 p-3" style="border-bottom:1px solid var(--border);">
            <input type="checkbox" value="${f.id}" ${included ? 'checked' : ''} onchange="toggleWizardFinding('${f.id}')">
            <div class="flex-1">
              <div class="font-semibold" style="font-size:13px;">${sanitizeInput(f.title || '')}</div>
              <div class="text-xs text-muted font-mono">CVSS: ${f.cvss_score || 'N/A'} ${f.cve ? '| ' + f.cve : ''}</div>
            </div>
            <select class="input" style="width:220px;padding:4px 8px;font-size:12px;" onchange="setWizardFindingArea('${f.id}', this.value)">
              ${codes.map(c => {
                const a = REPORT_AREAS.find(x => x.code === c);
                return `<option value="${c}" ${c === area ? 'selected' : ''}>${a.label}</option>`;
              }).join('')}
            </select>
          </div>`;
        }).join('')}
      </div>
    `).join('');
    updateWizardFindingsSummary();
  }).catch(() => {
    container.innerHTML = '<p class="text-muted text-sm p-4">Failed to load findings.</p>';
  });
}

function updateWizardFindingsSummary() {
  const el = document.getElementById('wizard-findings-summary');
  if (!el) return;
  const counts = {};
  reportWizardState.findings.forEach(f => {
    const code = reportWizardState.findingAreas[f.id] || defaultAreaFor(f);
    counts[code] = (counts[code] || 0) + 1;
  });
  const parts = selectedAreaCodes().map(c => `${c}: ${counts[c] || 0}`);
  el.textContent = `${reportWizardState.findings.length} finding(s) selected — ${parts.join(' · ')}`;
}

function toggleWizardFinding(findingId) {
  const idx = reportWizardState.findings.findIndex(x => x.id === findingId);
  if (idx >= 0) {
    reportWizardState.findings.splice(idx, 1);
  } else {
    const full = reportWizardState.allFindings.find(x => x.id === findingId);
    if (full) {
      reportWizardState.findings.push(full);
      if (!reportWizardState.findingAreas[findingId]) {
        reportWizardState.findingAreas[findingId] = defaultAreaFor(full);
      }
    }
  }
  updateWizardFindingsSummary();
}

function setWizardFindingArea(findingId, code) {
  reportWizardState.findingAreas[findingId] = code;
  updateWizardFindingsSummary();
}

function uploadReportLogo(input) {
  const file = input.files && input.files[0];
  if (!file) return;
  if (file.size > 2 * 1024 * 1024) {
    showToast('Logo too large (max 2MB)', 'error');
    input.value = '';
    return;
  }
  const reader = new FileReader();
  reader.onload = function(e) {
    reportWizardState.logo = e.target.result;
    const preview = document.getElementById('wizard-logo-preview');
    if (preview) preview.innerHTML = `<img src="${e.target.result}" style="width:100%;height:100%;object-fit:contain;">`;
    showToast('Logo uploaded', 'success');
  };
  reader.readAsDataURL(file);
  input.value = '';
}

// reportWizardPayload builds the engagement payload the server renders from.
//
// Both the report and the closing deck post exactly this. Building it in one
// place is what stops the deck and the report disagreeing about the client name,
// the dates, which findings were included, or which evidence is attached to
// them - a disagreement nobody would notice until it was in front of the client.
function reportWizardPayload() {
  const splitLines = v => (v ? v.split(/[\n,]+/).map(s => s.trim()).filter(Boolean) : []);
  const cleanAccounts = rows => rows
    .map(r => ({ account: (r.account || '').trim(), credentials: (r.credentials || '').trim() }))
    .filter(r => r.account || r.credentials);

  const areas = selectedAreaCodes().map(code => ({ code, scope: reportWizardState.areas[code] || '' }));

  const mappedFindings = reportWizardState.findings.map(f => ({
    title: f.title || '',
    description: f.description || '',
    impact: f.impact || '',
    cvss_vector: f.cvss_vector || '',
    cvss_score: String(f.cvss_score || ''),
    severity: f.severity || 'info',
    affected_system: f.affected_system || f.node_id || '',
    poc: f.poc || '',
    recommendation: f.remediation || f.recommendation || '',
    recommendation_header: f.recommendation_header || '',
    attack_vector: f.attack_vector || '',
    category: f.category || '',
    area: reportWizardState.findingAreas[f.id] || defaultAreaFor(f),
    exposure: f.exposure || '',
    vuln_id: '',
    poc_evidence_ids: Array.isArray(f.evidence_ids) ? f.evidence_ids : []
  }));

  return {
    company_name: reportWizardState.companyName,
    company_initials: reportWizardState.companyInitials,
    company_logo: reportWizardState.logo || '',
    engagement_name: reportWizardState.projectName || 'VAPT Report',
    ref_number: reportWizardState.refNumber,
    report_date: reportWizardState.reportDate,
    assessment_start: reportWizardState.assessmentStart,
    assessment_end: reportWizardState.assessmentEnd,
    tester_name: reportWizardState.testerName,
    approver_name: reportWizardState.approverName,
    approver_title: reportWizardState.approverTitle,
    version_label: reportWizardState.versionLabel,
    areas: areas,
    sections: selectedAreaCodes(),
    out_of_scope: splitLines(reportWizardState.outOfScope),
    findings: mappedFindings,
    tools: splitLines(reportWizardState.tools),
    test_accounts_existing: cleanAccounts(reportWizardState.accountsExisting),
    test_accounts_created: cleanAccounts(reportWizardState.accountsCreated),
    sync_onedrive: reportWizardState.odSync,
    onedrive_folder: reportWizardState.odFolder || ''
  };
}

async function generateReportDocument() {
  showToast('Compiling report document...', 'info');
  const btn = document.querySelector('[onclick="generateReportDocument()"]');
  if (btn) btn.disabled = true;
  try {
    const payload = reportWizardPayload();

    const res = await api.post('/reports/export', payload);
    const docxUrl = res.data?.docx_url;
    const pdfUrl = res.data?.pdf_url;
    const pdfError = res.data?.pdf_error;
    const unmatched = res.data?.unmatched_findings || [];
    const logoError = res.data?.logo_error;
    const odStatus = res.data?.od_status;
    const odDocxLink = res.data?.od_docx_link;
    const odPdfLink = res.data?.od_pdf_link;
    const odFolder = res.data?.od_folder;
    const odError = res.data?.od_error;

    const preview = document.getElementById('report-preview');
    if (preview) {
      let buttons = '';
      if (docxUrl) buttons += reportFileButtons('docx', docxUrl, '&#128196;', 'DOCX');
      if (pdfUrl) buttons += reportFileButtons('pdf', pdfUrl, '&#128213;', 'PDF');

      // A PDF is only offered when a real Word or LibreOffice layout engine
      // produced it. Anything less would not be the template.
      const pdfBlock = pdfError ? `
        <div class="text-sm" style="margin-top:16px;color:var(--warning);">
          ⚠️ The DOCX is ready, but no PDF was produced: ${sanitizeInput(pdfError)}<br>
          <span class="text-xs">Install Microsoft Word or LibreOffice on the server, or export the DOCX to PDF yourself.</span>
        </div>` : '';

      // A logo that was uploaded but could not be embedded used to be logged on
      // the server and dropped, which looks exactly like never uploading one.
      const logoBlock = logoError ? `
        <div class="text-sm" style="margin-top:16px;color:var(--warning);text-align:left;">
          &#9888; The client logo could not be added to the page header: ${sanitizeInput(logoError)}<br>
          <span class="text-xs">The report is complete otherwise. Re-upload the logo as a PNG or JPG and generate again.</span>
        </div>` : '';

      // Every test-type row a finding could not be tied to reads Pass. That is
      // the intended default, but a finding matching nothing at all means the
      // vulnerability register and the checklist disagree, so say which ones.
      const unmatchedBlock = unmatched.length ? `
        <div class="text-sm" style="margin-top:16px;color:var(--warning);text-align:left;">
          &#9888; ${unmatched.length} finding${unmatched.length === 1 ? '' : 's'} could not be matched to a test in their area's checklist,
          so those rows read <strong>Pass</strong>:
          <ul style="margin:6px 0 0 18px;">${unmatched.map(f => `<li>${sanitizeInput(f)}</li>`).join('')}</ul>
          <span class="text-xs">Reword the finding title to name the test, or set the row by hand in the DOCX.</span>
        </div>` : '';

      // The sync reports three outcomes: filed, refused because the server has
      // no OneDrive credentials, or attempted and failed. A partial upload -
      // DOCX in, PDF not - comes back as "ok" carrying the reason the PDF
      // is missing, so say both halves rather than claiming a clean sync.
      let odBlock = '';
      if (reportWizardState.odSync) {
        if (odStatus === 'ok') {
          const links = [
            odDocxLink ? `<a href="${odDocxLink}" target="_blank" style="color:var(--primary);">DOCX</a>` : '',
            odPdfLink ? `<a href="${odPdfLink}" target="_blank" style="color:var(--primary);">PDF</a>` : ''
          ].filter(Boolean).join(' · ');
          odBlock = `
            <div class="text-sm" style="margin-top:16px;color:#22C55E;">
              ☁️ Filed in OneDrive under <strong>${sanitizeInput(odFolder || '')}</strong>${links ? ' — open ' + links : ''}
            </div>
            ${odError ? `<div class="text-xs" style="margin-top:6px;color:var(--warning);">⚠️ ${sanitizeInput(odError)}</div>` : ''}`;
        } else if (odStatus === 'failed') {
          odBlock = `
            <div class="text-sm" style="margin-top:16px;color:var(--critical);">
              ⚠️ OneDrive sync failed: ${sanitizeInput(odError || 'unknown error')}
            </div>`;
        } else {
          odBlock = `
            <div class="text-sm" style="margin-top:16px;color:var(--warning);">
              ⚠️ ${sanitizeInput(odError || 'OneDrive is not set up on the server.')}
            </div>`;
        }
      }

      preview.innerHTML = `
        <div style="text-align:center;padding:40px 0;">
          <div style="font-size:32px;margin-bottom:8px;">${pdfError ? '⚠️' : '✅'}</div>
          <p class="font-semibold mb-4">${pdfError ? 'Report generated (DOCX only)' : 'Report generated successfully!'}</p>
          <div style="display:flex;gap:12px;justify-content:center;">${buttons}</div>
          ${pdfBlock}
          ${unmatchedBlock}
          ${logoBlock}
          ${odBlock}
        </div>
      `;
    }
    showToast(pdfError ? 'DOCX ready — PDF could not be produced' : 'Report generated — DOCX and PDF ready',
              pdfError ? 'warning' : 'success');
  } catch (e) {
    showToast('Report generation failed: ' + (e.message || 'Unknown error'), 'error');
  } finally {
    if (btn) btn.disabled = false;
  }
}
// -------- FINDING DETAIL --------
function renderFindingDetail() {
  return `
    <div class="flex" style="height:calc(100vh - 64px - 48px);margin:-24px;">
      <div style="width:400px;border-right:1px solid var(--border);overflow-y:auto;" class="p-3" id="analyzer-list">
        <div class="skeleton" style="height:80px;margin-bottom:8px;"></div>
        <div class="skeleton" style="height:80px;"></div>
      </div>
      <div class="flex-1 overflow-y-auto p-6" id="analyzer-detail">
        <div class="flex items-center justify-center" style="height:200px;color:var(--muted);">
          <p>Select a finding to view details</p>
        </div>
      </div>
    </div>
  `;
}

async function afterRenderFindingDetail() {
  const engId = MCOLLABORATOR.currentEngagement?.id;
  if (!engId) {
    const list = document.getElementById('analyzer-list');
    if (list) list.innerHTML = '<p class="text-muted p-4">No engagement selected.</p>';
    return;
  }
  try {
    const res = await api.get(`/engagements/${engId}/findings`);
    const findings = res.data || [];

    const list = document.getElementById('analyzer-list');
    list.innerHTML = findings.map(f => `
      <div class="finding-card p-3 card mb-2" style="cursor:pointer;border-color:transparent;" 
           onclick="showFindingDetail('${f.id}')" 
           onmouseover="this.style.borderColor='var(--primary)'" 
           onmouseout="this.style.borderColor='transparent'"
           id="fc-${f.id}">
        <div class="flex justify-between items-start mb-2">
          <span class="badge-severity ${f.severity}">${f.severity}</span>
          <span class="text-xs font-mono text-muted">CVSS: ${f.cvss_score}</span>
        </div>
        <h5 class="font-semibold" style="font-size:13px;">${f.title}</h5>
        <p class="text-xs text-muted truncate">${f.node_id || 'N/A'}</p>
      </div>
    `).join('');

    if (findings.length > 0) {
      showFindingDetail(findings[0].id);
    }
  } catch (e) {
    document.getElementById('analyzer-list').innerHTML = '<p class="text-muted p-4">Failed to load findings.</p>';
  }
}

async function showFindingDetail(findingId) {
  try {
    const res = await api.get(`/findings/${findingId}`);
    const f = res.data;
    if (!f) return;

    // Highlight active card
    document.querySelectorAll('.finding-card').forEach(c => c.style.borderColor = 'transparent');
    const card = document.getElementById(`fc-${findingId}`);
    if (card) card.style.borderColor = 'var(--primary)';

    document.getElementById('analyzer-detail').innerHTML = `
      <div style="max-width:900px;margin:0 auto;">
        <div class="flex items-start justify-between mb-6">
          <div>
            <div class="flex items-center gap-3 mb-2">
              <span class="badge-severity ${f.severity}" style="font-size:12px;padding:4px 12px;">
                ${f.severity.toUpperCase()}${f.severity === 'critical' || f.severity === 'high' ? ' RISK' : ''}
              </span>
              <span class="text-sm text-muted">Found: ${timeAgo(f.created_at)}</span>
            </div>
            <h1 class="font-display font-bold" style="font-size:28px;">${f.title}</h1>
            ${f.cvss_vector ? `<div class="flex items-center gap-2 mt-2" style="cursor:pointer;" onclick="navigator.clipboard.writeText('${f.cvss_vector}');showToast('Copied!','success')">
              <span class="text-sm text-muted">Vector:</span>
              <code class="font-mono text-sm" style="color:var(--primary);">${f.cvss_vector}</code>
            </div>` : ''}
          </div>
          <div class="card p-4" style="text-align:center;min-width:100px;">
            <div class="font-display font-bold" style="font-size:32px;color:${f.cvss_score >= 9 ? 'var(--critical)' : f.cvss_score >= 7 ? 'var(--warning)' : 'var(--primary)'};">${f.cvss_score}</div>
            <div class="text-xs text-muted">CVSS v3.1</div>
          </div>
        </div>
        <div style="display:flex;justify-content:flex-end;gap:8px;margin-bottom:16px;">
          ${canWriteFindings() ? `<button class="btn btn-secondary" onclick="MCOLLABORATOR.currentFinding=${JSON.stringify(f).replace(/"/g,'&quot;')};MCOLLABORATOR.navigate('#/finding-editor')">✏️ Edit Finding</button>` : ''}
          ${isAdmin() ? `<button class="btn btn-danger" onclick="deleteFinding('${f.id}')">Delete Finding</button>` : ''}
        </div>
        <div style="border-bottom:1px solid var(--border);margin-bottom:24px;">
          <div class="flex gap-6">
            <button class="tab-btn tab-btn-active" type="button">Evidence & Details</button>
            <button class="tab-btn" type="button" onclick="showToast('Remediation tab','info')">Remediation</button>
          </div>
        </div>
        <div class="mb-6">
          <h3 class="font-display font-bold mb-3">Description</h3>
          <p style="line-height:1.7;color:var(--text-muted);">${f.description || 'No description provided.'}</p>
        </div>
        ${f.remediation ? `
        <div class="mb-6">
          <h3 class="font-display font-bold mb-3">Remediation</h3>
          <p style="line-height:1.7;color:var(--text-muted);">${f.remediation}</p>
        </div>` : ''}
        ${f.poc ? `
        <div class="mb-6">
          <h3 class="font-display font-bold mb-3">Proof of Concept</h3>
          <div id="poc-content" style="background:${MCOLLABORATOR.theme==='insight'?'#0B1120':'var(--bg)'};padding:16px;border-radius:var(--radius);font-size:13px;line-height:1.7;"></div>
        </div>` : ''}
        ${(Array.isArray(f.evidence_ids) && f.evidence_ids.length) ? `
        <div class="mb-6">
          <h3 class="font-display font-bold mb-3">Evidence Attachments</h3>
          <div id="finding-detail-evidence" style="display:flex;flex-wrap:wrap;gap:10px;"></div>
        </div>` : ''}
      </div>
    `;
    // Set POC content as innerHTML to render images
    const pocDiv = document.getElementById('poc-content');
    if (pocDiv && f.poc) {
      pocDiv.innerHTML = f.poc;
    }
    // Render attached evidence thumbnails
    const evWrap = document.getElementById('finding-detail-evidence');
    if (evWrap && Array.isArray(f.evidence_ids)) {
      Promise.all(f.evidence_ids.map(id =>
        api.get(`/evidence/${id}`).then(r => r.data).catch(() => null)
      )).then(evs => {
        const shown = evs.filter(Boolean);
        evWrap.innerHTML = shown.length
          ? shown.map(ev => `
              <a href="/api/v1/evidence/${ev.id}/file" target="_blank" style="display:block;width:120px;text-align:center;text-decoration:none;" title="${ev.filename}">
                <div style="border:1px solid var(--border);border-radius:8px;overflow:hidden;height:90px;background:var(--bg);">
                  ${ev.mime_type?.includes('image')
                    ? `<img src="/api/v1/evidence/${ev.id}/file" style="width:100%;height:100%;object-fit:cover;" onerror="this.outerHTML='<div style=padding:30px 0;>📄</div>'">`
                    : `<div style="padding:30px 0;font-size:24px;">📄</div>`}
                </div>
                <div class="text-xs text-muted truncate" style="margin-top:4px;">${ev.filename}</div>
              </a>`).join('')
          : '';
      });
    }
  } catch (e) {
    document.getElementById('analyzer-detail').innerHTML = '<p class="text-muted">Error loading finding.</p>';
  }
}

// Permanently delete a finding. Admin-only; guarded in the UI and on the API.
async function deleteFinding(findingId) {
  if (!isAdmin()) { showToast('Only admins can delete findings', 'error'); return; }
  if (!confirm('Delete this finding permanently? This cannot be undone.')) return;
  try {
    await api.del('/findings/' + findingId);
    showToast('Finding deleted', 'success');
    MCOLLABORATOR.currentFinding = null;
    MCOLLABORATOR.navigate('#/ledger/project');
  } catch (e) {
    showToast(e.message, 'error');
  }
}

// -------- COMMAND CENTER: ACTIVE ENGAGEMENTS --------
function renderActiveEngagements() {
  return `
    <div class="flex" style="height:calc(100vh - 64px - 48px);margin:-24px;">
      <div style="width:300px;border-right:1px solid var(--border);overflow-y:auto;" class="p-3" id="active-eng-list">
        <div class="skeleton" style="height:64px;margin-bottom:8px;"></div>
        <div class="skeleton" style="height:64px;"></div>
      </div>
      <div class="flex-1 p-6 overflow-y-auto" id="active-eng-detail">
        <div class="flex items-center justify-center" style="height:200px;color:var(--muted);">
          <p>Select an engagement to view details</p>
        </div>
      </div>
    </div>
  `;
}

async function afterRenderActiveEngagements() {
  try {
    const res = await api.get('/engagements');
    const engagements = res.data || [];
    const list = document.getElementById('active-eng-list');
    list.innerHTML = engagements.map(e => `
      <div class="flex items-center gap-3 p-3 card mb-2" style="cursor:pointer;border-left:2px solid transparent;" 
           onclick="showEngagementDetail('${e.id}')"
           onmouseover="this.style.borderLeftColor='var(--primary)'"
           onmouseout="this.style.borderLeftColor='transparent'"
           id="aed-${e.id}">
        <div class="flex-1">
          <div class="font-semibold" style="font-size:13px;">${e.name}</div>
          <div style="font-size:11px;color:var(--muted);">${e.client_name || 'N/A'}</div>
          <div style="display:flex;gap:4px;margin-top:4px;">
            <span class="status-pill ${e.status}" style="font-size:9px;height:18px;padding:0 6px;">${e.status?.replace('_',' ')}</span>
          </div>
        </div>
      </div>
    `).join('');

    if (engagements.length > 0) {
      showEngagementDetail(engagements[0].id);
    }
  } catch (e) {
    document.getElementById('active-eng-list').innerHTML = '<p class="text-muted p-4">Failed to load.</p>';
  }
}

async function showEngagementDetail(engId) {
  try {
    const res = await api.get(`/engagements/${engId}`);
    const eng = res.data;
    if (!eng) return;

    document.querySelectorAll('#active-eng-list .card').forEach(c => c.style.borderLeftColor = 'transparent');
    const card = document.getElementById(`aed-${engId}`);
    if (card) card.style.borderLeftColor = 'var(--primary)';

    document.getElementById('active-eng-detail').innerHTML = `
      <div style="max-width:800px;">
        <div class="flex items-start justify-between mb-6">
          <div>
            <h2 class="font-display font-bold" style="font-size:24px;">${eng.name}</h2>
            <p class="text-muted">Client: ${eng.client_name || 'N/A'}</p>
          </div>
          <span class="status-pill ${eng.status}">${eng.status?.replace('_',' ')}</span>
        </div>
        <div class="card p-4 mb-6">
          <h4 class="font-display font-bold mb-4">Scope Timeline</h4>
          <div style="display:flex;gap:4px;height:40px;align-items:center;">
            ${(eng.timeline?.start_date ? [eng.timeline.start_date, eng.timeline.end_date] : ['Oct 1','Oct 31']).map((d,i) => `
              <div style="flex:${i===0?1:1};height:8px;background:${i===0?'var(--primary)':'var(--muted)'};border-radius:4px;opacity:0.5;position:relative;">
                <span style="position:absolute;top:12px;font-size:11px;font-family:var(--font-mono);color:var(--muted);">${d}</span>
              </div>
            `).join('')}
          </div>
        </div>
        <div class="card p-4 mb-6">
          <h4 class="font-display font-bold mb-4">Scope</h4>
          <div style="font-family:var(--font-mono);font-size:12px;">
            ${(eng.scope?.included || []).map(s => `<div class="mb-1">● ${s}</div>`).join('') || 'No scope defined'}
          </div>
        </div>
        <button class="btn btn-primary" onclick="MCOLLABORATOR.navigate('#/command/report-builder',{engagement:${JSON.stringify(eng).replace(/"/g,'&quot;')}})">Build Report</button>
      </div>
    `;
  } catch (e) {
    document.getElementById('active-eng-detail').innerHTML = '<p class="text-muted">Error loading engagement.</p>';
  }
}

// -------- COMMAND CENTER: VULNERABILITY FEED --------
function renderVulnerabilityFeed() {
  return `
    <div class="flex" style="height:calc(100vh - 64px - 48px);margin:-24px;">
      <div class="flex-1 overflow-auto">
        <table class="table" id="vuln-feed-table">
          <thead style="position:sticky;top:0;z-index:10;">
            <tr>
              <th style="width:80px;">Severity</th>
              <th>Title</th>
              <th style="width:140px;">CVE</th>
              <th style="width:80px;">CVSS</th>
              <th style="width:120px;">Host</th>
              <th style="width:60px;">Port</th>
              <th style="width:100px;">Status</th>
              <th style="width:50px;"></th>
            </tr>
          </thead>
          <tbody id="vuln-feed-body">
            <tr><td colspan="8" class="text-center text-muted">Loading...</td></tr>
          </tbody>
        </table>
      </div>
      <div style="width:240px;border-left:1px solid var(--border);padding:16px;overflow-y:auto;">
        <h4 class="font-display font-bold text-sm mb-4">Filters</h4>
        <div class="mb-4">
          <label style="font-size:12px;font-weight:600;display:block;margin-bottom:6px;">Severity</label>
          <label class="flex items-center gap-2 text-sm mb-2"><input type="checkbox" checked> Critical</label>
          <label class="flex items-center gap-2 text-sm mb-2"><input type="checkbox" checked> High</label>
          <label class="flex items-center gap-2 text-sm mb-2"><input type="checkbox" checked> Medium</label>
          <label class="flex items-center gap-2 text-sm"><input type="checkbox"> Low</label>
        </div>
        <div class="mb-4">
          <label style="font-size:12px;font-weight:600;display:block;margin-bottom:6px;">Status</label>
          <label class="flex items-center gap-2 text-sm mb-2"><input type="checkbox" checked> Open</label>
          <label class="flex items-center gap-2 text-sm"><input type="checkbox"> Mitigated</label>
        </div>
      </div>
    </div>
  `;
}

async function afterRenderVulnerabilityFeed() {
  const engId = MCOLLABORATOR.currentEngagement?.id;
  const tbody = document.getElementById('vuln-feed-body');
  if (!tbody) return;
  if (!engId) {
    tbody.innerHTML = '<tr><td colspan="8" class="text-center text-muted">No engagement selected.</td></tr>';
    return;
  }
  try {
    const res = await api.get(`/engagements/${engId}/findings`);
    const findings = res.data || [];

    const tbody = document.getElementById('vuln-feed-body');
    if (findings.length === 0) {
      tbody.innerHTML = '<tr><td colspan="8" class="text-center text-muted">Zero findings matching current parameters.</td></tr>';
      return;
    }
    tbody.innerHTML = findings.map(f => `
      <tr class="clickable-row ${f.severity === 'critical' ? 'vuln-critical' : ''}" onclick="MCOLLABORATOR.navigate('#/finding-detail',{finding:${JSON.stringify(f).replace(/"/g,'&quot;')}})"
          style="${f.severity === 'critical' ? 'animation: pulseCritical 2s infinite;' : ''}">
        <td><span class="badge-severity ${f.severity}" style="display:block;text-align:center;">[${f.severity.toUpperCase().slice(0,4)}]</span></td>
        <td class="font-semibold" style="font-size:13px;font-family:var(--font-mono);">${f.title}</td>
        <td class="font-mono text-xs">${f.cve || '—'}</td>
        <td class="font-mono font-bold" style="color:${f.cvss_score >= 9 ? 'var(--critical)' : f.cvss_score >= 7 ? 'var(--warning)' : 'var(--text)'};">${f.cvss_score}</td>
        <td class="font-mono text-xs">${f.node_id || '—'}</td>
        <td class="font-mono text-xs">443</td>
        <td><span class="status-pill ${f.status}" style="font-size:9px;height:18px;padding:0 6px;">${f.status}</span></td>
        <td><span style="color:var(--muted);font-size:18px;">⋯</span></td>
      </tr>
    `).join('');
  } catch (e) {
    document.getElementById('vuln-feed-body').innerHTML = '<tr><td colspan="8" class="text-center text-muted">Failed to load.</td></tr>';
  }
}

// -------- COMMAND CENTER: REPORT BUILDER --------
function renderCommandReportBuilder() {
  return `
    <div class="flex" style="height:calc(100vh - 64px - 48px);margin:-24px;">
      <div style="width:250px;border-right:1px solid var(--border);padding:16px;overflow-y:auto;">
        <input class="input w-full mb-4" placeholder="Search findings...">
        <div class="text-xs font-mono text-muted uppercase mb-3">Available Findings</div>
        <div id="builder-findings" class="flex flex-col gap-3">
          <div class="skeleton" style="height:80px;"></div>
          <div class="skeleton" style="height:80px;"></div>
        </div>
      </div>
      <div class="flex-1 overflow-y-auto p-8">
        <div class="report-canvas" id="report-canvas">
          <div style="border-bottom:1px solid var(--border);padding-bottom:16px;margin-bottom:24px;">
            <h1 class="font-display font-bold" style="font-size:28px;">${MCOLLABORATOR.currentEngagement?.name || 'Project X'} — Assessment Report</h1>
          </div>
          <h2 class="font-display font-bold" style="font-size:20px;color:var(--primary);margin-bottom:16px;">Executive Summary</h2>
          <p style="margin-bottom:24px;line-height:1.7;">This report outlines findings from the penetration test. ${MCOLLABORATOR.currentEngagement?.client_name || 'The assessment'} revealed critical vulnerabilities requiring immediate remediation.</p>
          <div id="builder-dropzone" class="hidden" style="border:2px dashed var(--primary);background:color-mix(in srgb, var(--primary) 10%, transparent);padding:24px;text-align:center;margin-bottom:24px;color:var(--primary);font-family:var(--font-mono);">
            Drop finding here to generate narrative block...
          </div>
        </div>
      </div>
      <div style="width:300px;border-left:1px solid var(--border);padding:16px;overflow-y:auto;">
        <h4 class="font-display font-bold mb-4">Document Settings</h4>
        <div class="mb-4">
          <label style="font-size:12px;font-weight:600;display:block;margin-bottom:6px;">Classification</label>
          <select class="input w-full" style="padding:6px 8px;">
            <option>Confidential — Client Eyes Only</option>
            <option>Internal — Cyberteq Falcon</option>
          </select>
        </div>
        <div class="mb-4">
          <label style="font-size:12px;font-weight:600;display:block;margin-bottom:6px;">Template Theme</label>
          <div class="flex gap-2">
            <button class="btn btn-primary" style="flex:1;padding:6px;">Cyber Dark</button>
            <button class="btn btn-secondary" style="flex:1;padding:6px;">Corp Light</button>
          </div>
        </div>
        <hr style="border:none;border-top:1px solid var(--border);margin:16px 0;">
        <div class="text-sm text-muted mb-2">Author: ${MCOLLABORATOR.user?.name || 'System'}</div>
        <div class="text-sm text-muted font-mono mb-2">Date: ${new Date().toLocaleDateString()}</div>
        <div class="text-sm text-muted font-mono">Version: 1.0.0-draft</div>
        <hr style="border:none;border-top:1px solid var(--border);margin:16px 0;">
        <button class="btn btn-primary w-full" onclick="showToast('Exporting report...','success');this.querySelector('span')&&(this.querySelector('span').textContent='COMPILING...')">Export PDF</button>
      </div>
    </div>
  `;
}

async function afterRenderCommandReportBuilder() {
  const container = document.getElementById('builder-findings');
  if (!container) return;
  const engId = MCOLLABORATOR.currentEngagement?.id;
  if (!engId) {
    container.innerHTML = '<p class="text-sm text-muted">No engagement selected.</p>';
    return;
  }
  try {
    const res = await api.get(`/engagements/${engId}/findings`);
    const findings = res.data || [];
    container.innerHTML = findings.map(f => `
      <div class="draggable-card" draggable="true" ondragstart="onDragStart(event,'${f.id}')">
        <div class="flex items-start justify-between">
          <span class="badge-severity ${f.severity}" style="font-size:10px;">${f.severity.toUpperCase().slice(0,4)}</span>
          <span style="color:var(--muted);font-size:14px;cursor:grab;">⠿</span>
        </div>
        <p class="text-sm truncate mt-1">${f.title}</p>
      </div>
    `).join('');
  } catch (e) {}
}

function onDragStart(event, findingId) {
  event.dataTransfer.setData('text/plain', findingId);
  const dz = document.getElementById('builder-dropzone');
  if (dz) dz.classList.remove('hidden');
}

// -------- BULK IMPORT FINDINGS --------
let bulkImportEngagementId = null;

function showBulkImportModal() {
  if (!MCOLLABORATOR.currentEngagement?.id) { showToast('No engagement selected', 'error'); return; }
  bulkImportEngagementId = MCOLLABORATOR.currentEngagement.id;
  
  const overlay = document.createElement('div');
  overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.6);z-index:9999;display:flex;align-items:center;justify-content:center;';
  overlay.id = 'bulk-import-overlay';
  overlay.innerHTML = `
    <div class="card" style="width:90%;max-width:700px;max-height:80vh;overflow-y:auto;">
      <div class="flex items-center justify-between mb-4">
        <h3 class="font-display font-bold">Bulk Import Findings</h3>
        <button class="btn btn-ghost text-sm" onclick="document.getElementById('bulk-import-overlay').remove()">✕</button>
      </div>
      <p class="text-xs text-muted mb-2">Paste a JSON array of findings below. Each finding must have: title, severity, description. Optional: cve, node_id, cvss_vector, cvss_score, status, poc, remediation, impact, likelihood, assigned_to.</p>
      <textarea id="bulk-import-json" class="input w-full font-mono" style="min-height:300px;font-size:12px;" placeholder='[
  {"title":"SQL Injection","severity":"critical","description":"SQL injection in login","status":"open","category":"web"},
  {"title":"XSS Vulnerability","severity":"high","description":"Stored XSS in profile","status":"open","category":"web"}
]'></textarea>
      <div class="flex justify-end gap-2 mt-4">
        <button class="btn btn-ghost" onclick="document.getElementById('bulk-import-overlay').remove()">Cancel</button>
        <button class="btn btn-primary" onclick="doBulkImport()">Import Findings</button>
      </div>
    </div>
  `;
  document.body.appendChild(overlay);
}

async function doBulkImport() {
  const textarea = document.getElementById('bulk-import-json');
  if (!textarea) return;
  let findings;
  try {
    findings = JSON.parse(textarea.value);
    if (!Array.isArray(findings)) throw new Error('Must be an array');
  } catch (e) {
    showToast('Invalid JSON: ' + e.message, 'error');
    return;
  }
  for (const f of findings) {
    if (!f.title || !f.severity) {
      showToast('Each finding needs title and severity', 'error');
      return;
    }
  }
  try {
    const res = await api.post('/engagements/' + bulkImportEngagementId + '/findings/bulk', { findings: findings });
    showToast('Imported ' + res.data.length + ' findings', 'success');
    document.getElementById('bulk-import-overlay').remove();
    // Force re-render to show new findings
    MCOLLABORATOR.render();
  } catch (e) {
    showToast('Import failed: ' + e.message, 'error');
  }
}

// -------- LIVE FINDINGS POLLING --------
let findingsPollInterval = null;
let lastFindingsCheck = null;

// Refresh cadence for picking up other analysts' changes. Defaults to 15s;
// override by setting FINDINGS_REFRESH_MS before the app loads (e.g. a value
// of 120000 for a 2-minute refresh).
const FINDINGS_REFRESH_MS = (typeof window.FINDINGS_REFRESH_MS !== 'undefined' && window.FINDINGS_REFRESH_MS > 0)
  ? window.FINDINGS_REFRESH_MS
  : 15000;

function startFindingsPolling(engagementId) {
  stopFindingsPolling();
  lastFindingsCheck = new Date().toISOString();
  findingsPollInterval = setInterval(async () => {
    if (!lastFindingsCheck) return;
    try {
      const res = await api.get('/engagements/' + engagementId + '/findings/changes?since=' + encodeURIComponent(lastFindingsCheck));
      if (res.data && res.data.changes && res.data.changes.length > 0) {
        lastFindingsCheck = res.data.checked_at;
        showToast(res.data.changes.length + ' finding(s) updated by other analysts', 'info');
        // Re-render to show updated findings
        const path = MCOLLABORATOR.currentRoute;
        if (path.includes('/ledger/' + engagementId)) {
          MCOLLABORATOR.render();
        }
      }
    } catch (e) {
      // Silently fail - polling should be resilient
    }
  }, FINDINGS_REFRESH_MS);
}

function stopFindingsPolling() {
  if (findingsPollInterval) {
    clearInterval(findingsPollInterval);
    findingsPollInterval = null;
  }
}

// -------- UTILITY FUNCTIONS --------
function timeAgo(dateStr) {
  if (!dateStr) return 'recently';
  const now = new Date();
  const date = new Date(dateStr);
  const seconds = Math.floor((now - date) / 1000);
  if (seconds < 60) return 'just now';
  if (seconds < 3600) return `${Math.floor(seconds/60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds/3600)}h ago`;
  return `${Math.floor(seconds/86400)}d ago`;
}

function formatBytes(bytes) {
  if (!bytes) return '—';
  const units = ['B','KB','MB','GB'];
  let i = 0;
  let size = bytes;
  while (size >= 1024 && i < units.length-1) { size /= 1024; i++; }
  return `${size.toFixed(1)} ${units[i]}`;
}

function showToast(message, type = 'info') {
  let container = document.querySelector('.toast-container');
  if (!container) {
    container = document.createElement('div');
    container.className = 'toast-container';
    document.body.appendChild(container);
  }
  const toast = document.createElement('div');
  toast.className = `toast ${type}`;
  toast.textContent = message;
  container.appendChild(toast);
  setTimeout(() => { toast.style.opacity = '0'; setTimeout(() => toast.remove(), 300); }, 3000);
}

// -------- ROLES --------
function isAdmin() {
  return MCOLLABORATOR.user?.role === 'admin';
}

function isProjectManager() {
  return MCOLLABORATOR.user?.role === 'project_manager';
}

// Whether the current user may create/edit findings. Analysts and admins work
// findings; project managers are reporting-only.
function canWriteFindings() {
  return isAdmin() || MCOLLABORATOR.user?.role === 'analyst';
}

// Generic permission check against the role grant list. Admins always pass.
function hasPermission(perm) {
  if (isAdmin()) return true;
  return (MCOLLABORATOR.user?.permissions || []).includes(perm);
}

// -------- MODAL HELPER --------
function openModal(id, title, bodyHtml, footerHtml) {
  closeModal(id);
  const overlay = document.createElement('div');
  overlay.id = id;
  overlay.className = 'modal-overlay';
  overlay.innerHTML = `
    <div class="card modal-panel">
      <div class="flex items-center justify-between mb-4">
        <h3 class="font-display font-bold">${title}</h3>
        <button class="btn btn-ghost text-sm" onclick="closeModal('${id}')">&#10005;</button>
      </div>
      ${bodyHtml}
      <div class="flex justify-end gap-2 mt-4">${footerHtml}</div>
    </div>
  `;
  overlay.addEventListener('click', ev => { if (ev.target === overlay) closeModal(id); });
  document.body.appendChild(overlay);
  return overlay;
}

function closeModal(id) {
  const el = document.getElementById(id);
  if (el) el.remove();
}

// -------- ADMIN: NEW PROJECT --------
function showNewEngagementModal() {
  if (!isAdmin()) { showToast('Only admins can create projects', 'error'); return; }
  const today = new Date().toISOString().slice(0, 10);
  openModal('new-engagement-overlay', 'New Project', `
    <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;">
      <div style="grid-column:1/3;">
        <label class="modal-label">Project Name *</label>
        <input class="input w-full" id="ne-name" placeholder="e.g. Project Alpha - External Network">
      </div>
      <div style="grid-column:1/3;">
        <label class="modal-label">Client Name *</label>
        <input class="input w-full" id="ne-client" placeholder="e.g. Apex Financial">
      </div>
      <div>
        <label class="modal-label">Methodology</label>
        <select class="input w-full" id="ne-methodology">
          <option value="owasp">OWASP</option>
          <option value="ptes">PTES</option>
          <option value="nist">NIST</option>
          <option value="custom">Custom</option>
        </select>
      </div>
      <div>
        <label class="modal-label">Status</label>
        <select class="input w-full" id="ne-status">
          <option value="planning">Planning</option>
          <option value="in_progress">In Progress</option>
          <option value="review">Review</option>
        </select>
      </div>
      <div>
        <label class="modal-label">Start Date</label>
        <input type="date" class="input w-full" id="ne-start" value="${today}">
      </div>
      <div>
        <label class="modal-label">End Date</label>
        <input type="date" class="input w-full" id="ne-end">
      </div>
      <div style="grid-column:1/3;">
        <label class="modal-label">In-Scope Targets</label>
        <textarea class="input w-full" id="ne-scope" rows="3" placeholder="One per line, e.g. 10.0.0.0/24" style="resize:vertical;font-family:var(--font-mono);font-size:12px;"></textarea>
      </div>
    </div>
  `, `
    <button class="btn btn-ghost" onclick="closeModal('new-engagement-overlay')">Cancel</button>
    <button class="btn btn-primary" onclick="createEngagement()">Create Project</button>
  `);
  setTimeout(() => document.getElementById('ne-name')?.focus(), 50);
}

async function createEngagement() {
  const name = document.getElementById('ne-name').value.trim();
  const client = document.getElementById('ne-client').value.trim();
  if (!name || !client) { showToast('Project name and client are required', 'error'); return; }
  if (isXSSAttempt(name) || isXSSAttempt(client)) { showToast('Invalid input detected', 'error'); return; }

  const scopeRaw = document.getElementById('ne-scope').value || '';
  const included = scopeRaw.split(/[\n,]+/).map(s => s.trim()).filter(Boolean);

  try {
    const res = await api.post('/engagements', {
      name,
      client_name: client,
      status: document.getElementById('ne-status').value,
      methodology: document.getElementById('ne-methodology').value,
      scope: { included, excluded: [] },
      timeline: {
        start_date: document.getElementById('ne-start').value,
        end_date: document.getElementById('ne-end').value
      },
      team: []
    });
    const eng = res.data;
    // Persist each in-scope target as a node so the Scope & Targets pane of
    // the project ledger is populated immediately.
    if (eng && included.length) {
      await Promise.all(included.map(t =>
        api.post(`/engagements/${eng.id}/nodes`, { target: t, type: 'host' }).catch(() => {})
      ));
    }
    closeModal('new-engagement-overlay');
    showToast('Project created', 'success');
    MCOLLABORATOR.render();
  } catch (e) {
    showToast(e.message || 'Failed to create project', 'error');
  }
}

// Upload a POC image to the evidence vault (persists to disk), then attach it
// to the finding's PoC and preview it in the editor.
async function insertPocImage(input) {
  const file = input.files && input.files[0];
  if (!file) return;
  if (file.size > 5 * 1024 * 1024) {
    showToast('Image too large (max 5MB)', 'error');
    input.value = '';
    return;
  }
  const engId = MCOLLABORATOR.currentEngagement?.id;
  if (!engId) {
    showToast('No engagement selected. Open a project first.', 'error');
    input.value = '';
    return;
  }
  const formData = new FormData();
  formData.append('file', file);
  formData.append('engagement_id', engId);
  formData.append('tags', '#poc');
  try {
    const res = await api.upload('/evidence/upload', formData);
    const ev = res.data;
    addPocEvidence(ev);
    showToast('Uploaded to evidence vault and attached to POC', 'success');
  } catch (e) {
    showToast('Upload failed: ' + e.message, 'error');
  }
  input.value = '';
}

// Read the current list of attached evidence ids from the hidden input.
function getPocEvidenceIds() {
  const el = document.getElementById('finding-evidence-ids');
  if (!el || !el.value) return [];
  return el.value.split(',').map(s => s.trim()).filter(Boolean);
}

// Persist the attached evidence id list back into the hidden input.
function setPocEvidenceIds(ids) {
  const el = document.getElementById('finding-evidence-ids');
  if (el) el.value = ids.join(',');
}

// Append an evidence record to the finding's PoC attachments: store its id,
// insert an <img> reference into the POC text, and re-render the previews.
function addPocEvidence(ev) {
  const ids = getPocEvidenceIds();
  if (!ids.includes(ev.id)) ids.push(ev.id);
  setPocEvidenceIds(ids);

  const ta = document.getElementById('finding-poc');
  const imgTag = `\n\n<img src="/api/v1/evidence/${ev.id}/file" alt="POC Screenshot" style="max-width:100%;border:1px solid var(--border);border-radius:4px;">\n`;
  ta.value = ta.value + imgTag;

  renderPocEvidenceList();
}

function removePocEvidence(evId) {
  setPocEvidenceIds(getPocEvidenceIds().filter(id => id !== evId));
  renderPocEvidenceList();
}

// Render thumbnails of the currently attached PoC evidence.
function renderPocEvidenceList() {
  const wrap = document.getElementById('finding-poc-evidence');
  const ta = document.getElementById('finding-poc');
  if (!wrap || !ta) return;
  const ids = getPocEvidenceIds();
  if (ids.length === 0) {
    wrap.innerHTML = '';
    return;
  }
  wrap.innerHTML = ids.map(id => `
    <div style="position:relative;border:1px solid var(--border);border-radius:6px;overflow:hidden;width:72px;height:72px;">
      <img src="/api/v1/evidence/${id}/file" alt="PoC" style="width:100%;height:100%;object-fit:cover;" onerror="this.style.display='none'">
      <button type="button" onclick="removePocEvidence('${id}')" title="Remove" style="position:absolute;top:2px;right:2px;width:18px;height:18px;border-radius:50%;border:none;background:rgba(0,0,0,0.6);color:#fff;font-size:11px;line-height:1;cursor:pointer;">✕</button>
    </div>
  `).join('');
}

// Modal picker: choose an existing evidence record from the current engagement
// to attach as a PoC screenshot.
function showEvidencePickerForPoc() {
  const engId = MCOLLABORATOR.currentEngagement?.id;
  if (!engId) {
    showToast('No engagement selected. Open a project first.', 'error');
    return;
  }
  const overlay = document.createElement('div');
  overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.6);z-index:9999;display:flex;align-items:center;justify-content:center;';
  overlay.id = 'evidence-picker-overlay';
  overlay.innerHTML = `
    <div class="card" style="width:90%;max-width:760px;max-height:80vh;overflow-y:auto;">
      <div class="flex items-center justify-between mb-4">
        <h3 class="font-display font-bold">Attach Evidence as PoC Screenshot</h3>
        <button class="btn btn-ghost text-sm" onclick="document.getElementById('evidence-picker-overlay').remove()">✕</button>
      </div>
      <div id="evidence-picker-list">
        <div class="p-4 text-sm text-muted">Loading evidence vault...</div>
      </div>
    </div>
  `;
  document.body.appendChild(overlay);
  loadEvidencePicker(engId);
}

async function loadEvidencePicker(engId) {
  const container = document.getElementById('evidence-picker-list');
  if (!container) return;
  try {
    const res = await api.get(`/evidence?engagement_id=${encodeURIComponent(engId)}`);
    const items = res.data || [];
    if (items.length === 0) {
      container.innerHTML = '<div class="p-4 text-sm text-muted">No evidence uploaded for this engagement yet. Use "+ Upload to Evidence" in the PoC section first.</div>';
      return;
    }
    container.innerHTML = items.map(ev => `
      <label class="flex items-center gap-3 p-3" style="cursor:pointer;border:1px solid var(--border);border-radius:8px;margin-bottom:8px;${ev.mime_type?.includes('image') ? '' : 'opacity:0.55;'}" onmouseover="this.style.borderColor='var(--primary)'" onmouseout="this.style.borderColor='var(--border)'">
        <input type="radio" name="poc-evidence-pick" value="${ev.id}" style="accent-color:var(--primary);" onchange="pickPocEvidence('${ev.id}','${ev.filename.replace(/'/g, "\\'")}')">
        ${ev.mime_type?.includes('image') ? `<img src="/api/v1/evidence/${ev.id}/file" style="width:56px;height:56px;object-fit:cover;border-radius:6px;" onerror="this.style.visibility='hidden'">` : '<span style="font-size:22px;">📄</span>'}
        <div class="flex-1">
          <div class="font-semibold text-sm">${ev.filename}</div>
          <div class="text-xs text-muted font-mono">${formatBytes(ev.size_bytes)} · ${timeAgo(ev.created_at)}</div>
        </div>
      </label>
    `).join('');
  } catch (e) {
    container.innerHTML = '<div class="p-4 text-sm text-muted">Failed to load evidence.</div>';
  }
}

function pickPocEvidence(id, filename) {
  const ev = { id: id, filename: filename };
  addPocEvidence(ev);
  showToast('Evidence attached to POC', 'success');
  const overlay = document.getElementById('evidence-picker-overlay');
  if (overlay) overlay.remove();
}

// -------- ADMIN: USER MANAGEMENT --------
function renderUserManagement() {
  return `
    <div>
      <div class="flex items-center justify-between mb-6">
        <h3 class="font-display font-bold">Team Members</h3>
        ${isAdmin() ? `<button class="btn btn-primary" onclick="showInviteUserModal()">+ Add User</button>` : ''}
      </div>
      <div class="card" id="user-management-table">
        <table class="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Email</th>
              <th>Role</th>
              <th>Status</th>
              <th>Password Expires</th>
              <th></th>
            </tr>
          </thead>
          <tbody id="user-management-body">
            <tr><td colspan="6" class="text-center text-muted">Loading...</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  `;
}

async function afterRenderUserManagement() {
  try {
    const res = await api.get('/users');
    const users = res.data || [];
    const tbody = document.getElementById('user-management-body');
    if (users.length === 0) {
      tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">No users found.</td></tr>';
      return;
    }
    tbody.innerHTML = users.map(u => {
      const expiry = new Date(u.password_expiry);
      const now = new Date();
      const daysLeft = Math.ceil((expiry - now) / (1000*60*60*24));
      const expiryClass = daysLeft <= 7 ? 'color:var(--critical)' : daysLeft <= 30 ? 'color:var(--warning)' : '';
      return `
      <tr>
        <td class="font-semibold">${u.name}</td>
        <td class="font-mono text-xs">${u.email}</td>
        <td><span class="status-pill ${u.role === 'admin' ? 'critical' : u.role === 'analyst' ? 'in_progress' : 'open'}">${u.role === 'project_manager' ? 'Project Manager' : u.role}</span></td>
        <td><span class="status-pill open">Active</span></td>
        <td class="font-mono text-xs" style="${expiryClass}">${daysLeft > 0 ? daysLeft + ' days' : 'EXPIRED'}</td>
        <td>
          ${u.role !== 'admin' ? `<button class="btn btn-ghost text-xs" onclick="deleteUser('${u.id}')">Remove</button>` : ''}
        </td>
      </tr>
    `}).join('');
  } catch (e) {
    document.getElementById('user-management-body').innerHTML = '<tr><td colspan="6" class="text-center text-muted">Failed to load users.</td></tr>';
  }
}

const MIN_PASSWORD_LENGTH = 8;

function showInviteUserModal() {
  if (!isAdmin()) { showToast('Only admins can add users', 'error'); return; }
  openModal('invite-user-overlay', 'Add User', `
    <div class="flex flex-col gap-3">
      <div>
        <label class="modal-label">Full Name *</label>
        <input class="input w-full" id="iu-name" placeholder="e.g. Sarah Jenkins">
      </div>
      <div>
        <label class="modal-label">Email *</label>
        <input type="email" class="input w-full" id="iu-email" placeholder="you@example.com">
      </div>
      <div>
        <label class="modal-label">Role *</label>
        <select class="input w-full" id="iu-role">
          <option value="analyst">Analyst</option>
          <option value="project_manager">Project Manager</option>
          <option value="admin">Admin</option>
        </select>
      </div>
      <hr style="border:none;border-top:1px solid var(--border);margin:4px 0;">
      <div>
        <label class="modal-label">Password *</label>
        <input type="password" class="input w-full" id="iu-password" autocomplete="new-password" placeholder="At least ${MIN_PASSWORD_LENGTH} characters">
      </div>
      <div>
        <label class="modal-label">Confirm Password *</label>
        <input type="password" class="input w-full" id="iu-password2" autocomplete="new-password" placeholder="Re-enter password">
      </div>
      <p class="text-xs text-muted">Share these credentials with the user directly. They should change the password after first sign-in.</p>
    </div>
  `, `
    <button class="btn btn-ghost" onclick="closeModal('invite-user-overlay')">Cancel</button>
    <button class="btn btn-primary" onclick="createUser()">Add User</button>
  `);
  setTimeout(() => document.getElementById('iu-name')?.focus(), 50);
}

async function createUser() {
  const name = document.getElementById('iu-name').value.trim();
  const email = document.getElementById('iu-email').value.trim();
  const role = document.getElementById('iu-role').value;
  const password = document.getElementById('iu-password').value;
  const confirm = document.getElementById('iu-password2').value;

  if (!name) { showToast('Full name is required', 'error'); return; }
  if (isXSSAttempt(name)) { showToast('Invalid input detected', 'error'); return; }
  if (!validateEmail(email)) { showToast('Enter a valid email address', 'error'); return; }
  if (password.length < MIN_PASSWORD_LENGTH) {
    showToast(`Password must be at least ${MIN_PASSWORD_LENGTH} characters`, 'error');
    return;
  }
  if (password !== confirm) { showToast('Passwords do not match', 'error'); return; }

  try {
    await api.post('/users', { name, email, role, password });
    closeModal('invite-user-overlay');
    showToast('User added', 'success');
    MCOLLABORATOR.render();
  } catch (e) {
    showToast(e.message || 'Failed to add user', 'error');
  }
}

async function deleteUser(userId) {
  if (!confirm('Remove this user?')) return;
  try {
    await api.del('/users/' + userId);
    showToast('User removed', 'success');
    MCOLLABORATOR.render();
  } catch (e) {
    showToast(e.message, 'error');
  }
}

// -------- AFTER RENDER DISPATCH --------
function afterRender(path) {
  switch (true) {
    case path === '/dashboard':
    case path === '/':
      setTimeout(afterRenderLedgerDashboard, 50);
      break;
    case path === '/ledger/project':
      setTimeout(afterRenderProjectLedger, 50);
      break;
    case path === '/evidence':
      setTimeout(afterRenderEvidenceVault, 50);
      break;
    case path === '/finding-detail':
      setTimeout(afterRenderFindingDetail, 50);
      break;
    case path === '/command/engagements':
      setTimeout(afterRenderActiveEngagements, 50);
      break;
    case path === '/command/feed':
      setTimeout(afterRenderVulnerabilityFeed, 50);
      break;
    case path === '/command/report-builder':
      setTimeout(afterRenderCommandReportBuilder, 50);
      break;
    case path === '/admin/users':
      setTimeout(afterRenderUserManagement, 50);
      break;
    case path === '/reports':
    case path === '/report-generator':
      break;
    case path === '/finding-editor':
      setTimeout(afterRenderFindingEditor, 50);
      break;
  }
}

// Pre-fill the finding editor from MCOLLABORATOR.currentFinding (if editing an
// existing finding) and render any attached PoC evidence thumbnails.
function afterRenderFindingEditor() {
  const f = MCOLLABORATOR.currentFinding;
  if (!f) return;
  const set = (id, val) => {
    const el = document.getElementById(id);
    if (el && val) el.value = val;
  };
  set('finding-title', f.title);
  set('finding-description', f.description);
  set('finding-impact', f.impact);
  set('finding-cvss-vector', f.cvss_vector);
  set('finding-affected', f.affected_system || f.node_id);
  set('finding-poc', f.poc);
  set('finding-recommendation', f.remediation);
  set('finding-cve', f.cve);
  const sev = document.getElementById('finding-severity');
  if (sev && f.severity) sev.value = f.severity;
  const status = document.getElementById('finding-status');
  if (status && f.status) status.value = f.status;
  // Findings recorded before the category listed assessment areas carry a loose
  // label ("web", "external"); map those onto the area they mean so the editor
  // opens on something real rather than silently resetting to the first option.
  const cat = document.getElementById('finding-category');
  const catCode = normalizeAreaCode(f.category);
  if (cat && catCode) cat.value = catCode;
  const cvss = document.getElementById('finding-cvss');
  if (cvss && f.cvss_score) cvss.value = f.cvss_score;
  if (Array.isArray(f.evidence_ids) && f.evidence_ids.length) {
    setPocEvidenceIds(f.evidence_ids);
    renderPocEvidenceList();
  }
}

// Pulse animation for critical vulns
const style = document.createElement('style');
style.textContent = `
  @keyframes pulseCritical {
    0%, 100% { background: transparent; }
    50% { background: color-mix(in srgb, var(--critical) 8%, transparent); }
  }
`;
document.head.appendChild(style);
