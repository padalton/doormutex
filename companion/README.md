# Companion (Linux)

The companion runs on the desktop and tells the hub what it can't know on its own: whether the **microphone** or **camera** is in use and whether the **screen is locked**. Before suspend and shutdown it **signs off**, so the hub can tell a PC that is off (light off) from a companion that crashed (blinking yellow). It also shows a **desktop notification** on every change of the light.

Reference setup: CachyOS, KDE Plasma 6 (Wayland), PipeWire. Only yes/no and application names go to the hub – never audio or video.

## How it detects

| Signal | Source |
|---|---|
| Microphone | `pw-dump`: capture streams (`Stream/Input/Audio`) in state *running*. Not counted: recordings of outputs (monitor ports, `stream.capture.sink`), level meters (`stream.monitor`, "Peak detect") and apps on the ignore list (EasyEffects keeps the microphone open all the time) |
| Camera | PipeWire video capture streams, plus processes holding `/dev/video*` open (`/proc/<pid>/fd`), for apps that use V4L2 directly |
| Screen lock | logind `LockedHint` of the user's graphical session (set by the screen locker) |
| Suspend / shutdown | logind `PrepareForSleep` / `PrepareForShutdown` with a *delay* inhibitor: goodbye first, then the system continues. Stopping the service (logout) also signs off |

## Install

```bash
cd companion
go build -o doormutex-companion ./cmd/doormutex-companion
install -Dm755 doormutex-companion ~/.local/bin/doormutex-companion
install -Dm644 doormutex-companion.service ~/.config/systemd/user/doormutex-companion.service

# config + token (create the token on the hub: doormutex-hub token create companion office)
install -Dm600 companion.example.yaml ~/.config/doormutex/companion.yaml
$EDITOR ~/.config/doormutex/companion.yaml
install -m600 /dev/null ~/.config/doormutex/token && $EDITOR ~/.config/doormutex/token

doormutex-companion check          # prints what it detects and asks the hub once
systemctl --user daemon-reload
systemctl --user enable --now doormutex-companion
journalctl --user -u doormutex-companion -f
```

Needs `pw-dump` (part of PipeWire) – nothing else.

## Google Calendar sign-in

```bash
doormutex-companion google login private   # account names from the hub profile's calendar.accounts
doormutex-companion google login work
```

Opens the browser with Google's sign-in (`xdg-open`); Google redirects back to the companion on `127.0.0.1`, which hands the code to the hub. The hub keeps the client secret and stores the refresh token encrypted. Details: [docs/google-calendar.md](../docs/google-calendar.md).

## Controls from the command line

The companion's hub address and token also drive the light from scripts, KDE global shortcuts or Stream Deck "run command" keys:

```bash
doormutex-companion set red            # until the next automatic change
doormutex-companion set red 30m        # or 1h30m …
doormutex-companion set green event    # until the current event ends; also: workday, forever
doormutex-companion set auto           # back to automatic
doormutex-companion pomodoro 25        # 25/5 (50 → 50/10, or "45m 15m"); add "yellow" for a yellow work phase
doormutex-companion pomodoro stop
doormutex-companion status
```

**KDE shortcut:** *System Settings → Keyboard → Shortcuts → Add New → Command or Script* → `doormutex-companion set red 30m`.

## Development

```bash
go test -race ./...
golangci-lint run
```

The tests use synthetic data only: a hand-written `pw-dump` document, a fake `/proc` tree, a fake hub. The D-Bus parts (`internal/desktop`) are thin wrappers; the logic around them (`internal/agent`) is tested with fakes.
