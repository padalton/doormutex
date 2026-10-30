# iPhone: web app and Apple Shortcuts

No native app, no App Store, no Apple developer account: an iPhone controls a doormutex light with the hub's web app and with Apple's *Shortcuts* app. Both talk to the hub API with a token.

## 1. A token for the phone

Create a token for the profile the phone may switch – it can't touch any other profile:

```bash
docker compose exec hub doormutex-hub token create teen-phone teen
```

## 2. Web app on the home screen

1. On the phone, open `http://<hub>:8080/#token=<token>` in Safari. The token moves into the phone's storage and disappears from the address bar.
2. *Share → Add to Home Screen.* The web app now opens full screen like an app.

It shows the light, the reason and until when; it sets red/yellow/green/off for a time, shows a message on the e-ink and starts a pomodoro. *Settings* changes the light per calendar event kind and the warning timing.

Within the home network plain HTTP is enough. Push notifications to the web app would need HTTPS (iOS 16.4 or later).

## 3. Shortcuts

Create one shortcut per action in the *Shortcuts* app:

1. **+** → *Add Action* → **Get Contents of URL**.
2. URL: `http://<hub>:8080/api/v1/override`
3. *Show More*:
   - Method: **PUT**
   - Headers: `Authorization` = `Bearer <token>`
   - Request Body: **JSON** with `status` = `dnd` and `mode` = `indefinite`
4. Name the shortcut, e.g. **Light red**. Siri now runs it with "Hey Siri, light red".

| Shortcut | Method | URL | JSON body |
|---|---|---|---|
| Light red | PUT | `/api/v1/override` | `status: dnd`, `mode: indefinite` |
| Light red for 1 h | PUT | `/api/v1/override` | `status: dnd`, `mode: duration`, `duration: 1h` |
| Light green | PUT | `/api/v1/override` | `status: free`, `mode: indefinite` |
| Light off | PUT | `/api/v1/override` | `status: away`, `mode: indefinite` |
| Light auto | DELETE | `/api/v1/override` | – |
| Message | PUT | `/api/v1/message` | `text:` *Ask Each Time*, `ttl: 3h` |

**Action Button** (iPhone 15 Pro and later): *Settings → Action Button → Shortcut* → pick *Light red*.

**Home screen widget:** long-press the home screen → **+** → *Shortcuts* → choose the shortcuts.

## 4. Automation: the iPhone's focus switches the light

*Shortcuts → Automation → **+** → Focus*:

- **Do Not Disturb** (or a custom focus such as "Gaming") – *When Turning On* → *Run Immediately* → run **Light red**.
- Same focus – *When Turning Off* → run **Light auto**.

The light now follows the phone's focus mode – no app, no companion.

## Room mode by voice (Alexa, Google Home)

With Home Assistant, every profile also has the entities `select.doormutex_<profile>_override` and `text.doormutex_<profile>_message`. Expose a script or scene that sets the select to Alexa or Google Home, and "Alexa, turn on do not disturb" switches the light.
