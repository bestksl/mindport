package server

const head = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="theme-color" content="#221b14">
<script>(function(){try{var t=localStorage.getItem('mindport-theme');if(t==='light'||(!t&&window.matchMedia('(prefers-color-scheme: light)').matches))document.documentElement.dataset.theme='light';}catch(e){}})();</script>
<link rel="manifest" href="/manifest.webmanifest">
<link rel="icon" href="/icon.svg">
<link rel="apple-touch-icon" href="/icon.svg">
<link rel="stylesheet" href="/static/style.css">
<title>思港 MindPort · 调度台</title>
</head>
<body>

{{define "topbar"}}
<header class="topbar">
  <a class="brand" href="/">
    <span class="brand-mark">{{template "pxLighthouse" .}}</span>
    <span class="brand-text">思港 <i>MindPort</i><em>调度长官台</em></span>
  </a>
  <div class="top-right">
    <button id="theme-toggle" class="icon-btn" type="button"
            aria-label="切换到日间配色（日港）" title="切换日港 / 夜港配色">
      <svg class="icon-sun" viewBox="0 0 12 12" shape-rendering="crispEdges" aria-hidden="true">
        <rect x="4" y="4" width="4" height="4"/>
        <rect x="5" y="1" width="2" height="2"/><rect x="5" y="9" width="2" height="2"/>
        <rect x="1" y="5" width="2" height="2"/><rect x="9" y="5" width="2" height="2"/>
        <rect x="2" y="2" width="1" height="1"/><rect x="9" y="2" width="1" height="1"/>
        <rect x="2" y="9" width="1" height="1"/><rect x="9" y="9" width="1" height="1"/>
      </svg>
      <svg class="icon-moon" viewBox="0 0 12 12" shape-rendering="crispEdges" aria-hidden="true">
        <rect x="4" y="2" width="4" height="1"/><rect x="3" y="3" width="6" height="6"/>
        <rect x="4" y="9" width="4" height="1"/>
        <rect x="5" y="4" width="1" height="1"/><rect x="4" y="6" width="1" height="1"/>
      </svg>
    </button>
    <details class="fleet" id="fleet">
      <summary class="fleet-btn">
        <i class="fleet-dot" id="fleet-dot"></i><span class="fleet-label">船队</span><b id="fleet-count">…</b><i class="caret"></i>
      </summary>
      <div class="fleet-pop">
        <div class="fleet-pop-head"><span>港内船位</span><span id="fleet-sub"></span></div>
        <div id="agents" class="agents">{{template "chips" .Agents}}</div>
      </div>
    </details>
  </div>
</header>
{{end}}

{{/* 16×16 像素灯塔：塔灯按摩尔斯节奏硬闪，夜港两束扫光，日港只亮灯 */}}
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

{{/* 页脚像素码头全景：吊机来回搬货箱、两艘帆船对开、波浪横移、远岸小灯错峰闪烁 */}}
{{define "harbor"}}
<div class="harbor" aria-hidden="true">
<svg viewBox="0 0 360 96" preserveAspectRatio="xMidYMax meet" shape-rendering="crispEdges">
  <rect class="star" x="58" y="12" width="2" height="2"/>
  <rect class="star s2" x="180" y="8" width="2" height="2"/>
  <rect class="star s3" x="258" y="20" width="2" height="2"/>

  <!-- 右岸小灯塔（灯与左上角主灯错峰闪） -->
  <g class="far-light">
    <rect x="333" y="28" width="2" height="1" class="px-cap"/>
    <rect x="332" y="29" width="4" height="1" class="px-cap"/>
    <rect x="332" y="30" width="1" height="1" class="px-frame"/>
    <rect x="333" y="30" width="2" height="1" class="px-lamp lamp-blink"/>
    <rect x="335" y="30" width="1" height="1" class="px-frame"/>
    <rect x="332" y="31" width="4" height="1" class="px-frame"/>
    <rect x="333" y="32" width="2" height="5" class="px-tower"/>
    <rect x="333" y="34" width="2" height="1" class="px-stripe"/>
    <rect x="332" y="37" width="4" height="3" class="px-tower"/>
    <rect x="332" y="39" width="4" height="1" class="px-stripe"/>
    <rect x="331" y="40" width="6" height="4" class="px-tower"/>
    <rect x="331" y="43" width="6" height="1" class="px-stripe"/>
    <rect x="330" y="44" width="8" height="18" class="px-base"/>
  </g>
  <rect x="318" y="62" width="42" height="8" class="px-base-dark"/>

  <!-- 左岸码头 -->
  <rect x="0" y="56" width="120" height="6" class="px-wood"/>
  <rect x="0" y="62" width="120" height="2" class="px-wood-dark"/>
  <rect x="8" y="64" width="4" height="8" class="px-wood-dark"/>
  <rect x="28" y="64" width="4" height="9" class="px-wood-dark"/>
  <rect x="48" y="64" width="4" height="8" class="px-wood-dark"/>
  <rect x="68" y="64" width="4" height="9" class="px-wood-dark"/>
  <rect x="88" y="64" width="4" height="8" class="px-wood-dark"/>
  <rect x="108" y="64" width="4" height="9" class="px-wood-dark"/>

  <!-- 门式吊机 -->
  <rect x="12" y="14" width="6" height="42" class="px-wood"/>
  <rect x="100" y="22" width="6" height="34" class="px-wood"/>
  <rect x="10" y="10" width="106" height="6" class="px-wood"/>
  <rect x="10" y="16" width="106" height="2" class="px-wood-dark"/>
  <rect x="13" y="10" width="3" height="2" class="px-lamp lamp-soft"/>
  <rect x="12" y="24" width="6" height="3" class="px-wood-dark"/>
  <rect x="12" y="36" width="6" height="3" class="px-wood-dark"/>
  <rect x="100" y="32" width="6" height="3" class="px-wood-dark"/>
  <rect x="100" y="44" width="6" height="3" class="px-wood-dark"/>

  <!-- 滑车 + 吊索 + 货箱：横梁上往复，两端吊起放下 -->
  <g class="crane-anim">
    <rect x="0" y="16" width="12" height="4" class="px-wood-dark"/>
    <rect x="1" y="20" width="2" height="2" class="px-base-dark"/>
    <rect x="9" y="20" width="2" height="2" class="px-base-dark"/>
    <rect x="5" y="20" width="2" height="18" class="px-rope"/>
    <rect x="4" y="38" width="4" height="2" class="px-base-dark"/>
    <g class="crate">
      <rect x="-2" y="40" width="16" height="11" class="px-cargo"/>
      <rect x="-2" y="40" width="16" height="2" class="px-crate-band"/>
      <rect x="5" y="40" width="2" height="11" class="px-crate-band"/>
    </g>
  </g>

  <!-- 水面与波浪（两层反向横移，图案周期无缝） -->
  <rect x="0" y="70" width="360" height="26" class="px-water-deep"/>
  <path class="wave wave-far" d="M-16 68h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8h4v-2h4v2h8v2h-384z"/>
  <path class="wave wave-near" d="M-12 74h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6h3v-2h3v2h6v3h-372z"/>

  <!-- 两艘帆船对开（远小近大），各自轻轻摇晃。
       船体直接内联两份：<use> 影子树吃不到外部 class 选择器，会渲染成黑剪影。 -->
  <g transform="translate(0,46)"><g class="ship-a"><g class="bob-a"><g transform="scale(1.25)">
    <rect x="7" y="1" width="1" height="11" class="px-rope"/>
    <rect x="8" y="1" width="2" height="1" class="px-flag"/>
    <path class="px-sail" d="M7 2h1v1H7zM6 3h2v1H6zM5 4h3v1H5zM4 5h4v1H4zM3 6h5v2H3zM2 8h6v2H2zM1 10h7v1H1z"/>
    <rect x="2" y="11" width="12" height="1" class="px-wood"/>
    <rect x="1" y="12" width="14" height="1" class="px-wood"/>
    <rect x="2" y="13" width="12" height="1" class="px-wood-dark"/>
    <rect x="4" y="14" width="8" height="1" class="px-wood-dark"/>
  </g></g></g></g>
  <g transform="translate(0,50) scale(-1,1)"><g class="ship-b"><g class="bob-b"><g transform="scale(1.55)">
    <rect x="7" y="1" width="1" height="11" class="px-rope"/>
    <rect x="8" y="1" width="2" height="1" class="px-flag"/>
    <path class="px-sail" d="M7 2h1v1H7zM6 3h2v1H6zM5 4h3v1H5zM4 5h4v1H4zM3 6h5v2H3zM2 8h6v2H2zM1 10h7v1H1z"/>
    <rect x="2" y="11" width="12" height="1" class="px-wood"/>
    <rect x="1" y="12" width="14" height="1" class="px-wood"/>
    <rect x="2" y="13" width="12" height="1" class="px-wood-dark"/>
    <rect x="4" y="14" width="8" height="1" class="px-wood-dark"/>
  </g></g></g></g>
</svg>
</div>
{{end}}

{{define "chips"}}
{{if not .}}<span class="chip muted" id="empty-agents">港里暂无报到的船</span>{{end}}
{{range .}}
<span class="chip {{if .Online}}online{{else}}offline{{end}}" data-actor="{{.Actor}}" title="{{if .Name}}会话：{{.Name}}&#10;{{end}}{{if .Dir}}目录：{{.Dir}}{{end}}{{if .Seen.IsZero}}{{else}}&#10;上次报到：{{.Seen.Format "01-02 15:04"}}{{end}}">
  <i class="dot"></i><b>{{actorName .Actor}}</b>{{if .Dir}}<i class="proj" title="项目目录：{{.Dir}}">📍{{baseName .Dir}}</i>{{end}}{{if .Name}}<i class="sname">「{{.Name}}」</i>{{end}}
  <em>{{if .Online}}{{if .Task}}在忙 {{.Task}}{{else if .Note}}{{.Note}}{{else if eq .RunState "blocked"}}受阻{{else if eq .RunState "working"}}工作中{{else}}在线待派{{end}}<i class="seen">· {{ago .Seen}}</i>{{else if .Seen.IsZero}}从未报到{{else}}上次报到 {{ago .Seen}}{{end}}</em>
</span>
{{end}}
{{end}}

{{/* 单艘船的派船目标：任意会话 / 每个具体会话（带简介，在线与离线都列）/ 新项目新开 */}}
{{define "berthoptions"}}
<option value="{{.Name}}">{{.Short}} · 任意会话{{if .Online}}（有在线会话）{{else}}（当前未报到）{{end}}</option>
{{range .Sessions}}{{if .Dir}}<option value="{{$.Name}}|{{.Dir}}">{{$.Short}} · {{if .Name}}「{{.Name}}」{{end}}📍{{baseName .Dir}}{{if .Online}}{{if .Task}} · 在忙 {{.Task}}{{else if eq .RunState "blocked"}} · 受阻{{else if eq .RunState "working"}} · 工作中{{else}} · 在线{{end}}{{else}} · 上次报到 {{ago .Seen}}（新开即接）{{end}}</option>{{end}}{{end}}
<option value="{{.Name}}|NEW" data-new="1">🆕 {{.Short}} · 在新项目里新开会话接令…</option>
{{end}}

{{define "shipoptions"}}
{{range .}}{{template "berthoptions" .}}{{end}}{{end}}

{{/* 改派下拉：排除当前船的「任意会话」与当前船位的会话，同船其他会话（含离线/新开）仍可改派 */}}
{{define "reassignoptions"}}
{{$cur := .R.Meta.AssigneeValue}}
{{$cwd := .R.Meta.AssignCwd}}
{{range $ship := .Ships}}
{{if ne $ship.Name $cur}}<option value="{{$ship.Name}}">{{$ship.Short}} · 任意会话{{if $ship.Online}}（有在线会话）{{else}}（当前未报到）{{end}}</option>{{end}}
{{range $s := $ship.Sessions}}{{if and $s.Dir (or (ne $ship.Name $cur) (ne $s.Dir $cwd))}}<option value="{{$ship.Name}}|{{$s.Dir}}">{{$ship.Short}} · {{if $s.Name}}「{{$s.Name}}」{{end}}📍{{baseName $s.Dir}}{{if $s.Online}}{{if $s.Task}} · 在忙 {{$s.Task}}{{else if eq $s.RunState "blocked"}} · 受阻{{else if eq $s.RunState "working"}} · 工作中{{else}} · 在线{{end}}{{else}} · 上次报到 {{ago $s.Seen}}（新开即接）{{end}}</option>{{end}}{{end}}
<option value="{{$ship.Name}}|NEW" data-new="1">🆕 {{$ship.Short}} · 在新项目里新开会话接令…</option>
{{end}}{{end}}`

const sseBoot = `
<script>
if ('serviceWorker' in navigator) navigator.serviceWorker.register('/sw.js');
// 日港 / 夜港配色切换（选择持久化；head 内联脚本已提前设置 data-theme，无闪烁）
(function () {
  var root = document.documentElement;
  var btn = document.getElementById('theme-toggle');
  var meta = document.querySelector('meta[name="theme-color"]');
  function apply(t) {
    root.dataset.theme = t;
    if (meta) meta.setAttribute('content', t === 'light' ? '#f1e6cf' : '#221b14');
    if (btn) {
      btn.setAttribute('aria-label', t === 'light' ? '切换到夜间配色（夜港）' : '切换到日间配色（日港）');
      btn.title = t === 'light' ? '切回夜港配色' : '切到日港配色';
    }
    try { localStorage.setItem('mindport-theme', t); } catch (e) {}
  }
  if (btn) btn.addEventListener('click', function () {
    apply(root.dataset.theme === 'light' ? 'dark' : 'light');
  });
})();
// 船队浮层：点外部或按 Esc 收起（展开状态由 details 持有，20s 轮询只换内部列表，不丢状态）
document.addEventListener('click', function (e) {
  var f = document.getElementById('fleet');
  if (f && f.open && !f.contains(e.target)) f.open = false;
});
document.addEventListener('keydown', function (e) {
  var f = document.getElementById('fleet');
  if (e.key === 'Escape' && f) f.open = false;
});
(function () {
  var es = new EventSource('/events');
  es.addEventListener('changed', function () {
    var t = document.activeElement && document.activeElement.tagName;
    if (t === 'INPUT' || t === 'TEXTAREA' || t === 'SELECT') return;
    location.reload();
  });
})();
// 调度命令类操作二次确认，防手机误触
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
// 派船选「🆕 新项目新开」：先问完整目录路径，再把占位值 actor|NEW 换成 actor|路径
document.addEventListener('submit', function (e) {
  var f = e.target;
  if (!f || !f.querySelector) return;
  var sel = f.querySelector('select[name="assignee"]');
  var opt = sel && sel.selectedOptions && sel.selectedOptions[0];
  if (!opt || !opt.hasAttribute('data-new')) return;
  var dir = window.prompt('新船位的完整项目目录路径，例如：\n/Users/jmzx/studio/xianyu\n\n派船后在该目录开一艘船（Claude Code 会话）即可接令');
  if (dir === null) { e.preventDefault(); return; }
  dir = dir.trim();
  if (!/^\//.test(dir) && !/^[A-Za-z]:[\\/]/.test(dir)) {
    window.alert('要填完整路径（从 / 或盘符开始），不能只写目录名');
    e.preventDefault();
    return;
  }
  sel.value = sel.value.replace(/\|NEW$/, '|' + dir);
}, true);
// 船队在线状态：20s 轮询 /api/agents，本地刷新顶栏灯与卡片状态行，不触发整页 reload
(function () {
  function actorName(a) { var i = a.indexOf('@'); return i > 0 ? a.slice(0, i) : a; }
  function baseName(p) { return p ? p.replace(/\/+$/, '').split('/').pop() : ''; }
  function ago(t) {
    var d = (Date.now() - new Date(t).getTime()) / 1000;
    if (d < 60) return '刚刚';
    if (d < 3600) return Math.floor(d / 60) + ' 分钟前';
    if (d < 86400) return Math.floor(d / 3600) + ' 小时前';
    return Math.floor(d / 86400) + ' 天前';
  }
  function chips(agents) {
    if (!agents.length) return '<span class="chip muted">港里暂无报到的船</span>';
    return agents.map(function (p) {
      var state, title;
      var seenZero = !p.seen || new Date(p.seen).getFullYear() < 2000;
      if (p.online) {
        var doing = p.task ? ('在忙 ' + p.task) :
          (p.note || (p.run_state === 'blocked' ? '受阻' :
            (p.run_state === 'working' ? '工作中' : '在线待派')));
        state = doing + '<i class="seen">· ' + ago(p.seen) + '</i>';
        title = '上次报到：' + new Date(p.seen).toLocaleString();
      } else if (seenZero) {
        state = '从未报到';
        title = '从未报到';
      } else {
        state = '上次报到 ' + ago(p.seen);
        title = '上次报到：' + new Date(p.seen).toLocaleString();
      }
      var proj = p.dir ? '<i class="proj" title="项目目录：' + p.dir + '">📍' + baseName(p.dir) + '</i>' : '';
      var sname = p.name ? '<i class="sname">「' + p.name + '」</i>' : '';
      var tip = (p.name ? '会话：' + p.name + '\n' : '') + (p.dir ? '目录：' + p.dir + '\n' : '') + title;
      return '<span class="chip ' + (p.online ? 'online' : 'offline') + '" data-actor="' + p.actor +
        '" title="' + tip.replace(/"/g, '&quot;') + '">' +
        '<i class="dot"></i><b>' + actorName(p.actor) + '</b>' + proj + sname + '<em>' + state + '</em></span>';
    }).join('');
  }
  function tick() {
    fetch('/api/agents').then(function (r) { return r.json(); }).then(function (agents) {
      var box = document.getElementById('agents');
      if (box) box.innerHTML = chips(agents);
      var live = agents.filter(function (p) { return p.online; }).length;
      var fleet = document.getElementById('fleet');
      if (fleet) {
        fleet.classList.toggle('has-live', live > 0);
        var cnt = document.getElementById('fleet-count');
        var sub = document.getElementById('fleet-sub');
        if (cnt) cnt.textContent = live;
        if (sub) sub.textContent = live + ' 在线 · 共 ' + agents.length + ' 个会话';
      }
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
  tick();
  setInterval(tick, 20000);
})();
</script>`

const boardHTML = head + `
{{template "topbar" .}}

{{if .Error}}<div class="err">⚠️ {{.Error}}</div>{{end}}

<section class="capture">
  <form method="post" action="/tasks" class="capture-row">
    <input name="title" placeholder="想到什么先扔下来…入港即立箱（草稿，随后补验收/派船）" autocomplete="off" autofocus required>
    <button type="submit" class="primary">入港</button>
  </form>
</section>

{{if .Drafts}}
<section class="drafts">
  <h2>📝 草稿·备货中 <small>{{len .Drafts}}</small></h2>
  <div class="draft-grid">
  {{range .Drafts}}
    <article class="card draft-card {{impClass .Meta.Importance}}" data-task="{{.Meta.ID}}">
      <div class="card-head">
        <span class="id">{{.Meta.ID}}</span>
        <span class="imp">● {{impZH .Meta.Importance}}</span>
      </div>
      <a class="titlelink" href="/tasks/{{.Meta.ID}}">{{.Meta.Title}}</a>
      {{if .Meta.Goal}}<div class="goal">🎯 {{.Meta.Goal}}</div>{{end}}
      <form method="post" action="/tasks/{{.Meta.ID}}/publish" class="publish-row">
        <select name="assignee" aria-label="放行去向">
          <option value="">⚓ 放进锚地</option>
          {{template "shipoptions" $.Ships}}
        </select>
        <button class="primary mini" data-confirm="确认放行 {{.Meta.ID}}？">放行</button>
      </form>
    </article>
  {{end}}
  </div>
</section>
{{end}}

<section class="board four">
  {{range .Cols}}
  <div class="col col-{{.Key}}">
    <h2>{{.Title}} <small>{{len .Tasks}}</small></h2>
    {{if not .Tasks}}<p class="empty">—</p>{{end}}
    {{range .Tasks}}
    <article class="card status-{{.Meta.Status}} {{impClass .Meta.Importance}}" data-task="{{.Meta.ID}}">
      <div class="card-head">
        <span class="id">{{.Meta.ID}}</span>
        <span class="imp">● {{impZH .Meta.Importance}}</span>
        {{if eq .Meta.Status "blocked"}}<span class="badge block">受阻</span>{{end}}
        {{if .Meta.Assignee}}<span class="assignee">🚢 {{actorName .Meta.AssigneeValue}}{{if .Meta.AssignCwd}} · 📍{{baseName .Meta.AssignCwd}}{{end}}</span>{{end}}
      </div>
      <a class="titlelink" href="/tasks/{{.Meta.ID}}">{{.Meta.Title}}</a>
      {{if .Meta.Goal}}<div class="goal">🎯 {{.Meta.Goal}}</div>{{end}}
      <div class="agent-line">{{with index $.ByTask .Meta.ID}}<i class="dot"></i>{{actorName .Actor}} 正在做{{if .Note}}：{{.Note}}{{end}}{{end}}</div>
      {{if eq .Meta.Status "anchored"}}
      <form method="post" action="/tasks/{{.Meta.ID}}/assign" class="publish-row">
        <select name="assignee" aria-label="派给哪艘船">
          {{template "shipoptions" $.Ships}}
        </select>
        <button class="mini" data-confirm="派船命令：{{.Meta.ID}} 派给所选船？只有这艘船能接令">派船</button>
      </form>
      {{end}}
    </article>
    {{end}}
  </div>
  {{end}}
</section>

{{template "harbor" .}}
<footer>调度长派船 · 船出港干活 · 归港必带结果 · 目标导向不谈紧急</footer>
` + sseBoot + `
</body></html>`

const taskHTML = head + `
{{template "topbar" .}}

<article class="detail">
  <div class="detail-head {{impClass .R.Meta.Importance}}">
    <a class="back" href="/">← 调度台</a>
    <h1><span class="id">{{.R.Meta.ID}}</span> {{.R.Meta.Title}}</h1>
    <div class="meta-line">
      <span class="status status-{{.R.Meta.Status}}">{{statusZH .R.Meta.Status}}</span>
      <span class="imp">● {{impZH .R.Meta.Importance}}</span>
      {{if .R.Meta.Assignee}}<span class="assignee">🚢 {{.R.Meta.AssigneeValue}}{{if .R.Meta.AssignCwd}} · 📍{{baseName .R.Meta.AssignCwd}}{{end}}</span>{{end}}
    </div>
    {{if .Agent}}
    <div class="agent-line detail-agent {{if .Agent.Online}}online{{else}}offline{{end}}">
      <i class="dot"></i>{{actorName .Agent.Actor}}{{if .Agent.Dir}} <i class="proj">📍{{baseName .Agent.Dir}}</i>{{end}}
      {{if .Agent.Online}}
        {{if eq .Agent.Task .R.Meta.ID}}正在做本票货{{if .Agent.Note}}：{{.Agent.Note}}{{end}}
        {{else if .Agent.Task}}已在做别的票货（{{.Agent.Task}}）{{else}}在线待派{{end}}<i class="seen">· {{ago .Agent.Seen}}</i>
      {{else}}上次报到 {{ago .Agent.Seen}}{{end}}
    </div>
    {{end}}
    {{if .R.Meta.Goal}}<div class="goal">🎯 {{.R.Meta.Goal}}</div>{{end}}
  </div>

  <div class="actions big">
    {{if eq .R.Meta.Status "draft"}}
    <div class="dispatch-box">
      <form method="post" action="/tasks/{{.R.Meta.ID}}/publish" class="publish-row">
        <input type="hidden" name="from" value="task">
        <button name="assignee" value="" class="primary" data-confirm="把 {{.R.Meta.ID}} 放进锚地？船在锚地看不到货">⚓ 放进锚地</button>
      </form>
      <form method="post" action="/tasks/{{.R.Meta.ID}}/publish" class="publish-row">
        <input type="hidden" name="from" value="task">
        <select name="assignee" aria-label="直接派船">
          {{template "shipoptions" .Ships}}
        </select>
        <button class="primary" data-confirm="直接派船：{{.R.Meta.ID}} 将命令给所选船，只有它能接令">🚢 打磨完，直接派船</button>
      </form>
    </div>
    <p class="hint">备货中：在下方「打磨任务单」的「## 验收标准」里补好完成定义。进锚地的货船看不到；派船是命令，不可任领。</p>
    <a class="btn-edit" href="#edit">✏️ 打磨任务单（标题 / 验收 / 重要程度 / 目标）</a>

    {{else if eq .R.Meta.Status "anchored"}}
    <form method="post" action="/tasks/{{.R.Meta.ID}}/assign" class="publish-row dispatch-box">
      <select name="assignee" aria-label="派给哪艘船">
        {{template "shipoptions" .Ships}}
      </select>
      <button class="primary" data-confirm="派船命令：{{.R.Meta.ID}} 派给所选船？只有这艘船能接令">🚢 派船</button>
    </form>
    <p class="hint">锚地待派：船在自己的终端上看不到这票货。派船后进入「待接令」，船 port claim 接令出港。</p>
    <a class="btn-edit" href="#edit">✏️ 重新打磨任务单（派船前都能改）</a>

    {{else if eq .R.Meta.Status "ready"}}
    <form method="post" action="/tasks/{{.R.Meta.ID}}/assign" class="publish-row dispatch-box">
      <select name="assignee" aria-label="改派给哪个船位">
        {{template "reassignoptions" .}}
      </select>
      <button data-confirm="改派 {{.R.Meta.ID}}？原船位将不能再接令">改派</button>
    </form>
    <p class="hint">已派船 · 等待 <b>{{.R.Meta.AssigneeValue}}</b>{{if .R.Meta.AssignCwd}} 在 <b>📍{{baseName .R.Meta.AssignCwd}}</b> 项目的会话{{end}}里 <code>port claim {{.R.Meta.ID}}</code> 接令。派船时可点名具体项目会话，防止别的项目里的会话接错票。调度台不能代领。</p>

    {{else if eq .R.Meta.Status "in_progress"}}
    <p class="banner sea">🌊 <b>{{.R.Meta.AssigneeValue}}</b> 已接令出港。接令/归港/受阻只在船自己的 CLI 完成；调度台在这里看进展、验货、批注。</p>

    {{else if eq .R.Meta.Status "blocked"}}
    <p class="banner block">⛔ <b>{{.R.Meta.AssigneeValue}}</b>{{if .R.Meta.AssignCwd}}（📍{{baseName .R.Meta.AssignCwd}}）{{end}}上报受阻（见日志）。可批注指示，或把货改派给别的船/船位：</p>
    <form method="post" action="/tasks/{{.R.Meta.ID}}/assign" class="publish-row dispatch-box">
      <select name="assignee" aria-label="改派给哪个船位">
        {{template "reassignoptions" .}}
      </select>
      <button data-confirm="受阻改派：{{.R.Meta.ID}} 从原船位改派给所选船位？">受阻改派</button>
    </form>

    {{else if eq .R.Meta.Status "done"}}
    <p class="banner done">🏁 已归港。对照验收标准验货，有问题在此批注，船下一票货会看到。</p>
    {{end}}
  </div>

  {{if or (eq .R.Meta.Status "draft") (eq .R.Meta.Status "anchored")}}
  <details class="edit" id="edit" open>
    <summary>✏️ 打磨任务单：标题 / 正文 / 验收标准 / 重要程度 / 目标 <small>派船后封舱（点此折叠）</small></summary>
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
      <button type="submit" class="primary">保存打磨结果</button>
    </form>
  </details>
  {{end}}

  {{if and .Accept (ne .R.Meta.Status "draft") (ne .R.Meta.Status "anchored")}}
  <section>
    <h2>验收标准 · 验货 <small>{{.AcceptDone}}/{{len .Accept}}</small></h2>
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
      <input name="line" placeholder="批注一笔…（船用 port log 汇报进展）" autocomplete="off" required>
      <button>批注</button>
    </form>
    <ul class="logs">
      {{range .Logs}}<li>{{.}}</li>{{end}}
    </ul>
  </section>
</article>
{{template "harbor" .}}
` + sseBoot + `
</body></html>`
