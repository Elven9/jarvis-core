# jarvis-core tasks. Run `just` to list them.

image := "jarvis-core"

# Image tag, from git. On a release tag it is the version (v0.1.0 -> 0.1.0); after one,
# <version>-<commits since>-g<commit>; before the first release, the commit. "-dirty" is added
# when there are uncommitted changes. Never "latest".
version := `git describe --tags --match 'v*' --always --dirty | sed 's/^v//'`

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
    docker run --rm -p 8080:8080 -v jarvis-data:/data {{image}}:{{version}}

# Remove build output
clean:
    rm -rf bin
