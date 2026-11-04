# Wiring video

Source of the tutorial video in [docs/wiring.md](../../wiring.md), made with [HyperFrames](https://hyperframes.heygen.com/) (HTML rendered to video). The scenes are generated from the same drawings as the step images ([`../wiring.mjs`](../wiring.mjs)), so pins and parts can't drift apart.

```bash
cd docs/wiring/video
node build.mjs                                  # writes index.html and compositions/
npx hyperframes preview                         # optional: look and edit in the browser
npx hyperframes render --output wiring.mp4
ffmpeg -i wiring.mp4 -c:v libx264 -preset slow -crf 24 -pix_fmt yuv420p -movflags +faststart -an wiring-web.mp4
```

The web version is published on doormutex.org (`static/video/wiring.mp4` in the site repository). `assets/light.png` is the enclosure rendered with OpenSCAD (`part="assembly"`, see [enclosure/render.sh](../../../enclosure/render.sh)) with the background made transparent.
