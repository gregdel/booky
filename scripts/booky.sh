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
	bun install --frozen-lockfile
	rm -rf web/dist
	mkdir -p web/dist
	cp web/src/index.html web/dist/index.html
	cp web/src/style.css web/dist/style.css
	bun build web/src/app.ts --target browser --format esm --minify --outfile web/dist/app.js
}

do_test() {
	do_web
	go test -count=1 ./...
}

do_build() {
	do_web
	mkdir -p bin
	CGO_ENABLED=0 go build -o bin/booky ./cmd/booky
}

do_run() {
	do_web
	go run ./cmd/booky -config config.yaml
}

do_release_check() {
	do_web
	bun run typecheck
	go test -count=1 ./...
	go vet ./...
	mkdir -p bin
	CGO_ENABLED=0 go build -o bin/booky ./cmd/booky
}

case "${1:-}" in
	web)
		do_web
		;;
	test)
		do_test
		;;
	build)
		do_build
		;;
	run)
		do_run
		;;
	release-check)
		do_release_check
		;;
	*)
		printf 'Usage: %s {web|test|build|run|release-check}\n' "$0" >&2
		exit 1
		;;
esac
