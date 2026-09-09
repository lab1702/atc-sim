// One worker per application mount and game keeps tabs from exhausting the
// browser's HTTP/1.1 connection pool with identical long-lived event streams.
let sharedWorkerFailed = false;

export function createGameStream(apiURL, gameID, scope) {
  const url = String(apiURL(`events?game=${encodeURIComponent(gameID)}`));
  if (typeof SharedWorker === "undefined" || sharedWorkerFailed)
    return new EventSource(url);

  let worker;
  try {
    worker = new SharedWorker(new URL("./events-worker.js", import.meta.url), {
      name: JSON.stringify([scope, gameID]),
    });
  } catch {
    sharedWorkerFailed = true;
    return new EventSource(url);
  }

  const port = worker.port;
  let closed = false;
  const stream = {
    onopen: null,
    onmessage: null,
    onerror: null,
    close() {
      if (closed) return;
      closed = true;
      worker.removeEventListener("error", workerError);
      port.onmessage = null;
      port.onmessageerror = null;
      try {
        port.postMessage({ type: "disconnect" });
      } catch {
        // The worker may already have crashed or closed this port.
      }
      port.close();
    },
  };

  function fail(fallback) {
    if (closed) return;
    if (fallback) sharedWorkerFailed = true;
    stream.close();
    stream.onerror?.({ type: "error", target: stream });
  }

  function workerError(event) {
    event.preventDefault();
    // A missing/blocked worker script or a crashed worker should not make the
    // application's next connection attempt repeat the same startup failure.
    fail(true);
  }

  worker.addEventListener("error", workerError);
  port.onmessageerror = () => fail(true);
  port.onmessage = ({ data }) => {
    if (closed) return;
    if (data.type === "error") {
      fail(data.fallback);
    } else if (data.type === "open") {
      stream.onopen?.({ type: "open", target: stream });
    } else if (data.type === "message") {
      stream.onmessage?.({
        type: "message",
        target: stream,
        data: data.data,
        lastEventId: data.lastEventId,
        origin: data.origin,
      });
    }
  };
  try {
    port.start();
    port.postMessage({ type: "connect", url, gameID });
  } catch {
    sharedWorkerFailed = true;
    stream.close();
    return new EventSource(url);
  }
  return stream;
}
