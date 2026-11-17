export PATH := /usr/local/go/bin:$(PATH)

APP_DIR ?= $(HOME)/App/WaTui
# v0.1.0 on a release; v0.1.0-3-gabc1234+dev for a local build from the dev branch
BRANCH  := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)$(if $(filter-out main,$(BRANCH)),+$(BRANCH))
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

# Branches: work on dev (make deploy installs it locally); main is what's
# released on GitHub and only moves here.
#
# make release V=v0.1.1 — run on dev after adding "## [v0.1.1] - date" to
# CHANGELOG.md: fast-forwards main to dev, tags it and pushes; GitHub builds
# the binaries and publishes the release (.github/workflows/release.yml).
release:
	@test -n "$(V)" || { echo "usage: make release V=v0.1.1"; exit 1; }
	@echo "$(V)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$$' || { echo "version must look like v1.2.3"; exit 1; }
	@test "$(BRANCH)" = dev || { echo "release from dev (it moves main up to dev); you are on $(BRANCH)"; exit 1; }
	@test -z "$$(git status --porcelain --untracked-files=no)" || { echo "commit your changes first"; exit 1; }
	@! git rev-parse -q --verify "refs/tags/$(V)" >/dev/null || { echo "$(V) is already tagged"; exit 1; }
	@grep -q '^## \[$(V)\]' CHANGELOG.md || { echo "add a '## [$(V)] - YYYY-MM-DD' section to CHANGELOG.md first"; exit 1; }
	git fetch origin main
	@git merge-base --is-ancestor origin/main HEAD || { echo "GitHub main has commits dev doesn't: git merge origin/main first"; exit 1; }
	go test ./...
	git push origin dev
	git push origin HEAD:main
	git fetch . HEAD:main
	git tag -a $(V) -m "watui $(V)"
	git push origin $(V)
	@echo "released $(V) from dev — GitHub is building it: https://github.com/hafizosina/watui/releases"
