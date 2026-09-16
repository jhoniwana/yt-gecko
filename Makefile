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