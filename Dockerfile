# syntax=docker/dockerfile:1
# Two stages on Debian trixie (golang:*-trixie is built on it), so the cgo binary runs on the glibc
# it was built with. Changing the Debian release means changing both stages.

# Keep the Go version equal to the go directive in go.mod.
FROM golang:1.27.4-trixie AS builder

ENV GOTOOLCHAIN=local \
    CGO_ENABLED=1

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -o /out/jarvis ./cmd/jarvis

FROM debian:trixie-slim

RUN useradd --system --uid 10001 --no-create-home --shell /usr/sbin/nologin jarvis \
 && mkdir /data && chown jarvis:jarvis /data

COPY --from=builder /out/jarvis /usr/local/bin/jarvis

ENV JARVIS_DATA_DIR=/data
VOLUME /data
USER jarvis
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/jarvis"]
