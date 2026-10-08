/* PeerBuddy — Prysm peer connectivity & scoring dashboard.
 * Vanilla JS, no build step. Polls the beacon node's REST API every 10 seconds. */
'use strict';
(function () {
  // ------------------------------------------------------------------ constants
  const POLL_MS = 10000;
  const STORAGE_URL = 'peerbuddy.nodeUrl';
  const STORAGE_THEME = 'peerbuddy.theme';
  const STATES = ['CONNECTED', 'CONNECTING', 'DISCONNECTING', 'DISCONNECTED'];
  const AGENT_TYPES = ['prysm', 'lighthouse', 'teku', 'nimbus', 'lodestar', 'grandine', 'erigon/caplin', 'js-libp2p', 'rust-libp2p', 'unknown'];
  // Same list and last-match-wins rule as the node's classifier; only used when the API omits agent_type.
  const KNOWN_AGENT_ORDER = ['erigon/caplin', 'grandine', 'js-libp2p', 'lighthouse', 'lodestar', 'nimbus', 'prysm', 'teku', 'rust-libp2p'];
  const AGENT_CLASS = {
    'prysm': 'agent-prysm', 'lighthouse': 'agent-lighthouse', 'teku': 'agent-teku', 'nimbus': 'agent-nimbus',
    'lodestar': 'agent-lodestar', 'grandine': 'agent-grandine', 'erigon/caplin': 'agent-erigon',
    'js-libp2p': 'agent-js', 'rust-libp2p': 'agent-rust', 'unknown': 'agent-unknown',
  };
  const AGENT_VAR = {
    'prysm': '--c-prysm', 'lighthouse': '--c-lighthouse', 'teku': '--c-teku', 'nimbus': '--c-nimbus',
    'lodestar': '--c-lodestar', 'grandine': '--c-grandine', 'erigon/caplin': '--c-erigon',
    'js-libp2p': '--c-js', 'rust-libp2p': '--c-rust', 'unknown': '--c-unknown',
  };
  const STRIKE_SOURCES = ['dial', 'rpc-status', 'rpc-ping', 'rpc-metadata', 'rpc-request', 'rpc-response', 'rate-limit', 'gossip', 'sync', 'backfill', 'das', 'unknown'];
  const SOURCE_COLOR = {
    'dial': '#38bdf8', 'rpc-status': '#7c8cf8', 'rpc-ping': '#fbbf24', 'rpc-metadata': '#c084fc',
    'rpc-request': '#fb7185', 'rpc-response': '#f97316', 'rate-limit': '#eab308', 'gossip': '#2dd4bf',
    'sync': '#60a5fa', 'backfill': '#a3e635', 'das': '#f472b6', 'unknown': '#94a3b8',
  };
  const ASPECTS = [['strikes', 'Strikes'], ['peer_status', 'Peer status'], ['gossip', 'Gossip'], ['bad_ip', 'Bad IP']];
  const ASPECT_CLASS = { strikes: 'b-bad', peer_status: 'b-warn', gossip: 'b-accent2', bad_ip: 'b-info' };
  const ASPECT_SHORT = { strikes: 'strikes', peer_status: 'status', gossip: 'gossip', bad_ip: 'bad IP' };
  const VIEWS = ['overview', 'peers', 'agents', 'gossip', 'chain'];
  const TENURE_BUCKETS = [
    { label: '< 1m', max: 60 }, { label: '1–5m', max: 300 }, { label: '5–30m', max: 1800 },
    { label: '30m–2h', max: 7200 }, { label: '2–12h', max: 43200 }, { label: '12h+', max: Infinity },
  ];

  // ------------------------------------------------------------------ utils
  const $ = (sel, root) => (root || document).querySelector(sel);
  const $$ = (sel, root) => Array.from((root || document).querySelectorAll(sel));
  const esc = s => String(s == null ? '' : s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const toInt = v => { if (v == null || v === '') return null; const n = Number(v); return Number.isFinite(n) ? n : null; };
  const cap = s => s ? s.charAt(0) + s.slice(1).toLowerCase() : '';
  const short = id => (id && id.length > 18) ? id.slice(0, 8) + '…' + id.slice(-6) : (id || '');
  const fmtNum = n => (n == null || !Number.isFinite(n)) ? '–' : n.toLocaleString();
  const pct = (a, b) => b ? Math.round(100 * a / b) : 0;
  const sum = arr => arr.reduce((s, v) => s + v, 0);

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
  const fmtScore = s => (s == null) ? '–' : (Math.abs(s) >= 100 ? Math.round(s).toLocaleString() : (Math.round(s * 10) / 10).toString());
  function median(sorted) { const n = sorted.length; return n ? (n % 2 ? sorted[(n - 1) / 2] : (sorted[n / 2 - 1] + sorted[n / 2]) / 2) : null; }
  function quantile(sorted, q) { const n = sorted.length; return n ? sorted[Math.min(n - 1, Math.floor(q * n))] : null; }
  function countBy(arr) { const m = new Map(); for (const k of arr) m.set(k, (m.get(k) || 0) + 1); return m; }
  function topN(map, n) { return Array.from(map.entries()).sort((a, b) => b[1] - a[1] || String(a[0]).localeCompare(String(b[0]))).slice(0, n); }
  function agentTypeOf(agent) { const a = (agent || '').toLowerCase(); let found = 'unknown'; for (const t of KNOWN_AGENT_ORDER) if (a.includes(t)) found = t; return found; }
  function shortTopic(t) { const m = /^\/eth2\/[0-9a-f]+\/(.+?)\/ssz_snappy$/.exec(t || ''); return m ? m[1] : (t || ''); }
  const agentColor = type => 'var(' + (AGENT_VAR[type] || '--c-unknown') + ')';
  const sourceColor = s => SOURCE_COLOR[s] || SOURCE_COLOR.unknown;
  function compare(a, b) { if (a == null && b == null) return 0; if (a == null) return 1; if (b == null) return -1; return typeof a === 'string' ? a.localeCompare(b) : a - b; }
  function copyText(text) { if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(text).catch(() => {}); }

  // ------------------------------------------------------------------ state
  const state = {
    nodeUrl: '',
    view: 'overview',
    mounted: null,
    auto: true,
    loading: false,
    error: null,
    warnings: [],
    fetchedAt: 0,
    nextAt: 0,
    spec: { slotsPerEpoch: 32, secondsPerSlot: 12, loaded: false },
    raw: {},
    cfg: {},
    node: {},
    peers: [],
    peersById: new Map(),
    agents: [],
    rejections: [],
    filters: {
      peers: defaultPeerFilters(),
      agents: { type: '', q: '', connectedOnly: false },
      gossip: { groupBy: 'topic', topic: '', agent: '', peer: '', sinceMs: 0, sincePreset: 'all' },
      chain: { q: '', connectedOnly: true },
    },
    sort: {
      peers: { key: 'rank', dir: 'asc' },
      agents: { key: 'peerCount', dir: 'desc' },
      chain: { key: 'headSlot', dir: 'desc' },
    },
    peersLimit: 300,
    selected: null,
    selectedDetail: null,
    selectedEth: null,
    selectedError: null,
    selectedLoading: false,
  };
  function defaultPeerFilters() {
    return { preset: 'connected', q: '', states: new Set(['CONNECTED']), dirs: new Set(), grey: 'any', aspect: '', type: '', source: '', striking: false, trustedOnly: false };
  }

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
    renderStatus();
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
    if (state.selected && !fatal) loadPeerDetail(state.selected);
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

    state.rejections = ((r.rejections && r.rejections.data) || []).map(rj => ({
      peerId: rj.peer_id, topic: rj.topic || '', topicShort: shortTopic(rj.topic), agent: rj.agent || '',
      agentType: rj.agent_type || agentTypeOf(rj.agent), reason: rj.reason || '', at: Date.parse(rj.timestamp) || 0,
    }));
    state.rejections.sort((a, b) => b.at - a.at);

    const connByAgent = new Map();
    for (const p of peers) if (p.connected) { const k = p.agent || 'unknown'; connByAgent.set(k, (connByAgent.get(k) || 0) + 1); }
    state.agents = ((r.agents && r.agents.data) || []).map(a => {
      const bySource = a.strikes_by_source || {};
      return {
        agent: a.agent || 'unknown', agentType: a.agent_type || 'unknown', peerCount: a.peer_count || 0,
        greyCount: a.grey_listed_peer_count || 0, bySource, strikesTotal: sum(Object.values(bySource)),
        rejections: a.gossip_rejections_count || 0, connected: connByAgent.get(a.agent || 'unknown') || 0,
      };
    });
    document.title = 'PeerBuddy · ' + peers.filter(p => p.connected).length + ' peers';
  }

  function enrichPeer(p, ctx) {
    const id = p.peer_id;
    const connected = p.connection_state === 'CONNECTED';
    const connectedAt = p.connected_at ? (Date.parse(p.connected_at) || 0) : 0;
    const details = p.grey_list_details || {};
    const aspects = ASPECTS.map(a => a[0]).filter(k => details[k]);
    const recovery = p.grey_list_recovery || {};
    let recoverySec = null, recoveryUnknown = false;
    for (const k of Object.keys(recovery)) {
      const s = parseGoDuration(recovery[k]);
      if (s == null) recoveryUnknown = true; else recoverySec = Math.max(recoverySec == null ? 0 : recoverySec, s);
    }
    const strikes = p.strikes || {};
    const history = (strikes.history || []).map(h => ({ source: h.source || 'unknown', reason: h.reason || '', at: Date.parse(h.timestamp) || 0 }));
    const h1 = ctx.now - 3600e3, h24 = ctx.now - 86400e3;
    const st = p.rpc_status || null;
    const cs = (st && st.chain_state) || null;
    const headSlot = cs ? toInt(cs.head_slot) : null;
    const eth = ctx.ethById.get(id);
    const gossip = p.gossip || {};
    const rejections = (gossip.rejections || []).map(rj => ({
      topic: rj.topic || '', topicShort: shortTopic(rj.topic), agent: rj.agent || '', agentType: rj.agent_type || agentTypeOf(rj.agent),
      reason: rj.reason || '', at: Date.parse(rj.timestamp) || 0,
    }));
    return {
      id, short: short(id), agent: p.agent || '', agentType: p.agent_type || agentTypeOf(p.agent),
      state: p.connection_state || 'DISCONNECTED', direction: p.direction || 'UNKNOWN', connected, connectedAt,
      tenureSec: connected && connectedAt ? (ctx.now - connectedAt) / 1000 : null,
      greyListed: !!p.grey_listed, details, aspects, exemption: p.grey_list_exemption || '',
      recovery, recoverySec, recoveryUnknown, recoveryEnd: recoverySec == null ? null : ctx.now + recoverySec * 1000,
      standing: strikes.standing_count || 0, threshold: strikes.grey_list_threshold || state.cfg.threshold || 5,
      history, lastStrikeAt: history.length ? history[history.length - 1].at : 0,
      strikes1h: history.filter(h => h.at >= h1).length, strikes24h: history.filter(h => h.at >= h24).length,
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

  // ------------------------------------------------------------------ components
  const badge = (text, cls, title) => '<span class="badge ' + cls + '"' + (title ? ' title="' + esc(title) + '"' : '') + '>' + esc(text) + '</span>';
  const stateBadge = s => badge(cap(s), { CONNECTED: 'b-ok', CONNECTING: 'b-info', DISCONNECTING: 'b-warn', DISCONNECTED: 'b-muted' }[s] || 'b-muted');
  const dirBadge = d => d === 'INBOUND' ? badge('in', 'b-outline', 'Inbound: the peer dialed us') : d === 'OUTBOUND' ? badge('out', 'b-accent2', 'Outbound: we dialed the peer') : '';
  const aspectBadge = (a, title) => badge(ASPECT_SHORT[a] || a, ASPECT_CLASS[a] || 'b-muted', title);
  const sourceBadge = s => '<span class="badge sm" style="background:' + sourceColor(s) + '22;color:' + sourceColor(s) + '">' + esc(s) + '</span>';
  const agentDot = type => '<span class="dot ' + (AGENT_CLASS[type] || 'agent-unknown') + '" title="' + esc(type) + '"></span>';
  const peerLink = (id, label) => '<span class="peer-link" data-peer="' + esc(id) + '" title="' + esc(id) + '">' + esc(label || short(id)) + '</span>';
  const copyBtn = text => '<button class="copy" type="button" data-copy="' + esc(text) + '" title="Copy">⧉</button>';

  function kpi(label, value, subs, cls) {
    return '<div class="card kpi ' + (cls || '') + '"><div class="kpi-l">' + esc(label) + '</div><div class="kpi-v">' + value + '</div><div class="kpi-s">' +
      (subs || []).filter(Boolean).map(s => '<span>' + s + '</span>').join('') + '</div></div>';
  }
  function card(title, body, sub, extraClass) {
    return '<div class="card ' + (extraClass || '') + '"><div class="card-h"><h2>' + esc(title) + '</h2>' + (sub ? '<span class="sub">' + sub + '</span>' : '') + '</div><div class="card-b">' + body + '</div></div>';
  }
  function strikeMeter(p) {
    const ratio = p.threshold ? p.standing / p.threshold : 0;
    const cls = ratio >= 1 ? 'bad' : ratio >= 0.6 ? 'warn' : '';
    return '<span class="meter" title="' + p.standing + ' standing strikes, grey-listed at ' + p.threshold + '"><span class="meter-track"><span class="meter-fill ' + cls + '" style="width:' + Math.min(100, ratio * 100) + '%"></span></span><span class="meter-v">' + p.standing + '/' + p.threshold + '</span></span>';
  }
  function bars(items, opts) {
    opts = opts || {};
    if (!items.length) return '<div class="empty">' + esc(opts.empty || 'Nothing to show') + '</div>';
    const mx = opts.max != null ? opts.max : Math.max.apply(null, items.map(i => i.value).concat([1]));
    const fmt = opts.fmt || fmtNum;
    return '<div class="bars">' + items.map(i =>
      '<div class="bar-row' + (opts.wide ? ' wide' : '') + '"><span class="bar-label' + (opts.dataKey ? ' clickable' : '') + '" title="' + esc(i.title || i.label) + '"' +
      (opts.dataKey ? ' data-' + opts.dataKey + '="' + esc(i.key != null ? i.key : i.label) + '"' : '') + '>' +
      (i.dot ? '<span class="dot" style="background:' + i.dot + '"></span>' : '') + esc(i.label) + '</span>' +
      '<span class="bar-track"><span class="bar-fill" style="width:' + Math.max(1, 100 * i.value / mx) + '%;background:' + (i.color || 'var(--accent)') + '"></span></span>' +
      '<span class="bar-v">' + fmt(i.value) + (opts.total ? '<small>' + pct(i.value, opts.total) + '%</small>' : '') + '</span></div>').join('') + '</div>';
  }
  function columns(items) {
    const mx = Math.max.apply(null, items.map(i => i.value).concat([1]));
    return '<div class="columns">' + items.map(i =>
      '<div class="col"><div class="col-bar ' + (i.cls || '') + '" style="height:' + Math.max(2, 100 * i.value / mx) + '%"><span class="col-v">' + i.value + '</span></div><div class="col-l">' + esc(i.label) + '</div></div>').join('') + '</div>';
  }
  function donut(items, label) {
    const total = sum(items.map(i => i.value));
    const r = 44, c = 2 * Math.PI * r;
    let off = 0;
    const segs = items.filter(i => i.value > 0).map(i => {
      const len = c * i.value / total;
      const s = '<circle r="' + r + '" cx="64" cy="64" fill="none" stroke="' + i.color + '" stroke-width="14" stroke-dasharray="' + len + ' ' + (c - len) + '" stroke-dashoffset="' + (-off) + '" transform="rotate(-90 64 64)"><title>' + esc(i.label) + ': ' + i.value + '</title></circle>';
      off += len;
      return s;
    }).join('');
    const legend = items.map(i => '<div class="legend-row"><span class="dot" style="background:' + i.color + '"></span><span>' + esc(i.label) + '</span><span class="n">' + i.value + '</span><span class="p">' + pct(i.value, total) + '%</span></div>').join('');
    return '<div class="donut"><svg viewBox="0 0 128 128">' + (total ? segs : '<circle r="' + r + '" cx="64" cy="64" fill="none" stroke="var(--surface-3)" stroke-width="14"/>') +
      '<text x="64" y="62" text-anchor="middle" class="donut-center">' + total + '</text><text x="64" y="76" text-anchor="middle" class="donut-center-l">' + esc(label) + '</text></svg>' +
      '<div class="donut-legend">' + (legend || '<span class="faint">No data</span>') + '</div></div>';
  }
  function stackBar(bySource, total) {
    const keys = Object.keys(bySource).sort((a, b) => bySource[b] - bySource[a]);
    if (!total) return '<span class="faint">—</span>';
    return '<span class="stack-bar" title="' + esc(keys.map(k => k + ': ' + bySource[k]).join(', ')) + '">' +
      keys.map(k => '<span style="width:' + (100 * bySource[k] / total) + '%;background:' + sourceColor(k) + '"></span>').join('') + '</span>';
  }
  function th(label, key, sort, cls) {
    const on = sort.key === key;
    return '<th class="sortable ' + (cls || '') + '" data-sort="' + key + '">' + esc(label) + (on ? '<span class="sort-ind">' + (sort.dir === 'asc' ? '▲' : '▼') + '</span>' : '') + '</th>';
  }
  function sortBy(rows, keyFn, dir) {
    const sign = dir === 'asc' ? 1 : -1;
    return rows.slice().sort((a, b) => sign * compare(keyFn(a), keyFn(b)));
  }
  function toggleSort(sortState, key, defaultDir) {
    if (sortState.key === key) sortState.dir = sortState.dir === 'asc' ? 'desc' : 'asc';
    else { sortState.key = key; sortState.dir = defaultDir || 'desc'; }
  }
  const liveSince = ts => '<span data-live="since" data-ts="' + ts + '">' + fmtDur((Date.now() - ts) / 1000) + '</span>';
  const liveAgo = ts => '<span data-live="ago" data-ts="' + ts + '">' + ago(ts) + '</span>';
  const liveCountdown = end => '<span data-live="countdown" data-end="' + end + '">' + fmtDur((end - Date.now()) / 1000) + '</span>';
  function deltaCell(d) {
    if (d == null) return '<span class="faint">—</span>';
    const spe = state.spec.slotsPerEpoch;
    const cls = d > 1 ? 'delta-pos' : d < -10 * spe ? 'delta-far' : d < -1 ? 'delta-neg' : '';
    return '<span class="' + cls + '">' + (d > 0 ? '+' : '') + d + '</span>';
  }
  function gossipCell(score) {
    const cls = score < state.cfg.gossipThreshold ? 'score-bad' : score < 0 ? 'score-neg' : '';
    return '<span class="' + cls + '" title="Grey-listed below ' + state.cfg.gossipThreshold + '">' + fmtScore(score) + '</span>';
  }
  function greyCell(p) {
    if (!p.aspects.length) return '<span class="faint">—</span>';
    let html = '<span class="badges">' + p.aspects.map(a => aspectBadge(a, p.details[a])).join('');
    if (!p.greyListed && p.exemption) html += badge('exempt: ' + p.exemption, 'b-accent', 'Refusal sources fire but the peer is exempt');
    return html + '</span>';
  }
  function recoveryCell(p) {
    if (!p.greyListed) return '<span class="faint">—</span>';
    if (p.recoveryEnd != null) return liveCountdown(p.recoveryEnd);
    if (p.recoveryUnknown) return '<span class="muted" title="Gossip and bad-IP recovery are not time based">unknown</span>';
    return '<span class="faint">—</span>';
  }
  function statusCell(p) {
    if (!p.status) return '<span class="faint">never</span>';
    return liveAgo(p.statusAt) + (p.validationError ? ' ' + badge('invalid', 'b-bad', p.validationError) : '');
  }

  // ------------------------------------------------------------------ render shell
  function render() {
    renderTabs();
    renderBanner();
    renderStatus();
    const main = $('#main');
    if (state.mounted !== state.view) {
      main.innerHTML = '';
      VIEW_IMPL[state.view].mount(main);
      state.mounted = state.view;
    }
    VIEW_IMPL[state.view].update(main);
    if (state.selected) renderDrawer();
  }
  function renderTabs() {
    $$('.tab').forEach(t => t.classList.toggle('on', t.dataset.view === state.view));
    const counts = { peers: state.peers.filter(p => p.connected).length, agents: state.agents.length, gossip: state.rejections.length };
    $$('.tab-count').forEach(el => { el.textContent = state.fetchedAt ? fmtNum(counts[el.dataset.count]) : ''; });
  }
  function renderBanner() {
    const b = $('#banner');
    if (state.error) { b.className = 'banner error'; b.innerHTML = '<b>Node unreachable.</b> ' + esc(state.error); b.hidden = false; }
    else if (state.warnings.length) { b.className = 'banner warn'; b.innerHTML = '<b>Partial data.</b> Some requests failed: ' + state.warnings.map(w => '<code>' + esc(w) + '</code>').join(', '); b.hidden = false; }
    else b.hidden = true;
  }
  function renderStatus() {
    const el = $('#status'), txt = $('#statusText');
    let cls = 'status', text;
    if (state.loading) { cls += ' loading'; text = 'Refreshing…'; }
    else if (state.error) { cls += ' error'; text = 'Error' + (state.auto && state.nextAt ? ' · retry in ' + Math.max(0, Math.ceil((state.nextAt - Date.now()) / 1000)) + 's' : ''); }
    else if (!state.fetchedAt) { text = 'Not connected'; }
    else {
      cls += state.auto ? ' ok' : ' paused';
      text = 'Updated ' + ago(state.fetchedAt) + (state.auto && state.nextAt ? ' · next in ' + Math.max(0, Math.ceil((state.nextAt - Date.now()) / 1000)) + 's' : ' · auto-refresh off');
    }
    el.className = cls;
    txt.textContent = text;
  }
  function tickLive() {
    renderStatus();
    $$('[data-live]').forEach(el => {
      const kind = el.dataset.live;
      if (kind === 'ago') el.textContent = ago(+el.dataset.ts);
      else if (kind === 'since') el.textContent = fmtDur((Date.now() - +el.dataset.ts) / 1000);
      else if (kind === 'countdown') { const left = (+el.dataset.end - Date.now()) / 1000; el.textContent = left > 0 ? fmtDur(left) : 'due now'; }
    });
  }

  // ------------------------------------------------------------------ overview
  function renderOverview(el) {
    if (!state.fetchedAt) { el.innerHTML = '<div class="empty">Waiting for the first response from the node…</div>'; return; }
    const ps = state.peers, cfg = state.cfg, node = state.node, now = Date.now();
    const conn = ps.filter(p => p.connected);
    const inbound = conn.filter(p => p.direction === 'INBOUND'), outbound = conn.filter(p => p.direction === 'OUTBOUND');
    const grey = ps.filter(p => p.greyListed);
    const greyConn = grey.filter(p => p.connected);
    const aspectCounts = countBy(grey.flatMap(p => p.aspects));
    const exempt = ps.filter(p => p.exemption).length;
    const allStrikes = ps.flatMap(p => p.history.map(h => ({ source: h.source, reason: h.reason, at: h.at, peer: p })));
    const strikes1h = allStrikes.filter(h => h.at >= now - 3600e3).length;
    const strikes24h = allStrikes.filter(h => h.at >= now - 86400e3).length;
    const striking = ps.filter(p => p.standing > 0).length;
    const bySource = countBy(allStrikes.map(h => h.source));
    const inTenures = inbound.map(p => p.tenureSec).filter(x => x != null).sort((a, b) => a - b);
    const outTenures = outbound.map(p => p.tenureSec).filter(x => x != null).sort((a, b) => a - b);
    const typeCounts = countBy(conn.map(p => p.agentType));
    const rej = state.rejections;
    const rejPeers = new Set(rej.map(r => r.peerId)).size;
    const withChain = conn.filter(p => p.headDelta != null);
    const atHead = withChain.filter(p => Math.abs(p.headDelta) <= 1).length;
    const behind = withChain.filter(p => p.headDelta < -1).length;
    const ahead = withChain.filter(p => p.headDelta > 1).length;

    const tenureCols = TENURE_BUCKETS.map((b, i) => {
      const lo = i ? TENURE_BUCKETS[i - 1].max : 0;
      return { label: b.label, value: inTenures.filter(t => t >= lo && t < b.max).length, cls: i >= 4 ? 'ok' : i >= 2 ? '' : 'info' };
    });
    const recentStrikes = allStrikes.sort((a, b) => b.at - a.at).slice(0, 12);
    const recentRej = rej.slice(0, 10);
    const ident = node.identity || {};
    const md = ident.metadata || {};

    el.innerHTML =
      '<div class="grid kpis">' +
      kpi('Connected peers', fmtNum(conn.length), [inbound.length + ' inbound', outbound.length + ' outbound', node.peerCount ? fmtNum(toInt(node.peerCount.connecting)) + ' connecting' : ''], 'accent') +
      kpi('Grey-listed', fmtNum(grey.length), ASPECTS.map(a => aspectCounts.get(a[0]) ? aspectBadge(a[0]) + ' ' + aspectCounts.get(a[0]) : '').concat([greyConn.length + ' still connected']), grey.length ? 'bad' : '') +
      kpi('Strikes last hour', fmtNum(strikes1h), [strikes24h + ' in 24h', striking + ' peers with standing strikes', '<span class="faint">from retained history</span>'], strikes1h ? 'warn' : '') +
      kpi('Gossip rejections', fmtNum(rej.length), [rejPeers + ' peers', 'cap ' + fmtNum(cfg.maxRejections) + ' per peer'], rej.length ? 'warn' : '') +
      kpi('Median inbound tenure', fmtDur(median(inTenures)), ['p90 ' + fmtDur(quantile(inTenures, 0.9)), 'oldest ' + fmtDur(inTenures.length ? inTenures[inTenures.length - 1] : null), 'outbound median ' + fmtDur(median(outTenures))], 'info') +
      kpi('Head slot', fmtNum(node.headSlot), [node.isSyncing ? badge('syncing', 'b-warn') : badge('in sync', 'b-ok'), node.isOptimistic ? badge('optimistic', 'b-warn') : '', node.syncDistance != null ? 'distance ' + node.syncDistance : '', atHead + ' peers at head · ' + ahead + ' ahead · ' + behind + ' behind'], node.isSyncing ? 'warn' : '') +
      '</div>' +
      '<div class="grid cols-4" style="margin-top:14px">' +
      card('Connected clients', donut(AGENT_TYPES.filter(t => typeCounts.get(t)).map(t => ({ label: t, value: typeCounts.get(t), color: agentColor(t) })), 'connected'), 'by agent type') +
      card('Grey-list verdicts', bars(ASPECTS.map(a => ({ label: a[1], value: aspectCounts.get(a[0]) || 0, color: { strikes: 'var(--bad)', peer_status: 'var(--warn)', gossip: 'var(--accent-2)', bad_ip: 'var(--info)' }[a[0]], key: a[0] })), { dataKey: 'aspect', max: Math.max(grey.length, 1) }) +
        '<div class="inline-note">' + grey.length + ' peers refused' + (exempt ? ' · ' + exempt + ' exempt (trusted)' : '') + ' · a peer may fire several aspects</div>', 'click to filter peers') +
      card('Strikes by source', bars(topN(bySource, 12).map(([k, v]) => ({ label: k, value: v, color: sourceColor(k), key: k })), { dataKey: 'source', total: allStrikes.length, empty: 'No strikes retained' }), 'click to filter peers') +
      card('Inbound tenure', columns(tenureCols) + '<div class="inline-note">Eviction protects the oldest quarter of inbound peers; grey-listed peers go first.</div>', inbound.length + ' inbound peers') +
      '</div>' +
      '<div class="grid cols-3" style="margin-top:14px">' +
      card('Recent strikes', recentStrikes.length ? '<div class="list">' + recentStrikes.map(h =>
        '<div class="list-row"><span class="list-time" title="' + esc(fmtTime(h.at)) + '">' + liveAgo(h.at) + '</span><span class="list-main">' + agentDot(h.peer.agentType) + peerLink(h.peer.id) + sourceBadge(h.source) + '<span class="reason">' + esc(h.reason) + '</span></span></div>').join('') + '</div>' : '<div class="empty">No strikes retained</div>') +
      card('Recent gossip rejections', recentRej.length ? '<div class="list">' + recentRej.map(r =>
        '<div class="list-row"><span class="list-time" title="' + esc(fmtTime(r.at)) + '">' + liveAgo(r.at) + '</span><span class="list-main">' + agentDot(r.agentType) + peerLink(r.peerId) + badge(r.topicShort, 'b-muted', r.topic) + '<span class="reason">' + esc(r.reason) + '</span></span></div>').join('') + '</div>' : '<div class="empty">No rejections retained</div>') +
      card('This node', '<dl class="kv">' +
        '<dt>Version</dt><dd>' + esc(node.version || '–') + '</dd>' +
        '<dt>Peer ID</dt><dd class="mono">' + esc(ident.peer_id || '–') + (ident.peer_id ? ' ' + copyBtn(ident.peer_id) : '') + '</dd>' +
        '<dt>Listening</dt><dd class="mono">' + esc(((ident.p2p_addresses || []).filter(a => !/\/ip4\/(127\.|10\.|172\.(1[6-9]|2\d|3[01])\.|192\.168\.)/.test(a))[0]) || (ident.p2p_addresses || [])[0] || '–') + '</dd>' +
        '<dt>Metadata</dt><dd>seq ' + esc(md.seq_number || '–') + ' · attnets <span class="mono">' + esc(md.attnets || '–') + '</span>' + (md.syncnets ? ' · syncnets <span class="mono">' + esc(md.syncnets) + '</span>' : '') + (md.custody_group_count ? ' · custody groups ' + esc(md.custody_group_count) : '') + '</dd>' +
        '<dt>Finalized</dt><dd>epoch ' + fmtNum(node.finalizedEpoch) + '</dd>' +
        '<dt>Scoring</dt><dd>' + cfg.threshold + ' strikes grey-list · 1 forgiven every ' + esc(cfg.decay || '–') + ' · history ' + fmtNum(cfg.historySize) + '</dd>' +
        '<dt>Status TTL</dt><dd>terminal status verdicts expire after ' + esc(cfg.statusTTL || '–') + '</dd>' +
        '<dt>Gossip</dt><dd>grey-listed below ' + fmtNum(cfg.gossipThreshold) + '</dd>' +
        '<dt>Tracked</dt><dd>' + fmtNum(cfg.trackedPeers) + ' peers in scorer · ' + fmtNum(ps.length) + ' listed · ' + fmtNum(cfg.peersWithRejections) + ' with rejections</dd>' +
        '</dl>') +
      '</div>';
  }

  // ------------------------------------------------------------------ peers
  const PEER_SORT = {
    rank: p => p.rank, peer: p => p.id, agent: p => p.agent || null, state: p => STATES.indexOf(p.state) * 2 + (p.direction === 'OUTBOUND' ? 1 : 0),
    tenure: p => p.tenureSec, strikes: p => p.standing, grey: p => (p.greyListed ? 100 : 0) + p.aspects.length,
    recovery: p => p.greyListed ? (p.recoverySec == null ? Infinity : p.recoverySec) : null, gossip: p => p.gossipScore,
    head: p => p.headDelta, rejections: p => p.rejectionsCount, status: p => p.statusAt || null,
  };
  const PRESETS = [['all', 'All'], ['connected', 'Connected'], ['greylisted', 'Grey-listed'], ['striking', 'With strikes'], ['trusted', 'Trusted']];

  function applyPreset(name) {
    const f = state.filters.peers;
    const q = f.q;
    Object.assign(f, defaultPeerFilters(), { q, preset: name, states: new Set() });
    if (name === 'connected') f.states = new Set(['CONNECTED']);
    if (name === 'greylisted') f.grey = 'yes';
    if (name === 'striking') f.striking = true;
    if (name === 'trusted') f.trustedOnly = true;
  }
  function filteredPeers() {
    const f = state.filters.peers, q = f.q.trim().toLowerCase();
    return state.peers.filter(p => {
      if (f.states.size && !f.states.has(p.state)) return false;
      if (f.dirs.size && !f.dirs.has(p.direction)) return false;
      if (f.grey === 'yes' && !p.greyListed) return false;
      if (f.grey === 'no' && p.greyListed) return false;
      if (f.aspect && !p.aspects.includes(f.aspect)) return false;
      if (f.type && p.agentType !== f.type) return false;
      if (f.source && !p.history.some(h => h.source === f.source)) return false;
      if (f.striking && p.standing <= 0) return false;
      if (f.trustedOnly && !p.trusted) return false;
      if (q && !(p.id.toLowerCase().includes(q) || p.agent.toLowerCase().includes(q) || p.addr.toLowerCase().includes(q) || p.agentType.includes(q))) return false;
      return true;
    });
  }
  function mountPeers(el) {
    const f = state.filters.peers;
    el.innerHTML =
      '<div class="toolbar">' +
      '<div class="toolbar-row">' +
      '<div class="segmented" id="peerPresets">' + PRESETS.map(p => '<button type="button" data-preset="' + p[0] + '">' + p[1] + '</button>').join('') + '</div>' +
      '<input class="input grow mono" id="peerSearch" placeholder="Search peer id, agent, address…" value="' + esc(f.q) + '">' +
      '<span class="result-count" id="peerCount"></span>' +
      '</div>' +
      '<div class="toolbar-row">' +
      '<div class="filter"><span>State</span><div class="chipset" id="peerStates">' + STATES.map(s => '<span class="chip" data-state="' + s + '">' + cap(s) + '</span>').join('') + '</div></div>' +
      '<div class="filter"><span>Direction</span><div class="chipset" id="peerDirs"><span class="chip" data-dir="INBOUND">Inbound</span><span class="chip" data-dir="OUTBOUND">Outbound</span></div></div>' +
      '<div class="filter"><span>Grey-listed</span><select class="select" id="peerGrey"><option value="any">Any</option><option value="yes">Yes</option><option value="no">No</option></select></div>' +
      '<div class="filter"><span>Aspect</span><select class="select" id="peerAspect"><option value="">Any</option>' + ASPECTS.map(a => '<option value="' + a[0] + '">' + a[1] + '</option>').join('') + '</select></div>' +
      '<div class="filter"><span>Client</span><select class="select" id="peerType"><option value="">All</option>' + AGENT_TYPES.map(t => '<option value="' + t + '">' + t + '</option>').join('') + '</select></div>' +
      '<div class="filter"><span>Strike source</span><select class="select" id="peerSource"><option value="">Any</option>' + STRIKE_SOURCES.map(s => '<option value="' + s + '">' + s + '</option>').join('') + '</select></div>' +
      '<button class="btn ghost small" id="peerClear" type="button">Reset</button>' +
      '</div></div>' +
      '<div class="card"><div class="card-b flush"><div class="table-wrap" id="peerTable"></div></div></div>';

    const changed = () => { f.preset = 'custom'; syncPeerToolbar(); state.peersLimit = 300; updatePeers(el); };
    $('#peerPresets', el).addEventListener('click', e => { const b = e.target.closest('[data-preset]'); if (!b) return; applyPreset(b.dataset.preset); syncPeerToolbar(); state.peersLimit = 300; updatePeers(el); });
    $('#peerSearch', el).addEventListener('input', e => { f.q = e.target.value; state.peersLimit = 300; updatePeers(el); });
    $('#peerStates', el).addEventListener('click', e => { const c = e.target.closest('[data-state]'); if (!c) return; const s = c.dataset.state; if (f.states.has(s)) f.states.delete(s); else f.states.add(s); changed(); });
    $('#peerDirs', el).addEventListener('click', e => { const c = e.target.closest('[data-dir]'); if (!c) return; const d = c.dataset.dir; if (f.dirs.has(d)) f.dirs.delete(d); else f.dirs.add(d); changed(); });
    $('#peerGrey', el).addEventListener('change', e => { f.grey = e.target.value; changed(); });
    $('#peerAspect', el).addEventListener('change', e => { f.aspect = e.target.value; changed(); });
    $('#peerType', el).addEventListener('change', e => { f.type = e.target.value; changed(); });
    $('#peerSource', el).addEventListener('change', e => { f.source = e.target.value; changed(); });
    $('#peerClear', el).addEventListener('click', () => { Object.assign(f, defaultPeerFilters()); $('#peerSearch', el).value = ''; syncPeerToolbar(); updatePeers(el); });
    $('#peerTable', el).addEventListener('click', e => {
      const h = e.target.closest('th[data-sort]');
      if (h) { toggleSort(state.sort.peers, h.dataset.sort, h.dataset.sort === 'peer' || h.dataset.sort === 'agent' || h.dataset.sort === 'rank' ? 'asc' : 'desc'); updatePeers(el); return; }
      if (e.target.closest('#peerMore')) { state.peersLimit = Infinity; updatePeers(el); return; }
      const row = e.target.closest('tr[data-peer]');
      if (row) openPeer(row.dataset.peer);
    });
    syncPeerToolbar();
  }
  function syncPeerToolbar() {
    const f = state.filters.peers;
    $$('#peerPresets button').forEach(b => b.classList.toggle('on', b.dataset.preset === f.preset));
    $$('#peerStates .chip').forEach(c => c.classList.toggle('on', f.states.has(c.dataset.state)));
    $$('#peerDirs .chip').forEach(c => c.classList.toggle('on', f.dirs.has(c.dataset.dir)));
    const set = (id, v) => { const el = $(id); if (el && el.value !== v) el.value = v; };
    set('#peerGrey', f.grey); set('#peerAspect', f.aspect); set('#peerType', f.type); set('#peerSource', f.source);
    const search = $('#peerSearch'); if (search && search.value !== f.q) search.value = f.q;
  }
  function updatePeers(el) {
    const s = state.sort.peers;
    const rows = sortBy(filteredPeers(), PEER_SORT[s.key] || PEER_SORT.rank, s.dir);
    const shown = rows.slice(0, state.peersLimit);
    const conn = rows.filter(p => p.connected);
    const tenures = conn.map(p => p.tenureSec).filter(t => t != null).sort((a, b) => a - b);
    $('#peerCount', el).innerHTML = '<b>' + fmtNum(rows.length) + '</b> of ' + fmtNum(state.peers.length) + ' peers' +
      (rows.length ? ' · <b>' + rows.filter(p => p.greyListed).length + '</b> grey-listed' : '') + (tenures.length ? ' · median tenure <b>' + fmtDur(median(tenures)) + '</b>' : '');
    $('#peerTable', el).innerHTML =
      '<table class="tbl"><thead><tr>' + th('Peer', 'peer', s) + th('Agent', 'agent', s) + th('State', 'state', s) + th('Tenure', 'tenure', s) + th('Strikes', 'strikes', s) +
      th('Grey-list', 'grey', s) + th('Un-greylist in', 'recovery', s) + th('Gossip', 'gossip', s, 'num') + th('Head Δ', 'head', s, 'num') + th('Rej.', 'rejections', s, 'num') + th('Last status', 'status', s) +
      '</tr></thead><tbody>' + (shown.map(peerRow).join('') || '<tr><td colspan="11"><div class="empty">No peers match these filters</div></td></tr>') + '</tbody></table>' +
      (rows.length > shown.length ? '<div class="table-foot"><span>Showing ' + shown.length + ' of ' + rows.length + '</span><button class="btn small" id="peerMore" type="button">Show all</button></div>' : '');
  }
  function peerRow(p) {
    return '<tr class="row' + (state.selected === p.id ? ' sel' : '') + '" data-peer="' + esc(p.id) + '">' +
      '<td><span class="peer-cell">' + agentDot(p.agentType) + '<span class="mono" title="' + esc(p.id) + '">' + esc(p.short) + '</span>' + (p.trusted ? badge('trusted', 'b-accent') : '') + '</span></td>' +
      '<td class="agent-cell" title="' + esc(p.agent) + '">' + (p.agent ? esc(p.agent) : '<span class="faint">unknown</span>') + '</td>' +
      '<td><span class="badges">' + stateBadge(p.state) + dirBadge(p.direction) + '</span></td>' +
      '<td>' + (p.connected && p.connectedAt ? liveSince(p.connectedAt) : '<span class="faint">—</span>') + '</td>' +
      '<td>' + strikeMeter(p) + '</td>' +
      '<td>' + greyCell(p) + '</td>' +
      '<td>' + recoveryCell(p) + '</td>' +
      '<td class="num">' + gossipCell(p.gossipScore) + '</td>' +
      '<td class="num">' + deltaCell(p.headDelta) + '</td>' +
      '<td class="num">' + (p.rejectionsCount ? p.rejectionsCount : '<span class="faint">0</span>') + '</td>' +
      '<td>' + statusCell(p) + '</td></tr>';
  }

  // ------------------------------------------------------------------ agents
  const AGENT_SORT = { agent: a => a.agent, agentType: a => a.agentType, peerCount: a => a.peerCount, connected: a => a.connected, greyCount: a => a.greyCount, strikesTotal: a => a.strikesTotal, rejections: a => a.rejections };
  function mountAgents(el) {
    const f = state.filters.agents;
    el.innerHTML =
      '<div class="toolbar"><div class="toolbar-row">' +
      '<input class="input grow" id="agentSearch" placeholder="Search agent string…" value="' + esc(f.q) + '">' +
      '<div class="filter"><span>Client</span><select class="select" id="agentType"><option value="">All</option>' + AGENT_TYPES.map(t => '<option value="' + t + '"' + (f.type === t ? ' selected' : '') + '>' + t + '</option>').join('') + '</select></div>' +
      '<label class="switch"><input type="checkbox" id="agentConnected"' + (f.connectedOnly ? ' checked' : '') + '><span class="switch-track"></span><span class="switch-label">Connected only</span></label>' +
      '<span class="result-count" id="agentCount"></span>' +
      '</div></div>' +
      '<div id="agentTypes" class="grid kpis" style="margin-bottom:14px"></div>' +
      '<div class="card"><div class="card-h"><h2>Agents</h2><span class="sub">every distinct agent string seen; click a row to list its peers</span></div><div class="card-b flush"><div class="table-wrap" id="agentTable"></div></div></div>';
    $('#agentSearch', el).addEventListener('input', e => { f.q = e.target.value; updateAgents(el); });
    $('#agentType', el).addEventListener('change', e => { f.type = e.target.value; updateAgents(el); });
    $('#agentConnected', el).addEventListener('change', e => { f.connectedOnly = e.target.checked; updateAgents(el); });
    $('#agentTypes', el).addEventListener('click', e => { const c = e.target.closest('[data-type]'); if (!c) return; f.type = f.type === c.dataset.type ? '' : c.dataset.type; $('#agentType', el).value = f.type; updateAgents(el); });
    $('#agentTable', el).addEventListener('click', e => {
      const h = e.target.closest('th[data-sort]');
      if (h) { toggleSort(state.sort.agents, h.dataset.sort, h.dataset.sort === 'agent' || h.dataset.sort === 'agentType' ? 'asc' : 'desc'); updateAgents(el); return; }
      const row = e.target.closest('tr[data-agent]');
      if (row) { const pf = state.filters.peers; applyPreset('all'); pf.q = row.dataset.agent === 'unknown' ? '' : row.dataset.agent; pf.type = row.dataset.agenttype; pf.preset = 'custom'; setView('peers'); }
    });
  }
  function updateAgents(el) {
    const f = state.filters.agents, q = f.q.trim().toLowerCase(), s = state.sort.agents;
    const byType = new Map();
    for (const p of state.peers) {
      const t = byType.get(p.agentType) || { peers: 0, connected: 0, grey: 0, strikes: 0 };
      t.peers++; if (p.connected) t.connected++; if (p.greyListed) t.grey++; t.strikes += p.standing;
      byType.set(p.agentType, t);
    }
    $('#agentTypes', el).innerHTML = AGENT_TYPES.filter(t => byType.get(t)).map(t => {
      const v = byType.get(t);
      return '<div class="card kpi" data-type="' + t + '" style="cursor:pointer;' + (f.type === t ? 'border-color:var(--accent)' : '') + '"><div class="kpi-l" style="display:flex;align-items:center;gap:6px">' + agentDot(t) + esc(t) + '</div>' +
        '<div class="kpi-v">' + v.connected + '<small>connected</small></div><div class="kpi-s"><span>' + v.peers + ' known</span><span>' + v.grey + ' grey-listed</span><span>' + v.strikes + ' standing strikes</span></div></div>';
    }).join('') || '<div class="empty">No peers yet</div>';

    let rows = state.agents.filter(a => (!f.type || a.agentType === f.type) && (!f.connectedOnly || a.connected > 0) && (!q || a.agent.toLowerCase().includes(q)));
    rows = sortBy(rows, AGENT_SORT[s.key] || AGENT_SORT.peerCount, s.dir);
    $('#agentCount', el).innerHTML = '<b>' + rows.length + '</b> of ' + state.agents.length + ' agents';
    $('#agentTable', el).innerHTML =
      '<table class="tbl"><thead><tr>' + th('Agent', 'agent', s) + th('Client', 'agentType', s) + th('Peers', 'peerCount', s, 'num') + th('Connected', 'connected', s, 'num') + th('Grey-listed', 'greyCount', s, 'num') + th('Strikes by source', 'strikesTotal', s) + th('Rejections', 'rejections', s, 'num') +
      '</tr></thead><tbody>' + (rows.map(a =>
        '<tr class="row" data-agent="' + esc(a.agent) + '" data-agenttype="' + esc(a.agentType) + '">' +
        '<td class="trunc" title="' + esc(a.agent) + '">' + esc(a.agent) + '</td>' +
        '<td><span class="peer-cell">' + agentDot(a.agentType) + esc(a.agentType) + '</span></td>' +
        '<td class="num">' + a.peerCount + '</td><td class="num">' + (a.connected || '<span class="faint">0</span>') + '</td>' +
        '<td class="num">' + (a.greyCount ? '<span class="score-bad">' + a.greyCount + '</span>' : '<span class="faint">0</span>') + '</td>' +
        '<td><span class="peer-cell">' + stackBar(a.bySource, a.strikesTotal) + '<span class="num muted">' + (a.strikesTotal || '') + '</span></span></td>' +
        '<td class="num">' + (a.rejections || '<span class="faint">0</span>') + '</td></tr>').join('') || '<tr><td colspan="7"><div class="empty">No agents match</div></td></tr>') + '</tbody></table>';
  }

  // ------------------------------------------------------------------ gossip rejections
  const GROUPS = [['topic', 'Topic'], ['reason', 'Reason'], ['agent', 'Agent'], ['agent_type', 'Client'], ['peer', 'Peer']];
  function mountGossip(el) {
    const f = state.filters.gossip;
    el.innerHTML =
      '<div id="gossipKpis" class="grid kpis" style="margin-bottom:14px"></div>' +
      '<div class="toolbar"><div class="toolbar-row">' +
      '<div class="filter"><span>Group by</span><div class="segmented" id="gossipGroup">' + GROUPS.map(g => '<button type="button" data-group="' + g[0] + '">' + g[1] + '</button>').join('') + '</div></div>' +
      '<div class="filter"><span>Since</span><div class="segmented" id="gossipSince"><button type="button" data-since="15m">15m</button><button type="button" data-since="1h">1h</button><button type="button" data-since="24h">24h</button><button type="button" data-since="all">All</button></div></div>' +
      '<span class="result-count" id="gossipCount"></span>' +
      '</div><div class="toolbar-row">' +
      '<input class="input" id="gossipTopic" placeholder="Topic contains…" value="' + esc(f.topic) + '">' +
      '<input class="input" id="gossipAgent" placeholder="Agent contains…" value="' + esc(f.agent) + '">' +
      '<input class="input mono" id="gossipPeer" placeholder="Peer id contains…" value="' + esc(f.peer) + '">' +
      '<button class="btn ghost small" id="gossipClear" type="button">Reset</button>' +
      '</div></div>' +
      '<div class="grid cols-2" style="margin-bottom:14px"><div class="card"><div class="card-h"><h2 id="gossipChartTitle">Rejections</h2><span class="sub">click a bar to filter</span></div><div class="card-b" id="gossipChart"></div></div>' +
      '<div class="card"><div class="card-h"><h2>What this means</h2></div><div class="card-b muted" style="font-size:13px">A rejection is a gossip message one of our topic validators returned <b>reject</b> for. The store keeps the most recent ' +
      '<span id="gossipCap"></span> per peer purely for observability: rejections do not feed scoring or grey-listing directly, but libp2p lowers the sender\'s gossip score for each invalid delivery, and that score grey-lists below ' +
      '<span id="gossipThresh"></span>. Entries disappear when the peer is pruned from the peer store.</div></div></div>' +
      '<div class="card"><div class="card-h"><h2>Rejection feed</h2><span class="sub">newest first</span></div><div class="card-b flush"><div class="table-wrap" id="gossipTable"></div></div></div>';
    $('#gossipGroup', el).addEventListener('click', e => { const b = e.target.closest('[data-group]'); if (!b) return; f.groupBy = b.dataset.group; updateGossip(el); });
    $('#gossipSince', el).addEventListener('click', e => { const b = e.target.closest('[data-since]'); if (!b) return; f.sincePreset = b.dataset.since; f.sinceMs = { '15m': 9e5, '1h': 36e5, '24h': 864e5, 'all': 0 }[f.sincePreset]; updateGossip(el); });
    $('#gossipTopic', el).addEventListener('input', e => { f.topic = e.target.value; updateGossip(el); });
    $('#gossipAgent', el).addEventListener('input', e => { f.agent = e.target.value; updateGossip(el); });
    $('#gossipPeer', el).addEventListener('input', e => { f.peer = e.target.value; updateGossip(el); });
    $('#gossipClear', el).addEventListener('click', () => { Object.assign(f, { topic: '', agent: '', peer: '', sinceMs: 0, sincePreset: 'all' }); $('#gossipTopic', el).value = ''; $('#gossipAgent', el).value = ''; $('#gossipPeer', el).value = ''; updateGossip(el); });
    $('#gossipChart', el).addEventListener('click', e => {
      const l = e.target.closest('[data-group]'); if (!l) return;
      const v = l.dataset.group;
      if (f.groupBy === 'topic') { f.topic = v; $('#gossipTopic', el).value = v; }
      else if (f.groupBy === 'agent') { f.agent = v === 'unknown' ? '' : v; $('#gossipAgent', el).value = f.agent; }
      else if (f.groupBy === 'peer') { f.peer = v; $('#gossipPeer', el).value = v; }
      else if (f.groupBy === 'agent_type') { f.agent = ''; f.typeKey = v; }
      updateGossip(el);
    });
    $('#gossipTable', el).addEventListener('click', e => { const p = e.target.closest('[data-peer]'); if (p) openPeer(p.dataset.peer); });
  }
  function filteredRejections() {
    const f = state.filters.gossip, now = Date.now();
    const t = f.topic.trim().toLowerCase(), a = f.agent.trim().toLowerCase(), p = f.peer.trim().toLowerCase();
    return state.rejections.filter(r =>
      (!f.sinceMs || r.at >= now - f.sinceMs) && (!t || r.topic.toLowerCase().includes(t)) && (!a || r.agent.toLowerCase().includes(a)) && (!p || r.peerId.toLowerCase().includes(p)));
  }
  function updateGossip(el) {
    const f = state.filters.gossip, cfg = state.cfg;
    $$('#gossipGroup button', el).forEach(b => b.classList.toggle('on', b.dataset.group === f.groupBy));
    $$('#gossipSince button', el).forEach(b => b.classList.toggle('on', b.dataset.since === f.sincePreset));
    $('#gossipCap', el).textContent = fmtNum(cfg.maxRejections);
    $('#gossipThresh', el).textContent = fmtNum(cfg.gossipThreshold);
    const all = state.rejections, rows = filteredRejections();
    const peersAll = new Set(all.map(r => r.peerId)).size;
    const last = all.length ? all[0].at : 0;
    const topTopic = topN(countBy(all.map(r => r.topicShort)), 1)[0];
    const topClient = topN(countBy(all.map(r => r.agentType)), 1)[0];
    $('#gossipKpis', el).innerHTML =
      kpi('Rejections retained', fmtNum(all.length), [peersAll + ' peers', 'cap ' + fmtNum(cfg.maxRejections) + ' per peer'], all.length ? 'warn' : '') +
      kpi('Latest rejection', last ? liveAgo(last) : '<span class="faint">none</span>', [last ? fmtTime(last) : '']) +
      kpi('Top topic', topTopic ? esc(topTopic[0]) : '<span class="faint">—</span>', [topTopic ? topTopic[1] + ' rejections' : '']) +
      kpi('Top client', topClient ? '<span class="peer-cell">' + agentDot(topClient[0]) + esc(topClient[0]) + '</span>' : '<span class="faint">—</span>', [topClient ? topClient[1] + ' rejections' : '']);
    $('#gossipCount', el).innerHTML = '<b>' + fmtNum(rows.length) + '</b> of ' + fmtNum(all.length) + ' rejections';
    const keyOf = r => f.groupBy === 'topic' ? r.topic : f.groupBy === 'reason' ? r.reason : f.groupBy === 'agent' ? (r.agent || 'unknown') : f.groupBy === 'agent_type' ? r.agentType : r.peerId;
    const labelOf = k => f.groupBy === 'topic' ? shortTopic(k) : f.groupBy === 'peer' ? short(k) : (k.length > 70 ? k.slice(0, 68) + '…' : k);
    const groups = topN(countBy(rows.map(keyOf)), 15);
    $('#gossipChartTitle', el).textContent = 'Rejections by ' + GROUPS.find(g => g[0] === f.groupBy)[1].toLowerCase();
    $('#gossipChart', el).innerHTML = bars(groups.map(([k, v]) => ({ label: labelOf(k), key: k, title: k, value: v, color: f.groupBy === 'agent_type' ? agentColor(k) : 'var(--accent-2)' })), { dataKey: 'group', wide: true, total: rows.length, empty: 'No rejections in this window' });
    const shown = rows.slice(0, 500);
    $('#gossipTable', el).innerHTML = '<table class="tbl"><thead><tr><th>When</th><th>Peer</th><th>Client</th><th>Agent</th><th>Topic</th><th>Reason</th></tr></thead><tbody>' +
      (shown.map(r => '<tr><td class="nowrap" title="' + esc(fmtTime(r.at)) + '">' + liveAgo(r.at) + '</td><td>' + peerLink(r.peerId) + '</td><td><span class="peer-cell">' + agentDot(r.agentType) + esc(r.agentType) + '</span></td>' +
        '<td class="agent-cell" title="' + esc(r.agent) + '">' + esc(r.agent || '–') + '</td><td title="' + esc(r.topic) + '">' + badge(r.topicShort, 'b-muted') + '</td><td class="trunc" style="white-space:normal;max-width:520px" title="' + esc(r.reason) + '">' + esc(r.reason) + '</td></tr>').join('') ||
        '<tr><td colspan="6"><div class="empty">No rejections match</div></td></tr>') + '</tbody></table>' +
      (rows.length > shown.length ? '<div class="table-foot"><span>Showing ' + shown.length + ' of ' + rows.length + '</span></div>' : '');
  }

  // ------------------------------------------------------------------ chain state
  const CHAIN_SORT = { peer: p => p.id, agent: p => p.agent || null, state: p => STATES.indexOf(p.state), headSlot: p => p.headSlot, headDelta: p => p.headDelta, finalizedEpoch: p => p.finalizedEpoch, earliestSlot: p => p.earliestSlot, statusAt: p => p.statusAt || null, error: p => p.validationError || null };
  function mountChain(el) {
    const f = state.filters.chain;
    el.innerHTML =
      '<div id="chainKpis" class="grid kpis" style="margin-bottom:14px"></div>' +
      '<div class="grid cols-3" id="chainCards" style="margin-bottom:14px"></div>' +
      '<div class="toolbar"><div class="toolbar-row">' +
      '<input class="input grow mono" id="chainSearch" placeholder="Search peer id or agent…" value="' + esc(f.q) + '">' +
      '<label class="switch"><input type="checkbox" id="chainConnected"' + (f.connectedOnly ? ' checked' : '') + '><span class="switch-track"></span><span class="switch-label">Connected only</span></label>' +
      '<span class="result-count" id="chainCount"></span></div></div>' +
      '<div class="card"><div class="card-h"><h2>Peer chain views</h2><span class="sub">from each peer\'s last validated status exchange</span></div><div class="card-b flush"><div class="table-wrap" id="chainTable"></div></div></div>';
    $('#chainSearch', el).addEventListener('input', e => { f.q = e.target.value; updateChain(el); });
    $('#chainConnected', el).addEventListener('change', e => { f.connectedOnly = e.target.checked; updateChain(el); });
    $('#chainTable', el).addEventListener('click', e => {
      const h = e.target.closest('th[data-sort]');
      if (h) { toggleSort(state.sort.chain, h.dataset.sort, h.dataset.sort === 'peer' || h.dataset.sort === 'agent' ? 'asc' : 'desc'); updateChain(el); return; }
      const row = e.target.closest('tr[data-peer]'); if (row) openPeer(row.dataset.peer);
    });
    $('#chainCards', el).addEventListener('click', e => { const p = e.target.closest('[data-peer]'); if (p) openPeer(p.dataset.peer); });
  }
  function updateChain(el) {
    const f = state.filters.chain, s = state.sort.chain, node = state.node, cfg = state.cfg, spe = state.spec.slotsPerEpoch;
    const conn = state.peers.filter(p => p.connected);
    const withChain = conn.filter(p => p.chain);
    const deltas = withChain.map(p => p.headDelta).filter(d => d != null);
    const buckets = [
      { label: '≥ 2 epochs ahead', test: d => d >= 2 * spe, cls: 'accent2' }, { label: 'ahead', test: d => d > 1 && d < 2 * spe, cls: 'info' },
      { label: 'at head (±1)', test: d => Math.abs(d) <= 1, cls: 'ok' }, { label: '≤ 1 epoch behind', test: d => d < -1 && d >= -spe, cls: '' },
      { label: '≤ 10 epochs behind', test: d => d < -spe && d >= -10 * spe, cls: 'warn' }, { label: '> 10 epochs behind', test: d => d < -10 * spe, cls: 'bad' },
    ].map(b => ({ label: b.label, value: deltas.filter(b.test).length, cls: b.cls }));
    const finCounts = topN(countBy(withChain.map(p => p.finalizedEpoch).filter(e => e != null)), 6);
    const digest = (withChain.find(p => p.forkDigest) || {}).forkDigest || (state.peers.find(p => p.forkDigest) || {}).forkDigest || '';
    const errPeers = state.peers.filter(p => p.validationError).sort((a, b) => b.statusAt - a.statusAt);
    const earliest = withChain.map(p => p.earliestSlot).filter(e => e != null).sort((a, b) => a - b);
    const highestPeer = withChain.reduce((m, p) => p.headSlot != null && p.headSlot > m ? p.headSlot : m, 0);
    $('#chainKpis', el).innerHTML =
      kpi('Our head', fmtNum(node.headSlot), ['epoch ' + fmtNum(node.headSlot != null ? Math.floor(node.headSlot / spe) : null), node.isSyncing ? badge('syncing', 'b-warn') : badge('in sync', 'b-ok'), node.isOptimistic ? badge('optimistic', 'b-warn') : '', node.elOffline ? badge('EL offline', 'b-bad') : ''], 'accent') +
      kpi('Our finalized epoch', fmtNum(node.finalizedEpoch), [node.finalizedRoot ? '<span class="mono">' + esc(node.finalizedRoot.slice(0, 12)) + '…</span>' : '']) +
      kpi("Peers' highest head", fmtNum(highestPeer || cfg.highestKnownHeadSlot), [node.headSlot != null && highestPeer ? (highestPeer - node.headSlot > 0 ? '+' : '') + (highestPeer - node.headSlot) + ' slots vs ours' : '', 'scorer high-water ' + fmtNum(cfg.highestKnownHeadSlot)], highestPeer > (node.headSlot || 0) + 2 ? 'warn' : '') +
      kpi('Peers with chain state', fmtNum(withChain.length), ['of ' + conn.length + ' connected', buckets[2].value + ' at head', (buckets[3].value + buckets[4].value + buckets[5].value) + ' behind']) +
      kpi('Status errors', fmtNum(errPeers.length), [errPeers.filter(p => p.connected).length + ' connected', 'terminal errors expire after ' + esc(cfg.statusTTL || '–')], errPeers.length ? 'warn' : '');
    $('#chainCards', el).innerHTML =
      card('Peer heads relative to ours', columns(buckets) + '<div class="inline-note">Δ = peer head slot − our head slot, from the peer\'s last validated status (refreshed about twice per epoch).</div>', withChain.length + ' peers') +
      card('Finalized epoch agreement', bars(finCounts.map(([e, n]) => ({ label: 'epoch ' + fmtNum(e) + (e === node.finalizedEpoch ? ' (ours)' : ''), value: n, color: e === node.finalizedEpoch ? 'var(--ok)' : e > (node.finalizedEpoch || 0) ? 'var(--accent-2)' : 'var(--warn)' })), { total: withChain.length, empty: 'No peer statuses yet' }) +
        '<dl class="kv" style="margin-top:12px"><dt>Fork digest</dt><dd class="mono">' + esc(digest || '–') + '</dd><dt>Earliest slot</dt><dd>peers can serve back to slot ' + fmtNum(earliest.length ? earliest[0] : null) + ' (median ' + fmtNum(median(earliest)) + ', latest ' + fmtNum(earliest.length ? earliest[earliest.length - 1] : null) + ')</dd></dl>', 'validated statuses share our digest') +
      card('Status validation errors', errPeers.length ? '<div class="list">' + errPeers.slice(0, 10).map(p =>
        '<div class="list-row"><span class="list-time">' + liveAgo(p.statusAt) + '</span><span class="list-main">' + agentDot(p.agentType) + peerLink(p.id) + stateBadge(p.state) + '<span class="reason">' + esc(p.validationError) + '</span></span></div>').join('') + '</div>' +
        (errPeers.length > 10 ? '<div class="inline-note">' + (errPeers.length - 10) + ' more; filter the Peers view by aspect “Peer status”.</div>' : '') : '<div class="empty">No peer failed status validation</div>', 'latest first');
    const q = f.q.trim().toLowerCase();
    let rows = state.peers.filter(p => p.chain && (!f.connectedOnly || p.connected) && (!q || p.id.toLowerCase().includes(q) || p.agent.toLowerCase().includes(q)));
    rows = sortBy(rows, CHAIN_SORT[s.key] || CHAIN_SORT.headSlot, s.dir);
    $('#chainCount', el).innerHTML = '<b>' + rows.length + '</b> peers with chain state';
    $('#chainTable', el).innerHTML =
      '<table class="tbl"><thead><tr>' + th('Peer', 'peer', s) + th('Agent', 'agent', s) + th('State', 'state', s) + th('Head slot', 'headSlot', s, 'num') + th('Δ', 'headDelta', s, 'num') + th('Finalized', 'finalizedEpoch', s, 'num') + th('Earliest slot', 'earliestSlot', s, 'num') + th('Status', 'statusAt', s) + th('Validation', 'error', s) +
      '</tr></thead><tbody>' + (rows.slice(0, 500).map(p =>
        '<tr class="row" data-peer="' + esc(p.id) + '"><td><span class="peer-cell">' + agentDot(p.agentType) + '<span class="mono" title="' + esc(p.id) + '">' + esc(p.short) + '</span></span></td>' +
        '<td class="agent-cell" title="' + esc(p.agent) + '">' + (p.agent ? esc(p.agent) : '<span class="faint">unknown</span>') + '</td><td><span class="badges">' + stateBadge(p.state) + dirBadge(p.direction) + '</span></td>' +
        '<td class="num">' + fmtNum(p.headSlot) + '</td><td class="num">' + deltaCell(p.headDelta) + '</td><td class="num">' + fmtNum(p.finalizedEpoch) + (p.finalizedEpoch != null && node.finalizedEpoch != null && p.finalizedEpoch !== node.finalizedEpoch ? ' <span class="faint">(' + (p.finalizedEpoch > node.finalizedEpoch ? '+' : '') + (p.finalizedEpoch - node.finalizedEpoch) + ')</span>' : '') + '</td>' +
        '<td class="num">' + fmtNum(p.earliestSlot) + '</td><td>' + liveAgo(p.statusAt) + '</td><td>' + (p.validationError ? badge('invalid', 'b-bad', p.validationError) + ' <span class="muted" style="font-size:12px">' + esc(p.validationError.length > 50 ? p.validationError.slice(0, 48) + '…' : p.validationError) + '</span>' : badge('ok', 'b-ok')) + '</td></tr>').join('') ||
        '<tr><td colspan="9"><div class="empty">No peers match</div></td></tr>') + '</tbody></table>';
  }

  // ------------------------------------------------------------------ drawer
  async function openPeer(id) {
    if (!id) return;
    state.selected = id; state.selectedDetail = null; state.selectedEth = null; state.selectedError = null; state.selectedLoading = true;
    $$('tr.row.sel').forEach(r => r.classList.remove('sel'));
    $$('tr[data-peer="' + CSS.escape(id) + '"]').forEach(r => r.classList.add('sel'));
    renderDrawer();
    await loadPeerDetail(id);
  }
  async function loadPeerDetail(id) {
    const [d, e] = await Promise.allSettled([api('/prysm/v1/node/peers/' + encodeURIComponent(id) + '/scoring?include_topic_scores=true'), api('/eth/v1/node/peers/' + encodeURIComponent(id))]);
    if (state.selected !== id) return;
    state.selectedLoading = false;
    if (d.status === 'fulfilled' && d.value && d.value.data) { state.selectedDetail = enrichPeer(d.value.data, peerCtx()); state.selectedError = null; }
    else state.selectedError = d.reason ? d.reason.message : 'no data';
    if (e.status === 'fulfilled' && e.value) state.selectedEth = e.value.data || null;
    renderDrawer();
  }
  function closeDrawer() {
    state.selected = null; state.selectedDetail = null;
    $('#drawer').hidden = true; $('#backdrop').hidden = true;
    $$('tr.row.sel').forEach(r => r.classList.remove('sel'));
  }
  function renderDrawer() {
    const id = state.selected; if (!id) return;
    const p = state.selectedDetail || state.peersById.get(id);
    const dr = $('#drawer'); dr.hidden = false; $('#backdrop').hidden = false;
    if (!p) {
      dr.innerHTML = '<div class="drawer-h"><div class="title"><h2>Peer</h2><div class="pid">' + esc(id) + '</div></div><button class="btn icon" id="drawerClose" type="button" aria-label="Close">✕</button></div><div class="drawer-b">' +
        (state.selectedLoading ? '<div class="empty"><span class="loading-row"></span> Loading…</div>' : '<div class="verdict warn"><div class="v-title">Not found</div><div class="v-body">' + esc(state.selectedError || 'The node has no record of this peer.') + '</div></div>') + '</div>';
      return;
    }
    const eth = state.selectedEth || {};
    const addr = eth.last_seen_p2p_address || p.addr, enr = eth.enr || p.enr;
    const cfg = state.cfg, node = state.node;
    const rejections = p.rejections.slice().sort((a, b) => b.at - a.at);
    const topics = p.topicScores ? Object.entries(p.topicScores).map(([t, v]) => ({ topic: t, short: shortTopic(t), mesh: v.time_in_mesh_ms || 0, first: v.first_message_deliveries || 0, meshDel: v.mesh_message_deliveries || 0, invalid: v.invalid_message_deliveries || 0 }))
      .filter(t => t.mesh || t.first || t.meshDel || t.invalid).sort((a, b) => (b.invalid - a.invalid) || (b.mesh - a.mesh)) : [];
    const verdictHtml = p.greyListed
      ? '<div class="verdict bad"><div class="v-title">' + badge('grey-listed', 'b-bad') + 'The node refuses this peer</div><div class="v-body">' + (p.recoveryEnd != null ? 'Expected to be white-listed in ' + liveCountdown(p.recoveryEnd) + '.' : p.recoveryUnknown ? 'Recovery is not time based (gossip score decay or IP colocation).' : '') + '</div></div>'
      : p.aspects.length && p.exemption
        ? '<div class="verdict warn"><div class="v-title">' + badge('exempt: ' + p.exemption, 'b-accent') + 'Refusal sources fire but the peer is exempt</div></div>'
        : '<div class="verdict ok"><div class="v-title">' + badge('clear', 'b-ok') + 'Not grey-listed</div><div class="v-body">' + (p.standing ? p.standing + ' standing strikes; grey-listed at ' + p.threshold + '.' : 'No standing strikes.') + '</div></div>';
    const aspectRows = p.aspects.map(a => '<div class="verdict' + (p.greyListed ? ' bad' : '') + '"><div class="v-title">' + aspectBadge(a) + (p.recovery[a] ? '<span class="muted" style="font-weight:500">recovery ' + (parseGoDuration(p.recovery[a]) != null ? liveCountdown(state.fetchedAt + parseGoDuration(p.recovery[a]) * 1000) : esc(p.recovery[a])) + '</span>' : '') + '</div><div class="v-body">' + esc(p.details[a]) + '</div></div>').join('');

    dr.innerHTML =
      '<div class="drawer-h"><div class="title"><h2>' + agentDot(p.agentType) + esc(p.agent || 'unknown agent') + '</h2>' +
      '<div class="pid"><span>' + esc(p.id) + '</span>' + copyBtn(p.id) + '</div>' +
      '<div class="badges" style="margin-top:6px">' + stateBadge(p.state) + dirBadge(p.direction) + (p.trusted ? badge('trusted', 'b-accent') : '') + (p.greyListed ? badge('grey-listed', 'b-bad') : '') + badge(p.agentType, 'b-muted') + '</div></div>' +
      '<button class="btn icon" id="drawerClose" type="button" aria-label="Close">✕</button></div>' +
      '<div class="drawer-b">' +
      (state.selectedLoading ? '<div class="inline-note"><span class="loading-row"></span> Loading full details…</div>' : '') +
      (state.selectedError && !state.selectedDetail ? '<div class="verdict warn"><div class="v-body">Detail request failed: ' + esc(state.selectedError) + '. Showing the list entry.</div></div>' : '') +

      '<div class="section"><h3>Connection</h3><dl class="kv">' +
      '<dt>State</dt><dd>' + stateBadge(p.state) + ' ' + dirBadge(p.direction) + '</dd>' +
      '<dt>Tenure</dt><dd>' + (p.connected && p.connectedAt ? liveSince(p.connectedAt) + ' <span class="faint">since ' + esc(fmtTime(p.connectedAt)) + '</span>' : p.connectedAt ? '<span class="muted">last connected ' + esc(fmtTime(p.connectedAt)) + '</span>' : '<span class="faint">never connected</span>') + '</dd>' +
      '<dt>Address</dt><dd class="mono">' + (addr ? esc(addr) + ' ' + copyBtn(addr) : '<span class="faint">unknown</span>') + '</dd>' +
      '<dt>ENR</dt><dd class="mono">' + (enr ? esc(enr.length > 60 ? enr.slice(0, 58) + '…' : enr) + ' ' + copyBtn(enr) : '<span class="faint">none</span>') + '</dd>' +
      '</dl></div>' +

      '<div class="section"><h3>Grey-list verdict</h3>' + verdictHtml + aspectRows + '</div>' +

      '<div class="section"><h3>Strikes <span class="sub">' + p.standing + ' standing · grey-listed at ' + p.threshold + ' · 1 forgiven every ' + esc(cfg.decay || '–') + '</span></h3>' +
      '<div style="margin-bottom:10px">' + strikeMeter(p) + '</div>' +
      (p.history.length ? '<div class="list">' + p.history.slice().reverse().map(h => '<div class="list-row"><span class="list-time" title="' + esc(fmtTime(h.at)) + '">' + liveAgo(h.at) + '</span><span class="list-main">' + sourceBadge(h.source) + '<span class="reason">' + esc(h.reason) + '</span></span></div>').join('') + '</div>' +
        '<div class="inline-note">' + p.history.length + ' retained of up to ' + fmtNum(cfg.historySize) + '; the standing count is what decay and grey-listing act on.</div>' : '<div class="empty">No strikes recorded</div>') + '</div>' +

      '<div class="section"><h3>Chain status <span class="sub">' + (p.status ? liveAgo(p.statusAt) : 'never exchanged') + '</span></h3>' +
      (p.validationError ? '<div class="verdict bad"><div class="v-title">' + badge('validation failed', 'b-bad') + '</div><div class="v-body">' + esc(p.validationError) + '</div></div>' : '') +
      (p.chain ? '<dl class="kv">' +
        '<dt>Head slot</dt><dd>' + fmtNum(p.headSlot) + ' <span class="muted">(' + deltaCell(p.headDelta) + ' vs ours ' + fmtNum(node.headSlot) + ')</span></dd>' +
        '<dt>Head root</dt><dd class="mono">' + esc(p.chain.head_root) + '</dd>' +
        '<dt>Finalized</dt><dd>epoch ' + fmtNum(p.finalizedEpoch) + (node.finalizedEpoch != null && p.finalizedEpoch != null ? ' <span class="muted">(ours ' + fmtNum(node.finalizedEpoch) + ')</span>' : '') + '</dd>' +
        '<dt>Finalized root</dt><dd class="mono">' + esc(p.chain.finalized_root) + '</dd>' +
        '<dt>Earliest slot</dt><dd>' + fmtNum(p.earliestSlot) + '</dd>' +
        '<dt>Fork digest</dt><dd class="mono">' + esc(p.chain.fork_digest) + '</dd>' +
        '</dl>' + (p.validationError ? '<div class="inline-note">Shown is the last status that passed validation.</div>' : '') : (p.status ? '<div class="empty">No parseable status stored</div>' : '<div class="empty">No status exchange yet</div>')) + '</div>' +

      '<div class="section"><h3>Gossip <span class="sub">score ' + gossipCell(p.gossipScore) + ' · penalty ' + fmtScore(p.behaviourPenalty) + ' · grey-listed below ' + fmtNum(cfg.gossipThreshold) + '</span></h3>' +
      (topics.length ? '<table class="tbl"><thead><tr><th>Topic</th><th class="num">Mesh</th><th class="num">First</th><th class="num">Mesh deliv.</th><th class="num">Invalid</th></tr></thead><tbody>' +
        topics.slice(0, 40).map(t => '<tr><td title="' + esc(t.topic) + '">' + esc(t.short) + '</td><td class="num">' + fmtDur(t.mesh / 1000) + '</td><td class="num">' + fmtScore(t.first) + '</td><td class="num">' + fmtScore(t.meshDel) + '</td><td class="num">' + (t.invalid ? '<span class="score-bad">' + fmtScore(t.invalid) + '</span>' : '0') + '</td></tr>').join('') + '</tbody></table>' +
        (topics.length > 40 ? '<div class="inline-note">' + (topics.length - 40) + ' more topics with activity.</div>' : '') : (state.selectedDetail ? '<div class="inline-note">No per-topic gossip activity recorded by libp2p for this peer.</div>' : '')) +
      '<h3 style="margin-top:14px">Rejected messages <span class="sub">' + rejections.length + ' retained</span></h3>' +
      (rejections.length ? '<div class="list">' + rejections.slice(0, 50).map(r => '<div class="list-row"><span class="list-time" title="' + esc(fmtTime(r.at)) + '">' + liveAgo(r.at) + '</span><span class="list-main">' + badge(r.topicShort, 'b-muted', r.topic) + '<span class="reason">' + esc(r.reason) + '</span></span></div>').join('') + '</div>' : '<div class="empty">No rejected gossip messages from this peer</div>') +
      '</div></div>';
  }

  // ------------------------------------------------------------------ views registry & navigation
  const VIEW_IMPL = {
    overview: { mount: () => {}, update: el => renderOverview(el) },
    peers: { mount: mountPeers, update: updatePeers },
    agents: { mount: mountAgents, update: updateAgents },
    gossip: { mount: mountGossip, update: updateGossip },
    chain: { mount: mountChain, update: updateChain },
  };
  function setView(v) {
    if (!VIEWS.includes(v)) v = 'overview';
    state.view = v;
    if (location.hash.replace('#', '') !== v) history.replaceState(null, '', '#' + v);
    render();
    window.scrollTo({ top: 0 });
  }

  // ------------------------------------------------------------------ wiring
  function defaultNodeUrl() {
    if (/^https?:$/.test(location.protocol)) return location.origin;
    return 'http://localhost:3500';
  }
  function setNodeUrl(url) {
    url = (url || '').trim();
    if (!url) url = defaultNodeUrl();
    if (!/^https?:\/\//i.test(url)) url = 'http://' + url;
    state.nodeUrl = url.replace(/\/+$/, '');
    try { localStorage.setItem(STORAGE_URL, state.nodeUrl); } catch (_) { /* storage unavailable */ }
    $('#nodeUrl').value = state.nodeUrl;
  }
  function setTheme(t) {
    document.documentElement.dataset.theme = t;
    try { localStorage.setItem(STORAGE_THEME, t); } catch (_) { /* storage unavailable */ }
  }
  function init() {
    let theme = 'dark', url = '';
    try { theme = localStorage.getItem(STORAGE_THEME) || 'dark'; url = localStorage.getItem(STORAGE_URL) || ''; } catch (_) { /* storage unavailable */ }
    setTheme(theme);
    // ?node=<url> overrides the stored node URL, so a dashboard link can carry its target.
    const paramUrl = new URLSearchParams(location.search).get('node');
    setNodeUrl(paramUrl || url);

    $('#nodeForm').addEventListener('submit', e => { e.preventDefault(); setNodeUrl($('#nodeUrl').value); state.spec.loaded = false; state.error = null; refresh('connect'); });
    $('#refreshBtn').addEventListener('click', () => refresh('manual'));
    $('#autoRefresh').addEventListener('change', e => { state.auto = e.target.checked; schedule(); renderStatus(); });
    $('#themeBtn').addEventListener('click', () => setTheme(document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark'));
    $('#tabs').addEventListener('click', e => { const t = e.target.closest('.tab'); if (t) setView(t.dataset.view); });
    window.addEventListener('hashchange', () => { const v = location.hash.replace('#', ''); if (v && v !== state.view) setView(v); });
    document.addEventListener('visibilitychange', () => { if (!document.hidden && state.auto && state.fetchedAt && Date.now() - state.fetchedAt > POLL_MS) refresh('visible'); });
    document.addEventListener('keydown', e => { if (e.key === 'Escape' && state.selected) closeDrawer(); });
    $('#backdrop').addEventListener('click', closeDrawer);
    $('#drawer').addEventListener('click', e => {
      if (e.target.closest('#drawerClose')) { closeDrawer(); return; }
      const c = e.target.closest('[data-copy]'); if (c) { copyText(c.dataset.copy); c.textContent = '✓'; setTimeout(() => { c.textContent = '⧉'; }, 900); return; }
      const p = e.target.closest('[data-peer]'); if (p) openPeer(p.dataset.peer);
    });
    $('#main').addEventListener('click', e => {
      const c = e.target.closest('[data-copy]'); if (c) { copyText(c.dataset.copy); return; }
      const src = e.target.closest('.bar-label[data-source]'); if (src) { applyPreset('all'); state.filters.peers.source = src.dataset.source; state.filters.peers.preset = 'custom'; setView('peers'); return; }
      const asp = e.target.closest('.bar-label[data-aspect]'); if (asp) { applyPreset('greylisted'); state.filters.peers.aspect = asp.dataset.aspect; state.filters.peers.preset = 'custom'; setView('peers'); return; }
      const pl = e.target.closest('.peer-link[data-peer]'); if (pl) { openPeer(pl.dataset.peer); }
    });

    setInterval(tickLive, 1000);
    setView(location.hash.replace('#', '') || 'overview');
    refresh('init');
  }
  init();
})();
