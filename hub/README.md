# Hub

The hub is the brain of doormutex: it collects signals (manual overrides, the desktop companion, the calendar, working hours, webhooks), decides the traffic light state and publishes it via REST/SSE and MQTT, including Home Assistant discovery.

See the [project README](../README.md) for the full concept and the [OpenAPI spec](../api/openapi.yaml) for the API.

## Run with Docker

```bash
cp config.example.yaml config.yaml      # adjust work hours, mqtt.url
docker compose -f compose.example.yaml up -d
docker compose -f compose.example.yaml exec hub doormutex-hub token create companion office
```

Open `http://localhost:8080/` for the web UI (it asks for a token). The token is printed once; store it in the client (companion, widget, Stream Deck, webhook). Tokens are stored as SHA-256 hashes. A hub serves several profiles (office, teen room, …); a token without a profile picks one with `?profile=`.

```bash
TOKEN=dm_…
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
| `doormutex-hub serve [-config path]` | run the hub (default; config from `DOORMUTEX_CONFIG`, default `/config/config.yaml`) |
| `doormutex-hub token create NAME [PROFILE]` | create an API token for a client; with a profile it only works for that profile |
| `doormutex-hub token list` / `token revoke NAME` | list / revoke tokens |
| `doormutex-hub google login\|logout PROFILE ACCOUNT` | connect or remove a Google account on a headless hub; on a desktop use `doormutex-companion google login ACCOUNT` ([guide](../docs/google-calendar.md)) |
| `doormutex-hub healthcheck` | used by the Docker `HEALTHCHECK` |
| `doormutex-hub version` | print the version |

Environment: `DOORMUTEX_CONFIG`, `DOORMUTEX_MQTT_URL`, `DOORMUTEX_MQTT_PASSWORD`, `DOORMUTEX_GOOGLE_CLIENT_SECRET` (secrets belong in the environment, not the file).

## Layout

| Package | Content |
|---|---|
| `internal/engine` | pure decision logic: claims, priorities, slots, phases – no clock, no I/O |
| `internal/holidays` | Austrian public holidays (Easter-based ones computed) + extra days off |
| `internal/schedule` | weekly working hours, DST-safe |
| `internal/calendar` | calendar provider interface + in-memory fake |
| `internal/calendar/google` | Google Calendar: several accounts, classification, OAuth login (CLI and through the companion) |
| `internal/secret` | AES-256-GCM encryption of the Google tokens at rest |
| `internal/service` | live state of one profile: inputs, expiry, message, change fan-out to SSE and MQTT |
| `internal/api` | REST + SSE handlers, bearer-token auth, profile selection |
| `internal/web` | embedded web UI / PWA (no build step) |
| `internal/store` | SQLite (pure Go): tokens, override/pomodoro, history |
| `internal/mqttpub` | MQTT publisher with Last Will, HA device discovery per profile, HA commands (select/text) |
| `internal/config` | YAML config |
| `cmd/doormutex-hub` | the binary |

## Development

```bash
DOORMUTEX_INTEGRATION=1 go test -race ./...   # incl. MQTT test against a throwaway Mosquitto in Docker
golangci-lint run
docker build -t doormutex-hub .
```

The MQTT integration test only talks to a container it starts itself on `127.0.0.1`, never to a real broker. It proves the Last Will by cutting the TCP connection through a proxy.
