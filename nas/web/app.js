(function () {
  const { S } = M, $ = s => document.querySelector(s), view = $('#view');
  const esc = s => String(s == null ? '' : s).replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
  const VIEWS = ['overview', 'proxies', 'profiles', 'connections', 'logs', 'config'];
  const modeName = { rule: '规则', global: '全局', direct: '直连' };
  let cur = 'overview', connQ = '', logLv = 'all', logPaused = false, drawer = null, busy = false, tt;
  const toast = m => { const t = $('#toast'); t.textContent = m; t.classList.add('on'); clearTimeout(tt); tt = setTimeout(() => t.classList.remove('on'), 2600); };
  const copy = (txt, msg) => M.copy(txt, ok => toast(ok ? msg : '复制失败，请手动选中复制'));
  const lat = (d, t) => t ? '<span class="spin" aria-label="测速中"></span>' : `<span class="lat ${M.latClass(d)}">${M.latText(d)}</span>`;
  const mainGroup = () => S.groups.find(g => g.type === 'Selector') || S.groups[0];
  async function act(fn, okMsg) {
    try { await fn(); if (okMsg) toast(okMsg); } catch (e) { if (e.status !== 401) toast(e.message || '操作失败'); }
  }

  function chart() {
    const W = 600, H = 150, P = 4, all = S.traffic.down.concat(S.traffic.up), mx = Math.max(...all, 1024) * 1.15;
    const pts = a => a.map((v, i) => `${(i / (a.length - 1) * W).toFixed(1)},${(H - P - v / mx * (H - 2 * P)).toFixed(1)}`).join(' ');
    const grid = [.25, .5, .75].map(f => `<line x1="0" x2="${W}" y1="${H * f}" y2="${H * f}" stroke="var(--line)"/>`).join('');
    const d = pts(S.traffic.down), u = pts(S.traffic.up), last = d.split(' ').at(-1).split(',');
    return `<svg class="chart" viewBox="0 0 ${W} ${H}" preserveAspectRatio="none" role="img" aria-label="近 60 秒流量，峰值 ${M.fmtRate(mx / 1.15)}">
      ${grid}<polygon points="0,${H} ${d} ${W},${H}" fill="var(--accent)" fill-opacity=".1"/>
      <polyline points="${d}" fill="none" stroke="var(--accent)" stroke-width="2" vector-effect="non-scaling-stroke"/>
      <polyline points="${u}" fill="none" stroke="var(--mute)" stroke-width="1.5" vector-effect="non-scaling-stroke"/>
      <circle cx="${last[0]}" cy="${last[1]}" r="3.5" fill="var(--accent)"/></svg>`;
  }

  const R = {
    overview() {
      const g = mainGroup(), end = g ? M.chainOf(g.name).at(-1) : null, node = end && S.byName[end], p = S.profiles.find(x => x.current);
      return `<div class="head"><div><h1>概览</h1><p class="sub">${S.running ? '内核运行中，局域网设备可通过下方地址上网' : '内核已暂停，代理端口不再转发流量'}</p></div>
        <div style="display:flex;gap:12px;align-items:center"><span class="label">内核</span><button class="switch" role="switch" aria-checked="${S.running}" data-act="run" aria-label="启停内核"></button></div></div>
      ${!S.passwordSet ? '<div class="card notice">还没有设置访问密码，同一局域网里的任何人都能打开这个页面。<a href="#config">去设置</a></div>' : ''}
      <div class="grid g4">
        <div class="card"><div class="label">下载</div><div class="big num" data-live="down"></div></div>
        <div class="card"><div class="label">上传</div><div class="big num" data-live="up"></div></div>
        <div class="card"><div class="label">本次累计 ↓ / ↑</div><div class="big num" style="font-size:20px" data-live="total"></div></div>
        <div class="card"><div class="label">活动连接</div><div class="big num" data-live="conns"></div></div>
      </div>
      <div class="grid g-hero">
        <div class="card"><div class="card-h"><h2>近 60 秒流量</h2><div class="legend"><span><i style="background:var(--accent)"></i>下载</span><span><i style="background:var(--mute)"></i>上传</span></div></div><div data-live="chart"></div></div>
        <div class="card" style="display:flex;flex-direction:column;gap:14px">
          <h2>代理地址</h2>
          <div class="addr"><code>${M.proxyUrl()}</code><button class="btn sm" data-copy="${M.proxyUrl()}">复制</button></div>
          <div class="addr"><code>socks5://${S.host}:${S.ports.mixed}</code><button class="btn sm ghost" data-copy="socks5://${S.host}:${S.ports.mixed}">复制</button></div>
          <div class="label">终端里用：</div>
          <div class="addr"><code>export https_proxy=${M.proxyUrl()} http_proxy=${M.proxyUrl()}</code><button class="btn sm ghost" data-copy="export https_proxy=${M.proxyUrl()} http_proxy=${M.proxyUrl()}">复制</button></div>
          ${S.allowLan ? '' : '<div class="label" style="color:var(--warn)">未允许局域网连接，只有 NAS 本机能用</div>'}
        </div>
      </div>
      <div class="grid g2">
        <div class="card"><div class="card-h"><h2>出站模式</h2><div class="seg" role="group" aria-label="出站模式">
          ${Object.entries(modeName).map(([k, v]) => `<button aria-pressed="${S.mode === k}" data-mode="${k}">${v}</button>`).join('')}</div></div>
          <div class="rows">
            <div class="row"><div class="grow"><div class="label">${g ? esc(g.name) : '策略组'}</div><div class="t" style="font-weight:600">${g ? esc(g.now) : '还没有订阅'}</div></div>${node ? lat(node.delay, node.testing) : ''}<a class="btn sm ghost" href="#proxies">切换</a></div>
            <div class="row"><div class="grow"><div class="label">当前订阅</div><div class="t" style="font-weight:600">${p ? esc(p.name) : '未添加'}</div></div>${p && p.total ? `<span class="pill num">${p.used} / ${p.total} GB</span>` : ''}<a class="btn sm ghost" href="#profiles">${p ? '管理' : '添加'}</a></div>
          </div></div>
        <div class="card"><div class="card-h"><h2>局域网设备</h2><span class="label">按来源 IP 统计当前连接</span></div><div class="rows" data-live="devices"></div></div>
      </div>`;
    },
    proxies() {
      if (!S.groups.length) return `<div class="head"><div><h1>代理</h1></div></div><div class="card empty">还没有可用的代理组。先到<a href="#profiles">订阅</a>添加一个订阅。</div>`;
      return `<div class="head"><div><h1>代理</h1><p class="sub">当前模式：${modeName[S.mode]}${S.mode === 'direct' ? '，所有流量直连，节点选择不生效' : ''}</p></div>
        <button class="btn" data-test-all>全部测速</button></div>
      ${S.groups.map(g => `<section class="card" aria-label="${esc(g.name)}"><div class="card-h"><div><h2>${esc(g.name)}</h2><div class="label">${g.type === 'Selector' ? '手动选择' : '自动 · ' + g.note} · 当前 <b style="color:var(--ink);font-weight:600">${esc(g.now)}</b></div></div>
        <button class="btn sm pearl" data-test-group="${esc(g.name)}">测速</button></div>
        <div class="nodes">${g.all.map(n => { const nd = S.byName[n]; const isGroup = S.groups.some(x => x.name === n); return `<button class="node" aria-pressed="${g.now === n}" data-g="${esc(g.name)}" data-n="${esc(n)}" ${g.type !== 'Selector' ? 'disabled' : ''}>
          <span class="n">${esc(n)}</span><span class="m"><span>${nd ? nd.type : (n === 'DIRECT' ? '直连' : n === 'REJECT' ? '拒绝' : isGroup ? '策略组' : '')}</span>${nd ? lat(nd.delay, nd.testing) : ''}</span></button>`; }).join('')}</div></section>`).join('')}`;
    },
    profiles() {
      return `<div class="head"><div><h1>订阅</h1><p class="sub">订阅按设定间隔在后台自动更新，NAS 重启后照常继续</p></div><button class="btn" data-add>添加订阅</button></div>
      ${S.profiles.length ? '' : '<div class="card empty">还没有订阅。点右上角「添加订阅」，粘贴机场给的链接。</div>'}
      <div class="grid g2">${S.profiles.map(p => {
        const pct = p.total ? Math.min(100, p.used / p.total * 100) : 0, left = p.expire ? M.daysLeft(p.expire) : null;
        return `<div class="card prof ${p.current ? 'cur' : ''}"><div class="card-h" style="margin:0"><div style="min-width:0"><h2>${esc(p.name)} ${p.current ? '<span class="pill" style="background:var(--accent);color:var(--on-accent)">使用中</span>' : ''}</h2>
          <div class="label t" style="max-width:340px">${p.url ? esc(p.url.replace(/(token|key|uuid)=[^&]+/gi, '$1=••••')) : '本地配置 · 不自动更新'}</div></div></div>
          ${p.total ? `<div><div class="kv"><span>已用流量</span><b class="num">${p.used} / ${p.total} GB</b></div><div class="bar" style="margin-top:6px"><i class="${pct > 85 ? 'warn' : ''}" style="width:${pct}%"></i></div></div>` : ''}
          <div class="kv"><span>到期</span><b class="num" style="${left != null && left < 30 ? 'color:var(--warn)' : ''}">${p.expire ? `${p.expire}（${left >= 0 ? '剩 ' + left + ' 天' : '已过期'}）` : '—'}</b></div>
          <div class="kv"><span>自动更新</span><b>${p.url ? (p.auto ? '每 ' + p.auto + ' 小时' : '关闭') : '—'}</b></div>
          <div class="kv"><span>上次更新</span><b>${p.updating ? '<span class="spin"></span> 更新中' : esc(p.updated)} · ${p.nodes} 个节点</b></div>
          ${p.lastError ? `<div class="label" style="color:var(--bad)">上次更新失败：${esc(p.lastError)}</div>` : ''}
          <div style="display:flex;gap:8px;flex-wrap:wrap">
            ${p.current ? '' : `<button class="btn sm" data-use="${p.id}">设为当前</button>`}
            ${p.url ? `<button class="btn sm ghost" data-upd="${p.id}" ${p.updating ? 'disabled' : ''}>立即更新</button>` : ''}
            <button class="btn sm pearl" data-ovr="${p.id}">覆写</button>
            ${p.current ? '' : `<button class="btn sm danger" data-del="${p.id}">删除</button>`}
          </div></div>`; }).join('')}</div>`;
    },
    connections() {
      return `<div class="head"><div><h1>连接</h1><p class="sub" data-live="connsub"></p></div>
        <div style="display:flex;gap:10px;flex-wrap:wrap"><input class="search" id="conn-q" type="search" placeholder="搜索域名、设备或规则" value="${esc(connQ)}"><button class="btn danger" data-closeall>全部断开</button></div></div>
      <div class="card" style="padding:8px 12px"><div class="tbl-wrap"><table><thead><tr><th>目标</th><th>来源设备</th><th>规则</th><th>链路</th><th style="text-align:right">↓</th><th style="text-align:right">↑</th><th>时长</th><th></th></tr></thead><tbody data-live="connrows"></tbody></table></div></div>`;
    },
    logs() {
      return `<div class="head"><div><h1>日志</h1><p class="sub">实时输出，保留最近 400 条。记录哪些级别由设置里的「日志级别」决定</p></div>
        <div style="display:flex;gap:10px;flex-wrap:wrap"><div class="seg" role="group" aria-label="日志级别">${[['all', '全部'], ['info', '信息'], ['warning', '警告'], ['error', '错误'], ['debug', '调试']].map(([k, v]) => `<button aria-pressed="${logLv === k}" data-lv="${k}">${v}</button>`).join('')}</div>
        <button class="btn sm pearl" data-pause>${logPaused ? '继续' : '暂停'}</button></div></div>
      <div class="term" id="term" tabindex="0" aria-label="日志输出"></div>`;
    },
    config() {
      const st = S.settings || {}, sw = (k, v, l) => `<button class="switch" role="switch" aria-checked="${!!v}" data-sw="${k}" aria-label="${l}"></button>`;
      return `<div class="head"><div><h1>设置</h1><p class="sub">保存后重新加载配置，代理端口改动几秒内生效</p></div><button class="btn" data-save>保存</button></div>
      <div class="grid g2">
        <div class="card"><h2 style="margin-bottom:6px">网络</h2><div class="set-group">
          <div class="set"><div><div>代理端口（HTTP + SOCKS5）</div><div class="d">Docker 里改了这里，也要同步改端口映射</div></div><input class="field num" id="s-mixed" value="${st.mixedPort}" inputmode="numeric"></div>
          <div class="set"><div><div>允许局域网连接</div><div class="d">关闭后只有 NAS 本机可用</div></div>${sw('allowLan', st.allowLan, '允许局域网连接')}</div>
          <div class="set"><div><div>IPv6</div><div class="d">家庭网络没有 IPv6 时保持关闭</div></div>${sw('ipv6', st.ipv6, 'IPv6')}</div>
          <div class="set"><div><div>测速地址</div></div><input class="field" id="s-test" value="${esc(st.testUrl)}"></div>
        </div></div>
        <div class="card"><h2 style="margin-bottom:6px">DNS</h2><div class="set-group">
          <div class="set"><div><div>上游 DNS</div><div class="d">逗号分隔；留空则沿用订阅里的 DNS</div></div><input class="field" id="s-dns" value="${esc((st.dns || []).join(', '))}"></div>
          <div class="set"><div><div>Fake-IP</div><div class="d">加快解析，部分 P2P 应用需关闭</div></div>${sw('fakeIP', st.fakeIP, 'Fake-IP')}</div>
          <div class="set"><div><div>日志级别</div></div><select class="field" id="s-log" style="width:140px">${['debug', 'info', 'warning', 'error', 'silent'].map(l => `<option ${st.logLevel === l ? 'selected' : ''}>${l}</option>`).join('')}</select></div>
        </div></div>
        <div class="card"><h2 style="margin-bottom:6px">订阅与规则数据</h2><div class="set-group">
          <div class="set"><div><div>订阅 User-Agent</div><div class="d">部分机场按 UA 下发配置</div></div><input class="field" id="s-ua" value="${esc(st.userAgent)}"></div>
          <div class="set"><div><div>更新 GeoIP / GeoSite</div><div class="d">规则里用到 GEOIP、GEOSITE 时需要</div></div><button class="btn sm ghost" data-geo>立即更新</button></div>
        </div></div>
        <div class="card"><h2 style="margin-bottom:6px">安全与备份</h2><div class="set-group">
          <div class="set"><div><div>访问密码</div><div class="d">${S.passwordSet ? '已设置；留空并保存可取消' : '未设置，任何人都能打开本页'}</div></div>
            <form id="pw-form" style="display:flex;gap:8px"><input class="field" id="s-pw" type="password" autocomplete="new-password" placeholder="新密码" style="width:140px"><button class="btn sm pearl" type="submit">保存密码</button></form></div>
          <div class="set"><div><div>导出全部配置</div><div class="d">订阅文件、覆写和设置，不含密码</div></div><a class="btn sm pearl" href="/api/export">导出</a></div>
          ${S.passwordSet ? '<div class="set"><div>退出登录</div><button class="btn sm danger" data-logout>退出</button></div>' : ''}
        </div></div>
      </div>`;
    },
  };

  function live() {
    const set = (k, h) => { const el = view.querySelector(`[data-live="${k}"]`); if (el) el.innerHTML = h; };
    set('down', M.fmtRate(S.traffic.down.at(-1)).replace(/ (\S+)$/, '<small>$1</small>'));
    set('up', M.fmtRate(S.traffic.up.at(-1)).replace(/ (\S+)$/, '<small>$1</small>'));
    set('total', `${M.fmtB(S.total.down)} / ${M.fmtB(S.total.up)}`);
    set('conns', S.conns.length);
    set('chart', chart());
    set('devices', S.devices.length ? S.devices.map(d => `<div class="row"><div class="grow"><div class="t num" style="font-weight:600">${esc(d.name)}</div></div><span class="pill num">${d.conns} 连接</span><span class="label num" style="width:80px;text-align:right">↓ ${M.fmtB(d.down)}</span></div>`).join('') : '<div class="empty">暂时没有设备在用代理</div>');
    const q = connQ.toLowerCase(), list = S.conns.filter(c => !q || [c.host, c.dev, c.src, c.rule, c.chain.join(' ')].join(' ').toLowerCase().includes(q));
    set('connsub', `${S.conns.length} 条活动连接${q ? `，筛选出 ${list.length} 条` : ''}`);
    set('connrows', list.length ? list.slice(0, 300).map(c => `<tr><td class="host">${esc(c.host)}<span class="label">:${c.port} ${c.net}</span></td><td class="num">${esc(c.dev)}</td>
      <td><span class="pill">${esc(c.rule)}</span></td><td>${c.chain.map(esc).join(' › ')}</td><td class="num" style="text-align:right">${M.fmtB(c.down)}</td><td class="num" style="text-align:right">${M.fmtB(c.up)}</td>
      <td class="num">${M.dur(Date.now() - c.start)}</td><td><button class="btn sm danger" data-close="${esc(c.id)}">断开</button></td></tr>`).join('') : `<tr><td colspan="8" class="empty">${q ? '没有匹配的连接' : '暂无活动连接'}</td></tr>`);
    const term = view.querySelector('#term');
    if (term && !logPaused) {
      const atBottom = term.scrollHeight - term.scrollTop - term.clientHeight < 40;
      term.innerHTML = S.logs.filter(l => logLv === 'all' || l.lv === logLv).slice(-200).map(l => `<div class="l"><span class="ts">${l.ts}</span><span class="lv ${esc(l.lv)}">${esc(l.lv.toUpperCase())}</span><span>${esc(l.msg)}</span></div>`).join('') || '<div class="empty">暂无日志</div>';
      if (atBottom) term.scrollTop = term.scrollHeight;
    }
    $('#c-conn').textContent = S.conns.length;
    $('#side-up').textContent = S.running ? '已运行 ' + M.fmtUp(S.uptime) : '已暂停';
  }
  function chrome() {
    $('#c-nodes').textContent = S.nodes.length || ''; $('#c-prof').textContent = S.profiles.length || '';
    $('#side-ver').textContent = `mihomo ${S.coreVersion} · ${S.version}`;
    $('#side-dot').className = 'dot' + (S.running ? '' : ' off'); $('#side-state').textContent = S.running ? '内核运行中' : '内核已暂停';
    document.querySelectorAll('#nav a').forEach(a => a.dataset.v === cur ? a.setAttribute('aria-current', 'page') : a.removeAttribute('aria-current'));
  }
  function render() {
    if (S.needLogin) { $('#login').hidden = false; $('.app').hidden = true; setTimeout(() => $('#l-pw').focus(), 0); return; }
    $('#login').hidden = true; $('.app').hidden = false;
    if (!S.ready) return;
    view.innerHTML = R[cur](); if (drawer) view.insertAdjacentHTML('beforeend', drawer());
    chrome(); live();
    const t = view.querySelector('#term'); if (t) t.scrollTop = t.scrollHeight;
  }
  function go() { const h = location.hash.slice(1); cur = VIEWS.includes(h) ? h : 'overview'; drawer = null; render(); window.scrollTo(0, 0); }

  function addDrawer() {
    return `<div class="drawer" data-dclose><form class="panel" id="add-form"><h2 style="font-size:24px">添加订阅</h2>
      <div class="seg" role="group" aria-label="来源"><button type="button" aria-pressed="true" data-src="url">订阅链接</button><button type="button" aria-pressed="false" data-src="text">粘贴配置</button></div>
      <div id="src-url" style="display:flex;flex-direction:column;gap:8px"><label class="label" for="a-url">订阅链接</label><input class="field" id="a-url" type="url" placeholder="https://…">
        <label class="label" for="a-auto">自动更新</label><select class="field" id="a-auto"><option value="6">每 6 小时</option><option value="12">每 12 小时</option><option value="24" selected>每天</option><option value="0">不自动更新</option></select></div>
      <div id="src-text" hidden style="display:flex;flex-direction:column;gap:8px"><label class="label" for="a-text">Clash YAML 或节点链接（每行一个）</label><textarea class="field" id="a-text" rows="10" style="font-family:var(--mono);font-size:12px"></textarea></div>
      <label class="label" for="a-name">名称（可留空，默认读取订阅里的名称）</label><input class="field" id="a-name">
      <div class="label" id="a-err" style="color:var(--bad)"></div>
      <div style="display:flex;gap:10px"><button class="btn" type="submit" id="a-submit">下载并添加</button><button class="btn ghost" type="button" data-dclose>取消</button></div></form></div>`;
  }
  function ovrDrawer(id) {
    const p = S.profiles.find(x => x.id === id), o = p.override || {};
    return () => `<div class="drawer" data-dclose><form class="panel" id="ovr-form" data-id="${p.id}"><h2 style="font-size:24px">覆写 · ${esc(p.name)}</h2>
      <p class="sub" style="margin:0">订阅更新后，这些设置仍会叠加在原配置之上。端口、DNS 和模式始终使用全局设置。</p>
      <label class="label" for="o-name">名称</label><input class="field" id="o-name" value="${esc(p.name)}">
      ${p.url ? `<label class="label" for="o-auto">自动更新（小时，0 为关闭）</label><input class="field num" id="o-auto" inputmode="numeric" value="${p.auto}">` : ''}
      <label class="label" for="o-rules">追加规则（写在订阅规则之前，每行一条，# 开头为注释）</label>
      <textarea class="field" id="o-rules" rows="7" style="font-family:var(--mono);font-size:13px" placeholder="DOMAIN-SUFFIX,nas.home.arpa,DIRECT&#10;IP-CIDR,192.168.1.0/24,DIRECT,no-resolve">${esc((o.prependRules || []).join('\n'))}</textarea>
      <label class="label" for="o-excl">排除节点（正则，匹配节点名）</label><input class="field" id="o-excl" placeholder="(?i)过期|剩余|官网" value="${esc(o.excludeFilter || '')}">
      <div class="label" id="o-err" style="color:var(--bad)"></div>
      <div style="display:flex;gap:10px"><button class="btn" type="submit">保存覆写</button><button class="btn ghost" type="button" data-dclose>取消</button></div></form></div>`;
  }

  document.addEventListener('click', e => {
    const t = e.target.closest('button,[data-dclose]'); if (!t) return; const d = t.dataset;
    if (d.dclose !== undefined && (e.target === t || t.tagName === 'BUTTON')) { drawer = null; render(); return; }
    if (d.act === 'run') act(() => M.toggleRun(), S.running ? '内核已暂停' : '内核已启动');
    else if (d.mode) act(() => M.setMode(d.mode), '已切换为' + modeName[d.mode] + '模式');
    else if (d.copy) copy(d.copy, '已复制');
    else if (d.g && d.n) act(() => M.select(d.g, d.n), `${d.g} → ${d.n}`);
    else if (d.testGroup !== undefined) M.test(S.groups.find(g => g.name === d.testGroup).all);
    else if (t.hasAttribute('data-test-all')) M.test(S.nodes.map(n => n.name));
    else if (d.upd) act(() => M.updateProfile(d.upd), '订阅已更新');
    else if (d.use) act(() => M.useProfile(d.use), '已切换订阅，配置已重新加载');
    else if (d.del) { if (d.confirm) act(() => M.removeProfile(d.del), '订阅已删除'); else { d.confirm = 1; t.textContent = '确认删除？'; } }
    else if (d.ovr) { drawer = ovrDrawer(d.ovr); render(); }
    else if (t.hasAttribute('data-add')) { drawer = addDrawer; render(); $('#a-url').focus(); }
    else if (d.src) { document.querySelectorAll('[data-src]').forEach(b => b.setAttribute('aria-pressed', b === t)); $('#src-url').hidden = d.src !== 'url'; $('#src-text').hidden = d.src !== 'text'; $('#a-submit').textContent = d.src === 'url' ? '下载并添加' : '添加'; }
    else if (d.close) act(() => M.closeConn(d.close));
    else if (t.hasAttribute('data-closeall')) { if (d.confirm) act(() => M.closeAll(), '已断开全部连接'); else { d.confirm = 1; t.textContent = '确认全部断开？'; } }
    else if (d.lv) { logLv = d.lv; render(); }
    else if (t.hasAttribute('data-pause')) { logPaused = !logPaused; render(); }
    else if (d.sw) t.setAttribute('aria-checked', t.getAttribute('aria-checked') !== 'true');
    else if (t.hasAttribute('data-save')) {
      const on = k => view.querySelector(`[data-sw="${k}"]`).getAttribute('aria-checked') === 'true';
      const s = Object.assign({}, S.settings, {
        mixedPort: +$('#s-mixed').value, allowLan: on('allowLan'), ipv6: on('ipv6'), fakeIP: on('fakeIP'), testUrl: $('#s-test').value.trim(),
        dns: $('#s-dns').value.split(/[,，\s]+/).map(x => x.trim()).filter(Boolean), logLevel: $('#s-log').value, userAgent: $('#s-ua').value.trim(),
      });
      act(() => M.saveSettings(s), '设置已保存，配置已重新加载');
    }
    else if (t.hasAttribute('data-geo')) { t.innerHTML = '<span class="spin"></span>'; act(() => M.updateGeo(), 'GeoIP 与 GeoSite 已更新').finally(() => { t.textContent = '立即更新'; }); }
    else if (t.hasAttribute('data-logout')) act(() => M.logout());
  });
  document.addEventListener('submit', async e => {
    e.preventDefault(); const f = e.target;
    if (f.id === 'login-form') {
      const btn = f.querySelector('button'); btn.disabled = true; $('#l-err').textContent = '';
      try { await M.login($('#l-pw').value); go(); } catch (err) { $('#l-err').textContent = err.message; } finally { btn.disabled = false; }
    }
    if (f.id === 'add-form' && !busy) {
      const isUrl = !$('#src-url').hidden, btn = $('#a-submit'), label = btn.textContent;
      const body = { name: $('#a-name').value.trim(), url: isUrl ? $('#a-url').value.trim() : '', content: isUrl ? '' : $('#a-text').value, autoHours: isUrl ? +$('#a-auto').value : 0 };
      if (isUrl && !body.url) { $('#a-err').textContent = '请填写订阅链接'; return; }
      busy = true; btn.disabled = true; btn.innerHTML = '<span class="spin"></span> 处理中'; $('#a-err').textContent = '';
      try { await M.addProfile(body); drawer = null; render(); toast('订阅已添加'); }
      catch (err) { $('#a-err').textContent = err.message; btn.disabled = false; btn.textContent = label; }
      finally { busy = false; }
    }
    if (f.id === 'ovr-form') {
      const id = f.dataset.id, auto = $('#o-auto');
      const body = { name: $('#o-name').value, override: { prependRules: $('#o-rules').value.split('\n'), excludeFilter: $('#o-excl').value.trim() } };
      if (auto) body.autoHours = Math.max(0, +auto.value || 0);
      try { await M.patchProfile(id, body); drawer = null; render(); toast('覆写已保存'); } catch (err) { $('#o-err').textContent = err.message; }
    }
    if (f.id === 'pw-form') {
      const pw = $('#s-pw').value;
      act(() => M.setPassword(pw), pw ? '访问密码已设置' : '已取消访问密码');
    }
  });
  document.addEventListener('input', e => { if (e.target.id === 'conn-q') { connQ = e.target.value; live(); } });
  document.addEventListener('keydown', e => { if (e.key === 'Escape' && drawer) { drawer = null; render(); } });
  M.on(k => {
    if (k === 'tick') { if (S.ready && !S.needLogin) live(); }
    else if (k === 'login') render();
    else if (!drawer && (cur !== 'config' || k === 'settings' || k === 'all')) render();
  });
  window.addEventListener('hashchange', go);
  M.boot().then(go).catch(e => { view.innerHTML = `<div class="card empty">无法连接后端：${esc(e.message)}</div>`; });
})();
