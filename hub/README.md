# Hub

The hub is the brain of office-traffic-light: it collects signals (manual overrides, the desktop companion, the calendar, working hours, webhooks), decides the traffic light state and publishes it via REST/SSE and MQTT, including Home Assistant discovery.

See the [project README](../README.md) for the full concept and the [OpenAPI spec](../api/openapi.yaml) for the API.

## Run with Docker

```bash
cp config.example.yaml config.yaml      # adjust work hours, mqtt.url
docker compose -f compose.example.yaml up -d
docker compose -f compose.example.yaml exec hub otl-hub token create companion
```

The token is printed once; store it in the client (companion, widget, Stream Deck, webhook). Tokens are stored as SHA-256 hashes.

```bash
TOKEN=otl_…
curl -s -H "Authorization: Bearer $TOKEN" localhost:8080/api/v1/status | jq
curl -s -X PUT -H "Authorization: Bearer $TOKEN" \
  -d '{"status":"dnd","mode":"duration","duration":"30m"}' localhost:8080/api/v1/override | jq
curl -N -H "Authorization: Bearer $TOKEN" localhost:8080/api/v1/events   # live stream
```

Without a companion heartbeat the light is `away` (off). For a quick test without the companion set `presence.require_companion: false`, or send a heartbeat:

```bash
curl -s -X POST -H "Authorization: Bearer $TOKEN" -d '{"locked":false}' localhost:8080/api/v1/signals/companion
```

## Commands

| Command | Purpose |
|---|---|
| `otl-hub serve [-config path]` | run the hub (default; config from `OTL_CONFIG`, default `/config/config.yaml`) |
| `otl-hub token create NAME` | create an API token for a client |
| `otl-hub token list` / `token revoke NAME` | list / revoke tokens |
| `otl-hub healthcheck` | used by the Docker `HEALTHCHECK` |
| `otl-hub version` | print the version |

Environment: `OTL_CONFIG`, `OTL_MQTT_URL`, `OTL_MQTT_PASSWORD` (the password is never read from the file).

## Layout

| Package | Content |
|---|---|
| `internal/engine` | pure decision logic: claims, priorities, slots, phases – no clock, no I/O |
| `internal/holidays` | Austrian public holidays (Easter-based ones computed) + extra days off |
| `internal/schedule` | weekly working hours, DST-safe |
| `internal/calendar` | calendar provider interface + in-memory fake (Google follows) |
| `internal/service` | live state: inputs, expiry, change fan-out to SSE and MQTT |
| `internal/api` | REST + SSE handlers, bearer-token auth |
| `internal/store` | SQLite (pure Go): tokens, override/focus, history |
| `internal/mqttpub` | MQTT publisher with Last Will, HA device discovery |
| `internal/config` | YAML config |
| `cmd/otl-hub` | the binary |

## Development

```bash
OTL_INTEGRATION=1 go test -race ./...   # incl. MQTT test against a throwaway Mosquitto in Docker
golangci-lint run
docker build -t otl-hub .
```

The MQTT integration test only talks to a container it starts itself on `127.0.0.1`, never to a real broker. It proves the Last Will by cutting the TCP connection through a proxy.
