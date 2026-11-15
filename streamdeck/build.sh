#!/bin/sh
# Builds the Stream Deck plugin: the bundle in build/streamdeck/ and the
# installable build/doormutex-VERSION.streamDeckPlugin (a zip). GoReleaser
# runs it for a release (.goreleaser.yaml).
#
#   streamdeck/build.sh             version "dev"
#   VERSION=0.2.0 streamdeck/build.sh
#
# Needs Go and python3 (for the zip).
set -eu
root=$(cd "$(dirname "$0")/.." && pwd)
version=${VERSION:-dev}
version=${version#v}
out=$root/build/streamdeck
bundle=$out/org.doormutex.trafficlight.sdPlugin

rm -rf "$out"
mkdir -p "$bundle/bin"
cp -R "$root/streamdeck/org.doormutex.trafficlight.sdPlugin/." "$bundle/"
cp "$root/streamdeck/launch-mac.sh" "$bundle/bin/launch-mac.sh"
chmod +x "$bundle/bin/launch-mac.sh"

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
	os=${target%/*}
	arch=${target#*/}
	ext=""
	[ "$os" = windows ] && ext=.exe
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -C "$root/companion" -trimpath \
		-ldflags "-s -w -X main.version=$version" \
		-o "$bundle/bin/doormutex-streamdeck-$os-$arch$ext" ./cmd/doormutex-streamdeck
done

# The static pictures, drawn by the same code as the live keys.
go run -C "$root/companion" ./cmd/doormutex-streamdeck -write-icons "$bundle/imgs"

# Elgato wants a numeric version: 0.2.0-rc1 → 0.2.0, "dev" → 0.0.0.
case $version in
[0-9]*) v=${version%%-*} ;;
*) v=0.0.0 ;;
esac
sed -i.bak "s/\"Version\": \"0.0.0\"/\"Version\": \"$v\"/" "$bundle/manifest.json"
rm "$bundle/manifest.json.bak"

# The file name is free; the folder inside the zip names the plugin.
pkg=$root/build/doormutex-$version.streamDeckPlugin
python3 - "$out" "$pkg" <<'PY'
import os, sys, zipfile
out, pkg = sys.argv[1], sys.argv[2]
name = "org.doormutex.trafficlight.sdPlugin"
with zipfile.ZipFile(pkg, "w", zipfile.ZIP_DEFLATED) as z:
    for dirpath, _, files in os.walk(os.path.join(out, name)):
        for f in sorted(files):
            p = os.path.join(dirpath, f)
            info = zipfile.ZipInfo.from_file(p, os.path.relpath(p, out))
            with open(p, "rb") as fh:
                z.writestr(info, fh.read(), zipfile.ZIP_DEFLATED)  # keeps the executable bits
PY
echo "built $pkg"
