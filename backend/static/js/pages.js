// ============================================================
// mCollaborator — All Page Render Functions
// ============================================================

// -------- LOGIN --------
function renderLogin() {
  return `
    <div class="login-card">
      <div class="flex items-center gap-3 mb-4">
        <img src="/images/cyberteq-logo.png" alt="Cyberteq" style="height:40px;">
        <div>
          <h1 style="margin:0;">mCollaborator</h1>
          <p style="margin:0;">By Cyberteq Falcon</p>
        </div>
      </div>
      <p>Sign in to your workspace.</p>
      <form onsubmit="handleLogin(event)">
        <label for="email">Email</label>
        <input type="email" id="email" class="input" value="username@cyberteq.io" placeholder="username@cyberteq.io" required>
        <label for="password">Password</label>
        <input type="password" id="password" class="input" value="admin123" placeholder="••••••••" required>
        <button type="submit" class="btn btn-primary">Sign In</button>
        <p style="margin-top:16px;font-size:12px;color:var(--muted);text-align:center;">
          Demo: username@cyberteq.io / admin123
        </p>
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
  if (!validateCyberteqEmail(email)) {
    showToast('Only @cyberteq.io email addresses are allowed', 'error');
    return;
  }
  const safeEmail = sanitizeInput(email);
  try {
    const res = await api.post('/auth/login', { email: safeEmail, password });
    STITCH.saveToken(res.data.token);
    STITCH.user = res.data.user;
    window.location.hash = '#/dashboard';
  } catch (err) {
    showToast(err.message, 'error');
  }
}

// -------- SIDEBAR --------
function renderSidebar(type, currentPath) {
  const isActive = (path) => currentPath === path ? 'active' : '';

  if (type === 'cyberpunk') {
    return `
      <div class="sidebar">
        <div style="padding:8px 0;display:flex;justify-content:center;">
          <img src="/images/cyberteq-logo.png" alt="C" style="height:28px;object-fit:cover;object-position:left;clip-path:inset(0 60% 0 0);">
        </div>
        <div class="nav-item" onclick="STITCH.navigate('#/command/dashboard')" title="Global Threat Dashboard">
          <span class="${isActive('/command/dashboard')}" style="font-size:20px;">⬡</span>
        </div>
        <div class="nav-item ${isActive('/command/engagements')}" onclick="STITCH.navigate('#/command/engagements')" title="Active Engagements">
          <span style="font-size:20px;">◉</span>
        </div>
        <div class="nav-item ${isActive('/command/feed')}" onclick="STITCH.navigate('#/command/feed')" title="Vulnerability Feed">
          <span style="font-size:20px;">☰</span>
        </div>
        <div class="nav-item ${isActive('/command/report-builder')}" onclick="STITCH.navigate('#/command/report-builder')" title="Report Builder">
          <span style="font-size:20px;">▣</span>
        </div>
        <div class="nav-item ${isActive('/command/threat-feed')}" onclick="STITCH.navigate('#/command/threat-feed')" title="Threat Feed">
          <span style="font-size:20px;">⚠</span>
        </div>
        <div class="nav-item ${isActive('/admin/users')}" onclick="STITCH.navigate('#/admin/users')" title="User Management">
          <span style="font-size:20px;">👥</span>
        </div>
        <div style="flex:1;"></div>
        <div class="nav-item ${isActive('/dashboard')}" onclick="STITCH.navigate('#/dashboard')" title="Ledger Dashboard">
          <span style="font-size:16px;">📋</span>
        </div>
        <div class="nav-item ${isActive('/insight/dashboard')}" onclick="STITCH.navigate('#/insight/dashboard')" title="Insight Dashboard">
          <span style="font-size:16px;">📊</span>
        </div>
      </div>
    `;
  }

  if (type === 'ledger') {
    return `
      <div class="sidebar">
        <div class="flex items-center gap-3 px-4 mb-6">
          <img src="/images/cyberteq-logo.png" alt="Cyberteq" style="height:24px;object-fit:contain;">
          <div>
            <div class="font-display font-bold" style="color:#fff;">Cyberteq</div>
            <div style="font-size:11px;color:#9CA3AF;font-family:var(--font-mono);">mCollaborator</div>
          </div>
        </div>
        <div class="nav-item ${isActive('/dashboard')}" onclick="STITCH.navigate('#/dashboard')">
          <span>📊</span> Dashboard
        </div>
        <div class="nav-item ${isActive('/ledger/project')}" onclick="STITCH.navigate('#/ledger/project')">
          <span>📁</span> Project Ledger
        </div>
        <div class="nav-item ${isActive('/finding-editor')}" onclick="STITCH.navigate('#/finding-editor')">
          <span>✏️</span> Finding Editor
        </div>
        <div class="nav-item ${isActive('/evidence')}" onclick="STITCH.navigate('#/evidence')">
          <span>🗄️</span> Evidence Vault
        </div>
        <div class="nav-item ${isActive('/reports')}" onclick="STITCH.navigate('#/reports')">
          <span>📄</span> Report Generator
        </div>
        <div class="nav-item ${isActive('/command/threat-feed')}" onclick="STITCH.navigate('#/command/threat-feed')">
          <span>⚠</span> Threat Feed
        </div>
        <div style="flex:1;"></div>
        <div class="nav-item" style="font-size:11px;color:#6B7280;">
          <span style="font-size:11px;">👤</span> ${STITCH.user?.name || 'User'}
        </div>
      </div>
    `;
  }

  // Insight sidebar
  return `
    <div class="sidebar">
      <div class="flex items-center gap-3 px-3 mb-6">
          <img src="/images/cyberteq-logo.png" alt="Cyberteq" style="height:24px;object-fit:contain;">
          <div>
          <div class="font-display font-bold">Falcon Insight</div>
          <div style="font-size:11px;color:var(--muted);font-family:var(--font-mono);">mCollaborator</div>
        </div>
      </div>
      <div class="nav-item ${isActive('/insight/dashboard')}" onclick="STITCH.navigate('#/insight/dashboard')">
        <span>📊</span> Command Dashboard
      </div>
      <div class="nav-item ${isActive('/insight/projects')}" onclick="STITCH.navigate('#/insight/projects')">
        <span>📋</span> Audit Projects
      </div>
      <div class="nav-item ${isActive('/insight/analyzer')}" onclick="STITCH.navigate('#/insight/analyzer')">
        <span>🔍</span> Finding Analyzer
      </div>
      <div class="nav-item ${isActive('/insight/generator')}" onclick="STITCH.navigate('#/insight/generator')">
        <span>🤖</span> Insight Generator
      </div>
      <div style="flex:1;"></div>
      <div style="padding:12px;border-top:1px solid var(--border);margin-top:16px;">
        <div class="flex items-center gap-3">
          <div style="width:32px;height:32px;border-radius:8px;background:var(--surface-hover);display:flex;align-items:center;justify-content:center;font-size:12px;font-weight:600;">AJ</div>
          <div>
            <div style="font-size:13px;font-weight:500;">${STITCH.user?.name || 'User'}</div>
            <div style="font-size:11px;color:var(--muted);font-family:var(--font-mono);">Lead Analyst</div>
          </div>
        </div>
      </div>
    </div>
  `;
}

// -------- LEDGER: DASHBOARD --------
function renderLedgerDashboard() {
  return `
    <div class="dashboard-grid">
      <div class="flex flex-col gap-4" style="grid-column:1/3;">
        <div class="flex items-center justify-between border-b pb-2">
          <h3 class="font-display font-bold">Active Projects</h3>
          <span class="text-sm text-muted">4 Ongoing</span>
        </div>
        <div id="ledger-project-list">
          <div class="skeleton" style="height:80px;margin-bottom:8px;"></div>
          <div class="skeleton" style="height:80px;margin-bottom:8px;"></div>
          <div class="skeleton" style="height:80px;"></div>
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

async function afterRenderLedgerDashboard() {
  try {
    const [engRes, actRes] = await Promise.all([
      api.get('/engagements'),
      api.get('/activities')
    ]);
    const engagements = engRes.data || [];
    const activities = actRes.data || [];

    document.getElementById('ledger-project-list').innerHTML = engagements.map(e => `
      <a class="flex items-start gap-4 p-4 card mb-3" style="cursor:pointer;" onclick="STITCH.navigate('#/ledger/project',{engagement:${JSON.stringify(e).replace(/"/g,'&quot;')}})">
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
    `).join('');

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
          <button class="btn btn-sm btn-ghost" onclick="showBulkImportModal()">+ Bulk Import</button>
          <button class="btn btn-primary" onclick="STITCH.navigate('#/finding-editor')">+ New Finding</button>
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
  try {
    const engId = STITCH.currentEngagement?.id || 'eng_001';
    const [nodesRes, findingsRes, engRes] = await Promise.all([
      api.get(`/engagements/${engId}/nodes`),
      api.get(`/engagements/${engId}/findings`),
      api.get(`/engagements/${engId}`)
    ]);
    const nodes = nodesRes.data || [];
    const findings = findingsRes.data || [];
    const eng = engRes.data || STITCH.currentEngagement || {};

    document.getElementById('project-nodes').innerHTML = nodes.length ? nodes.map(n => `
      <div class="flex items-center gap-2 px-2 py-2" style="cursor:pointer;border-radius:var(--radius);font-family:var(--font-mono);font-size:13px;hover:background:var(--surface-hover);" onmouseover="this.style.background='var(--surface-hover)'" onmouseout="this.style.background=''">
        <span style="font-size:14px;color:${STITCH.theme==='cyberpunk'?'var(--primary)':STITCH.theme==='ledger'?'var(--primary)':'var(--muted)'};">●</span>
        ${n.target}
        <span class="text-xs text-muted">(${n.type})</span>
      </div>
    `).join('') : '<p class="text-sm text-muted">No nodes found.</p>';

    document.getElementById('project-findings').innerHTML = findings.map(f => `
      <div class="flex items-start gap-3 p-3 card mb-2" style="cursor:pointer;" onclick="STITCH.navigate('#/insight/analyzer',{finding:${JSON.stringify(f).replace(/"/g,'&quot;')}})">
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
    `;
  } catch (e) {
    ['project-nodes','project-findings','project-meta'].forEach(id => {
      const el = document.getElementById(id);
      if (el) el.innerHTML = '<p class="text-muted text-sm">Failed to load.</p>';
    });
  }
  const path = STITCH.currentRoute;
  const match = path.match(/\/ledger\/([^\/]+)/);
  if (match) startFindingsPolling(match[1]);
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
              <button type="button" onclick="document.getElementById('poc-image-input').click()" style="position:absolute;bottom:8px;right:8px;background:var(--primary);color:var(--bg);border:none;border-radius:4px;padding:4px 10px;font-size:11px;font-weight:600;cursor:pointer;">+ Add Image</button>
              <input type="file" id="poc-image-input" accept="image/*" style="display:none;" onchange="insertPocImage(this)">
            </div>
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
              <label style="font-size:11px;color:var(--muted);font-weight:600;">Category</label>
              <select class="input w-full" id="finding-category" style="padding:4px 8px;">
                <option value="external">External Penetration Test</option>
                <option value="internal">Internal Penetration Test</option>
                <option value="web">Web Application Penetration Test</option>
                <option value="code_review">Code Review</option>
                <option value="architecture">Network Architecture Review</option>
                <option value="active_directory">Active Directory Assessment</option>
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
          <button class="btn btn-secondary" onclick="STITCH.navigate('#/ledger/project')">Discard</button>
          <button class="btn btn-primary" onclick="showToast('Finding saved as draft','success');STITCH.navigate('#/ledger/project')">Publish to Ledger</button>
        </div>
      </div>
    </div>
  `;
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
    const res = await api.get('/evidence');
    const items = res.data || [];
    const rows = document.getElementById('evidence-rows');
    if (items.length === 0) {
      rows.innerHTML = '<div class="p-4 text-sm text-muted text-center">No evidence uploaded.</div>';
      return;
    }
    rows.innerHTML = items.map(ev => `
      <div class="flex items-center px-4 py-3 border-b" style="cursor:pointer;" onmouseover="this.style.background='var(--surface-hover)'" onmouseout="this.style.background=''">
        <div style="flex:2;display:flex;align-items:center;gap:8px;">
          <span style="font-size:18px;">${ev.mime_type?.includes('image') ? '🖼️' : ev.mime_type?.includes('pcap') ? '📡' : '📄'}</span>
          <span class="font-mono" style="font-size:13px;">${ev.filename}</span>
        </div>
        <div style="flex:1;font-size:13px;">${ev.uploaded_by || 'Unknown'}</div>
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
  for (const file of files) {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('engagement_id', STITCH.currentEngagement?.id || 'eng_001');
    formData.append('tags', '#upload');
    try {
      await api.upload('/evidence/upload', formData);
      showToast(`Uploaded ${file.name}`, 'success');
    } catch (e) {
      showToast(`Failed to upload ${file.name}`, 'error');
    }
  }
  STITCH.render();
}

// -------- LEDGER: REPORT GENERATOR WIZARD --------
let reportWizardState = { step: 1, companyName: '', logo: '', projectName: '', sections: [], findings: [], format: 'docx' };

function renderReportGenerator() {
  reportWizardState = { step: 1, companyName: '', logo: '', projectName: '', sections: [], findings: [], format: 'docx' };
  return `
    <div style="max-width:900px;margin:0 auto;">
      <div class="flex items-center justify-between mb-6">
        <h2 class="font-display font-bold" style="font-size:22px;">Report Generation Wizard</h2>
        <div class="flex items-center gap-2">
          ${[1,2,3,4].map(s => `
            <div style="width:32px;height:32px;border-radius:50%;display:flex;align-items:center;justify-content:center;font-size:13px;font-weight:700;
              background:${s === 1 ? 'var(--primary)' : 'var(--surface-hover)'};color:${s === 1 ? 'var(--bg)' : 'var(--muted)'};"
              id="wizard-step-${s}">${s}</div>
            ${s < 4 ? '<div style="width:24px;height:2px;background:var(--border);"></div>' : ''}
          `).join('')}
        </div>
      </div>
      <div class="card" style="padding:32px;" id="report-wizard-content">
        ${renderReportStep1()}
      </div>
    </div>
  `;
}

function renderReportStep1() {
  return `
    <h3 class="font-display font-bold mb-6" style="font-size:18px;">Step 1: Company Info</h3>
    <div style="margin-bottom:20px;">
      <label style="font-size:13px;font-weight:600;display:block;margin-bottom:6px;">Company Name</label>
      <input class="input w-full" id="wizard-company-name" placeholder="e.g. Acme Corp" value="${reportWizardState.companyName}" oninput="reportWizardState.companyName=this.value">
    </div>
    <div style="margin-bottom:20px;">
      <label style="font-size:13px;font-weight:600;display:block;margin-bottom:6px;">Company Logo</label>
      <div class="flex items-center gap-4">
        <div style="width:80px;height:80px;border:2px dashed var(--border);border-radius:var(--radius);display:flex;align-items:center;justify-content:center;overflow:hidden;" id="wizard-logo-preview">
          ${reportWizardState.logo ? `<img src="${reportWizardState.logo}" style="width:100%;height:100%;object-fit:contain;">` : '<span style="color:var(--muted);font-size:24px;">+</span>'}
        </div>
        <div>
          <button class="btn btn-secondary" onclick="document.getElementById('wizard-logo-input').click()">Upload Logo</button>
          <input type="file" id="wizard-logo-input" accept="image/*" style="display:none;" onchange="uploadReportLogo(this)">
          <div class="text-xs text-muted mt-2">PNG, JPG up to 2MB</div>
        </div>
      </div>
    </div>
    <div style="margin-bottom:20px;">
      <label style="font-size:13px;font-weight:600;display:block;margin-bottom:6px;">Project Name</label>
      <input class="input w-full" id="wizard-project-name" placeholder="e.g. FAL-2025-001" value="${reportWizardState.projectName || STITCH.currentEngagement?.name || ''}" oninput="reportWizardState.projectName=this.value">
    </div>
    <div style="display:flex;justify-content:flex-end;margin-top:24px;">
      <button class="btn btn-primary" onclick="reportWizardNext()">Next Step →</button>
    </div>
  `;
}

function renderReportStep2() {
  const sectionOptions = [
    { id: 'ept', label: 'External Penetration Test (EPT)' },
    { id: 'wpt', label: 'Web Application Penetration Test (WPT)' },
    { id: 'nar', label: 'Network Architecture Review (NAR)' },
    { id: 'codereview', label: 'Code Review' },
    { id: 'ad', label: 'Active Directory Assessment' }
  ];
  return `
    <h3 class="font-display font-bold mb-6" style="font-size:18px;">Step 2: Report Sections</h3>
    <p class="text-sm text-muted mb-6">Select which sections to include in the report.</p>
    <div class="flex flex-col gap-3">
      ${sectionOptions.map(s => `
        <label class="flex items-center gap-3 p-4 card" style="cursor:pointer;${reportWizardState.sections.includes(s.id) ? 'border-color:var(--primary);' : ''}"
               onmouseover="this.style.borderColor='var(--primary)'" onmouseout="this.style.borderColor='${reportWizardState.sections.includes(s.id) ? 'var(--primary)' : 'var(--border)'}'">
          <input type="checkbox" value="${s.id}" ${reportWizardState.sections.includes(s.id) ? 'checked' : ''}
                 onchange="if(this.checked){reportWizardState.sections.push('${s.id}')}else{reportWizardState.sections=reportWizardState.sections.filter(x=>x!=='${s.id}')}">
          <span style="font-size:14px;">${s.label}</span>
        </label>
      `).join('')}
    </div>
    <div style="display:flex;justify-content:space-between;margin-top:24px;">
      <button class="btn btn-secondary" onclick="reportWizardBack()">← Back</button>
      <button class="btn btn-primary" onclick="reportWizardNext()">Next Step →</button>
    </div>
  `;
}

function renderReportStep3() {
  return `
    <h3 class="font-display font-bold mb-6" style="font-size:18px;">Step 3: Select Findings</h3>
    <p class="text-sm text-muted mb-4">Choose which findings to include in the report.</p>
    <div id="wizard-findings-list" style="max-height:400px;overflow-y:auto;">
      <div class="text-muted text-sm p-4">Loading findings...</div>
    </div>
    <div style="display:flex;justify-content:space-between;margin-top:24px;">
      <button class="btn btn-secondary" onclick="reportWizardBack()">← Back</button>
      <button class="btn btn-primary" onclick="reportWizardNext()">Next Step →</button>
    </div>
  `;
}

function renderReportStep4() {
  return `
    <h3 class="font-display font-bold mb-6" style="font-size:18px;">Step 4: Generate Report</h3>
    <div class="flex flex-col gap-4 mb-6">
      <div class="card p-4">
        <h4 class="font-semibold mb-2">Report Summary</h4>
        <div style="font-size:13px;color:var(--text-muted);">
          <div class="mb-1"><strong>Company:</strong> ${reportWizardState.companyName || 'Not specified'}</div>
          <div class="mb-1"><strong>Project:</strong> ${reportWizardState.projectName || 'Not specified'}</div>
          <div class="mb-1"><strong>Sections:</strong> ${reportWizardState.sections.length || 0} selected</div>
          <div><strong>Findings:</strong> ${reportWizardState.findings.length || 0} included</div>
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

function reportWizardNext() {
  if (reportWizardState.step === 1) {
    reportWizardState.companyName = document.getElementById('wizard-company-name')?.value || reportWizardState.companyName;
    reportWizardState.projectName = document.getElementById('wizard-project-name')?.value || reportWizardState.projectName;
  }
  if (reportWizardState.step < 4) {
    reportWizardState.step++;
  }
  renderReportWizardStep();
}

function reportWizardBack() {
  if (reportWizardState.step > 1) {
    reportWizardState.step--;
  }
  renderReportWizardStep();
}

function renderReportWizardStep() {
  const container = document.getElementById('report-wizard-content');
  if (!container) return;
  const stepFns = { 1: renderReportStep1, 2: renderReportStep2, 3: renderReportStep3, 4: renderReportStep4 };
  container.innerHTML = stepFns[reportWizardState.step]();
  [1,2,3,4].forEach(s => {
    const el = document.getElementById(`wizard-step-${s}`);
    if (el) {
      el.style.background = s === reportWizardState.step ? 'var(--primary)' : s < reportWizardState.step ? 'var(--primary)' : 'var(--surface-hover)';
      el.style.color = s <= reportWizardState.step ? 'var(--bg)' : 'var(--muted)';
    }
  });
  if (reportWizardState.step === 3) loadWizardFindings();
}

function loadWizardFindings() {
  const engId = STITCH.currentEngagement?.id || 'eng_001';
  const container = document.getElementById('wizard-findings-list');
  if (!container) return;
  api.get(`/engagements/${engId}/findings`).then(res => {
    const findings = res.data || [];
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
        ${items.map(f => `
          <label class="flex items-center gap-3 p-3" style="cursor:pointer;border-bottom:1px solid var(--border);" onmouseover="this.style.background='var(--surface-hover)'" onmouseout="this.style.background=''">
            <input type="checkbox" value="${f.id}" ${reportWizardState.findings.includes(f.id) ? 'checked' : ''}
                   onchange="if(this.checked){reportWizardState.findings.push('${f.id}')}else{reportWizardState.findings=reportWizardState.findings.filter(x=>x!=='${f.id}')}">
            <div class="flex-1">
              <div class="font-semibold" style="font-size:13px;">${f.title}</div>
              <div class="text-xs text-muted font-mono">CVSS: ${f.cvss_score || 'N/A'} ${f.cve ? '| ' + f.cve : ''}</div>
            </div>
          </label>
        `).join('')}
      </div>
    `).join('');
  }).catch(() => {
    container.innerHTML = '<p class="text-muted text-sm p-4">Failed to load findings.</p>';
  });
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

async function generateReportDocument() {
  showToast('Compiling report document...', 'info');
  try {
    const res = await api.post('/reports/export', {
      engagement_id: STITCH.currentEngagement?.id || 'eng_001',
      company_name: reportWizardState.companyName,
      project_name: reportWizardState.projectName,
      logo: reportWizardState.logo,
      sections: reportWizardState.sections,
      finding_ids: reportWizardState.findings,
      format: reportWizardState.format
    });
    const filename = res.data?.filename;
    if (filename) {
      downloadReport(filename);
    } else {
      showToast('Report generated', 'success');
    }
    const preview = document.getElementById('report-preview');
    if (preview) {
      preview.innerHTML = `
        <div style="text-align:center;padding:40px 0;">
          <div style="font-size:32px;margin-bottom:8px;">✅</div>
          <p class="font-semibold">Report generated successfully!</p>
          <button class="btn btn-primary mt-4" onclick="downloadReport('${filename || ''}')">⬇ Download Report</button>
        </div>
      `;
    }
  } catch (e) {
    showToast('Report generation failed: ' + (e.message || 'Unknown error'), 'error');
  }
}

function downloadReport(filename) {
  if (!filename) return;
  window.open(`/api/v1/reports/download/${filename}`, '_blank');
}

// -------- INSIGHT: COMMAND DASHBOARD --------
function renderInsightDashboard() {
  return `
    <div class="dashboard-grid" style="grid-template-columns:repeat(3,1fr);">
      <div class="metric-widget">
        <div class="label">Total Findings</div>
        <div class="value" id="insight-total">—</div>
      </div>
      <div class="metric-widget">
        <div class="label">Critical</div>
        <div class="value" style="color:var(--critical);" id="insight-critical">—</div>
      </div>
      <div class="metric-widget">
        <div class="label">Open</div>
        <div class="value" style="color:var(--primary);" id="insight-open">—</div>
      </div>
    </div>
    <div class="flex gap-4 mt-6" style="height:400px;">
      <div class="card flex-1 p-4">
        <h4 class="font-display font-bold mb-4">Findings Over Time</h4>
        <div id="insight-chart" style="height:300px;">
          <div class="skeleton" style="height:300px;"></div>
        </div>
      </div>
      <div class="card" style="width:350px;padding:16px;">
        <h4 class="font-display font-bold mb-4">Top Assets</h4>
        <div id="insight-assets">
          <div class="skeleton" style="height:40px;margin-bottom:8px;"></div>
          <div class="skeleton" style="height:40px;"></div>
        </div>
      </div>
    </div>
  `;
}

async function afterRenderInsightDashboard() {
  try {
    const [sevRes, statusRes, chartRes, assetsRes] = await Promise.all([
      api.get('/analytics/severity-count'),
      api.get('/analytics/status-count'),
      api.get('/analytics/findings-over-time'),
      api.get('/analytics/top-assets')
    ]);
    const sev = sevRes.data || {};
    const status = statusRes.data || {};
    const chart = chartRes.data || [];
    const assets = assetsRes.data || [];

    const total = Object.values(sev).reduce((a,b) => a+b, 0);
    document.getElementById('insight-total').textContent = total;
    document.getElementById('insight-critical').textContent = sev.critical || 0;
    document.getElementById('insight-open').textContent = (status.open || 0) + (status.draft || 0);

    // Simple chart bars
    const chartContainer = document.getElementById('insight-chart');
    const maxVal = Math.max(...chart.flatMap(m => [m.critical, m.high, m.medium, m.low]), 1);
    chartContainer.innerHTML = `
      <div style="display:flex;align-items:flex-end;gap:8px;height:250px;padding-top:20px;">
        ${chart.map(m => `
          <div style="flex:1;display:flex;flex-direction:column;align-items:center;gap:4px;">
            <div style="width:100%;display:flex;flex-direction:column;align-items:center;gap:2px;justify-content:flex-end;height:220px;">
              <div style="width:60%;background:var(--critical);border-radius:2px;height:${(m.critical/maxVal)*180}px;min-height:4px;" title="Critical: ${m.critical}"></div>
              <div style="width:60%;background:var(--warning);border-radius:2px;height:${(m.high/maxVal)*180}px;min-height:4px;" title="High: ${m.high}"></div>
              <div style="width:60%;background:var(--primary);border-radius:2px;height:${(m.medium/maxVal)*180}px;min-height:4px;" title="Medium: ${m.medium}"></div>
              <div style="width:60%;background:var(--muted);border-radius:2px;height:${(m.low/maxVal)*180}px;min-height:4px;" title="Low: ${m.low}"></div>
            </div>
            <span class="text-xs text-muted font-mono">${m.month?.slice(5) || ''}</span>
          </div>
        `).join('')}
      </div>
      <div class="flex gap-4 mt-2">
        <span class="text-xs flex items-center gap-1"><span style="width:8px;height:8px;border-radius:2px;background:var(--critical);display:inline-block;"></span> Critical</span>
        <span class="text-xs flex items-center gap-1"><span style="width:8px;height:8px;border-radius:2px;background:var(--warning);display:inline-block;"></span> High</span>
        <span class="text-xs flex items-center gap-1"><span style="width:8px;height:8px;border-radius:2px;background:var(--primary);display:inline-block;"></span> Medium</span>
        <span class="text-xs flex items-center gap-1"><span style="width:8px;height:8px;border-radius:2px;background:var(--muted);display:inline-block;"></span> Low</span>
      </div>
    `;

    document.getElementById('insight-assets').innerHTML = assets.map(a => `
      <div class="flex items-center justify-between py-2 border-b" style="font-size:13px;">
        <span class="font-mono" style="font-size:12px;">${a.target || 'Unknown'}</span>
        <span style="color:var(--critical);font-family:var(--font-mono);font-weight:700;">${a.count} findings</span>
      </div>
    `).join('') || '<p class="text-sm text-muted">No data</p>';
  } catch (e) {
    ['insight-total','insight-critical','insight-open'].forEach(id => {
      const el = document.getElementById(id);
      if (el) el.textContent = '—';
    });
  }
}

// -------- INSIGHT: AUDIT PROJECTS --------
function renderAuditProjects() {
  return `
    <div class="flex items-center gap-4 mb-6">
      <input class="input" style="flex:1;" placeholder="Search projects..." oninput="filterAuditProjects(this.value)">
      <button class="btn btn-primary" onclick="showNewEngagementModal()">+ New Audit</button>
    </div>
    <div class="card" id="audit-projects-table">
      <table class="table">
        <thead>
          <tr>
            <th>Project ID</th>
            <th>Client</th>
            <th>Methodology</th>
            <th>Status</th>
            <th>Targets</th>
            <th>Due</th>
            <th></th>
          </tr>
        </thead>
        <tbody id="audit-projects-body">
          <tr><td colspan="7" class="text-center text-muted">Loading...</td></tr>
        </tbody>
      </table>
    </div>
  `;
}

async function afterRenderAuditProjects() {
  try {
    const res = await api.get('/engagements');
    const engagements = res.data || [];
    const tbody = document.getElementById('audit-projects-body');
    if (engagements.length === 0) {
      tbody.innerHTML = '<tr><td colspan="7" class="text-center text-muted">No projects found.</td></tr>';
      return;
    }
    tbody.innerHTML = engagements.map(e => `
      <tr onclick="STITCH.navigate('#/ledger/project',{engagement:${JSON.stringify(e).replace(/"/g,'&quot;')}})">
        <td class="font-mono" style="font-size:12px;">${e.id?.slice(0,8)}</td>
        <td>${e.client_name || 'N/A'}</td>
        <td><span class="font-mono text-xs">${(e.methodology || 'N/A').toUpperCase()}</span></td>
        <td><span class="status-pill ${e.status}">${e.status?.replace('_',' ')}</span></td>
        <td>${e.scope?.included?.length || 0}</td>
        <td class="font-mono text-xs">${e.timeline?.end_date || 'N/A'}</td>
        <td><span style="color:var(--muted);">…</span></td>
      </tr>
    `).join('');
  } catch (e) {
    document.getElementById('audit-projects-body').innerHTML = '<tr><td colspan="7" class="text-center text-muted">Failed to load.</td></tr>';
  }
}

function filterAuditProjects(val) {
  const rows = document.querySelectorAll('#audit-projects-body tr');
  rows.forEach(r => {
    r.style.display = r.textContent.toLowerCase().includes(val.toLowerCase()) ? '' : 'none';
  });
}

// -------- INSIGHT: FINDING ANALYZER --------
function renderFindingAnalyzer() {
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

async function afterRenderFindingAnalyzer() {
  try {
    const engId = STITCH.currentEngagement?.id || 'eng_001';
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
        <div style="border-bottom:1px solid var(--border);margin-bottom:24px;">
          <div class="flex gap-6">
            <button class="pb-3" style="border-bottom:2px solid var(--primary);color:var(--primary);font-size:13px;font-weight:600;background:none;border-top:none;border-left:none;border-right:none;cursor:pointer;">Evidence & Details</button>
            <button class="pb-3" style="border-bottom:2px solid transparent;color:var(--muted);font-size:13px;background:none;border-top:none;border-left:none;border-right:none;cursor:pointer;" onclick="showToast('Remediation tab','info')">Remediation</button>
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
          <div id="poc-content" style="background:${STITCH.theme==='insight'?'#0B1120':'var(--bg)'};padding:16px;border-radius:var(--radius);font-size:13px;line-height:1.7;"></div>
        </div>` : ''}
      </div>
    `;
    // Set POC content as innerHTML to render images
    const pocDiv = document.getElementById('poc-content');
    if (pocDiv && f.poc) {
      pocDiv.innerHTML = f.poc;
    }
  } catch (e) {
    document.getElementById('analyzer-detail').innerHTML = '<p class="text-muted">Error loading finding.</p>';
  }
}

// -------- INSIGHT: INSIGHT GENERATOR --------
function renderInsightGenerator() {
  return `
    <div class="document-container" style="padding-bottom:80px;">
      <div class="card" style="padding:32px;">
        <input class="font-display font-bold" style="font-size:28px;width:100%;border:none;background:transparent;color:var(--text);outline:none;" value="Systemic Risk Analysis: Q3" placeholder="Report Title...">
        <div class="flex items-center gap-6 text-sm text-muted font-mono mt-2 mb-8">
          <span>Generated: ${new Date().toLocaleDateString()}</span>
          <span>Author: ${STITCH.user?.name || 'Analyst'}</span>
        </div>
        <div class="block-wrapper mb-6" style="position:relative;">
          <div class="flex items-start gap-3">
            <div class="font-display font-bold" style="font-size:20px;color:var(--primary);flex-shrink:0;">Executive Summary</div>
            <div style="line-height:1.7;">Analysis of recent telemetry indicates a systemic risk posture centered around legacy authentication mechanisms. The concentration of critical vulnerabilities suggests an immediate need for zero-trust architecture overhaul.</div>
          </div>
        </div>
        <div class="block-wrapper mb-6" style="position:relative;">
          <h4 class="font-display font-bold mb-4">Authentication Bypass Frequency</h4>
          <div style="height:200px;background:var(--bg);border-radius:var(--radius);padding:24px;display:flex;align-items:flex-end;gap:16px;">
            ${['May','Jun','Jul','Aug','Sep'].map((m,i) => `
              <div style="flex:1;display:flex;flex-direction:column;align-items:center;gap:8px;">
                <div style="width:60%;background:${i===3?'var(--critical)':'var(--primary)'};border-radius:4px;height:${[40,60,100,180,80][i]}px;opacity:0.8;"></div>
                <span class="text-xs text-muted font-mono">${m}</span>
              </div>
            `).join('')}
          </div>
        </div>
        <div style="text-align:center;margin-top:24px;">
          <button class="btn btn-secondary" onclick="showToast('Add block','info')">+ Add Insight Block</button>
        </div>
      </div>
      <div style="position:fixed;bottom:0;left:0;right:0;background:var(--surface);border-top:1px solid var(--border);padding:16px 24px;display:flex;justify-content:center;gap:12px;">
        <button class="btn btn-secondary" onclick="showToast('Exporting PDF...','info')">Export PDF</button>
        <button class="btn btn-secondary" onclick="showToast('Exporting DOCX...','info')">Export DOCX</button>
        <button class="btn btn-primary" onclick="showToast('Draft saved','success')">Save Draft</button>
      </div>
    </div>
  `;
}

// -------- COMMAND CENTER: GLOBAL THREAT DASHBOARD --------
function renderGlobalThreatDashboard() {
  return `
    <div class="dashboard-grid" style="grid-template-columns:repeat(4,1fr);">
      <div class="metric-widget" style="border-top:2px solid var(--primary);">
        <div class="label">Infrastructure Health</div>
        <div class="value" style="color:var(--primary);font-size:28px;">98.2%</div>
        <div class="text-xs text-muted">24 services monitored</div>
      </div>
      <div class="metric-widget" style="border-top:2px solid var(--critical);">
        <div class="label">Critical Alerts</div>
        <div class="value" style="color:var(--critical);" id="cmd-critical">—</div>
        <div class="text-xs text-muted">Requires immediate action</div>
      </div>
      <div class="metric-widget" style="border-top:2px solid var(--warning);">
        <div class="label">Active Engagements</div>
        <div class="value" id="cmd-engagements">—</div>
        <div class="text-xs text-muted">In progress</div>
      </div>
      <div class="metric-widget" style="border-top:2px solid var(--primary);">
        <div class="label">Total Findings</div>
        <div class="value" id="cmd-findings">—</div>
        <div class="text-xs text-muted">Across all projects</div>
      </div>
    </div>
    <div class="flex gap-4 mt-6">
      <div class="card flex-1 p-4" style="height:300px;">
        <h4 class="font-display font-bold mb-4" style="color:var(--critical);">⚠ Critical Alerts Feed</h4>
        <div id="cmd-alerts">
          <div class="flex items-center gap-3 p-3 mb-2" style="border:1px solid var(--critical);background:color-mix(in srgb, var(--critical) 10%, transparent);">
            <span class="badge-severity critical">CRIT</span>
            <div class="flex-1">
              <div class="font-semibold" style="font-size:13px;">SQL Injection in Auth Endpoint</div>
              <div class="text-xs text-muted font-mono">Source: 10.0.0.15 • 2 min ago</div>
            </div>
          </div>
          <div class="flex items-center gap-3 p-3 mb-2" style="border:1px solid var(--critical);background:color-mix(in srgb, var(--critical) 10%, transparent);">
            <span class="badge-severity critical">CRIT</span>
            <div class="flex-1">
              <div class="font-semibold" style="font-size:13px;">RCE via File Upload</div>
              <div class="text-xs text-muted font-mono">Source: api.apex.com • 15 min ago</div>
            </div>
          </div>
        </div>
      </div>
      <div class="card p-4" style="width:350px;">
        <h4 class="font-display font-bold mb-4">System Status</h4>
        <div class="gauge-ring mx-auto mb-4">
          <svg viewBox="0 0 120 120">
            <circle cx="60" cy="60" r="54" fill="none" stroke="var(--muted)" stroke-width="6" opacity="0.3"/>
            <circle cx="60" cy="60" r="54" fill="none" stroke="var(--primary)" stroke-width="6" stroke-dasharray="339.292" stroke-dashoffset="30" stroke-linecap="round"/>
          </svg>
        </div>
        <div style="text-align:center;">
          <div class="font-display font-bold" style="font-size:24px;color:var(--primary);">Operational</div>
          <div class="text-xs text-muted mt-2">All systems nominal</div>
        </div>
      </div>
    </div>
  `;
}

async function afterRenderGlobalThreatDashboard() {
  try {
    const [engRes, sevRes] = await Promise.all([
      api.get('/engagements'),
      api.get('/analytics/global-severity')
    ]);
    const engagements = engRes.data || [];
    const severity = sevRes.data || {};
    document.getElementById('cmd-engagements').textContent = engagements.filter(e => e.status !== 'closed').length;
    document.getElementById('cmd-findings').textContent = Object.values(severity).reduce((a,b) => a+b, 0);
    document.getElementById('cmd-critical').textContent = severity.critical || 0;
  } catch (e) {
    // Fallback to static
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
        <button class="btn btn-primary" onclick="STITCH.navigate('#/command/report-builder',{engagement:${JSON.stringify(eng).replace(/"/g,'&quot;')}})">Build Report</button>
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
  try {
    const engId = STITCH.currentEngagement?.id || 'eng_001';
    const res = await api.get(`/engagements/${engId}/findings`);
    const findings = res.data || [];

    const tbody = document.getElementById('vuln-feed-body');
    if (findings.length === 0) {
      tbody.innerHTML = '<tr><td colspan="8" class="text-center text-muted">Zero findings matching current parameters.</td></tr>';
      return;
    }
    tbody.innerHTML = findings.map(f => `
      <tr class="${f.severity === 'critical' ? 'vuln-critical' : ''}" onclick="STITCH.navigate('#/insight/analyzer',{finding:${JSON.stringify(f).replace(/"/g,'&quot;')}})"
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
            <h1 class="font-display font-bold" style="font-size:28px;">${STITCH.currentEngagement?.name || 'Project X'} — Assessment Report</h1>
          </div>
          <h2 class="font-display font-bold" style="font-size:20px;color:var(--primary);margin-bottom:16px;">Executive Summary</h2>
          <p style="margin-bottom:24px;line-height:1.7;">This report outlines findings from the penetration test. ${STITCH.currentEngagement?.client_name || 'The assessment'} revealed critical vulnerabilities requiring immediate remediation.</p>
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
        <div class="text-sm text-muted mb-2">Author: ${STITCH.user?.name || 'System'}</div>
        <div class="text-sm text-muted font-mono mb-2">Date: ${new Date().toLocaleDateString()}</div>
        <div class="text-sm text-muted font-mono">Version: 1.0.0-draft</div>
        <hr style="border:none;border-top:1px solid var(--border);margin:16px 0;">
        <button class="btn btn-primary w-full" onclick="showToast('Exporting report...','success');this.querySelector('span')&&(this.querySelector('span').textContent='COMPILING...')">Export PDF</button>
      </div>
    </div>
  `;
}

async function afterRenderCommandReportBuilder() {
  try {
    const engId = STITCH.currentEngagement?.id || 'eng_001';
    const res = await api.get(`/engagements/${engId}/findings`);
    const findings = res.data || [];
    const container = document.getElementById('builder-findings');
    if (!container) return;
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

// -------- COMMAND CENTER: GLOBAL THREAT FEED --------
function renderGlobalThreatFeed() {
  return `
    <div style="display:flex;height:calc(100vh - 64px - 48px);margin:-24px;">
      <div style="width:220px;border-right:1px solid var(--border);padding:16px;overflow-y:auto;">
        <h4 class="font-display font-bold text-sm mb-4">Filters</h4>
        <div class="mb-4">
          <label style="font-size:12px;font-weight:600;display:block;margin-bottom:6px;">Severity</label>
          <label class="flex items-center gap-2 text-sm mb-2"><input type="checkbox" id="tf-sev-critical" checked> <span style="color:var(--critical);">Critical</span></label>
          <label class="flex items-center gap-2 text-sm mb-2"><input type="checkbox" id="tf-sev-high" checked> <span style="color:var(--warning);">High</span></label>
          <label class="flex items-center gap-2 text-sm mb-2"><input type="checkbox" id="tf-sev-medium" checked> Medium</label>
          <label class="flex items-center gap-2 text-sm"><input type="checkbox" id="tf-sev-low"> Low</label>
        </div>
        <div class="mb-4">
          <label style="font-size:12px;font-weight:600;display:block;margin-bottom:6px;">Source</label>
          <select class="input w-full" id="tf-source-filter" style="padding:4px 8px;font-size:12px;" onchange="filterThreatFeed()">
            <option value="">All Sources</option>
            <option value="cve">CVE/NVD</option>
            <option value="osint">OSINT</option>
            <option value="internal">Internal</option>
            <option value="ti-feed">TI Feed</option>
          </select>
        </div>
        <button class="btn btn-secondary w-full" style="font-size:12px;" onclick="afterRenderGlobalThreatFeed()">↻ Refresh</button>
      </div>
      <div style="flex-1;display:flex;flex-direction:column;overflow:hidden;">
        <div style="padding:12px 20px;border-bottom:1px solid var(--border);display:flex;align-items:center;justify-content:space-between;background:var(--surface);">
          <div class="flex items-center gap-3">
            <h3 class="font-display font-bold" style="font-size:16px;">Live Threat Intelligence</h3>
            <span style="display:inline-flex;align-items:center;gap:4px;font-size:11px;font-family:var(--font-mono);color:var(--primary);">
              <span style="width:6px;height:6px;border-radius:50%;background:var(--primary);animation:pulseCritical 2s infinite;"></span>
              LIVE
            </span>
          </div>
          <div class="text-xs text-muted font-mono" id="tf-refresh-indicator">Auto-refresh: 60s</div>
        </div>
        <div style="flex:1;overflow:auto;">
          <table class="table" id="threat-feed-table">
            <thead style="position:sticky;top:0;z-index:10;">
              <tr>
                <th style="width:90px;">Severity</th>
                <th>Title</th>
                <th style="width:120px;">Source</th>
                <th style="width:130px;">Category</th>
                <th style="width:130px;">Published</th>
                <th style="width:80px;"></th>
              </tr>
            </thead>
            <tbody id="threat-feed-body">
              <tr><td colspan="6" class="text-center text-muted">Loading threat intelligence...</td></tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  `;
}

let threatFeedInterval = null;

async function afterRenderGlobalThreatFeed() {
  if (threatFeedInterval) clearInterval(threatFeedInterval);

  async function fetchThreats() {
    try {
      const res = await api.get('/threat-feed');
      const threats = res.data || [];
      const tbody = document.getElementById('threat-feed-body');
      if (!tbody) return;

      const sevCritical = document.getElementById('tf-sev-critical')?.checked;
      const sevHigh = document.getElementById('tf-sev-high')?.checked;
      const sevMedium = document.getElementById('tf-sev-medium')?.checked;
      const sevLow = document.getElementById('tf-sev-low')?.checked;
      const sourceFilter = document.getElementById('tf-source-filter')?.value || '';

      const filtered = threats.filter(t => {
        const s = (t.severity || '').toLowerCase();
        if (s === 'critical' && !sevCritical) return false;
        if (s === 'high' && !sevHigh) return false;
        if (s === 'medium' && !sevMedium) return false;
        if (s === 'low' && !sevLow) return false;
        if (sourceFilter && (t.source || '').toLowerCase() !== sourceFilter) return false;
        return true;
      });

      if (filtered.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">No threats match current filters.</td></tr>';
        return;
      }

      tbody.innerHTML = filtered.map(t => `
        <tr style="${t.severity === 'critical' ? 'animation: pulseCritical 2s infinite;' : ''}">
          <td><span class="badge-severity ${t.severity}" style="display:block;text-align:center;">[${(t.severity || 'N/A').toUpperCase().slice(0,4)}]</span></td>
          <td class="font-semibold" style="font-size:13px;">${t.title || 'Untitled'}</td>
          <td class="font-mono text-xs">${t.source || 'Unknown'}</td>
          <td class="text-xs">${t.category || '—'}</td>
          <td class="font-mono text-xs">${t.published ? new Date(t.published).toLocaleDateString() : '—'}</td>
          <td><button class="btn btn-ghost" style="font-size:11px;padding:2px 6px;" onclick="showToast('Threat details: ${t.id}','info')">View</button></td>
        </tr>
      `).join('');

      const indicator = document.getElementById('tf-refresh-indicator');
      if (indicator) indicator.textContent = `Updated: ${new Date().toLocaleTimeString()} | ${filtered.length} items`;
    } catch (e) {
      const tbody = document.getElementById('threat-feed-body');
      if (tbody) tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">Failed to load threat feed.</td></tr>';
    }
  }

  await fetchThreats();
  threatFeedInterval = setInterval(fetchThreats, 60000);
}

function filterThreatFeed() {
  afterRenderGlobalThreatFeed();
}

// -------- BULK IMPORT FINDINGS --------
let bulkImportEngagementId = null;

function showBulkImportModal() {
  const path = STITCH.currentRoute;
  const match = path.match(/\/ledger\/([^\/]+)/);
  if (!match) { showToast('No engagement selected', 'error'); return; }
  bulkImportEngagementId = match[1];
  
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
    STITCH.render();
  } catch (e) {
    showToast('Import failed: ' + e.message, 'error');
  }
}

// -------- LIVE FINDINGS POLLING --------
let findingsPollInterval = null;
let lastFindingsCheck = null;

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
        const path = STITCH.currentRoute;
        if (path.includes('/ledger/' + engagementId)) {
          STITCH.render();
        }
      }
    } catch (e) {
      // Silently fail - polling should be resilient
    }
  }, 15000); // Poll every 15 seconds
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

function showNewEngagementModal() {
  const name = prompt('Engagement name:');
  if (!name) return;
  const client = prompt('Client name:');
  api.post('/engagements', { name, client_name: client, status: 'planning', methodology: 'owasp', scope: {included:[],excluded:[]}, timeline: {start_date:new Date().toISOString().slice(0,10),end_date:''}, team: [] })
    .then(() => { showToast('Engagement created','success'); STITCH.render(); })
    .catch(e => showToast(e.message,'error'));
}

function insertPocImage(input) {
  const file = input.files && input.files[0];
  if (!file) return;
  if (file.size > 5 * 1024 * 1024) {
    showToast('Image too large (max 5MB)', 'error');
    input.value = '';
    return;
  }
  const reader = new FileReader();
  reader.onload = function(e) {
    const ta = document.getElementById('finding-poc');
    const imgTag = `\n\n<img src="${e.target.result}" alt="POC Screenshot" style="max-width:100%;border:1px solid var(--border);border-radius:4px;">\n`;
    const start = ta.selectionStart;
    const end = ta.selectionEnd;
    ta.value = ta.value.substring(0, start) + imgTag + ta.value.substring(end);
    ta.selectionStart = ta.selectionEnd = start + imgTag.length;
    ta.focus();
    showToast('Image attached to POC', 'success');
  };
  reader.readAsDataURL(file);
  input.value = '';
}

// -------- ADMIN: USER MANAGEMENT --------
function renderUserManagement() {
  return `
    <div>
      <div class="flex items-center justify-between mb-6">
        <h3 class="font-display font-bold">Team Members</h3>
        <button class="btn btn-primary" onclick="showInviteUserModal()">+ Invite User</button>
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
        <td><span class="status-pill ${u.role === 'admin' ? 'critical' : u.role === 'analyst' ? 'in_progress' : 'draft'}">${u.role}</span></td>
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

function showInviteUserModal() {
  const name = prompt('Full name:');
  if (!name) return;
  const email = prompt('Email (must be @cyberteq.io):');
  if (!email) return;
  if (!email.endsWith('@cyberteq.io')) {
    showToast('Only @cyberteq.io emails are allowed', 'error');
    return;
  }
  const role = prompt('Role (admin, analyst, or user):');
  if (!role || !['admin','analyst','user'].includes(role)) {
    showToast('Invalid role', 'error');
    return;
  }
  api.post('/users', { name, email, role })
    .then(() => { showToast('User invited successfully', 'success'); STITCH.render(); })
    .catch(e => showToast(e.message, 'error'));
}

async function deleteUser(userId) {
  if (!confirm('Remove this user?')) return;
  try {
    await api.del('/users/' + userId);
    showToast('User removed', 'success');
    STITCH.render();
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
    case path === '/insight/dashboard':
      setTimeout(afterRenderInsightDashboard, 50);
      break;
    case path === '/insight/projects':
      setTimeout(afterRenderAuditProjects, 50);
      break;
    case path === '/insight/analyzer':
      setTimeout(afterRenderFindingAnalyzer, 50);
      break;
    case path === '/command/dashboard':
      setTimeout(afterRenderGlobalThreatDashboard, 50);
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
    case path === '/command/threat-feed':
      setTimeout(afterRenderGlobalThreatFeed, 50);
      break;
    case path === '/admin/users':
      setTimeout(afterRenderUserManagement, 50);
      break;
    case path === '/reports':
    case path === '/report-generator':
      break;
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
