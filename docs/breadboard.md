# Breadboard-Prototyp & Teileliste

Der erste Hardware-Prototyp entsteht aus fertigen Modulen auf dem Breadboard – ganz ohne SMD-Löten. Die Teile sind in wenigen Tagen da, die Platine von JLCPCB dauert Wochen. Was wir hier messen (Strom, Helligkeit, Pinbelegung), fließt direkt in den Schaltplan.

> Stand 2026-10-29. Preise sind grobe Richtwerte; Verfügbarkeit vor der Bestellung im Shop prüfen.

## Zwei Ausbaustufen

| Stufe | Versorgung | Ziel |
|---|---|---|
| **1 – Firmware & Optik** | USB-C vom PC | WLAN + MQTT, LEDs per LEDC-PWM (auch im Light Sleep – das größte technische Risiko), E-Ink mit Teil-Refresh, Lichtsensor, Linsen-Testdrucke, Strommessung am Modul |
| **2 – Akku** | 18650 über Lader + Schutz | Power-Path (USB rein/raus im Betrieb), Fuel Gauge, Tiefentladeschutz, echte Laufzeit, Grün bei niedriger Akkuspannung |

**Messung:** Das ESP32-C6-DevKitC-1 hat den Jumper **J5**. Zieht man ihn ab und hängt ein Amperemeter dazwischen, misst man nur den Strom des Moduls, ohne USB-Chip und Power-LED des Boards.

**Tipp für Stufe 1:** Die LED-Schiene an 3,3 V statt 5 V hängen. Das entspricht ungefähr der Akkuspannung kurz vor der Abschaltung, also dem schlechtesten Fall für die grüne LED (Durchlassspannung ~2,7–3,0 V).

## Teileliste

### Stufe 1 – Firmware & Optik

| # | Teil | Menge | Hersteller-Nr. / Hinweis | ca. € |
|---|---|---|---|---|
| 1 | **ESP32-C6-DevKitC-1** (8 MB Flash) | 2 | Espressif `ESP32-C6-DEVKITC-1-N8` – eins als Ersatz bzw. für Tests mit Netzteil | 2 × 10 |
| 2 | **2,9″-E-Paper-Modul** s/w, 296 × 128, SPI, mit Treiberplatine | 1 | Waveshare `12956` („2.9inch e-Paper Module"), am besten V2 (Controller SSD1680, wie auf der späteren Platine) | 15–20 |
| 3 | **Lichtsensor VEML7700**, STEMMA QT | 1 | Adafruit `4162` | 5 |
| 4 | **N-MOSFET**, Logic-Level, THT | 3 (+2) | Infineon `IRLB8721PBF` (schaltet sicher mit 3,3 V am Gate) | 5 |
| 5 | **LEDs 5 mm**, hell, breiter Abstrahlwinkel | je 20 | Rot ~625 nm, Gelb/Amber ~590 nm, Grün ~520 nm – genug für einen Ring aus 6–8 LEDs pro Kammer hinter der 72-mm-Linse | 6–8 |
| 6 | **Widerstands-Sortiment** E12, ¼ W | 1 | für LED-Vorwiderstände (47–330 Ω) und Gate-Pulldowns (10 kΩ) | 5–10 |
| 7 | **Taster** 6 × 6 mm | 5 | Info-Taster, Experimente | 1 |
| 8 | **Breadboard** 830 Kontakte | 2 | Das DevKit ist breit – mit zwei Boards nebeneinander bleiben auf beiden Seiten Kontakte frei | 2 × 5 |
| 9 | **Jumper-Kabel** m-m und m-f | je 1 Set | | 2 × 4 |
| 10 | **STEMMA-QT-Kabel** auf Stiftleiste | 2 | Adafruit `4209` – für VEML7700 und später MAX17048 | 2 × 1 |
| 11 | **USB-C-Datenkabel** | 1 | falls keines herumliegt | – |
| 12 | **Filament: PETG transparent** | 1 Rolle | für Linsen | 20–30 |

### Stufe 2 – Akku

| # | Teil | Menge | Hersteller-Nr. / Hinweis | ca. € |
|---|---|---|---|---|
| 13 | **Lader mit Power-Path** | 1 | Adafruit `4755` („Universal USB/DC/Solar Lithium Ion/Polymer charger – bq24074"), derselbe Lader-IC wie auf der Platine | 15–20 |
| 14 | **Fuel Gauge MAX17048**, STEMMA QT | 1 | Adafruit `5580` | 6–8 |
| 15 | **1S-Schutzplatine** DW01A + 8205A | 2 | generisch, „1S 3,7 V Li-Ion BMS / Protection Board" – die Vape-Zellen haben keinen eingebauten Schutz | 2 × 1 |
| 16 | **18650-Halter** mit Kabeln | 2 | einzeln, kein Doppelhalter | 2 × 2 |
| 17 | **Buck-Boost 3,3 V** | 1 | z. B. Pololu `S7V8F3` (2,7–11,8 V → 3,3 V). Speist das DevKit über den 3V3-Pin – dann *kein* USB am DevKit anstecken | 10–15 |
| 18 | **NTC 10 kΩ** (B ≈ 3435) | 1 | optional, für die Ladetemperatur-Überwachung am bq24074 | 1 |

### Optional – fürs genaue Messen

| Teil | Hinweis | ca. € |
|---|---|---|
| **INA226-Modul** mit 0,1-Ω-Shunt – die Sparvariante | Hängt zwischen Versorgung und Prototyp. Das zweite DevKit liest den Strom ein paar hundert Mal pro Sekunde aus und schickt ihn per MQTT ins Homelab, z. B. nach Grafana. Einzelne WLAN-Beacons sieht man damit nicht, aber Mittelwerte im mA-Bereich und mAh pro Tag – und das reicht fürs Energiebudget. Nebenbei entsteht ein eigenes kleines Open-Source-Werkzeug | ~5 |
| **Nordic Power Profiler Kit II** (`NRF-PPK2`) – die Luxusvariante | Misst von µA bis 1 A mit hoher Abtastrate. Damit sieht man jeden WLAN-Beacon, jeden Light-Sleep-Zyklus und jeden E-Ink-Refresh einzeln | ~100 |

**Summe:** Stufe 1 ca. 85–110 € (inkl. Filament), Stufe 2 ca. 40–55 €, ohne PPK2.

## Wo bestellen (Lieferung nach Österreich)

| Shop | Wofür | Warum |
|---|---|---|
| **[DigiKey.at](https://www.digikey.at)** – Hauptbestellung | Espressif, Adafruit, MOSFETs, LEDs, Widerstände, Breadboards, Kabel, PPK2 | Ab 50 € versandkostenfrei, meist in ~2 Tagen da. Mit **UPS** als Versandart sind Zoll und Einfuhrabgaben schon bezahlt (DDP) |
| **[BerryBase](https://www.berrybase.de)** (DE) | Waveshare-E-Paper; führt auch den Adafruit bq24074 | EU-Versand ohne Zoll; Waveshare ist hier gut sortiert |
| [reichelt.at](https://www.reichelt.at) | Alternative für die Basics (Breadboard, Kabel, LEDs, Widerstände, Taster) | österreichischer Shop eines deutschen Händlers, schnell; Espressif/Adafruit-Sortiment nicht geprüft |
| Pimoroni (UK) | – | Seit dem Brexit mit Zoll-Abwicklung, seit Juli 2026 dazu eine EU-Pauschale pro Warenart. Lohnt sich nur für Pimoroni-eigene Produkte |
| Mouser | – | liefert Adafruit-Produkte nicht an Privatkunden in der EU |
| Generische Teile (15, 16) | BerryBase, Amazon oder AliExpress | Cent-Artikel, überall zu haben |

## Sicherheit beim Akku-Aufbau (Stufe 2)

- Die Zelle hängt **immer** hinter einer Schutzplatine, auch auf dem Breadboard.
- Zellen vorher prüfen (siehe [README → Zellen-Check](../README.md#akku--sicherheit)).
- Die ersten Ladezyklen nicht unbeaufsichtigt laufen lassen, Aufbau auf eine nicht brennbare Unterlage (Fliese) stellen.
- Breadboard-Kontakte vertragen nur ~1 A. Der Akku-Pfad (Zelle → Schutz → Lader) wird darum mit Klemmen oder gelöteten Litzen verbunden, nicht übers Breadboard.

## Danach

Die Pinbelegung legen wir beim Firmware-Start fest. Dabei sind die Strapping-Pins des C6 zu beachten, und die LEDs kommen auf LP-GPIOs. Die Belegung landet dann als Tabelle und Fritzing-artige Skizze in `firmware/`.
