package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/charmbracelet/bubbletea"
	"github.com/jhoniwana/yt-gecko/internal/assets"
	"github.com/jhoniwana/yt-gecko/internal/auth"
	"github.com/jhoniwana/yt-gecko/internal/core"
	"github.com/jhoniwana/yt-gecko/internal/tui"
	"github.com/spf13/cobra"
)

var (
	audioOnly  bool
	maxResults int
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search YouTube and play the top result",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(c *cobra.Command, args []string) error {
		return runSearch(strings.Join(args, " "), audioOnly, maxResults)
	},
}

var rootCmd = &cobra.Command{
	Use:           "yt-gecko",
	Short:         "Terminal client for YouTube and YouTube Music",
	Long:          "Fast. Lightweight. Sticks to your terminal.",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(c *cobra.Command, args []string) error {
		g := gecko()
		defer g.Cleanup()
		return runTUI(g)
	},
}

// runTUI starts the interactive interface, enabling kitty-graphics
// thumbnails when the terminal implements the protocol. Mouse reporting is on
// so the scroll wheel navigates and ctrl+wheel zooms the cards.
func runTUI(g *core.GeckoCore) error {
	// Portable builds carry yt-dlp and mpv inside the binary and unpack them
	// into the user cache on first run; default builds use PATH.
	if tools, err := assets.Ensure(); err == nil {
		g.SetTools(tools)
		auth.SetYTDLPPath(tools.YTDLP)
		auth.SetJSRuntime(tools.QJS)
		if tools.Extracted {
			fmt.Fprintln(os.Stderr, "yt-gecko: unpacked bundled tools (first run only)")
		}
	} else {
		fmt.Fprintln(os.Stderr, "yt-gecko: bundled tools unavailable, using PATH:", err)
	}

	if tui.ProbeGraphics(os.Stdin, os.Stdout) {
		gw := tui.NewGFXWriter(os.Stdout)
		p := tea.NewProgram(tui.New(g, gw), tea.WithAltScreen(), tea.WithOutput(gw), tea.WithMouseAllMotion())
		_, err := p.Run()
		return err
	}
	return tui.Run(g)
}

func init() {
	searchCmd.Flags().BoolVarP(&audioOnly, "audio", "a", false, "play audio only (music mode)")
	searchCmd.Flags().IntVarP(&maxResults, "count", "n", 10, "number of search results")
	rootCmd.AddCommand(searchCmd)
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func gecko() *core.GeckoCore {
	g, err := core.NewGeckoCore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	return g
}

func runSearch(query string, audio bool, count int) error {
	g := gecko()
	defer g.Cleanup()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	go func() {
		<-sig
		g.Cleanup()
		os.Exit(0)
	}()

	results, err := g.Search(query, count)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return fmt.Errorf("no results for %q", query)
	}

	var lastErr error
	for _, r := range results {
		client, err := g.Play(r.URL, audio)
		if err != nil {
			lastErr = err
			fmt.Fprintf(os.Stderr, "Skipping %q: %v\n", r.Title, err)
			continue
		}
		_ = client
		fmt.Printf("Now playing: %s\n%s by %s\n\n", r.Title, r.DurationString(), r.Uploader)
		fmt.Println("Press Ctrl+C to stop playback.")
		return blockUntilDoneOrSignal(g)
	}
	return fmt.Errorf("all results failed to play: %w", lastErr)
}

func blockUntilDoneOrSignal(g *core.GeckoCore) error {
	done := make(chan error, 1)
	go func() { done <- g.Wait() }()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)

	select {
	case <-sig:
		return nil
	case err := <-done:
		return playbackError(err)
	}
}

// playbackError reports a playback failure but treats a user-terminated mpv
// (via Ctrl+C, which kills mpv) as a clean stop.
func playbackError(err error) error {
	if err == nil {
		return nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ProcessState != nil {
		if ws, ok := ee.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return nil
		}
	}
	return err
}
