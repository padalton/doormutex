# doormutex firmware

ESP-IDF (C) for the ESP32-C6 traffic light. **Smart hub, dumb light:** the hub publishes the state with the lamp pattern via MQTT; the light shows it, reports battery and signal, and blinks yellow by itself when it can't trust the hub any more.

> **Status: skeleton.** It builds, and the core logic is unit-tested on the host, but it hasn't run on hardware yet. That comes with the [breadboard prototype](../docs/breadboard.md). What's real and what's stubbed is listed [below](#status).

## Layout

```
firmware/
├── main/                  app_main: wiring, failsafe loop, telemetry; Kconfig (pins, timings)
├── components/
│   ├── dm_core/           pure C, no ESP-IDF APIs – host-tested
│   │                      state JSON, failsafe, lamp patterns, LED duty,
│   │                      Improv protocol, console/settings, HA discovery
│   ├── dm_lights/         LEDC PWM + blink timer
│   ├── dm_net/            Wi-Fi (power save), MQTT, SNTP
│   ├── dm_provision/      NVS settings; Improv + text console on USB
│   ├── dm_telemetry/      MAX17048 fuel gauge, charger pins
│   └── dm_eink/           SSD1681 panel – interface only
├── test/host/             unit tests (CMake + gcc, cJSON fetched)
├── esphome/               ESPHome example for mains-powered builds
├── partitions.csv         4 MB: NVS + two OTA slots
└── sdkconfig.defaults     esp32c6, light sleep, USB console
```

## Build

With ESP-IDF v5.5 installed:

```bash
cd firmware
idf.py set-target esp32c6 build
idf.py -p /dev/ttyACM0 flash monitor
```

Without installing anything except Docker – the same image CI uses:

```bash
docker run --rm -u "$(id -u):$(id -g)" -e HOME=/tmp -e IDF_COMPONENT_MANAGER=0 \
  -v "$PWD:/project" -w /project/firmware espressif/idf:v5.5.6 \
  idf.py set-target esp32c6 build
```

Flash a Docker build from the host with `esptool` (`pip install esptool`) – or in the browser, see [Setup](#setup):

```bash
# in the container: idf.py merge-bin -o doormutex-esp32c6-merged.bin  (one image at offset 0)
esptool.py --chip esp32c6 write_flash 0x0 build/doormutex-esp32c6-merged.bin
```

CI builds every change and keeps the images as an artifact (*Actions → Firmware → doormutex-firmware-esp32c6*). The app image is about 1.1 MB of the 1.9 MB slot.

**Host tests** – no ESP-IDF needed:

```bash
cmake -S firmware/test/host -B build/host && cmake --build build/host && ctest --test-dir build/host
```

They cover the RFC 3339 parser, the state document (incl. the README example and unknown patterns), the failsafe, the blink timing (four green blinks in the StVO last call), LED compensation, Improv framing between log lines, the console and the HA discovery – with AddressSanitizer and UBSan.

## Pins (breadboard defaults)

ESP32-C6-DevKitC-1. All pins are in `idf.py menuconfig → doormutex → Pins`. **To be verified on the breadboard** – the assignment lands in the schematic afterwards.

| Function | GPIO | Note |
|---|---|---|
| Red / yellow / green (MOSFET gates) | 2 / 3 / 6 | LP GPIOs, so the LP core could blink them in deep sleep later. 10 kΩ gate pull-down keeps them dark during boot |
| I²C SDA / SCL | 22 / 23 | MAX17048 fuel gauge (0x36), VEML7700 light sensor (0x10) – STEMMA QT |
| E-ink DIN / CLK / CS / DC / RST / BUSY | 18 / 19 / 20 / 21 / 10 / 11 | Waveshare 1.54″ V2 (SSD1681) |
| Charger CHG / PGOOD | – (off) | stage 2 (BQ24074): both open-drain, active low |
| USB D− / D+ | 12 / 13 | native USB: flashing, console, Improv |

Kept free: 4, 5, 8, 9, 15 (strapping; 8 is the DevKit's RGB LED, 9 the BOOT button), 0/1 (32 kHz crystal for more precise light sleep), 16/17 (UART0).

## Setup

1. **Flash and Wi-Fi in the browser:** ESP Web Tools flashes over USB (Chrome/Edge) and hands over the Wi-Fi credentials via Improv. *Next step: a manifest and a flasher page on the project homepage.* Until then, `idf.py flash` and Improv from [ESP Web Tools' test page](https://web.esphome.io/) work.
2. **MQTT and profile on the console** – the same USB port, any serial terminal (`idf.py monitor`, `picocom /dev/ttyACM0`):

   ```
   set mqtt_url mqtt://192.168.1.10:1883
   set mqtt_user doormutex-light
   set mqtt_pass ••••••••
   set profile office
   restart
   ```

   | Command | |
   |---|---|
   | `show` | settings (password masked), device ID, Wi-Fi |
   | `set KEY VALUE` | `mqtt_url` (`mqtt://`, `mqtts://`, `ws://`, `wss://`), `mqtt_user`, `mqtt_pass`, `profile`, `base_topic` (default `doormutex`), `discovery_prefix` (`homeassistant`), `ntp` (`pool.ntp.org`), `brightness` (1–100, default 40). No value = default. Applies after `restart` |
   | `identify` | lamps cycle red → yellow → green |
   | `restart`, `factory-reset` | the latter forgets settings and Wi-Fi |

   Settings live in NVS (namespace `doormutex`), Wi-Fi in the Wi-Fi driver's own NVS area.

## MQTT

The light subscribes to the hub's topics and has three of its own (`<id>` = `doormutex-` + the last three bytes of the MAC):

| Topic | Direction | Content |
|---|---|---|
| `doormutex/<profile>/state` | hub → light | retained state JSON; the light uses `light` (and `until` for the failsafe) |
| `doormutex/hub/online` | hub → light | `offline` (the hub's Last Will) → yellow blinks |
| `doormutex/<profile>/display` | hub → light | e-ink frame, 5000 bytes (200 × 200, 1 bit, rows, MSB = left) |
| `doormutex/device/<id>/telemetry` | light → | retained: `battery` (%), `voltage` (V), `charging`, `usb`, `rssi`, `light`, `fault`, `uptime`, `firmware` |
| `doormutex/device/<id>/online` | light → | `online` / `offline` (Last Will) |
| `doormutex/device/<id>/cmd` | → light | `identify`, `restart`, `ota <url>` (not implemented yet) |

**Home Assistant:** the light announces its own device (`homeassistant/device/doormutex_<mac>/config`), linked to its profile's hub device: battery, battery voltage, charging, USB power, Wi-Fi signal, light fault, and *Identify* / *Restart* buttons. Entity IDs follow the hub's scheme, e.g. `sensor.doormutex_office_battery`.

## Lamp patterns and failsafe

| `light` | Lamps |
|---|---|
| `off`, `red`, `yellow`, `green`, `red_yellow` | steady |
| `green_blink` | green 500 ms on / 500 ms off – four blinks in the StVO last call (4 s) |
| `red_yellow_alternate` | red and yellow take turns every 500 ms |
| `yellow_blink` | yellow 500 / 500 – fault |

The hub schedules the phases (StVO: green blinks 4 s, yellow 3 s, red+yellow 2 s – or a lead time) and publishes each change; the light only renders. Every pattern starts with its lit step, so a change is visible at once.

**Failsafe** – the light blinks yellow on its own when:

| Reason (`fault` in telemetry) | When |
|---|---|
| `hub_offline` | the hub's Last Will arrived |
| `disconnected` | Wi-Fi or MQTT down for more than 120 s |
| `no_state` | no state 60 s after boot (dark until then: `booting`) |
| `expired` | `until` is more than 120 s in the past – the hub publishes at every change, so it's stuck (needs a synchronised clock) |
| `unknown_light` | a pattern this firmware doesn't know (newer hub) |

The hub publishes only on change, so the state's `updated` time can be hours old – that alone is no fault.

## Power

| Mode | Behaviour | Status |
|---|---|---|
| Active | Wi-Fi modem sleep (listen interval 3 beacons), automatic light sleep, permanent MQTT; LEDC keeps running in light sleep on the RC_FAST clock (`LEDC_SLEEP_MODE_KEEP_ALIVE`) | configured, **current not measured yet** |
| Away (PC locked) | LEDs off, deep sleep, poll every 2 min | TODO |
| Battery empty | "battery empty" on the e-ink, then off (~3.3 V) | TODO |
| USB | no saving needed; the cell is charged or held | reported via PGOOD |

LED brightness: duty = brightness² (perceived brightness), scaled down by (3.0 V − V<sub>f</sub>) / (V<sub>cell</sub> − V<sub>f</sub>) above 3.0 V, so the light stays even as the cell drains (needs the fuel gauge).

**Wi-Fi 6 TWT** would let the radio sleep much longer, but needs a Wi-Fi 6 access point; with Wi-Fi 5 (e.g. UniFi AC) the beacon-based power save above is what's left.

## Status

| Part | State |
|---|---|
| State parsing, failsafe, patterns, LED duty | ✅ real, host-tested |
| LEDC driver, blink timer, identify | ✅ real, not on hardware yet |
| Wi-Fi, MQTT (Last Will, chunked messages, reconnect), SNTP | ✅ real, not on hardware yet |
| Improv serial + console + NVS settings | ✅ real (protocol host-tested), not tried with ESP Web Tools yet |
| HA discovery + telemetry | ✅ real, host-tested payloads |
| MAX17048, charger pins | ✅ real, untested |
| OTA rollback confirmation | ✅ the first state from the hub marks the image valid |
| E-ink (SSD1681) | ⏳ interface + frame check only – driver TODO (no GPL drivers, see README → Open source) |
| Light sensor (VEML7700) → brightness | ⏳ TODO |
| Away mode / deep sleep, battery-empty cut-off | ⏳ TODO |
| OTA download (`ota <url>`) | ⏳ TODO |
| Info button | ⏳ TODO |
| ESP Web Tools manifest + flasher page | ⏳ next step |

## ESPHome (mains power)

[`esphome/doormutex-mains.yaml`](esphome/doormutex-mains.yaml) does the same on any ESP32 with three LEDs – without light sleep, e-ink or battery. Same topics, same patterns, same failsafe; brightness as a number in HA.

```bash
cd firmware/esphome
cp secrets.example.yaml secrets.yaml   # fill in
esphome run doormutex-mains.yaml
```
