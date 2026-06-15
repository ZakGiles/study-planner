# Study Planner

A desktop spaced-repetition study planner. Add **topics** to revise, schedule
study **sessions** (by hand or with a spaced-repetition plan), tick them off, and
let "adaptive" topics re-space themselves based on how well you recalled the
material.

Built with [Wails](https://wails.io): a **Go** backend + a **Svelte + TypeScript**
frontend, shipped as a single native binary. Data is stored locally in **SQLite**.

## Quick start

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest   # once
wails dev      # run with hot reload
wails build    # produce a distributable binary in build/bin/
go test ./...  # run the Go tests
```

See [docs/05](docs/05-dev-workflow.md) for prerequisites and details.
