# syntax=docker/dockerfile:1

# Stage 1: Build React Admin Dashboard
FROM node:22-alpine AS admin-builder
WORKDIR /app/admin
COPY admin/package.json admin/package-lock.json ./
RUN npm ci
COPY admin/ ./
RUN npm run build

# Stage 2: Build Go binaries
FROM golang:1.24-alpine AS go-builder
WORKDIR /app
RUN apk add --no-cache git ca-certificates tzdata
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Copy compiled React admin SPA for embed.FS compilation
COPY --from=admin-builder /app/admin/dist ./admin/dist
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -extldflags '-static'" \
    -o /bin/go-cms ./cmd/go-cms

# Stage 3: Minimal hardened runtime image
FROM alpine:3.21
RUN addgroup -g 10001 -S appgroup && \
    adduser -u 10001 -S appuser -G appgroup
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy unified Go CMS executable
COPY --from=go-builder /bin/go-cms /app/go-cms

# Copy configuration, default themes, plugins, and OpenAPI specs
COPY --chown=appuser:appgroup config.example.yaml /app/config.yaml
COPY --chown=appuser:appgroup themes /app/themes
COPY --chown=appuser:appgroup plugins /app/plugins
COPY --chown=appuser:appgroup docs/openapi.yaml /app/docs/openapi.yaml
RUN mkdir -p /app/uploads && chown -R appuser:appgroup /app

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/app/go-cms"]
CMD ["start", "--config", "/app/config.yaml"]
