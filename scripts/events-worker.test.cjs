const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const workerCode = fs.readFileSync(
  path.join(__dirname, "../web/events-worker.js"),
  "utf8",
);
const gameID = "game-one";
const eventsURL = "https://example.test/tower/api/events?game=game-one";

class FakePort {
  messages = [];
  closed = false;
  started = false;

  start() {
    this.started = true;
  }
  postMessage(message) {
    this.messages.push(message);
  }
  close() {
    this.closed = true;
  }
  receive(data) {
    this.onmessage?.({ data });
  }
}

function workerHarness({ throwOnSource = false } = {}) {
  const sources = [];
  class FakeEventSource {
    closed = false;
    constructor(url) {
      if (throwOnSource) throw new Error("EventSource unavailable in worker");
      this.url = url;
      sources.push(this);
    }
    close() {
      this.closed = true;
    }
    open() {
      this.onopen();
    }
    message(data, lastEventId = "state-1") {
      this.onmessage({ data, lastEventId, origin: "https://example.test" });
    }
    error() {
      this.onerror();
    }
  }
  const self = { location: { origin: "https://example.test" } };
  vm.runInNewContext(workerCode, { self, URL, EventSource: FakeEventSource });
  function join(overrides = {}) {
    const port = new FakePort();
    self.onconnect({ ports: [port] });
    port.receive({ type: "connect", url: eventsURL, gameID, ...overrides });
    return port;
  }
  return { sources, join };
}

test("six tabs share one EventSource and all receive open and state", () => {
  const { sources, join } = workerHarness();
  const ports = Array.from({ length: 6 }, () => join());
  assert.equal(sources.length, 1);
  assert.equal(sources[0].url, eventsURL);
  sources[0].open();
  sources[0].message('{"time":42}');
  for (const port of ports) {
    assert.equal(port.started, true);
    assert.deepEqual(port.messages.map((message) => message.type), [
      "open",
      "message",
    ]);
    assert.equal(port.messages[1].data, '{"time":42}');
    assert.equal(port.messages[1].lastEventId, "state-1");
  }
});

test("a joining tab receives open and only the latest snapshot", () => {
  const { sources, join } = workerHarness();
  join();
  sources[0].open();
  sources[0].message("old");
  sources[0].message("latest");
  const joining = join();
  assert.equal(sources.length, 1);
  assert.deepEqual(joining.messages.map((message) => message.type), [
    "open",
    "message",
  ]);
  assert.equal(joining.messages[1].data, "latest");
});

test("the last disconnect closes the source and clears the cached snapshot", () => {
  const { sources, join } = workerHarness();
  const first = join();
  const second = join();
  sources[0].open();
  sources[0].message("old game state");
  first.receive({ type: "disconnect" });
  assert.equal(first.closed, true);
  assert.equal(sources[0].closed, false);
  sources[0].message("new game state");
  assert.equal(first.messages.length, 2);
  assert.equal(second.messages.length, 3);
  second.receive({ type: "disconnect" });
  assert.equal(sources[0].closed, true);
  const next = join();
  assert.equal(sources.length, 2);
  assert.deepEqual(next.messages, []);
  sources[0].message("late stale event");
  assert.deepEqual(next.messages, []);
});

test("source errors notify every tab, close the source, and allow a fresh join", () => {
  const { sources, join } = workerHarness();
  const ports = [join(), join()];
  sources[0].open();
  sources[0].error();
  assert.equal(sources[0].closed, true);
  for (const port of ports) {
    assert.equal(port.closed, true);
    assert.equal(port.messages.at(-1).type, "error");
    assert.equal(port.messages.at(-1).fallback, false);
  }
  const next = join();
  assert.equal(sources.length, 2);
  assert.deepEqual(next.messages, []);
});

test("worker EventSource startup failure requests fallback", () => {
  const { sources, join } = workerHarness({ throwOnSource: true });
  const port = join();
  assert.equal(sources.length, 0);
  assert.equal(port.closed, true);
  assert.equal(port.messages[0].type, "error");
  assert.equal(port.messages[0].fallback, true);
});

test("mismatched games and foreign origins cannot join or replace the stream", () => {
  const { sources, join } = workerHarness();
  const valid = join();
  const invalid = [
    join({ gameID: "another-game" }),
    join({ url: "https://other.test/api/events?game=game-one" }),
    join({ url: "https://example.test/other/api/events?game=game-one" }),
    join({ url: `${eventsURL}&game=game-one` }),
    join({ url: "https://example.test/tower/api/events" }),
    join({ url: "not a URL" }),
  ];
  for (const port of invalid) {
    assert.equal(port.closed, true);
    assert.equal(port.messages[0].type, "error");
  }
  assert.equal(sources.length, 1);
  assert.equal(sources[0].closed, false);
  sources[0].open();
  assert.equal(valid.messages[0].type, "open");
});

test("a broken port is removed without interrupting other tabs", () => {
  const { sources, join } = workerHarness();
  const broken = join();
  const valid = join();
  broken.postMessage = () => {
    throw new Error("Port closed");
  };
  sources[0].open();
  assert.equal(broken.closed, true);
  assert.equal(sources[0].closed, false);
  assert.equal(valid.messages[0].type, "open");
  valid.onmessageerror();
  assert.equal(sources[0].closed, true);
});

// Evaluate only the small browser adapter in a VM. Its worker URL is made
// explicit because CommonJS tests do not have import.meta.url.
const adapterCode = fs
  .readFileSync(path.join(__dirname, "../web/events.js"), "utf8")
  .replace("export function createGameStream", "function createGameStream")
  .replace("import.meta.url", '"https://example.test/tower/events.js"');

function adapterHarness({ supportsWorker = true, throwOnWorker = false } = {}) {
  const workers = [];
  const directSources = [];
  class FakeSharedWorker {
    port = new FakePort();
    constructor(url, options) {
      if (throwOnWorker) throw new Error("Worker blocked");
      this.url = String(url);
      this.options = options;
      workers.push(this);
    }
    addEventListener(type, handler) {
      this[type] = handler;
    }
    removeEventListener(type, handler) {
      if (this[type] === handler) delete this[type];
    }
  }
  class FakeEventSource {
    constructor(url) {
      this.url = url;
      directSources.push(this);
    }
  }
  const context = vm.createContext({
    URL,
    EventSource: FakeEventSource,
    ...(supportsWorker ? { SharedWorker: FakeSharedWorker } : {}),
  });
  vm.runInContext(adapterCode, context);
  const connect = (game = gameID) =>
    context.createGameStream(
      (suffix) => new URL(`https://example.test/tower/api/${suffix}`),
      game,
      "atc-game-session:/tower/",
    );
  return { workers, directSources, connect };
}

test("adapter resolves worker at the application prefix and forwards events", () => {
  const { workers, connect } = adapterHarness();
  const stream = connect();
  const worker = workers[0];
  assert.equal(worker.url, "https://example.test/tower/events-worker.js");
  assert.equal(
    worker.options.name,
    JSON.stringify(["atc-game-session:/tower/", gameID]),
  );
  assert.equal(worker.port.messages[0].url, eventsURL);
  assert.equal(worker.port.messages[0].gameID, gameID);
  const received = [];
  stream.onopen = (event) => received.push(event.type);
  stream.onmessage = (event) => received.push(event.data);
  worker.port.receive({ type: "open" });
  worker.port.receive({ type: "message", data: "state" });
  assert.deepEqual(received, ["open", "state"]);
  stream.close();
  stream.close();
  assert.equal(worker.port.closed, true);
  assert.equal(worker.port.messages.filter((m) => m.type === "disconnect").length, 1);
  worker.port.receive({ type: "message", data: "late" });
  assert.deepEqual(received, ["open", "state"]);
});

test("worker crashes notify the caller and use direct EventSource on reconnect", () => {
  const { workers, directSources, connect } = adapterHarness();
  const stream = connect();
  let errors = 0;
  stream.onerror = () => errors++;
  workers[0].error({ preventDefault() {} });
  assert.equal(errors, 1);
  assert.equal(workers[0].port.closed, true);
  connect();
  assert.equal(workers.length, 1);
  assert.equal(directSources.length, 1);
  assert.equal(directSources[0].url, eventsURL);
});

test("normal SSE errors preserve shared-worker use on reconnect", () => {
  const { workers, directSources, connect } = adapterHarness();
  const stream = connect();
  let errors = 0;
  stream.onerror = () => errors++;
  workers[0].port.receive({ type: "error", fallback: false });
  assert.equal(errors, 1);
  assert.equal(workers[0].port.closed, true);
  connect();
  assert.equal(workers.length, 2);
  assert.equal(directSources.length, 0);
});

test("worker setup or port failures request fallback without hiding the error", () => {
  for (const failure of ["worker-setup", "port"]) {
    const { workers, directSources, connect } = adapterHarness();
    const stream = connect();
    let errors = 0;
    stream.onerror = () => errors++;
    if (failure === "worker-setup") {
      workers[0].port.receive({ type: "error", fallback: true });
    } else {
      workers[0].port.postMessage = () => {
        throw new Error("Port unavailable");
      };
      workers[0].port.onmessageerror();
    }
    assert.equal(errors, 1);
    assert.equal(workers[0].port.closed, true);
    connect();
    assert.equal(workers.length, 1);
    assert.equal(directSources.length, 1);
  }
});

test("missing or blocked SharedWorker falls back to direct EventSource", () => {
  for (const options of [
    { supportsWorker: false },
    { throwOnWorker: true },
  ]) {
    const { workers, directSources, connect } = adapterHarness(options);
    connect();
    assert.equal(workers.length, 0);
    assert.equal(directSources.length, 1);
    assert.equal(directSources[0].url, eventsURL);
  }
});
