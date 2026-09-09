// This is a classic SharedWorker so module-worker support is not required.
// Authentication remains the browser's same-origin session cookie. The game
// marker only lets the server reject a connection for a different game.
const clients = new Set();
let source = null;
let sourceURL = null;
let sourceGame = null;
let opened = false;
let latest = null;

function reset() {
  source?.close();
  source = null;
  sourceURL = null;
  sourceGame = null;
  opened = false;
  latest = null;
}

function disconnect(port) {
  clients.delete(port);
  port.onmessage = null;
  port.onmessageerror = null;
  port.close();
  if (clients.size === 0) reset();
}

function send(port, message) {
  try {
    port.postMessage(message);
  } catch {
    disconnect(port);
  }
}

function broadcast(message) {
  for (const port of clients) send(port, message);
}

function fail(fallback = false) {
  reset();
  // Let each page use its existing bootstrap/reconnect flow. Do not leave an
  // automatic EventSource retry running behind those new connection attempts.
  for (const port of clients) {
    send(port, { type: "error", fallback });
    disconnect(port);
  }
}

function connect(port, message) {
  let url;
  try {
    url = new URL(message.url);
    if (
      url.origin !== self.location.origin ||
      url.username ||
      url.password ||
      typeof message.gameID !== "string" ||
      message.gameID.length === 0 ||
      message.gameID.length > 128 ||
      url.searchParams.getAll("game").length !== 1 ||
      url.searchParams.get("game") !== message.gameID ||
      (sourceURL !== null &&
        (sourceURL !== url.href || sourceGame !== message.gameID))
    )
      throw new Error("Mismatched game stream");
  } catch {
    send(port, { type: "error" });
    disconnect(port);
    return;
  }

  if (clients.has(port)) return;
  clients.add(port);
  if (source !== null) {
    if (opened) send(port, { type: "open" });
    if (latest !== null) send(port, latest);
    return;
  }

  sourceURL = url.href;
  sourceGame = message.gameID;
  let nextSource;
  try {
    nextSource = new EventSource(sourceURL);
  } catch {
    fail(true);
    return;
  }
  source = nextSource;
  nextSource.onopen = () => {
    if (source !== nextSource) return;
    opened = true;
    broadcast({ type: "open" });
  };
  nextSource.onmessage = (event) => {
    if (source !== nextSource) return;
    latest = {
      type: "message",
      data: event.data,
      lastEventId: event.lastEventId,
      origin: event.origin,
    };
    broadcast(latest);
  };
  nextSource.onerror = () => {
    if (source === nextSource) fail();
  };
}

self.onconnect = ({ ports }) => {
  const port = ports[0];
  port.onmessage = ({ data }) => {
    if (data?.type === "connect") connect(port, data);
    else if (data?.type === "disconnect") disconnect(port);
  };
  port.onmessageerror = () => disconnect(port);
  port.start();
};
