package core

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jhoniwana/yt-gecko/internal/auth"
)

const (
	innerTubeBrowsePath  = "/youtubei/v1/browse"
	innerTubeNextPath    = "/youtubei/v1/next"
	innerTubeHomeURL     = "https://www.youtube.com/"
	innerTubeFallbackKey = "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8"
	innerTubeTimeout     = 20 * time.Second
)

var (
	innerTubeAPIKeyRe = regexp.MustCompile(`"INNERTUBE_API_KEY":"([^"]+)"`)
	visitorDataRe     = regexp.MustCompile(`"VISITOR_DATA":"([^"]+)"`)
)

// visitorData scrapes the visitor id the web client sends with every request;
// responses can differ without it.
func visitorData(cookies, pageURL string) string {
	req, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Cookie", cookies)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) Gecko/20100101 Firefox/130.0")
	client := &http.Client{Timeout: innerTubeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return ""
	}
	if m := visitorDataRe.FindSubmatch(body); len(m) == 2 {
		return string(m[1])
	}
	return ""
}

// SubscriptionsFeed lists recent videos from the signed-in account's
// subscriptions shelf.
func (g *GeckoCore) SubscriptionsFeed(maxResults int) ([]SearchResult, error) {
	return g.browseFeed("FEsubscriptions", maxResults)
}

// HomeFeed returns the signed-in account's personalized "For you" shelf
// (YouTube's home recommendations), which are only served by innerTube to an
// authenticated session. When not signed in, fall back to a hashtag shelf.
func (g *GeckoCore) HomeFeed(maxResults int) ([]SearchResult, error) {
	return g.browseFeed("FEwhat_to_watch", maxResults)
}

// browseFeed fetches a signed-in innerTube browse shelf (FEsubscriptions for
// the subscriptions tab, FEwhat_to_watch for the personalized home). YouTube
// exposes these only through the innerTube API with an authenticated session
// (the plain webpage and yt-dlp's tab extractor both return an empty shell):
// the cookies are loaded from the chosen browser via yt-dlp, the WEB client
// is authenticated with the SAPISIDHASH token, and the first page of
// lockupViewModels is flattened into search results.
func (g *GeckoCore) browseFeed(browseID string, maxResults int) ([]SearchResult, error) {
	s, err := g.newInnerTubeSession()
	if err != nil {
		return nil, err
	}

	body := map[string]any{
		"context":  webContext(),
		"browseId": browseID,
	}
	raw, err := s.post(innerTubeBrowsePath, "https://www.youtube.com/feed/subscriptions", body)
	if err != nil {
		return nil, err
	}
	return parseLockupFeed(raw, maxResults)
}

// RelatedVideos returns the "up next" recommendations YouTube shows on the
// watch page for a video. The first next call exposes the related chip cloud;
// its continuation token yields the lockupViewModel shelf of suggested videos.
func (g *GeckoCore) RelatedVideos(videoID string, maxResults int) ([]SearchResult, error) {
	s, err := g.newInnerTubeSession()
	if err != nil {
		return nil, err
	}

	referer := "https://www.youtube.com/watch?v=" + videoID
	raw, err := s.post(innerTubeNextPath, referer, map[string]any{
		"context": webContext(),
		"videoId": videoID,
	})
	if err != nil {
		return nil, err
	}

	var cont string
	if node, ok := findNode(raw, "relatedChipCloudRenderer"); ok {
		cont = findToken(node)
	}
	if cont == "" {
		// Older/guest layouts may inline the suggestions instead.
		return parseLockupFeed(raw, maxResults)
	}

	raw, err = s.post(innerTubeNextPath, referer, map[string]any{
		"context":      webContext(),
		"continuation": cont,
	})
	if err != nil {
		return nil, err
	}
	return parseRelatedItems(raw, maxResults)
}

// parseRelatedItems flattens the related-videos continuation response: the
// suggestions live in reloadContinuationItemsCommand.continuationItems[], each
// a lockupViewModel of the same shape as the home/feed shelves.
func parseRelatedItems(raw []byte, maxResults int) ([]SearchResult, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse related videos: %w", err)
	}

	items, ok := findNode(doc, "continuationItems")
	if !ok {
		return nil, fmt.Errorf("no related videos in response")
	}

	var list []any
	if arr, ok := items.([]any); ok {
		list = arr
	}
	var results []SearchResult
	for _, it := range list {
		if len(results) >= maxResults {
			break
		}
		node, ok := it.(map[string]any)
		if !ok {
			continue
		}
		lm, ok := node["lockupViewModel"].(map[string]any)
		if !ok {
			continue
		}
		if r, ok := lockupResult(lm); ok {
			results = append(results, finishResult(r))
		}
	}
	return results, nil
}

// innerTubeSession carries the ingredients of one authenticated innerTube
// request: the browser cookie header, the signing SAPISID and the web API keys
// (the main site and YouTube Music each expose their own).
type innerTubeSession struct {
	cookies     string
	sapisid     string
	key         string
	musicKey    string
	visitorData string
}

// newInnerTubeSession loads the selected browser's cookies and the current
// innerTube web API key. Both are dumped fresh on every call because browser
// sessions rotate cookies frequently (a jar older than ~30 minutes silently
// returns a logged-out shell with no subscriptions).
func (g *GeckoCore) newInnerTubeSession() (*innerTubeSession, error) {
	b := g.Browser()
	if b == "" {
		return nil, fmt.Errorf("not signed in")
	}

	jar, err := g.dumpCookieJar(b)
	if err != nil {
		return nil, err
	}
	defer os.Remove(jar)

	cookies, sapisid, err := parseNetscapeJar(jar)
	if err != nil {
		return nil, err
	}
	if sapisid == "" {
		return nil, fmt.Errorf("no YouTube session cookies found in %s; sign in to YouTube in the browser first", b.Label())
	}

	key, err := innerTubeKey(cookies, innerTubeFallbackKey)
	if err != nil {
		return nil, err
	}
	musicKey, err := innerTubeKeyFor(cookies, musicHomeURL, musicFallbackKey)
	if err != nil {
		return nil, err
	}
	return &innerTubeSession{
		cookies:     cookies,
		sapisid:     sapisid,
		key:         key,
		musicKey:    musicKey,
		visitorData: visitorData(cookies, musicHomeURL),
	}, nil
}

// post sends one authenticated innerTube request to the main YouTube site.
func (s *innerTubeSession) post(path, referer string, body map[string]any) ([]byte, error) {
	return s.postTo("https://www.youtube.com", path, referer, body)
}

// postTo sends one authenticated innerTube request to the given origin, which
// may be the main site or YouTube Music (WEB_REMIX), and returns the body.
func (s *innerTubeSession) postTo(origin, path, referer string, body map[string]any) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	key := s.key
	clientName := "1"
	clientVersion := "2.20260913.00.00"
	if origin == musicOrigin && s.musicKey != "" {
		key = s.musicKey
		clientName = "67"
		clientVersion = "1.20260913.16.00"
	}
	req, err := http.NewRequest(http.MethodPost, origin+path+"?key="+key, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", s.cookies)
	req.Header.Set("Authorization", sapisidHashFor(s.sapisid, origin))
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) Gecko/20100101 Firefox/130.0")
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", referer)
	req.Header.Set("X-YouTube-Client-Name", clientName)
	req.Header.Set("X-YouTube-Client-Version", clientVersion)
	req.Header.Set("X-Goog-Api-Format-Version", "1")
	if s.visitorData != "" {
		req.Header.Set("X-Goog-Visitor-Id", s.visitorData)
	}

	client := &http.Client{Timeout: innerTubeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("youtube request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("youtube request: status %d", resp.StatusCode)
	}
	return raw, nil
}

// webContext is the innerTube client context used by the WEB client requests.
func webContext() map[string]any {
	return map[string]any{
		"client": map[string]any{
			"clientName":    "WEB",
			"clientVersion": "2.20250501.01.00",
			"hl":            "en",
			"gl":            "US",
		},
	}
}

// findNode returns the first value for a key found anywhere in a decoded JSON
// document.
func findNode(root any, key string) (any, bool) {
	switch n := root.(type) {
	case map[string]any:
		if v, ok := n[key]; ok {
			return v, true
		}
		for _, v := range n {
			if r, ok := findNode(v, key); ok {
				return r, true
			}
		}
	case []any:
		for _, v := range n {
			if r, ok := findNode(v, key); ok {
				return r, true
			}
		}
	}
	return nil, false
}

// findToken returns the first continuation token (a "token" string) found
// under a node, used for the related chip cloud's continuation.
func findToken(node any) string {
	switch n := node.(type) {
	case map[string]any:
		if v, ok := n["token"].(string); ok && v != "" {
			return v
		}
		for _, v := range n {
			if t := findToken(v); t != "" {
				return t
			}
		}
	case []any:
		for _, v := range n {
			if t := findToken(v); t != "" {
				return t
			}
		}
	}
	return ""
}

// dumpCookieJar asks yt-dlp to load the browser's cookies and write them as a
// Netscape jar in a temporary file. The file is transient (the caller removes
// it immediately after parsing) and never stored or printed. The jar path must
// not exist beforehand: yt-dlp refuses an empty pre-created file.
func (g *GeckoCore) dumpCookieJar(b auth.Browser) (string, error) {
	f, err := os.CreateTemp("", "yt-gecko-cookies-*.txt")
	if err != nil {
		return "", err
	}
	jar := f.Name()
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(jar); err != nil {
		return "", err
	}

	args := append(auth.CookiesFromBrowserArgs(b),
		"--cookies", jar,
	)
	args = append(args, g.jsRuntimeArgs()...)
	args = append(args,
		"--simulate", "--skip-download", "--",
		"--simulate", "--skip-download", "--",
		"https://www.youtube.com/watch?v=jNQXAC9IVRw")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, g.ytdlp(), args...)
	cmd.Stderr = &stderr
	err = cmd.Run()
	// The probe video can fail (bot checks, canaries) even when the cookie
	// extraction worked, so the jar contents decide, not the exit status.
	if fi, statErr := os.Stat(jar); statErr != nil || fi.Size() == 0 {
		_ = os.Remove(jar)
		if err == nil {
			err = fmt.Errorf("yt-dlp wrote no cookie jar")
		}
		return "", fmt.Errorf("read %s cookies: %w: %s", b.Label(), err, strings.TrimSpace(stderr.String()))
	}
	return jar, nil
}

// parseNetscapeJar reads a yt-dlp Netscape jar and returns the Cookie header
// for YouTube plus the SAPISID value used to build the auth token.
func parseNetscapeJar(path string) (cookies string, sapisid string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var pairs []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		domain, name, value := fields[0], fields[5], fields[6]
		if !strings.HasSuffix(domain, "youtube.com") {
			continue
		}
		if name == "SAPISID" {
			sapisid = value
		}
		pairs = append(pairs, name+"="+value)
	}
	return strings.Join(pairs, "; "), sapisid, nil
}

// sapisidHash builds the Authorization token for innerTube: a SHA1 of
// "<millis> <sapisid> <origin>" signed by browsers for each request. The
// origin must match the request's own origin, so YouTube Music requests are
// signed with https://music.youtube.com or the session reads as logged out.
func sapisidHash(sapisid string) string {
	return sapisidHashFor(sapisid, "https://www.youtube.com")
}

// sapisidHashFor signs for a specific origin.
func sapisidHashFor(sapisid, origin string) string {
	ms := time.Now().UnixMilli()
	sum := sha1.Sum([]byte(fmt.Sprintf("%d %s %s", ms, sapisid, origin)))
	return fmt.Sprintf("SAPISIDHASH %d_%s", ms, hex.EncodeToString(sum[:]))
}

// innerTubeKey finds the current innerTube web API key from the homepage so
// the request matches what the browser would send; falls back to known key.
func innerTubeKey(cookies string, fallback string) (string, error) {
	return innerTubeKeyFor(cookies, innerTubeHomeURL, fallback)
}

// innerTubeKeyFor scrapes the API key of a specific YouTube origin, since the
// main site and YouTube Music publish different keys.
func innerTubeKeyFor(cookies, pageURL, fallback string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Cookie", cookies)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) Gecko/20100101 Firefox/130.0")
	client := &http.Client{Timeout: innerTubeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fallback, nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fallback, nil
	}
	if m := innerTubeAPIKeyRe.FindSubmatch(body); len(m) == 2 {
		return string(m[1]), nil
	}
	return fallback, nil
}

// parseLockupFeed walks the innerTube response collecting lockupViewModels,
// which carry the modern subscriptions shelf items.
func parseLockupFeed(raw []byte, maxResults int) ([]SearchResult, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse youtube feed: %w", err)
	}

	var items []SearchResult
	var walk func(any)
	walk = func(node any) {
		if len(items) >= maxResults {
			return
		}
		switch n := node.(type) {
		case map[string]any:
			if lm, ok := n["lockupViewModel"].(map[string]any); ok {
				if r, ok := lockupResult(lm); ok {
					items = append(items, r)
					return
				}
			}
			for _, v := range n {
				walk(v)
			}
		case []any:
			for _, v := range n {
				walk(v)
			}
		}
	}
	walk(doc)

	for i := range items {
		items[i] = finishResult(items[i])
	}
	return items, nil
}

// finishResult converts a raw item into the shared result shape, sanitizing
// the title and filling the watch URL.
func finishResult(r SearchResult) SearchResult {
	return SearchResult{
		ID:         r.ID,
		Title:      sanitizeTitle(r.Title),
		URL:        "https://www.youtube.com/watch?v=" + r.ID,
		Duration:   r.Duration,
		Uploader:   r.Uploader,
		PlaylistID: r.PlaylistID,
		ThumbURL:   r.ThumbURL,
	}
}

// lockupResult flattens a YouTube lockupViewModel into a search result.
// Entries without a real title (ads, promo cards) are skipped.
func lockupResult(vm map[string]any) (SearchResult, bool) {
	id, _ := vm["contentId"].(string)
	if id == "" {
		return SearchResult{}, false
	}

	var title, author, duration string
	if md, ok := vm["metadata"].(map[string]any); ok {
		if lm, ok := md["lockupMetadataViewModel"].(map[string]any); ok {
			if t, ok := lm["title"].(map[string]any); ok {
				title, _ = t["content"].(string)
			}
			if img, ok := lm["image"].(map[string]any); ok {
				if dvm, ok := img["decoratedAvatarViewModel"].(map[string]any); ok {
					if a11y, ok := dvm["a11yLabel"].(string); ok {
						author = strings.TrimSpace(strings.TrimPrefix(a11y, "Go to channel "))
					}
				}
			}
		}
	}
	if title == "" {
		return SearchResult{}, false
	}

	if img, ok := vm["contentImage"].(map[string]any); ok {
		if tv, ok := img["thumbnailViewModel"].(map[string]any); ok {
			if overlays, ok := tv["overlays"].([]any); ok && len(overlays) > 0 {
				if top, ok := overlays[0].(map[string]any); ok {
					if bottom, ok := top["thumbnailBottomOverlayViewModel"].(map[string]any); ok {
						if badges, ok := bottom["badges"].([]any); ok && len(badges) > 0 {
							if badge, ok := badges[0].(map[string]any); ok {
								if tb, ok := badge["thumbnailBadgeViewModel"].(map[string]any); ok {
									duration, _ = tb["text"].(string)
								}
							}
						}
					}
				}
			}
		}
	}

	return SearchResult{
		ID:       id,
		Title:    title,
		Uploader: author,
		Duration: parseClock(duration),
	}, true
}

// parseClock parses "M:SS", "MM:SS" or "H:MM:SS" into seconds.
func parseClock(s string) int {
	parts := strings.Split(s, ":")
	if len(parts) == 0 {
		return 0
	}
	total := 0
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0
		}
		total = total*60 + n
	}
	return total
}
