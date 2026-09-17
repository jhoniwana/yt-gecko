package core

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// SearchResult is a single YouTube search hit. PlaylistID is set for YouTube
// Music playlists, mixes and albums: entering them expands the playlist
// instead of playing a single video. ThumbURL carries an explicit thumbnail
// (music playlists are not addressable by the i.ytimg.com/<videoID> pattern).
type SearchResult struct {
	ID         string
	Title      string
	URL        string
	Duration   int
	Uploader   string
	PlaylistID string
	ThumbURL   string
}

// DurationString formats the duration as m:ss.
func (r SearchResult) DurationString() string {
	return fmt.Sprintf("%d:%02d", r.Duration/60, r.Duration%60)
}

// DurationLabel is the duration for list rows: empty when unknown, so music
// entries without a duration do not show a misleading 0:00.
func (r SearchResult) DurationLabel() string {
	if r.Duration <= 0 {
		return ""
	}
	return r.DurationString()
}

// Search queries YouTube via yt-dlp and returns the top hits.
func (g *GeckoCore) Search(query string, maxResults int) ([]SearchResult, error) {
	return g.listVideos(fmt.Sprintf("ytsearch%d:%s", maxResults, query))
}

// Feed lists recent videos for a topic from YouTube's hashtag feed. It is
// the closest reliable source for a content home without signed-in API
// access, since the homepage HTML no longer embeds video data. yt-dlp returns
// the whole hashtag page, so the list is capped to maxResults.
func (g *GeckoCore) Feed(tag string, maxResults int) ([]SearchResult, error) {
	results, err := g.listVideos(fmt.Sprintf("https://www.youtube.com/hashtag/%s", tag))
	if err != nil {
		return nil, err
	}
	if maxResults > 0 && len(results) > maxResults {
		results = results[:maxResults]
	}
	return results, nil
}

func (g *GeckoCore) listVideos(source string) ([]SearchResult, error) {
	args := []string{
		"--flat-playlist",
		"--print", "%(id)s||%(title)s||%(duration)s||%(uploader)s",
	}
	args = append(args, g.jsRuntimeArgs()...)
	args = append(args, g.authArgs()...)
	args = append(args, source)

	var stderr bytes.Buffer
	cmd := exec.Command(g.ytdlp(), args...)
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp search failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	results, err := parseResults(out)
	if err != nil {
		return nil, fmt.Errorf("parse yt-dlp output: %w", err)
	}
	return results, nil
}

func parseResults(out []byte) ([]SearchResult, error) {
	var results []SearchResult
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "||")
		if len(parts) < 4 {
			continue
		}
		duration, err := strconv.Atoi(parts[2])
		if err != nil {
			duration = 0
		}
		results = append(results, SearchResult{
			ID:       parts[0],
			Title:    sanitizeTitle(parts[1]),
			URL:      "https://www.youtube.com/watch?v=" + parts[0],
			Duration: duration,
			Uploader: parts[3],
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// sanitizeTitle strips emoji and other decorative unicode so terminal output
// stays clean. Keeps accented letters and ordinary punctuation.
func sanitizeTitle(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 0x1F000 && r <= 0x1FFFF: // emoji
			continue
		case r >= 0x2600 && r <= 0x27BF: // misc symbols / dingbats
			continue
		case r == 0x200D || r == 0xFE0F: // zero-width joiner / variation selector
			continue
		case r == '\t' || r == '\r' || r == '\n':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
