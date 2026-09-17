//go:build portable

package assets

import (
	"archive/tar"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed payload/yt-dlp payload/mpv.tar.gz payload/qjs
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

	tarball := filepath.Join(dir, "mpv.tar.gz")
	if err := writePayload(tarball, "payload/mpv.tar.gz"); err != nil {
		return Tools{}, err
	}
	extractDir := filepath.Join(dir, "mpv")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return Tools{}, err
	}
	if err := extractTarGz(tarball, extractDir); err != nil {
		return Tools{}, fmt.Errorf("extract bundled mpv: %w", err)
	}
	_ = os.Remove(tarball)

	if err := os.WriteFile(stamp, []byte("ok"), 0o644); err != nil {
		return Tools{}, err
	}
	t := toolsFor(dir)
	t.Extracted = true
	return t, nil
}

// extractTarGz unpacks a gzipped tar archive, preserving modes and symlinks.
func extractTarGz(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.Clean(hdr.Name))
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path in archive: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
}

// writePayload copies an embedded file out of the binary with exec bits.
func writePayload(dst, name string) error {
	data, err := fs.ReadFile(payload, name)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}

// findLoader returns the dynamic loader shipped in an AppImage rootfs, if
// any. Running mpv through it keeps the bundled glibc and libraries in use,
// independent of the host distribution.
func findLoader(base string) string {
	for _, name := range []string{"ld-linux-x86-64.so.2", "ld-linux.so.2", "ld-musl-x86_64.so.1"} {
		p := filepath.Join(base, "usr", "lib", name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
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
			MPVLoader:  findLoader(base),
		}
	}
	return Tools{YTDLP: filepath.Join(dir, "yt-dlp"), QJS: filepath.Join(dir, "qjs")}
}
