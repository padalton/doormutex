#!/usr/bin/env python3
"""Prepares release files that aren't Go builds; run by GoReleaser.

    packaging/prepare.py VERSION

Writes to build/ (gitignored):
  plasmoid/                        the widget package with VERSION in metadata.json
  doormutex-VERSION.plasmoid       the same as a zip – for the KDE Store and
                                   `kpackagetool6 -i`
  systemd/doormutex-companion.service
                                   the user unit for distribution packages
                                   (binary in /usr/bin instead of ~/.local/bin)
"""

import json
import pathlib
import shutil
import sys
import zipfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
OUT = ROOT / "build"


def main() -> None:
    if len(sys.argv) != 2:
        sys.exit("usage: prepare.py VERSION")
    version = sys.argv[1].removeprefix("v")
    shutil.rmtree(OUT, ignore_errors=True)
    OUT.mkdir()

    # Widget: copy, stamp the version, zip.
    pkg = OUT / "plasmoid"
    shutil.copytree(ROOT / "plasmoid" / "package", pkg)
    meta_path = pkg / "metadata.json"
    meta = json.loads(meta_path.read_text())
    meta["KPlugin"]["Version"] = version
    meta_path.write_text(json.dumps(meta, indent=4, ensure_ascii=False) + "\n")
    with zipfile.ZipFile(OUT / f"doormutex-{version}.plasmoid", "w", zipfile.ZIP_DEFLATED) as z:
        for f in sorted(pkg.rglob("*")):
            if f.is_file():
                z.write(f, f.relative_to(pkg).as_posix())

    # Companion unit for packages.
    unit = (ROOT / "companion" / "doormutex-companion.service").read_text()
    packaged = unit.replace("%h/.local/bin/doormutex-companion", "/usr/bin/doormutex-companion")
    if packaged == unit:
        sys.exit("prepare.py: ExecStart in companion/doormutex-companion.service has changed – update the replacement")
    packaged = "\n".join(l for l in packaged.splitlines() if not l.startswith("#   ") and l != "# systemd user unit – install with:")
    (OUT / "systemd").mkdir()
    (OUT / "systemd" / "doormutex-companion.service").write_text(
        "# systemd user unit from the doormutex-companion package:\n"
        "#   systemctl --user enable --now doormutex-companion\n" + packaged + "\n")


if __name__ == "__main__":
    main()
