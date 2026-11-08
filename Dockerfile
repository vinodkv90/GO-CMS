# syntax=docker/dockerfile:1

# Stage 1: Build binary
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Cache go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build statically compiled binary with security flags
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -extldflags '-static'" \
    -o /bin/cms-server ./cmd/server

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -extldflags '-static'" \
    -o /bin/cms-migrate ./cmd/migrate

# Stage 2: Minimal hardened runtime image
FROM alpine:3.20

# Create non-root user and group
RUN addgroup -g 10001 -S appgroup && \
    adduser -u 10001 -S appuser -G appgroup

# Install runtime certificates and tzdata
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /bin/cms-server /app/cms-server
COPY --from=builder /bin/cms-migrate /app/cms-migrate

# Copy configuration and static templates/themes
COPY --chown=appuser:appgroup config.example.yaml /app/config.yaml
RUN mkdir -p /app/uploads /app/themes /app/plugins && \
    chown -R appuser:appgroup /app

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/app/cms-server"]
CMD ["-config", "/app/config.yaml"]

