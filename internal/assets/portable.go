//go:build portable

package assets

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed payload/yt-dlp payload/mpv.AppImage payload/qjs
var payload embed.FS

// payloadVersion names the cache directory; bump it whenever the embedded
// payloads change so stale extractions are not reused.
const payloadVersion = "1"

// Ensure extracts the embedded tools into the user cache (once per version)
// and returns their paths. mpv comes as an AppImage: it is extracted with its
// own --appimage-extract flag, which needs no FUSE, and then removed.
func Ensure() (Tools, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return Tools{}, err
	}
	dir := filepath.Join(cache, "yt-gecko", "tools", payloadVersion)
	stamp := filepath.Join(dir, ".ok")
	if _, err := os.Stat(stamp); err == nil {
		return toolsFor(dir), nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Tools{}, err
	}

	if err := writePayload(filepath.Join(dir, "yt-dlp"), "payload/yt-dlp"); err != nil {
		return Tools{}, err
	}
	if err := writePayload(filepath.Join(dir, "qjs"), "payload/qjs"); err != nil {
		return Tools{}, err
	}

	appImage := filepath.Join(dir, "mpv.AppImage")
	if err := writePayload(appImage, "payload/mpv.AppImage"); err != nil {
		return Tools{}, err
	}
	extractDir := filepath.Join(dir, "mpv")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return Tools{}, err
	}
	cmd := exec.Command(appImage, "--appimage-extract")
	cmd.Dir = extractDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return Tools{}, fmt.Errorf("extract bundled mpv: %w: %s", err, out)
	}
	_ = os.Remove(appImage)

	if err := os.WriteFile(stamp, []byte("ok"), 0o644); err != nil {
		return Tools{}, err
	}
	t := toolsFor(dir)
	t.Extracted = true
	return t, nil
}

// writePayload copies an embedded file out of the binary with exec bits.
func writePayload(dst, name string) error {
	data, err := fs.ReadFile(payload, name)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}

// toolsFor resolves the paths inside an extracted cache directory. AppImages
// ship in two layouts: the standard usr/ tree and the juNest rootfs used by
// some mpv builds (the binary and libraries live under .junest/usr).
func toolsFor(dir string) Tools {
	root := filepath.Join(dir, "mpv", "squashfs-root")
	for _, base := range []string{filepath.Join(root, ".junest"), root} {
		bin := filepath.Join(base, "usr", "bin", "mpv")
		if fi, err := os.Stat(bin); err != nil || fi.IsDir() {
			continue
		}
		libs := []string{filepath.Join(base, "usr", "lib")}
		for _, sub := range []string{"x86_64-linux-gnu", "lib"} {
			p := filepath.Join(base, "usr", "lib", sub)
			if fi, err := os.Stat(p); err == nil && fi.IsDir() {
				libs = append(libs, p)
			}
		}
		return Tools{
			YTDLP:      filepath.Join(dir, "yt-dlp"),
			MPV:        bin,
			MPVLibDirs: libs,
			QJS:        filepath.Join(dir, "qjs"),
		}
	}
	return Tools{YTDLP: filepath.Join(dir, "yt-dlp"), QJS: filepath.Join(dir, "qjs")}
}
