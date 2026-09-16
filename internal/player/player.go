package player

import "github.com/jhoniwana/yt-gecko/internal/core"

// Playback wraps an active mpv client with conveniences for the TUI.
type Playback struct {
	client *core.Client
}

// New wraps the given mpv client.
func New(client *core.Client) *Playback {
	return &Playback{client: client}
}

// Position returns the current playback position in seconds.
func (p *Playback) Position() (float64, error) { return p.client.GetPosition() }

// Duration returns the total media duration in seconds.
func (p *Playback) Duration() (float64, error) { return p.client.GetDuration() }

// TogglePause cycles play/pause.
func (p *Playback) TogglePause() error { return p.client.TogglePause() }

// Seek jumps the playback position.
func (p *Playback) Seek(seconds float64) error { return p.client.Seek(seconds) }

// SetVolume adjusts the volume in percent.
func (p *Playback) SetVolume(percent int) error { return p.client.SetVolume(percent) }
