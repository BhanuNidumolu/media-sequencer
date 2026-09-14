import React, { useCallback, useEffect, useState } from 'react'
import { api } from './api'
import WindowBox from './components/WindowBox.jsx'
import AddMediaForm from './components/AddMediaForm.jsx'
import SyncControl from './components/SyncControl.jsx'

// How often the frontend asks the backend "what should each window be
// showing right now". Polling (rather than WebSockets) keeps deployment
// simple - see README "Assumptions & Tradeoffs" for the reasoning and
// how this would be swapped for a push-based connection.
const POLL_INTERVAL_MS = 1000

export default function App() {
  const [windows, setWindows] = useState([])       // full playlists, for admin forms
  const [displays, setDisplays] = useState([])      // what each window shows right now
  const [syncState, setSyncState] = useState(null)
  const [loadError, setLoadError] = useState('')

  const refreshWindows = useCallback(async () => {
    try {
      const data = await api.getWindows()
      setWindows(data)
    } catch (err) {
      setLoadError(err.message)
    }
  }, [])

  const refreshState = useCallback(async () => {
    try {
      const data = await api.getState()
      setDisplays(data.windows)
      setSyncState(data.sync)
      setLoadError('')
    } catch (err) {
      setLoadError(err.message)
    }
  }, [])

  // Initial load.
  useEffect(() => {
    refreshWindows()
    refreshState()
  }, [refreshWindows, refreshState])

  // Continuous polling for playback state.
  useEffect(() => {
    const id = setInterval(refreshState, POLL_INTERVAL_MS)
    return () => clearInterval(id)
  }, [refreshState])

  return (
    <div className="app">
      <header className="app__header">
        <h1>Multi-Window Media Sequencer</h1>
        <p>Each window loops its own playlist. Sync makes every window show one item together.</p>
      </header>

      {loadError && (
        <div className="banner banner--error">
          Could not reach backend: {loadError}. Is it running and is VITE_API_URL set correctly?
        </div>
      )}

      <section className="window-grid">
        {displays.map((d) => {
          const win = windows.find((w) => w.id === d.windowId)
          return (
            <WindowBox key={d.windowId} display={d} playlist={win?.playlist || []} />
          )
        })}
      </section>

      <section className="control-panels">
        {windows.length > 0 && (
          <>
            <AddMediaForm windows={windows} onAdded={refreshWindows} />
            <SyncControl windows={windows} syncState={syncState} onSynced={refreshState} />
          </>
        )}
      </section>
    </div>
  )
}
