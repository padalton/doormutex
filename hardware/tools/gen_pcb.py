# SPDX-FileCopyrightText: 2026 The doormutex contributors
# SPDX-License-Identifier: CERN-OHL-S-2.0
"""Creates hardware/doormutex.kicad_pcb from design.py: outline, layers,
footprints with their nets, and a first placement by zones.

    python3 hardware/tools/gen_pcb.py

The board is the back wall of the light's three chambers (enclosure,
z = height above the bottom of the light):

  * front (F.Cu, towards the lenses): the LEDs in the middle of each
    reflector cone, the e-paper socket in the sign, the light sensor in a
    corner of the green chamber;
  * back (B.Cu, towards the back cover): everything else – the ESP32 at
    the top with its antenna past the top edge, the 18650 holder in the
    middle, power at the bottom with USB-C and the INFO button on the
    bottom edge.

The paths that carry real current (USB to the charger, the cell's leads, SYS
to the buck-boost, 3.3 V up to the ESP32) are laid here as wide tracks
(ROUTES); route_pcb.py protects them and lets Freerouting do the rest.
Running it again replaces the board, so placement tweaks belong in PLACE
below, not in the PCB file.
"""

import os
import sys

import pcbnew

HERE = os.path.dirname(os.path.abspath(__file__))
HW = os.path.dirname(HERE)
sys.path.insert(0, HERE)
import design  # noqa: E402

OUT = os.path.join(HW, "doormutex.kicad_pcb")
FP_DIRS = {"doormutex": os.path.join(HW, "lib", "doormutex.pretty")}
KICAD_FP = os.environ.get("KICAD_FOOTPRINT_DIR", "/usr/share/kicad/footprints")

REV, DATE = "v1.0", "2026-11"   # printed on the board: revision and the month it was ordered
URL = "https://doormutex.org"   # the QR code on the back

# ---- geometry (mm). Board coordinates: x across (0 left seen from the front),
# y down from the top edge. Origin on the page at ORIGIN.
W, L = 41.0, 191.5
ORIGIN = (50.0, 30.0)
# Enclosure (pcb build): sign 66 mm, three 45 mm chambers, 5 mm roof
SIGN_H = 66.0
Z_TOP = SIGN_H + 2 * 45 + 45 / 2 + 15.5  # board top edge, 15.5 above the red chamber's centre (bottom edge at 2.5)


def y_of(z):
    return Z_TOP - z


LAMP_Y = {"R": y_of(SIGN_H + 112.5), "Y": y_of(SIGN_H + 67.5), "G": y_of(SIGN_H + 22.5)}  # chamber centres
X_MID = W / 2

# Every part: ref -> (x, y, rotation, side[, pad]), side "F" (front, towards
# the lenses) or "B" (back). x, y in board coordinates seen from the front; a
# part on the back is mirrored by KiCad. (x, y) is the footprint's origin, or
# with a pad number the centre of that pad – for parts that must line up
# with a pin of their neighbour.
B, F = "B", "F"
UX, UY = 30.0, 147.65   # U6, the buck-boost: its power pins face right (x + 0.82)
PLACE = {
    # -- top of the back: ESP32, its antenna past the top edge into a
    # plastic-only roof; the bulk capacitor at its 3V3 pin
    "U7": (X_MID, 6.2, 180, B),
    "C13": (29.0, 2.3, 0, B), "C12": (29.6, 5.0, 0, B),        # at 3V3 (pin 3)
    "R14": (29.0, 8.4, 0, B), "C14": (29.0, 9.9, 0, B),        # EN
    "R15": (15.5, 15.0, 90, B), "R16": (17.0, 15.0, 90, B),    # BOOT, IO8 straps
    "R19": (12.3, 3.0, 90, B), "R20": (12.3, 6.0, 90, B),      # I²C pull-ups
    "Y1": (24.0, 15.4, 0, B), "C15": (21.2, 15.4, 90, B), "C16": (26.8, 15.4, 90, B),
    "SW1": (8.0, 19.0, 0, B), "SW2": (33.0, 19.0, 0, B),       # RESET, BOOT
    # -- left of the holder: a lamp driver at the height of its chamber, the
    # NTC against the cell, the test points
    "Q3": (5.0, 25.5, 0, B), "R21": (2.0, 29.0, 90, B), "R22": (3.5, 29.0, 90, B), "R23": (5.0, 29.0, 90, B), "R24": (6.5, 29.0, 90, B),
    "Q4": (5.0, 58.0, 0, B), "R25": (2.0, 61.5, 90, B), "R26": (3.5, 61.5, 90, B), "R27": (5.0, 61.5, 90, B), "R28": (6.5, 61.5, 90, B),
    "Q5": (5.0, 100.0, 0, B), "R29": (2.0, 103.5, 90, B), "R30": (3.5, 103.5, 90, B), "R31": (5.0, 103.5, 90, B), "R32": (6.5, 103.5, 90, B),
    "TH1": (8.6, 70.0, 90, B),
    "TP1": (5.0, 36.0, 0, B), "TP2": (5.0, 38.6, 0, B), "TP3": (5.0, 41.2, 0, B), "TP4": (5.0, 43.8, 0, B),
    "TP5": (5.0, 46.4, 0, B), "TP6": (5.0, 49.0, 0, B), "TP7": (5.0, 51.6, 0, B),
    # bare pads: the lamp signals as the three lamps of the printed traffic
    # light (LIGHT below), I²C at the gauge, the buck-boost's enable, the
    # charger's status outputs
    "TP8": (4.5, 78.0, 0, B), "TP9": (4.5, 81.6, 0, B), "TP10": (4.5, 85.2, 0, B),
    "TP11": (34.2, 51.2, 0, B), "TP12": (37.4, 51.2, 0, B),
    "TP13": (24.6, 144.2, 0, B), "TP14": (11.0, 144.6, 0, B), "TP15": (8.0, 144.6, 0, B),
    # -- right of the holder, at its minus end (top): protection, sense
    # resistor and gauge in the order the current takes
    "U3": (35.0, 29.5, 0, B), "R7": (38.9, 28.0, 90, B), "C5": (38.9, 30.8, 90, B), "R8": (38.9, 33.9, 90, B),
    "R6": (25.2, 30.0, 180, B),                                 # Q1's gate, at the cell's minus
    "Q2": (35.0, 35.0, 0, B),
    "R9": (36.35, 37.6, 270, B, "1"),                           # P_N right below Q2's pin 3
    "U4": (36.8, 43.6, 0, B, "5"),                              # below the resistor, room for SCL and SDA to leave between the sense tracks
    "C6": (35.2, 47.7, 180, B, "1"), "C7": (36.42, 47.7, 0, B, "1"), "R18": (38.9, 47.2, 90, B),
    # -- holder: plus at the bottom, next to the charger (rotation chosen below)
    "BT1": (X_MID, 70.0, 270, B),
    # -- below the holder, left: the cell's plus lead on its way down to the charger
    "J3": (9.5, 113.7, 90, B), "R5": (9.5, 116.8, 180, B),
    "F1": (8.02, 119.15, 270, B, "1"), "Q1": (8.02, 124.36, 270, B, "3"),
    # -- bottom left: charger; VBUS leaves it downwards, SYS to the right, the cell to the left
    "U2": (7.0, 150.0, 0, B),
    "C4": (3.9, 151.45, 270, B, "1"),                           # BAT
    "R3": (6.05, 152.55, 270, B, "1"), "R33": (7.05, 152.55, 270, B, "1"),   # ISET, TMR
    "C2": (7.75, 154.8, 0, B, "1"), "C1": (7.75, 156.25, 0, B, "1"),   # IN
    "R4": (9.35, 151.45, 270, B, "1"),                          # ILIM
    "C3": (10.6, 150.3, 270, B, "1"),                           # OUT
    # -- bottom right: buck-boost as in the data sheet's layout – the coil at
    # the two switch pins, input and output capacitor at the ends of the row –
    # and the supervisor that enables it
    "U6": (UX, UY, 0, B), "L1": (UX + 2.75, UY, 90, B),
    "C9": (UX + 0.82, UY + 2.35, 180, B, "1"),
    "C10": (UX + 0.82, UY - 2.35, 180, B, "1"), "C11": (UX + 0.82, UY - 3.85, 180, B, "1"),
    "R13": (UX - 2.09, UY - 0.5, 180, B, "1"), "R12": (UX - 2.09, UY - 1.56, 270, B, "2"),
    "U5": (22.5, 147.6, 0, B), "C8": (25.2, 147.0, 90, B), "R10": (25.2, 149.0, 90, B), "R11": (26.6, 149.7, 90, B),
    # -- bottom edge: USB-C with its mouth 0.6 mm past the edge (the data
    # sheet's board edge is another 0.4 mm back; this way the mouth clears the
    # enclosure's bottom wall when the board goes in), ESD, INFO button
    "J1": (X_MID, L - 3.05, 180, B), "U1": (X_MID, L - 12.0, 0, B), "R1": (14.0, L - 12.5, 90, B), "R2": (27.0, L - 12.5, 90, B),
    "SW3": (30.0, L - 2.1, 180, B), "R17": (30.0, L - 6.1, 0, B),
    # -- front: LEDs in the middle of the cones, the e-paper socket in the
    # sign, the light sensor in the top corner of the green chamber (a light
    # channel in the front shell)
    # J2: pins at the top, opening down towards the module's plug below it;
    # its back end stays 0.65 mm below the wall between sign and green chamber
    "J2": (X_MID, y_of(SIGN_H - 2.6), 180, F), "C18": (30.6, y_of(SIGN_H - 2.6), 90, F),   # C18 at pin 8, 3.3 V
    # U8: pads towards the board edge; its sensitive area is 1.5 mm from the
    # pad row, at ALS below
    "U8": (1.9, 88.0, 270, F), "C17": (2.6, 92.6, 0, F),
}
ALS = (3.4, 88.0)  # centre of U8's sensitive area – the enclosure's light channel (pcb_als)
for i, col in enumerate("RYG"):
    PLACE[f"D{2 * i + 1}"] = (X_MID - 1.5, LAMP_Y[col], 90, F)
    PLACE[f"D{2 * i + 2}"] = (X_MID + 1.5, LAMP_Y[col], 90, F)
# Parts whose PLACE point is the centre of their outline, not their origin
CENTRED = {"J3", "BT1"}
# Next to the cones, away from the e-paper module; the top two are 11 mm
# from the antenna – nylon screws there.
MOUNT = [(3.0, 6.0), (W - 3.0, 6.0), (3.0, 113.0), (W - 3.0, 113.0)]

# ---- the paths that carry current, laid by hand: (net, layer, width, points).
# A point is (x, y) or "REF.PAD" (the pad's centre). Widths: 1 mm of 35 µm
# copper is 0.5 mΩ per mm; the inner layers are half as thick, so nothing
# heavy goes there.
TRUNK_X = 32.3   # 3.3 V runs up the front, between the holder's pegs and the right column
ROUTES = [
    # USB-C -> charger IN (up to 1.5 A): both pin pairs to the front, up, back at the charger
    ("VBUS", B, 0.5, ["J1.A9", (18.05, L - 9.3)]), ("VBUS", B, 0.5, ["J1.A4", (22.95, L - 9.3)]),
    ("VBUS", F, 1.5, [(22.95, L - 9.3), (9.95, L - 9.3), (7.75, L - 11.5), (7.75, 157.65)]),
    ("VBUS", F, 0.9, [(7.2, 157.65), (8.3, 157.65)]), ("VBUS", B, 0.9, [(7.2, 157.65), (8.3, 157.65)]),
    ("VBUS", B, 0.9, [(7.75, 157.65), "C1.1", "C2.1"]),
    ("VBUS", B, 0.28, ["C2.1", "U2.13"]),
    # cell plus -> 0 Ω -> PTC -> Q1 -> charger BAT (1 A)
    ("CELL_P", B, 1.2, [(17.8, 112.6), (13.6, 116.8), "R5.1"]),
    ("CELL_PM", B, 1.2, ["R5.2", "F1.1"]),
    ("CELL_PF", B, 0.6, ["F1.2", "Q1.3"]),
    ("VBAT", B, 0.6, ["Q1.2", (7.07, 128.4)]),
    ("VBAT", B, 1.0, [(7.07, 128.4), (3.9, 131.6), (3.9, 150.0), "C4.1"]),
    ("VBAT", B, 0.75, [(3.9, 150.0), (5.5, 150.0)]),             # across pins 2 and 3
    ("VBAT", B, 0.28, ["U2.2", (5.5, 150.0), "U2.3"]),
    # cell minus -> protection MOSFETs -> sense resistor -> ground (1 A)
    ("CELL_N", B, 1.0, [(23.6, 32.9), (37.7, 32.9), (37.7, 34.05)]), ("CELL_N", B, 0.6, [(37.7, 34.05), "Q2.1"]),
    ("P_N", B, 0.8, ["Q2.3", "R9.1"]),
    ("GND", B, 0.9, ["R9.2", (38.3, 39.6)]), ("GND", B, 0.9, [(38.3, 39.2), (38.3, 40.4)]),
    # the gauge measures across the resistor's pads, not across the tracks to them
    ("P_N", B, 0.2, [(35.85, 37.6), (34.9, 37.6), (34.9, 42.6), (35.6, 43.05), "U4.2"]),
    ("GND", B, 0.2, [(36.9, 40.0), (37.35, 40.6), (37.35, 43.0), "U4.5"]),
    # charger OUT -> buck-boost (0.7 A at the Wi-Fi peaks), passing below C9's ground pad
    ("SYS", B, 0.75, [(8.5, 150.0), (10.6, 150.0)]),             # across pins 10 and 11, to C3
    ("SYS", B, 0.28, ["U2.10", (8.5, 150.0), "U2.11"]),
    ("SYS", B, 1.0, [(10.6, 150.0), (12.4, 150.0), (13.8, 151.4), (UX + 0.82, 151.4), "C9.1"]),
    ("SYS", B, 0.5, ["C9.1", (UX + 0.82, UY + 1.1)]),         # ends on pin 10, clear of pin 9
    # buck-boost: coil, output capacitors, and 3.3 V up the front to the ESP32
    ("SW2", B, 0.3, ["U6.7", (UX + 2.2, UY - 0.5)]), ("SW1", B, 0.3, ["U6.9", (UX + 2.2, UY + 0.5)]),
    ("3V3", B, 0.5, [(UX + 0.82, UY - 1.1), "C10.1"]), ("3V3", B, 0.9, ["C10.1", "C11.1"]),
    ("3V3", B, 1.0, [(UX + 0.82, UY - 3.15), (TRUNK_X, UY - 3.15), (TRUNK_X, UY - 4.15)]),
    ("3V3", F, 2.0, [(TRUNK_X, UY - 3.15), (TRUNK_X, 6.0)]),
    ("3V3", B, 1.0, [(TRUNK_X, 6.0), (TRUNK_X, 7.0), (28.65, 7.0), "C12.1"]),
    ("3V3", B, 0.4, ["C12.1", (28.0, 3.83), "U7.3"]),
    # the feedback divider senses 3.3 V at the output capacitor, not somewhere along the track
    ("3V3", B, 0.25, ["R12.1", (UX - 2.09, UY - 5.75), (UX + 0.82, UY - 5.75), "C11.1"]),
    # buck-boost ground: both ground pins and the capacitors' ground pads go
    # straight down to the plane
    ("GND", B, 0.3, ["U6.8", "U6.3", (UX - 1.7, UY + 0.3), "U6.2"]),
    ("GND", B, 0.5, ["C9.2", (UX - 1.95, UY + 2.35)]),
    ("GND", B, 0.6, ["C10.2", "C11.2", (UX - 0.73, UY - 5.0)]),
]
VIAS = [  # net, x, y, diameter, drill
    ("VBUS", 18.05, L - 9.3, 0.8, 0.4), ("VBUS", 22.95, L - 9.3, 0.8, 0.4), ("VBUS", 7.2, 157.65, 0.8, 0.4), ("VBUS", 8.3, 157.65, 0.8, 0.4),
    ("GND", 38.3, 39.2, 0.8, 0.4), ("GND", 38.3, 40.4, 0.8, 0.4),
    ("GND", UX - 1.7, UY + 0.3, 0.6, 0.3), ("GND", UX - 1.95, UY + 2.35, 0.6, 0.3), ("GND", UX - 0.73, UY - 5.0, 0.6, 0.3),
    ("3V3", TRUNK_X, UY - 3.15, 0.8, 0.4), ("3V3", TRUNK_X, UY - 4.15, 0.8, 0.4), ("3V3", TRUNK_X, 6.0, 0.8, 0.4), ("3V3", TRUNK_X, 7.0, 0.8, 0.4),
    # the charger's thermal pad: its heat goes into the ground plane and the front pour
    ("GND", 6.55, 149.55, 0.6, 0.3), ("GND", 7.45, 149.55, 0.6, 0.3), ("GND", 6.55, 150.45, 0.6, 0.3), ("GND", 7.45, 150.45, 0.6, 0.3),
    # ties the back's ground pour under the bare-copper name to the plane
    ("GND", 11.6, 155.3, 0.6, 0.3),
]
# No copper here: (layers, x0, y0, x1, y1, tracks and vias too?); "Bt" keeps
# tracks and vias out of the back but lets the ground pour in
KEEPOUTS = [
    # the strip of board under the module's antenna end, above its first pad row
    ("all", 13.2, 0.0, 27.8, 0.8, True),
    # SKRTLAE010 data sheet: no copper under the plunger, between the frame pads
    ("B", 29.0, L - 1.75, 31.0, L - 0.5, True),
    # the gauge's CSN pin hangs on the sense resistor's pad by its own track only
    ("B", 36.6, 40.3, 37.8, 44.05, False),
    # nothing but ground under the bare-copper name, a flat ground under the QR code
    ("Bt", 12.4, 153.2, 36.6, 157.4, True), ("Bt", 25.4, 158.4, 39.8, 172.8, True),
]
# What is printed on the board besides the footprints' own outlines:
# (text, x, y, side, size[, angle]). The cell's polarity beside the holder's
# contacts and, large, where the board shows through the holder's open floor;
# the test points by their nets; who made this and under which licence.
LABELS = [
    ("+", 14.0, 111.2, B, 2.4), ("+", 27.0, 111.2, B, 2.4), ("-", 14.0, 28.8, B, 2.4), ("-", 27.0, 28.8, B, 2.4),
    ("+", 20.5, 88.0, B, 4.0), ("-", 20.5, 52.0, B, 4.0),
    ("USB", 8.2, 36.0, B, 0.8), ("BAT", 8.2, 38.6, B, 0.8), ("SYS", 8.2, 41.2, B, 0.8), ("3V3", 8.2, 43.8, B, 0.8),
    ("GND", 8.2, 46.4, B, 0.8), ("P_N", 8.2, 49.0, B, 0.8), ("TX", 8.2, 51.6, B, 0.8),   # TP1 … TP7
    ("SDA", 34.2, 53.0, B, 0.8), ("SCL", 37.4, 53.0, B, 0.8), ("3V3EN", 24.6, 142.5, B, 0.8),
    ("CHG", 11.0, 142.9, B, 0.8), ("PG", 8.0, 142.9, B, 0.8),
    ("R5", 13.4, 119.0, B, 0.9), ("J3", 9.5, 111.4, B, 0.9),
    ("RESET", 8.0, 23.0, B, 1.0), ("BOOT", 33.0, 23.0, B, 1.0), ("INFO", 34.6, L - 3.2, B, 1.0),
    ("BUSY", 12.0, 128.4, B, 0.9), ("3V3", 29.4, 128.4, B, 0.9), ("BUSY", 12.4, 128.3, F, 0.9), ("3V3", 28.8, 128.3, F, 0.9),
    # title block, left of the QR code
    ("PCB " + REV, 18.2, 160.2, B, 1.5), (DATE, 18.2, 162.6, B, 1.2),
    ("doormutex.org", 18.2, 165.2, B, 1.1), ("CERN-OHL-S-2.0", 18.2, 167.4, B, 0.9),
    # The lamps by what they say, in errno: the front faces the reflector
    # cones and is never seen again once the light is closed.
    ("16 EBUSY", X_MID, LAMP_Y["R"] + 8.0, F, 1.2), ("11 EAGAIN", X_MID, LAMP_Y["Y"] + 8.0, F, 1.2), ("0 OK", X_MID, LAMP_Y["G"] + 8.0, F, 1.2),
    ("// if you can read this, I am not busy", X_MID, 160.0, F, 0.9),
    # under the cell, readable through the holder's floor when the cell is out
    ("mutex_lock(&door);", 20.5, 70.0, B, 1.6, 90),
    # under the holder's left rail: only a board without its holder shows it
    ("/* no deadlocks were harmed in the making of this board */", 13.3, 64.0, B, 1.0, 90),
    # the traffic light: U+1F6A6 is its code point
    ("U+1F6A6", 4.6, 89.6, B, 0.8),
]
# The name in bare copper (gold with ENIG): an opening in the solder mask
# over the ground pour. (text, x, y, size) on the back.
WORDMARK = ("doormutex", 24.5, 155.3, 3.0)
# The traffic light's housing around TP8…TP10, on the back: (x0, y0, x1, y1)
LIGHT = (2.5, 75.9, 6.5, 87.3)
QR = (25.6, 158.6, 0.45, 3)   # x, y of its top left corner, module size, quiet zone in modules
OSHW = (18.2, 171.9)           # the open hardware gear


def mm(v):
    return pcbnew.FromMM(v)


def vec(x, y):
    return pcbnew.VECTOR2I(mm(ORIGIN[0] + x), mm(ORIGIN[1] + y))


def load_fp(fpid):
    lib, name = fpid.split(":")
    path = FP_DIRS.get(lib, os.path.join(KICAD_FP, lib + ".pretty"))
    fp = pcbnew.FootprintLoad(path, name)
    if fp is None:
        sys.exit(f"footprint {fpid} not found in {path}")
    return fp


def footprint_id(p):
    if p["footprint"]:
        return p["footprint"]
    # imported parts: the symbol's default footprint
    import gen_schematic as gs
    return gs.prop(gs.lib_symbol(p["lib_id"]), "Footprint")


def pin_nets(p):
    import gen_schematic as gs
    sym = gs.lib_symbol(p["lib_id"])
    return gs.resolve_pins(p, gs.pins_of(sym))


def pad_of(fp, number):
    pads = [p for p in fp.Pads() if p.GetNumber() == number]
    if not pads:
        sys.exit(f"{fp.GetReference()}: no pad {number}")
    return pads[0]


def main():
    board = pcbnew.BOARD()
    board.SetCopperLayerCount(4)
    ds = board.GetDesignSettings()
    ds.SetBoardThickness(mm(1.6))
    # JLCPCB 4-layer capabilities, with a little margin
    ds.m_MinClearance = mm(0.127)
    ds.m_TrackMinWidth = mm(0.127)
    ds.m_ViasMinSize = mm(0.45)
    ds.m_MinThroughDrill = mm(0.3)
    ds.m_ViasMinAnnularWidth = mm(0.13)
    ds.m_CopperEdgeClearance = mm(0.3)
    ds.m_HoleClearance = mm(0.2)
    nc = ds.m_NetSettings.GetDefaultNetclass()
    nc.SetClearance(mm(0.127))
    nc.SetTrackWidth(mm(0.2))
    nc.SetViaDiameter(mm(0.6))
    nc.SetViaDrill(mm(0.3))

    nets = {}

    def net(name):
        if name not in nets:
            n = pcbnew.NETINFO_ITEM(board, name)
            board.Add(n)
            nets[name] = n
        return nets[name]

    fps = {}
    for p in design.parts:
        if p["ref"].startswith("#"):
            continue
        fp = load_fp(footprint_id(p))
        fp.SetReference(p["ref"])
        fp.SetValue(p["value"])
        if p["lcsc"]:
            fp.SetField("LCSC", p["lcsc"])
            fp.GetField("LCSC").SetVisible(False)
        if p["dnp"]:
            fp.SetDNP(True)
        board.Add(fp)
        for num, n in pin_nets(p).items():
            for pad in fp.Pads():
                if pad.GetNumber() == num and n:
                    pad.SetNet(net(n))
        fps[p["ref"]] = fp

    for ref, (x, y, rot, side, *anchor) in PLACE.items():
        fp = fps[ref]
        if side == "B":
            fp.SetLayerAndFlip(pcbnew.B_Cu)
        fp.SetOrientationDegrees(rot)
        fp.SetPosition(vec(x, y))
        if ref in CENTRED:
            c = fp.GetBoundingBox(False).GetCenter()
            fp.Move(pcbnew.VECTOR2I(vec(x, y).x - c.x, vec(x, y).y - c.y))
        if anchor:  # (x, y) is the centre of this pad
            c = pad_of(fp, anchor[0]).GetPosition()
            fp.Move(pcbnew.VECTOR2I(vec(x, y).x - c.x, vec(x, y).y - c.y))
    # the holder's plus (pad 2) belongs at the bottom, next to the charger
    bt = fps["BT1"]
    pads = {p.GetNumber(): p.GetPosition().y for p in bt.Pads() if p.GetNumber() in ("1", "2")}
    if pads["2"] < pads["1"]:
        bt.SetOrientationDegrees(bt.GetOrientationDegrees() + 180)
    # KiCad mirrors back-side parts top to bottom: check that what must face
    # outwards does – the antenna up, the USB-C opening and the INFO
    # actuator down (the outline reaches past the pads on that side).
    def overhang(ref):
        fp = fps[ref]
        bb = fp.GetBoundingBox(False)
        ys = [p.GetPosition().y for p in fp.Pads()]
        return pcbnew.ToMM(min(ys) - bb.GetTop()), pcbnew.ToMM(bb.GetBottom() - max(ys))
    up, _ = overhang("U7")
    if up < 4.0:
        sys.exit(f"U7: the antenna doesn't point up ({up:.1f} mm above the pads)")
    for ref in ("J1", "SW3"):
        top, bottom = overhang(ref)
        if bottom < top:
            sys.exit(f"{ref}: the opening/actuator doesn't point down")
    # EasyEDA marks locating pegs and shell tabs without a net as plated
    # holes without copper around them: make them plain (NPTH) holes.
    for fp in fps.values():
        fp.SetLocalClearance(None)  # EasyEDA sets 0.2 mm on some parts; use the board rules
        for pad in fp.Pads():
            pad.SetLocalClearance(None)
            if pad.GetAttribute() == pcbnew.PAD_ATTRIB_PTH and not pad.GetNetname():
                if pad.GetSizeX() <= pad.GetDrillSizeX() + mm(0.05):
                    pad.SetAttribute(pcbnew.PAD_ATTRIB_NPTH)
                    pad.SetLayerSet(pcbnew.PAD.UnplatedHoleMask())
    unplaced = [r for r in fps if r not in PLACE]
    if unplaced:
        sys.exit("no place for: " + ", ".join(unplaced))

    # Silkscreen. EasyEDA's footprints put their reference 4 mm beside the
    # part, across whatever lies there: the references go to the fab layer,
    # at the part (LABELS prints what is worth reading on the board). An
    # outline that would land on a pad or past the board edge is dropped –
    # the factory would cut it off there anyway.
    edge = pcbnew.BOX2I(vec(0.3, 0.3), pcbnew.VECTOR2I(mm(W - 0.6), mm(L - 0.6)))
    openings = {side: [p.GetBoundingBox() for f in fps.values() for p in f.Pads() if p.IsOnLayer(side)] for side in (pcbnew.F_Mask, pcbnew.B_Mask)}
    for fp in fps.values():
        back = fp.GetLayer() == pcbnew.B_Cu
        ref = fp.Reference()
        ref.SetLayer(pcbnew.B_Fab if back else pcbnew.F_Fab)
        ref.SetPosition(fp.GetBoundingBox(False).GetCenter())
        ref.SetTextSize(pcbnew.VECTOR2I(mm(0.5), mm(0.5)))
        ref.SetTextThickness(mm(0.08))
        for g in list(fp.GraphicalItems()):
            if g.GetLayer() not in (pcbnew.F_SilkS, pcbnew.B_SilkS):
                continue
            box = g.GetBoundingBox()
            box.Inflate(mm(0.1))
            if not edge.Contains(box) or any(box.Intersects(o) for o in openings[pcbnew.B_Mask if back else pcbnew.F_Mask]):
                fp.Remove(g)

    # Mounting holes (M2.5) into the enclosure's ribs
    for i, (x, y) in enumerate(MOUNT, start=1):
        h = load_fp("MountingHole:MountingHole_2.7mm_M2.5")
        h.SetReference(f"H{i}")
        h.SetPosition(vec(x, y))
        board.Add(h)

    # Outline with rounded corners
    r = 1.0
    seg = [((r, 0), (W - r, 0)), ((W, r), (W, L - r)), ((W - r, L), (r, L)), ((0, L - r), (0, r))]
    for (ax, ay), (bx, by) in seg:
        s = pcbnew.PCB_SHAPE(board, pcbnew.SHAPE_T_SEGMENT)
        s.SetStart(vec(ax, ay))
        s.SetEnd(vec(bx, by))
        s.SetLayer(pcbnew.Edge_Cuts)
        s.SetWidth(mm(0.1))
        board.Add(s)
    k = r * (1 - 0.70710678)
    for (sx, sy), (mx, my), (ex, ey) in [((0, r), (k, k), (r, 0)), ((W - r, 0), (W - k, k), (W, r)),
                                         ((W, L - r), (W - k, L - k), (W - r, L)), ((r, L), (k, L - k), (0, L - r))]:
        a = pcbnew.PCB_SHAPE(board, pcbnew.SHAPE_T_ARC)
        a.SetArcGeometry(vec(sx, sy), vec(mx, my), vec(ex, ey))
        a.SetLayer(pcbnew.Edge_Cuts)
        a.SetWidth(mm(0.1))
        board.Add(a)

    # The hand-laid tracks and vias. A wrong net on a pad they end on would
    # be a short, so every "REF.PAD" point is checked against the track's net.
    def point(spec, netname):
        if isinstance(spec, str):
            ref, number = spec.split(".")
            pad = pad_of(fps[ref], number)
            if pad.GetNetname() != netname:
                sys.exit(f"ROUTES: {spec} is on {pad.GetNetname()!r}, not {netname!r}")
            return pad.GetPosition()
        return vec(*spec)

    for netname, side, width, points in ROUTES:
        pts = [point(s, netname) for s in points]
        for a, b in zip(pts, pts[1:]):
            tr = pcbnew.PCB_TRACK(board)
            tr.SetStart(a)
            tr.SetEnd(b)
            tr.SetWidth(mm(width))
            tr.SetLayer(pcbnew.B_Cu if side == "B" else pcbnew.F_Cu)
            tr.SetNet(net(netname))
            tr.SetLocked(True)
            board.Add(tr)
    for netname, x, y, diameter, drill in VIAS:
        via = pcbnew.PCB_VIA(board)
        via.SetViaType(pcbnew.VIATYPE_THROUGH)
        via.SetLayerPair(pcbnew.F_Cu, pcbnew.B_Cu)
        via.SetPosition(vec(x, y))
        via.SetWidth(pcbnew.F_Cu, mm(diameter))
        via.SetDrill(mm(drill))
        via.SetNet(net(netname))
        via.SetLocked(True)
        board.Add(via)

    for layers, x0, y0, x1, y1, tracks_too in KEEPOUTS:
        z = pcbnew.ZONE(board)
        z.SetIsRuleArea(True)
        z.SetDoNotAllowZoneFills(layers != "Bt")
        z.SetDoNotAllowTracks(tracks_too)
        z.SetDoNotAllowVias(tracks_too)
        z.SetDoNotAllowPads(False)
        z.SetDoNotAllowFootprints(False)
        if layers == "all":
            z.SetLayerSet(pcbnew.LSET.AllCuMask())
        else:
            z.SetLayer(pcbnew.B_Cu if layers in ("B", "Bt") else pcbnew.F_Cu)
        o = z.Outline()
        o.NewOutline()
        for x, y in [(x0, y0), (x1, y0), (x1, y1), (x0, y1)]:
            o.Append(mm(ORIGIN[0] + x), mm(ORIGIN[1] + y))
        board.Add(z)

    def text(string, x, y, layer, size, angle=0):
        s = pcbnew.PCB_TEXT(board)
        s.SetText(string)
        s.SetPosition(vec(x, y))
        s.SetLayer(layer)
        s.SetTextSize(pcbnew.VECTOR2I(mm(size), mm(size)))
        s.SetTextThickness(mm(max(0.15, size * 0.15)))
        s.SetTextAngle(pcbnew.EDA_ANGLE(angle, pcbnew.DEGREES_T))
        s.SetMirrored(layer in (pcbnew.B_SilkS, pcbnew.B_Mask))
        board.Add(s)

    for string, x, y, side, size, *angle in LABELS:
        text(string, x, y, pcbnew.B_SilkS if side == "B" else pcbnew.F_SilkS, size, *angle)
    text(WORDMARK[0], WORDMARK[1], WORDMARK[2], pcbnew.B_Mask, WORDMARK[3])

    def silk(shape, layer=pcbnew.B_SilkS, width=0.2):
        shape.SetLayer(layer)
        shape.SetWidth(mm(width))
        board.Add(shape)

    # the traffic light's housing and post; its lamps are TP8…TP10
    x0, y0, x1, y1 = LIGHT
    box = pcbnew.PCB_SHAPE(board, pcbnew.SHAPE_T_RECT)
    box.SetStart(vec(x0, y0))
    box.SetEnd(vec(x1, y1))
    silk(box)
    post = pcbnew.PCB_SHAPE(board, pcbnew.SHAPE_T_SEGMENT)
    post.SetStart(vec((x0 + x1) / 2, y1))
    post.SetEnd(vec((x0 + x1) / 2, y1 + 1.2))
    silk(post, width=0.5)

    # QR code: the light modules are print, the dark ones bare board – one
    # polygon, mirrored so that it reads from the back
    sys.path.insert(0, os.environ.get("KICAD_PLUGIN_DIR", "/usr/share/kicad/scripting/plugins"))
    import kicad_qrcode
    qr = kicad_qrcode.QRCode.getMinimumQRCode(URL, kicad_qrcode.ErrorCorrectLevel.M)
    qx, qy, m, quiet = QR
    n = qr.getModuleCount()
    side = (n + 2 * quiet) * m

    def square(poly, x, y, w, h):
        poly.NewOutline()
        for px, py in ((x, y), (x + w, y), (x + w, y + h), (x, y + h)):
            poly.Append(mm(ORIGIN[0] + px), mm(ORIGIN[1] + py))

    light, dark = pcbnew.SHAPE_POLY_SET(), pcbnew.SHAPE_POLY_SET()
    square(light, qx, qy, side, side)
    for row in range(n):
        col = 0
        while col < n:
            if not qr.isDark(row, col):
                col += 1
                continue
            run = col
            while run < n and qr.isDark(row, run):
                run += 1
            # column 0 is on the right in board coordinates: the back is seen mirrored
            square(dark, qx + side - (quiet + run) * m, qy + (quiet + row) * m, (run - col) * m, m)
            col = run
    dark.Simplify()
    light.BooleanSubtract(dark)
    light.Fracture()
    code = pcbnew.PCB_SHAPE(board, pcbnew.SHAPE_T_POLY)
    code.SetPolyShape(light)
    code.SetFilled(True)
    silk(code, width=0)

    # the open source hardware gear
    gear = load_fp("Symbol:OSHW-Logo_5.7x6mm_SilkScreen")
    gear.SetReference("G1")
    gear.Reference().SetVisible(False)
    gear.Value().SetVisible(False)
    gear.SetAttributes(pcbnew.FP_BOARD_ONLY | pcbnew.FP_EXCLUDE_FROM_BOM | pcbnew.FP_EXCLUDE_FROM_POS_FILES)
    board.Add(gear)                     # a footprint can only be flipped once it is on a board
    gear.SetLayerAndFlip(pcbnew.B_Cu)
    gear.SetPosition(vec(*OSHW))

    # Ground on the inner layer next to the parts (inner layer 2 is 0.2 mm
    # below the back, where everything but the LEDs sits), everywhere
    zone = pcbnew.ZONE(board)
    zone.SetLayer(pcbnew.In2_Cu)
    zone.SetNet(net("GND"))
    outline = zone.Outline()
    outline.NewOutline()
    for x, y in [(0, 0), (W, 0), (W, L), (0, L)]:
        outline.Append(mm(ORIGIN[0] + x), mm(ORIGIN[1] + y))
    zone.SetMinThickness(mm(0.2))
    zone.SetPadConnection(pcbnew.ZONE_CONNECTION_THT_THERMAL)
    board.Add(zone)

    board.Save(OUT)
    print(f"wrote {os.path.relpath(OUT)}: {len(fps)} footprints, {len(nets)} nets, {W} × {L} mm, 4 layers")


if __name__ == "__main__":
    main()
