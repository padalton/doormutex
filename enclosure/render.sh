#!/bin/sh
# SPDX-FileCopyrightText: 2026 The doormutex contributors
# SPDX-License-Identifier: CERN-OHL-S-2.0
#
# Renders every printable part to STL (in print orientation) and the
# assembly views to PNG. Any OpenSCAD warning fails the run.
#
#   enclosure/render.sh [output dir]      (default: enclosure/build)
#   OPENSCAD=/path/to/openscad enclosure/render.sh
set -eu

cd "$(dirname "$0")"
out=${1:-build}
mkdir -p "$out"
openscad=${OPENSCAD:-openscad}
size=${IMGSIZE:-900,1200}

# Manifold always produces closed meshes; --hardwarnings turns every
# warning (e.g. a non-manifold result or an unknown variable) into an error.
for part in front back foot side_plate visor diffuser; do
  echo "STL  $part"
  "$openscad" --backend Manifold --hardwarnings -q -D "part=\"$part\"" -o "$out/doormutex-$part.stl" doormutex.scad
done

# Fit check: module dummies against the printed parts and the parts against
# each other must not overlap.
echo "CHK  collisions"
log=$("$openscad" --backend Manifold -D 'part="collisions"' -o "$out/collisions.stl" doormutex.scad 2>&1) || true
case $log in
  *"top level object is empty"*) ;;
  *) echo "$log" >&2; echo "Parts or modules overlap – open $out/collisions.stl" >&2; exit 1 ;;
esac

# Previews: the assembly from the front left and from behind, the exploded view.
png() {
  echo "PNG  $1"
  "$openscad" --backend Manifold --hardwarnings -q -D "part=\"$2\"" --imgsize "$size" \
    --colorscheme Tomorrow --projection p --viewall --autocenter --camera "$3" -o "$out/$1.png" doormutex.scad
}
png assembly assembly 0,0,0,70,0,-30,0
png assembly-back assembly 0,0,0,70,0,150,0
png exploded exploded 0,0,0,65,0,-50,0
