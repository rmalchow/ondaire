/**
 * ondaire-card — a single ondaire room, styled in ondaire's own idiom instead
 * of the stock media-control card.
 *
 * Every action goes through a standard HA service call or an ondaire websocket
 * command, so the browser only ever talks to Home Assistant — HA (via the
 * ondaire integration) does the actual proxying to the ondaire master. No
 * bundler: this mirrors the project's low-tooling bias (see site/build.mjs).
 *
 * Layout: a persistent now-playing header + four always-present tabs
 * (Players / Media / Streams / Queue). The tabs replaced transient popovers,
 * which a re-render could yank away mid-interaction; tab state lives on the
 * instance and survives re-renders.
 */

const ICON_PLAY = '<svg viewBox="0 0 24 24" width="20" height="20"><path fill="currentColor" d="M8 5v14l11-7z"/></svg>';
const ICON_PAUSE = '<svg viewBox="0 0 24 24" width="20" height="20"><path fill="currentColor" d="M6 5h4v14H6zm8 0h4v14h-4z"/></svg>';
const ICON_STOP = '<svg viewBox="0 0 24 24" width="18" height="18"><path fill="currentColor" d="M6 6h12v12H6z"/></svg>';
const ICON_NEXT = '<svg viewBox="0 0 24 24" width="18" height="18"><path fill="currentColor" d="M6 6l8.5 6L6 18zm10 0h2v12h-2z"/></svg>';
const ICON_PREV = '<svg viewBox="0 0 24 24" width="18" height="18"><path fill="currentColor" d="M18 6l-8.5 6 8.5 6zM8 6H6v12h2z"/></svg>';
const ICON_VOLUME = '<svg viewBox="0 0 24 24" width="18" height="18"><path fill="currentColor" d="M3 10v4h4l5 5V5L7 10zm13.5 2a4.5 4.5 0 0 0-2.5-4v8a4.5 4.5 0 0 0 2.5-4z"/></svg>';
const ICON_MUTE = '<svg viewBox="0 0 24 24" width="18" height="18"><path fill="currentColor" d="M3 10v4h4l5 5V5L7 10zm14.7-2.3l-1.4-1.4L14 8.6 11.7 6.3l-1.4 1.4L12.6 10 10.3 12.3l1.4 1.4L14 11.4l2.3 2.3 1.4-1.4L15.4 10z"/></svg>';
const ICON_NOTE = '<svg viewBox="0 0 24 24" width="32" height="32"><path fill="currentColor" d="M12 3v10.55A4 4 0 1 0 14 17V7h4V3z"/></svg>';
const ICON_NOTE_SMALL = '<svg viewBox="0 0 24 24" width="16" height="16"><path fill="currentColor" d="M12 3v10.55A4 4 0 1 0 14 17V7h4V3z"/></svg>';
const ICON_FOLDER = '<svg viewBox="0 0 24 24" width="16" height="16"><path fill="currentColor" d="M10 4H2v16h20V6H12z"/></svg>';
const ICON_PLUS = '<svg viewBox="0 0 24 24" width="16" height="16"><path fill="currentColor" d="M11 5h2v6h6v2h-6v6h-2v-6H5v-2h6z"/></svg>';
const ICON_TRASH = '<svg viewBox="0 0 24 24" width="16" height="16"><path fill="currentColor" d="M6 7h12l-1 14H7zM9 4h6l1 2H8z"/></svg>';

const TABS = [
  ["players", "Players"],
  ["media", "Media"],
  ["streams", "Streams"],
  ["queue", "Queue"],
];

function esc(value) {
  return String(value ?? "").replace(/[&<>"']/g, (c) => (
    { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]
  ));
}

function fmtTime(sec) {
  sec = Math.max(0, Math.floor(sec || 0));
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

const STYLE = `
  :host { --ondaire-accent: #f7b733; }
  ha-card { padding: 16px; }
  .error { color: var(--error-color, #db4437); }
  .header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; }
  .name { font-size: 1.1em; font-weight: 500; color: var(--primary-text-color); }
  .pill { font-size: 0.7em; text-transform: uppercase; letter-spacing: .04em; padding: 2px 8px; border-radius: 999px; color: var(--secondary-text-color); background: var(--divider-color); }
  .pill.playing { color: #1c1304; background: var(--ondaire-accent); }
  .body { display: flex; gap: 14px; }
  .body.unavailable { opacity: 0.5; pointer-events: none; }
  .art { flex: 0 0 72px; width: 72px; height: 72px; border-radius: 8px; background-color: var(--divider-color); background-size: cover; background-position: center; display: flex; align-items: center; justify-content: center; color: var(--secondary-text-color); }
  .meta { flex: 1; min-width: 0; }
  .title { font-weight: 500; color: var(--primary-text-color); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sub { font-size: 0.85em; color: var(--secondary-text-color); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; margin-bottom: 8px; }
  .progress-track { height: 4px; border-radius: 2px; background: var(--divider-color); cursor: pointer; }
  .progress-fill { height: 100%; border-radius: 2px; background: var(--ondaire-accent); }
  .times { display: flex; justify-content: space-between; font-size: 0.75em; color: var(--secondary-text-color); margin-top: 3px; }
  .transport { display: flex; align-items: center; gap: 4px; margin-top: 8px; }
  .icon-btn { border: none; background: none; cursor: pointer; color: var(--primary-text-color); padding: 6px; border-radius: 50%; display: flex; }
  .icon-btn:hover { background: var(--divider-color); }
  .icon-btn:disabled { color: var(--disabled-text-color); cursor: default; }
  .icon-btn:disabled:hover { background: none; }
  .icon-btn.play { color: var(--primary-color); }
  .icon-btn.small { padding: 4px; }
  .volume-row { display: flex; align-items: center; gap: 8px; margin-top: 6px; }
  .volume { flex: 1; accent-color: var(--primary-color); }
  /* tabs */
  .tabbar { display: flex; gap: 2px; margin-top: 14px; border-bottom: 1px solid var(--divider-color); }
  .tab { flex: 1; text-align: center; padding: 8px 4px; font-size: 0.85em; cursor: pointer; color: var(--secondary-text-color); border-bottom: 2px solid transparent; margin-bottom: -1px; user-select: none; }
  .tab:hover { color: var(--primary-text-color); }
  .tab.active { color: var(--primary-text-color); border-bottom-color: var(--ondaire-accent); font-weight: 500; }
  .tabcontent { margin-top: 10px; min-height: 60px; }
  .list { max-height: 260px; overflow-y: auto; display: flex; flex-direction: column; }
  .muted { color: var(--secondary-text-color); font-size: 0.85em; padding: 8px 2px; }
  .crumb { display: flex; align-items: center; gap: 6px; margin-bottom: 6px; }
  .row { display: flex; align-items: center; gap: 8px; padding: 6px 2px; color: var(--primary-text-color); font-size: 0.9em; min-width: 0; }
  .row .label { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  button.row { border: none; background: none; text-align: left; cursor: pointer; width: 100%; font: inherit; }
  button.row:hover { background: var(--divider-color); }
  .row .sub2 { color: var(--secondary-text-color); }
  .text-btn { border: none; background: none; cursor: pointer; color: var(--primary-color); font: inherit; font-size: 0.85em; padding: 4px 0; }
  /* players */
  .spk { display: flex; align-items: center; gap: 12px; padding: 9px 0; }
  .spk-name { flex: 0 0 30%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--primary-text-color); font-size: 0.9em; }
  .spk.off .spk-name { color: var(--secondary-text-color); }
  .spk-vol { flex: 1; accent-color: var(--ondaire-accent); }
  /* non-joined (or in-flight) speakers show a static level track, no handle */
  .spk-track { flex: 1; height: 5px; border-radius: 3px; background: var(--divider-color); }
  .spk-track-fill { height: 100%; border-radius: 3px; background: color-mix(in srgb, var(--secondary-text-color) 45%, transparent); }
  /* toggle sized for touch (HA ha-switch is ~this big) */
  .sw { position: relative; flex: 0 0 auto; width: 46px; height: 26px; border-radius: 999px; background: var(--divider-color); cursor: pointer; transition: background .15s; }
  .sw.on { background: var(--ondaire-accent); }
  .sw::after { content: ""; position: absolute; top: 3px; left: 3px; width: 20px; height: 20px; border-radius: 50%; background: #fff; transition: transform .15s; box-shadow: 0 1px 2px rgba(0,0,0,.35); }
  .sw.on::after { transform: translateX(20px); }
  .sw.busy { opacity: 0.55; pointer-events: none; cursor: default; }
`;

class OndaireCard extends HTMLElement {
  static getStubConfig(hass) {
    const id = hass
      ? Object.keys(hass.states).find(
          (e) => e.startsWith("media_player.") && hass.entities?.[e]?.platform === "ondaire",
        )
      : undefined;
    return { entity: id || "" };
  }

  setConfig(config) {
    if (!config || !config.entity) {
      throw new Error("ondaire-card: 'entity' is required");
    }
    this._config = config;
    this._tab = "players";
    this._media = {
      stack: [], cur: null, loading: false, error: "",
      query: "", results: null, searching: false, searchErr: "",
    };
    this._streams = { items: null, loading: false, error: "" };
    this._queue = { items: null, loading: false, error: "", forUri: null };
    this._dragging = false;
    // entity_id -> desired joined state while a join/unjoin is in flight.
    this._pending = {};
    this._pendingTimers = {};
  }

  set hass(hass) {
    this._hass = hass;
    const sig = this._relevantSig(hass);
    if (sig !== this._lastSig && !this._dragging) {
      this._lastSig = sig;
      this._render();
    }
  }

  getCardSize() {
    return 6;
  }

  connectedCallback() {
    this._tickTimer = window.setInterval(() => this._tick(), 1000);
  }

  disconnectedCallback() {
    window.clearInterval(this._tickTimer);
  }

  // --- change detection ----------------------------------------------------
  _relevantSig(hass) {
    // Signature of ONLY the fields that affect what we render — deliberately
    // NOT last_updated, which bumps on every heartbeat because media_position /
    // media_position_updated_at change each frame. Position is handled by
    // _tick() (targeted DOM write, no re-render), so folding it in here would
    // rebuild the DOM every few seconds and drop clicks mid-interaction.
    const ids = [this._config.entity, ...this._playbackIds(hass)];
    return ids.map((id) => this._entSig(hass.states[id])).join("|");
  }

  _entSig(s) {
    if (!s) return "∅";
    const a = s.attributes;
    return [
      s.state,
      a.volume_level,
      a.is_volume_muted,
      a.media_title,
      a.media_artist,
      a.media_album_name,
      a.media_content_id,
      a.media_duration,
      a.entity_picture,
      a.ondaire_playback,
      (a.group_members || []).join(","),
    ].join("~");
  }

  _playbackIds(hass) {
    const entities = hass.entities || {};
    return Object.keys(entities)
      .filter((id) => {
        if (!id.startsWith("media_player.")) return false;
        if (entities[id].platform !== "ondaire") return false;
        const s = hass.states[id];
        return (
          s &&
          s.state !== "unavailable" &&
          s.attributes.ondaire_playback === true &&
          id !== this._config.entity
        );
      })
      .sort();
  }

  get _stateObj() {
    return this._hass && this._config ? this._hass.states[this._config.entity] : undefined;
  }

  _position(stateObj) {
    const a = stateObj.attributes;
    let position = a.media_position || 0;
    const duration = a.media_duration || 0;
    if (stateObj.state === "playing" && a.media_position_updated_at) {
      const elapsed = (Date.now() - new Date(a.media_position_updated_at).getTime()) / 1000;
      position += Math.max(0, elapsed);
    }
    return { position: duration ? Math.min(position, duration) : position, duration };
  }

  _tick() {
    const stateObj = this._stateObj;
    const root = this.shadowRoot?.querySelector(".root");
    if (!stateObj || !root) return;
    const fill = root.querySelector(".progress-fill");
    const elapsedEl = root.querySelector(".elapsed");
    if (!fill) return;
    const { position, duration } = this._position(stateObj);
    fill.style.width = `${duration ? Math.min(100, (position / duration) * 100) : 0}%`;
    if (elapsedEl) elapsedEl.textContent = fmtTime(position);
  }

  _call(service, data) {
    return this._hass.callService("media_player", service, {
      entity_id: this._config.entity,
      ...data,
    });
  }

  // --- render --------------------------------------------------------------
  // The skeleton (top / tabbar / tabcontent) is built once. On each state
  // change only `.top` and the tab-active markers refresh; the tab content is
  // left alone (re-rendered only on tab switch, navigation, or fetch) so a
  // hass tick never resets a scroll position or steals focus from the search
  // box. The Players tab is the exception — it mirrors live speaker state.
  _buildSkeleton() {
    this.attachShadow({ mode: "open" });
    this.shadowRoot.innerHTML = `<style>${STYLE}</style><ha-card><div class="root">
      <div class="top"></div>
      <div class="tabbar">${TABS.map(([id, label]) =>
        `<div class="tab" data-tab="${id}">${label}</div>`,
      ).join("")}</div>
      <div class="tabcontent"></div>
    </div></ha-card>`;
    this.shadowRoot.querySelectorAll(".tab[data-tab]").forEach((el) =>
      el.addEventListener("click", () => this._switchTab(el.dataset.tab)),
    );
  }

  _render() {
    let first = false;
    if (!this.shadowRoot) {
      this._buildSkeleton();
      first = true;
    }
    const top = this.shadowRoot.querySelector(".top");
    const stateObj = this._stateObj;
    if (!stateObj) {
      top.innerHTML = `<div class="error">Entity not found: ${esc(this._config.entity)}</div>`;
      return;
    }

    const a = stateObj.attributes;
    const unavailable = stateObj.state === "unavailable";
    const playing = stateObj.state === "playing";
    const { position, duration } = this._position(stateObj);

    const joined = this._joinedSpeakers();
    const groupVol = this._groupVolume(joined);
    const groupMuted = joined.length > 0 && joined.every((s) => s.attributes.is_volume_muted);
    const noGroup = joined.length === 0;

    top.innerHTML = `
      <div class="header">
        <div class="name">${esc(a.friendly_name || this._config.entity)}</div>
        <div class="pill ${playing ? "playing" : ""}">${esc(stateObj.state)}</div>
      </div>
      <div class="body ${unavailable ? "unavailable" : ""}">
        <div class="art">${ICON_NOTE}</div>
        <div class="meta">
          <div class="title">${esc(a.media_title || "Nothing playing")}</div>
          <div class="sub">${esc([a.media_artist, a.media_album_name].filter(Boolean).join(" — "))}</div>
          <div class="progress">
            <div class="progress-track"><div class="progress-fill" style="width:${duration ? (position / duration) * 100 : 0}%"></div></div>
            <div class="times"><span class="elapsed">${fmtTime(position)}</span><span class="duration">${fmtTime(duration)}</span></div>
          </div>
          <div class="transport">
            <button class="icon-btn" data-action="prev" disabled title="Not supported by ondaire">${ICON_PREV}</button>
            <button class="icon-btn play" data-action="playpause">${playing ? ICON_PAUSE : ICON_PLAY}</button>
            <button class="icon-btn" data-action="stop">${ICON_STOP}</button>
            <button class="icon-btn" data-action="next">${ICON_NEXT}</button>
          </div>
          <div class="volume-row" title="${noGroup ? "Join a speaker to set group volume" : "Group volume — scales all joined speakers"}">
            <button class="icon-btn small" data-action="groupmute" ${noGroup ? "disabled" : ""}>${groupMuted ? ICON_MUTE : ICON_VOLUME}</button>
            <input type="range" class="volume" min="0" max="1" step="0.01" value="${groupVol}" ${noGroup ? "disabled" : ""}>
          </div>
        </div>
      </div>`;

    const art = a.entity_picture ? this._hass.hassUrl(a.entity_picture) : "";
    if (art) {
      const artEl = top.querySelector(".art");
      artEl.style.backgroundImage = `url("${art}")`;
      artEl.textContent = "";
    }

    this._wireTop(top, stateObj);
    this.shadowRoot.querySelectorAll(".tab[data-tab]").forEach((el) =>
      el.classList.toggle("active", el.dataset.tab === this._tab),
    );
    // Render tab content on first paint, and refresh only the live Players tab
    // on subsequent state changes (others persist until the user acts).
    if (first || this._tab === "players") this._renderActiveTab();
  }

  _wireTop(root, stateObj) {
    root.querySelector('[data-action="playpause"]')?.addEventListener("click", () =>
      this._call(stateObj.state === "playing" ? "media_pause" : "media_play"),
    );
    root.querySelector('[data-action="stop"]')?.addEventListener("click", () => this._call("media_stop"));
    root.querySelector('[data-action="next"]')?.addEventListener("click", () => this._call("media_next_track"));

    root.querySelector('[data-action="groupmute"]')?.addEventListener("click", () => {
      const joined = this._joinedSpeakers();
      const mute = !(joined.length > 0 && joined.every((s) => s.attributes.is_volume_muted));
      for (const s of joined) {
        this._hass.callService("media_player", "volume_mute", {
          entity_id: s.entity_id,
          is_volume_muted: mute,
        });
      }
    });
    this._wireSlider(root.querySelector(".volume"), (v) => this._setGroupVolume(v));

    root.querySelector(".progress-track")?.addEventListener("click", (e) => {
      if (!stateObj.attributes.media_duration) return;
      const rect = e.currentTarget.getBoundingClientRect();
      const ratio = Math.min(1, Math.max(0, (e.clientX - rect.left) / rect.width));
      this._call("media_seek", { seek_position: ratio * stateObj.attributes.media_duration });
    });
  }

  _wireSlider(el, onCommit) {
    if (!el) return;
    // Suppress re-render while the thumb is held, then commit on release.
    const down = () => (this._dragging = true);
    const up = () => (this._dragging = false);
    el.addEventListener("pointerdown", down);
    el.addEventListener("pointerup", up);
    el.addEventListener("pointercancel", up);
    el.addEventListener("change", (e) => {
      this._dragging = false;
      onCommit(parseFloat(e.target.value));
    });
  }

  // --- tabs ----------------------------------------------------------------
  _tabEl() {
    return this.shadowRoot?.querySelector(".tabcontent");
  }

  _switchTab(tab) {
    this._tab = tab;
    this.shadowRoot.querySelectorAll(".tab[data-tab]").forEach((el) =>
      el.classList.toggle("active", el.dataset.tab === tab),
    );
    this._renderActiveTab();
  }

  _renderActiveTab() {
    const el = this._tabEl();
    if (!el) return;
    if (this._tab === "players") return this._renderPlayers(el);
    if (this._tab === "media") return this._renderMedia(el);
    if (this._tab === "streams") return this._renderStreams(el);
    if (this._tab === "queue") return this._renderQueue(el);
  }

  // --- Players tab ---------------------------------------------------------
  _isJoined(speakerState) {
    const members = speakerState.attributes.group_members;
    return Array.isArray(members) && members[0] === this._config.entity;
  }

  _joinedSpeakers() {
    return this._playbackIds(this._hass)
      .map((id) => this._hass.states[id])
      .filter((s) => s && this._isJoined(s));
  }

  _groupVolume(joined) {
    if (!joined.length) return 0;
    const sum = joined.reduce((t, s) => t + (s.attributes.volume_level || 0), 0);
    return sum / joined.length;
  }

  _setGroupVolume(target) {
    // Proportional (Sonos-style): preserve each speaker's relative balance.
    const joined = this._joinedSpeakers();
    if (!joined.length) return;
    const cur = this._groupVolume(joined);
    for (const s of joined) {
      const v = s.attributes.volume_level || 0;
      const next = cur > 0 ? Math.min(1, Math.max(0, v * (target / cur))) : target;
      this._hass.callService("media_player", "volume_set", {
        entity_id: s.entity_id,
        volume_level: next,
      });
    }
  }

  _renderPlayers(el) {
    const joinedIds = new Set(this._joinedSpeakers().map((s) => s.entity_id));
    const speakers = this._playbackIds(this._hass).map((id) => this._hass.states[id]);

    // Reconcile optimistic toggles: once real state matches the desired state,
    // the pending flag (and its "busy" lock) is cleared and the toggle re-enables.
    for (const id of Object.keys(this._pending)) {
      if (joinedIds.has(id) === this._pending[id]) this._clearPending(id);
    }

    if (!speakers.length) {
      el.innerHTML = `<div class="muted">No speakers found on the network</div>`;
      return;
    }
    el.innerHTML = `<div class="list">${speakers
      .map((s) => {
        const id = s.entity_id;
        const busy = id in this._pending;
        // Optimistic: reflect the desired state instantly while in flight.
        const on = busy ? this._pending[id] : joinedIds.has(id);
        const name = s.attributes.friendly_name || id;
        const vol = s.attributes.volume_level != null ? s.attributes.volume_level : 0;
        // Handle only when this speaker is actually joined and settled; otherwise
        // just show its level as a static track (no draggable thumb).
        const interactive = on && !busy;
        const volCtl = interactive
          ? `<input type="range" class="spk-vol" data-vol="${esc(id)}" min="0" max="1" step="0.01" value="${vol}">`
          : `<div class="spk-track" aria-hidden="true"><div class="spk-track-fill" style="width:${Math.round(vol * 100)}%"></div></div>`;
        return `
          <div class="spk ${on ? "on" : "off"}">
            <div class="sw ${on ? "on" : ""} ${busy ? "busy" : ""}" data-toggle="${esc(id)}" role="switch" aria-checked="${on}"></div>
            <div class="spk-name">${esc(name)}</div>
            ${volCtl}
          </div>`;
      })
      .join("")}</div>`;

    el.querySelectorAll(".sw[data-toggle]").forEach((sw) => {
      sw.addEventListener("click", () => {
        const id = sw.dataset.toggle;
        if (id in this._pending) return; // already in flight — ignore
        const desired = !sw.classList.contains("on");
        this._pending[id] = desired;
        // Safety net: clear the lock even if no state update arrives.
        this._pendingTimers[id] = window.setTimeout(() => {
          this._clearPending(id);
          if (this._tab === "players") this._renderActiveTab();
        }, 6000);
        this._renderActiveTab(); // repaint: optimistic state + busy lock

        const done = () => {
          // Let the reconcile-on-render clear it when real state lands; if the
          // call failed, a repaint reverts to the true (unchanged) state.
          if (this._tab === "players") this._renderActiveTab();
        };
        const call = desired
          ? this._hass.callService("media_player", "join", {
              entity_id: this._config.entity,
              group_members: [id],
            })
          : this._hass.callService("media_player", "unjoin", { entity_id: id });
        Promise.resolve(call).then(done, () => {
          this._clearPending(id);
          done();
        });
      });
    });
    el.querySelectorAll(".spk-vol[data-vol]").forEach((sl) => {
      this._wireSlider(sl, (v) =>
        this._hass.callService("media_player", "volume_set", {
          entity_id: sl.dataset.vol,
          volume_level: v,
        }),
      );
    });
  }

  _clearPending(id) {
    delete this._pending[id];
    if (this._pendingTimers[id]) {
      window.clearTimeout(this._pendingTimers[id]);
      delete this._pendingTimers[id];
    }
  }

  // --- Media tab (library tree via browse_media, + search) -----------------
  _renderMedia(el) {
    // Persistent search box + a body we can refresh without disturbing the
    // input (keeps focus while typing / results while scrolling).
    el.innerHTML = `
      <div class="crumb">
        <input class="search" type="search" placeholder="Search library…" value="${esc(this._media.query)}">
        ${this._media.query ? `<button class="text-btn" data-media-clear>Clear</button>` : ""}
      </div>
      <div class="media-body"></div>`;

    const input = el.querySelector(".search");
    input?.addEventListener("input", (e) => this._onSearchInput(e.target.value));
    el.querySelector("[data-media-clear]")?.addEventListener("click", () => {
      this._media.query = "";
      this._media.results = null;
      this._renderActiveTab();
    });
    this._renderMediaBody();
    // Keep the caret at the end after a value-preserving re-render.
    if (input && this._media.query) {
      input.focus();
      const n = input.value.length;
      input.setSelectionRange(n, n);
    }
  }

  _mediaBodyEl() {
    return this.shadowRoot?.querySelector(".media-body");
  }

  _onSearchInput(value) {
    this._media.query = value;
    window.clearTimeout(this._searchTimer);
    if (!value.trim()) {
      this._media.results = null;
      this._renderMediaBody();
      return;
    }
    // Debounce so we don't fire a query per keystroke.
    this._searchTimer = window.setTimeout(() => this._runSearch(value.trim()), 350);
  }

  async _runSearch(query) {
    const m = this._media;
    m.searching = true;
    m.searchErr = "";
    this._renderMediaBody();
    try {
      const res = await this._hass.connection.sendMessagePromise({
        type: "ondaire/search",
        entity_id: this._config.entity,
        query,
      });
      // Ignore a stale response if the query moved on.
      if (m.query.trim() !== query) return;
      m.results = res.items || [];
    } catch (err) {
      m.searchErr = err.message || String(err);
    }
    m.searching = false;
    this._renderMediaBody();
  }

  _renderMediaBody() {
    const body = this._mediaBodyEl();
    if (!body) return;
    const m = this._media;

    // Search mode.
    if (m.query.trim()) {
      if (m.searching) return void (body.innerHTML = `<div class="muted">Searching…</div>`);
      if (m.searchErr) return void (body.innerHTML = `<div class="error">${esc(m.searchErr)}</div>`);
      const items = m.results || [];
      body.innerHTML = `<div class="list">${
        items
          .map((c) => {
            const cid = esc(c.media_content_id);
            const ct = esc(c.media_content_type);
            if (c.can_expand) {
              // Album/folder hit: open it, or add the whole thing to the queue.
              return `<div class="row">
                <button class="row" style="flex:1;padding:0" data-expand-search="${cid}" data-type="${ct}">
                  <span>${ICON_FOLDER}</span><span class="label">${esc(c.title)}</span>
                </button>
                <button class="icon-btn small" data-enqdir="${cid}" title="Add folder to queue">${ICON_PLUS}</button>
              </div>`;
            }
            return `<div class="row">
              <span>${ICON_NOTE_SMALL}</span><span class="label">${esc(c.title)}</span>
              <button class="icon-btn small" data-play="${cid}" data-type="${ct}" title="Play now">${ICON_PLAY}</button>
              <button class="icon-btn small" data-enq="${cid}" data-type="${ct}" title="Add to queue">${ICON_PLUS}</button>
            </div>`;
          })
          .join("") || `<div class="muted">No matches</div>`
      }</div>`;
      this._wireMediaItemButtons(body);
      return;
    }

    // Browse mode.
    if (m.loading) return void (body.innerHTML = `<div class="muted">Loading…</div>`);
    if (m.error) return void (body.innerHTML = `<div class="error">${esc(m.error)}</div>`);
    if (!m.cur) {
      this._browseMedia("library", "library", []);
      body.innerHTML = `<div class="muted">Loading…</div>`;
      return;
    }
    const children = m.cur.children || [];
    body.innerHTML = `
      ${m.stack.length ? `<div class="crumb"><button class="text-btn" data-media-up>← Back</button></div>` : ""}
      <div class="list">
        ${
          children
            .map((c) => {
              const cid = esc(c.media_content_id);
              const ct = esc(c.media_content_type);
              if (c.can_expand) {
                // Folder: click name to open; [+] enqueues the whole folder.
                return `<div class="row">
                  <button class="row" style="flex:1;padding:0" data-expand="${cid}" data-type="${ct}">
                    <span>${ICON_FOLDER}</span><span class="label">${esc(c.title)}</span>
                  </button>
                  <button class="icon-btn small" data-enqdir="${cid}" title="Add folder to queue">${ICON_PLUS}</button>
                </div>`;
              }
              const play = c.can_play ? `<button class="icon-btn small" data-play="${cid}" data-type="${ct}" title="Play now">${ICON_PLAY}</button>` : "";
              const add = c.can_play ? `<button class="icon-btn small" data-enq="${cid}" data-type="${ct}" title="Add to queue">${ICON_PLUS}</button>` : "";
              return `<div class="row">
                <span>${ICON_NOTE_SMALL}</span><span class="label">${esc(c.title)}</span>${play}${add}
              </div>`;
            })
            .join("") || `<div class="muted">Empty</div>`
        }
      </div>`;

    body.querySelector("[data-media-up]")?.addEventListener("click", () => {
      const prev = m.stack.pop() || { id: "library", type: "library" };
      this._browseMedia(prev.id, prev.type, m.stack.slice());
    });
    body.querySelectorAll("[data-expand]").forEach((b) =>
      b.addEventListener("click", () => {
        m.stack.push({ id: m.cur.media_content_id, type: m.cur.media_content_type });
        this._browseMedia(b.dataset.expand, b.dataset.type, m.stack.slice());
      }),
    );
    this._wireMediaItemButtons(body);
  }

  _wireMediaItemButtons(scope) {
    scope.querySelectorAll("[data-play]").forEach((b) =>
      b.addEventListener("click", (e) => {
        e.stopPropagation();
        this._call("play_media", { media_content_id: b.dataset.play, media_content_type: b.dataset.type });
      }),
    );
    scope.querySelectorAll("[data-enq]").forEach((b) =>
      b.addEventListener("click", (e) => {
        e.stopPropagation();
        this._call("play_media", {
          media_content_id: b.dataset.enq,
          media_content_type: b.dataset.type,
          enqueue: "add",
        });
      }),
    );
    scope.querySelectorAll("[data-enqdir]").forEach((b) =>
      b.addEventListener("click", (e) => {
        e.stopPropagation();
        this._hass.connection.sendMessagePromise({
          type: "ondaire/enqueue_dir",
          entity_id: this._config.entity,
          content_id: b.dataset.enqdir,
        });
      }),
    );
    // Search-result folder → jump into it in browse mode (clears the search).
    scope.querySelectorAll("[data-expand-search]").forEach((b) =>
      b.addEventListener("click", () =>
        this._openSearchFolder(b.dataset.expandSearch, b.dataset.type),
      ),
    );
  }

  _openSearchFolder(contentId, contentType) {
    const m = this._media;
    m.query = "";
    m.results = null;
    m.stack = [{ id: "library", type: "library" }]; // Back → library root
    m.cur = null;
    m.loading = true; // suppresses the auto-"library" browse in render
    this._renderActiveTab(); // reset the (now empty) search box
    this._browseMedia(contentId, contentType, m.stack.slice());
  }

  async _browseMedia(contentId, contentType, stack) {
    const m = this._media;
    m.loading = true;
    m.error = "";
    this._renderMediaBody();
    try {
      const res = await this._browseWS(contentId, contentType);
      m.cur = res;
      m.stack = stack;
    } catch (err) {
      m.error = err.message || String(err);
    }
    m.loading = false;
    this._renderMediaBody();
  }

  _browseWS(contentId, contentType) {
    return this._hass.connection.sendMessagePromise({
      type: "media_player/browse_media",
      entity_id: this._config.entity,
      ...(contentId && contentId !== "root"
        ? { media_content_id: contentId, media_content_type: contentType }
        : {}),
    });
  }

  // --- Streams tab (saved stream presets) ----------------------------------
  _renderStreams(el) {
    const st = this._streams;
    if (st.loading) return void (el.innerHTML = `<div class="muted">Loading…</div>`);
    if (st.error) return void (el.innerHTML = `<div class="error">${esc(st.error)}</div>`);
    if (st.items === null) {
      this._fetchStreams();
      el.innerHTML = `<div class="muted">Loading…</div>`;
      return;
    }
    if (!st.items.length) {
      el.innerHTML = `<div class="muted">No stream presets configured</div>`;
      return;
    }
    el.innerHTML = `<div class="list">${st.items
      .map(
        (c) => `<button class="row" data-play="${esc(c.media_content_id)}" data-type="${esc(c.media_content_type)}">
          <span>${ICON_NOTE_SMALL}</span><span class="label">${esc(c.title)}</span>${ICON_PLAY}
        </button>`,
      )
      .join("")}</div>`;
    el.querySelectorAll("[data-play]").forEach((b) =>
      b.addEventListener("click", () =>
        this._call("play_media", { media_content_id: b.dataset.play, media_content_type: b.dataset.type }),
      ),
    );
  }

  async _fetchStreams() {
    const st = this._streams;
    st.loading = true;
    st.error = "";
    try {
      const res = await this._browseWS("presets", "presets");
      st.items = res.children || [];
    } catch (err) {
      st.error = err.message || String(err);
    }
    st.loading = false;
    if (this._tab === "streams") this._renderActiveTab();
  }

  // --- Queue tab (ondaire websocket) ---------------------------------------
  _renderQueue(el) {
    const q = this._queue;
    const curUri = this._stateObj?.attributes.media_content_id || "";
    // Refetch when the now-playing track changed since the last queue load.
    if (q.items !== null && q.forUri !== curUri && !q.loading) {
      q.items = null;
    }
    if (q.loading) return void (el.innerHTML = `<div class="muted">Loading…</div>`);
    if (q.error) {
      el.innerHTML = `<div class="error">${esc(q.error)}</div>
        <button class="text-btn" data-q-refresh>Retry</button>`;
      el.querySelector("[data-q-refresh]")?.addEventListener("click", () => this._fetchQueue());
      return;
    }
    if (q.items === null) {
      this._fetchQueue();
      el.innerHTML = `<div class="muted">Loading…</div>`;
      return;
    }
    if (!q.items.length) {
      el.innerHTML = `<div class="muted">Queue is empty</div>
        <button class="text-btn" data-q-refresh>Refresh</button>`;
      el.querySelector("[data-q-refresh]")?.addEventListener("click", () => this._fetchQueue());
      return;
    }
    el.innerHTML = `
      <div class="list">
        ${q.items
          .map((it, i) => {
            const label = it.title || it.uri;
            const sub = it.artist ? ` <span class="sub2">· ${esc(it.artist)}</span>` : "";
            return `<div class="row">
              <span class="label">${i + 1}. ${esc(label)}${sub}</span>
              <button class="icon-btn small" data-qplay="${i}" data-uri="${esc(it.uri)}" title="Play now">${ICON_PLAY}</button>
              <button class="icon-btn small" data-qdel="${i}" data-uri="${esc(it.uri)}" title="Remove">${ICON_TRASH}</button>
            </div>`;
          })
          .join("")}
      </div>
      <button class="text-btn" data-q-refresh>Refresh</button>`;

    el.querySelector("[data-q-refresh]")?.addEventListener("click", () => this._fetchQueue());
    el.querySelectorAll("[data-qplay]").forEach((b) =>
      b.addEventListener("click", () => this._queueAction("play", +b.dataset.qplay, b.dataset.uri)),
    );
    el.querySelectorAll("[data-qdel]").forEach((b) =>
      b.addEventListener("click", () => this._queueAction("remove", +b.dataset.qdel, b.dataset.uri)),
    );
  }

  async _fetchQueue() {
    const q = this._queue;
    q.loading = true;
    q.error = "";
    q.forUri = this._stateObj?.attributes.media_content_id || "";
    if (this._tab === "queue") this._renderActiveTab();
    try {
      const res = await this._hass.connection.sendMessagePromise({
        type: "ondaire/queue/list",
        entity_id: this._config.entity,
      });
      q.items = res.items || [];
    } catch (err) {
      q.error = err.message || String(err);
    }
    q.loading = false;
    if (this._tab === "queue") this._renderActiveTab();
  }

  async _queueAction(action, index, uri) {
    try {
      await this._hass.connection.sendMessagePromise({
        type: `ondaire/queue/${action}`,
        entity_id: this._config.entity,
        index,
        uri,
      });
    } catch (err) {
      this._queue.error = err.message || String(err);
    }
    this._fetchQueue();
  }
}

// HA's frontend replaces window.customElements with a scoped-registry polyfill
// AFTER extra modules like this one may already have run; a define made against
// the native registry before the swap is invisible to the polyfill's get(), and
// Lovelace then reports "custom element not found". Register now, and register
// again if/when the registry object is swapped out from under us.
function register() {
  try {
    if (!customElements.get("ondaire-card")) {
      customElements.define("ondaire-card", OndaireCard);
    }
  } catch (e) {
    /* someone else won the race — fine */
  }
}
register();
const initialRegistry = window.customElements;
const registryWatch = setInterval(() => {
  if (window.customElements !== initialRegistry || customElements.get("home-assistant")) {
    register();
    clearInterval(registryWatch);
  }
}, 50);
setTimeout(() => clearInterval(registryWatch), 20000);

window.customCards = window.customCards || [];
window.customCards.push({
  type: "ondaire-card",
  name: "Ondaire",
  description: "Control a single ondaire room — players, media, streams, and queue, all proxied through Home Assistant.",
});
