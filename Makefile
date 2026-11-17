export PATH := /usr/local/go/bin:$(PATH)

APP_DIR ?= $(HOME)/App/WaTui
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test deploy install run dev screenshots release

build:
	go build -ldflags "$(LDFLAGS)" -o bin/watui ./cmd/watui

test:
	go test ./...

deploy: test build
	install -Dm755 bin/watui $(APP_DIR)/watui
	install -Dm644 README.md $(APP_DIR)/README.md
	mkdir -p $(HOME)/.local/bin
	ln -sfn $(APP_DIR)/watui $(HOME)/.local/bin/watui
	@echo "deployed $(VERSION) to $(APP_DIR)/watui (linked from ~/.local/bin/watui)"

install: deploy

run: build
	./bin/watui

dev:
	watuidev

screenshots:
	./scripts/screenshots.sh

# make release V=v0.1.1 — tag and push a release; GitHub builds the binaries and
# publishes it (.github/workflows/release.yml). Add "## [v0.1.1] - date" to
# CHANGELOG.md first.
release:
	@test -n "$(V)" || { echo "usage: make release V=v0.1.1"; exit 1; }
	@echo "$(V)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$$' || { echo "version must look like v1.2.3"; exit 1; }
	@test "$$(git rev-parse --abbrev-ref HEAD)" = main || { echo "release from main"; exit 1; }
	@test -z "$$(git status --porcelain --untracked-files=no)" || { echo "commit your changes first"; exit 1; }
	@! git rev-parse -q --verify "refs/tags/$(V)" >/dev/null || { echo "$(V) is already tagged"; exit 1; }
	@grep -q '^## \[$(V)\]' CHANGELOG.md || { echo "add a '## [$(V)] - YYYY-MM-DD' section to CHANGELOG.md first"; exit 1; }
	go test ./...
	git tag -a $(V) -m "watui $(V)"
	git push origin main $(V)
	@echo "pushed $(V) — GitHub is building the release: https://github.com/hafizosina/watui/releases"
