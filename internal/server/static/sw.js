// 思港 Service Worker：MVP 阶段仅缓存应用外壳，页面永远走网络
// （任务状态必须实时；未来再加断网暂存与补传）。
const CACHE = "mindport-v0";
const SHELL = ["/", "/static/style.css", "/icon.svg", "/manifest.webmanifest"];

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
  if (req.method !== "GET") return; // POST（录入/领单）一律直连
  // 页面与 API 永远网络优先，失败才回退缓存
  if (req.mode === "navigate" || req.url.includes("/api/")) {
    e.respondWith(
      fetch(req).catch(() => caches.match(req).then((r) => r || caches.match("/")))
    );
    return;
  }
  // 静态资源缓存优先
  e.respondWith(caches.match(req).then((r) => r || fetch(req)));
});
