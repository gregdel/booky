.PHONY: web test build run release-check

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

release-check: web
	bun run typecheck
	go test -count=1 ./...
	go vet ./...
	mkdir -p bin
	CGO_ENABLED=0 go build -o bin/booky ./cmd/booky
