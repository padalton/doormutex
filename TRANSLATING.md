# Translating doormutex

doormutex is written in English; German is maintained alongside. Every other language is welcome – a translation is a plain text file, no programming needed. Missing texts fall back to English, so a partial translation already helps.

| Part | Files | Format |
|---|---|---|
| Web app (phone, browser) | [`hub/internal/web/static/i18n/`](hub/internal/web/static/i18n/) | JSON: `en.json` is the source, one file per language, listed in `languages.json` |
| E-ink sign on the light (rendered by the hub) | [`hub/internal/eink/i18n/`](hub/internal/eink/i18n/) | JSON, same style; keep the texts short – the sign is 200 pixels wide. Also add the language to `EINK_LANGS` in the web app's `app.js` and to `eink_language` in `api/openapi.yaml`, and render the test images (`go test ./internal/eink -update`) to check that everything fits |
| Desktop companion (notifications, terminal) | [`companion/internal/i18n/`](companion/internal/i18n/) | JSON, same style; the language comes from `LANGUAGE` / `LC_ALL` / `LC_MESSAGES` / `LANG` |
| KDE Plasma widget | [`plasmoid/translate/`](plasmoid/translate/) | gettext: `template.pot` is the source, one `<lang>.po` per language |

Use the language code without region (`fr`, `it`, `pt`) unless the region really matters (`pt-BR` → web `pt-BR.json`, widget `pt_BR.po`).

## Add a language

**Web app and companion (JSON):**

1. Copy `en.json` to `<lang>.json` and translate the values – never the keys.
2. Keep placeholders such as `{time}` or `{status}` exactly as they are; they may move within the sentence.
3. Web app only: add the language to `languages.json` with its own name, e.g. `"fr": "Français"`, and add the file to the `SHELL` list in `static/sw.js` so it works offline.

**Plasma widget (gettext):**

1. `cp plasmoid/translate/template.pot plasmoid/translate/<lang>.po`, set `Language: <lang>` in the header.
2. Translate with any `.po` editor (Lokalize, Poedit) or a text editor. Keep `%1` placeholders.
3. Run `plasmoid/translate/update.sh` (needs gettext) – it compiles the translation into the widget package. Commit the `.po` and the generated `.mo`.

Then open a pull request. The tests check that every catalog is valid, only uses known keys and keeps the placeholders (`go test ./...` in `hub` and `companion`); CI checks that the widget's `.mo` files are up to date.

## Changing English texts (developers)

- **Web app / companion:** add the key to `en.json` (and `de.json`); other languages fall back to English until translated. `TestUsedKeysExist` fails if the page or script uses a key that `en.json` lacks.
- **Widget:** wrap texts in `i18n("…")`, then run `plasmoid/translate/update.sh` to refresh `template.pot` and merge it into every `.po`; update `de.po`.

## Translation platform

The formats are what [Weblate](https://weblate.org) understands directly (JSON key-value and gettext `.po`). Once there are outside translators, the project can be connected to [Hosted Weblate](https://hosted.weblate.org) (free for libre projects): translations are then made in the browser and arrive as pull requests. Until then, pull requests with the files above are the way to go.
