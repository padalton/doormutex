<!--
SPDX-FileCopyrightText: 2026 The doormutex contributors
SPDX-License-Identifier: CERN-OHL-S-2.0
-->

# Hardware

The KiCad project (schematic, layout, production files) follows in milestone M1b, after the [breadboard prototype](../docs/breadboard.md).

Everything in this directory and in `enclosure/` is licensed under the [CERN Open Hardware Licence v2 – Strongly Reciprocal](../LICENSES/CERN-OHL-S-2.0.txt).

## Magnet mount

Reference spot: the light sits on a **steel newel post of the stair railing** at the end of a ~5 m hallway. The enclosure gets magnet pockets in two places, both optional parameters in the OpenSCAD model:

- **Foot:** the light stands on the post. The magnets keep it from being knocked off; weight keeps it upright.
- **Side wall:** on the railing or handrail, so the light can sit higher, at eye level. Here the magnets carry the whole weight – through friction.

**Check first:** a fridge magnet must stick to the railing. Many railings are stainless steel (austenitic, e.g. 1.4301/304), which is barely magnetic. Then a clamp or a thin steel plate (glued or clamped on) is needed instead.

### Holding force – an estimate

| | Value |
|---|---|
| Mass | ~150 g PETG + ~45 g cell + ~25 g PCB, LEDs, e-ink ≈ **220 g** → 2.2 N |
| Friction on the side wall | printed PETG on painted steel μ ≈ 0.3; with a rubber pad μ ≈ 0.6 |
| Needed pull force, side wall, safety factor 3 | 2.2 N / 0.6 × 3 ≈ **11 N (≈ 1.1 kg)** with a rubber pad; ≈ 2.2 kg without |
| Proposal | **4 × Ø 12 × 3 mm neodymium discs (N42–N52)** in the side wall, 2 in the foot |

Datasheet pull forces (roughly 2.5–3.5 kg for a Ø 12 × 3 mm disc) apply to direct contact with thick, flat steel. A printed skin of 0.6 mm plus a pad, a thin tube wall and a round profile each cost a lot – expect a third to half of the datasheet value. Four discs still leave a margin; the tolerance test print checks it on the real railing.

### Construction

- **Pockets:** blind holes 0.1–0.2 mm larger than the magnet. Either **embedded** – pause the print at the pocket's top layer, drop the magnets in, continue; 0.6 mm (3 layers) of PETG cover them, no glue – or **open** pockets, pressed in and secured with a drop of CA glue or epoxy.
- **Polarity** doesn't matter on steel. Neodymium is fine up to ~80 °C – no issue for PETG or the cell.
- **Pad:** 0.5–1 mm rubber or silicone sheet on the contact face: higher friction (above) and no scratches on the paint. Felt protects too but slides.
- **Shape:** the contact face follows the profile – flat for square posts, a shallow V or radius for round tubes, so that more than one line touches. Post and rail sizes are parameters.
- **Fall protection:** a slot for a cable tie or cord around the post. A falling light on a staircase is a hazard, and it carries a cell.
- **Antenna:** the ESP32-C6 antenna sits at the top edge of the PCB; keep it away from the steel (no magnet wall directly behind it), large steel parts detune and shield it.

## Wi-Fi (UniFi)

Reference network: UniFi Dream Router (Wi-Fi 6) plus a UniFi AC Mesh (Wi-Fi 5) as an access point in the basement.

- **2.4 GHz:** the ESP32-C6 only speaks 2.4 GHz (Wi-Fi 6). The SSID the light uses needs the 2.4 GHz band enabled.
- **TWT** (target wake time, saves battery) exists only with Wi-Fi 6: possible with the Dream Router, not with the AC Mesh. Whether the access point actually grants TWT to the C6 is measured on the breadboard – the light must not depend on it.
- **mDNS** (`doormutex-<name>.local`): works without settings inside one network. If the light lives in its own IoT network/VLAN, enable *Multicast DNS* for the networks of the light, the hub and the phones, and don't let multicast/broadcast filtering on the SSID block it.
- **Battery tuning (optional, measure first):** a higher DTIM period on the 2.4 GHz SSID lets the light sleep longer between beacons; if the light has trouble connecting or roaming, try the SSID without *Fast Roaming* (802.11r).
