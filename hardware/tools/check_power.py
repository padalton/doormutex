# SPDX-FileCopyrightText: 2026 The doormutex contributors
# SPDX-License-Identifier: CERN-OHL-S-2.0
"""Checks the copper of the power paths in hardware/doormutex.kicad_pcb.

    python3 hardware/tools/check_power.py

For each path in PATHS it builds a resistor network from the tracks and vias
of that net (zones are left out, so ground is not checked here) and solves it
for the resistance between two pads. A path that is missing or above its
limit fails the check: an autorouter is happy with 0.2 mm on a thin inner
layer, the ESP32's 0.5 A Wi-Fi peaks and 1.5 A from USB are not.

Copper: 35 µm on the outer layers, 17.5 µm on JLCPCB's inner layers.
"""

import math
import os
import sys

import pcbnew

HERE = os.path.dirname(os.path.abspath(__file__))
PCB = os.path.join(os.path.dirname(HERE), "doormutex.kicad_pcb")

RHO = 1.72e-8  # Ω·m
THICKNESS = {"F.Cu": 35e-6, "B.Cu": 35e-6, "In1.Cu": 17.5e-6, "In2.Cu": 17.5e-6}
VIA = 0.0015   # Ω for a 0.3–0.4 mm via through the board
JOIN = 1e-6    # Ω between pieces of copper that touch

# net, from pad, to pad, current in A, limit in mΩ, what
PATHS = [
    ("VBUS", "J1.A4", "U2.13", 1.5, 30, "USB-C to the charger's IN"),
    ("VBUS", "J1.A9", "U2.13", 1.5, 30, "USB-C (second pin pair) to IN"),
    ("CELL_P", "BT1.2", "R5.1", 1.0, 10, "cell plus to the 0 Ω"),
    ("CELL_PM", "R5.2", "F1.1", 1.0, 5, "0 Ω to the PTC"),
    ("CELL_PF", "F1.2", "Q1.3", 1.0, 5, "PTC to Q1"),
    ("VBAT", "Q1.2", "U2.2", 1.0, 25, "Q1 to the charger's BAT"),
    ("CELL_N", "BT1.1", "Q2.1", 1.0, 15, "cell minus to the protection MOSFETs"),
    ("P_N", "Q2.3", "R9.1", 1.0, 5, "protection MOSFETs to the sense resistor"),
    ("SYS", "U2.10", "U6.10", 0.7, 25, "charger OUT to the buck-boost"),
    ("SW1", "U6.9", "L1.1", 1.0, 5, "buck-boost to the coil"),
    ("SW2", "U6.7", "L1.2", 1.0, 5, "coil to the buck-boost"),
    ("3V3", "U6.6", "C10.1", 0.5, 5, "buck-boost to its output capacitor"),
    ("3V3", "U6.6", "U7.3", 0.5, 60, "3.3 V to the ESP32"),
    ("3V3", "U6.6", "C12.1", 0.5, 60, "3.3 V to the bulk capacitor at the ESP32"),
]


def solve(matrix, rhs):
    """Gaussian elimination with partial pivoting (the networks are small)."""
    n = len(rhs)
    a = [row[:] + [rhs[i]] for i, row in enumerate(matrix)]
    for col in range(n):
        piv = max(range(col, n), key=lambda r: abs(a[r][col]))
        if abs(a[piv][col]) < 1e-18:
            return None
        a[col], a[piv] = a[piv], a[col]
        for r in range(col + 1, n):
            f = a[r][col] / a[col][col]
            if f:
                rowc = a[col]
                rowr = a[r]
                for k in range(col, n + 1):
                    rowr[k] -= f * rowc[k]
    x = [0.0] * n
    for r in range(n - 1, -1, -1):
        x[r] = (a[r][n] - sum(a[r][k] * x[k] for k in range(r + 1, n))) / a[r][r]
    return x


def resistance(board, netname, pad_a, pad_b):
    """Ohms between two pads over the tracks and vias of one net, None if they are not joined."""
    mm = pcbnew.ToMM
    nodes = {}

    def node(layer, x, y):
        return nodes.setdefault((layer, round(x, 3), round(y, 3)), len(nodes))

    edges, segments = [], []
    for t in board.GetTracks():
        if t.GetNetname() != netname:
            continue
        if t.GetClass() == "PCB_VIA":
            p = t.GetPosition()
            ids = [node(layer, mm(p.x), mm(p.y)) for layer in ("F.Cu", "In1.Cu", "In2.Cu", "B.Cu")]
            for i, j in zip(ids, ids[1:]):
                edges.append((i, j, VIA / 3))
            continue
        s, e, layer = t.GetStart(), t.GetEnd(), board.GetLayerName(t.GetLayer())
        length = math.hypot(mm(e.x - s.x), mm(e.y - s.y))
        if length == 0:
            continue
        seg = (layer, mm(s.x), mm(s.y), mm(e.x), mm(e.y), mm(t.GetWidth()), length)
        segments.append(seg)
    for layer, x1, y1, x2, y2, w, length in segments:
        node(layer, x1, y1)
        node(layer, x2, y2)
    # every node that lies on a segment splits it there (T-junctions, vias on a track)
    for layer, x1, y1, x2, y2, w, length in segments:
        on = []
        for (nl, x, y), i in nodes.items():
            if nl != layer:
                continue
            u = ((x - x1) * (x2 - x1) + (y - y1) * (y2 - y1)) / (length * length)
            u = min(1.0, max(0.0, u))
            if math.hypot(x1 + u * (x2 - x1) - x, y1 + u * (y2 - y1) - y) < w / 2 - 0.01:
                on.append((u, i))
        on.sort()
        ohm_per_mm = RHO * 1e-3 / (w * 1e-3 * THICKNESS[layer])
        for (u0, i), (u1, j) in zip(on, on[1:]):
            if i != j:
                edges.append((i, j, max(JOIN, (u1 - u0) * length * ohm_per_mm)))
    # pads join whatever ends on them, through-hole pads on every layer
    terminal = {}
    for fp in board.GetFootprints():
        for pad in fp.Pads():
            if pad.GetNetname() != netname:
                continue
            box = pad.GetBoundingBox()
            x0, y0, x1, y1 = mm(box.GetLeft()), mm(box.GetTop()), mm(box.GetRight()), mm(box.GetBottom())
            name = f"{fp.GetReference()}.{pad.GetNumber()}"
            hub = terminal.get(name)
            for (layer, x, y), i in list(nodes.items()):
                if layer in THICKNESS and pad.IsOnLayer(board.GetLayerID(layer)) and x0 <= x <= x1 and y0 <= y <= y1:
                    if hub is None:
                        hub = terminal[name] = node("pad:" + name, 0, 0)
                    edges.append((hub, i, JOIN))
    a, b = terminal.get(pad_a), terminal.get(pad_b)
    if a is None or b is None:
        return None
    # keep the part of the network that hangs on a, then solve with b grounded
    adj = {}
    for i, j, r in edges:
        adj.setdefault(i, []).append(j)
        adj.setdefault(j, []).append(i)
    seen, todo = {a}, [a]
    while todo:
        for k in adj.get(todo.pop(), []):
            if k not in seen:
                seen.add(k)
                todo.append(k)
    if b not in seen:
        return None
    index = {k: n for n, k in enumerate(sorted(seen - {b}))}
    g = [[0.0] * len(index) for _ in index]
    for i, j, r in edges:
        if i not in seen or j not in seen:
            continue
        for p, q in ((i, j), (j, i)):
            if p in index:
                g[index[p]][index[p]] += 1 / r
                if q in index:
                    g[index[p]][index[q]] -= 1 / r
    rhs = [0.0] * len(index)
    rhs[index[a]] = 1.0
    v = solve(g, rhs)
    return None if v is None else v[index[a]]


def check(pcb=PCB):
    board = pcbnew.LoadBoard(pcb)
    ok = True
    print("power paths (tracks and vias only):")
    for netname, a, b, amps, limit, what in PATHS:
        r = resistance(board, netname, a, b)
        if r is None:
            print(f"  FAIL {netname:8} {a:>6} -> {b:<6} not joined by tracks – {what}")
            ok = False
            continue
        good = r * 1e3 <= limit
        ok = ok and good
        print(f"  {'ok  ' if good else 'FAIL'} {netname:8} {a:>6} -> {b:<6} {r * 1e3:6.1f} mΩ (limit {limit}), "
              f"{r * amps * 1e3:5.1f} mV at {amps} A – {what}")
    return ok


if __name__ == "__main__":
    sys.exit(0 if check(sys.argv[1] if len(sys.argv) > 1 else PCB) else 1)
