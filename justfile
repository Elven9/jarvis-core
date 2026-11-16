# jarvis-core tasks. Run `just` to list them.

image := "jarvis-core"

# Image tag, from git. On a release tag it is the version (v0.1.0 -> 0.1.0); after one,
# <version>-<commits since>-g<commit>; before the first release, the commit. "-dirty" is added
# when there are uncommitted changes. Never "latest".
version := `git describe --tags --match 'v*' --always --dirty | sed 's/^v//'`

# Data volume of the server, shared by image-run and docker compose.
volume := "jarvis-data"

default:
    @just --list

# Build the server binary into bin/
build:
    CGO_ENABLED=1 go build -trimpath -o bin/jarvis ./cmd/jarvis

# Run all tests with the race detector
test:
    CGO_ENABLED=1 go test -race ./...

# Format check, vet and golangci-lint
lint:
    test -z "$(gofmt -l .)" || (gofmt -l . && exit 1)
    go vet ./...
    golangci-lint run ./...

# Format the code
fmt:
    gofmt -w .

# Lint and test
check: lint test

# Run the server locally (data in ./data)
run:
    CGO_ENABLED=1 go run ./cmd/jarvis

# Print the version the image is tagged with
version:
    @echo {{version}}

# Build the container image for this machine's architecture, tagged with the version
image:
    docker build -t {{image}}:{{version}} .

# Run the image of the current version on port 8080 with a named data volume
image-run:
    docker run --rm -p 8080:8080 -v {{volume}}:/data {{image}}:{{version}}

# Build the image of the current version and start the server and the tunnel
up: image
    IMAGE_TAG={{version}} docker compose up -d --remove-orphans

# Stop the server and the tunnel (data is kept)
down:
    IMAGE_TAG={{version}} docker compose down

# Follow the logs of the server and the tunnel
logs:
    IMAGE_TAG={{version}} docker compose logs -f

# Save the data volume to a .tgz (default: backups/<volume>-<time>.tgz). Stop the server first.
export-volume-data file="":
    #!/usr/bin/env bash
    set -euo pipefail
    docker volume inspect {{volume}} >/dev/null 2>&1 || { echo "volume {{volume}} does not exist" >&2; exit 1; }
    if [ -n "$(docker ps -q --filter volume={{volume}})" ]; then
        echo "a running container uses {{volume}}; stop it first (just down)" >&2; exit 1
    fi
    file="{{file}}"
    if [ -z "$file" ]; then file="backups/{{volume}}-$(date +%Y%m%d-%H%M%S).tgz"; fi
    mkdir -p "$(dirname "$file")"
    dir="$(cd "$(dirname "$file")" && pwd)"; name="$(basename "$file")"
    docker run --rm -v {{volume}}:/data:ro -v "$dir":/backup debian:trixie-slim \
        tar --numeric-owner -czf "/backup/$name" -C /data .
    echo "exported {{volume}} to $dir/$name"

# Load a .tgz from export-volume-data into a new data volume. Refuses if the volume exists.
import-volume-data file:
    #!/usr/bin/env bash
    set -euo pipefail
    [ -f "{{file}}" ] || { echo "no such file: {{file}}" >&2; exit 1; }
    if docker volume inspect {{volume}} >/dev/null 2>&1; then
        echo "volume {{volume}} already exists; remove it first (docker volume rm {{volume}})" >&2; exit 1
    fi
    dir="$(cd "$(dirname "{{file}}")" && pwd)"; name="$(basename "{{file}}")"
    docker volume create {{volume}} >/dev/null
    docker run --rm -v {{volume}}:/data -v "$dir":/backup:ro debian:trixie-slim \
        tar --numeric-owner -xzpf "/backup/$name" -C /data
    echo "imported $dir/$name into {{volume}}"

# Remove build output
clean:
    rm -rf bin
