package core

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

const ipcTimeout = time.Second

// Client controls a live mpv instance over its IPC unix socket.
type Client struct {
	socketPath string
}

type mpvRequest struct {
	Command   []any `json:"command"`
	RequestID int64 `json:"request_id"`
}

type mpvResponse struct {
	Data      any    `json:"data"`
	Error     string `json:"error"`
	RequestID int64  `json:"request_id"`
}

func (c *Client) send(command []any) (*mpvResponse, error) {
	conn, err := net.DialTimeout("unix", c.socketPath, ipcTimeout)
	if err != nil {
		return nil, fmt.Errorf("connect to mpv: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(ipcTimeout)); err != nil {
		return nil, fmt.Errorf("set deadline: %w", err)
	}

	req := mpvRequest{Command: command, RequestID: time.Now().UnixMilli()}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, fmt.Errorf("send command: %w", err)
	}

	var resp mpvResponse
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&resp); err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.Error != "" && resp.Error != "success" {
		return nil, fmt.Errorf("mpv error: %s", resp.Error)
	}
	return &resp, nil
}

func (c *Client) property(name string) (any, error) {
	resp, err := c.send([]any{"get_property", name})
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetPosition returns the current playback position in seconds.
func (c *Client) GetPosition() (float64, error) {
	data, err := c.property("time-pos")
	if err != nil {
		return 0, err
	}
	pos, _ := data.(float64)
	return pos, nil
}

// GetDuration returns the media duration in seconds.
func (c *Client) GetDuration() (float64, error) {
	data, err := c.property("duration")
	if err != nil {
		return 0, err
	}
	dur, _ := data.(float64)
	return dur, nil
}

// TogglePause cycles play/pause.
func (c *Client) TogglePause() error {
	_, err := c.send([]any{"cycle", "pause"})
	return err
}

// Seek jumps the playback position.
func (c *Client) Seek(seconds float64) error {
	_, err := c.send([]any{"set_property", "time-pos", seconds})
	return err
}

// EOFReached reports whether the current file played to its end. Used by the
// music player to advance the queue.
func (c *Client) EOFReached() bool {
	data, err := c.property("eof-reached")
	if err != nil {
		return false
	}
	eof, _ := data.(bool)
	return eof
}

// SetVolume sets the volume in percent.
func (c *Client) SetVolume(percent int) error {
	_, err := c.send([]any{"set_property", "volume", percent})
	return err
}
