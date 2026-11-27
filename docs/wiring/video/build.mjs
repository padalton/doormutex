// Builds the HyperFrames project for the wiring tutorial from the same
// drawings as docs/wiring.md: index.html and compositions/*.html.
//
//   node build.mjs && npx hyperframes render --output wiring.mp4
//   VARIANT=pololu node build.mjs && npx hyperframes render --output wiring-pololu.mp4
//
// VARIANT picks the stage 2 parts: tp4056 (default: TP4056 module and
// TPS63020, with a chapter on moving its solder jumper to 3.3 V) or pololu
// (protection board and Pololu S7V8F3).

import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import { pathToFileURL } from "node:url";
import { resolve } from "node:path";

const src = resolve(process.env.WIRING || new URL("../wiring.mjs", import.meta.url).pathname);
const { svg, STEPS, VARIANTS, jumperSvg } = await import(pathToFileURL(src).href);
const VARIANT = process.env.VARIANT || "tp4056";
if (!VARIANTS.includes(VARIANT)) throw new Error(`VARIANT must be one of ${VARIANTS.join(", ")}`);
const TP = VARIANT === "tp4056";

const W = 1920, H = 1080;
const BAND = 860; // the diagram uses y < BAND, the captions the band below
const STEP = 7.5; // seconds per step
const LEAD = 3.5; // chapter title at the start of a stage
const TAIL = 4.5; // complete stage at the end

// what each step's caption says; pins exactly as in docs/wiring.md
const DETAIL = {
  1: "3V3 → + rail · G → − rail · bridge the two + rails",
  2: "source to GND · 10 kΩ gate to GND keeps the LEDs dark at boot",
  3: "+ rail → resistor → long leg · short leg → drain · green 47 Ω, red and yellow 100 Ω",
  4: "green → GPIO 6 · red → GPIO 2 · yellow → GPIO 3",
  5: "DIN 18 · CLK 19 · CS 20 · DC 21 · RST 10 · BUSY 11 · 3.3 V and GND",
  6: "SDA → GPIO 22 · SCL → GPIO 23 · 3.3 V and GND",
  7: "into the port marked USB, not UART · then flash the firmware",
  8: TP ? "+ → B+ · − → B− · leave the TP4056's USB socket empty · cell stays out"
    : "+ → B+ · − → B− · solder or use terminals · cell stays out",
  9: TP ? "OUT+ → LIPO · OUT− → GND · charger GND → − rail" : "P+ → LIPO · P− → GND · charger GND → − rail",
  10: "BAT → cell + · SCL 23 · SDA 22 · shares the bus with the light sensor",
  11: TP ? "OUT → VIN · VOUT → + rail · only with the jumper on 3.3 V · DevKit USB unplugged"
    : "OUT → VIN · VOUT → + rail · the DevKit's USB stays unplugged",
  12: "CHG → GPIO 7 · PGOOD → GPIO 17 (RX) · set both in menuconfig",
  13: "USB-C into the charger · then insert the cell",
};
const ACCENT = { 1: "#e5322d", 2: "#3b82f6", 3: "#22c55e", 4: "#f5b301", 5: "#c084fc", 6: "#38bdf8", 7: "#e8e9ec",
  8: "#f97316", 9: "#f97316", 10: "#38bdf8", 11: "#ef8a3b", 12: "#e879f9", 13: "#e8e9ec" };

// camera focus per step: [x, y, w, h] in drawing coordinates
const FOCUS = {
  1: [0, 40, 1920, 1040], 2: [30, 200, 640, 680], 3: [30, 200, 640, 680], 4: [30, 160, 1120, 740],
  5: [760, 90, 1150, 760], 6: [1020, 60, 900, 420], 7: [620, 560, 760, 520],
  8: [90, 170, 680, 380], 9: [0, 330, 800, 480], 10: [580, 80, 1320, 440], 11: [0, 500, 1240, 560],
  12: [540, 100, 900, 700], 13: [0, 160, 1240, 920],
};
const FULL = [0, 0, W, H];

function cam([x, y, w, h], max = 2.1) {
  const s = Math.min(W / w, BAND / h, max);
  return { scale: +s.toFixed(4), x: +(W / 2 - s * (x + w / 2)).toFixed(1), y: +(BAND / 2 - s * (y + h / 2)).toFixed(1) };
}

const esc = (s) => String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;");

function stageScene(stage, id, title, sub, steps) {
  const dur = LEAD + steps.length * STEP + TAIL;
  const prefix = id + "-";
  const drawing = svg(stage, 0, { caption: false, prefix, variant: VARIANT }).replace(/<style>[\s\S]*?<\/style>/, "");
  let caps = `<div id="${id}-chapter" class="cap chapter clip" data-start="0" data-duration="${LEAD}" data-track-index="2">
      <div class="cap-in"><div class="kicker">Stage ${stage}</div><div class="title">${esc(title)}</div><div class="detail">${esc(sub)}</div></div>
    </div>`;
  steps.forEach((k, i) => {
    const t = LEAD + i * STEP;
    caps += `
    <div id="${id}-cap-${k}" class="cap clip" data-start="${t}" data-duration="${STEP}" data-track-index="2">
      <div class="cap-in"><div class="num" style="color:${ACCENT[k]}">${k}</div>
      <div class="txt"><div class="title">${esc(STEPS[k])}</div><div class="detail">${esc(DETAIL[k])}</div></div></div>
    </div>`;
  });
  caps += `
    <div id="${id}-done" class="cap clip" data-start="${LEAD + steps.length * STEP}" data-duration="${TAIL}" data-track-index="2">
      <div class="cap-in"><div class="kicker">Stage ${stage}</div><div class="title">${stage === 1 ? "Done – flash and check the LEDs" : "Done – it runs on the cell"}</div></div>
    </div>`;

  // timeline
  const full = cam(FULL, 1);
  let js = `const tl = gsap.timeline({ paused: true });
const q = (s) => document.querySelectorAll(s);
// tween only what exists: not every step has parts, wires or dots
const at = (m, s, ...a) => { const els = typeof s === "string" ? q(s) : s; if (els.length) tl[m](els, ...a); };
tl.set("#${id}-cam", { transformOrigin: "0 0", scale: ${full.scale}, x: ${full.x}, y: ${full.y} }, 0);
`;
  for (const k of steps) {
    js += `at("set", "#step-${k} > :not(.wire-g)", { opacity: 0 }, 0);
at("set", "#step-${k} .wire, #step-${k} .halo", { strokeDasharray: "1 2", strokeDashoffset: 1 }, 0);
at("set", "#step-${k} .dot", { scale: 0, transformOrigin: "50% 50%" }, 0);
`;
  }
  // chapter caption
  js += `tl.fromTo("#${id}-chapter .cap-in > *", { opacity: 0, filter: "blur(8px)" }, { opacity: 1, filter: "blur(0px)", duration: 0.2, stagger: 0.08, ease: "power4.out" }, 0.3);
tl.to("#${id}-chapter .cap-in", { opacity: 0, filter: "blur(6px)", duration: 0.18, ease: "power3.in" }, ${LEAD - 0.25});
`;
  steps.forEach((k, i) => {
    const t = LEAD + i * STEP;
    const c = cam(FOCUS[k]);
    js += `
// step ${k}
tl.fromTo("#${id}-cap-${k} .num", { opacity: 0, filter: "blur(8px)" }, { opacity: 1, filter: "blur(0px)", duration: 0.2, ease: "power4.out" }, ${t + 0.05});
tl.fromTo("#${id}-cap-${k} .title", { opacity: 0, filter: "blur(8px)" }, { opacity: 1, filter: "blur(0px)", duration: 0.2, ease: "power4.out" }, ${t + 0.12});
tl.fromTo("#${id}-cap-${k} .detail", { opacity: 0, filter: "blur(6px)" }, { opacity: 1, filter: "blur(0px)", duration: 0.2, ease: "power4.out" }, ${t + 0.35});
tl.to("#${id}-cap-${k} .cap-in", { opacity: 0, filter: "blur(6px)", duration: 0.18, ease: "power3.in" }, ${t + STEP - 0.25});
tl.to("#${id}-cam", { scale: ${c.scale}, x: ${c.x}, y: ${c.y}, duration: 1.1, ease: "power2.inOut" }, ${t});
${i > 0 ? `tl.to("#step-${steps[i - 1]}", { opacity: 0.3, duration: 0.5, ease: "power2.inOut" }, ${t});` : ""}
at("to", "#step-${k} > :not(.wire-g)", { opacity: 1, duration: 0.4, stagger: 0.04, ease: "power2.out" }, ${t + 1.0});
q("#step-${k} .wire-g").forEach((g, j) => {
  const t0 = ${t + 1.4} + j * 0.3;
  at("to", g.querySelectorAll(".halo, .wire"), { strokeDashoffset: 0, duration: 0.6, ease: "power2.inOut" }, t0);
  at("to", g.querySelectorAll(".dot"), { scale: 1, duration: 0.18, ease: "expo.out" }, t0 + 0.5);
});
`;
  });
  const tEnd = LEAD + steps.length * STEP;
  js += `
// complete stage
tl.to("#${id}-cam", { scale: ${full.scale}, x: ${full.x}, y: ${full.y}, duration: 1.2, ease: "power2.inOut" }, ${tEnd});
tl.to(q(${JSON.stringify(steps.map((k) => "#step-" + k).join(", "))}), { opacity: 1, duration: 0.5, ease: "power2.inOut" }, ${tEnd + 0.4});
tl.fromTo("#${id}-done .cap-in > *", { opacity: 0, filter: "blur(8px)" }, { opacity: 1, filter: "blur(0px)", duration: 0.2, stagger: 0.08, ease: "power4.out" }, ${tEnd + 0.6});
window.__timelines["${id}"] = tl;`;

  const html = `<template>
  <style>
    #root { position: absolute; inset: 0; background: #1b1d22; overflow: hidden; font-family: Inter, sans-serif; }
    #${id}-view { position: absolute; left: 0; top: 0; width: ${W}px; height: ${BAND}px; overflow: hidden; }
    #${id}-cam { position: absolute; left: 0; top: 0; width: ${W}px; height: ${H}px; }
    #${id}-cam svg { display: block; }
    .band { position: absolute; left: 0; top: ${BAND}px; width: ${W}px; height: ${H - BAND}px; background: #121418; }
    .cap { position: absolute; left: 0; top: ${BAND}px; width: ${W}px; height: ${H - BAND}px; }
    .cap-in { position: absolute; left: 150px; right: 150px; top: 0; bottom: 0; display: flex; align-items: center; gap: 40px; color: #e8e9ec; }
    .num { font-size: 120px; font-weight: 900; line-height: 1; min-width: 150px; text-align: right; font-variant-numeric: tabular-nums; }
    .txt { display: flex; flex-direction: column; gap: 12px; }
    .title { font-size: 48px; font-weight: 700; letter-spacing: -0.01em; }
    .detail { font-size: 32px; font-weight: 400; color: #b9bdc6; }
    .chapter .cap-in, #${id}-done .cap-in { flex-direction: column; align-items: flex-start; justify-content: center; gap: 8px; }
    .kicker { font-size: 30px; font-weight: 700; color: #f5b301; }
  </style>
  <div id="root" data-composition-id="${id}" data-width="${W}" data-height="${H}">
    <div id="${id}-view"><div id="${id}-cam">${drawing}</div></div>
    <div class="band" data-layout-allow-occlusion></div>
    ${caps}
  </div>
  <script>
${js}
  </script>
</template>
`;
  return { html, dur };
}

// The TPS63020 module ships at 4.2 V: open that jumper, close 3.3 V,
// measure. Before stage 2, so the module never meets the DevKit at 4.2 V.
const JSTEPS = [
  { k: "A", dur: 6.5, accent: "#f5b301", title: "Find the closed jumper",
    detail: "a solder blob or a thin trace across two pads · on delivery: 4.2 V", focus: [640, 400, 640, 200] },
  { k: "B", dur: 10, accent: "#d9925a", title: "Open the 4.2 V jumper",
    detail: "flux, then soak the blob up with braid · a trace: cut it with a scalpel", focus: [640, 260, 640, 340] },
  { k: "C", dur: 7.5, accent: "#c9ccd2", title: "Close the 3.3 V jumper",
    detail: "flux on the pads · a little solder on the tip · touch both pads", focus: [560, 260, 640, 340] },
  { k: "D", dur: 9, accent: "#22c55e", title: "Measure before connecting",
    detail: "a cell on VIN and GND · VOUT must read 3.3 V · only then onto the + rail", focus: [80, 160, 1760, 620] },
];

function jumperScene() {
  const id = "jumper";
  const dur = LEAD + JSTEPS.reduce((a, s) => a + s.dur, 0) + TAIL;
  const drawing = jumperSvg();
  let caps = `<div id="${id}-chapter" class="cap chapter clip" data-start="0" data-duration="${LEAD}" data-track-index="2">
      <div class="cap-in"><div class="kicker">Before stage 2</div><div class="title">Set the TPS63020 to 3.3 V</div><div class="detail">It ships at 4.2 V – that destroys the ESP32-C6</div></div>
    </div>`;
  let t = LEAD;
  const starts = {};
  for (const s of JSTEPS) {
    starts[s.k] = t;
    caps += `
    <div id="${id}-cap-${s.k}" class="cap clip" data-start="${t}" data-duration="${s.dur}" data-track-index="2">
      <div class="cap-in"><div class="num" style="color:${s.accent}">${s.k}</div>
      <div class="txt"><div class="title">${esc(s.title)}</div><div class="detail">${esc(s.detail)}</div></div></div>
    </div>`;
    t += s.dur;
  }
  const tEnd = t;
  caps += `
    <div id="${id}-done" class="cap clip" data-start="${tEnd}" data-duration="${TAIL}" data-track-index="2">
      <div class="cap-in"><div class="kicker">Buck-boost</div><div class="title">Done – 3.3 V, on to stage 2</div></div>
    </div>`;

  const full = cam([0, 120, W, 700], 1);
  const camTo = (f, at, d = 1.1) => { const c = cam(f); return `tl.to("#${id}-cam", { scale: ${c.scale}, x: ${c.x}, y: ${c.y}, duration: ${d}, ease: "power2.inOut" }, ${at});\n`; };
  const fadeIn = (sel, at, d = 0.4) => `tl.fromTo("${sel}", { opacity: 0 }, { opacity: 1, duration: ${d}, ease: "power2.out" }, ${at});\n`;
  const fadeOut = (sel, at, d = 0.4) => `tl.to("${sel}", { opacity: 0, duration: ${d}, ease: "power2.in" }, ${at});\n`;
  const draw = (group, at) => `q("${group} .wire-g").forEach((g, j) => {
  tl.to(g.querySelectorAll(".halo, .wire"), { strokeDashoffset: 0, duration: 0.6, ease: "power2.inOut" }, ${at} + j * 0.3);
  tl.to(g.querySelectorAll(".dot"), { scale: 1, duration: 0.18, ease: "expo.out" }, ${at} + j * 0.3 + 0.5);
});\n`;
  const caption = (k, t0, d) => `tl.fromTo("#${id}-cap-${k} .num", { opacity: 0, filter: "blur(8px)" }, { opacity: 1, filter: "blur(0px)", duration: 0.2, ease: "power4.out" }, ${t0 + 0.05});
tl.fromTo("#${id}-cap-${k} .title", { opacity: 0, filter: "blur(8px)" }, { opacity: 1, filter: "blur(0px)", duration: 0.2, ease: "power4.out" }, ${t0 + 0.12});
tl.fromTo("#${id}-cap-${k} .detail", { opacity: 0, filter: "blur(6px)" }, { opacity: 1, filter: "blur(0px)", duration: 0.2, ease: "power4.out" }, ${t0 + 0.35});
tl.to("#${id}-cap-${k} .cap-in", { opacity: 0, filter: "blur(6px)", duration: 0.18, ease: "power3.in" }, ${t0 + d - 0.25});
`;
  const step = Object.fromEntries(JSTEPS.map((s) => [s.k, s]));
  const { A, B, C, D } = starts;
  // the iron's tip sits at the group's origin
  const IRON_OFF = { x: 1500, y: -120 };

  let js = `const tl = gsap.timeline({ paused: true });
const q = (s) => document.querySelectorAll(s);
tl.set("#${id}-cam", { transformOrigin: "0 0", scale: ${full.scale}, x: ${full.x}, y: ${full.y} }, 0);
tl.set("#j-blob33, #j-flux42, #j-flux33, #j-ring, #j-braid, #j-braid-wet, #j-meter, #j-beep, #j-d-ol, #j-d-v, #j-cell, #j-ok", { opacity: 0 }, 0);
tl.set("#j-blob33", { scale: 0, transformOrigin: "50% 50%" }, 0);
tl.set("#j-bead", { opacity: 0 }, 0);
tl.set("#j-braid", { y: -260 }, 0);
tl.set("#j-iron", { x: ${IRON_OFF.x}, y: ${IRON_OFF.y} }, 0);
tl.set("#j-ptr", { rotation: 0, svgOrigin: "1650 525" }, 0);
tl.set("#j-pc .wire, #j-pc .halo, #j-pv .wire, #j-pv .halo, #j-cw .wire, #j-cw .halo", { strokeDasharray: "1 2", strokeDashoffset: 1 }, 0);
tl.set("#j-pc .dot, #j-pv .dot, #j-cw .dot", { scale: 0, transformOrigin: "50% 50%" }, 0);
tl.fromTo("#${id}-chapter .cap-in > *", { opacity: 0, filter: "blur(8px)" }, { opacity: 1, filter: "blur(0px)", duration: 0.2, stagger: 0.08, ease: "power4.out" }, 0.3);
tl.to("#${id}-chapter .cap-in", { opacity: 0, filter: "blur(6px)", duration: 0.18, ease: "power3.in" }, ${LEAD - 0.25});

// A – find the closed jumper
${caption("A", A, step.A.dur)}${camTo(step.A.focus, A)}${fadeIn("#j-ring", A + 1.2, 0.3)}tl.to("#j-ring", { opacity: 0.25, duration: 0.45, ease: "sine.inOut", yoyo: true, repeat: 5 }, ${A + 1.6});

// B – braid and iron soak up the 4.2 V blob, then the continuity test stays silent
${caption("B", B, step.B.dur)}${camTo(step.B.focus, B)}${fadeOut("#j-ring", B + 0.2, 0.3)}${fadeIn("#j-flux42", B + 0.7)}tl.to("#j-braid", { opacity: 1, duration: 0.2 }, ${B + 1.3});
tl.to("#j-braid", { y: 0, duration: 0.8, ease: "power3.out" }, ${B + 1.3});
tl.to("#j-iron", { x: 975, y: 480, duration: 0.9, ease: "power3.out" }, ${B + 2.1});
tl.to("#j-iron", { y: 492, duration: 0.25, ease: "sine.inOut", yoyo: true, repeat: 3 }, ${B + 3.0});
tl.to("#j-blob42", { opacity: 0, scale: 0.4, transformOrigin: "50% 50%", duration: 1.2, ease: "power2.in" }, ${B + 3.1});
${fadeIn("#j-braid-wet", B + 3.3, 1.0)}tl.to("#j-iron", { x: ${IRON_OFF.x}, y: ${IRON_OFF.y}, duration: 0.8, ease: "power2.in" }, ${B + 4.6});
tl.to("#j-braid", { y: -300, opacity: 0, duration: 0.7, ease: "power2.in" }, ${B + 4.9});
${fadeOut("#j-flux42", B + 5.3)}${camTo([640, 180, 1200, 660], B + 5.5)}${fadeIn("#j-meter", B + 6.0)}tl.to("#j-ptr", { rotation: -50, duration: 0.5, ease: "power2.inOut" }, ${B + 6.5});
${draw("#j-pc", B + 6.9)}${fadeOut("#j-d-idle", B + 8.1, 0.15)}${fadeIn("#j-d-ol", B + 8.1, 0.15)}${fadeIn("#j-beep", B + 8.3)}
// C – solder on the tip, bridge the 3.3 V pads
${caption("C", C, step.C.dur)}${fadeOut("#j-pc, #j-beep", C, 0.4)}${camTo(step.C.focus, C)}${fadeIn("#j-flux33", C + 1.0)}tl.set("#j-iron", { x: 560, y: -200 }, ${C + 1.5});
tl.set("#j-bead", { opacity: 1 }, ${C + 1.5});
tl.to("#j-iron", { x: 772, y: 498, duration: 0.9, ease: "power3.out" }, ${C + 1.6});
tl.to("#j-bead", { opacity: 0, duration: 0.8, ease: "power2.in" }, ${C + 2.8});
tl.to("#j-blob33", { opacity: 1, scale: 1, duration: 0.8, ease: "back.out(1.6)" }, ${C + 2.9});
tl.to("#j-iron", { x: 560, y: -200, duration: 0.8, ease: "power2.in" }, ${C + 4.4});
${fadeOut("#j-flux33", C + 4.8)}
// D – a cell on the input, 3.3 V at the output
${caption("D", D, step.D.dur)}${camTo(step.D.focus, D)}${fadeIn("#j-cell", D + 0.8)}${draw("#j-cw", D + 1.2)}tl.to("#j-ptr", { rotation: 50, duration: 0.6, ease: "power2.inOut" }, ${D + 2.2});
tl.set("#j-d-ol", { opacity: 0 }, ${D + 2.2});
tl.set("#j-d-idle", { opacity: 1 }, ${D + 2.2});
${draw("#j-pv", D + 2.8)}${fadeOut("#j-d-idle", D + 4.0, 0.15)}${fadeIn("#j-d-v", D + 4.0, 0.15)}${fadeIn("#j-ok", D + 4.6, 0.3)}
// done
${camTo([0, 120, W, 700], tEnd, 1.2)}tl.fromTo("#${id}-done .cap-in > *", { opacity: 0, filter: "blur(8px)" }, { opacity: 1, filter: "blur(0px)", duration: 0.2, stagger: 0.08, ease: "power4.out" }, ${tEnd + 0.6});
window.__timelines["${id}"] = tl;`;

  const html = `<template>
  <style>
    #root { position: absolute; inset: 0; background: #1b1d22; overflow: hidden; font-family: Inter, sans-serif; }
    #${id}-view { position: absolute; left: 0; top: 0; width: ${W}px; height: ${BAND}px; overflow: hidden; }
    #${id}-cam { position: absolute; left: 0; top: 0; width: ${W}px; height: ${H}px; }
    #${id}-cam svg { display: block; }
    .band { position: absolute; left: 0; top: ${BAND}px; width: ${W}px; height: ${H - BAND}px; background: #121418; }
    .cap { position: absolute; left: 0; top: ${BAND}px; width: ${W}px; height: ${H - BAND}px; }
    .cap-in { position: absolute; left: 150px; right: 150px; top: 0; bottom: 0; display: flex; align-items: center; gap: 40px; color: #e8e9ec; }
    .num { font-size: 120px; font-weight: 900; line-height: 1; min-width: 150px; text-align: right; }
    .txt { display: flex; flex-direction: column; gap: 12px; }
    .title { font-size: 48px; font-weight: 700; letter-spacing: -0.01em; }
    .detail { font-size: 32px; font-weight: 400; color: #b9bdc6; }
    .chapter .cap-in, #${id}-done .cap-in { flex-direction: column; align-items: flex-start; justify-content: center; gap: 8px; }
    .kicker { font-size: 30px; font-weight: 700; color: #f5b301; }
  </style>
  <div id="root" data-composition-id="${id}" data-width="${W}" data-height="${H}">
    <div id="${id}-view"><div id="${id}-cam">${drawing}</div></div>
    <div class="band" data-layout-allow-occlusion></div>
    ${caps}
  </div>
  <script>
${js}
  </script>
</template>
`;
  return { html, dur };
}

function introScene() {
  const html = `<template>
  <style>
    #root { position: absolute; inset: 0; background: #1b1d22; overflow: hidden; font-family: Inter, sans-serif; color: #e8e9ec; }
    #intro-light { position: absolute; left: 300px; top: 70px; height: 940px; }
    #intro-text { position: absolute; left: 820px; top: 330px; width: 960px; }
    #intro-kicker { font-size: 34px; font-weight: 700; color: #f5b301; margin-bottom: 18px; }
    #intro-title { font-size: 92px; font-weight: 900; line-height: 1.02; letter-spacing: -0.02em; }
    #intro-sub { font-size: 36px; color: #b9bdc6; margin-top: 30px; line-height: 1.4; }
  </style>
  <div id="root" data-composition-id="intro" data-width="${W}" data-height="${H}">
    <img id="intro-light" class="clip" src="assets/light.png" data-start="0" data-duration="8" data-track-index="1" alt="" />
    <div id="intro-text" class="clip" data-start="0" data-duration="8" data-track-index="2">
      <div id="intro-kicker">doormutex</div>
      <div id="intro-title"><span class="w">Wiring</span> <span class="w">the</span> <span class="w">breadboard</span> <span class="w">prototype</span></div>
      <div id="intro-sub">Stage 1 on USB, stage 2 on an 18650 cell.<br/>${TP
        ? "Parts: TP4056 module, TPS63020 buck-boost.<br/>13 steps, plus its 3.3 V jumper."
        : "Alternative parts: protection board, Pololu S7V8F3.<br/>13 steps."}</div>
    </div>
  </div>
  <script>
const tl = gsap.timeline({ paused: true });
tl.fromTo("#intro-light", { scale: 1.06, transformOrigin: "50% 60%" }, { scale: 1, duration: 1.6, ease: "expo.out" }, 0);
tl.fromTo("#intro-title .w", { opacity: 0, filter: "blur(6px)" }, { opacity: 1, filter: "blur(0px)", duration: 0.13, stagger: 0.07, ease: "power4.out" }, 0.4);
tl.fromTo("#intro-sub", { opacity: 0, filter: "blur(8px)" }, { opacity: 1, filter: "blur(0px)", duration: 0.2, ease: "power4.out" }, 1.1);
tl.to("#intro-text", { opacity: 0, filter: "blur(8px)", duration: 0.2, ease: "power3.in" }, 7.6);
tl.to("#intro-light", { opacity: 0, filter: "blur(8px)", duration: 0.2, ease: "power3.in" }, 7.6);
window.__timelines["intro"] = tl;
  </script>
</template>
`;
  return { html, dur: 8 };
}

function outroScene() {
  const html = `<template>
  <style>
    #root { position: absolute; inset: 0; background: #1b1d22; overflow: hidden; font-family: Inter, sans-serif; color: #e8e9ec; }
    #outro-light { position: absolute; left: 300px; top: 70px; height: 940px; }
    #outro-text { position: absolute; left: 820px; top: 360px; width: 980px; }
    #outro-title { font-size: 80px; font-weight: 900; line-height: 1.05; letter-spacing: -0.02em; }
    #outro-sub { font-size: 36px; color: #b9bdc6; margin-top: 34px; line-height: 1.5; }
    #outro-sub b { color: #e8e9ec; }
  </style>
  <div id="root" data-composition-id="outro" data-width="${W}" data-height="${H}">
    <img id="outro-light" class="clip" src="assets/light.png" data-start="0" data-duration="7" data-track-index="1" alt="" />
    <div id="outro-text" class="clip" data-start="0" data-duration="7" data-track-index="2">
      <div id="outro-title"><span class="w">Next:</span> <span class="w">flash</span> <span class="w">it.</span></div>
      <div id="outro-sub">Pin tables and every step:<br/><b>docs/wiring.md</b> · <b>doormutex.org</b></div>
    </div>
  </div>
  <script>
const tl = gsap.timeline({ paused: true });
tl.fromTo("#outro-light", { opacity: 0, filter: "blur(8px)" }, { opacity: 1, filter: "blur(0px)", duration: 0.4, ease: "power4.out" }, 0.1);
tl.fromTo("#outro-title .w", { opacity: 0, filter: "blur(6px)" }, { opacity: 1, filter: "blur(0px)", duration: 0.13, stagger: 0.09, ease: "power4.out" }, 0.4);
tl.fromTo("#outro-sub", { opacity: 0, filter: "blur(8px)" }, { opacity: 1, filter: "blur(0px)", duration: 0.2, ease: "power4.out" }, 1.0);
window.__timelines["outro"] = tl;
  </script>
</template>
`;
  return { html, dur: 7 };
}

const scenes = [
  ["intro", introScene()],
  ["stage1", stageScene(1, "stage1", "USB power", "LEDs, e-paper and light sensor – 7 steps", [1, 2, 3, 4, 5, 6, 7])],
  ...(TP ? [["jumper", jumperScene()]] : []),
  ["stage2", stageScene(2, "stage2", "Battery", "Cell, protection, charger, fuel gauge – 6 steps", [8, 9, 10, 11, 12, 13])],
  ["outro", outroScene()],
];

rmSync("compositions", { recursive: true, force: true });
mkdirSync("compositions", { recursive: true });
let t = 0, hosts = "";
for (const [id, { html, dur }] of scenes) {
  writeFileSync(`compositions/${id}.html`, html);
  hosts += `
      <div id="${id}" data-composition-id="${id}" data-composition-src="compositions/${id}.html" data-start="${t}" data-duration="${dur}" data-track-index="1" data-track-kind="graphics" data-width="${W}" data-height="${H}"></div>`;
  t += dur;
}

writeFileSync("index.html", `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=${W}, height=${H}" />
    <script src="https://cdn.jsdelivr.net/npm/gsap@3.15.0/dist/gsap.min.js"></script>
    <style>
      * { margin: 0; padding: 0; box-sizing: border-box; }
      html, body { margin: 0; width: ${W}px; height: ${H}px; overflow: hidden; background: #1b1d22; }
      #root { width: 100%; height: 100%; position: relative; }
    </style>
  </head>
  <body>
    <div id="root" data-composition-id="main" data-start="0" data-duration="${t}" data-width="${W}" data-height="${H}">${hosts}
    </div>
    <script>
      window.__timelines["main"] = gsap.timeline({ paused: true });
    </script>
  </body>
</html>
`);
console.log("total", t, "s");
