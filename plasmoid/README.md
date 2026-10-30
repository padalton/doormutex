# Plasma widget

A KDE Plasma 6 widget for the panel or the desktop: a mini traffic light as the icon, the state with reason and "until" in the tooltip, and a popup to override the light.

- **Panel icon:** the active lamp lights up, blinking phases blink. Right-click: red/green for 30 min, automatic, open web UI.
- **Popup:** state, reason, until when, next free slot, e-ink message; override red/yellow/green/off for 15 min … until the next automatic change / until changed, *Auto*; pomodoro 25/5 and 50/10 (red, or yellow with the *yellow* button or the widget setting); message for the e-ink; today's plan as a coloured bar.
- **Live:** follows the hub's event stream (Server-Sent Events); while the stream is down it polls every 15 s.
- **Languages:** English and German (Plasma's language setting). Translations live in [`translate/`](translate/) – see [TRANSLATING.md](../TRANSLATING.md); `translate/update.sh` extracts the strings and compiles the `.po` files into `package/contents/locale/`.

## Install

- **KDE Store:** *Add Widgets → Get New Widgets → Download New Plasma Widgets*, search for *doormutex*.
- **With the companion package** (AUR `doormutex-companion-bin`, `.deb`, `.rpm`): the widget is installed system-wide to `/usr/share/plasma/plasmoids/` – nothing else to do.
- **Release file:** `doormutex-X.Y.Z.plasmoid` from the [releases](https://github.com/padalton/doormutex/releases):

  ```bash
  kpackagetool6 -t Plasma/Applet -i doormutex-X.Y.Z.plasmoid   # first time
  kpackagetool6 -t Plasma/Applet -u doormutex-X.Y.Z.plasmoid   # update
  ```

- **From the repository:**

  ```bash
  kpackagetool6 -t Plasma/Applet -i plasmoid/package    # first time
  kpackagetool6 -t Plasma/Applet -u plasmoid/package    # update
  ```

A copy installed for the user (`kpackagetool6`, KDE Store) takes precedence over the system-wide one from a package; remove it with `kpackagetool6 -t Plasma/Applet -r org.doormutex.trafficlight`.

Then *Add Widgets → doormutex*, and in its settings enter the hub address and a token:

```bash
doormutex-hub token create widget office
```

The token is stored in the Plasma configuration of your user (`~/.config/plasma-org.kde.plasma.desktop-appletsrc`).

## Develop

```bash
plasmoidviewer -a plasmoid/package
/usr/lib/qt6/bin/qmllint -I /usr/lib/qt6/qml plasmoid/package/contents/ui/*.qml
```

CI packs `doormutex.plasmoid` (a zip of `package/`) as a build artifact. Releases attach `doormutex-X.Y.Z.plasmoid` with the version stamped into `metadata.json` (`packaging/prepare.py`) – the file the KDE Store expects.
