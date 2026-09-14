import React, { useMemo, useState } from 'react'
import { api } from '../api'

export default function SyncControl({ windows, syncState, onSynced }) {
  // Flatten every window's playlist into one pickable list so the user
  // can choose any item (e.g. "M2") to sync, regardless of which window
  // it originally belongs to.
  const allItems = useMemo(
    () =>
      windows.flatMap((w) =>
        w.playlist.map((item) => ({ ...item, windowName: w.name }))
      ),
    [windows]
  )

  const [mediaId, setMediaId] = useState('')
  const [duration, setDuration] = useState(10)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  async function handleSync(e) {
    e.preventDefault()
    setError('')
    setSubmitting(true)
    try {
      await api.triggerSync(mediaId, Number(duration))
      onSynced?.()
    } catch (err) {
      setError(err.message)
    } finally {
      setSubmitting(false)
    }
  }

  const secondsLeft = syncState?.active
    ? Math.max(0, Math.round((new Date(syncState.endsAt) - Date.now()) / 1000))
    : 0

  return (
    <form className="panel-form" onSubmit={handleSync}>
      <h3>Sync playback</h3>

      <label>
        Media item
        <select value={mediaId} onChange={(e) => setMediaId(e.target.value)} required>
          <option value="" disabled>Select an item...</option>
          {allItems.map((item) => (
            <option key={item.id} value={item.id}>
              {item.windowName} — {item.type} ({item.id})
            </option>
          ))}
        </select>
      </label>

      <label>
        Sync duration (seconds)
        <input
          type="number"
          min="1"
          value={duration}
          onChange={(e) => setDuration(e.target.value)}
        />
      </label>

      {error && <p className="form-error">{error}</p>}

      <button type="submit" disabled={submitting || !mediaId}>
        {submitting ? 'Syncing...' : 'Trigger sync'}
      </button>

      {syncState?.active && (
        <p className="sync-status">
          Sync active — {secondsLeft}s remaining
        </p>
      )}
    </form>
  )
}
