// mCollaborator API Client
const BASE = '/api/v1';

const api = {
  async request(method, path, body) {
    const headers = { 'Content-Type': 'application/json' };
    if (MCOLLABORATOR.token) {
      headers['Authorization'] = `Bearer ${MCOLLABORATOR.token}`;
    }
    const opts = { method, headers };
    if (body && method !== 'GET') {
      opts.body = JSON.stringify(body);
    }
    try {
      const res = await fetch(`${BASE}${path}`, opts);
      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error?.message || 'Request failed');
      }
      return data;
    } catch (e) {
      if (e.message.includes('401') || e.message.includes('Unauthorized')) {
        MCOLLABORATOR.logout();
      }
      throw e;
    }
  },

  get(path) { return this.request('GET', path); },
  post(path, body) { return this.request('POST', path, body); },
  put(path, body) { return this.request('PUT', path, body); },
  patch(path, body) { return this.request('PATCH', path, body); },
  del(path) { return this.request('DELETE', path); },

  // uploadBase resolves where a multipart POST should be sent.
  //
  // In a browser that is the ordinary relative path. Inside the desktop window
  // it must not be: WebView2 intercepts every request to the app's own
  // wails:// origin and Wails rebuilds it from an opaque stream before proxying
  // it on, and a multipart body does not survive that - the server received the
  // file part with nothing in it, so a perfectly good report was reported as
  // corrupt. An absolute loopback URL is not intercepted, so uploads go
  // straight at the server. Everything else stays on the proxied path, which
  // works and keeps one origin for the session token.
  _uploadBase: null,
  async uploadBase() {
    if (this._uploadBase !== null) return this._uploadBase;
    this._uploadBase = BASE;
    try {
      const shell = window.go && window.go.main && window.go.main.App;
      if (shell && shell.ServerURL) {
        const origin = await shell.ServerURL();
        if (origin) this._uploadBase = `${origin.replace(/\/$/, '')}${BASE}`;
      }
    } catch (e) {
      // Not in the desktop shell, or the binding is missing because the app
      // was built before it existed. The relative path is the right fallback.
    }
    return this._uploadBase;
  },

  async upload(path, formData) {
    const headers = {};
    if (MCOLLABORATOR.token) {
      headers['Authorization'] = `Bearer ${MCOLLABORATOR.token}`;
    }
    const base = await this.uploadBase();
    const res = await fetch(`${base}${path}`, { method: 'POST', headers, body: formData });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error?.message || 'Upload failed');
    return data;
  }
};
