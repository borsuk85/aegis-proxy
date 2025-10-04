# Aegis

[![CI](https://github.com/USER/REPO/actions/workflows/ci.yml/badge.svg)](https://github.com/USER/REPO/actions/workflows/ci.yml)
[![Docker](https://github.com/USER/REPO/actions/workflows/docker.yml/badge.svg)](https://github.com/USER/REPO/actions/workflows/docker.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/USER/REPO)](https://goreportcard.com/report/github.com/USER/REPO)

Caching reverse proxy with failover mechanism providing high availability for upstream services.

## Features

- **Cache with TTL**: Store responses with configurable expiration times
- **Intelligent failover**: Automatically serve from cache when upstream fails (5xx, timeout)
- **Selective caching**: Cache only GET and HEAD methods
- **Monitoring**: `/stats` endpoint with cache metrics
- **Security**: Automatic filtering of hop-by-hop headers
- **Thread-safe**: Handle concurrent requests with RWMutex locks

## Installation

### Local

```bash
go build -o aegis main.go
```

### Docker

```bash
# Build image
docker build -t aegis .

# Run
docker run -p 8009:8009 \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  aegis
```

### Docker Compose

```bash
# Start with mock upstream (httpbin)
docker-compose up -d

# Check logs
docker-compose logs -f aegis

# Stop
docker-compose down
```

## Configuration

Aegis is configured exclusively via **YAML file**. This simplifies configuration management and makes it more transparent.

### Configuration File

Aegis automatically searches for configuration file in the following locations:
- `config.yaml` (current directory - default)
- `aegis.yaml` (current directory)
- `/etc/aegis/config.yaml`

You can also specify a custom path: `./aegis -config /path/to/config.yaml`

**Example configuration file (`config.yaml`):**

```yaml
server:
  listen: ":8009"
  upstream: "http://localhost:3030"
  timeout: "1s"

cache:
  ttl: "5m"

  # HTTP headers to include in cache key
  # This allows different caching for different header values
  key_headers:
    - Authorization      # Different cache per user
    - Accept-Language    # Different cache per language
    # - X-Tenant-ID      # Different cache per tenant
```

**Note:** Full example is available in `config.example.yaml`.

### Parameters

| YAML Parameter | Default Value | Description |
|----------------|---------------|-------------|
| `server.listen` | `:8009` | Proxy listen address |
| `server.upstream` | `http://localhost:3030` | Upstream service URL |
| `server.timeout` | `1s` | Timeout for upstream requests |
| `cache.ttl` | `0` | Cache TTL (0 = no expiration) |
| `cache.key_headers` | `[]` | List of HTTP headers to include in cache key |

### Running

```bash
# With default config.yaml
./aegis

# With custom config file
./aegis -config /etc/aegis/production.yaml

# Docker (config.yaml mounted as volume)
docker run -p 8009:8009 \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  aegis
```

## Usage Examples

### Basic Usage

```bash
# Standard request - returns response from upstream or cache
curl http://localhost:8009/api/users

# Check cache status in response
curl -i http://localhost:8009/api/data

# Cache statistics
curl http://localhost:8009/stats
```

## Response Headers

### X-Cache

Indicates cache status for the request:

- `MISS`: Response fetched from upstream and saved to cache
- `HIT-BACKUP`: Response served from cache (upstream unavailable)
- `PASS`: Response fetched from upstream but not cached (e.g., 4xx status)
- `BYPASS`: Cache bypassed (method other than GET/HEAD)

### X-Served-By

Always set to `Aegis` - proxy identifier.

### X-Backup-Saved-At

Timestamp when response was saved to cache (only for `X-Cache: HIT-BACKUP`).

## /stats Endpoint

Returns JSON with cache metrics:

```json
{
  "cache_size": 42,
  "memory_bytes": 1048576,
  "memory_kb": 1024.00,
  "memory_mb": 1.00
}
```

## Advanced Caching

### Cache per user/tenant

By configuring `cache.key_headers`, you can create separate cache for different HTTP header values:

```yaml
cache:
  key_headers:
    - Authorization  # Different cache for each user
```

**Example:**
```bash
# User 1
curl -H "Authorization: Bearer user1-token" http://localhost:8009/api/profile
# X-Cache: MISS - saved to cache for user1

# User 2
curl -H "Authorization: Bearer user2-token" http://localhost:8009/api/profile
# X-Cache: MISS - saved to cache for user2 (different key)

# User 1 again
curl -H "Authorization: Bearer user1-token" http://localhost:8009/api/profile
# X-Cache: MISS - returned from user1 cache
```

### Cache per language/region

```yaml
cache:
  key_headers:
    - Accept-Language
    - X-Region
```

**Use cases:**
- **Multi-tenant SaaS**: `X-Tenant-ID` - separate cache per client
- **Internationalization**: `Accept-Language` - cache per language
- **API versioning**: `Accept` or `X-API-Version` - cache per API version
- **A/B testing**: `X-Experiment-Variant` - cache per test variant

### Multi-tenant Example

```yaml
cache:
  ttl: "10m"
  key_headers:
    - X-Tenant-ID
    - Authorization
```

Each tenant + user combination will have a separate cache entry.

## How It Works

1. **GET/HEAD request with success (2xx)**:
   - Response saved to cache
   - Returns header `X-Cache: MISS`

2. **GET/HEAD request with 5xx error or timeout**:
   - Attempt to serve from cache
   - If cache exists: `X-Cache: HIT-BACKUP`
   - If no cache: `502 Bad Gateway`

3. **GET/HEAD request with 4xx error**:
   - Response returned without caching
   - Header `X-Cache: PASS`

4. **POST/PUT/DELETE request**:
   - Cache completely bypassed
   - Header `X-Cache: BYPASS`

## Tests

```bash
# Run all tests
go test ./... -v

# Tests with coverage
go test ./... -cover

# Tests for specific package
go test ./internal/cache -v
go test ./internal/proxy -v
go test ./internal/utils -v

# Benchmarks
go test ./... -bench=.
```

### Test Coverage

Tests are organized in respective packages:

- **internal/cache**: basic operations, TTL, concurrency, memory (5 tests)
- **internal/proxy**:
  - Basic: forwarding, failover, timeout, stats (9 tests)
  - Cache key: with headers, multi-header, case-sensitivity (5 tests)
- **internal/utils**: URL joining, header filtering, context management (7 tests)

Total: **26 unit and integration tests**

## CI/CD

GitHub Actions workflows automatically run on every push and pull request:

### CI Pipeline (.github/workflows/ci.yml)
- **Test**: Runs all tests with race detection on Go 1.23 and 1.24
- **Lint**: Runs `go vet` and `staticcheck` for code quality
- **Build**: Compiles binary and uploads as artifact
- **Coverage**: Uploads test coverage to Codecov (optional)

### Docker Pipeline (.github/workflows/docker.yml)
- Builds Docker image on every push
- Tags images based on branch/tag/commit SHA
- Can push to GitHub Container Registry (commented out by default)

To enable image publishing, uncomment the push steps in `docker.yml` and ensure `GITHUB_TOKEN` has package write permissions.

## Test Script

Example script `test_requests.sh`:

```bash
#!/bin/bash

# Basic test
curl -i http://localhost:8009/

# Test with stats
curl http://localhost:8009/stats

# POST test (bypass cache)
curl -X POST http://localhost:8009/api/data -d '{"test": true}'
```

## Architecture

### Request Flow

```
┌─────────┐      ┌─────────┐      ┌──────────┐
│ Client  │─────▶│  Aegis  │─────▶│ Upstream │
└─────────┘      │         │      └──────────┘
                 │  Cache  │
                 └─────────┘
                      │
                      ▼
                 ┌─────────┐
                 │ Backup  │
                 │Response │
                 └─────────┘
```

### Project Structure

```
/services/Aegis/
├── main.go                      # Application entry point
├── config.example.yaml          # Example configuration file
├── internal/
│   ├── cache/                   # Cache management
│   │   ├── cache.go            # Thread-safe cache implementation
│   │   └── cache_test.go       # Cache tests
│   ├── proxy/                   # Reverse proxy logic
│   │   ├── proxy.go            # HTTP request handling
│   │   ├── proxy_test.go       # Proxy tests
│   │   └── proxy_cachekey_test.go  # Cache key with headers tests
│   ├── config/                  # Configuration
│   │   └── config.go           # Load from YAML
│   └── utils/                   # Helper functions
│       ├── http.go             # HTTP handling (headers, URL)
│       └── http_test.go        # Utils tests
├── go.mod
├── go.sum
├── Dockerfile
├── docker-compose.yml
└── README.md
```

### Packages

- **main**: Minimalist entry point, HTTP server setup
- **internal/cache**: Thread-safe in-memory cache with TTL
- **internal/proxy**: Reverse proxy with failover and configurable cache keys
- **internal/config**: Load configuration from YAML file
- **internal/utils**: Helper functions (headers, URL, context)

## Requirements

- Go 1.23+
- No external dependencies (stdlib only)
- Docker (optional)

## License

Aegis Project
