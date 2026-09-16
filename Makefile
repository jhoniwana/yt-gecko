.PHONY: build install install-local launcher clean test build-linux build-macos build-all

BINARY_NAME  := yt-gecko
BINARY_TARGET:= ./bin
LDFLAGS      := -s -w

BIN_DIR   := $(HOME)/.local/bin
APP_DIR   := $(HOME)/.local/share/applications
ICON_SIZES := 64 128 256
HK_DIR    := $(HOME)/.local/share/icons/hicolor
ASSETS    := assets

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_TARGET)/$(BINARY_NAME) .

install: build
	sudo install -m 0755 $(BINARY_TARGET)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)

# install-local: build + put the binary in ~/.local/bin (no sudo needed).
install-local: build
	mkdir -p $(BIN_DIR)
	install -m 0755 $(BINARY_TARGET)/$(BINARY_NAME) $(BIN_DIR)/$(BINARY_NAME)

# launcher: install the app-menu shortcut (.desktop) + hicolor icon, so the
# app shows up in the desktop's application menu. Depends on install-local.
launcher: install-local
	mkdir -p $(APP_DIR) $(HK_DIR)
	for s in $(ICON_SIZES); do \
		install -Dm 0644 $(ASSETS)/icon/$$s.png $(HK_DIR)/$$sx$$s/apps/yt-gecko.png; \
	done
	install -m 0644 $(ASSETS)/yt-gecko.desktop $(APP_DIR)/yt-gecko.desktop
	-update-desktop-database $(APP_DIR) 2>/dev/null || true
	-gtk-update-icon-cache -f -t $(HK_DIR) 2>/dev/null || true

clean:
	rm -rf $(BINARY_TARGET)

test:
	go test ./...

build-linux:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINARY_TARGET)/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINARY_TARGET)/$(BINARY_NAME)-linux-arm64 .

build-macos:
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINARY_TARGET)/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINARY_TARGET)/$(BINARY_NAME)-darwin-arm64 .

build-all: build-linux build-macos
# --- Portable single-file build -------------------------------------------
# `make assets` downloads the payloads (they are gitignored: ~150 MB), and
# `make portable` builds one binary with everything embedded. On first run the
# binary unpacks yt-dlp and mpv into the user cache, so it needs nothing
# installed on the target machine.
YTDLP_URL   := https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_linux
MPV_APPIMAGE:= https://github.com/ivan-hc/MPV-appimage/releases/download/continuous/mpv-Media-Player_0.41.0-6-archimage5.0-x86_64.AppImage
QJS_URL     := https://github.com/quickjs-ng/quickjs/releases/download/v0.16.2/qjs-linux-x86_64

.PHONY: assets portable

assets:
	mkdir -p internal/assets/payload
	[ -x internal/assets/payload/yt-dlp ] || curl -L --fail -o internal/assets/payload/yt-dlp $(YTDLP_URL)
	[ -x internal/assets/payload/mpv.AppImage ] || curl -L --fail -o internal/assets/payload/mpv.AppImage $(MPV_APPIMAGE)
	[ -x internal/assets/payload/qjs ] || curl -L --fail -o internal/assets/payload/qjs $(QJS_URL)
	chmod +x internal/assets/payload/yt-dlp internal/assets/payload/mpv.AppImage internal/assets/payload/qjs

portable: assets
	go build -tags portable -ldflags "$(LDFLAGS)" -o $(BINARY_TARGET)/$(BINARY_NAME)-portable .
	@ls -lh $(BINARY_TARGET)/$(BINARY_NAME)-portable
