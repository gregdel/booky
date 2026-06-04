.PHONY: web test build run

web:
	bun install --frozen-lockfile
	bun run build:web

test: web
	go test -count=1 ./...

build: web
	mkdir -p bin
	CGO_ENABLED=0 go build -o bin/booky ./cmd/booky

run: web
	go run ./cmd/booky -config config.yaml
