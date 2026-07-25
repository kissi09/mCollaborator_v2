// STITCH Global State Management
let stitchToken, stitchTheme;
try {
  stitchToken = localStorage.getItem('stitch_token');
  stitchTheme = localStorage.getItem('stitch_theme') || 'cyberpunk';
} catch (e) {
  stitchToken = null;
  stitchTheme = 'cyberpunk';
}

const STITCH = {
  user: null,
  token: stitchToken,
  theme: stitchTheme,
  currentRoute: '',
  currentEngagement: null,
  currentFinding: null,
  currentNode: null,
  currentReport: null,

  async init() {
    if (this.token) {
      try {
        const res = await api.get('/users/me');
        this.user = res.data;
        this.applyTheme();
      } catch (e) {
        this.token = null;
        localStorage.removeItem('stitch_token');
      }
    }
    this.initRouter();
  },

  saveToken(token) {
    this.token = token;
    localStorage.setItem('stitch_token', token);
  },

  logout() {
    this.token = null;
    this.user = null;
    localStorage.removeItem('stitch_token');
    api.post('/auth/logout').catch(() => {});
    window.location.hash = '#/login';
    this.render();
  },

  setTheme(theme) {
    this.theme = theme;
    localStorage.setItem('stitch_theme', theme);
    this.applyTheme();
  },

  applyTheme() {
    document.documentElement.setAttribute('data-theme', this.theme);
  },

  navigate(route, data) {
    if (data) {
      if (data.engagement) this.currentEngagement = data.engagement;
      if (data.finding) this.currentFinding = data.finding;
      if (data.node) this.currentNode = data.node;
      if (data.report) this.currentReport = data.report;
    }
    window.location.hash = route;
  },

  getRouteParams() {
    const hash = window.location.hash.slice(1) || '/login';
    const parts = hash.split('/').filter(Boolean);
    return { path: hash, parts };
  },

  initRouter() {
    const handleRoute = () => {
      const path = window.location.hash.slice(1) || '/login';
      this.currentRoute = path;
      this.render();
    };
    window.addEventListener('hashchange', handleRoute);
    handleRoute();
  },

  render() {
    const path = this.currentRoute;
    const parts = path.split('/').filter(Boolean);
    const main = document.getElementById('app');

    if (!this.token && path !== '/login') {
      window.location.hash = '#/login';
      return;
    }

    let content = '';
    
    // Auth gate
    if (path === '/login') {
      content = renderLogin();
      main.innerHTML = `<div class="login-page">${content}</div>`;
      return;
    }

    // Determine sidebar type
    let sidebarType = 'cyberpunk'; // default
    if (path.startsWith('/ledger') || path.startsWith('/finding-editor') || path.startsWith('/evidence') || path.startsWith('/report-generator')) {
      sidebarType = 'ledger';
    } else if (path.startsWith('/insight')) {
      sidebarType = 'insight';
    }

    const sidebar = renderSidebar(sidebarType, path);
    
    let pageContent = '';
    switch (true) {
      case path === '/dashboard':
      case path === '/':
        pageContent = renderLedgerDashboard();
        break;
      case path === '/ledger/project':
        pageContent = renderProjectLedger();
        break;
      case path === '/finding-editor':
        pageContent = renderFindingEditor();
        break;
      case path === '/evidence':
        pageContent = renderEvidenceVault();
        break;
      case path === '/report-generator':
      case path === '/reports':
        pageContent = renderReportGenerator();
        break;
      case path === '/insight/dashboard':
        pageContent = renderInsightDashboard();
        break;
      case path === '/insight/projects':
        pageContent = renderAuditProjects();
        break;
      case path === '/insight/analyzer':
        pageContent = renderFindingAnalyzer();
        break;
      case path === '/insight/generator':
        pageContent = renderInsightGenerator();
        break;
      case path === '/command/dashboard':
        pageContent = renderGlobalThreatDashboard();
        break;
      case path === '/command/engagements':
        pageContent = renderActiveEngagements();
        break;
      case path === '/command/feed':
        pageContent = renderVulnerabilityFeed();
        break;
      case path === '/command/report-builder':
        pageContent = renderCommandReportBuilder();
        break;
      default:
        pageContent = renderLedgerDashboard();
    }

    main.innerHTML = `
      <div class="app-shell">
        ${sidebar}
        <div class="main-area">
          <div class="topbar">
            <div class="flex items-center gap-3">
              <h2 class="font-display text-lg font-bold">${getPageTitle(path)}</h2>
            </div>
            <div class="flex items-center gap-4">
              <select class="input" style="width:auto;padding:4px 8px;font-size:12px;" onchange="STITCH.setTheme(this.value)">
                <option value="cyberpunk" ${STITCH.theme === 'cyberpunk' ? 'selected' : ''}>Cyberpunk</option>
                <option value="ledger" ${STITCH.theme === 'ledger' ? 'selected' : ''}>Ledger</option>
                <option value="insight" ${STITCH.theme === 'insight' ? 'selected' : ''}>Insight</option>
              </select>
              <span class="text-sm text-muted">${STITCH.user?.name || ''}</span>
              <button class="btn btn-ghost text-sm" onclick="STITCH.logout()">Logout</button>
            </div>
          </div>
          <div class="content">
            ${pageContent}
          </div>
        </div>
      </div>
    `;
    
    // Wire up any dynamic interactions
    afterRender(path);
  }
};

function getPageTitle(path) {
  const titles = {
    '/dashboard': 'Dashboard',
    '/': 'Dashboard',
    '/ledger/project': 'Project Ledger',
    '/finding-editor': 'Finding Editor',
    '/evidence': 'Evidence Vault',
    '/report-generator': 'Report Generator',
    '/reports': 'Report Generator',
    '/insight/dashboard': 'Command Dashboard',
    '/insight/projects': 'Audit Projects',
    '/insight/analyzer': 'Finding Analyzer',
    '/insight/generator': 'Insight Generator',
    '/command/dashboard': 'Global Threat Dashboard',
    '/command/engagements': 'Active Engagements',
    '/command/feed': 'Vulnerability Feed',
    '/command/report-builder': 'Report Builder',
    '/login': 'Login',
  };
  return titles[path] || 'STITCH';
}
