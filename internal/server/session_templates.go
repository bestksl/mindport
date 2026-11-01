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
  <div class="conn-banner" id="conn-banner" role="status" hidden><span class="conn-dot"></span>实时连接断了，正在自动重连…</div>
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
      </span>
      <span class="s-actbtns" id="s-actbtns">
        <button type="button" class="actbtn" id="btn-notify" title="完成、待你输入或待审批时，手机通知我">🔔 <span id="notify-word">通知</span></button>
        <button type="button" class="actbtn danger" id="btn-scuttle" title="把船队从思港抹掉（CC 历史文件保留），等同 Ctrl-X">🧨 拆船</button>
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
    </div>
    <div class="tl" id="tl">
      {{if .HasLog}}{{range .Events}}{{template "tlEvent" .}}{{end}}
      {{else}}<p class="tl-empty">会话刚开，记录还没落盘，稍候自动出现…</p>{{end}}
    </div>
  </div>

  <div class="tl-typing" id="tl-typing" hidden><span class="dots"><i></i><i></i><i></i></span><span>船队正在思考…</span></div>
  <div class="tl-live" id="tl-live" hidden></div>
  <div class="tl-perms" id="tl-perms" hidden></div>
  <div class="tl-queue" id="tl-queue" hidden>
    <div class="q-head"><span><span class="emo">📬 </span>已排队 <b id="q-n">0</b> 句，这轮跑完按顺序自动说</span>
      <button type="button" class="mini danger" id="q-cancel" title="中断当前轮并清空排队（Ctrl-X 语义）">⏹ 中断并清空</button>
    </div>
    <div class="q-list" id="q-list"></div>
  </div>

  <div class="tl-actions" id="tl-actions" hidden>
    <button type="button" class="msgact" id="btn-regen" data-debug="regenerate" title="中断当前回答，就上一句话重新作答"><span class="emo">🔄</span> 重新生成</button>
    <button type="button" class="msgact" id="btn-copy-last" data-debug="copy" title="复制船队最近一次回复"><span class="emo">📋</span> 复制</button>
  </div>

  <form class="say-dock" id="say-form" action="/sessions/{{.SID}}/say?dir={{.Dir | urlquery}}" method="post">
    <div class="say-note" id="say-note" hidden></div>
    <div class="dock-mode">
      <div class="mode-menu" id="mode-menu">
        <button type="button" class="mode-chip" id="mode-chip" data-mode="auto" data-debug="mode chip"
          title="切换船队的工作模式">
          <span class="mode-ico" id="mode-ico">⚡</span><span class="mode-word" id="mode-word">Auto</span><span class="mode-caret">▾</span>
        </button>
        <div class="mode-pop" id="mode-pop" hidden>
          <button type="button" class="mode-opt" data-mode="auto" data-debug="mode auto">
            <span class="mo-ico">⚡</span><span class="mo-tx"><b>Auto</b><i>默认：能自动的自动干，危险操作才找你批</i></span><span class="mo-tick">✓</span>
          </button>
          <button type="button" class="mode-opt" data-mode="plan" data-debug="mode plan">
            <span class="mo-ico">🗺️</span><span class="mo-tx"><b>Plan</b><i>只调研、出计划，不动手改任何东西</i></span><span class="mo-tick">✓</span>
          </button>
          <div class="mo-sep"></div>
          <button type="button" class="mode-opt mo-yolo" data-mode="yolo" data-debug="mode yolo">
            <span class="mo-ico">🔓</span><span class="mo-tx"><b>无人值守</b><i>不再逐条请批，放手让它一路做完（谨慎）</i></span><span class="mo-tick">✓</span>
          </button>
        </div>
      </div>
    </div>
    <div class="say-row">
      <textarea name="msg" rows="1" maxlength="8000" enterkeyhint="send" autocomplete="off"
        placeholder="{{if .Busy}}船队正忙：发送 = 停下这轮换跑你这句；Ctrl+Enter = 排队{{else}}说点什么…（Enter 发送）{{end}}"
        id="say-input"></textarea>
      <button type="button" class="say-mic" id="say-mic" data-debug="voice" title="按住说话" hidden>🎤</button>
      <button type="submit" class="primary" id="say-btn" data-debug="send">📨 发送</button>
    </div>
    <div class="voice-overlay" id="voice-overlay" hidden>
      <div class="vo-inner" id="vo-inner">
        <span class="vo-wave"><i></i><i></i><i></i><i></i><i></i></span>
        <span class="vo-tx" id="vo-tx">正在听…</span>
        <span class="vo-hint">松开发送 · 上滑取消</span>
      </div>
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
  {{if eq .Role "assistant"}}<div class="tl-meta">{{if .Sidechain}}<span class="tl-who" data-debug="subship">🤖 子代理</span>{{end}}<time class="tl-ts" datetime="{{.TS}}">{{.TS}}</time></div>
  {{else if not (onlyResults .)}}<div class="tl-meta"><time class="tl-ts" datetime="{{.TS}}">{{.TS}}</time></div>{{end}}
  <div class="tl-body">{{range .Blocks}}{{template "tlBlock" .}}{{end}}</div>
  {{if eq .Role "user"}}{{if not (onlyResults .)}}{{$ut := userText .}}{{if $ut}}<div class="bub-acts"><button type="button" class="bub-act" data-act="edit" data-debug="edit" data-text="{{$ut}}" title="编辑后重发（会停下当前轮）">✏️</button><button type="button" class="bub-act" data-act="copy" data-debug="copy" data-text="{{$ut}}" title="复制这句话">📋</button></div>{{end}}{{end}}{{end}}
</div>
{{end}}
{{end}}

{{define "tlBlock"}}
{{if eq .Kind "text"}}<div class="tl-text{{if .HTML}} md{{end}}">{{if .HTML}}{{rawHTML .HTML}}{{else}}{{.Text}}{{end}}</div>{{end}}{{if eq .Kind "thinking"}}<details class="tl-think"><summary data-debug="thinking"><span class="think-ico">🧠</span><span class="think-tx">已深度思考</span></summary><pre>{{.Text}}</pre></details>{{end}}{{if eq .Kind "tool"}}<details class="tl-tool ucall {{if .ResultErr}}uc-err{{else if .Result}}uc-ok{{else}}uc-run{{end}}" data-tid="{{.ToolID}}"{{if .ResultErr}} open{{end}}><summary data-debug="{{if .ResultErr}}tool error{{else if .Result}}tool{{else}}tool (running){{end}}"><span class="uc-ico">{{toolEmoji .Name}}</span><b class="uc-name">{{.Name}}</b>{{if .Summary}}<span class="tl-sum">{{.Summary}}</span>{{end}}<i class="uc-state">{{if .ResultErr}}⚠ 失败{{else if .Result}}✓{{else}}<span class="uc-spin"></span>进行中{{end}}</i></summary>{{if .Text}}<pre class="uc-in">{{.Text}}</pre>{{end}}{{if .Result}}<pre class="uc-out {{if .ResultErr}}r-err{{else}}r-ok{{end}}" data-debug="{{if .ResultErr}}tool error{{else}}tool result{{end}}">{{.Result}}</pre>{{end}}</details>{{end}}{{if eq .Kind "result"}}<details class="tl-tool ucall uc-orphan {{if .IsError}}uc-err{{else}}uc-ok{{end}}" data-tid="{{.ToolID}}"{{if .IsError}} open{{end}}><summary data-debug="{{if .IsError}}tool error{{else}}tool result{{end}}"><span class="uc-ico">🧰</span><b class="uc-name">工具回报</b><i class="uc-state">{{if .IsError}}⚠ 失败{{else}}✓{{end}}</i></summary><pre class="uc-out {{if .IsError}}r-err{{else}}r-ok{{end}}" data-debug="{{if .IsError}}tool error{{else}}tool result{{end}}">{{.Text}}</pre></details>{{end}}{{if eq .Kind "interrupt"}}<div class="tl-sys interrupt" data-debug="interrupted"><span class="emo">✋ </span>已中断 · 上一段输出就停在这里</div>{{end}}{{if eq .Kind "continue"}}<div class="tl-sys continue" data-debug="compacted"><span class="emo">🧵 </span>会话曾被压缩，从这里续接</div>{{end}}
{{end}}

<script>
{{define "tlEventJS"}}
(function () {
  var SID = "{{.SID}}";
  var DIR = {{.Dir}}; // html/template 在 JS 上下文自动输出带引号的转义字面量
  function dirQS() { return DIR ? '?dir=' + encodeURIComponent(DIR) : ''; }
  function dirAmp() { return DIR ? '&dir=' + encodeURIComponent(DIR) : ''; }
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
    if (b.kind === 'text') return textHTML(b.text, b.html);
    if (b.kind === 'thinking')
      return '<details class="tl-think"><summary data-debug="thinking"><span class="think-ico">🧠</span><span class="think-tx">已深度思考</span><span class="think-chevron">⌄</span></summary><pre>' + esc(b.text) + '</pre></details>';
    if (b.kind === 'tool') return toolCallHTML({ tid: b.tid, name: b.name, summary: b.summary, input: b.text, result: b.result, rerr: b.rerr });
    if (b.kind === 'result') return orphanResultHTML(b.tid, b.text, b.err);
    if (b.kind === 'interrupt') return '<div class="tl-sys interrupt" data-debug="interrupted"><span class="emo">✋ </span>已中断 · 上一段输出就停在这里</div>';
    if (b.kind === 'continue') return '<div class="tl-sys continue" data-debug="compacted"><span class="emo">🧵 </span>会话曾被压缩，从这里续接</div>';
    return '';
  }
  // assistant 回复：服务端给了安全 Markdown HTML 就富文本渲染，否则纯文本兜底
  function textHTML(text, html) {
    if (html) return '<div class="tl-text md">' + html + '</div>';
    return '<div class="tl-text">' + esc(text) + '</div>';
  }
  // 一张工具卡：命令/文件 + 状态（进行中转圈 / ✓ / ⚠）+ 入参 + 产出，点开看详情
  function toolCallHTML(o) {
    var cls = o.rerr ? 'uc-err' : (o.result ? 'uc-ok' : 'uc-run');
    var state = o.rerr ? '⚠ 失败' : (o.result ? '✓' : '<span class="uc-spin"></span>进行中');
    var inPre = o.input ? '<pre class="uc-in">' + prettyJSON(o.input) + '</pre>' : '';
    var outPre = o.result ? '<pre class="uc-out ' + (o.rerr ? 'r-err' : 'r-ok') + '" data-debug="' + (o.rerr ? 'tool error' : 'tool result') + '">' + esc(o.result) + '</pre>' : '';
    var dbg = o.rerr ? 'tool error' : (o.result ? 'tool' : 'tool (running)');
    return '<details class="tl-tool ucall ' + cls + '" data-tid="' + esc(o.tid || '') + '"' + (o.rerr ? ' open' : '') + '>' +
      '<summary data-debug="' + dbg + '"><span class="uc-ico">' + toolEmoji(o.name) + '</span><b class="uc-name">' + esc(o.name) + '</b>' +
      (o.summary ? '<span class="tl-sum">' + esc(o.summary) + '</span>' : '') +
      '<i class="uc-state">' + state + '</i></summary>' + inPre + outPre + '</details>';
  }
  function orphanResultHTML(tid, text, isErr) {
    return '<details class="tl-tool ucall uc-orphan ' + (isErr ? 'uc-err' : 'uc-ok') + '" data-tid="' + esc(tid || '') + '"' + (isErr ? ' open' : '') + '>' +
      '<summary data-debug="' + (isErr ? 'tool error' : 'tool result') + '"><span class="uc-ico">🧰</span><b class="uc-name">工具回报</b><i class="uc-state">' + (isErr ? '⚠ 失败' : '✓') + '</i></summary>' +
      '<pre class="uc-out ' + (isErr ? 'r-err' : 'r-ok') + '" data-debug="' + (isErr ? 'tool error' : 'tool result') + '">' + esc(text) + '</pre></details>';
  }
  function eventUserText(ev) {
    return (ev.blocks || []).filter(function (b) { return b.kind === 'text'; })
      .map(function (b) { return b.text; }).join('\n');
  }
  function eventHTML(ev) {
    if (ev.role === 'system') {
      return '<div class="tl-ev tl-system" data-id="' + esc(ev.id) + '"><div class="tl-body">' +
        ev.blocks.map(blockHTML).join('') + '</div></div>';
    }
    var cls = 'tl-ev tl-' + ev.role + (ev.side ? ' side' : '') + (onlyResult(ev) ? ' only-result' : '');
    var meta = '';
    if (!onlyResult(ev)) {
      var who = ev.role === 'assistant' && ev.side ? '<span class="tl-who" data-debug="subship">🤖 子代理</span>' : '';
      meta = '<div class="tl-meta">' + who + '<time class="tl-ts" datetime="' + esc(ev.ts) + '">' + esc(ev.ts) + '</time></div>';
    }
    var acts = '';
    if (ev.role === 'user' && !onlyResult(ev)) {
      var ut = eventUserText(ev);
      if (ut) acts = '<div class="bub-acts">' +
        '<button type="button" class="bub-act" data-act="edit" data-debug="edit" data-text="' + esc(ut) + '" title="编辑后重发（会停下当前轮）">✏️</button>' +
        '<button type="button" class="bub-act" data-act="copy" data-debug="copy" data-text="' + esc(ut) + '" title="复制这句话">📋</button></div>';
    }
    return '<div class="' + cls + '" data-id="' + esc(ev.id) + '">' + meta +
      '<div class="tl-body">' + ev.blocks.map(blockHTML).join('') + '</div>' + acts + '</div>';
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

  // ---- 复制：气泡 / 代码块 / 最后一次回复 共用 ----
  function legacyCopy(t) {
    try {
      var ta = document.createElement('textarea');
      ta.value = t; ta.style.position = 'fixed'; ta.style.opacity = '0';
      document.body.appendChild(ta); ta.select();
      var ok = document.execCommand('copy');
      ta.remove();
      return ok;
    } catch (e) { return false; }
  }
  function copyText(t, btn) {
    t = String(t == null ? '' : t).replace(/▍$/, '').replace(/ $/,'');
    function done() {
      if (!btn) return;
      var old = btn.innerHTML;
      btn.innerHTML = '<span class="emo">✓ </span>已复制';
      btn.classList.add('copied');
      setTimeout(function () { btn.innerHTML = old; btn.classList.remove('copied'); }, 1400);
    }
    var p = (navigator.clipboard && navigator.clipboard.writeText)
      ? navigator.clipboard.writeText(t)
      : (legacyCopy(t) ? Promise.resolve() : Promise.reject());
    return p.then(done).catch(function () {
      if (legacyCopy(t)) done(); else flash('⚠️ 复制失败，请长按选择文本', 'note-err');
    });
  }
  // Markdown 代码块右上角挂「复制」（服务端首批 + 增量插入后都要接一次）
  function wireCodeCopy(scope) {
    (scope || listEl).querySelectorAll('.md pre').forEach(function (pre) {
      if (pre.dataset.cw) return;
      pre.dataset.cw = '1';
      var wrap = document.createElement('div');
      wrap.className = 'code-wrap';
      pre.parentNode.insertBefore(wrap, pre);
      wrap.appendChild(pre);
      var c = document.createElement('button');
      c.type = 'button'; c.className = 'code-copy'; c.setAttribute('data-debug', 'copy');
      c.innerHTML = '<span class="emo">📋 </span>复制';
      c.addEventListener('click', function () { copyText((pre.querySelector('code') || pre).innerText, c); });
      wrap.appendChild(c);
    });
  }
  function appendEvents(evs) {
    var empty = listEl.querySelector('.tl-empty');
    if (empty) empty.remove();
    var html = '';
    evs.forEach(function (ev) { if (!seen[ev.id]) { seen[ev.id] = 1; html += eventHTML(ev); } });
    if (html) {
      listEl.insertAdjacentHTML('beforeend', html);
      wireCodeCopy(listEl);
    }
    fmtTimes();
  }
  fmtTimes();
  wireCodeCopy(listEl);
  stick();

  var MARK_TEXT = { good: '✅ 圆满结束', stop: '⏹ 中途停止' };
  var lastState = null;
  function stateKey(d) {
    if (!d.online) return 'offline';
    if (d.status === 'busy') return 'working';
    if (d.status === 'waiting') return 'waiting';
    if (d.status === 'restarting') return 'restarting';
    return 'idle'; // 进程在线、轮次已停，随时能再说话
  }
  function stateText(k) {
    return k === 'working' ? '工作中' : k === 'waiting' ? '待你输入'
      : k === 'restarting' ? '♻️ 重连中' : k === 'idle' ? '待命' : '已停止';
  }

  // ---- 逐字直播：当前轮还没落进 transcript 的 thinking/text/tool 块 ----
  function liveBlockHTML(b) {
    if (b.kind === 'thinking')
      // 思考过程实时展开（像豆包滚动它在想什么），落盘成正式事件后默认折叠
      return '<details class="tl-think tl-live-blk is-live" open>' +
        '<summary data-debug="thinking"><span class="think-ico">🧠</span><span class="think-tx">思考中</span><span class="uc-spin think-spin"></span></summary>' +
        '<pre>' + esc(b.text) + '</pre></details>';
    if (b.kind === 'text')
      // 服务端已按 GFM 渲染（脚本无害转义）；中途未闭合的代码围栏由 goldmark 退化成段落，闭合后自动成形
      return '<div class="tl-text tl-live-blk' + (b.html ? ' md' : '') + '">' +
        (b.html || esc(b.text)) + '<span class="caret-blink">▍</span></div>';
    if (b.kind === 'tool') {
      var res = b._result;
      return toolCallHTML({
        tid: b.tid, name: b.name, summary: b.summary, input: b.text,
        result: res ? res.text : '', rerr: res ? res.err : false
      }).replace('<details ', '<details data-live="1" ');
    }
    if (b.kind === 'result') {
      // 兜底：没找到对应进行中工具（折叠失败/跨帧）时单列
      return orphanResultHTML(b.tid, b.text, b.err);
    }
    return '';
  }
  function renderLive(blocks) {
    var el = document.getElementById('tl-live');
    if (!el) return;
    if (!blocks || !blocks.length) { el.hidden = true; el.innerHTML = ''; return; }
    var stickNow = nearBottom();
    el.innerHTML = foldLive(blocks).map(liveBlockHTML).join('');
    el.hidden = false;
    wireCodeCopy(el);
    if (stickNow) stick();
  }
  // 把 tool_result 并进同 tid 的进行中工具块（命令+产出一张卡）
  function foldLive(blocks) {
    var byId = {}, out = [];
    blocks.forEach(function (b) {
      if (b.kind === 'result' && b.tid && byId[b.tid]) { byId[b.tid]._result = b; }
      else { out.push(b); if (b.kind === 'tool' && b.tid) byId[b.tid] = b; }
    });
    return out;
  }
  // 工具入参是流式 JSON：能解析就缩进美化，没闭合/非 JSON 原样显示
  function prettyJSON(s) {
    var t = String(s || '').trim();
    if (!t) return '';
    try { return esc(JSON.stringify(JSON.parse(t), null, 2)); } catch (e) { return esc(t); }
  }

  // ---- 忙时排队：CLI 自动排队续跑，这里让人看见队里有几句、可一键全撤 ----
  function renderQueue(queued) {
    var box = document.getElementById('tl-queue');
    if (!box) return;
    var n = queued ? queued.length : 0;
    box.hidden = n === 0;
    if (!n) return;
    document.getElementById('q-n').textContent = String(n);
    document.getElementById('q-list').innerHTML = queued.map(function (q) {
      return '<div class="q-item"><span class="emo">🧭 </span>' + esc(q) + '</div>';
    }).join('');
  }

  // ---- 手机端工具审批：卡片 + 批准/拒绝 + 本船队放手模式 ----
  function ageWord(sec) {
    sec = sec || 0;
    if (sec < 60) return '刚刚';
    if (sec < 3600) return Math.floor(sec / 60) + ' 分钟';
    return Math.floor(sec / 3600) + ' 小时';
  }
  function permCardHTML(p) {
    var detail = (p.detail && p.detail !== p.summary)
      ? '<pre class="perm-detail">' + esc(p.detail) + '</pre>' : '';
    return '<div class="perm-card" data-pid="' + esc(p.id) + '">' +
      '<div class="perm-head"><span class="perm-tool"><span class="emo">' + (p.emoji || '🔧') +
      ' </span><b>' + esc(p.tool) + '</b></span><span class="perm-age">' + ageWord(p.age_sec) + '</span></div>' +
      '<div class="perm-sum">' + esc(p.summary || p.tool) + '</div>' + detail +
      '<div class="perm-actions"><button type="button" class="perm-yes" data-dec="allow" data-debug="perm allow">✅ 批准执行</button>' +
      '<button type="button" class="perm-no" data-dec="deny" data-debug="perm deny">⛔ 拒绝</button></div></div>';
  }
  function renderPerms(perms, on) {
    var box = document.getElementById('tl-perms');
    if (!box) return;
    if (!on || !perms || !perms.length) { box.hidden = true; box.innerHTML = ''; return; }
    var stickNow = nearBottom();
    box.innerHTML = '<div class="perm-banner"><span class="emo">🔐 </span>船队要执行操作，等你拍板（不批 5 分钟后自动拒绝）：</div>' +
      perms.map(permCardHTML).join('');
    box.hidden = false;
    if (stickNow) stick();
  }
  function postPerm(pid, decision) {
    var body = new URLSearchParams();
    body.set('id', pid); body.set('decision', decision);
    return fetch('/sessions/' + encodeURIComponent(SID) + '/perm/decide' + dirQS(), {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: body
    }).then(function (r) { if (r.status === 401) { location.href = '/login'; throw 0; } return r.json(); });
  }
  var permBox = document.getElementById('tl-perms');
  if (permBox) permBox.addEventListener('click', function (e) {
    var btn = e.target.closest('button[data-dec]');
    if (!btn) return;
    var card = btn.closest('.perm-card');
    var pid = card && card.getAttribute('data-pid');
    if (!pid) return;
    var dec = btn.getAttribute('data-dec');
    card.querySelectorAll('button').forEach(function (b) { b.disabled = true; });
    postPerm(pid, dec).then(function (d) {
      if (d.error) { flash('⚠️ ' + d.error, 'note-err'); card.querySelectorAll('button').forEach(function (b) { b.disabled = false; }); return; }
      card.classList.add('decided', 'dec-' + dec);
      card.querySelector('.perm-actions').innerHTML = dec === 'allow'
        ? '<span class="perm-done ok">✅ 已批准，执行中</span>' : '<span class="perm-done no">⛔ 已拒绝</span>';
      kickPoll();
    }).catch(function (er) { if (er) flash('⚠️ 审批没递出去', 'note-err'); });
  });
  // ---- Auto / Plan / 无人值守：输入坞上一颗不占地的模式芯片 ----
  var MODE_META = {
    auto: { ico: '⚡', word: 'Auto' },
    plan: { ico: '🗺️', word: 'Plan' },
    yolo: { ico: '🔓', word: '无人值守' }
  };
  var modeChip = document.getElementById('mode-chip'),
      modePop = document.getElementById('mode-pop'),
      modeMenu = document.getElementById('mode-menu');
  function paintMode(mode) {
    mode = MODE_META[mode] ? mode : 'auto';
    if (!modeChip) return;
    modeChip.setAttribute('data-mode', mode);
    modeChip.classList.toggle('is-plan', mode === 'plan');
    modeChip.classList.toggle('is-yolo', mode === 'yolo');
    document.getElementById('mode-ico').textContent = MODE_META[mode].ico;
    document.getElementById('mode-word').textContent = MODE_META[mode].word;
    if (modePop) modePop.querySelectorAll('.mode-opt').forEach(function (b) {
      b.classList.toggle('sel', b.getAttribute('data-mode') === mode);
    });
  }
  if (modeChip) {
    modeChip.addEventListener('click', function (e) {
      e.stopPropagation();
      modePop.hidden = !modePop.hidden;
    });
    modePop.addEventListener('click', function (e) {
      var opt = e.target.closest('.mode-opt');
      if (!opt) return;
      var mode = opt.getAttribute('data-mode');
      modePop.hidden = true;
      var body = new URLSearchParams(); body.set('mode', mode);
      modeChip.disabled = true;
      fetch('/sessions/' + encodeURIComponent(SID) + '/mode' + dirQS(), {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: body
      }).then(function (r) { if (r.status === 401) { location.href = '/login'; throw 0; } return r.json(); })
        .then(function (d) {
          if (d.error) { flash('⚠️ ' + d.error, 'note-err'); return; }
          paintMode(d.mode);
          if (d.mode === 'plan') flash('🗺️ Plan 模式：只调研出计划，不会动手改东西', 'note-ok');
          else if (d.mode === 'yolo') flash('🔓 无人值守：这轮起不再逐条请批，谨慎使用', 'note-ok');
          else flash('⚡ Auto 模式：能自动的自动干，危险操作仍会请批', 'note-ok');
          if (d.note) flash('ℹ️ ' + d.note, 'note-err');
          kickPoll();
        }).catch(function (er) { if (er) flash('⚠️ 模式没切成', 'note-err'); })
        .finally(function () { modeChip.disabled = false; });
    });
    document.addEventListener('click', function (e) {
      if (modeMenu && !modeMenu.contains(e.target)) modePop.hidden = true;
    });
    paintMode('auto');
  }

  // ---- 手机通知：完成 / 待你输入 / 待审批（仅页面在后台时响）----
  var notifyBtn = document.getElementById('btn-notify'),
    notifyWord = document.getElementById('notify-word');
  function notifyOn() {
    return ('Notification' in window) && Notification.permission === 'granted' &&
      localStorage.getItem('mp-notify') === '1';
  }
  function paintNotify() {
    if (!notifyBtn) return;
    if (!('Notification' in window)) { notifyBtn.disabled = true; notifyWord.textContent = '不支持'; return; }
    if (Notification.permission === 'denied') { notifyBtn.disabled = true; notifyWord.textContent = '被浏览器拦'; return; }
    notifyBtn.classList.toggle('on', notifyOn());
    notifyWord.textContent = notifyOn() ? '通知开' : (Notification.permission === 'granted' ? '点此开通知' : '通知');
  }
  function ping(title, body, tag) {
    if (!notifyOn() || !document.hidden) return;
    try {
      var n = new Notification(title, { body: body, tag: tag || 'mindport', renotify: true });
      n.onclick = function () { window.focus(); n.close(); };
    } catch (e) {}
  }
  if (notifyBtn) notifyBtn.addEventListener('click', function () {
    if (!('Notification' in window)) return;
    if (Notification.permission === 'granted') {
      var turnOn = localStorage.getItem('mp-notify') !== '1';
      localStorage.setItem('mp-notify', turnOn ? '1' : '0');
      if (turnOn) ping('思港通知已开', '船队完成或待你审批时会提醒你', 'mp-test');
      paintNotify();
    } else {
      Notification.requestPermission().then(function (p) {
        if (p === 'granted') { localStorage.setItem('mp-notify', '1'); ping('思港通知已开', '以后船队完成会提醒你', 'mp-test'); }
        paintNotify();
      });
    }
  });
  paintNotify();

  // 状态翻转 → 通知（只在真实轮询数据上跑；首次只建档不响，避免一开页就吵）
  var prevStatus = null, prevPermIds = null;
  function checkNotify(d) {
    var s = stateKey(d);
    var idSet = {};
    (d.perms || []).forEach(function (p) { idSet[p.id] = p; });
    if (prevPermIds) {
      Object.keys(idSet).forEach(function (id) {
        if (!prevPermIds[id]) {
          var p = idSet[id];
          ping('🔐 待审批 · ' + p.tool, p.summary || p.tool, 'perm-' + id);
        }
      });
    }
    if (prevStatus === 'working' && s === 'idle') {
      ping('✅ 船队跑完一轮', '回到待命，可以再派下一句', 'done-' + SID);
    } else if (prevStatus === 'working' && s === 'waiting') {
      ping('⏸ 船队待你输入', d.waiting_for || '它停下等你说话', 'wait-' + SID);
    }
    prevStatus = s;
    prevPermIds = idSet;
  }

  // ---- 乐观气泡：话一送出就先挂上「你」的气泡，权威 transcript 行到了再顶替 ----
  var pendingMine = [];
  var pendingSeq = 0;
  function appendPendingUser(msg, tag) {
    var seq = ++pendingSeq;
    var el = document.createElement('div');
    el.className = 'tl-ev tl-user tl-pending';
    el.setAttribute('data-pseq', String(seq));
    var now = new Date();
    var iso = now.toISOString();
    var label = String(now.getHours()).padStart(2, '0') + ':' +
      String(now.getMinutes()).padStart(2, '0') + ':' + String(now.getSeconds()).padStart(2, '0');
    el.innerHTML = '<div class="tl-meta"><time class="tl-ts" datetime="' + iso + '">' + label +
      '</time><i class="pend-tag' + (tag === '待终端接收' ? ' pend-wait' : '') + '">' + esc(tag || '已送达') +
      '</i></div><div class="tl-body"><div class="tl-text">' + esc(msg) + '</div></div>' +
      '<div class="bub-acts"><button type="button" class="bub-act" data-act="edit" data-debug="edit" data-text="' + esc(msg) +
      '" title="取回输入框，改完重发">✏️</button>' +
      '<button type="button" class="bub-act" data-act="copy" data-debug="copy" data-text="' + esc(msg) +
      '" title="复制这句话">📋</button></div>';
    listEl.appendChild(el);
    stick();
    return { seq: seq, text: msg };
  }
  function reconcilePending(evs) {
    if (!pendingMine.length || !evs || !evs.length) return;
    evs.forEach(function (ev) {
      if (ev.role !== 'user') return;
      var txt = (ev.blocks || []).filter(function (b) { return b.kind === 'text'; })
        .map(function (b) { return b.text; }).join('');
      for (var i = 0; i < pendingMine.length; i++) {
        if (pendingMine[i].text === txt) {
          var el = listEl.querySelector('[data-pseq="' + pendingMine[i].seq + '"]');
          if (el) el.remove();
          pendingMine.splice(i, 1);
          break;
        }
      }
    });
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
      'st-restarting', 'marked', 'mark-good', 'mark-stop');
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
    var busy = d.online && d.status === 'busy' && !d.mark;
    var restarting = d.online && d.status === 'restarting' && !d.mark;
    renderLive(busy ? d.streaming : null);
    // 重连中排队话照样亮着：它们会由新桥原样重投，让人确认话没丢
    renderQueue((busy || restarting) ? d.queued : null);
    renderPerms(d.perms, d.perm_on);
    if (d.mode) paintMode(d.mode);
    if (typing) typing.hidden = !(busy && !(d.streaming && d.streaming.length));
    refreshDock();
    syncMsgActions(!busy && !restarting);
    if (inp && document.documentElement.dataset.stealth !== '1') {
      inp.placeholder = busy
        ? '船队正忙：发送 = 停下这轮换跑你这句；Ctrl+Enter = 排队等它跑完'
        : restarting ? '♻️ 新桥接续中，马上就能继续说…'
        : '说点什么…（Enter 发送）';
    }
    if (pend) { if (d.pending > 0) { pn.textContent = d.pending; pend.hidden = false; } else pend.hidden = true; }
  }

  // ---- 豆包式发送坞：空闲=发送；忙+有字=停止并改发；忙+空=停止 ----
  var dockMode = 'send';
  function refreshDock() {
    if (!btn || !input) return;
    var busyNow = !!(lastState && stateKey(lastState) === 'working');
    var hasText = input.value.trim() !== '';
    var mode = busyNow ? (hasText ? 'stopfirst' : 'stop') : 'send';
    dockMode = mode;
    btn.classList.toggle('is-stop', mode === 'stop');
    btn.classList.toggle('is-stopfirst', mode === 'stopfirst');
    if (mode === 'stop') {
      btn.innerHTML = '■';
      btn.title = '停止当前回答（等同终端 Esc）';
      btn.setAttribute('aria-label', '停止生成');
      btn.setAttribute('data-debug', 'stop');
    } else if (mode === 'stopfirst') {
      btn.innerHTML = '⏩';
      btn.title = '停止当前回答，立刻改发这句';
      btn.setAttribute('aria-label', '停止并发送');
      btn.setAttribute('data-debug', 'stop+send');
    } else {
      btn.innerHTML = '<span class="emo">📨 </span>发送';
      btn.title = '发送（Enter）';
      btn.setAttribute('aria-label', '发送');
      btn.setAttribute('data-debug', 'send');
    }
  }
  // 「重新生成 / 复制」操作条：待命且画面里有船队回复时才出现
  function lastAssistantText() {
    var live = document.getElementById('tl-live');
    var nodes = (live && !live.hidden) ? live.querySelectorAll('.tl-text') : [];
    if (!nodes.length) nodes = listEl.querySelectorAll('.tl-assistant:not(.superseded) .tl-text');
    if (!nodes.length) return '';
    return nodes[nodes.length - 1].innerText;
  }
  function syncMsgActions(show) {
    var bar = document.getElementById('tl-actions');
    if (!bar) return;
    var hasReply = !!listEl.querySelector('.tl-assistant:not(.superseded) .tl-text') ||
      !!((function () {
        var live = document.getElementById('tl-live');
        return live && !live.hidden && live.querySelector('.tl-text');
      })());
    bar.hidden = !(show && hasReply);
  }
  // 重新生成：把最后一句真人话之后的旧回答客户端折叠（线性 transcript 不能云端分叉）
  function supersedeLastTurn() {
    var users = listEl.querySelectorAll('.tl-user:not(.tl-pending)');
    var lastUser = users[users.length - 1];
    if (!lastUser) return;
    var n = lastUser.nextElementSibling, killed = [];
    while (n) {
      if (n.classList.contains('tl-user')) break;
      if (n.classList.contains('tl-assistant') || n.classList.contains('tl-system')) killed.push(n);
      n = n.nextElementSibling;
    }
    if (!killed.length) return;
    var tag = document.createElement('button');
    tag.type = 'button'; tag.className = 'tl-sys regen-tag';
    tag.setAttribute('data-debug', 'regenerated (click to toggle)');
    tag.textContent = '↻ 上一版回答已折叠，点这里对照查看';
    lastUser.parentNode.insertBefore(tag, killed[0]);
    function apply(hide) { killed.forEach(function (el) { el.classList.toggle('superseded', hide); }); }
    apply(true);
    tag.addEventListener('click', function () {
      var expanded = tag.classList.toggle('expanded');
      apply(!expanded);
      tag.textContent = expanded ? '↻ 收起上一版回答' : '↻ 上一版回答已折叠，点这里对照查看';
    });
  }

  // 增量轮询：SSE 事件即时踢一脚，1s 轮询兜底；页面隐藏时暂停
  var timer = null, active = false, requeue = false, kickTimer = null;
  var failStreak = 0;
  function setBanner(show) {
    var b = document.getElementById('conn-banner');
    if (b) b.hidden = !show;
  }
  function poll() {
    if (active) { requeue = true; return; }
    active = true;
    fetch('/api/sessions/' + encodeURIComponent(SID) + '/stream?from=' + cursor + dirAmp(), { headers: { 'Accept': 'application/json' } })
      .then(function (r) { if (r.status === 401) { location.href = '/login'; throw new Error('auth'); } return r.json(); })
      .then(function (d) {
        failStreak = 0; setBanner(false);
        if (d.events && d.events.length) {
          var stickNow = nearBottom();
          appendEvents(d.events);
          reconcilePending(d.events);
          if (stickNow) stick();
        }
        if (d.cursor) cursor = d.cursor;
        setStatus({ online: d.online, status: d.status, mark: d.mark,
          waiting_for: d.waiting_for, pending: d.pending,
          streaming: d.streaming, queued: d.queued, bridge: !!d.bridge,
          perms: d.perms, mode: d.mode, perm_on: d.perm_on });
        checkNotify(d);
      }).catch(function (e) {
        if (e && e.message === 'auth') return;
        failStreak++;
        if (failStreak >= 3) setBanner(true);
      }).finally(function () {
        active = false;
        if (requeue) { requeue = false; setTimeout(poll, 60); }
      });
  }
  function kickPoll() { // SSE 事件合并 90ms 内的风暴
    if (kickTimer) return;
    kickTimer = setTimeout(function () { kickTimer = null; poll(); }, 90);
  }
  try {
    var es = new EventSource('/api/live');
    es.onmessage = function (e) {
      var ev; try { ev = JSON.parse(e.data); } catch (x) { return; }
      if (ev.type === 'perm' || (ev.sid === SID && (ev.type === 'state' || ev.type === 'partial'))) {
        failStreak = 0; setBanner(false);
        kickPoll();
      }
    };
  } catch (e) { /* SSE 起不来就纯 1s 轮询 */ }
  function startTimer() { if (!timer) timer = setInterval(poll, 1000); }
  function stopTimer() { if (timer) { clearInterval(timer); timer = null; } }
  document.addEventListener('visibilitychange', function () {
    if (document.hidden) stopTimer(); else { poll(); startTimer(); }
  });
  startTimer();
  refreshDock();
  syncMsgActions(true);

  // 手机键盘避让：把 visualViewport 的实时高度写给 CSS（.session 用 var(--vvh)），
  // 输入坞不再被软键盘盖住
  if (window.visualViewport) {
    var vv = window.visualViewport;
    function syncVV() {
      document.documentElement.style.setProperty('--vvh', vv.height + 'px');
    }
    vv.addEventListener('resize', syncVV);
    vv.addEventListener('scroll', syncVV);
    syncVV();
  }

  // 上翻：128KB 一块，行边界对齐，插入后保持视觉位置
  var loadingOlder = false;
  function loadOlder() {
    if (loadingOlder || !hasMore) return Promise.resolve();
    loadingOlder = true;
    var btn = document.getElementById('tl-more');
    if (btn) btn.disabled = true;
    return fetch('/api/sessions/' + encodeURIComponent(SID) + '/history?before=' + oldest + dirAmp())
      .then(function (r) { return r.json(); })
      .then(function (d) {
        if (!d.events) return;
        var prevH = wrapEl.scrollHeight, html = '';
        d.events.forEach(function (ev) { if (!seen[ev.id]) { seen[ev.id] = 1; html += eventHTML(ev); } });
        if (html) {
          listEl.insertAdjacentHTML('afterbegin', html);
          wireCodeCopy(listEl);
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
  var enqueueHint = false; // Ctrl/Cmd+Enter = 忙时排队等跑完
  if (input) {
    input.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
        e.preventDefault();
        enqueueHint = !!(e.ctrlKey || e.metaKey);
        form.requestSubmit();
      }
    });
    input.addEventListener('input', function () {
      input.style.height = 'auto';
      input.style.height = Math.min(input.scrollHeight, 120) + 'px';
      refreshDock();
    });
  }
  // 忙且输入框空时，主按钮是「■ 停止」：点击只停，不发空消息
  if (btn) btn.addEventListener('click', function (e) {
    if (dockMode === 'stop') {
      e.preventDefault();
      btn.disabled = true;
      postAction('stop', { cancel_all: '0' }).then(function (d) {
        if (d.error) flash('⚠️ ' + d.error, 'note-err');
        else flash('✋ 已停止，船队就停在这里', 'note-ok');
        kickPoll();
      }).catch(function () { flash('⚠️ 停止令没送达', 'note-err'); })
        .finally(function () { btn.disabled = false; refreshDock(); });
    }
  });
  // ---- 🎤 豆包式按住说话：短按照常打字/发送，麦克风按住说话、松开发送、上滑取消 ----
  var mic = document.getElementById('say-mic'),
      vOverlay = document.getElementById('voice-overlay'),
      vTx = document.getElementById('vo-tx'),
      vInner = document.getElementById('vo-inner');
  var SR = window.SpeechRecognition || window.webkitSpeechRecognition;
  var voiceOK = !!SR && window.isSecureContext !== false;
  if (mic) {
    mic.hidden = false;
    if (!SR) {
      mic.classList.add('mic-na');
      mic.title = '此浏览器不支持语音识别（建议手机 Chrome / iOS 14.5+ Safari）';
    } else if (window.isSecureContext === false) {
      mic.classList.add('mic-na');
      mic.title = '语音识别要求 HTTPS 或本机 localhost（当前 http 地址会被浏览器禁用麦克风）';
    }
  }
  var recog = null, recFinal = '', recActive = false, recCancelled = false, startY = 0;
  function voiceTip() {
    if (!SR) flash('🎤 这个浏览器不支持语音识别，建议手机 Chrome 或 iOS 14.5 以上 Safari', 'note-err');
    else flash('🔒 语音识别要用 https 打开思港（当前 http 地址，浏览器禁用了麦克风）', 'note-err');
  }
  function setVoiceText(t) { if (vTx) vTx.textContent = t || '正在听…'; }
  function startVoice(e) {
    e.preventDefault();
    if (!voiceOK) { voiceTip(); return; }
    if (recActive) return;
    try { mic.setPointerCapture(e.pointerId); } catch (x) {}
    recog = new SR();
    recog.lang = 'zh-CN'; recog.interimResults = true; recog.continuous = false; recog.maxAlternatives = 1;
    recFinal = ''; recCancelled = false;
    startY = e.clientY;
    recog.onresult = function (ev) {
      var interim = '';
      for (var i = ev.resultIndex; i < ev.results.length; i++) {
        var tr = ev.results[i];
        if (tr.isFinal) recFinal += tr[0].transcript; else interim += tr[0].transcript;
      }
      setVoiceText((recFinal + interim).replace(/\s+/g, ' ').trim());
    };
    recog.onerror = function (ev) {
      if (ev.error === 'not-allowed' || ev.error === 'service-not-allowed')
        flash('🎤 麦克风权限被拒，请在浏览器地址栏允许麦克风后重试', 'note-err');
      else if (ev.error && ev.error !== 'aborted' && ev.error !== 'no-speech' && ev.error !== 'canceled')
        flash('🎤 语音识别出错：' + ev.error, 'note-err');
    };
    recog.onend = function () { finishVoice(); };
    try { recog.start(); recActive = true; mic.classList.add('recording'); if (vOverlay) { vOverlay.hidden = false; vInner.classList.remove('cancel'); } setVoiceText('正在听…'); }
    catch (x) { recActive = false; }
  }
  function finishVoice() {
    if (!recActive) return;
    recActive = false;
    mic.classList.remove('recording');
    if (vOverlay) vOverlay.hidden = true;
    if (vInner) vInner.classList.remove('cancel');
    var text = (recFinal || '').replace(/\s+/g, ' ').trim();
    if (!recCancelled && text) {
      // 追加到已有文字后，松开即发（与豆包按住说话一致）
      input.value = (input.value.trim() ? input.value.trim() + ' ' : '') + text;
      input.dispatchEvent(new Event('input'));
      form.requestSubmit();
    }
  }
  if (mic) {
    mic.addEventListener('pointerdown', startVoice);
    mic.addEventListener('pointermove', function (e) {
      if (!recActive) return;
      if (startY - e.clientY > 50) { recCancelled = true; if (vInner) vInner.classList.add('cancel'); setVoiceText('松开取消'); }
      else if (startY - e.clientY < 30) { recCancelled = false; if (vInner) vInner.classList.remove('cancel'); setVoiceText('松开发送 · 上滑取消'); }
    });
    mic.addEventListener('pointerup', function () {
      if (!recActive) return;
      if (recCancelled) { try { recog.abort(); } catch (x) {} finishVoice(); }
      else { try { recog.stop(); } catch (x) {} }
    });
    mic.addEventListener('pointercancel', function () { if (recActive) { recCancelled = true; try { recog.abort(); } catch (x) {} finishVoice(); } });
    mic.addEventListener('contextmenu', function (e) { e.preventDefault(); });
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
    // 豆包式插话：忙时直接发送 = 停掉当前轮换跑这句；Ctrl/Cmd+Enter 才排队等它跑完
    var stopFirst = dockMode === 'stopfirst' && !enqueueHint;
    enqueueHint = false;
    if (stopFirst) body.set('stop_first', '1');
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
        if (d.delivered === 'mail') {
          // 桥真起不来才落信箱：话也要挂出来（标待接收），绝不能像没发过一样消失
          pendingMine.push(appendPendingUser(msg, '待终端接收'));
          flash('📬 ' + d.detail + '；你的话已留在画面上，终端会话下一轮会收到', 'note-mail');
        } else {
          // 即时桥：先挂乐观气泡，权威 transcript 行到了自动顶替
          pendingMine.push(appendPendingUser(msg, d.delivered === 'live' && d.detail && d.detail.indexOf('唤醒') >= 0 ? '唤醒中…' : '已送达'));
          flash('⛵ ' + (d.detail || '已送达，船队立刻接着跑'), 'note-ok');
          // 重连中发的话：服务端会等新桥就位再注入，本地维持 restarting 态
          if (lastState && lastState.status !== 'restarting')
            setStatus(Object.assign({}, lastState, { online: true, status: 'busy', mark: '' }));
        }
        kickPoll();
        setTimeout(poll, 400);
      })
      .catch(function (e) { if (e) flash('⚠️ 网络岔了，没递出去', 'note-err'); })
      .finally(function () { btn.disabled = false; refreshDock(); });
  });

  // 气泡操作（事件委托）：✏️ 取回输入框改完重发；📋 复制这句话
  listEl.addEventListener('click', function (e) {
    var b = e.target.closest('button.bub-act');
    if (!b) return;
    var act = b.getAttribute('data-act'), text = b.getAttribute('data-text') || '';
    if (act === 'copy') { copyText(text, b); return; }
    if (act === 'edit') {
      input.value = text;
      input.style.height = 'auto';
      input.style.height = Math.min(input.scrollHeight, 120) + 'px';
      input.focus();
      refreshDock();
      flash('✏️ 已取回输入框：改完直接发送（船队正忙时会停下当前轮再发）', 'note-ok');
    }
  });
  // 🔄 重新生成：中断当前轮，就上一句话重新作答
  var regenBtn = document.getElementById('btn-regen');
  if (regenBtn) regenBtn.addEventListener('click', function () {
    regenBtn.disabled = true;
    fetch(actionURL('regenerate'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' }
    }).then(function (r) { if (r.status === 401) { location.href = '/login'; throw 0; } return r.json(); })
      .then(function (d) {
        if (d.error) { flash('⚠️ ' + d.error, 'note-err'); return; }
        supersedeLastTurn();
        flash('🔄 ' + (d.detail || '已让船队重新作答'), 'note-ok');
        if (lastState) setStatus(Object.assign({}, lastState, { online: true, status: 'busy', mark: '' }));
        kickPoll(); setTimeout(poll, 400);
      }).catch(function (e) { if (e) flash('⚠️ 重新生成没递出去', 'note-err'); })
      .finally(function () { regenBtn.disabled = false; });
  });
  // 📋 复制船队最近一次回复
  var copyLastBtn = document.getElementById('btn-copy-last');
  if (copyLastBtn) copyLastBtn.addEventListener('click', function () {
    var t = lastAssistantText();
    if (!t) { flash('还没有可复制的回复', 'note-err'); return; }
    copyText(t, copyLastBtn);
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

  // ---- 即时桥动作：中断这轮（Esc）/ 中断并清空排队（Ctrl-X）/ 拆船 ----
  function actionURL(act) {
    return '/sessions/' + encodeURIComponent(SID) + '/' + act + (DIR ? '?dir=' + encodeURIComponent(DIR) : '');
  }
  function postAction(act, params) {
    return fetch(actionURL(act), {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams(params || {})
    }).then(function (r) { if (r.status === 401) { location.href = '/login'; throw 0; } return r.json(); });
  }
  var qCancel = document.getElementById('q-cancel');
  if (qCancel) qCancel.addEventListener('click', function () {
    postAction('stop', { cancel_all: '1' }).then(function (d) {
      if (d.error) { flash('⚠️ ' + d.error, 'note-err'); return; }
      flash('✋ 已中断，排队的话全部撤回', 'note-ok');
      kickPoll();
    }).catch(function () { flash('⚠️ 没撤成', 'note-err'); });
  });
  var scuttle = document.getElementById('btn-scuttle');
  if (scuttle) scuttle.addEventListener('click', function () {
    var name = document.querySelector('.session h1') ? document.querySelector('.session h1').textContent.trim() : '这支船队';
    if (!window.confirm('拆船？「' + name + '」会从思港消失（CC 的历史文件保留，不在列表删除），不可撤销。')) return;
    scuttle.disabled = true;
    postAction('delete', {}).then(function (d) {
      if (d.error) { flash('⚠️ ' + d.error, 'note-err'); scuttle.disabled = false; return; }
      location.href = '/sessions';
    }).catch(function () { flash('⚠️ 拆船令没送达', 'note-err'); scuttle.disabled = false; });
  });
})();
{{end}}
{{template "tlEventJS" .}}
</script>
{{template "tabbar" .}}
</body></html>`
