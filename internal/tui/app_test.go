package tui

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jhoniwana/yt-gecko/internal/core"
)

func newTestModel() *Model {
	g, err := core.NewGeckoCore()
	if err != nil {
		panic(err)
	}
	return New(g, nil)
}

// sampleFeed returns n fake results sharing an uploader and duration.
func sampleFeed(n int) []core.SearchResult {
	feed := make([]core.SearchResult, n)
	for i := range feed {
		feed[i] = core.SearchResult{
			ID:       "abc_" + string(rune('a'+i)),
			Title:    "Test video title that wraps across several rows of text",
			Uploader: "Channel",
			Duration: 593,
		}
	}
	return feed
}

// TestViewFitsHeight makes sure the home view never renders taller than the
// window in both card mode (kitty-graphics) and plain-row mode, at any height
// a tiling window can produce. The list page size derives from the true
// leftover rows, so small windows shrink instead of overflowing.
func TestViewFitsHeight(t *testing.T) {
	for _, gfx := range []bool{false, true} {
		for h := 14; h <= 50; h++ {
			g, err := core.NewGeckoCore()
			if err != nil {
				t.Fatal(err)
			}
			m := New(g, nil)
			if gfx {
				m.gfx = NewGFXWriter(io.Discard)
			}
			m.width = 120
			m.height = h
			m.mode = modeHome
			m.tab = 0
			m.feeds = make([][]core.SearchResult, len(feedSections))
			m.feeds[m.tab] = sampleFeed(9)
			m.feedBusy = make([]bool, len(feedSections))
			m.feedErrors = make([]error, len(feedSections))
			m.verified = true
			m.authName = "zen"

			view := m.View()
			got := lipgloss.Height(view)
			if got > h {
				t.Fatalf("gfx=%v height %d: view %d rows (overflow %d)", gfx, h, got, got-h)
			}
			if !m.cardMode() && h >= 24 && got < h-2 {
				t.Fatalf("gfx=%v height %d: view %d rows, leaves wasted space", gfx, h, got)
			}
		}
	}
}

// TestCardPageSizes checks that the card page size fits the whole frame
// inside the window height, shrinking rather than hiding the key hint line:
// on the measured layout a frame costs 16 + 8*per rows.
func TestCardPageSizes(t *testing.T) {
	g, err := core.NewGeckoCore()
	if err != nil {
		t.Fatal(err)
	}
	m := New(g, nil)
	m.gfx = NewGFXWriter(io.Discard)
	m.verified = true
	m.width = 120

	cases := []struct {
		height int
		want   int
	}{
		{35, 5},
		{30, 4},
		{25, 3},
		{22, 2},
	}
	for _, c := range cases {
		m.height = c.height
		if got := m.pageSize(); got != c.want {
			t.Fatalf("height %d: want %d cards, got %d", c.height, c.want, got)
		}
	}

	// Below the card threshold the same frame runs plain rows.
	m.height = 21
	if got := m.pageSize(); got != 3 {
		t.Fatalf("height 21: want 3 plain rows, got %d", got)
	}
}

// TestListFitsWidth makes sure a rendered list box is never wider than the
// window: on narrow screens the border used to end past the edge and wrap onto
// the following line, which looked like the UI ignoring the resize.
func TestListFitsWidth(t *testing.T) {
	for _, width := range []int{46, 56, 66, 80, 120, 200} {
		g, err := core.NewGeckoCore()
		if err != nil {
			t.Fatal(err)
		}
		m := New(g, nil)
		m.gfx = NewGFXWriter(io.Discard)
		m.width = width
		m.height = 35
		m.mode = modeHome
		m.tab = 0
		m.feeds = make([][]core.SearchResult, len(feedSections))
		m.feeds[0] = sampleFeed(3)
		m.feedBusy = make([]bool, len(feedSections))
		m.feedErrors = make([]error, len(feedSections))
		m.verified = true
		m.authName = "zen"

		view := m.View()
		for _, line := range strings.Split(view, "\n") {
			if n := lipgloss.Width(line); n > width {
				t.Fatalf("width %d: line of %d columns: %q", width, n, truncate(line, 40))
			}
		}
	}
}

func TestInputBackspace(t *testing.T) {
	m := newTestModel()
	m.q = "hello"
	m.mode = modeInput

	cases := []tea.KeyMsg{
		{Type: tea.KeyBackspace},
		{Type: tea.KeyDelete},
		{Type: tea.KeyCtrlH},
		{Type: tea.KeyRunes, Runes: []rune{0x7f}},
		{Type: tea.KeyRunes, Runes: []rune{0x08}},
	}
	for _, c := range cases {
		before := m.q
		model, _ := m.Update(c)
		m = model.(*Model)
		want := before[:len(before)-1]
		if m.q != want {
			t.Fatalf("key %v: want %q, got %q", c.Type, want, m.q)
		}
	}
	if m.q != "" {
		t.Fatalf("expected empty query, got %q", m.q)
	}
}

func TestInputRunes(t *testing.T) {
	m := newTestModel()
	m.q = ""
	m.mode = modeInput
	for _, c := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'l', 'o'}},
		{Type: tea.KeyRunes, Runes: []rune{'f', 'i'}},
	} {
		model, _ := m.Update(c)
		m = model.(*Model)
	}
	if m.q != "lofi" {
		t.Fatalf("expected lofi, got %q", m.q)
	}
}
