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

// The phone app's own "Incoming call" alert (phone.js) comes through here
// too, so it gets the same round picture.
self.addEventListener("message", function (e) {
  if (e.data && e.data.show) e.waitUntil(show(e.data.show));
});

// MARK is the notification's picture when the business has no logo.
var MARK = "/assets/brand/app-icon-192.png";
var rounds = {};

// roundIcon cuts the business's logo (or the mark) into a circle with
// see-through corners, on white like everywhere else, so desktops that draw
// the picture square (Omarchy's mako) still show a circle. It gives a data:
// address; if that takes over a second, the plain picture.
function roundIcon(url) {
  url = url || MARK;
  if (!rounds[url]) {
    rounds[url] = fetch(url).then(function (r) {
      if (!r.ok) throw new Error("icon " + r.status);
      return r.blob();
    }).then(createImageBitmap).then(function (img) {
      var size = 192, c = new OffscreenCanvas(size, size), g = c.getContext("2d");
      g.beginPath();
      g.arc(size / 2, size / 2, size / 2, 0, 2 * Math.PI);
      g.clip();
      g.fillStyle = "#fff";
      g.fillRect(0, 0, size, size);
      var k = Math.max(size / img.width, size / img.height), w = img.width * k, h = img.height * k;
      g.drawImage(img, (size - w) / 2, (size - h) / 2, w, h);
      return c.convertToBlob({ type: "image/png" });
    }).then(function (blob) {
      return new Promise(function (ok, fail) {
        var f = new FileReader();
        f.onload = function () { ok(f.result); };
        f.onerror = fail;
        f.readAsDataURL(blob);
      });
    }).catch(function () {
      // A logo that won't load gives the round mark; the mark, itself.
      delete rounds[url];
      return url === MARK ? MARK : roundIcon(MARK);
    });
  }
  return Promise.race([rounds[url], new Promise(function (ok) { setTimeout(function () { ok(url); }, 1000); })]);
}

// show puts up one notification with the round picture.
function show(n) {
  return roundIcon(n.icon).then(function (icon) {
    return self.registration.showNotification(n.title || "minivoip", {
      body: n.body || "",
      tag: n.tag || "minivoip",
      renotify: !!n.renotify,
      silent: !!n.silent,
      requireInteraction: !!n.requireInteraction,
      vibrate: n.vibrate || [200],
      icon: icon,
      badge: MARK,
      data: { url: n.url || "/app" },
    });
  });
}

// handle shows the notification, then sends the server a receipt saying
// what this phone did with it (push/push.go, store/pushlog.go).
function handle(n) {
  var reg = self.registration;
  var shownAs = n.kind || "other";
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
        n = { title: (n.title || "").replace("Incoming call", "Missed call"), body: "Someone called from your website. They may leave a message.",
          tag: n.tag, id: n.id, icon: n.icon, url: n.url };
      }
      var ring = shownAs === "ring";
      done = show({
        title: n.title, body: n.body, tag: n.tag, renotify: n.renotify, icon: n.icon, url: n.url,
        silent: n.kind === "answered",
        requireInteraction: ring,
        vibrate: ring ? [500, 250, 500, 250, 500] : [200],
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
