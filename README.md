# alugil

Small HTTP forwarding gateway for Docker-reachable services.

## What it does

- loads a small YAML allowlist config
- serves `GET /health`
- proxies `/:service/:port/*path` to `http://<service>:<port>/<path>`
- rejects anything not explicitly allowed
- writes JSON logs to `log_path`
- prints concise `warn` / `error` lines to stdout

No extra request tracing layer, no custom request ID handling, no added X-header machinery beyond what Go's reverse proxy does on its own.

## Run locally

1. Copy the example config:
   - `cp config.example.yaml config.yaml`
2. Start the server:
   - `go run ./cmd/alugil -config config.yaml`
3. Check health:
   - `curl http://localhost:8080/health`
4. Proxy an allowed service:
   - `curl http://localhost:8080/docmost/3000/api/health`

## Behavior

- route format: `/:service/:port/*path`
- unknown or disallowed targets return `404`
- malformed ports return `400`
- upstream proxy failures return `502`, or `504` on timeout
- the `/<service>/<port>` prefix is stripped before forwarding upstream

## Config

```yaml
listen_addr: ":8080"
log_path: "./alugil.log"
services:
  docmost: [3000]
  filebrowser: [80]
```

## Docker

Build the image:
- `docker build -t alugil .`

Run it:
- `docker run --rm -p 8080:8080 -v "$PWD/config.yaml:/app/config.yaml:ro" alugil -config /app/config.yaml`

Compose example:
- see `docker-compose.yaml`
- it assumes:
  - external network `proxy-net`
  - Traefik routes `route.deps.me` to this container
  - your real config lives at `./config.yaml`
  - logs are written under `./logs`

## Current structure

```text
cmd/alugil/main.go
internal/config/config.go
internal/config/config_test.go
internal/server/server.go
internal/server/server_test.go
config.example.yaml
Dockerfile
README.md
```
