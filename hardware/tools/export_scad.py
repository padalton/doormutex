# SPDX-FileCopyrightText: 2026 The doormutex contributors
# SPDX-License-Identifier: CERN-OHL-S-2.0
"""Hands the board to the enclosure model.

    python3 hardware/tools/export_scad.py

Writes every part of hardware/doormutex.kicad_pcb as a box (outline on the
board, height above it, side) into the marked block of
enclosure/doormutex.scad. The PCB build's collision check (render.sh) then
tests the printed parts against the board as it really is, not against a
few hand-drawn dummies.

It also compares the model's board parameters (pcb_size, pcb_holes,
pcb_buttons, pcb_info_x, pcb_als, pcb_als_box, pcb_usb_overhang) with
gen_pcb.py and fails if they have drifted apart.
"""

import math
import os
import re
import sys

import pcbnew

HERE = os.path.dirname(os.path.abspath(__file__))
HW = os.path.dirname(HERE)
sys.path.insert(0, HERE)
import gen_pcb  # noqa: E402

PCB = os.path.join(HW, "doormutex.kicad_pcb")
SCAD = os.path.join(os.path.dirname(HW), "enclosure", "doormutex.scad")
BEGIN = "// BEGIN PCB PARTS – written by hardware/tools/export_scad.py, do not edit"
END = "// END PCB PARTS"

# Height above the board in mm, by the start of the footprint's name
HEIGHT = [
    ("R_0402", 0.4), ("C_0402", 0.55), ("R_0603", 0.5), ("C_0603", 0.9), ("C_0805", 1.35),
    ("R0603", 0.9), ("R0805", 0.65), ("R1206", 0.65), ("F1812", 1.3),
    ("SOT-23", 1.3), ("QFN-16", 1.0), ("TDFN-10", 0.8), ("VSON-10", 1.0), ("IND-SMD", 1.2),
    ("WIFIM-SMD", 2.6), ("USB_C_Receptacle", 3.3), ("KEY-SMD_SKRTLAE010", 3.4), ("SW-SMD_4P", 1.6),
    ("SMD_RH-5015", 1.6), ("BAT-SMD_1042P", 21.0),   # the holder with a cell in it
    ("LED", 0.8), ("SENSOR-SMD_EML7700-TT", 2.35), ("CONN-TH_S8B-PH", 4.8),
    ("PinHeader_1x02", 8.6), ("Crystal_SMD_3215", 0.9),
]
# The part's body in the footprint's own coordinates (x0, y0, x1, y1), where
# the footprint's drawing is larger than the part or leaves the part out
BODY = {
    "USB_C_Receptacle_HRO_TYPE-C-31-M-12": (-4.47, -4.77, 4.47, 3.65),   # pads to mouth
    "BAT-SMD_1042P": (-43.35, -10.35, 43.35, 10.35),
    "KEY-SMD_SKRTLAE010": (-2.3, -1.88, 2.3, 1.98),                      # with the plunger
    "CONN-TH_S8B-PH-K-S-LF-SN": (-9.0, -6.25, 9.0, 1.35),
    "SENSOR-SMD_EML7700-TT": (-3.43, -2.54, 3.43, 0.46),
    "LED0805-RD_RED_XL-2012SURC-FJ": (-1.0, -0.63, 1.0, 0.63),
    "LED0805-RD_GREEN": (-1.0, -0.63, 1.0, 0.63),
    "LED-SMD_L2.0-W1.3_XL-2012UYC-FJ": (-1.0, -0.63, 1.0, 0.63),
}


def height_of(name):
    for prefix, h in HEIGHT:
        if name.startswith(prefix):
            return h
    return None


def board_box(fp, body):
    """Box of the part on the board, mm from the board's top left corner seen from the front."""
    ox, oy = gen_pcb.ORIGIN
    if body is None:
        bb = fp.GetBoundingBox(False)
        return (pcbnew.ToMM(bb.GetLeft()) - ox, pcbnew.ToMM(bb.GetTop()) - oy, pcbnew.ToMM(bb.GetRight()) - ox, pcbnew.ToMM(bb.GetBottom()) - oy)
    a = math.radians(fp.GetOrientationDegrees())
    flip = -1 if fp.GetLayer() == pcbnew.B_Cu else 1   # KiCad mirrors a back-side footprint top to bottom
    px, py = pcbnew.ToMM(fp.GetPosition().x) - ox, pcbnew.ToMM(fp.GetPosition().y) - oy
    xs, ys = [], []
    for x, y in ((body[0], body[1]), (body[2], body[1]), (body[2], body[3]), (body[0], body[3])):
        y *= flip
        xs.append(px + x * math.cos(a) + y * math.sin(a))
        ys.append(py - x * math.sin(a) + y * math.cos(a))
    return (min(xs), min(ys), max(xs), max(ys))


def parts():
    board = pcbnew.LoadBoard(PCB)
    rows, unknown = [], []
    for fp in sorted(board.GetFootprints(), key=lambda f: (f.GetReference().rstrip("0123456789"), int("0" + f.GetReference().lstrip("ABCDEFGHIJKLMNOPQRSTUVWXYZ")))):
        name = fp.GetFPIDAsString().split(":")[-1]
        if name.startswith(("MountingHole", "TestPoint_Pad", "OSHW")):
            continue  # nothing that stands on the board
        h = height_of(name)
        if h is None:
            unknown.append(f"{fp.GetReference()} ({name})")
            continue
        x0, y0, x1, y1 = board_box(fp, BODY.get(name))
        rows.append((fp.GetReference(), x0, y0, x1, y1, h, 0 if fp.GetLayer() == pcbnew.B_Cu else 1))
    if unknown:
        sys.exit("no height in export_scad.py for: " + ", ".join(unknown))
    return board, rows


def scad_value(text, name):
    m = re.search(r"^%s\s*=\s*(.+?);" % re.escape(name), text, re.M)
    if not m:
        sys.exit(f"{name} not found in {os.path.relpath(SCAD)}")
    return eval(m.group(1), {"__builtins__": {}})   # numbers and lists of numbers


def check_parameters(text, board):
    """The enclosure's idea of the board against gen_pcb.py's."""
    place = gen_pcb.PLACE
    by_ref = {f.GetReference(): f for f in board.GetFootprints()}
    mouth = board_box(by_ref["J1"], BODY["USB_C_Receptacle_HRO_TYPE-C-31-M-12"])[3]
    sensor = board_box(by_ref["U8"], BODY["SENSOR-SMD_EML7700-TT"])
    want = {
        "pcb_size": [gen_pcb.W, gen_pcb.L],
        "pcb_holes": [list(p) for p in gen_pcb.MOUNT],
        "pcb_buttons": [list(place["SW1"][:2]), list(place["SW2"][:2])],
        "pcb_info_x": place["SW3"][0],
        "pcb_als": list(gen_pcb.ALS),
        "pcb_als_box": [round(v, 2) for v in sensor],
        "pcb_usb_overhang": round(mouth - gen_pcb.L, 2),
        "sign_h_pcb": gen_pcb.SIGN_H,
    }
    bad = []
    for name, value in want.items():
        have = scad_value(text, name)
        if have != value and not (isinstance(value, float) and abs(have - value) < 1e-6):
            bad.append(f"{name}: scad has {have}, the board has {value}")
    # the first LED of each lamp must sit 1.5 mm beside the middle of its chamber
    chamber, sign = scad_value(text, "chamber_h"), scad_value(text, "sign_h_pcb")
    top = sign + 2.5 * chamber + 15.5
    for i, ref in enumerate(("D5", "D3", "D1")):
        centre = top - (sign + chamber * (i + 0.5))
        if abs(place[ref][1] - centre) > 0.01 or abs(place[ref][0] + 1.5 - gen_pcb.W / 2) > 0.01:
            bad.append(f"{ref} at {place[ref][:2]} is not at its chamber's centre ({gen_pcb.W / 2}, {centre})")
    return bad


def main():
    board, rows = parts()
    text = open(SCAD, encoding="utf-8").read()
    if BEGIN not in text or END not in text:
        sys.exit(f"markers not found in {os.path.relpath(SCAD)}")
    lines = [BEGIN, "// [x0, y0, x1, y1, height, 1 = front / 0 = back] in board mm seen from the front", "pcb_parts = ["]
    lines += [f"  [{x0:.2f}, {y0:.2f}, {x1:.2f}, {y1:.2f}, {h:g}, {front}],  // {ref}" for ref, x0, y0, x1, y1, h, front in rows]
    lines += ["];"]
    head, rest = text.split(BEGIN, 1)
    new = head + "\n".join(lines) + "\n" + END + rest.split(END, 1)[1]
    if new != text:
        open(SCAD, "w", encoding="utf-8").write(new)
    print(f"{os.path.relpath(SCAD)}: {len(rows)} parts")
    bad = check_parameters(new, board)
    for line in bad:
        print("MISMATCH " + line)
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main())
