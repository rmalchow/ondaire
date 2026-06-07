// Cluster store + websocket connection state (J arch §3.1). One websocket, the
// single writer of cluster.snapshot. Auto-reconnect with capped backoff.

// Reactive singleton; components import and read fields directly.
export const cluster = $state({
  snapshot: { nodes: [], groups: [] }, // latest Snapshot; empty until first frame
  status: "connecting", // "connecting" | "open" | "reconnecting" | "closed"
  lastError: "", // last ws error message, for the header
  receivedAt: 0, // ms epoch of last cluster frame (staleness hint)
});

let socket = null;
let backoff = 500; // ms; 0.5→1→2→4→max 8s
const MAX_BACKOFF = 8000;
let reconnectTimer = null;
let stopped = false;

// wsURL derives ws(s)://<host>/api/ws from the page location.
function wsURL() {
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return proto + "//" + window.location.host + "/api/ws";
}

function scheduleReconnect() {
  if (stopped) return;
  if (reconnectTimer) return;
  const delay = backoff;
  backoff = Math.min(backoff * 2, MAX_BACKOFF);
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    open();
  }, delay);
}

function open() {
  if (stopped) return;
  let ws;
  try {
    ws = new WebSocket(wsURL());
  } catch (e) {
    cluster.lastError = e && e.message ? e.message : "ws open failed";
    cluster.status = "reconnecting";
    scheduleReconnect();
    return;
  }
  socket = ws;
  ws.onopen = () => {
    cluster.status = "open";
    cluster.lastError = "";
    backoff = 500; // reset on a clean open
  };
  ws.onmessage = (ev) => {
    let msg;
    try {
      msg = JSON.parse(ev.data);
    } catch {
      return; // ignore non-JSON
    }
    if (msg && msg.type === "cluster" && msg.data) {
      cluster.snapshot = msg.data;
      cluster.receivedAt = Date.now();
    }
    // unknown types ignored (forward-compat)
  };
  ws.onerror = () => {
    cluster.lastError = "websocket error";
  };
  ws.onclose = () => {
    if (socket === ws) socket = null;
    if (stopped) {
      cluster.status = "closed";
      return;
    }
    cluster.status = "reconnecting";
    scheduleReconnect();
  };
}

// connect opens the websocket and wires auto-reconnect. Idempotent while a
// socket is live.
export function connect() {
  stopped = false;
  if (socket && (socket.readyState === 0 || socket.readyState === 1)) return;
  backoff = 500;
  open();
}

// disconnect closes the socket and stops reconnect (HMR teardown).
export function disconnect() {
  stopped = true;
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  if (socket) {
    try {
      socket.close();
    } catch {
      // ignore
    }
    socket = null;
  }
  cluster.status = "closed";
}
