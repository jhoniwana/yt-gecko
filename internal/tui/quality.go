package tui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jhoniwana/yt-gecko/internal/auth"
	"github.com/jhoniwana/yt-gecko/internal/core"
)

// openQuality shows the playback quality menu, starting on the active option.
func (m *Model) openQuality() tea.Cmd {
	m.returnMode = m.mode
	m.mode = modeQuality
	m.cursor = 0
	for i, q := range coreQualityOptions() {
		if q == m.core.Quality() {
			m.cursor = i
		}
	}
	return nil
}

// coreQualityOptions exposes the engine's selectable qualities to the menu.
func coreQualityOptions() []string {
	return core.QualityOptions
}

// updateQuality handles keys on the quality menu: pick an option and apply it
// (restarting playback at the new quality when a video is playing), or cancel.
// Pasted text arrives as a single multi-rune message, so j/k runs are handled
// rune by rune.
func (m *Model) updateQuality(msg tea.KeyMsg) tea.Cmd {
	opts := coreQualityOptions()
	move := func(delta int) {
		m.cursor = clampInt(m.cursor+delta, 0, len(opts)-1)
	}
	cancel := func() tea.Cmd {
		m.mode = m.returnMode
		return m.thumbCmd()
	}
	switch msg.String() {
	case "j", "down":
		move(1)
		return nil
	case "k", "up":
		move(-1)
		return nil
	case "enter":
		q := opts[m.cursor]
		m.core.SetQuality(q)
		_ = auth.SaveQuality(q)
		m.mode = m.returnMode
		if m.playing && m.current != nil {
			return m.replayCmd()
		}
		return m.thumbCmd()
	case "v", "esc", "q", "h":
		return cancel()
	}
	if msg.Type == tea.KeyRunes {
		for _, r := range msg.Runes {
			switch r {
			case 'j':
				move(1)
			case 'k':
				move(-1)
			case 'v', 'q', 'h':
				return cancel()
			}
		}
	}
	return nil
}

// replayCmd re-resolves and restarts the current video with the active
// quality, so switching quality while watching applies immediately.
func (m *Model) replayCmd() tea.Cmd {
	item := *m.current
	m.loading = true
	m.loadingText = "Switching quality..."
	return tea.Batch(func() tea.Msg {
		client, err := m.core.Play(item.URL, false)
		if err != nil {
			return errMsg{err}
		}
		return playbackMsg{client}
	}, m.spinCmd())
}

// renderQuality draws the quality menu with the active option marked.
func (m *Model) renderQuality() string {
	var b strings.Builder
	active := m.core.Quality()
	b.WriteString(m.styles.Accent.Render("Playback quality") + "\n\n")
	for i, q := range coreQualityOptions() {
		label := q
		if q == "auto" {
			label = "auto (best)"
		}
		label = truncate(label, m.wrapWidth()-8)
		switch {
		case i == m.cursor && q == active:
			b.WriteString(m.styles.Sel.Render("> "+label+"  ●") + "\n")
		case i == m.cursor:
			b.WriteString(m.styles.Active.Render("> "+label) + "\n")
		case q == active:
			b.WriteString(m.styles.Hover.Render("  "+label+"  ●") + "\n")
		default:
			b.WriteString(m.styles.Hint.Render("  "+label) + "\n")
		}
	}
	b.WriteString("\n" + m.styles.Muted.Render("● active"))
	box := m.styles.Box.Render(b.String())
	width := m.width - 4
	if width < 20 {
		width = 20
	}
	return lipgloss.Place(width, lipgloss.Height(box), lipgloss.Center, lipgloss.Top, box)
}
