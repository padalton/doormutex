# SPDX-FileCopyrightText: 2026 The doormutex contributors
# SPDX-License-Identifier: CERN-OHL-S-2.0
"""Routes hardware/doormutex.kicad_pcb with Freerouting and runs the DRC.

    python3 hardware/tools/gen_pcb.py && python3 hardware/tools/route_pcb.py

Exports the board as Specctra DSN (net classes from doormutex.kicad_pro),
lets Freerouting route it around the tracks gen_pcb.py has laid, imports the
session, pours ground on the outer layers, fills all zones, runs the DRC and
checks the copper resistance of the power paths (check_power.py).
Freerouting 2.x needs Java 25 or newer: set JAVA (default
/usr/lib/jvm/java-25-openjdk/bin/java) and FREEROUTING_JAR.
"""

import os
import subprocess
import sys
import tempfile

import pcbnew

HERE = os.path.dirname(os.path.abspath(__file__))
HW = os.path.dirname(HERE)
sys.path.insert(0, HERE)
import check_power  # noqa: E402

PCB = os.path.join(HW, "doormutex.kicad_pcb")
JAVA = os.environ.get("JAVA", "/usr/lib/jvm/java-25-openjdk/bin/java")
JAR = os.environ.get("FREEROUTING_JAR", "/usr/lib/freerouting/freerouting-executable.jar")
PASSES = os.environ.get("PASSES", "40")
NECK_UM = 250   # the narrowest Freerouting may make a track where the class width does not fit
LOG = os.environ.get("FREEROUTING_LOG")  # keep Freerouting's whole output in this file


def fill(board):
    pcbnew.ZONE_FILLER(board).Fill(board.Zones())


def ground_pour(board, layer):
    bb = board.GetBoardEdgesBoundingBox()
    z = pcbnew.ZONE(board)
    z.SetLayer(layer)
    z.SetNet(board.FindNet("GND"))
    o = z.Outline()
    o.NewOutline()
    for x, y in [(bb.GetLeft(), bb.GetTop()), (bb.GetRight(), bb.GetTop()), (bb.GetRight(), bb.GetBottom()), (bb.GetLeft(), bb.GetBottom())]:
        o.Append(x, y)
    z.SetMinThickness(pcbnew.FromMM(0.2))
    z.SetLocalClearance(pcbnew.FromMM(0.25))
    z.SetAssignedPriority(0)
    # SMD pads solid; through-hole pads (hand-soldered) with thermal reliefs
    z.SetPadConnection(pcbnew.ZONE_CONNECTION_THT_THERMAL)
    board.Add(z)


def edge_keepouts(board):
    """No tracks or vias along the board edges (Freerouting doesn't know the
    edge clearance): 0.4 mm all round, 1 mm at the top, where the ESP32's
    pads sit right below the edge (its antenna is past it)."""
    bb = board.GetBoardEdgesBoundingBox()
    l, t, r, b = bb.GetLeft(), bb.GetTop(), bb.GetRight(), bb.GetBottom()
    e, top = pcbnew.FromMM(0.4), pcbnew.FromMM(1.0)
    for x0, y0, x1, y1 in [(l, t, r, t + top), (l, b - e, r, b), (l, t, l + e, b), (r - e, t, r, b)]:
        z = pcbnew.ZONE(board)
        z.SetIsRuleArea(True)
        z.SetDoNotAllowTracks(True)
        z.SetDoNotAllowVias(True)
        z.SetDoNotAllowPads(False)
        z.SetDoNotAllowFootprints(False)
        z.SetDoNotAllowZoneFills(False)
        z.SetLayerSet(pcbnew.LSET.AllCuMask())
        o = z.Outline()
        o.NewOutline()
        for x, y in [(x0, y0), (x1, y0), (x1, y1), (x0, y1)]:
            o.Append(x, y)
        board.Add(z)


def fix_vias(board):
    """Freerouting returns a few vias with too little copper around the hole
    (0.6 mm with a 0.4 mm drill): shrink the drill, never the clearance."""
    sizes = {}
    for v in board.GetTracks():
        if v.GetClass() != "PCB_VIA":
            continue
        w, d = pcbnew.ToMM(v.GetWidth(pcbnew.F_Cu)), pcbnew.ToMM(v.GetDrillValue())
        sizes[(round(w, 2), round(d, 2))] = sizes.get((round(w, 2), round(d, 2)), 0) + 1
        if (w - d) / 2 < 0.13:
            v.SetDrill(pcbnew.FromMM(round(w - 0.3, 2)))
    print("vias (diameter, drill): count", sizes)


def drop_stubs(board):
    """Freerouting leaves the odd stub behind: a millimetre of track that ends
    on nothing, or lies inside a wide hand-laid track. The DRC that follows
    would report a connection that depended on one."""
    removed = 0
    while True:
        board.BuildConnectivity()
        conn = board.GetConnectivity()
        stubs = [t for t in board.GetTracks() if t.GetClass() == "PCB_TRACK" and not t.IsLocked()
                 and t.GetLength() < pcbnew.FromMM(2) and conn.TestTrackEndpointDangling(t, False)]
        if not stubs:
            break
        for t in stubs:
            board.Remove(t)
        removed += len(stubs)
    if removed:
        print(f"removed {removed} dangling stubs")


def vias_in_pads(board):
    """Vias of the router whose hole lies in a surface-mount pad, or whose
    copper touches one: the DRC has nothing against them, the reflow oven
    has. The vias gen_pcb.py placed (locked) are meant to be where they are."""
    holes, touching = [], []
    vias = [v for v in board.GetTracks() if v.GetClass() == "PCB_VIA" and not v.IsLocked()]
    for fp in board.GetFootprints():
        for pad in fp.Pads():
            if pad.GetAttribute() != pcbnew.PAD_ATTRIB_SMD:
                continue
            layer = pcbnew.B_Cu if pad.IsOnLayer(pcbnew.B_Cu) else pcbnew.F_Cu
            shape = pad.GetEffectivePolygon(layer, pcbnew.ERROR_INSIDE)
            for v in vias:
                name = f"{fp.GetReference()}.{pad.GetNumber()}"
                if shape.Collide(v.GetPosition(), v.GetDrillValue() // 2):
                    holes.append(name)
                elif shape.Collide(v.GetPosition(), v.GetWidth(layer) // 2):
                    touching.append(name)
    return holes, touching


def off_copper(board):
    """Texts cut into the solder mask (the name in bare copper) that do not lie
    on the ground pour all over: they would show bare board instead."""
    bad = []
    for d in board.GetDrawings():
        if d.GetClass() != "PCB_TEXT" or d.GetLayer() not in (pcbnew.F_Mask, pcbnew.B_Mask):
            continue
        layer = pcbnew.F_Cu if d.GetLayer() == pcbnew.F_Mask else pcbnew.B_Cu
        pours = [z for z in board.Zones() if not z.GetIsRuleArea() and z.GetNetname() == "GND" and z.IsOnLayer(layer)]
        bb = d.GetBoundingBox()
        step = pcbnew.FromMM(0.25)
        points = [pcbnew.VECTOR2I(x, y) for x in range(bb.GetLeft(), bb.GetRight() + 1, step) for y in range(bb.GetTop(), bb.GetBottom() + 1, step)]
        if not all(any(z.HitTestFilledArea(layer, p, 0) for z in pours) for p in points):
            bad.append(d.GetText())
    return bad


def main():
    board = pcbnew.LoadBoard(PCB)
    # inner layer 2 is the ground plane: not for routing
    board.SetLayerType(pcbnew.In2_Cu, pcbnew.LT_POWER)
    edge_keepouts(board)
    fill(board)
    with tempfile.TemporaryDirectory() as tmp:
        dsn, ses = os.path.join(tmp, "board.dsn"), os.path.join(tmp, "board.ses")
        if not pcbnew.ExportSpecctraDSN(board, dsn):
            sys.exit("DSN export failed")
        # The tracks and vias gen_pcb.py has laid are locked: KiCad exports
        # them as fixed wiring, Freerouting routes around them.
        # A connection that does not fit at its net class's width (0.5 mm
        # between the pins of a QFN) makes Freerouting 2.6 start over with a
        # narrower "neck" width for those nets – 0.15 mm unless told otherwise.
        # Freerouting takes the vias already on the board as its cue: four
        # thermal vias in the charger's pad and it puts vias into every
        # other pad as well, where the solder then runs down the hole. Say no.
        cmd = [JAVA, "-jar", JAR, "-de", dsn, "-do", ses, "-mp", PASSES, "--gui.enabled=false", f"--router.neck_width_um={NECK_UM}",
               "--router.autorouter.via_in_pad=false", "--router.autorouter.via_touching_pad=false"]
        print("freerouting:", " ".join(cmd[2:]))
        r = subprocess.run(cmd, capture_output=True, text=True, timeout=3600)
        log = (r.stdout + r.stderr).splitlines()
        if LOG:
            open(LOG, "w", encoding="utf-8").write("\n".join(log) + "\n")
        for line in log[-15:]:
            print("  " + line)
        if not os.path.exists(ses):
            sys.exit("no session file – routing failed")
        if not pcbnew.ImportSpecctraSES(board, ses):
            sys.exit("SES import failed")
    fix_vias(board)
    drop_stubs(board)
    for layer in (pcbnew.F_Cu, pcbnew.B_Cu):
        ground_pour(board, layer)
    fill(board)
    board.Save(PCB)
    tracks = [t for t in board.GetTracks() if t.GetClass() == "PCB_TRACK"]
    vias = [t for t in board.GetTracks() if t.GetClass() == "PCB_VIA"]
    print(f"routed: {len(tracks)} track segments, {len(vias)} vias")
    with tempfile.TemporaryDirectory() as tmp:
        rpt = os.path.join(tmp, "drc.rpt")
        subprocess.run(["kicad-cli", "pcb", "drc", "--severity-error", "-o", rpt, PCB], capture_output=True)
        errors = [l for l in open(rpt, encoding="utf-8") if l.startswith("[")]
        kinds = {}
        for l in errors:
            k = l.split("]")[0] + "]"
            kinds[k] = kinds.get(k, 0) + 1
        print(f"DRC: {len(errors)} errors", dict(sorted(kinds.items(), key=lambda kv: -kv[1])))
    ok = check_power.check(PCB)
    thin = sorted({t.GetNetname() for t in tracks if pcbnew.ToMM(t.GetWidth()) < 0.199})
    if thin:
        print("tracks below 0.2 mm on: " + ", ".join(thin))
    bare = off_copper(board)
    if bare:
        print("cut into the mask but not on the ground pour all over: " + ", ".join(bare))
    holes, touching = vias_in_pads(board)
    print(f"router vias with their hole in a pad: {len(holes)}, with their copper on a pad: {len(touching)}")
    if holes or touching:
        print("  " + ", ".join(sorted(set(holes + touching))))
    if errors or thin or bare or holes or touching or not ok:
        sys.exit(1)


if __name__ == "__main__":
    main()
