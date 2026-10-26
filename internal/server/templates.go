package server

const head = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="theme-color" content="#0b1c2c">
<link rel="manifest" href="/manifest.webmanifest">
<link rel="icon" href="/icon.svg">
<link rel="apple-touch-icon" href="/icon.svg">
<link rel="stylesheet" href="/static/style.css">
<title>思港 MindPort · 调度台</title>
</head>
<body>

{{define "topbar"}}
<header class="topbar">
  <a class="brand" href="/">思港 <span>MindPort</span><em>调度长官台</em></a>
  <div class="top-right">
    <div id="agents" class="agents">{{template "chips" .Agents}}</div>
  </div>
</header>
{{end}}

{{define "chips"}}
{{if not .}}<span class="chip muted" id="empty-agents">港里暂无报到的船</span>{{end}}
{{range .}}
<span class="chip {{if .Online}}online{{else}}offline{{end}}" data-actor="{{.Actor}}" title="{{if .Dir}}[{{.Dir}}] {{end}}{{if .Seen.IsZero}}从未报到{{else}}上次报到：{{.Seen.Format "01-02 15:04"}}{{end}}">
  <i class="dot"></i><b>{{actorName .Actor}}</b>{{if .Dir}}<i class="proj" title="项目目录：{{.Dir}}">📍{{baseName .Dir}}</i>{{end}}
  <em>{{if .Online}}{{if .Task}}在忙 {{.Task}}{{else if .Note}}{{.Note}}{{else}}在线待派{{end}}<i class="seen">· {{ago .Seen}}</i>{{else if .Seen.IsZero}}从未报到{{else}}上次报到 {{ago .Seen}}{{end}}</em>
</span>
{{end}}
{{end}}

{{/* 单艘船的派船目标：任意会话 / 各船位（在线与离线都列）/ 新项目新开 */}}
{{define "berthoptions"}}
<option value="{{.Name}}">{{.Short}} · 任意会话{{if .Online}}（有在线船位）{{else}}（当前未报到）{{end}}</option>
{{range .Berths}}<option value="{{$.Name}}|{{.Dir}}">{{$.Short}} · 📍{{.Base}}{{if .Online}}{{if .Task}} · 在忙 {{.Task}}{{else}} · 在线{{end}}{{else}} · 上次报到 {{ago .Seen}}（新开即接）{{end}}</option>{{end}}
<option value="{{.Name}}|NEW" data-new="1">🆕 {{.Short}} · 在新项目里新开会话接令…</option>
{{end}}

{{define "shipoptions"}}
{{range .}}{{template "berthoptions" .}}{{end}}{{end}}

{{/* 改派下拉：排除当前船的「任意会话」与当前船位，同船其他船位（含离线/新开）仍可改派 */}}
{{define "reassignoptions"}}
{{$cur := .R.Meta.AssigneeValue}}
{{$cwd := .R.Meta.AssignCwd}}
{{range $ship := .Ships}}
{{if ne $ship.Name $cur}}<option value="{{$ship.Name}}">{{$ship.Short}} · 任意会话{{if $ship.Online}}（有在线船位）{{else}}（当前未报到）{{end}}</option>{{end}}
{{range $b := $ship.Berths}}{{if or (ne $ship.Name $cur) (ne $b.Dir $cwd)}}<option value="{{$ship.Name}}|{{$b.Dir}}">{{$ship.Short}} · 📍{{$b.Base}}{{if $b.Online}}{{if $b.Task}} · 在忙 {{$b.Task}}{{else}} · 在线{{end}}{{else}} · 上次报到 {{ago $b.Seen}}（新开即接）{{end}}</option>{{end}}{{end}}
<option value="{{$ship.Name}}|NEW" data-new="1">🆕 {{$ship.Short}} · 在新项目里新开会话接令…</option>
{{end}}{{end}}`

const sseBoot = `
<script>
if ('serviceWorker' in navigator) navigator.serviceWorker.register('/sw.js');
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
        state = (p.task ? ('在忙 ' + p.task) : (p.note || '在线待派')) +
                '<i class="seen">· ' + ago(p.seen) + '</i>';
        title = '上次报到：' + new Date(p.seen).toLocaleString();
      } else if (seenZero) {
        state = '从未报到';
        title = '从未报到';
      } else {
        state = '上次报到 ' + ago(p.seen);
        title = '上次报到：' + new Date(p.seen).toLocaleString();
      }
      var proj = p.dir ? '<i class="proj" title="项目目录：' + p.dir + '">📍' + baseName(p.dir) + '</i>' : '';
      return '<span class="chip ' + (p.online ? 'online' : 'offline') + '" data-actor="' + p.actor +
        '" title="' + (p.dir ? '[' + p.dir + '] ' : '') + title + '">' +
        '<i class="dot"></i><b>' + actorName(p.actor) + '</b>' + proj + '<em>' + state + '</em></span>';
    }).join('');
  }
  function tick() {
    fetch('/api/agents').then(function (r) { return r.json(); }).then(function (agents) {
      var box = document.getElementById('agents');
      if (box) box.innerHTML = chips(agents);
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
` + sseBoot + `
</body></html>`
