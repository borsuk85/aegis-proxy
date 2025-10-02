# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /build

# Copy go mod files (if they exist)
COPY go.* ./
RUN go mod download 2>/dev/null || true

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o aegis .

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/aegis .

# Expose default port
EXPOSE 8009

# Run as non-root user
RUN addgroup -g 1000 aegis && \
    adduser -D -u 1000 -G aegis aegis && \
    chown -R aegis:aegis /app

USER aegis

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8010/stats || exit 1

ENTRYPOINT ["/app/aegis"]
