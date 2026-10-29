# Google Calendar

The hub reads events of one or more Google accounts per profile – for example a private and a work account – and turns them into traffic light states (see [README → Google Calendar](../README.md#google-calendar)). It only asks for the scope `calendar.events.readonly` and only requests times, event type, conference data, your own response, location/description (to find video links) and the mirror mark. Titles are never requested.

> Until the project homepage exists (README → *Google sign-in for self-hosters*), everyone uses an **own OAuth client** as described here. While that client is in "Testing", Google lets refresh tokens expire after **7 days** – then run `google login` again.

## 1. OAuth client (once)

1. Open the [Google Cloud console](https://console.cloud.google.com/), create a project, e.g. `doormutex`.
2. *APIs & Services → Library*: enable the **Google Calendar API**.
3. *Google Auth Platform → Branding*: app name `doormutex`, your e-mail as support and developer contact.
4. *Audience*: user type **External**, publishing status **Testing**; add every Google account you want to connect (private and work) as a **test user**.
5. *Data access*: add the scope `https://www.googleapis.com/auth/calendar.events.readonly`.
6. *Clients → Create client*: application type **Desktop app**. Note the client ID and the client secret.

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

```bash
# .env next to compose.yaml – never commit it
DOORMUTEX_GOOGLE_CLIENT_SECRET=GOCSPX-…
```

## 3. Sign in (per account)

```bash
docker compose exec -it hub doormutex-hub google login office private
docker compose exec -it hub doormutex-hub google login office work
```

The command prints a Google address. Open it in a browser and sign in with the matching account.

- **Hub on the same machine as the browser:** Google redirects to a local port and the command finishes on its own.
- **Hub in Docker or on a server:** the browser ends on an error page for `127.0.0.1`. That's expected – copy the full address from the browser's address bar and paste it into the terminal.

The hub picks up the token at its next calendar refresh (`calendar_poll`, default 2 minutes). Check the connection:

```bash
curl -s -H "Authorization: Bearer $TOKEN" localhost:8080/api/v1/calendar | jq
```

`doormutex-hub google logout office work` removes a token from the hub. To revoke the access at Google as well: *Google account → Security → Third-party apps*.

## What the hub does with events

| Event | Kind | Default |
|---|---|---|
| Google Meet (`hangoutLink`/`conferenceData`), conference add-on (Zoom, Teams …), or a Zoom/Teams/Webex/Jitsi/Whereby/GoTo link in location or description | `video_call` | red |
| `eventType: focusTime` | `focus` | yellow |
| `eventType: outOfOffice`, timed | `out_of_office` | red |
| `eventType: outOfOffice`, all day | day off | no working hours |
| everything else | `other` | yellow |
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
