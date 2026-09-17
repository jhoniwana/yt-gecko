package tui

import (
	"os"
	"os/exec"
	"strings"
)

// Icons holds the glyphs the UI draws. When a Nerd Font is installed the UI
// uses its icons; otherwise it falls back to widely available Unicode shapes
// so nothing renders as a missing-glyph box. Set YT_GECKO_ICONS=nerd or
// YT_GECKO_ICONS=plain to force a set.
type Icons struct {
	Play, Pause, Prev, Next string
	Search, Subs, Keys      string
	Quality, Audio, Video   string
	Shuffle, Autoplay       string
	User, Music             string
}

var icons = plainIcons()

func plainIcons() Icons {
	return Icons{
		Play: "▶", Pause: "⏸", Prev: "⏮", Next: "⏭",
		Search: "⌕", Subs: "★", Keys: "?",
		Quality: "⚙", Audio: "♫", Video: "▣",
		Shuffle: "⇄", Autoplay: "∞",
		User: "●", Music: "♪",
	}
}

func nerdIcons() Icons {
	return Icons{
		Play: "\uf04b", Pause: "\uf04c", Prev: "\uf048", Next: "\uf051",
		Search: "\uf002", Subs: "\uf005", Keys: "\uf11c",
		Quality: "\uf013", Audio: "\uf001", Video: "\uf03d",
		Shuffle: "\uf074", Autoplay: "\uf01e",
		User: "\uf007", Music: "\uf001",
	}
}

// detectIcons picks the icon set once at startup.
func detectIcons() {
	switch strings.ToLower(os.Getenv("YT_GECKO_ICONS")) {
	case "nerd", "nerdfont", "nerd-font":
		icons = nerdIcons()
		return
	case "plain", "ascii", "unicode":
		icons = plainIcons()
		return
	}
	if hasNerdFont() {
		icons = nerdIcons()
		return
	}
	icons = plainIcons()
}

// hasNerdFont asks fontconfig whether any Nerd Font family is installed.
func hasNerdFont() bool {
	if _, err := exec.LookPath("fc-list"); err != nil {
		return false
	}
	out, err := exec.Command("fc-list", ":family").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "nerd")
}
