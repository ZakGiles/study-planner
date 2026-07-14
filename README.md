# Study Planner

A cross-platform desktop app for spaced-repetition study. Add the **tasks** you
want to revise, group them into **subjects**, lay out study **sessions** (by hand
or from a spaced-repetition plan), tick them off as you go, and let "adaptive"
tasks re-space themselves based on how well you recalled the material. A built-in
focus timer tracks the time you actually spend studying.

Built with [Wails](https://wails.io): a **Go** backend and a **Svelte +
TypeScript** frontend, shipped as a single native binary — and also as a
[browser-only build on GitHub Pages](https://zakgiles.github.io/study-planner/).
All data lives locally — **SQLite** on desktop, your browser's **localStorage**
on the web — there is no server and nothing leaves your machine.

## Features

- **Tasks** — name, description, colour, and tags, with manual drag-to-reorder
  and an archive for things you've finished. New tasks are auto-assigned a
  distinct palette colour.
- **Subjects** — group tasks under collapsible subject headers (with an
  *Ungrouped* bucket for the rest). Subjects have their own colour and order;
  deleting one just ungroups its tasks rather than removing them.
- **Spaced repetition** — generate a schedule from a start date using the
  default offsets (study now, then after 1, 3, 7, 14 and 30 days) or your own
  intervals. Merge into an existing schedule or replace it.
- **Adaptive rescheduling** — grade a review *again / hard / good / easy* and an
  SM-2-lite pass re-spaces the task's remaining reviews, anchored to today so
  overdue schedules catch up.
- **Agenda & calendar** — see what's due today, one-click **reschedule all
  overdue** reviews to today, and browse sessions on a calendar.
- **Focus timer** — a Pomodoro-style focus block, optionally tied to a task.
  Completed blocks are logged (abandoned time is not).
- **Stats** — track completed sessions and focus time over time.
- **Native due-today notifications** — a daily summary of due and overdue
  reviews, refreshed at each local midnight while the app is running.

## Quick start

Requires [Go](https://go.dev) 1.25+ and [Node.js](https://nodejs.org) (for the
frontend build). Install the Wails CLI once:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Then, from the project root:

```bash
wails dev      # run with hot reload
wails build    # produce a distributable binary in build/bin/
```

## Web version

The same app runs in the browser at
**<https://zakgiles.github.io/study-planner/>** — no install, no server. The
Svelte UI is identical; behind it, a TypeScript port of the Go backend
(`frontend/src/lib/backend/local/`) implements the same API over a single JSON
blob in the browser's localStorage. Which backend to use is detected at load
(`frontend/src/lib/backend/index.ts`), so one build serves both the desktop
shell and the web. Every push to `main` deploys via the
[Pages workflow](.github/workflows/pages.yml) (one-time repo setup: *Settings →
Pages → Source: GitHub Actions*). To try it locally in a plain browser:
`npm --prefix frontend run dev`.

Web-specific notes:

- **Data stays in that browser.** Nothing syncs — the desktop app and the web
  app (and each browser profile) each hold their own copy.
- **Backup** — Settings gains *Export data* / *Import data* (a JSON file): the
  insurance against cleared site data. Calendar export downloads the .ics
  instead of opening a save dialog.
- **Notifications** — an opt-in Settings toggle shows the due-today summary via
  the browser Notification API, once per day while the app is open.
- **Launch on login** is desktop-only and hidden on the web.
- Keep the app to **one tab**: two open tabs hold independent copies of the
  state and the last write wins.

## Development

```bash
go test ./...                      # Go backend tests
npm --prefix frontend run check    # Svelte/TypeScript type-check
npm --prefix frontend test         # vitest suite for the web (local) backend
npm --prefix frontend run build    # build the frontend bundle
```

The Wails CLI runs `npm install` and `npm run build` in `frontend/`
automatically as part of `wails dev` / `wails build`.

## Project layout

```
main.go             Wails entry point; embeds frontend/dist and wires the App
app.go              App methods bound to the frontend (subjects, tasks, sessions, focus, grading)
models.go           Domain types (Subject, Task, Session, FocusSession) and scheduling helpers
store.go            SQLite-backed store; whole-graph save() on every mutation
*_test.go           Go unit tests
frontend/src/       Svelte UI — App.svelte plus lib/ (Calendar, Focus, Stats, TaskCard, …)
frontend/src/lib/backend/  Backend adapter: Wails bindings on desktop, TS localStorage port (+ its vitest suite) on the web
.github/workflows/  Cross-platform release builds + GitHub Pages deploy
```

The Go layer keeps all subjects and tasks in memory as the authoritative working
copy; each mutating method persists the whole graph to SQLite in a single
transaction and returns the fresh, sorted state so the frontend can replace its
state in one go.

## Data & storage

On desktop, data is stored in `data.db` inside your OS config directory — for
example `~/Library/Application Support/study-planner` on macOS. A legacy
`data.json`, if present, is imported once on first run and kept alongside the
database as a backup.

On the web, everything lives under the `study-planner:data` localStorage key of
the browser you use — clearing site data deletes it, so keep a JSON backup
(Settings → Backup).

## Releases

Tagging a release triggers the [release workflow](.github/workflows/release.yml),
which builds native binaries for **macOS** (universal), **Windows** (amd64) and
**Linux** (amd64) and attaches them to the GitHub release.
