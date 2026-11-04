// Wiring diagrams for the breadboard prototype (docs/wiring.md).
//
// One drawing per stage; every part and wire belongs to a step. The same
// SVG feeds the step images in the docs and the tutorial video, which
// animates the elements of each step in turn.
//
//   node docs/wiring/wiring.mjs          writes docs/wiring/*.svg
//
// Pin numbers come from firmware/main/Kconfig.projbuild (breadboard
// defaults) and the ESP32-C6-DevKitC-1 v1.2 header tables J1/J3.

import { writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export const W = 1920, H = 1080;

const C = {
  bg: "#1b1d22", board: "#262930", boardEdge: "#30343c", hole: "#1d2025",
  text: "#e8e9ec", muted: "#9096a1", pcb: "#14161a",
  v33: "#e5322d", gnd: "#3b82f6", red: "#ff4b42", yellow: "#f5b301", green: "#22c55e",
  spi: ["#c084fc", "#f472b6", "#fb923c", "#2dd4bf"], rst: "#e5e7eb", busy: "#a3e635",
  sda: "#38bdf8", scl: "#fde047", bat: "#f97316", sys: "#ef8a3b", status: ["#e879f9", "#5eead4"],
};

// DevKit: J1 on the left, J3 on the right, pin 1 at the antenna end.
const KIT = { x: 800, y: 140, w: 320, h: 740, j1: 830, j3: 1090 };
const pinY = (n) => 200 + (n - 1) * 40;
const J1 = ["3V3", "RST", "4", "5", "6", "7", "0", "1", "8", "10", "11", "2", "3", "5V", "G", "NC"];
const J3 = ["G", "TX", "RX", "15", "23", "22", "21", "20", "19", "18", "9", "G", "13", "12", "G", "NC"];
const gpio = (g) => {
  const i = J1.indexOf(String(g));
  if (i >= 0) return { x: KIT.j1, y: pinY(i + 1), side: -1 };
  const k = J3.indexOf(String(g));
  if (k >= 0) return { x: KIT.j3, y: pinY(k + 1), side: 1 };
  throw new Error("no pin " + g);
};

const RAIL = { lp: 70, ln: 112, rn: 1806, rp: 1848, top: 110, bottom: 1000 };

// ---- primitives

const esc = (s) => String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;");
const font = `font-family="Inter, 'DejaVu Sans', 'Segoe UI', Arial, sans-serif"`;
function text(x, y, s, { size = 20, fill = C.text, anchor = "start", weight = 500, cls = "" } = {}) {
  return `<text x="${x}" y="${y}" ${font} font-size="${size}" font-weight="${weight}" fill="${fill}" text-anchor="${anchor}"${cls ? ` class="${cls}"` : ""}>${esc(s)}</text>`;
}
const pts = (p) => p.map(([x, y], i) => (i ? "L" : "M") + x + " " + y).join(" ");

// wire: polyline with a dark halo so crossings stay readable; dots mark
// the ends that sit in a contact.
function wire(p, color, { dots = [0, p.length - 1], width = 7, curve = null } = {}) {
  const d = curve || pts(p);
  let s = `<g class="wire-g"><path class="halo" d="${d}" fill="none" stroke="${C.bg}" stroke-width="${width + 6}" stroke-linecap="round" stroke-linejoin="round" pathLength="1"/>`;
  s += `<path class="wire" d="${d}" fill="none" stroke="${color}" stroke-width="${width}" stroke-linecap="round" stroke-linejoin="round" pathLength="1"/>`;
  for (const i of dots) {
    const [x, y] = p[i];
    s += `<circle class="dot" cx="${x}" cy="${y}" r="7" fill="${color}" stroke="${C.bg}" stroke-width="3"/>`;
  }
  return s + "</g>";
}

function resistor(x1, y, x2, bands, label) {
  const m = (x1 + x2) / 2, w = 64;
  let s = `<path d="M${x1} ${y} H${x2}" stroke="#b9bcc4" stroke-width="4"/>`;
  s += `<rect x="${m - w / 2}" y="${y - 12}" width="${w}" height="24" rx="11" fill="#d9c7a1"/>`;
  bands.forEach((b, i) => (s += `<rect x="${m - 22 + i * 13}" y="${y - 12}" width="6" height="24" fill="${b}"/>`));
  if (label) s += text(m, y - 20, label, { size: 18, anchor: "middle", fill: C.muted });
  return s;
}
// 4-band colour codes for the values used here
const BANDS = {
  "47 Ω": ["#ffd400", "#7a2bd8", "#111", "#c9a227"],
  "100 Ω": ["#7a4a1c", "#111", "#7a4a1c", "#c9a227"],
  "10 kΩ": ["#7a4a1c", "#111", "#ff7a00", "#c9a227"],
};

// 5 mm LED lying flat, anode (long leg) on the left
function led(x, y, color) {
  return `<path d="M${x} ${y} h18" stroke="#b9bcc4" stroke-width="4"/>` +
    `<path d="M${x + 66} ${y} h18" stroke="#b9bcc4" stroke-width="4"/>` +
    `<path d="M${x + 18} ${y - 20} h28 a20 20 0 0 1 0 40 h-28 z" fill="${color}" opacity=".9"/>` +
    `<rect x="${x + 14}" y="${y - 23}" width="6" height="46" rx="2" fill="${color}"/>` +
    `<circle cx="${x + 46}" cy="${y}" r="7" fill="#fff" opacity=".45"/>` +
    text(x + 4, y - 30, "+", { size: 22, fill: C.muted, weight: 700 }) +
    text(x + 84, y - 30, "−", { size: 22, fill: C.muted, weight: 700, anchor: "end" });
}

// TO-220 MOSFET seen from the front, rotated: legs point left, G D S top to bottom
function mosfet(x, y) {
  let s = "";
  [["G", -26], ["D", 0], ["S", 26]].forEach(([n, dy]) => {
    s += `<path d="M${x} ${y + dy} H${x + 36}" stroke="#b9bcc4" stroke-width="5"/>`;
    s += text(x + 18, y + dy - 6, n, { size: 15, fill: C.muted, anchor: "middle", weight: 700 });
  });
  s += `<rect x="${x + 36}" y="${y - 44}" width="54" height="88" rx="4" fill="#2b2d33" stroke="#4a4e57"/>`;
  s += `<rect x="${x + 90}" y="${y - 44}" width="30" height="88" rx="3" fill="#a9adb6"/>`;
  s += `<circle cx="${x + 105}" cy="${y}" r="9" fill="${C.bg}"/>`;
  s += `<text x="${x + 63}" y="${y}" ${font} font-size="13" fill="${C.muted}" text-anchor="middle" transform="rotate(-90 ${x + 63} ${y})" dy="5">IRLB3813</text>`;
  return s;
}

function moduleBox(x, y, w, h, title, sub, fill = "#1f2a44") {
  return `<rect x="${x}" y="${y}" width="${w}" height="${h}" rx="10" fill="${fill}" stroke="#3a4560" stroke-width="2"/>` +
    text(x + 16, y + 32, title, { size: 22, weight: 700 }) + (sub ? text(x + 16, y + 56, sub, { size: 16, fill: C.muted }) : "");
}
// a labelled pad/pin on a module edge
function pad(x, y, label, side = "right", color = "#c8ccd4") {
  const tx = side === "right" ? x + 14 : side === "left" ? x - 14 : x;
  const anchor = side === "right" ? "start" : side === "left" ? "end" : "middle";
  const ty = side === "top" ? y - 14 : side === "bottom" ? y + 26 : y + 6;
  return `<circle cx="${x}" cy="${y}" r="8" fill="#d4af37" stroke="#8a6d1d" stroke-width="2"/>` +
    text(tx, ty, label, { size: 17, fill: color, anchor, weight: 600 });
}

function boards() {
  let s = `<defs><pattern id="holes" width="20" height="20" patternUnits="userSpaceOnUse"><rect x="7" y="7" width="6" height="6" rx="1" fill="${C.hole}"/></pattern></defs>`;
  for (const [x, w] of [[30, 920], [970, 920]]) {
    s += `<rect x="${x}" y="90" width="${w}" height="930" rx="18" fill="${C.board}" stroke="${C.boardEdge}" stroke-width="2"/>`;
    s += `<rect x="${x + 110}" y="120" width="${w - 220}" height="870" fill="url(#holes)" opacity=".8"/>`;
  }
  // power rails along the outer edges: + outside, − inside
  for (const [x, col, lab] of [[RAIL.lp, C.v33, "+"], [RAIL.ln, C.gnd, "−"], [RAIL.rp, C.v33, "+"], [RAIL.rn, C.gnd, "−"]]) {
    s += `<path d="M${x} ${RAIL.top} V${RAIL.bottom}" stroke="${col}" stroke-width="4" opacity=".55"/>`;
    s += `<rect x="${x - 9}" y="${RAIL.top}" width="18" height="${RAIL.bottom - RAIL.top}" fill="url(#holes)" opacity=".9"/>`;
    s += text(x, RAIL.top - 4, lab, { size: 22, fill: col, anchor: "middle", weight: 800 });
  }
  return s;
}

function devkit({ usbPlugged = true, usbCrossed = false } = {}) {
  const { x, y, w, h } = KIT;
  let s = `<rect x="${x}" y="${y}" width="${w}" height="${h}" rx="12" fill="#0f2f2a" stroke="#1e5047" stroke-width="2"/>`;
  // antenna + module shield
  s += `<rect x="${x + 92}" y="${y + 14}" width="${w - 184}" height="34" rx="4" fill="#123d36"/>`;
  s += `<path d="M${x + 100} ${y + 40} h20 v-18 h20 v18 h20 v-18 h20 v18 h20 v-18 h20 v18" fill="none" stroke="#c9a227" stroke-width="3"/>`;
  s += `<rect x="${x + 92}" y="${y + 60}" width="${w - 184}" height="200" rx="6" fill="#b8bcc4"/>`;
  s += text(x + w / 2, y + 150, "ESP32-C6", { size: 20, anchor: "middle", fill: "#2a2d33", weight: 800 });
  s += text(x + w / 2, y + 176, "WROOM-1", { size: 17, anchor: "middle", fill: "#2a2d33", weight: 600 });
  s += text(x + w / 2, y + 590, "ESP32-C6-", { size: 18, anchor: "middle", fill: "#7fd6c4", weight: 700 });
  s += text(x + w / 2, y + 614, "DevKitC-1", { size: 18, anchor: "middle", fill: "#7fd6c4", weight: 700 });
  // headers
  J1.forEach((n, i) => {
    const py = pinY(i + 1);
    s += `<rect x="${KIT.j1 - 12}" y="${py - 12}" width="24" height="24" fill="#111" rx="2"/><circle cx="${KIT.j1}" cy="${py}" r="6" fill="#d4af37"/>`;
  });
  J3.forEach((n, i) => {
    const py = pinY(i + 1);
    s += `<rect x="${KIT.j3 - 12}" y="${py - 12}" width="24" height="24" fill="#111" rx="2"/><circle cx="${KIT.j3}" cy="${py}" r="6" fill="#d4af37"/>`;
  });
  s += text(KIT.j1, 178, "J1", { size: 16, fill: C.muted, anchor: "middle" });
  s += text(KIT.j3, 178, "J3", { size: 16, fill: C.muted, anchor: "middle" });
  // USB-C ports at the bottom; firmware uses the one marked "USB"
  const by = y + h;
  s += `<rect x="${x + 70}" y="${by - 26}" width="62" height="30" rx="10" fill="#9aa0a8"/>` + text(x + 101, by - 36, "UART", { size: 15, anchor: "middle", fill: C.muted });
  s += `<rect x="${x + w - 132}" y="${by - 26}" width="62" height="30" rx="10" fill="#9aa0a8"/>` + text(x + w - 101, by - 36, "USB", { size: 15, anchor: "middle", fill: "#d7f5ee", weight: 700 });
  if (usbCrossed) {
    const cx = x + w - 101;
    s += `<path d="M${cx - 24} ${by + 18} l48 48 M${cx + 24} ${by + 18} l-48 48" stroke="${C.red}" stroke-width="8" stroke-linecap="round"/>`;
  }
  return s;
}

// Pin labels go on top of everything: jumpers cross the board.
function pinLabels() {
  const col = (n) => (n === "3V3" || n === "5V" ? "#ff8a80" : n === "G" ? "#93c5fd" : "#d7f5ee");
  const lab = (x, y, n, anchor) =>
    `<text x="${x}" y="${y}" ${font} font-size="17" font-weight="700" fill="${col(n)}" text-anchor="${anchor}" stroke="#0f2f2a" stroke-width="5" paint-order="stroke">${esc(n)}</text>`;
  let s = "";
  J1.forEach((n, i) => (s += lab(KIT.j1 + 22, pinY(i + 1) + 6, n, "start")));
  J3.forEach((n, i) => (s += lab(KIT.j3 - 22, pinY(i + 1) + 6, n, "end")));
  return `<g id="pin-labels">${s}</g>`;
}

// ---- stage 1

const LED_ROWS = [
  // in pin order (J1 top to bottom) so no gate wire crosses another channel
  { color: C.green, name: "green", gpio: 6, y: 320, r: "47 Ω", lane: 760 },
  { color: C.red, name: "red", gpio: 2, y: 560, r: "100 Ω", lane: 782 },
  { color: C.yellow, name: "yellow", gpio: 3, y: 800, r: "100 Ω", lane: 804 },
];
const MX = 470; // MOSFET leg ends

function stage1() {
  const steps = {};
  const add = (n, s) => (steps[n] = (steps[n] || "") + s);
  const L = RAIL;

  // 1 – power rails from the DevKit
  add(1, wire([[KIT.j1, pinY(1)], [L.lp, pinY(1)]], C.v33));
  add(1, wire([[KIT.j1, pinY(15)], [796, pinY(15)], [796, 920], [L.ln, 920]], C.gnd));
  add(1, wire([[KIT.j3, pinY(1)], [L.rn, pinY(1)]], C.gnd));
  add(1, wire([[L.lp, 132], [L.lp, 120], [L.rp, 120], [L.rp, 132]], C.v33, { dots: [0, 3], curve: `M${L.lp} 132 C ${L.lp} 60, ${L.rp} 60, ${L.rp} 132` }));

  for (const r of LED_ROWS) {
    const y = r.y;
    // 2 – MOSFET, source to GND, 10 kΩ gate pull-down
    add(2, mosfet(MX, y));
    add(2, wire([[MX, y + 26], [L.ln, y + 26]], C.gnd));
    add(2, wire([[MX, y - 26], [MX - 30, y - 26], [MX - 30, y - 64], [360, y - 64]], "#b9bcc4", { dots: [0], width: 4 }));
    add(2, resistor(250, y - 64, 360, BANDS["10 kΩ"], "10 kΩ"));
    add(2, wire([[250, y - 64], [L.ln, y - 64]], C.gnd, { dots: [1] }));
    // 3 – LED and series resistor from + to drain
    add(3, led(330, y, r.color));
    add(3, wire([[414, y], [MX, y]], "#b9bcc4", { dots: [1], width: 4 }));
    add(3, resistor(190, y, 330, BANDS[r.r], r.r));
    add(3, wire([[190, y], [L.lp, y]], C.v33, { dots: [1] }));
    // 4 – gate to the GPIO
    const p = gpio(r.gpio);
    add(4, wire([[MX - 30, y - 64], [MX - 30, y - 90], [r.lane, y - 90], [r.lane, p.y], [p.x, p.y]], r.color, { dots: [4] }));
    add(4, text(r.lane - 14, y - 98, `GPIO ${r.gpio}`, { size: 18, fill: r.color, anchor: "end", weight: 700 }));
  }

  // 5 – e-paper module (Waveshare 1.54" V2) on the right. Its header,
  // read bottom to top, is VCC GND DIN CLK CS DC RST BUSY.
  const ex = 1330, ey = 400;
  let e = `<rect x="${ex}" y="${ey}" width="400" height="370" rx="10" fill="#202634" stroke="#3a4560" stroke-width="2"/>`;
  e += text(ex + 130, ey + 36, "e-paper 1.54″ V2", { size: 22, weight: 700 });
  e += text(ex + 130, ey + 60, "Waveshare · SSD1681", { size: 16, fill: C.muted });
  e += `<rect x="${ex + 130}" y="${ey + 84}" width="240" height="240" rx="6" fill="#e9e7df"/>`;
  e += `<rect x="${ex + 146}" y="${ey + 100}" width="208" height="64" rx="6" fill="#1b1d22"/>`;
  e += text(ex + 250, ey + 142, "COME IN", { size: 28, anchor: "middle", fill: "#e9e7df", weight: 800 });
  e += text(ex + 250, ey + 210, "until 17:00", { size: 22, anchor: "middle", fill: "#1b1d22", weight: 600 });
  e += text(ex + 250, ey + 250, "doormutex", { size: 18, anchor: "middle", fill: "#5d616a" });
  const EP = { BUSY: 440, RST: 480, DC: 520, CS: 560, CLK: 600, DIN: 640, GND: 680, VCC: 720 };
  for (const [n, py] of Object.entries(EP)) e += pad(ex + 30, py, n, "right");
  add(5, e);
  // SPI on J3, one step down each (lower pins turn first, so nothing crosses)
  [["DC", 21, 1225], ["CS", 20, 1200], ["CLK", 19, 1175], ["DIN", 18, 1150]].forEach(([n, g, jx], i) => {
    const p = gpio(g);
    add(5, wire([[p.x, p.y], [jx, p.y], [jx, EP[n]], [ex + 30, EP[n]]], C.spi[i]));
  });
  // RST and BUSY sit on J1: jumpers across the board
  const busy = gpio(11), rst = gpio(10);
  add(5, wire([[busy.x, busy.y], [ex + 30, EP.BUSY]], C.busy,
    { curve: `M${busy.x} ${busy.y} C 940 ${busy.y}, 930 260, ${KIT.j3} 260 L 1262 260 C 1306 260, 1300 ${EP.BUSY}, ${ex + 30} ${EP.BUSY}` }));
  add(5, wire([[rst.x, rst.y], [ex + 30, EP.RST]], C.rst,
    { curve: `M${rst.x} ${rst.y} C 900 ${rst.y}, 900 300, ${KIT.j3} 300 L 1246 300 C 1290 300, 1284 ${EP.RST}, ${ex + 30} ${EP.RST}` }));
  add(5, wire([[ex + 30, EP.GND], [1316, EP.GND], [1316, 800], [L.rn, 800]], C.gnd));
  add(5, wire([[ex + 30, EP.VCC], [1340, EP.VCC], [1340, 780], [L.rp, 780]], C.v33));

  // 6 – VEML7700 light sensor via a STEMMA QT to pin cable
  const vx = 1150, vy = 100;
  let v = moduleBox(vx, vy, 220, 140, "VEML7700", "light sensor", "#1d2440");
  v += pad(vx + 30, vy + 140, "SCL", "top") + pad(vx + 74, vy + 140, "SDA", "top");
  v += pad(vx + 220, vy + 64, "VIN", "left") + pad(vx + 220, vy + 104, "GND", "left");
  add(6, v);
  const scl = gpio(23), sda = gpio(22);
  add(6, wire([[scl.x, scl.y], [vx + 30, scl.y], [vx + 30, vy + 140]], C.scl));
  add(6, wire([[sda.x, sda.y], [vx + 74, sda.y], [vx + 74, vy + 140]], C.sda));
  add(6, wire([[vx + 220, vy + 64], [L.rp, vy + 64]], C.v33));
  add(6, wire([[vx + 220, vy + 104], [L.rn, vy + 104]], C.gnd));

  // 7 – USB to the PC
  const ux = KIT.x + KIT.w - 101;
  add(7, wire([[ux, KIT.y + KIT.h + 4], [ux, 1080]], "#d1d5db", { dots: [], width: 16 }));
  add(7, text(ux + 20, 1050, "USB-C to the PC", { size: 20, fill: C.text, weight: 700 }));

  return { steps, base: boards() + devkit() };
}

// ---- stage 2: only the new parts; the stage 1 wiring stays. The power
// chain takes the left half, the fuel gauge sits next to the I²C pins.

function stage2() {
  const steps = {};
  const add = (n, s) => (steps[n] = (steps[n] || "") + s);
  const L = RAIL;

  // carried over from stage 1, dimmed: the rails and their feed
  let base = boards() + devkit({ usbCrossed: true });
  base += `<g opacity=".3">` +
    wire([[KIT.j1, pinY(1)], [L.lp, pinY(1)]], C.v33) +
    wire([[KIT.j1, pinY(15)], [796, pinY(15)], [796, 920], [L.ln, 920]], C.gnd) +
    wire([[KIT.j3, pinY(1)], [L.rn, pinY(1)]], C.gnd) +
    wire([[L.lp, 132], [L.rp, 132]], C.v33, { curve: `M${L.lp} 132 C ${L.lp} 60, ${L.rp} 60, ${L.rp} 132` }) +
    `</g>`;
  base += text(1520, 640, "stage 1 wiring stays:", { size: 24, fill: C.muted, anchor: "middle", weight: 700 });
  base += text(1520, 672, "LEDs, e-paper and light sensor", { size: 20, fill: C.muted, anchor: "middle" });
  base += text(1520, 700, "are hidden here for clarity", { size: 20, fill: C.muted, anchor: "middle" });

  // 8 – cell holder and protection board (off the breadboard)
  const hx = 180, hy = 240;
  let c = `<rect x="${hx}" y="${hy}" width="440" height="90" rx="14" fill="#111317" stroke="#3a3d45" stroke-width="2"/>`;
  c += `<rect x="${hx + 22}" y="${hy + 15}" width="396" height="60" rx="28" fill="#2d6cdf" opacity=".85"/>`;
  c += text(hx + 220, hy + 54, "18650", { size: 24, anchor: "middle", weight: 800 });
  c += text(hx + 404, hy + 55, "+", { size: 30, anchor: "middle", fill: "#fff", weight: 800 });
  c += text(hx + 36, hy + 55, "−", { size: 30, anchor: "middle", fill: "#fff", weight: 800 });
  c += text(hx + 220, hy - 14, "cell holder – insert the cell last", { size: 18, anchor: "middle", fill: C.muted });
  const px = 290, py = 380;
  c += moduleBox(px, py, 200, 120, "", "", "#2a2133");
  c += text(px + 100, py + 62, "protection", { size: 20, weight: 700, anchor: "middle" });
  c += text(px + 100, py + 82, "DW01A + 8205A", { size: 14, fill: C.muted, anchor: "middle" });
  c += pad(px + 40, py, "B−", "bottom") + pad(px + 160, py, "B+", "bottom");
  c += pad(px + 40, py + 120, "P−", "top") + pad(px + 160, py + 120, "P+", "top");
  add(8, c);
  add(8, wire([[hx + 440, hy + 45], [660, hy + 45], [660, 350], [px + 160, 350], [px + 160, py]], C.bat, { dots: [4] }));
  add(8, wire([[hx, hy + 45], [150, hy + 45], [150, 350], [px + 40, 350], [px + 40, py]], "#94a3b8", { dots: [4] }));

  // 9 – charger with power path (Adafruit bq24074)
  const bx = 200, by = 560;
  let b = moduleBox(bx, by, 380, 200, "", "", "#1d2440");
  b += text(bx + 40, by + 130, "bq24074", { size: 22, weight: 700 });
  b += text(bx + 40, by + 154, "charger · power path", { size: 16, fill: C.muted });
  b += pad(px + 40, by, "GND", "bottom") + pad(px + 160, by, "LIPO", "bottom");
  b += pad(bx + 380, by + 50, "PGOOD", "left") + pad(bx + 380, by + 90, "CHG", "left");
  b += pad(bx, by + 60, "GND", "right") + pad(bx + 200, by + 200, "OUT", "top");
  b += `<rect x="${bx - 30}" y="${by + 120}" width="34" height="56" rx="10" fill="#9aa0a8"/>`;
  add(9, b);
  add(9, wire([[px + 160, py + 120], [px + 160, by]], C.bat));
  add(9, wire([[px + 40, py + 120], [px + 40, by]], "#94a3b8"));
  add(9, wire([[bx, by + 60], [L.ln, by + 60]], C.gnd));
  add(9, text(505, 516, "solder or use", { size: 15, fill: C.muted }));
  add(9, text(505, 535, "terminals – not via", { size: 15, fill: C.muted }));
  add(9, text(505, 554, "the breadboard", { size: 15, fill: C.muted }));

  // 10 – fuel gauge: cell voltage from the cell's + lead, I²C next to the light sensor
  const gx = 1160, gy = 320;
  let g = moduleBox(gx, gy, 260, 160, "", "", "#1d2440");
  g += text(gx + 16, gy + 124, "MAX17048", { size: 22, weight: 700 });
  g += text(gx + 16, gy + 148, "fuel gauge", { size: 16, fill: C.muted });
  g += pad(gx, gy + 40, "SCL", "right") + pad(gx, gy + 80, "SDA", "right");
  g += pad(gx + 200, gy, "BAT", "bottom");
  g += pad(gx + 260, gy + 60, "VIN", "left") + pad(gx + 260, gy + 104, "GND", "left");
  add(10, g);
  const scl = gpio(23), sda = gpio(22);
  add(10, wire([[scl.x, scl.y], [gx, gy + 40]], C.scl));
  add(10, wire([[sda.x, sda.y], [gx, gy + 80]], C.sda));
  add(10, wire([[660, hy + 45], [660, 128], [gx + 200, 128], [gx + 200, gy]], C.bat));
  add(10, wire([[gx + 260, gy + 60], [L.rp, gy + 60]], C.v33));
  add(10, wire([[gx + 260, gy + 104], [L.rn, gy + 104]], C.gnd));

  // 11 – buck-boost 3.3 V from the charger output onto the + rail
  const rx = 300, ry = 790;
  let r = moduleBox(rx, ry, 240, 110, "", "", "#2a2133");
  r += text(rx + 110, ry + 62, "S7V8F3", { size: 22, weight: 700 });
  r += text(rx + 110, ry + 86, "buck-boost 3.3 V", { size: 15, fill: C.muted });
  r += pad(rx + 100, ry, "VIN", "bottom");
  r += pad(rx, ry + 44, "VOUT", "right") + pad(rx, ry + 84, "GND", "right");
  add(11, r);
  add(11, wire([[bx + 200, by + 200], [rx + 100, ry]], C.sys));
  add(11, wire([[rx, ry + 44], [L.lp, ry + 44]], C.v33));
  add(11, wire([[rx, ry + 84], [L.ln, ry + 84]], C.gnd));
  add(11, text(KIT.x + KIT.w - 101, 1052, "unplug the DevKit's USB", { size: 20, anchor: "middle", fill: C.red, weight: 700 }));

  // 12 – charge status: CHG → GPIO 7, PGOOD → GPIO 17 (RX)
  const chg = gpio(7), pg = gpio("RX");
  add(12, wire([[bx + 380, by + 90], [740, by + 90], [740, chg.y], [chg.x, chg.y]], C.status[0]));
  add(12, wire([[bx + 380, by + 50], [700, by + 50], [700, 150], [1130, 150], [1130, pg.y], [pg.x, pg.y]], C.status[1]));
  add(12, text(730, by + 124, "CHG → GPIO 7", { size: 18, fill: C.status[0], weight: 700, anchor: "end" }));
  add(12, text(1142, 240, "PGOOD → GPIO 17", { size: 18, fill: C.status[1], weight: 700 }));

  // 13 – power: USB-C into the charger, then the cell
  add(13, wire([[bx - 30, by + 148], [0, by + 148]], "#d1d5db", { dots: [], width: 16 }));
  add(13, text(24, by + 190, "USB-C 5 V", { size: 20, weight: 700 }));

  return { steps, base };
}

// ---- output

export const STEPS = {
  1: "Rails: 3V3 and GND from the DevKit",
  2: "MOSFETs, source to GND, 10 kΩ pull-down",
  3: "LEDs with series resistors",
  4: "Gates to GPIO 6, 2, 3",
  5: "E-paper module",
  6: "Light sensor (I²C)",
  7: "USB to the PC",
  8: "Cell holder and protection board",
  9: "Charger",
  10: "Fuel gauge",
  11: "Buck-boost onto the 3V3 rail",
  12: "Charge status pins",
  13: "Power up",
};

const STYLE = `
.step{transition:opacity .3s}
svg[data-show] .step{display:none}
svg[data-show] .step.done{display:inline;opacity:.28}
svg[data-show] .step.now{display:inline;opacity:1}
`;

// svg returns the drawing of one stage. show = step to highlight
// (earlier steps dimmed, later hidden); 0 or undefined shows everything.
// prefix keeps ids unique when both stages share a page (the video).
export function svg(stage, show = 0, { caption = true, prefix = "" } = {}) {
  const { steps, base } = stage === 1 ? stage1() : stage2();
  let s = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${W} ${H}" width="${W}" height="${H}"${show ? ` data-show="${show}"` : ""}>`;
  s += `<style>${STYLE}</style><rect width="${W}" height="${H}" fill="${C.bg}"/>`;
  s += `<g id="base">${base}</g>`;
  for (const [n, body] of Object.entries(steps)) {
    const k = Number(n);
    const cls = !show ? "step" : k < show ? "step done" : k === show ? "step now" : "step";
    s += `<g id="step-${k}" class="${cls}" data-step="${k}">${body}</g>`;
  }
  s += pinLabels();
  if (caption && show) {
    s += `<rect x="0" y="0" width="${W}" height="64" fill="${C.bg}" opacity=".92"/>`;
    s += text(32, 44, `Stage ${stage} · step ${show}: ${STEPS[show]}`, { size: 30, weight: 700 });
  }
  s += "</svg>\n";
  return prefix ? s.replace(/id="(holes|base|pin-labels)"/g, `id="${prefix}$1"`).replace(/url\(#holes\)/g, `url(#${prefix}holes)`) : s;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const dir = dirname(fileURLToPath(import.meta.url));
  writeFileSync(join(dir, "stage1.svg"), svg(1));
  writeFileSync(join(dir, "stage2.svg"), svg(2));
  for (let k = 1; k <= 13; k++) writeFileSync(join(dir, `step-${String(k).padStart(2, "0")}.svg`), svg(k <= 7 ? 1 : 2, k));
}
