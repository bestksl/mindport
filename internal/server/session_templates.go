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
    <p class="login-sub" data-debug="token required">请出示入港证（本机执行 <code>port web-token</code> 取得）</p>
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
  <div class="session-head st-{{.StateKey}}{{if .HasMark}} marked mark-{{.Mark.Outcome}}{{end}}" id="s-head">
    <a class="back" href="{{if .FromList}}/sessions?dir={{.Dir | urlquery}}{{else}}/{{end}}" data-debug="back"><span class="emo">← </span>{{if .FromList}}船队列表{{else}}回港口{{end}}</a>
    <h1 data-debug="session">
      {{if .Name}}<span class="emo">⛵ </span>{{.Name}}{{else}}<span class="emo">⛵ </span>船队 <code>{{.ShortSID}}</code>{{end}}
      {{if .Dir}}<i class="s-dir" title="{{.Dir}}"><span class="emo">📍</span>{{.Short}}</i>{{end}}
    </h1>
    <div class="s-status" id="s-status">
      {{if not .Found}}<span class="badge" id="s-badge" data-debug="state: unknown">没查到这支船队</span>
      {{else if .HasMark}}<span class="badge mark mark-{{.Mark.Outcome}}" id="s-badge" data-debug="mark: {{.Mark.Outcome}}"><i class="dot"></i>{{.MarkText}}</span>
      {{else}}<span class="badge {{.StateKey}}" id="s-badge" data-debug="state: {{.StateKey}}"{{if .WaitingFor}} title="{{.WaitingFor}}"{{end}}><i class="dot"></i>{{.StateText}}{{if and (eq .StateKey "waiting") .WaitingFor}} · {{.WaitingFor}}{{end}}</span>{{end}}
      <code class="s-sid" title="{{if .Anon}}进程 pid{{else}}会话完整 id{{end}}">{{if .Anon}}PID {{.AnonPID}}{{else}}{{.ShortSID}}{{end}}</code>
      {{if and .Found (not .Anon)}}<span class="s-markbtns" id="s-markbtns">
        <button type="button" class="markbtn good" data-outcome="good" data-debug="mark good"{{if .HasMark}} hidden{{end}}>✅ 圆满结束</button>
        <button type="button" class="markbtn stop" data-outcome="stop" data-debug="mark stop"{{if .HasMark}} hidden{{end}}>⏹ 中途停止</button>
        <button type="button" class="markbtn cancel" id="s-mark-cancel" data-debug="mark cancel"{{if not .HasMark}} hidden{{end}}>撤结局</button>
      </span>{{end}}
    </div>
    <div class="s-pending" id="s-pending" {{if not .Pending}}hidden{{end}}><span class="emo">📬 </span>有 <b id="s-pending-n">{{.Pending}}</b> 条留言排队中，船队下一轮开工自动收到</div>
    {{if .BoundTasks}}
    <div class="s-cargo"><span class="emo">🧷 </span><span>这支船队上挂着 {{len .BoundTasks}} 个集装箱：</span>
      {{range .BoundTasks}}<a class="cargo-tag" href="/tasks/{{.Meta.ID}}">{{.Meta.ID}} {{.Meta.Title}}</a>{{end}}
    </div>
    {{end}}
  </div>

  {{if .Found}}
  {{if .Anon}}
  <div class="anon-card" data-debug="anon">
    <div class="anon-emo">🪝</div>
    <h2>这支船队刚出船台，还没说过话</h2>
    <p>思港在进程表里看到了一个开着的 Claude 终端（PID {{.AnonPID}}），但它还没有对话记录。第一句话得在它<b>自己的终端</b>里开口——说出口，这里就自动变成正常船台。</p>
    <p class="anon-dir" title="{{.Dir}}"><span class="emo">📍</span>{{.Dir}}</p>
    <p class="anon-hint">不想切过去？<a href="/sessions?dir={{.Dir | urlquery}}">回列表</a>看全部船队，或从<a href="/#tasks">任务上</a>新开一支能直接说话的船队。</p>
  </div>
  <script>
  (function () {
    var SID = {{.SID}}; // proc-<pid>，与 /api/sessions 行的 sid 同键
    setInterval(function () {
      if (document.hidden) return;
      fetch('/api/sessions?dir=' + encodeURIComponent({{.Dir}}), { headers: { 'Accept': 'application/json' } })
        .then(function (r) { return r.json(); })
        .then(function (d) {
          var alive = (d.rows || []).some(function (r) { return r.sid === SID; });
          // 匿名行消失 = 在终端开口、已配成真 sid；跳列表看它以真面目入列
          if (!alive) location.replace('/sessions?dir=' + encodeURIComponent({{.Dir}}));
        }).catch(function () {});
    }, 8000);
  })();
  </script>
  {{else}}
  <div class="tl-wrap" id="tl-wrap">
    <div class="tl-edge" id="tl-top">
      {{if .HasLog}}{{if gt .From 0}}<button type="button" id="tl-more">↑ 翻更早的记录</button>{{else}}<span>· 已是会话开端 ·</span>{{end}}{{end}}
      <button type="button" id="think-toggle" class="think-toggle" aria-pressed="true">思考：显</button>
    </div>
    <div class="tl" id="tl">
      {{if .HasLog}}{{range .Events}}{{template "tlEvent" .}}{{end}}
      {{else}}<p class="tl-empty">会话刚开，记录还没落盘，稍候自动出现…</p>{{end}}
    </div>
  </div>

  <div class="tl-typing" id="tl-typing" hidden><span class="dots"><i></i><i></i><i></i></span><span>船队正在思考…</span></div>

  <form class="say-dock" id="say-form" action="/sessions/{{.SID}}/say?dir={{.Dir | urlquery}}" method="post">
    <div class="say-note" id="say-note" hidden></div>
    <div class="say-row">
      <textarea name="msg" rows="1" maxlength="8000" enterkeyhint="send" autocomplete="off"
        placeholder="{{if .Busy}}船队正忙：留言会排队，它下一轮自动收到{{else}}说点什么…（Enter 发送，Shift+Enter 换行）{{end}}"
        id="say-input"></textarea>
      <button type="submit" class="primary" id="say-btn" data-debug="send">📨 发送</button>
    </div>
  </form>
  {{end}}{{/* .Anon 分支结束 */}}
  {{else}}
  <p class="tl-empty">心跳、官方发现口和你给的目录里都没有这个会话文件。它可能已被清理，或 id/目录抄错了。<br><a href="/sessions">回船队列表</a> · <a href="/">回港口</a></p>
  {{end}}
</main>

{{define "tlEvent"}}
{{if eq .Role "system"}}
<div class="tl-ev tl-system" data-id="{{.ID}}"><div class="tl-body">{{range .Blocks}}{{template "tlBlock" .}}{{end}}</div></div>
{{else}}
<div class="tl-ev tl-{{.Role}}{{if .Sidechain}} side{{end}}{{if onlyResults .}} only-result{{end}}" data-id="{{.ID}}">
  <div class="tl-meta">
    {{if eq .Role "assistant"}}<span class="tl-who" data-debug="{{if .Sidechain}}subship{{else}}ship{{end}}"><span class="emo">⛵ </span>{{if .Sidechain}}子代理{{else}}Claude{{end}}</span>
    {{else if onlyResults .}}<span class="tl-who" data-debug="tool-result"><span class="emo">🧰 </span>工具回报</span>
    {{else}}<span class="tl-who" data-debug="you"><span class="emo">🧭 </span>你</span>{{end}}
    <time class="tl-ts" datetime="{{.TS}}">{{.TS}}</time>
  </div>
  <div class="tl-body">{{range .Blocks}}{{template "tlBlock" .}}{{end}}</div>
</div>
{{end}}
{{end}}

{{define "tlBlock"}}
{{if eq .Kind "text"}}<div class="tl-text">{{.Text}}</div>{{end}}{{if eq .Kind "thinking"}}<details class="tl-think" open><summary data-debug="thinking"><span class="emo">🤔 </span>思考</summary><pre>{{.Text}}</pre></details>{{end}}{{if eq .Kind "tool"}}<div class="tl-tool" data-tid="{{.ToolID}}"><details><summary><span class="emo">{{toolEmoji .Name}} </span><b>{{.Name}}</b>{{if .Summary}} <span class="tl-sum">{{.Summary}}</span>{{end}}</summary><pre>{{.Text}}</pre></details></div>{{end}}{{if eq .Kind "result"}}<div class="tl-result {{if .IsError}}r-err{{else}}r-ok{{end}}" data-tid="{{.ToolID}}"><details><summary data-debug="{{if .IsError}}tool error{{else}}tool result{{end}}"><span class="emo">{{if .IsError}}⚠️{{else}}✅{{end}} </span>{{if .IsError}}工具回报失败{{else}}工具回报{{end}}</summary><pre>{{.Text}}</pre></details></div>{{end}}{{if eq .Kind "interrupt"}}<div class="tl-sys interrupt" data-debug="interrupted"><span class="emo">✋ </span>已中断 · 你按了 Ctrl-C，上一段输出就停在这里</div>{{end}}{{if eq .Kind "continue"}}<div class="tl-sys continue" data-debug="compacted"><span class="emo">🧵 </span>会话曾被压缩，从这里续接</div>{{end}}
{{end}}

<script>
{{define "tlEventJS"}}
(function () {
  var SID = "{{.SID}}";
  var DIR = {{.Dir}}; // html/template 在 JS 上下文自动输出带引号的转义字面量
  function dirQS() { return DIR ? '&dir=' + encodeURIComponent(DIR) : ''; }
  var cursor = {{.Cursor}};
  var oldest = {{.From}};
  var hasMore = {{if and .HasLog (gt .From 0)}}true{{else}}false{{end}};
  var listEl = document.getElementById('tl');
  var wrapEl = document.getElementById('tl-wrap');
  if (!listEl || !wrapEl) return;
  var seen = {};
  var thinkShown = true; // 思考默认像 CLI 一样直接可见，头部可一键隐掉
  document.querySelectorAll('#tl .tl-ev').forEach(function (el) { seen[el.getAttribute('data-id')] = 1; });
  document.querySelectorAll('#tl .tl-think').forEach(function (d) { d.setAttribute('data-synced', '1'); });

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
    if (b.kind === 'thinking') return '<details class="tl-think" open><summary data-debug="thinking"><span class="emo">🤔 </span>思考</summary><pre>' + esc(b.text) + '</pre></details>';
    if (b.kind === 'tool') return '<div class="tl-tool" data-tid="' + esc(b.tid) + '"><details><summary><span class="emo">' + toolEmoji(b.name) + ' </span><b>' + esc(b.name) + '</b>' +
      (b.summary ? ' <span class="tl-sum">' + esc(b.summary) + '</span>' : '') + '</summary><pre>' + esc(b.text) + '</pre></details></div>';
    if (b.kind === 'result') return '<div class="tl-result ' + (b.err ? 'r-err' : 'r-ok') + '" data-tid="' + esc(b.tid) +
      '"><details><summary data-debug="' + (b.err ? 'tool error' : 'tool result') + '"><span class="emo">' + (b.err ? '⚠️' : '✅') + ' </span>' + (b.err ? '工具回报失败' : '工具回报') + '</summary><pre>' + esc(b.text) + '</pre></details></div>';
    if (b.kind === 'interrupt') return '<div class="tl-sys interrupt" data-debug="interrupted"><span class="emo">✋ </span>已中断 · 你按了 Ctrl-C，上一段输出就停在这里</div>';
    if (b.kind === 'continue') return '<div class="tl-sys continue" data-debug="compacted"><span class="emo">🧵 </span>会话曾被压缩，从这里续接</div>';
    return '';
  }
  function eventHTML(ev) {
    if (ev.role === 'system') {
      return '<div class="tl-ev tl-system" data-id="' + esc(ev.id) + '"><div class="tl-body">' +
        ev.blocks.map(blockHTML).join('') + '</div></div>';
    }
    var who;
    if (ev.role === 'assistant') {
      who = '<span class="tl-who" data-debug="' + (ev.side ? 'subship' : 'ship') + '"><span class="emo">⛵ </span>' + (ev.side ? '子代理' : 'Claude') + '</span>';
    } else if (onlyResult(ev)) {
      who = '<span class="tl-who" data-debug="tool-result"><span class="emo">🧰 </span>工具回报</span>';
    } else {
      who = '<span class="tl-who" data-debug="you"><span class="emo">🧭 </span>你</span>';
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
    if (html) {
      listEl.insertAdjacentHTML('beforeend', html);
      // 新到的思考块按当前总开关决定开合，之后用户可单独展开
      listEl.querySelectorAll('.tl-think:not([data-synced])').forEach(function (d) {
        d.open = thinkShown; d.setAttribute('data-synced', '1');
      });
    }
    fmtTimes();
  }
  fmtTimes();
  stick();

  var MARK_TEXT = { good: '✅ 圆满结束', stop: '⏹ 中途停止' };
  var lastState = null;
  function stateKey(d) {
    if (!d.online) return 'offline';
    if (d.status === 'busy') return 'working';
    if (d.status === 'waiting') return 'waiting';
    return 'idle'; // 进程在线但这一轮已停
  }
  function stateText(k) {
    return k === 'working' ? '工作中' : k === 'waiting' ? '待你输入' : '已停止';
  }
  function syncMarkBtns(mark) {
    var box = document.getElementById('s-markbtns');
    if (!box) return;
    box.querySelectorAll('.markbtn[data-outcome]').forEach(function (b) { b.hidden = !!mark; });
    var c = document.getElementById('s-mark-cancel');
    if (c) c.hidden = !mark;
  }
  function setStatus(d) {
    lastState = d;
    var head = document.getElementById('s-head'),
        pend = document.getElementById('s-pending'), pn = document.getElementById('s-pending-n'),
        typing = document.getElementById('tl-typing'),
        inp = document.getElementById('say-input');
    if (!head) return;
    var key = stateKey(d);
    head.classList.remove('online', 'offline', 'st-working', 'st-waiting', 'st-idle', 'st-offline',
      'marked', 'mark-good', 'mark-stop');
    head.classList.add('st-' + key);
    head.classList.toggle('online', !!d.online);
    head.classList.toggle('offline', !d.online);
    if (d.mark) head.classList.add('marked', 'mark-' + d.mark);

    var badge = document.getElementById('s-badge');
    if (badge) {
      var cls, txt, title = '';
      if (d.mark) { cls = 'badge mark mark-' + d.mark; txt = MARK_TEXT[d.mark]; }
      else {
        cls = 'badge ' + key;
        txt = stateText(key);
        if (key === 'waiting' && d.waiting_for) { title = d.waiting_for; txt += ' · ' + d.waiting_for; }
      }
      badge.outerHTML = '<span class="' + cls + '" id="s-badge"' + (title ? ' title="' + esc(title) + '"' : '') +
        ' data-debug="state: ' + (d.mark ? 'mark-' + d.mark : key) + '"><i class="dot"></i>' + esc(txt) + '</span>';
    }
    syncMarkBtns(d.mark);
    if (typing) typing.hidden = !(d.online && d.status === 'busy' && !d.mark);
    if (inp && document.documentElement.dataset.stealth !== '1') {
      inp.placeholder = d.status === 'busy'
        ? '船队正忙：留言会排队，它下一轮自动收到'
        : '说点什么…（Enter 发送，Shift+Enter 换行）';
    }
    if (pend) { if (d.pending > 0) { pn.textContent = d.pending; pend.hidden = false; } else pend.hidden = true; }
  }

  // 增量轮询：1s 一轮（对齐 CLI 的跟手感），页面隐藏时暂停（回来立即补一轮）
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
        setStatus({ online: d.online, status: d.status, mark: d.mark,
          waiting_for: d.waiting_for, pending: d.pending });
      }).catch(function () {}).finally(function () { active = false; });
  }
  function startTimer() { if (!timer) timer = setInterval(poll, 1000); }
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
          listEl.querySelectorAll('.tl-think:not([data-synced])').forEach(function (d) {
            d.open = thinkShown; d.setAttribute('data-synced', '1');
          });
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
    if (!msg) {
      // 语音/输入法偶发焦点丢失：别让用户对着无反馈发呆
      input.focus();
      flash('先说点什么再发', 'note-err');
      return;
    }
    btn.disabled = true;
    // 显式带上校验过的字符串（而不是 new FormData 重新读表单），
    // 保证「发出去的」和「校验过的」是同一个值。
    var body = new URLSearchParams();
    body.set('msg', msg);
    fetch(form.action, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: body
    })
      .then(function (r) { if (r.status === 401) { location.href = '/login'; throw 0; } return r.json(); })
      .then(function (d) {
        if (d.error) { flash('⚠️ ' + d.error, 'note-err'); return; }
        input.value = '';
        input.style.height = 'auto';
        if (d.delivered === 'resume') {
          flash('⛵ ' + d.detail, 'note-ok');
          // poller 30s 才刷新一轮快照：先乐观切到「工作中」，下一轮 poll 自动校正
          if (lastState) setStatus(Object.assign({}, lastState, { online: true, status: 'busy', mark: '' }));
        } else flash('📬 ' + d.detail, 'note-mail');
        setTimeout(poll, 400);
      })
      .catch(function (e) { if (e) flash('⚠️ 网络岔了，没递出去', 'note-err'); })
      .finally(function () { btn.disabled = false; });
  });

  // 思考总开关：像 CLI 一样默认直接可见，嫌长可一键隐掉（新到块跟随）
  var thinkBtn = document.getElementById('think-toggle');
  if (thinkBtn) thinkBtn.addEventListener('click', function () {
    thinkShown = !thinkShown;
    listEl.querySelectorAll('.tl-think').forEach(function (d) { d.open = thinkShown; });
    thinkBtn.textContent = '思考：' + (thinkShown ? '显' : '隐');
    thinkBtn.setAttribute('aria-pressed', thinkShown ? 'true' : 'false');
  });

  // 结局标记：✅ 圆满结束 / ⏹ 中途停止；再开口说话时服务端自动撤掉
  function markAction(path, outcome) {
    var body = new URLSearchParams();
    if (outcome) { body.set('outcome', outcome); body.set('dir', DIR); }
    return fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: body
    }).then(function (r) { if (r.status === 401) { location.href = '/login'; throw 0; } return r.json(); });
  }
  document.querySelectorAll('.markbtn[data-outcome]').forEach(function (b) {
    b.addEventListener('click', function () {
      var outcome = b.getAttribute('data-outcome');
      markAction('/sessions/' + encodeURIComponent(SID) + '/mark', outcome).then(function (d) {
        if (d.error) { flash('⚠️ ' + d.error, 'note-err'); return; }
        if (lastState) setStatus(Object.assign({}, lastState, { mark: outcome }));
      }).catch(function () { flash('⚠️ 标记没递出去', 'note-err'); });
    });
  });
  var cancelMark = document.getElementById('s-mark-cancel');
  if (cancelMark) cancelMark.addEventListener('click', function () {
    markAction('/sessions/' + encodeURIComponent(SID) + '/unmark', '').then(function (d) {
      if (d.error) { flash('⚠️ ' + d.error, 'note-err'); return; }
      if (lastState) setStatus(Object.assign({}, lastState, { mark: '' }));
      poll();
    }).catch(function () { flash('⚠️ 没撤成', 'note-err'); });
  });
})();
{{end}}
{{template "tlEventJS" .}}
</script>
</body></html>`
