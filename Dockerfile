# syntax=docker/dockerfile:1
# Two stages on the same Ubuntu base, so the cgo binary runs on the glibc it was built with.
# Changing one base image means changing both.

FROM ubuntu:26.10 AS builder

# Keep GO_VERSION equal to the go directive in go.mod.
# Checksums: https://go.dev/dl/?mode=json&include=all
ARG GO_VERSION=1.27.4
ARG GO_SHA256_AMD64=7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840
ARG GO_SHA256_ARM64=3392582988a273a87ab5043ee1b2c9c17a2c3c00580bf9326c47a2e0aaaeac92

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl gcc libc6-dev \
 && rm -rf /var/lib/apt/lists/*

RUN set -eu; \
    arch="$(dpkg --print-architecture)"; \
    case "$arch" in \
      amd64) sha="$GO_SHA256_AMD64" ;; \
      arm64) sha="$GO_SHA256_ARM64" ;; \
      *) echo "unsupported architecture: $arch" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/go.tgz "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz"; \
    echo "$sha  /tmp/go.tgz" | sha256sum -c -; \
    tar -C /usr/local -xzf /tmp/go.tgz; \
    rm /tmp/go.tgz

ENV PATH=/usr/local/go/bin:$PATH \
    GOTOOLCHAIN=local \
    CGO_ENABLED=1

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -o /out/jarvis ./cmd/jarvis

FROM ubuntu:26.10

RUN useradd --system --uid 10001 --no-create-home --shell /usr/sbin/nologin jarvis \
 && mkdir /data && chown jarvis:jarvis /data

COPY --from=builder /out/jarvis /usr/local/bin/jarvis

ENV JARVIS_DATA_DIR=/data
VOLUME /data
USER jarvis
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/jarvis"]
