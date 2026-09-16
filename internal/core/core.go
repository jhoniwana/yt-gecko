package core

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jhoniwana/yt-gecko/internal/auth"
)

// GeckoCore is the yt-gecko engine. It owns the ephemeral temp directory,
// the mpv process, and the search facilities.
type GeckoCore struct {
	tempDir    string
	socketPath string
	mpv        *exec.Cmd
	mpvDone    chan error
	mpvErr     bytes.Buffer
	browser    auth.Browser
	quality    string
}

// QualityOptions are the selectable playback qualities. "auto" lets yt-dlp
// pick the best stream that plays in one piece; the rest cap the height (and
// "audio" plays audio only).
var QualityOptions = []string{"auto", "1080p", "720p", "480p", "360p", "240p", "audio"}

// SetQuality selects the playback quality. Unknown values fall back to auto.
func (g *GeckoCore) SetQuality(q string) {
	for _, known := range QualityOptions {
		if q == known {
			g.quality = q
			return
		}
	}
	g.quality = "auto"
}

// Quality returns the active playback quality.
func (g *GeckoCore) Quality() string {
	if g.quality == "" {
		return "auto"
	}
	return g.quality
}

// NewGeckoCore creates an engine backed by a private temp directory.
func NewGeckoCore() (*GeckoCore, error) {
	tempDir, err := os.MkdirTemp("", "yt_gecko_")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	return &GeckoCore{
		tempDir:    tempDir,
		socketPath: filepath.Join(tempDir, "mpv.sock"),
	}, nil
}

// SetBrowser activates cookie reuse from a real browser for authenticated
// YouTube access (age-restricted videos, history, subscriptions). Pass "" to
// go back to anonymous access.
func (g *GeckoCore) SetBrowser(b auth.Browser) {
	g.browser = b
}

// Browser returns the browser currently used for authenticated access.
func (g *GeckoCore) Browser() auth.Browser {
	return g.browser
}

// authArgs returns the yt-dlp flags that reuse the active browser cookies.
func (g *GeckoCore) authArgs() []string {
	return auth.CookiesFromBrowserArgs(g.browser)
}

// Cleanup stops playback and removes all temporary state.
func (g *GeckoCore) Cleanup() {
	_ = g.Stop()
	if g.tempDir != "" {
		_ = os.RemoveAll(g.tempDir)
		g.tempDir = ""
	}
}

// Wait blocks until playback ends and returns its outcome. Safe to call once
// per Play; after it returns, Stop skips process reaping.
func (g *GeckoCore) Wait() error {
	if g.mpvDone == nil {
		return fmt.Errorf("no playback in progress")
	}
	err := <-g.mpvDone
	g.mpvDone = nil
	if err != nil {
		if msg := strings.TrimSpace(g.mpvErr.String()); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
	}
	return err
}
