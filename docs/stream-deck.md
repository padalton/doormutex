# Stream Deck on Linux

The README plans a Stream Deck plugin based on the official Elgato SDK. This page compares the ways to get there on Linux and describes what works **today** without a plugin.

## Today: "run command" keys

Since the companion has [command-line controls](../companion/README.md#controls-from-the-command-line), any Stream Deck software that can run a command can switch the light – no token handling per key:

| Key | Command |
|---|---|
| 🔴 red | `doormutex-companion set red` |
| 🔴 30 min | `doormutex-companion set red 30m` |
| 🟡 yellow | `doormutex-companion set yellow` |
| 🟢 green | `doormutex-companion set green` |
| ⚫ off | `doormutex-companion set off` |
| auto | `doormutex-companion set auto` |
| 🍅 25 | `doormutex-companion pomodoro 25` |
| 🍅 stop | `doormutex-companion pomodoro stop` |

What these keys can't do: show the **live state** on the key (which lamp is on, until when, mic/cam). That needs a real plugin.

## Options for a real plugin

| | [OpenDeck](https://github.com/nekename/OpenDeck) | [StreamController](https://github.com/StreamController/StreamController) |
|---|---|---|
| Plugin format | Elgato SDK plugins (JavaScript/TypeScript, also the official SDK) | own Python plugin API |
| Platforms | Linux, Windows, macOS – and the same plugin runs in Elgato's app on Windows/macOS | Linux only |
| Live keys (images/titles updated from code) | yes (SDK `setImage`/`setTitle`) | yes |
| Distribution | OpenDeck plugin store / Elgato Marketplace | StreamController store |
| Fit with this project | one codebase for every OS, matches the README plan (`streamdeck/`, TypeScript) | nice Linux UI, but a second codebase if Windows/macOS users want it too |

**Proposal:** keep the README plan – an Elgato-SDK plugin in TypeScript under `streamdeck/`, tested in OpenDeck on Linux. It follows the hub's event stream (SSE) for live keys and uses the same API token model as the widget. Until it exists, the run-command keys above cover switching.

**Open question:** which Stream Deck software do you use on CachyOS today (OpenDeck, StreamController, something else)? If it's StreamController and you never need Windows/macOS, a StreamController plugin would be quicker to get running.
