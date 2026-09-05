import { AirportView } from "./render.js";

const $ = (id) => document.getElementById(id);
const escapeText = (s) =>
  String(s ?? "").replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ],
  );
const phases = {
  gate: "At gate",
  taxi: "Taxiing",
  holdshort: "Holding short",
  lineup: "Lined up",
  takeoff: "Takeoff roll",
  departure: "Climbing out",
  approach: "On approach",
  landing: "Landing roll",
  "taxi-in": "Taxi to gate",
  goaround: "Going around",
  complete: "Complete",
};
const timeLabel = (seconds) =>
  new Date(Math.floor(seconds) * 1000).toISOString().slice(11, 19);
let state,
  airport,
  selected = null,
  filter = "all",
  views = [],
  connected = false,
  pending = false,
  lastLog = "",
  uiTime = 0,
  mode = "map",
  stream = null,
  retryTimer = null,
  streamTimer = null,
  connecting = false,
  retryDelay = 1000,
  connectionAttempt = 0,
  gameID = null,
  unconfirmedGames = 0,
  missedInstruction = false;
const strips = new Map();
const dirtyVectors = new Set();
const gameChannel = createGameChannel();

function createGameChannel() {
  if (typeof BroadcastChannel === "undefined") return null;
  try {
    const channel = new BroadcastChannel("atc-game-session");
    channel.onmessage = ({ data }) => {
      if (
        typeof data === "string" &&
        data.length > 0 &&
        data.length <= 128 &&
        gameID !== null &&
        data !== gameID
      )
        reconnect("Your browser's game changed. Reconnecting…", 0);
    };
    return channel;
  } catch {
    // Game identity checks also protect browsers without tab messaging.
    return null;
  }
}

function result(message, error = false) {
  $("command-result").textContent = message;
  $("command-result").classList.toggle("error", error);
}
async function post(path, body) {
  if (!connected)
    throw new Error("Wait for your tower to reconnect before sending instructions.");
  const attempt = connectionAttempt;
  const response = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Game-ID": gameID },
    body: JSON.stringify(body),
  });
  if (attempt !== connectionAttempt)
    throw new Error(
      "The connection changed before this instruction was confirmed. Check your tower before trying again.",
    );
  if (response.status === 401) {
    missedInstruction = true;
    reconnect("Your game is no longer available. Reconnecting…", 0);
    throw new Error(
      "Your game is no longer available. This instruction was not sent. Reconnecting…",
    );
  }
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || "Command was not accepted");
  if (attempt !== connectionAttempt)
    throw new Error(
      "The connection changed before this instruction was confirmed. Check your tower before trying again.",
    );
  accept(data);
  return data;
}
function connection(ok) {
  connected = ok;
  $("connection").classList.toggle("live", ok);
  $("connection-label").textContent = ok ? "CONNECTED" : "RECONNECTING";
  for (const id of ["pause", "rate", "difficulty", "reset-simulation"])
    $(id).disabled = !ok;
  if (state) renderUI();
}
function closeStream() {
  clearTimeout(streamTimer);
  stream?.close();
  stream = null;
}
function reconnect(message, delay = retryDelay) {
  closeStream();
  connection(false);
  result(message, true);
  if (retryTimer !== null || connecting) return;
  retryTimer = setTimeout(() => {
    retryTimer = null;
    connectGame();
  }, delay);
  retryDelay = Math.min(retryDelay * 2, 30000);
}
async function connectGame() {
  if (connecting) return;
  connecting = true;
  connectionAttempt++;
  connection(false);
  closeStream();
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 10000);
  let nextDelay = retryDelay;
  try {
    const fetchState = () =>
      fetch("/api/state", {
        cache: "no-store",
        signal: controller.signal,
      });
    // Tabs sharing cookies must not create their first games simultaneously.
    const initial =
      typeof navigator !== "undefined" && navigator.locks
        ? await navigator.locks.request(
            "atc-game-session",
            { signal: controller.signal },
            fetchState,
          )
        : await fetchState();
    if (!initial.ok) {
      if (initial.status === 503) {
        const retryAfter = Number(initial.headers.get("Retry-After"));
        if (Number.isFinite(retryAfter) && retryAfter > 0)
          nextDelay = Math.max(nextDelay, Math.min(retryAfter, 120) * 1000);
        throw new Error("All towers are busy. Waiting for a space…");
      }
      throw new Error("Your tower is unavailable. Reconnecting…");
    }
    const data = await initial.json();
    const firstGame = !state;
    const newGame = initial.headers.get("X-Session-Created") === "true";
    const nextGameID = initial.headers.get("X-Game-ID");
    if (!nextGameID)
      throw new Error("Your game could not be identified. Reconnecting…");
    const identityChanged = gameID !== nextGameID;
    const gameChanged = gameID !== null && identityChanged;
    if (newGame && ++unconfirmedGames >= 3) {
      $("connection-label").textContent = "OFFLINE";
      result(
        "Your browser could not keep your game. Allow cookies for this site, then reload.",
        true,
      );
      return;
    }
    if (gameChanged) selectAircraft(null);
    gameID = nextGameID;
    if (identityChanged) gameChannel?.postMessage(gameID);
    accept(data);
    $("strips").querySelector("p")?.remove();
    if (firstGame && state.aircraft.length)
      selectAircraft(
        state.aircraft.find((a) => a.phase === "holdshort")?.id ||
          state.aircraft[0].id,
      );
    const nextStream = new EventSource(
      `/api/events?game=${encodeURIComponent(gameID)}`,
    );
    stream = nextStream;
    streamTimer = setTimeout(() => {
      if (stream === nextStream)
        reconnect("Your tower is taking too long to respond. Reconnecting…");
    }, 15000);
    nextStream.onopen = () => {
      if (stream !== nextStream) return;
      clearTimeout(streamTimer);
      retryDelay = 1000;
      unconfirmedGames = 0;
      connection(true);
      result(
        missedInstruction
          ? "Your tower is ready. Your last instruction was not sent; review your game before continuing."
          : newGame && !firstGame
            ? "Your previous game ended. A new game is ready."
            : gameChanged
              ? "Your browser switched games. Review your tower before continuing."
              : firstGame
                ? ""
                : "Reconnected to your tower.",
      );
      missedInstruction = false;
    };
    nextStream.onmessage = (e) => {
      if (stream !== nextStream) return;
      try {
        accept(JSON.parse(e.data));
      } catch (err) {
        console.error(err);
      }
    };
    nextStream.onerror = () => {
      if (stream === nextStream)
        reconnect("Connection lost. Reconnecting to your tower…");
    };
  } catch (e) {
    // Retry only the session connection; the airport views stay initialized.
    connecting = false;
    reconnect(
      e.name === "AbortError"
        ? "Your tower is taking too long to respond. Reconnecting…"
        : e.name === "TypeError"
          ? "Your tower is unavailable. Reconnecting…"
          : e.message,
      nextDelay,
    );
  } finally {
    clearTimeout(timeout);
    connecting = false;
  }
}
function accept(data) {
  state = data;
  if (
    selected &&
    !state.aircraft.some((a) => a.id === selected && a.phase !== "complete")
  )
    selectAircraft(null);
  views.forEach((v) => v.update(state));
  if (performance.now() - uiTime > 180) {
    uiTime = performance.now();
    renderUI();
  }
}
function selectAircraft(id) {
  dirtyVectors.clear();
  selected = id;
  views.forEach((v) => v.select(id));
  const a = state?.aircraft.find((a) => a.id === id);
  if (a) {
    $("runway").value = a.runway;
    $("heading").value = Math.round(a.targetHeading ?? a.heading) % 360;
    $("altitude").value = Math.max(
      1200,
      Math.round((a.targetAltitude ?? a.altitude) / 100) * 100,
    );
    $("speed").value = Math.max(
      100,
      Math.min(320, Math.round(a.targetSpeed ?? a.speed)),
    );
  }
  result("");
  renderUI();
}

function renderUI() {
  if (!state) return;
  $("clock").textContent = timeLabel(state.time);
  $("pause").innerHTML = state.paused
    ? "▶ <span>Resume</span>"
    : "Ⅱ <span>Pause</span>";
  $("pause").setAttribute(
    "aria-label",
    state.paused ? "Resume simulation" : "Pause simulation",
  );
  $("rate").value = String(state.rate);
  $("difficulty").value = state.difficulty;
  $("traffic-count").textContent = state.aircraft.filter(
    (a) => a.phase !== "complete",
  ).length;
  $("arrivals-stat").textContent = state.stats.arrivals;
  $("departures-stat").textContent = state.stats.departures;
  $("goarounds-stat").textContent = state.stats.goArounds;
  const visible = state.aircraft.filter(
    (a) => a.phase !== "complete" && (filter === "all" || a.kind === filter),
  );
  const visibleIDs = new Set(visible.map((a) => a.id));
  for (const [id, el] of strips)
    if (!visibleIDs.has(id)) {
      el.remove();
      strips.delete(id);
    }
  for (const a of visible) {
    let el = strips.get(a.id);
    if (!el) {
      el = document.createElement("button");
      el.addEventListener("click", () => selectAircraft(a.id));
      strips.set(a.id, el);
      $("strips").append(el);
    }
    el.className = `strip ${a.kind}${a.id === selected ? " selected" : ""}`;
    el.setAttribute("aria-pressed", String(a.id === selected));
    el.setAttribute(
      "aria-label",
      `Select ${a.callsign}, ${phases[a.phase] || a.phase}`,
    );
    const html = `<div class="strip-top"><strong>${escapeText(a.callsign)}</strong><span>${escapeText(a.type)}</span></div><div class="strip-route">${a.kind === "arrival" ? "↙ ARRIVAL" : "↗ DEPARTURE"} &nbsp; · &nbsp; RWY ${escapeText(a.runway)}</div><div class="strip-bottom"><span>${escapeText(phases[a.phase] || a.phase)}</span><b>${a.altitude > airport.elevation + 50 ? `${Math.round(a.altitude).toLocaleString()}′` : `${Math.round(a.speed)} kt`}</b></div>${a.alert ? `<div class="strip-alert">${escapeText(a.alert)}</div>` : ""}`;
    if (el.innerHTML !== html) el.innerHTML = html;
  }
  $("runway-status").innerHTML = state.runways
    .map(
      (r) =>
        `<span class="runway-chip ${r.occupiedBy || r.reservedBy ? "busy" : ""}" title="${escapeText(r.id)}: ${escapeText(r.occupiedBy ? "Occupied by " + r.occupiedBy : r.reservedBy ? "Reserved by " + r.reservedBy : "Available")}"><i></i>${escapeText(r.id)}</span>`,
    )
    .join("");
  const a = state.aircraft.find((a) => a.id === selected);
  $("empty-selection").hidden = !!a;
  $("selected-control").hidden = !a;
  if (a) {
    $("selected-kind").textContent = a.kind.toUpperCase();
    $("selected-callsign").textContent = a.callsign;
    $("selected-type").textContent =
      `${a.type} · ${a.gate ? "Gate " + a.gate : "Inbound to DTW"}`;
    $("selected-phase").textContent = phases[a.phase] || a.phase;
    $("selected-heading").textContent =
      `${String(Math.round(a.heading) % 360).padStart(3, "0")}°`;
    $("selected-altitude").innerHTML =
      `${Math.round(a.altitude).toLocaleString()}<small> ft</small>`;
    $("selected-speed").innerHTML = `${Math.round(a.speed)}<small> kt</small>`;
    $("clearance").textContent = a.clearance || "Awaiting instructions";
    $("aircraft-alert").hidden = !a.alert;
    $("aircraft-alert").textContent = a.alert;
    const airborne = ["approach", "departure", "goaround"].includes(a.phase);
    const targets = {
      heading: Math.round(a.targetHeading) % 360,
      altitude: Math.max(1200, Math.round(a.targetAltitude / 100) * 100),
      speed: Math.max(100, Math.min(320, Math.round(a.targetSpeed))),
    };
    for (const [id, value] of Object.entries(targets))
      if (!dirtyVectors.has(id) && document.activeElement !== $(id))
        $(id).value = value;
    const enabled = {
      taxi: ["gate", "holdshort"].includes(a.phase) && a.kind === "departure",
      hold: ["taxi", "taxi-in"].includes(a.phase),
      resume: a.clearance === "Hold position",
      lineup: a.phase === "holdshort",
      takeoff: ["holdshort", "lineup"].includes(a.phase),
      land: a.kind === "arrival" && a.phase === "approach",
      goaround: a.kind === "arrival" && a.phase === "approach",
    };
    for (const b of document.querySelectorAll("[data-action]")) {
      b.hidden =
        (["taxi", "lineup", "takeoff"].includes(b.dataset.action) &&
          a.kind === "arrival") ||
        (["land", "goaround"].includes(b.dataset.action) &&
          a.kind === "departure");
      b.disabled = !enabled[b.dataset.action] || !connected || pending;
    }
    for (const input of $("vector-form").querySelectorAll("input,button"))
      input.disabled = !airborne || !connected || pending;
    $("runway").disabled =
      !["gate", "holdshort", "approach", "goaround"].includes(a.phase) ||
      !connected ||
      pending;
  } else $("selected-kind").textContent = "SELECT A FLIGHT";
  for (const id of ["pause", "rate", "difficulty"]) $(id).disabled = !connected;
  const logKey = JSON.stringify(state.events);
  if (logKey !== lastLog) {
    lastLog = logKey;
    $("event-log").innerHTML = state.events
      .slice(-12)
      .reverse()
      .map(
        (e) =>
          `<div class="log-row ${escapeText(e.level)}"><time>${timeLabel(e.time)}</time><span>${escapeText(e.message)}</span></div>`,
      )
      .join("");
  }
}

async function issue(action, values = {}) {
  if (!selected || pending || !connected) return;
  pending = true;
  renderUI();
  try {
    await post("/api/command", { aircraftId: selected, action, ...values });
    if (action === "vector") dirtyVectors.clear();
    result("Clearance acknowledged.");
  } catch (e) {
    result(e.message, true);
  } finally {
    pending = false;
    renderUI();
  }
}
async function control(values) {
  try {
    await post("/api/control", values);
    renderUI();
    return true;
  } catch (e) {
    result(e.message, true);
    return false;
  }
}

async function boot() {
  connection(false);
  $("connection-label").textContent = "CONNECTING";
  try {
    const response = await fetch("/api/airport");
    if (!response.ok) throw new Error("Airport data could not be loaded");
    airport = await response.json();
    $("runway").innerHTML = airport.runways
      .map(
        (r) =>
          `<option value="${escapeText(r.id)}">${escapeText(r.id)} &nbsp; / &nbsp; ${escapeText(r.opposite)} · ${Math.round(r.length).toLocaleString()} m</option>`,
      )
      .join("");
    $("altitude").min = String(
      Math.ceil((airport.elevation + 500) / 100) * 100,
    );
    $("altitude").max = "20000";
    $("speed").max = "320";
    $("data-credit").innerHTML =
      '<a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">© OpenStreetMap contributors</a> · DTW static layout';
    try {
      views = [
        new AirportView(
          $("main-viewport"),
          $("main-labels"),
          airport,
          "map",
          selectAircraft,
        ),
        new AirportView(
          $("secondary-viewport"),
          $("secondary-labels"),
          airport,
          "tower",
          selectAircraft,
        ),
      ];
      $("loading").remove();
    } catch (e) {
      $("loading").textContent = /webgl/i.test(e.message)
        ? "WebGL 2 is unavailable. Enable hardware acceleration and reload."
        : "The airport view could not start. Reload to retry.";
      $("render-status").textContent = "WebGL unavailable";
      console.error(e);
    }
    let lastFrame = performance.now(),
      frames = 0;
    function animate(now) {
      requestAnimationFrame(animate);
      if (document.hidden) return;
      for (const view of views) view.render(now);
      frames++;
      if (now - lastFrame >= 1000) {
        if (views.length)
          $("render-status").textContent =
            `WEBGL 2 · ${Math.round((frames * 1000) / (now - lastFrame))} FPS`;
        frames = 0;
        lastFrame = now;
      }
    }
    requestAnimationFrame(animate);
    connectGame();
  } catch (e) {
    if ($("loading")) $("loading").textContent = e.message;
    connection(false);
    $("connection-label").textContent = "OFFLINE";
    result(e.message, true);
  }
}

$("pause").addEventListener(
  "click",
  () => state && control({ paused: !state.paused }),
);
$("rate").addEventListener("change", (e) =>
  control({ rate: Number(e.target.value) }),
);
$("difficulty").addEventListener("change", (e) =>
  control({ difficulty: e.target.value }),
);
for (const b of document.querySelectorAll("[data-filter]"))
  b.addEventListener("click", () => {
    filter = b.dataset.filter;
    document
      .querySelectorAll("[data-filter]")
      .forEach((b) =>
        b.classList.toggle("active", b.dataset.filter === filter),
      );
    renderUI();
  });
for (const b of document.querySelectorAll("[data-view]"))
  b.addEventListener("click", () => {
    mode = b.dataset.view;
    views[0]?.setMode(mode);
    views[1]?.setMode(mode === "map" ? "tower" : "map");
    document
      .querySelectorAll("[data-view]")
      .forEach((b) => b.classList.toggle("active", b.dataset.view === mode));
    $("main-view-title").textContent =
      mode === "map" ? "Ground radar" : "Tower view";
    $("secondary-view-title").textContent =
      mode === "map" ? "Tower view" : "Ground radar";
    $("camera-label").textContent =
      mode === "map" ? "CONTROL TOWER" : "AIRPORT OVERVIEW";
    $("main-viewport").classList.toggle("is-tower", mode === "tower");
    $("view-hint").textContent =
      mode === "map"
        ? "Drag to pan · Scroll to zoom · Click an aircraft"
        : "Drag to look · Scroll to zoom · Click an aircraft";
    $("tower-hint").textContent =
      mode === "map"
        ? "Drag to look · Scroll to zoom"
        : "Drag to pan · Scroll to zoom";
  });
$("zoom-in").addEventListener("click", () => views[0]?.zoom(0.8));
$("zoom-out").addEventListener("click", () => views[0]?.zoom(1.25));
$("reset-view").addEventListener("click", () => views[0]?.reset());
$("track-selected").addEventListener("click", () => {
  const v = views.find((v) => v.mode === "tower");
  v?.track();
  $("track-selected").textContent = v?.tracking
    ? "Stop tracking ↗"
    : "Track selected ↗";
});
for (const b of document.querySelectorAll("[data-action]"))
  b.addEventListener("click", () =>
    issue(
      b.dataset.action,
      ["taxi", "land", "lineup", "takeoff"].includes(b.dataset.action)
        ? { runway: $("runway").value }
        : {},
    ),
  );
for (const id of ["heading", "altitude", "speed"])
  $(id).addEventListener("input", () => dirtyVectors.add(id));
$("vector-form").addEventListener("submit", (e) => {
  e.preventDefault();
  if (!dirtyVectors.size) {
    result("Adjust heading, altitude, or speed first.");
    return;
  }
  const values = {};
  for (const id of dirtyVectors) values[id] = Number($(id).value);
  issue("vector", values);
});
$("help-button").addEventListener("click", () => $("help").showModal());
for (const id of ["close-help", "start-playing"])
  $(id).addEventListener("click", () => $("help").close());
$("reset-simulation").addEventListener("click", async () => {
  if (!(await control({ reset: true }))) return;
  selectAircraft(
    state?.aircraft.find((a) => a.phase === "holdshort")?.id || null,
  );
  $("help").close();
  views.forEach((v) => v.reset());
  $("track-selected").textContent = "Track selected ↗";
});
document.addEventListener("keydown", (e) => {
  if (
    /INPUT|SELECT|TEXTAREA|BUTTON/.test(document.activeElement.tagName) ||
    $("help").open
  )
    return;
  if (e.code === "Space") {
    e.preventDefault();
    if (connected && state) control({ paused: !state.paused });
  }
  if (e.code === "Escape") selectAircraft(null);
});
boot();
