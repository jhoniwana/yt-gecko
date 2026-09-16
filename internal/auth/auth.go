package auth

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Browser identifies a supported browser for cookie extraction.
type Browser string

// Supported browsers for yt-dlp cookie extraction. The Chromium family and
// Firefox itself have native yt-dlp support; Firefox forks (Zen, LibreWolf,
// Waterfox, Floorp) reuse the Firefox extraction engine pointed at their own
// profile root.
const (
	BrowserFirefox   Browser = "firefox"
	BrowserZen       Browser = "zen"
	BrowserLibreWolf Browser = "librewolf"
	BrowserWaterfox  Browser = "waterfox"
	BrowserFloorp    Browser = "floorp"
	BrowserChrome    Browser = "chrome"
	BrowserChromium  Browser = "chromium"
	BrowserBrave     Browser = "brave"
	BrowserVivaldi   Browser = "vivaldi"
	BrowserOpera     Browser = "opera"
	BrowserEdge      Browser = "edge"
	BrowserWhale     Browser = "whale"
)

// allBrowsers lists detection priority order: Firefox family first (the
// extraction is the most reliable), then Chromium derivatives.
var allBrowsers = []Browser{
	BrowserFirefox,
	BrowserZen,
	BrowserLibreWolf,
	BrowserWaterfox,
	BrowserFloorp,
	BrowserChrome,
	BrowserChromium,
	BrowserBrave,
	BrowserVivaldi,
	BrowserOpera,
	BrowserEdge,
	BrowserWhale,
}

// Label returns the human-readable name of a browser.
func (b Browser) Label() string {
	switch b {
	case BrowserFirefox:
		return "Firefox"
	case BrowserZen:
		return "Zen"
	case BrowserLibreWolf:
		return "LibreWolf"
	case BrowserWaterfox:
		return "Waterfox"
	case BrowserFloorp:
		return "Floorp"
	case BrowserChrome:
		return "Google Chrome"
	case BrowserChromium:
		return "Chromium"
	case BrowserBrave:
		return "Brave"
	case BrowserVivaldi:
		return "Vivaldi"
	case BrowserOpera:
		return "Opera"
	case BrowserEdge:
		return "Microsoft Edge"
	case BrowserWhale:
		return "Naver Whale"
	}
	if s := string(b); len(s) > 0 {
		return strings.ToUpper(s[:1]) + s[1:]
	}
	return "Unknown"
}

// isFirefoxFork reports whether the browser is a Firefox derivative that
// yt-dlp does not know by name, so its cookies must be read through the
// `firefox:<profile-root>` selector.
func (b Browser) isFirefoxFork() bool {
	switch b {
	case BrowserZen, BrowserLibreWolf, BrowserWaterfox, BrowserFloorp:
		return true
	}
	return false
}

// CookiesFromBrowserArgs returns the yt-dlp flag that reuses login cookies
// from the given browser. It returns nil when no browser is configured or its
// profile directory is missing. Cookies are handled by yt-dlp itself and never
// written or printed here.
func CookiesFromBrowserArgs(b Browser) []string {
	if b == "" {
		return nil
	}
	if b.isFirefoxFork() {
		dir := b.profileDir()
		if dir == "" {
			return nil
		}
		return []string{"--cookies-from-browser", "firefox:" + dir}
	}
	return []string{"--cookies-from-browser", string(b)}
}

// profileDir returns the first existing profile root for the browser, or ""
// when it is not installed. Candidates cover the standard XDG location plus
// the snap and flatpak sandboxes used on Linux distributions.
func (b Browser) profileDir() string {
	for _, dir := range b.profileCandidates() {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			return dir
		}
	}
	return ""
}

// HasProfile reports whether a usable profile directory exists for the
// browser. Detection uses this to separate "ready to use" browsers from
// browsers that are merely installed.
func (b Browser) HasProfile() bool {
	return b.profileDir() != ""
}

// profileCandidates lists every place the browser may keep its profile on
// Linux, newest sandbox layouts included.
func (b Browser) profileCandidates() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(home, ".config")
	}
	join := func(parts ...string) string {
		return filepath.Join(append([]string{home}, parts...)...)
	}
	switch b {
	case BrowserFirefox:
		return []string{
			join(".mozilla", "firefox"),
			join("snap", "firefox", "common", ".mozilla", "firefox"),
			join(".var", "app", "org.mozilla.firefox", ".mozilla", "firefox"),
		}
	case BrowserZen:
		return []string{
			join(".zen"),
			join(".var", "app", "app.zen_browser.zen", ".zen"),
		}
	case BrowserLibreWolf:
		return []string{
			join(".librewolf"),
			join(".var", "app", "io.gitlab.librewolf-community", ".librewolf"),
		}
	case BrowserWaterfox:
		return []string{
			join(".waterfox"),
			join(".var", "app", "net.waterfox.waterfox", ".waterfox"),
		}
	case BrowserFloorp:
		return []string{
			join(".floorp"),
			join(".var", "app", "one.ablaze.floorp", ".floorp"),
		}
	case BrowserChrome:
		return []string{
			filepath.Join(cfg, "google-chrome"),
			join(".var", "app", "com.google.Chrome", "config", "google-chrome"),
		}
	case BrowserChromium:
		return []string{
			filepath.Join(cfg, "chromium"),
			join("snap", "chromium", "common", "chromium"),
			join(".var", "app", "org.chromium.Chromium", "config", "chromium"),
		}
	case BrowserBrave:
		return []string{
			filepath.Join(cfg, "BraveSoftware", "Brave-Browser"),
			join("snap", "brave", "common", ".config", "BraveSoftware", "Brave-Browser"),
			join(".var", "app", "com.brave.Browser", "config", "BraveSoftware", "Brave-Browser"),
		}
	case BrowserVivaldi:
		return []string{
			filepath.Join(cfg, "vivaldi"),
			join(".var", "app", "com.vivaldi.Vivaldi", "config", "vivaldi"),
		}
	case BrowserOpera:
		return []string{
			filepath.Join(cfg, "opera"),
			join("snap", "opera", "current", ".config", "opera"),
			join(".var", "app", "com.opera.Opera", "config", "opera"),
		}
	case BrowserEdge:
		return []string{
			filepath.Join(cfg, "microsoft-edge"),
			join(".var", "app", "com.microsoft.Edge", "config", "microsoft-edge"),
		}
	case BrowserWhale:
		return []string{
			filepath.Join(cfg, "naver-whale"),
			join(".var", "app", "com.naver.Whale", "config", "naver-whale"),
		}
	}
	return nil
}

// binaryNames lists the executable names the browser may ship as, used to
// detect installed browsers that have no profile yet.
func (b Browser) binaryNames() []string {
	switch b {
	case BrowserFirefox:
		return []string{"firefox"}
	case BrowserZen:
		return []string{"zen-browser", "zen"}
	case BrowserLibreWolf:
		return []string{"librewolf"}
	case BrowserWaterfox:
		return []string{"waterfox"}
	case BrowserFloorp:
		return []string{"floorp"}
	case BrowserChrome:
		return []string{"google-chrome", "google-chrome-stable"}
	case BrowserChromium:
		return []string{"chromium", "chromium-browser"}
	case BrowserBrave:
		return []string{"brave", "brave-browser"}
	case BrowserVivaldi:
		return []string{"vivaldi", "vivaldi-stable"}
	case BrowserOpera:
		return []string{"opera"}
	case BrowserEdge:
		return []string{"microsoft-edge", "microsoft-edge-stable"}
	case BrowserWhale:
		return []string{"naver-whale", "whale"}
	}
	return nil
}

// Installed reports whether the browser's executable is on PATH.
func (b Browser) Installed() bool {
	for _, name := range b.binaryNames() {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

// DetectBrowsers lists the browsers usable on this system. Browsers with a
// profile come first, with the desktop's default browser at the very top so
// the most likely account is preselected; installed browsers without a
// profile are appended (the login screen asks the user to sign in there
// first).
func DetectBrowsers() []Browser {
	var ready, installed []Browser
	for _, b := range allBrowsers {
		switch {
		case b.HasProfile():
			ready = append(ready, b)
		case b.Installed():
			installed = append(installed, b)
		}
	}
	if def := DefaultBrowser(); def != "" {
		ready = moveFirst(ready, def)
		installed = moveFirst(installed, def)
	}
	return append(ready, installed...)
}

// moveFirst moves b to the front of the slice when present.
func moveFirst(list []Browser, b Browser) []Browser {
	for i, cur := range list {
		if cur == b {
			out := make([]Browser, 0, len(list))
			out = append(out, b)
			out = append(out, list[:i]...)
			out = append(out, list[i+1:]...)
			return out
		}
	}
	return list
}

// DefaultBrowser returns the desktop's default web browser, or "" when it
// cannot be determined or is not supported.
func DefaultBrowser() Browser {
	if name := xdgDefaultBrowser(); name != "" {
		if b := browserFromDesktopName(name); b != "" {
			return b
		}
	}
	return browserFromMimeApps()
}

// xdgDefaultBrowser asks xdg-settings for the default web browser (a
// .desktop file name like "firefox.desktop").
func xdgDefaultBrowser() string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "xdg-settings", "get", "default-web-browser").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// browserFromMimeApps reads the x-scheme-handler/https default from the
// standard mimeapps.list files, which works even without xdg-settings.
func browserFromMimeApps() Browser {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(home, ".config")
	}
	paths := []string{
		filepath.Join(cfg, "mimeapps.list"),
		filepath.Join(home, ".local", "share", "applications", "mimeapps.list"),
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if b := browserFromMimeAppsData(string(data)); b != "" {
			return b
		}
	}
	return ""
}

// browserFromMimeAppsData finds the default https handler in a mimeapps.list
// body. The [Default Applications] section is scanned first; any later
// x-scheme-handler/https entry is used as a fallback.
func browserFromMimeAppsData(data string) Browser {
	var fallback Browser
	inDefaults := false
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inDefaults = strings.EqualFold(line, "[Default Applications]")
			continue
		}
		if !strings.HasPrefix(line, "x-scheme-handler/https=") {
			continue
		}
		value := strings.TrimPrefix(line, "x-scheme-handler/https=")
		value = strings.SplitN(value, ";", 2)[0]
		if b := browserFromDesktopName(value); b != "" {
			if inDefaults {
				return b
			}
			if fallback == "" {
				fallback = b
			}
		}
	}
	return fallback
}

// browserFromDesktopName maps a .desktop file name or desktop entry to a
// supported browser. Order matters: more specific names are checked first so
// "google-chrome" does not fall into the Chromium bucket.
func browserFromDesktopName(name string) Browser {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "librewolf"):
		return BrowserLibreWolf
	case strings.Contains(n, "waterfox"):
		return BrowserWaterfox
	case strings.Contains(n, "floorp"):
		return BrowserFloorp
	case strings.Contains(n, "zen"):
		return BrowserZen
	case strings.Contains(n, "firefox"):
		return BrowserFirefox
	case strings.Contains(n, "brave"):
		return BrowserBrave
	case strings.Contains(n, "vivaldi"):
		return BrowserVivaldi
	case strings.Contains(n, "opera"):
		return BrowserOpera
	case strings.Contains(n, "edge"):
		return BrowserEdge
	case strings.Contains(n, "whale"):
		return BrowserWhale
	case strings.Contains(n, "chromium"):
		return BrowserChromium
	case strings.Contains(n, "chrome"):
		return BrowserChrome
	}
	return ""
}

// ytdlpPath is the yt-dlp binary to run; the portable build sets the bundled
// copy here. Empty means PATH.
var ytdlpPath string

// SetYTDLPPath points cookie verification at a specific yt-dlp binary.
func SetYTDLPPath(p string) { ytdlpPath = p }

// jsRuntime is an optional JavaScript runtime (QuickJS) for yt-dlp.
var jsRuntime string

// SetJSRuntime points yt-dlp at a bundled JavaScript runtime.
func SetJSRuntime(p string) { jsRuntime = p }

// ytdlp returns the yt-dlp binary to run.
func ytdlp() string {
	if ytdlpPath != "" {
		return ytdlpPath
	}
	return "yt-dlp"
}

// VerifyBrowserCookies checks that yt-dlp can load the browser's cookies by
// dumping them to a transient jar and looking for a YouTube session cookie.
// This avoids probing a video, which can fail on bot checks even when the
// cookies are perfectly fine.
func VerifyBrowserCookies(b Browser) error {
	args := CookiesFromBrowserArgs(b)
	if args == nil {
		return fmt.Errorf("%s has no profile to read cookies from", b.Label())
	}
	f, err := os.CreateTemp("", "yt-gecko-verify-*.txt")
	if err != nil {
		return err
	}
	jar := f.Name()
	f.Close()
	_ = os.Remove(jar) // yt-dlp refuses to write over an existing empty file
	defer os.Remove(jar)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	args = append(args,
		"--ignore-config",
		"--no-warnings",
	)
	if jsRuntime != "" {
		args = append(args, "--js-runtimes", "quickjs:"+jsRuntime)
	}
	args = append(args,
		"--cookies", jar,
		"--simulate",
		"--skip-download",
		"--",
		"https://www.youtube.com/watch?v=jNQXAC9IVRw")

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, ytdlp(), args...)
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	data, readErr := os.ReadFile(jar)
	if readErr != nil || len(data) == 0 {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("yt-dlp could not read %s cookies: %w: %s", b.Label(), runErr, msg)
		}
		return fmt.Errorf("yt-dlp could not read %s cookies: %w", b.Label(), runErr)
	}
	if !jarHasSessionCookie(string(data)) {
		return fmt.Errorf("%s is not signed in to YouTube (no session cookie found)", b.Label())
	}
	return nil
}

// jarHasSessionCookie reports whether a Netscape cookie jar carries a YouTube
// session cookie, which is what authenticated requests need.
func jarHasSessionCookie(jar string) bool {
	for _, line := range strings.Split(jar, "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		if !strings.HasSuffix(fields[0], "youtube.com") {
			continue
		}
		switch fields[5] {
		case "SAPISID", "__Secure-3PAPISID", "SID", "__Secure-1PSID":
			if fields[6] != "" {
				return true
			}
		}
	}
	return false
}
