package server

// 助手页（v0.8 单栏套壳）：一栏表达全部在用会话——
// 🌊 工作中 / 🟡 待你输入 / ⚓ 待命 / 🏁 已完成（折叠）/ 🕒 历史（折叠）。
// 顶部一张「新建会话」表单，像 CLI 一样随时新开一个空会话。
// 状态靠 /api/live SSE 即时翻灯（8s 轮询兜底），变化即按服务端数据全量重排。
const dirListHTML = head + `
{{template "topbar" .}}

<main class="fleet-page" id="fleet-page">
  <details class="fleet-new-box" id="fleet-new-box" open>
    <summary><span class="emo">⛵ </span>新建会话<i>选项目目录、起个名，第一句话可先说也可先不说</i></summary>
    <form class="fleet-new-form" id="fleet-new-form" data-fleet-new action="/fleets/new">
      <div class="fn-grid">
        <input name="dir" list="known-dir-list" required spellcheck="false" autocomplete="off"
               placeholder="项目目录（绝对路径）" enterkeyhint="go" aria-label="项目目录">
        <input name="name" required maxlength="40" spellcheck="false" autocomplete="off"
               placeholder="会话名，如 数学-1加2（目标线短码-需求，≤40 字）" aria-label="会话名">
        {{if .Routes}}
        <select name="route" aria-label="挂到哪条目标线（可选）">
          <option value="">不挂目标线</option>
          {{range .Routes}}<option value="{{.Meta.ID}}">{{.Meta.Code}} · {{.Meta.Title}}</option>{{end}}
        </select>
        {{end}}
      </div>
      <datalist id="known-dir-list">{{range .KnownDirs}}<option value="{{.Dir}}">{{end}}</datalist>
      <textarea name="msg" rows="2" maxlength="8000"
                placeholder="第一句话（可空——先新开一个空会话，进去再说，和终端一样）"></textarea>
      <div class="fn-foot">
        <span class="fn-err" id="fn-err" data-fn-err role="alert" hidden></span>
        <button type="submit" class="primary"><span class="emo">⛵ </span>新开会话</button>
      </div>
    </form>
  </details>

  {{if .Error}}<div class="dir-err" data-debug="error"><span class="dir-err-txt"><span class="emo">⚠️ </span>{{.Error}}</span><button type="button" class="dir-err-x" data-err-close aria-label="关掉报错">✕</button></div>
  <p class="dir-back-recent"><a href="/sessions">← 看全部会话</a></p>{{end}}

  {{if not .NoDir}}
  <section class="dir-list-head">
    <h2><span class="emo">📂 </span><b>{{.Short}}</b><code class="dir-full" title="{{.Path}}">{{.Path}}</code></h2>
    <span><a class="dir-all-link" href="/sessions"><span class="emo">🕒 </span>全部会话</a></span>
  </section>
  {{end}}

  <div class="fleet-col" id="fleet-col" data-dir="{{.Path}}">
    <section class="fleet-seg" data-seg="working">
      <h2 class="seg-head"><i class="dot"></i><span class="emo">🌊 </span>工作中<b class="seg-n"></b></h2>
      <div class="seg-body">{{range index .Groups "working"}}{{template "dirRow" .}}{{end}}</div>
      <p class="seg-empty">没有正在工作的会话</p>
    </section>
    <section class="fleet-seg" data-seg="waiting">
      <h2 class="seg-head"><i class="dot"></i><span class="emo">🟡 </span>待你输入<b class="seg-n"></b></h2>
      <div class="seg-body">{{range index .Groups "waiting"}}{{template "dirRow" .}}{{end}}</div>
      <p class="seg-empty">没有卡住等你的会话</p>
    </section>
    <section class="fleet-seg" data-seg="idle">
      <h2 class="seg-head"><i class="dot"></i><span class="emo">⚓ </span>待命<b class="seg-n"></b></h2>
      <div class="seg-body">{{range index .Groups "idle"}}{{template "dirRow" .}}{{end}}</div>
      <p class="seg-empty">没有待命会话</p>
    </section>
    <details class="fleet-seg seg-fold" data-seg="done">
      <summary class="seg-head"><i class="dot"></i><span class="emo">🏁 </span>已完成<b class="seg-n"></b></summary>
      <div class="seg-body">{{range index .Groups "done"}}{{template "dirRow" .}}{{end}}</div>
      <p class="seg-empty">还没有已完成的会话</p>
    </details>
    <details class="fleet-seg seg-fold" data-seg="offline">
      <summary class="seg-head"><i class="dot"></i><span class="emo">🕒 </span>历史会话<b class="seg-n"></b></summary>
      <div class="seg-body">{{range index .Groups "offline"}}{{template "dirRow" .}}{{end}}</div>
      <p class="seg-empty">没有历史会话</p>
    </details>
  </div>

  {{if .NoDir}}
  <details class="dir-known-fold">
    <summary>按目录精确翻找（贴绝对路径，或点下面已知目录）</summary>
    <form class="dir-bar" method="get" action="/sessions" id="dir-form">
      <input type="text" name="dir" value="{{.Path}}" spellcheck="false" list="known-dir-list"
        placeholder="贴项目目录（~/Documents/GitHub/xxx 也行）" aria-label="项目目录" enterkeyhint="search">
      <button type="submit" class="mini"><span class="emo">📂 </span>翻会话</button>
    </form>
    <div class="dir-chips" id="known-chips">
      {{range .KnownDirs}}
      <a class="dir-chip" href="/sessions?dir={{.Dir | urlquery}}">
        <span class="emo">📍</span> <b>{{.Short}}</b><i title="{{.Dir}}">{{.Dir}}</i>
      </a>
      {{end}}
    </div>
  </details>
  <div id="recent-chips" class="dir-chips" data-label="最近浏览"></div>
  {{end}}
</main>

{{define "dirRow"}}
{{if .Anon}}
<div class="dir-row dir-row-anon is-waiting" data-sid="{{.SID}}" data-anon="1" title="进程开着但还没说话，说第一句后就能点进会话页">
  <span class="dir-state"><i class="dot"></i></span>
  <span class="dir-main">
    <span class="dir-title">{{.Title}}</span>
    <span class="dir-meta">
      <em class="st-waiting" data-debug="waiting">待你输入</em><b>·</b>
      <i class="dir-proj"><span class="emo">📁 </span>{{.Proj}}</i><b>·</b><span class="anon-note">还没说话，点不开</span>
    </span>
  </span>
</div>
{{else}}
<a class="dir-row {{.Class}}{{if .MarkText}} has-mark{{end}}{{if .Perms}} has-perm{{end}}" data-sid="{{.SID}}"
   href="/sessions/{{.SID}}?dir={{.Dir | urlquery}}" title="{{.Dir}}">
  <span class="dir-state"><i class="dot"></i></span>
  <span class="dir-main">
    <span class="dir-title">{{if .Name}}{{.Name}}{{else if .Title}}{{.Title}}{{else}}<code>{{slice .SID 0 8}}</code><span data-debug="(untitled)">（无题首句）</span>{{end}}</span>
    <span class="dir-meta">
      {{if .Perms}}<em class="st-perm" data-debug="perm">🔐 待批准 {{.Perms}}<b>·</b></em>{{end}}
      <span class="row-state">{{if .MarkText}}<em class="st-mark st-mark-{{.Mark}}" data-debug="mark {{.Mark}}">{{.MarkText}}</em>{{end}}<em class="st-working" data-debug="working">工作中</em><em class="st-restarting">♻️ 重连中</em><em class="st-waiting" data-debug="waiting">待你输入</em><em class="st-stopped" data-debug="stopped">已停止</em><b>·</b></span>
      {{if .ShowProj}}<i class="dir-proj"><span class="emo">📁 </span>{{.Proj}}</i><b>·</b>{{end}}
      {{if not .HideCount}}<span class="row-events">{{.Events}}</span> 条消息<b>·</b>{{end}}<span class="row-stamp">更新 {{if .LastTS.IsZero}}刚刚{{else}}{{.LastTS.Format "01-02 15:04"}}{{end}}</span>
    </span>
  </span>
  <span class="dir-go emo">→</span>
</a>
{{end}}
{{end}}

<script src="/static/fleet-new.js?v=22"></script>
<script>
(function () {
  // 报错条可一键关掉：视觉移除 + 清地址栏 ?dir=，避免刷新把错误带回来。
  document.addEventListener('click', function (e) {
    if (!e.target.closest || !e.target.closest('[data-err-close]')) return;
    var err = document.querySelector('.dir-err');
    if (err) err.remove();
    var back = document.querySelector('.dir-back-recent');
    if (back) back.remove();
    try { history.replaceState(null, '', '/sessions'); } catch (x) {}
  });

  // ---------- 单栏全量重排 ----------
  var col = document.getElementById('fleet-col');
  var dirQS = new URLSearchParams(location.search).get('dir') || (col && col.dataset.dir) || '';
  var apiURL = '/api/sessions' + (dirQS ? '?dir=' + encodeURIComponent(dirQS) : '');
  var SEGS = ['working', 'waiting', 'idle', 'done', 'offline'];

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function markText(m) { return m === 'good' ? '✅ 圆满结束' : m === 'stop' ? '⏹ 中途停止' : ''; }
  function groupOf(r) {
    if (r.mark === 'good') return 'done';
    if (r.anon) return 'waiting';
    if (r.online && (r.status === 'busy' || r.status === 'restarting')) return 'working';
    if (r.online && r.status === 'waiting') return 'waiting';
    if (r.online) return 'idle';
    return 'offline';
  }
  function fmtStamp(t) {
    if (!t) return '';
    var d = new Date(t * 1000);
    return String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0') + ' ' +
      String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0');
  }
  function rowHTML(r) {
    if (r.anon) {
      return '<div class="dir-row dir-row-anon is-waiting" data-sid="' + esc(r.sid) + '" data-anon="1">' +
        '<span class="dir-state"><i class="dot"></i></span><span class="dir-main">' +
        '<span class="dir-title">新开的会话（还没说话）</span><span class="dir-meta">' +
        '<em class="st-waiting">待你输入</em><b>·</b><i class="dir-proj"><span class="emo">📁 </span>' + esc(r.proj) + '</i><b>·</b><span class="anon-note">还没说话，点不开</span>' +
        '</span></span></div>';
    }
    var title = r.name || r.title || '';
    var titleHTML = title ? esc(title)
      : '<code>' + esc(r.sid.slice(0, 8)) + '</code><span>（无题首句）</span>';
    var mk = markText(r.mark);
    var cls = 'is-' + groupOf(r) + (r.online && r.status === 'restarting' ? ' is-restarting' : '') + (mk ? ' has-mark' : '') + (r.perms ? ' has-perm' : '');
    var permTag = r.perms ? '<em class="st-perm" data-debug="perm">🔐 待批准 ' + r.perms + '<b>·</b></em>' : '';
    var meta = permTag + '<span class="row-state">' +
      (mk ? '<em class="st-mark st-mark-' + esc(r.mark) + '">' + mk + '</em>' : '') +
      '<em class="st-working">工作中</em><em class="st-restarting">♻️ 重连中</em><em class="st-waiting">待你输入</em><em class="st-stopped">已停止</em><b>·</b></span>';
    if (r.proj) meta += '<i class="dir-proj"><span class="emo">📁 </span>' + esc(r.proj) + '</i><b>·</b>';
    if (r.events) meta += '<span class="row-events">' + r.events + '</span> 条消息<b>·</b>';
    var st = fmtStamp(r.stamp);
    meta += '<span class="row-stamp">更新 ' + (st || '刚刚') + '</span>';
    return '<a class="dir-row ' + cls + '" data-sid="' + esc(r.sid) + '" href="/sessions/' +
      encodeURIComponent(r.sid) + '?dir=' + encodeURIComponent(r.dir || '') + '" title="' + esc(r.dir || '') + '">' +
      '<span class="dir-state"><i class="dot"></i></span><span class="dir-main">' +
      '<span class="dir-title">' + titleHTML + '</span><span class="dir-meta">' + meta +
      '</span></span><span class="dir-go emo">→</span></a>';
  }

  var folded = { done: true, offline: true }; // 记住用户开合
  function render(rows) {
    var groups = { working: [], waiting: [], idle: [], done: [], offline: [] };
    rows.forEach(function (r) { groups[groupOf(r)].push(r); });
    SEGS.forEach(function (key) {
      var seg = col.querySelector('[data-seg="' + key + '"]');
      if (!seg) return;
      var body = seg.querySelector('.seg-body');
      var empty = seg.querySelector('.seg-empty');
      var n = seg.querySelector('.seg-n');
      // 组内：等审批的会话顶到最前（最该被看见），其余按更新时间倒序
      groups[key].sort(function (a, b) {
        if (!!a.perms !== !!b.perms) return (b.perms ? 1 : 0) - (a.perms ? 1 : 0);
        return (b.stamp || 0) - (a.stamp || 0);
      });
      body.innerHTML = groups[key].map(rowHTML).join('');
      if (n) n.textContent = groups[key].length ? String(groups[key].length) : '';
      if (empty) empty.hidden = groups[key].length > 0;
      if (key === 'done' || key === 'offline') {
        seg.hidden = groups[key].length === 0;
        seg.open = !folded[key] && groups[key].length > 0;
      }
    });
  }

  // ---------- 数据：8s 轮询兜底 + SSE 即时触发（合并 200ms 内的风暴）----------
  var timer = null, active = false, queued = false, kickAt = 0;
  function tick() {
    if (active) { queued = true; return; }
    active = true;
    fetch(apiURL, { headers: { 'Accept': 'application/json' }, credentials: 'same-origin' })
      .then(function (r) { if (r.status === 401) { location.href = '/login'; throw 0; } return r.json(); })
      .then(function (d) { if (d && d.rows) render(d.rows); })
      .catch(function () {})
      .finally(function () {
        active = false;
        if (queued) { queued = false; setTimeout(tick, 60); }
      });
  }
  function schedule() { timer = setTimeout(function () { tick(); schedule(); }, 8000); }
  document.addEventListener('visibilitychange', function () {
    if (document.hidden) { clearTimeout(timer); timer = null; }
    else if (!timer) { tick(); schedule(); }
  });
  try {
    var es = new EventSource('/api/live');
    es.onmessage = function (e) {
      var ev;
      try { ev = JSON.parse(e.data); } catch (x) { return; }
      // fleet=任何会话翻灯/新生死；perm=某个会话等审批；本页相关 state 也立刻拉
      if (ev.type === 'fleet' || ev.type === 'perm' || (ev.type === 'state' && ev.sid)) {
        var now = Date.now();
        if (now < kickAt) return;
        kickAt = now + 200;
        setTimeout(tick, 30);
      }
    };
  } catch (e) { /* SSE 起不来就靠 8s 轮询 */ }
  tick();
  schedule();

  // ---------- 最近浏览目录：localStorage 落/取 ----------
  try {
    var KEY = 'mindport-recent-dirs';
    if (dirQS) {
      var cur = JSON.parse(localStorage.getItem(KEY) || '[]').filter(function (x) { return x !== dirQS; });
      cur.unshift(dirQS);
      localStorage.setItem(KEY, JSON.stringify(cur.slice(0, 6)));
    } else {
      var box = document.getElementById('recent-chips');
      if (box) JSON.parse(localStorage.getItem(KEY) || '[]').forEach(function (d) {
        var a = document.createElement('a');
        a.className = 'dir-chip dir-chip-recent';
        a.href = '/sessions?dir=' + encodeURIComponent(d);
        var icon = document.createElement('span'); icon.className = 'emo'; icon.textContent = '🕘 ';
        var name = document.createElement('b'); name.textContent = d.split('/').pop();
        var full = document.createElement('i'); full.textContent = d; full.title = d;
        a.appendChild(icon); a.appendChild(name); a.appendChild(full);
        box.appendChild(a);
      });
    }
  } catch (e) {}

  // 新建表单首次新开会话成功后，折叠组自动展开待命组（落点在那里）
  document.addEventListener('mp-fleet-born', function () { tick(); });
})();
</script>
{{template "tabbar" .}}
</body></html>`
