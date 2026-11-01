// 思港 Service Worker：MVP 阶段仅缓存应用外壳，页面永远走网络
// （任务状态必须实时；实时推送走 /events 的 SSE，不经 SW）。
const CACHE = "mindport-v30";
const SHELL = ["/", "/static/style.css", "/static/fleet-new.js", "/icon.svg", "/manifest.webmanifest"];

self.addEventListener("install", (e) => {
  e.waitUntil(caches.open(CACHE).then((c) => c.addAll(SHELL)).catch(() => {}));
  self.skipWaiting();
});

self.addEventListener("activate", (e) => {
  e.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k)))
    )
  );
  self.clients.claim();
});

self.addEventListener("fetch", (e) => {
  const req = e.request;
  const url = new URL(req.url);
  if (url.pathname === "/events" || url.pathname.startsWith("/api/")) return; // 直连，不拦截
  if (req.method !== "GET") return; // POST（录入/领单）一律直连
  // 页面永远网络优先，失败才回退缓存
  if (req.mode === "navigate") {
    e.respondWith(
      fetch(req).catch(() => caches.match(req).then((r) => r || caches.match("/")))
    );
    return;
  }
  // 静态资源 stale-while-revalidate：先用缓存瞬开，后台拉新版本落缓存，
  // 这样升级二进制后下一次刷新就能看到新 CSS（旧策略是缓存优先且永不刷新）
  e.respondWith(
    caches.match(req).then((cached) => {
      const network = fetch(req)
        .then((res) => {
          if (res.ok) {
            const copy = res.clone();
            caches.open(CACHE).then((c) => c.put(req, copy));
          }
          return res;
        })
        .catch(() => cached);
      return cached || network;
    })
  );
});
