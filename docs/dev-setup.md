# Entwicklungsumgebung

Alle Werkzeuge, die für Software, Firmware, Platine und Gehäuse gebraucht werden. Referenz-System ist **CachyOS/Arch mit KDE Plasma 6**. Für andere Distributionen gelten dieselben Pakete unter ähnlichen Namen.

> Stand 2026-10-29. Was schon installiert ist, überspringt `--needed` automatisch.

## Auf einen Blick

```bash
# 1. Pakete aus den offiziellen Repos
sudo pacman -S --needed \
  go golangci-lint sqlite mosquitto jq github-cli pre-commit \
  docker docker-buildx docker-compose nodejs npm \
  plasma-sdk \
  git cmake ninja ccache gperf flex bison dfu-util libusb python esptool \
  kicad kicad-library kicad-library-3d \
  prusa-slicer

# 2. Serielle Schnittstelle ohne sudo (ESP32 über USB) – danach ab- und wieder anmelden
sudo usermod -aG uucp "$USER"

# 3. ESP-IDF (fest auf ein Release, nicht master)
mkdir -p ~/esp && git clone -b v6.1 --recursive https://github.com/espressif/esp-idf.git ~/esp/esp-idf
~/esp/esp-idf/install.sh esp32c6

# 4. OpenSCAD-Entwicklerversion (Manifold-Backend) + BOSL2
paru -S openscad-git
git clone https://github.com/BelfrySCAD/BOSL2.git ~/.local/share/OpenSCAD/libraries/BOSL2

# 5. Stream Deck
flatpak install flathub me.amankhanna.opendeck
npm install -g @elgato/cli

# 6. Go-Werkzeuge (landen in ~/go/bin – das muss im PATH sein)
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc
```

## Im Detail

### Hub & Companion (Go)

| Werkzeug | Installation | Wofür |
|---|---|---|
| Go | `sudo pacman -S go` | Hub und Companion |
| golangci-lint | `sudo pacman -S golangci-lint` | Linter, läuft auch in der CI |
| oapi-codegen | `go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest` | Server- und Client-Code aus der OpenAPI-Spec in `api/` |
| SQLite | `sudo pacman -S sqlite` | `sqlite3`-CLI zum Reinschauen in die Hub-Datenbank |
| Docker + Buildx + Compose | `sudo pacman -S docker docker-buildx docker-compose` | Hub-Image bauen und lokal starten |
| Mosquitto | `sudo pacman -S mosquitto` | `mosquitto_sub`/`mosquitto_pub` zum Debuggen. Den Broker selbst nicht aktivieren – der läuft im Homelab |
| jq | `sudo pacman -S jq` | API- und MQTT-JSON lesen |

Arm64-Images (z. B. für einen Raspberry Pi im Homelab) baut die CI. Wer sie lokal bauen will, braucht zusätzlich `qemu-user-static-binfmt`.

### Plasma-Widget (QML)

| Werkzeug | Installation | Wofür |
|---|---|---|
| plasma-sdk | `sudo pacman -S plasma-sdk` | `plasmoidviewer` – Widget im eigenen Fenster testen, ohne das Panel neu zu starten |
| kpackagetool6 | ist bei Plasma dabei | Widget installieren/aktualisieren: `kpackagetool6 -t Plasma/Applet -u plasmoid/package` |

### Stream-Deck-Plugin (TypeScript)

| Werkzeug | Installation | Wofür |
|---|---|---|
| Node.js + npm | `sudo pacman -S nodejs npm` | Plugin bauen |
| Elgato CLI | `npm install -g @elgato/cli` | `streamdeck create`, `streamdeck link`, `streamdeck pack` |
| OpenDeck | `flatpak install flathub me.amankhanna.opendeck` | Stream-Deck-Software für Linux, führt Elgato-Plugins aus |

Damit das Stream Deck ohne Root erreichbar ist, braucht es eine udev-Regel. Die passende Datei und Anleitung stehen im [OpenDeck-README](https://github.com/nekename/OpenDeck). Ein Flatpak kann udev-Regeln nicht selbst installieren.

### Firmware (ESP-IDF, C)

| Werkzeug | Installation | Wofür |
|---|---|---|
| Build-Abhängigkeiten | `sudo pacman -S --needed git cmake ninja ccache gperf flex bison dfu-util libusb python` | laut Espressif-Anleitung für Arch |
| ESP-IDF v6.1 | `git clone -b v6.1 --recursive https://github.com/espressif/esp-idf.git ~/esp/esp-idf && ~/esp/esp-idf/install.sh esp32c6` | Toolchain, `idf.py` |
| esptool | `sudo pacman -S esptool` | Flashen/Auslesen ohne IDF-Umgebung |
| Gruppe `uucp` | `sudo usermod -aG uucp "$USER"` | Zugriff auf `/dev/ttyACM*` ohne sudo |

Pro Shell aktivieren: `. ~/esp/esp-idf/export.sh`. Bequemer ist ein Alias in der `~/.zshrc`, z. B. `alias get_idf='. ~/esp/esp-idf/export.sh'`.

Das AUR-Paket `esp-idf` ist derzeit als veraltet markiert, deshalb der direkte Git-Clone. Alternative: Espressifs Installations-Manager (`paru -S eim-cli`).

Wer das Nordic PPK2 zum Strommessen nutzt (siehe [Teileliste](breadboard.md)), braucht noch *nRF Connect for Desktop* mit der App *Power Profiler*. Das gibt es als AppImage bei Nordic.

### Platine (KiCad)

| Werkzeug | Installation | Wofür |
|---|---|---|
| KiCad 10 | `sudo pacman -S kicad kicad-library kicad-library-3d` | Schaltplan, Layout, 3D-Ansicht |
| Plugins | im KiCad *Plugin and Content Manager*: **Interactive HTML BOM**, **JLCPCB Tools** (o. ä.) | iBOM, LCSC-Teilenummern, Fertigungsdaten |
| KiBot | in der CI per Docker-Image; lokal optional `paru -S kibot` | Gerber, BOM/CPL, Schaltplan-PDF, Renders – automatisch |

### Gehäuse (OpenSCAD)

| Werkzeug | Installation | Wofür |
|---|---|---|
| OpenSCAD (Entwicklerversion) | `paru -S openscad-git` (kompiliert eine Weile) oder das Nightly-AppImage von [openscad.org/downloads](https://openscad.org/downloads.html#snapshots) | Die Version 2021.01 aus den Repos und vom Flathub ist zu alt: ohne Manifold-Backend rendert sie um Größenordnungen langsamer |
| BOSL2 | `git clone https://github.com/BelfrySCAD/BOSL2.git ~/.local/share/OpenSCAD/libraries/BOSL2` | Bibliothek für Fasen, Rundungen, Schwalbenschwänze, Gewinde, Textur |
| PrusaSlicer | `sudo pacman -S prusa-slicer` | Slicen für den MK3S, M600-Farbwechsel |

In OpenSCAD unter *Einstellungen → Erweitert* das Backend auf **Manifold** stellen. Die CI rendert STL/3MF kopflos per `openscad -o …` mit demselben Backend.

### Repo & Qualität

| Werkzeug | Installation | Wofür |
|---|---|---|
| GitHub CLI | `sudo pacman -S github-cli` | Issues, PRs, Releases, Actions-Logs |
| pre-commit | `sudo pacman -S pre-commit` | Formatierung und Linter vor jedem Commit (`pre-commit install` im Repo) |

## Prüfen, ob alles da ist

```bash
go version && golangci-lint version
plasmoidviewer --version
. ~/esp/esp-idf/export.sh >/dev/null && idf.py --version
kicad-cli version
openscad --version
streamdeck --version
flatpak info me.amankhanna.opendeck | grep -i version
id -nG | grep -qw uucp && echo "uucp ok" || echo "uucp fehlt – neu anmelden?"
```
