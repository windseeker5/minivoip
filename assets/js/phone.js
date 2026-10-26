// Owner's app (/app), on the phone and the computer. Stays connected to the
// call socket, rings on "incoming" and answers with a direct WebRTC call
// (ported from the Python proof of concept's owner.js). Also runs the tabs,
// the Available switch, the call and voicemail lists, and installing. Push
// notifications (sw.js) wake it when closed. Inside the Android app
// (android/), window.MinivoipAndroid is the app, which rings natively.
(function () {
  var root = document.querySelector("[data-phone]");
  if (!root) return;
  var $ = function (sel) { return root.querySelector(sel); };

  var ws, pc, mic, ac, ringStop, timerIv, ringIv, meet, link;
  var ringingId = null, liveId = null, wsDelay = 1000, offlineTimer = null;
  var wide = matchMedia("(min-width: 900px)");
  var android = window.MinivoipAndroid || null;
  // The Android app's ring screen answers by opening /app?answer=<call id>:
  // answer that call as soon as the socket offers it.
  var answerId = new URLSearchParams(location.search).get("answer");
  if (answerId) history.replaceState(null, "", location.pathname + location.hash);

  function show(p) {
    var call = $("[data-call-screen]");
    call.classList.toggle("is-ringing", p === "incoming");
    call.classList.toggle("is-talking", p === "talking");
    root.querySelectorAll("[data-show]").forEach(function (el) {
      el.hidden = el.dataset.show.split(" ").indexOf(p) === -1;
    });
  }

  function online(on) {
    root.dataset.online = String(on);
    $("[data-conn]").textContent = on ? "online" : "offline, reconnecting…";
    // A short drop (a page reload, a network switch) shouldn't flash the banner.
    clearTimeout(offlineTimer);
    if (on) $("[data-offline]").hidden = true;
    else offlineTimer = setTimeout(function () { $("[data-offline]").hidden = false; }, 2500);
  }

  // ----------------------------------------------------------- socket

  function connectWs() {
    if (ws && ws.readyState < 2) return;
    var proto = location.protocol === "https:" ? "wss:" : "ws:";
    ws = new WebSocket(proto + "//" + location.host + "/ws?role=owner&line=" + encodeURIComponent(root.dataset.line));
    ws.onopen = function () {
      wsDelay = 1000;
      online(true);
      send({ type: "hello" });
    };
    ws.onclose = function () {
      online(false);
      if (ringingId) { stopRinging(); ringingId = null; show("ready"); }
      if (liveId) endCall();
      setTimeout(connectWs, wsDelay);
      wsDelay = Math.min(wsDelay * 2, 15000);
    };
    ws.onmessage = function (e) {
      var m = JSON.parse(e.data);
      if (m.type === "incoming") incoming(m);
      else if (m.type === "missed" || m.type === "taken") {
        if (ringingId === m.id) { stopRinging(); ringingId = null; show("ready"); }
        refreshCalls();
      } else if (m.type === "offer") answer(m);
      else if (m.type === "ice" && pc && m.candidate) pc.addIceCandidate(m.candidate).catch(function () {});
      else if (m.type === "hangup" && m.id === liveId) endCall();
      else if (m.type === "voicemail") { refreshVoicemail(); refreshCalls(); }
      else if (m.type === "room") roomWaiting(m.names || []);
    };
  }

  function send(m) { if (ws && ws.readyState === 1) ws.send(JSON.stringify(m)); }

  // ----------------------------------------------------------- lists

  // Reload a list from the server, unless a message in it is playing.
  function refresh(url, box) {
    fetch(url).then(function (r) { return r.ok ? r.text() : null; }).then(function (html) {
      if (!html || box.querySelector(".ap-play.is-playing")) return;
      box.innerHTML = html;
      newDot();
    });
  }
  function refreshCalls() { refresh("/app/calls", $("[data-calls-box]")); }
  function refreshVoicemail() { refresh("/app/voicemails", $("[data-voicemail-box]")); }

  // The red dot on the Voicemail tab: messages not played yet.
  function newDot() {
    $("[data-vm-dot]").hidden = !$("[data-voicemail-box] [data-new]");
  }

  // Play buttons: one message at a time; the audio element sits next to its
  // button. voicemail.js marks a message played when it starts.
  var playing = null;
  function playPause(btn, audio) {
    if (playing && playing.audio !== audio) { playing.audio.pause(); }
    if (audio.paused) {
      audio.play().catch(function () {});
      playing = { btn: btn, audio: audio };
    } else {
      audio.pause();
    }
  }
  root.addEventListener("play", function (e) { mark(e.target, true); }, true);
  root.addEventListener("pause", function (e) { mark(e.target, false); }, true);
  root.addEventListener("ended", function (e) { mark(e.target, false); }, true);
  function mark(audio, on) {
    if (audio.tagName !== "AUDIO") return;
    var btn = audio === vmAudio ? $("[data-vm-play]") : audio.previousElementSibling;
    if (btn && btn.classList.contains("ap-play")) btn.classList.toggle("is-playing", on);
    // Played: the message isn't new any more, in both lists.
    var row = audio.closest("[data-voicemail]");
    if (on && row) {
      root.querySelectorAll('[data-voicemail="' + row.dataset.voicemail + '"] [data-new]').forEach(function (b) { b.remove(); });
      newDot();
    }
  }

  // On a computer, a voicemail opens on the right with a bigger player.
  var vmAudio = null;
  function openVoicemail(row) {
    root.querySelectorAll(".ap-row.is-open").forEach(function (r) { r.classList.remove("is-open"); });
    row.classList.add("is-open");
    if (vmAudio) vmAudio.pause();
    vmAudio = row.querySelector("audio");
    $("[data-detail-idle]").hidden = true;
    $("[data-detail-vm]").hidden = false;
    $("[data-vm-when]").textContent = row.dataset.when;
    $("[data-vm-title]").textContent = row.dataset.title;
    var opt = $("[data-vm-option]");
    opt.textContent = "";
    if (row.dataset.option) {
      opt.append("The caller chose ");
      var em = document.createElement("em");
      em.textContent = "“" + row.dataset.option + "”";
      opt.append(em);
    } else {
      opt.textContent = "Left from your website";
    }
    $("[data-vm-delete]").action = "/voicemail/" + row.dataset.voicemail + "/delete";
    vmTime();
  }
  function vmTime() {
    if (!vmAudio) return;
    var d = vmAudio.duration, t = vmAudio.currentTime || 0;
    $("[data-vm-bar]").style.width = d > 0 && isFinite(d) ? (100 * t / d) + "%" : "0";
    $("[data-vm-time]").textContent = mmss(t);
  }
  root.addEventListener("timeupdate", function (e) { if (e.target === vmAudio) vmTime(); }, true);

  function mmss(s) {
    s = Math.floor(s);
    return Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
  }

  // ----------------------------------------------------------- tabs

  var tabs = ["calls", "voicemail", "menu", "room", "me"];
  function openTab(name) {
    if (tabs.indexOf(name) === -1) name = "calls";
    root.querySelectorAll("[data-tab]").forEach(function (b) { b.setAttribute("aria-selected", String(b.dataset.tab === name)); });
    root.querySelectorAll("[data-panel]").forEach(function (p) { p.hidden = p.dataset.panel !== name; });
    if (history.replaceState) history.replaceState(null, "", name === "calls" ? location.pathname + location.search : "#" + name);
    if (!wide.matches) window.scrollTo(0, 0);
  }
  openTab(location.hash.slice(1));

  // ----------------------------------------------------------- available

  var availBtn = $("[data-avail-btn]"), availMenu = $("[data-avail-menu]");
  function availOpen(on) {
    availMenu.hidden = !on;
    availBtn.setAttribute("aria-expanded", String(on));
    if (on) {
      var mode = $("[data-avail-state]").dataset.mode;
      availMenu.querySelectorAll("[data-mode]").forEach(function (b) { b.setAttribute("aria-current", String(b.dataset.mode === mode)); });
    }
  }
  function setAvailable(mode) {
    availOpen(false);
    var body = new URLSearchParams({ mode: mode });
    fetch("/app/available", { method: "POST", body: body }).then(function (r) { return r.ok ? r.text() : null; }).then(function (html) {
      if (html) $("[data-avail-state]").outerHTML = html;
      pauseTimer();
    });
  }
  // A pause ends by itself on the server; show it here too.
  var pauseT = null;
  function pauseTimer() {
    clearTimeout(pauseT);
    var el = $("[data-avail-state]"), end = Date.parse(el.dataset.until || "");
    if (!end) return;
    pauseT = setTimeout(function () {
      el.dataset.mode = "on";
      el.dataset.until = "";
      el.querySelector("span").textContent = "Available";
    }, Math.max(0, end - Date.now()) + 1000);
  }
  pauseTimer();
  document.addEventListener("click", function (e) {
    if (!availMenu.hidden && !e.target.closest(".ap-avail")) availOpen(false);
  });
  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape" && !availMenu.hidden) { availOpen(false); availBtn.focus(); }
  });

  // ----------------------------------------------------------- ringing

  function ringtone() {
    try {
      ac = ac || new (window.AudioContext || window.webkitAudioContext)();
      ac.resume();
    } catch (e) { return function () {}; }
    var stopped = false;
    function burst() {
      if (stopped) return;
      var t = ac.currentTime, g = ac.createGain();
      g.gain.value = 0.15;
      g.connect(ac.destination);
      [0, 0.5].forEach(function (off) {
        [880, 1100].forEach(function (f, i) {
          var o = ac.createOscillator();
          o.type = "triangle";
          o.frequency.value = f;
          o.connect(g);
          o.start(t + off + i * 0.12);
          o.stop(t + off + i * 0.12 + 0.12);
        });
      });
      if (navigator.vibrate) navigator.vibrate([400, 200, 400]);
    }
    burst();
    var iv = setInterval(burst, 2500);
    return function () { stopped = true; clearInterval(iv); if (navigator.vibrate) navigator.vibrate(0); };
  }

  // Chrome only lets a page make sound after a tap on it, so the ringtone is
  // unlocked on every tap; otherwise a call on an untouched app rings silently.
  function unlockAudio() {
    try {
      ac = ac || new (window.AudioContext || window.webkitAudioContext)();
      if (ac.state !== "running") ac.resume();
    } catch (e) {}
  }
  ["pointerdown", "keydown"].forEach(function (ev) { document.addEventListener(ev, unlockAudio, true); });

  function stopRinging() {
    clearInterval(ringIv);
    stopFlash();
    if (ringStop) { ringStop(); ringStop = null; }
  }

  // The call screen says what the caller chose: "Someone chose “Book a room”".
  function callText(title, option, sub) {
    var h = $("[data-call-title]");
    h.textContent = title;
    if (option) {
      h.append(" ");
      var em = document.createElement("em");
      em.textContent = "“" + option + "”";
      h.append(em);
    }
    $("[data-call-sub]").textContent = sub;
  }

  var callOption = "";
  function incoming(m) {
    if (liveId) return;
    ringingId = m.id;
    callOption = m.option || "";
    show("incoming");
    $("[data-status]").textContent = "Incoming call";
    if (m.option) callText("Someone chose", m.option, "A visitor on your website heard your greeting and picked this choice.");
    else callText("Someone is calling", "", "A visitor on your website is calling you.");
    var start = Date.now(), ring = +root.dataset.ring || 30;
    function tick() {
      var s = Math.floor((Date.now() - start) / 1000), left = Math.max(0, ring - s);
      $("[data-timer]").textContent = mmss(s);
      $("[data-countdown]").textContent = left > 0 ? "voicemail in " + mmss(left) : "going to voicemail…";
    }
    stopRinging();
    tick();
    ringIv = setInterval(tick, 500);
    if (answerId === m.id) { answerId = null; return accept(); }
    ringStop = ringtone();
    $('[data-action="accept"]').focus();
    if (!document.hasFocus()) callMeOver(m);
  }

  // The app is open but behind other windows (a computer at work): show an
  // alert that brings it to the front, and flash the tab title. The alert
  // has the push's tag, so it replaces the push instead of adding a second.
  var title = document.title, flashIv = null;
  function callMeOver(m) {
    try { window.focus(); } catch (e) {}
    var on = false;
    clearInterval(flashIv);
    flashIv = setInterval(function () {
      on = !on;
      document.title = on ? "📞 Incoming call" : title;
    }, 900);
    if (!reg || !window.Notification || Notification.permission !== "granted") return;
    reg.showNotification("Incoming call", {
      body: (m.option ? "Someone chose “" + m.option + "”. " : "A website visitor is calling. ") + "Click to answer.",
      tag: "call-" + m.id,
      renotify: true,
      requireInteraction: true,
      icon: "/assets/brand/app-icon-192.png",
      data: { url: "/app" },
    }).catch(function () {});
  }
  function stopFlash() {
    clearInterval(flashIv);
    flashIv = null;
    document.title = title;
  }

  // ----------------------------------------------------------- answering

  function accept() {
    if (!ringingId) return;
    var id = ringingId;
    stopRinging();
    clearNotifications();
    $("[data-status]").textContent = "Allow the microphone…";
    var ice = fetch("/api/ice").then(function (r) { return r.json(); }).catch(function () { return { iceServers: [] }; });
    navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true } })
      .then(function (stream) {
        mic = stream;
        $("[data-status]").textContent = "Connecting…";
        return ice;
      }).then(function (cfg) {
        if (ringingId !== id) { endCall(); return; }
        // ?relay=1 forces the relay (for testing).
        if (/[?&]relay=1/.test(location.search)) cfg.iceTransportPolicy = "relay";
        ringingId = null;
        liveId = id;
        show("connecting");
        pc = new RTCPeerConnection(cfg);
        // The voice, plus cameras, screens and the whiteboard (meet.js).
        meet = MinivoipMeet.Session({ stage: $("[data-stage]"), me: { name: root.dataset.meName, initials: root.dataset.meInitials }, mic: mic, onChange: meetChanged });
        link = meet.link(id, pc, { name: "Caller", onAudio: function (stream) {
          var el = $("[data-remote]");
          el.srcObject = stream;
          el.play().catch(function () {});
        } });
        pc.onicecandidate = function (e) {
          if (e.candidate) send({ type: "ice", id: liveId, candidate: e.candidate.toJSON() });
        };
        pc.onconnectionstatechange = function () {
          if (!pc) return;
          if (pc.connectionState === "connected") talking();
          else if (pc.connectionState === "failed") { send({ type: "hangup", id: liveId }); endCall(); }
        };
        send({ type: "accept", id: id });
      }).catch(function () {
        $("[data-status]").textContent = "Microphone blocked: allow it to answer.";
      });
  }

  function answer(m) {
    if (!pc || !link || m.id !== liveId) return;
    link.answer(m.sdp).then(function (sdp) { send({ type: "answer", id: liveId, sdp: sdp }); });
  }

  // meetChanged follows the stage: the call screen makes room for it while a
  // camera, a screen or the whiteboard is on, and the buttons say what's on.
  function meetChanged(m) {
    $("[data-call-screen]").dataset.stageOn = m.active() ? "on" : "";
    pressed("camera", !!m.cam);
    pressed("screen", !!m.scr);
    pressed("board", m.boardOpen);
  }
  function pressed(action, on) {
    var b = $('[data-action="' + action + '"]');
    if (b) b.setAttribute("aria-pressed", String(on));
  }
  function meetNote(t) {
    var n = $("[data-meet-note]");
    n.textContent = t || "";
    n.hidden = !t;
  }

  function talking() {
    show("talking");
    $("[data-status]").textContent = "On the line";
    callText("Talking with a website caller", "", callOption ? "They chose “" + callOption + "”." : "");
    var start = Date.now();
    clearInterval(timerIv);
    $("[data-timer]").textContent = "0:00";
    timerIv = setInterval(function () {
      $("[data-timer]").textContent = mmss((Date.now() - start) / 1000);
    }, 500);
  }

  function endCall() {
    clearInterval(timerIv);
    $("[data-timer]").textContent = "";
    $('[data-action="mute"]').setAttribute("aria-pressed", "false");
    if (meet) { meet.end(); meet = null; link = null; }
    meetNote("");
    if (pc) { pc.close(); pc = null; }
    if (mic) { mic.getTracks().forEach(function (t) { t.stop(); }); mic = null; }
    liveId = null;
    show("ready");
    refreshCalls();
  }

  // For tests and support: how the live call is connected and how much
  // voice came in.
  window.minivoipCallStats = function () {
    if (!pc) return Promise.resolve(null);
    return pc.getStats().then(function (st) {
      var out = { route: null, bytesIn: 0, videoIn: 0 };
      st.forEach(function (x) {
        if (x.type === "candidate-pair" && x.nominated && x.state === "succeeded") {
          var l = st.get(x.localCandidateId), r = st.get(x.remoteCandidateId);
          out.route = (l && l.candidateType) + ">" + (r && r.candidateType);
        }
        if (x.type === "inbound-rtp" && x.kind === "audio") out.bytesIn = x.bytesReceived;
        if (x.type === "inbound-rtp" && x.kind === "video") out.videoIn += x.bytesReceived;
      });
      return out;
    });
  };

  root.addEventListener("click", function (e) {
    var t = e.target;
    var tabBtn = t.closest("[data-tab]");
    if (tabBtn) return openTab(tabBtn.dataset.tab);
    if (t.closest("[data-avail-btn]")) return availOpen(availMenu.hidden);
    var mode = t.closest("[data-avail-menu] [data-mode]");
    if (mode) return setAvailable(mode.dataset.mode);
    var play = t.closest("[data-play]");
    if (play) return playPause(play, play.nextElementSibling);
    if (t.closest("[data-vm-play]")) return vmAudio && playPause($("[data-vm-play]"), vmAudio);
    var open = t.closest("[data-open]");
    if (open) {
      var row = open.closest("[data-voicemail]");
      if (wide.matches) return openVoicemail(row);
      var a = row.querySelector("audio");
      return playPause(a.previousElementSibling, a);
    }
    if (t.closest("[data-copy]")) return copyLink(t.closest("[data-copy]"));
    if (t.closest("[data-copy-room]")) return copyRoom(t.closest("[data-copy-room]"));
    if (t.closest("[data-install-btn]")) return install();
    var b = t.closest("[data-action]");
    if (!b) return;
    if (b.dataset.action === "accept") accept();
    else if (b.dataset.action === "decline") {
      if (ringingId) send({ type: "decline", id: ringingId });
      stopRinging();
      clearNotifications();
      ringingId = null;
      show("ready");
    } else if (b.dataset.action === "hangup") {
      send({ type: "hangup", id: liveId });
      endCall();
    } else if (b.dataset.action === "mute") {
      var on = b.getAttribute("aria-pressed") !== "true";
      b.setAttribute("aria-pressed", String(on));
      if (meet) meet.mute(on);
      else if (mic) mic.getTracks().forEach(function (t) { t.enabled = !on; });
    } else if (b.dataset.action === "camera" && meet) {
      meetNote("");
      meet.camera().catch(function (e) { meetNote(MinivoipMeet.cameraError(e, "The camera is blocked. Allow it for this app, then try again.")); });
    } else if (b.dataset.action === "screen" && meet) {
      meetNote("");
      meet.screen().catch(function (e) { if (e && e.name !== "NotAllowedError") meetNote("Your screen couldn't be shown. Try again."); });
    } else if (b.dataset.action === "board" && meet) {
      meet.board();
    }
  });
  // Showing a screen needs a computer (and a Premium line: Basic has no button).
  var shareBtn = $("[data-share-btn]");
  if (shareBtn && window.MinivoipMeet && MinivoipMeet.canShare) shareBtn.hidden = false;

  // ----------------------------------------------------------- push
  // Push notifications wake the phone when this page is closed or asleep.
  var box = document.querySelector("[data-push]");
  var reg = null;

  function keyBytes(b64) {
    var s = atob(b64.replace(/-/g, "+").replace(/_/g, "/") + "===".slice((b64.length + 3) % 4));
    var out = new Uint8Array(s.length);
    for (var i = 0; i < s.length; i++) out[i] = s.charCodeAt(i);
    return out;
  }
  function pushUI(state, text) {
    box.querySelector("[data-push-state]").textContent = text;
    box.querySelector("[data-push-on]").hidden = state !== "off";
    box.querySelector("[data-push-test]").hidden = state !== "on";
    box.querySelector("[data-push-off]").hidden = state !== "on";
  }
  function post(url, body) {
    return fetch(url, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body || {}) });
  }
  function subscribe() {
    return reg.pushManager.getSubscription().then(function (sub) {
      return sub || reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: keyBytes(root.dataset.vapid) });
    }).then(function (sub) {
      return post("/app/push/subscribe", sub.toJSON()).then(function (r) { if (!r.ok) throw new Error("save"); });
    });
  }
  function pushState() {
    var ios = /iPhone|iPad|iPod/.test(navigator.userAgent);
    var standalone = matchMedia("(display-mode: standalone)").matches || navigator.standalone;
    if (!("serviceWorker" in navigator) || !("PushManager" in window) || !window.Notification) {
      box.querySelector("[data-push-ios]").hidden = !(ios && !standalone);
      return pushUI("none", ios && !standalone ? "Notifications need the app on your home screen." : "This browser can't get notifications. Keep this page open to receive calls.");
    }
    if (Notification.permission === "denied") return pushUI("none", "Notifications are blocked for this site. Allow them in your browser settings.");
    if (Notification.permission === "granted") {
      // Keep the server's copy fresh: subscriptions can change.
      return subscribe().then(function () { pushUI("on", "On. This phone rings even when the app is closed."); },
        function () { pushUI("off", "Notifications are allowed, but turning them on failed. Try again."); });
    }
    pushUI("off", "Off. Calls only ring while this page is open.");
  }

  // The Android app rings through Firebase, not web push. MinivoipAndroid
  // .start() asks the phone for its permissions and Firebase token, then
  // calls minivoipAndroid(token, ready); ready means calls can fill the
  // lock screen.
  var androidToken = "";
  window.minivoipAndroid = function (token, ready) {
    androidToken = token;
    if (!token) return pushUI("none", "Ringing isn't set up on this server yet.");
    post("/app/push/subscribe", { endpoint: "fcm:" + token }).then(function (r) {
      if (!r.ok) throw new Error("save");
      if (ready) pushUI("on", "On. This phone rings like a phone call, even when it's locked.");
      else pushUI("off", "Almost: allow minivoip to send notifications and show calls on the lock screen.");
    }).catch(function () { pushUI("off", "Turning on failed. Try again."); });
  };

  if (box && android) {
    pushUI("busy", "Checking this phone…");
    android.start();
  } else if (box && "serviceWorker" in navigator) {
    navigator.serviceWorker.register("/sw.js").then(function (r) {
      reg = r;
      return navigator.serviceWorker.ready;
    }).then(pushState, function () { pushUI("none", "Couldn't set up notifications on this browser."); });
  } else if (box) {
    pushState();
  }

  if (box) box.addEventListener("click", function (e) {
    var b = e.target.closest("button");
    if (!b) return;
    if (b.hasAttribute("data-push-on") && android) {
      android.settings();
    } else if (b.hasAttribute("data-push-off") && android) {
      post("/app/push/unsubscribe", { endpoint: "fcm:" + androidToken }).then(function () { pushUI("off", "Off on this phone."); });
    } else if (b.hasAttribute("data-push-on")) {
      Notification.requestPermission().then(function (p) {
        if (p !== "granted") return pushState();
        pushUI("busy", "Turning on…");
        subscribe().then(pushState, function () { pushUI("off", "Turning on failed. Try again."); });
      });
    } else if (b.hasAttribute("data-push-test")) {
      b.disabled = true;
      post("/app/push/test").then(function (r) { return r.json(); }).then(function (j) {
        pushUI("on", j.sent > 0 ? "Test sent to " + j.sent + " device(s). It should appear in a few seconds." : "The test couldn't be delivered. Turn notifications off and on again.");
      }).finally(function () { b.disabled = false; });
    } else if (b.hasAttribute("data-push-off")) {
      reg.pushManager.getSubscription().then(function (sub) {
        if (!sub) return;
        return post("/app/push/unsubscribe", { endpoint: sub.endpoint }).then(function () { return sub.unsubscribe(); });
      }).then(function () { pushUI("off", "Off on this phone."); });
    }
  });

  // ----------------------------------------------------------- meeting room

  // roomWaiting shows who waits in the meeting room (calls/room.go), with a
  // Join link; pages/app.templ waitingText says the same.
  function roomWaiting(names) {
    var alert = $("[data-room-alert]"), t = "";
    if (!alert) return; // Basic: no meeting room
    if (names.length === 1) t = names[0] + " is waiting in your meeting room";
    else if (names.length === 2) t = names[0] + " and " + names[1] + " are waiting in your meeting room";
    else if (names.length > 2) t = names[0] + " and " + (names.length - 1) + " others are waiting in your meeting room";
    $("[data-room-who]").textContent = t;
    alert.hidden = !names.length;
  }

  function copyRoom(btn) {
    var done = function () {
      btn.textContent = "Copied";
      setTimeout(function () { btn.textContent = "Copy"; }, 1600);
    };
    if (navigator.clipboard) navigator.clipboard.writeText($("[data-room-url]").textContent.trim()).then(done, function () {});
  }

  // ----------------------------------------------------------- call link

  function copyLink(btn) {
    var text = "https://" + $("[data-call-url]").textContent.trim();
    var done = function () {
      btn.textContent = "Copied";
      setTimeout(function () { btn.textContent = "Copy"; }, 1600);
    };
    if (navigator.clipboard) navigator.clipboard.writeText(text).then(done, function () {});
  }

  // ----------------------------------------------------------- install
  // Chrome (Android and computers) offers its own install prompt; iPhone
  // needs Share → Add to Home Screen.

  var installEvt = null;
  function installState() {
    var text = $("[data-install-text]"), btn = $("[data-install-btn]");
    var standalone = matchMedia("(display-mode: standalone)").matches || navigator.standalone;
    var ios = /iPhone|iPad|iPod/.test(navigator.userAgent);
    // On Android, the app (android/) rings like a phone call; the web app can't.
    var androidWeb = !android && /Android/.test(navigator.userAgent);
    $("[data-android-get]").hidden = !androidWeb;
    btn.hidden = !installEvt || androidWeb;
    if (android) text.textContent = "Installed. You're using the minivoip Android app.";
    else if (androidWeb) text.textContent = "Get the minivoip Android app: it rings like a phone call, even when your phone is locked.";
    else if (standalone) text.textContent = "Installed. You're using the minivoip app.";
    else if (installEvt) text.textContent = "Install the app to open it from your home screen or dock, in its own window.";
    else if (ios) text.textContent = "In Safari, tap Share, then “Add to Home Screen”.";
    else text.textContent = "In Chrome, open the browser menu and pick “Install minivoip” (or “Add to Home screen”).";
  }
  window.addEventListener("beforeinstallprompt", function (e) {
    e.preventDefault();
    installEvt = e;
    installState();
  });
  window.addEventListener("appinstalled", function () { installEvt = null; installState(); });
  function install() {
    if (!installEvt) return;
    installEvt.prompt();
    installEvt.userChoice.finally(function () { installEvt = null; installState(); });
  }
  installState();

  // Answering here: clear the ringing notifications.
  function clearNotifications() {
    if (android) android.clearRing();
    if (reg && reg.getNotifications) reg.getNotifications().then(function (l) { l.forEach(function (n) { n.close(); }); });
  }

  newDot();
  connectWs();
})();
