# SPDX-FileCopyrightText: 2026 The doormutex contributors
# SPDX-License-Identifier: CERN-OHL-S-2.0
"""Writes hardware/doormutex.kicad_sch from design.py and checks it.

    python3 hardware/tools/gen_schematic.py

Every part gets a short wire and a net label on each pin (no-connect flags
on the free ones), grouped by block. Afterwards kicad-cli runs the ERC and
exports the netlist, and the script compares it with design.py: what the
schematic connects must be exactly what the design says.
"""

import os
import subprocess
import sys
import tempfile
import uuid as uuidlib
import xml.etree.ElementTree as ET

HERE = os.path.dirname(os.path.abspath(__file__))
HW = os.path.dirname(HERE)
sys.path.insert(0, HERE)

import design  # noqa: E402
from sexpr import Sym, dump, find, find_all, parse  # noqa: E402

KICAD_SYMBOLS = os.environ.get("KICAD_SYMBOL_DIR", "/usr/share/kicad/symbols")
PROJECT = "doormutex"
OUT = os.path.join(HW, "doormutex.kicad_sch")

# Stable UUIDs: derived from names, so a regenerated file only changes where
# the design does.
NS = uuidlib.UUID("6b1d3c56-2f0e-4c62-9d3e-6f6d7a6e7865")


def uid(*names):
    return str(uuidlib.uuid5(NS, "/".join(names)))


ROOT = uid("root")

# ---- symbol libraries

_libs = {}


def load_lib(name):
    if name not in _libs:
        path = os.path.join(HW, "lib", "doormutex.kicad_sym") if name == "doormutex" else os.path.join(KICAD_SYMBOLS, name + ".kicad_sym")
        _libs[name] = parse(open(path, encoding="utf-8").read())
    return _libs[name]


def lib_symbol(lib_id):
    """The symbol definition, with 'extends' resolved, named lib_id."""
    lib, name = lib_id.split(":")
    syms = {s[1]: s for s in find_all(load_lib(lib), "symbol")}
    s = syms[name]
    ext = find(s, "extends")
    if ext:
        base = syms[ext[1]]
        props = {p[1]: p for p in find_all(s, "property")}
        out = [Sym("symbol"), lib_id]
        for c in base[2:]:
            if isinstance(c, list) and c and c[0] == "property" and c[1] in props:
                out.append(props.pop(c[1]))
            elif isinstance(c, list) and c and c[0] == "symbol":
                out.append([c[0], c[1].replace(ext[1], name, 1)] + c[2:])
            else:
                out.append(c)
        out.extend(props.values())
        return out
    return [s[0], lib_id] + s[2:]


def pins_of(sym):
    out = []

    def walk(node, unit):
        for c in node:
            if not (isinstance(c, list) and c):
                continue
            if c[0] == "symbol":
                walk(c, int(c[1].rsplit("_", 2)[-2]))
            elif c[0] == "pin":
                at = find(c, "at")
                out.append(dict(number=find(c, "number")[1], name=find(c, "name")[1],
                                x=float(at[1]), y=float(at[2]), angle=float(at[3]) if len(at) > 3 else 0.0, unit=unit))

    walk(sym, 0)
    return out


def body_box(sym, pins):
    xs = [p["x"] for p in pins] or [0.0]
    ys = [p["y"] for p in pins] or [0.0]

    def walk(node):
        for c in node:
            if isinstance(c, list) and c:
                if c[0] in ("start", "end", "center", "xy"):
                    xs.append(float(c[1]))
                    ys.append(float(c[2]))
                elif c[0] not in ("property",):
                    walk(c)

    walk([x for x in sym if not (isinstance(x, list) and x and x[0] == "property")])
    return min(xs), min(ys), max(xs), max(ys)


def prop(sym, name):
    p = [x for x in find_all(sym, "property") if x[1] == name]
    return p[0][2] if p else ""


# ---- layout

STUB = 2.54  # wire from the pin end to the label
CHAR = 1.15  # label character width at 1.27 mm
GRID = 2.54


def snap(v):
    return round(v / GRID) * GRID


def resolve_pins(p, pins):
    by_num = {q["number"]: q for q in pins}
    by_name = {}
    for q in pins:
        by_name.setdefault(q["name"], []).append(q)
    out = {}
    for key, net in p["pins"].items():
        if key in by_num:
            out[key] = net
        elif len(by_name.get(key, [])) == 1:
            out[by_name[key][0]["number"]] = net
        else:
            sys.exit(f"{p['ref']}: no pin {key!r} in {p['lib_id']}")
    missing = [q["number"] for q in pins if q["number"] not in out]
    if missing and not p["ref"].startswith("#"):
        sys.exit(f"{p['ref']}: pins without a net or NC in design.py: {missing}")
    return out


def outward(angle):
    """Direction away from the body for a pin, in schematic coordinates."""
    return {0: (-1, 0), 90: (0, 1), 180: (1, 0), 270: (0, -1)}[int(angle) % 360]


def label_len(net):
    return len(net) * CHAR + 2


def footprint_of(p, sym):
    return p["footprint"] or prop(sym, "Footprint")


def build():
    syms = {}
    items = []
    placed = []
    page_w, x0, y = 800.0, 15.0, 25.0
    blocks = []
    for p in design.parts:
        if p["block"] not in blocks:
            blocks.append(p["block"])
    for block in blocks:
        items.append([Sym("text"), block, [Sym("exclude_from_sim"), Sym("no")], [Sym("at"), x0, y, 0],
                      [Sym("effects"), [Sym("font"), [Sym("size"), 3, 3], [Sym("bold"), Sym("yes")]], [Sym("justify"), Sym("left"), Sym("bottom")]],
                      [Sym("uuid"), uid("text", block)]])
        y += 8
        x, row_h = x0, 0.0
        for p in [q for q in design.parts if q["block"] == block]:
            if p["lib_id"] not in syms:
                syms[p["lib_id"]] = lib_symbol(p["lib_id"])
            sym = syms[p["lib_id"]]
            pins = pins_of(sym)
            nets = resolve_pins(p, pins)
            bx0, by0, bx1, by1 = body_box(sym, pins)
            left = max([label_len(nets[q["number"]] or "") for q in pins if q["angle"] == 0 and nets.get(q["number"])] + [0])
            right = max([label_len(nets[q["number"]] or "") for q in pins if q["angle"] == 180 and nets.get(q["number"])] + [0])
            top = max([label_len(nets[q["number"]] or "") for q in pins if q["angle"] == 270 and nets.get(q["number"])] + [0])
            bottom = max([label_len(nets[q["number"]] or "") for q in pins if q["angle"] == 90 and nets.get(q["number"])] + [0])
            w = (bx1 - bx0) + left + right + 2 * STUB + 6
            h = (by1 - by0) + top + bottom + 2 * STUB + 8
            if x + w > page_w and x > x0:
                x, y = x0, y + row_h
                row_h = 0.0
            # symbol origin so that the box starts at (x, y); schematic y grows down
            ox = snap(x + left + STUB - bx0 + 4)
            oy = snap(y + top + STUB + by1 + 6)
            placed.append((p, sym, pins, nets, ox, oy, (bx0, by0, bx1, by1)))
            x += w
            row_h = max(row_h, h)
        y += row_h + 5
    return syms, items, placed, y


def symbol_instance(p, sym, pins, ox, oy, box):
    bx0, by0, bx1, by1 = box
    ref, value = p["ref"], p["value"]
    power = p["lib_id"].startswith("power:")
    hidden = [Sym("hide"), Sym("yes")]

    def field(name, val, dx, dy, hide=False, left=False):
        eff = [Sym("effects"), [Sym("font"), [Sym("size"), 1.27, 1.27]]]
        if left:
            eff.append([Sym("justify"), Sym("left")])
        if hide:
            eff.append(hidden)
        return [Sym("property"), name, val, [Sym("at"), ox + dx, oy + dy, 0], eff]

    # Parts standing upright (pins only at the top and bottom) get their
    # reference and value beside the body; the net labels sit above and below.
    upright = pins and all(int(q["angle"]) % 180 == 90 for q in pins)
    if upright:
        ref_field = field("Reference", ref, bx1 + 1.5, -(by0 + by1) / 2 - 1.3, hide=power, left=True)
        val_field = field("Value", value, bx1 + 1.5, -(by0 + by1) / 2 + 1.3, left=True)
    else:
        ref_field = field("Reference", ref, (bx0 + bx1) / 2, -by1 - 2.5, hide=power)
        val_field = field("Value", value, (bx0 + bx1) / 2, -by0 + 3.5)

    node = [Sym("symbol"), [Sym("lib_id"), p["lib_id"]], [Sym("at"), ox, oy, 0], [Sym("unit"), 1],
            [Sym("exclude_from_sim"), Sym("no")], [Sym("in_bom"), Sym("no") if power else Sym("yes")],
            [Sym("on_board"), Sym("no") if power else Sym("yes")], [Sym("dnp"), Sym("yes") if p["dnp"] else Sym("no")],
            [Sym("uuid"), uid("sym", ref)],
            ref_field,
            val_field,
            field("Footprint", footprint_of(p, sym), 0, 0, hide=True),
            field("Datasheet", prop(sym, "Datasheet"), 0, 0, hide=True),
            field("Description", prop(sym, "Description"), 0, 0, hide=True)]
    if p["lcsc"]:
        node.append(field("LCSC", p["lcsc"], 0, 0, hide=True))
    for q in pins:
        node.append([Sym("pin"), q["number"], [Sym("uuid"), uid("pin", ref, q["number"])]])
    node.append([Sym("instances"), [Sym("project"), PROJECT,
                                    [Sym("path"), "/" + ROOT, [Sym("reference"), ref], [Sym("unit"), 1]]]])
    return node


def connections(p, pins, nets, ox, oy):
    out = []
    for q in pins:
        px, py = ox + q["x"], oy - q["y"]
        net = nets.get(q["number"])
        key = (p["ref"], q["number"])
        if net is None:
            if not p["ref"].startswith("#"):
                out.append([Sym("no_connect"), [Sym("at"), px, py], [Sym("uuid"), uid("nc", *key)]])
            continue
        dx, dy = outward(q["angle"])
        ex, ey = px + dx * STUB, py + dy * STUB
        out.append([Sym("wire"), [Sym("pts"), [Sym("xy"), px, py], [Sym("xy"), ex, ey]],
                    [Sym("stroke"), [Sym("width"), 0], [Sym("type"), Sym("default")]], [Sym("uuid"), uid("wire", *key)]])
        angle = {(-1, 0): 180, (1, 0): 0, (0, -1): 90, (0, 1): 270}[(dx, dy)]
        justify = [Sym("justify"), Sym("right") if angle in (180, 270) else Sym("left"), Sym("bottom")]
        out.append([Sym("label"), net, [Sym("at"), ex, ey, angle], [Sym("fields_autoplaced"), Sym("yes")],
                    [Sym("effects"), [Sym("font"), [Sym("size"), 1.27, 1.27]], justify], [Sym("uuid"), uid("label", *key)]])
    return out


def write():
    syms, items, placed, height = build()
    lib_symbols = [Sym("lib_symbols")] + [syms[k] for k in sorted(syms)]
    body = [Sym("kicad_sch"), [Sym("version"), Sym("20250114")], [Sym("generator"), "doormutex_gen_schematic"],
            [Sym("generator_version"), "1"], [Sym("uuid"), ROOT], [Sym("paper"), "A1"],
            [Sym("title_block"), [Sym("title"), "doormutex light – PCB v1"], [Sym("company"), "The doormutex contributors"],
             [Sym("comment"), 1, "Generated by hardware/tools/gen_schematic.py from design.py – edit design.py, not this file."],
             [Sym("comment"), 2, "CERN-OHL-S-2.0"]],
            lib_symbols]
    body += items
    for p, sym, pins, nets, ox, oy, box in placed:
        body.append(symbol_instance(p, sym, pins, ox, oy, box))
        body += connections(p, pins, nets, ox, oy)
    body.append([Sym("sheet_instances"), [Sym("path"), "/", [Sym("page"), "1"]]])
    with open(OUT, "w", encoding="utf-8") as f:
        f.write(dump(body) + "\n")
    if height > 580:
        print(f"warning: the layout is {height:.0f} mm tall, more than A1")
    return placed


# ---- checks

def check(placed):
    ok = True
    with tempfile.TemporaryDirectory() as tmp:
        net_xml = os.path.join(tmp, "net.xml")
        erc = os.path.join(tmp, "erc.rpt")
        subprocess.run(["kicad-cli", "sch", "export", "netlist", "--format", "kicadxml", "-o", net_xml, OUT], check=True,
                       capture_output=True)
        r = subprocess.run(["kicad-cli", "sch", "erc", "--severity-error", "-o", erc, OUT], capture_output=True, text=True)
        report = open(erc, encoding="utf-8").read()
        errors = [l for l in report.splitlines() if l.startswith("[")]
        print(f"ERC: {len(errors)} errors")
        for l in errors[:30]:
            print("  " + l)
        ok = ok and not errors
        got = {}
        for net in ET.parse(net_xml).getroot().iter("net"):
            for node in net.iter("node"):
                got[(node.get("ref"), node.get("pin"))] = net.get("name").lstrip("/")
    for p, sym, pins, nets, *_ in placed:
        if p["ref"].startswith("#"):
            continue
        for num, net in nets.items():
            have = got.get((p["ref"], num))
            if net is None:
                continue
            if have != net:
                print(f"netlist: {p['ref']}.{num} is on {have!r}, design says {net!r}")
                ok = False
    # Each design net must be one net in the schematic (labels by name do that,
    # but a typo in a net name would make a net with one pin).
    count = {}
    for p in design.parts:
        for net in p["pins"].values():
            if net:
                count[net] = count.get(net, 0) + 1
    single = [n for n, k in count.items() if k == 1]
    if single:
        print("nets with a single pin (typo?):", ", ".join(sorted(single)))
        ok = False
    print("netlist matches design.py" if ok else "CHECK FAILED")
    return ok


if __name__ == "__main__":
    placed = write()
    print(f"wrote {os.path.relpath(OUT)}: {len(placed)} symbols")
    sys.exit(0 if check(placed) else 1)
