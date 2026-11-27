# SPDX-FileCopyrightText: 2026 The doormutex contributors
# SPDX-License-Identifier: CERN-OHL-S-2.0
"""Creates hardware/doormutex.kicad_pcb from design.py: outline, layers,
footprints with their nets, and a first placement by zones.

    python3 hardware/tools/gen_pcb.py

The board is the back wall of the light's three chambers (enclosure,
z = height above the bottom of the light):

  * front (F.Cu, towards the lenses): the LEDs in the middle of each
    reflector cone, the e-paper socket and the light sensor in the sign;
  * back (B.Cu, towards the back cover): everything else – the ESP32 at
    the top with its antenna past the top edge, the 18650 holder in the
    middle, power at the bottom with USB-C and the INFO button on the
    bottom edge.

Routing is not done here (see README). Running it again replaces the board,
so placement tweaks belong in PLACE below, not in the PCB file.
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

# Every part: ref -> (x, y, rotation, side), side "F" (front, towards the
# lenses) or "B" (back). x, y in board coordinates seen from the front; a
# part on the back is mirrored by KiCad, its (x, y) is still the centre.
B, F = "B", "F"
PLACE = {
    # -- top of the back: ESP32, its antenna past the top edge (5.7 mm of
    # module above the pads) into a plastic-only roof
    "U7": (X_MID, 6.2, 180, B),
    "C13": (29.0, 2.3, 0, B), "C12": (29.6, 5.0, 0, B),        # at 3V3 (pin 3)
    "R14": (29.0, 8.2, 0, B), "C14": (29.0, 9.7, 0, B),        # EN
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
    # -- right of the holder, at its minus end (top): protection and gauge
    "U3": (35.0, 29.5, 0, B), "R7": (38.9, 28.0, 90, B), "C5": (38.9, 30.8, 90, B), "R8": (38.9, 33.6, 90, B),
    "Q2": (35.0, 35.0, 0, B), "R9": (35.0, 39.6, 90, B),
    "U4": (35.0, 45.0, 0, B), "C6": (38.9, 44.0, 90, B), "C7": (38.9, 46.8, 90, B), "R18": (32.4, 48.6, 90, B),
    # -- holder: plus at the bottom, next to the charger (rotation chosen below)
    "BT1": (X_MID, 70.0, 270, B),
    # -- below the holder, right: the cell's plus lead
    "R5": (34.5, 117.6, 0, B), "J3": (34.5, 121.2, 90, B), "F1": (34.5, 125.4, 0, B),
    "Q1": (34.0, 130.0, 0, B), "R6": (37.9, 130.0, 90, B),
    # -- bottom left: charger
    "U2": (7.0, 150.0, 0, B), "R3": (3.0, 145.4, 90, B), "R4": (4.5, 145.4, 90, B),
    "C1": (11.4, 146.0, 90, B), "C2": (11.4, 149.6, 90, B), "C3": (11.4, 153.4, 90, B), "C4": (3.0, 154.6, 90, B),
    # -- bottom right: supervisor and buck-boost
    "U5": (32.2, 136.0, 0, B), "R10": (36.0, 134.0, 90, B), "C8": (37.6, 134.0, 90, B), "R11": (36.0, 138.4, 90, B),
    "U6": (32.0, 150.0, 0, B), "L1": (32.0, 146.0, 0, B), "C9": (28.2, 150.0, 90, B),
    "C10": (36.0, 150.0, 90, B), "C11": (37.8, 150.0, 90, B), "R12": (35.6, 154.0, 0, B), "R13": (35.6, 155.6, 0, B),
    # -- bottom edge: USB-C (opening flush with the edge), ESD, INFO button
    "J1": (X_MID, L - 4.9, 180, B), "U1": (X_MID, L - 13.0, 0, B), "R1": (14.0, L - 14.0, 90, B), "R2": (27.0, L - 14.0, 90, B),
    "SW3": (30.0, L - 2.1, 180, B), "R17": (30.0, L - 6.1, 0, B),
    # -- front: LEDs in the middle of the cones, e-paper socket and its
    # fallback header in the sign, the light sensor in a corner between two
    # chambers (a light channel in the front shell)
    # J2: pins at the top, opening down towards the module's plug below it
    "J2": (X_MID, y_of(SIGN_H - 1.5), 180, F), "C18": (31.0, y_of(SIGN_H - 3.0), 90, F),
    "U8": (3.5, 87.0, 90, F), "C17": (3.5, 91.6, 90, F),
}
for i, col in enumerate("RYG"):
    PLACE[f"D{2 * i + 1}"] = (X_MID - 1.5, LAMP_Y[col], 90, F)
    PLACE[f"D{2 * i + 2}"] = (X_MID + 1.5, LAMP_Y[col], 90, F)
# Parts whose PLACE point is the centre of their outline, not their origin
CENTRED = {"J4", "J3", "BT1"}
ZONES = []
# Next to the cones, away from the e-paper module; the top two are 11 mm
# from the antenna – nylon screws there.
MOUNT = [(3.0, 6.0), (W - 3.0, 6.0), (3.0, 113.0), (W - 3.0, 113.0)]


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


def place_in_zone(board_fps, zone):
    """Packs the zone's footprints in rows, left to right, by courtyard size."""
    name, x0, y0, x1, y1, refs = zone
    x, y, row_h = x0, y0, 0.0
    for ref in refs:
        fp = board_fps.get(ref)
        if fp is None:
            continue
        fp.SetLayerAndFlip(pcbnew.B_Cu)
        bb = fp.GetBoundingBox(False)
        w, h = pcbnew.ToMM(bb.GetWidth()) + 0.8, pcbnew.ToMM(bb.GetHeight()) + 0.8
        if x + w > x1 and x > x0:
            x, y = x0, y + row_h
            row_h = 0.0
        if y + h > y1:
            print(f"warning: zone {name!r} is full at {ref}")
        # move so the bounding box starts at (x, y)
        dx = mm(ORIGIN[0] + x) - bb.GetLeft()
        dy = mm(ORIGIN[1] + y) - bb.GetTop()
        fp.Move(pcbnew.VECTOR2I(dx, dy))
        x += w
        row_h = max(row_h, h)


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

    for ref, (x, y, rot, side) in PLACE.items():
        fp = fps[ref]
        if side == "B":
            fp.SetLayerAndFlip(pcbnew.B_Cu)
        fp.SetOrientationDegrees(rot)
        fp.SetPosition(vec(x, y))
        if ref in CENTRED:
            c = fp.GetBoundingBox(False).GetCenter()
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

    # Ground on the inner layer 1, everywhere
    zone = pcbnew.ZONE(board)
    zone.SetLayer(pcbnew.In1_Cu)
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
