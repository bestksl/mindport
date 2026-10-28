// 新开船队统一提交器（v0.8）：任何 <form data-fleet-new> 自动接管。
// - fetch 提交到表单原生 action（/fleets/new 或 /tasks/Txxxx/launch）
// - 成功按返回 {url} 跳转；错误就地显示在表单内 [data-fn-err]
// - 提交期间锁按钮防双击（旧版双击能开出两支船）
(function () {
  function bind(form) {
    if (form.__fnBound) return;
    form.__fnBound = true;
    form.addEventListener('submit', function (e) {
      e.preventDefault();
      var errEl = form.querySelector('[data-fn-err]') || document.getElementById('fn-err');
      var btn = form.querySelector('button[type="submit"]');
      var orig = btn ? btn.textContent : '';
      if (errEl) { errEl.textContent = ''; errEl.hidden = true; }
      if (btn) { btn.disabled = true; btn.textContent = '⛵ 开船中…'; }

      fetch(form.getAttribute('action'), {
        method: 'POST',
        body: new FormData(form),
        credentials: 'same-origin'
      })
        .then(function (r) {
          return r.json().then(
            function (d) { return { ok: r.ok, d: d }; },
            function () { return { ok: false, d: { error: '服务端回了个看不懂的响应（HTTP ' + r.status + '）' } }; }
          );
        })
        .then(function (res) {
          if (res.ok && res.d && res.d.url) {
            // 保住开船现场：整页跳过去看逐字直播
            location.href = res.d.url;
            return;
          }
          if (res.ok && form.dataset.success === 'reload') { location.reload(); return; }
          var msg = (res.d && res.d.error) || '开船失败，稍后再试';
          if (errEl) { errEl.textContent = '⚠️ ' + msg; errEl.hidden = false; }
          if (btn) { btn.disabled = false; btn.textContent = orig; }
        })
        .catch(function () {
          if (errEl) { errEl.textContent = '⚠️ 网络岔了，没开成'; errEl.hidden = false; }
          if (btn) { btn.disabled = false; btn.textContent = orig; }
        });
    });
  }

  function initAll() {
    document.querySelectorAll('form[data-fleet-new]').forEach(bind);
  }
  initAll();
  document.addEventListener('mp-ready', initAll);
})();
