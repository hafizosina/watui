export PATH := /usr/local/go/bin:$(PATH)

APP_DIR := $(HOME)/App/WaTui
VERSION := $(shell git describe --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test deploy install run dev screenshots

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
