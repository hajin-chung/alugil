# alugil

Small HTTP forwarding gateway for Docker-reachable services.

## Usage

```
cp config.example.yaml config.yaml
go run ./cmd/alugil -config config.yaml
```

## Config

```yaml
listen_addr: ":8080"
log_path: "./alugil.log"
services:
  docmost: [3000]
  filebrowser: [80]
```

## Behavior

- `/:service/:port/*path` → `http://<service>:<port>/<path>`
- Visiting `/:service/:port` sets a cookie and redirects to `/`, so relative asset paths resolve correctly
- Unknown or disallowed targets return `404`, malformed ports return `400`
- Upstream failures return `502`, or `504` on timeout

## Docker

```
docker build -t alugil .
docker run --rm -p 8080:8080 -v "$PWD/config.yaml:/app/config.yaml:ro" alugil -config /app/config.yaml
```

See `docker-compose.yaml` for a Traefik + compose setup.
