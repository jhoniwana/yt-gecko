package tui

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Small hand-rolled implementation of the kitty graphics protocol, so
// thumbnails can be drawn as real images in terminals that implement it
// (kitty, ghostty, wezterm, foot). Everything is framed as APC escapes:
// ESC _ G <control data> ; <payload> ESC \.

const (
	apcG     = "\x1b_G" // start of a graphics APC
	apcEnd   = "\x1b\\" // string terminator (ST)
	gfxChunk = 2600     // raw payload bytes per chunk (base64 stays under 4K)

	phRuneR = '\U0010EEEE' // kitty unicode image placeholder

	// Thumbnail card grid at zoom 0. Terminal cells are about twice as tall
	// as they are wide, so a 14x4 cell block is the 16:9 rectangle YouTube
	// thumbnails need; the image fills the placeholder with no letterbox.
	thumbCols = 14
	thumbRows = 4
)

// thumbCols returns the thumbnail width in cells for the current zoom level.
// Music covers are always the fixed square grid: the album art and the shelf
// grid must agree or kitty stretches the image.
func (m *Model) thumbCols() int {
	if m.musicLayout() {
		return albumCols
	}
	return thumbCols + 2*m.zoom
}

// thumbRows returns the thumbnail height in cells for the current zoom level.
func (m *Model) thumbRows() int {
	if m.musicLayout() {
		return albumRows
	}
	return thumbRows + m.zoom
}

// musicLayout reports whether thumbnails are drawn as square album covers:
// the music home shelves and the music player use that shape.
func (m *Model) musicLayout() bool {
	if m.mode == modeMusic {
		return true
	}
	return m.mode == modeHome && m.tab == musicTab && len(m.musicShelves) > 0
}

// gfxWriter wraps the program's output so stacked graphics escapes are
// flushed immediately after the text frame that triggered them. The graphics
// layer is text-only (Unicode placeholders), so it never touches the cursor:
// bubbletea's diff renderer stays perfectly in sync across redraws, resizes
// and reflows. All rendering runs on bubbletea's single render goroutine, so
// no locking is needed.
//
// Fd reports the underlying terminal's file descriptor so bubbletea treats
// the writer as a real TTY output and delivers WindowSizeMsg on startup and
// across resizes; bubbletea never reads or closes the output, so Read and
// Close are deliberate no-ops.
type gfxWriter struct {
	w       io.Writer
	fd      uintptr
	pending strings.Builder
}

func NewGFXWriter(w io.Writer) *gfxWriter {
	g := &gfxWriter{w: w}
	if f, ok := w.(*os.File); ok {
		g.fd = f.Fd()
	}
	return g
}

// Fd lets bubbletea see this writer as term.File (io.ReadWriteCloser + Fd).
func (g *gfxWriter) Fd() uintptr { return g.fd }

// Read satisfies io.Reader; bubbletea never reads from the output side.
func (g *gfxWriter) Read([]byte) (int, error) { return 0, io.EOF }

// Close satisfies io.Closer; the underlying file is owned by the caller.
func (g *gfxWriter) Close() error { return nil }

// Write implements io.Writer and appends any queued graphics escapes once the
// frame bytes have been handed to the terminal.
func (g *gfxWriter) Write(p []byte) (int, error) {
	n, err := g.w.Write(p)
	if g.pending.Len() > 0 {
		g.w.Write([]byte(g.pending.String()))
		g.pending.Reset()
	}
	return n, err
}

// queue buffers a graphics escape sequence for the next frame flush.
func (g *gfxWriter) queue(s string) {
	g.pending.WriteString(s)
}

// kittyTransmit registers a PNG in the terminal's image registry (quiet mode,
// no reply). Images stay in the registry until deleted, so each video is
// transmitted once per session and placeholders just reference its id.
func kittyTransmit(id int64, data []byte) string {
	var b strings.Builder
	enc := base64.StdEncoding
	first := true
	for len(data) > 0 {
		n := gfxChunk
		if n > len(data) {
			n = len(data)
		}
		payload := enc.EncodeToString(data[:n])
		m := 0 // last chunk
		if n < len(data) {
			m = 1 // more chunks follow
		}
		if first {
			fmt.Fprintf(&b, "%sa=t,i=%d,f=100,q=2,m=%d;%s%s", apcG, id, m, payload, apcEnd)
			first = false
		} else {
			fmt.Fprintf(&b, "%sm=%d;%s%s", apcG, m, payload, apcEnd)
		}
		data = data[n:]
	}
	return b.String()
}

// kittyVirtual declares the cell rectangle the placeholder glyphs fill for a
// registered image id (U=1 virtual placement). C=1 asks kitty to keep the
// cursor where it is, so the APC does not desync bubbletea with bubble cocoa on
// the next frame (which would smear the diff renderer across key presses).
func kittyVirtual(id int64, cols, rows int) string {
	return fmt.Sprintf("%sa=p,U=1,i=%d,c=%d,r=%d,q=2,C=1;%s", apcG, id, cols, rows, apcEnd)
}

// phDia maps a placeholder grid row or column number to the combining
// diacritic that encodes it (kitty's rowcolumn-diacritics list, class 230).
// Entries cover 0..15, which is enough for the thumbCols x thumbRows grid.
var phDia = []rune{
	0x0305, 0x030d, 0x030e, 0x0310, 0x0312, 0x033d, 0x033e, 0x033f,
	0x0346, 0x034a, 0x034b, 0x034c, 0x0350, 0x0351, 0x0352, 0x0357,
}

// kittyPlaceholder builds a cols x rows block of printable placeholder cells.
// The foreground color encodes the image id; each cell also carries combining
// diacritics for its grid row and column, so every cell maps to the correct
// part of the image even when a cell is redrawn on its own (which bubbletea's
// diff renderer does all the time). Without the diacritics every cell would
// fall back to row 0 and the image would render as a single corrupted strip.
func kittyPlaceholder(id int64, cols, rows int) string {
	var b strings.Builder
	if cols > len(phDia) || rows > len(phDia) {
		panic("placeholder grid exceeds the diacritic table")
	}
	fg := fmt.Sprintf("\x1b[38;5;%dm", id%256)
	for r := 0; r < rows; r++ {
		b.WriteString(fg)
		for c := 0; c < cols; c++ {
			b.WriteRune(phRuneR)
			b.WriteRune(phDia[r])
			b.WriteRune(phDia[c])
		}
		b.WriteString("\x1b[39m")
		if r < rows-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// kittyClear returns the escape bytes that drop every image and placement.
func kittyClear() string {
	return fmt.Sprintf("%sa=d,d=A;%s", apcG, apcEnd)
}

// kittyDelete drops a single registered image (data and placements).
func kittyDelete(id int64) string {
	return fmt.Sprintf("%sa=d,d=i,i=%d,q=2;%s", apcG, id, apcEnd)
}

// ProbeGraphics reports whether the controlling terminal implements the kitty
// graphics protocol. Per the spec we send a query action (a=q) right after a
// primary device attributes request (ESC[c): a supporting terminal answers
// with an ESC_G... reply, every other terminal answers only the DA request.
//
// The probe must run in raw, non-blocking mode: in canonical mode the line
// discipline holds the answer (it has no trailing newline), SetReadDeadline
// does not work on ttys, and the query would otherwise be echoed to the
// screen. The saved termios and fd flags are restored before returning.
func ProbeGraphics(stdin *os.File, stdout io.Writer) bool {
	if stdin == nil {
		return false
	}
	fi, err := stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false // stdin is not an interactive terminal
	}

	fd := int(stdin.Fd())
	var saved *unix.Termios
	if ti, err := unix.IoctlGetTermios(fd, unix.TCGETS); err == nil {
		c := *ti
		c.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
			unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
		c.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
		c.Cflag &^= unix.CSIZE | unix.PARENB
		c.Cflag |= unix.CS8
		c.Oflag &^= unix.OPOST
		c.Cc[unix.VMIN] = 1
		c.Cc[unix.VTIME] = 0
		if err := unix.IoctlSetTermios(fd, unix.TCSETS, &c); err == nil {
			saved = ti
		}
	}
	_ = unix.SetNonblock(fd, true)
	defer func() {
		_ = unix.SetNonblock(fd, false)
		if saved != nil {
			_ = unix.IoctlSetTermios(fd, unix.TCSETS, saved)
		}
	}()

	// 1x1 RGBA probe image. Even a rejected payload still proves the
	// terminal PARSED the APC and answered, which is all we need.
	probe := "\x1b[c" + apcG + "a=q,i=971,s=1,v=1,f=32;AAAA" + apcEnd
	if _, err := io.WriteString(stdout, probe); err != nil {
		return false
	}

	deadline := time.Now().Add(400 * time.Millisecond)
	buf := make([]byte, 512)
	found := false
	for time.Now().Before(deadline) {
		n, err := stdin.Read(buf)
		if n > 0 && bytes.Contains(buf[:n], []byte{0x1b, 0x5f}) {
			found = true
		}
		if err != nil {
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			break
		}
		if n == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
	return found
}
