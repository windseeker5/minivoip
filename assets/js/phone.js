// Owner's phone app (/app). Stays connected to the call socket, rings on
// "incoming" and answers with a direct WebRTC call. Ported from the Python
// proof of concept's owner.js. No push notifications yet: the page must be open.
(function () {
  var root = document.querySelector("[data-phone]");
  if (!root) return;
  var $ = function (sel) { return root.querySelector(sel); };

  var ws, pc, mic, ac, ringStop, timerIv;
  var ringingId = null, liveId = null, wsDelay = 1000;

  function show(p) {
    var call = $(".cs-call");
    if (call) {
      call.classList.toggle("is-ringing", p === "incoming");
      call.classList.toggle("is-talking", p === "talking");
    }
    root.querySelectorAll("[data-show]").forEach(function (el) {
      el.hidden = el.dataset.show.split(" ").indexOf(p) === -1;
    });
  }

  // ----------------------------------------------------------- socket

  function connectWs() {
    if (ws && ws.readyState < 2) return;
    var proto = location.protocol === "https:" ? "wss:" : "ws:";
    ws = new WebSocket(proto + "//" + location.host + "/ws?role=owner&line=" + encodeURIComponent(root.dataset.line));
    ws.onopen = function () {
      wsDelay = 1000;
      $("[data-dot]").className = "is-on";
      $("[data-conn]").textContent = "Online";
      send({ type: "hello" });
    };
    ws.onclose = function () {
      $("[data-dot]").className = "";
      $("[data-conn]").textContent = "Offline, reconnecting…";
      if (liveId) endCall();
      setTimeout(connectWs, wsDelay);
      wsDelay = Math.min(wsDelay * 2, 15000);
    };
    ws.onmessage = function (e) {
      var m = JSON.parse(e.data);
      if (m.type === "incoming") incoming(m);
      else if (m.type === "missed" || m.type === "taken") {
        if (ringingId === m.id) { stopRinging(); ringingId = null; show("ready"); }
      } else if (m.type === "offer") answer(m);
      else if (m.type === "ice" && pc && m.candidate) pc.addIceCandidate(m.candidate).catch(function () {});
      else if (m.type === "hangup" && m.id === liveId) endCall();
      else if (m.type === "voicemail") refreshVoicemail();
    };
  }

  function send(m) { if (ws && ws.readyState === 1) ws.send(JSON.stringify(m)); }

  // A caller left a message: reload the list under the call card.
  function refreshVoicemail() {
    fetch("/app/voicemails").then(function (r) { return r.ok ? r.text() : null; }).then(function (html) {
      if (html) document.querySelector("[data-voicemail-box]").innerHTML = html;
    });
  }

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

  function stopRinging() { if (ringStop) { ringStop(); ringStop = null; } }

  function incoming(m) {
    if (liveId) return;
    ringingId = m.id;
    show("incoming");
    $("[data-status]").textContent = "Website call";
    $("[data-option]").textContent = m.option ? "Pressed: " + m.option : "";
    $("[data-timer]").textContent = "";
    stopRinging();
    ringStop = ringtone();
    $('[data-action="accept"]').focus();
  }

  // ----------------------------------------------------------- answering

  function accept() {
    if (!ringingId) return;
    var id = ringingId;
    stopRinging();
    clearNotifications();
    $("[data-status]").textContent = "Connecting…";
    var ice = fetch("/api/ice").then(function (r) { return r.json(); }).catch(function () { return { iceServers: [] }; });
    navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true } })
      .then(function (stream) {
        mic = stream;
        return ice;
      }).then(function (cfg) {
        if (ringingId !== id) { endCall(); return; }
        // ?relay=1 forces the relay (for testing).
        if (/[?&]relay=1/.test(location.search)) cfg.iceTransportPolicy = "relay";
        ringingId = null;
        liveId = id;
        show("connecting");
        pc = new RTCPeerConnection(cfg);
        mic.getTracks().forEach(function (t) { pc.addTrack(t, mic); });
        pc.ontrack = function (e) {
          var el = $("[data-remote]");
          el.srcObject = e.streams[0];
          el.play().catch(function () {});
        };
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
    if (!pc || m.id !== liveId) return;
    pc.setRemoteDescription({ type: "offer", sdp: m.sdp })
      .then(function () { return pc.createAnswer(); })
      .then(function (a) { return pc.setLocalDescription(a); })
      .then(function () { send({ type: "answer", id: liveId, sdp: pc.localDescription.sdp }); });
  }

  function talking() {
    show("talking");
    $("[data-status]").textContent = "On the line";
    var start = Date.now();
    clearInterval(timerIv);
    timerIv = setInterval(function () {
      var s = Math.floor((Date.now() - start) / 1000);
      $("[data-timer]").textContent = Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
    }, 500);
  }

  function endCall() {
    clearInterval(timerIv);
    $("[data-timer]").textContent = "";
    $('[data-action="mute"]').setAttribute("aria-pressed", "false");
    if (pc) { pc.close(); pc = null; }
    if (mic) { mic.getTracks().forEach(function (t) { t.stop(); }); mic = null; }
    liveId = null;
    show("ready");
  }

  // For tests and support: how the live call is connected and how much
  // voice came in.
  window.minivoipCallStats = function () {
    if (!pc) return Promise.resolve(null);
    return pc.getStats().then(function (st) {
      var out = { route: null, bytesIn: 0 };
      st.forEach(function (x) {
        if (x.type === "candidate-pair" && x.nominated && x.state === "succeeded") {
          var l = st.get(x.localCandidateId), r = st.get(x.remoteCandidateId);
          out.route = (l && l.candidateType) + ">" + (r && r.candidateType);
        }
        if (x.type === "inbound-rtp" && x.kind === "audio") out.bytesIn = x.bytesReceived;
      });
      return out;
    });
  };

  root.addEventListener("click", function (e) {
    var b = e.target.closest("[data-action]");
    if (!b) return;
    if (b.dataset.action === "accept") accept();
    else if (b.dataset.action === "decline") {
      if (ringingId) send({ type: "decline", id: ringingId });
      stopRinging();
      ringingId = null;
      show("ready");
    } else if (b.dataset.action === "hangup") {
      send({ type: "hangup", id: liveId });
      endCall();
    } else if (b.dataset.action === "mute") {
      var on = b.getAttribute("aria-pressed") !== "true";
      b.setAttribute("aria-pressed", String(on));
      if (mic) mic.getTracks().forEach(function (t) { t.enabled = !on; });
    }
  });

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

  if (box && "serviceWorker" in navigator) {
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
    if (b.hasAttribute("data-push-on")) {
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

  // Answering here: clear the ringing notifications.
  function clearNotifications() {
    if (reg && reg.getNotifications) reg.getNotifications().then(function (l) { l.forEach(function (n) { n.close(); }); });
  }

  connectWs();
})();
