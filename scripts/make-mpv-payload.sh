#!/bin/sh
# Builds the embedded mpv payload from the upstream AppImage:
#   - unpacks the AppImage (its own --appimage-extract needs no FUSE)
#   - rewrites the absolute DT_NEEDED entry juNest leaves for libmujs
#   - sets a relative RUNPATH so the bundled libraries are found anywhere
#   - repacks the rootfs as a tar.gz for go:embed
# Usage: make-mpv-payload.sh <output-dir> <mpv.AppImage>
set -eu
out="$1"
appimage="$2"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

cp "$appimage" "$work/mpv.AppImage"
chmod +x "$work/mpv.AppImage"
(cd "$work" && ./mpv.AppImage --appimage-extract >/dev/null)

mpv="$work/squashfs-root/.junest/usr/bin/mpv"
[ -x "$mpv" ] || mpv="$work/squashfs-root/usr/bin/mpv"
[ -x "$mpv" ] || { echo "mpv binary not found in the AppImage" >&2; exit 1; }

# juNest's rootfs does not ship every library its mpv build links against
# (freetype, harfbuzz...). Copy the missing closure from the build host into
# the rootfs so the payload works on machines without them.
extras="$work/squashfs-root/.junest/usr/lib/yt-gecko-extras"
here="$(cd "$(dirname "$0")" && pwd)"
python3 "$here/bundle-missing-libs.py" "$work/squashfs-root" "$mpv" "$extras"

if command -v patchelf >/dev/null 2>&1; then
	patchelf --replace-needed /usr/lib/libmujs.so libmujs.so "$mpv" || true
	patchelf --set-rpath '$ORIGIN/../lib:$ORIGIN/../lib/yt-gecko-extras:$ORIGIN/../lib/x86_64-linux-gnu' "$mpv" || true
else
	echo "warning: patchelf not found; the payload may need system libraries" >&2
fi

tar -C "$work" -czf "$out/mpv.tar.gz" squashfs-root
ls -lh "$out/mpv.tar.gz"
