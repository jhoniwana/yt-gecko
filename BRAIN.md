# yt-gecko - Project Brain

Organizational master document. Update this file whenever new decisions,
requirements, or findings appear so nothing gets lost.

Tagline: "Fast. Lightweight. Sticks to your terminal."

---

## 1. Overview

- Name: yt-gecko
- Language: Go (single static binary, max Linux compatibility)
- Goal: Terminal client for YouTube + YouTube Music combining:
  - Easy visual login (real browser cookies, no config needed)
  - Native speed (no frame-accurate ASCII rendering of video)
  - Pure streaming (no disk downloads, plays from stream in RAM)
  - Ephemeral (no cache, everything deleted on exit)
  - Your algorithm (subscriptions, history, mixes)

## 2. Why Go

- Single static binary, no runtime needed
- Works on any Linux distro out of the box
- Cross-compiles easily (Linux, macOS, Windows)
- Fast startup and execution
- Easy distribution (one executable)

## 3. Architecture

### Layer 1: Backend (data & auth)
- Extractor: yt-dlp as subprocess (most reliable). mpv's own bundled
  extractor is stale and produces 403 URLs, so yt-gecko resolves streams
  itself and hands mpv a direct stream URL (see section 6 findings).
- Auth: reuse browser cookies via `yt-dlp --cookies-from-browser`. Cookies
  are handled by yt-dlp inside the subprocess; never written or printed by us.
- Streaming: direct stream URLs, NEVER write files to disk.
- Ephemeral: everything in RAM (/tmp temp dir), deleted on exit.

### Layer 2: Frontend (TUI)
- Framework: Bubble Tea (github.com/charmbracelet/bubbletea)
- Styling: Lip Gloss (github.com/charmbracelet/lipgloss)
- Search interaction: '/' opens the input prompt, Enter searches.
- Images: kitty-graphics (github.com/edouardparis/kitty-graphics) for
  thumbnails via Kitty/Sixel protocol (LATER phase, not in v0.1)

### Layer 3: Playback
- Engine: mpv in headless mode (--no-terminal, no MPI window)
- IPC: Unix socket for bidirectional control (pause, volume, position)
- Pure streaming: mpv plays directly from the stream, no disk buffer
- Ephemeral config: --cache=no, --save-position-on-quit=no

## 4. Project structure (as built)

```
yt-gecko/
|-- BRAIN.md                    # this file
|-- README.md                   # usage + install (no emojis)
|-- LICENSE                     # MIT (c) jhoniwana
|-- go.mod / go.sum
|-- main.go                     # thin entry, wires everything
|-- Makefile                    # build / install / test / cross-compile
|-- cmd/
|   `-- root.go                 # cobra CLI (root + search), signal handling
|-- internal/
|   |-- core/
|   |   |-- core.go             # GeckoCore: temp dir, cleanup, Wait, browser auth
|   |   |-- mpv.go              # Play(Retry)/ResolveStream/Stop, mpv args
|   |   |-- ipc.go              # Client: unix-socket JSON IPC to mpv
|   |   `-- search.go           # Search/Feed via yt-dlp, result parsing, sanitizeTitle
|   |-- player/
|   |   `-- player.go           # Playback wrapper for the TUI over core.Client
|   |-- tui/
|   |   |-- app.go              # Bubble Tea model, update/view, key handling
|   |   |-- home.go             # home feed tabs, feed loading
|   |   |-- search.go           # result list rendering (YouTube-style cards)
|   |   |-- graphics.go         # kitty graphics protocol (transmit/virtual/placeholder, probe, GFX writer)
|   |   |-- thumbnail.go        # thumbnail fetching, fetch/crop/resize, preload cmd
|   |   |-- login.go            # browser login flow + openBrowser
|   |   |-- player.go           # now-playing view + progress bar
|   |   `-- styles.go           # Lip Gloss styles
|   `-- auth/
|       |-- auth.go             # browser detect, cookie flags (incl. Zen via firefox:<dir>), verify
|       |-- persist.go          # remember/reuse the chosen browser across sessions
|       `-- auth_test.go        # label, detection, cookie-flags, persist tests
`-- assets/
    `-- logo.txt                # ASCII gecko logo
```

NOTE: `internal/player` and `internal/core` both touch mpv. Decision:
`internal/core` owns process lifecycle + raw IPC. `internal/player` is a thin
typed wrapper for the TUI. Do not duplicate IPC logic - core exposes it,
player consumes it.

## 5. Current status

- [x] Go module initialized (go 1.26.5, module github.com/jhoniwana/yt-gecko)
- [x] internal/core: search, stream resolution, mpv lifecycle, IPC
- [x] internal/tui: home feed view, browser login flow, search + playback (Bubble Tea + Lip Gloss)
- [x] cmd/root.go: `yt-gecko` (TUI) and `yt-gecko search` (play top hit)
- [x] Login via real browser cookies (pick browser, verify, reuse for protected content)
- [x] End-to-end playback verified: search -> resolve HLS -> mpv streams
- [x] Graceful cleanup verified: SIGINT kills mpv and removes temp dirs
- [x] Home feed: real videos browsable+playable via hashtag feeds (For you/Lofi/Gaming/News) + Subscriptions (gated on sign-in)
- [x] Real thumbnails via the kitty graphics protocol (inline images, aspect-matched 12x7 cards; plain-row fallback in non-kitty terminals)
- [x] Backspace fixed for all terminal representations (KeyBackspace/KeyDelete/KeyCtrlH + 0x7f/0x08 runes)
- [x] Login success now lands on the status screen (was stuck on login view), auto-opens the browser
- [x] Saved login: the chosen browser label persists to ~/.config/yt-gecko/browser; next runs reuse its cookies immediately with a background recheck; 'u' activates cookies even if verification fails (transient / bot-check errors)
- [x] REAL PERSONALIZED For you (2026-09-15): the For you tab now fetches
      the signed-in account's actual home feed. Below innerTube this needed two
      things beyond FEsubscriptions: browseId FEwhat_to_watch (FEhome 400s,
      FEwhat_to_watch when logged out is just a guest "Home" shell with
      logged_in=0 and no lockups) and a FRESH cookie dump - jars go stale fast
      (Zen rotated SIDCC/SIDTS within ~30 min and even FEsubscriptions flipped
      to logged_in=0+empty on an old /tmp jar, then worked again with a new
      dump). The app re-dumps cookies per call, so it is always fresh.
      Signed-in path (tui/home.go loadFeed): verified -> core.HomeFeed /
      core.SubscriptionsFeed (innerTube WEB + SAPISIDHASH); anonymous -> For you
      falls back to the music hashtag and Subscriptions prompts to log in.
      Live-verified: For you = 12 personalized items (page 1 of 4, e.g. Create
      mods, AOE2, El FedeWolf) - no generic music - and Subscriptions still
      shows the account's real shelf. core.HomeFeed shares browseFeed.
- [ ] Removed boot background recheck (Init no longer calls recheckCmd): the
      orange "could not re-verify" banner fired on almost every boot because the
      yt-dlp canary hits a JS challenge, but every feed call now re-dumps fresh
      cookies lazily, so the warning only confused. 'u' remains on the login
      screen for the manual flow.
- [ ] Subscriptions / history feeds fully verified / history feeds fully verified after sign-in
- [ ] Video/audio toggle inside the TUI (m key)
- [ ] CLI flag to reuse browser cookies in `yt-gecko search` (e.g. --browser)
- [ ] Playlist / channel browsing in the TUI
- [ ] Release binaries (make build-all), tagged downloads
- [ ] Cross-machine test (macOS/Windows TUI quirks)

## 6. Key decisions, findings & gotchas

- NO emojis anywhere in code, docs, or file contents. Keep all sources pure
  ASCII (including README/BRAIN diagrams - no box-drawing glyphs).
- Clean code: small focused types, no global state, explicit error wrapping
  with `%w`, package-level doc comments only.
- Never write media to disk. Temp dirs are in /tmp and always removed via
  GeckoCore.Cleanup().
- Cookie extraction must NOT print/persist secrets. Only pass-through flags.
- Keep mpv quiet but DIAGNOSABLE: use --no-terminal but NOT --really-quiet,
  because mpv's stderr is captured into GeckoCore.mpvErr and appended to
  Wait() errors. --really-quiet silently swallows every failure reason.
- mpv's bundled youtube-dl/yt-dlp hook is unreliable on modern YouTube
  (produces 403 URLs). ALWAYS resolve via our own yt-dlp (+ follow redirects
  implicitly) and give mpv the direct stream/manifest URL.
- HLS IS THE RELIABLE PATH. YouTube's edge servers intermittently reject
  direct http format URLs with 403 (especially combined with IPv4/IPv6
  egress mismatch and no cookies). HLS manifest URLs
  (manifest.googlegv-video.com/api/manifest/hls_playlist/...) stream without
  problems. ResolveStream prefers `-f b` (combined, usually m3u8) then
  `best`/`bestaudio`/default as fallbacks.
- ffmpeg/mpv's first probe sends `Range: bytes=0-`. Some servers reject
  unbounded ranges. Fix: `--demuxer-lavf-o=initial_request_size=524288` so
  the initial request is a bounded range. Verified required for playback.
- Reconnect flags must use the modern names: `--stream-lavf-o=reconnect=1`,
  `reconnect_streamed=1`, `reconnect_on_network_error=1`,
  `reconnect_delay_max=5`. Old `--reconnect-streamed` and `--watch-later`
  options no longer exist in mpv v0.41.
- `ytsearchmusicN:` search scheme no longer exists in yt-dlp (2026.07.04).
  YouTube Music mode for v0.1 = regular search + audio-only playback
  (--no-video). Revisit when yt-dlp restores a music search extractor.
- Play() retries failed loads up to 3 times with a fresh stream URL because
  YouTube edge servers intermittently 403. Fail-fast grace: if mpv exits
  within 5s of starting, it is treated as a load failure.
- cmd/runSearch skips unavailable results and plays the next hit instead of
  failing, because search often surfaces geo-blocked/unavailable videos.
- Signal safety: install the SIGINT/SIGTERM handler BEFORE search/resolve so
  Ctrl+C at any point cleans up cleanly (kills mpv, removes temp dir). This
  was a real leak: SIGINT during resolve used to leave orphan mpv + dirs.
- mpv exit status 2 = load/playback error; treat signaled exits as a clean
  user stop (playbackError helper in cmd).
- "Login with your browser" = reuse the real browser's cookies via
  `--cookies-from-browser`. VerifyBrowserCookies validates the cookie
  database by extracting a tiny public video; it CANNOT prove the account
  is signed in (yt-dlp 2026.07.04 has no working auth-only canary URL;
  feed/subscriptions, list=LL/WL/FL/HL, `?disable_polymer=1`,
  `youtubetab:client=mweb` and raw-cookie HTML scraping all yield zero
  entries - verified live on 2026-09-15). Cookies still unlock sign-in
  protected playback: ResolveStream passes them to yt-dlp ALWAYS, and the
  Subscriptions tab uses them via innerTube (see feed.go).
  The chosen browser persists (auth.SaveBrowser) to
  `$XDG_CONFIG_HOME/yt-gecko/browser`; boot loads it instantly (no verify
  step), re-verifies in the background, and the status screen's 'u' key
  activates the cookies anyway when verification fails so transient
  bot-check errors never block playback. Press 'o' opens the real browser
  for the visual sign-in step.
- Zen Browser support: Zen is a Firefox fork with no cookie engine of its
  own. CookiesFromBrowserArgs maps BrowserZen to
  `--cookies-from-browser firefox:<profile-root>` with the root at
  `~/.zen` (profiles.ini lives there), so yt-dlp decrypts Zen's NSS cookies
  exactly like Firefox's. Verified live (2026-09-15): Zen appears in the
  login list, verifies, and persists as `zen`.
- Homepage HTML is useless as a feed source: off the shelf (~890KB) it
  contains ytInitialData but ZERO videoRenderer nodes - the real home
  shelf is built by JS at runtime, so there is nothing to parse without a
  headless browser. The reliable no-JS feed sources that DO extract are
  hashtag pages (`https://www.youtube.com/hashtag/<tag>`); *feed* uses
  those, with tag = music, lofi, gaming, news. The signed-in Subscriptions
  shelf needed innerTube (see core/feed.go notes above).
- Thumbnails: `https://i.ytimg.com/vi/<id>/maxresdefault.jpg` is a plain JPEG
  (stdlib image/jpeg, no cgo), true 16:9 (1280x720). Fallback hqdefault.jpg if
  maxres 404s (older/short videos). Cached in RAM per video id and painted via
  the kitty graphics protocol as an inline image; ASCII row list fallback in
  terminals without kitty graphics.
- The old ANSI half-block thumbnails (built at runtime as `string(rune(0x2580))`
  from hqdefault, only under cursor, width>=110) were fully replaced by kitty
  graphics inline images; the rune() trick is no longer used in the TUI paths.
- Emoji stripping: titles are scrubbed by core.sanitizeTitle - drops
  astral emoji blocks (U+1F000-U+1FFFF), misc symbols (U+2600-U+27BF),
  ZWJ (U+200D) and variation selector U+FE0F, plus control chars. Keeps
  layout stable and satisfies the no-emoji project rule even for titles
  like ".. (crying emoji)..".
- Backspace arrives in several forms depending on terminal/keymap: as
  tea.KeyBackspace, tea.KeyDelete, tea.KeyCtrlH, or as DEL (0x7f) /
  backspace (0x08) bytes inside KeyRunes. updateInput handles ALL of
  these. Verification trap: tmux `paste-buffer` wraps bytes in bracketed
  paste, and `send-keys -l` echoes control chars verbatim - neither
  simulates a real keypress. Use `tmux send-keys BSpace` (the actual
  backspace key) or the unit test.
- Layout guardrails: the home art must stay <= 8 lines so the home screen
  fits 24-row terminals; long status messages are word-wrapped to 70 chars
  so the box hugs the text instead of overflowing the pane.
- kitty graphics = REAL inline thumbnails, replacing the ANSI half-block art:
  - Probe: send `\x1b_Gi=31,s=1,v=1,f=24,q=1;C\x1b\\\x1b_Gc;OK\x1b\\` under the
    raw termios config (no ICANON, VMIN=1, nonblocking read of 32ms) so kitty's
    async `\x1b_Gi=31;OK\x1b\\` reply reaches us before mode switch; sans
    graphics replies nothing. GFX transport = stdout raw write; stdlib does not
    parse any reply, kitty output is the only consumer (impossible to end up in
    the TUI text).
  - Transmit a PNG: `\x1b_Ga=t,i=<id>,f=100,q=2,m=[01];base64...\x1b\\`
    (chunked at 4096 bytes/send when payload exceeds it; base64 padding kept).
  - Virtual placement: `\x1b_Ga=p,U=1,i=<id>,c=<cols>,r=<rows>,q=2,C=1\x1b\\`
  - Drop a placeholder: `\x1b[38;5;<id%256>m` + `\U0010EEEE\x1b[39m` per cell.
    EACH cell MUST carry row/col combining marks in order
    (\U0305=\U0305..\U0357 = rows/cols 0..15): without them the cell inherits
    the previous cell's row/column, so the image literally gets sliced into
    strips (verified on screen). First mark = row, second = column.
  - Clip with a transmit id that reuses the same id, cap at 1 (C=1) guard then
    `\x1b_Ga=d,d=A\x1b\\` on cleanup.
  - gfxWriter MUST implement Fd() (real os.Stdout fd), Read() (io.EOF session)
    and Close() (no-op) so Bubble Tea's resize detection works; WithOutput with
    a plain writer makes tea think the output is not a terminal, m.width goes 0
    and the whole layout collapses.
  - Thumbnails: `maxresdefault.jpg` (1280x720, TRUE 16:9, no letterbox bars)
    with `hqdefault.jpg` fallback; center-crop to the grid aspect then resize.
    hqdefault is 4:3 with black bars, so with the 12x7 grid kitty letterboxes it
    and the image paints only ~46% of the block - looks "sliced". Cards are
    12 cols x 7 rows (aspect ~16:9); the whole visible page (3 cards) preloads
    async via thumbCmd(), deduped in m.thumbs/m.thumbBusy, so the shelf fills
    like YouTube instead of only under the cursor.
  - Gates: kitty + width >= 60 -> cards; non-kitty width >= 110 -> cards;
    below that a compact row list. pageSize 3 when cards active, 8 otherwise.
- Emoji stripping: titles are scrubbed by core.sanitizeTitle
- Verified live (tmux pty): home feed renders + plays, thumbnails render,
  login picks+verifies Firefox, top bar flips to "Firefox", search+play
  works, backspace deletes, q cleanly exits with zero leftover mpv or temp
  dirs.

## 7. CLI surface (current)

```text
yt-gecko                 # open interactive TUI
yt-gecko search <query>  # search + play first working hit
yt-gecko search -a       # audio-only (music mode)
yt-gecko search -n <N>   # number of search results (default 10)
```

Planned: `subs`, `history`, `music` subcommands (need auth feeds).

## 8. Keyboard shortcuts (interactive mode)

Home screen (feed browser):

- j / k or arrows - move the cursor
- Left / Right (or [ / ]) - switch feed tab
- Enter - play the selected video
- / - jump straight to search
- a - account / login screen
- q - quit

Results / playlist playback:

- j / k / arrows - navigate results
- Enter - play the selected result
- Space - pause / resume
- / - new search
- h - back to home

Login screen:

- j / k - pick a browser
- Enter - verify its cookies (success auto-opens the browser)
- o - open the real browser to sign in to YouTube
- u - (on the status screen) use the browser's cookies anyway even if the
      quick verification failed
- Esc - back

Music tab (YouTube Music home, shelf grid):

- j / k - move a row (the grid column count)
- h / l or arrows - move one card
- Enter or click - play a song, or expand a mix/album/playlist into the queue
- Esc / Backspace - leave a browsed page (mood/playlist) back to the home
- / - search, a - account, v - quality, q - quit

Music player (audio-only, no window):

- Space / p - pause or resume
- n / p - next or previous track in the queue
- Left / Right - seek 5s
- - / + - volume down/up
- j / k - move the queue cursor, Enter - play the selected row
- Click the seek bar, transport buttons, volume bar or any queue row
- v - quality, / - search, h / Esc - back to the list

Global mouse:

- Wheel - move the selection; Ctrl+Wheel - zoom the interface
- Hover highlights every clickable element (header icons, tabs, cards,
  queue rows)

## 9. Resource targets

| Mode  | RAM    | CPU  | Disk |
|-------|--------|------|------|
| Audio | ~50MB  | ~2%  | 0    |
| Video | ~150MB | ~10% | 0    |

Chrome comparison: ~20x less RAM, ~10x less CPU, 0 bytes disk vs GBs cache.

## 10. Dependencies (final)

- github.com/charmbracelet/bubbletea v1.3.10 (TUI)
- github.com/charmbracelet/lipgloss v1.1.0 (styling)
- github.com/charmbracelet/x/ansi (line clamping in View)
- github.com/spf13/cobra v1.10.2 (CLI)
- yt-dlp (external binary, subprocess; standalone build is self-contained)
- mpv (external binary, subprocess; audio-only mode needs no video output)
- kitty graphics: implemented in-tree (no external kitty-graphics dependency)

## 11. YouTube Music + music player (this iteration)

### Music home (WEB_REMIX)
- YouTube Music uses its OWN innerTube client and key: clientName WEB_REMIX
  (name header 67, version 1.20260913.16.00), origin https://music.youtube.com
  and key AIzaSyC9XL3ZjWddXya6X74dJoCTL-WEYFDNX30 (scraped from the music
  homepage; the main-site key returns a reduced home).
- CRITICAL: SAPISIDHASH must be signed with the request origin. Signing music
  requests with https://www.youtube.com makes YouTube Music treat the session
  as logged out (`"logged_in": "0"`), which silently returns generic shelves
  and empty liked/library pages. `sapisidHashFor(sapisid, origin)` fixed it.
- The home is paginated: the first response carries a few carousels plus a
  sectionListRenderer continuation; following it (up to musicHomePages=4)
  yields the full shelf list (Listen again, Quick picks, Your daily discover,
  Forgotten favorites, Fresh finds, Music videos for you, mood shelves...).
- Library shelves are fetched explicitly: FEmusic_liked_playlists (a
  gridRenderer of the user's playlists) and FEmusic_liked_videos (liked songs;
  first song's cover becomes the "Liked Music" auto-playlist card).
- Parsers handle musicTwoRowItemRenderer, musicResponsiveListItemRenderer and
  gridRenderer sections, preserving shelf order by walking the section arrays
  structurally (map walking would scramble the carousels).
- Playlists/mixes/albums are VL-prefixed on the wire; the parser strips VL for
  display and musicBrowseWireID re-adds it for browsing.
- Mood chips carry browseId FEmusic_home plus a base64 params blob; browsing a
  mood means re-requesting the home with that params.

### Music player (modeMusic)
- Playing from the Music tab is audio-only: mpv --no-video --keep-open=yes, so
  no window ever opens and the process stays alive at EOF so the player can
  advance the queue (eof-reached property polled every second).
- Player UI: square cover art (16x8 cells, real kitty image cropped 1:1 or
  half-block art), clickable seek bar, clickable transport (prev/play/next)
  and clickable volume bar; every control also has a key binding.
- Queue: playlists expand via MusicBrowse + FlattenShelves. Wide queues use a
  three-row row (4x2 mini cover, title, artist+duration); narrow/short queues
  use one row per track with a 2x1 mini cover so many more tracks fit.
- Short windows (<30 rows) drop the cover and the key hint from the player box
  so the queue keeps most of the height.
- The mini covers are half-block art rendered from the cached decoded image
  and invalidated when the real thumbnail arrives.

### Layout gotcha (cover art "distortion")
- The app style pads two columns per side, so the content area is width-4.
  A frame wider than that is WRAPPED by lipgloss, which shreds boxes and
  splits the cover art into strips (looked like resize distortion). The player
  now sizes its panels to fit and View clamps every line with ansi.Truncate
  before rendering.

## 12. Login & cookies (this iteration)

- Supported browsers now cover the Chromium family and Firefox forks: Firefox,
  Zen, LibreWolf, Waterfox, Floorp, Chrome, Chromium, Brave, Vivaldi, Opera,
  Edge, Whale. Each has standard, snap and flatpak profile candidates.
- Detection lists profile-ready browsers first, with the desktop's default
  browser (xdg-settings, falling back to mimeapps.list) at the very top.
- yt-dlp cookie extraction timeouts raised to 90s: a cold browser profile can
  take longer than 30s, and the context kill surfaced as "signal killed".
- browsh (terminal browser login) was tested and discarded: Google blocks the
  headless Firefox it drives ("This browser or app may not be secure"), and a
  visible GUI Firefox is required for the login to pass.

## 13. Next steps

1. Music home keyboard access for the mood chips (a "c" menu like the quality
   selector) - the chips are mouse-only today.
2. Playlist pagination in the queue (currently the first ~100 tracks).
3. Build release binaries via make build-all and tag on GitHub.
4. Portable distribution: single Go binary with embedded yt-dlp (39 MB
   standalone) is easy; mpv is the heavy part (108 MB AppImage or 204 shared
   libraries), so decide between embedded mpv extraction on first run, an
   AppImage bundle, or requiring mpv from the distro.
5. Test on additional kitty-graphics terminals (Ghostty, Konsole, WezTerm).
6. Revisit `feed/subscriptions` extraction when yt-dlp ships a working
   signed-in tab extractor (hard YouTube-side block today).
## 14. Open questions

- Merge mode ("open in browser while audio keeps playing"): which key binding?
- Should the thumbnail be shown on demand full-size (e.g. zoom on Enter)?
- Distribution shape: one big self-contained binary (embedded yt-dlp + mpv),
  a portable tarball, or an AppImage? Trade-off documented in section 13.
- Music: gapless/next-track preloading, and whether to cache stream URLs
  between queue tracks to cut the yt-dlp resolve time.