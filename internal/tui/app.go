package tui

import (
	"bytes"
	"image"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/lipgloss"
	"github.com/jhoniwana/yt-gecko/internal/auth"
	"github.com/jhoniwana/yt-gecko/internal/core"
	"golang.org/x/sys/unix"
)

type mode int

const (
	modeHome mode = iota
	modeInput
	modeSearch
	modeLogin
	modeStatus
	modeWatch
	modeQuality
	modeMusic
)

// Model is the root Bubble Tea model for yt-gecko.
type Model struct {
	core   *core.GeckoCore
	styles Styles
	width  int
	height int
	mode   mode
	// compact drops the cards and sign-in hint on very short windows so the
	// key hint line always stays on screen.
	compact bool

	tab        int
	feeds      [][]core.SearchResult
	feedBusy   []bool
	feedErrors []error

	q          string
	qCursor    int
	inputMusic bool
	results    []core.SearchResult
	cursor  int
	current *core.SearchResult
	client  *core.Client
	playing bool
	loading bool

	related     []core.SearchResult
	relatedBusy bool
	relatedErr  error
	returnMode  mode

	browsers []auth.Browser
	loginCur int
	authName string
	verified bool
	busy     bool

	pendingBrowser auth.Browser
	recheckErr     string

	message string
	msgOK   bool
	err     error

	thumbs    map[string]string
	thumbBusy map[string]bool
	thumbsImg map[string]image.Image
	minis     map[string][]string

	gfx    *gfxWriter
	imgs   map[string]int64
	imgSeq int64

	// zoom scales the thumbnail cards (like a browser's page zoom). Positive
	// steps make the cards bigger, negative smaller; the whole layout reflows
	// and the cached artwork is re-encoded at the new cell size.
	zoom int

	// bodyTop is the terminal row where the body starts (below the top bar),
	// and zones are the clickable regions rebuilt on every frame: header
	// icons, section tabs and result cards.
	bodyTop int
	zones   []clickZone

	// hoverX/hoverY track the mouse for hover highlighting of clickable
	// elements (header icons, tabs, cards).
	hoverX, hoverY int

	// loadingText drives the spinner and status text while a video or search
	// is being resolved, so clicks always give visible feedback.
	loadingText string
	spin        int

	// Music player state. audioOnly selects the audio-only engine (no video
	// window) and the dedicated player UI; queue is the expanded playlist
	// being played; pos/dur/volume mirror mpv for the progress bar and volume
	// indicator; albumID is the kitty image id used for the square album art.
	audioOnly  bool
	queue      []core.SearchResult
	pos, dur   float64
	volume     int
	albumID    int64
	trackEnded bool

	// Music home: the YouTube Music shelves of the account, flattened for
	// navigation, the mood chips, and the current browse page ("" = home).
	musicShelves  []core.MusicShelf
	musicItems    []core.SearchResult
	musicChips    []core.MusicChip
	musicBrowseID string
	musicTitle    string
	thumbLayout   bool
}

// spinnerFrames is the braille spinner used for in-flight operations.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func spinnerFrame(i int) string {
	return spinnerFrames[i%len(spinnerFrames)]
}

type spinMsg struct{}

// spinCmd advances the spinner while a load is in flight.
func (m *Model) spinCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return spinMsg{} })
}

// clickZone is a rectangular, clickable region in terminal cells (x inclusive,
// y absolute from the top of the screen).
type clickZone struct {
	x0, x1, y0, y1 int
	act            func(*Model, int, int) tea.Cmd
}

func (m *Model) addZone(x0, x1, y int, act func(*Model, int, int) tea.Cmd) {
	m.zones = append(m.zones, clickZone{x0: x0, x1: x1, y0: y, y1: y, act: act})
}

func (m *Model) addZoneSpan(x0, x1, y0, y1 int, act func(*Model, int, int) tea.Cmd) {
	m.zones = append(m.zones, clickZone{x0: x0, x1: x1, y0: y0, y1: y1, act: act})
}

func (m *Model) hitZone(x, y int) func(*Model, int, int) tea.Cmd {
	for _, z := range m.zones {
		if y >= z.y0 && y <= z.y1 && x >= z.x0 && x <= z.x1 {
			return z.act
		}
	}
	return nil
}

// New creates the root model backed by the given engine. gfx is the graphics
// writer when the terminal supports the kitty graphics protocol, else nil.
func New(g *core.GeckoCore, gfx *gfxWriter) *Model {
	m := &Model{
		core:      g,
		styles:    defaultStyles(),
		thumbs:    make(map[string]string),
		thumbBusy: make(map[string]bool),
		thumbsImg: make(map[string]image.Image),
		minis:     make(map[string][]string),
		imgs:      make(map[string]int64),
		gfx:       gfx,
		browsers:  auth.DetectBrowsers(),
	}
	if b := g.Browser(); b != "" {
		m.authName = b.Label()
		m.verified = true
	} else if b := auth.LoadBrowser(); b != "" {
		m.core.SetBrowser(b)
		m.authName = b.Label()
		m.verified = true
	}
	if q := auth.LoadQuality(); q != "" {
		m.core.SetQuality(q)
	}
	return m
}

// Run launches the interactive TUI on the given engine.
func Run(g *core.GeckoCore) error {
	_, err := tea.NewProgram(New(g, nil), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

// Init returns the initial command (load the first home feed). Cookies are
// re-validated lazily on every feed/playback call with a fresh browser dump,
// so no background verify is needed at boot.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.ensureFeeds(), m.sizeProbe())
}

// sizeProbe keeps the model in sync with the real terminal size even when the
// emulator fails to emit WindowSizeMsg (e.g. a kitty window resized or split
// while the app gains no focus, or a tiling WM resizing under it). It polls
// the controlling terminal with a winsize ioctl and re-emits WindowSizeMsg
// whenever the size differs from the last one delivered.
func (m *Model) sizeProbe() tea.Cmd {
	return func() tea.Msg {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		var curW, curH int
		for range t.C {
			f, err := os.Open("/dev/tty")
			if err != nil {
				return nil
			}
			var sz *unix.Winsize
			if sz, err = unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ); err != nil {
				f.Close()
				return nil
			}
			f.Close()
			if curW == 0 {
				curW, curH = int(sz.Col), int(sz.Row)
				continue
			}
			if int(sz.Col) != curW || int(sz.Row) != curH {
				curW, curH = int(sz.Col), int(sz.Row)
				return tea.WindowSizeMsg{Width: curW, Height: curH}
			}
		}
		return nil
	}
}

// Update handles messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m, m.updateKey(msg)
	case tea.MouseMsg:
		return m, m.updateMouse(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.compact = msg.Height < 20
		return m, tea.Batch(m.reanchor(), m.sizeProbe())
	case feedMsg:
		m.feedBusy[msg.tab] = false
		if msg.err != nil {
			m.feedErrors[msg.tab] = msg.err
		} else {
			m.feeds[msg.tab] = msg.results
		}
		m.err = nil
		return m, m.thumbCmd()
	case searchResultsMsg:
		m.results = msg.results
		m.cursor = 0
		m.mode = modeSearch
		m.err = nil
		m.loading = false
		return m, m.thumbCmd()
	case playbackMsg:
		m.client = msg.client
		m.playing = true
		m.loading = false
		m.err = nil
		m.cursor = 0
		m.volume = 100
		m.trackEnded = false
		if m.audioOnly {
			m.mode = modeMusic
			m.pos, m.dur = 0, 0
			m.updateAlbumArt()
			return m, tea.Batch(m.musicTick(), m.thumbCmd())
		}
		m.mode = modeWatch
	case relatedMsg:
		m.relatedBusy = false
		m.err = nil
		if msg.err != nil {
			m.relatedErr = msg.err
		} else {
			m.related = msg.results
		}
		return m, m.thumbCmd()
	case thumbMsg:
		m.thumbBusy[msg.id] = false
		if msg.err != nil {
			break
		}
		if msg.art != "" {
			m.thumbs[msg.id] = msg.art
		}
		if len(msg.data) > 0 {
			if img, _, err := image.Decode(bytes.NewReader(msg.data)); err == nil {
				m.thumbsImg[msg.id] = img
				delete(m.minis, msg.id) // rebuild the mini cover with real art
				m.registerImage(msg.id, img)
				if m.audioOnly && m.current != nil && m.current.ID == msg.id {
					m.updateAlbumArt()
				}
			}
		}
	case loginResultMsg:
		m.busy = false
		m.pendingBrowser = msg.browser
		m.mode = modeStatus
		if msg.err != nil {
			m.message = msg.err.Error()
			m.msgOK = false
		} else {
			m.core.SetBrowser(msg.browser)
			_ = auth.SaveBrowser(msg.browser)
			m.authName = msg.browser.Label()
			m.verified = true
			m.recheckErr = ""
			m.message = "Signed in with " + m.authName + ". Cookies are reused for sign-in protected videos. Press o to open YouTube in the browser, or Enter to return home."
			m.msgOK = true
			return m, openBrowser(youtubeSignInURL)
		}
		return m, nil
	case recheckMsg:
		m.recheckErr = ""
		if msg.err != nil {
			m.recheckErr = "could not re-verify " + msg.browser.Label() + " cookies"
		}
		return m, nil
	case musicQueueMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			break
		}
		m.queue = msg.tracks
		m.cursor = 0
		return m, m.musicPlay(0)
	case musicHomeMsg:
		m.feedBusy[msg.tab] = false
		if msg.err != nil {
			m.feedErrors[msg.tab] = msg.err
			break
		}
		m.musicShelves = msg.home.Shelves
		m.musicChips = msg.home.Chips
		m.musicItems = core.FlattenShelves(msg.home.Shelves)
		m.musicBrowseID = ""
		m.musicTitle = "Home"
		m.cursor = 0
		return m, m.thumbCmd()
	case musicBrowseMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			break
		}
		m.musicShelves = msg.shelves
		m.musicItems = core.FlattenShelves(msg.shelves)
		m.musicBrowseID = msg.browseID
		m.musicTitle = msg.title
		m.cursor = 0
		return m, m.thumbCmd()
	case errMsg:
		m.err = msg.err
		m.loading = false
	case spinMsg:
		if m.loading {
			m.spin++
			return m, m.spinCmd()
		}
	case musicTickMsg:
		if msg.err == nil {
			if msg.pos >= 0 {
				m.pos = msg.pos
			}
			if msg.dur > 0 {
				m.dur = msg.dur
			}
		}
		if m.mode == modeMusic && m.playing {
			if msg.eof && !m.trackEnded {
				m.trackEnded = true
				return m, m.musicNext(1)
			}
			// Keep polling even when mpv has no clock yet: the first ticks
			// fail while the stream is still opening.
			return m, m.musicTick()
		}
	}
	return m, nil
}

func (m *Model) quitCmd() tea.Cmd {
	if m.gfx != nil {
		m.gfx.queue(kittyClear())
	}
	return tea.Quit
}

func (m *Model) updateKey(msg tea.KeyMsg) tea.Cmd {
	if msg.Type == tea.KeyCtrlC {
		return m.quitCmd()
	}
	if msg.String() == "tab" {
		if m.mode == modeSearch {
			m.mode = modeHome
			return m.thumbCmd()
		}
		if m.mode == modeHome {
			return m.switchTab(1)
		}
	}
	switch m.mode {
	case modeHome:
		return m.updateHome(msg)
	case modeInput:
		return m.updateInput(msg)
	case modeSearch:
		return m.updateSearch(msg)
	case modeLogin:
		return m.updateLogin(msg)
	case modeStatus:
		if msg.String() == "o" {
			return openBrowser(youtubeSignInURL)
		}
		if msg.String() == "u" {
			return m.useCookiesAnyway()
		}
		m.mode = modeHome
		return m.ensureFeeds()
	case modeWatch:
		return m.updateWatch(msg)
	case modeQuality:
		return m.updateQuality(msg)
	case modeMusic:
		return m.updateMusic(msg)
	}
	return nil
}

// useCookiesAnyway activates the last chosen (or saved) browser's cookies even
// when the verification step failed, so transient cookie or bot-check errors
// never block sign-in protected playback. Returns nil when nothing to use.
func (m *Model) useCookiesAnyway() tea.Cmd {
	b := m.pendingBrowser
	if b == "" {
		b = m.core.Browser()
	}
	if b == "" {
		return nil
	}
	m.core.SetBrowser(b)
	_ = auth.SaveBrowser(b)
	m.authName = b.Label()
	m.verified = true
	m.recheckErr = ""
	m.message = "Using " + m.authName + " cookies anyway. Press o to open YouTube in the browser, or Enter to return home."
	m.msgOK = true
	return nil
}

func (m *Model) switchTab(delta int) tea.Cmd {
	next := m.tab + delta
	if next < 0 || next >= len(feedSections) {
		return nil
	}
	m.tab = next
	m.cursor = 0
	m.err = nil
	m.audioOnly = next == musicTab
	return tea.Batch(m.loadFeed(next), m.thumbCmd())
}

func (m *Model) updateHome(msg tea.KeyMsg) tea.Cmd {
	if m.tab == musicTab && len(m.musicShelves) > 0 {
		return m.updateMusicHome(msg)
	}
	return m.updateHomeBase(msg)
}

// updateMusicHome navigates the YouTube Music shelf grid: j/k move by row,
// h/l (and the arrows) move item by item, and esc/backspace leave a browsed
// page. Every other key behaves like the regular home screen.
func (m *Model) updateMusicHome(msg tea.KeyMsg) tea.Cmd {
	cols := m.musicGridCols()
	switch msg.String() {
	case "j", "down":
		return m.moveCurrent(cols)
	case "k", "up":
		return m.moveCurrent(-cols)
	case "h", "left":
		return m.moveCurrent(-1)
	case "l", "right":
		return m.moveCurrent(1)
	case "esc", "backspace", "u":
		if m.musicBrowseID != "" {
			return m.musicBack()
		}
		return nil
	}
	return m.updateHomeBase(msg)
}

func (m *Model) updateHomeBase(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		return m.moveCurrent(1)
	case "k", "up":
		return m.moveCurrent(-1)
	case "right", "]":
		return m.switchTab(1)
	case "left", "[":
		return m.switchTab(-1)
	case "enter":
		if m.tab == len(feedSections)-1 && !m.verified {
			m.mode = modeLogin
			return nil
		}
		return m.openAt(m.cursor)
	case " ", "p":
		m.togglePause()
		return m.thumbCmd()
	case "/":
		m.openInput(m.tab == musicTab)
	case "a", "A":
		m.mode = modeLogin
	case "v", "V":
		return m.openQuality()
	case "q", "Q":
		return m.quitCmd()
	}
	return m.thumbCmd()
}

func (m *Model) updateSearch(msg tea.KeyMsg) tea.Cmd {
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
	case "/", "s":
		m.openInput(m.audioOnly)
	case "v", "V":
		return m.openQuality()
	case "h", "Q", "esc":
		m.mode = modeHome
		return m.ensureFeeds()
	case "q":
		m.mode = modeHome
		return m.ensureFeeds()
	}
	return m.thumbCmd()
}

func (m *Model) updateInput(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEnter:
		if strings.TrimSpace(m.q) == "" {
			m.backToList()
			return nil
		}
		return m.search()
	case tea.KeyEsc:
		m.backToList()
	case tea.KeyLeft:
		if m.qCursor > 0 {
			m.qCursor--
		}
	case tea.KeyRight:
		if m.qCursor < len([]rune(m.q)) {
			m.qCursor++
		}
	case tea.KeyHome, tea.KeyCtrlA:
		m.qCursor = 0
	case tea.KeyEnd, tea.KeyCtrlE:
		m.qCursor = len([]rune(m.q))
	case tea.KeyCtrlU:
		m.q = ""
		m.qCursor = 0
	case tea.KeySpace:
		m.insertAtCursor(" ")
	case tea.KeyBackspace, tea.KeyDelete, tea.KeyCtrlH:
		m.deleteBeforeCursor()
	case tea.KeyRunes:
		for _, r := range msg.Runes {
			if r == 0x7f || r == 0x08 { // DEL and backspace arrive as runes on some terminals
				m.deleteBeforeCursor()
				continue
			}
			m.insertAtCursor(string(r))
		}
	}
	return nil
}

// insertAtCursor inserts text at the query cursor.
func (m *Model) insertAtCursor(s string) {
	r := []rune(m.q)
	if m.qCursor > len(r) {
		m.qCursor = len(r)
	}
	ins := []rune(s)
	r = append(r[:m.qCursor], append(ins, r[m.qCursor:]...)...)
	m.qCursor += len(ins)
	m.q = string(r)
}

// deleteBeforeCursor removes the rune before the query cursor.
func (m *Model) deleteBeforeCursor() {
	r := []rune(m.q)
	if m.qCursor <= 0 || len(r) == 0 {
		return
	}
	if m.qCursor > len(r) {
		m.qCursor = len(r)
	}
	r = append(r[:m.qCursor-1], r[m.qCursor:]...)
	m.qCursor--
	m.q = string(r)
}

// openInput enters the search box. fromMusic selects the YouTube Music
// catalogue and the audio-only player for the results.
func (m *Model) openInput(fromMusic bool) {
	m.inputMusic = fromMusic
	m.mode = modeInput
	m.qCursor = len([]rune(m.q))
}

func (m *Model) backToList() {
	if m.mode == modeInput {
		if m.q != "" && len(m.results) > 0 {
			m.mode = modeSearch
			return
		}
		m.mode = modeHome
	}
}

// currentList returns the videos shown in the active list view.
func (m *Model) currentList() []core.SearchResult {
	switch m.mode {
	case modeSearch:
		return m.results
	case modeWatch:
		return m.related
	case modeMusic:
		if len(m.queue) > 0 {
			return m.queue
		}
		if m.returnMode == modeSearch {
			return m.results
		}
		return m.feedResults()
	}
	if m.tab == musicTab && len(m.musicItems) > 0 {
		return m.musicItems
	}
	return m.feedResults()
}

// registerImage transmits the fetched thumbnail PNG into the terminal's image
// registry and declares the placeholder rectangle for it. Later frames simply
// print placeholder cells (with the image id encoded in their color) wherever
// the thumbnail should appear, so redraws, reflows and resizes stay perfectly
// in sync with the text.
func (m *Model) registerImage(vid string, img image.Image) {
	if m.gfx == nil || img == nil {
		return
	}
	png, err := thumbPNG(img, m.thumbCols(), m.thumbRows())
	if err != nil {
		return
	}
	if m.imgSeq >= 255 { // 8-bit id space in the placeholder foreground color
		return
	}
	m.imgSeq++
	id := m.imgSeq
	if old, ok := m.imgs[vid]; ok {
		m.gfx.queue(kittyDelete(old))
	}
	m.imgs[vid] = id
	m.gfx.queue(kittyTransmit(id, png) + kittyVirtual(id, m.thumbCols(), m.thumbRows()))
}

// reanchor re-declares the virtual kitty placements after a window resize, so
// the terminal repaints every thumbnail at its new cell coordinates. The PNGs
// stay in the image registry (transmitted once per session); only the cell
// mappings need refreshing because resize reflows the gutter text.
func (m *Model) reanchor() tea.Cmd {
	if m.gfx != nil {
		for _, id := range m.imgs {
			m.gfx.queue(kittyVirtual(id, m.thumbCols(), m.thumbRows()))
		}
		if m.albumID != 0 {
			m.gfx.queue(kittyVirtual(m.albumID, albumCols, albumRows))
		}
	}
	return nil
}

// updateMouse handles the scroll wheel: a plain wheel moves the selection like
// the j/k keys, and ctrl+wheel zooms the interface (bigger or smaller cards),
// mirroring the browser shortcut.
func (m *Model) updateMouse(msg tea.MouseMsg) tea.Cmd {
	if msg.Action == tea.MouseActionMotion {
		m.hoverX, m.hoverY = msg.X, msg.Y
		return nil
	}
	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		if act := m.hitZone(msg.X, msg.Y); act != nil {
			return act(m, msg.X, msg.Y)
		}
		return nil
	}
	up := msg.Button == tea.MouseButtonWheelUp
	down := msg.Button == tea.MouseButtonWheelDown
	if !up && !down {
		return nil
	}
	if msg.Ctrl {
		if up {
			return m.zoomBy(1)
		}
		return m.zoomBy(-1)
	}
	switch m.mode {
	case modeHome, modeSearch, modeWatch:
		if up {
			return m.moveCurrent(-1)
		}
		return m.moveCurrent(1)
	case modeMusic:
		if up {
			return m.moveCurrent(-1)
		}
		return m.moveCurrent(1)
	case modeQuality:
		opts := coreQualityOptions()
		if up && m.cursor > 0 {
			m.cursor--
		}
		if down && m.cursor < len(opts)-1 {
			m.cursor++
		}
	}
	return nil
}

// zoomBy scales the thumbnail cards and re-encodes the cached artwork at the
// new cell size, so zooming repaints real images instead of stretching the old
// ones. The work is done inline (small PNGs, single render goroutine), which
// keeps the kitty image registry consistent without locking.
func (m *Model) zoomBy(delta int) tea.Cmd {
	z := clampInt(m.zoom+delta, -1, 1)
	if z == m.zoom {
		return nil
	}
	m.zoom = z
	m.reencodeThumbs()
	return m.thumbCmd()
}

// reencodeThumbs rebuilds every cached thumbnail at the current zoom: block
// art is re-rendered from the decoded pixels, and kitty images are re-encoded
// and re-transmitted. Clearing the registry first keeps the 8-bit image id
// space from filling up across many zoom steps.
func (m *Model) reencodeThumbs() {
	cols, rows := m.thumbCols(), m.thumbRows()
	if m.gfx != nil {
		m.gfx.queue(kittyClear())
		m.imgs = make(map[string]int64)
		m.imgSeq = 0
	}
	for vid, img := range m.thumbsImg {
		if img == nil {
			continue
		}
		if m.gfx != nil {
			m.registerImage(vid, img)
			continue
		}
		m.thumbs[vid] = renderBlockArt(img, cols, rows)
	}
}

// thumbsOn reports whether thumbnails should be fetched and drawn in the
// current mode: real images need a supporting terminal and a reasonably wide
// window, block art needs a wide window to look right.
func (m *Model) thumbsOn() bool {
	if m.gfx != nil {
		return m.width >= 60
	}
	return m.width >= 110
}

func (m *Model) moveCurrent(delta int) tea.Cmd {
	list := m.currentList()
	if len(list) == 0 {
		return nil
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(list) {
		m.cursor = len(list) - 1
	}
	return m.thumbCmd()
}

func (m *Model) togglePause() {
	m.playing = !m.playing
	if m.client != nil {
		_ = m.client.TogglePause()
	}
}

func (m *Model) search() tea.Cmd {
	query := strings.TrimSpace(m.q)
	if query == "" {
		return nil
	}
	m.loading = true
	m.loadingText = "Searching..."
	music := m.inputMusic
	m.audioOnly = music
	return tea.Batch(func() tea.Msg {
		var results []core.SearchResult
		var err error
		if music {
			results, err = m.core.MusicSearch(query, 20)
		} else {
			results, err = m.core.Search(query, 10)
		}
		if err != nil {
			return errMsg{err}
		}
		return searchResultsMsg{results}
	}, m.spinCmd())
}

func (m *Model) playAt(i int) tea.Cmd {
	list := m.currentList()
	if i < 0 || i >= len(list) {
		return nil
	}
	m.current = &list[i]
	audio := m.audioOnly
	return func() tea.Msg {
		client, err := m.core.Play(list[i].URL, audio)
		if err != nil {
			return errMsg{err}
		}
		return playbackMsg{client}
	}
}

func chomp(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return string(r[:len(r)-1])
}

// View renders the current frame.
func (m *Model) View() string {
	width := m.width
	if width < 40 {
		width = 80
	}
	gap := "\n\n"
	if m.compact {
		gap = "\n"
	}
	m.zones = m.zones[:0]
	m.bodyTop = strings.Count(gap, "\n")
	m.syncThumbLayout()
	content := m.topBar() + gap + m.body() + "\n\n" + m.help()
	// The app style pads two columns per side; wider lines would be wrapped by
	// lipgloss (which shreds boxes and cover art), so clamp every line first.
	content = truncateLines(content, width-4)
	out := m.styles.App.Width(width).Render(content)
	// The frame must be exactly as tall as the terminal: a shorter frame
	// leaves stale/blank rows below, and a taller one makes bubbletea drop
	// lines from the TOP (losing the top bar). Fit to height so every cell
	// is drawn every frame.
	if m.height > 0 {
		out = fitHeight(out, m.height)
	}
	return out
}

// truncateLines clamps every line of s to w cells (ANSI-aware), so lipgloss
// never wraps a rendered frame.
func truncateLines(s string, w int) string {
	if w < 20 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if lipgloss.Width(l) > w {
			lines[i] = ansi.Truncate(l, w, "")
		}
	}
	return strings.Join(lines, "\n")
}

// fitHeight returns s with exactly h lines. When s is taller it keeps the
// first h lines (never let the renderer drop our header); when shorter it
// pads with blank lines so the frame covers the full terminal height.
func fitHeight(s string, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		return strings.Join(lines[:h], "\n")
	}
	if len(lines) < h {
		return s + strings.Repeat("\n", h-len(lines))
	}
	return s
}

// listWidth is the column budget for a single text line inside the list box,
// accounting for app padding, the border, box padding and the line's lead.
// Calibrated so the box's own width equals the app's content width exactly:
// a 66-column window shows a box that ends exactly at column 66 instead of
// wrapping its border onto the next line.
func (m *Model) listWidth() int {
	n := m.width - 12
	if n < 20 {
		n = 20
	}
	return n
}

// wrapWidth is the column budget for free-standing hint and message text.
func (m *Model) wrapWidth() int {
	n := m.width - 12
	if n < 40 {
		n = 40
	}
	return n
}

// topBar draws the header. The icons are clickable (search, subscriptions,
// key help, account), which makes the interface discoverable without knowing
// the key bindings first.
func (m *Model) topBar() string {
	status := m.styles.Hint.Render("no account")
	if m.verified {
		status = m.styles.Success.Render("● " + m.authName)
	}
	type seg struct {
		text string
		act  func(*Model, int, int) tea.Cmd
	}
	segs := []seg{
		{"▶ yt-gecko", nil},
		{"    ", nil},
		{"⌕ Search", func(m *Model, _, _ int) tea.Cmd { m.mode = modeInput; return nil }},
		{"    ", nil},
		{"★ Subs", func(m *Model, _, _ int) tea.Cmd { return m.switchTab(len(feedSections) - 1) }},
		{"    ", nil},
		{"? Keys", func(m *Model, _, _ int) tea.Cmd {
			m.message = keysHelp
			m.msgOK = true
			m.mode = modeStatus
			return nil
		}},
		{"    ", nil},
		{"⚙ " + m.core.Quality(), func(m *Model, _, _ int) tea.Cmd { return m.openQuality() }},
		{"        ", nil},
	}
	var b strings.Builder
	x := 2 // the app style pads two columns on the left
	for _, s := range segs {
		style := m.styles.Help
		if s.act != nil {
			m.addZone(x, x+lipgloss.Width(s.text)-1, 0, s.act)
			if m.hoverY == 0 && m.hoverX >= x && m.hoverX <= x+lipgloss.Width(s.text)-1 {
				style = m.styles.Hover
			}
		}
		b.WriteString(style.Render(s.text))
		x += lipgloss.Width(s.text)
	}
	statusStyle := m.styles.Success
	if m.hoverY == 0 && m.hoverX >= x && m.hoverX <= x+lipgloss.Width(status)-1 {
		statusStyle = m.styles.Hover
	}
	m.addZone(x, x+lipgloss.Width(status)-1, 0, func(m *Model, _, _ int) tea.Cmd {
		m.mode = modeLogin
		m.cursor = 0
		return nil
	})
	b.WriteString(statusStyle.Render(status))
	b.WriteString("    ")
	if m.loading {
		b.WriteString(m.styles.Bar.Render(spinnerFrame(m.spin)+" "+truncate(m.loadingText, 40)) + "    ")
	}
	if m.playing && m.current != nil {
		b.WriteString(m.styles.Bar.Render("playing: " + truncate(m.current.Title, 40)))
	}
	return b.String()
}

// keysHelp is the cheat sheet shown from the header's ? icon.
const keysHelp = "j/k or wheel: move    enter: play    space: pause    [ ]: sections    /: search    h: back    a: account    ctrl+wheel: zoom    q: quit"

func (m *Model) body() string {
	switch m.mode {
	case modeHome:
		return m.renderHome()
	case modeSearch:
		return m.renderSearch()
	case modeInput:
		return m.renderInput()
	case modeLogin:
		return m.renderLogin()
	case modeStatus:
		return m.renderStatus()
	case modeWatch:
		return m.renderWatch()
	case modeQuality:
		return m.renderQuality()
	case modeMusic:
		return m.renderMusic()
	}
	return ""
}

func (m *Model) renderHome() string {
	var b strings.Builder
	above := 0
	if m.err != nil && m.mode == modeHome {
		er := m.styles.Error.Render("Error: " + m.err.Error())
		b.WriteString(er + "\n\n")
		above += lipgloss.Height(er) + 2
	}
	b.WriteString(m.feedTabBar() + "\n")
	above += 1
	if !m.verified {
		hint := m.styles.Hint.Render(wrapText("Not signed in - press a to log in with your browser. Unlocks subscriptions and sign-in protected videos.", m.wrapWidth()))
		b.WriteString(hint + "\n")
		above += lipgloss.Height(hint) + 1
	} else if m.mode == modeHome && !m.playing && !m.compact {
		ok := m.styles.Success.Render("Signed in with " + m.authName + " - press a to change account")
		b.WriteString(ok + "\n")
		above += lipgloss.Height(ok) + 1
	}
	if m.recheckErr != "" {
		warn := m.styles.Warn.Render(m.recheckErr + ". If sign-in protected videos fail to open, press a and log in again.")
		b.WriteString(warn + "\n")
		above += lipgloss.Height(warn) + 1
	}
	if !m.compact {
		b.WriteString("\n")
		above += 1
	}

	tab := m.tab
	if tab >= len(feedSections) {
		return b.String()
	}
	if m.feedBusy[tab] {
		loading := m.styles.Hint.Render("Loading " + feedSections[tab] + "...")
		b.WriteString(loading + "\n\n")
		above += lipgloss.Height(loading) + 2
	}
	if m.feedErrors[tab] != nil {
		warn := m.styles.Warn.Render(m.feedErrors[tab].Error())
		b.WriteString(warn + "\n\n")
		above += lipgloss.Height(warn) + 2
	}
	if tab == len(feedSections)-1 && !m.verified {
		b.WriteString(m.styles.Hint.Render("Press a to log in with your browser; the subscriptions feed unlocks after sign-in.") + "\n")
		above += 1
	}
	if m.playing && m.current != nil {
		// Compact now-playing strip; clicking it opens the full player.
		strip := "♪ " + truncate(m.current.Title, 40) + "   " + fmtTime(m.pos) + " / " + fmtTime(m.dur)
		y := m.bodyTop + above
		m.addZone(2, m.width-2, y, func(m *Model, _, _ int) tea.Cmd {
			if m.audioOnly {
				m.mode = modeMusic
			} else {
				m.mode = modeWatch
			}
			return nil
		})
		b.WriteString(m.styles.Bar.Render(strip) + "\n\n")
		above += 2
	}
	if tab == musicTab && len(m.musicShelves) > 0 {
		b.WriteString(m.renderMusicHome(above))
		return b.String()
	}
	b.WriteString(m.renderList("Home", m.feedResults(), above))
	return b.String()
}

func (m *Model) renderSearch() string {
	var b strings.Builder
	above := 0
	if m.err != nil {
		er := m.styles.Error.Render("Error: " + m.err.Error())
		b.WriteString(er + "\n\n")
		above += lipgloss.Height(er) + 2
	}
	if m.loading && len(m.results) == 0 {
		b.WriteString(m.styles.Hint.Render("Searching...") + "\n\n")
		above += 3
	}
	if m.playing && m.current != nil {
		// Compact now-playing strip; clicking it opens the full player.
		strip := "♪ " + truncate(m.current.Title, 40) + "   " + fmtTime(m.pos) + " / " + fmtTime(m.dur)
		y := m.bodyTop + above
		m.addZone(2, m.width-2, y, func(m *Model, _, _ int) tea.Cmd {
			if m.audioOnly {
				m.mode = modeMusic
			} else {
				m.mode = modeWatch
			}
			return nil
		})
		b.WriteString(m.styles.Bar.Render(strip) + "\n\n")
		above += 2
	}
	heading := "Results: " + m.q
	if m.audioOnly {
		heading = "Music: " + m.q
	}
	b.WriteString(m.renderList(heading, m.results, above))
	return b.String()
}

func (m *Model) renderInput() string {
	title := "Search YouTube"
	if m.inputMusic {
		title = "Search YouTube Music"
	}
	q := []rune(m.q)
	cursor := clampInt(m.qCursor, 0, len(q))
	var b strings.Builder
	b.WriteString(m.styles.Accent.Render(title) + "\n\n")
	b.WriteString(m.styles.Header.Render("Search: "))
	b.WriteString(string(q[:cursor]))
	b.WriteString(m.styles.Accent.Render("_"))
	b.WriteString(string(q[cursor:]))
	b.WriteString("\n\n")
	b.WriteString(m.styles.Muted.Render("enter: search    esc: cancel    ←/→: move    ctrl+u: clear"))
	if m.loading {
		b.WriteString("\n" + m.styles.Bar.Render(spinnerFrame(m.spin)+" "+m.loadingText))
	}
	if m.err != nil {
		b.WriteString("\n" + m.styles.Error.Render(wrapText(m.err.Error(), m.wrapWidth())))
	}
	return m.styles.Box.Render(b.String())
}

func (m *Model) renderStatus() string {
	var b strings.Builder
	if m.msgOK {
		b.WriteString(m.styles.Success.Render(wrapText(m.message, m.wrapWidth())))
	} else {
		b.WriteString(m.styles.Error.Render(wrapText(m.message, m.wrapWidth())))
	}
	hint := "any key: home    o: open browser"
	if !m.msgOK && m.pendingBrowser != "" {
		hint += "    u: use " + m.pendingBrowser.Label() + " cookies anyway"
	}
	b.WriteString("\n\n" + m.styles.Hint.Render(hint))
	return m.styles.Box.Render(b.String())
}

func (m *Model) help() string {
	switch m.mode {
	case modeHome:
		return m.styles.Help.Render("j/k: navigate    enter: play    [ ]: sections    /: search    v: quality    ctrl+wheel: zoom    q: quit")
	case modeSearch:
		return m.styles.Help.Render("j/k: navigate    enter: play    space: pause    /: search    h: home    q: home")
	case modeInput:
		return m.styles.Help.Render("type a query, enter: search    esc: cancel")
	case modeLogin:
		return m.styles.Help.Render("j/k: navigate    enter: verify    o: open browser    esc: back")
	case modeStatus:
		return m.styles.Help.Render("")
	case modeWatch:
		return m.styles.Help.Render("j/k: pick next    enter: watch    space: pause    v: quality    [ ]: skip    /: search    h: back")
	case modeQuality:
		return m.styles.Help.Render("j/k: pick quality    enter: apply    esc: cancel")
	case modeMusic:
		return m.styles.Help.Render("space: pause    n/p: next/prev    ←/→: seek    -/+: volume    j/k: queue    enter: play    v: quality    h: back")
	}
	return ""
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "~"
}

// wrapText word-wraps s to a maximum line width, preserving flow text.
func wrapText(s string, n int) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return s
	}
	var b strings.Builder
	lineLen := 0
	for _, w := range words {
		if lineLen > 0 && lineLen+1+len(w) > n {
			b.WriteString("\n")
			lineLen = 0
		}
		if lineLen > 0 {
			b.WriteString(" ")
		}
		b.WriteString(w)
		lineLen += len(w)
	}
	return b.String()
}

type feedMsg struct {
	tab     int
	results []core.SearchResult
	err     error
}

type searchResultsMsg struct {
	results []core.SearchResult
}

type playbackMsg struct {
	client *core.Client
}

type relatedMsg struct {
	results []core.SearchResult
	err     error
}

type thumbMsg struct {
	id   string
	art  string
	data []byte
	err  error
}

type errMsg struct {
	err error
}
