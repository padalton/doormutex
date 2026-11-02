# Breadboard prototype & parts list

The first hardware prototype is built from ready-made modules on a breadboard – no SMD soldering at all. The parts arrive within days; the PCB from JLCPCB takes weeks. What we measure here (current, brightness, pin assignment) feeds directly into the schematic.

> Parts as of 2026-10-29, shops and links checked on 2026-11-02. Prices are guidelines; check availability before ordering. Shop recommendations assume delivery to Austria.

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
| 2 | **1.54″ e-paper module** b/w, 200 × 200, SPI, with driver board | 1 | Waveshare "1.54inch e-Paper Module", version **V2** / V2.1, SKU **12955** (12561 is the bare panel without driver board) (SSD1681 controller, as on the later PCB) | 12–15 |
| 3 | **VEML7700 light sensor**, STEMMA QT | 1 | Adafruit `4162` | 5 |
| 4 | **N-MOSFET**, logic level, THT | 3 (+2) | Infineon `IRLB3813PBF` (V<sub>GS(th)</sub> ≤ 2.35 V, fully on at 3.3 V for LED currents). The `IRLB8721PBF` named earlier is obsolete; `IRLB4132PBF` or `IRLZ44NPBF` work as well | 5–12 |
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

Checked on 2026-11-02: every link below opened the right product, stock and prices as of that day (incl. 20 % VAT unless noted). Stock changes quickly; Adafruit boards in particular are often sold out at the smaller shops. Links are plain product links – the project uses no affiliate links.

**Four orders cover stages 1 + 2 and the enclosure:**

| Shop | What | approx. € | Shipping to AT |
|---|---|---|---|
| **DigiKey** | all electronics except the items below | 102 | free from €75, ~2 days, duties and VAT prepaid (DDP) |
| **Opencircuit** (NL) | buck-boost, 18650 holders, protection boards | 26 | €11.50, free from €150 |
| **Amazon.de** | e-paper module (sold by Waveshare), magnets, inserts, screws, silicone sheet, velcro | 63 | per seller |
| Prusa (CZ) | PETG clear + black, if not at hand | 53 | varies (check the cart) |

Why not one shop: BerryBase had none of the Adafruit boards, the e-paper module or the cell holder in stock; DigiKey has no Waveshare displays and no cell protection boards, and the Pololu regulator there is a US marketplace item (customs on delivery, doesn't count towards free shipping).

### DigiKey

| # | Part | DigiKey no. | Qty | € each excl. VAT |
|---|---|---|---|---|
| 1 | [ESP32-C6-DevKitC-1-N8](https://www.digikey.at/de/products/detail/espressif-systems/ESP32-C6-DEVKITC-1-N8/17728861) | `1965-ESP32-C6-DEVKITC-1-N8-ND` | 2 | 7.99 |
| 3 | [VEML7700, Adafruit 4162](https://www.digikey.at/de/products/detail/adafruit-industries-llc/4162/9997696) | `1528-2891-ND` | 1 | 4.39 |
| 4 | [IRLB3813PBF](https://www.digikey.at/de/products/detail/infineon-technologies/IRLB3813PBF/2118485) | `IRLB3813PBF-ND` | 5 | 2.49 |
| 5 | [LED red 623 nm, Würth 151053RS03000](https://www.digikey.at/de/products/result?keywords=151053RS03000) | `732-5021-ND` | 10 | 0.20 |
| 5 | [LED yellow 590 nm, Würth 151053YS04500](https://www.digikey.at/de/products/result?keywords=151053YS04500) | `732-5022-ND` | 10 | 0.20 |
| 5 | [LED green 522 nm, Würth 151053GS03000](https://www.digikey.at/de/products/result?keywords=151053GS03000) | `732-5020-ND` | 10 | 0.20 |
| 6 | [Resistor kit, SparkFun 10969](https://www.digikey.at/de/products/result?keywords=sparkfun%2010969) (20 values × 25, incl. 47–330 Ω and 10 kΩ) | `1568-10969-ND` | 1 | 9.32 |
| 7 | [Push button 6 × 6 mm, TE 1825910-6](https://www.digikey.at/de/products/detail/te-connectivity-alcoswitch-switches/1825910-6/1632536) | `450-1650-ND` | 5 | 0.12 |
| 8 | [Breadboard 830, Adafruit 239](https://www.digikey.at/de/products/detail/adafruit-industries-llc/239/7244929) | `1528-239-ND` | 2 | 5.28 |
| 9 | [Jumper wires m-m, Adafruit 1957](https://www.digikey.at/de/products/detail/adafruit-industries-llc/1957/6827090) | `1528-1967-ND` | 1 | 1.73 |
| 9 | [Jumper wires m-f, Adafruit 1954](https://www.digikey.at/de/products/detail/adafruit-industries-llc/1954/6827087) | `1528-1964-ND` | 1 | 1.73 |
| 10 | [STEMMA QT to pins, Adafruit 4209](https://www.digikey.at/de/products/detail/adafruit-industries-llc/4209/10230003) | `1528-4209-ND` | 2 | 0.84 |
| 13 | [bq24074 charger, Adafruit 4755](https://www.digikey.at/de/products/detail/adafruit-industries-llc/4755/13231325) | `1528-4755-ND` | 1 | 13.27 |
| 14 | [MAX17048, Adafruit 5580](https://www.digikey.at/de/products/detail/adafruit-industries-llc/5580/16719287) | `1528-5580-ND` | 1 | 5.28 |
| 18 | [NTC 10 kΩ B 3435, Vishay NTCLE350E4103FLB0](https://www.digikey.at/de/products/detail/vishay-beyschlag-draloric-bc-components/NTCLE350E4103FLB0/12612895) | `56-NTCLE350E4103FLB0-ND` | 2 | 0.96 |

≈ €85 excl. / €102 incl. VAT. Stage 1 alone stays below the free-shipping threshold, so order both stages together. The LEDs are water-clear with a 30–45° beam – bright and directional, the diffuser lens spreads the light.

**All at once:** cart → *Mehrere hinzufügen* (bulk add), one line per part as `quantity, DigiKey no.` (if the form expects the columns the other way round, swap them):

```
2, 1965-ESP32-C6-DEVKITC-1-N8-ND
1, 1528-2891-ND
5, IRLB3813PBF-ND
10, 732-5021-ND
10, 732-5022-ND
10, 732-5020-ND
1, 1568-10969-ND
5, 450-1650-ND
2, 1528-239-ND
1, 1528-1967-ND
1, 1528-1964-ND
2, 1528-4209-ND
1, 1528-4755-ND
1, 1528-5580-ND
2, 56-NTCLE350E4103FLB0-ND
```

Or try the FastAdd link (fills the cart in one click; not tested from a browser yet): [add all to the DigiKey cart](https://www.digikey.at/classic/ordering/fastadd.aspx?part1=1965-ESP32-C6-DEVKITC-1-N8-ND&qty1=2&part2=1528-2891-ND&qty2=1&part3=IRLB3813PBF-ND&qty3=5&part4=732-5021-ND&qty4=10&part5=732-5022-ND&qty5=10&part6=732-5020-ND&qty6=10&part7=1568-10969-ND&qty7=1&part8=450-1650-ND&qty8=5&part9=1528-239-ND&qty9=2&part10=1528-1967-ND&qty10=1&part11=1528-1964-ND&qty11=1&part12=1528-4209-ND&qty12=2&part13=1528-4755-ND&qty13=1&part14=1528-5580-ND&qty14=1&part15=56-NTCLE350E4103FLB0-ND&qty15=2)

### Opencircuit

| # | Part | Qty | € each |
|---|---|---|---|
| 17 | [Pololu S7V8F3 buck-boost 3.3 V (#2122)](https://opencircuit.shop/product/3.3V-Step-Up-Step-Down-Voltage-Regulator-2) | 1 | 15.10 |
| 16 | [SparkFun holder for one 18650, 15 cm leads (PRT-12899)](https://opencircuit.shop/product/battery-holder-1x18650-wire-leads) | 2 | 4.75 |
| 15 | [1S protection board 3.7 V](https://opencircuit.shop/product/3.7V-lipo-batterij-protection-board) (cuts off at 4.25 V / 2.45 V, ≤ 2 A) | 2 | 0.45 |

The Pololu regulator and the holders come from an external warehouse (5–7 days). Wire the protection board between cell and the bq24074's battery input: cell on `B+`/`B−`, the charger on `P+`/`P−` (the pad names vary by board).

### Amazon.de

| Part | Pack | € |
|---|---|---|
| [Waveshare 1.54″ e-paper module V2 (sold by Waveshare)](https://www.amazon.de/dp/B0728BJTZC) | 1 | 18.14 |
| [Disc magnets Ø 12 × 3 mm, N45](https://www.amazon.de/dp/B00PEAINV4) | 20 | 13.01 |
| [Heat-set inserts ruthex M3 × 5.7](https://www.amazon.de/dp/B08BCRZZS3) | 100 | 9.06 |
| [Screw box M2/M2.5/M3, countersunk hex socket](https://www.amazon.de/dp/B0DQTTBCB5) – M3 × 8 and × 10; M2 and M2.5 start at 6 mm, which still fits the standoffs | 874 | 7.86 |
| [Self-adhesive silicone sheet 1 mm, 3 × 20 × 20 cm](https://www.amazon.de/dp/B0F59PF6JK) | 3 | 8.05 |
| [Velcro straps 12 mm](https://www.amazon.de/dp/B077BCT9M1) | 30 | 7.04 |

Alternatives: magnets from [supermagnete.at](https://www.supermagnete.at/scheibenmagnete-neodym/scheibenmagnet-12mm-3mm_S-12-03-N) (10 for €5.40, €20 minimum order); the e-paper module from [OpenElab](https://openelab.io/de/products/waveshare-54inch-e-ink-display) (€11.50, may ship from China).

### Filament (Prusa)

[Prusament PETG Clear](https://www.prusa3d.com/product/prusament-petg-clear-1kg/) for the lenses and [Prusament PETG Jet Black](https://www.prusa3d.com/product/prusament-petg-jet-black-1kg/) for the shell, 1 kg each (€32.99, on sale at the time of checking).

### Optional – measuring

| Part | Where | € |
|---|---|---|
| Nordic PPK2 | [DigiKey `1490-NRF-PPK2-ND`](https://www.digikey.at/de/products/result?keywords=NRF-PPK2) | 114 |
| Current sensor for the budget logger | [Adafruit INA228, DigiKey `1528-5832-ND`](https://www.digikey.at/de/products/result?keywords=adafruit%20INA228) (no bare INA226 board there; the INA228 is the more precise successor) | 16 |

### Shops that didn't make it

| Shop | Why not |
|---|---|
| Botland (PL) | sells to private customers only in Poland and Germany; elsewhere in the EU only to businesses |
| BerryBase (DE) | Adafruit boards, e-paper module and cell holder sold out at the time of checking; good for basics (breadboards, LEDs, resistors) if you order there anyway – €9.90 to AT, free from €200 |
| Kiwi Electronics (NL) | fallback for the bq24074 (Adafruit 4755) if DigiKey runs out |
| Mouser | doesn't ship Adafruit products to private customers in the EU |
| Pimoroni (UK) | customs since Brexit |

## Safety for the battery build (stage 2)

- The cell is **always** behind a protection board, also on the breadboard.
- Check cells beforehand (see [README → Battery & safety](../README.md#battery--safety)).
- Don't leave the first charge cycles unattended; put the build on a non-flammable surface (a tile).
- Breadboard contacts only handle ~1 A. Wire the battery path (cell → protection → charger) with terminals or soldered leads, not through the breadboard.

## Next

The pin assignment is fixed when the firmware starts. It has to respect the C6 strapping pins, and the LEDs go on LP GPIOs. The assignment then lands as a table and a Fritzing-style sketch in `firmware/`.
