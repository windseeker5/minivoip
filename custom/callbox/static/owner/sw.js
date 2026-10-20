/* Owner PWA service worker: turns pushes into ringing notifications and opens the app on a tap.
 * iPhone merges notifications that share a tag without a sound, so there each ring gets its own tag; Android
 * re-alerts on the same tag with renotify, which keeps the tray to one notification per call. */

var IOS = /iPhone|iPad|iPod|Macintosh/.test(self.navigator.userAgent);

self.addEventListener("install", function () { self.skipWaiting(); });
self.addEventListener("activate", function (e) { e.waitUntil(self.clients.claim()); });

function closeCall(id) {
  return self.registration.getNotifications().then(function (list) {
    list.forEach(function (n) { if (!id || n.tag.indexOf("call-" + id) === 0) n.close(); });
  });
}

self.addEventListener("push", function (e) {
  var d = {};
  try { d = e.data.json(); } catch (err) { d = { title: "Appel", body: e.data ? e.data.text() : "" }; }
  var opts = { body: d.body, icon: "icons/icon-192.png", badge: "icons/icon-192.png", data: { url: d.url || "./" },
    lang: "fr-CA" };
  var work;
  if (d.kind === "ring") {
    opts.tag = IOS ? "call-" + d.call + "-" + d.n : "call-" + d.call;
    opts.renotify = !IOS;
    opts.requireInteraction = true;
    opts.vibrate = [700, 300, 700, 300, 700];
    work = self.registration.showNotification(d.title, opts);
  } else if (d.kind === "missed") {
    opts.tag = "missed-" + d.call;
    work = closeCall(d.call).then(function () { return self.registration.showNotification(d.title, opts); });
  } else {
    opts.tag = (d.kind || "note") + "-" + d.call;
    opts.vibrate = [200, 100, 200];
    work = self.registration.showNotification(d.title, opts);
  }
  // Tell an open app right away too, so it can ring in the foreground.
  work = work.then(function () {
    return self.clients.matchAll({ type: "window" }).then(function (cs) {
      cs.forEach(function (c) { c.postMessage({ type: "push", data: d }); });
    });
  });
  e.waitUntil(work);
});

self.addEventListener("notificationclick", function (e) {
  var url = new URL(e.notification.data && e.notification.data.url || "./", self.registration.scope).href;
  e.notification.close();
  e.waitUntil(closeCall(null).then(function () {
    return self.clients.matchAll({ type: "window", includeUncontrolled: true });
  }).then(function (cs) {
    var app = cs.filter(function (c) { return c.url.indexOf(self.registration.scope) === 0; })[0];
    if (app) {
      app.postMessage({ type: "open", url: url });
      return app.focus();
    }
    return self.clients.openWindow(url);
  }));
});

self.addEventListener("pushsubscriptionchange", function (e) {
  e.waitUntil(fetch("../api/me", { credentials: "same-origin" }).then(function (r) { return r.json(); }).then(function (me) {
    var raw = atob(me.vapid.replace(/-/g, "+").replace(/_/g, "/") + "===".slice((me.vapid.length + 3) % 4));
    var key = new Uint8Array(raw.length);
    for (var i = 0; i < raw.length; i++) key[i] = raw.charCodeAt(i);
    return self.registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key });
  }).then(function (sub) {
    return fetch("../api/push/subscribe", { method: "POST", credentials: "same-origin",
      headers: { "Content-Type": "application/json", "X-Call": "1" }, body: JSON.stringify(sub.toJSON()) });
  }));
});
