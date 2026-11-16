# Google Calendar

The hub reads events of one or more Google accounts per profile – for example a private and a work account – and turns them into traffic light states (see [README → Google Calendar](../README.md#google-calendar)). It only asks for the scope `calendar.events.readonly` and only requests times, event type, conference data, your own response, location/description (to find video links) and the mirror mark. Titles are never requested.

> Until the shared project client exists (README → *Google sign-in for self-hosters*), everyone uses an **own OAuth client** as described here. While that client is in "Testing", Google lets refresh tokens expire after **7 days** – then sign in again (step 3), or publish the client (step 1, *Long-lived sign-in*).

## 1. OAuth client (once)

1. Open the [Google Cloud console](https://console.cloud.google.com/), create a project, e.g. `doormutex`.
2. *APIs & Services → Library*: enable the **Google Calendar API**.
3. *Google Auth Platform → Branding*: app name `doormutex`, your e-mail as support and developer contact.
4. *Audience*: user type **External**, publishing status **Testing**; add every Google account you want to connect (private and work) as a **test user**.
5. *Data access*: add the scope `https://www.googleapis.com/auth/calendar.events.readonly`.
6. *Clients → Create client*: application type **Desktop app**. Note the client ID and the client secret.
7. Optional, for signing in from the web UI: *Clients → Create client* once more, application type **Web application**, *Authorized redirect URIs*: `https://doormutex.org/oauth/` (the relay, see step 3). No JavaScript origins are needed. Note this client's ID and secret too. If your client is published, the relay's domain must be listed under *Branding → Authorized domains*.

**Long-lived sign-in:** a client in "Testing" gets refresh tokens that expire after 7 days. To get rid of that, publish it: *Audience → Publish app*. Google first wants a complete *Branding* page – home page, privacy policy and terms of service on a domain you have verified in Google Search Console, and that domain under *Authorized domains*. Until Google has verified the app, the sign-in shows a warning ("Google hasn't verified this app") that you click through via *Advanced*; for a handful of accounts at home that's fine. Tokens issued while the client was in "Testing" keep their 7-day limit – sign in once more after publishing.

**Work account in Google Workspace:** if the admin blocks third-party apps, the admin can trust the client ID in the *Admin console → Security → Access and data control → API controls → Manage third-party app access*.

## 2. Configuration

```yaml
google:
  client_id: 1234567890-abc.apps.googleusercontent.com
  # the secret comes from DOORMUTEX_GOOGLE_CLIENT_SECRET (or google.client_secret)

profiles:
  - id: office
    calendar:
      provider: google
      accounts:
        - name: private # any name; used in the login command
        - name: work
          calendars: [primary] # default; add shared calendars by ID if needed
```

With the web client (step 1.7) as well:

```yaml
google:
  client_id: 1234567890-abc.apps.googleusercontent.com
  web:
    client_id: 1234567890-xyz.apps.googleusercontent.com
    # relay: https://doormutex.org/oauth/ – the default
```

```bash
# .env next to compose.yaml – never commit it
DOORMUTEX_GOOGLE_CLIENT_SECRET=GOCSPX-…
DOORMUTEX_GOOGLE_WEB_CLIENT_SECRET=GOCSPX-… # only with the web client
```

Either client alone is enough: without the desktop client there is no sign-in through the companion or the command line, and without the web client there are no sign-in buttons in the web UI. A token keeps working only with the client that issued it, so don't delete a client while accounts still use it – sign them in again with the other one first.

## 3. Sign in (per account)

### In the web UI

With the web client configured, *Settings → Calendar accounts* has a **Sign in** (or **Sign in again**) button per account. It works from any browser that can reach the hub, including a phone, and the hub doesn't have to be reachable from the internet:

1. The hub creates Google's sign-in URL with the relay `https://doormutex.org/oauth/` as redirect.
2. The relay page remembers in this browser which hub the sign-in came from. The first time, it asks you to confirm the hub's address. Then it continues to Google.
3. After the sign-in, Google redirects to the relay page. It checks that the sign-in was started in this browser and sends the browser back to the hub's web UI with `code` and `state` in the address fragment. The web UI hands them to the hub, and the hub exchanges them.

The relay is a static page that runs in your browser. doormutex.org has no server that sees your hub or your tokens. The code passing through is worthless on its own: the exchange also needs the PKCE verifier and the client secret, and both never leave the hub. To use your own relay, host the page from the [website repository](https://github.com/padalton/doormutex-site) yourself and set `google.web.relay`.

### With the companion

On the desktop with the [companion](../companion/README.md) installed and configured (hub address and the profile's token):

```bash
doormutex-companion google login private
doormutex-companion google login work
```

The browser opens with Google's sign-in; choose the matching account and allow read access to the calendar. Google then redirects the browser to the companion on `127.0.0.1`, the companion hands the one-time code to the hub, and the hub stores the token – nothing to copy or paste. The client secret and the refresh token never leave the hub, and the hub reads the new account right away.

How it works: the companion listens on a random loopback port and asks the hub (`POST /api/v1/calendar/accounts/{account}/login`) for Google's sign-in URL with that address as redirect. The hub keeps the PKCE verifier; the companion passes `code` and `state` back (`…/login/complete`) and the hub exchanges them. A started sign-in is valid for 10 minutes and works once. A profile-scoped token can only sign in its own profile's accounts.

### Without a desktop (headless hub)

```bash
docker compose exec -it hub doormutex-hub google login office private
```

The command prints a Google address. Open it in any browser and sign in. If the browser runs on the hub's machine, the command finishes on its own; otherwise the browser ends on an error page for `127.0.0.1` – copy the full address from the address bar and paste it into the terminal.

### Check

When an account can't be read, the day view of the web UI shows a warning, and `GET /api/v1/timeline` carries `calendar_problem` (`login_expired`, `not_connected` or `unavailable`) – also for read-only tokens such as the Android app's. Without it, a missing calendar just looks like a free day. Details in the web UI (*Settings → Calendar accounts*) or:

```bash
curl -s -H "Authorization: Bearer $TOKEN" localhost:8080/api/v1/calendar | jq
```

`doormutex-hub google logout office work` removes a token from the hub. To revoke the access at Google as well: *Google account → Security → Third-party apps*.

## Token storage

Refresh tokens are stored **encrypted** (AES-256-GCM) in the hub's database. The key comes from

- `DOORMUTEX_SECRET_KEY` – 32 bytes, base64 (`openssl rand -base64 32`), e.g. from an `.env` file or a Docker secret; or
- `secret.key` next to the database (`/data/secret.key` in Docker), created with mode `0600` on first start.

The encryption only helps if the key isn't stored together with the database: with the key file, **keep `secret.key` out of the backups of `hub.db`** (or back it up separately), or use `DOORMUTEX_SECRET_KEY`. Losing the key only means signing in again. Tokens stored by an older hub without encryption are encrypted on the next start.

## What the hub does with events

| Event | Kind | Default |
|---|---|---|
| Google Meet (`hangoutLink`/`conferenceData`), conference add-on (Zoom, Teams …), or a Zoom/Teams/Webex/Jitsi/Whereby/GoTo link in location or description | `video_call` | red |
| `eventType: focusTime` | `focus` | yellow |
| `eventType: outOfOffice`, timed | `out_of_office` | red |
| `eventType: outOfOffice`, all day | day off | no working hours |
| everything else | `other` | yellow |
| answered "maybe" or not answered yet (`tentative`, `needsAction`) | its kind, marked tentative | yellow – setting *Tentative* in the web UI: a status, "like the event kind" or ignore |
| cancelled, declined, working location, birthdays, other all-day events, "free" (transparent) events | ignored | – |

Focus time and out of office only exist in Workspace accounts.

## Mirrored copies (Apps Script)

If a script copies work events into the private calendar as blockers (e.g. so that a booking page such as HubSpot sees them), the hub would count every work meeting twice once both accounts are connected. Let the script mark its copies with the private extended property `doormutex`:

```javascript
// CalendarApp
const copy = privateCal.createEvent('Blocker', start, end);
copy.setTag('doormutex', '1');

// or the Advanced Calendar service
Calendar.Events.insert({
  summary: 'Blocker',
  start: { dateTime: start.toISOString() },
  end: { dateTime: end.toISOString() },
  extendedProperties: { private: { doormutex: '1' } },
}, 'primary');
```

- **Both accounts connected (default `mirror: ignore`):** the hub skips the marked copies and reads the originals from the work account.
- **Work account can't be connected** (`mirror: use` on the private account): let the script write the kind into the mark – `video_call`, `focus`, `out_of_office` or `other` – and the hub takes the kind from the copy. A mark that isn't a kind (`'1'`) is classified like any other event.

The key can be changed with `google.mirror_key`.
