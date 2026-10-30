// doormutex web app – plain JavaScript, no build step. It only uses the
// public hub API (see api/openapi.yaml) with the token stored on this device.
"use strict";

const TOKEN_KEY = "doormutex.token";
const PROFILE_KEY = "doormutex.profile";
const LANG_KEY = "doormutex.lang";
const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => [...document.querySelectorAll(sel)];

const COLOR = { dnd: "c-dnd", working: "c-working", free: "c-free", away: "c-away" };
const KINDS = ["video_call", "focus", "out_of_office", "other"];
const LEADS = ["stvo", "1m", "3m", "5m", "10m"];
const EINK_LANGS = ["en", "de"]; // catalogs of the hub's e-ink renderer (hub/internal/eink/i18n)

let token = "";
let profile = "";
let events = null;
let state = null;
let mode = { mode: "duration", duration: "1h" };

// ---- translations: i18n/<lang>.json, English as fallback for missing keys

let lang = "en";
let languages = { en: "English" };
let fallback = {};
let catalog = {};

// t returns the text for key with {name} placeholders filled in.
function t(key, vars = {}) {
  const s = catalog[key] ?? fallback[key] ?? key;
  return s.replace(/\{(\w+)\}/g, (m, k) => (k in vars ? String(vars[k]) : m));
}

async function fetchJSON(path) {
  const resp = await fetch(path);
  if (!resp.ok) throw new Error(`${path}: ${resp.status}`);
  return resp.json();
}

// pickLanguage: the choice on this device, else the first browser language
// with a catalog, else English.
function pickLanguage() {
  const chosen = load(LANG_KEY);
  if (chosen && languages[chosen]) return chosen;
  for (const l of navigator.languages || [navigator.language || "en"]) {
    const base = l.toLowerCase().split("-")[0];
    if (languages[l]) return l;
    if (languages[base]) return base;
  }
  return "en";
}

async function loadLanguage() {
  try { languages = await fetchJSON("i18n/languages.json"); } catch { /* offline: keep English */ }
  try { fallback = await fetchJSON("i18n/en.json"); } catch { fallback = {}; }
  lang = pickLanguage();
  catalog = lang === "en" ? fallback : await fetchJSON(`i18n/${lang}.json`).catch(() => ({}));
  document.documentElement.lang = lang;
  for (const el of $$("[data-i18n]")) el.textContent = t(el.dataset.i18n);
  for (const el of $$("[data-i18n-placeholder]")) el.placeholder = t(el.dataset.i18nPlaceholder);
  for (const el of $$("[data-i18n-title]")) el.title = t(el.dataset.i18nTitle);
  for (const el of $$("[data-i18n-aria]")) el.setAttribute("aria-label", t(el.dataset.i18nAria));
  for (const el of $$("[data-i18n-alt]")) el.alt = t(el.dataset.i18nAlt);
  timeFmt = new Intl.DateTimeFormat(lang, { hour: "2-digit", minute: "2-digit" });
  dayFmt = new Intl.DateTimeFormat(lang, { weekday: "short", hour: "2-digit", minute: "2-digit" });
}

const statusText = (s) => (["dnd", "working", "free", "away"].includes(s) ? t(`status.${s}`) : s);
const reasonText = (r) => (catalog[`reason.${r}`] ?? fallback[`reason.${r}`] ? t(`reason.${r}`) : r);
const kindText = (k) => (catalog[`kind.${k}`] ?? fallback[`kind.${k}`] ? t(`kind.${k}`) : k);

let timeFmt = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" });
let dayFmt = new Intl.DateTimeFormat(undefined, { weekday: "short", hour: "2-digit", minute: "2-digit" });

function fmtTime(iso) {
  if (!iso) return "";
  const d = new Date(iso);
  const sameDay = d.toDateString() === new Date().toDateString();
  return (sameDay ? timeFmt : dayFmt).format(d);
}

// fmtDur turns "25m0s" or "5m" into "25 min".
function fmtDur(s) {
  const m = /^(?:(\d+)h)?(?:(\d+)m)?(?:0s)?$/.exec(s || "");
  if (!m) return s;
  return t("duration.min", { n: Number(m[1] || 0) * 60 + Number(m[2] || 0) });
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
    logout(t("login.invalid"));
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
  $("#status-text").textContent = st.phase === "fault" ? t("status.fault") : statusText(st.status);
  $("#status-until").textContent = st.until ? t("state.until", { time: fmtTime(st.until) }) : t("state.until_further_notice");
  let reason = reasonText(st.reason);
  if (st.reason === "calendar" && st.detail) reason += ` – ${kindText(st.detail)}`;
  else if (st.detail) reason += ` – ${st.detail}`;
  $("#status-reason").textContent = reason;
  const next = [];
  if (st.next) next.push(t("state.then", { status: statusText(st.next.status) }));
  if (st.next_free && st.status !== "free") next.push(t("state.free_from", { time: fmtTime(st.next_free.start) }));
  $("#status-next").textContent = next.join(" · ");
  const msg = $("#status-message");
  msg.hidden = !st.message;
  msg.textContent = st.message || "";
  if (document.activeElement !== $("#message")) $("#message").value = st.message || "";
  const o = st.override;
  $("#override-info").textContent = !o ? t("state.automatic")
    : o.until ? t("state.override_until", { status: statusText(o.status), time: fmtTime(o.until) })
    : o.until_change ? t("state.override_next_change", { status: statusText(o.status) })
    : t("state.override_indefinite", { status: statusText(o.status) });
  const p = st.pomodoro;
  $("#pomo-info").textContent = !p ? ""
    : t(p.end ? "state.pomodoro_until" : "state.pomodoro", { work: fmtDur(p.work), break: fmtDur(p.break), time: fmtTime(p.end) });
  document.title = `${statusText(st.status)} · doormutex`;
  renderEink();
}

// renderEink shows the e-ink frame the hub renders for the light. An <img>
// can't send the token, so it is fetched and shown as a blob URL; the
// browser revalidates with the ETag, so an unchanged frame isn't sent again.
let einkURL = "";
let einkSeq = 0;
async function renderEink() {
  const seq = ++einkSeq;
  try {
    const url = new URL("/api/v1/eink.png", location.origin);
    if (profile) url.searchParams.set("profile", profile);
    const resp = await fetch(url, { headers: { Authorization: `Bearer ${token}` }, cache: "no-cache" });
    if (!resp.ok || seq !== einkSeq) return;
    const blob = await resp.blob();
    if (seq !== einkSeq) return;
    if (einkURL) URL.revokeObjectURL(einkURL);
    einkURL = URL.createObjectURL(blob);
    $("#eink").src = einkURL;
  } catch { /* offline: keep the last frame */ }
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
    seg.className = `seg ${COLOR[s.status] || ""}`;
    seg.style.width = `${((b - a) / span) * 100}%`;
    seg.title = `${statusText(s.status)} ${timeFmt.format(a)}–${timeFmt.format(b)}`;
    bar.append(seg);
    const li = document.createElement("li");
    const dot = document.createElement("span");
    dot.className = `dot ${COLOR[s.status] || ""}`;
    li.append(dot, `${timeFmt.format(a)}–${timeFmt.format(b)}  ${statusText(s.status)} (${reasonText(s.reason)})`);
    list.append(li);
  }
  const mark = document.createElement("div");
  mark.className = "nowmark";
  mark.style.left = `${((Date.now() - from) / span) * 100}%`;
  bar.append(mark);
}

// statusSelect builds a <select> of statuses plus extra choices.
function statusSelect(id, current, extra = {}) {
  const sel = document.createElement("select");
  sel.id = id;
  for (const v of ["dnd", "working", "free", "away"]) sel.append(new Option(statusText(v), v, false, current === v));
  for (const [v, text] of Object.entries(extra)) sel.append(new Option(text, v, false, current === v));
  return sel;
}

function labelFor(id, text) {
  const l = document.createElement("label");
  l.textContent = text;
  l.htmlFor = id;
  return l;
}

async function renderSettings() {
  const [st, cal] = await Promise.all([api("GET", "/api/v1/settings"), api("GET", "/api/v1/calendar")]);
  const kinds = $("#kinds");
  kinds.replaceChildren();
  for (const kind of KINDS) {
    const sel = statusSelect(`kind-${kind}`, st.calendar[kind], { ignore: t("settings.ignore") });
    sel.addEventListener("change", () => saveSettings({ calendar: { [kind]: sel.value } }));
    kinds.append(labelFor(sel.id, kindText(kind)), sel);
  }
  const tent = statusSelect("tentative", st.tentative, { kind: t("settings.like_kind"), ignore: t("settings.ignore") });
  tent.addEventListener("change", () => saveSettings({ tentative: tent.value }));
  kinds.append(labelFor(tent.id, kindText("tentative")), tent);

  for (const id of ["last_call", "ending_soon"]) {
    const sel = $(`#${id}`);
    sel.replaceChildren();
    const opts = LEADS.includes(st[id]) ? LEADS : [...LEADS, st[id]];
    for (const v of opts) sel.append(new Option(v === "stvo" ? t("settings.stvo") : fmtDur(v), v, false, st[id] === v));
    sel.onchange = () => saveSettings({ [id]: sel.value });
  }
  const pomo = $("#pomodoro_work_status");
  pomo.replaceChildren(...["dnd", "working"].map((v) =>
    new Option(`${t(`color.${v}`)} – ${statusText(v)}`, v, false, st.pomodoro_work_status === v)));
  pomo.onchange = () => saveSettings({ pomodoro_work_status: pomo.value });

  const eink = $("#eink_language");
  eink.replaceChildren(...EINK_LANGS.map((v) => new Option(languages[v] || v, v, false, st.eink_language === v)));
  eink.onchange = async () => {
    await saveSettings({ eink_language: eink.value });
    renderEink();
  };

  const acc = $("#accounts");
  acc.replaceChildren();
  if (!cal.accounts.length) {
    const li = document.createElement("li");
    li.textContent = cal.provider === "none" ? t("settings.no_calendar") : t("settings.provider", { provider: cal.provider });
    acc.append(li);
  }
  for (const a of cal.accounts) {
    const li = document.createElement("li");
    const dot = document.createElement("span");
    dot.className = `dot ${a.connected && !a.error ? "c-free" : a.connected ? "c-working" : "c-dnd"}`;
    const info = a.connected
      ? (a.last_sync ? t("settings.synced", { time: fmtTime(a.last_sync), n: a.events }) : t("settings.connected"))
      : t("settings.not_connected");
    li.append(dot, `${a.name}: ${info}${a.error ? ` – ${a.error}` : ""}`);
    acc.append(li);
  }
  renderLanguages();
}

function renderLanguages() {
  const sel = $("#language");
  const chosen = load(LANG_KEY);
  sel.replaceChildren(new Option(t("settings.language_auto"), "", false, !chosen),
    ...Object.entries(languages).map(([code, name]) => new Option(name, code, false, chosen === code)));
  sel.onchange = async () => {
    store(LANG_KEY, sel.value);
    await loadLanguage();
    if (state) renderState(state);
    renderSettings().catch((err) => showError(err, "#settings-info"));
  };
}

async function saveSettings(change) {
  try {
    await api("PUT", "/api/v1/settings", change);
    $("#settings-info").textContent = t("settings.saved");
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
    dot.className = `dot ${COLOR[e.status] || ""}`;
    li.append(dot, `${dayFmt.format(new Date(e.at))}  ${statusText(e.status)} (${reasonText(e.reason)}${e.detail ? `: ${e.detail}` : ""})`);
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
    $$(".tab").forEach((tab) => { tab.hidden = tab.id !== `tab-${b.dataset.tab}`; });
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
    runShortcut();
  } catch (err) {
    if (err.message !== "unauthorized") $("#login-error").textContent = String(err.message || err);
  }
}

// App shortcuts (long-press on the home-screen icon, see the manifest) open
// ./?action=dnd|working|free|away|auto. The app – not the server – then
// sets the light: a red/yellow/green/off lasts until the next automatic
// change. The parameter is removed so a reload doesn't repeat it.
function runShortcut() {
  const params = new URLSearchParams(location.search);
  const action = params.get("action");
  if (!action) return;
  params.delete("action");
  const rest = params.toString();
  history.replaceState(null, "", location.pathname + (rest ? `?${rest}` : "") + location.hash);
  if (action === "auto") act(() => api("DELETE", "/api/v1/override"));
  else if (COLOR[action]) act(() => api("PUT", "/api/v1/override", { status: action, mode: "next_change" }));
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
loadLanguage().finally(start);
if ("serviceWorker" in navigator) navigator.serviceWorker.register("sw.js").catch(() => {});
