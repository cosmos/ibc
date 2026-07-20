# syntax=docker/dockerfile:1

FROM golang:1.26-trixie AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -o /out/ibc ./cmd/ibc/...

FROM debian:trixie-slim AS certs
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=certs --chown=nonroot:nonroot /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt

COPY --from=builder --chown=nonroot:nonroot /out/ibc /bin/ibc

USER nonroot:nonroot
ENTRYPOINT ["/bin/ibc"]
