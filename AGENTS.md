# AGENTS.md

## Platform & Toolchain

- **macOS only.** All source files carry `//go:build darwin`. Do not attempt to build or run on Linux/Windows.
- Go 1.22. Single external dependency: `gopkg.in/yaml.v3`. All HTTP, OAuth2, JSON, and logging use stdlib.
- **`CGO_ENABLED=1` is required** for every build (`make build` sets it). CGo is pulled in by the ObjC bridge files (`watcher_bridge.m`, `callback.go`) even though the active watcher implementation uses `osascript` polling.

## Dev Commands

All normal workflows go through `make`:

| Command | Purpose |
|---|---|
| `make build` | Compiles to `./build/teams-music` |
| `make clean` | Removes `./build/` |
| `sudo make install` | Builds + installs binary to `/usr/local/bin/teams-music` |
| `make init` | Creates `~/.config/teams-music/config.yaml` (run after install) |
| `make agent-install` | Installs LaunchAgent — **must NOT use sudo** |
| `make agent-uninstall` | Removes LaunchAgent |
| `make agent-status` | Shows LaunchAgent status via `launchctl` |
| `make logs` / `make logs-err` | Tails stdout/stderr log files |

**No test, lint, or format targets exist.** There are no `*_test.go` files anywhere.

## Install Order Matters

`sudo make install` → `make init` → `make agent-install` (in that order, agent-install without sudo).

## Architecture

```
osascript poll (debounce_seconds interval)
  → musicwatcher.Watcher (emits on state/track change only)
  → daemon.Debouncer (play: delayed; pause/stop: immediate; duplicate: dropped)
  → daemon.Service.processEvents
  → teams.GraphClient → POST /users/{id}/presence/setStatusMessage
```

Key packages under `internal/`:
- `auth/` — Device Code OAuth2 flow, token refresh, token stored at `~/.config/teams-music/token.json` (chmod 0600)
- `config/` — YAML load/validate; live config at `~/.config/teams-music/config.yaml` (not in repo; `configs/config.yaml` is the example)
- `daemon/` — orchestrator, debouncer, LaunchAgent plist management
- `musicwatcher/` — polling watcher + legacy ObjC bridge (both coexist in package)
- `teams/` — Graph API client with 429 backoff and 401 token-refresh retry

## Non-Obvious Quirks

- **Template rendering is hand-rolled string replacement**, not `text/template`, despite `{{.Name}}` / `{{.Artist}}` / `{{.Album}}` syntax. Do not refactor to `text/template` without verifying the expansion code in `daemon/service.go`.
- **`status.timezone`** takes a Microsoft timezone name (e.g. `"W. Europe Standard Time"`), not an IANA zone. Wrong format silently sets incorrect expiry.
- **Log messages are in German** (e.g. `"Initialisiere Authentifizierung..."`). This is intentional; don't "fix" them.
- **Pre-compiled arm64 binary is checked into git** at `build/teams-music`. README directs users to use it directly. Keep it in sync if you change the build.
- **LaunchAgent label:** `de.teams-music`; plist at `~/Library/LaunchAgents/de.teams-music.plist`.
- `azure.tenant_id` and `azure.client_id` are required config fields with no defaults; the daemon will fail to start without them.
