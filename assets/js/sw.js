// Service worker, served at /sw.js so it covers the whole site. It shows the
// push notifications that wake the owner's phone (push/push.go sends them)
// and opens the phone app when one is tapped.

self.addEventListener("install", function () { self.skipWaiting(); });
self.addEventListener("activate", function (e) { e.waitUntil(self.clients.claim()); });

self.addEventListener("push", function (e) {
  var n = {};
  try { n = e.data ? e.data.json() : {}; } catch (err) { n = { title: "minivoip", body: e.data && e.data.text() }; }
  var ring = n.kind === "ring";
  e.waitUntil(self.registration.showNotification(n.title || "minivoip", {
    body: n.body || "",
    tag: n.tag || "minivoip",
    renotify: !!n.renotify,
    requireInteraction: ring,
    vibrate: ring ? [500, 250, 500, 250, 500] : [200],
    icon: "/assets/brand/app-icon-192.png",
    badge: "/assets/brand/app-icon-192.png",
    data: { url: n.url || "/app" },
  }));
});

self.addEventListener("notificationclick", function (e) {
  e.notification.close();
  var url = new URL((e.notification.data && e.notification.data.url) || "/app", self.location.origin).href;
  e.waitUntil(self.clients.matchAll({ type: "window", includeUncontrolled: true }).then(function (list) {
    for (var i = 0; i < list.length; i++) {
      if (list[i].url.indexOf("/app") !== -1 && "focus" in list[i]) return list[i].focus();
    }
    return self.clients.openWindow(url);
  }));
});
