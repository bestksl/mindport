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
<title>思港 MindPort</title>
</head>
<body>

{{define "topbar"}}
<header class="topbar">
  <a class="brand" href="/">思港 <span>MindPort</span></a>
  <div class="top-right">
    <div id="agents" class="agents">{{template "chips" .Agents}}</div>
    <form method="post" action="/identity" class="identity">
      <input type="hidden" name="back" value="{{if .IsTask}}/tasks/{{.BackID}}{{else}}/{{end}}">
      <select name="actor" onchange="this.form.submit()" aria-label="当前身份">
        <option value="jmzx@studio" {{if eq .Actor "jmzx@studio"}}selected{{end}}>👤 你本人</option>
        <option value="claude@studio" {{if eq .Actor "claude@studio"}}selected{{end}}>🤖 Claude</option>
        <option value="codex@studio" {{if eq .Actor "codex@studio"}}selected{{end}}>🤖 Codex</option>
      </select>
    </form>
  </div>
</header>
{{end}}

{{define "chips"}}
{{if not .}}<span class="chip muted" id="empty-agents">港里暂无报到的执行者</span>{{end}}
{{range .}}
<span class="chip {{if .Online}}online{{else}}offline{{end}}" data-actor="{{.Actor}}">
  <i class="dot"></i>{{actorName .Actor}}
  <em>{{if .Online}}{{if .Note}}{{.Note}}{{else}}在线{{end}}{{else}}{{ago .Seen}}在{{end}}</em>
</span>
{{end}}
{{end}}`

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
// 状态推进类操作二次确认，防手机误触
document.addEventListener('click', function (e) {
  var el = e.target.closest('[data-confirm]');
  if (el && !window.confirm(el.getAttribute('data-confirm'))) e.preventDefault();
});
// 执行者在线状态：20s 轮询 /api/agents，本地刷新顶栏灯与卡片状态行，不触发整页 reload
(function () {
  function actorName(a) { var i = a.indexOf('@'); return i > 0 ? a.slice(0, i) : a; }
  function ago(t) {
    var d = (Date.now() - new Date(t).getTime()) / 1000;
    if (d < 60) return '刚刚';
    if (d < 3600) return Math.floor(d / 60) + ' 分钟前';
    if (d < 86400) return Math.floor(d / 3600) + ' 小时前';
    return Math.floor(d / 86400) + ' 天前';
  }
  function chips(agents) {
    if (!agents.length) return '<span class="chip muted">港里暂无报到的执行者</span>';
    return agents.map(function (p) {
      var state = p.online ? (p.note || '在线') : (ago(p.seen) + '在');
      return '<span class="chip ' + (p.online ? 'online' : 'offline') + '" data-actor="' + p.actor + '">' +
        '<i class="dot"></i>' + actorName(p.actor) + '<em>' + state + '</em></span>';
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
  <form method="post" action="/thoughts" class="capture-row">
    <input name="title" placeholder="念头闪现，先扔进港…" autocomplete="off" autofocus required>
    <button type="submit" class="primary">入港</button>
  </form>
  <form method="post" action="/tasks" class="capture-row taskform">
    <input name="title" placeholder="派一个活（默认先进草稿打磨）…" autocomplete="off">
    <select name="importance" aria-label="重要程度">
      <option value="critical">关键</option>
      <option value="high">重要</option>
      <option value="normal" selected>普通</option>
      <option value="someday">也许</option>
    </select>
    <label class="publish-check"><input type="checkbox" name="publish" value="1">直接发布</label>
    <button type="submit">派活</button>
  </form>
</section>

{{if .Drafts}}
<section class="drafts">
  <h2>📝 草稿·打磨中 <small>{{len .Drafts}}</small></h2>
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
        <select name="assignee" aria-label="指派给">
          <option value="">留空 · 任领</option>
          <option value="claude@studio">指派 Claude</option>
          <option value="codex@studio">指派 Codex</option>
        </select>
        <button class="primary mini" data-confirm="确认发布 {{.Meta.ID}}？发布后执行者可领取">发布出港</button>
      </form>
    </article>
  {{end}}
  </div>
</section>
{{end}}

<section class="board">
  {{range .Cols}}
  <div class="col">
    <h2>{{.Title}} <small>{{len .Tasks}}</small></h2>
    {{if not .Tasks}}<p class="empty">—</p>{{end}}
    {{range .Tasks}}
    <article class="card status-{{.Meta.Status}} {{impClass .Meta.Importance}}" data-task="{{.Meta.ID}}">
      <div class="card-head">
        <span class="id">{{.Meta.ID}}</span>
        <span class="imp">● {{impZH .Meta.Importance}}</span>
        {{if eq .Meta.Status "blocked"}}<span class="badge block">受阻</span>{{end}}
        {{if .Meta.Assignee}}<span class="assignee">⛟ {{actorName .Meta.AssigneeValue}}</span>{{end}}
      </div>
      <a class="titlelink" href="/tasks/{{.Meta.ID}}">{{.Meta.Title}}</a>
      {{if .Meta.Goal}}<div class="goal">🎯 {{.Meta.Goal}}</div>{{end}}
      <div class="agent-line">{{with index $.ByTask .Meta.ID}}<i class="dot"></i>{{actorName .Actor}} 正在做{{if .Note}}：{{.Note}}{{end}}{{end}}</div>
      <div class="actions">
        {{if eq .Meta.Status "ready"}}
        <form method="post" action="/tasks/{{.Meta.ID}}/claim"><button class="mini">领取</button></form>
        {{else if eq .Meta.Status "in_progress"}}
        <form method="post" action="/tasks/{{.Meta.ID}}/done"><button class="primary mini" data-confirm="确认 {{.Meta.ID}} 归港？">归港</button></form>
        {{end}}
      </div>
    </article>
    {{end}}
  </div>
  {{end}}
</section>

<section class="inbox">
  <h2>🌊 Inbox 闪念 <small>{{len .Thoughts}}</small></h2>
  {{if not .Thoughts}}<p class="empty">还没有接住的闪念</p>{{end}}
  <div class="thought-list">
  {{range .Thoughts}}
  <article class="card thought">
    <div class="title">{{.Meta.Title}}</div>
    <div class="card-head">
      <span>{{.Meta.Created.Format "01-02 15:04"}}{{if .Meta.Source}} · {{.Meta.Source}}{{end}}</span>
      <span class="spacer"></span>
      <form method="post" action="/thoughts/promote">
        <input type="hidden" name="name" value="{{baseName .Path}}">
        <button type="submit" class="mini">转任务 →</button>
      </form>
    </div>
  </article>
  {{end}}
  </div>
</section>

<footer>目标导向 · 不谈紧急 · 只问重要</footer>
` + sseBoot + `
</body></html>`

const taskHTML = head + `
{{template "topbar" .}}

<article class="detail">
  <div class="detail-head {{impClass .R.Meta.Importance}}">
    <a class="back" href="/">← 看板</a>
    <h1><span class="id">{{.R.Meta.ID}}</span> {{.R.Meta.Title}}</h1>
    <div class="meta-line">
      <span class="status status-{{.R.Meta.Status}}">{{statusZH .R.Meta.Status}}</span>
      <span class="imp">● {{impZH .R.Meta.Importance}}</span>
      {{if .R.Meta.Assignee}}<span>⛟ {{.R.Meta.AssigneeValue}}</span>{{end}}
    </div>
    {{if .Agent}}
    <div class="agent-line detail-agent {{if .Agent.Online}}online{{else}}offline{{end}}">
      <i class="dot"></i>{{actorName .Agent.Actor}}
      {{if .Agent.Online}}在线{{if .Agent.Note}}：{{.Agent.Note}}{{end}}{{else}}{{ago .Agent.Seen}}还在{{end}}
    </div>
    {{end}}
    {{if .R.Meta.Goal}}<div class="goal">🎯 {{.R.Meta.Goal}}</div>{{end}}
  </div>

  <div class="actions big">
    {{if eq .R.Meta.Status "draft"}}
    <form method="post" action="/tasks/{{.R.Meta.ID}}/publish" class="publish-box">
      <input type="hidden" name="from" value="task">
      <select name="assignee" aria-label="指派给">
        <option value="">留空 · 发布后任领</option>
        <option value="claude@studio" {{if eq .R.Meta.AssigneeValue "claude@studio"}}selected{{end}}>指派 Claude</option>
        <option value="codex@studio" {{if eq .R.Meta.AssigneeValue "codex@studio"}}selected{{end}}>指派 Codex</option>
      </select>
      <button class="primary" data-confirm="确认发布 {{.R.Meta.ID}}？发布后执行者可领取">📣 发布出港</button>
    </form>
    <p class="hint">打磨中：在下方正文的「## 验收标准」里补好完成定义，再发布给执行者。</p>
    {{else if eq .R.Meta.Status "ready"}}
    <form method="post" action="/tasks/{{.R.Meta.ID}}/claim"><button class="primary">🚢 领取开工</button></form>
    {{else if eq .R.Meta.Status "in_progress"}}
    <form method="post" action="/tasks/{{.R.Meta.ID}}/done"><button class="primary" data-confirm="确认 {{.R.Meta.ID}} 归港？">🏁 归港</button></form>
    <form method="post" action="/tasks/{{.R.Meta.ID}}/block" class="inline">
      <input name="reason" placeholder="阻塞原因…" required>
      <button data-confirm="确认标记为受阻？">⛔ 受阻上浮</button>
    </form>
    {{else if eq .R.Meta.Status "blocked"}}
    <form method="post" action="/tasks/{{.R.Meta.ID}}/unblock"><button class="primary">▶️ 解封继续</button></form>
    <form method="post" action="/tasks/{{.R.Meta.ID}}/done"><button data-confirm="受阻状态下直接归港？确认">🏁 直接归港</button></form>
    {{end}}
  </div>

  {{if .Accept}}
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
      <input name="line" placeholder="记一笔进展…（agent 用 port log）" autocomplete="off" required>
      <button>记录</button>
    </form>
    <ul class="logs">
      {{range .Logs}}<li>{{.}}</li>{{end}}
    </ul>
  </section>

  <details class="edit">
    <summary>✏️ 编辑标题 / 正文 / 验收 / 重要程度 / 目标</summary>
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
      <button type="submit" class="primary">保存</button>
    </form>
  </details>
</article>
` + sseBoot + `
</body></html>`
