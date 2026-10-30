# doormutex 🚦

*A mutex for your door.*

**A traffic light for my basement office.** From the hallway the family can see whether it's worth coming downstairs:
🔴 **red** = meeting, please don't disturb · 🟡 **yellow** = working, come in if it's important · 🟢 **green** = come in · ⚫ **off** = not in the office.

The state is detected automatically (microphone/camera, Google Calendar, working hours, screen lock) and can be overridden at any time from a KDE widget, a Stream Deck or the web UI. Open source & open hardware: software, firmware, PCB (KiCad) and a 3D-printed enclosure (OpenSCAD).

> **Status: early development.** This README is the project definition and feature set (as of 2026-10-30). The repo is public from day one so anyone can follow the project as it grows. Software: [`hub/`](hub/) (with web UI), [`companion/`](companion/), [`plasmoid/`](plasmoid/).
>
> More documents: [Breadboard prototype & parts list](docs/breadboard.md) · [Development setup](docs/dev-setup.md) · [Google Calendar](docs/google-calendar.md) · [iPhone & Shortcuts](docs/apple-shortcuts.md) · [Hub](hub/README.md) · [API (OpenAPI)](api/openapi.yaml)

## Signals

One traffic light, three chambers, just like on the road: red on top, yellow in the middle, green at the bottom. The position tells the meaning, even without telling the colours apart.

| Light | State | Meaning | E-ink sign (example) |
|---|---|---|---|
| 🔴 red | `dnd` | Meeting – please don't come in | "Meeting · until 14:30" |
| 🟡 yellow | `working` | Working – only if it's important | "Working · next meeting 15:00" |
| 🟢 green | `free` | Come in! | "Free · next appointment tomorrow 09:00" |
| ⚫ off | `away` | Not in the office (PC locked or off) | "Not in the office" |

**Phases**, modelled on the Austrian road traffic regulations (StVO § 38):

| Light | Phase | When |
|---|---|---|
| 🟢 blinking | **Last call** | before a meeting, coming from green – "quick, I'm in a meeting soon" |
| 🔴 / 🟡 alternating | **Last call from yellow** | before a meeting, coming from yellow. The StVO has no such signal, and blinking yellow is taken by the fault |
| 🔴 + 🟡 | **Ending soon** | before a meeting ends and yellow or green follows. In the StVO, red+yellow means: stop, green is coming |
| 🟡 blinking | **Fault** | hub or Wi-Fi gone, data too old, or the companion went silent without signing off. Blinking yellow means "caution" in the StVO |
| off + "battery empty" | **Battery empty** | cut-off threshold reached |

**Timing:** by default the phases follow the StVO – green blinks four times, yellow shows ≈ 3 s before red, red+yellow ≈ 1–2 s before green. That's authentic, but over in seconds. A switch per phase (config and web UI) turns it into a real warning with a lead time instead, e.g. 3 or 5 minutes.

### E-ink sign

A small e-ink display sits below the light. It shows **absolute times instead of a countdown** ("until 14:30" rather than "23 min left"). So the content only changes when the state changes: no motion sensor needed, no power while idle.

```
┌────────────────────┐
│ MEETING            │
│ until 14:30        │  ← large font
│ camera on          │
│ then work → 17:00  │
│ ▮▮▮▯ 72 %    14:02 │
└────────────────────┘
```

1.54″, 200 × 200 pixels. The light already shows the colour; the sign adds the small print. "until 14:30" is ~5 mm tall, the rest ~2 mm: readable when you stand at the top of the stairs.

Content: reason, until when, what comes next, hints such as "camera on" (don't walk through the picture), battery level and a "last updated" time. That time is refreshed every 15 min with a partial refresh so a frozen display stands out. Event titles are never shown.

## Feature set

### Must – v1

- **Traffic light hardware:** three chambers (red/yellow/green), ESP32-C6, Wi-Fi + MQTT, 18650 cell with safe charging electronics, USB-C for charging *and* permanent power
- **E-ink sign:** until when, what's next, reason, battery
- **Enclosure as a miniature traffic light** (OpenSCAD) with a removable wall mount
- **Automatic detection:**
  - microphone or camera in use → red
  - Google Calendar → red or yellow, by rules
  - working hours (incl. Austrian public holidays) → yellow
  - PC locked → off
  - otherwise → green
- **Manual override with duration:** from 15 min to "until end of work day", "until the next automatic change" or indefinite; "auto" resets
- **KDE Plasma widget** (panel and desktop): see the detected state and its reason, override with a time, pomodoro
- **Desktop notification** on every state change
- **Last call** and **failsafe** (see Signals)
- **Home Assistant via MQTT discovery:** state, state-change event, current slot, next free slot, battery
- **Hub as a Docker container** in the homelab

### Should

- **Ending soon** phase (red+yellow)
- **Stream Deck XL** as a control panel
- **Flashing and Wi-Fi setup in the browser** (ESP Web Tools + Improv), OTA updates
- **Automatic brightness** via light sensor, night dimming
- **Battery runtime estimate** in HA
- **Profiles:** several people and lights on one hub, each with its own rules, calendars and tokens
- **Room mode** for people without a PC companion – a teenager's room, the parents' bedroom or living room: controlled by phone (web app, Apple Shortcuts), web UI or Home Assistant, and through HA by voice via Alexa or Google Home (see [Profiles & room mode](#profiles--room-mode))

### Could

- **Matter over Wi-Fi** (uncertified, without the logo): Apple Home, Google Home, Alexa and HA directly, next to MQTT. Matter has no traffic-light device type, so each state becomes an on/off endpoint (mutually exclusive)
- Generic **webhook signals**, e.g. for Claude Code (see below)
- **ESPHome example config** for builds on mains power ("bring your own lamp")
- **Prometheus metrics** and a Grafana dashboard (meeting statistics)
- **Wi-Fi 6 TWT** on the ESP32-C6 for longer battery life, if the router supports it
- Companion for Windows/macOS, GNOME extension
- "Tell me when free" push (ntfy) – for families who don't just text anyway

### Won't – on purpose

| Idea | Why not |
|---|---|
| A second light at the basement door | The decision is made at the top of the stairs – one light is enough |
| Knock button / back channel | The family texts on WhatsApp anyway |
| Dedicated control desk ("signal box") | Stream Deck XL + widget cover it |
| Motion / mmWave sensor on the device | The LEDs stay on (visible from the hallway) and the e-ink only changes with the state. An mmWave sensor (~70–80 mA) would drain the battery in 1–2 days |
| ESP-NOW / Thread / Zigbee | Wi-Fi is good everywhere, MQTT broker and HA exist – Wi-Fi is simpler |
| Countdown on LEDs | too much power – the e-ink shows absolute times |
| Work/private detection via folder paths, VPN etc. | doesn't fit: everything runs on one machine, paths are personal |
| Ready-made HA automations | entities + triggers only; everyone builds their own automations |
| RGB LEDs (WS2812) | real single colours, position encodes meaning; WS2812 draw current even when "off" |

## Architecture

```mermaid
flowchart LR
  subgraph desk["Desk (KDE Plasma)"]
    comp["Companion<br/>mic · camera · lock<br/>notifications"]
    widget["Plasma widget<br/>state · override · pomodoro"]
    deck["Stream Deck XL<br/>OpenDeck plugin"]
  end
  subgraph lab["Homelab (Docker)"]
    hub["Hub<br/>rule engine · API · web UI<br/>e-ink rendering"]
    mqtt[("MQTT broker")]
    ha["Home Assistant"]
  end
  gcal["Google Calendar"] --> hub
  comp -->|signals + heartbeat| hub
  hub -->|state changes| comp
  widget <--> hub
  deck <--> hub
  hub <--> mqtt
  mqtt <--> ha
  mqtt <-->|Wi-Fi| light["🚦 Traffic light at the stairs<br/>ESP32-C6 · e-ink · 18650"]
  ha -.->|your own automation| hue["Hue stair light"]
```

**Principle: smart hub, dumb light.** The hub decides everything – state, phase, blink pattern, e-ink content – and publishes it via MQTT. The light only displays, reports telemetry and switches to "fault" by itself when the hub goes quiet.

| Component | Tech | Job |
|---|---|---|
| Hub | Go, Docker, SQLite | Rule engine, calendar, working hours, REST/SSE API, web UI, MQTT + HA discovery, e-ink rendering |
| Companion | Go, systemd user service | Microphone/camera (PipeWire), screen lock, desktop notifications, OAuth helper |
| Plasma widget | QML (Plasma 6) | State, override, pomodoro |
| Stream Deck plugin | TypeScript (Elgato SDK) | Control panel on the Stream Deck XL |
| Firmware | C, ESP-IDF | Wi-Fi power saving, MQTT, LEDs, e-ink, battery |
| Project homepage | GitHub Pages (static) + own domain | Required for Google's OAuth branding (description, privacy policy, terms), plus docs, web flasher (ESP Web Tools needs HTTPS) and the OAuth redirect relay. No backend, no user data |
| PCB | KiCad | ESP32-C6, charging, LED and e-ink drivers – assembled by JLCPCB |
| Enclosure | OpenSCAD + BOSL2 | Miniature traffic light + removable wall mount |

Home Assistant is not a separate component: the hub announces its entities via MQTT discovery and HA picks them up.

**Why Go?** One static binary per component, tiny Docker images, good libraries for MQTT, D-Bus and Google APIs – and the companion can later be cross-compiled for Windows/macOS.

**API first:** the OpenAPI spec in [`api/openapi.yaml`](api/openapi.yaml) is the contract between hub, companion, widget and Stream Deck plugin. A test keeps spec and router in sync.

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/status` | current state incl. reason, slot, next free slot |
| `GET /api/v1/timeline?from=…&to=…` | planned slots from calendar, working hours and timers |
| `PUT` / `DELETE /api/v1/override` | set an override (status + duration or end) / clear it |
| `PUT` / `DELETE /api/v1/pomodoro` | start / stop a pomodoro timer (work phase red by default; setting `pomodoro_work_status` or `work_status` per request for yellow) |
| `POST /api/v1/signals/companion` | companion heartbeat (microphone, camera, lock) and goodbye before shutdown/suspend |
| `POST` / `DELETE /api/v1/signals/{source}` | webhook signals with TTL |
| `GET /api/v1/events` | live updates (Server-Sent Events) |
| `GET /api/v1/profiles` | profiles the token may see, with their state |
| `GET` / `PUT /api/v1/settings` | light per calendar event kind, warning timing (saved, overrides the config) |
| `PUT` / `DELETE /api/v1/message` | e-ink message |
| `GET /api/v1/calendar`, `GET /api/v1/history` | calendar account health, recent status changes |

## State logic

Every source contributes a *claim*: status, priority and validity. The best valid claim wins.

| Prio | Source | Result | Details |
|---|---|---|---|
| 1 | Microphone or camera in use | `dnd` | counts after ≥ 10 s (debounce), holds for 2 min (reconnects, chat after the call) |
| 2 | PC locked, shut down or suspended | `away` | the companion signs off; silence without a goodbye is a fault (see below) |
| 3 | Manual override (widget, Stream Deck, web UI, phone, HA) or pomodoro | any | always ends: after a duration, "until the event ends", "until end of work day", "until the next automatic change" – or explicitly indefinite |
| 4 | Calendar event | by event kind | see [Google Calendar](#google-calendar) |
| 5 | Working hours | `working` | weekly plan, public holidays, extra days off |
| 6 | Webhook signals (optional) | usually `working` | with TTL |
| 7 | Default | `free` | – |

- **Microphone/camera beat everything,** including a manual override: a spontaneous Slack or Teams call turns the light red at once. The override isn't cleared and applies again after the call.
- **Microphone/camera beat the lock screen:** nobody locks the PC by hand during a call. But in a meeting where you only listen, you don't touch the mouse, and after a few minutes the screen locks itself. It stays red anyway.
- **Locking beats the override:** a nerd only locks the PC when leaving the desk. The override isn't cleared either; after unlocking it applies again while it's still valid.
- **Heartbeats:** the companion reports every 30 s and signs off on shutdown, suspend and lock (logind). If it is silent for 90 s *without* signing off (crash, killed), the light shows a fault (yellow blinks) until the end of the work day, then off.
- **Public holidays and days off** only remove the working hours. Calendar events still count: PC on with meetings on a holiday → red as usual (holiday workers, self-employed); PC on without events → green; PC off → off.
- **Failsafe on the light:** hub offline (MQTT Last Will) or state too old → yellow blinks.
- **Planning ahead:** the hub knows the future (calendar + working hours). From it, it computes the current slot, the next change and the next free slot – for the e-ink, the widget and HA.
- **Unknown end:** a call without a calendar entry or a locked screen has no known end. Then `until` and `next_free` are `null` instead of a guess. If a calendar meeting is running during a call, the call is assumed to end with the meeting.

## Profiles & room mode

One hub, several **profiles** – one per person and light. Each profile has its own rules, calendars, working hours, MQTT topics and API tokens. A token only controls its own profile: the teenager's phone can't switch the office light.

**Room mode** is a profile without a PC companion and usually without a calendar. It is for everyone who isn't the nerd in the basement: a teenager's room, the parents' bedroom or living room.

- **Controls:** phone, web UI and Home Assistant – and through HA by voice via Alexa or Google Home.
- **No companion required:** the light shows what was set last; when that ends, it falls back to the profile's default (e.g. off).
- **The e-ink shows a short message** ("Gaming until 21:00").
- **The office profile can also run without a companion,** controlled only via HA (light and message) – for setups where the companion can't run.

**Configuration** (see [`hub/config.example.yaml`](hub/config.example.yaml)):

```yaml
profiles:
  - id: office
    mode: office # companion, calendar, working hours
  - id: teen
    name: Teen room
    mode: room # no companion, no working hours, off by default
```

```bash
doormutex-hub token create teen-phone teen   # works only for profile "teen"
doormutex-hub token create widget            # all profiles, choose with ?profile=
```

The API serves the token's profile without any extra parameter, so the same client works for every profile. `GET /api/v1/profiles` lists the profiles a token may see.

Where room mode runs for people *without* a homelab (no Docker, no HA) is still open – see [open questions](#open-questions).

## Signal sources

### Companion (Linux)

Implemented in [`companion/`](companion/) – install as a systemd user service, see its README.

- **Microphone:** PipeWire capture streams (`media.class = Stream/Input/Audio`) in state "running". Ignored:
  - level meters (pavucontrol, the Plasma volume applet)
  - recordings of outputs (monitor sources, e.g. screen recording)
  - apps on a configurable ignore list
- **Camera:** open `/dev/video*` devices and PipeWire video streams.
- **Screen lock:** logind `LockedHint` of the user's graphical session (a systemd user service has no session of its own).
- **Goodbye:** before suspend and shutdown (logind delay inhibitor) and when the service stops; silence without a goodbye is a fault.
- **Notifications** on every state change, with the reason ("🔴 microphone in use – Firefox") and quick actions ("wrong detection", "clear override").
- **Reference setup:** CachyOS, KDE Plasma 6 (Wayland), PipeWire.

### Google Calendar

- **Connection:** Google Calendar API (recommended: near real time, provides `eventType`, attendees and conference links) or the secret iCal address (simpler, but sometimes heavily delayed).
- **Several Google accounts per profile,** e.g. private and work: each account has its own sign-in and refresh token, and any number of calendars per account.
- **Mirrored blockers:** in the reference setup HubSpot's "book a meeting" page reads the private calendar, so an Apps Script copies work events into it as blockers (the day job comes first). With both accounts connected the hub would see every work meeting twice. The script therefore marks its copies (`extendedProperties.private`), and the hub ignores marked copies when the source account is connected. If the work account can't be connected (e.g. the employer blocks third-party apps), the script can write the event kind into the mark instead, and the hub reads the kind from the copy.
- **Event kinds and default lights** – the light per kind can be changed in the web UI:

  | Kind | Detected via | Default |
  |---|---|---|
  | Video call | Google Meet: `conferenceData` (`hangoutsMeet`); Zoom, Teams, Webex, Jitsi: conference add-on or link in location/description | 🔴 `dnd` |
  | Focus time | `eventType: focusTime` | 🟡 `working` |
  | Out of office, timed | `eventType: outOfOffice` | 🔴 `dnd` – in the reference setup private appointments are blocked as OOO in the work calendar. If the PC is locked, the light is off anyway |
  | Out of office, all day | `eventType: outOfOffice`, all-day | day off (vacation): no working hours that day |
  | Other event | – | 🟡 `working` – e.g. a phone call on the mobile, which the companion can't see |
  | Tentative | invitation answered "maybe" or not answered yet (`responseStatus` `tentative` / `needsAction`) | 🟡 `working`, whatever the kind – or "like the kind", or ignored |
  | Ignored | working location, birthdays, other all-day events, events marked "free", declined events | – |

- **Overlapping events:** the strictest status wins (`dnd` before `working`), not the one that started last.
- **Calendar and microphone/camera complement each other:** a call known from the calendar gives the last call ahead of time; microphone/camera catch spontaneous calls and always win.
- **Focus time and out of office only exist in Google Workspace accounts** and only on primary calendars – in a private Gmail calendar there are plain events only.
- **Privacy:** location and description are only read to find video links; titles and contents are never stored or shown. Before connecting a work account, check the employer's IT policy.
- **Set-up:** [docs/google-calendar.md](docs/google-calendar.md) – OAuth client, `doormutex-hub google login <profile> <account>`, mirrored copies. `GET /api/v1/calendar` shows the health of every account.

#### Google sign-in for self-hosters

Two separate problems:

1. **Long-lived tokens.** While an OAuth app is in "Testing", refresh tokens expire after 7 days. To **publish** the app, the Google Auth Platform requires a complete branding page: homepage, privacy policy, terms of service and an authorized domain. The homepage must describe the app; a bare login page is not enough. For verification by Google you must also prove that you own the domain (Search Console). → **Without a real project homepage there is no long-term operation.**
2. **Redirect.** Every installation runs its hub under a different address, usually without public HTTPS. Web clients need an HTTPS redirect to a public domain. Desktop clients are the exception: they may redirect to loopback (`127.0.0.1`). Google does not allow the device flow ("enter a code on another device") for calendar scopes.

**Plan:**

- **Project homepage with its own domain** (GitHub Pages + custom domain, verified in Search Console): description, logo, screenshots, privacy policy, terms of service, docs and web flasher.
- **Stage 1 – for the reference build:** an own OAuth client whose branding points to the project homepage. It can then be published and the 7-day limit is gone. Until verification Google shows a warning at sign-in and caps unverified apps at 100 users.
- **Stage 2 – for everyone else:** a **shared project OAuth client**, verified by Google. `calendar.events.readonly` is a "sensitive" scope, not a "restricted" one: it needs a justification and a demo video, but no paid security assessment. After that, nobody needs their own Google Cloud console.
  - **Sign-in via the companion:** client type "desktop app", loopback redirect to `127.0.0.1`. Google explicitly does not treat the client secret of installed apps as a secret, so it may live in open-source code.
  - **Sign-in via the web UI without companion:** a **redirect relay** on the homepage, modelled on `my.home-assistant.io`. Google redirects to `https://<homepage>/oauth/callback`, and the static page forwards code and `state` to the hub address this browser stored locally before. The target never comes from URL parameters, otherwise the relay would be an open redirect. Still open: web clients need the client secret for every token refresh in the hub. To be checked before stage 2 starts.
- **Fallback without OAuth:** the calendar's secret iCal address – without `eventType` and attendees, and Google sometimes updates the feed with a long delay.

### Working hours

Weekly plan (e.g. Mon–Fri 08:00–17:00), public holidays per country (Austria built in), extra days off from the config, vacation and absence from the calendar.

### Webhooks (optional) – e.g. Claude Code

Any tool can set a claim with a TTL via `POST /api/v1/signals/<name>`. Example of a Claude Code hook in `~/.claude/settings.json`:

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "curl -fsS -m 2 -X POST \"$DOORMUTEX_URL/api/v1/signals/claude\" -H \"Authorization: Bearer $DOORMUTEX_TOKEN\" -d '{\"status\":\"working\",\"ttl\":\"30m\"}' || true"
          }
        ]
      }
    ]
  }
}
```

> Only a partly reliable work signal: sessions often stay open for days, run over SSH on servers, and long output gets read without a new prompt. Hence optional.

## Controls

### KDE Plasma widget

Implemented in [`plasmoid/`](plasmoid/) – install with `kpackagetool6`, see its README.

- **Panel:** a mini traffic light as icon (the active lamp lights up), tooltip with reason and "until".
- **Right-click menu:** "open web UI", "auto", quick overrides.
- **Popup:**
  - detected state with source ("microphone in use", "calendar: meeting until 14:30")
  - override: red / yellow / green / off for 15 min, 30 min, 1 h, 2 h, until the event ends, until end of work day, until the next automatic change, or indefinite
  - "auto" clears the override
  - pomodoro, e.g. 25/5 or 50/10 (work phase red, break green – yellow work phase as a setting in the web UI or per start in the widget)
  - day bar: today's slots as a coloured bar
- Works in the panel and on the desktop; distributed via the KDE Store ("Get New Widgets").
- **Other desktops:** the web UI can be installed as a PWA; a GNOME extension would be a nice community project.

### Stream Deck XL

A plugin based on the official Elgato SDK. On Linux it runs in [OpenDeck](https://github.com/nekename/OpenDeck), which supports Elgato SDK plugins; on Windows/macOS in the Elgato app. Alternative: a plugin for [StreamController](https://github.com/StreamController/StreamController).

Proposed layout (8 × 4) – **the left column is the traffic light:**

| | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 |
|---|---|---|---|---|---|---|---|---|
| **A** | 🔴 red | state | until 14:30 | next free | 15 min | 30 min | 1 h | ∞ |
| **B** | 🟡 yellow | reason | 🎙 mic | 📷 camera | 🍅 25 | 🍅 50 | break 5 | ⏱ 12:34 |
| **C** | 🟢 green | 🔋 72 % | | | | | | |
| **D** | auto | | | | | | | |

- The lamp keys show live which lamp is on. Pressing one sets an override (duration via a duration key or long press).
- Free keys stay available for other things.

### Web UI

<p><img src="docs/images/web-now.png" alt="Web UI: now" width="260"> <img src="docs/images/web-settings.png" alt="Web UI: settings" width="260"></p>

Served by the hub at `http://<hub>:8080/` – plain HTML/CSS/JS embedded into the binary (no build step, no CDN, strict CSP), installable as a web app (PWA). It only uses the public API with the token entered on the device; `/#token=…` sets up a phone in one tap.

- **Now:** the light (incl. blinking phases), reason, until when, what's next; override with a duration (15 min … until changed) or *auto*; e-ink message; pomodoro
- **Day:** today's planned slots as a bar and a list
- **Settings:** light per calendar event kind, warning timing (StVO or a lead time), calendar account health
- **History:** recent status changes
- **Profiles:** a selector when the token may see several profiles

Later: working hours, profiles and tokens, device health (battery, Wi-Fi, firmware).

### Phone (iPhone and Android)

No native app – so no Apple developer account and no app store. Step-by-step: [docs/apple-shortcuts.md](docs/apple-shortcuts.md).

- **Web app:** the web UI can be added to the home screen ("Add to Home Screen") and then opens like an app. Within the home network plain HTTP is enough for controlling the light; push notifications need HTTPS (on iOS from 16.4, for web apps on the home screen).
- **Apple Shortcuts:** a shortcut calls the hub API with the profile's token. That gives Siri ("traffic light red"), the Action Button and home-screen widgets – and automations such as "when the *Do Not Disturb* focus turns on → light red". The iPhone's focus mode then switches the light.

## Home Assistant & MQTT

The hub only creates entities via MQTT discovery (one device per profile, device-based discovery) – everyone builds their own automations. Entity IDs contain the profile, e.g. `sensor.doormutex_office_status` or `sensor.doormutex_teen_status`.

| Entity | Type | Content |
|---|---|---|
| `sensor.doormutex_office_status` | enum | `free`, `working`, `dnd`, `away` – attributes: reason, detail, phase, light, since, until, message |
| `event.doormutex_office_status_changed` | event | every state change (from → to, reason) – the trigger for your own automations |
| `sensor.doormutex_office_slot_start` / `…_slot_end` | timestamp | current slot |
| `sensor.doormutex_office_next_free_start` / `…_next_free_end` | timestamp | next free slot |
| `sensor.doormutex_battery` | % | battery level, plus voltage and runtime estimate (announced by the light itself) |
| `binary_sensor.doormutex_charging` | binary | charging / on USB (announced by the light itself) |
| diagnostics | – | Wi-Fi signal, firmware version, online |
| `select.doormutex_office_override` | select | `auto`, `free`, `working`, `dnd`, `away` – control from HA, e.g. for room mode or by voice via Alexa/Google Home. A status stays until `auto` – in office mode with the companion only until the next automatic change (screen locked, meeting, end of work day); without the companion (HA-only, room mode) HA or the web UI is in charge and it stays until changed |
| `text.doormutex_office_message` | text | short message for the e-ink (one line, ≤ 64 characters) |

| Topic | Content |
|---|---|
| `doormutex/<profile>/state` | retained JSON (see below), same document as `GET /api/v1/status` |
| `doormutex/<profile>/event` | `status_changed` events for the HA event entity (not retained) |
| `doormutex/<profile>/override/set` | command from HA: `auto`, `free`, `working`, `dnd`, `away` |
| `doormutex/<profile>/message/set` | command from HA: e-ink message, empty clears it |
| `doormutex/hub/online` | `online` / `offline`, retained; `offline` is the hub's Last Will → failsafe on the light |
| `doormutex/<profile>/display` | retained bitmap for the e-ink, rendered by the hub (reserved, comes with the firmware) |
| `doormutex/device/<id>/telemetry` | battery, charging, RSSI, firmware |
| `doormutex/device/<id>/cmd` | OTA, restart, identify |

```json
{
  "status": "dnd",
  "phase": "ending_soon",
  "light": "red_yellow",
  "reason": "calendar",
  "detail": "video_call",
  "since": "2026-10-28T14:00:00+01:00",
  "until": "2026-10-28T14:30:00+01:00",
  "next": { "status": "working", "reason": "work_hours", "since": "2026-10-28T14:30:00+01:00", "until": "2026-10-28T17:00:00+01:00" },
  "next_free": { "start": "2026-10-28T17:00:00+01:00", "end": "2026-10-29T08:00:00+01:00" },
  "updated": "2026-10-28T14:25:03+01:00"
}
```

`light` is one of `off`, `red`, `yellow`, `green`, `green_blink`, `red_yellow`, `yellow_blink`.

### Bring your own lamp – example

The Hue motion sensor at the top of the stairs triggers while the traffic light shows red or yellow → the stair light blinks four times in the matching colour:

```yaml
alias: Stairs – traffic light warning via Hue
mode: single
triggers:
  - trigger: state
    entity_id: binary_sensor.stairs_motion
    to: "on"
conditions:
  - condition: state
    entity_id: sensor.doormutex_office_status
    state: ["dnd", "working"]
actions:
  - variables:
      # Hue yellow looks greenish – orange reads as traffic-light yellow
      colour: "{{ 'red' if is_state('sensor.doormutex_office_status', 'dnd') else 'orange' }}"
  - action: scene.create
    data:
      scene_id: stairs_before
      snapshot_entities: [light.stair_light]
  - repeat:
      count: 4
      sequence:
        - action: light.turn_on
          target: { entity_id: light.stair_light }
          data: { color_name: "{{ colour }}", brightness_pct: 100, transition: 0 }
        - delay: { milliseconds: 600 }
        - action: light.turn_off
          target: { entity_id: light.stair_light }
          data: { transition: 0 }
        - delay: { milliseconds: 400 }
  - action: scene.turn_on
    target: { entity_id: scene.stairs_before }
```

## Hardware

```mermaid
flowchart LR
  usb["USB-C 5 V"] --> chg["Charger with power path<br/>NTC · safety timer"]
  cell["18650 · 1S1P<br/>replaceable"] --- prot["Cell protection<br/>over-charge/-discharge · short<br/>reverse polarity · PTC"]
  prot --- chg
  chg -->|SYS| uvlo["Under-voltage<br/>cut-off"]
  uvlo --> reg["Buck-boost 3.3 V"]
  reg --> mcu["ESP32-C6-MINI-1"]
  uvlo --> led["LED driver<br/>3 × MOSFET"]
  mcu -->|PWM| led
  led --> lamps["🔴 🟡 🟢<br/>1–2 LEDs per chamber"]
  mcu -->|SPI| epd["E-ink 1.54″"]
  mcu -->|I²C| gauge["Fuel gauge"]
  mcu -->|I²C| als["Light sensor"]
  usb -->|USB Serial/JTAG| mcu
```

| Block | Candidate | Why |
|---|---|---|
| MCU | ESP32-C6-MINI-1 | Wi-Fi 6 (TWT), BLE, native USB for flashing, LP core |
| Charger | BQ24074 (alternatively MCP73871) | Power path: on USB the system runs directly from the supply and the cell becomes a UPS. Plus NTC input, safety timer, input over-voltage protection |
| Cell protection | DW01A + FS8205A | over-charge, over-discharge, over-current/short |
| Reverse polarity | P-MOSFET | in case the cell is inserted the wrong way |
| Under-voltage cut-off | voltage supervisor + load switch | disconnects at ~3.0 V, protects the cell during long storage |
| 3.3 V | buck-boost, e.g. TPS63802 | stable 3.3 V over the whole cell range, also during Wi-Fi current peaks |
| Fuel gauge | MAX17048 | state of charge in % + discharge rate → runtime in HA |
| Light sensor | VEML7700 | automatic brightness, night dimming |
| LED driver | 3 × N-MOSFET, PWM via LEDC | 1–2 SMD LEDs per chamber, directly on the PCB. The LEDs sit on LP GPIOs so the LP core could blink them in deep sleep |
| LEDs | red ~625 nm, yellow/amber ~590 nm, green ~520 nm (or signal green ~505 nm) | single colours like a real traffic light; rated for 20 mA, dimmed in operation |
| E-ink | 1.54″ b/w, 200 × 200, SSD1681 (e.g. GDEY0154D67, approx. 32 × 37 mm) | partial refresh; the driver circuit sits on our PCB, the panel plugs into an FPC connector |
| USB-C | 5.1 kΩ CC resistors, ESD protection (USBLC6) | charging, permanent power, flashing |
| Buttons | boot, reset, info | info shows battery/IP; long press = setup mode |

### Battery & safety

The light runs on used 18650 cells from old vapes. That makes the protection circuitry mandatory, not optional:

| Protection against | Implementation |
|---|---|
| Over-charge | charger IC with proper CC/CV (4.2 V), plus the protection IC as a second line (~4.3 V) |
| Over-discharge | three stages: firmware shuts down at ~3.3 V and shows "battery empty" on the e-ink → hardware cut-off at ~3.0 V → protection IC at ~2.4 V |
| Over-current / short | protection IC + resettable fuse (PTC) |
| Temperature | NTC at the cell – charging only between 0 and 45 °C |
| Reverse polarity | P-MOSFET |
| Endless charging | safety timer in the charger IC |

**Cell check before use:**

- Check the wrap: vape cells often have cracks at the positive terminal – a short-circuit risk. Re-wrapping costs cents.
- No dents, bulges or corrosion.
- Don't charge cells that fell below ~2 V.
- Measure capacity and internal resistance with an analysing charger, sort out and label weak cells.
- Only one cell per device (1S1P). No parallel cells with different states of charge.

### Energy budget

An estimate – measured on the prototype:

| Consumer | Assumption | mAh/day |
|---|---|---|
| Wi-Fi connected + light sleep while you're in the office | 2–5 mA × 10 h | 20–50 |
| LEDs (one chamber, dimmed via light sensor) | 2–8 mA × 10 h | 20–80 |
| Away mode: deep sleep, poll every 2 min | 14 h | 5–20 |
| E-ink, sensors, quiescent currents | – | 1–3 |
| **Total** | | **≈ 45–150** |

With ~2,000 mAh usable capacity of a used 18650 that's **≈ 2–6 weeks** per charge. With two cells in rotation (one in the light, one in the charger) a swap takes 30 seconds.

The Wi-Fi figures are based on [Espressif's measurements](https://docs.espressif.com/projects/esp-idf/en/v6.0/esp32/api-guides/low-power-mode/low-power-mode-wifi.html): around 2–3 mA in light sleep, measured on the classic ESP32 in a shielded room. A real home network will be worse.

**Levers if that's not enough:**

- **LED brightness** is the biggest single item; a dark hallway needs little.
- **Wi-Fi 6 with TWT**, if the router supports it.
- **Longer poll interval** in away mode.
- **Plan B: a USB cable from the basement.** Thanks to the power path the light then runs on the power supply and the cell becomes a UPS – no redesign.

### Manufacturing

- **KiCad**, 2 layers (4 layers optional for a clean ground and RF).
- **JLCPCB with assembly:** parts from the LCSC catalogue, "basic parts" where possible (extended parts cost a setup fee). Note alternatives in the schematic right away.
- **Production files from CI** via KiBot: Gerbers, drill files, BOM and placement file in JLCPCB format, schematic PDF, iBOM.
- **Manual work afterwards:** plug the e-ink panel into the FPC, insert the cell. Maybe solder the 18650 holder – two THT joints any soldering iron can do.
- **Antenna:** keep the ESP32-C6 module area clear: no ground and no cell underneath.
- **Rough costs:** see [Costs](#costs).
- **First comes the breadboard prototype** from ready-made modules ([parts list](docs/breadboard.md)). The parts arrive in a few days, the PCB takes weeks. The firmware runs before the PCB exists, and the current measurements inform the schematic.

### Costs

Rough estimate before the schematic, as of 2026-10-29, incl. shipping and 20 % VAT. Reliable only with a finished BOM.

**Development up to the first PCB:**

| Item | approx. € |
|---|---|
| Breadboard stage 1 (incl. transparent PETG) | 80–105 |
| Breadboard stage 2 (battery) | 40–55 |
| Shipping (BerryBase; DigiKey free from €50) | 5–10 |
| Dark grey/black filament for the enclosure, if not at hand | 20–25 |
| **before ordering the PCB** | **≈ 150–200** |
| first JLCPCB order (5 PCBs, 2 assembled) + e-ink panel | 90–130 |
| optional: Nordic PPK2 – or an INA226 logger for ~€5 (see [parts list](docs/breadboard.md)) | 100 / 5 |

**Finished light per unit** (assembled PCB, panel, filament, screws; cell from stock):

| JLCPCB order size | PCB per unit | Light in total |
|---|---|---|
| 2 assembled (single build) | ~€45–50 | **~€60–75** |
| 5 assembled (with friends) | ~€25 | **~€40–50** |
| 30+ (group order/kit) | ~€13–15 | **~€25–35** |

At small quantities the fixed costs per order dominate: setup fee, ~$3 per "extended part", shipping, import. The parts themselves cost only ~$10–14 per board.

**Levers:**

- basic instead of extended parts where possible
- drop the fuel gauge (voltage via ADC)
- phototransistor instead of VEML7700
- **Lite variant** without battery: off-the-shelf C6 board + e-ink module + LEDs on a USB power supply, ~€25–35 plus filament, same enclosure
- **"Bring your own lamp"** via HA/Hue needs no hardware at all

## Firmware

**ESP-IDF in C.** Battery operation with a permanent connection needs automatic light sleep and fine control over Wi-Fi power save, which ESPHome can't do. There will still be an ESPHome example config for mains-powered builds.

| Mode | When | Behaviour |
|---|---|---|
| Active | you're in the office | Wi-Fi stays connected (light sleep + power save), permanent MQTT connection → a state change reaches the light in ~1 s |
| Away | PC locked | LEDs off, deep sleep, poll every 2 min (configurable). After unlocking, the light comes back on within that interval |
| Battery empty | below the cut-off threshold | "battery empty" on the e-ink, then off |
| USB | power supply connected | no power saving needed, the cell is charged or held |

- **LEDs** via LEDC hardware PWM: dimming, blinking, red+yellow. The output keeps running in light sleep (`LEDC_SLEEP_MODE_KEEP_ALIVE`; the right clock source on the C6 is still to be verified). Brightness compensates for the falling cell voltage.
- **E-ink:** shows the bitmap rendered by the hub. The "last updated" time uses partial refresh, with a regular full refresh against ghosting.
- **Failsafe:** if the Last Will reports the hub "offline" or the state is too old, yellow blinks.
- **Telemetry:** battery, charging state, RSSI, firmware. The device announces itself in HA via MQTT discovery.
- **Setup in the browser:** [ESP Web Tools](https://esphome.github.io/esp-web-tools/) flashes over USB-C (Chrome/Edge, Web Serial) and hands over the Wi-Fi credentials via Improv. Then you enter the MQTT broker on the device page.
- **OTA:** triggered via MQTT, images from GitHub Releases, automatic rollback on a failed boot.

## Enclosure

```
   ┌─────────────┐
   │  ╭───────╮  │  ← visor
   │  │  RED  │  │
   │  ╰───────╯  │
   │  ╭───────╮  │
   │  │YELLOW │  │
   │  ╰───────╯  │
   │  ╭───────╮  │
   │  │ GREEN │  │
   │  ╰───────╯  │
   ├─────────────┤
   │ e-ink sign  │  ← additional sign
   └──────┬──────┘
        USB-C
```

- **Form:** miniature traffic light with three chambers and visors, the e-ink sign below. Dark grey or black like the original.
- **Requirement:** the colour must be recognisable over **5 m** in subdued daylight (indoors, no window nearby).
- **Compact with classic proportions.** The e-ink panel sets the width: 1.54″ is approx. 32 × 37 mm, with a frame that's **~45 mm**. Every chamber is square, and so is the sign – like a real signal head with an additional sign. All dimensions approx.:

  | Part | Size (W × H × D) | Note |
  |---|---|---|
  | Chamber (3×) | 45 × 45 mm | lens Ø ~30 mm, 1–2 LEDs behind it |
  | Sign | 45 × ~50 mm | e-ink 1.54″ |
  | Roof | 45 × ~5 mm | |
  | **Enclosure total** | **~45 × 190 × 38 mm**, ~60 mm deep with visors | one PCB as the back wall, 18650 standing behind it; ~150 g PETG + 45 g cell |

- **Visibility, calculated:**
  - **Size:** a 30 mm lens at 5 m subtends ~21 arcminutes. A real 200 mm traffic light at 100 m is only ~7. Our light looks three times as large as a real one at 100 metres.
  - **Brightness:** one LED at 5 mA (~0.3–0.5 lm) behind 7 cm² of diffuser gives roughly 100–150 cd/m². That stands out clearly indoors; 20 mA is about four times brighter. One LED per chamber is enough on paper, the light sensor does the rest.
  - **Verification:** the breadboard test with 5 mm LEDs and lens test prints confirms it.
- **Printing:** at ~190 mm the enclosure fits the MK3S (250 × 210 mm) in one piece. Visors, lenses, back cover and wall mount are separate parts.
- **PCB as the back wall:** ~40 × 175 mm, the LEDs sit directly behind the chambers. No cables, no extra boards. Larger than 100 × 100 mm costs only a few dollars more at JLCPCB.
- **Mounting:** at ~200 g adhesive strips hold. On a staircase with kids, screws or cable ties are still the sturdier choice.
- **Larger variants** via parameters. With 2.13″ (approx. 59 × 29 mm) the enclosure becomes ~70 mm wide and ~240 mm tall – tight on the MK3S. With 2.9″ (approx. 79 × 37 mm) it becomes ~100 mm wide and ~365 mm tall; it must be split into chambers, and 72 mm lenses need LED rings.
- **Lenses in transparent PETG**, printed as separate flat parts, 0.8–1.2 mm thick, with a diffuser structure on the inside. Plus:
  - a white reflector cone behind the lens
  - light-tight walls so red doesn't bleed into yellow

  A lens test print with different thicknesses and patterns shows what looks best. No stronger LEDs needed.
- **Optimised for FDM and the Prusa MK3S**, i.e. simple geometry that prints easily:
  - every part has a flat side for the bed and prints **without supports**
  - overhangs at most 45°, bridges at most ~10 mm, horizontal holes as teardrops
  - chamfers instead of fillets at the bed (no elephant-foot issues)
  - wall thicknesses as multiples of the line width (0.45 mm → 1.35 / 1.8 mm)
  - tolerances as parameters, calibrated by a test print: ~0.2 mm for sliding fits, ~0.1 mm for press fits
  - screws (M2.5/M3) into printed bosses or heat-set inserts, otherwise snap fits
  - single extruder: lenses are separate parts, text and logo via colour change at a layer (M600)
- **Back:** battery compartment behind the PCB, USB-C at the bottom (charging in place from a power bank works too), window for the light sensor, buttons behind pin holes. The ESP32-C6 antenna sits at the top edge of the PCB with no cell behind it.
- **Removable wall mount** – a separate plate with
  - keyholes for screws,
  - a smooth surface for removable adhesive strips (tesa Powerstrips, Command),
  - slots for cable ties.

  The light hooks in from above (dovetail + snap) and lifts off for charging or swapping the cell. Optionally a clamp for the stair railing.
- **Parametric with OpenSCAD + BOSL2:** scale, display size, mount type and tolerances are adjustable. The files are Customizer-compatible and therefore also work with the OpenSCAD configurator on MakerWorld.
- **Iteration:**
  1. tolerance test print: hook-in, snap, lens seat, USB-C cut-out, inserts
  2. lens light test
  3. enclosure v1 → photos → adjustments
- **Tooling:** a recent OpenSCAD with the Manifold backend (a snapshot/nightly if needed) – renders orders of magnitude faster.

## Security & privacy

- Everything runs locally. The only external connection is the Google Calendar API.
- The companion only sends yes/no signals and app names, never audio or video.
- Event titles stay in the hub. E-ink, MQTT and HA only see categories such as "meeting".
- One API token per client (companion, widget, Stream Deck, webhooks), stored hashed; MQTT with user and password, TLS optional.

## Open source

```
doormutex/
├── api/          OpenAPI spec – the contract between all components
├── hub/          Go – rule engine, API, web UI, MQTT, HA discovery, e-ink rendering
├── companion/    Go – Linux daemon: PipeWire, lock, notifications, OAuth
├── plasmoid/     QML – KDE Plasma 6 widget
├── streamdeck/   TypeScript – Stream Deck plugin (Elgato SDK)
├── firmware/     C – ESP-IDF; esphome/ with an example config for mains-powered builds
├── hardware/     KiCad – schematic, layout, production files
├── enclosure/    OpenSCAD + BOSL2 – enclosure, wall mount, test prints
├── site/         project homepage (GitHub Pages) incl. web flasher
└── docs/         docs, HA examples, build guide, photos
```

- **CI/CD (GitHub Actions):**
  - hub: tests (incl. MQTT integration against a throwaway Mosquitto), lint, Docker build ✅ – later a multi-arch image (amd64/arm64) on GHCR
  - companion binaries, later an AUR package
  - widget as `.plasmoid` for the KDE Store
  - Stream Deck plugin package
  - firmware build with web flasher on GitHub Pages
  - KiBot: Gerbers, BOM/CPL for JLCPCB, schematic PDF, iBOM, 3D renders
  - OpenSCAD: STL/3MF with preview images
- **Licences** – real open source, so that the hardware can be [OSHWA certified](https://certification.oshwa.org/). Per path in [`REUSE.toml`](REUSE.toml), texts in [`LICENSES/`](LICENSES/), checked with `reuse lint`:
  - software and firmware: [Apache-2.0](LICENSES/Apache-2.0.txt)
  - hardware and enclosure: [CERN-OHL-S-2.0](LICENSES/CERN-OHL-S-2.0.txt)
  - docs and photos: [CC BY-SA 4.0](LICENSES/CC-BY-SA-4.0.txt)
- **Third-party licences:** all dependencies so far are permissive (Go modules: BSD-3, MIT/Apache-2.0, Paho under EPL-2.0/EDL-1.0; ESP-IDF: Apache-2.0; BOSL2: BSD-2; KiCad libraries: CC BY-SA with an exception for designs). Shipping binaries or devices only requires including their notices. To keep it that way: no GPL e-paper drivers (e.g. GxEPD2) in the firmware, and fonts under the OFL.
- **How it differs:** busylights (Luxafor, Kuando), PresenceLight and countless "on air" lamps mostly show "in a call yes/no" at the desk. This one is for the family: a traffic light you can see from a distance, with times, a rule engine with many sources and a failsafe, Linux first, and battery-capable open hardware.

## Roadmap

| Milestone | Content |
|---|---|
| **M0 – software core** | Hub ✅ (merged). In review as stacked PRs #3–#8: review rules, profiles + room mode, Google Calendar, web UI, companion, Plasma widget. A Hue lamp via HA stands in for the light → 1–2 weeks of family testing without new hardware |
| **M1a – breadboard** | [Prototype from ready-made modules](docs/breadboard.md): firmware, LEDs + lens test, e-ink, current measurement, then battery operation. Runs in parallel to M0 |
| **M1b – PCB + enclosure v1** | KiCad schematic + layout + review based on the measurements, JLCPCB order, tolerance test prints, enclosure v1 |
| **M2 – polish** | current measurement and optimisation (light sleep, away mode, TWT), e-ink layouts, Stream Deck plugin, web flasher, OTA |
| **M3 – release** | CI, KDE Store, Printables/MakerWorld, Docker image, OSHWA certification |

## Open questions

1. **Mounting spot:** photos of the stairs and hallway. How long is the line of sight? It drives LED brightness and enclosure size.
2. **E-ink:** 1.54″ (decided 2026-10-29). Check on the breadboard whether the text is readable from the top of the stairs.
3. **Cells:** which 18650s are they (type, age)? Is there a charger with capacity test?
4. **Router:** does it support Wi-Fi 6 with TWT (make/model)?
5. **Homelab:** is there a reverse proxy with an HTTPS domain? No longer needed for OAuth (desktop client + loopback), but needed for push notifications to the web app.
6. **Project domain:** which domain for the homepage (needed for Google OAuth branding)?
7. **Room mode without a homelab:** proposal: a standalone mode in which the light hosts its own web UI and API at `doormutex-<name>.local` (ESP-IDF HTTP server, setup via a Wi-Fi QR code on the e-ink, pairing code against siblings). Alternatives: a Home Assistant add-on or a hosted service. Decide before the firmware architecture is fixed.
8. **Warning patterns:** red/yellow alternating for "last call from yellow" and StVO timing are proposals – check on the breadboard whether they read well from the stairs.

## Risks

| Risk | Mitigation |
|---|---|
| Real battery life (home network broadcasts, LED brightness) | measure early on the dev board; plan B (USB) is built in |
| LEDC output in light sleep on the C6 | verify early; fallback: LED driver IC with its own PWM |
| Used cells | check and sort out consistently |
| False alarms from microphone detection | ignore list, "wrong detection" action in the notification |
| Google OAuth is fiddly | step-by-step guide with screenshots; iCal address as fallback |
| Parts not available at JLCPCB/LCSC | note alternatives in the schematic |
| Plugin doesn't run cleanly in OpenDeck | test early; fallback: StreamController plugin |
| Work Google Workspace blocks third-party OAuth apps | not an issue in the reference setup (the owner is the Workspace admin); for others: mirroring script with kind marks (see Google Calendar) or the iCal address |
| Selling hardware later (e.g. Kickstarter) | open licences allow it. The real effort is regulatory: CE (EMC, Radio Equipment Directive incl. its cybersecurity requirements for connected devices), RoHS, WEEE and battery registration, new certified cells (never reused ones), UN 38.3 for shipping. A USB-powered variant without battery avoids a large part of it. A DIY kit is no way around it – kits sold to consumers count as products too. What works without CE: bare PCBs, or buyers ordering assembled boards from the published files at JLCPCB themselves (built for own use) |

## Decision log

| Date | Decision | Reason |
|---|---|---|
| 2026-10-27 | Project start as a brainstorm | – |
| 2026-10-28 | One light at the top of the stairs instead of two | that's where the decision is made |
| 2026-10-28 | Wi-Fi + MQTT instead of ESP-NOW/Thread | Wi-Fi is good everywhere, broker and HA exist |
| 2026-10-28 | LEDs stay on while you're in the office; no motion sensor | visible from the hallway |
| 2026-10-28 | E-ink with absolute times instead of an LED countdown | power; no sensor needed |
| 2026-10-28 | 18650 cell + USB-C with power path | cells at hand; plan B permanent power without redesign |
| 2026-10-28 | No knock button, no dedicated control desk | WhatsApp; Stream Deck + widget |
| 2026-10-28 | HA: entities + triggers only | everyone builds their own automations |
| 2026-10-28 | Claude Code hooks only optional | unreliable as a work signal |
| 2026-10-28 | Assembled PCB (JLCPCB), enclosure in OpenSCAD | no SMD soldering possible; preference |
| 2026-10-28 | Go (hub/companion), C/ESP-IDF (firmware) | single binary + Docker; full control over power saving |
| 2026-10-29 | Lenses in transparent PETG | looks most real, little light loss |
| 2026-10-29 | Enclosure only slightly larger than the electronics, optimised for FDM | compact, easy to print, simple geometry |
| 2026-10-29 | First prototype on a breadboard, PCB afterwards | modules arrive in days, the PCB in weeks; measurements feed the schematic |
| 2026-10-29 | Google: project homepage with its own domain; first an own client, later a shared verified project client (companion loopback + redirect relay à la Home Assistant) | Google requires homepage, privacy policy and terms to publish; in Testing tokens expire after 7 days |
| 2026-10-29 | Project homepage on GitHub Pages, no backend | OAuth branding, docs, web flasher, redirect relay |
| 2026-10-29 | E-ink 1.54″ instead of 2.9″, compact light ~45 × 190 mm | 5 m recognition works with 30 mm lenses; prints in one piece, one LED per chamber, one PCB; cheaper. (Briefly considered: a large 100 × 365 mm light with 2.9″ – three print parts, LED rings, more power) |
| 2026-10-29 | Repo public from day one | people should be able to follow the project as it grows |
| 2026-10-29 | Everything on GitHub in English | open source / open hardware for an international audience |
| 2026-10-29 | Hub: SSE instead of WebSocket for live updates | one-way stream is enough, works through every proxy, trivial in QML/TypeScript/curl |
| 2026-10-29 | Hub: pure-Go SQLite (modernc), distroless static image (~26 MB) | no cgo, easy multi-arch builds |
| 2026-10-29 | Hub: API tokens created via CLI (`doormutex-hub token create`), stored as SHA-256 | no default password; tokens are 256-bit random |
| 2026-10-29 | Hub: unknown slot ends stay `null` (call without calendar entry, locked screen); a call during a calendar meeting is assumed to end with the meeting | honest instead of guessed times on the e-ink and in HA |
| 2026-10-29 | Hub: unclassified calendar events count as `dnd` by default (superseded below) | mirrored events lose attendees; red is the safer guess |
| 2026-10-29 | Hub: last call only from green, ending soon only if yellow or green follows (last call extended below) | follows the StVO meaning (red+yellow → green is coming) |
| 2026-10-29 | Hub: override and focus timer replace each other | one manual source at a time is easier to understand |
| 2026-10-29 | Hub: HA device-based discovery with `default_entity_id`; battery/charging come from the light itself | stable entity IDs; each device announces what it measures |
| 2026-10-29 | Hub: e-ink rendering not part of M0, `display` topic reserved | the display (1.54″) and firmware come later |
| 2026-10-29 | Review: microphone/camera beat everything, including a manual override | spontaneous Slack/Teams calls must turn red at once |
| 2026-10-29 | Review: locking the PC beats override and calendar → off; the override isn't cleared | a nerd only locks the PC when leaving the desk |
| 2026-10-29 | Review: the companion signs off on shutdown/suspend/lock; silence without goodbye = fault (yellow blinks) until end of work day | blinking yellow means "out of order" in the StVO; a PC that is off is normal |
| 2026-10-29 | Review: phases use StVO timing by default, a switch per phase gives a lead time (e.g. 3/5 min) | authentic by default, useful as a warning when wanted |
| 2026-10-29 | Review: last call from yellow = red/yellow alternating | the StVO has nothing for it; blinking yellow is the fault |
| 2026-10-29 | Review: several Google accounts per profile (private + work); the HubSpot mirroring script stays and marks its copies, the hub ignores them | conference data and event types arrive intact; no double events |
| 2026-10-29 | Review: event kinds → light, editable in the web UI. Defaults: video call (Meet, Zoom, Teams …) red, focus time yellow, timed OOO red, all-day OOO day off, other events yellow | private appointments are blocked as OOO in the work calendar; phone calls aren't visible to the companion |
| 2026-10-29 | Review: holidays only remove working hours; calendar events still count | holiday workers and self-employed |
| 2026-10-29 | Review: focus timer renamed to pomodoro, work phase red | avoids confusion with Google focus time (yellow) |
| 2026-10-29 | Review: companion required by default; profiles can run without it, controlled via phone, web UI or HA | the office needs the companion; other rooms don't |
| 2026-10-29 | Review: profiles and room mode (teenager, parents); phone via web app + Apple Shortcuts, no native app | no Apple developer account; voice via HA → Alexa/Google Home |
| 2026-10-29 | Goal for the licence: no commercial redistribution by others (superseded below) | keeps the option to sell it later |
| 2026-10-29 | Licence: real open source (OSI + OSHWA) after all; selling it yourself stays possible | OSHWA certification is worth more for marketing than a non-commercial clause |
| 2026-10-29 | Overlapping calendar events: the strictest status wins | the M0 engine lets the latest start win, so a yellow event could interrupt a red meeting |
| 2026-10-29 | Licences confirmed: Apache-2.0 (software, firmware), CERN-OHL-S-2.0 (hardware, enclosure), CC BY-SA 4.0 (docs); REUSE-compliant | real open source, OSHWA certification for marketing |
| 2026-10-29 | Matter over Wi-Fi as an additional interface, uncertified and without the logo | Apple Home, Google Home and Alexa without HA; certification is expensive |
| 2026-10-29 | mDNS name `doormutex[-<name>].local` | English project name, several lights per network |
| 2026-10-29 | Project renamed from the working title office-traffic-light to **doormutex** | the old name already existed on GitHub and was too narrow for room mode; a mutex lets only one into the critical section |
| 2026-10-30 | Implemented the review rules in the hub (PR m1-rules). Fault is reported as status `away` with phase `fault`; calendar kind `meeting` became `video_call` | `away` keeps HA automations simple (nobody to disturb is known); the kind name matches the README table |
| 2026-10-30 | Profiles: one service per profile, profile chosen by the token or `?profile=`; MQTT topics and HA devices per profile (`doormutex/<profile>/…`, `sensor.doormutex_<profile>_…`); HA control via `select` (indefinite override, `auto` clears) and `text` (message) | the same client works for every profile; a teenager's token can't touch the office |
| 2026-10-30 | Google Calendar: own HTTP client against the Calendar API (no Google SDK), `x/oauth2` with PKCE; scope `calendar.events.readonly`; titles are not even requested; login via loopback or pasting the redirect address; accounts fail independently (last good events are kept) | small dependency footprint, testable with `httptest`; works when the hub runs in Docker on another machine |
| 2026-10-30 | Web UI: plain HTML/CSS/JS embedded with `go:embed`, no npm, strict CSP, service worker never caches the API; runtime settings (light per event kind, phase timing) saved in the hub and override the config | nothing to build or install, nothing loaded from outside the hub; the settings belong where people change them |
| 2026-10-30 | Companion: separate Go module, polls `pw-dump` every 2 s instead of a PipeWire client library, camera also via `/proc/*/fd`, logind via D-Bus (`godbus`), heartbeat on change and every 30 s | no cgo, no PipeWire headers; `pw-dump` costs a few milliseconds; V4L2 apps (Zoom, Chrome) don't show up in PipeWire |
| 2026-10-30 | Plasma widget: plain QML with `XMLHttpRequest`, reads the SSE stream from a streaming request and polls as a fallback; token in the Plasma config | no compiled plugin, installs with `kpackagetool6`; KWallet would be nicer (open question) |
