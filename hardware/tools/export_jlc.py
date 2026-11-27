# SPDX-FileCopyrightText: 2026 The doormutex contributors
# SPDX-License-Identifier: CERN-OHL-S-2.0
"""Writes the JLCPCB order files to hardware/production/.

    python3 hardware/tools/export_jlc.py

  doormutex-gerbers.zip   Gerber + drill files (4 layers)
  doormutex-bom.csv       Comment, Designator, Footprint, LCSC – what JLCPCB places
  doormutex-cpl.csv       Designator, Mid X, Mid Y, Layer, Rotation

Parts that are not fitted (DNP) or hand-soldered through-hole parts (the
e-paper socket) are left out of BOM and CPL.

JLCPCB's placement file follows EasyEDA's conventions, not KiCad's:

  * Mid X / Mid Y is the centre of the part's pads, not the footprint's origin;
  * the rotation of a part on the bottom is what you see looking at the
    bottom: 180° minus KiCad's angle. KiCad's own position file gives its
    angle unchanged, which turns every bottom part at 0° or 180° upside down.

The footprints in lib/ are EasyEDA's, so their 0° is JLCPCB's 0°. A footprint
from KiCad's library can be drawn turned against JLCPCB's: ROTATION holds
the difference for those. Still check the placement preview on JLCPCB's
order page – it shows the parts on the pads as their machine will place them.
"""

import csv
import os
import shutil
import subprocess
import sys
import tempfile
import zipfile

import pcbnew

HERE = os.path.dirname(os.path.abspath(__file__))
HW = os.path.dirname(HERE)
PCB = os.path.join(HW, "doormutex.kicad_pcb")
OUT = os.path.join(HW, "production")
HAND_SOLDERED = {"J2"}  # through-hole, soldered at home
# Footprints from KiCad's library: degrees to add so that 0° is the part as
# EasyEDA draws it. Two-pad resistors and capacitors lie the same way in both.
ROTATION = {
    # pads at the back, mouth at the front, A1 on the left – as EasyEDA's footprint for C165948
    "USB_C_Receptacle_HRO_TYPE-C-31-M-12": 0,
}


def gerbers():
    with tempfile.TemporaryDirectory() as tmp:
        layers = "F.Cu,In1.Cu,In2.Cu,B.Cu,F.Paste,B.Paste,F.Silkscreen,B.Silkscreen,F.Mask,B.Mask,Edge.Cuts"
        subprocess.run(["kicad-cli", "pcb", "export", "gerbers", "--layers", layers, "--subtract-soldermask",
                        "--no-protel-ext", "-o", tmp + "/", PCB], check=True, capture_output=True)
        # No drill maps (they draw a legend beside the board) and no job file:
        # the layers and the two drill files are all JLCPCB reads.
        subprocess.run(["kicad-cli", "pcb", "export", "drill", "--format", "excellon", "--excellon-separate-th",
                        "-o", tmp + "/", PCB], check=True, capture_output=True)
        files = sorted(f for f in os.listdir(tmp) if f.endswith((".gbr", ".drl")))
        path = os.path.join(OUT, "doormutex-gerbers.zip")
        with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED) as z:
            for f in files:
                z.write(os.path.join(tmp, f), f)
        return path, len(files)


def pad_centre(fp):
    """Centre of the box around the part's pads (holes without a pad number are not pads)."""
    box = None
    for pad in fp.Pads():
        if not pad.GetNumber():
            continue
        b = pad.GetBoundingBox()
        if box is None:
            box = b
        else:
            box.Merge(b)
    return box.GetCenter() if box is not None else fp.GetPosition()


def jlc_rotation(fp):
    angle = fp.GetOrientationDegrees()
    if fp.GetLayer() == pcbnew.B_Cu:
        angle = 180 - angle
    return (angle + ROTATION.get(fp.GetFPIDAsString().split(":")[-1], 0)) % 360


def bom_cpl():
    board = pcbnew.LoadBoard(PCB)
    origin = board.GetDesignSettings().GetAuxOrigin()
    rows, groups = [], {}
    for fp in board.GetFootprints():
        ref = fp.GetReference()
        if ref.startswith(("H", "G", "#")) or fp.IsDNP() or ref in HAND_SOLDERED:
            continue
        if fp.GetFPIDAsString().split(":")[-1].startswith("TestPoint_Pad"):
            continue  # a bare pad, no part
        field = fp.GetField("LCSC")
        lcsc = field.GetText() if field else ""
        if not lcsc:
            print(f"warning: {ref} has no LCSC number – not assembled")
            continue
        pos = pad_centre(fp)
        bottom = fp.GetLayer() == pcbnew.B_Cu
        rows.append([ref, f"{pcbnew.ToMM(pos.x - origin.x):.3f}mm", f"{-pcbnew.ToMM(pos.y - origin.y):.3f}mm",
                     "Bottom" if bottom else "Top", f"{jlc_rotation(fp):.0f}"])
        key = (fp.GetValue(), fp.GetFPIDAsString().split(":")[-1], lcsc)
        groups.setdefault(key, []).append(ref)
    bom = os.path.join(OUT, "doormutex-bom.csv")
    with open(bom, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f)
        w.writerow(["Comment", "Designator", "Footprint", "LCSC"])
        for (value, footprint, lcsc), refs in sorted(groups.items(), key=lambda kv: kv[1][0]):
            w.writerow([value, ",".join(sorted(refs, key=lambda r: (r.rstrip("0123456789"), int("0" + r.lstrip("ABCDEFGHIJKLMNOPQRSTUVWXYZ"))))), footprint, lcsc])
    cpl = os.path.join(OUT, "doormutex-cpl.csv")
    with open(cpl, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f)
        w.writerow(["Designator", "Mid X", "Mid Y", "Layer", "Rotation"])
        w.writerows(sorted(rows))
    return bom, cpl, len(rows), len(groups)


def main():
    if os.path.isdir(OUT):
        shutil.rmtree(OUT)
    os.makedirs(OUT)
    z, n = gerbers()
    bom, cpl, parts, lines = bom_cpl()
    print(f"{os.path.relpath(z)}: {n} files")
    print(f"{os.path.relpath(bom)}: {lines} lines, {os.path.relpath(cpl)}: {parts} placements")


if __name__ == "__main__":
    sys.exit(main())
