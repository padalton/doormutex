// doormutex web app – plain JavaScript, no build step. It only uses the
// public hub API (see api/openapi.yaml) with the token stored on this device.
"use strict";

const TOKEN_KEY = "doormutex.token";
const PROFILE_KEY = "doormutex.profile";
const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => [...document.querySelectorAll(sel)];

const STATUS = {
  dnd: { text: "Do not disturb", color: "c-dnd" },
  working: { text: "Working", color: "c-working" },
  free: { text: "Come in", color: "c-free" },
  away: { text: "Not here", color: "c-away" },
};
const REASON = {
  override: "set by hand",
  pomodoro: "pomodoro",
  pomodoro_break: "pomodoro break",
  microphone: "microphone in use",
  camera: "camera in use",
  screen_locked: "screen locked",
  pc_off: "PC off",
  companion_lost: "companion not responding",
  companion_offline: "PC not running",
  calendar: "calendar",
  work_hours: "working hours",
  signal: "signal",
  default: "nothing planned",
};
const KIND = {
  video_call: "Video call (Meet, Zoom, Teams …)",
  focus: "Focus time",
  out_of_office: "Out of office (timed)",
  other: "Other events",
};
const LEADS = ["stvo", "1m", "3m", "5m", "10m"];

let token = "";
let profile = "";
let events = null;
let state = null;
let mode = { mode: "duration", duration: "1h" };

const timeFmt = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" });
const dayFmt = new Intl.DateTimeFormat(undefined, { weekday: "short", hour: "2-digit", minute: "2-digit" });

function fmtTime(iso) {
  if (!iso) return "";
  const d = new Date(iso);
  const sameDay = d.toDateString() === new Date().toDateString();
  return (sameDay ? timeFmt : dayFmt).format(d);
}

function store(key, value) {
  try {
    if (value) localStorage.setItem(key, value);
    else localStorage.removeItem(key);
  } catch { /* private mode: keep it in memory only */ }
}

function load(key) {
  try { return localStorage.getItem(key) || ""; } catch { return ""; }
}

// api calls the hub; it adds the token and the selected profile.
async function api(method, path, body) {
  const url = new URL(path, location.origin);
  if (profile && path !== "/api/v1/profiles") url.searchParams.set("profile", profile);
  const resp = await fetch(url, {
    method,
    headers: { Authorization: `Bearer ${token}`, ...(body ? { "Content-Type": "application/json" } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await resp.json().catch(() => ({}));
  if (resp.status === 401) {
    logout("The token is no longer valid.");
    throw new Error("unauthorized");
  }
  if (!resp.ok) throw new Error(data.error || resp.statusText);
  return data;
}

function showError(err, where = "#error") {
  const el = $(where);
  el.textContent = err ? String(err.message || err) : "";
  if (err) setTimeout(() => { if (el.textContent === String(err.message || err)) el.textContent = ""; }, 6000);
}

// ---- rendering

function renderLight(light) {
  const [red, yellow, green] = $$("#light .lamp");
  for (const l of [red, yellow, green]) l.className = l.className.split(" ").slice(0, 2).join(" ");
  const on = (l, extra = "") => l.classList.add("on", ...(extra ? [extra] : []));
  switch (light) {
    case "red": on(red); break;
    case "yellow": on(yellow); break;
    case "green": on(green); break;
    case "green_blink": on(green, "blink"); break;
    case "red_yellow": on(red); on(yellow); break;
    case "red_yellow_alternate": on(red, "blink"); on(yellow, "alt"); break;
    case "yellow_blink": on(yellow, "blink"); break;
  }
}

function renderState(st) {
  state = st;
  renderLight(st.light);
  const s = STATUS[st.status] || { text: st.status };
  $("#status-text").textContent = st.phase === "fault" ? "Fault – companion not responding" : s.text;
  $("#status-until").textContent = st.until ? `until ${fmtTime(st.until)}` : "until further notice";
  let reason = REASON[st.reason] || st.reason;
  if (st.detail && st.reason !== "calendar") reason += ` – ${st.detail}`;
  if (st.reason === "calendar" && st.detail) reason += ` – ${KIND[st.detail] || st.detail}`;
  $("#status-reason").textContent = reason;
  const next = [];
  if (st.next) next.push(`then ${(STATUS[st.next.status] || {}).text || st.next.status}`);
  if (st.next_free && st.status !== "free") next.push(`free from ${fmtTime(st.next_free.start)}`);
  $("#status-next").textContent = next.join(" · ");
  const msg = $("#status-message");
  msg.hidden = !st.message;
  msg.textContent = st.message || "";
  if (document.activeElement !== $("#message")) $("#message").value = st.message || "";
  $("#override-info").textContent = st.override
    ? `Override ${STATUS[st.override.status]?.text || st.override.status}${st.override.until ? ` until ${fmtTime(st.override.until)}` : st.override.until_change ? " until the next automatic change" : " until changed"}`
    : "Automatic";
  $("#pomo-info").textContent = st.pomodoro
    ? `Pomodoro ${st.pomodoro.work.replace("m0s", " min")} / ${st.pomodoro.break.replace("m0s", " min")}${st.pomodoro.end ? ` until ${fmtTime(st.pomodoro.end)}` : ""}`
    : "";
  document.title = `${s.text} · doormutex`;
}

async function renderDay() {
  const from = new Date();
  from.setHours(0, 0, 0, 0);
  const to = new Date(from.getTime() + 24 * 3600e3);
  const tl = await api("GET", `/api/v1/timeline?from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`);
  const bar = $("#timeline");
  const list = $("#slots");
  bar.replaceChildren();
  list.replaceChildren();
  const span = to - from;
  for (const s of tl.slots) {
    const a = Math.max(new Date(s.since), from);
    const b = Math.min(new Date(s.until), to);
    const seg = document.createElement("div");
    seg.className = `seg ${STATUS[s.status]?.color || ""}`;
    seg.style.width = `${((b - a) / span) * 100}%`;
    seg.title = `${STATUS[s.status]?.text} ${timeFmt.format(a)}–${timeFmt.format(b)}`;
    bar.append(seg);
    const li = document.createElement("li");
    const dot = document.createElement("span");
    dot.className = `dot ${STATUS[s.status]?.color || ""}`;
    li.append(dot, `${timeFmt.format(a)}–${timeFmt.format(b)}  ${STATUS[s.status]?.text || s.status} (${REASON[s.reason] || s.reason})`);
    list.append(li);
  }
  const mark = document.createElement("div");
  mark.className = "nowmark";
  mark.style.left = `${((Date.now() - from) / span) * 100}%`;
  bar.append(mark);
}

async function renderSettings() {
  const [st, cal] = await Promise.all([api("GET", "/api/v1/settings"), api("GET", "/api/v1/calendar")]);
  const kinds = $("#kinds");
  kinds.replaceChildren();
  for (const [kind, label] of Object.entries(KIND)) {
    const l = document.createElement("label");
    l.textContent = label;
    l.htmlFor = `kind-${kind}`;
    const sel = document.createElement("select");
    sel.id = `kind-${kind}`;
    for (const v of ["dnd", "working", "free", "away", "ignore"]) {
      sel.append(new Option(v === "ignore" ? "ignore" : STATUS[v].text, v, false, st.calendar[kind] === v));
    }
    sel.addEventListener("change", () => saveSettings({ calendar: { [kind]: sel.value } }));
    kinds.append(l, sel);
  }
  for (const id of ["last_call", "ending_soon"]) {
    const sel = $(`#${id}`);
    sel.replaceChildren();
    const opts = LEADS.includes(st[id]) ? LEADS : [...LEADS, st[id]];
    for (const v of opts) sel.append(new Option(v === "stvo" ? "StVO (seconds)" : v.replace("m", " min"), v, false, st[id] === v));
    sel.onchange = () => saveSettings({ [id]: sel.value });
  }
  const acc = $("#accounts");
  acc.replaceChildren();
  if (!cal.accounts.length) {
    const li = document.createElement("li");
    li.textContent = cal.provider === "none" ? "No calendar configured." : `Provider: ${cal.provider}`;
    acc.append(li);
  }
  for (const a of cal.accounts) {
    const li = document.createElement("li");
    const dot = document.createElement("span");
    dot.className = `dot ${a.connected && !a.error ? "c-free" : a.connected ? "c-working" : "c-dnd"}`;
    const info = a.connected ? (a.last_sync ? `synced ${fmtTime(a.last_sync)}, ${a.events} events` : "connected") : "not connected";
    li.append(dot, `${a.name}: ${info}${a.error ? ` – ${a.error}` : ""}`);
    acc.append(li);
  }
}

async function saveSettings(change) {
  try {
    await api("PUT", "/api/v1/settings", change);
    $("#settings-info").textContent = "Saved.";
    setTimeout(() => { $("#settings-info").textContent = ""; }, 2000);
  } catch (err) {
    showError(err, "#settings-info");
  }
}

async function renderHistory() {
  const h = await api("GET", "/api/v1/history?limit=50");
  const list = $("#history");
  list.replaceChildren();
  for (const e of h) {
    const li = document.createElement("li");
    const dot = document.createElement("span");
    dot.className = `dot ${STATUS[e.status]?.color || ""}`;
    li.append(dot, `${dayFmt.format(new Date(e.at))}  ${STATUS[e.status]?.text || e.status} (${REASON[e.reason] || e.reason}${e.detail ? `: ${e.detail}` : ""})`);
    list.append(li);
  }
}

// ---- live updates

function connect() {
  if (events) events.close();
  const url = new URL("/api/v1/events", location.origin);
  url.searchParams.set("access_token", token);
  if (profile) url.searchParams.set("profile", profile);
  events = new EventSource(url);
  events.addEventListener("state", (e) => renderState(JSON.parse(e.data)));
  events.onopen = () => $("#live").classList.add("on");
  events.onerror = () => $("#live").classList.remove("on"); // EventSource reconnects on its own
}

// ---- actions

async function act(fn) {
  try {
    const st = await fn();
    if (st && st.status) renderState(st);
  } catch (err) {
    if (err.message !== "unauthorized") showError(err);
  }
}

function bind() {
  $$("nav button").forEach((b) => b.addEventListener("click", () => {
    $$("nav button").forEach((x) => x.classList.toggle("active", x === b));
    $$(".tab").forEach((t) => { t.hidden = t.id !== `tab-${b.dataset.tab}`; });
    const render = { day: renderDay, settings: renderSettings, history: renderHistory }[b.dataset.tab];
    if (render) render().catch((err) => showError(err));
  }));
  $$("#durations button").forEach((b) => b.addEventListener("click", () => {
    $$("#durations button").forEach((x) => x.classList.toggle("active", x === b));
    mode = { mode: b.dataset.mode, duration: b.dataset.duration };
  }));
  $$(".set-btn[data-status]").forEach((b) => b.addEventListener("click", () => act(() => {
    const body = { status: b.dataset.status, mode: mode.mode };
    if (mode.mode === "duration") body.duration = mode.duration;
    return api("PUT", "/api/v1/override", body);
  })));
  $("#auto").addEventListener("click", () => act(() => api("DELETE", "/api/v1/override")));
  $("#message-form").addEventListener("submit", (e) => {
    e.preventDefault();
    act(() => api("PUT", "/api/v1/message", { text: $("#message").value }));
    $("#message").blur();
  });
  $("#message-clear").addEventListener("click", () => act(() => api("DELETE", "/api/v1/message")));
  $$(".pomo").forEach((b) => b.addEventListener("click", () => act(() =>
    api("PUT", "/api/v1/pomodoro", { work: b.dataset.work, break: b.dataset.break, cycles: 4 }))));
  $("#pomo-stop").addEventListener("click", () => act(() => api("DELETE", "/api/v1/pomodoro")));
  $("#profile").addEventListener("change", (e) => {
    profile = e.target.value;
    store(PROFILE_KEY, profile);
    start();
  });
  $("#logout").addEventListener("click", () => logout(""));
  $("#login-form").addEventListener("submit", (e) => {
    e.preventDefault();
    token = $("#token").value.trim();
    store(TOKEN_KEY, token);
    start();
  });
}

function logout(reason) {
  if (events) events.close();
  token = "";
  store(TOKEN_KEY, "");
  $("#app").hidden = true;
  $("#profile").hidden = true;
  $("#login").hidden = false;
  $("#login-error").textContent = reason;
}

async function start() {
  if (!token) return logout("");
  try {
    const list = await api("GET", "/api/v1/profiles");
    const sel = $("#profile");
    sel.replaceChildren(...list.map((p) => new Option(p.name, p.id, false, p.id === profile)));
    if (!list.some((p) => p.id === profile)) profile = list[0]?.id || "";
    sel.value = profile;
    sel.hidden = list.length < 2;
    $("#login").hidden = true;
    $("#app").hidden = false;
    const cur = list.find((p) => p.id === profile);
    if (cur) renderState(cur.state);
    connect();
  } catch (err) {
    if (err.message !== "unauthorized") $("#login-error").textContent = String(err.message || err);
  }
}

// A link like https://hub/#token=dm_… sets up a phone in one tap; the
// fragment never reaches the server and is removed right away.
function tokenFromFragment() {
  const m = location.hash.match(/token=([A-Za-z0-9_-]+)/);
  if (!m) return;
  store(TOKEN_KEY, m[1]);
  history.replaceState(null, "", location.pathname + location.search);
}

tokenFromFragment();
token = load(TOKEN_KEY);
profile = load(PROFILE_KEY);
bind();
start();
if ("serviceWorker" in navigator) navigator.serviceWorker.register("sw.js").catch(() => {});
