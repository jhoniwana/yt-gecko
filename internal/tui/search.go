package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/jhoniwana/yt-gecko/internal/core"
)

const resultsPerPage = 8

// cardMode reports whether to draw YouTube-style thumbnail cards. Real images
// need a kitty-graphics terminal and a reasonably wide window; block art needs
// a wide window to look right. Cards are only worth their height once the
// window can hold at least two of them (measured: a card shelf takes 8 rows
// per card plus 16 rows of fixed chrome).
func (m *Model) cardMode() bool {
	if m.compact {
		return false
	}
	if m.height < 22 {
		return false
	}
	return (m.gfx != nil && m.thumbsOn()) || m.width >= 110
}

// pageSize is how many items fit per page. The card layout draws a tall
// thumbnail beside every item, so the page adapts to the terminal height. On
// very short windows the page shrinks to one item instead of overflowing the
// screen.

// perPageFor fits the whole frame (top bar, tabs, optional sign-in hint, the
// list box, the page footer and the key hint line) inside h rows. The height
// budget below was calibrated against real renders in kitty:
//
//   - above the box: 7 rows in normal mode, 4 in compact (no sign-in hint,
//     single-line gaps)
//   - plain rows: 3 rows per item, box chrome 6
//   - cards: 8 rows per card (7 thumbnail + separator), box chrome 6
//   - below the box (blank + two-line key hint): 3 rows
//
// A frame therefore costs 16 + 3*per plain, or 16 + 8*per in card mode.
func (m *Model) perPageFit() int {
	h := m.height
	if h < 1 {
		h = 35
	}
	switch {
	case m.cardMode():
		per := (h - 10) / (m.thumbRows() + 1)
		return clampInt(per, 1, 20)
	case m.compact:
		per := (h - 8) / 3
		return clampInt(per, 1, 12)
	default:
		per := (h - 10) / 3
		return clampInt(per, 1, 40)
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (m *Model) pageSize() int {
	return m.perPageFit()
}

// renderList renders the shared search/home result list. Cards look like
// YouTube search results: thumbnail on the left, title and meta on the right.
func (m *Model) renderList(heading string, list []core.SearchResult, above int) string {
	if len(list) == 0 {
		return m.styles.Hint.Render("Nothing here yet.")
	}

	maxW := m.listWidth()
	if maxW < 24 {
		maxW = 80
	}
	hover := m.cursor
	if hover >= len(list) {
		hover = len(list) - 1
	}

	per := m.perPageFit()
	start := m.cursor / per * per
	end := min(start+per, len(list))
	page := (m.cursor / per) + 1
	total := (len(list) + per - 1) / per
	head := heading + "  ·  page " + strconv.Itoa(page) + " of " + strconv.Itoa(total)

	var b strings.Builder
	b.WriteString(m.styles.Accent.Render(truncate(head, maxW)) + "\n")
	// Register a click zone per visible card: clicking a result opens it.
	cost := 3
	if m.cardMode() {
		cost = m.thumbRows() + 1
	}
	y := m.bodyTop + above + 2 // below the box top border and the heading
	for i := start; i < end; i++ {
		idx := i
		m.addZone(2, m.width-2, y, func(m *Model, _, _ int) tea.Cmd { return m.openAt(idx) })
		hovered := m.hoverY >= y && m.hoverY < y+cost && m.hoverX >= 2 && m.hoverX <= m.width-2
		m.renderCard(&b, list[i], i == hover, hovered, maxW)
		y += cost
	}
	// The last card ends with a blank separator; drop it so the box hugs the
	// content instead of leaving a stray empty row.
	body := strings.TrimSuffix(b.String(), "\n")
	return m.styles.Box.Render(body)
}

// renderCard draws one result as a two-column row: thumbnail art on the left,
// wrapped title and meta on the right. Both the graphics-mode placeholder
// block and the block-art fallback fill the same rectangle, so the layout is
// identical on every terminal. Very narrow windows fall back to plain rows.
// The selected card and the card under the mouse are lit up with a subtle
// background so clickable rows read as clickable.
func (m *Model) renderCard(b *strings.Builder, r core.SearchResult, selected, hovered bool, maxW int) {
	hot := selected || hovered
	hotStyle := m.styles.Sel
	if hovered && !selected {
		hotStyle = m.styles.Hover
	}
	if !m.cardMode() {
		prefix := "  "
		if selected {
			prefix = m.styles.Active.Render("> ")
		}
		title := truncate(r.Title, maxW-len(prefix))
		meta := strings.TrimRight("    "+r.Uploader+"  "+r.DurationLabel(), " ")
		if hot {
			b.WriteString(prefix + hotStyle.Width(maxW-len(prefix)).Render(title) + "\n")
			b.WriteString(hotStyle.Width(maxW).Render(meta) + "\n")
		} else {
			b.WriteString(prefix + title + "\n")
			b.WriteString(m.styles.Dim.Render(meta) + "\n")
		}
		b.WriteString("\n")
		return
	}
	cols, rows := m.thumbCols(), m.thumbRows()
	infoW := maxW - cols - 2
	if infoW < 16 {
		prefix := "  "
		if selected {
			prefix = m.styles.Active.Render("> ")
		}
		title := truncate(r.Title, maxW-len(prefix))
		meta := strings.TrimRight("    "+r.Uploader+"  "+r.DurationLabel(), " ")
		if hot {
			b.WriteString(prefix + hotStyle.Width(maxW-len(prefix)).Render(title) + "\n")
			b.WriteString(hotStyle.Width(maxW).Render(meta) + "\n")
		} else {
			b.WriteString(prefix + title + "\n")
			b.WriteString(m.styles.Dim.Render(meta) + "\n")
		}
		b.WriteString("\n")
		return
	}

	prefix := "  "
	if hot {
		prefix = hotStyle.Render("> ")
	} else if selected {
		prefix = m.styles.Active.Render("> ")
	}

	info := strings.Split(wrapText(r.Title, infoW), "\n")
	meta := strings.TrimRight(r.Uploader+"  "+r.DurationLabel(), " ")
	for len(info) < rows-1 {
		info = append(info, "")
	}
	info = info[:rows-1]
	info = append(info, meta)

	gfx := m.gfx != nil && m.thumbsOn()
	for row := 0; row < rows; row++ {
		art := ""
		switch {
		case gfx:
			if id, ok := m.imgs[r.ID]; ok {
				art = kittyArtSlice(id, cols, row)
			}
		default:
			if pixels := m.thumbs[r.ID]; pixels != "" {
				lines := strings.Split(pixels, "\n")
				if row < len(lines) {
					art = lines[row]
				}
			}
		}
		text := truncate(info[row], infoW)
		switch {
		case hot:
			text = hotStyle.Width(infoW).Render(text)
		case row == rows-1:
			text = m.styles.Dim.Render(text)
		}
		b.WriteString(prefix)
		b.WriteString(padCells(art, cols))
		b.WriteString("  ")
		b.WriteString(text)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

// kittyArtSlice writes one row of a placeholder block: the placeholder cells
// carry their grid row (and column) diacritics, so kitty paints the full image
// across the thumbCols x thumbRows rectangle anchored at this row's start.
func kittyArtSlice(id int64, cols, row int) string {
	var sb strings.Builder
	sb.WriteString("\x1b[38;5;" + strconv.FormatInt(id%256, 10) + "m")
	for c := 0; c < cols; c++ {
		sb.WriteRune(phRuneR)
		sb.WriteRune(phDia[row])
		sb.WriteRune(phDia[c])
	}
	sb.WriteString("\x1b[39m")
	return sb.String()
}

// padCells pads a rendered art row to the requested cell count so box borders
// align whether the row came from images, block art or nothing.
func padCells(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func (m *Model) thumbnail(id string) string {
	if !m.thumbsOn() {
		return ""
	}
	return m.thumbs[id]
}
