package core

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	socketReadyTimeout = 2 * time.Second
	socketRetryDelay   = 100 * time.Millisecond
	loadGracePeriod    = 5 * time.Second
	maxPlayRetries     = 3
)

// Play starts mpv on url and returns a client for IPC control.
// YouTube URLs are resolved to direct stream URLs with yt-dlp so mpv never
// has to extract video URLs itself. Because YouTube's edge servers reject
// some stream URLs, failed loads are retried with a fresh stream URL.
func (g *GeckoCore) Play(url string, audioOnly bool) (*Client, error) {
	if err := g.Stop(); err != nil {
		return nil, err
	}
	if audioOnly || g.Quality() == "audio" {
		audioOnly = true
	}

	for attempt := 1; attempt <= maxPlayRetries; attempt++ {
		target := url
		var audioURL string
		if isYouTubeURL(url) {
			streams, err := g.ResolveStream(url, audioOnly)
			if err != nil {
				return nil, err
			}
			target = streams[0]
			if len(streams) > 1 {
				audioURL = streams[1]
			}
		}

		if err := g.start(target, audioURL, audioOnly); err != nil {
			return nil, err
		}
		if !g.exitedWithin(loadGracePeriod) {
			return &Client{socketPath: g.socketPath}, nil
		}
		_ = g.Stop()
	}
	return nil, fmt.Errorf("mpv failed to load the stream after %d attempts", maxPlayRetries)
}

// ResolveStream extracts direct stream URLs for a video via yt-dlp, honoring
// the selected quality. The result is one URL for progressive formats, or two
// (video, audio) for DASH streams where the selected height needs separate
// tracks. HLS manifests are preferred over direct http URLs because YouTube
// rejects many direct requests (403) while HLS playback stays reliable.
func (g *GeckoCore) ResolveStream(url string, audioOnly bool) ([]string, error) {
	formats := qualityFormats(g.Quality(), audioOnly)

	var lastErr error
	for _, format := range formats {
		args := []string{"--skip-download", "--get-url"}
		args = append(args, g.jsRuntimeArgs()...)
		if format != "" {
			args = append(args, "-f", format)
		}
		args = append(args, g.authArgs()...)
		args = append(args, url)

		var stderr bytes.Buffer
		cmd := exec.Command(g.ytdlp(), args...)
		cmd.Stderr = &stderr

		out, err := cmd.Output()
		if err != nil {
			lastErr = fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
			continue
		}
		var streams []string
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				streams = append(streams, line)
			}
		}
		if len(streams) > 0 {
			if len(streams) > 2 {
				streams = streams[:2]
			}
			return streams, nil
		}
		lastErr = fmt.Errorf("yt-dlp returned no stream url")
	}
	return nil, lastErr
}

// qualityFormats maps a quality selection to yt-dlp format selectors, tried in
// order. Progressive ("b") formats carry video and audio in one stream and are
// preferred; DASH candidates (bv*+ba) cover heights that only exist as
// separate tracks (1080p and up) and resolve to two URLs.
func qualityFormats(quality string, audioOnly bool) []string {
	if audioOnly {
		return []string{"bestaudio", "b", ""}
	}
	switch quality {
	case "1080p", "720p", "480p", "360p", "240p":
		h := strings.TrimSuffix(quality, "p")
		return []string{
			"b[height<=" + h + "]/b",
			"bv*[height<=" + h + "]+ba/b[height<=" + h + "]/b",
			"b",
		}
	}
	return []string{"b", "best", ""}
}

func isYouTubeURL(url string) bool {
	return strings.Contains(url, "youtube.com/watch") || strings.Contains(url, "youtu.be/")
}

func (g *GeckoCore) start(target, audioURL string, audioOnly bool) error {
	_ = os.Remove(g.socketPath)

	args := mpvArgs(g.socketPath, audioOnly)
	if audioURL != "" {
		args = append(args, "--audio-file="+audioURL)
	}
	args = append(args, target)

	cmd := g.mpvCommand(args)
	if g.mpvEnv != nil {
		cmd.Env = g.mpvEnv
	}
	g.mpvErr.Reset()
	cmd.Stderr = &g.mpvErr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start mpv: %w", err)
	}
	g.mpv = cmd
	g.mpvDone = make(chan error, 1)
	go func(done chan error) { done <- cmd.Wait() }(g.mpvDone)

	if err := waitForSocket(g.socketPath); err != nil {
		_ = g.Stop()
		return err
	}
	return nil
}

func (g *GeckoCore) exitedWithin(grace time.Duration) bool {
	if g.mpvDone == nil {
		return true
	}
	select {
	case <-g.mpvDone:
		return true
	case <-time.After(grace):
		return false
	}
}

// Stop terminates the running mpv process, if any.
func (g *GeckoCore) Stop() error {
	if g.mpv == nil || g.mpv.Process == nil {
		return nil
	}
	if err := g.mpv.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("kill mpv: %w", err)
	}
	if g.mpvDone != nil {
		select {
		case <-g.mpvDone:
		case <-time.After(time.Second):
		}
	}
	g.mpv = nil
	g.mpvDone = nil
	return nil
}

func mpvArgs(socketPath string, audioOnly bool) []string {
	args := []string{
		"--no-terminal",
		"--input-ipc-server=" + socketPath,
		"--cache=no",
		"--demuxer-max-bytes=50MiB",
		"--demuxer-max-back-bytes=10MiB",
		"--save-position-on-quit=no",
		"--network-timeout=10",
		"--stream-lavf-o=reconnect=1",
		"--stream-lavf-o=reconnect_streamed=1",
		"--stream-lavf-o=reconnect_on_network_error=1",
		"--stream-lavf-o=reconnect_delay_max=5",
		"--demuxer-lavf-o=initial_request_size=524288",
	}
	if audioOnly {
		// keep-open keeps mpv alive at the end of a track so the player UI
		// can advance to the next queue item over IPC.
		args = append(args, "--no-video", "--keep-open=yes")
	}
	return args
}

func waitForSocket(path string) error {
	deadline := time.Now().Add(socketReadyTimeout)
	for time.Now().Before(deadline) {
		if fi, err := os.Stat(path); err == nil && fi.Mode()&os.ModeSocket != 0 {
			return nil
		}
		time.Sleep(socketRetryDelay)
	}
	return fmt.Errorf("mpv IPC socket not ready at %s", path)
}
