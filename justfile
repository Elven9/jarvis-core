# jarvis-core tasks. Run `just` to list them.

image := "jarvis-core"

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

# Build the container image for this machine's architecture
image:
    docker build -t {{image}} .

# Run the container image on port 8080 with a named data volume
image-run:
    docker run --rm -p 8080:8080 -v jarvis-data:/data {{image}}

# Remove build output
clean:
    rm -rf bin
