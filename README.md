# Multi-Window Media Sequencer

**Submitted by:** N BHANU PRASAD
**Role:** Backend Development Intern — Assignment Submission
**Stack:** Go (backend) + React (frontend)

**Live URLs:**
- Frontend: https://media-sequencer-eight.vercel.app
- Backend: https://media-sequencer-mrdb.onrender.com

**Repo:** https://github.com/BhanuNidumolu/media-sequencer

## What this is

This is my submission for the Multi-Window Media Sequencer assignment.
The app runs multiple display "windows," each looping its own media
playlist independently and forever. On top of that, there's a **sync**
action: trigger it with any media item, and every window switches to
showing that same item together for a set duration, then goes back to
its own sequence exactly where it would have been anyway.

- **Backend:** Go, with one dependency — a pure-Go SQLite driver (no C toolchain needed)
- **Frontend:** React (Vite)
- **Persistence:** SQLite (embedded, single-file database)

Honestly, the sync part is what hooked me on this assignment — once I
figured out that playback position could just be "elapsed time modulo
playlist length" instead of some counter I had to babysit, a lot of the
hard parts (resuming after sync, surviving a restart, two people
clicking sync at once) just... stopped being problems. I've written up
the full reasoning below, plus every assumption I made where the spec
left room to interpret things differently.

**A note on AI usage:** AI wrote most of the frontend components (the
React structure, JSX, and styling in `App.jsx`, `WindowBox.jsx`,
`AddMediaForm.jsx`, `SyncControl.jsx`) — I then went through and
reviewed/adjusted them, fixed spots where they didn't match what the
backend actually returns, and tested them against the real running app.
The backend is a different story though — the sync algorithm, the
storage layer, the API design, all of that's mine, and it's what I've
put the most effort into explaining below.

**A note on learning Go for this:** I come from Java/Spring Boot, so
this was genuinely my first real Go project — got shortlisted and then
realized I'd need to pick up a new language fast. Syntax wasn't really
the hard part. What actually took getting used to was designing around
explicit error returns instead of exceptions, and thinking about
concurrency with goroutines/mutexes (`store.go`'s `sync.Mutex`) instead
of letting a framework handle it for me. I ended up working through
`currentItemForWindow`'s edge cases by hand — what happens right at
t=0, exactly on an item boundary, after 500 loops — mostly to convince
*myself* the time-based approach actually held up. That's also why I
went and wrote real tests for those exact cases afterward (see "Running
tests" below) — I wanted more than "it worked when I clicked around."

---

## Requirements checklist

Mapped directly against the assignment brief, so it's easy to verify
nothing was missed:

| Requirement | Status | Where |
|---|---|---|
| React frontend | ✅ | `frontend/` |
| Multiple display windows on screen | ✅ | `WindowBox.jsx`, rendered per-window in `App.jsx` |
| Continuous playback per window, no stopping between items | ✅ | time-based `currentItemForWindow` + 1s polling in `App.jsx` |
| Image, video, and blank/fallback support | ✅ | `WindowBox.jsx` |
| Reflect playlist changes from backend | ✅ | `refreshWindows()` runs after every add |
| Controls to add media + trigger sync | ✅ | `AddMediaForm.jsx`, `SyncControl.jsx` |
| Golang backend | ✅ | `backend/` |
| Persistent storage (candidate's choice) | ✅ | SQLite via `store.go` |
| Dynamically add media to a window's list | ✅ | `POST /api/windows/{id}/media` |
| Sync logic — one item shown on all windows | ✅ | `computeDisplay()`'s time-window override in `sync.go` |
| 5-hour cycle, list repeats within it | ✅ (documented assumption) | `MaxCycleSeconds` in `models.go` — see "How sync behavior works" |
| Blank only appears when explicitly configured | ✅ | seed data + `currentItemForWindow`'s per-item walk |
| Deployed React frontend, live URL | ✅ | Vercel — see Live URLs above |
| Deployed Go backend, live URL | ✅ | Render — see Live URLs above |
| Seed data matching example windows/lists | ⚠️ see note | `seed.go` |
| README: setup, sync explanation, deployment, assumptions, API docs | ✅ | this file |

**Note on the seed-data row:** `seed.go` seeds 3 example windows with
image/video/blank items covering every media type, but I want to flag
this honestly rather than just claim a match — worth a final check
against whatever specific example windows/lists were shown in the
original brief, since I want to be sure my interpretation lines up
exactly before the discussion round.

---

## Project structure

```
media-sequencer/
├── backend/          # Go API server
│   ├── main.go        # server setup, routing, startup seeding
│   ├── models.go       # data types
│   ├── store.go         # persistence layer (SQLite, thread-safe)
│   ├── sync.go            # core playback position + sync computation
│   ├── handlers.go         # HTTP handlers
│   ├── upload.go            # file upload endpoint
│   ├── seed.go                 # example seed data
│   └── data/store.db             # created automatically on first run
└── frontend/          # React app
    └── src/
        ├── App.jsx      # polls backend, renders windows + admin controls
        ├── api.js         # fetch wrapper
        └── components/      # WindowBox, AddMediaForm, SyncControl
```

---

## How sync behavior works (read this first)

This was the main design problem in the assignment, so here's how I
thought about it.

**I compute playback position from time, not from a stored counter.**
Every window has a `createdAt` timestamp. To figure out what a window
should be showing *right now*, the backend does:

1. `elapsed = now - window.createdAt` (in seconds)
2. `position = elapsed mod totalPlaylistDuration` — this gives the
   position inside one loop of the playlist
3. Walk the playlist, adding up each item's duration, until `position`
   lands inside one item — that's the current item.

I went with this instead of storing a "current index" per window because
it's stateless: a server restart, a new browser tab, or two clients
asking at the same instant all get the same, consistent answer without
me having to keep anything in sync manually.

**Sync is an override on top of that, not a state mutation.** When
`/api/sync` is called with a media item and a duration, the backend just
stores:
```
{ item, startedAt, endsAt }
```
Every time `/api/state` is asked what each window should show, it first
checks whether there's an active sync that hasn't expired. If there is,
every window returns that same item, ignoring its own playlist position
for that moment. Once the sync expires (or if there isn't one), each
window falls back to its own time-based computation from step 1.

The part I liked most about this approach: resuming after sync needed no
special-case logic at all. No window's underlying position was ever
touched, so once the sync ends it just continues exactly where the time
formula says it should be — "resume" is really just the *absence* of an
active override, not code I had to go write separately.

**On the 5-hour cycle:** I read "the total play size for each window
must be treated as 5 hours" as a ceiling on the loop length rather than a
literal requirement that every playlist adds up to exactly 5 hours of
media. I implemented it as `MaxCycleSeconds = 18000`. In practice a
playlist's real duration is much shorter than that, so it naturally
repeats several times within any 5-hour window, which is what "keep
playing its configured list again and again within that cycle" is
describing. I've called this out again under Assumptions below in case
the intent was different.

---

## Local setup

### Option A — Docker (no Go or Node install needed)

Requires only [Docker Desktop](https://www.docker.com/products/docker-desktop/).

```bash
docker compose up --build
```
This builds both images (the Go and Node toolchains run *inside* the
containers, not on your machine) and starts them together:
- Backend → `http://localhost:8080`
- Frontend → `http://localhost:5173`

`backend/data/` is mounted as a volume, so `store.db` survives
`docker compose down` / restarts. Stop with `Ctrl+C`, or `docker compose
down` to remove the containers (data on disk is untouched either way).

To rebuild after changing code: `docker compose up --build` again.

### Option B — Native (Go + Node installed locally)

#### Backend

```bash
cd backend
go mod tidy   # fetches the SQLite driver + writes go.sum (one-time, needs internet)
go run .
```
The server starts on `http://localhost:8080`, creates `data/store.db`
if it doesn't exist, and seeds it with 3 example windows on first run
only.

Environment variables (all optional, sensible defaults shown):

| Variable          | Default              | Purpose                                  |
|-------------------|----------------------|-------------------------------------------|
| `PORT`             | `8080`               | HTTP port                                 |
| `DATA_FILE`         | `./data/store.db`   | Where the SQLite database is stored    |
| `FRONTEND_ORIGIN`    | `*`                  | CORS allowed origin (set to your deployed frontend URL in production) |
| `BACKEND_PUBLIC_URL`  | `http://localhost:PORT` | This service's own public URL, used to build absolute links for uploaded files (set to your deployed backend URL in production) |

#### Frontend

```bash
cd frontend
cp .env.example .env     # then edit VITE_API_URL if needed
npm install
npm run dev
```
Opens on `http://localhost:5173` and polls the backend every second for
current playback state.

---

## Running tests

The backend's Go test suite runs entirely inside Docker — no local Go
install needed:

```bash
docker compose run --rm backend-tests
```

This builds a throwaway container (using `backend/Dockerfile.test`) from
the same Go base image the real backend uses, and runs `go test ./... -v`
against `sync_test.go`, `handlers_test.go`, and `store_test.go`. It's kept
out of the normal `docker compose up` via a `test` profile, so it never
starts alongside the actual backend/frontend services.

If you do have Go installed locally and want a faster feedback loop while
iterating, `cd backend && go test ./... -v` works the same way.

---

## API documentation

All responses are JSON. Base path assumed: `http://localhost:8080`
(or `https://media-sequencer-mrdb.onrender.com` for the deployed version).

### `GET /api/windows`
Returns every window with its full playlist (used by the admin forms).
```json
[
  {
    "id": "window-1",
    "name": "Window A - Entrance Display",
    "createdAt": "2026-09-13T10:00:00Z",
    "playlist": [
      { "id": "m-seed-1", "type": "image", "url": "https://...", "durationSeconds": 8 }
    ]
  }
]
```

### `GET /api/state`
Returns what every window should be displaying **right now**. This is
the endpoint the frontend polls continuously.
```json
{
  "windows": [
    {
      "windowId": "window-1",
      "windowName": "Window A - Entrance Display",
      "item": { "id": "m-seed-1", "type": "image", "url": "https://...", "durationSeconds": 8 },
      "isSynced": false,
      "positionSeconds": 3
    }
  ],
  "sync": { "active": false },
  "serverTime": "2026-09-13T10:05:00Z"
}
```

### `POST /api/windows/{id}/media`
Adds a new item to the end of a window's playlist.
```json
// request body
{ "type": "image", "url": "https://example.com/pic.jpg", "durationSeconds": 10 }
```
`type` must be `image`, `video`, or `blank` (omit `url` for `blank`).
Returns the updated window.

### `POST /api/sync`
Triggers every window to show one item together.
```json
// request body
{ "mediaId": "m-seed-1", "durationSeconds": 10 }
```
`mediaId` must belong to an existing playlist item somewhere (its
type/url are looked up automatically). Returns the resulting sync state.

### `GET /api/sync`
Returns the current sync state (mainly useful for debugging).

### `POST /api/upload`
Uploads a file (image or video) from disk and returns a URL for it.
Content-Type must be `multipart/form-data` with the file under field name
`file`. Accepts jpg/jpeg/png/gif/webp (image) and mp4/webm/mov (video), up
to 20MB.
```json
// response
{ "url": "https://media-sequencer-mrdb.onrender.com/uploads/u-1234567890.jpg", "type": "image" }
```
The returned `url` is then passed straight to `POST
/api/windows/{id}/media` - uploading and pasting a link both end up
calling the exact same add-media endpoint once there's a URL either way.
Uploaded files are served back out at `GET /uploads/{filename}`.

### `GET /healthz`
Plain `200 ok` — for uptime checks on the hosting platform.

---

## Deployment

Both pieces are live right now — see Live URLs at the top. Here's how
I set them up:

### Backend (Go) — Render
1. New → Web Service → connected this repo, root directory `backend`.
2. Build command: `go mod tidy && go build -o server .`
3. Start command: `./server`
4. Env vars: `FRONTEND_ORIGIN` set to the deployed frontend's URL, and
   `BACKEND_PUBLIC_URL` set to this service's own URL, so uploaded-file
   links resolve correctly.
5. **Persistence note:** Render's free tier filesystem is ephemeral (it
   resets on redeploy) — this affects both `store.db` and anything in
   `data/uploads/`. If data needs to survive redeploys, either use a paid
   plan with a persistent disk mounted at `DATA_FILE`'s directory, or
   swap SQLite for a managed database (e.g. Postgres) plus object storage
   (S3-style) for uploads (see Assumptions below for how contained that
   change would be).

### Frontend (React) — Vercel
1. Imported this repo, root directory `frontend`.
2. Framework preset: Vite (auto-detected).
3. Env var `VITE_API_URL` set to the deployed backend's URL.
4. Vercel runs `npm run build` and serves `dist/` automatically.

---

## Assumptions & tradeoffs

I want to be upfront about the calls I made where the spec left room for
interpretation, and why I made them:

- **Persistence — SQLite, not a client/server database.** I chose this
  because it's a real relational database — proper tables, SQL, ACID
  transactions — but embedded as a single file, so there's no separate
  database server to install, configure, or connect to over a network.
  Backend startup runs a migration that creates the schema if it doesn't
  exist yet (`store.go`'s `migrate()`), and I capped the connection pool
  to one connection (`db.SetMaxOpenConns(1)`) since SQLite only allows
  one writer at a time anyway — this avoids "database is locked" errors
  under concurrent requests rather than working around them with retries.
  The tradeoff is the same as any embedded database: it doesn't support
  multiple backend instances writing concurrently the way Postgres would.
  I actually built this with a JSON file first, then deliberately swapped
  it for SQLite to see how contained that change would really be — it
  ended up touching only the method bodies inside `store.go`
  (`AllWindows`, `AddMediaToWindow`, `SetSyncState`, etc.), replacing file
  reads/writes with SQL queries. The handlers, the sync logic, and every
  API response stayed identical, since they only ever depended on
  `Store`'s public method signatures, never on how it stored things
  internally. The same would hold true for swapping SQLite for Postgres
  later, if this needed to scale beyond a single instance.
- **Polling instead of WebSockets.** The frontend polls `/api/state`
  every second instead of holding a persistent connection. This keeps
  deployment simple (no special hosting requirements for long-lived
  connections), at the cost of up to ~1s latency on sync events and a bit
  more request volume. Moving to WebSocket/SSE would only touch
  `api.js`/`App.jsx` on the frontend and add one broadcast handler on the
  backend — `sync.go`'s actual computation wouldn't need to change, since
  it's already timestamp-driven rather than push-driven.
- **Sync duration default.** If a sync request omits `durationSeconds`,
  the backend defaults to 10 seconds.
- **Media source of truth for sync.** A synced item is looked up by ID
  from whichever window's playlist currently contains it. I didn't build
  a separate "global media library," since every addable item already
  lives in some window's playlist by the time it can be synced.
- **5-hour cycle** — treated as a ceiling on loop length rather than a
  literal requirement that playlists total exactly 5 hours of content.
  Full reasoning is under "How sync behavior works" above.
- **File uploads land on local disk, not object storage.** `POST
  /api/upload` saves files under `data/uploads/` next to `store.db`, so
  they're covered by the same volume/disk in Docker and in local dev. In
  a production system handling real traffic I'd put this behind S3 (or
  similar) instead, since local disk doesn't scale across multiple
  backend instances and, on most free hosting tiers, doesn't survive a
  redeploy either — flagged again above under Deployment.

---

## What I'd add with more time

A few things I deliberately left out to keep the submission focused on
the core sync/playback problem, but would build next:

- **Delete/remove media items.** Right now there's `POST
  /api/windows/{id}/media` to add an item, but no matching delete — once
  something's in a playlist it's there for good. A real deployment would
  need `DELETE /api/windows/{id}/media/{itemId}`, plus cleaning up the
  matching file under `data/uploads/` if the item was an uploaded file
  rather than an external URL, so storage doesn't grow forever.
- **Reordering playlist items.** Items always append to the end; there's
  no way to move an item earlier/later in a window's sequence without
  deleting and re-adding everything after it.
- **Editing or removing whole windows.** Windows are currently fixed to
  what's seeded at startup — there's no create/delete-window endpoint,
  only add-media-to-an-existing-window.
- **WebSockets/SSE instead of polling**, as mentioned above under
  Assumptions — would cut sync latency from ~1s to near-instant and
  reduce request volume at scale.
- **Basic auth on the admin endpoints** (`add media`, `sync`, upload) —
  right now anyone with the backend URL can modify playlists or trigger
  sync, which is fine for this assignment but not for a real deployment.
- **More test coverage.** The current suite (33 tests) covers the sync
  algorithm, HTTP validation, and the storage layer, including a
  concurrency test for simultaneous writes. What's missing: frontend
  component tests (React Testing Library), and an integration test that
  spins up the real server and hits it over HTTP end-to-end rather than
  through Go's `httptest` in-process — closer to how it actually behaves
  in Docker.
