package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCookiesFromBrowserArgs(t *testing.T) {
	if got := CookiesFromBrowserArgs(""); got != nil {
		t.Errorf("expected nil for empty browser, got %v", got)
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	// A browser with no profile on disk cannot yield cookies.
	if got := CookiesFromBrowserArgs(BrowserZen); got != nil {
		t.Errorf("expected nil for missing profile, got %v", got)
	}
	if err := os.MkdirAll(filepath.Join(home, ".zen"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		b    Browser
		want string
	}{
		{BrowserFirefox, "--cookies-from-browser firefox"},
		{BrowserBrave, "--cookies-from-browser brave"},
		{BrowserVivaldi, "--cookies-from-browser vivaldi"},
		{BrowserOpera, "--cookies-from-browser opera"},
		{BrowserZen, "--cookies-from-browser firefox:" + home + "/.zen"},
	} {
		got := CookiesFromBrowserArgs(c.b)
		if len(got) != 2 || got[0]+" "+got[1] != c.want {
			t.Errorf("CookiesFromBrowserArgs(%q) = %v, want %q", c.b, got, c.want)
		}
	}
}

func TestBrowserLabel(t *testing.T) {
	cases := []struct {
		b    Browser
		want string
	}{
		{BrowserFirefox, "Firefox"},
		{BrowserZen, "Zen"},
		{BrowserLibreWolf, "LibreWolf"},
		{BrowserWaterfox, "Waterfox"},
		{BrowserFloorp, "Floorp"},
		{BrowserChrome, "Google Chrome"},
		{BrowserChromium, "Chromium"},
		{BrowserBrave, "Brave"},
		{BrowserVivaldi, "Vivaldi"},
		{BrowserOpera, "Opera"},
		{BrowserEdge, "Microsoft Edge"},
		{BrowserWhale, "Naver Whale"},
		{Browser(""), "Unknown"},
	}
	for _, c := range cases {
		if got := c.b.Label(); got != c.want {
			t.Errorf("Label(%q) = %q, want %q", c.b, got, c.want)
		}
	}
}

func TestDetectBrowsers(t *testing.T) {
	// Only ever report known browsers, each either profile-ready or installed.
	found := DetectBrowsers()
	for _, b := range found {
		known := false
		for _, k := range allBrowsers {
			if b == k {
				known = true
			}
		}
		if !known {
			t.Errorf("DetectBrowsers returned unknown browser %q", b)
		}
		if !b.HasProfile() && !b.Installed() {
			t.Errorf("DetectBrowsers returned %q with neither profile nor binary", b)
		}
	}
}

func TestBrowserFromDesktopName(t *testing.T) {
	cases := []struct {
		name string
		want Browser
	}{
		{"firefox.desktop", BrowserFirefox},
		{"zen.desktop", BrowserZen},
		{"librewolf.desktop", BrowserLibreWolf},
		{"org.mozilla.firefox.desktop", BrowserFirefox},
		{"google-chrome.desktop", BrowserChrome},
		{"chromium-browser.desktop", BrowserChromium},
		{"brave-browser.desktop", BrowserBrave},
		{"vivaldi-stable.desktop", BrowserVivaldi},
		{"opera.desktop", BrowserOpera},
		{"microsoft-edge.desktop", BrowserEdge},
		{"naver-whale.desktop", BrowserWhale},
		{"thunderbird.desktop", ""},
	}
	for _, c := range cases {
		if got := browserFromDesktopName(c.name); got != c.want {
			t.Errorf("browserFromDesktopName(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestBrowserFromMimeAppsData(t *testing.T) {
	data := `[Default Applications]
text/html=firefox.desktop
x-scheme-handler/https=brave-browser.desktop
x-scheme-handler/http=brave-browser.desktop

[Added Associations]
x-scheme-handler/https=chromium.desktop;
`
	if got := browserFromMimeAppsData(data); got != BrowserBrave {
		t.Errorf("got %q, want brave", got)
	}

	// Entries outside [Default Applications] only count as a fallback.
	fallback := "[Added Associations]\nx-scheme-handler/https=opera.desktop;\n"
	if got := browserFromMimeAppsData(fallback); got != BrowserOpera {
		t.Errorf("fallback got %q, want opera", got)
	}

	if got := browserFromMimeAppsData("[Default Applications]\ntext/html=vlc.desktop\n"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestSaveLoadBrowser(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if got := LoadBrowser(); got != "" {
		t.Fatalf("LoadBrowser on clean config = %q, want empty", got)
	}
	if err := SaveBrowser(BrowserFirefox); err != nil {
		t.Fatalf("SaveBrowser: %v", err)
	}
	if got := LoadBrowser(); got != BrowserFirefox {
		t.Fatalf("LoadBrowser after save = %q, want firefox", got)
	}
}

func TestLoadBrowserIgnoresJunk(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// Saving an empty browser is a no-op and must not wipe the saved value.
	_ = SaveBrowser(BrowserFirefox)
	_ = SaveBrowser("")
	if got := LoadBrowser(); got != BrowserFirefox {
		t.Fatalf("LoadBrowser after blank save = %q, want firefox", got)
	}
	// Unknown values are rejected rather than trusted blindly.
	_ = SaveBrowser(Browser("weird"))
	if got := LoadBrowser(); got != "" {
		t.Fatalf("LoadBrowser after junk save = %q, want empty", got)
	}
}
