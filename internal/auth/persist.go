package auth

import (
	"os"
	"path/filepath"
	"strings"
)

// configBase returns yt-gecko's per-user config directory, creating it.
func configBase() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	dir := filepath.Join(base, "yt-gecko")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

const browserFile = "browser"

// SaveBrowser persists the cookie browser name so later sessions reuse the
// same browser cookies immediately. The cookies themselves stay inside the
// browser profile; only the browser label is stored.
func SaveBrowser(b Browser) error {
	if b == "" {
		return nil
	}
	dir, err := configBase()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, browserFile), []byte(string(b)), 0o600)
}

// LoadBrowser returns the browser saved by an earlier session, or "" when
// none was saved (or it is no longer a supported browser).
func LoadBrowser() Browser {
	dir, err := configBase()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, browserFile))
	if err != nil {
		return ""
	}
	b := Browser(strings.TrimSpace(string(data)))
	for _, known := range allBrowsers {
		if b == known {
			return b
		}
	}
	return ""
}

const qualityFile = "quality"

// SaveQuality persists the selected playback quality so it sticks across
// sessions. Unknown values are ignored (the engine falls back to auto).
func SaveQuality(q string) error {
	if q == "" {
		return nil
	}
	dir, err := configBase()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, qualityFile), []byte(q), 0o600)
}

// LoadQuality returns the saved playback quality, or "" when none was saved.
func LoadQuality() string {
	dir, err := configBase()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, qualityFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

const tourFile = "tour"

// SaveTourSeen records that the first-run tour was completed.
func SaveTourSeen() error {
	dir, err := configBase()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, tourFile), []byte("seen"), 0o600)
}

// TourSeen reports whether the tour was already shown.
func TourSeen() bool {
	dir, err := configBase()
	if err != nil {
		return true
	}
	_, err = os.Stat(filepath.Join(dir, tourFile))
	return err == nil
}

// ForgetTour clears the tour marker so the walkthrough shows again.
func ForgetTour() error {
	dir, err := configBase()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, tourFile))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
