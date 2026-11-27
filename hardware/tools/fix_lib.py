# SPDX-FileCopyrightText: 2026 The doormutex contributors
# SPDX-License-Identifier: CERN-OHL-S-2.0
"""Fixes what an easyeda2kicad import gets wrong in lib/. Run it after
re-importing a part:

    python3 hardware/tools/fix_lib.py

Symbols: EasyEDA marks the pins of passive parts as inputs or power inputs,
which makes KiCad's ERC report undriven pins on nets that are fine.

Footprints: see FOOTPRINT below.
"""

import os
import re

LIBDIR = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "lib")
LIB = os.path.join(LIBDIR, "doormutex.kicad_sym")

# Symbols whose pins are all passive (ESD diodes, resistors, NTC, switch, cell holder).
PASSIVE = ["USBLC6-2SC6", "NCP18XH103F03RB", "1206W4F0000T5E", "SKRTLAE010", "1042"]

text = open(LIB, encoding="utf-8").read()
for name in PASSIVE:
    start = text.index(f'(symbol "{name}"')
    # the symbol ends where the next top-level symbol starts (or the library ends)
    nxt = text.find('\n  (symbol "', start + 1)
    end = nxt if nxt != -1 else len(text)
    block = re.sub(r"\(pin (input|power_in|output|bidirectional|unspecified) ", "(pin passive ", text[start:end])
    text = text[:start] + block + text[end:]
open(LIB, "w", encoding="utf-8").write(text)
print("pins set to passive:", ", ".join(PASSIVE))

# Footprint corrections: name -> [(as imported, corrected)].
#
# The cell holder (Keystone 1042) sits on three pegs: two snap-in ones and a
# 1.57 mm locating pin in a 2.4 mm hole. EasyEDA's footprint has that hole
# 35.80 mm from the centre; the pin is at 36.13 mm in EasyEDA's own 3D model
# of the part and in KiCad's footprint of it
# (Battery:BatteryHolder_Keystone_1042_1x18650). With the hole 0.33 mm off,
# 0.09 mm of the 0.42 mm play around the pin would be left on one side.
FOOTPRINT = {
    "BAT-SMD_1042P": [
        ("(at 35.80 8.00) (size 2.40 2.40)", "(at 36.13 8.00) (size 2.40 2.40)"),
        ("(center 35.80 8.00) (end 36.20 8.00)", "(center 36.13 8.00) (end 36.53 8.00)"),
    ],
}
for name, fixes in FOOTPRINT.items():
    path = os.path.join(LIBDIR, "doormutex.pretty", name + ".kicad_mod")
    fp = open(path, encoding="utf-8").read()
    for old, new in fixes:
        if old in fp:
            fp = fp.replace(old, new)
        elif new not in fp:
            raise SystemExit(f"{name}: neither {old!r} nor its correction found - has the footprint changed?")
    open(path, "w", encoding="utf-8").write(fp)
    print("footprint corrected:", name)
