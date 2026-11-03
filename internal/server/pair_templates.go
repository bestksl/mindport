package server

// 配对落地页：短码放在 URL fragment（#code=XXXX），不会进服务端日志/Referer。
// 页面 JS 取码后 POST /api/pair 换取本设备独立会话；取不到/失败时露出
// 手动输入表单（表单直交 /api/pair，无 JS 也能完成，服务端 302 回首屏）。
const pairHTML = head + `
<main class="login-wrap">
  <div class="login-card" data-debug="pairing">
    <div class="login-brand" data-debug="mindportd">
      <span class="brand-mark">{{template "pxLighthouse" .}}</span>
      <span class="brand-text">思港 <i>MindPort</i></span>
    </div>
    <h1 class="pair-title" id="pair-title">正在配对这台设备…</h1>
    <div class="login-err" id="pair-err" {{if not .Err}}hidden{{end}}>⚠️ <span id="pair-err-text">{{.Err}}</span></div>

    <form id="pair-form" class="pair-form" method="post" action="/api/pair" hidden>
      <p class="login-sub">输入电脑上 <code>port pair</code> 给出的 8 位配对码</p>
      <input name="code" class="pair-code-input" placeholder="配对码" autocomplete="one-time-code"
             maxlength="8" pattern="[23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{8}" autofocus required>
      <button type="submit" class="primary">完成配对</button>
    </form>

    <p class="pair-ok" id="pair-ok" hidden>配对成功，正在进入…</p>
  </div>
</main>
<script>
(function () {
  var form = document.getElementById('pair-form');
  var title = document.getElementById('pair-title');
  var errBox = document.getElementById('pair-err');
  var errText = document.getElementById('pair-err-text');
  var okBox = document.getElementById('pair-ok');

  function showForm() {
    form.hidden = false;
    title.textContent = '把这台设备接入思港';
    var input = form.querySelector('input[name=code]');
    if (input) input.focus();
  }
  function fail(msg) {
    errText.textContent = msg || '配对失败';
    errBox.hidden = false;
    showForm();
  }

  var params = new URLSearchParams(location.hash.replace(/^#/, ''));
  var code = (params.get('code') || '').trim().toUpperCase();
  if (!code) { showForm(); return; }

  fetch('/api/pair', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code: code })
  }).then(function (r) {
    return r.json().then(function (d) { return { ok: r.ok, d: d }; });
  }).then(function (res) {
    if (res.ok && res.d.ok) {
      title.hidden = true;
      okBox.hidden = false;
      location.replace('/');
    } else {
      fail(res.d && res.d.error);
    }
  }).catch(function () { fail('网络错误，请手动输入配对码'); });
})();
</script>
</body></html>`
