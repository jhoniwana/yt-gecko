package tui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"

	_ "golang.org/x/image/webp"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
)

var thumbClient = &http.Client{Timeout: 6 * time.Second}

// syncThumbLayout rebuilds the cached artwork when the thumbnail shape
// changes (video cards are 16:9, music covers are square), so nothing is
// stretched and no download is repeated: decoded images are kept and only
// re-encoded at the new size.
func (m *Model) syncThumbLayout() {
	music := m.musicLayout()
	if music == m.thumbLayout {
		return
	}
	m.thumbLayout = music
	m.albumID = 0
	m.imgs = make(map[string]int64)
	m.minis = make(map[string][]string)
	for id := range m.thumbs {
		if _, ok := m.thumbsImg[id]; !ok {
			delete(m.thumbs, id)
		}
	}
	m.reencodeThumbs()
	if m.mode == modeMusic {
		m.updateAlbumArt()
	}
}

// thumbCmd schedules async thumbnail fetches for every card on the current
// page, so the whole shelf is painted at once (like YouTube's search/home).
// Fetches are deduplicated and cached; failures simply leave the placeholder.
func (m *Model) thumbCmd() tea.Cmd {
	m.syncThumbLayout()
	if !m.thumbsOn() {
		return nil
	}
	list := m.currentList()
	if len(list) == 0 {
		return nil
	}
	per := m.pageSize()
	start := m.cursor / per * per
	end := min(start+per, len(list))
	if m.mode == modeMusic {
		// The queue shows many more rows than a card page; fetch the ones on
		// screen so their mini covers fill in.
		per = (m.height - m.bodyTop - 3) / 3
		if per < 1 {
			per = 1
		}
		if per > 20 {
			per = 20
		}
		start = 0
		if m.cursor >= per {
			start = m.cursor - per + 1
		}
		end = min(start+per, len(list))
	}

	var cmds []tea.Cmd
	cols, rows := m.thumbCols(), m.thumbRows()
	for i := start; i < end; i++ {
		item := list[i]
		vid := item.ID
		if vid == "" || m.thumbs[vid] != "" || m.thumbBusy[vid] {
			continue
		}
		m.thumbBusy[vid] = true
		url := thumbnailURL(vid)
		if item.ThumbURL != "" {
			url = item.ThumbURL
		}
		cmds = append(cmds, func(vid string) tea.Cmd {
			return func() tea.Msg {
				art, data, err := fetchThumb(url, vid, cols, rows)
				return thumbMsg{id: vid, art: art, data: data, err: err}
			}
		}(vid))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// thumbnailURL returns a thumbnail built from the video id, so no extra
// parsing or image libraries beyond the standard library are needed.
// maxresdefault is true 16:9 with no baked-in letterbox bars; every ordinary
// video has it, but a few channel defaults do not, so fetchThumb falls back.
func thumbnailURL(id string) string {
	return "https://i.ytimg.com/vi/" + id + "/maxresdefault.jpg"
}

// fetchThumb downloads the video's thumbnail from an explicit URL (the music
// shelves provide their own, since playlist ids are not video ids). It returns
// both the ANSI half-block art used on terminals without graphics support and
// the raw JPEG bytes (plus a small PNG copy) used on kitty-graphics terminals.
// The art is rendered for the given cell rectangle so it matches the zoom.
func fetchThumb(url, id string, cols, rows int) (string, []byte, error) {
	data, err := getThumb(url)
	if err != nil && id != "" {
		data, err = getThumb("https://i.ytimg.com/vi/" + id + "/hqdefault.jpg")
	}
	if err != nil {
		return "", nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", nil, err
	}
	return renderBlockArt(img, cols, rows), data, nil
}

func getThumb(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// Google's image CDN answers with webp when the client accepts anything;
	// asking for jpeg/png keeps the common path cheap (webp is still decoded
	// as a fallback thanks to the x/image/webp import).
	req.Header.Set("Accept", "image/jpeg,image/png")
	resp, err := thumbClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("thumb http %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

// cropToAspect center-crops img to the w:h aspect ratio, keeping as much of
// the source as possible. YouTube's maxresdefault is already 16:9, so for the
// default card grid this is normally a no-op.
func cropToAspect(img image.Image, w, h int) image.Image {
	b := img.Bounds()
	srcW := b.Max.X - b.Min.X
	srcH := b.Max.Y - b.Min.Y
	if srcW <= 0 || srcH <= 0 {
		return img
	}
	srcAspect := float64(srcW) / float64(srcH)
	targetAspect := float64(w) / float64(h)
	if srcAspect > targetAspect {
		cw := int(float64(srcH) * targetAspect)
		if cw > srcW {
			cw = srcW
		}
		x0 := b.Min.X + (srcW-cw)/2
		return subImage(img, x0, b.Min.Y, cw, srcH)
	}
	ch := int(float64(srcW) / targetAspect)
	if ch > srcH {
		ch = srcH
	}
	y0 := b.Min.Y + (srcH-ch)/2
	return subImage(img, b.Min.X, y0, srcW, ch)
}

func subImage(img image.Image, x, y, w, h int) image.Image {
	return img.(interface {
		SubImage(r image.Rectangle) image.Image
	}).SubImage(image.Rect(x, y, x+w, y+h))
}

// thumbPNG re-encodes a decoded image as a small PNG suitable for the kitty
// graphics protocol. It only depends on the standard library (image/png).
// JPEG is not a protocol format, so thumbnails are downscaled and sent as PNG.
func thumbPNG(img image.Image, cols, rows int) ([]byte, error) {
	// Each text cell is about two pixels tall in block-art terms, so the
	// rectangle's real aspect is cols x rows*2; crop to that so the image is
	// not letterboxed inside the placeholder grid.
	img = cropToAspect(img, cols, rows*2)
	bounds := img.Bounds()
	srcW := bounds.Max.X - bounds.Min.X
	srcH := bounds.Max.Y - bounds.Min.Y
	if srcW <= 0 || srcH <= 0 {
		return nil, fmt.Errorf("empty image")
	}
	w := 176
	h := w * srcH / srcW
	if h < 1 {
		h = 1
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy := bounds.Min.Y + y*srcH/h
		for x := 0; x < w; x++ {
			sx := bounds.Min.X + x*srcW/w
			out.Set(x, y, img.At(sx, sy))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// renderBlockArt scales img into a cols x rows grid. Each cell renders the
// upper half-block glyph ("▀") with the foreground set to the top pixel and
// the background set to the bottom pixel, giving full vertical resolution
// with only rows lines. The glyph is built at runtime to keep source ASCII.
func renderBlockArt(img image.Image, cols, rows int) string {
	bounds := img.Bounds()
	srcW := bounds.Max.X - bounds.Min.X
	srcH := bounds.Max.Y - bounds.Min.Y
	if srcW <= 0 || srcH <= 0 {
		return ""
	}

	cellW := srcW / cols
	cellH := srcH / (rows * 2)
	if cellW < 1 {
		cellW = 1
	}
	if cellH < 1 {
		cellH = 1
	}

	upper := string(rune(0x2580))
	var lines []string
	for r := 0; r < rows; r++ {
		var sb strings.Builder
		for c := 0; c < cols; c++ {
			top := sampleCenter(img, bounds, c*cellW, r*2*cellH, cellW, cellH)
			bottom := sampleCenter(img, bounds, c*cellW, r*2*cellH+cellH, cellW, cellH)
			sb.WriteString(ansiFore(top))
			sb.WriteString(ansiBack(bottom))
			sb.WriteString(upper)
			sb.WriteString("\x1b[0m")
		}
		lines = append(lines, sb.String())
	}
	return strings.Join(lines, "\n")
}

// sampleCenter returns the color at the center of the source rectangle.
func sampleCenter(img image.Image, b image.Rectangle, x, y, w, h int) color.Color {
	cx := b.Min.X + x + w/2
	cy := b.Min.Y + y + h/2
	if cx > b.Max.X-1 {
		cx = b.Max.X - 1
	}
	if cy > b.Max.Y-1 {
		cy = b.Max.Y - 1
	}
	return img.At(cx, cy)
}

func ansiFore(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r>>8, g>>8, b>>8)
}

func ansiBack(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
}
