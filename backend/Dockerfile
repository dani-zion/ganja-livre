# ─── Stage 1: Builder ─────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

# Install only what's needed for compilation
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Cache dependency layer separately from source
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

# Build both binaries: API server and Temporal worker
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s -extldflags '-static'" \
    -o /dist/server ./cmd/server

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s -extldflags '-static'" \
    -o /dist/worker ./cmd/worker

# ─── Stage 2: API server image ────────────────────────────────────────────────
FROM scratch AS server

# Copy TLS certificates and timezone data
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

COPY --from=builder /dist/server /server

EXPOSE 8080

ENTRYPOINT ["/server"]

# ─── Stage 3: Worker image ────────────────────────────────────────────────────
FROM scratch AS worker-image

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

COPY --from=builder /dist/worker /worker

ENTRYPOINT ["/worker"]
