# syntax=docker/dockerfile:1
# Two stages on the same Ubuntu base, so the cgo binary runs on the glibc it was built with.
# Changing one base image means changing both.

FROM ubuntu:26.10 AS builder

# Keep GO_VERSION equal to the go directive in go.mod.
# Checksums: https://go.dev/dl/?mode=json&include=all
ARG GO_VERSION=1.27.3
ARG GO_SHA256_AMD64=65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40
ARG GO_SHA256_ARM64=a06e99309dbcbdca1a8ccd2282c3ee146e55930886e26b4f1193269587165f49

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
