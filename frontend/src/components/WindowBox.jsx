import React from 'react'

// Renders whatever one window is currently displaying: an image, a
// looping muted video, or a blank placeholder. `display` is one entry
// from the /api/state response's `windows` array. `playlist` is that
// window's full item list from /api/windows - shown below the media so
// you can immediately confirm a newly added item made it into the
// queue, even before playback reaches it (new items go to the end of
// the list and play once the loop gets there, not instantly).
export default function WindowBox({ display, playlist = [] }) {
  const { windowName, item, isSynced } = display

  return (
    <div className={`window-box ${isSynced ? 'window-box--synced' : ''}`}>
      <div className="window-box__header">
        <span>{windowName}</span>
        {isSynced && <span className="sync-badge">SYNCED</span>}
      </div>
      <div className="window-box__media">
        {item.type === 'image' && (
          <img src={item.url} alt="" />
        )}
        {item.type === 'video' && (
          // key forces React to remount the <video> when the source
          // changes, so playback restarts cleanly for the new item.
          <video key={item.id || item.url} src={item.url} autoPlay muted loop playsInline />
        )}
        {item.type === 'blank' && (
          <div className="window-box__blank">blank</div>
        )}
      </div>
      {playlist.length > 0 && (
        <ol className="window-box__queue">
          {playlist.map((p) => (
            <li key={p.id} className={p.id === item.id ? 'is-current' : ''}>
              {p.type} · {p.durationSeconds}s{p.id === item.id ? ' (now playing)' : ''}
            </li>
          ))}
        </ol>
      )}
    </div>
  )
}
