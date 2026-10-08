/* PeerBuddy — Prysm peer connectivity & scoring dashboard.
 * Vanilla JS, no build step. Polls the beacon node's REST API every 10 seconds. */
'use strict';
(function () {
  // ------------------------------------------------------------------ constants
  const POLL_MS = 10000;
  const LS = { url: 'peerbuddy.nodeUrl', theme: 'peerbuddy.theme', auto: 'peerbuddy.auto' };
  const STATES = ['CONNECTED', 'CONNECTING', 'DISCONNECTING', 'DISCONNECTED'];
  const CLIENTS = ['prysm', 'lighthouse', 'teku', 'nimbus', 'lodestar', 'grandine', 'erigon/caplin', 'rust-libp2p', 'js-libp2p', 'unknown'];
  const CLIENT_VAR = {
    'prysm': '--c-prysm', 'lighthouse': '--c-lighthouse', 'teku': '--c-teku', 'nimbus': '--c-nimbus', 'lodestar': '--c-lodestar',
    'grandine': '--c-grandine', 'erigon/caplin': '--c-erigon', 'rust-libp2p': '--c-rust', 'js-libp2p': '--c-js', 'unknown': '--c-unknown',
  };
  // Same list and last-match-wins rule as the node's classifier; only used when the API omits agent_type.
  const KNOWN_AGENT_ORDER = ['erigon/caplin', 'grandine', 'js-libp2p', 'lighthouse', 'lodestar', 'nimbus', 'prysm', 'teku', 'rust-libp2p'];
  const SOURCES = ['dial', 'rpc-status', 'rpc-ping', 'rpc-metadata', 'rpc-request', 'rpc-response', 'rate-limit', 'gossip', 'sync', 'backfill', 'das', 'unknown'];
  const ASPECTS = [['strikes', 'Strikes'], ['peer_status', 'Peer status'], ['gossip', 'Gossip score'], ['bad_ip', 'IP colocation']];
  const ASPECT_NAME = Object.fromEntries(ASPECTS);
  const ASPECT_CLS = { strikes: 'bad', peer_status: 'warn', gossip: 'info', bad_ip: '' };
  const WINDOWS = [['15m', 9e5], ['1h', 36e5], ['6h', 216e5], ['24h', 864e5], ['all', 0]];
  const WINDOW_MS = Object.fromEntries(WINDOWS);
  const VIEWS = { overview: 'Overview', peers: 'Peers', clients: 'Clients', greylist: 'Grey list', gossip: 'Gossip', chain: 'Chain', peer: 'Peer' };
  const TENURE_BUCKETS = [
    { label: '< 1m', max: 60 }, { label: '1–5m', max: 300 }, { label: '5–30m', max: 1800 },
    { label: '30m–2h', max: 7200 }, { label: '2–12h', max: 43200 }, { label: '12h+', max: Infinity },
  ];

  // ------------------------------------------------------------------ utils
  const $ = (sel, root) => (root || document).querySelector(sel);
  const $$ = (sel, root) => Array.from((root || document).querySelectorAll(sel));
  const esc = s => String(s == null ? '' : s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const attr = s => esc(s);
  const toInt = v => { if (v == null || v === '') return null; const n = Number(v); return Number.isFinite(n) ? n : null; };
  const cap = s => s ? s.charAt(0) + s.slice(1).toLowerCase() : '';
  const short = id => (id && id.length > 18) ? id.slice(0, 8) + '…' + id.slice(-6) : (id || '');
  const fmtNum = n => (n == null || !Number.isFinite(n)) ? '–' : n.toLocaleString();
  const pct = (a, b) => b ? Math.round(100 * a / b) : 0;
  const sum = arr => arr.reduce((s, v) => s + v, 0);
  const clamp = (v, lo, hi) => Math.max(lo, Math.min(hi, v));
  const plural = (n, s) => n + ' ' + s + (n === 1 ? '' : 's');

  function parseGoDuration(s) {
    if (!s || s === 'unknown') return null;
    const re = /(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)/g;
    let m, total = 0, any = false;
    while ((m = re.exec(s))) {
      any = true;
      const v = parseFloat(m[1]);
      switch (m[2]) {
        case 'h': total += v * 3600; break;
        case 'm': total += v * 60; break;
        case 's': total += v; break;
        case 'ms': total += v / 1e3; break;
        case 'us': case 'µs': total += v / 1e6; break;
        default: total += v / 1e9;
      }
    }
    return any ? total : null;
  }
  function fmtDur(sec) {
    if (sec == null || !Number.isFinite(sec)) return '–';
    sec = Math.max(0, Math.round(sec));
    if (sec < 60) return sec + 's';
    if (sec < 3600) return Math.floor(sec / 60) + 'm ' + (sec % 60) + 's';
    if (sec < 86400) return Math.floor(sec / 3600) + 'h ' + Math.floor((sec % 3600) / 60) + 'm';
    return Math.floor(sec / 86400) + 'd ' + Math.floor((sec % 86400) / 3600) + 'h';
  }
  const ago = ts => ts ? fmtDur((Date.now() - ts) / 1000) + ' ago' : '–';
  const fmtTime = ts => ts ? new Date(ts).toLocaleString(undefined, { hour12: false }) : '–';
  const fmtClock = ts => ts ? new Date(ts).toLocaleTimeString(undefined, { hour12: false }) : '–';
  const fmtScore = s => (s == null) ? '–' : (Math.abs(s) >= 100 ? Math.round(s).toLocaleString() : (Math.round(s * 10) / 10).toString());
  function median(sorted) { const n = sorted.length; return n ? (n % 2 ? sorted[(n - 1) / 2] : (sorted[n / 2 - 1] + sorted[n / 2]) / 2) : null; }
  function quantile(sorted, q) { const n = sorted.length; return n ? sorted[Math.min(n - 1, Math.floor(q * n))] : null; }
  function countBy(arr) { const m = new Map(); for (const k of arr) m.set(k, (m.get(k) || 0) + 1); return m; }
  function topN(map, n) { return Array.from(map.entries()).sort((a, b) => b[1] - a[1] || String(a[0]).localeCompare(String(b[0]))).slice(0, n); }
  function agentTypeOf(agent) { const a = (agent || '').toLowerCase(); let found = 'unknown'; for (const t of KNOWN_AGENT_ORDER) if (a.includes(t)) found = t; return found; }
  function shortTopic(t) { const m = /^\/eth2\/[0-9a-f]+\/(.+?)\/ssz_snappy$/.exec(t || ''); return m ? m[1] : (t || ''); }
  const clientColor = type => 'var(' + (CLIENT_VAR[type] || '--c-unknown') + ')';
  const sourceColor = s => 'var(--s-' + (SOURCES.includes(s) ? s : 'unknown') + ')';
  function compare(a, b) { if (a == null && b == null) return 0; if (a == null) return 1; if (b == null) return -1; return typeof a === 'string' ? a.localeCompare(b) : a - b; }
  function copyText(text) { if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(text).catch(() => {}); }
  function medianTenure(peers) { return median(peers.map(p => p.tenureSec).filter(t => t != null).sort((a, b) => a - b)); }
  const cleanVerdict = s => (s || '').replace(/^peer is grey-listed: /, '').replace(/^peer is from a bad IP: /, '');
  function setPath(obj, path, val) { const parts = path.split('.'); let o = obj; for (let i = 0; i < parts.length - 1; i++) o = o[parts[i]]; o[parts[parts.length - 1]] = val; }
  function getPath(obj, path) { return path.split('.').reduce((o, k) => (o == null ? o : o[k]), obj); }

  // ------------------------------------------------------------------ state
  const scope = { client: '', agent: '', window: '1h' };
  const state = {
    nodeUrl: '', auto: true, loading: false, error: null, warnings: [], fetchedAt: 0, nextAt: 0,
    spec: { slotsPerEpoch: 32, secondsPerSlot: 12, loaded: false },
    raw: {}, cfg: {}, node: {}, peers: [], peersById: new Map(), agents: [], rejections: [], strikeEvents: [],
    route: { view: 'overview', param: '' },
    mounted: '',
    cursor: -1,
    filters: {
      peers: { preset: 'connected', q: '', dir: '', aspect: '', source: '', peerLimit: 300 },
      clients: { q: '', preset: 'all' },
      greylist: { aspect: '', reason: '' },
      gossip: { groupBy: 'topic', q: '', pick: '' },
      chain: { q: '', connectedOnly: true },
    },
    sort: {
      peers: { key: 'rank', dir: 'asc' }, agents: { key: 'connected', dir: 'desc' }, chain: { key: 'headSlot', dir: 'desc' },
      grey: { key: 'recovery', dir: 'asc' }, near: { key: 'strikes', dir: 'desc' },
    },
    detail: { id: '', data: null, eth: null, error: null, loading: false },
    dd: { open: false, q: '', cursor: 0 },
    nodeFormOpen: false,
  };
  const memo = { key: '', v: null };
  const windowMs = () => WINDOW_MS[scope.window] || 0;
  const sinceTs = () => windowMs() ? Date.now() - windowMs() : 0;
  const winLabel = () => scope.window === 'all' ? 'all retained' : 'last ' + scope.window;

  // ------------------------------------------------------------------ api & polling
  const nodeBase = () => state.nodeUrl.replace(/\/+$/, '');
  async function api(path) {
    const res = await fetch(nodeBase() + path, { headers: { Accept: 'application/json' }, cache: 'no-store' });
    if (!res.ok) {
      let msg = 'HTTP ' + res.status;
      try { const j = await res.json(); if (j && j.message) msg += ': ' + j.message; } catch (_) { /* no body */ }
      throw new Error(msg);
    }
    return res.json();
  }

  let pollTimer = null;
  function schedule() {
    clearTimeout(pollTimer);
    if (!state.auto) { state.nextAt = 0; return; }
    state.nextAt = Date.now() + POLL_MS;
    pollTimer = setTimeout(() => refresh('poll'), POLL_MS);
  }

  async function refresh(reason) {
    if (state.loading) return;
    if (reason === 'poll' && document.hidden) { schedule(); return; }
    state.loading = true;
    renderLive();
    const started = Date.now();
    const reqs = {
      config: '/prysm/v1/node/peers/scoring/config',
      peers: '/prysm/v1/node/peers/scoring',
      agents: '/prysm/v1/node/peers/scoring/agents',
      rejections: '/prysm/v1/node/gossip/rejections',
      ethPeers: '/eth/v1/node/peers',
      syncing: '/eth/v1/node/syncing',
      identity: '/eth/v1/node/identity',
      version: '/eth/v1/node/version',
      peerCount: '/eth/v1/node/peer_count',
      trusted: '/prysm/v1/node/trusted_peers',
      finality: '/eth/v1/beacon/states/head/finality_checkpoints',
    };
    if (!state.spec.loaded) reqs.spec = '/eth/v1/config/spec';
    const keys = Object.keys(reqs);
    const results = await Promise.allSettled(keys.map(k => api(reqs[k])));
    const raw = {}, failed = [];
    results.forEach((r, i) => { if (r.status === 'fulfilled') raw[keys[i]] = r.value; else failed.push({ key: keys[i], err: r.reason }); });
    if (raw.spec && raw.spec.data) {
      state.spec.slotsPerEpoch = toInt(raw.spec.data.SLOTS_PER_EPOCH) || 32;
      state.spec.secondsPerSlot = toInt(raw.spec.data.SECONDS_PER_SLOT) || 12;
      state.spec.loaded = true;
      delete raw.spec;
    }
    const required = ['config', 'peers'];
    const fatal = failed.find(f => required.includes(f.key));
    if (fatal) {
      const hint = /HTTP 404/.test(fatal.err.message)
        ? ' The node does not serve the peer scoring API; it needs a build that includes it.'
        : (fatal.err instanceof TypeError ? ' Check the URL, that the node is running, and that this origin is allowed by --http-cors-domain.' : '');
      state.error = 'Cannot load ' + reqs[fatal.key] + ' from ' + nodeBase() + ' (' + fatal.err.message + ').' + hint;
    } else {
      state.error = null;
      state.raw = Object.assign({}, state.raw, raw);
      state.fetchedAt = started;
      derive();
    }
    state.warnings = failed.filter(f => !required.includes(f.key) && f.key !== 'spec').map(f => reqs[f.key] + ': ' + f.err.message);
    state.loading = false;
    schedule();
    render();
    if (state.route.view === 'peer' && state.route.param && !fatal) loadPeerDetail(state.route.param);
  }

  // ------------------------------------------------------------------ derived model
  function peerCtx() {
    const r = state.raw;
    const ethById = new Map();
    for (const p of ((r.ethPeers && r.ethPeers.data) || [])) ethById.set(p.peer_id, p);
    const trusted = new Set(((r.trusted && r.trusted.peers) || []).map(p => p.peer_id));
    return { now: Date.now(), ethById, trusted, ourHead: state.node.headSlot };
  }

  function derive() {
    const r = state.raw;
    const cfg = (r.config && r.config.data) || {};
    state.cfg = {
      threshold: toInt(cfg.strike_grey_list_threshold) == null ? 5 : toInt(cfg.strike_grey_list_threshold),
      historySize: toInt(cfg.strike_history_size),
      decay: cfg.decay_interval || '',
      decaySec: parseGoDuration(cfg.decay_interval),
      gossipThreshold: toInt(cfg.gossip_grey_list_threshold) == null ? -16000 : toInt(cfg.gossip_grey_list_threshold),
      statusTTL: cfg.status_grey_list_ttl || '',
      maxRejections: toInt(cfg.max_gossip_rejections_per_peer),
      ourHeadSlot: toInt(cfg.our_head_slot),
      highestKnownHeadSlot: toInt(cfg.highest_known_head_slot),
      trackedPeers: toInt(cfg.tracked_peer_count),
      peersWithRejections: toInt(cfg.peers_with_gossip_rejections),
    };
    const sync = (r.syncing && r.syncing.data) || {};
    const fin = (r.finality && r.finality.data && r.finality.data.finalized) || {};
    state.node = {
      version: (r.version && r.version.data && r.version.data.version) || '',
      identity: (r.identity && r.identity.data) || null,
      headSlot: toInt(sync.head_slot) == null ? state.cfg.ourHeadSlot : toInt(sync.head_slot),
      syncDistance: toInt(sync.sync_distance),
      isSyncing: !!sync.is_syncing,
      isOptimistic: !!sync.is_optimistic,
      elOffline: !!sync.el_offline,
      finalizedEpoch: toInt(fin.epoch),
      finalizedRoot: fin.root || '',
      peerCount: (r.peerCount && r.peerCount.data) || null,
    };
    const ctx = peerCtx();
    ctx.now = state.fetchedAt;
    const peers = ((r.peers && r.peers.data) || []).map(p => enrichPeer(p, ctx));
    peers.sort((a, b) => (b.greyListed - a.greyListed) || (b.standing - a.standing) || compare(a.id, b.id));
    peers.forEach((p, i) => { p.rank = i; });
    state.peers = peers;
    state.peersById = new Map(peers.map(p => [p.id, p]));
    state.strikeEvents = peers.flatMap(p => p.history.map(h => ({ at: h.at, source: h.source, reason: h.reason, peer: p }))).sort((a, b) => b.at - a.at);

    state.rejections = ((r.rejections && r.rejections.data) || []).map(rj => ({
      peerId: rj.peer_id, topic: rj.topic || '', topicShort: shortTopic(rj.topic), agent: rj.agent || '',
      agentType: rj.agent_type || agentTypeOf(rj.agent), reason: rj.reason || '', at: Date.parse(rj.timestamp) || 0,
    }));
    state.rejections.sort((a, b) => b.at - a.at);

    const byAgent = new Map();
    for (const p of peers) {
      const k = p.agent || 'unknown';
      let a = byAgent.get(k);
      if (!a) { a = { agent: k, agentType: p.agentType, peers: [] }; byAgent.set(k, a); }
      a.peers.push(p);
    }
    for (const a of ((r.agents && r.agents.data) || [])) {
      const k = a.agent || 'unknown';
      if (!byAgent.has(k)) byAgent.set(k, { agent: k, agentType: a.agent_type || 'unknown', peers: [] });
    }
    state.agents = Array.from(byAgent.values());
    memo.key = '';
    document.title = 'PeerBuddy · ' + peers.filter(p => p.connected).length + ' peers';
  }

  function enrichPeer(p, ctx) {
    const id = p.peer_id;
    const connected = p.connection_state === 'CONNECTED';
    const connectedAt = p.connected_at ? (Date.parse(p.connected_at) || 0) : 0;
    const details = p.grey_list_details || {};
    const aspects = ASPECTS.map(a => a[0]).filter(k => details[k]);
    const recovery = p.grey_list_recovery || {};
    const recoverySec = {};
    let recoveryMax = null, recoveryUnknown = false;
    for (const k of Object.keys(recovery)) {
      const s = parseGoDuration(recovery[k]);
      recoverySec[k] = s;
      if (s == null) recoveryUnknown = true; else recoveryMax = Math.max(recoveryMax == null ? 0 : recoveryMax, s);
    }
    const strikes = p.strikes || {};
    const history = (strikes.history || []).map(h => ({ source: h.source || 'unknown', reason: h.reason || '', at: Date.parse(h.timestamp) || 0 }));
    const st = p.rpc_status || null;
    const cs = (st && st.chain_state) || null;
    const headSlot = cs ? toInt(cs.head_slot) : null;
    const eth = ctx.ethById.get(id);
    const gossip = p.gossip || {};
    const rejections = (gossip.rejections || []).map(rj => ({
      topic: rj.topic || '', topicShort: shortTopic(rj.topic), agent: rj.agent || '', agentType: rj.agent_type || agentTypeOf(rj.agent),
      reason: rj.reason || '', at: Date.parse(rj.timestamp) || 0,
    }));
    const standing = strikes.standing_count || 0;
    const threshold = strikes.grey_list_threshold || state.cfg.threshold || 5;
    const greyListed = !!p.grey_listed;
    return {
      id, short: short(id), agent: p.agent || '', agentType: p.agent_type || agentTypeOf(p.agent),
      state: p.connection_state || 'DISCONNECTED', direction: p.direction || 'UNKNOWN', connected, connectedAt,
      tenureSec: connected && connectedAt ? (ctx.now - connectedAt) / 1000 : null,
      greyListed, details, aspects, exemption: p.grey_list_exemption || '',
      recovery, recoverySec, recoveryMax, recoveryUnknown, recoveryEnd: recoveryMax == null ? null : ctx.now + recoveryMax * 1000,
      standing, threshold, nearThreshold: !greyListed && standing > 0 && standing >= threshold - 1,
      history, lastStrike: history.length ? history[history.length - 1] : null,
      status: st, chain: cs, headSlot,
      headDelta: (headSlot != null && ctx.ourHead != null) ? headSlot - ctx.ourHead : null,
      finalizedEpoch: cs ? toInt(cs.finalized_epoch) : null, earliestSlot: cs ? toInt(cs.earliest_available_slot) : null,
      forkDigest: (cs && cs.fork_digest) || '', validationError: (st && st.validation_error) || '',
      statusAt: st ? (Date.parse(st.last_updated) || 0) : 0,
      gossipScore: gossip.score == null ? 0 : gossip.score, behaviourPenalty: gossip.behaviour_penalty == null ? 0 : gossip.behaviour_penalty,
      topicScores: gossip.topic_scores || null, rejections, rejectionsCount: rejections.length,
      addr: (eth && eth.last_seen_p2p_address) || '', enr: (eth && eth.enr) || '', trusted: ctx.trusted.has(id),
    };
  }

  // Everything a view shows is derived from the scoped slice: client/agent narrow the peer set,
  // the window narrows time-stamped events (strikes, rejections).
  function scoped() {
    const key = [state.fetchedAt, scope.client, scope.agent, scope.window, Math.floor(Date.now() / 30000)].join('|');
    if (memo.key === key) return memo.v;
    const inScope = p => (!scope.client || p.agentType === scope.client) && (!scope.agent || (p.agent || 'unknown') === scope.agent);
    const peers = state.peers.filter(inScope);
    const ids = new Set(peers.map(p => p.id));
    const since = sinceTs();
    const strikes = state.strikeEvents.filter(e => ids.has(e.peer.id) && e.at >= since);
    const rejections = state.rejections.filter(r =>
      (!scope.client || r.agentType === scope.client) && (!scope.agent || (r.agent || 'unknown') === scope.agent) && r.at >= since);
    const agents = state.agents.filter(a => (!scope.client || a.agentType === scope.client) && (!scope.agent || a.agent === scope.agent));
    memo.key = key;
    memo.v = { peers, ids, strikes, rejections, agents, since, connected: peers.filter(p => p.connected), grey: peers.filter(p => p.greyListed) };
    return memo.v;
  }
  function agentStats(a) {
    const s = scoped();
    const connected = a.peers.filter(p => p.connected);
    const strikes = s.strikes.filter(e => (e.peer.agent || 'unknown') === a.agent);
    const bySource = countBy(strikes.map(e => e.source));
    const rejections = s.rejections.filter(r => (r.agent || 'unknown') === a.agent).length;
    return { connected: connected.length, known: a.peers.length, grey: a.peers.filter(p => p.greyListed).length, strikes: strikes.length, bySource, rejections, tenure: medianTenure(connected) };
  }

  // ------------------------------------------------------------------ components
  const badge = (text, cls, tip) => '<span class="badge ' + (cls || '') + '"' + (tip ? ' data-tip="' + attr(tip) + '"' : '') + '>' + esc(text) + '</span>';
  const stateBadge = s => badge(cap(s), { CONNECTED: 'ok', CONNECTING: 'info', DISCONNECTING: 'warn', DISCONNECTED: '' }[s] || '');
  const dirBadge = d => d === 'INBOUND' ? badge('in', 'outline sm', 'Inbound: the peer dialed us') : d === 'OUTBOUND' ? badge('out', 'outline sm', 'Outbound: we dialed the peer') : '';
  const aspectBadge = (a, tip) => badge(ASPECT_NAME[a] || a, ASPECT_CLS[a] || '', tip);
  const dot = type => '<span class="dot" style="background:' + clientColor(type) + '" data-tip="' + attr(type) + '"></span>';
  const clientLabel = type => '<span class="peer-cell">' + dot(type) + esc(type) + '</span>';
  const srcLabel = s => '<span class="src"><span class="sw" style="background:' + sourceColor(s) + '"></span>' + esc(s) + '</span>';
  const peerLink = (id, label) => '<span class="link" data-peer="' + attr(id) + '" title="' + attr(id) + '">' + esc(label || short(id)) + '</span>';
  const copyBtn = text => '<button class="copy" type="button" data-copy="' + attr(text) + '" title="Copy">⧉</button>';
  const peerCell = p => '<span class="peer-cell">' + dot(p.agentType) + '<span class="id" title="' + attr(p.id) + '">' + esc(p.short) + '</span>' +
    (p.agent ? '<span class="ag" title="' + attr(p.agent) + '">' + esc(p.agent) + '</span>' : '<span class="ag faint">unknown agent</span>') +
    (p.trusted ? badge('trusted', 'accent sm') : '') + '</span>';
  const liveSince = ts => '<span data-live="since" data-ts="' + ts + '">' + fmtDur((Date.now() - ts) / 1000) + '</span>';
  const liveAgo = ts => '<span data-live="ago" data-ts="' + ts + '">' + ago(ts) + '</span>';
  const liveCountdown = end => '<span data-live="countdown" data-end="' + end + '">' + fmtDur((end - Date.now()) / 1000) + '</span>';

  function tile(o) {
    const tag = o.href ? 'a' : 'div';
    return '<' + tag + ' class="tile ' + (o.cls || '') + (o.pick ? ' pick' : '') + (o.on ? ' on' : '') + '"' + (o.href ? ' href="' + attr(o.href) + '"' : '') +
      (o.data ? ' ' + o.data : '') + '><div class="tile-l">' + o.label + '</div><div class="tile-v">' + o.value + (o.unit ? '<small>' + esc(o.unit) + '</small>' : '') + '</div>' +
      '<div class="tile-s">' + (o.sub || []).filter(Boolean).map(s => '<span>' + s + '</span>').join('') + '</div></' + tag + '>';
  }
  function sec(title, body, o) {
    o = o || {};
    return '<section class="sec' + (o.cls ? ' ' + o.cls : '') + '"><div class="sec-h"><h2>' + esc(title) + '</h2>' + (o.sub ? '<span class="sub">' + o.sub + '</span>' : '') +
      (o.right ? '<span class="right">' + o.right + '</span>' : '') + '</div>' + body + '</section>';
  }
  function meter(p) {
    const ratio = p.threshold ? p.standing / p.threshold : 0;
    const cls = ratio >= 1 ? 'bad' : ratio >= 0.6 ? 'warn' : '';
    return '<span class="meter" data-tip="' + attr(p.standing + ' standing strikes; grey-listed at ' + p.threshold + '. One strike is forgiven every ' + (state.cfg.decay || '?') + '.') + '"><span class="meter-t"><span class="meter-f ' + cls + '" style="width:' + clamp(ratio * 100, 0, 100) + '%"></span></span><span class="meter-v">' + p.standing + '/' + p.threshold + '</span></span>';
  }
  // Horizontal bars. items: {label, value, color, key, tip, on}; opts: {max, total, data, wide, fmt, empty}
  function bars(items, o) {
    o = o || {};
    if (!items.length) return '<div class="empty">' + esc(o.empty || 'Nothing to show') + '</div>';
    const mx = o.max != null ? o.max : Math.max.apply(null, items.map(i => i.value).concat([1]));
    const fmt = o.fmt || fmtNum;
    return '<div class="bars">' + items.map(i =>
      '<div class="bar' + (o.wide ? ' wide' : '') + (o.tall ? ' tall' : '') + (o.data ? ' pick' : '') + (i.on ? ' on' : '') + '"' +
      (o.data ? ' data-' + o.data + '="' + attr(i.key != null ? i.key : i.label) + '"' : '') + (i.tip || i.title ? ' data-tip="' + attr(i.tip || i.title) + '"' : '') + '>' +
      '<span class="bar-l">' + (i.dot ? '<span class="sw" style="background:' + i.dot + '"></span>' : '') + esc(i.label) + '</span>' +
      '<span class="bar-t"><span class="bar-f" style="width:' + Math.max(1, 100 * i.value / mx) + '%;background:' + (i.color || 'var(--accent)') + '"></span></span>' +
      '<span class="bar-v">' + fmt(i.value) + (o.total ? '<small>' + pct(i.value, o.total) + '%</small>' : '') + '</span></div>').join('') + '</div>';
  }
  function cols(items, o) {
    o = o || {};
    const mx = Math.max.apply(null, items.map(i => i.value).concat([1]));
    return '<div class="cols">' + items.map(i =>
      '<div class="col' + (o.data ? ' pick' : '') + '"' + (o.data ? ' data-' + o.data + '="' + attr(i.key != null ? i.key : i.label) + '"' : '') + (i.tip ? ' data-tip="' + attr(i.tip) + '"' : '') + '>' +
      '<div class="col-b ' + (i.cls || '') + '" style="height:' + Math.max(2, 100 * i.value / mx) + '%"><span class="col-v">' + fmtNum(i.value) + '</span></div><div class="col-l">' + esc(i.label) + '</div></div>').join('') + '</div>';
  }
  // Stacked composition bar plus legend table. items: {label, value, color, key, on}; o.cols: [{h, get(item)}]
  function stack(items, o) {
    o = o || {};
    const total = sum(items.map(i => i.value));
    if (!total) return '<div class="empty">' + esc(o.empty || 'Nothing to show') + '</div>';
    const extra = o.cols || [];
    return '<div class="stack">' + items.filter(i => i.value > 0).map(i =>
      '<span style="width:' + (100 * i.value / total) + '%;background:' + i.color + '" data-tip="' + attr(i.label + ': ' + i.value + ' (' + pct(i.value, total) + '%)') + '"' + (o.data ? ' data-' + o.data + '="' + attr(i.key != null ? i.key : i.label) + '"' : '') + '></span>').join('') + '</div>' +
      '<div class="legend" style="--cols:' + (extra.length + 1) + '">' +
      '<div class="legend-row head"><span></span><span>' + esc(o.head || '') + '</span><span class="n">' + esc(o.valueHead || 'count') + '</span>' + extra.map(c => '<span class="n">' + esc(c.h) + '</span>').join('') + '</div>' +
      items.map(i => '<div class="legend-row' + (o.data ? ' pick' : '') + (i.on ? ' on' : '') + '"' + (o.data ? ' data-' + o.data + '="' + attr(i.key != null ? i.key : i.label) + '"' : '') + '>' +
        '<span class="sw" style="background:' + i.color + '"></span><span class="trunc">' + esc(i.label) + '</span><span class="n">' + fmtNum(i.value) + '</span>' +
        extra.map(c => '<span class="n' + (c.cls ? ' ' + c.cls(i) : '') + '">' + c.get(i) + '</span>').join('') + '</div>').join('') + '</div>';
  }
  function miniStack(bySource, total, max) {
    const keys = Object.keys(bySource).sort((a, b) => bySource[b] - bySource[a]);
    if (!total) return '<span class="faint">—</span>';
    const w = max ? Math.max(10, Math.round(110 * total / max)) : 110;
    return '<span class="stack mini" style="width:' + w + 'px" data-tip="' + attr(keys.map(k => k + ': ' + bySource[k]).join('\n')) + '">' +
      keys.map(k => '<span style="width:' + (100 * bySource[k] / total) + '%;background:' + sourceColor(k) + '"></span>').join('') + '</span>';
  }
  function th(label, key, sort, cls) {
    const on = sort.key === key;
    return '<th class="sort ' + (cls || '') + (on ? ' on' : '') + '" data-sort="' + key + '">' + esc(label) + (on ? '<span class="ind">' + (sort.dir === 'asc' ? '▲' : '▼') + '</span>' : '') + '</th>';
  }
  function sortBy(rows, keyFn, dir) { const sign = dir === 'asc' ? 1 : -1; return rows.slice().sort((a, b) => sign * compare(keyFn(a), keyFn(b))); }
  function toggleSort(s, key, defaultDir) { if (s.key === key) s.dir = s.dir === 'asc' ? 'desc' : 'asc'; else { s.key = key; s.dir = defaultDir || 'desc'; } }
  function seg(items, on, dataKey) {
    return '<div class="seg">' + items.map(i => '<button type="button" data-' + dataKey + '="' + attr(i[0]) + '" class="' + (on === i[0] ? 'on' : '') + '">' + esc(i[1]) + (i[2] != null ? '<span class="n">' + fmtNum(i[2]) + '</span>' : '') + '</button>').join('') + '</div>';
  }
  function kv(pairs) { return '<dl class="kv">' + pairs.filter(Boolean).map(p => '<dt>' + esc(p[0]) + '</dt><dd>' + p[1] + '</dd>').join('') + '</dl>'; }
  function feedRow(at, main) { return '<div class="feed-row"><span class="feed-t" title="' + attr(fmtTime(at)) + '">' + liveAgo(at) + '</span><span class="feed-m">' + main + '</span></div>'; }
  // Statuses refresh about twice per epoch, so a peer up to half an epoch behind is current.
  function headBucket(d) {
    const spe = state.spec.slotsPerEpoch;
    if (d > 1) return 'ahead';
    if (d >= -spe) return 'current';
    if (d >= -10 * spe) return 'behind';
    return 'far';
  }
  const HEAD_BUCKETS = [
    { key: 'ahead', label: 'ahead', cls: 'info', tip: 'More than one slot ahead of our head' },
    { key: 'current', label: 'within 1 epoch', cls: 'good', tip: 'Within one epoch of our head; statuses refresh every half epoch, so this is current' },
    { key: 'behind', label: '1–10 behind', cls: 'warn', tip: 'Between one and ten epochs behind' },
    { key: 'far', label: '10+ behind', cls: 'bad', tip: 'More than ten epochs behind' },
  ];
  function deltaCell(d) {
    if (d == null) return '<span class="faint">—</span>';
    const b = headBucket(d);
    const cls = b === 'ahead' ? 'pos' : b === 'far' ? 'far' : b === 'behind' ? 'neg' : '';
    return '<span class="' + cls + '">' + (d > 0 ? '+' : '') + d + '</span>';
  }
  function gossipCell(score) {
    const cls = score < state.cfg.gossipThreshold ? 'sbad' : score < 0 ? 'sneg' : '';
    return '<span class="' + cls + '" data-tip="' + attr('Gossip score; grey-listed below ' + fmtNum(state.cfg.gossipThreshold)) + '">' + fmtScore(score) + '</span>';
  }
  function greyCell(p) {
    if (!p.aspects.length) return '<span class="faint">—</span>';
    let html = '<span class="badges">' + p.aspects.map(a => aspectBadge(a, cleanVerdict(p.details[a]))).join('');
    if (!p.greyListed && p.exemption) html += badge('exempt: ' + p.exemption, 'accent sm', 'A refusal source fires but the peer is exempt');
    return html + '</span>';
  }
  function recoveryCell(p) {
    if (!p.greyListed) return '<span class="faint">—</span>';
    if (p.recoveryEnd != null) return liveCountdown(p.recoveryEnd);
    if (p.recoveryUnknown) return '<span class="muted" data-tip="Gossip-score and IP-colocation recovery are not time based">not timed</span>';
    return '<span class="faint">—</span>';
  }
  function statusCell(p) {
    if (!p.status) return '<span class="faint">never</span>';
    return liveAgo(p.statusAt) + (p.validationError ? ' ' + badge('invalid', 'bad sm', p.validationError) : '');
  }
  function lastStrikeCell(p) {
    const h = p.lastStrike;
    if (!h) return '<span class="faint">—</span>';
    return srcLabel(h.source) + ' <span class="reason" title="' + attr(h.reason) + '">' + esc(h.reason.length > 64 ? h.reason.slice(0, 62) + '…' : h.reason) + '</span> <span class="faint nowrap">' + liveAgo(h.at) + '</span>';
  }
  function emptyState(title, body, cta) {
    return '<div class="empty"><b>' + esc(title) + '</b>' + (body ? '<div>' + body + '</div>' : '') + (cta ? '<div class="cta">' + cta + '</div>' : '') + '</div>';
  }

  // Standard peer table shared by Peers, Clients and the peer lists elsewhere.
  const PEER_SORT = {
    rank: p => p.rank, peer: p => p.id, agent: p => p.agent || null, state: p => STATES.indexOf(p.state) * 2 + (p.direction === 'OUTBOUND' ? 1 : 0),
    tenure: p => p.tenureSec, strikes: p => p.standing, grey: p => (p.greyListed ? 100 : 0) + p.aspects.length,
    recovery: p => p.greyListed ? (p.recoveryMax == null ? Infinity : p.recoveryMax) : null, gossip: p => p.gossipScore,
    head: p => p.headDelta, rejections: p => p.rejectionsCount, status: p => p.statusAt || null,
  };
  function peerTable(rows, sort, limit, emptyMsg) {
    const shown = limit ? rows.slice(0, limit) : rows;
    // The recovery column only earns its space when a grey-listed peer is on screen.
    const rec = shown.some(p => p.greyListed);
    return '<div class="tbl-wrap"><table class="tbl" data-table="peers"><thead><tr>' + th('Peer', 'peer', sort) + th('State', 'state', sort) + th('Tenure', 'tenure', sort) + th('Strikes', 'strikes', sort) +
      th('Grey list', 'grey', sort) + (rec ? th('Recovers in', 'recovery', sort) : '') + th('Head Δ', 'head', sort, 'num') + th('Gossip', 'gossip', sort, 'num') + th('Rej.', 'rejections', sort, 'num') + th('Last status', 'status', sort) +
      '</tr></thead><tbody>' + (shown.map(p => peerRow(p, rec)).join('') || '<tr><td colspan="10">' + emptyState(emptyMsg || 'No peers match', 'Widen the scope or reset the filters.') + '</td></tr>') + '</tbody></table>' +
      (rows.length > shown.length ? '<div class="tbl-foot"><span>Showing ' + shown.length + ' of ' + fmtNum(rows.length) + '</span><button class="btn sm" type="button" data-more="1">Show all</button></div>' : '') + '</div>';
  }
  function peerRow(p, rec) {
    return '<tr class="row" data-nav data-peer="' + attr(p.id) + '">' +
      '<td>' + peerCell(p) + '</td>' +
      '<td><span class="badges">' + stateBadge(p.state) + dirBadge(p.direction) + '</span></td>' +
      '<td class="tight">' + (p.connected && p.connectedAt ? liveSince(p.connectedAt) : '<span class="faint">—</span>') + '</td>' +
      '<td>' + meter(p) + '</td>' +
      '<td>' + greyCell(p) + '</td>' +
      (rec ? '<td class="tight">' + recoveryCell(p) + '</td>' : '') +
      '<td class="num">' + deltaCell(p.headDelta) + '</td>' +
      '<td class="num">' + gossipCell(p.gossipScore) + '</td>' +
      '<td class="num">' + (p.rejectionsCount ? p.rejectionsCount : '<span class="faint">0</span>') + '</td>' +
      '<td class="tight">' + statusCell(p) + '</td></tr>';
  }

  // One tick per connected peer, grouped by client; a chart that is also a picker.
  function peerStrip(conn) {
    if (!conn.length) return '';
    const typeCounts = countBy(conn.map(p => p.agentType));
    const order = CLIENTS.filter(t => typeCounts.get(t)).sort((a, b) => typeCounts.get(b) - typeCounts.get(a));
    const sorted = conn.slice().sort((a, b) => order.indexOf(a.agentType) - order.indexOf(b.agentType) || (b.tenureSec || 0) - (a.tenureSec || 0));
    const ticks = sorted.map(p => '<span class="tick' + (p.greyListed ? ' grey' : '') + (p.nearThreshold ? ' near' : '') + (p.direction === 'OUTBOUND' ? ' out' : '') + '" tabindex="0" style="background:' + clientColor(p.agentType) + '" data-peer="' + attr(p.id) + '" data-tip="' +
      attr((p.agent || 'unknown agent') + '\n' + p.short + ' · ' + (p.direction === 'INBOUND' ? 'inbound' : p.direction === 'OUTBOUND' ? 'outbound' : 'direction unknown') + ' · ' + fmtDur(p.tenureSec) +
        '\nstrikes ' + p.standing + '/' + p.threshold + (p.greyListed ? ' · grey-listed (' + p.aspects.map(a => ASPECT_NAME[a]).join(', ') + ')' : p.nearThreshold ? ' · one strike from grey-listing' : '') +
        (p.headDelta != null ? '\nhead ' + (p.headDelta > 0 ? '+' : '') + p.headDelta + ' vs ours' : '')) + '"></span>').join('');
    const legend = order.map(t => '<span class="item"><span class="key-tick" style="background:' + clientColor(t) + '"></span>' + esc(t) + ' <span class="n">' + typeCounts.get(t) + '</span></span>').join('') +
      '<span class="item"><span class="key-tick grey"></span>grey-listed</span><span class="item"><span class="key-tick near"></span>one strike from the threshold</span><span class="item"><span class="key-tick out"></span>outbound (short tick)</span>';
    return '<div class="strip-wrap"><div class="strip">' + ticks + '</div><div class="strip-legend">' + legend + '</div></div>';
  }

  // ------------------------------------------------------------------ shell
  function render() {
    renderNav();
    renderLive();
    renderNodeBlock();
    const main = $('#main');
    const key = state.route.view + '/' + state.route.param;
    if (state.mounted !== key) {
      state.cursor = -1;
      main.innerHTML = '<div id="banner" hidden></div><div id="pageH"></div><div id="scopeBar" class="scope"></div><div id="view"></div>';
      VIEW_IMPL[state.route.view].mount($('#view'));
      state.mounted = key;
      window.scrollTo({ top: 0 });
    }
    renderBanner();
    renderHeader();
    renderScope();
    VIEW_IMPL[state.route.view].update($('#view'));
    applyCursor();
  }
  function renderNav() {
    $$('#nav a').forEach(a => a.classList.toggle('on', a.dataset.view === state.route.view || (state.route.view === 'peer' && a.dataset.view === 'peers')));
    $$('#nav a').forEach(a => { const v = a.dataset.view; a.setAttribute('href', hashFor(v, '')); });
    if (!state.fetchedAt) return;
    const s = scoped();
    const counts = { peers: s.connected.length, clients: new Set(s.peers.map(p => p.agentType)).size, greylist: s.grey.length, gossip: s.rejections.length, chain: s.connected.filter(p => p.validationError).length };
    $$('#nav .n').forEach(el => {
      const k = el.dataset.count, v = counts[k];
      el.textContent = v ? fmtNum(v) : '';
      el.classList.toggle('alert', (k === 'greylist' || k === 'chain') && v > 0);
    });
  }
  function renderBanner() {
    const b = $('#banner'); if (!b) return;
    if (state.error) { b.className = 'banner error'; b.innerHTML = '<b>Node unreachable.</b> ' + esc(state.error); b.hidden = false; }
    else if (state.warnings.length) { b.className = 'banner warn'; b.innerHTML = '<b>Partial data.</b> Some requests failed: ' + state.warnings.map(w => '<code>' + esc(w) + '</code>').join(', '); b.hidden = false; }
    else b.hidden = true;
  }
  function renderLive() {
    const el = $('#live'), txt = $('#liveText'), sub = $('#liveSub');
    let cls = 'live', text, s = '';
    if (state.loading) { cls += ' loading'; text = 'Refreshing…'; }
    else if (state.error) { cls += ' error'; text = 'Node unreachable'; s = state.auto && state.nextAt ? 'retry in ' + Math.max(0, Math.ceil((state.nextAt - Date.now()) / 1000)) + 's' : ''; }
    else if (!state.fetchedAt) { text = 'Not connected'; }
    else {
      cls += state.auto ? ' ok' : ' paused';
      text = 'Updated ' + ago(state.fetchedAt);
      s = state.auto && state.nextAt ? 'next in ' + Math.max(0, Math.ceil((state.nextAt - Date.now()) / 1000)) + 's' : 'auto-refresh off';
    }
    el.className = cls; txt.textContent = text; sub.textContent = s;
  }
  function renderNodeBlock() {
    const el = $('#nodeBlock');
    const ident = state.node.identity || {};
    el.innerHTML = '<div><b>' + esc(state.node.version || 'node') + '</b></div>' +
      (ident.peer_id ? '<div class="mono" title="' + attr(ident.peer_id) + '">' + esc(short(ident.peer_id)) + ' ' + copyBtn(ident.peer_id) + '</div>' : '') +
      '<div><span class="link" data-node-toggle="1" style="font-family:var(--sans);font-size:11.5px">' + esc(state.nodeUrl.replace(/^https?:\/\//, '')) + '</span></div>';
    $('#nodeForm').classList.toggle('open', state.nodeFormOpen);
  }
  function renderHeader() {
    const h = $('#pageH');
    const v = state.route.view;
    if (v === 'peer') { h.innerHTML = ''; return; }
    const s = state.fetchedAt ? scoped() : null;
    const ledes = {
      overview: s ? fmtNum(s.connected.length) + ' connected · ' + fmtNum(s.peers.length) + ' known' : '',
      peers: 'Every peer the node remembers, connected or not.',
      clients: 'Client types, their agents, and the peers behind each.',
      greylist: 'Who the node refuses, why, and who is close.',
      gossip: 'Messages our validators rejected, and the gossip scores that follow.',
      chain: 'What peers report about the chain, compared with our own view.',
    };
    h.innerHTML = '<div class="page-h"><h1>' + esc(VIEWS[v]) + '</h1><span class="lede">' + esc(ledes[v] || '') + '</span></div>';
  }
  function scopeCounts() {
    const byType = new Map();
    for (const p of state.peers) { const t = byType.get(p.agentType) || { conn: 0, known: 0 }; t.known++; if (p.connected) t.conn++; byType.set(p.agentType, t); }
    return byType;
  }
  function renderScope() {
    const bar = $('#scopeBar');
    if (state.route.view === 'peer') { bar.hidden = true; return; }
    bar.hidden = false;
    const byType = scopeCounts();
    const active = !!(scope.client || scope.agent);
    bar.className = 'scope' + (active ? ' active' : '');
    const s = state.fetchedAt ? scoped() : null;
    bar.innerHTML =
      '<span class="scope-l">Scope</span>' +
      '<label class="field" data-tip="Client type · connected / known"><span>Client</span><select class="select" data-scope="client"><option value="">All clients</option>' +
      CLIENTS.filter(t => byType.get(t) || t === scope.client).map(t => { const c = byType.get(t) || { conn: 0, known: 0 }; return '<option value="' + attr(t) + '"' + (scope.client === t ? ' selected' : '') + '>' + esc(t) + ' · ' + c.conn + ' / ' + c.known + '</option>'; }).join('') + '</select></label>' +
      '<div class="field"><span>Agent</span><div class="dd" id="agentDD"><button class="btn dd-btn" type="button" data-dd="toggle"><span class="trunc">' + esc(scope.agent || 'All agents') + '</span> ▾</button><div class="dd-menu" id="ddMenu" hidden></div></div></div>' +
      '<div class="field"><span>Events in</span>' + seg(WINDOWS.map(w => [w[0], w[0] === 'all' ? 'all' : w[0]]), scope.window, 'win') + '</div>' +
      (s ? '<span class="scope-note">' + (active ? fmtNum(s.connected.length) + ' connected · ' + fmtNum(s.peers.length) + ' known in scope' : 'Narrow every view to one client or agent') + '</span>' : '') +
      (active ? '<button class="btn ghost sm clear" type="button" data-scope-clear="1">Clear scope</button>' : '');
    if (state.dd.open) renderDD();
  }
  function renderDD() {
    const menu = $('#ddMenu'); if (!menu) return;
    menu.hidden = !state.dd.open;
    if (!state.dd.open) return;
    const q = state.dd.q.trim().toLowerCase();
    const items = state.agents.filter(a => (!scope.client || a.agentType === scope.client) && (!q || a.agent.toLowerCase().includes(q)))
      .map(a => ({ a, conn: a.peers.filter(p => p.connected).length, known: a.peers.length }))
      .sort((x, y) => y.conn - x.conn || y.known - x.known || x.a.agent.localeCompare(y.a.agent));
    state.dd.cursor = clamp(state.dd.cursor, 0, Math.max(0, items.length - 1));
    const list = items.map((it, i) => '<div class="dd-item' + (i === state.dd.cursor ? ' cursor' : '') + (scope.agent === it.a.agent ? ' on' : '') + '" data-dd-pick="' + attr(it.a.agent) + '">' + dot(it.a.agentType) + '<span class="trunc" title="' + attr(it.a.agent) + '">' + esc(it.a.agent) + '</span><span class="n">' + it.conn + ' / ' + it.known + '</span></div>').join('');
    const had = $('#ddSearch');
    menu.innerHTML = '<input class="input" id="ddSearch" placeholder="Filter agents… (' + fmtNum(items.length) + ')" value="' + attr(state.dd.q) + '" autocomplete="off">' +
      '<div class="dd-list">' + (scope.agent ? '<div class="dd-item" data-dd-pick="">' + '<span class="trunc muted">All agents</span></div>' : '') + (list || '<div class="dd-empty">No agent matches</div>') + '</div>';
    if (!had) { const inp = $('#ddSearch'); inp.focus(); inp.setSelectionRange(inp.value.length, inp.value.length); }
    else { const inp = $('#ddSearch'); const pos = state.dd.caret == null ? inp.value.length : state.dd.caret; inp.focus(); inp.setSelectionRange(pos, pos); }
    const c = $('.dd-item.cursor', menu); if (c) c.scrollIntoView({ block: 'nearest' });
  }
  function setScope(patch) {
    Object.assign(scope, patch);
    if (scope.agent) { const a = state.agents.find(x => x.agent === scope.agent); if (a && scope.client && a.agentType !== scope.client) scope.agent = ''; }
    memo.key = '';
    state.cursor = -1;
    syncHash();
    render();
  }
  function tickLive() {
    renderLive();
    $$('[data-live]').forEach(el => {
      const kind = el.dataset.live;
      if (kind === 'ago') el.textContent = ago(+el.dataset.ts);
      else if (kind === 'since') el.textContent = fmtDur((Date.now() - +el.dataset.ts) / 1000);
      else if (kind === 'countdown') { const left = (+el.dataset.end - Date.now()) / 1000; el.textContent = left > 0 ? fmtDur(left) : 'due now'; }
    });
  }
  const waiting = () => emptyState('Waiting for the node…', 'The first response has not arrived yet.');

  // ------------------------------------------------------------------ overview
  function updateOverview(el) {
    if (!state.fetchedAt) { el.innerHTML = waiting(); return; }
    const s = scoped(), cfg = state.cfg, node = state.node, spe = state.spec.slotsPerEpoch;
    const conn = s.connected, inbound = conn.filter(p => p.direction === 'INBOUND'), outbound = conn.filter(p => p.direction === 'OUTBOUND');
    const grey = s.grey, greyConn = grey.filter(p => p.connected);
    const aspectCounts = countBy(grey.flatMap(p => p.aspects));
    const near = s.peers.filter(p => p.nearThreshold);
    const strikes = s.strikes, bySource = countBy(strikes.map(e => e.source));
    const rej = s.rejections, rejPeers = new Set(rej.map(r => r.peerId)).size;
    const errPeers = conn.filter(p => p.validationError);
    const withChain = conn.filter(p => p.headDelta != null);
    const hb = countBy(withChain.map(p => headBucket(p.headDelta)));
    const atHead = hb.get('current') || 0, ahead = hb.get('ahead') || 0, behind = (hb.get('behind') || 0) + (hb.get('far') || 0);
    const typeCounts = countBy(conn.map(p => p.agentType));
    const knownCounts = countBy(s.peers.map(p => p.agentType));
    const greyByType = countBy(grey.map(p => p.agentType));
    const clientItems = CLIENTS.filter(t => knownCounts.get(t)).map(t => ({ label: t, key: t, value: typeCounts.get(t) || 0, color: clientColor(t), on: scope.client === t, known: knownCounts.get(t), grey: greyByType.get(t) || 0, tenure: medianTenure(conn.filter(p => p.agentType === t)) }))
      .sort((a, b) => b.value - a.value || b.known - a.known);
    const topReasons = topN(countBy(strikes.map(e => e.source + ' · ' + e.reason)), 8);
    const feed = strikes.map(e => ({ at: e.at, html: dot(e.peer.agentType) + peerLink(e.peer.id) + srcLabel(e.source) + '<span class="reason">' + esc(e.reason) + '</span>' }))
      .concat(rej.map(r => ({ at: r.at, html: dot(r.agentType) + peerLink(r.peerId) + badge('rejected', 'info sm') + badge(r.topicShort, 'sm', r.topic) + '<span class="reason">' + esc(r.reason) + '</span>' })))
      .concat(s.peers.filter(p => p.validationError && p.statusAt >= s.since).map(p => ({ at: p.statusAt, html: dot(p.agentType) + peerLink(p.id) + badge('status invalid', 'warn sm') + '<span class="reason">' + esc(p.validationError) + '</span>' })))
      .sort((a, b) => b.at - a.at).slice(0, 14);
    const ident = node.identity || {}, md = ident.metadata || {};
    const listen = ((ident.p2p_addresses || []).filter(a => !/\/ip4\/(127\.|10\.|172\.(1[6-9]|2\d|3[01])\.|192\.168\.)/.test(a))[0]) || (ident.p2p_addresses || [])[0] || '';

    el.innerHTML =
      '<div class="hero"><div>' +
      '<div class="hero-num">' + fmtNum(conn.length) + '<small>connected peer' + (conn.length === 1 ? '' : 's') + (scope.client || scope.agent ? ' in scope' : '') + '</small></div>' +
      '<div class="hero-sub"><span>' + fmtNum(inbound.length) + ' inbound · ' + fmtNum(outbound.length) + ' outbound</span><span>' + fmtNum(s.peers.length) + ' known</span>' +
      (node.peerCount && toInt(node.peerCount.connecting) ? '<span>' + fmtNum(toInt(node.peerCount.connecting)) + ' connecting</span>' : '') +
      '<span>median tenure ' + fmtDur(medianTenure(conn)) + '</span>' + (greyConn.length ? '<span class="badge bad">' + greyConn.length + ' grey-listed still connected</span>' : '') + '</div>' +
      peerStrip(conn) + '</div>' +
      '<div class="hero-side">' +
      '<div class="row"><span class="k">Sync</span><span class="v">' + (node.isSyncing ? badge('syncing', 'warn') : badge('in sync', 'ok')) + (node.isOptimistic ? ' ' + badge('optimistic', 'warn') : '') + (node.elOffline ? ' ' + badge('EL offline', 'bad') : '') + (node.syncDistance ? ' <span class="muted">distance ' + node.syncDistance + '</span>' : '') + '</span></div>' +
      '<div class="row"><span class="k">Head</span><span class="v">slot ' + fmtNum(node.headSlot) + ' <span class="muted">· epoch ' + fmtNum(node.headSlot != null ? Math.floor(node.headSlot / spe) : null) + '</span></span></div>' +
      '<div class="row"><span class="k">Finalized</span><span class="v">epoch ' + fmtNum(node.finalizedEpoch) + '</span></div>' +
      '<div class="row"><span class="k">Peers vs us</span><span class="v">' + atHead + ' current · ' + ahead + ' ahead · ' + behind + ' behind <span class="muted">of ' + withChain.length + ' with status</span></span></div>' +
      '<div class="row"><span class="k">Scorer</span><span class="v">' + fmtNum(cfg.trackedPeers) + ' tracked · grey-list at ' + cfg.threshold + ' strikes · 1 forgiven / ' + esc(cfg.decay || '?') + '</span></div>' +
      '</div></div>' +

      '<div class="tiles">' +
      tile({ label: 'Grey-listed', value: fmtNum(grey.length), cls: grey.length ? 'bad' : 'good', href: hashFor('greylist', ''), sub: ASPECTS.filter(a => aspectCounts.get(a[0])).map(a => aspectCounts.get(a[0]) + ' ' + a[1].toLowerCase()).concat(grey.length ? [] : ['no peer refused']) }) +
      tile({ label: 'Strikes, ' + winLabel(), value: fmtNum(strikes.length), cls: strikes.length ? 'warn' : '', href: hashFor('greylist', ''), sub: [topN(bySource, 1).map(([k, v]) => 'mostly ' + k + ' (' + v + ')')[0], plural(new Set(strikes.map(e => e.peer.id)).size, 'peer')] }) +
      tile({ label: 'One strike from grey-listing', value: fmtNum(near.length), cls: near.length ? 'warn' : '', href: hashFor('greylist', ''), sub: [near.filter(p => p.connected).length + ' connected'] }) +
      tile({ label: 'Gossip rejections, ' + winLabel(), value: fmtNum(rej.length), cls: rej.length ? 'info' : '', href: hashFor('gossip', ''), sub: [rej.length ? plural(rejPeers, 'peer') : 'nothing rejected', topN(countBy(rej.map(r => r.topicShort)), 1).map(([k, v]) => k + ' (' + v + ')')[0]] }) +
      tile({ label: 'Invalid status, connected', value: fmtNum(errPeers.length), cls: errPeers.length ? 'warn' : '', href: hashFor('chain', ''), sub: [topN(countBy(errPeers.map(p => p.validationError)), 1).map(([k]) => k)[0] || 'every status validated'] }) +
      '</div>' +

      '<div class="grid-2"><div class="colstack">' +
      sec('Connected peers by client', stack(clientItems, { data: 'client', head: 'client', valueHead: 'connected', cols: [{ h: 'known', get: i => fmtNum(i.known) }, { h: 'grey-listed', get: i => i.grey ? fmtNum(i.grey) : '<span class="faint">0</span>', cls: i => i.grey ? 'bad' : '' }, { h: 'median tenure', get: i => fmtDur(i.tenure) }] }), { sub: 'click a client to scope every view to it' }) +
      sec('Strikes by source, ' + winLabel(), bars(topN(bySource, 12).map(([k, v]) => ({ label: k, key: k, value: v, color: sourceColor(k) })), { data: 'source', total: strikes.length, empty: 'No strikes in this window' }) +
        '<div class="note">Strikes feed the standing count; ' + cfg.threshold + ' standing strikes grey-list a peer and one is forgiven every ' + esc(cfg.decay || '?') + '. The node retains the last ' + fmtNum(cfg.historySize) + ' per peer.</div>', { sub: 'click to list the peers' }) +
      '</div><div class="colstack">' +
      sec('Why peers are grey-listed', bars(ASPECTS.map(a => ({ label: a[1], key: a[0], value: aspectCounts.get(a[0]) || 0, color: { strikes: 'var(--bad)', peer_status: 'var(--warn)', gossip: 'var(--info)', bad_ip: 'var(--neutral-2)' }[a[0]] })), { data: 'aspect', max: Math.max(grey.length, 1), total: grey.length }) +
        '<div class="note">' + plural(grey.length, 'peer') + ' refused' + (greyConn.length ? ', ' + greyConn.length + ' still connected and due to be dropped' : '') + '. A peer can fire several aspects.</div>' +
        '<div class="sec-h" style="margin-top:14px"><h2>Top strike reasons, ' + esc(winLabel()) + '</h2></div>' +
        bars(topReasons.map(([k, v]) => ({ label: k, key: k, value: v, color: sourceColor(k.split(' · ')[0]), tip: k })), { data: 'reason', tall: true, total: strikes.length, empty: 'No strikes in this window' }), { sub: 'click to list the peers' }) +
      sec('Recent activity', feed.length ? '<div class="feed">' + feed.map(f => feedRow(f.at, f.html)).join('') + '</div>' : emptyState('Quiet', 'No strikes, rejections or invalid statuses in the ' + winLabel() + ' window.'), { sub: 'strikes, rejections and invalid statuses, newest first' }) +
      '</div></div>' +

      sec('This node', '<div class="grid-2" style="margin-bottom:0">' + kv([
        ['Version', esc(node.version || '–')],
        ['Peer ID', ident.peer_id ? '<span class="mono">' + esc(ident.peer_id) + '</span> ' + copyBtn(ident.peer_id) : '–'],
        ['Listening', listen ? '<span class="mono">' + esc(listen) + '</span>' : '–'],
        ['Metadata', 'seq ' + esc(md.seq_number || '–') + ' · attnets <span class="mono">' + esc(md.attnets || '–') + '</span>' + (md.syncnets ? ' · syncnets <span class="mono">' + esc(md.syncnets) + '</span>' : '') + (md.custody_group_count ? ' · custody groups ' + esc(md.custody_group_count) : '')],
      ]) + kv([
        ['Strikes', cfg.threshold + ' standing strikes grey-list · 1 forgiven every ' + esc(cfg.decay || '–') + ' · last ' + fmtNum(cfg.historySize) + ' retained'],
        ['Peer status', 'a failed status validation grey-lists for ' + esc(cfg.statusTTL || '–')],
        ['Gossip', 'grey-listed below score ' + fmtNum(cfg.gossipThreshold) + ' · last ' + fmtNum(cfg.maxRejections) + ' rejections kept per peer'],
        ['Tracked', fmtNum(cfg.trackedPeers) + ' peers in the scorer · ' + fmtNum(state.peers.length) + ' listed · ' + fmtNum(cfg.peersWithRejections) + ' with rejections'],
      ]) + '</div>');
  }

  // ------------------------------------------------------------------ peers
  const PRESETS = [['connected', 'Connected'], ['greylisted', 'Grey-listed'], ['striking', 'With strikes'], ['disconnected', 'Disconnected'], ['all', 'All']];
  function presetMatch(p, preset) {
    switch (preset) {
      case 'connected': return p.connected;
      case 'greylisted': return p.greyListed;
      case 'striking': return p.standing > 0;
      case 'disconnected': return p.state === 'DISCONNECTED';
      default: return true;
    }
  }
  function filteredPeers(f, pool) {
    const q = f.q.trim().toLowerCase();
    return pool.filter(p => presetMatch(p, f.preset) &&
      (!f.dir || p.direction === f.dir) && (!f.aspect || p.aspects.includes(f.aspect)) && (!f.source || p.history.some(h => h.source === f.source)) &&
      (!q || p.id.toLowerCase().includes(q) || p.agent.toLowerCase().includes(q) || p.addr.toLowerCase().includes(q) || p.agentType.includes(q)));
  }
  function mountPeers(el) {
    const f = state.filters.peers;
    el.innerHTML =
      '<div class="filters"><div id="peerPresets"></div>' +
      '<input class="input grow mono" data-f="filters.peers.q" data-search placeholder="Search peer id, agent, address…" value="' + attr(f.q) + '">' +
      '<span class="hint"><span><span class="kbd">↑</span> <span class="kbd">↓</span> move</span><span><span class="kbd">↵</span> open</span><span><span class="kbd">/</span> search</span></span></div>' +
      '<div class="filters">' +
      '<label class="field">Direction<select class="select" data-f="filters.peers.dir"><option value="">any</option><option value="INBOUND">inbound</option><option value="OUTBOUND">outbound</option></select></label>' +
      '<label class="field">Grey-list aspect<select class="select" data-f="filters.peers.aspect"><option value="">any</option>' + ASPECTS.map(a => '<option value="' + a[0] + '">' + a[1] + '</option>').join('') + '</select></label>' +
      '<label class="field">Strike source<select class="select" data-f="filters.peers.source"><option value="">any</option>' + SOURCES.map(s => '<option value="' + s + '">' + s + '</option>').join('') + '</select></label>' +
      '<button class="btn ghost sm" type="button" data-reset="peers">Reset</button></div>' +
      '<div class="summary" id="peerSummary"></div><div id="peerTable"></div>';
  }
  function updatePeers(el) {
    if (!state.fetchedAt) { $('#peerTable', el).innerHTML = waiting(); return; }
    const f = state.filters.peers, s = scoped(), sort = state.sort.peers;
    syncControls(el, 'filters.peers');
    $('#peerPresets', el).innerHTML = seg(PRESETS.map(p => [p[0], p[1], s.peers.filter(x => presetMatch(x, p[0])).length]), f.preset, 'preset');
    const rows = sortBy(filteredPeers(f, s.peers), PEER_SORT[sort.key] || PEER_SORT.rank, sort.dir);
    const conn = rows.filter(p => p.connected);
    $('#peerSummary', el).innerHTML = '<span><b>' + fmtNum(rows.length) + '</b> of ' + fmtNum(s.peers.length) + ' peers</span>' +
      (rows.length ? '<span><b>' + rows.filter(p => p.greyListed).length + '</b> grey-listed</span><span><b>' + rows.filter(p => p.standing > 0).length + '</b> with standing strikes</span>' : '') +
      (conn.length ? '<span>median tenure <b>' + fmtDur(medianTenure(conn)) + '</b></span>' : '');
    $('#peerTable', el).innerHTML = peerTable(rows, sort, f.peerLimit);
  }
  function syncControls(el, prefix) {
    $$('[data-f^="' + prefix + '"]', el).forEach(c => {
      const v = getPath(state, c.dataset.f);
      if (c.type === 'checkbox') c.checked = !!v;
      else if (document.activeElement !== c && c.value !== String(v == null ? '' : v)) c.value = v == null ? '' : v;
    });
  }

  // ------------------------------------------------------------------ clients
  const AGENT_SORT = { agent: a => a.agent, agentType: a => a.agentType, connected: a => a.st.connected, known: a => a.st.known, grey: a => a.st.grey, strikes: a => a.st.strikes, rejections: a => a.st.rejections, tenure: a => a.st.tenure };
  function mountClients(el) { el.innerHTML = '<div id="clientTiles" class="tiles"></div><div id="clientBody"></div>'; }
  function updateClients(el) {
    if (!state.fetchedAt) { $('#clientBody', el).innerHTML = waiting(); return; }
    const f = state.filters.clients, s = scoped();
    // Type tiles use the agent-free slice so one client can be compared with the others.
    const pool = scope.agent ? state.peers : s.peers;
    const since = s.since;
    const types = CLIENTS.filter(t => pool.some(p => p.agentType === t) || scope.client === t).map(t => {
      const ps = state.peers.filter(p => p.agentType === t), conn = ps.filter(p => p.connected);
      const ids = new Set(ps.map(p => p.id));
      return { t, conn: conn.length, known: ps.length, grey: ps.filter(p => p.greyListed).length, strikes: state.strikeEvents.filter(e => ids.has(e.peer.id) && e.at >= since).length,
        rej: state.rejections.filter(r => r.agentType === t && r.at >= since).length, tenure: medianTenure(conn), agents: new Set(ps.map(p => p.agent || 'unknown')).size };
    }).sort((a, b) => b.conn - a.conn || b.known - a.known);
    $('#clientTiles', el).innerHTML = types.map(x => tile({ label: dot(x.t) + esc(x.t), value: fmtNum(x.conn), unit: 'connected', pick: true, on: scope.client === x.t, data: 'data-client="' + attr(x.t) + '"',
      sub: [x.known + ' known', plural(x.agents, 'agent'), x.grey ? '<span style="color:var(--bad)">' + x.grey + ' grey-listed</span>' : '', x.strikes ? x.strikes + ' strikes' : '', x.rej ? x.rej + ' rejections' : '', 'tenure ' + fmtDur(x.tenure)] })).join('') ||
      emptyState('No peers yet');

    if (scope.agent) {
      const a = state.agents.find(x => x.agent === scope.agent) || { agent: scope.agent, agentType: agentTypeOf(scope.agent), peers: [] };
      const st = agentStats(a);
      const rows = sortBy(a.peers.filter(p => presetMatch(p, f.preset)), PEER_SORT[state.sort.peers.key] || PEER_SORT.rank, state.sort.peers.dir);
      const topReasons = topN(countBy(s.strikes.filter(e => (e.peer.agent || 'unknown') === a.agent).map(e => e.source + ' · ' + e.reason)), 6);
      const rejTopics = topN(countBy(s.rejections.filter(r => (r.agent || 'unknown') === a.agent).map(r => r.topicShort + ' · ' + r.reason)), 6);
      $('#clientBody', el).innerHTML =
        '<div class="panel" style="margin-bottom:18px"><div class="page-h" style="margin-bottom:8px"><h1 style="font-size:16px;display:flex;align-items:center;gap:8px">' + dot(a.agentType) + '<span class="mono" style="font-size:14px">' + esc(a.agent) + '</span></h1>' + badge(a.agentType, 'outline') +
        '<span class="right"><button class="btn ghost sm" type="button" data-scope-agent="">Back to all agents</button></span></div>' +
        '<div class="tiles" style="margin-bottom:0">' +
        tile({ label: 'Connected', value: fmtNum(st.connected), unit: 'of ' + st.known + ' known' }) +
        tile({ label: 'Grey-listed', value: fmtNum(st.grey), cls: st.grey ? 'bad' : '' }) +
        tile({ label: 'Strikes, ' + winLabel(), value: fmtNum(st.strikes), cls: st.strikes ? 'warn' : '', sub: [miniStack(Object.fromEntries(st.bySource), st.strikes)] }) +
        tile({ label: 'Rejections, ' + winLabel(), value: fmtNum(st.rejections), cls: st.rejections ? 'info' : '' }) +
        tile({ label: 'Median tenure', value: fmtDur(st.tenure) }) +
        '</div>' +
        (topReasons.length || rejTopics.length ? '<div class="grid-2" style="margin:14px 0 0">' +
          (topReasons.length ? sec('Strike reasons', bars(topReasons.map(([k, v]) => ({ label: k, value: v, color: sourceColor(k.split(' · ')[0]), tip: k })), { tall: true, total: st.strikes })) : '') +
          (rejTopics.length ? sec('Rejected gossip', bars(rejTopics.map(([k, v]) => ({ label: k, value: v, color: 'var(--info)', tip: k })), { tall: true, total: st.rejections })) : '') + '</div>' : '') +
        '</div>' +
        '<div class="filters">' + seg([['connected', 'Connected', a.peers.filter(p => p.connected).length], ['greylisted', 'Grey-listed', a.peers.filter(p => p.greyListed).length], ['all', 'All', a.peers.length]], f.preset, 'cpreset') +
        '<span class="hint"><span><span class="kbd">↑</span> <span class="kbd">↓</span> move</span><span><span class="kbd">↵</span> open peer</span></span></div>' +
        peerTable(rows, state.sort.peers, 0, 'No peers of this agent match');
      return;
    }
    const q = f.q.trim().toLowerCase(), sort = state.sort.agents;
    let rows = s.agents.map(a => ({ a, st: agentStats(a) })).filter(r => !q || r.a.agent.toLowerCase().includes(q));
    rows = sortBy(rows.map(r => Object.assign({ agent: r.a.agent, agentType: r.a.agentType }, r)), AGENT_SORT[sort.key] || AGENT_SORT.connected, sort.dir);
    const maxStrikes = Math.max.apply(null, rows.map(r => r.st.strikes).concat([1]));
    $('#clientBody', el).innerHTML =
      '<div class="filters"><input class="input grow" data-f="filters.clients.q" data-search placeholder="Search agent strings…" value="' + attr(f.q) + '">' +
      '<span class="summary" style="margin:0"><b>' + fmtNum(rows.length) + '</b> agents' + (scope.client ? ' running ' + esc(scope.client) : '') + '</span>' +
      '<span class="hint"><span><span class="kbd">↑</span> <span class="kbd">↓</span> move</span><span><span class="kbd">↵</span> open agent</span></span></div>' +
      '<div class="tbl-wrap"><table class="tbl" data-table="agents"><thead><tr>' + th('Agent', 'agent', sort) + th('Client', 'agentType', sort) + th('Connected', 'connected', sort, 'num') + th('Known', 'known', sort, 'num') + th('Grey-listed', 'grey', sort, 'num') +
      th('Strikes, ' + winLabel(), 'strikes', sort) + th('Rejections', 'rejections', sort, 'num') + th('Median tenure', 'tenure', sort, 'num') + '</tr></thead><tbody>' +
      (rows.map(r => '<tr class="row" data-nav data-agent="' + attr(r.agent) + '"><td class="agent" title="' + attr(r.agent) + '"><span class="mono" style="color:var(--ink)">' + esc(r.agent) + '</span></td><td>' + clientLabel(r.agentType) + '</td>' +
        '<td class="num">' + (r.st.connected || '<span class="faint">0</span>') + '</td><td class="num">' + r.st.known + '</td><td class="num">' + (r.st.grey ? '<span class="sbad">' + r.st.grey + '</span>' : '<span class="faint">0</span>') + '</td>' +
        '<td><span class="peer-cell">' + miniStack(Object.fromEntries(r.st.bySource), r.st.strikes, maxStrikes) + '<span class="num muted">' + (r.st.strikes || '') + '</span></span></td>' +
        '<td class="num">' + (r.st.rejections || '<span class="faint">0</span>') + '</td><td class="num">' + fmtDur(r.st.tenure) + '</td></tr>').join('') || '<tr><td colspan="8">' + emptyState('No agents match') + '</td></tr>') + '</tbody></table></div>';
  }

  // ------------------------------------------------------------------ grey list
  const GREY_SORT = { peer: p => p.id, agent: p => p.agent || null, state: p => STATES.indexOf(p.state), aspect: p => p.aspects.join(','), reason: p => cleanVerdict(p.details[p.aspects[0]] || ''), recovery: p => p.recoveryMax == null ? Infinity : p.recoveryMax, strikes: p => p.standing };
  const NEAR_SORT = { peer: p => p.id, agent: p => p.agent || null, state: p => STATES.indexOf(p.state), strikes: p => p.standing, last: p => p.lastStrike ? p.lastStrike.at : 0 };
  function mountGreylist(el) { el.innerHTML = '<div id="greyTiles" class="tiles"></div><div id="greyBody"></div>'; }
  function updateGreylist(el) {
    if (!state.fetchedAt) { $('#greyBody', el).innerHTML = waiting(); return; }
    const f = state.filters.greylist, s = scoped(), cfg = state.cfg;
    const grey = s.grey, aspectCounts = countBy(grey.flatMap(p => p.aspects));
    const exempt = s.peers.filter(p => p.exemption);
    const near = s.peers.filter(p => p.nearThreshold);
    const reasonKey = e => e.source + ' · ' + e.reason;
    const reasons = topN(countBy(s.strikes.map(reasonKey)), 14);
    const bySource = countBy(s.strikes.map(e => e.source));
    const reasonPeers = f.reason ? new Set(s.strikes.filter(e => reasonKey(e) === f.reason).map(e => e.peer.id)) : null;
    const matchReason = p => !reasonPeers || reasonPeers.has(p.id);
    const aspectLabel = { strikes: 'Refused for strikes', peer_status: 'Refused for peer status', gossip: 'Refused for gossip score', bad_ip: 'Refused for IP colocation' };
    const aspectRule = { strikes: cfg.threshold + ' standing strikes', peer_status: 'invalid status, ' + esc(cfg.statusTTL || '?') + ' TTL', gossip: 'score below ' + fmtNum(cfg.gossipThreshold), bad_ip: 'too many peers from one IP' };
    const aspectTone = { strikes: 'bad', peer_status: 'warn', gossip: 'info', bad_ip: '' };

    $('#greyTiles', el).innerHTML =
      tile({ label: 'Grey-listed now', value: fmtNum(grey.length), cls: grey.length ? 'bad' : 'good', pick: true, on: !f.aspect, data: 'data-gaspect=""', sub: [grey.filter(p => p.connected).length + ' still connected', exempt.length ? exempt.length + ' exempt (trusted)' : 'every aspect'] }) +
      ASPECTS.map(a => tile({ label: aspectLabel[a[0]], value: fmtNum(aspectCounts.get(a[0]) || 0), pick: true, on: f.aspect === a[0], data: 'data-gaspect="' + a[0] + '"', cls: aspectCounts.get(a[0]) ? aspectTone[a[0]] : '', sub: [aspectRule[a[0]]] })).join('') +
      tile({ label: 'One strike from grey-listing', value: fmtNum(near.length), cls: near.length ? 'warn' : '', sub: [near.filter(p => p.connected).length + ' connected', 'standing at ' + (cfg.threshold - 1) + ' of ' + cfg.threshold] });

    let rows = grey.filter(p => (!f.aspect || p.aspects.includes(f.aspect)) && matchReason(p));
    const gs = state.sort.grey;
    rows = sortBy(rows, GREY_SORT[gs.key] || GREY_SORT.recovery, gs.dir);
    const ns = state.sort.near;
    const nearRows = sortBy(near.filter(matchReason), NEAR_SORT[ns.key] || NEAR_SORT.strikes, ns.dir);
    const reasonBanner = f.reason ? '<div class="summary"><span>Peers struck for <b class="mono">' + esc(f.reason) + '</b> in the ' + esc(winLabel()) + ' window</span><button class="btn ghost sm" type="button" data-greason="' + attr(f.reason) + '">Clear</button></div>' : '';

    $('#greyBody', el).innerHTML =
      sec('Grey-listed peers', reasonBanner +
        '<div class="tbl-wrap"><table class="tbl" data-table="grey"><thead><tr>' + th('Peer', 'peer', gs) + th('State', 'state', gs) + th('Aspect', 'aspect', gs) + th('Why', 'reason', gs) + th('Recovers in', 'recovery', gs) + th('Strikes', 'strikes', gs) + '</tr></thead><tbody>' +
        (rows.map(p => '<tr class="row" data-nav data-peer="' + attr(p.id) + '"><td>' + peerCell(p) + '</td><td><span class="badges">' + stateBadge(p.state) + dirBadge(p.direction) + '</span></td>' +
          '<td><span class="badges">' + p.aspects.map(a => aspectBadge(a)).join('') + '</span></td>' +
          '<td class="reason">' + p.aspects.map(a => esc(cleanVerdict(p.details[a]))).join('<br>') + '</td>' +
          '<td class="tight">' + recoveryCell(p) + '</td><td>' + meter(p) + '</td></tr>').join('') ||
          '<tr><td colspan="6">' + emptyState(grey.length ? 'No grey-listed peer matches' : 'No peer is grey-listed', grey.length ? 'Pick another aspect or clear the reason filter.' : 'Nothing in scope is refused right now.') + '</td></tr>') +
        '</tbody></table></div>', { sub: fmtNum(rows.length) + ' of ' + fmtNum(grey.length) + ' · the reason is the node\'s own verdict text' }) +
      '<div class="grid-2">' +
      sec('Strike reasons, ' + winLabel(), bars(reasons.map(([k, v]) => ({ label: k, key: k, value: v, color: sourceColor(k.split(' · ')[0]), tip: k, on: f.reason === k })), { data: 'greason', tall: true, total: s.strikes.length, empty: 'No strikes in this window' }) +
        (bySource.size ? '<div class="sec-h" style="margin-top:16px"><h2>By source</h2></div>' + bars(topN(bySource, 12).map(([k, v]) => ({ label: k, value: v, color: sourceColor(k) })), { total: s.strikes.length }) : '') +
        '<div class="note">Source is where the strike came from (dial, an RPC handler, gossip validation, sync…); the reason is that handler\'s own label. Click a reason to keep only the peers it hit.</div>', { sub: fmtNum(s.strikes.length) + ' strikes on ' + plural(new Set(s.strikes.map(e => e.peer.id)).size, 'peer') }) +
      sec('One strike from grey-listing', '<div class="tbl-wrap"><table class="tbl compact" data-table="near"><thead><tr>' + th('Peer', 'peer', ns) + th('Strikes', 'strikes', ns) + th('Last strike', 'last', ns) + '</tr></thead><tbody>' +
        (nearRows.map(p => '<tr class="row" data-nav data-peer="' + attr(p.id) + '"><td><div class="peer-cell">' + peerCell(p) + '</div><div class="badges" style="margin-top:3px">' + stateBadge(p.state) + dirBadge(p.direction) + '</div></td><td>' + meter(p) + '</td><td class="reason" style="min-width:160px">' + lastStrikeCell(p) + '</td></tr>').join('') ||
          '<tr><td colspan="3">' + emptyState('Nobody is close', 'No peer in scope is within one strike of the threshold.') + '</td></tr>') + '</tbody></table></div>' +
        '<div class="note">The next strike before a decay tick refuses these peers; a connected one is dropped at the following status round.</div>', { sub: fmtNum(nearRows.length) + ' peers' }) +
      '</div>' +
      sec('Scoring rules', '<div class="grid-2" style="margin-bottom:0">' + kv([
        ['Strikes', cfg.threshold + ' standing strikes grey-list a peer; one is forgiven every ' + esc(cfg.decay || '–') + '. Recovery is the decays still needed.'],
        ['Peer status', 'A status exchange that fails validation grey-lists the peer until the verdict expires after ' + esc(cfg.statusTTL || '–') + '.'],
      ]) + kv([
        ['Gossip score', 'libp2p\'s score grey-lists below ' + fmtNum(cfg.gossipThreshold) + '; it recovers as the score decays, so no time estimate is given.'],
        ['IP colocation', 'More than the allowed peers from one IP refuses the newcomers; clears when the others leave.'],
      ]) + '</div><div class="note">Trusted peers are exempt from every verdict, and grey-listed peers are never forgotten by the peer store.</div>');
  }

  // ------------------------------------------------------------------ gossip
  const GROUPS = [['topic', 'Topic'], ['reason', 'Reason'], ['agent_type', 'Client'], ['agent', 'Agent'], ['peer', 'Peer']];
  function mountGossip(el) { el.innerHTML = '<div id="gossipTiles" class="tiles"></div><div id="gossipBody"></div>'; }
  function updateGossip(el) {
    if (!state.fetchedAt) { $('#gossipBody', el).innerHTML = waiting(); return; }
    const f = state.filters.gossip, s = scoped(), cfg = state.cfg;
    const all = s.rejections, q = f.q.trim().toLowerCase();
    const keyOf = r => f.groupBy === 'topic' ? r.topic : f.groupBy === 'reason' ? r.reason : f.groupBy === 'agent' ? (r.agent || 'unknown') : f.groupBy === 'agent_type' ? r.agentType : r.peerId;
    const labelOf = k => f.groupBy === 'topic' ? shortTopic(k) : f.groupBy === 'peer' ? short(k) : (k.length > 70 ? k.slice(0, 68) + '…' : k);
    const rows = all.filter(r => (!f.pick || keyOf(r) === f.pick) && (!q || r.topic.toLowerCase().includes(q) || r.reason.toLowerCase().includes(q) || r.agent.toLowerCase().includes(q) || r.peerId.toLowerCase().includes(q)));
    const peersAll = new Set(all.map(r => r.peerId)).size;
    const topTopic = topN(countBy(all.map(r => r.topicShort)), 1)[0], topReason = topN(countBy(all.map(r => r.reason)), 1)[0];
    $('#gossipTiles', el).innerHTML =
      tile({ label: 'Rejections, ' + winLabel(), value: fmtNum(all.length), cls: all.length ? 'info' : '', sub: [plural(peersAll, 'peer'), 'last ' + fmtNum(cfg.maxRejections) + ' kept per peer'] }) +
      tile({ label: 'Latest rejection', value: all.length ? liveAgo(all[0].at) : '<span class="faint">none</span>', sub: [all.length ? fmtTime(all[0].at) : ''] }) +
      tile({ label: 'Top topic', value: topTopic ? '<span style="font-size:16px">' + esc(topTopic[0]) + '</span>' : '<span class="faint">—</span>', sub: [topTopic ? plural(topTopic[1], 'rejection') : ''] }) +
      tile({ label: 'Top reason', value: topReason ? '<span style="font-size:14px;font-family:var(--mono)">' + esc(topReason[0].length > 60 ? topReason[0].slice(0, 58) + '…' : topReason[0]) + '</span>' : '<span class="faint">—</span>', sub: [topReason ? plural(topReason[1], 'rejection') : ''] });

    const groups = topN(countBy(rows.map(keyOf)), 15);
    const conn = s.connected;
    const scoreBuckets = [
      { label: 'grey-listed', test: v => v < cfg.gossipThreshold, cls: 'bad' }, { label: 'below −1,000', test: v => v >= cfg.gossipThreshold && v < -1000, cls: 'warn' },
      { label: '−1,000 … <0', test: v => v >= -1000 && v < 0, cls: 'warn' }, { label: '0', test: v => v === 0, cls: '' }, { label: '0 … 10', test: v => v > 0 && v < 10, cls: 'good' }, { label: '10+', test: v => v >= 10, cls: 'good' },
    ].map(b => ({ label: b.label, value: conn.filter(p => b.test(p.gossipScore)).length, cls: b.cls }));
    const lowest = conn.filter(p => p.gossipScore < 0 || p.behaviourPenalty > 0).sort((a, b) => a.gossipScore - b.gossipScore || b.behaviourPenalty - a.behaviourPenalty).slice(0, 8);
    const shown = rows.slice(0, 400);
    $('#gossipBody', el).innerHTML =
      '<div class="filters"><label class="field">Group by ' + seg(GROUPS, f.groupBy, 'group') + '</label>' +
      '<input class="input grow" data-f="filters.gossip.q" data-search placeholder="Search topic, reason, agent, peer…" value="' + attr(f.q) + '">' +
      (f.pick ? '<span class="summary" style="margin:0"><span>only <b class="mono">' + esc(labelOf(f.pick)) + '</b></span><button class="btn ghost sm" type="button" data-gpick="">Clear</button></span>' : '') +
      '<span class="summary" style="margin:0 0 0 auto"><b>' + fmtNum(rows.length) + '</b> of ' + fmtNum(all.length) + '</span></div>' +
      '<div class="grid-2">' +
      sec('Rejections by ' + GROUPS.find(g => g[0] === f.groupBy)[1].toLowerCase(), bars(groups.map(([k, v]) => ({ label: labelOf(k), key: k, value: v, tip: k, color: f.groupBy === 'agent_type' ? clientColor(k) : 'var(--info)', on: f.pick === k })), { data: 'gpick', wide: true, total: rows.length, empty: all.length ? 'No rejection matches' : 'No rejections in this window' }) +
        '<div class="note">A rejection is a gossip message a topic validator returned <b>reject</b> for. Rejections are kept for observability only; libp2p separately lowers the sender\'s gossip score for each invalid delivery, and that score grey-lists below ' + fmtNum(cfg.gossipThreshold) + '.</div>', { sub: 'click to filter the feed' }) +
      sec('Gossip score of connected peers', cols(scoreBuckets) +
        (lowest.length ? '<div class="sec-h" style="margin-top:16px"><h2>Lowest scores</h2></div><div class="tbl-wrap"><table class="tbl"><thead><tr><th>Peer</th><th class="num">Score</th><th class="num">Behaviour penalty</th></tr></thead><tbody>' +
          lowest.map(p => '<tr class="row" data-peer="' + attr(p.id) + '"><td>' + peerCell(p) + '</td><td class="num">' + gossipCell(p.gossipScore) + '</td><td class="num">' + fmtScore(p.behaviourPenalty) + '</td></tr>').join('') + '</tbody></table></div>' :
          '<div class="note">No connected peer has a negative score or a behaviour penalty.</div>'), { sub: 'mirrored from libp2p; grey-listed below ' + fmtNum(cfg.gossipThreshold) }) +
      '</div>' +
      sec('Rejection feed', '<div class="tbl-wrap"><table class="tbl"><thead><tr><th>When</th><th>Peer</th><th>Client</th><th>Topic</th><th>Reason</th></tr></thead><tbody>' +
        (shown.map(r => '<tr class="row" data-nav data-peer="' + attr(r.peerId) + '"><td class="tight" title="' + attr(fmtTime(r.at)) + '">' + liveAgo(r.at) + '</td><td>' + peerLink(r.peerId) + '</td><td><span class="peer-cell">' + dot(r.agentType) + '<span class="agent" title="' + attr(r.agent) + '">' + esc(r.agent || r.agentType) + '</span></span></td>' +
          '<td>' + badge(r.topicShort, 'sm', r.topic) + '</td><td class="reason" style="white-space:normal" title="' + attr(r.reason) + '">' + esc(r.reason) + '</td></tr>').join('') ||
          '<tr><td colspan="5">' + emptyState(all.length ? 'No rejection matches' : 'Nothing rejected', all.length ? 'Clear the search or the pick.' : 'No validator rejected a message from a peer in scope during the ' + winLabel() + ' window.') + '</td></tr>') + '</tbody></table>' +
        (rows.length > shown.length ? '<div class="tbl-foot">Showing ' + shown.length + ' of ' + fmtNum(rows.length) + '</div>' : '') + '</div>', { sub: 'newest first' });
  }

  // ------------------------------------------------------------------ chain
  const CHAIN_SORT = { peer: p => p.id, agent: p => p.agent || null, state: p => STATES.indexOf(p.state), headSlot: p => p.headSlot, headDelta: p => p.headDelta, finalizedEpoch: p => p.finalizedEpoch, earliestSlot: p => p.earliestSlot, statusAt: p => p.statusAt || null, error: p => p.validationError || null };
  function mountChain(el) { el.innerHTML = '<div id="chainTiles" class="tiles"></div><div id="chainCards" class="grid-3"></div><div id="chainBody"></div>'; }
  function updateChain(el) {
    if (!state.fetchedAt) { $('#chainBody', el).innerHTML = waiting(); return; }
    const f = state.filters.chain, s = scoped(), node = state.node, cfg = state.cfg, spe = state.spec.slotsPerEpoch, sort = state.sort.chain;
    const conn = s.connected, withChain = conn.filter(p => p.chain);
    const deltas = withChain.map(p => p.headDelta).filter(d => d != null);
    const hb = countBy(deltas.map(headBucket));
    const buckets = HEAD_BUCKETS.map(b => ({ label: b.label, value: hb.get(b.key) || 0, cls: b.cls, tip: b.tip }));
    const behindCount = buckets[2].value + buckets[3].value;
    const finCounts = topN(countBy(withChain.map(p => p.finalizedEpoch).filter(e => e != null)), 6);
    const digests = topN(countBy(s.peers.filter(p => p.forkDigest).map(p => p.forkDigest)), 4);
    const errPeers = s.peers.filter(p => p.validationError).sort((a, b) => b.statusAt - a.statusAt);
    const earliest = withChain.map(p => p.earliestSlot).filter(e => e != null).sort((a, b) => a - b);
    const highestPeer = withChain.reduce((m, p) => p.headSlot != null && p.headSlot > m ? p.headSlot : m, 0);
    $('#chainTiles', el).innerHTML =
      tile({ label: 'Our head', value: fmtNum(node.headSlot), cls: node.isSyncing ? 'warn' : 'good', sub: ['epoch ' + fmtNum(node.headSlot != null ? Math.floor(node.headSlot / spe) : null), node.isSyncing ? badge('syncing', 'warn') : badge('in sync', 'ok'), node.isOptimistic ? badge('optimistic', 'warn') : '', node.elOffline ? badge('EL offline', 'bad') : ''] }) +
      tile({ label: 'Our finalized epoch', value: fmtNum(node.finalizedEpoch), sub: [node.finalizedRoot ? '<span class="mono">' + esc(node.finalizedRoot.slice(0, 14)) + '…</span>' : ''] }) +
      tile({ label: "Peers' highest head", value: fmtNum(highestPeer || cfg.highestKnownHeadSlot), cls: highestPeer > (node.headSlot || 0) + 2 ? 'warn' : '', sub: [node.headSlot != null && highestPeer ? (highestPeer - node.headSlot > 0 ? '+' : '') + (highestPeer - node.headSlot) + ' slots vs ours' : '', 'scorer high-water ' + fmtNum(cfg.highestKnownHeadSlot)] }) +
      tile({ label: 'Peers with a chain view', value: fmtNum(withChain.length), unit: 'of ' + conn.length + ' connected', sub: [buckets[1].value + ' current', behindCount + ' behind'] }) +
      tile({ label: 'Invalid statuses', value: fmtNum(errPeers.length), cls: errPeers.length ? 'warn' : '', sub: [errPeers.filter(p => p.connected).length + ' connected', 'verdicts expire after ' + esc(cfg.statusTTL || '–')] });
    $('#chainCards', el).innerHTML =
      sec('Peer heads relative to ours', cols(buckets) + '<div class="note">Δ = peer head slot − our head slot, from the peer\'s last validated status (refreshed about twice per epoch).</div>', { sub: withChain.length + ' peers' }) +
      sec('Finalized epoch agreement', bars(finCounts.map(([e, n]) => ({ label: 'epoch ' + fmtNum(e) + (e === node.finalizedEpoch ? ' (ours)' : ''), value: n, color: e === node.finalizedEpoch ? 'var(--good)' : e > (node.finalizedEpoch || 0) ? 'var(--info)' : 'var(--warn)' })), { total: withChain.length, empty: 'No peer statuses yet' }) +
        kv([
          ['Fork digest' + (digests.length > 1 ? 's' : ''), digests.length ? digests.map(([d, n]) => '<span class="mono">' + esc(d) + '</span> <span class="faint">' + n + '</span>').join(' · ') : '–'],
          ['Earliest slot', earliest.length ? 'back to ' + fmtNum(earliest[0]) + ' <span class="faint">(median ' + fmtNum(median(earliest)) + ')</span>' : '–'],
        ]).replace('<dl class="kv">', '<dl class="kv" style="margin-top:12px">')) +
      sec('Invalid statuses', errPeers.length ? '<div class="feed">' + errPeers.slice(0, 10).map(p => feedRow(p.statusAt, dot(p.agentType) + peerLink(p.id) + stateBadge(p.state) + '<span class="reason">' + esc(p.validationError) + '</span>')).join('') + '</div>' +
        (errPeers.length > 10 ? '<div class="note">' + (errPeers.length - 10) + ' more in the table below.</div>' : '') : emptyState('Every status validated', 'No peer in scope failed status validation.'), { sub: 'latest first' });
    const q = f.q.trim().toLowerCase();
    let rows = s.peers.filter(p => p.chain && (!f.connectedOnly || p.connected) && (!q || p.id.toLowerCase().includes(q) || p.agent.toLowerCase().includes(q)));
    rows = sortBy(rows, CHAIN_SORT[sort.key] || CHAIN_SORT.headSlot, sort.dir);
    $('#chainBody', el).innerHTML =
      '<div class="filters"><input class="input grow mono" data-f="filters.chain.q" data-search placeholder="Search peer id or agent…" value="' + attr(f.q) + '">' +
      '<label class="switch"><input type="checkbox" data-f="filters.chain.connectedOnly"' + (f.connectedOnly ? ' checked' : '') + '><span class="switch-track"></span>Connected only</label>' +
      '<span class="summary" style="margin:0 0 0 auto"><b>' + fmtNum(rows.length) + '</b> peers with a chain view</span></div>' +
      '<div class="tbl-wrap"><table class="tbl" data-table="chain"><thead><tr>' + th('Peer', 'peer', sort) + th('State', 'state', sort) + th('Head slot', 'headSlot', sort, 'num') + th('Δ', 'headDelta', sort, 'num') + th('Finalized', 'finalizedEpoch', sort, 'num') + th('Earliest slot', 'earliestSlot', sort, 'num') + th('Status', 'statusAt', sort) + th('Validation', 'error', sort) +
      '</tr></thead><tbody>' + (rows.slice(0, 500).map(p =>
        '<tr class="row" data-nav data-peer="' + attr(p.id) + '"><td>' + peerCell(p) + '</td><td><span class="badges">' + stateBadge(p.state) + dirBadge(p.direction) + '</span></td>' +
        '<td class="num">' + fmtNum(p.headSlot) + '</td><td class="num">' + deltaCell(p.headDelta) + '</td><td class="num">' + fmtNum(p.finalizedEpoch) + (p.finalizedEpoch != null && node.finalizedEpoch != null && p.finalizedEpoch !== node.finalizedEpoch ? ' <span class="faint">(' + (p.finalizedEpoch > node.finalizedEpoch ? '+' : '') + (p.finalizedEpoch - node.finalizedEpoch) + ')</span>' : '') + '</td>' +
        '<td class="num">' + fmtNum(p.earliestSlot) + '</td><td class="tight">' + liveAgo(p.statusAt) + '</td><td>' + (p.validationError ? badge('invalid', 'bad sm') + ' <span class="reason">' + esc(p.validationError) + '</span>' : badge('ok', 'ok sm')) + '</td></tr>').join('') ||
        '<tr><td colspan="8">' + emptyState('No peers match') + '</td></tr>') + '</tbody></table></div>';
  }

  // ------------------------------------------------------------------ peer page
  async function loadPeerDetail(id) {
    const d = state.detail;
    if (d.id !== id) { d.id = id; d.data = null; d.eth = null; d.error = null; }
    d.loading = true;
    const [res, eth] = await Promise.allSettled([api('/prysm/v1/node/peers/' + encodeURIComponent(id) + '/scoring?include_topic_scores=true'), api('/eth/v1/node/peers/' + encodeURIComponent(id))]);
    if (state.detail.id !== id) return;
    d.loading = false;
    if (res.status === 'fulfilled' && res.value && res.value.data) { d.data = enrichPeer(res.value.data, peerCtx()); d.error = null; }
    else d.error = res.reason ? res.reason.message : 'no data';
    if (eth.status === 'fulfilled' && eth.value) d.eth = eth.value.data || null;
    if (state.route.view === 'peer' && state.route.param === id) updatePeer($('#view'));
  }
  function mountPeer(el) {
    el.innerHTML = '';
    if (state.detail.id !== state.route.param) { state.detail = { id: state.route.param, data: null, eth: null, error: null, loading: false }; }
    if (state.fetchedAt) loadPeerDetail(state.route.param);
  }
  function updatePeer(el) {
    const id = state.route.param, d = state.detail;
    const p = (d.id === id && d.data) || state.peersById.get(id);
    const back = '<a class="crumb" href="#" data-back="1">← Back</a>';
    if (!p) {
      el.innerHTML = back + '<div class="peer-head"><div class="ident"><h1>Peer</h1><div class="pid">' + esc(id) + '</div></div></div>' +
        (d.loading || !state.fetchedAt ? '<div class="empty"><span class="spin"></span> Loading…</div>' : '<div class="verdict warn"><div class="vt">Not found</div><div class="vb">' + esc(d.error || 'The node has no record of this peer.') + '</div></div>');
      return;
    }
    const eth = d.eth || {}, cfg = state.cfg, node = state.node;
    const addr = eth.last_seen_p2p_address || p.addr, enr = eth.enr || p.enr;
    const rejections = p.rejections.slice().sort((a, b) => b.at - a.at);
    const topics = p.topicScores ? Object.entries(p.topicScores).map(([t, v]) => ({ topic: t, short: shortTopic(t), mesh: v.time_in_mesh_ms || 0, first: v.first_message_deliveries || 0, meshDel: v.mesh_message_deliveries || 0, invalid: v.invalid_message_deliveries || 0 }))
      .filter(t => t.mesh || t.first || t.meshDel || t.invalid).sort((a, b) => (b.invalid - a.invalid) || (b.mesh - a.mesh)) : [];
    const verdict = p.greyListed
      ? '<div class="verdict bad"><div class="vt">' + badge('grey-listed', 'bad') + 'The node refuses this peer</div><div class="vb">' +
        (p.recoveryEnd != null ? 'Expected to be white-listed in ' + liveCountdown(p.recoveryEnd) + (p.recoveryUnknown ? ', if the untimed aspects clear too' : '') + '.' : p.recoveryUnknown ? 'Recovery is not time based (gossip score decay or IP colocation).' : '') +
        (p.connected ? ' It is still connected and will be dropped at the next status round.' : '') + '</div></div>'
      : p.aspects.length && p.exemption
        ? '<div class="verdict warn"><div class="vt">' + badge('exempt: ' + p.exemption, 'accent') + 'A refusal fires but the peer is exempt</div></div>'
        : '<div class="verdict ok"><div class="vt">' + badge('clear', 'ok') + 'Not grey-listed</div><div class="vb">' + (p.nearThreshold ? 'One more strike before decay would grey-list it.' : p.standing ? p.standing + ' standing strikes; grey-listed at ' + p.threshold + '.' : 'No standing strikes.') + '</div></div>';
    const aspects = p.aspects.map(a => '<div class="aspect ' + a + '"><div class="at">' + aspectBadge(a) +
      (p.recovery[a] ? '<span class="muted" style="font-weight:500">' + (p.recoverySec[a] != null ? 'recovers in ' + liveCountdown(state.fetchedAt + p.recoverySec[a] * 1000) : '<span data-tip="Clears when the score decays back above the threshold or the colocated peers leave">recovery not timed</span>') + '</span>' : '') + '</div><div class="ab reason">' + esc(cleanVerdict(p.details[a])) + '</div></div>').join('');
    const history = p.history.slice().reverse();

    el.innerHTML = back +
      '<div class="peer-head"><div class="ident"><h1>' + dot(p.agentType) + '<span class="mono" style="font-size:16px">' + esc(p.agent || 'unknown agent') + '</span>' + badge(p.agentType, 'outline') + (p.trusted ? badge('trusted', 'accent') : '') + (p.greyListed ? badge('grey-listed', 'bad') : '') + '</h1>' +
      '<div class="pid"><span>' + esc(p.id) + '</span>' + copyBtn(p.id) + '</div>' +
      '<div class="facts">' + stateBadge(p.state) + dirBadge(p.direction) +
      (p.connected && p.connectedAt ? '<span>connected for ' + liveSince(p.connectedAt) + ' <span class="faint">since ' + esc(fmtTime(p.connectedAt)) + '</span></span>' : p.connectedAt ? '<span class="muted">last connected ' + esc(fmtTime(p.connectedAt)) + '</span>' : '<span class="faint">never connected</span>') +
      (addr ? '<span class="mono" title="' + attr(addr) + '">' + esc(addr) + '</span>' + copyBtn(addr) : '<span class="faint">address unknown</span>') +
      (enr ? '<span class="mono" title="' + attr(enr) + '">' + esc(enr.length > 44 ? enr.slice(0, 42) + '…' : enr) + '</span>' + copyBtn(enr) : '') +
      '</div></div>' +
      '<div class="actions">' + (p.agent ? '<a class="btn sm" href="' + attr(hashForScope({ client: p.agentType, agent: p.agent }, 'clients')) + '">Peers of this agent</a>' : '') + '<a class="btn sm" href="' + attr(hashForScope({ client: p.agentType, agent: '' }, 'clients')) + '">All ' + esc(p.agentType) + ' peers</a>' +
      (d.loading ? '<span class="spin" data-tip="Loading full details"></span>' : '') + '</div></div>' +
      (d.error && !d.data ? '<div class="banner warn">Detail request failed: ' + esc(d.error) + '. Showing the list entry.</div>' : '') +

      '<div class="grid-2">' +
      sec('Grey-list verdict', verdict + aspects + '<div style="margin:10px 0 4px">' + meter(p) + ' <span class="muted" style="font-size:12px;margin-left:6px">' + p.standing + ' standing · grey-listed at ' + p.threshold + ' · 1 forgiven every ' + esc(cfg.decay || '–') + '</span></div>') +
      sec('Chain view', (p.validationError ? '<div class="verdict bad"><div class="vt">' + badge('validation failed', 'bad') + '</div><div class="vb reason">' + esc(p.validationError) + '</div></div>' : '') +
        (p.chain ? kv([
          ['Head', 'slot ' + fmtNum(p.headSlot) + ' <span class="muted">(' + deltaCell(p.headDelta) + ' vs ours ' + fmtNum(node.headSlot) + ')</span>'],
          ['Head root', '<span class="mono">' + esc(p.chain.head_root) + '</span>'],
          ['Finalized', 'epoch ' + fmtNum(p.finalizedEpoch) + (node.finalizedEpoch != null && p.finalizedEpoch != null ? ' <span class="muted">(ours ' + fmtNum(node.finalizedEpoch) + ')</span>' : '')],
          ['Finalized root', '<span class="mono">' + esc(p.chain.finalized_root) + '</span>'],
          ['Earliest slot', fmtNum(p.earliestSlot)],
          ['Fork digest', '<span class="mono">' + esc(p.chain.fork_digest) + '</span>'],
          ['Last status', p.status ? liveAgo(p.statusAt) + ' <span class="faint">' + esc(fmtTime(p.statusAt)) + '</span>' : 'never'],
        ]) + (p.validationError ? '<div class="note">Shown is the last status that passed validation.</div>' : '') : emptyState(p.status ? 'No parseable status stored' : 'No status exchange yet')), { sub: p.status ? '' : 'no exchange yet' }) +
      '</div>' +

      sec('Strikes', history.length ? '<div class="timeline">' + history.map(h => '<div class="tl-row"><span class="tl-t">' + esc(fmtTime(h.at)) + '<small>' + liveAgo(h.at) + '</small></span><span>' + srcLabel(h.source) + '</span><span class="reason">' + esc(h.reason) + '</span></div>').join('') + '</div>' +
        '<div class="note">' + history.length + ' retained of up to ' + fmtNum(cfg.historySize) + '; the standing count is what decay and grey-listing act on.</div>' : emptyState('No strikes recorded'), { sub: p.standing + ' standing · newest first' }) +

      '<div class="grid-2">' +
      sec('Gossip', kv([['Score', gossipCell(p.gossipScore) + ' <span class="muted">grey-listed below ' + fmtNum(cfg.gossipThreshold) + '</span>'], ['Behaviour penalty', fmtScore(p.behaviourPenalty)]]) +
        (topics.length ? '<div class="tbl-wrap" style="margin-top:10px"><table class="tbl"><thead><tr><th>Topic</th><th class="num">In mesh</th><th class="num">First</th><th class="num">Mesh deliveries</th><th class="num">Invalid</th></tr></thead><tbody>' +
          topics.slice(0, 40).map(t => '<tr><td title="' + attr(t.topic) + '">' + esc(t.short) + '</td><td class="num">' + fmtDur(t.mesh / 1000) + '</td><td class="num">' + fmtScore(t.first) + '</td><td class="num">' + fmtScore(t.meshDel) + '</td><td class="num">' + (t.invalid ? '<span class="sbad">' + fmtScore(t.invalid) + '</span>' : '0') + '</td></tr>').join('') + '</tbody></table></div>' +
          (topics.length > 40 ? '<div class="note">' + (topics.length - 40) + ' more topics with activity.</div>' : '') : (d.data ? '<div class="note">No per-topic gossip activity recorded by libp2p for this peer.</div>' : ''))) +
      sec('Rejected messages', rejections.length ? '<div class="timeline">' + rejections.slice(0, 50).map(r => '<div class="tl-row rej"><span class="tl-t">' + esc(fmtTime(r.at)) + '<small>' + liveAgo(r.at) + '</small></span><span>' + badge(r.topicShort, 'sm', r.topic) + '</span><span class="reason">' + esc(r.reason) + '</span></div>').join('') + '</div>' : emptyState('No rejected gossip from this peer'), { sub: rejections.length + ' retained' }) +
      '</div>';
  }

  // ------------------------------------------------------------------ views registry & routing
  const VIEW_IMPL = {
    overview: { mount: () => {}, update: updateOverview },
    peers: { mount: mountPeers, update: updatePeers },
    clients: { mount: mountClients, update: updateClients },
    greylist: { mount: mountGreylist, update: updateGreylist },
    gossip: { mount: mountGossip, update: updateGossip },
    chain: { mount: mountChain, update: updateChain },
    peer: { mount: mountPeer, update: updatePeer },
  };
  function parseHash() {
    const h = location.hash.replace(/^#\/?/, '');
    const qi = h.indexOf('?');
    const pathPart = qi >= 0 ? h.slice(0, qi) : h, qs = qi >= 0 ? h.slice(qi + 1) : '';
    const segs = pathPart.split('/').filter(Boolean);
    const view = VIEWS[segs[0]] ? segs[0] : 'overview';
    let param = segs.slice(1).join('/');
    try { param = decodeURIComponent(param); } catch (_) { /* keep raw */ }
    return { view, param, q: new URLSearchParams(qs) };
  }
  function scopeQuery(sc) {
    const q = new URLSearchParams();
    if (sc.client) q.set('client', sc.client);
    if (sc.agent) q.set('agent', sc.agent);
    if (sc.window && sc.window !== '1h') q.set('window', sc.window);
    const s = q.toString();
    return s ? '?' + s : '';
  }
  const hashFor = (view, param) => '#/' + view + (param ? '/' + encodeURIComponent(param) : '') + scopeQuery(scope);
  const hashForScope = (patch, view) => '#/' + view + scopeQuery(Object.assign({}, scope, patch));
  function syncHash() { history.replaceState(null, '', hashFor(state.route.view, state.route.param)); }
  function onRoute() {
    const r = parseHash();
    state.route = { view: r.view, param: r.param };
    const client = r.q.get('client') || '', agent = r.q.get('agent') || '', win = r.q.get('window') || '1h';
    scope.client = CLIENTS.includes(client) ? client : '';
    scope.agent = agent;
    scope.window = WINDOW_MS[win] != null ? win : '1h';
    memo.key = '';
    state.dd.open = false;
    render();
  }
  const navigate = (view, param) => { location.hash = hashFor(view, param); };
  function goBack() { if (history.length > 1 && state.cameFrom) history.back(); else navigate('peers', ''); }

  // ------------------------------------------------------------------ keyboard cursor
  const navRows = () => $$('#view [data-nav]');
  function applyCursor() {
    const rows = navRows();
    rows.forEach((r, i) => r.classList.toggle('cursor', i === state.cursor));
  }
  function moveCursor(delta) {
    const rows = navRows(); if (!rows.length) return;
    state.cursor = clamp(state.cursor + delta, 0, rows.length - 1);
    applyCursor();
    rows[state.cursor].scrollIntoView({ block: 'nearest' });
  }
  function openCursor() {
    const row = navRows()[state.cursor]; if (!row) return;
    if (row.dataset.peer) navigate('peer', row.dataset.peer);
    else if (row.dataset.agent != null) setScope({ agent: row.dataset.agent });
  }

  // ------------------------------------------------------------------ wiring
  function defaultNodeUrl() { return /^https?:$/.test(location.protocol) ? location.origin : 'http://localhost:3500'; }
  function setNodeUrl(url) {
    url = (url || '').trim();
    if (!url) url = defaultNodeUrl();
    if (!/^https?:\/\//i.test(url)) url = 'http://' + url;
    state.nodeUrl = url.replace(/\/+$/, '');
    try { localStorage.setItem(LS.url, state.nodeUrl); } catch (_) { /* storage unavailable */ }
    $('#nodeUrl').value = state.nodeUrl;
  }
  function setTheme(t) {
    document.documentElement.dataset.theme = t;
    try { localStorage.setItem(LS.theme, t); } catch (_) { /* storage unavailable */ }
  }
  let typing = null;
  function onFieldInput(c, immediate) {
    const path = c.dataset.f;
    const val = c.type === 'checkbox' ? c.checked : c.value;
    setPath(state, path, val);
    if (path === 'filters.peers.q') state.filters.peers.peerLimit = 300;
    state.cursor = -1;
    clearTimeout(typing);
    if (immediate || c.type !== 'text') VIEW_IMPL[state.route.view].update($('#view'));
    else typing = setTimeout(() => { VIEW_IMPL[state.route.view].update($('#view')); applyCursor(); }, 120);
  }
  function showTip(target, ev) {
    const tip = $('#tip');
    tip.textContent = target.dataset.tip;
    tip.hidden = false;
    moveTip(ev);
  }
  function moveTip(ev) {
    const tip = $('#tip'); if (tip.hidden) return;
    const pad = 14, w = tip.offsetWidth, h = tip.offsetHeight;
    let x = ev.clientX + pad, y = ev.clientY + pad;
    if (x + w > window.innerWidth - 8) x = ev.clientX - w - pad;
    if (y + h > window.innerHeight - 8) y = ev.clientY - h - pad;
    tip.style.left = Math.max(4, x) + 'px'; tip.style.top = Math.max(4, y) + 'px';
  }
  function init() {
    let theme = 'dark', url = '', auto = '1';
    try { theme = localStorage.getItem(LS.theme) || 'dark'; url = localStorage.getItem(LS.url) || ''; auto = localStorage.getItem(LS.auto) || '1'; } catch (_) { /* storage unavailable */ }
    setTheme(theme);
    state.auto = auto !== '0';
    $('#autoRefresh').checked = state.auto;
    // ?node=<url> overrides the stored node URL, so a dashboard link can carry its target.
    const paramUrl = new URLSearchParams(location.search).get('node');
    setNodeUrl(paramUrl || url);

    $('#nodeForm').addEventListener('submit', e => { e.preventDefault(); setNodeUrl($('#nodeUrl').value); state.spec.loaded = false; state.error = null; state.nodeFormOpen = false; refresh('connect'); });
    $('#nodeBlock').addEventListener('click', e => {
      if (e.target.closest('[data-node-toggle]')) { state.nodeFormOpen = !state.nodeFormOpen; $('#nodeForm').classList.toggle('open', state.nodeFormOpen); if (state.nodeFormOpen) $('#nodeUrl').focus(); }
      const c = e.target.closest('[data-copy]'); if (c) copyText(c.dataset.copy);
    });
    $('#refreshBtn').addEventListener('click', () => refresh('manual'));
    $('#autoRefresh').addEventListener('change', e => { state.auto = e.target.checked; try { localStorage.setItem(LS.auto, state.auto ? '1' : '0'); } catch (_) { /* ignore */ } schedule(); renderLive(); });
    $('#themeBtn').addEventListener('click', () => setTheme(document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark'));
    window.addEventListener('hashchange', onRoute);
    document.addEventListener('visibilitychange', () => { if (!document.hidden && state.auto && state.fetchedAt && Date.now() - state.fetchedAt > POLL_MS) refresh('visible'); });

    const main = $('#main');
    main.addEventListener('click', e => {
      const t = e.target;
      const a = t.closest('a[href^="#"]');
      if (a && !a.dataset.back) { state.cameFrom = true; return; } // plain hash links navigate; remember we came from inside
      const hit = sel => t.closest(sel);
      let el;
      if ((el = hit('[data-back]'))) { e.preventDefault(); goBack(); return; }
      if ((el = hit('[data-copy]'))) { copyText(el.dataset.copy); el.textContent = '✓'; setTimeout(() => { el.textContent = '⧉'; }, 900); return; }
      if ((el = hit('[data-dd]'))) { state.dd.open = !state.dd.open; state.dd.q = ''; state.dd.cursor = 0; state.dd.caret = null; renderDD(); return; }
      if ((el = hit('[data-dd-pick]'))) { state.dd.open = false; const ag = el.dataset.ddPick; const a2 = state.agents.find(x => x.agent === ag); setScope({ agent: ag, client: ag && a2 ? a2.agentType : scope.client }); return; }
      if ((el = hit('[data-scope-clear]'))) { setScope({ client: '', agent: '' }); return; }
      if ((el = hit('[data-scope-agent]'))) { setScope({ agent: el.dataset.scopeAgent }); return; }
      if ((el = hit('[data-win]'))) { setScope({ window: el.dataset.win }); return; }
      if ((el = hit('[data-client]'))) { const c = el.dataset.client; const same = scope.client === c; setScope({ client: same ? '' : c, agent: same ? scope.agent : '' }); if (state.route.view !== 'clients') navigate('clients', ''); return; }
      if ((el = hit('[data-agent]'))) { setScope({ agent: el.dataset.agent }); return; }
      if ((el = hit('[data-aspect]'))) { state.filters.greylist.aspect = el.dataset.aspect; navigate('greylist', ''); return; }
      if ((el = hit('[data-gaspect]'))) { state.filters.greylist.aspect = el.dataset.gaspect; VIEW_IMPL.greylist.update($('#view')); return; }
      if ((el = hit('[data-greason]'))) { state.filters.greylist.reason = state.filters.greylist.reason === el.dataset.greason ? '' : el.dataset.greason; if (state.route.view !== 'greylist') navigate('greylist', ''); else VIEW_IMPL.greylist.update($('#view')); return; }
      if ((el = hit('[data-reason]'))) { state.filters.greylist.reason = el.dataset.reason; state.filters.greylist.aspect = ''; navigate('greylist', ''); return; }
      if ((el = hit('[data-source]'))) { const f = state.filters.peers; f.preset = 'all'; f.source = el.dataset.source; f.q = ''; f.aspect = ''; f.dir = ''; navigate('peers', ''); return; }
      if ((el = hit('[data-gpick]'))) { state.filters.gossip.pick = state.filters.gossip.pick === el.dataset.gpick ? '' : el.dataset.gpick; VIEW_IMPL.gossip.update($('#view')); return; }
      if ((el = hit('[data-group]'))) { state.filters.gossip.groupBy = el.dataset.group; state.filters.gossip.pick = ''; VIEW_IMPL.gossip.update($('#view')); return; }
      if ((el = hit('[data-preset]'))) { state.filters.peers.preset = el.dataset.preset; state.filters.peers.peerLimit = 300; state.cursor = -1; VIEW_IMPL.peers.update($('#view')); return; }
      if ((el = hit('[data-cpreset]'))) { state.filters.clients.preset = el.dataset.cpreset; state.cursor = -1; VIEW_IMPL.clients.update($('#view')); return; }
      if ((el = hit('[data-reset]'))) { Object.assign(state.filters.peers, { preset: 'connected', q: '', dir: '', aspect: '', source: '', peerLimit: 300 }); state.cursor = -1; VIEW_IMPL.peers.update($('#view')); return; }
      if ((el = hit('[data-more]'))) { state.filters.peers.peerLimit = Infinity; VIEW_IMPL[state.route.view].update($('#view')); applyCursor(); return; }
      if ((el = hit('th[data-sort]'))) {
        const table = el.closest('table').dataset.table;
        const sortState = { peers: state.sort.peers, agents: state.sort.agents, chain: state.sort.chain, grey: state.sort.grey, near: state.sort.near }[table];
        if (sortState) { toggleSort(sortState, el.dataset.sort, /^(peer|agent|agentType|rank|aspect|reason)$/.test(el.dataset.sort) ? 'asc' : 'desc'); VIEW_IMPL[state.route.view].update($('#view')); applyCursor(); }
        return;
      }
      if ((el = hit('[data-peer]'))) { state.cameFrom = true; navigate('peer', el.dataset.peer); return; }
    });
    main.addEventListener('input', e => { const c = e.target.closest('[data-f]'); if (c && c.type !== 'checkbox' && c.tagName !== 'SELECT') onFieldInput(c, false); });
    main.addEventListener('change', e => {
      const c = e.target.closest('[data-f]'); if (c && (c.type === 'checkbox' || c.tagName === 'SELECT')) { onFieldInput(c, true); return; }
      const sc = e.target.closest('[data-scope]'); if (sc) { const patch = {}; patch[sc.dataset.scope] = sc.value; if (sc.dataset.scope === 'client') patch.agent = ''; setScope(patch); }
    });
    main.addEventListener('input', e => {
      if (e.target.id === 'ddSearch') { state.dd.q = e.target.value; state.dd.caret = e.target.selectionStart; state.dd.cursor = 0; renderDD(); }
    });
    main.addEventListener('keydown', e => {
      if (e.target.id === 'ddSearch') {
        if (e.key === 'ArrowDown') { e.preventDefault(); state.dd.cursor++; state.dd.caret = e.target.selectionStart; renderDD(); }
        else if (e.key === 'ArrowUp') { e.preventDefault(); state.dd.cursor--; state.dd.caret = e.target.selectionStart; renderDD(); }
        else if (e.key === 'Enter') { e.preventDefault(); const c = $('#ddMenu .dd-item.cursor'); if (c) c.click(); }
        else if (e.key === 'Escape') { state.dd.open = false; renderDD(); }
        e.stopPropagation();
      }
    });
    document.addEventListener('click', e => { if (state.dd.open && !e.target.closest('#agentDD')) { state.dd.open = false; renderDD(); } });
    document.addEventListener('keydown', e => {
      const tag = (e.target.tagName || '').toLowerCase();
      const inField = tag === 'input' || tag === 'select' || tag === 'textarea';
      if (e.key === 'Escape') { if (inField) { e.target.blur(); return; } if (state.dd.open) { state.dd.open = false; renderDD(); return; } if (state.route.view === 'peer') goBack(); return; }
      if (inField) return;
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === '/') { const s = $('#view [data-search]'); if (s) { e.preventDefault(); s.focus(); s.select(); } return; }
      if (e.key === 'ArrowDown' || e.key === 'j') { e.preventDefault(); moveCursor(1); return; }
      if (e.key === 'ArrowUp' || e.key === 'k') { e.preventDefault(); moveCursor(-1); return; }
      if (e.key === 'Enter') { openCursor(); return; }
      if (e.key === 'r') { refresh('manual'); }
    });
    document.addEventListener('mouseover', e => { const t = e.target.closest('[data-tip]'); if (t) showTip(t, e); else $('#tip').hidden = true; });
    document.addEventListener('mousemove', moveTip);
    document.addEventListener('focusin', e => { const t = e.target.closest && e.target.closest('[data-tip]'); if (t) { const r = t.getBoundingClientRect(); showTip(t, { clientX: r.left, clientY: r.bottom }); } });
    document.addEventListener('focusout', () => { $('#tip').hidden = true; });

    setInterval(tickLive, 1000);
    if (!location.hash) history.replaceState(null, '', '#/overview');
    onRoute();
    refresh('init');
  }
  init();
})();
