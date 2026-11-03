package server

// 目标列表页：既定目标一览 + 新建目标。
const routeListHTML = head + `
{{template "topbar" .}}
<main class="routes-page">
  <h1 class="page-title"><span class="emo">🧭</span> 目标</h1>
  <p class="page-sub">既定的、反复推进的目标。目标下面放临时性小需求，干完即收起；
  为目标开的会话统一叫「短码-需求」，完事像 git 分支一样抛掉。</p>

  {{if .Error}}<div class="dir-err" data-debug="error"><span class="dir-err-txt"><span class="emo">⚠️ </span>{{.Error}}</span><button type="button" class="dir-err-x" data-err-close>✕</button></div>{{end}}

  <details class="route-new" {{if .Error}}open{{end}}>
    <summary><span class="emo">➕</span> 新开一个目标</summary>
    <form method="post" action="/routes" class="route-form">
      <div class="route-form-row">
        <input name="title" required maxlength="40" placeholder="目标名，如：数学计算测试" enterkeyhint="next">
        <input name="code" required maxlength="4" class="route-code-input" placeholder="短码" title="1-4 个字，如 数学 / 咸鱼">
      </div>
      <textarea name="desc" rows="3" maxlength="2000" placeholder="这个目标要做什么？写两句描述（可后补）"></textarea>
      <button class="primary" type="submit">新建目标</button>
    </form>
  </details>

  <div class="route-cards">
    {{range .Routes}}
    <a class="route-card {{if eq .R.Meta.Status "archived"}}route-card-archived{{end}}" href="/routes/{{.R.Meta.ID}}">
      <span class="route-code">{{.R.Meta.Code}}</span>
      <span class="route-card-main">
        <b>{{.R.Meta.Title}}</b>
        <i>{{.R.Meta.ID}}{{if eq .R.Meta.Status "archived"}} · 已归档{{end}}</i>
      </span>
      <span class="route-stats">
        {{if .LiveCount}}<em class="rs-live">⛵ {{.LiveCount}} 进行中</em>{{end}}
        {{if .OpenTasks}}<em class="rs-open">🧩 {{.OpenTasks}} 待办</em>{{end}}
        {{if .DoneTasks}}<em class="rs-done">✅ {{.DoneTasks}}</em>{{end}}
      </span>
    </a>
    {{else}}
    <p class="dir-hint">还没有目标。先开一个，比如「数学计算测试 / 数学」。</p>
    {{end}}
  </div>
</main>
{{template "tabbar" .}}
<script>
document.addEventListener('click', function (e) {
  if (e.target.closest && e.target.closest('[data-err-close]')) {
    var x = document.querySelector('.dir-err'); if (x) x.remove();
  }
});
</script>
</body></html>`

// 目标详情页：描述 + 开会话 + 临时需求（done 折叠）+ 会话。
const routeHTML = head + `
{{template "topbar" .}}
<main class="route-page" data-route="{{.R.Meta.ID}}" data-code="{{.R.Meta.Code}}">
  <p class="back-line"><a href="/routes">← 全部目标</a></p>
  <header class="route-head">
    <span class="route-code route-code-lg">{{.R.Meta.Code}}</span>
    <div>
      <h1>{{.R.Meta.Title}}</h1>
      <i>{{.R.Meta.ID}}{{if eq .R.Meta.Status "archived"}} · 已归档{{end}}</i>
    </div>
  </header>
  {{if .Error}}<div class="dir-err"><span class="dir-err-txt"><span class="emo">⚠️ </span>{{.Error}}</span><button type="button" class="dir-err-x" data-err-close>✕</button></div>{{end}}
  <section class="route-desc{{if not .DescHTML}} route-desc-empty{{end}}">
    {{if .DescHTML}}{{.DescHTML}}{{else}}<i>还没写描述。</i>{{end}}
  </section>

  <details class="route-edit-fold">
    <summary>✏️ 改名称 / 短码 / 描述</summary>
    <form method="post" action="/routes/{{.R.Meta.ID}}/edit" class="route-form">
      <div class="route-form-row">
        <input name="title" value="{{.R.Meta.Title}}" maxlength="40" required>
        <input name="code" value="{{.R.Meta.Code}}" maxlength="4" class="route-code-input" required>
      </div>
      <textarea name="desc" rows="3" maxlength="2000">{{.R.Body | routeDesc}}</textarea>
      <button class="primary" type="submit">保存</button>
    </form>
  </details>

  <section class="route-launch">
    <h2>⛵ 开一个会话</h2>
    <form id="fleet-new-form" class="fleet-new-form" data-fleet-new action="/fleets/new" method="post">
      <input type="hidden" name="route" value="{{.R.Meta.ID}}">
      <div class="fn-row">
        <input name="name" id="fn-name" required maxlength="40" placeholder="会话名（短码-需求）" value="{{.R.Meta.Code}}-">
        <input name="dir" list="known-dir-list-r" required spellcheck="false" placeholder="项目目录绝对路径">
        <datalist id="known-dir-list-r">{{range .Dirs}}<option value="{{.Dir}}">{{end}}</datalist>
      </div>
      <textarea name="msg" rows="2" maxlength="8000" placeholder="第一句话（让它干什么）。也可以先空着，新开会话后再说"></textarea>
      <div class="fn-foot">
        <span class="fn-err" id="fn-err" data-fn-err role="alert" hidden></span>
        <button class="primary" type="submit" id="fn-submit"><span class="emo">⛵ </span>新开会话</button>
      </div>
    </form>
  </section>

  <section class="route-needs">
    <h2>🧩 临时需求</h2>
    <form class="need-new" method="post" action="/routes/{{.R.Meta.ID}}/needs">
      <input name="title" required maxlength="120" placeholder="记一个这个目标下的小需求，回车存成草稿">
      <button type="submit">记</button>
    </form>
    <ul class="need-list">
      {{range .OpenTasks}}
      <li><a href="/tasks/{{.Meta.ID}}">{{.Meta.Title}}</a>
        <em class="need-status need-{{.Meta.Status}}">{{statusZH .Meta.Status}}</em></li>
      {{end}}
    </ul>
    {{if .DoneTasks}}
    <details class="need-done-fold">
      <summary>✅ 已完成 {{len .DoneTasks}} 个（点开回看）</summary>
      <ul class="need-list need-list-done">
        {{range .DoneTasks}}<li><a href="/tasks/{{.Meta.ID}}">{{.Meta.Title}}</a></li>{{end}}
      </ul>
    </details>
    {{end}}
  </section>

  <section class="route-fleets">
    <h2>⛵ 这个目标下的会话</h2>
    {{if .Fleets}}
    <div class="fleet-cards">
      {{range .Fleets}}
      <a class="fleet-card fc-{{.StateKey}}" href="/sessions/{{.FI.SID}}?dir={{.FI.Dir | urlquery}}">
        <span class="dot"></span>
        <span class="fc-main">
          <b>{{if .FI.Name}}{{.FI.Name}}{{else}}{{.ShortSID}}{{end}}</b>
          <i>{{.StateText}} · 📁 {{baseName .FI.Dir}}</i>
        </span>
        <span class="dir-go emo">→</span>
      </a>
      {{end}}
    </div>
    {{else}}
    <p class="dir-hint">还没为这个目标开过会话。</p>
    {{end}}
  </section>

  <form method="post" action="/routes/{{.R.Meta.ID}}/archive" class="route-archive-form">
    {{if eq .R.Meta.Status "archived"}}
    <input type="hidden" name="revive" value="1"><button class="ghost-btn" type="submit">↩️ 恢复目标</button>
    {{else}}
    <button class="ghost-btn" type="submit">🌫 归档目标</button>
    {{end}}
  </form>
</main>
{{template "tabbar" .}}
<script>
document.addEventListener('click', function (e) {
  if (e.target.closest && e.target.closest('[data-err-close]')) {
    var x = document.querySelector('.dir-err'); if (x) x.remove();
  }
});
</script>
<script src="/static/fleet-new.js?v=22"></script>
</body></html>`
