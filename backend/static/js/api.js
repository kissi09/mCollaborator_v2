// STITCH API Client
const BASE = '/api/v1';

const api = {
  async request(method, path, body) {
    const headers = { 'Content-Type': 'application/json' };
    if (STITCH.token) {
      headers['Authorization'] = `Bearer ${STITCH.token}`;
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
        STITCH.logout();
      }
      throw e;
    }
  },

  get(path) { return this.request('GET', path); },
  post(path, body) { return this.request('POST', path, body); },
  put(path, body) { return this.request('PUT', path, body); },
  patch(path, body) { return this.request('PATCH', path, body); },
  del(path) { return this.request('DELETE', path); },

  async upload(path, formData) {
    const headers = {};
    if (STITCH.token) {
      headers['Authorization'] = `Bearer ${STITCH.token}`;
    }
    const res = await fetch(`${BASE}${path}`, { method: 'POST', headers, body: formData });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error?.message || 'Upload failed');
    return data;
  }
};
