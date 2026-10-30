# Android: web app, shortcuts, tiles and automations

No native app and no Play Store: an Android phone controls a doormutex light with the hub's web app and – for one-tap buttons, Quick Settings tiles and automations – with free apps that call the hub API. Everything uses a token that is limited to one profile.

## 1. A token for the phone

```bash
docker compose exec hub doormutex-hub token create teen-phone teen
```

The token can only see and switch the profile `teen`. A lost phone: `doormutex-hub token revoke teen-phone`.

## 2. Web app on the home screen

Open `https://<hub>/#token=<token>` in Chrome. The token moves into the phone's storage and disappears from the address bar. Then *⋮ → Add to home screen → Install*.

**HTTPS is needed on Android.** Chrome only installs a web app (own window, app shortcuts, offline shell) from a secure origin. Over plain `http://<hub>:8080` the page works in the browser, and *Add to home screen* creates a bookmark – but no installed app. In a home network the cleanest way is the reverse proxy you may already run, with a certificate from a DNS challenge for an internal hostname (no port forwarding, nothing exposed to the internet):

```caddyfile
# Caddy with the DNS plugin of your DNS provider (e.g. caddy-dns/cloudflare)
doormutex.home.example.com {
	tls {
		dns cloudflare {env.CLOUDFLARE_API_TOKEN}
	}
	reverse_proxy hub-host:8080 {
		flush_interval -1 # pass the live event stream through without buffering
	}
}
```

Point `doormutex.home.example.com` to the proxy's LAN address in the router's local DNS (UniFi: *Settings → Routing → DNS*, or a host override in Pi-hole/AdGuard). The token travels only inside the home network, and the hub API is still protected by it.

**App shortcuts:** long-press the installed app's icon → **Red**, **Green**, **Automatic**. Red and green last until the next automatic change (screen locked, a meeting starts, the work day ends); the app opens briefly and sets the light. Drag a shortcut onto the home screen for a one-tap button.

## 3. Buttons and Quick Settings tiles: HTTP Shortcuts

[HTTP Shortcuts](https://http-shortcuts.rmy.ch/) is an open-source app (F-Droid and Play Store) that sends HTTP requests from home-screen icons, widgets and Quick Settings tiles.

Create one shortcut per action: *+ → Regular shortcut*:

| Field | Value |
|---|---|
| Method | `PUT` (`DELETE` for *auto*) |
| URL | `https://<hub>/api/v1/override` |
| Request headers | `Authorization` = `Bearer <token>` |
| Request body | *Custom text*, type `application/json`: `{"status":"dnd","mode":"next_change"}` |
| Response handling | *Show nothing* or a toast |

| Shortcut | Method | URL | Body |
|---|---|---|---|
| Light red | PUT | `/api/v1/override` | `{"status":"dnd","mode":"next_change"}` |
| Light red for 1 h | PUT | `/api/v1/override` | `{"status":"dnd","mode":"duration","duration":"1h"}` |
| Light green | PUT | `/api/v1/override` | `{"status":"free","mode":"next_change"}` |
| Light off | PUT | `/api/v1/override` | `{"status":"away","mode":"indefinite"}` |
| Light auto | DELETE | `/api/v1/override` | – |
| Message | PUT | `/api/v1/message` | `{"text":"{{message}}","ttl":"3h"}` with a variable *message* of type *text input* |

Without a companion (room mode) use `"mode":"indefinite"` – the light then stays until it is changed again.

- **Home screen:** long-press the home screen → *Widgets → HTTP Shortcuts*, or place single shortcuts as icons.
- **Quick Settings tile:** in a shortcut's *Trigger & execution settings* enable *Show in Quick Settings tile*, then edit the Quick Settings panel and drag the HTTP Shortcuts tile in. One swipe down, one tap: light red.
- **Store the token once:** create a global variable `token` (type *constant*, marked *sensitive*) and use `Bearer {{token}}` in every shortcut.

## 4. Automation: the phone's Do Not Disturb switches the light

With [MacroDroid](https://www.macrodroid.com/) or [Tasker](https://tasker.joaoapps.com/):

- **MacroDroid:** trigger *Device Events → Do Not Disturb / Priority Mode* (enabled) → action *Connectivity → HTTP Request* (`PUT /api/v1/override`, header and body as above). A second macro for *disabled* → `DELETE /api/v1/override`.
- **Tasker:** profile *State → Audio → Do Not Disturb* → entry task *Net → HTTP Request* (PUT as above), exit task `DELETE`. Tasker can also run an HTTP Shortcuts shortcut via its plugin.

Android's *Modes* (Bedtime, Gaming, Driving, own modes on Android 15+) all switch Do Not Disturb on, so a "Gaming" mode on the teenager's phone turns the light red and back to automatic when it ends.

## 5. Voice: Google Assistant and Google Home

There is no direct Google Home integration – that would need a cloud service. With **Home Assistant**, every profile has the entities `select.doormutex_<profile>_override` and `text.doormutex_<profile>_message`:

- Create scripts such as *Light red* (`select.select_option` → `dnd`) and *Light auto* (→ `auto`) and expose them to Google Assistant (Home Assistant Cloud or the manual Google Assistant integration). "Hey Google, activate light red" then runs the script; Google Home routines can run them too.
- The **Home Assistant Android app** offers Quick Settings tiles and home-screen widgets for scripts and entities – an alternative to HTTP Shortcuts if HA is in use anyway.

From Home Assistant, a status lasts until the next automatic change when the profile uses the companion, and until changed in room mode.
