# syntax=docker/dockerfile:1

# ── Stage 1: build ────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache ca-certificates git

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/cacheproxyfy ./main.go

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/healthcheck ./cmd/healthcheck/main.go

# Pre-create the artifact directory so it exists under the nonroot uid.
RUN mkdir -p /app/data/artifacts

# ── Stage 2: runtime ──────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/cacheproxyfy /cacheproxyfy
COPY --from=builder /out/healthcheck /healthcheck
COPY --from=builder /app/data/artifacts /app/data/artifacts
COPY --from=builder /src/cacheproxyfy.yaml /app/cacheproxyfy.yaml

WORKDIR /app

EXPOSE 8080
EXPOSE 9090

HEALTHCHECK --interval=10s --timeout=5s --start-period=15s --retries=5 \
    CMD ["/healthcheck"]

ENTRYPOINT ["/cacheproxyfy"]
