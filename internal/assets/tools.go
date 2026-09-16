// Package assets resolves the external tools yt-gecko drives (yt-dlp and
// mpv). The default build uses whatever is on PATH; the portable build
// (go build -tags portable) embeds both tools and extracts them to the user
// cache on first run, so the binary works on machines with nothing installed.
package assets

// Tools holds the resolved paths of the external tools. Empty fields mean
// "use PATH".
type Tools struct {
	YTDLP      string
	MPV        string
	MPVLibDirs []string
	// QJS is a bundled QuickJS runtime for yt-dlp's JavaScript challenges
	// (nsig/sig solving); empty means "let yt-dlp find deno/node".
	QJS string
	// Extracted is true when this call unpacked the embedded payloads.
	Extracted bool
}
