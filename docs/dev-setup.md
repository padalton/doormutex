# Development setup

All tools needed for software, firmware, PCB and enclosure. The reference system is **CachyOS/Arch with KDE Plasma 6**; other distributions have the same packages under similar names.

> As of 2026-10-29. `--needed` skips whatever is already installed.

## At a glance

```bash
# 1. Packages from the official repos
sudo pacman -S --needed \
  go golangci-lint sqlite mosquitto jq github-cli pre-commit \
  docker docker-buildx docker-compose nodejs npm \
  plasma-sdk \
  git cmake ninja ccache gperf flex bison dfu-util libusb python esptool \
  kicad kicad-library kicad-library-3d \
  prusa-slicer

# 2. Serial port without sudo (ESP32 over USB) – log out and back in afterwards
sudo usermod -aG uucp "$USER"

# 3. ESP-IDF (pinned to a release, not master)
mkdir -p ~/esp && git clone -b v6.1 --recursive https://github.com/espressif/esp-idf.git ~/esp/esp-idf
~/esp/esp-idf/install.sh esp32c6

# 4. OpenSCAD development version (Manifold backend) + BOSL2
paru -S openscad-git
git clone https://github.com/BelfrySCAD/BOSL2.git ~/.local/share/OpenSCAD/libraries/BOSL2

# 5. Stream Deck
flatpak install flathub me.amankhanna.opendeck
npm install -g @elgato/cli

# 6. Go tools end up in ~/go/bin – put it on the PATH
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc
```

## In detail

### Hub & companion (Go)

| Tool | Install | For |
|---|---|---|
| Go | `sudo pacman -S go` | hub and companion |
| golangci-lint | `sudo pacman -S golangci-lint` | linter, also runs in CI (config in `hub/.golangci.yml`) |
| SQLite | `sudo pacman -S sqlite` | `sqlite3` CLI to look into the hub database |
| Docker + Buildx + Compose | `sudo pacman -S docker docker-buildx docker-compose` | build and run the hub image; the MQTT integration test starts a throwaway Mosquitto container |
| Mosquitto | `sudo pacman -S mosquitto` | `mosquitto_sub`/`mosquitto_pub` for debugging. Don't enable the broker itself – it runs in the homelab |
| jq | `sudo pacman -S jq` | read API and MQTT JSON |
| oapi-codegen (optional) | `go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest` | generate clients from `api/openapi.yaml`; the hub itself uses hand-written handlers plus a spec/router sync test |

Arm64 images (e.g. for a Raspberry Pi in the homelab) are built by CI. To build them locally you also need `qemu-user-static-binfmt`.

Run the hub tests including the MQTT integration test:

```bash
cd hub
DOORMUTEX_INTEGRATION=1 go test -race ./...
golangci-lint run
```

### Plasma widget (QML)

| Tool | Install | For |
|---|---|---|
| plasma-sdk | `sudo pacman -S plasma-sdk` | `plasmoidviewer` – test the widget in its own window without restarting the panel |
| kpackagetool6 | ships with Plasma | install/update the widget: `kpackagetool6 -t Plasma/Applet -u plasmoid/package` |

### Stream Deck plugin (TypeScript)

| Tool | Install | For |
|---|---|---|
| Node.js + npm | `sudo pacman -S nodejs npm` | build the plugin |
| Elgato CLI | `npm install -g @elgato/cli` | `streamdeck create`, `streamdeck link`, `streamdeck pack` |
| OpenDeck | `flatpak install flathub me.amankhanna.opendeck` | Stream Deck software for Linux, runs Elgato plugins |

The Stream Deck needs a udev rule to be accessible without root. The file and instructions are in the [OpenDeck README](https://github.com/nekename/OpenDeck); a Flatpak cannot install udev rules itself.

### Firmware (ESP-IDF, C)

| Tool | Install | For |
|---|---|---|
| Build dependencies | `sudo pacman -S --needed git cmake ninja ccache gperf flex bison dfu-util libusb python` | per Espressif's instructions for Arch |
| ESP-IDF v6.1 | `git clone -b v6.1 --recursive https://github.com/espressif/esp-idf.git ~/esp/esp-idf && ~/esp/esp-idf/install.sh esp32c6` | toolchain, `idf.py` |
| esptool | `sudo pacman -S esptool` | flash/read without the IDF environment |
| group `uucp` | `sudo usermod -aG uucp "$USER"` | access to `/dev/ttyACM*` without sudo |

Activate per shell: `. ~/esp/esp-idf/export.sh`. More convenient: an alias in `~/.zshrc`, e.g. `alias get_idf='. ~/esp/esp-idf/export.sh'`.

The AUR package `esp-idf` is currently flagged out of date, hence the direct git clone. Alternative: Espressif's installation manager (`paru -S eim-cli`).

If you measure current with the Nordic PPK2 (see [parts list](breadboard.md)), you also need *nRF Connect for Desktop* with the *Power Profiler* app, available as an AppImage from Nordic.

### PCB (KiCad)

| Tool | Install | For |
|---|---|---|
| KiCad 10 | `sudo pacman -S kicad kicad-library kicad-library-3d` | schematic, layout, 3D view |
| Plugins | in KiCad's *Plugin and Content Manager*: **Interactive HTML BOM**, **JLCPCB Tools** (or similar) | iBOM, LCSC part numbers, production files |
| KiBot | in CI via Docker image; locally optional `paru -S kibot` | Gerbers, BOM/CPL, schematic PDF, renders – automated |

### Enclosure (OpenSCAD)

| Tool | Install | For |
|---|---|---|
| OpenSCAD (development version) | `paru -S openscad-git` (compiles for a while) or the nightly AppImage from [openscad.org/downloads](https://openscad.org/downloads.html#snapshots) | version 2021.01 from the repos and Flathub is too old: without the Manifold backend it renders orders of magnitude slower |
| BOSL2 | `git clone https://github.com/BelfrySCAD/BOSL2.git ~/.local/share/OpenSCAD/libraries/BOSL2` | library for chamfers, fillets, dovetails, threads, textures |
| PrusaSlicer | `sudo pacman -S prusa-slicer` | slicing for the MK3S, M600 colour changes |

In OpenSCAD set *Preferences → Advanced* backend to **Manifold**. CI renders STL/3MF headless via `openscad -o …` with the same backend.

### Repo & quality

| Tool | Install | For |
|---|---|---|
| GitHub CLI | `sudo pacman -S github-cli` | issues, PRs, releases, Actions logs |
| pre-commit | `sudo pacman -S pre-commit` | formatting and linters before every commit (`pre-commit install` in the repo) |

## Check that everything is there

```bash
go version && golangci-lint version
plasmoidviewer --version
. ~/esp/esp-idf/export.sh >/dev/null && idf.py --version
kicad-cli version
openscad --version
npm ls -g @elgato/cli
flatpak info me.amankhanna.opendeck | grep -i version
id -nG | grep -qw uucp && echo "uucp ok" || echo "uucp missing – log in again?"
```
