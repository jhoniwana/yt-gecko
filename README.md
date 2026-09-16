# yt-gecko

Fast. Lightweight. Sticks to your terminal.

A terminal client for YouTube and YouTube Music that combines the best of both worlds:

- Easy visual login (real browser cookies, no configuration needed)
- Native speed (no ASCII rendering of video)
- Pure streaming (no disk downloads, plays from the stream in RAM)
- Ephemeral (no cache, everything deleted on exit)
- Your algorithm (subscriptions, history, mixes)

## Installation

### From binary

Grab the latest release for your platform, or build from source:

```bash
wget https://github.com/jhoniwana/yt-gecko/releases/latest/download/yt-gecko-linux-amd64
chmod +x yt-gecko-linux-amd64
sudo mv yt-gecko-linux-amd64 /usr/local/bin/yt-gecko
```

### From source

```bash
git clone https://github.com/jhoniwana/yt-gecko
cd yt-gecko
make build
sudo make install
```

### System dependencies

```bash
# Linux (Debian/Ubuntu)
sudo apt install mpv

# Linux (Arch)
sudo pacman -S mpv

# macOS
brew install mpv
```

yt-dlp is also required. Install it from https://github.com/yt-dlp/yt-dlp
or via your package manager.

## Usage

```bash
# Open the interactive interface
yt-gecko

# Quick search and play the top result
yt-gecko search "lofi hip hop"

# Music mode (audio only)
yt-gecko search "bohemian rhapsody" --audio

# Limit results
yt-gecko search "jazz" --count 5
```

## Keyboard shortcuts (interactive mode)

Home screen (feed browser):

- `j` / `k` / arrows - Move the cursor
- `Left` / `Right` (or `[` / `]`) - Switch feed tab (For you / Lofi / Gaming / News / Subscriptions)
- `Enter` - Play the selected video
- `/` - Jump straight to search
- `a` - Account / login screen
- `q` - Quit
- `u` - (login status screen) use the browser's cookies anyway if verification failed

Search / results:

- `j` / `k` / arrows - Navigate results (the whole page of thumbnails loads up front)
- `Enter` - Play the selected result
- `Space` - Pause / resume
- `/` - New search
- `h` - Back to home

Login screen:

- `j` / `k` - Pick a browser
- `Enter` - Verify its cookies (success auto-opens your browser)
- `o` - Open your browser to sign in to YouTube
- `Esc` - Back

## Login

yt-gecko does not ask for a password. Press `a` on the home screen, pick the
browser you already use (Firefox, Zen, Chrome, Chromium, Brave or Edge), and
yt-gecko verifies that it can reuse its cookies. The chosen browser is
remembered, so the next launch reuses the same cookies
immediately (re-verified in the background); press `u` on the status screen to
use them anyway if the quick check fails. The cookies themselves never leave
your browser profile - only the browser name is remembered. To change account,
just log out of the browser and sign in with another one. Press `o` at any
login/logout screen to open YouTube in your browser and sign in visually.

Note: signed-in cookies unlock sign-in protected videos AND your personal
feeds: the For you tab becomes your real YouTube home (personalized
recommendations from your watch history) and the Subscriptions tab is your
subscriptions shelf - both fetched via YouTube's innerTube API with the browser
session (see `core.HomeFeed` / `core.SubscriptionsFeed`). Without a verified
login they fall back to a generic hashtag shelf and a sign-in prompt.

## Features

### Pure streaming

yt-gecko NEVER downloads files to disk. Everything plays directly from
YouTube's stream in memory.

### Home feed with thumbnails

The home screen browses real video feeds (your personalized For you plus
hashtag shelves like lofi, gaming and news when signed out, and your
Subscriptions when signed in) and plays them on Enter. In terminals with the
kitty graphics protocol (kitty, Ghostty, Konsole, ...) each card shows the
video's real thumbnail as an inline image, arranged like a YouTube shelf; other
terminals fall back to a compact row list. Search results get the same card
treatment.

### Ephemeral

Leaves no trace. On exit, all temporary files and sessions are deleted.

### Easy login

Uses cookies from your browser (Firefox/Chrome) automatically. yt-dlp
handles cookie access, so no secrets are ever written or stored by yt-gecko.

## Architecture

```
yt-gecko/
|-- BRAIN.md               # project master document (decisions, status, next steps)
|-- main.go                # thin entry point
|-- cmd/                   # cobra CLI
|   |-- root.go
|-- internal/
|   |-- core/              # engine: temp dir, mpv lifecycle, search/feeds, IPC
|   |-- player/            # playback control wrapper for the TUI
|   |-- tui/               # Bubble Tea interface (app, home, search, login, thumbs)
|   |-- auth/              # browser cookie auth
|-- assets/
    |-- logo.txt           # ASCII gecko logo
```

## Development

```bash
go mod download
go run .                       # interactive TUI mode
go run . search "lofi hip hop"
make test                      # run the test suite
make build                     # build to ./bin/yt-gecko
make build-all                 # cross-compile linux + macOS binaries
```

## Why gecko?

Geckos are small, fast, agile, and lightweight - exactly what this client
aspires to be.

## License

MIT (c) jhoniwana