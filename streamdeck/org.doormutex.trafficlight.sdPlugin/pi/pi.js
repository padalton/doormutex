// Property inspector of the doormutex plugin: the hub (global, for every
// key) and the key's own settings. Plain Stream Deck SDK protocol.
"use strict";

const T = {
  de: {
    lamp: "Lampe", red: "Rot – bitte nicht stören", yellow: "Gelb – ich arbeite", green: "Grün – herein",
    off: "Aus – nicht da", duration: "Wie lange", next_change: "Bis zur nächsten automatischen Änderung",
    end_of_event: "Bis der aktuelle Termin endet", end_of_workday: "Bis Arbeitsende", indefinite: "Bis ich zurückschalte",
    work: "Arbeit (min)", break: "Pause (min)", hub_heading: "Hub – für alle doormutex-Tasten", hub_url: "Adresse",
    token: "Token", profile: "Profil", profile_hint: "das Profil des Tokens",
    token_hint: "Ein Token mit vollen Rechten: Weboberfläche → Einstellungen → Handy hinzufügen (nicht „nur ansehen“), oder am Hub: doormutex-hub token create streamdeck office",
  },
};

let ws = null;
let uuid = "";
let settings = {};
let global = {};

const $ = (id) => document.getElementById(id);
const send = (event, payload) => ws && ws.readyState === 1 &&
  ws.send(JSON.stringify(payload === undefined ? { event, context: uuid } : { event, context: uuid, payload }));

function translate(lang) {
  const t = T[(lang || "").slice(0, 2)];
  if (!t) return;
  document.querySelectorAll("[data-t]").forEach((el) => { if (t[el.dataset.t]) el.textContent = t[el.dataset.t]; });
  document.querySelectorAll("[data-tp]").forEach((el) => { if (t[el.dataset.tp]) el.placeholder = t[el.dataset.tp]; });
}

function fillKey() {
  $("lamp-colour").value = settings.lamp || "red";
  $("lamp-duration").value = settings.duration || "next_change";
  $("pomodoro-work").value = parseInt(settings.work, 10) || "";
  $("pomodoro-break").value = parseInt(settings.break, 10) || "";
}

function fillHub() {
  $("hub-url").value = global.hub || "";
  $("hub-token").value = global.token || "";
  $("hub-profile").value = global.profile || "";
}

function saveKey() {
  settings.lamp = $("lamp-colour").value;
  settings.duration = $("lamp-duration").value;
  const w = parseInt($("pomodoro-work").value, 10);
  const b = parseInt($("pomodoro-break").value, 10);
  settings.work = w > 0 ? w + "m" : "";
  settings.break = b > 0 ? b + "m" : "";
  send("setSettings", settings);
}

function saveHub() {
  global = { hub: $("hub-url").value.trim(), token: $("hub-token").value.trim(), profile: $("hub-profile").value.trim() };
  send("setGlobalSettings", global);
}

// Called by the Stream Deck software (Elgato or OpenDeck).
window.connectElgatoStreamDeckSocket = (port, inUUID, registerEvent, info, actionInfo) => {
  uuid = inUUID;
  let action = "";
  try {
    const a = JSON.parse(actionInfo);
    action = a.action || "";
    settings = (a.payload && a.payload.settings) || {};
  } catch (e) { /* keep defaults */ }
  try { translate(JSON.parse(info).application.language); } catch (e) { /* English */ }
  $("lamp").hidden = !action.endsWith(".lamp");
  $("pomodoro").hidden = !action.endsWith(".pomodoro");
  fillKey();

  ws = new WebSocket("ws://127.0.0.1:" + port);
  ws.onopen = () => {
    ws.send(JSON.stringify({ event: registerEvent, uuid }));
    send("getGlobalSettings");
  };
  ws.onmessage = (e) => {
    const m = JSON.parse(e.data);
    if (m.event === "didReceiveGlobalSettings") {
      global = (m.payload && m.payload.settings) || {};
      fillHub();
    } else if (m.event === "didReceiveSettings") {
      settings = (m.payload && m.payload.settings) || {};
      fillKey();
    }
  };
};

["lamp-colour", "lamp-duration", "pomodoro-work", "pomodoro-break"].forEach((id) => $(id).addEventListener("change", saveKey));
["hub-url", "hub-token", "hub-profile"].forEach((id) => $(id).addEventListener("change", saveHub));
