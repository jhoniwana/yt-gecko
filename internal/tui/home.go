package tui

import (
	"errors"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jhoniwana/yt-gecko/internal/core"
)

// feedSections are the content shelves shown on the home screen. Larger
// hashtags behave like YouTube's music/lofi/gaming/news hubs without a
// signed-in API and are reliable to extract with yt-dlp. The For you and
// Subscriptions slots are signed-in innerTube shelves (see core.HomeFeed and
// core.SubscriptionsFeed) when a verified browser session exists, and fall
// back to a hashtag (For you) or a login prompt (Subscriptions) otherwise.
var feedSections = []string{"For you", "Lofi", "Gaming", "News", "Music", "Subscriptions"}

// musicTab is the index of the Music section: playing from it uses the
// audio-only engine and opens the dedicated player instead of the watch page.
const musicTab = 4

var errSubscriptions = errors.New("press a to log in with your browser to load your subscriptions feed")

// feedTag maps a home tab to its YouTube hashtag. Returns "" for the two
// innerTube-only slots ("For you" and "Subscriptions").
func (m *Model) feedTag(tab int) string {
	switch tab {
	case 1:
		return "lofi"
	case 2:
		return "gaming"
	case 3:
		return "news"
	case musicTab:
		return "music"
	}
	return ""
}

// feedTabBar renders the top tab bar of the home screen. Each section is a
// click zone, so the tabs work with the mouse too.
func (m *Model) feedTabBar() string {
	var b strings.Builder
	b.WriteString("  ")
	x := 2
	for i, name := range feedSections {
		label := " " + name + " "
		if i == m.tab {
			label = "[" + name + "]"
		}
		m.addZone(x, x+lipgloss.Width(label)-1, m.bodyTop, func(i int) func(*Model, int, int) tea.Cmd {
			return func(m *Model, _, _ int) tea.Cmd { return m.switchTab(i) }
		}(i))
		hovered := m.hoverY == m.bodyTop && m.hoverX >= x && m.hoverX <= x+lipgloss.Width(label)-1
		switch {
		case i == m.tab && hovered:
			b.WriteString(m.styles.Hover.Render(label))
		case i == m.tab:
			b.WriteString(m.styles.Active.Render(label))
		case hovered:
			b.WriteString(m.styles.Hover.Render(label))
		default:
			b.WriteString(m.styles.Dim.Render(label))
		}
		b.WriteString("  ")
		x += lipgloss.Width(label) + 2
	}
	return b.String()
}

// ensureFeeds initializes the feed caches. Returns the initial load command.
func (m *Model) ensureFeeds() tea.Cmd {
	if len(m.feedBusy) == 0 {
		m.feedBusy = make([]bool, len(feedSections))
		m.feeds = make([][]core.SearchResult, len(feedSections))
		m.feedErrors = make([]error, len(feedSections))
	}
	if m.mode != modeHome {
		return nil
	}
	return m.loadFeed(m.tab)
}

// loadFeed triggers an async load of the feed for the given tab.
func (m *Model) loadFeed(tab int) tea.Cmd {
	if tab < 0 || tab >= len(feedSections) || m.feedBusy[tab] {
		return nil
	}
	m.feedBusy[tab] = true
	m.feedErrors[tab] = nil

	if !m.verified || m.core.Browser() == "" {
		// Anonymous path: hashtag shelves for the generic tabs; a login
		// prompt for the innerTube-only Subscriptions slot.
		switch tab {
		case 5:
			return func() tea.Msg {
				return feedMsg{tab: tab, err: errSubscriptions}
			}
		case 0:
			return func() tea.Msg {
				results, err := m.core.Feed("music", 12)
				return feedMsg{tab: tab, results: results, err: err}
			}
		}
		return m.feedFromTag(tab)
	}

	// Signed-in path: personal home and subscriptions come from innerTube.
	switch tab {
	case 0:
		return func() tea.Msg {
			results, err := m.core.HomeFeed(12)
			return feedMsg{tab: tab, results: results, err: err}
		}
	case musicTab:
		// The account's real YouTube Music home: shelves of songs, mixes and
		// albums (songs play audio-only, mixes expand).
		return func() tea.Msg {
			home, err := m.core.MusicHome(60)
			return musicHomeMsg{tab: tab, home: home, err: err}
		}
	case 5:
		return func() tea.Msg {
			results, err := m.core.SubscriptionsFeed(12)
			return feedMsg{tab: tab, results: results, err: err}
		}
	}
	return m.feedFromTag(tab)
}

// feedFromTag loads a plain hashtag shelf for the tab's mapped tag.
func (m *Model) feedFromTag(tab int) tea.Cmd {
	tag := m.feedTag(tab)
	return func() tea.Msg {
		results, err := m.core.Feed(tag, 12)
		return feedMsg{tab: tab, results: results, err: err}
	}
}

// feedResults returns the cached videos of the active home tab.
func (m *Model) feedResults() []core.SearchResult {
	if m.tab < 0 || m.tab >= len(m.feeds) {
		return nil
	}
	return m.feeds[m.tab]
}
