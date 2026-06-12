#!/bin/sh

set -eu

script_dir=$(dirname "$0")
repo_root=$(CDPATH= cd "$script_dir/.." && pwd)
: "${GOCACHE:=/tmp/booky-go-cache}"
: "${GOMODCACHE:=/tmp/booky-go-mod-cache}"
export GOCACHE GOMODCACHE

mkdir -p "$GOCACHE" "$GOMODCACHE"

do_web() {
	cd "$repo_root"
	if ! command -v rsvg-convert >/dev/null 2>&1; then
		printf 'rsvg-convert is required to build app icons. Install librsvg2-bin or another package that provides rsvg-convert.\n' >&2
		exit 1
	fi
	bun install --frozen-lockfile
	rm -rf web/dist
	mkdir -p web/dist
	cp web/src/index.html web/dist/index.html
	cp web/src/style.css web/dist/style.css
	cp web/src/icon.svg web/dist/icon.svg
	rsvg-convert -w 192 -h 192 web/src/icon.svg -o web/dist/icon-192.png
	rsvg-convert -w 512 -h 512 web/src/icon.svg -o web/dist/icon-512.png
	rsvg-convert -w 180 -h 180 web/src/icon.svg -o web/dist/apple-touch-icon.png
	bun build web/src/app.ts --target browser --format esm --minify --outfile web/dist/app.js
}

do_test() {
	cd "$repo_root"
	bun install --frozen-lockfile
	bun test web/src/*.test.ts
	go test -count=1 ./internal/...
}

do_build() {
	do_web
	bun run typecheck
	go test -count=1 ./...
	go vet ./...
	mkdir -p bin
	CGO_ENABLED=0 go build -o bin/booky ./cmd/booky
}

do_run() {
	do_web
	go run ./cmd/booky -config config.yaml
}

case "${1:-}" in
	test)
		do_test
		;;
	build)
		do_build
		;;
	run)
		do_run
		;;
	*)
		printf 'Usage: %s {run|test|build}\n' "$0" >&2
		exit 1
		;;
esac
