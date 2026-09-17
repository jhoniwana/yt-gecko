package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/jhoniwana/yt-gecko/internal/auth"
	"github.com/jhoniwana/yt-gecko/internal/core"
	"github.com/jhoniwana/yt-gecko/internal/tui"
	"github.com/spf13/cobra"
)

// doctorCmd prints a support report: tool versions, audio/video capabilities
// and a decoding self-test. It is what to ask for when playback misbehaves on
// someone else's machine.
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check playback tools and print a support report",
	RunE: func(c *cobra.Command, args []string) error {
		g := gecko()
		defer g.Cleanup()
		report(g)
		return nil
	},
}

func report(g *core.GeckoCore) {
	fmt.Println("yt-gecko doctor")
	fmt.Println("===============")
	fmt.Printf("os/arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("config:     %s\n", auth.ConfigPath())
	fmt.Printf("cache:      %s\n", cacheHint())
	ytdlp, mpv := toolPath("yt-dlp"), toolPath("mpv")
	if p := g.YTDLPPath(); p != "" {
		ytdlp = p + " (bundled)"
	}
	if p := g.MPVPath(); p != "" {
		mpv = p + " (bundled)"
	}
	fmt.Printf("tools:      yt-dlp=%s\n            mpv=%s\n", ytdlp, mpv)

	if term := os.Getenv("TERM"); term != "" {
		fmt.Printf("terminal:   TERM=%s\n", term)
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		fmt.Printf("display:    wayland (%s)\n", os.Getenv("WAYLAND_DISPLAY"))
	} else if os.Getenv("DISPLAY") != "" {
		fmt.Printf("display:    x11 (%s)\n", os.Getenv("DISPLAY"))
	} else {
		fmt.Println("display:    none (video playback needs a graphical session)")
	}
	if tui.ProbeGraphics(os.Stdin, os.Stdout) {
		fmt.Println("graphics:   kitty graphics protocol supported (real thumbnails)")
	} else {
		fmt.Println("graphics:   not supported here (block-art thumbnails)")
	}

	fmt.Println()
	fmt.Println("audio outputs mpv sees:")
	for _, line := range mpvLines(g, "--terminal=yes", "--audio-device=help") {
		fmt.Println("  " + line)
	}

	fmt.Println()
	fmt.Println("decoding self-test (silent 440 Hz tone):")
	if out, err := runMPVErr(g, "--no-video", "--ao=null", "--no-terminal",
		"--demuxer-lavf-o=analyzeduration=1000000",
		"av://lavfi:sine=frequency=440:duration=1"); err == nil {
		fmt.Println("  ok: mpv decoded and played audio")
	} else {
		fmt.Println("  FAILED: " + cleanNoise(out))
	}

	fmt.Println()
	fmt.Println("real audio output test (0.5 s tone on the default device):")
	if out, err := runMPVErr(g, "--no-video", "--no-terminal",
		"--demuxer-lavf-o=analyzeduration=1000000",
		"av://lavfi:sine=frequency=440:duration=0.5"); err == nil {
		fmt.Println("  ok: the default audio output works")
	} else {
		fmt.Println("  FAILED: " + cleanNoise(out))
		fmt.Println("  (no sound server? mpv can still decode, but nothing is audible)")
	}

	fmt.Println()
	fmt.Println("stream resolve self-test:")
	if url, err := g.ResolveStream("https://www.youtube.com/watch?v=jNQXAC9IVRw", true); err != nil {
		fmt.Println("  FAILED: " + firstLine(err.Error()))
	} else if len(url) == 0 {
		fmt.Println("  FAILED: yt-dlp returned no stream")
	} else {
		fmt.Println("  ok: yt-dlp resolved a stream URL")
	}
}

// mpvLines runs mpv and returns its output lines, without the cosmetic
// warnings (fonts, sound servers) that are meaningless in this report.
func mpvLines(g *core.GeckoCore, args ...string) []string {
	out, _ := runMPVErr(g, args...)
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || noiseLine(line) {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return []string{"(no output)"}
	}
	return lines
}

// cleanNoise removes cosmetic warnings from a block of output.
func cleanNoise(out string) string {
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" || noiseLine(line) {
			continue
		}
		lines = append(lines, strings.TrimSpace(line))
	}
	if len(lines) == 0 {
		return "(no detail)"
	}
	return strings.Join(lines, "; ")
}

// noiseLine reports whether a line is a cosmetic warning unrelated to
// playback capability.
func noiseLine(line string) bool {
	for _, frag := range []string{"Fontconfig error", "ALSA lib", "pw.loop", "Cannot access file /usr/share/alsa"} {
		if strings.Contains(line, frag) {
			return true
		}
	}
	return false
}

// runMPVErr runs mpv with the given args and returns its combined output and
// exit error. Cosmetic warnings do not make the run fail.
func runMPVErr(g *core.GeckoCore, args ...string) (string, error) {
	cmd := g.MPVCommand(append([]string{"--msg-level=all=warn"}, args...)...)
	done := make(chan struct {
		out []byte
		err error
	}, 1)
	go func() {
		out, err := cmd.CombinedOutput()
		done <- struct {
			out []byte
			err error
		}{out, err}
	}()
	select {
	case res := <-done:
		return strings.TrimSpace(string(res.out)), res.err
	case <-time.After(25 * time.Second):
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("timed out")
	}
}

func toolPath(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return "(not found)"
}

func cacheHint() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return dir + "/yt-gecko"
	}
	return "?"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
