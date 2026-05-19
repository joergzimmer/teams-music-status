# 🎵 teams-music

**Automatically displays the current Apple Music track as your Microsoft Teams status.**

![macOS](https://img.shields.io/badge/macOS-only-blue)
![Go](https://img.shields.io/badge/Go-1.22+-00ADD8)
![License](https://img.shields.io/badge/License-MIT-green)

---

## What does this tool do?

I wanted my Teams-Status to show my currently playing song. Just like MSN or ICQ did in the good old days. So here it is!

`teams-music` is a macOS service that automatically synchronizes your Microsoft Teams status with the currently playing song from Apple Music.

- 🎶 Song playing → Teams status: `🎵 Bohemian Rhapsody – Queen`
- ⏸️ Music paused → Status is cleared
- ⏭️ Fast skipping → Debouncer prevents API spam

The tool runs as a macOS LaunchAgent in the background and starts automatically on every login.

---

## ✨ Features

- **Event-based** – no polling, reacts instantly to track changes via `NSDistributedNotificationCenter`
- **Debouncing** – configurable debouncer prevents unnecessary API calls when skipping tracks quickly
- **Auto-Refresh** – OAuth2 token is automatically renewed (Device Code Flow + Refresh Token)
- **LaunchAgent** – runs as a background service with automatic restart on crash
- **Configurable** – status template, expiry, timezone, log level via YAML config
- **No external dependencies** – pure Go + macOS system frameworks
- **Secure** – Delegated Permissions (only your own status), token stored with `0600` permissions

---

## 📋 Prerequisites

| What | Details |
|---|---|
| **macOS** | Ventura 13+ recommended (older versions should work too) |
| **Apple Music** | Must be installed (default on macOS) |
| **Microsoft Teams** | Active account in a Microsoft 365 organization |
| **Entra ID App Registration** | One-time setup required (instructions below) |

---

## 🚀 Quick Install

The repository includes a pre-compiled binary – no build required.
If you prefer shortcut commands, see the Makefile targets in [Build from Source](#-build-from-source).

### 1. Clone the repository

```bash
git clone https://github.com/joergzimmer/teams-music-status.git
cd teams-music-status
```

### 2. Install the binary

```bash
sudo cp ./build/teams-music /usr/local/bin/teams-music
sudo chmod 755 /usr/local/bin/teams-music
# or:
# sudo make install
```

### 3. Create config

```bash
teams-music --init
# ✅ Example config created: ~/.config/teams-music/config.yaml
# or:
# make init
```

### 4. Edit config

```bash
vim ~/.config/teams-music/config.yaml
```

Enter your **Tenant ID** and **Client ID** (see [Entra ID Setup](#-entra-id-app-registration-setup)):

```yaml
azure:
  client_id: "YOUR-CLIENT-ID"
  tenant_id: "YOUR-TENANT-ID"
```

### 5. First launch (authentication)

```bash
teams-music
```

On first launch, the Device Code Flow appears:

```
╔══════════════════════════════════════════════════════════╗
║  Authentication required                                 ║
╠══════════════════════════════════════════════════════════╣
║  1. Open:   https://microsoft.com/devicelogin            ║
║  2. Code:   ABCD-EFGH                                    ║
║  3. Sign in with your Microsoft account                  ║
╚══════════════════════════════════════════════════════════╝
```

1. Open browser → https://microsoft.com/devicelogin
2. Enter the code
3. Sign in with your Microsoft account
4. Confirm permissions

After successful login, the token is saved. Press **Ctrl+C** to exit.

> 💡 This step is only required once. The refresh token renews itself automatically afterwards.

### 6. Install as background service

```bash
teams-music --install
# or:
# make agent-install
```

**Done!** 🎉 The service is now running in the background and starts automatically on every login.

---

## 🔐 Entra ID App Registration Setup

To allow the tool to change your Teams status, you need an App Registration in Microsoft Entra ID.

### Step 1: Register the app

1. Open https://entra.microsoft.com
2. **Applications** → **App registrations** → **➕ New registration**

| Field | Value |
|---|---|
| Name | `teams-music` |
| Supported account types | **Accounts in this organizational directory only** |
| Redirect URI | *(leave empty)* |

3. Click **Register**

### Step 2: Note the IDs

On the **Overview** page you'll find:

| Field | Usage |
|---|---|
| **Application (client) ID** | → `client_id` in the config |
| **Directory (tenant) ID** | → `tenant_id` in the config |

### Step 3: Add API permission

1. **API permissions** → **➕ Add a permission**
2. **Microsoft Graph** → **Delegated permissions**
3. Search: **`Presence.ReadWrite`** → check the box
4. **Add permissions**

> ⚠️ If an orange warning symbol appears: a tenant admin needs to click **"Grant admin consent"** once.

### Step 4: Enable Public Client

1. **Authentication** → scroll down to **Advanced settings**
2. **"Allow public client flows"** → **Yes**
3. **Save**

> Without this setting, the Device Code Flow will fail.

### Step 5: No client secret needed ✅

Since we use the Device Code Flow (Public Client), no client secret is required.

---

## ⚙️ Configuration

The config is located at `~/.config/teams-music/config.yaml`:

```yaml
# Microsoft Entra ID App Registration
azure:
  client_id: "YOUR-CLIENT-ID"
  tenant_id: "YOUR-TENANT-ID"

# Status message
status:
  # Available placeholders: {{.Name}}, {{.Artist}}, {{.Album}}
  template: "🎵 {{.Name}} – {{.Artist}}"
  # Expiry time in minutes (status disappears automatically)
  expiry_minutes: 10
  # Clear status when music is paused/stopped
  clear_on_pause: true
  # Timezone for the expiry calculation
  timezone: "W. Europe Standard Time"

# Runtime behavior
behavior:
  # Wait time in seconds before updating status
  # (prevents spam when skipping tracks quickly)
  debounce_seconds: 10
  # Log level: debug, info, warn, error
  log_level: "info"
  # Log file (empty = stdout only / LaunchAgent logs)
  # log_file: "~/Library/Logs/teams-music.log"
```

### Template Examples

| Template | Result |
|---|---|
| `🎵 {{.Name}} – {{.Artist}}` | 🎵 Bohemian Rhapsody – Queen |
| `🎧 {{.Artist}}: {{.Name}}` | 🎧 Queen: Bohemian Rhapsody |
| `Now Playing: {{.Name}} ({{.Album}})` | Now Playing: Bohemian Rhapsody (A Night at the Opera) |

---

## 🛠️ CLI Flags

| Flag | Description |
|---|---|
| `--init` | Creates an example config at `~/.config/teams-music/config.yaml` |
| `--config <path>` | Uses an alternative config file |
| `--install` | Installs the macOS LaunchAgent (autostart on login) |
| `--uninstall` | Uninstalls the LaunchAgent |
| `--status` | Shows the LaunchAgent status |

---

## 🖥️ Service Management

| Action | Command | Makefile shortcut |
|---|---|---|
| **Check status** | `teams-music --status` | `make agent-status` |
| **View logs** | `tail -f ~/Library/Logs/teams-music.out.log` | `make logs` |
| **Error logs** | `tail -f ~/Library/Logs/teams-music.err.log` | `make logs-err` |
| **Stop + remove service** | `teams-music --uninstall` | `make agent-uninstall` |
| **Restart service** | `launchctl kickstart -k gui/$(id -u)/de.teams-music` | — |
| **Stop only** | `launchctl bootout gui/$(id -u)/de.teams-music` | — |
| **Start again** | `launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/de.teams-music.plist` | — |

---

## 🔨 Build from Source

If you want to build the binary yourself:

### Prerequisites

- Go 1.22+
- Xcode Command Line Tools (`xcode-select --install`)

### Build

```bash
make build
# → ./build/teams-music
```

### Makefile Targets

| Target | Description |
|---|---|
| `make build` | Build binary to `./build/teams-music` |
| `sudo make install` | Copy binary to `/usr/local/bin/` |
| `sudo make uninstall` | Uninstall LaunchAgent and remove binary from `/usr/local/bin/` |
| `make init` | Create example config |
| `make agent-install` | Install LaunchAgent (without sudo!) |
| `make agent-uninstall` | Uninstall LaunchAgent |
| `make agent-status` | Show LaunchAgent status |
| `make logs` | Follow stdout logs |
| `make logs-err` | Follow stderr logs |
| `make clean` | Clean build directory |

> ⚠️ **Important:** `sudo make install` and `make agent-install` must be run **separately**. The LaunchAgent must **not** be installed with sudo.

---

## 🏗️ Architecture

```
Apple Music
    │
    │  com.apple.Music.playerInfo (NSDistributedNotificationCenter)
    ▼
┌──────────────┐
│ Music Watcher│ → TrackInfo Events (event-based, no polling)
└──────┬───────┘
       │
       ▼
┌──────────────┐
│  Debouncer   │ → Pause/Stop: forward immediately
│  (3s delay)  │ → Playing: wait 3s, send only last track
│              │ → Same track: ignore
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Status       │ → Playing: setStatusMessage("🎵 Track – Artist")
│ Updater      │ → Pause/Stop: clearStatusMessage()
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Graph API    │ → POST /users/{id}/presence/setStatusMessage
│ Client       │ → Retry on 429 (throttling)
│              │ → Auto-refresh on 401 (token expired)
└──────────────┘
```

---

## 📁 Project Structure

```
teams-music/
├── cmd/teams-music/
│   └── main.go                    # Entrypoint + CLI flags
├── internal/
│   ├── auth/
│   │   ├── auth.go                # TokenManager
│   │   ├── devicecode.go          # Device Code Flow + Token Refresh
│   │   └── tokenstore.go          # Token persistence (~/.config/teams-music/token.json)
│   ├── config/
│   │   └── config.go              # Load YAML config + defaults + validation
│   ├── daemon/
│   │   ├── debouncer.go           # Debounce logic
│   │   ├── launchagent.go         # LaunchAgent install/uninstall/status
│   │   └── service.go             # Orchestration of all modules
│   ├── musicwatcher/
│   │   ├── callback.go            # CGo callback (Go ← ObjC)
│   │   ├── initial.go             # Initial track query via osascript
│   │   ├── types.go               # TrackInfo, PlayerState
│   │   ├── watcher.go             # Watcher lifecycle + RunLoop
│   │   ├── watcher_bridge.h       # C header
│   │   └── watcher_bridge.m       # ObjC bridge (NSDistributedNotificationCenter)
│   └── teams/
│       ├── client.go              # Graph API HTTP client + retry
│       ├── presence.go            # setStatusMessage / clearStatusMessage / getPresence
│       └── types.go               # Request/response types
├── configs/
│   └── config.example.yaml        # Example config
├── build/
│   └── teams-music                # Pre-compiled binary (macOS arm64)
├── Makefile
├── go.mod
├── go.sum
└── README.md
```

---

## 📂 Files After Installation

| File | Description |
|---|---|
| `/usr/local/bin/teams-music` | Binary |
| `~/.config/teams-music/config.yaml` | Configuration |
| `~/.config/teams-music/token.json` | OAuth token (chmod 0600) |
| `~/Library/LaunchAgents/de.teams-music.plist` | LaunchAgent |
| `~/Library/Logs/teams-music.out.log` | Stdout log |
| `~/Library/Logs/teams-music.err.log` | Stderr log |

---

## 🐛 Troubleshooting

### "AADSTS7000218: The request body must contain 'client_assertion' or 'client_secret'"

→ **"Allow public client flows"** is not enabled.  
Fix: Entra ID → App Registration → Authentication → Advanced Settings → **Allow public client flows** → **Yes**

### "AADSTS65001: The user or administrator has not consented"

→ **Admin consent** is missing.  
Fix: A tenant admin needs to click **"Grant admin consent"** under API Permissions.

### Device Code Flow doesn't appear (LaunchAgent)

→ The agent cannot open a browser. You need to run `teams-music` manually in a terminal **once** to complete the Device Code Flow. The token renews automatically after that.

### "Bootstrap failed: 125: Domain does not support specified action"

→ The agent has already been automatically loaded by macOS.  
Fix: Check `teams-music --status` – it's likely already running.

### Status doesn't change in Teams

→ It can take up to **2–3 minutes** for Teams to display the status change (Teams uses internal polling).  
→ Check logs: `tail -f ~/Library/Logs/teams-music.out.log`

### macOS Tahoe (macOS 26): AppleScript `current track` bug

→ On macOS Tahoe, `current track` only works for downloaded songs. This tool uses `NSDistributedNotificationCenter` instead, which is **not affected**.

### Token expired / refresh fails

→ Delete the token file and re-authenticate:

```bash
rm ~/.config/teams-music/token.json
teams-music    # starts Device Code Flow again
```

### Full uninstall

```bash
sudo make uninstall
rm -rf ~/.config/teams-music
```

---

## 🔒 Security

- **Delegated Permissions**: The tool can only change **your own** status – not other users'
- **Device Code Flow**: No client secret needed, no secret stored on disk (except the token)
- **Token file**: Stored with `chmod 0600` (only your user can read it)
- **No network listener**: The tool does not open any port or accept incoming connections

---

## 📄 License

MIT License – see [LICENSE](LICENSE)

---

## 🤝 Contributing

1. Create a fork
2. Feature branch: `git checkout -b feature/my-feature`
3. Commit changes
4. Open a Pull Request

---

*Built with ❤️ and 🎵 on macOS with the help of Claude*
