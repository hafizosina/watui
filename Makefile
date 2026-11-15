export PATH := /usr/local/go/bin:$(PATH)

.PHONY: build install test run

build:
	go build -o bin/watui ./cmd/watui

install: build
	install -Dm755 bin/watui $(HOME)/.local/bin/watui

test:
	go test ./...

run: build
	./bin/watui
