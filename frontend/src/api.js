// Base URL of the Golang backend. Set VITE_API_URL in a .env file (or
// your hosting provider's env var settings) when deploying, e.g.
//   VITE_API_URL=https://your-backend.onrender.com
const API_BASE = import.meta.env.VITE_API_URL || 'http://localhost:8080'

async function request(path, options = {}) {
  const res = await fetch(`${API_BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Request failed: ${res.status}`)
  }
  return res.json()
}

// Separate from request() above: file uploads use FormData, and the
// browser needs to set its own multipart Content-Type header (with the
// boundary) - passing our own JSON header here would break the upload.
async function uploadFile(file) {
  const form = new FormData()
  form.append('file', file)
  const res = await fetch(`${API_BASE}/api/upload`, {
    method: 'POST',
    body: form,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Upload failed: ${res.status}`)
  }
  return res.json() // { url, type }
}

export const api = {
  getWindows: () => request('/api/windows'),
  getState: () => request('/api/state'),
  addMedia: (windowId, item) =>
    request(`/api/windows/${windowId}/media`, {
      method: 'POST',
      body: JSON.stringify(item),
    }),
  triggerSync: (mediaId, durationSeconds) =>
    request('/api/sync', {
      method: 'POST',
      body: JSON.stringify({ mediaId, durationSeconds }),
    }),
  uploadFile,
}
