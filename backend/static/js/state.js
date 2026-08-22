function sanitizeInput(str) {
  return str.replace(/[<>"'&]/g, function(c) {
    return {'<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;','&':'&amp;'}[c];
  });
}

function isXSSAttempt(str) {
  const patterns = /<script|javascript:|on\w+\s*=|<iframe|<object|<embed|<form|eval\(|document\.|window\./i;
  return patterns.test(str);
}

// Any email domain is accepted — we only check the address is well formed.
function validateEmail(email) {
  if (!email) return false;
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim());
}

// mCollaborator Global State Management
let stitchToken, stitchTheme;
try {
  stitchToken = localStorage.getItem('stitch_token');
  stitchTheme = localStorage.getItem('stitch_theme') || 'insight';
} catch (e) {
  stitchToken = null;
  stitchTheme = 'insight';
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
    if (typeof stopFindingsPolling === 'function') stopFindingsPolling();
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

    // First-login gate: a user with must_change_password cannot leave the
    // change-password screen until they set a new password.
    if (this.user && this.user.must_change_password && path !== '/change-password') {
      main.innerHTML = `<div class="login-page">${renderChangePassword()}</div>`;
      return;
    }

    const isActive = (p) => path === p || (p !== '/dashboard' && p !== '/' && path.startsWith(p)) ? 'active' : '';

    // Role-aware navigation: each role only sees the tabs it can use.
    const navLink = (href, label) =>
      `<a class="nav-link ${isActive(href)}" href="${href}">${label}</a>`;
    const role = this.user?.role || '';
    const navTabs = [navLink('#/dashboard', 'Dashboard'), navLink('#/ledger/project', 'Projects')];
    if (role === 'admin' || role === 'analyst') navTabs.push(navLink('#/finding-editor', 'Findings'));
    if (role === 'admin' || role === 'analyst') navTabs.push(navLink('#/evidence', 'Evidence'));
    navTabs.push(navLink('#/reports', 'Reports'));
    if (role === 'admin') navTabs.push(navLink('#/admin/users', 'Users'));
    if (role === 'admin' || role === 'project_manager') navTabs.push(navLink('#/closure-prep', 'Closure Prep'));

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
      case path === '/finding-detail':
        pageContent = renderFindingDetail();
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
      case path === '/admin/users':
        pageContent = renderUserManagement();
        break;
      case path === '/closure-prep':
        pageContent = renderClosurePrep();
        break;
      case path === '/change-password':
        pageContent = renderChangePassword();
        break;
      default:
        pageContent = renderLedgerDashboard();
    }

    main.innerHTML = `
      <div class="app-shell">
        <nav class="navbar">
          <div class="nav-left">
            <a class="nav-brand" href="#/dashboard">
              <img src="/images/cyberteq-mark.png" alt="" class="brand-mark">
              <span class="nav-title">mCollaborator</span>
            </a>
            <div class="nav-links">${navTabs.join('')}</div>
          </div>
          <div class="nav-right">
            <select class="nav-select" onchange="STITCH.setTheme(this.value)">
              <option value="cyberpunk" ${STITCH.theme === 'cyberpunk' ? 'selected' : ''}>Cyberpunk</option>
              <option value="ledger" ${STITCH.theme === 'ledger' ? 'selected' : ''}>Ledger</option>
              <option value="insight" ${STITCH.theme === 'insight' ? 'selected' : ''}>Slate</option>
            </select>
            <span class="nav-user">${STITCH.user?.name || ''}</span>
            <button class="btn btn-ghost text-sm" onclick="STITCH.logout()">Logout</button>
          </div>
        </nav>
        <div class="content">
          ${pageContent}
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
    '/finding-detail': 'Finding Detail',
    '/command/engagements': 'Active Engagements',
    '/command/feed': 'Vulnerability Feed',
    '/command/report-builder': 'Report Builder',
    '/admin/users': 'User Management',
    '/closure-prep': 'Closure Prep',
    '/change-password': 'Change Password',
    '/login': 'Login',
  };
  return titles[path] || 'mCollaborator';
}
