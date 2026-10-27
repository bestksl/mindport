package server

// 登录页（ADR-0008 鉴权）：入港证 = `port web-token` 打出的共享密钥。
// 刻意独立于顶栏/船队：未认证身份什么船位信息都看不到。
const loginHTML = head + `
<main class="login-wrap">
  <form class="login-card" method="post" action="/login" data-debug="auth">
    <div class="login-brand" data-debug="mindportd">
      <span class="brand-mark">{{template "pxLighthouse" .}}</span>
      <span class="brand-text">思港 <i>MindPort</i></span>
    </div>
    <p class="login-sub" data-debug="token required">调度内港，请出示入港证（本机执行 <code>port web-token</code> 取得）</p>
    {{if .Error}}<div class="login-err">⚠️ {{.Error}}</div>{{end}}
    <input type="password" name="key" placeholder="web token" autocomplete="current-password" autofocus required>
    <input type="hidden" name="next" value="{{.Next}}">
    <button type="submit" class="primary" data-debug="submit">入港</button>
  </form>
</main>
</body></html>`

// 会话船台页：终端式 transcript 时间线 + 底部回话坞。首批事件服务端渲染，
// 之后由内联脚本按字节游标幂等增量拉取，向上滚动按需翻更早历史。
const sessionHTML = head + `
{{template "topbar" .}}

<main class="session" data-sid="{{.SID}}">
  <div class="session-head {{if .Online}}online{{else}}offline{{end}}" id="s-head">
    <a class="back" href="{{if .FromList}}/sessions?dir={{.Dir | urlquery}}{{else}}/{{end}}" data-debug="back"><span class="emo">← </span>{{if .FromList}}会话列表{{else}}调度台{{end}}</a>
    <h1 data-debug="session">
      {{if .Name}}<span class="emo">⛵ </span>{{.Name}}{{else}}<span class="emo">⛵ </span>会话 <code>{{.ShortSID}}</code>{{end}}
      {{if .Dir}}<i class="s-dir" title="{{.Dir}}"><span class="emo">📍</span>{{.Short}}</i>{{end}}
    </h1>
    <div class="s-status" id="s-status">
      {{if not .Found}}<span class="badge" data-debug="state: unknown">港里没查到这艘船</span>
      {{else if .Online}}
        {{if .Busy}}<span class="badge busy" data-debug="state: busy"><i class="dot"></i>船在忙</span>{{else}}<span class="badge idle" data-debug="state: idle"><i class="dot"></i>在线待派</span>{{end}}
      {{else}}<span class="badge offline" data-debug="state: offline">已离港·可回看可唤醒</span>{{end}}
      <code class="s-sid" title="会话完整 id">{{.ShortSID}}</code>
    </div>
    <div class="s-pending" id="s-pending" {{if not .Pending}}hidden{{end}}><span class="emo">📬 </span>信箱里有 <b id="s-pending-n">{{.Pending}}</b> 条留言，等船下一轮开工自动送达</div>
  </div>

  {{if .Found}}
  <div class="tl-wrap" id="tl-wrap">
    <div class="tl-edge" id="tl-top">
      {{if .HasLog}}{{if gt .From 0}}<button type="button" id="tl-more">↑ 翻更早的记录</button>{{else}}<span>· 已是会话开端 ·</span>{{end}}{{end}}
    </div>
    <div class="tl" id="tl">
      {{if .HasLog}}{{range .Events}}{{template "tlEvent" .}}{{end}}
      {{else}}<p class="tl-empty">船刚报到，会话记录还没落盘，稍候自动出现…</p>{{end}}
    </div>
  </div>

  <form class="say-dock" id="say-form" action="/sessions/{{.SID}}/say?dir={{.Dir | urlquery}}" method="post">
    <div class="say-note" id="say-note" hidden></div>
    <div class="say-row">
      <textarea name="msg" rows="1" maxlength="4000"
        placeholder="{{if .Busy}}船正忙：留言进信箱，它下一轮开工自动收到{{else}}给船补一句话，继续派活…（Enter 发送）{{end}}"
        id="say-input"></textarea>
      <button type="submit" class="primary" id="say-btn" data-debug="send">📨 传话</button>
    </div>
  </form>
  {{else}}
  <p class="tl-empty">心跳、官方发现口和你给的目录里都没有这个会话文件。它可能已被清理，或 id/目录抄错了。<br><a href="/sessions">按目录翻全部会话</a> · <a href="/">回调度台</a></p>
  {{end}}
</main>

{{define "tlEvent"}}
<div class="tl-ev tl-{{.Role}}{{if .Sidechain}} side{{end}}{{if onlyResults .}} only-result{{end}}" data-id="{{.ID}}">
  <div class="tl-meta">
    {{if eq .Role "assistant"}}<span class="tl-who" data-debug="{{if .Sidechain}}subship{{else}}ship{{end}}"><span class="emo">⛵ </span>{{if .Sidechain}}子船{{else}}船{{end}}</span>
    {{else if onlyResults .}}<span class="tl-who" data-debug="tool-result"><span class="emo">🧰 </span>工具回报</span>
    {{else}}<span class="tl-who" data-debug="dispatcher"><span class="emo">🧭 </span>长官</span>{{end}}
    <time class="tl-ts" datetime="{{.TS}}">{{.TS}}</time>
  </div>
  <div class="tl-body">{{range .Blocks}}{{template "tlBlock" .}}{{end}}</div>
</div>
{{end}}

{{define "tlBlock"}}
{{if eq .Kind "text"}}<div class="tl-text">{{.Text}}</div>{{end}}{{if eq .Kind "thinking"}}<details class="tl-think"><summary data-debug="thinking"><span class="emo">🤔 </span>思考过程</summary><pre>{{.Text}}</pre></details>{{end}}{{if eq .Kind "tool"}}<div class="tl-tool" data-tid="{{.ToolID}}"><details><summary><span class="emo">{{toolEmoji .Name}} </span><b>{{.Name}}</b>{{if .Summary}} <span class="tl-sum">{{.Summary}}</span>{{end}}</summary><pre>{{.Text}}</pre></details></div>{{end}}{{if eq .Kind "result"}}<div class="tl-result {{if .IsError}}r-err{{else}}r-ok{{end}}" data-tid="{{.ToolID}}"><details><summary data-debug="{{if .IsError}}tool error{{else}}tool result{{end}}"><span class="emo">{{if .IsError}}⚠️{{else}}✅{{end}} </span>{{if .IsError}}工具回报失败{{else}}工具回报{{end}}</summary><pre>{{.Text}}</pre></details></div>{{end}}
{{end}}

<script>
{{define "tlEventJS"}}
(function () {
  var SID = "{{.SID}}";
  var DIR = {{printf "%q" .Dir}};
  function dirQS() { return DIR ? '&dir=' + encodeURIComponent(DIR) : ''; }
  var cursor = {{.Cursor}};
  var oldest = {{.From}};
  var hasMore = {{if and .HasLog (gt .From 0)}}true{{else}}false{{end}};
  var listEl = document.getElementById('tl');
  var wrapEl = document.getElementById('tl-wrap');
  if (!listEl || !wrapEl) return;
  var seen = {};
  document.querySelectorAll('#tl .tl-ev').forEach(function (el) { seen[el.getAttribute('data-id')] = 1; });

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function onlyResult(ev) {
    return ev.role === 'user' && ev.blocks.length > 0 &&
      ev.blocks.every(function (b) { return b.kind === 'result'; });
  }
  var TOOL_EMOJI = {Bash: '⌘', BashOutput: '⌘', Edit: '✏️', MultiEdit: '✏️',
    Write: '📝', NotebookEdit: '📝', Read: '👀', TodoWrite: '✓', TaskUpdate: '✓',
    Agent: '🤖', Task: '🤖', WebFetch: '🌐', WebSearch: '🌐'};
  function toolEmoji(n) { return TOOL_EMOJI[n] || '🔧'; }
  function blockHTML(b) {
    if (b.kind === 'text') return '<div class="tl-text">' + esc(b.text) + '</div>';
    if (b.kind === 'thinking') return '<details class="tl-think"><summary data-debug="thinking"><span class="emo">🤔 </span>思考过程</summary><pre>' + esc(b.text) + '</pre></details>';
    if (b.kind === 'tool') return '<div class="tl-tool" data-tid="' + esc(b.tid) + '"><details><summary><span class="emo">' + toolEmoji(b.name) + ' </span><b>' + esc(b.name) + '</b>' +
      (b.summary ? ' <span class="tl-sum">' + esc(b.summary) + '</span>' : '') + '</summary><pre>' + esc(b.text) + '</pre></details></div>';
    if (b.kind === 'result') return '<div class="tl-result ' + (b.err ? 'r-err' : 'r-ok') + '" data-tid="' + esc(b.tid) +
      '"><details><summary data-debug="' + (b.err ? 'tool error' : 'tool result') + '"><span class="emo">' + (b.err ? '⚠️' : '✅') + ' </span>' + (b.err ? '工具回报失败' : '工具回报') + '</summary><pre>' + esc(b.text) + '</pre></details></div>';
    return '';
  }
  function eventHTML(ev) {
    var who;
    if (ev.role === 'assistant') {
      who = '<span class="tl-who" data-debug="' + (ev.side ? 'subship' : 'ship') + '"><span class="emo">⛵ </span>' + (ev.side ? '子船' : '船') + '</span>';
    } else if (onlyResult(ev)) {
      who = '<span class="tl-who" data-debug="tool-result"><span class="emo">🧰 </span>工具回报</span>';
    } else {
      who = '<span class="tl-who" data-debug="dispatcher"><span class="emo">🧭 </span>长官</span>';
    }
    return '<div class="tl-ev tl-' + ev.role + (ev.side ? ' side' : '') + (onlyResult(ev) ? ' only-result' : '') +
      '" data-id="' + esc(ev.id) + '">' +
      '<div class="tl-meta">' + who + '<time class="tl-ts" datetime="' + esc(ev.ts) + '">' + esc(ev.ts) + '</time></div>' +
      '<div class="tl-body">' + ev.blocks.map(blockHTML).join('') + '</div></div>';
  }
  function fmtTimes(scope) {
    (scope || listEl).querySelectorAll('time.tl-ts').forEach(function (t) {
      if (t.dataset.done) return;
      var d = new Date(t.getAttribute('datetime'));
      if (isNaN(d.getTime())) return;
      t.textContent = (d.getMonth() + 1) + '-' + d.getDate() + ' ' +
        String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0') +
        ':' + String(d.getSeconds()).padStart(2, '0');
      t.dataset.done = '1';
    });
  }
  function nearBottom() { return wrapEl.scrollHeight - wrapEl.scrollTop - wrapEl.clientHeight < 72; }
  function stick() { wrapEl.scrollTop = wrapEl.scrollHeight; }
  function appendEvents(evs) {
    var empty = listEl.querySelector('.tl-empty');
    if (empty) empty.remove();
    var html = '';
    evs.forEach(function (ev) { if (!seen[ev.id]) { seen[ev.id] = 1; html += eventHTML(ev); } });
    if (html) listEl.insertAdjacentHTML('beforeend', html);
    fmtTimes();
  }
  fmtTimes();
  stick();

  function setStatus(d) {
    var head = document.getElementById('s-head'), st = document.getElementById('s-status'),
        pend = document.getElementById('s-pending'), pn = document.getElementById('s-pending-n'),
        inp = document.getElementById('say-input');
    if (!head) return;
    head.classList.toggle('online', !!d.online);
    head.classList.toggle('offline', !d.online);
    var html = d.online
      ? (d.busy ? '<span class="badge busy" data-debug="state: busy"><i class="dot"></i>船在忙</span>'
                : '<span class="badge idle" data-debug="state: idle"><i class="dot"></i>在线待派</span>')
      : '<span class="badge offline" data-debug="state: offline">已离港·可回看可唤醒</span>';
    if (st) st.innerHTML = html + '<code class="s-sid">' + esc(SID.slice(0, 8)) + '</code>';
    if (inp && document.documentElement.dataset.stealth !== '1') {
      inp.placeholder = d.busy ? '船正忙：留言进信箱，它下一轮开工自动收到' : '给船补一句话，继续派活…（Enter 发送）';
    }
    if (pend) { if (d.pending > 0) { pn.textContent = d.pending; pend.hidden = false; } else pend.hidden = true; }
  }

  // 增量轮询：1.5s 一轮，页面隐藏时暂停（回来立即补一轮）
  var timer = null, active = false;
  function poll() {
    if (active) return;
    active = true;
    fetch('/api/sessions/' + encodeURIComponent(SID) + '/stream?from=' + cursor + dirQS(), { headers: { 'Accept': 'application/json' } })
      .then(function (r) { if (r.status === 401) { location.href = '/login'; throw new Error('auth'); } return r.json(); })
      .then(function (d) {
        if (d.events && d.events.length) {
          var stickNow = nearBottom();
          appendEvents(d.events);
          if (stickNow) stick();
        }
        if (d.cursor) cursor = d.cursor;
        setStatus({ online: d.online, busy: d.busy, pending: d.pending });
      }).catch(function () {}).finally(function () { active = false; });
  }
  function startTimer() { if (!timer) timer = setInterval(poll, 1500); }
  function stopTimer() { if (timer) { clearInterval(timer); timer = null; } }
  document.addEventListener('visibilitychange', function () {
    if (document.hidden) stopTimer(); else { poll(); startTimer(); }
  });
  startTimer();

  // 上翻：128KB 一块，行边界对齐，插入后保持视觉位置
  var loadingOlder = false;
  function loadOlder() {
    if (loadingOlder || !hasMore) return Promise.resolve();
    loadingOlder = true;
    var btn = document.getElementById('tl-more');
    if (btn) btn.disabled = true;
    return fetch('/api/sessions/' + encodeURIComponent(SID) + '/history?before=' + oldest + dirQS())
      .then(function (r) { return r.json(); })
      .then(function (d) {
        if (!d.events) return;
        var prevH = wrapEl.scrollHeight, html = '';
        d.events.forEach(function (ev) { if (!seen[ev.id]) { seen[ev.id] = 1; html += eventHTML(ev); } });
        if (html) {
          listEl.insertAdjacentHTML('afterbegin', html);
          wrapEl.scrollTop += wrapEl.scrollHeight - prevH;
          fmtTimes();
        }
        if (d.events.length === 0 && d.from === oldest) { hasMore = false; }
        if (d.from === 0) {
          hasMore = false;
          var top = document.getElementById('tl-top');
          if (top) top.innerHTML = '<span>· 已是会话开端 ·</span>';
        } else {
          oldest = d.from;
        }
      }).catch(function () {}).finally(function () {
        loadingOlder = false;
        if (btn) btn.disabled = false;
      });
  }
  var moreBtn = document.getElementById('tl-more');
  if (moreBtn) moreBtn.addEventListener('click', loadOlder);
  wrapEl.addEventListener('scroll', function () { if (wrapEl.scrollTop < 32) loadOlder(); });

  // 回话坞：Enter 发送（Shift 换行、中文组词中不触发），自适应高度
  var form = document.getElementById('say-form'), note = document.getElementById('say-note'),
      input = document.getElementById('say-input'), btn = document.getElementById('say-btn');
  if (input && document.documentElement.dataset.stealth === '1') {
    input.placeholder = 'msg (enter=send)';
  }
  if (input) {
    input.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) { e.preventDefault(); form.requestSubmit(); }
    });
    input.addEventListener('input', function () {
      input.style.height = 'auto';
      input.style.height = Math.min(input.scrollHeight, 120) + 'px';
    });
  }
  function flash(msg, kind) {
    if (!note) return;
    note.textContent = msg;
    note.className = 'say-note ' + kind;
    note.hidden = false;
    clearTimeout(note._t);
    note._t = setTimeout(function () { note.hidden = true; }, 6000);
  }
  if (form) form.addEventListener('submit', function (e) {
    e.preventDefault();
    var msg = input.value.trim();
    if (!msg) return;
    btn.disabled = true;
    fetch(form.action, { method: 'POST', body: new FormData(form) })
      .then(function (r) { if (r.status === 401) { location.href = '/login'; throw 0; } return r.json(); })
      .then(function (d) {
        if (d.error) { flash('⚠️ ' + d.error, 'note-err'); return; }
        input.value = '';
        input.style.height = 'auto';
        if (d.delivered === 'resume') flash('⛵ ' + d.detail, 'note-ok');
        else flash('📬 ' + d.detail, 'note-mail');
        setTimeout(poll, 400);
      })
      .catch(function (e) { if (e) flash('⚠️ 网络岔了，没递出去', 'note-err'); })
      .finally(function () { btn.disabled = false; });
  });
})();
{{end}}
{{template "tlEventJS" .}}
</script>
</body></html>`
