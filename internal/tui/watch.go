package tui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// openAt starts playing the list item at index i and loads the video's watch
// page (the "up next" recommendations) in the background. The watch page is
// shown immediately with an "opening" state, so a click gives instant
// feedback instead of the video popping up out of nowhere later. Items opened
// from the Music tab play audio-only and land on the dedicated player.
func (m *Model) openAt(i int) tea.Cmd {
	list := m.currentList()
	if i < 0 || i >= len(list) {
		return nil
	}
	item := list[i]
	m.returnMode = m.mode
	m.err = nil
	if m.audioOnly && item.PlaylistID != "" {
		// A YouTube Music mix, album or playlist: expand it first.
		return m.loadMusicQueue(item)
	}
	if m.audioOnly {
		m.queue = nil // playing a single song from the Music tab
	}
	// playAt must run while currentList still points at the origin list.
	play := m.playAt(i)
	m.current = &item
	m.loading = true
	m.loadingText = "Opening " + truncate(item.Title, 40) + "..."
	if m.audioOnly {
		m.mode = modeMusic
	} else {
		m.mode = modeWatch
	}
	m.cursor = 0
	var related tea.Cmd
	if !m.audioOnly {
		related = m.loadRelated(item.ID)
	}
	return tea.Batch(play, related, m.spinCmd())
}

// loadRelated fetches the watch-page recommendations for a video.
func (m *Model) loadRelated(videoID string) tea.Cmd {
	m.related = nil
	m.relatedBusy = true
	m.relatedErr = nil
	return func() tea.Msg {
		related, err := m.core.RelatedVideos(videoID, 12)
		if err != nil {
			return relatedMsg{err: err}
		}
		return relatedMsg{results: related}
	}
}

// updateWatch handles keys on the watch page: pick a recommendation and play
// it (which reloads its own recommendations), pause, skip, or go back to the
// list the video was opened from.
func (m *Model) updateWatch(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		return m.moveCurrent(1)
	case "k", "up":
		return m.moveCurrent(-1)
	case "enter":
		return m.openAt(m.cursor)
	case " ", "p":
		m.togglePause()
		return m.thumbCmd()
	case "/":
		m.mode = modeInput
	case "v", "V":
		return m.openQuality()
	case "h", "Q", "esc", "q":
		m.mode = m.returnMode
		m.err = nil
		return m.ensureFeeds()
	}
	return m.thumbCmd()
}

// renderWatch draws the watch page: what is playing on top, and the "up next"
// recommendations below it, exactly like YouTube's sidebar.
func (m *Model) renderWatch() string {
	var b strings.Builder
	above := 0

	if m.current != nil {
		t := m.current
		head := strings.TrimRight(t.Title+"\n"+t.Uploader+"  "+t.DurationLabel(), " ")
		title := "Now playing"
		if m.loading {
			title = spinnerFrame(m.spin) + " Opening..."
		} else if !m.playing {
			title = "Paused"
		}
		title += "  ·  " + m.core.Quality()
		box := m.styles.Box.Render(m.styles.Accent.Render(title) + "\n\n" +
			m.styles.Header.Render(wrapText(head, m.wrapWidth())))
		b.WriteString(box + "\n")
		above += lipgloss.Height(box)
	}

	if m.err != nil {
		er := m.styles.Error.Render(wrapText("Error: "+m.err.Error(), m.wrapWidth()))
		b.WriteString(er + "\n\n")
		above += lipgloss.Height(er) + 2
	}

	if m.relatedBusy && len(m.related) == 0 {
		b.WriteString(m.styles.Hint.Render(spinnerFrame(m.spin)+" Loading recommendations...") + "\n\n")
		above += 3
	}
	if m.relatedErr != nil {
		warn := m.styles.Warn.Render("No recommendations: " + m.relatedErr.Error())
		b.WriteString(warn + "\n\n")
		above += lipgloss.Height(warn) + 2
	}

	b.WriteString(m.renderList("Up next", m.related, above))
	return b.String()
}