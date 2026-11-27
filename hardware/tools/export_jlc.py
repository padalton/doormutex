# SPDX-FileCopyrightText: 2026 The doormutex contributors
# SPDX-License-Identifier: CERN-OHL-S-2.0
"""Writes the JLCPCB order files to hardware/production/.

    python3 hardware/tools/export_jlc.py

  doormutex-gerbers.zip   Gerber + drill files (4 layers)
  doormutex-bom.csv       Comment, Designator, Footprint, LCSC – what JLCPCB places
  doormutex-cpl.csv       Designator, Mid X, Mid Y, Layer, Rotation

Parts that are not fitted (DNP) or hand-soldered through-hole parts (the
e-paper socket) are left out of BOM and CPL. Check the placement preview in
JLCPCB's order page: rotations of parts from different libraries can differ
from JLCPCB's own models.
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


def gerbers():
    with tempfile.TemporaryDirectory() as tmp:
        layers = "F.Cu,In1.Cu,In2.Cu,B.Cu,F.Paste,B.Paste,F.Silkscreen,B.Silkscreen,F.Mask,B.Mask,Edge.Cuts"
        subprocess.run(["kicad-cli", "pcb", "export", "gerbers", "--layers", layers, "--subtract-soldermask",
                        "--no-protel-ext", "-o", tmp + "/", PCB], check=True, capture_output=True)
        subprocess.run(["kicad-cli", "pcb", "export", "drill", "--format", "excellon", "--excellon-separate-th",
                        "--generate-map", "--map-format", "gerberx2", "-o", tmp + "/", PCB], check=True, capture_output=True)
        path = os.path.join(OUT, "doormutex-gerbers.zip")
        with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED) as z:
            for f in sorted(os.listdir(tmp)):
                z.write(os.path.join(tmp, f), f)
        return path, len(os.listdir(tmp))


def bom_cpl():
    board = pcbnew.LoadBoard(PCB)
    origin = board.GetDesignSettings().GetAuxOrigin()
    rows, groups = [], {}
    for fp in board.GetFootprints():
        ref = fp.GetReference()
        if ref.startswith(("H", "#")) or fp.IsDNP() or ref in HAND_SOLDERED:
            continue
        field = fp.GetField("LCSC")
        lcsc = field.GetText() if field else ""
        if not lcsc:
            print(f"warning: {ref} has no LCSC number – not assembled")
            continue
        pos = fp.GetPosition()
        bottom = fp.GetLayer() == pcbnew.B_Cu
        rows.append([ref, f"{pcbnew.ToMM(pos.x - origin.x):.3f}mm", f"{-pcbnew.ToMM(pos.y - origin.y):.3f}mm",
                     "Bottom" if bottom else "Top", f"{fp.GetOrientationDegrees() % 360:.0f}"])
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
