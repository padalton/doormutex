# Breadboard prototype & parts list

The first hardware prototype is built from ready-made modules on a breadboard – no SMD soldering at all. The parts arrive within days; the PCB from JLCPCB takes weeks. What we measure here (current, brightness, pin assignment) feeds directly into the schematic.

> As of 2026-10-29. Prices are rough guidelines; check availability in the shop before ordering. Shop recommendations assume delivery to Austria.

## Two stages

| Stage | Power | Goal |
|---|---|---|
| **1 – firmware & optics** | USB-C from the PC | visibility at 5 m (one LED behind a 30 mm lens), Wi-Fi + MQTT, LEDs via LEDC PWM (also in light sleep – the biggest technical risk), e-ink with partial refresh, light sensor, lens test prints, module current measurement |
| **2 – battery** | 18650 via charger + protection | power path (plug/unplug USB while running), fuel gauge, over-discharge protection, real runtime, green LED at low cell voltage |

**Measuring:** the ESP32-C6-DevKitC-1 has jumper **J5**. Pull it and put an ammeter in its place to measure only the module current, without the board's USB chip and power LED.

**Tip for stage 1:** feed the LED rail from 3.3 V instead of 5 V. That is roughly the cell voltage just before cut-off, i.e. the worst case for the green LED (forward voltage ~2.7–3.0 V).

## Parts list

### Stage 1 – firmware & optics

| # | Part | Qty | Manufacturer no. / note | approx. € |
|---|---|---|---|---|
| 1 | **ESP32-C6-DevKitC-1** (8 MB flash) | 2 | Espressif `ESP32-C6-DEVKITC-1-N8` – one as a spare or for tests on mains power | 2 × 10 |
| 2 | **1.54″ e-paper module** b/w, 200 × 200, SPI, with driver board | 1 | Waveshare "1.54inch e-Paper Module", version **V2** (SSD1681 controller, as on the later PCB) | 12–15 |
| 3 | **VEML7700 light sensor**, STEMMA QT | 1 | Adafruit `4162` | 5 |
| 4 | **N-MOSFET**, logic level, THT | 3 (+2) | Infineon `IRLB8721PBF` (switches reliably with 3.3 V at the gate) | 5 |
| 5 | **5 mm LEDs**, bright, wide viewing angle | 10 each | red ~625 nm, yellow/amber ~590 nm, green ~520 nm – one per chamber, the rest for brightness and lens comparisons | 5 |
| 6 | **Resistor assortment** E12, ¼ W | 1 | LED series resistors (47–330 Ω) and gate pull-downs (10 kΩ) | 5–10 |
| 7 | **Push buttons** 6 × 6 mm | 5 | info button, experiments | 1 |
| 8 | **Breadboard** 830 points | 2 | the DevKit is wide – with two boards side by side, contacts stay free on both sides | 2 × 5 |
| 9 | **Jumper wires** m-m and m-f | 1 set each | | 2 × 4 |
| 10 | **STEMMA QT cable** to pin header | 2 | Adafruit `4209` – for VEML7700 and later MAX17048 | 2 × 1 |
| 11 | **USB-C data cable** | 1 | if none is lying around | – |
| 12 | **Filament: transparent PETG** | 1 spool | for the lenses | 20–30 |

### Stage 2 – battery

| # | Part | Qty | Manufacturer no. / note | approx. € |
|---|---|---|---|---|
| 13 | **Charger with power path** | 1 | Adafruit `4755` ("Universal USB/DC/Solar Lithium Ion/Polymer charger – bq24074"), the same charger IC as on the PCB | 15–20 |
| 14 | **MAX17048 fuel gauge**, STEMMA QT | 1 | Adafruit `5580` | 6–8 |
| 15 | **1S protection board** DW01A + 8205A | 2 | generic "1S 3.7 V Li-ion BMS / protection board" – the vape cells have no built-in protection | 2 × 1 |
| 16 | **18650 holder** with leads | 2 | single, not a double holder | 2 × 2 |
| 17 | **Buck-boost 3.3 V** | 1 | e.g. Pololu `S7V8F3` (2.7–11.8 V → 3.3 V). Feeds the DevKit via its 3V3 pin – then do *not* plug USB into the DevKit | 10–15 |
| 18 | **NTC 10 kΩ** (B ≈ 3435) | 1 | optional, for charge temperature monitoring on the bq24074 | 1 |

### Optional – for precise measurements

| Part | Note | approx. € |
|---|---|---|
| **INA226 module** with 0.1 Ω shunt – the budget option | Sits between supply and prototype. The second DevKit reads the current a few hundred times per second and sends it via MQTT to the homelab, e.g. to Grafana. It won't show individual Wi-Fi beacons, but averages in the mA range and mAh per day – enough for the energy budget. As a bonus it becomes a small open-source tool of its own | ~5 |
| **Nordic Power Profiler Kit II** (`NRF-PPK2`) – the deluxe option | Measures from µA to 1 A at a high sample rate: every Wi-Fi beacon, every light-sleep cycle and every e-ink refresh individually | ~100 |

**Total:** stage 1 approx. €80–105 (incl. filament), stage 2 approx. €40–55, without PPK2.

## Where to order (delivery to Austria)

| Shop | For | Why |
|---|---|---|
| **[DigiKey.at](https://www.digikey.at)** – main order | Espressif, Adafruit, MOSFETs, LEDs, resistors, breadboards, cables, PPK2 | free shipping from €50, usually there in ~2 days. With **UPS** as carrier, duties and import fees are prepaid (DDP) |
| **[BerryBase](https://www.berrybase.de)** (DE) | Waveshare e-paper; also stocks the Adafruit bq24074 | EU shipping without customs; good Waveshare selection |
| [reichelt.at](https://www.reichelt.at) | alternative for the basics (breadboard, wires, LEDs, resistors, buttons) | Austrian shop of a German distributor, fast; Espressif/Adafruit range not checked |
| Pimoroni (UK) | – | customs handling since Brexit, plus an EU flat fee per item type since July 2026. Only worth it for Pimoroni's own products |
| Mouser | – | does not ship Adafruit products to private customers in the EU |
| Generic parts (15, 16) | BerryBase, Amazon or AliExpress | cent items, available everywhere |

## Safety for the battery build (stage 2)

- The cell is **always** behind a protection board, also on the breadboard.
- Check cells beforehand (see [README → Battery & safety](../README.md#battery--safety)).
- Don't leave the first charge cycles unattended; put the build on a non-flammable surface (a tile).
- Breadboard contacts only handle ~1 A. Wire the battery path (cell → protection → charger) with terminals or soldered leads, not through the breadboard.

## Next

The pin assignment is fixed when the firmware starts. It has to respect the C6 strapping pins, and the LEDs go on LP GPIOs. The assignment then lands as a table and a Fritzing-style sketch in `firmware/`.
