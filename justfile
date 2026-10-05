# The everyday commands (tech-design §15). `just` lists them.

# the version CI pins too (.github/workflows/ci.yml)
lint_version := "2.14.0"

# the databases docker-compose.yml starts; set them to test against others
export SQLMUX_TEST_PG := env("SQLMUX_TEST_PG", "postgres://sqlmux:sqlmux@localhost:55432/sqlmux?sslmode=disable")
export SQLMUX_TEST_MYSQL := env("SQLMUX_TEST_MYSQL", "sqlmux:sqlmux@tcp(localhost:53306)/sqlmux")

default:
    @just --list

# build bin/sqlmux
build:
    go build -o bin/sqlmux ./cmd/sqlmux

# run sqlmux from source
run *args:
    go run ./cmd/sqlmux {{args}}

# unit tests, golden files included
test:
    go test ./...

# the tests that need the databases of `just db-up` (in the main workspace only: AGENTS.md)
test-integration:
    go test -tags integration ./...

# end-to-end scripts in tmux, all of them or the named ones (just e2e f0.1 f0.2); one build for all
e2e *scripts:
    #!/usr/bin/env bash
    set -u
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT
    go build -o "$tmp/sqlmux" ./cmd/sqlmux || exit 1
    export E2E_BIN="$tmp/sqlmux" E2E_NOBUILD=1
    list="{{scripts}}"
    [ -n "$list" ] || list=$(cd e2e && ls *.sh | sed 's/\.sh$//' | grep -vx -e lib -e palette)
    failed=()
    for name in $list; do
        bash "e2e/$name.sh" || failed+=("$name")
    done
    [ ${#failed[@]} -eq 0 ] || { echo "e2e failed: ${failed[*]}"; exit 1; }

# golangci-lint at the pinned version, with .golangci.yml
lint:
    @golangci-lint --version | grep -q "version {{lint_version}} " || { echo "golangci-lint {{lint_version}} wanted: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v{{lint_version}}" >&2; exit 1; }
    golangci-lint run ./...

# gofmt every file
fmt:
    gofmt -w .

# go mod tidy
tidy:
    go mod tidy

# what every commit passes: vet, tests with the databases, lint, gofmt, go mod tidy
check:
    go vet ./...
    go vet -tags integration ./...
    go test ./...
    just test-integration
    just lint
    @test -z "$(gofmt -l .)" || { gofmt -l . >&2; echo "gofmt: just fmt" >&2; exit 1; }
    go mod tidy
    @git diff --quiet go.mod go.sum || { echo "go mod tidy changed go.mod or go.sum" >&2; exit 1; }

# rewrite the golden files (default: every package that has some) with a throwaway home and clipboard
golden *pkgs:
    #!/usr/bin/env bash
    set -eu
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT
    mkdir "$tmp/bin"
    printf '#!/bin/sh\ncat >"%s/clipboard"\n' "$tmp" >"$tmp/bin/pbcopy"
    printf '#!/bin/sh\ncat "%s/clipboard" 2>/dev/null\n' "$tmp" >"$tmp/bin/pbpaste"
    chmod +x "$tmp/bin/pbcopy" "$tmp/bin/pbpaste"
    export PATH="$tmp/bin:$PATH" XDG_CONFIG_HOME="$tmp/config" XDG_STATE_HOME="$tmp/state" XDG_DATA_HOME="$tmp/data"
    pkgs="{{pkgs}}"
    go test ${pkgs:-./internal/app ./internal/sqlkit ./internal/ui} -update

# rewrite the editor's expected results with nvim (internal/editor/testdata)
generate:
    go generate ./internal/editor

# start PG 17 and MySQL 8.4 of docker-compose.yml, seeded on their first start
db-up:
    docker compose up -d --wait postgres mysql

# stop them, the data kept
db-down:
    docker compose down

# start them over: a changed testdata/seed/*.sql is loaded only into an empty volume
db-reset:
    docker compose down -v
    docker compose up -d --wait postgres mysql

# the keys table or toml of `sqlmux keys` (--format md|toml, --check)
keys *args:
    go run ./cmd/sqlmux keys {{args}}

# remove bin/
clean:
    rm -rf bin
