// 数据层：把 FlClash NAS 后端与 mihomo REST（/api/core）整理成界面使用的状态
(function () {
  const S = {
    ready: false, needLogin: false, running: true, mode: 'rule', host: location.hostname,
    ports: { mixed: 7890 }, allowLan: true, passwordSet: false, coreVersion: '', version: '',
    nodes: [], byName: {}, groups: [], profiles: [], current: '', devices: [],
    conns: [], logs: [], traffic: { up: Array(60).fill(0), down: Array(60).fill(0) }, total: { up: 0, down: 0 },
    settings: null, uptime: 0,
  };
  const GROUP_TYPES = new Set(['Selector', 'URLTest', 'Fallback', 'LoadBalance', 'Relay']);
  const subs = new Set();
  const emit = k => subs.forEach(f => f(k));

  class HttpError extends Error { constructor(status, msg) { super(msg); this.status = status; } }
  async function req(method, path, body) {
    const r = await fetch(path, { method, headers: body ? { 'Content-Type': 'application/json' } : {}, body: body ? JSON.stringify(body) : undefined, credentials: 'same-origin' });
    if (r.status === 401) { S.needLogin = true; emit('login'); throw new HttpError(401, '需要登录'); }
    const text = await r.text();
    let data = null; try { data = text ? JSON.parse(text) : null; } catch (e) { data = { message: text }; }
    if (!r.ok) throw new HttpError(r.status, (data && (data.error || data.message)) || ('请求失败 ' + r.status));
    return data;
  }
  const enc = encodeURIComponent;

  function relTime(iso) {
    const t = new Date(iso).getTime(); if (!t || t < 1e12) return '从未';
    const s = Math.max(0, (Date.now() - t) / 1000);
    if (s < 60) return '刚刚'; if (s < 3600) return Math.floor(s / 60) + ' 分钟前'; if (s < 86400) return Math.floor(s / 3600) + ' 小时前';
    const d = new Date(t); return `${d.getMonth() + 1} 月 ${d.getDate()} 日`;
  }
  const GB = b => +(b / 1073741824).toFixed(1);
  function normProfile(p, current, prev) {
    const d = p.expire ? new Date(p.expire * 1000) : null;
    return {
      id: p.id, name: p.name, url: p.url || null, auto: p.autoHours, nodes: p.nodes, lastError: p.lastError || '',
      used: p.total ? GB(p.upload + p.download) : null, total: p.total ? GB(p.total) : null,
      expire: d ? `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}` : null,
      updated: relTime(p.updatedAt), current: p.id === current, updating: prev ? prev.updating : false, override: p.override || { prependRules: [], excludeFilter: '' },
    };
  }

  async function loadStatus() {
    const st = await req('GET', '/api/status');
    Object.assign(S, { running: st.running, mode: st.mode, host: st.host || location.hostname, allowLan: st.allowLan, passwordSet: st.passwordSet, coreVersion: st.coreVersion, version: st.version, uptime: st.uptime });
    S.ports.mixed = st.mixedPort;
  }
  async function loadProfiles() {
    const r = await req('GET', '/api/profiles');
    const prev = Object.fromEntries(S.profiles.map(p => [p.id, p]));
    S.current = r.current; S.profiles = (r.profiles || []).map(p => normProfile(p, r.current, prev[p.id]));
  }
  async function loadSettings() { S.settings = await req('GET', '/api/settings'); }
  async function loadProxies() {
    const { proxies } = await req('GET', '/api/core/proxies');
    const order = (proxies.GLOBAL && proxies.GLOBAL.all) || Object.keys(proxies);
    const names = order.filter(n => proxies[n] && GROUP_TYPES.has(proxies[n].type));
    if (S.mode === 'global' && proxies.GLOBAL) names.unshift('GLOBAL');
    const groups = names.map(n => {
      const g = proxies[n];
      return { name: n, type: g.type, now: g.now || '', fixed: g.fixed || '', all: g.all || [], note: g.type === 'URLTest' ? '自动选最快' : g.type === 'Fallback' ? '故障时自动切换' : g.type === 'LoadBalance' ? '负载均衡' : '' };
    });
    const seen = new Set(), nodes = [];
    groups.forEach(g => g.all.forEach(n => {
      const p = proxies[n]; if (!p || GROUP_TYPES.has(p.type) || seen.has(n) || ['Direct', 'Reject', 'RejectDrop', 'Pass', 'Compatible'].includes(p.type)) return;
      seen.add(n); const h = (p.history || []).at(-1); const old = S.byName[n];
      nodes.push({ name: n, type: p.type, delay: h ? h.delay : (old ? old.delay : null), testing: old ? old.testing : false });
    }));
    S.groups = groups; S.nodes = nodes; S.byName = Object.fromEntries(nodes.map(n => [n.name, n]));
  }

  let lastTotals = null;
  async function pollConnections() {
    const r = await req('GET', '/api/core/connections');
    const now = { up: r.uploadTotal || 0, down: r.downloadTotal || 0, t: performance.now() };
    if (lastTotals) {
      const dt = Math.max(.25, (now.t - lastTotals.t) / 1000);
      S.traffic.up.push(Math.max(0, (now.up - lastTotals.up) / dt)); S.traffic.down.push(Math.max(0, (now.down - lastTotals.down) / dt));
      S.traffic.up.shift(); S.traffic.down.shift();
    }
    lastTotals = now; S.total = { up: now.up, down: now.down };
    S.conns = (r.connections || []).map(c => {
      const m = c.metadata || {};
      return { id: c.id, host: m.host || m.destinationIP || '?', port: m.destinationPort, net: (m.network || '').toUpperCase(), src: m.sourceIP || '', dev: m.sourceIP || '本机',
        rule: c.rulePayload ? `${c.rule},${c.rulePayload}` : c.rule, chain: (c.chains || []).slice().reverse(), up: c.upload, down: c.download, start: new Date(c.start).getTime() };
    }).sort((a, b) => b.start - a.start);
    const dev = {};
    S.conns.forEach(c => { const d = dev[c.src] || (dev[c.src] = { ip: c.src || '本机', name: c.src || '本机', conns: 0, down: 0 }); d.conns++; d.down += c.down; });
    S.devices = Object.values(dev).sort((a, b) => b.conns - a.conns).slice(0, 8);
  }

  let logAbort = null;
  async function streamLogs() {
    if (logAbort) return;
    logAbort = new AbortController();
    try {
      const level = (S.settings && S.settings.logLevel) || 'info';
      if (level === 'silent') return;
      const r = await fetch('/api/core/logs?level=' + level, { signal: logAbort.signal, credentials: 'same-origin' });
      if (r.status === 401) { S.needLogin = true; emit('login'); return; }
      const reader = r.body.getReader(), dec = new TextDecoder(); let buf = '';
      for (;;) {
        const { value, done } = await reader.read(); if (done) break;
        buf += dec.decode(value, { stream: true });
        let i; while ((i = buf.indexOf('\n')) >= 0) {
          const line = buf.slice(0, i).trim(); buf = buf.slice(i + 1); if (!line) continue;
          try { const l = JSON.parse(line); S.logs.push({ lv: l.type === 'warn' ? 'warning' : l.type, ts: new Date().toTimeString().slice(0, 8), msg: l.payload }); if (S.logs.length > 400) S.logs.shift(); } catch (e) { /* 忽略非 JSON 行 */ }
        }
      }
    } catch (e) { /* 断开后重连 */ }
    finally { logAbort = null; if (!S.needLogin && S.settings && S.settings.logLevel !== 'silent') setTimeout(streamLogs, 1000); }
  }

  async function refreshAll() {
    await Promise.all([loadStatus(), loadProfiles(), loadSettings(), loadProxies()]);
    S.ready = true; S.needLogin = false; emit('all');
  }

  let ticking = false;
  async function tick() {
    if (ticking || S.needLogin || !S.ready) return; ticking = true;
    try { await pollConnections(); S.uptime++; emit('tick'); } catch (e) { /* 下一秒重试 */ } finally { ticking = false; }
  }
  setInterval(tick, 1000);
  setInterval(() => { if (S.ready && !S.needLogin) loadProxies().then(() => emit('proxies')).catch(() => {}); }, 15000);
  setInterval(() => { if (S.ready && !S.needLogin) loadProfiles().then(() => emit('profile')).catch(() => {}); }, 60000);

  const M = {
    S, HttpError,
    on(f) { subs.add(f); },
    async boot() { try { await refreshAll(); streamLogs(); } catch (e) { if (e.status !== 401) throw e; } },
    async login(password) { await req('POST', '/api/login', { password }); S.needLogin = false; await M.boot(); },
    async logout() { await req('POST', '/api/logout'); location.reload(); },
    fmtB(b) { if (b == null) return '—'; const u = ['B', 'KB', 'MB', 'GB', 'TB']; let i = 0; while (b >= 1024 && i < 4) { b /= 1024; i++; } return (b >= 100 || i === 0 ? b.toFixed(0) : b.toFixed(1)) + ' ' + u[i]; },
    fmtRate(b) { return M.fmtB(b) + '/s'; },
    latClass(d) { if (!d) return 'idle'; return d < 150 ? 'good' : d < 300 ? 'warn' : 'bad'; },
    latText(d) { if (d === 0) return '超时'; if (d == null) return '未测'; return d + ' ms'; },
    fmtUp(s) { const d = Math.floor(s / 86400), h = Math.floor(s % 86400 / 3600), m = Math.floor(s % 3600 / 60); return d ? `${d} 天 ${h} 小时` : h ? `${h} 小时 ${m} 分` : `${m} 分钟`; },
    dur(ms) { const s = Math.max(0, Math.floor(ms / 1000)); return s < 60 ? s + ' 秒' : s < 3600 ? Math.floor(s / 60) + ' 分' : Math.floor(s / 3600) + ' 小时'; },
    daysLeft(d) { return Math.ceil((new Date(d + 'T23:59:59') - Date.now()) / 864e5); },
    proxyUrl() { return `http://${S.host}:${S.ports.mixed}`; },
    chainOf(group) {
      const chain = [group]; let cur = group;
      for (let i = 0; i < 5; i++) { const g = S.groups.find(x => x.name === cur); if (!g || !g.now) break; chain.push(g.now); cur = g.now; }
      return chain;
    },
    async setMode(m) { await req('POST', '/api/mode', { mode: m }); S.mode = m; await loadProxies(); emit('mode'); },
    async toggleRun() { await req('POST', '/api/running', { running: !S.running }); S.running = !S.running; emit('run'); },
    async select(group, name) { await req('PUT', `/api/core/proxies/${enc(group)}`, { name }); await loadProxies(); emit('select'); },
    async unfix(group) { await req('DELETE', `/api/core/proxies/${enc(group)}`); await loadProxies(); emit('select'); },
    test(names) {
      const url = enc((S.settings && S.settings.testUrl) || 'https://www.gstatic.com/generate_204');
      let i = 0; const queue = names.filter(n => S.byName[n]);
      queue.forEach(n => { S.byName[n].testing = true; }); emit('delay');
      const worker = async () => {
        while (i < queue.length) {
          const n = queue[i++], node = S.byName[n];
          try { const r = await req('GET', `/api/core/proxies/${enc(n)}/delay?timeout=5000&url=${url}`); node.delay = r.delay || 0; } catch (e) { if (e.status === 401) return; node.delay = 0; }
          node.testing = false; emit('delay');
        }
      };
      return Promise.all(Array.from({ length: 6 }, worker));
    },
    async updateProfile(id) {
      const p = S.profiles.find(x => x.id === id); if (p) { p.updating = true; emit('profile'); }
      try { await req('POST', `/api/profiles/${id}/update`); } finally { if (p) p.updating = false; await loadProfiles(); if (id === S.current) await loadProxies(); emit('profile'); }
    },
    async useProfile(id) { await req('POST', `/api/profiles/${id}/use`); await Promise.all([loadProfiles(), loadProxies(), loadStatus()]); emit('profile'); },
    async addProfile(body) { await req('POST', '/api/profiles', body); await Promise.all([loadProfiles(), loadProxies(), loadStatus()]); emit('profile'); },
    async removeProfile(id) { await req('DELETE', `/api/profiles/${id}`); await loadProfiles(); emit('profile'); },
    async patchProfile(id, body) { await req('PATCH', `/api/profiles/${id}`, body); await Promise.all([loadProfiles(), loadProxies()]); emit('profile'); },
    async saveSettings(s) {
      const prevLevel = S.settings && S.settings.logLevel;
      await req('PUT', '/api/settings', s); await Promise.all([loadSettings(), loadStatus(), loadProxies()]);
      if (logAbort && S.settings.logLevel !== prevLevel) logAbort.abort();
      else if (!logAbort) streamLogs();
      emit('settings');
    },
    async setPassword(pw) { await req('POST', '/api/password', { password: pw }); await loadStatus(); emit('settings'); },
    async updateGeo() { await req('POST', '/api/core/configs/geo', {}); },
    async closeConn(id) { await req('DELETE', `/api/core/connections/${enc(id)}`); S.conns = S.conns.filter(c => c.id !== id); emit('tick'); },
    async closeAll() { await req('DELETE', '/api/core/connections'); S.conns = []; emit('tick'); },
    copy(text, done) {
      const fallback = () => {
        const ta = document.createElement('textarea'); ta.value = text; ta.style.position = 'fixed'; ta.style.opacity = '0'; document.body.appendChild(ta); ta.select();
        let ok = false; try { ok = document.execCommand('copy'); } catch (e) { ok = false; } ta.remove(); done && done(ok);
      };
      if (navigator.clipboard && window.isSecureContext) navigator.clipboard.writeText(text).then(() => done && done(true), fallback); else fallback();
    },
  };
  window.M = M;
})();
