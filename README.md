# Proxy Harold 🚀

A fast, caching CORS proxy server written in Go. Similar to [corsproxy.io](https://corsproxy.io) but self-hosted.

## Features

- **URL Proxying** - Fetch any HTTP/HTTPS URL
- **Caching** - BadgerDB for fast key-value storage with TTL
- **Rate Limiting** - Per-IP token bucket rate limiter
- **CORS Support** - Enables cross-origin requests from any domain
- **Graceful Shutdown** - Handles SIGINT/SIGTERM properly

## Quick Start

```bash
# Build
go build -o proxy-harold ./cmd/server

# Run
./proxy-harold
```

Server starts on `http://localhost:8080`

## Usage

```bash
# Fetch a URL
curl "http://localhost:8080/?url=https://api.example.com/data"

# Check if cached (look for X-Cache header)
curl -I "http://localhost:8080/?url=https://api.example.com/data"
```

### From JavaScript

```javascript
// No CORS issues!
const response = await fetch('http://localhost:8080/?url=https://api.example.com/data');
const data = await response.json();
```

## Configuration

Set via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server port |
| `CACHE_TTL` | `1h` | Cache time-to-live |
| `CACHE_DIR` | `./cache_data` | BadgerDB storage path |
| `RATE_LIMIT` | `100` | Requests per second per IP |
| `RATE_BURST` | `200` | Burst size for rate limit |
| `FETCH_TIMEOUT` | `30s` | Upstream fetch timeout |
| `MAX_RESPONSE_SIZE` | `10485760` | Max response size (10MB) |

## Cloudflare Tunnel

No TLS needed! Cloudflare handles HTTPS termination.

```bash
# Install cloudflared, then:
cloudflared tunnel --url http://localhost:8080
```

## API

### `GET /?url=<URL>`

Proxies the given URL and returns its content.

**Response Headers:**
- `Access-Control-Allow-Origin: *`
- `X-Cache: HIT | MISS`
- `X-RateLimit-Remaining: <number>`

**Error Responses:**
```json
{"error": "message", "code": 400}
```

### `GET /health`

Health check endpoint.

```json
{"status": "ok"}
```

## Development

```bash
# Run tests
go test -v ./...

# Run with race detector
go test -race ./...

# Run server in dev mode
go run ./cmd/server
```

## License

MIT
