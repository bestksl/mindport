package server

const head = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="theme-color" content="#0f2233">
<link rel="manifest" href="/manifest.webmanifest">
<link rel="icon" href="/icon.svg">
<link rel="apple-touch-icon" href="/icon.svg">
<link rel="stylesheet" href="/static/style.css">
<title>思港 MindPort</title>
</head>
<body>`

const sseBoot = `
<script>
if ('serviceWorker' in navigator) navigator.serviceWorker.register('/sw.js');
(function () {
  var es = new EventSource('/events');
  es.addEventListener('changed', function () {
    var t = document.activeElement && document.activeElement.tagName;
    if (t === 'INPUT' || t === 'TEXTAREA' || t === 'SELECT') return; // 不打断输入
    location.reload();
  });
})();
</script>`

const boardHTML = head + `
<header class="topbar">
  <div>
    <h1>思港 <span>MindPort</span></h1>
  </div>
  <form method="post" action="/identity" class="identity">
    <input type="hidden" name="back" value="/">
    <select name="actor" onchange="this.form.submit()" aria-label="当前身份">
      <option value="jmzx@studio" {{if eq .Actor "jmzx@studio"}}selected{{end}}>👤 你本人</option>
      <option value="claude@studio" {{if eq .Actor "claude@studio"}}selected{{end}}>🤖 Claude</option>
      <option value="codex@studio" {{if eq .Actor "codex@studio"}}selected{{end}}>🤖 Codex</option>
    </select>
  </form>
</header>

{{if .Error}}<div class="err">⚠️ {{.Error}}</div>{{end}}

<section class="capture">
  <form method="post" action="/thoughts">
    <input name="title" placeholder="念头闪现，先扔进港…" autocomplete="off" autofocus required>
    <button type="submit" class="primary">入港</button>
  </form>
  <form method="post" action="/tasks" class="taskform">
    <input name="title" placeholder="或直接建一个任务…" autocomplete="off">
    <select name="importance" aria-label="重要程度">
      <option value="critical">关键</option>
      <option value="high">重要</option>
      <option value="normal" selected>普通</option>
      <option value="someday">也许</option>
    </select>
    <button type="submit">建任务</button>
  </form>
</section>

<section class="board">
  {{range .Cols}}
  <div class="col">
    <h2>{{.Title}} <small>{{len .Tasks}}</small></h2>
    {{if not .Tasks}}<p class="empty">—</p>{{end}}
    {{range .Tasks}}
    <article class="card status-{{.Meta.Status}} {{impClass .Meta.Importance}}">
      <div class="card-head">
        <span class="id">{{.Meta.ID}}</span>
        <span class="imp">● {{impZH .Meta.Importance}}</span>
        {{if eq .Meta.Status "blocked"}}<span class="badge block">受阻</span>{{end}}
        {{if .Meta.Assignee}}<span class="assignee">⛟ {{actorName .Meta.AssigneeValue}}</span>{{end}}
      </div>
      <a class="titlelink" href="/tasks/{{.Meta.ID}}">{{.Meta.Title}}</a>
      {{if .Meta.Goal}}<div class="goal">🎯 {{.Meta.Goal}}</div>{{end}}
      <div class="actions">
        {{if eq .Meta.Status "ready"}}
        <form method="post" action="/tasks/{{.Meta.ID}}/claim"><button>领取</button></form>
        {{else if eq .Meta.Status "in_progress"}}
        <form method="post" action="/tasks/{{.Meta.ID}}/done"><button class="primary">归港</button></form>
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
</section>

<footer>目标导向 · 不谈紧急 · 只问重要</footer>
` + sseBoot + `
</body></html>`

const taskHTML = head + `
<header class="topbar">
  <a class="back" href="/">← 看板</a>
  <form method="post" action="/identity" class="identity">
    <input type="hidden" name="back" value="/tasks/{{.R.Meta.ID}}">
    <select name="actor" onchange="this.form.submit()">
      <option value="jmzx@studio" {{if eq .Actor "jmzx@studio"}}selected{{end}}>👤 你本人</option>
      <option value="claude@studio" {{if eq .Actor "claude@studio"}}selected{{end}}>🤖 Claude</option>
      <option value="codex@studio" {{if eq .Actor "codex@studio"}}selected{{end}}>🤖 Codex</option>
    </select>
  </form>
</header>

<article class="detail">
  <div class="detail-head {{impClass .R.Meta.Importance}}">
    <h1><span class="id">{{.R.Meta.ID}}</span> {{.R.Meta.Title}}</h1>
    <div class="meta-line">
      <span class="status status-{{.R.Meta.Status}}">{{statusZH .R.Meta.Status}}</span>
      <span class="imp">● {{impZH .R.Meta.Importance}}</span>
      {{if .R.Meta.Assignee}}<span>⛟ {{.R.Meta.AssigneeValue}}</span>{{end}}
    </div>
    {{if .R.Meta.Goal}}<div class="goal">🎯 {{.R.Meta.Goal}}</div>{{end}}
  </div>

  <div class="actions big">
    {{if eq .R.Meta.Status "ready"}}
    <form method="post" action="/tasks/{{.R.Meta.ID}}/claim"><button class="primary">🚢 领取</button></form>
    {{else if eq .R.Meta.Status "in_progress"}}
    <form method="post" action="/tasks/{{.R.Meta.ID}}/done"><button class="primary">🏁 归港</button></form>
    <form method="post" action="/tasks/{{.R.Meta.ID}}/block" class="inline">
      <input name="reason" placeholder="阻塞原因…" required>
      <button>⛔ 受阻</button>
    </form>
    {{else if eq .R.Meta.Status "blocked"}}
    <form method="post" action="/tasks/{{.R.Meta.ID}}/unblock"><button>▶️ 解封</button></form>
    <form method="post" action="/tasks/{{.R.Meta.ID}}/done"><button class="primary">🏁 直接归港</button></form>
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
