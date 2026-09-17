package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jhoniwana/yt-gecko/internal/core"
)

// Album art is a square block: terminal cells are about twice as tall as they
// are wide, so 16x8 cells is a 1:1 pixel square. kitty-graphics terminals get
// the real image, everything else gets half-block art.
const (
	albumCols = 16
	albumRows = 8
)

// musicHomeMsg carries the account's YouTube Music home shelves.
type musicHomeMsg struct {
	tab  int
	home *core.MusicHome
	err  error
}

// musicBrowseMsg carries a browsed music page (playlist, album, mood).
type musicBrowseMsg struct {
	browseID string
	title    string
	shelves  []core.MusicShelf
	err      error
}

// musicQueueMsg carries an expanded playlist (a YouTube Music mix, album or
// liked-songs list) back to the player.
type musicQueueMsg struct {
	tracks []core.SearchResult
	err    error
}

// loadMusicQueue expands a playlist/mix/album entry into its tracks and then
// plays the first one.
func (m *Model) loadMusicQueue(item core.SearchResult) tea.Cmd {
	m.loading = true
	m.loadingText = "Loading " + truncate(item.Title, 36) + "..."
	id := item.PlaylistID
	m.queue = nil
	m.mode = modeMusic
	return tea.Batch(func() tea.Msg {
		shelves, err := m.core.MusicBrowse(id, "", 100)
		if err != nil {
			return musicQueueMsg{err: err}
		}
		return musicQueueMsg{tracks: core.FlattenShelves(shelves)}
	}, m.spinCmd())
}

// openMusicBrowse loads a music page (a mood chip, a playlist) into the music
// home view.
func (m *Model) openMusicBrowse(browseID, params, title string) tea.Cmd {
	m.loading = true
	m.loadingText = "Loading " + truncate(title, 36) + "..."
	return tea.Batch(func() tea.Msg {
		shelves, err := m.core.MusicBrowse(browseID, params, 60)
		return musicBrowseMsg{browseID: browseID, title: title, shelves: shelves, err: err}
	}, m.spinCmd())
}

// musicBack returns from a browsed music page to the home shelves.
func (m *Model) musicBack() tea.Cmd {
	m.musicShelves = nil
	m.musicItems = nil
	m.musicBrowseID = ""
	m.musicTitle = "Home"
	m.cursor = 0
	m.feedBusy[m.tab] = false
	return m.loadFeed(musicTab)
}

// musicGridCols is how many album cards fit in one grid row.
func (m *Model) musicGridCols() int {
	cardW := albumCols + 2
	cols := (m.listWidth() + 2) / cardW
	if cols < 1 {
		cols = 1
	}
	return cols
}

// shelfHeight is the rendered height of one shelf: a title, the card rows and
// a gap.
func shelfHeight(items, cols int) int {
	rows := (items + cols - 1) / cols
	if rows < 1 {
		rows = 1
	}
	return 1 + rows*(albumRows+2) + 1
}

// cardArtLine returns one row of a card's cover art: the kitty image
// placeholder row, half-block art, or blanks until the thumbnail arrives.
func (m *Model) cardArtLine(r core.SearchResult, row int) string {
	if m.gfx != nil && m.thumbsOn() {
		if id, ok := m.imgs[r.ID]; ok {
			return kittyArtSlice(id, albumCols, row)
		}
		return ""
	}
	if art := m.thumbs[r.ID]; art != "" {
		lines := strings.Split(art, "\n")
		if row < len(lines) {
			return lines[row]
		}
	}
	return ""
}

// renderMusicHome draws the YouTube Music home: the mood chips row and the
// shelves as grids of square covers, scrolled so the selection stays visible.
func (m *Model) renderMusicHome(above int) string {
	var b strings.Builder

	// Mood chips, with a Back chip while browsing a page.
	if len(m.musicChips) > 0 || m.musicBrowseID != "" {
		x := 2
		y := m.bodyTop + above
		if m.musicBrowseID != "" {
			label := " ← Back "
			style := m.styles.Dim
			if m.hoverY == y && m.hoverX >= x && m.hoverX <= x+lipgloss.Width(label)-1 {
				style = m.styles.Hover
			}
			m.addZone(x, x+lipgloss.Width(label)-1, y, func(m *Model, _, _ int) tea.Cmd { return m.musicBack() })
			b.WriteString(style.Render(label) + " ")
			x += lipgloss.Width(label) + 1
		}
		for _, c := range m.musicChips {
			chip := c
			label := " " + chip.Label + " "
			style := m.styles.Dim
			if m.musicBrowseID != "" && chip.BrowseID == m.musicBrowseID {
				style = m.styles.Active
			} else if m.hoverY == y && m.hoverX >= x && m.hoverX <= x+lipgloss.Width(label)-1 {
				style = m.styles.Hover
			}
			m.addZone(x, x+lipgloss.Width(label)-1, y, func(m *Model, _, _ int) tea.Cmd {
				return m.openMusicBrowse(chip.BrowseID, chip.Params, chip.Label)
			})
			b.WriteString(style.Render(label) + " ")
			x += lipgloss.Width(label) + 1
		}
		b.WriteString("\n\n")
		above += 2
	}

	cols := m.musicGridCols()
	budget := m.height - m.bodyTop - above - 3 // help line and gaps
	if budget < albumRows+3 {
		budget = albumRows + 3
	}

	// Scroll so the shelf holding the cursor stays inside the budget.
	heights := make([]int, len(m.musicShelves))
	for i, sh := range m.musicShelves {
		heights[i] = shelfHeight(len(sh.Items), cols)
	}
	si, _ := m.cursorShelf()
	start := 0
	for start < si {
		h := 0
		for i := start; i <= si; i++ {
			h += heights[i]
		}
		if h <= budget {
			break
		}
		start++
	}

	y := m.bodyTop + above
	for i := start; i < len(m.musicShelves) && y-m.bodyTop-above < budget; i++ {
		sh := m.musicShelves[i]
		base := m.shelfBase(i)
		title := sh.Title
		if i == si {
			title = m.styles.Accent.Render(title)
		} else {
			title = m.styles.Muted.Render(title)
		}
		b.WriteString(title + "\n")
		y++

		rows := (len(sh.Items) + cols - 1) / cols
		for row := 0; row < rows; row++ {
			// One click zone per card, covering cover art and both text rows.
			for c := 0; c < cols; c++ {
				off := row*cols + c
				if off >= len(sh.Items) {
					break
				}
				idx := base + off
				x0 := 2 + c*(albumCols+2)
				cardTop := y
				m.addZoneSpan(x0, x0+albumCols-1, cardTop, cardTop+albumRows+1, func(idx int) func(*Model, int, int) tea.Cmd {
					return func(m *Model, _, _ int) tea.Cmd { return m.openAt(idx) }
				}(idx))
			}
			// Cover art rows, joined horizontally.
			for artRow := 0; artRow < albumRows; artRow++ {
				var line strings.Builder
				for c := 0; c < cols && row*cols+c < len(sh.Items); c++ {
					it := m.musicItems[base+row*cols+c]
					line.WriteString(padCells(m.cardArtLine(it, artRow), albumCols))
					line.WriteString("  ")
				}
				b.WriteString(line.String() + "\n")
				y++
			}
			// Title and subtitle rows under the covers.
			for textRow := 0; textRow < 2; textRow++ {
				var line strings.Builder
				for c := 0; c < cols && row*cols+c < len(sh.Items); c++ {
					idx := base + row*cols + c
					it := m.musicItems[idx]
					text, style := it.Title, m.styles.Header
					if textRow == 1 {
						text, style = it.Uploader, m.styles.Dim
					}
					if idx == m.cursor {
						style = m.styles.Sel
					}
					line.WriteString(style.Width(albumCols).Render(truncate(text, albumCols)))
					line.WriteString("  ")
				}
				b.WriteString(line.String() + "\n")
				y++
			}
		}
		b.WriteString("\n")
		y++
	}
	return b.String()
}

// shelfBase returns the index of the first item of shelf i in musicItems.
func (m *Model) shelfBase(i int) int {
	base := 0
	for j := 0; j < i && j < len(m.musicShelves); j++ {
		base += len(m.musicShelves[j].Items)
	}
	return base
}

// cursorShelf maps the cursor to its shelf and local index.
func (m *Model) cursorShelf() (int, int) {
	local := m.cursor
	for i, sh := range m.musicShelves {
		if local < len(sh.Items) {
			return i, local
		}
		local -= len(sh.Items)
	}
	if len(m.musicShelves) == 0 {
		return 0, 0
	}
	return len(m.musicShelves) - 1, 0
}

// musicTickMsg carries the mpv clock for the progress bar.
type musicTickMsg struct {
	pos, dur float64
	eof      bool
	err      error
}

// musicTick polls mpv once a second while the player is visible. The IPC
// client is stateless, so the tick can run on a command goroutine.
func (m *Model) musicTick() tea.Cmd {
	c := m.client
	if c == nil {
		return nil
	}
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		pos, err := c.GetPosition()
		dur, _ := c.GetDuration()
		return musicTickMsg{pos: pos, dur: dur, eof: c.EOFReached(), err: err}
	})
}

// updateMusic handles the player keys: transport, seek, volume and the queue.
func (m *Model) updateMusic(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case " ", "p":
		m.togglePause()
	case "n", ">":
		return m.musicNext(1)
	case "P", "<":
		return m.musicNext(-1)
	case "left":
		m.seekBy(-5)
	case "right":
		m.seekBy(5)
	case "-", "_":
		m.changeVolume(-5)
	case "+", "=":
		m.changeVolume(5)
	case "j", "down":
		return m.moveCurrent(1)
	case "k", "up":
		return m.moveCurrent(-1)
	case "enter":
		return m.musicPlay(m.cursor)
	case "v", "V":
		return m.openQuality()
	case "/":
		m.mode = modeInput
	case "h", "esc", "q", "Q":
		m.mode = m.returnMode
		m.err = nil
		return m.ensureFeeds()
	}
	return nil
}

// musicNext plays the next (or previous) track in the queue, keeping the
// queue cursor on whatever is playing.
func (m *Model) musicNext(delta int) tea.Cmd {
	list := m.currentList()
	next := m.cursor + delta
	if next < 0 || next >= len(list) {
		return nil
	}
	return m.musicPlay(next)
}

// musicPlay starts a queue item in audio-only mode and stays on the player.
// Unlike openAt it never switches modes and keeps the queue cursor in sync
// with the playing track.
func (m *Model) musicPlay(i int) tea.Cmd {
	list := m.currentList()
	if i < 0 || i >= len(list) {
		return nil
	}
	item := list[i]
	m.audioOnly = true
	m.cursor = i
	m.err = nil
	m.trackEnded = false
	m.current = &item
	m.loading = true
	m.loadingText = "Opening " + truncate(item.Title, 40) + "..."
	m.mode = modeMusic
	m.pos, m.dur = 0, 0
	m.volume = 100
	return tea.Batch(func() tea.Msg {
		client, err := m.core.Play(item.URL, true)
		if err != nil {
			return errMsg{err}
		}
		return playbackMsg{client}
	}, m.spinCmd())
}

// seekBy moves the playback position by delta seconds.
func (m *Model) seekBy(delta float64) {
	if m.client == nil {
		return
	}
	target := m.pos + delta
	if target < 0 {
		target = 0
	}
	if m.dur > 0 && target > m.dur-1 {
		target = m.dur - 1
	}
	if err := m.client.Seek(target); err == nil {
		m.pos = target
	}
}

// changeVolume nudges the mpv volume.
func (m *Model) changeVolume(delta int) {
	m.volume = clampInt(m.volume+delta, 0, 130)
	if m.client != nil {
		_ = m.client.SetVolume(m.volume)
	}
}

// updateAlbumArt encodes the current track's thumbnail as a square kitty image
// (or leaves the block-art path alone) so the player can show cover art.
func (m *Model) updateAlbumArt() {
	if m.gfx == nil || m.current == nil {
		return
	}
	img := m.thumbsImg[m.current.ID]
	if img == nil {
		return
	}
	png, err := thumbPNG(img, albumCols, albumRows)
	if err != nil {
		return
	}
	if m.imgSeq >= 255 { // 8-bit id space in the placeholder foreground color
		return
	}
	if m.albumID != 0 {
		m.gfx.queue(kittyDelete(m.albumID))
	}
	m.imgSeq++
	m.albumID = m.imgSeq
	m.gfx.queue(kittyTransmit(m.albumID, png) + kittyVirtual(m.albumID, albumCols, albumRows))
}

// albumArt renders the cover: a real kitty image, half-block art from the
// decoded pixels, or a dim placeholder until the thumbnail arrives.
func (m *Model) albumArt() string {
	if m.current == nil {
		return ""
	}
	if m.gfx != nil && m.albumID != 0 {
		rows := make([]string, albumRows)
		for r := range rows {
			rows[r] = kittyArtSlice(m.albumID, albumCols, r)
		}
		return strings.Join(rows, "\n")
	}
	if img := m.thumbsImg[m.current.ID]; img != nil {
		return renderBlockArt(img, albumCols, albumRows)
	}
	blank := m.styles.Muted.Render(strings.Repeat("░", albumCols))
	rows := make([]string, albumRows)
	for i := range rows {
		rows[i] = blank
	}
	return strings.Join(rows, "\n")
}

// progressBar draws the seek bar with the elapsed portion highlighted.
func (m *Model) progressBar(width int) string {
	if width < 8 {
		width = 8
	}
	frac := 0.0
	if m.dur > 0 {
		frac = m.pos / m.dur
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	fill := int(float64(width) * frac)
	return m.styles.Accent.Render(strings.Repeat("▬", fill)) +
		m.styles.Muted.Render(strings.Repeat("─", width-fill))
}

// volumeBar shows the volume as a small bar plus percentage.
func (m *Model) volumeBar() string {
	cells := 10
	fill := m.volume * cells / 130
	if fill > cells {
		fill = cells
	}
	if fill < 0 {
		fill = 0
	}
	bar := m.styles.Header.Render(strings.Repeat("▬", fill)) +
		m.styles.Muted.Render(strings.Repeat("─", cells-fill))
	return bar + m.styles.Hint.Render(fmt.Sprintf(" %d%%", m.volume))
}

// fmtTime renders seconds as m:ss.
func fmtTime(sec float64) string {
	if sec < 0 || math.IsNaN(sec) {
		sec = 0
	}
	s := int(sec + 0.5)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// renderMusic draws the Spotify-style player: cover art, a clickable seek
// bar, transport buttons and volume on the left, the queue on the right
// (stacked on narrow windows). Every control is also clickable.
func (m *Model) renderMusic() string {
	if m.current == nil {
		msg := "Nothing playing. Pick a track from the Music tab."
		if m.err != nil {
			msg = "Error: " + m.err.Error()
		} else if m.loading {
			msg = spinnerFrame(m.spin) + " " + m.loadingText
		}
		return m.styles.Hint.Render(wrapText(msg, m.wrapWidth()))
	}

	contentX := 5 // app padding 2 + box border 1 + box padding 2
	contentY := m.bodyTop + 1
	barW := albumCols + 10
	// Short windows drop the cover and the key hint so the queue still has
	// room to breathe (the cover needs 8 rows plus two text rows).
	compactPlayer := m.height < 30

	var lines []string
	line := func(s string) { lines = append(lines, s) }
	y := func() int { return contentY + len(lines) }

	title := "♪ Now playing"
	if m.loading {
		title = spinnerFrame(m.spin) + " Opening..."
	} else if !m.playing {
		title = "♪ Paused"
	}
	line(m.styles.Accent.Render(title))
	if m.err != nil {
		line(m.styles.Error.Render(truncate(m.err.Error(), barW)))
	}
	if !compactPlayer {
		for _, art := range strings.Split(m.albumArt(), "\n") {
			line(art)
		}
	}
	line(m.styles.Header.Render(truncate(m.current.Title, barW)))
	line(m.styles.Dim.Render(truncate(m.current.Uploader, barW)))
	line("")

	// Seek bar: click anywhere on it to jump there.
	seekY := y()
	line(m.progressBar(barW))
	elapsed, total := fmtTime(m.pos), fmtTime(m.dur)
	pad := barW - len(elapsed) - len(total)
	if pad < 1 {
		pad = 1
	}
	line(m.styles.Hint.Render(elapsed) + strings.Repeat(" ", pad) + m.styles.Hint.Render(total))
	m.addZone(contentX, contentX+barW-1, seekY, func(m *Model, x, _ int) tea.Cmd {
		if m.dur <= 0 {
			return nil
		}
		frac := float64(x-contentX+1) / float64(barW)
		if frac < 0 {
			frac = 0
		}
		if frac > 1 {
			frac = 1
		}
		m.seekBy(frac*m.dur - m.pos)
		return nil
	})

	// Transport row: clickable buttons plus a clickable volume bar.
	transY := y()
	var row strings.Builder
	x := contentX
	button := func(label string, act func(*Model) tea.Cmd) {
		m.addZone(x, x+lipgloss.Width(label)-1, transY, func(m *Model, _, _ int) tea.Cmd { return act(m) })
		row.WriteString(label)
		x += lipgloss.Width(label)
	}
	hovered := func(x0, w int) bool {
		return m.hoverY == transY && m.hoverX >= x0 && m.hoverX < x0+w
	}
	btnStyle := func(x0, w int) lipgloss.Style {
		if hovered(x0, w) {
			return m.styles.Hover
		}
		return m.styles.Header
	}
	b1 := " ⏮ "
	button(btnStyle(x, lipgloss.Width(b1)).Render(b1), func(m *Model) tea.Cmd { return m.musicNext(-1) })
	row.WriteString(" ")
	x++
	b2 := " ⏯ "
	if !m.playing {
		b2 = " ▶ "
	}
	button(btnStyle(x, lipgloss.Width(b2)).Render(b2), func(m *Model) tea.Cmd { m.togglePause(); return nil })
	row.WriteString(" ")
	x++
	b3 := " ⏭ "
	button(btnStyle(x, lipgloss.Width(b3)).Render(b3), func(m *Model) tea.Cmd { return m.musicNext(1) })
	row.WriteString("   vol ")
	x += 3 + len("vol ")

	// Volume: clickable minus, bar and plus.
	minusX := x
	button(m.styles.Dim.Render("-"), func(m *Model) tea.Cmd { m.changeVolume(-5); return nil })
	row.WriteString(" ")
	x++
	volW := 8
	volFill := m.volume * volW / 130
	if volFill > volW {
		volFill = volW
	}
	if volFill < 0 {
		volFill = 0
	}
	volX := x
	row.WriteString(m.styles.Header.Render(strings.Repeat("▬", volFill)) + m.styles.Muted.Render(strings.Repeat("─", volW-volFill)))
	m.addZone(volX, volX+volW-1, transY, func(m *Model, vx, _ int) tea.Cmd {
		frac := float64(vx-volX+1) / float64(volW)
		m.changeVolume(int(frac*130) - m.volume)
		return nil
	})
	x += volW
	plusX := x
	button(m.styles.Dim.Render("+"), func(m *Model) tea.Cmd { m.changeVolume(5); return nil })
	row.WriteString(m.styles.Hint.Render(fmt.Sprintf(" %d%%", m.volume)))
	_ = minusX
	_ = plusX
	line(row.String())
	if !compactPlayer {
		line(m.styles.Muted.Render("space ⏯  n/p ⏮⏭  ←/→ ±5s  -/+ vol"))
	}

	leftBox := m.styles.Box.Render(strings.Join(lines, "\n"))

	// The player and the queue must fit inside the app's content area (the
	// app style pads two columns on each side); anything wider is wrapped by
	// lipgloss, which shreds the cover art into strips.
	rightW := m.width - lipgloss.Width(leftBox) - 8
	if rightW < 30 {
		queue := m.renderQueue(m.listWidth(), 2, m.bodyTop+lipgloss.Height(leftBox)+2)
		return leftBox + "\n\n" + queue
	}
	queue := m.renderQueue(rightW, 2+lipgloss.Width(leftBox)+2, m.bodyTop)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftBox, "  ", queue)
}

// renderQueue draws the play queue. Wide queues get a three-row layout with
// a 4x2 mini cover per track; narrow (stacked) queues fall back to one row per
// track with a 2x1 mini cover, so many more tracks fit. Rows are clickable.
func (m *Model) renderQueue(width, x0, y0 int) string {
	list := m.currentList()
	if width < 24 {
		width = 24
	}
	avail := m.height - y0 - 3
	// A narrow or short queue (the stacked layout) uses one row per track so
	// many more fit; wide queues get the roomier three-row rows.
	compact := width < 46 || avail < 24
	itemRows, miniCols, miniRows := 3, 4, 2
	if compact {
		itemRows, miniCols, miniRows = 1, 2, 1
	}
	rows := avail / itemRows
	if rows < 1 {
		rows = 1
	}
	if rows > len(list) {
		rows = len(list)
	}
	start := 0
	if m.cursor >= rows {
		start = m.cursor - rows + 1
	}
	end := min(start+rows, len(list))

	playingIdx := -1
	if m.current != nil {
		for i, r := range list {
			if r.ID == m.current.ID {
				playingIdx = i
				break
			}
		}
	}

	var b strings.Builder
	for i := start; i < end; i++ {
		r := list[i]
		rowY := y0 + 2 + (i-start)*itemRows // border plus header line
		m.addZoneSpan(x0, x0+width-1, rowY, rowY+itemRows-1, func(i int) func(*Model, int, int) tea.Cmd {
			return func(m *Model, _, _ int) tea.Cmd { return m.musicPlay(i) }
		}(i))

		// Border (2) + padding (4) + marker (2) + cover + space (1).
		textW := width - 9 - miniCols
		if textW < 10 {
			textW = 10
		}
		art := m.miniThumbLines(r.ID, miniCols, miniRows)
		mark := "  "
		if i == playingIdx {
			mark = m.styles.Success.Render("▶ ")
		}
		title := truncate(r.Title, textW)
		meta := truncate(r.Uploader+"  "+r.DurationLabel(), textW)

		if compact {
			// Leave room for the trailing duration so the row never wraps.
			dur := r.DurationLabel()
			ctW := textW - len(dur) - 2
			if ctW < 6 {
				ctW = 6
			}
			title = truncate(r.Title, ctW)
			line := mark + padCells(art[0], miniCols) + " "
			switch {
			case i == m.cursor:
				line += m.styles.Sel.Render(title)
			case i == playingIdx:
				line += m.styles.Success.Render(title)
			default:
				line += title
			}
			if dur != "" {
				line += m.styles.Dim.Render("  " + dur)
			}
			b.WriteString(line + "\n")
			continue
		}
		switch {
		case i == m.cursor:
			b.WriteString(mark + padCells(art[0], miniCols) + " " + m.styles.Sel.Width(textW).Render(title) + "\n")
			b.WriteString("  " + padCells(art[1], miniCols) + " " + m.styles.Sel.Width(textW).Render(meta) + "\n")
		case i == playingIdx:
			b.WriteString(mark + padCells(art[0], miniCols) + " " + m.styles.Success.Render(title) + "\n")
			b.WriteString("  " + padCells(art[1], miniCols) + " " + m.styles.Dim.Render(meta) + "\n")
		default:
			b.WriteString(mark + padCells(art[0], miniCols) + " " + title + "\n")
			b.WriteString("  " + padCells(art[1], miniCols) + " " + m.styles.Dim.Render(meta) + "\n")
		}
		b.WriteString("\n")
	}
	head := m.styles.Accent.Render("Queue") + m.styles.Muted.Render(fmt.Sprintf("  ·  %d tracks", len(list)))
	return m.styles.Box.Width(width).Render(head + "\n" + strings.TrimSuffix(b.String(), "\n"))
}

// miniThumbLines renders (and caches) the tiny queue cover for a track: real
// half-block art from the decoded pixels, or a dim placeholder until the
// thumbnail arrives.
func (m *Model) miniThumbLines(id string, cols, rows int) []string {
	if lines, ok := m.minis[id]; ok && len(lines) == rows {
		return lines
	}
	var lines []string
	if img := m.thumbsImg[id]; img != nil {
		lines = strings.Split(renderBlockArt(img, cols, rows), "\n")
	}
	for len(lines) < rows {
		lines = append(lines, m.styles.Muted.Render(strings.Repeat("░", cols)))
	}
	m.minis[id] = lines
	return lines
}

// autoplayMsg carries freshly fetched radio/related tracks when the queue ran
// out and autoplay is on.
type autoplayMsg struct {
	tracks []core.SearchResult
	err    error
}
