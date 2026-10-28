package server

// 按目录翻会话：贴一个项目绝对路径列全部 Claude 会话；不给路径则聚合
// ~/.claude/projects 下全部项目的最近会话（路径由 transcript cwd 自证）。
const dirListHTML = head + `
{{template "topbar" .}}

<main class="dir-page">
  <form class="dir-bar" method="get" action="/sessions" id="dir-form">
    <input type="text" name="dir" value="{{.Path}}" spellcheck="false" list="known-dir-list"
      placeholder="贴项目目录（~/Documents/GitHub/xxx 也行），留空看全部最近会话"
      aria-label="项目目录" enterkeyhint="search">
    <datalist id="known-dir-list">{{range .KnownDirs}}<option value="{{.Dir}}">{{end}}</datalist>
    <button type="submit" class="primary" data-debug="open"><span class="emo">📂 </span>翻会话</button>
  </form>

  {{if .Error}}<div class="dir-err" data-debug="error"><span class="emo">⚠️ </span>{{.Error}}</div>
  <p class="dir-back-recent"><a href="/sessions">← 看全部最近会话</a></p>{{end}}

  {{if .NoDir}}
  <div id="recent-chips" class="dir-chips" data-label="最近浏览"></div>
  {{if .Rows}}
  <section class="dir-list-head">
    <h2><span class="emo">🕒 </span><span data-debug="recent sessions">最近会话 · 全部项目</span></h2>
    <span>{{len .Rows}} <span data-debug="sessions">个会话</span></span>
  </section>
  <div class="dir-list" id="dir-list">
    {{range .Rows}}{{template "dirRow" .}}{{end}}
  </div>
  {{end}}
  {{if .KnownDirs}}
  <details class="dir-known-fold">
    <summary><span data-debug="all berths ({{len .KnownDirs}})">港内已知船位（心跳里出现过的目录，{{len .KnownDirs}}）</span></summary>
    <div class="dir-chips">
      {{range .KnownDirs}}
      <a class="dir-chip" href="/sessions?dir={{.Dir | urlquery}}">
        <span class="emo">📍</span> <b>{{.Short}}</b><i title="{{.Dir}}">{{.Dir}}</i>
      </a>
      {{end}}
    </div>
  </details>
  {{end}}
  {{else}}
  <section class="dir-list-head">
    <h2>
      <span class="emo">📂 </span><b data-debug="dir">{{.Short}}</b>
      <code class="dir-full" title="{{.Path}}">{{.Path}}</code>
    </h2>
    <span><a class="dir-all-link" href="/sessions" data-debug="all recent"><span class="emo">🕒 </span>全部最近</a></span>
  </section>
  <div class="dir-list" id="dir-list">
    {{range .Rows}}{{template "dirRow" .}}{{end}}
  </div>
  {{if not .Rows}}<p class="dir-hint" data-debug="no sessions here">这个目录下还没有会话记录。</p>{{end}}
  {{end}}
</main>

{{define "dirRow"}}
{{if .Anon}}
<div class="dir-row dir-row-anon is-waiting" data-sid="{{.SID}}" data-anon="1" title="进程开着但还没说话，说第一句后就能点进船台">
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
<a class="dir-row {{.Class}}{{if .MarkText}} has-mark{{end}}" data-sid="{{.SID}}"
   href="/sessions/{{.SID}}?dir={{.Dir | urlquery}}" title="{{.Dir}}">
  <span class="dir-state"><i class="dot"></i></span>
  <span class="dir-main">
    <span class="dir-title">{{if .Title}}{{.Title}}{{else}}<code>{{slice .SID 0 8}}</code><span data-debug="(untitled)">（无题首句）</span>{{end}}</span>
    <span class="dir-meta">
      <span class="row-state">{{if .MarkText}}<em class="st-mark st-mark-{{.Mark}}" data-debug="mark {{.Mark}}">{{.MarkText}}</em>{{end}}<em class="st-working" data-debug="working">工作中</em><em class="st-waiting" data-debug="waiting">待你输入</em><em class="st-stopped" data-debug="stopped">已停止</em><b>·</b></span>
      {{if .ShowProj}}<i class="dir-proj"><span class="emo">📁 </span>{{.Proj}}</i><b>·</b>{{end}}
      {{if not .HideCount}}<span class="row-events">{{.Events}}</span> <span data-debug="msgs">条消息</span><b>·</b>{{end}}<span data-debug="updated">更新</span> {{.LastTS.Format "01-02 15:04"}}
    </span>
  </span>
  <span class="dir-go emo">→</span>
</a>
{{end}}
{{end}}

<script>
(function () {
  // 状态轻量轮询：只切行 class，徽标由 CSS 显隐，不打断滚动
  var listEl = document.getElementById('dir-list');
  var dirQS = new URLSearchParams(location.search).get('dir') || '';
  var apiURL = '/api/sessions' + (dirQS ? '?dir=' + encodeURIComponent(dirQS) : '');
  var timer = null;

  function markText(m) { return m === 'good' ? '✅ 圆满结束' : '⏹ 中途停止'; }
  function patchRow(r) {
    var el = listEl.querySelector('[data-sid="' + r.sid + '"]');
    if (!el) return;
    el.classList.remove('is-working', 'is-waiting', 'is-idle', 'is-offline');
    if (!r.online) el.classList.add('is-offline');
    else if (r.status === 'busy') el.classList.add('is-working');
    else if (r.status === 'waiting') el.classList.add('is-waiting');
    else el.classList.add('is-idle');
    // 结局标记覆盖状态文案；标记可能在页面加载后才产生，就地补一个 em
    var state = el.querySelector('.row-state');
    var mk = state && state.querySelector('.st-mark');
    if (r.mark) {
      el.classList.add('has-mark');
      if (!mk && state) { mk = document.createElement('em'); state.insertBefore(mk, state.firstChild); }
      if (mk) { mk.className = 'st-mark st-mark-' + r.mark; mk.textContent = markText(r.mark); }
    } else {
      el.classList.remove('has-mark');
      if (mk) mk.remove();
    }
    var ev = el.querySelector('.row-events');
    if (ev && r.events) ev.textContent = r.events;
  }
  function tick() {
    fetch(apiURL, { headers: { 'Accept': 'application/json' }, credentials: 'same-origin' })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (d) {
        if (!d || !d.rows) return;
        d.rows.forEach(patchRow);
        // 空会话（proc-<pid>）被关掉：响应里已缺席，就地移除行
        var live = {};
        d.rows.forEach(function (r) { live[r.sid] = true; });
        listEl.querySelectorAll('[data-anon="1"]').forEach(function (el) {
          if (!live[el.getAttribute('data-sid')]) el.remove();
        });
        // 空会话一开口就有了真 sid：响应里有、页面没有 → 静默重载一次
        var missing = d.rows.some(function (r) {
          return !r.anon && r.sid.indexOf('proc-') !== 0 && !listEl.querySelector('[data-sid="' + r.sid + '"]');
        });
        if (missing && !document.hidden) location.reload();
      })
      .catch(function () {});
  }
  function schedule() { timer = setTimeout(function () { tick(); schedule(); }, 8000); }
  document.addEventListener('visibilitychange', function () {
    if (document.hidden) { clearTimeout(timer); timer = null; }
    else if (!timer) { tick(); schedule(); }
  });
  schedule();

  // 最近浏览目录：列表页落 localStorage，空态页注入快捷 chips
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
})();
</script>
</body></html>`
