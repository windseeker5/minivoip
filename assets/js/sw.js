// Service worker, served at /sw.js so it covers the whole site. It shows the
// push notifications that wake the owner's phone (push/push.go sends them)
// and opens the phone app when one is tapped.

self.addEventListener("install", function () { self.skipWaiting(); });
self.addEventListener("activate", function (e) { e.waitUntil(self.clients.claim()); });

self.addEventListener("push", function (e) {
  var n = {};
  try { n = e.data ? e.data.json() : {}; } catch (err) { n = { title: "minivoip", body: e.data && e.data.text() }; }
  e.waitUntil(handle(n));
});

// handle shows the notification, then sends the server a receipt saying
// what this phone did with it (push/push.go, store/pushlog.go).
function handle(n) {
  var reg = self.registration;
  var shownAs = n.kind || "other";
  var data = { url: n.url || "/app" };
  return appVisible().then(function (open) {
    var done;
    if (n.kind === "answered" && open) {
      // The app on screen answered: just remove the ringing.
      shownAs = "cleared";
      done = reg.getNotifications({ tag: n.tag }).then(function (l) { l.forEach(function (x) { x.close(); }); });
    } else {
      if (n.kind === "ring" && n.until && Date.now() > n.until) {
        // Arrived after the call stopped ringing.
        shownAs = "late";
        n = { title: "Missed call", body: "A website visitor called. They may leave a message.", tag: n.tag, id: n.id };
      }
      var ring = shownAs === "ring";
      done = reg.showNotification(n.title || "minivoip", {
        body: n.body || "",
        tag: n.tag || "minivoip",
        renotify: !!n.renotify,
        silent: n.kind === "answered",
        requireInteraction: ring,
        vibrate: ring ? [500, 250, 500, 250, 500] : [200],
        icon: "/assets/brand/app-icon-192.png",
        badge: "/assets/brand/app-icon-192.png",
        data: data,
      });
    }
    return done.then(function () { return receipt(n.id, shownAs, open ? "app on screen" : "app not on screen"); });
  });
}

function appVisible() {
  return self.clients.matchAll({ type: "window", includeUncontrolled: true }).then(function (list) {
    return list.some(function (c) { return c.visibilityState === "visible" && c.url.indexOf("/app") !== -1; });
  });
}

function receipt(id, shownAs, note) {
  if (!id) return Promise.resolve();
  return fetch("/app/push/receipt", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ id: id, shownAs: shownAs, note: note }),
  }).catch(function () {});
}

self.addEventListener("notificationclick", function (e) {
  e.notification.close();
  var url = new URL((e.notification.data && e.notification.data.url) || "/app", self.location.origin).href;
  // The app's notifications bring the app forward; a meeting room's opens the room.
  var app = new URL(url).pathname.indexOf("/app") === 0;
  e.waitUntil(self.clients.matchAll({ type: "window", includeUncontrolled: true }).then(function (list) {
    for (var i = 0; i < list.length; i++) {
      var same = app ? list[i].url.indexOf("/app") !== -1 : list[i].url.split("?")[0] === url;
      if (same && "focus" in list[i]) return list[i].focus();
    }
    return self.clients.openWindow(url);
  }));
});
