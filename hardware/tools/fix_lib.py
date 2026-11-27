# SPDX-FileCopyrightText: 2026 The doormutex contributors
# SPDX-License-Identifier: CERN-OHL-S-2.0
"""Fixes pin types in lib/doormutex.kicad_sym after an easyeda2kicad import.

EasyEDA symbols mark the pins of passive parts as inputs or power inputs,
which makes KiCad's ERC report undriven pins on nets that are fine. Run it
after re-importing a part:

    python3 hardware/tools/fix_lib.py
"""

import os
import re

LIB = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "lib", "doormutex.kicad_sym")

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
