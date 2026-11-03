package server

const head = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="theme-color" content="#221b14">
<script>(function(){try{var t=localStorage.getItem('mindport-theme');var legacy={dark:'night',light:'day'};if(legacy[t])t=legacy[t];if(['night','day','abyss','porcelain'].indexOf(t)<0)t=window.matchMedia('(prefers-color-scheme: light)').matches?'day':'night';document.documentElement.dataset.theme=t;}catch(e){}
try{if(sessionStorage.getItem('mpdbg')==='1'){document.documentElement.dataset.stealth='1';document.title='mindportd — debug';var mc=document.querySelector('meta[name="theme-color"]');if(mc)mc.setAttribute('content','#ffffff');}}catch(e){}})();</script>
<link rel="manifest" href="/manifest.webmanifest">
<link rel="icon" href="/icon.svg">
<link rel="apple-touch-icon" href="/icon.svg">
<link rel="stylesheet" href="/static/style.css">
<title>思港 MindPort</title>
</head>
<body>

{{define "topbar"}}
<header class="topbar">
  <a class="brand" href="/" data-debug="mindportd">
    <span class="brand-mark" title="">{{template "pxLighthouse" .}}</span>
    <span class="brand-text">思港 <i>MindPort</i><em>想法停泊的港口</em></span>
  </a>
  <div class="top-sea">{{template "topSea" .}}</div>
  <div class="top-right">
    <button id="phone-btn" class="phone-btn" type="button" aria-label="手机预览" title="手机预览（390 宽调试）">📱</button>
    <button id="stealth-btn" class="stealth-btn" type="button" aria-label="dev">
      <span class="s-go">&lt;/&gt;</span><span class="s-back">✕</span>
    </button>
    <details class="themes pop" id="themes">
      <summary class="themes-btn" role="button" aria-label="选择配色主题" title="选择配色主题">
        <svg viewBox="0 0 8" height="17" width="17" shape-rendering="crispEdges" aria-hidden="true">
          <rect x="0" y="0" width="3" height="3" fill="#c06a45"/>
          <rect x="4" y="0" width="3" height="3" fill="#f0b23f"/>
          <rect x="0" y="4" width="3" height="3" fill="#6fb3d9"/>
          <rect x="4" y="4" width="3" height="3" fill="#7bc96f"/>
        </svg>
        <i class="caret"></i>
      </summary>
      <div class="themes-pop">
        <button type="button" class="theme-opt" data-theme="night">
          <span class="sw"><i style="background:#221b14"></i><i style="background:#f0b23f"></i><i style="background:#c06a45"></i></span>
          <span class="t-name">夜港<small>木舱暖夜</small></span>
        </button>
        <button type="button" class="theme-opt" data-theme="day">
          <span class="sw"><i style="background:#f1e6cf"></i><i style="background:#a86f1c"></i><i style="background:#b35a38"></i></span>
          <span class="t-name">日港<small>羊皮海图</small></span>
        </button>
        <button type="button" class="theme-opt" data-theme="abyss">
          <span class="sw"><i style="background:#0b1c2c"></i><i style="background:#f2b544"></i><i style="background:#5aa9e6"></i></span>
          <span class="t-name">深海<small>墨蓝冷夜</small></span>
        </button>
        <button type="button" class="theme-opt" data-theme="porcelain">
          <span class="sw"><i style="background:#eef3f4"></i><i style="background:#2f5d8c"></i><i style="background:#356d9e"></i></span>
          <span class="t-name">青瓷<small>青花白瓷</small></span>
        </button>
      </div>
    </details>
    <a class="fleet-btn" id="fleet" href="/sessions" data-debug="fleet"
       title="助手＝全部会话：每个 Claude 会话就是一位助手">
      <i class="fleet-dot" id="fleet-dot"></i><span class="fleet-label" data-debug="fleet label">助手</span><b id="fleet-count"></b>
    </a>
    {{if .Auth}}<a class="logout-btn" href="/logout" title="登出" data-debug="x" aria-label="登出">⇥</a>{{end}}
  </div>
</header>
{{end}}

{{/* 16×16 像素灯塔：塔灯按摩尔斯节奏硬闪，夜港两束扫光，日港只亮灯 */}}
{{/* 底部主导航（类 app tab bar）：任务台 / 助手 / 目标线。
     高亮按路径本地判定，助手角标 20s 轻量轮询 /api/sessions。
     登录页不调用此模板。 */}}
{{define "tabbar"}}
<nav class="tabbar" aria-label="主导航">
  <a class="tab tab-board" href="/" data-tab="board">
    <span class="tab-ic">⚓</span>
    <span class="tab-tx" data-debug="board">任务台</span>
  </a>
  <a class="tab tab-fleet" href="/sessions" data-tab="fleet">
    <span class="tab-ic">⛵<i class="tab-badge" id="tab-fleet-badge" hidden></i></span>
    <span class="tab-tx" data-debug="fleet">助手</span>
  </a>
  <a class="tab tab-routes" href="/routes" data-tab="routes">
    <span class="tab-ic">🧭<i class="tab-badge" id="tab-route-badge" hidden></i></span>
    <span class="tab-tx" data-debug="routes">目标线</span>
  </a>
</nav>
<script>
(function () {
  var p = location.pathname;
  var which = (p === '/' || p.indexOf('/tasks/') === 0) ? 'board'
    : p.indexOf('/routes') === 0 ? 'routes'
    : (p.indexOf('/sessions') === 0) ? 'fleet' : '';
  if (which) {
    var a = document.querySelector('.tab-' + which);
    if (a) { a.classList.add('active'); a.setAttribute('aria-current', 'page'); }
  }
  function tick() {
    fetch('/api/sessions').then(function (r) { return r.json(); }).then(function (d) {
      var rows = d.rows || [];
      var n = rows.filter(function (r) { return r.online; }).length;
      var b = document.getElementById('tab-fleet-badge');
      if (b) { b.textContent = n ? String(n) : ''; b.hidden = !n; }
      // 顶栏助手灯（sseBoot）复用同一份轮询，不重复请求
      window.dispatchEvent(new CustomEvent('mp-sessions', { detail: { rows: rows, live: n } }));
    }).catch(function () {});
    // 目标线角标：在役目标线下未完成的需求数
    fetch('/api/routes/summary').then(function (r) { return r.json(); }).then(function (d) {
      var b = document.getElementById('tab-route-badge');
      if (!b) return;
      var n = d.open_needs || 0;
      b.textContent = n ? String(n) : '';
      b.hidden = !n;
    }).catch(function () {});
  }
  tick();
  setInterval(tick, 20000);
})();
// 顶栏 📱：桌面调试用——同页套一个 390 宽 iframe（媒体查询按 iframe 宽度生效，
// 等于真机布局）；✕ 退出、↻ 重载。同源 iframe 共享 localStorage（主题等一致）。
(function () {
  var btn = document.getElementById('phone-btn');
  if (!btn) return;
  btn.addEventListener('click', function () {
    if (document.getElementById('mp-phone-overlay')) return;
    var ov = document.createElement('div');
    ov.id = 'mp-phone-overlay';
    ov.className = 'mp-phone-overlay';
    var bar = document.createElement('div');
    bar.className = 'mp-phone-bar';
    var ttl = document.createElement('span');
    ttl.className = 'mp-phone-ttl';
    ttl.textContent = '📱 手机预览 · 390';
    var reload = document.createElement('button');
    reload.type = 'button';
    reload.className = 'mp-phone-x';
    reload.textContent = '↻';
    reload.title = '重新加载';
    var close = document.createElement('button');
    close.type = 'button';
    close.className = 'mp-phone-x';
    close.textContent = '✕';
    close.title = '退出手机预览';
    var wrap = document.createElement('div');
    wrap.className = 'mp-phone-frame';
    var fr = document.createElement('iframe');
    fr.src = location.pathname + location.search;
    fr.title = '手机预览';
    wrap.appendChild(fr);
    bar.appendChild(ttl);
    bar.appendChild(reload);
    bar.appendChild(close);
    ov.appendChild(bar);
    ov.appendChild(wrap);
    document.body.appendChild(ov);
    reload.addEventListener('click', function () { fr.src = fr.src; });
    close.addEventListener('click', function () { ov.remove(); });
  });
})();
</script>
{{end}}

{{define "pxLighthouse"}}
<svg class="px-lighthouse" viewBox="0 0 16 16" shape-rendering="crispEdges" aria-hidden="true">
  <g class="beam-blink">
    <path class="px-beam" d="M0 2h7v1H0zM9 2h7v1H9zM0 4h8v1H0zM8 4h8v1H8z"/>
  </g>
  <rect x="7" y="1" width="2" height="1" class="px-cap"/>
  <rect x="6" y="2" width="4" height="1" class="px-cap"/>
  <rect x="6" y="3" width="1" height="1" class="px-frame"/>
  <rect x="7" y="3" width="2" height="1" class="px-lamp lamp-blink"/>
  <rect x="9" y="3" width="1" height="1" class="px-frame"/>
  <rect x="6" y="4" width="4" height="1" class="px-frame"/>
  <rect x="6" y="5" width="4" height="2" class="px-tower"/>
  <rect x="6" y="7" width="4" height="1" class="px-stripe"/>
  <rect x="5" y="8" width="6" height="1" class="px-tower"/>
  <rect x="5" y="9" width="6" height="1" class="px-stripe"/>
  <rect x="5" y="10" width="6" height="1" class="px-tower"/>
  <rect x="4" y="11" width="8" height="1" class="px-stripe"/>
  <rect x="4" y="12" width="8" height="1" class="px-tower"/>
  <rect x="3" y="13" width="10" height="1" class="px-base"/>
  <rect x="3" y="14" width="10" height="1" class="px-base"/>
  <rect x="3" y="15" width="10" height="1" class="px-base-dark"/>
</svg>
{{end}}

{{/* 顶栏中间的微型海景：一艘 Q 版集装箱货轮在港内往返，左岸迷你吊机轻吊货箱。
     收进顶栏一行，不再占独立横带。viewBox 宽 240 = 远波周期 10 × 近波周期 8 的公倍数，横移无缝。 */}}
{{define "topSea"}}
<svg class="mini-sea" viewBox="0 0 240 40" preserveAspectRatio="xMidYMax meet" shape-rendering="crispEdges" aria-hidden="true">
  <!-- 左岸迷你码头 + 小吊机（窄屏隐藏，只留船和水） -->
  <g class="mini-port">
    <rect x="-6" y="26" width="58" height="3" class="px-wood"/>
    <rect x="-6" y="29" width="58" height="1" class="px-wood-dark"/>
    <rect x="6" y="29" width="3" height="7" class="px-wood-dark"/>
    <rect x="24" y="29" width="3" height="7" class="px-wood-dark"/>
    <rect x="42" y="29" width="3" height="7" class="px-wood-dark"/>
    <rect x="14" y="12" width="4" height="15" class="px-wood"/>
    <rect x="10" y="9" width="42" height="4" class="px-wood"/>
    <rect x="10" y="13" width="42" height="1" class="px-wood-dark"/>
    <rect x="12" y="9" width="3" height="2" class="px-lamp lamp-soft"/>
    <rect x="31" y="13" width="1" height="7" class="px-rope"/>
    <g class="mini-lift">
      <rect x="27" y="20" width="9" height="5" class="px-cargo"/>
      <rect x="27" y="20" width="9" height="1" class="px-crate-band"/>
      <rect x="31" y="20" width="1" height="5" class="px-crate-band"/>
    </g>
  </g>

  <!-- 水（波浪路径由周期单元铺满 240 宽，CSS 按单元宽度横移循环） -->
  <rect x="0" y="30" width="240" height="10" class="px-water-deep"/>
  <path class="mini-wave-far" d="M0 30h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4h3v-1h3v1h4v1h-240z"/>
  <path class="mini-wave-near" d="M0 33h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4h2v-1h2v1h4v2h-240z"/>

  <!-- Q 版集装箱货轮：港内往返 + 随浪轻晃（内联船体：<use> 影子树吃不到外部 class） -->
  <g class="mini-ship"><g class="mini-bob"><g transform="translate(0,12) scale(1.2)">
    <rect class="px-smoke s1" x="1" y="4" width="2" height="2"/>
    <rect class="px-smoke s2" x="3" y="2" width="2" height="2"/>
    <rect x="2" y="6" width="2" height="2" class="px-stack"/>
    <rect x="1" y="8" width="4" height="3" class="px-cabin"/>
    <rect x="2" y="9" width="2" height="1" class="px-frame"/>
    <rect x="6" y="8" width="4" height="3" class="px-box-a"/>
    <rect x="6" y="8" width="4" height="1" class="px-box-band"/>
    <rect x="7" y="8" width="1" height="3" class="px-box-band"/>
    <rect x="11" y="8" width="4" height="3" class="px-box-b"/>
    <rect x="11" y="8" width="4" height="1" class="px-box-band"/>
    <rect x="12" y="8" width="1" height="3" class="px-box-band"/>
    <rect x="16" y="8" width="4" height="3" class="px-box-d"/>
    <rect x="16" y="8" width="4" height="1" class="px-box-band"/>
    <rect x="17" y="8" width="1" height="3" class="px-box-band"/>
    <rect x="11" y="5" width="4" height="3" class="px-box-c"/>
    <rect x="11" y="5" width="4" height="1" class="px-box-band"/>
    <rect x="12" y="5" width="1" height="3" class="px-box-band"/>
    <rect x="0" y="11" width="22" height="1" class="px-hull"/>
    <rect x="0" y="12" width="22" height="1" class="px-hull"/>
    <rect x="1" y="13" width="20" height="1" class="px-hull"/>
    <rect x="3" y="14" width="16" height="1" class="px-hull-dark"/>
  </g></g></g>
</svg>
{{end}}

{{/* 指派目标：任意会话 / 每个具体会话（带简介，在线与离线都列）/ 新项目新开 */}}
{{define "berthoptions"}}
<option value="{{.Name}}">{{.Short}} · 任意会话{{if .Online}}（有在线会话）{{else}}（当前不在线）{{end}}</option>
{{range .Sessions}}{{if .Dir}}<option value="{{$.Name}}|{{.Dir}}">{{$.Short}} · {{if .Name}}「{{.Name}}」{{end}}📍{{baseName .Dir}}{{if .Online}}{{if .Task}} · 在忙 {{.Task}}{{else if eq .RunState "blocked"}} · 受阻{{else if eq .RunState "working"}} · 工作中{{else}} · 在线{{end}}{{else}} · 上次在线 {{ago .Seen}}（新开即领取）{{end}}</option>{{end}}{{end}}
<option value="{{.Name}}|NEW" data-new="1">🆕 {{.Short}} · 在新项目里新开会话来做…</option>
{{end}}

{{define "shipoptions"}}
{{range .}}{{template "berthoptions" .}}{{end}}{{end}}

{{/* 重新指派下拉：排除当前助手的「任意会话」与当前工作目录的会话，同助手其他会话（含离线/新开）仍可指派 */}}
{{define "reassignoptions"}}
{{$cur := .R.Meta.AssigneeValue}}
{{$cwd := .R.Meta.AssignCwd}}
{{range $ship := .Ships}}
{{if ne $ship.Name $cur}}<option value="{{$ship.Name}}">{{$ship.Short}} · 任意会话{{if $ship.Online}}（有在线会话）{{else}}（当前不在线）{{end}}</option>{{end}}
{{range $s := $ship.Sessions}}{{if and $s.Dir (or (ne $ship.Name $cur) (ne $s.Dir $cwd))}}<option value="{{$ship.Name}}|{{$s.Dir}}">{{$ship.Short}} · {{if $s.Name}}「{{$s.Name}}」{{end}}📍{{baseName $s.Dir}}{{if $s.Online}}{{if $s.Task}} · 在忙 {{$s.Task}}{{else if eq $s.RunState "blocked"}} · 受阻{{else if eq $s.RunState "working"}} · 工作中{{else}} · 在线{{end}}{{else}} · 上次在线 {{ago $s.Seen}}（新开即领取）{{end}}</option>{{end}}{{end}}
<option value="{{$ship.Name}}|NEW" data-new="1">🆕 {{$ship.Short}} · 在新项目里新开会话来做…</option>
{{end}}{{end}}

{{/* 看板上一张任务卡：dot = dict("T" 任务记录, "Board" 看板根数据) */}}
{{define "taskcard"}}
{{$t := .T}}
<article class="card status-{{$t.Meta.Status}} {{impClass $t.Meta.Importance}}" data-task="{{$t.Meta.ID}}">
  <div class="card-head">
    <span class="id">{{$t.Meta.ID}}</span>
    <span class="imp">● {{impZH $t.Meta.Importance}}</span>
    {{if eq $t.Meta.Status "blocked"}}<span class="badge block">受阻</span>{{end}}
    {{if $t.Meta.Assignee}}<span class="assignee">🚢 {{actorName $t.Meta.AssigneeValue}}{{if $t.Meta.AssignCwd}} · 📍{{baseName $t.Meta.AssignCwd}}{{end}}</span>{{end}}
    {{if $t.Meta.Session}}<a class="card-fleet" href="/sessions/{{$t.Meta.Session}}{{if $t.Meta.SessionDir}}?dir={{$t.Meta.SessionDir | urlquery}}{{end}}" title="关联在这个会话上">🧷 {{slice $t.Meta.Session 0 8}}</a>{{end}}
  </div>
  <a class="titlelink" href="/tasks/{{$t.Meta.ID}}">{{$t.Meta.Title}}</a>
  {{if $t.Meta.Goal}}<div class="goal">🎯 {{$t.Meta.Goal}}</div>{{end}}
  <div class="agent-line">{{with index .Board.ByTask $t.Meta.ID}}<i class="dot"></i>{{actorName .Actor}} 正在做{{if .Note}}：{{.Note}}{{end}}{{end}}</div>
  {{if eq $t.Meta.Status "anchored"}}
  <form method="post" action="/tasks/{{$t.Meta.ID}}/assign" class="publish-row">
    <select name="assignee" aria-label="指派给谁">
      {{template "shipoptions" .Board.Ships}}
    </select>
    <button class="mini" data-confirm="指派：{{$t.Meta.ID}} 交给所选助手？只有它能领取" data-debug="dispatch">指派</button>
  </form>
  <form method="post" action="/tasks/{{$t.Meta.ID}}/delete" class="del-form card-del">
    <button class="danger mini" data-confirm="删除任务 {{$t.Meta.ID}}？它还在待命、还没开始，删除后不可恢复">🗑 删掉</button>
  </form>
  {{end}}
</article>
{{end}}`

const sseBoot = `
<script>
if ('serviceWorker' in navigator) navigator.serviceWorker.register('/sw.js');
// 港口配色选择（4 主题；选择持久化，head 内联脚本已提前设置 data-theme，无闪烁）
(function () {
  var root = document.documentElement;
  var meta = document.querySelector('meta[name="theme-color"]');
  var colors = { night: '#221b14', day: '#f1e6cf', abyss: '#0b1c2c', porcelain: '#eef3f4' };
  function apply(t) {
    if (!colors[t]) t = 'night';
    root.dataset.theme = t;
    if (meta) meta.setAttribute('content', colors[t]);
    document.querySelectorAll('.theme-opt').forEach(function (b) {
      b.setAttribute('aria-checked', b.dataset.theme === t ? 'true' : 'false');
    });
    try { localStorage.setItem('mindport-theme', t); } catch (e) {}
  }
  document.addEventListener('click', function (e) {
    var b = e.target.closest && e.target.closest('.theme-opt');
    if (b) apply(b.dataset.theme);
  });
  apply(root.dataset.theme);
})();
// 防老板皮相：顶栏一个 </> 按钮，单击切到黑白 debug 台；状态存 sessionStorage
// （刷新不露馅，关标签即恢复）。不用 Esc、不用连点。
(function () {
  var btn = document.getElementById('stealth-btn');
  if (!btn) return;
  function on() { return document.documentElement.dataset.stealth === '1'; }
  function sync() {
    btn.classList.toggle('on', on());
    btn.setAttribute('aria-label', on() ? 'exit debug view' : 'dev view');
    btn.title = on() ? '退出' : '';
  }
  sync();
  btn.addEventListener('click', function () {
    try {
      if (on()) sessionStorage.removeItem('mpdbg');
      else sessionStorage.setItem('mpdbg', '1');
    } catch (e) {}
    location.reload();
  });
  if (on()) {
    var cap = document.querySelector('.capture-row input[name="title"]');
    if (cap) cap.placeholder = 'new task title …';
    var lg = document.querySelector('.logform input[name="line"]');
    if (lg) lg.placeholder = 'append log line …';
  }
})();
// 助手/配色弹层：点外部或按 Esc 收起（展开状态由 details 持有，20s 轮询只换助手内部列表，不丢状态）
document.addEventListener('click', function (e) {
  document.querySelectorAll('details.pop').forEach(function (d) {
    if (d.open && !d.contains(e.target)) d.open = false;
  });
});
document.addEventListener('keydown', function (e) {
  if (e.key === 'Escape') document.querySelectorAll('details.pop').forEach(function (d) { d.open = false; });
});
(function () {
  var es = new EventSource('/events');
  es.addEventListener('changed', function () {
    var t = document.activeElement && document.activeElement.tagName;
    if (t === 'INPUT' || t === 'TEXTAREA' || t === 'SELECT') return;
    location.reload();
  });
})();
// 重要操作二次确认，防手机误触
document.addEventListener('click', function (e) {
  var el = e.target.closest('[data-confirm]');
  if (el && !window.confirm(el.getAttribute('data-confirm'))) e.preventDefault();
});
// 「打磨任务单」锚点：滚到编辑区并展开 details
document.addEventListener('click', function (e) {
  var a = e.target.closest('a.btn-edit[href^="#"]');
  if (!a) return;
  var d = document.querySelector(a.getAttribute('href'));
  if (d) d.open = true;
});
// 指派时选「🆕 新项目新开」：先问完整目录路径，再把占位值 actor|NEW 换成 actor|路径
document.addEventListener('submit', function (e) {
  var f = e.target;
  if (!f || !f.querySelector) return;
  var sel = f.querySelector('select[name="assignee"]');
  var opt = sel && sel.selectedOptions && sel.selectedOptions[0];
  if (!opt || !opt.hasAttribute('data-new')) return;
  var dir = window.prompt('新工作目录的完整项目路径，例如：\n/Users/jmzx/studio/xianyu\n\n指派后，在该目录开一个 Claude Code 会话即可领取');
  if (dir === null) { e.preventDefault(); return; }
  dir = dir.trim();
  if (!/^\//.test(dir) && !/^[A-Za-z]:[\\/]/.test(dir)) {
    window.alert('要填完整路径（从 / 或盘符开始），不能只写目录名');
    e.preventDefault();
    return;
  }
  sel.value = sel.value.replace(/\|NEW$/, '|' + dir);
}, true);
// 助手在线数由底部 tab bar 的统一轮询驱动（mp-sessions 事件），这里只负责点亮
// 顶栏助手灯；/api/agents 20s 轮询给看板卡片显示「谁正在做哪个任务」。本地刷新不 reload。
(function () {
  function actorName(a) { var i = a.indexOf('@'); return i > 0 ? a.slice(0, i) : a; }
  window.addEventListener('mp-sessions', function (e) {
    var live = e.detail.live;
    var fleet = document.getElementById('fleet');
    if (!fleet) return;
    fleet.classList.toggle('has-live', live > 0);
    fleet.title = live + ' 位助手工作中 · 点开看全部会话';
    var cnt = document.getElementById('fleet-count');
    if (cnt) cnt.textContent = live ? String(live) : '';
  });
  function tickAgents() {
    fetch('/api/agents').then(function (r) { return r.json(); }).then(function (agents) {
      var byTask = {};
      agents.forEach(function (p) { if (p.online && p.task) byTask[p.task] = p; });
      document.querySelectorAll('[data-task]').forEach(function (card) {
        var slot = card.querySelector('.agent-line');
        if (!slot) return;
        var p = byTask[card.getAttribute('data-task')];
        slot.innerHTML = p ? '<i class="dot"></i>' + actorName(p.actor) + ' 正在做' +
          (p.note ? '：' + p.note : '') : '';
        card.classList.toggle('has-agent', !!p);
      });
    }).catch(function () {});
  }
  tickAgents();
  setInterval(tickAgents, 20000);
})();
</script>`

const boardHTML = head + `
{{template "topbar" .}}

{{if .Error}}<div class="err">⚠️ {{.Error}}</div>{{end}}

<section class="capture">
  <form method="post" action="/tasks" class="capture-row">
    <input name="title" placeholder="想到什么先记下来…自动建成任务草稿（随后补验收/指派）" autocomplete="off" autofocus required>
    <button type="submit" class="primary" data-debug="add">记下</button>
  </form>
  <p class="capture-hint" data-debug="ideas only">想到就先记下来 · 任务可以关联到具体会话跟着走 · 做完一起看结果 · 只问重要不谈紧急</p>
</section>

{{/* 手机端类 app 分段：一次只看一个区，取代四列纵向长滚动；桌面端隐藏，仍是看板 */}}
<nav class="board-seg" id="board-seg" aria-label="任务分区">
  <button type="button" data-seg="drafts" data-n="{{len .Drafts}}"><span class="seg-label">📝 草稿</span><i class="seg-n">{{len .Drafts}}</i></button>
  {{range .Cols}}<button type="button" data-seg="{{.Key}}" data-n="{{len .Tasks}}"><span class="seg-label">{{.Seg}}</span><i class="seg-n">{{len .Tasks}}</i></button>{{end}}
</nav>

{{if .Drafts}}
<section class="drafts">
  <h2 data-debug="drafts">📝 草稿·准备中 <small>{{len .Drafts}}</small></h2>
  <div class="draft-grid">
  {{range .Drafts}}
    <article class="card draft-card {{impClass .Meta.Importance}}" data-task="{{.Meta.ID}}">
      <div class="card-head">
        <span class="id">{{.Meta.ID}}</span>
        <span class="imp">● {{impZH .Meta.Importance}}</span>
      </div>
      <a class="titlelink" href="/tasks/{{.Meta.ID}}">{{.Meta.Title}}</a>
      {{if .Meta.Goal}}<div class="goal">🎯 {{.Meta.Goal}}</div>{{end}}
      <div class="draft-actions">
        <form method="post" action="/tasks/{{.Meta.ID}}/publish" class="publish-row">
          <select name="assignee" aria-label="放行去向">
            <option value="">⏸ 放进待命区</option>
            {{template "shipoptions" $.Ships}}
          </select>
          <button class="primary mini" data-confirm="确认放行 {{.Meta.ID}}？" data-debug="publish">放行</button>
        </form>
        <form method="post" action="/tasks/{{.Meta.ID}}/delete" class="del-form">
          <button class="danger mini" data-confirm="删除草稿 {{.Meta.ID}}？删除后不可恢复">🗑</button>
        </form>
      </div>
    </article>
  {{end}}
  </div>
</section>
{{end}}

<section class="board four">
  {{range .Cols}}
  <div class="col col-{{.Key}}">
    {{if eq .Key "done"}}
    <details class="done-fold">
      <summary class="done-fold-head" data-debug="{{.Key}}">
        <span class="done-fold-title">{{.Title}} <small>{{len .Tasks}}</small></span>
        <i class="fold-ic" aria-hidden="true">▸</i>
      </summary>
      <div class="done-fold-body">
        {{if not .Tasks}}<p class="empty">—</p>{{end}}
        {{range .Tasks}}{{template "taskcard" (dict "T" . "Board" $)}}{{end}}
      </div>
    </details>
    {{else}}
    <h2 data-debug="{{.Key}}">{{.Title}} <small>{{len .Tasks}}</small></h2>
    {{if not .Tasks}}<p class="empty">—</p>{{end}}
    {{range .Tasks}}{{template "taskcard" (dict "T" . "Board" $)}}{{end}}
    {{end}}
  </div>
  {{end}}
</section>
{{template "tabbar" .}}
` + boardSegJS + sseBoot + `
</body></html>`

// 看板手机端分段切换：body[data-seg] 配合 CSS 一次只显一个区；
// 选择存 localStorage（SSE 整页 reload 后保持），记忆段为空时落到第一个非空段。
const boardSegJS = `
<script>
(function () {
  var nav = document.getElementById('board-seg');
  if (!nav) return;
  var btns = Array.prototype.slice.call(nav.querySelectorAll('[data-seg]'));
  function keys() { return btns.map(function (b) { return b.dataset.seg; }); }
  function count(k) {
    var b = nav.querySelector('[data-seg="' + k + '"]');
    return parseInt(b && b.getAttribute('data-n') || '0', 10);
  }
  function apply(k, persist) {
    document.body.dataset.seg = k;
    btns.forEach(function (b) {
      var on = b.dataset.seg === k;
      b.classList.toggle('active', on);
      b.setAttribute('aria-pressed', on ? 'true' : 'false');
    });
    if (persist) { try { localStorage.setItem('mindport-board-seg', k); } catch (e) {} }
  }
  // 「已完成」折叠态记忆：默认收起，展开选择存本机（SSE 整页 reload 后保持）
  (function () {
    var fold = document.querySelector('.done-fold');
    if (!fold) return;
    var open = null;
    try { open = localStorage.getItem('mindport-done-open'); } catch (e) {}
    if (open === '1') fold.open = true;
    fold.addEventListener('toggle', function () {
      try { localStorage.setItem('mindport-done-open', fold.open ? '1' : '0'); } catch (e) {}
    });
  })();
  var saved = null;
  try { saved = localStorage.getItem('mindport-board-seg'); } catch (e) {}
  var cur = keys().indexOf(saved) >= 0 && count(saved) > 0 ? saved
    : keys().filter(function (k) { return count(k) > 0; })[0] || keys()[0];
  apply(cur, false);
  nav.addEventListener('click', function (e) {
    var b = e.target.closest && e.target.closest('[data-seg]');
    if (!b) return;
    apply(b.dataset.seg, true);
    var top = nav.getBoundingClientRect().top + window.pageYOffset - 56;
    window.scrollTo(0, Math.max(0, top));
  });
})();
</script>`

const taskHTML = head + `
{{template "topbar" .}}

<article class="detail">
  <div class="detail-head {{impClass .R.Meta.Importance}}">
    <a class="back" href="/">← 回首页</a>
    <h1><span class="id">{{.R.Meta.ID}}</span> {{.R.Meta.Title}}</h1>
    <div class="meta-line">
      <span class="status status-{{.R.Meta.Status}}" data-debug="{{.R.Meta.Status}}">{{statusZH .R.Meta.Status}}</span>
      <span class="imp">● {{impZH .R.Meta.Importance}}</span>
      {{if .R.Meta.Assignee}}<span class="assignee">🚢 {{.R.Meta.AssigneeValue}}{{if .R.Meta.AssignCwd}} · 📍{{baseName .R.Meta.AssignCwd}}{{end}}</span>{{end}}
    </div>
    {{if .Agent}}
    <div class="agent-line detail-agent {{if .Agent.Online}}online{{else}}offline{{end}}">
      <i class="dot"></i>{{actorName .Agent.Actor}}{{if .Agent.Dir}} <i class="proj">📍{{baseName .Agent.Dir}}</i>{{end}}
      {{if .Agent.Online}}
        {{if eq .Agent.Task .R.Meta.ID}}正在做这个任务{{if .Agent.Note}}：{{.Agent.Note}}{{end}}
        {{else if .Agent.Task}}已在做别的任务（{{.Agent.Task}}）{{else}}在线待指派{{end}}<i class="seen">· {{ago .Agent.Seen}}</i>
      {{else}}上次在线 {{ago .Agent.Seen}}{{end}}
    </div>
    {{end}}
    {{if .R.Meta.Goal}}<div class="goal">🎯 {{.R.Meta.Goal}}</div>{{end}}
  </div>

  <div class="actions big">
    {{if eq .R.Meta.Status "draft"}}
    <div class="dispatch-box">
      <form method="post" action="/tasks/{{.R.Meta.ID}}/publish" class="publish-row">
        <input type="hidden" name="from" value="task">
        <button name="assignee" value="" class="primary" data-confirm="把 {{.R.Meta.ID}} 放进待命区？助手看不到待命中的任务" data-debug="anchor">⏸ 放进待命区</button>
      </form>
      <form method="post" action="/tasks/{{.R.Meta.ID}}/publish" class="publish-row">
        <input type="hidden" name="from" value="task">
        <select name="assignee" aria-label="直接指派">
          {{template "shipoptions" .Ships}}
        </select>
        <button class="primary" data-confirm="直接指派：{{.R.Meta.ID}} 将交给所选助手，只有它能领取" data-debug="dispatch">🚢 打磨完，直接指派</button>
      </form>
    </div>
    <p class="hint">准备中：在下方「打磨任务单」的「## 验收标准」里补好完成定义。待命中的任务助手看不到；指派是定向的，其他人不能领取。</p>
    <form method="post" action="/tasks/{{.R.Meta.ID}}/delete" class="del-form detail-del">
      <button class="danger" data-confirm="删除草稿 {{.R.Meta.ID}}？删除后不可恢复" data-debug="del">🗑 删除草稿</button>
    </form>

    {{else if eq .R.Meta.Status "anchored"}}
    <form method="post" action="/tasks/{{.R.Meta.ID}}/assign" class="publish-row dispatch-box">
      <select name="assignee" aria-label="指派给谁">
        {{template "shipoptions" .Ships}}
      </select>
      <button class="primary" data-confirm="指派：{{.R.Meta.ID}} 交给所选助手？只有它能领取" data-debug="dispatch">🚢 指派</button>
    </form>
    <p class="hint">待命中：助手在自己的终端上看不到这个任务。指派后进入「待领取」，助手用 port claim 领取后开始。</p>
    <div class="anchored-tools">
      <a class="btn-edit" href="#edit">✏️ 重新打磨任务单（指派前都能改）</a>
      <form method="post" action="/tasks/{{.R.Meta.ID}}/delete" class="del-form detail-del">
        <button class="danger mini" data-confirm="删除任务 {{.R.Meta.ID}}？它还在待命、还没开始，删除后不可恢复" data-debug="del">🗑 删除</button>
      </form>
    </div>

    {{else if eq .R.Meta.Status "ready"}}
    <form method="post" action="/tasks/{{.R.Meta.ID}}/assign" class="publish-row dispatch-box">
      <select name="assignee" aria-label="重新指给谁">
        {{template "reassignoptions" .}}
      </select>
      <button data-confirm="重新指派 {{.R.Meta.ID}}？原助手将不能再领取" data-debug="reassign">重新指派</button>
    </form>
    <p class="hint">已指派 · 等待 <b>{{.R.Meta.AssigneeValue}}</b>{{if .R.Meta.AssignCwd}} 在 <b>📍{{baseName .R.Meta.AssignCwd}}</b> 项目的会话{{end}}里 <code>port claim {{.R.Meta.ID}}</code> 领取。指派时可点名具体项目工作目录，防止别的项目会话领错任务。网页不能代领。</p>

    {{else if eq .R.Meta.Status "in_progress"}}
    <p class="banner sea">🌊 <b>{{.R.Meta.AssigneeValue}}</b> 已领取并开始。领取/完成/受阻只能由它在 CLI 操作；你在这里看进展、验收、批注。</p>

    {{else if eq .R.Meta.Status "blocked"}}
    <p class="banner block">⛔ <b>{{.R.Meta.AssigneeValue}}</b>{{if .R.Meta.AssignCwd}}（📍{{baseName .R.Meta.AssignCwd}}）{{end}}上报受阻（见日志）。可批注指示，或把任务重新指派给别的助手/工作目录：</p>
    <form method="post" action="/tasks/{{.R.Meta.ID}}/assign" class="publish-row dispatch-box">
      <select name="assignee" aria-label="重新指给谁">
        {{template "reassignoptions" .}}
      </select>
      <button data-confirm="受阻后重新指派：{{.R.Meta.ID}} 从原助手改派给所选目标？" data-debug="reassign">受阻重新指派</button>
    </form>

    {{else if eq .R.Meta.Status "done"}}
    <p class="banner done">🏁 已完成。对照验收标准逐项验收，有问题在此批注，助手做下一个任务时会看到。</p>
    {{end}}
  </div>

  <section class="fleet-bind">
    <h2 data-debug="bind fleet">🧷 关联到哪个会话 <small>把这个任务关联到一个具体会话，跟着它推进</small></h2>
    {{if .BoundSID}}
    <div class="bound-line">
      <a class="bound-fleet" href="/sessions/{{.BoundSID}}{{if .BoundDir}}?dir={{.BoundDir | urlquery}}{{end}}">
        <i class="dot"></i><b>{{.BoundTitle}}</b><em>{{.BoundStateText}}</em>
      </a>
      <form method="post" action="/tasks/{{.R.Meta.ID}}/unbind" class="unbind-form">
        <button class="mini" data-confirm="把 {{.R.Meta.ID}} 从这个会话取下？">取下</button>
      </form>
    </div>
    {{else if .FleetOpts}}
    <form method="post" action="/tasks/{{.R.Meta.ID}}/bind" class="bind-row">
      <select name="fleet" aria-label="关联到哪个会话">
        {{range .FleetOpts}}<option value="{{.SID}}|{{.Dir}}">{{if .Title}}{{.Title}}{{else}}{{.Short}}{{end}} · {{.StateText}}{{if .Proj}} · 📁{{.Proj}}{{end}}</option>{{end}}
      </select>
      <button class="mini" data-debug="bind">🧷 关联到这个会话</button>
    </form>
    {{else}}
    <p class="hint">本机还扫不到 Claude 会话。在项目目录里开一个 Claude，或先去助手页贴目录扫一下。</p>
    {{end}}
    <details class="launch-box" {{if not .BoundSID}}open{{end}}>
      <summary>🆕 新开一个会话来做 <small>在项目目录里起一个新的 Claude 会话，自动关联本任务</small></summary>
      <form action="/tasks/{{.R.Meta.ID}}/launch" class="launch-form" data-fleet-new>
        <label>会话开在哪个项目目录（绝对路径）</label>
        <input name="dir" list="launch-dir-list" required spellcheck="false" autocomplete="off"
               value="{{.LaunchDirDefault}}" placeholder="/Users/jmzx/Documents/GitHub/xxx" enterkeyhint="go">
        <datalist id="launch-dir-list">{{range .LaunchDirs}}<option value="{{.Dir}}">{{end}}</datalist>
        <div class="launch-name-row">
          <span class="launch-name-cell">
            <label>会话名（留空用「{{.LaunchName}}」）</label>
            <input name="name" maxlength="40" spellcheck="false" autocomplete="off" placeholder="{{.LaunchName}}">
          </span>
          {{if .LaunchRoutes}}
          <span class="launch-name-cell">
            <label>挂到哪个目标线</label>
            <select name="route" aria-label="挂到哪个目标线">
              <option value="">不挂目标线</option>
              {{$cur := .R.Meta.Route}}
              {{range .LaunchRoutes}}<option value="{{.Meta.ID}}" {{if eq .Meta.ID $cur}}selected{{end}}>{{.Meta.Code}} · {{.Meta.Title}}</option>{{end}}
            </select>
          </span>
          {{end}}
        </div>
        <label>给新会话的第一句话（它一开工就看到，可改）</label>
        <textarea name="msg" rows="5" maxlength="8000">{{.LaunchPrompt}}</textarea>
        <div class="fn-foot">
          <span class="fn-err" data-fn-err role="alert" hidden></span>
          <button class="primary mini" type="submit"><span class="emo">⛵ </span>新开会话并关联</button>
        </div>
      </form>
    </details>
  </section>

  {{if or (eq .R.Meta.Status "draft") (eq .R.Meta.Status "anchored")}}
  <details class="edit" id="edit" {{if eq .R.Meta.Status "draft"}}open{{end}}>
    <summary>✏️ 打磨任务单：标题 / 正文 / 验收标准 / 重要程度 / 目标 <small>指派后锁定（点此折叠）</small></summary>
    <form method="post" action="/tasks/{{.R.Meta.ID}}/edit">
      <label>标题</label>
      <input name="title" value="{{.R.Meta.Title}}" required>
      <label>重要程度（只问重要，不谈紧急）</label>
      <div class="seg">
        {{$imp := .R.Meta.Importance}}
        <label><input type="radio" name="importance" value="critical" {{if eq $imp "critical"}}checked{{end}}>关键</label>
        <label><input type="radio" name="importance" value="high" {{if eq $imp "high"}}checked{{end}}>重要</label>
        <label><input type="radio" name="importance" value="normal" {{if eq $imp "normal"}}checked{{end}}>普通</label>
        <label><input type="radio" name="importance" value="someday" {{if eq $imp "someday"}}checked{{end}}>也许</label>
      </div>
      <label>服务的目标</label>
      <input name="goal" value="{{.R.Meta.Goal}}" placeholder="这个任务为哪个目标服务？">
      <label>正文（Markdown；在「## 验收标准」下加 - [ ] 即新增验收项）</label>
      <textarea name="body" rows="16">{{.R.Body}}</textarea>
      <button type="submit" class="primary" data-debug="save">保存打磨结果</button>
    </form>
  </details>
  {{end}}

  {{if and .Accept (ne .R.Meta.Status "draft") (ne .R.Meta.Status "anchored")}}
  <section>
    <h2>验收标准 <small>{{.AcceptDone}}/{{len .Accept}}</small></h2>
    <ul class="accept">
      {{range .Accept}}
      <li class="{{if .Checked}}done{{end}}">
        <form method="post" action="/tasks/{{$.R.Meta.ID}}/check" class="checkform">
          <input type="hidden" name="n" value="{{.N}}">
          <input type="hidden" name="checked" value="{{if .Checked}}0{{else}}1{{end}}">
          <button class="checkbox {{if .Checked}}on{{end}}" aria-label="切换验收项">{{if .Checked}}✓{{end}}</button>
        </form>
        <span class="accept-text">{{.HTML}}</span>
      </li>
      {{end}}
    </ul>
  </section>
  {{end}}

  {{if .DescriptionHTML}}
  <section>
    <h2>描述</h2>
    <div class="md">{{.DescriptionHTML}}</div>
  </section>
  {{end}}

  {{if .Extra}}<section><div class="md">{{.Extra}}</div></section>{{end}}

  <section>
    <h2>过程日志</h2>
    <form method="post" action="/tasks/{{.R.Meta.ID}}/log" class="logform">
      <input name="line" placeholder="批注一笔…（助手用 port log 汇报进展）" autocomplete="off" required>
      <button data-debug="append">批注</button>
    </form>
    <ul class="logs">
      {{range .Logs}}<li>{{.}}</li>{{end}}
    </ul>
  </section>
</article>
<script src="/static/fleet-new.js?v=22"></script>
{{template "tabbar" .}}
` + sseBoot + `
</body></html>`
