package server

// 按目录翻会话：贴一个项目绝对路径，扫盘列出该目录全部 Claude 会话（含历史）。
const dirListHTML = head + `
{{template "topbar" .}}

<main class="dir-page">
  <form class="dir-bar" method="get" action="/sessions">
    <input type="text" name="dir" value="{{.Path}}" spellcheck="false"
      placeholder="贴项目目录绝对路径，如 /Users/jmzx/Documents/GitHub/mindport"
      aria-label="项目目录" autofocus>
    <button type="submit" class="primary" data-debug="open"><span class="emo">📂 </span>翻会话</button>
  </form>

  {{if .Error}}<div class="dir-err" data-debug="error"><span class="emo">⚠️ </span>{{.Error}}</div>{{end}}

  {{if .NoDir}}
  <section class="dir-known">
    <h2 data-debug="berths">港内已知船位（心跳里出现过的目录）</h2>
    {{if .KnownDirs}}
    <div class="dir-chips">
      {{range .KnownDirs}}
      <a class="dir-chip" href="/sessions?dir={{.Dir | urlquery}}">
        <span class="emo">📍</span> <b>{{.Short}}</b><i title="{{.Dir}}">{{.Dir}}</i>
      </a>
      {{end}}
    </div>
    {{else}}
    <p class="dir-hint" data-debug="no heartbeats yet — paste an absolute path above">还没有任何会话心跳。直接在上面贴目录路径也行——不需要会话在线。</p>
    {{end}}
    <p class="dir-hint" data-debug="scans all sessions incl. offline, resume works like claude --resume">按目录扫盘：列出该项目下 <code>~/.claude/projects</code> 里的全部历史会话，
      离港的也能点开看全量记录、继续派话，跟本地 <code>claude --resume</code> 一个意思。</p>
  </section>
  {{else}}
  <section class="dir-list-head">
    <h2>
      <span class="emo">📂 </span><b data-debug="dir">{{.Short}}</b>
      <code class="dir-full" title="{{.Path}}">{{.Path}}</code>
    </h2>
    <span>{{len .Rows}} <span data-debug="sessions">个会话</span></span>
  </section>
  <div class="dir-list">
    {{range .Rows}}
    <a class="dir-row {{if .Online}}{{if eq .Status "busy"}}is-busy{{else if eq .Status "waiting"}}is-busy{{else}}is-online{{end}}{{else}}is-off{{end}}"
       href="/sessions/{{.SID}}?dir={{$.Path | urlquery}}">
      <span class="dir-state"><i class="dot"></i></span>
      <span class="dir-main">
        <span class="dir-title">{{if .Title}}{{.Title}}{{else}}<code>{{slice .SID 0 8}}</code><span data-debug="(untitled)">（无题首句）</span>{{end}}</span>
        <span class="dir-meta">
          {{if .Online}}{{if eq .Status "busy"}}<span data-debug="busy">船在忙</span>{{else if eq .Status "waiting"}}<span data-debug="waiting">候令中</span>{{else}}<span data-debug="idle">在线待派</span>{{end}}
          ·{{end}} {{.Events}} <span data-debug="msgs">条消息</span> · <span data-debug="updated">更新</span> {{.LastTS.Format "01-02 15:04"}}
        </span>
      </span>
      <span class="dir-go emo">→</span>
    </a>
    {{end}}
  </div>
  {{end}}
</main>
</body></html>`
