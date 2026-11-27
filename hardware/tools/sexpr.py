# SPDX-FileCopyrightText: 2026 The doormutex contributors
# SPDX-License-Identifier: CERN-OHL-S-2.0
"""A small S-expression reader and writer for KiCad files."""


class Sym(str):
    """An unquoted atom (keyword or number) – written without quotes."""


def parse(text):
    pos = 0
    n = len(text)

    def skip():
        nonlocal pos
        while pos < n and text[pos] in " \t\r\n":
            pos += 1

    def read():
        nonlocal pos
        skip()
        c = text[pos]
        if c == "(":
            pos += 1
            out = []
            while True:
                skip()
                if text[pos] == ")":
                    pos += 1
                    return out
                out.append(read())
        if c == '"':
            pos += 1
            buf = []
            while text[pos] != '"':
                if text[pos] == "\\":
                    pos += 1
                    buf.append({"n": "\n", "t": "\t"}.get(text[pos], text[pos]))
                else:
                    buf.append(text[pos])
                pos += 1
            pos += 1
            return "".join(buf)
        start = pos
        while pos < n and text[pos] not in " \t\r\n()":
            pos += 1
        return Sym(text[start:pos])

    return read()


def dump(node, indent=0):
    if isinstance(node, list):
        if not node:
            return "()"
        inner = [dump(x, indent + 1) for x in node]
        flat = "(" + " ".join(inner) + ")"
        if len(flat) < 100 and "\n" not in flat:
            return flat
        pad = "\n" + "\t" * (indent + 1)
        return "(" + inner[0] + "".join(pad + s for s in inner[1:]) + "\n" + "\t" * indent + ")"
    if isinstance(node, Sym):
        return str(node)
    if isinstance(node, bool):
        return "yes" if node else "no"
    if isinstance(node, (int, float)):
        s = f"{node:.4f}".rstrip("0").rstrip(".")
        return s if s not in ("-0", "") else "0"
    s = str(node).replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n")
    return f'"{s}"'


def find(node, key):
    """The first child list starting with key."""
    for c in node:
        if isinstance(c, list) and c and c[0] == key:
            return c
    return None


def find_all(node, key):
    return [c for c in node if isinstance(c, list) and c and c[0] == key]
