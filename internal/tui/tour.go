package tui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/jhoniwana/yt-gecko/internal/auth"
)

// The tour is a short first-run walkthrough. It shows once (a marker file in
// the config dir) and can be reopened any time from the header's tour button.

// tourPage is one step of the tour.
type tourPage struct {
	Title string
	Body  string
}

func (m *Model) tourPages() []tourPage {
	return []tourPage{
		{
			Title: "Welcome to yt-gecko",
			Body: strings.Join([]string{
				snakeLogo,
				"",
				"YouTube and YouTube Music in your terminal.",
				"",
				icons.Play + "  enter      play the selected item",
				icons.Pause + "  space      pause or resume",
				icons.Search + "  /          search (music tab searches music)",
				icons.Keys + "  ?          this tour        q  quit",
			}, "\n"),
		},
		{
			Title: "Sections",
			Body: strings.Join([]string{
				"The top row holds the sections: For you, Lofi, Gaming,",
				"News, Music and Subscriptions.",
				"",
				"  [ ] or ←/→   change section",
				"  j/k or wheel move the cursor",
				"  enter        play the selected card",
				"",
				"For you and Subscriptions use your account: press a to",
				"sign in with the browser you already use for YouTube.",
			}, "\n"),
		},
		{
			Title: "Music",
			Body: strings.Join([]string{
				"The Music tab is your real YouTube Music: Listen again,",
				"Quick picks, mixes, albums and Liked Music.",
				"",
				"Mixes, albums and playlists open as a queue; songs play",
				"audio-only, so no window ever pops up.",
				"",
				"  s  shuffle      r  autoplay (keeps playing similar)",
				"  m  audio/video  v  quality (auto, 1080p... audio)",
			}, "\n"),
		},
		{
			Title: "Player",
			Body: strings.Join([]string{
				"The player shows the cover, a seek bar and the queue.",
				"Everything is clickable too.",
				"",
				"  n / p        next / previous track",
				"  ←/→          seek 5 seconds",
				"  - / +        volume",
				"  click        seek bar, buttons, volume, queue rows",
			}, "\n"),
		},
		{
			Title: "Mouse and zoom",
			Body: strings.Join([]string{
				"  hover        highlights everything clickable",
				"  click        header icons, tabs, cards, queue rows",
				"  wheel        move the selection",
				"  ctrl+wheel   zoom the interface (bigger/smaller cards)",
				"",
				"Press enter or esc when you are done; reopen this tour",
				"any time with the " + icons.Keys + " button or the ? key.",
			}, "\n"),
		},
	}
}

// openTour shows the walkthrough, starting at the first page.
func (m *Model) openTour() tea.Cmd {
	// Opening the tour from the tour itself (or from the quality menu) must
	// not overwrite where the user actually came from, or closing it would
	// bounce back into the tour.
	if m.mode != modeTour && m.mode != modeQuality {
		m.returnMode = m.mode
	}
	m.tourPage = 0
	m.tourNoShow = true
	m.mode = modeTour
	return nil
}

// closeTour leaves the tour. It only remembers the tour as seen when the
// "don't show again" option is enabled (it is by default).
func (m *Model) closeTour() tea.Cmd {
	m.mode = m.returnMode
	if m.mode == modeTour || m.mode == modeQuality {
		m.mode = modeHome
	}
	if m.tourNoShow {
		_ = auth.SaveTourSeen()
	} else {
		_ = auth.ForgetTour()
	}
	return m.thumbCmd()
}

// updateTour handles the tour keys: page back and forth, toggle the
// "don't show again" option, or close.
func (m *Model) updateTour(msg tea.KeyMsg) tea.Cmd {
	pages := m.tourPages()
	switch msg.String() {
	case "d":
		m.tourNoShow = !m.tourNoShow
		return nil
	case "right", "j", "l", "down", " ", "n":
		if m.tourPage < len(pages)-1 {
			m.tourPage++
		} else {
			return m.closeTour()
		}
	case "left", "k", "h", "up", "p":
		if m.tourPage > 0 {
			m.tourPage--
		}
	case "enter", "esc", "q":
		return m.closeTour()
	}
	return nil
}

// renderTour draws the current tour page as a centered card.
func (m *Model) renderTour() string {
	pages := m.tourPages()
	if m.tourPage >= len(pages) {
		m.tourPage = len(pages) - 1
	}
	page := pages[m.tourPage]

	var b strings.Builder
	b.WriteString(m.styles.Accent.Render(page.Title) + "\n\n")
	b.WriteString(page.Body + "\n\n")
	progress := "  "
	for i := range pages {
		if i == m.tourPage {
			progress += "● "
		} else {
			progress += "○ "
		}
	}
	check := "[ ]"
	if m.tourNoShow {
		check = "[x]"
	}
	b.WriteString(m.styles.Muted.Render(progress) + "\n")
	b.WriteString(m.styles.Help.Render("←/→: pages    enter: done") + "\n")
	b.WriteString(m.styles.Help.Render("d: ") + m.styles.Accent.Render(check+" don't show again"))
	box := m.styles.Box.Width(m.tourWidth()).Render(b.String())
	return lipglossPlace(box, m.width)
}

// tourWidth keeps the tour readable on narrow windows.
func (m *Model) tourWidth() int {
	w := m.width - 8
	if w > 66 {
		w = 66
	}
	if w < 30 {
		w = 30
	}
	return w - 4 // the box pads two columns per side
}
