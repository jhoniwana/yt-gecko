package core

import (
	"encoding/json"
	"fmt"
	"strings"
)

// YouTube Music is served by its own innerTube client (WEB_REMIX) on the
// music.youtube.com origin, with its own API key. The account's home is
// paginated: the first page carries a few carousels plus a continuation
// token, and every continuation adds more shelves ("Listen again", "Quick
// picks", mixes...). This file flattens those carousels into titled shelves.

const (
	musicOrigin       = "https://music.youtube.com"
	musicBrowsePath   = "/youtubei/v1/browse"
	musicSearchPath   = "/youtubei/v1/search"
	musicHomeBrowseID = "FEmusic_home"

	// musicHomePages bounds how many home pages are fetched: enough for the
	// full shelf list without turning startup into a crawl.
	musicHomePages = 4
)

// musicContext is the innerTube client context used by YouTube Music.
func musicContext() map[string]any {
	return map[string]any{
		"client": map[string]any{
			"clientName":    "WEB_REMIX",
			"clientVersion": "1.20260913.16.00",
			"hl":            "en",
			"gl":            "US",
		},
	}
}

// musicFallbackKey is the public innerTube key of music.youtube.com (the main
// site uses a different one; the wrong key returns a reduced home).
const musicFallbackKey = "AIzaSyC9XL3ZjWddXya6X74dJoCTL-WEYFDNX30"

// musicHomeURL is the page the music API key and visitor id are scraped from.
const musicHomeURL = "https://music.youtube.com/"

// MusicShelf is one titled carousel of the music home (Listen again, Quick
// picks, Albums for you...).
type MusicShelf struct {
	Title string
	Items []SearchResult
}

// MusicChip is a mood/genre chip from the music home header. The selection is
// carried in Params, so browsing it means re-requesting FEmusic_home with it.
type MusicChip struct {
	Label    string
	BrowseID string
	Params   string
}

// MusicHome is the structured YouTube Music home: shelves in display order
// plus the mood chips row.
type MusicHome struct {
	Shelves []MusicShelf
	Chips   []MusicChip
}

// MusicHome returns the signed-in account's YouTube Music home: the personal
// library (playlists and Liked Music) followed by the home shelves, following
// the page continuations so the shelf list matches what the web player shows.
func (g *GeckoCore) MusicHome(maxResults int) (*MusicHome, error) {
	s, err := g.newInnerTubeSession()
	if err != nil {
		return nil, err
	}

	home := &MusicHome{}

	// The library comes first: the liked-playlists page holds the user's own
	// playlists, and the liked-songs page gives Liked Music a cover and a
	// playable auto playlist (unless the grid already listed it).
	var lib []SearchResult
	if shelves, err := g.musicPageWith(s, "FEmusic_liked_playlists", "", 30); err == nil {
		lib = append(lib, FlattenShelves(shelves)...)
	}
	hasLiked := false
	for _, it := range lib {
		if it.Title == "Liked Music" || it.PlaylistID == "FEmusic_liked_videos" {
			hasLiked = true
			break
		}
	}
	if !hasLiked {
		if shelves, err := g.musicPageWith(s, "FEmusic_liked_videos", "", 3); err == nil {
			if items := FlattenShelves(shelves); len(items) > 0 {
				card := SearchResult{
					ID:         "FEmusic_liked_videos",
					Title:      "Liked Music",
					Uploader:   "Auto playlist",
					PlaylistID: "FEmusic_liked_videos",
					ThumbURL:   items[0].ThumbURL,
				}
				lib = append([]SearchResult{card}, lib...)
			}
		}
	}
	if len(lib) > 0 {
		home.Shelves = append(home.Shelves, MusicShelf{Title: "Your library", Items: lib})
	}

	raw, err := s.postTo(musicOrigin, musicBrowsePath, musicOrigin+"/", map[string]any{
		"context":  musicContext(),
		"browseId": musicHomeBrowseID,
	})
	if err != nil {
		if len(home.Shelves) == 0 {
			return nil, err
		}
		return home, nil
	}

	if doc, err := decodeJSON(raw); err == nil {
		home.Chips = parseMusicChips(doc)
	}

	total := 0
	for page := 0; page < musicHomePages; page++ {
		doc, err := decodeJSON(raw)
		if err != nil {
			if page == 0 && len(home.Shelves) == 0 {
				return nil, err
			}
			break
		}
		shelves := parseShelfSections(musicSections(doc), maxResults, &total)
		home.Shelves = append(home.Shelves, shelves...)
		if total >= maxResults {
			break
		}
		cont := continuationToken(doc)
		if cont == "" {
			break
		}
		raw, err = s.postTo(musicOrigin, musicBrowsePath, musicOrigin+"/", map[string]any{
			"context":      musicContext(),
			"continuation": cont,
		})
		if err != nil {
			break
		}
	}

	if len(home.Shelves) == 0 {
		return nil, fmt.Errorf("no music items in response")
	}
	return home, nil
}

// MusicSearch queries the YouTube Music catalogue (songs, albums, artists,
// playlists) instead of the video site, so results play audio-only and mixes
// expand into a queue.
func (g *GeckoCore) MusicSearch(query string, maxResults int) ([]SearchResult, error) {
	s, err := g.newInnerTubeSession()
	if err != nil {
		return nil, err
	}
	raw, err := s.postTo(musicOrigin, musicSearchPath, musicOrigin+"/", map[string]any{
		"context": musicContext(),
		"query":   query,
	})
	if err != nil {
		return nil, err
	}
	doc, err := decodeJSON(raw)
	if err != nil {
		return nil, err
	}
	total := 0
	shelves := parseShelfSections(musicSections(doc), maxResults, &total)
	items := FlattenShelves(shelves)
	if len(items) == 0 {
		return nil, fmt.Errorf("no music results")
	}
	return items, nil
}

// MusicBrowse loads a YouTube Music page (a playlist, album, mix or a mood
// selected through params) as its shelves. Pass "" for params when not
// selecting a mood. Playlist and album ids are VL-prefixed on the wire; the
// parser strips that prefix for display, so it is added back here.
func (g *GeckoCore) MusicBrowse(browseID, params string, maxResults int) ([]MusicShelf, error) {
	s, err := g.newInnerTubeSession()
	if err != nil {
		return nil, err
	}
	return g.musicPageWith(s, browseID, params, maxResults)
}

// musicPageWith browses a music page on an existing session.
func (g *GeckoCore) musicPageWith(s *innerTubeSession, browseID, params string, maxResults int) ([]MusicShelf, error) {
	body := map[string]any{
		"context":  musicContext(),
		"browseId": musicBrowseWireID(browseID),
	}
	if params != "" {
		body["params"] = params
	}
	raw, err := s.postTo(musicOrigin, musicBrowsePath, musicOrigin+"/", body)
	if err != nil {
		return nil, err
	}
	doc, err := decodeJSON(raw)
	if err != nil {
		return nil, err
	}
	total := 0
	shelves := parseShelfSections(musicSections(doc), maxResults, &total)
	if len(shelves) == 0 {
		return nil, fmt.Errorf("no music items in response")
	}
	return shelves, nil
}

// musicBrowseWireID re-adds the VL prefix that playlist and album browse ids
// carry on the wire (the parser strips it for display). Special FEmusic_*
// pages are used verbatim.
func musicBrowseWireID(id string) string {
	if id == "" || strings.HasPrefix(id, "VL") || strings.HasPrefix(id, "FEmusic") {
		return id
	}
	return "VL" + id
}

// FlattenShelves concatenates shelf items in order.
func FlattenShelves(shelves []MusicShelf) []SearchResult {
	var out []SearchResult
	for _, s := range shelves {
		out = append(out, s.Items...)
	}
	return out
}

// musicSections returns the ordered section array of a browse response,
// covering the home (single column), playlist/album pages (two column) and
// the continuation pages of both.
func musicSections(doc map[string]any) []any {
	paths := [][]string{
		{"contents", "singleColumnBrowseResultsRenderer", "tabs", "0", "tabRenderer", "content", "sectionListRenderer", "contents"},
		{"contents", "tabbedSearchResultsRenderer", "tabs", "0", "tabRenderer", "content", "sectionListRenderer", "contents"},
		{"contents", "twoColumnBrowseResultsRenderer", "secondaryContents", "sectionListRenderer", "contents"},
		{"continuationContents", "sectionListContinuation", "contents"},
		{"continuationContents", "musicPlaylistShelfContinuation", "contents"},
		{"continuationContents", "musicShelfContinuation", "contents"},
	}
	for _, p := range paths {
		if s, ok := dig(doc, p...).([]any); ok {
			return s
		}
	}
	return nil
}

// parseShelfSections flattens ordered sections into titled shelves, stopping
// once total reaches maxResults.
func parseShelfSections(sections []any, maxResults int, total *int) []MusicShelf {
	var out []MusicShelf
	for _, section := range sections {
		if *total >= maxResults {
			break
		}
		m, ok := section.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"musicCarouselShelfRenderer", "musicShelfRenderer", "musicPlaylistShelfRenderer", "gridRenderer", "itemSectionRenderer"} {
			renderer, ok := m[key].(map[string]any)
			if !ok {
				continue
			}
			contents, _ := renderer["contents"].([]any)
			if key == "gridRenderer" {
				// Library pages use a plain grid of tiles.
				contents, _ = renderer["items"].([]any)
			}
			var items []SearchResult
			for _, content := range contents {
				if *total >= maxResults {
					break
				}
				if r, ok := musicItem(content); ok {
					items = append(items, finishResult(r))
					*total++
				}
			}
			if len(items) > 0 {
				out = append(out, MusicShelf{Title: shelfTitle(renderer), Items: items})
			}
		}
	}
	return out
}

// continuationToken returns the next-page token of a browse response.
func continuationToken(doc map[string]any) string {
	if conts, ok := dig(doc, "contents", "singleColumnBrowseResultsRenderer", "tabs", "0", "tabRenderer", "content", "sectionListRenderer", "continuations").([]any); ok {
		if t := findContinuation(conts); t != "" {
			return t
		}
	}
	if node, ok := findNode(doc, "continuations"); ok {
		if t := findContinuation(node); t != "" {
			return t
		}
	}
	return ""
}

// findContinuation returns the first continuation token under a node.
func findContinuation(node any) string {
	switch n := node.(type) {
	case map[string]any:
		if v, ok := n["continuation"].(string); ok && v != "" {
			return v
		}
		for _, v := range n {
			if t := findContinuation(v); t != "" {
				return t
			}
		}
	case []any:
		for _, v := range n {
			if t := findContinuation(v); t != "" {
				return t
			}
		}
	}
	return ""
}

// shelfTitle extracts the display title of a carousel or shelf section.
func shelfTitle(renderer map[string]any) string {
	header, ok := renderer["header"].(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"musicCarouselShelfBasicHeaderRenderer", "musicShelfRenderer", "musicHeaderRenderer"} {
		if h, ok := header[key].(map[string]any); ok {
			if t := runsText(h["title"]); t != "" {
				return t
			}
		}
	}
	if t := runsText(header["title"]); t != "" {
		return t
	}
	return ""
}

// parseMusicChips collects the mood/genre chips of the music home header.
func parseMusicChips(doc map[string]any) []MusicChip {
	node, ok := findNode(doc, "chipCloudRenderer")
	if !ok {
		return nil
	}
	m, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	chips, _ := m["chips"].([]any)
	var out []MusicChip
	for _, c := range chips {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		chip, ok := cm["chipCloudChipRenderer"].(map[string]any)
		if !ok {
			continue
		}
		label := runsText(chip["text"])
		if label == "" {
			continue
		}
		id, params := "", ""
		if nav, ok := chip["navigationEndpoint"].(map[string]any); ok {
			if be, ok := nav["browseEndpoint"].(map[string]any); ok {
				id, _ = be["browseId"].(string)
				params, _ = be["params"].(string)
			}
		}
		if id == "" {
			continue
		}
		out = append(out, MusicChip{Label: label, BrowseID: id, Params: params})
	}
	return out
}

// parseMusicShelves flattens a response into plain results in shelf order.
func parseMusicShelves(raw []byte, maxResults int) ([]SearchResult, error) {
	doc, err := decodeJSON(raw)
	if err != nil {
		return nil, err
	}
	total := 0
	shelves := parseShelfSections(musicSections(doc), maxResults, &total)
	if len(shelves) == 0 {
		return nil, fmt.Errorf("no music items in response")
	}
	return FlattenShelves(shelves), nil
}

// decodeJSON decodes a response body into a generic document.
func decodeJSON(raw []byte) (map[string]any, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse music response: %w", err)
	}
	return doc, nil
}

// musicItem flattens one music renderer into a result.
func musicItem(node any) (SearchResult, bool) {
	m, ok := node.(map[string]any)
	if !ok {
		return SearchResult{}, false
	}
	var r SearchResult
	var ok2 bool
	if tr, ok := m["musicTwoRowItemRenderer"].(map[string]any); ok {
		r, ok2 = musicTwoRow(tr)
	} else if rr, ok := m["musicResponsiveListItemRenderer"].(map[string]any); ok {
		r, ok2 = musicResponsive(rr)
	}
	if !ok2 || r.ID == "" {
		// Buttons like "Shuffle all" carry no video or playlist.
		return SearchResult{}, false
	}
	return r, true
}

// musicTwoRow parses the tile renderer used across the music home carousels.
func musicTwoRow(tr map[string]any) (SearchResult, bool) {
	title := runsText(tr["title"])
	if title == "" {
		return SearchResult{}, false
	}
	uploader := musicArtist(runsText(tr["subtitle"]))

	var videoID, playlistID string
	if nav, ok := tr["navigationEndpoint"].(map[string]any); ok {
		if we, ok := nav["watchEndpoint"].(map[string]any); ok {
			videoID, _ = we["videoId"].(string)
			playlistID, _ = we["playlistId"].(string)
		}
		if be, ok := nav["browseEndpoint"].(map[string]any); ok {
			if id, _ := be["browseId"].(string); strings.HasPrefix(id, "VL") {
				playlistID = strings.TrimPrefix(id, "VL")
			}
		}
	}

	r := musicResult(title, uploader, videoID, playlistID)
	r.ThumbURL = musicThumb(tr["thumbnailRenderer"])
	return r, true
}

// musicResponsive parses the list renderer used by playlist and liked-songs
// views.
func musicResponsive(rr map[string]any) (SearchResult, bool) {
	cols, _ := rr["flexColumns"].([]any)
	title := ""
	if len(cols) > 0 {
		title = flexColumnText(cols[0])
	}
	if title == "" {
		return SearchResult{}, false
	}
	uploader := ""
	if len(cols) > 1 {
		uploader = musicArtist(flexColumnText(cols[1]))
	}

	var videoID string
	if pid, ok := rr["playlistItemData"].(map[string]any); ok {
		videoID, _ = pid["videoId"].(string)
	}
	if videoID == "" {
		if nav, ok := rr["navigationEndpoint"].(map[string]any); ok {
			if we, ok := nav["watchEndpoint"].(map[string]any); ok {
				videoID, _ = we["videoId"].(string)
			}
		}
	}

	duration := 0
	if fixed, ok := rr["fixedColumns"].([]any); ok && len(fixed) > 0 {
		if fc, ok := fixed[0].(map[string]any); ok {
			if fr, ok := fc["musicResponsiveListItemFixedColumnRenderer"].(map[string]any); ok {
				duration = parseClock(runsText(fr["text"]))
			}
		}
	}

	r := musicResult(title, uploader, videoID, "")
	r.Duration = duration
	r.ThumbURL = musicThumb(rr["thumbnail"])
	return r, true
}

// musicResult fills the shared result shape for a music entry. Playlists,
// mixes and albums keep their playlist id so the UI can expand them; songs
// use their video id directly.
func musicResult(title, uploader, videoID, playlistID string) SearchResult {
	if videoID != "" && playlistID != "" {
		// A mix: keep the playlist so entering it queues the whole thing.
		return SearchResult{
			ID:         videoID,
			Title:      title,
			Uploader:   uploader,
			PlaylistID: playlistID,
		}
	}
	if videoID != "" {
		return SearchResult{ID: videoID, Title: title, Uploader: uploader}
	}
	return SearchResult{
		ID:         playlistID,
		Title:      title,
		Uploader:   uploader,
		PlaylistID: playlistID,
	}
}

// musicThumb extracts the largest thumbnail URL from a music thumbnail
// renderer ("thumbnailRenderer" for tiles, "thumbnail" for list rows).
func musicThumb(node any) string {
	m, ok := node.(map[string]any)
	if !ok {
		return ""
	}
	if tr, ok := m["musicThumbnailRenderer"].(map[string]any); ok {
		m = tr
	}
	thumb, ok := m["thumbnail"].(map[string]any)
	if !ok {
		return ""
	}
	thumbs, _ := thumb["thumbnails"].([]any)
	if len(thumbs) == 0 {
		return ""
	}
	// The last entry is the highest resolution.
	if last, ok := thumbs[len(thumbs)-1].(map[string]any); ok {
		url, _ := last["url"].(string)
		return url
	}
	return ""
}

// runsText joins the runs of a runs[] text node.
func runsText(node any) string {
	m, ok := node.(map[string]any)
	if !ok {
		return ""
	}
	if s, ok := m["simpleText"].(string); ok {
		return strings.TrimSpace(s)
	}
	runs, _ := m["runs"].([]any)
	var parts []string
	for _, r := range runs {
		if rm, ok := r.(map[string]any); ok {
			if t, ok := rm["text"].(string); ok {
				parts = append(parts, t)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

// flexColumnText extracts the text of a musicResponsiveListItemFlexColumn.
func flexColumnText(node any) string {
	m, ok := node.(map[string]any)
	if !ok {
		return ""
	}
	fc, ok := m["musicResponsiveListItemFlexColumnRenderer"].(map[string]any)
	if !ok {
		return ""
	}
	return runsText(fc["text"])
}

// musicArtist cleans a music subtitle ("Song • Artist", "Artist • 53M plays")
// down to the artist names.
func musicArtist(sub string) string {
	sub = strings.TrimSpace(sub)
	for _, prefix := range []string{"Song • ", "Video • ", "Album • ", "Single • ", "EP • ", "Playlist • "} {
		sub = strings.TrimPrefix(sub, prefix)
	}
	if i := strings.Index(sub, " • "); i > 0 {
		// Keep the artist part, drop trailing stats like play counts.
		sub = sub[:i]
	}
	return strings.TrimSpace(sub)
}

// dig walks a decoded JSON document along a path of map keys and array
// indexes ("0" for the first element), returning nil when the path is absent.
func dig(root any, path ...string) any {
	cur := root
	for _, key := range path {
		switch n := cur.(type) {
		case map[string]any:
			cur = n[key]
		case []any:
			idx := 0
			if _, err := fmt.Sscanf(key, "%d", &idx); err != nil || idx < 0 || idx >= len(n) {
				return nil
			}
			cur = n[idx]
		default:
			return nil
		}
		if cur == nil {
			return nil
		}
	}
	return cur
}
