import React, { useState } from 'react'
import { api } from '../api'

export default function AddMediaForm({ windows, onAdded }) {
  const [windowId, setWindowId] = useState(windows[0]?.id || '')
  const [type, setType] = useState('image')
  const [source, setSource] = useState('link') // 'link' | 'upload'
  const [url, setUrl] = useState('')
  const [file, setFile] = useState(null)
  const [duration, setDuration] = useState(8)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(e) {
    e.preventDefault()
    setError('')
    setSubmitting(true)
    try {
      let mediaUrl = url
      let mediaType = type

      // Blank items have no file/url at all, so skip straight to add-media.
      if (type !== 'blank' && source === 'upload') {
        if (!file) throw new Error('Choose a file first')
        // Upload first to get back a URL, then use the exact same
        // add-media call as the link path below - the backend doesn't
        // need to know or care whether a URL came from a paste or a file.
        const uploaded = await api.uploadFile(file)
        mediaUrl = uploaded.url
        mediaType = uploaded.type // backend infers image/video from the file extension
      }

      await api.addMedia(windowId, {
        type: mediaType,
        url: type === 'blank' ? '' : mediaUrl,
        durationSeconds: Number(duration),
      })

      setUrl('')
      setFile(null)
      onAdded?.()
    } catch (err) {
      setError(err.message)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <form className="panel-form" onSubmit={handleSubmit}>
      <h3>Add media to a window</h3>

      <label>
        Window
        <select value={windowId} onChange={(e) => setWindowId(e.target.value)}>
          {windows.map((w) => (
            <option key={w.id} value={w.id}>{w.name}</option>
          ))}
        </select>
      </label>

      <label>
        Type
        <select value={type} onChange={(e) => setType(e.target.value)}>
          <option value="image">Image</option>
          <option value="video">Video</option>
          <option value="blank">Blank</option>
        </select>
      </label>

      {type !== 'blank' && (
        <>
          <div className="source-toggle" role="radiogroup" aria-label="Media source">
            <label>
              <input
                type="radio"
                name="source"
                checked={source === 'link'}
                onChange={() => setSource('link')}
              />
              Link
            </label>
            <label>
              <input
                type="radio"
                name="source"
                checked={source === 'upload'}
                onChange={() => setSource('upload')}
              />
              Upload from PC
            </label>
          </div>

          {source === 'link' ? (
            <label>
              Media URL
              <input
                type="text"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="https://..."
                required
              />
            </label>
          ) : (
            <label>
              File ({type === 'image' ? 'jpg, png, gif, webp' : 'mp4, webm, mov'})
              <input
                type="file"
                accept={type === 'image' ? 'image/*' : 'video/*'}
                onChange={(e) => setFile(e.target.files?.[0] || null)}
                required
              />
            </label>
          )}
        </>
      )}

      <label>
        Duration (seconds)
        <input
          type="number"
          min="1"
          value={duration}
          onChange={(e) => setDuration(e.target.value)}
          required
        />
      </label>

      {error && <p className="form-error">{error}</p>}

      <button type="submit" disabled={submitting}>
        {submitting ? (source === 'upload' ? 'Uploading...' : 'Adding...') : 'Add to playlist'}
      </button>
    </form>
  )
}
