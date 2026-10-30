# Plasma widget

A KDE Plasma 6 widget for the panel or the desktop: a mini traffic light as the icon, the state with reason and "until" in the tooltip, and a popup to override the light.

- **Panel icon:** the active lamp lights up, blinking phases blink. Right-click: red/green for 30 min, automatic, open web UI.
- **Popup:** state, reason, until when, next free slot, e-ink message; override red/yellow/green/off for 15 min … until the next automatic change / until changed, *Auto*; pomodoro 25/5 and 50/10 (red, or yellow with the *yellow* button or the widget setting); message for the e-ink; today's plan as a coloured bar.
- **Live:** follows the hub's event stream (Server-Sent Events); while the stream is down it polls every 15 s.

## Install

```bash
kpackagetool6 -t Plasma/Applet -i plasmoid/package    # first time
kpackagetool6 -t Plasma/Applet -u plasmoid/package    # update
```

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

CI packs `doormutex.plasmoid` (a zip of `package/`) as a build artifact – the file the KDE Store expects.
