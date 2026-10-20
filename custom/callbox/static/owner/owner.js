/* Owner PWA: log in once, turn on notifications, answer calls, listen to voicemail.
 * The socket to /call/ws tells this screen about calls while it's open; pushes (sw.js) cover the rest.
 * Answering waits for the guest's WebRTC offer, which comes right after "accept". */
(function () {
  "use strict";

  var API = new URL("../api/", location.href);
  var WS_URL = new URL("../ws?role=owner", location.href);
  WS_URL.protocol = WS_URL.protocol === "https:" ? "wss:" : "ws:";
  var $ = function (id) { return document.getElementById(id); };

  var me = null, reg = null, ws = null, wsDelay = 1000;
  var ringingId = null, liveId = null, pc = null, mic = null, timerIv = 0, ringStop = null, ac = null;
  var params = new URLSearchParams(location.search);

  function api(path, opts) {
    opts = opts || {};
    opts.credentials = "same-origin";
    opts.headers = Object.assign({ "X-Call": "1" }, opts.json ? { "Content-Type": "application/json" } : {}, opts.headers);
    if (opts.json) { opts.body = JSON.stringify(opts.json); delete opts.json; }
    return fetch(new URL(path, API), opts);
  }

  function show(id) {
    ["login", "home", "call"].forEach(function (v) { $(v).hidden = v !== id; });
  }

  // ------------------------------------------------------------- login

  $("login-form").addEventListener("submit", function (e) {
    e.preventDefault();
    $("login-err").textContent = "";
    api("login", { method: "POST", json: { passcode: $("code").value } }).then(function (r) {
      if (r.status === 429) throw new Error("Trop d'essais, attendez quelques minutes.");
      if (!r.ok) throw new Error("Code invalide.");
      return boot();
    }).catch(function (err) { $("login-err").textContent = err.message; });
  });

  $("logout").addEventListener("click", function () {
    api("logout", { method: "POST" }).then(function () { location.reload(); });
  });

  // ------------------------------------------------------------- notifications

  function standalone() {
    return matchMedia("(display-mode: standalone)").matches || navigator.standalone === true;
  }

  function keyBytes(b64) {
    var raw = atob(b64.replace(/-/g, "+").replace(/_/g, "/") + "===".slice((b64.length + 3) % 4));
    var out = new Uint8Array(raw.length);
    for (var i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
    return out;
  }

  function subscribe() {
    return reg.pushManager.getSubscription().then(function (sub) {
      return sub || reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: keyBytes(me.vapid) });
    }).then(function (sub) {
      return api("push/subscribe", { method: "POST", json: sub.toJSON() });
    });
  }

  function pushState() {
    var ios = /iPhone|iPad|iPod/.test(navigator.userAgent);
    var on = $("push-on"), test = $("push-test"), hint = $("push-hint"), state = $("push-state");
    on.hidden = test.hidden = hint.hidden = true;
    if (!("Notification" in window) || !reg || !("pushManager" in reg)) {
      state.textContent = "Ce navigateur ne peut pas recevoir les notifications.";
      if (ios && !standalone()) {
        hint.hidden = false;
        hint.textContent = "Sur iPhone : touchez Partager, puis « Sur l'écran d'accueil ». Ouvrez ensuite l'app depuis son icône.";
      }
      return;
    }
    if (Notification.permission === "granted") {
      state.textContent = "Ce téléphone sonne quand quelqu'un appelle.";
      test.hidden = false;
      subscribe().catch(function () { state.textContent = "Notifications permises, mais l'abonnement a échoué. Réessayez."; on.hidden = false; });
    } else if (Notification.permission === "denied") {
      state.textContent = "Les notifications sont bloquées. Permettez-les dans les réglages du téléphone pour cette app.";
    } else {
      state.textContent = "Ce téléphone ne sonne pas encore.";
      on.hidden = false;
      if (!standalone()) {
        hint.hidden = false;
        hint.textContent = ios
          ? "Sur iPhone, ajoutez d'abord l'app à l'écran d'accueil (Partager → « Sur l'écran d'accueil ») et ouvrez-la de là."
          : "Conseil : installez l'app (menu ⋮ → « Ajouter à l'écran d'accueil ») pour qu'elle s'ouvre comme une app.";
      }
    }
  }

  $("push-on").addEventListener("click", function () {
    Notification.requestPermission().then(pushState);
  });

  $("push-test").addEventListener("click", function () {
    var b = $("push-test");
    b.disabled = true;
    b.textContent = "Envoyé, verrouillez le téléphone…";
    api("push/test", { method: "POST" }).then(function (r) { return r.json(); }).then(function (j) {
      setTimeout(function () { b.disabled = false; b.textContent = "Envoyer un test"; }, 4000);
      if (!j.devices) $("push-state").textContent = "Aucun appareil abonné. Activez les notifications.";
    });
  });

  // ------------------------------------------------------------- voicemail

  function loadVoicemail() {
    api("voicemails").then(function (r) { return r.ok ? r.json() : []; }).then(function (list) {
      var ul = $("vms");
      ul.innerHTML = "";
      if (!list.length) { ul.innerHTML = '<li class="empty">Aucun message.</li>'; return; }
      list.forEach(function (v) {
        var li = document.createElement("li");
        li.className = "vm" + (v.heard ? "" : " is-new") + (params.get("vm") === v.id ? " is-focus" : "");
        var when = new Date(v.created * 1000).toLocaleString("fr-CA", { dateStyle: "medium", timeStyle: "short" });
        li.innerHTML = '<div class="vm-top"><span></span><button class="del" type="button">Supprimer</button></div>' +
          '<audio controls preload="none"></audio>';
        li.querySelector("span").textContent = when + (v.heard ? "" : " · nouveau");
        li.querySelector("audio").src = new URL("voicemail/" + v.id, API).href;
        li.querySelector(".del").addEventListener("click", function () {
          api("voicemail/" + v.id, { method: "DELETE" }).then(loadVoicemail);
        });
        ul.appendChild(li);
      });
    });
  }

  // ------------------------------------------------------------- socket

  function connectWs() {
    if (ws && ws.readyState < 2) return;
    ws = new WebSocket(WS_URL);
    ws.onopen = function () {
      wsDelay = 1000;
      $("dot").classList.add("is-on");
      $("conn").textContent = "Prêt à recevoir les appels";
      send({ type: "hello" });
    };
    ws.onclose = function () {
      $("dot").classList.remove("is-on");
      $("conn").textContent = "Hors ligne, reconnexion…";
      if (liveId) endCall("Ligne coupée");
      setTimeout(connectWs, wsDelay);
      wsDelay = Math.min(wsDelay * 2, 15000);
    };
    ws.onmessage = function (e) {
      var m = JSON.parse(e.data);
      if (m.type === "incoming") incoming(m.id);
      else if (m.type === "missed" || m.type === "taken") {
        if (ringingId === m.id) { stopRinging(); ringingId = null; show("home"); loadVoicemail(); }
      } else if (m.type === "offer") answer(m);
      else if (m.type === "ice" && pc && m.candidate) pc.addIceCandidate(m.candidate).catch(function () {});
      else if (m.type === "hangup" && m.id === liveId) endCall("Appel terminé");
    };
  }

  function send(m) { if (ws && ws.readyState === 1) ws.send(JSON.stringify(m)); }

  // ------------------------------------------------------------- calls

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

  function incoming(id) {
    if (liveId) return;
    ringingId = id;
    show("call");
    $("call").classList.add("ringing");
    $("call-status").textContent = "Appel entrant…";
    $("call-timer").textContent = "";
    $("ring-btns").hidden = false;
    $("talk-btns").hidden = true;
    stopRinging();
    ringStop = ringtone();
    $("accept").focus();
  }

  function clearNotifications() {
    if (reg && reg.getNotifications) reg.getNotifications().then(function (l) { l.forEach(function (n) { n.close(); }); });
  }

  $("accept").addEventListener("click", function () {
    if (!ringingId) return;
    var id = ringingId;
    stopRinging();
    clearNotifications();
    $("call-status").textContent = "Connexion…";
    var ice = api("turn").then(function (r) { return r.json(); }).catch(function () { return {}; });
    navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true } })
      .then(function (stream) {
        mic = stream;
        return ice;
      }).then(function (cfg) {
        if (ringingId !== id) { mic.getTracks().forEach(function (t) { t.stop(); }); mic = null; return; }
        ringingId = null;
        liveId = id;
        pc = new RTCPeerConnection(cfg);
        mic.getTracks().forEach(function (t) { pc.addTrack(t, mic); });
        cands = {};
        pc.ontrack = function (e) {
          $("remote").srcObject = e.streams[0];
          $("remote").play().catch(function (err) { report("play blocked: " + err.name); });
        };
        pc.onicecandidate = function (e) {
          if (!e.candidate) return;
          cands[e.candidate.type] = (cands[e.candidate.type] || 0) + 1;
          send({ type: "ice", id: liveId, candidate: e.candidate.toJSON() });
        };
        pc.oniceconnectionstatechange = function () { if (pc) report("ice " + pc.iceConnectionState); };
        pc.onconnectionstatechange = function () {
          if (!pc) return;
          report("conn " + pc.connectionState);
          if (pc.connectionState === "connected") { talking(); setTimeout(function () { report("5 s later"); }, 5000); }
          else if (pc.connectionState === "failed") { send({ type: "hangup", id: liveId }); endCall("Connexion impossible"); }
        };
        $("call").classList.remove("ringing");
        send({ type: "accept", id: id });
      }).catch(function () {
        $("call-status").textContent = "Micro refusé : permettez le micro pour répondre.";
      });
  });

  // Tells the server how the audio connection is doing (state, route, bytes heard); it logs it for debugging.
  var cands = {};
  function report(what) {
    var p = pc, info = { what: what, gathered: cands, net: (navigator.connection || {}).type };
    if (!p) return send({ type: "diag", id: liveId, info: info });
    p.getStats().then(function (s) {
      s.forEach(function (x) {
        if (x.type === "candidate-pair" && x.nominated && x.state === "succeeded") {
          var l = s.get(x.localCandidateId), r = s.get(x.remoteCandidateId);
          info.route = (l && l.candidateType) + ">" + (r && r.candidateType);
        }
        if (x.type === "inbound-rtp" && x.kind === "audio") info.heardBytes = x.bytesReceived;
      });
      send({ type: "diag", id: liveId, info: info });
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
    $("call-status").textContent = "En ligne";
    $("ring-btns").hidden = true;
    $("talk-btns").hidden = false;
    var start = Date.now();
    clearInterval(timerIv);
    timerIv = setInterval(function () {
      var s = Math.floor((Date.now() - start) / 1000);
      $("call-timer").textContent = (s < 600 ? "0" : "") + Math.floor(s / 60) + ":" + (s % 60 < 10 ? "0" : "") + (s % 60);
    }, 500);
  }

  $("decline").addEventListener("click", function () {
    if (ringingId) send({ type: "decline", id: ringingId });
    stopRinging();
    clearNotifications();
    ringingId = null;
    show("home");
  });

  $("hangup").addEventListener("click", function () {
    send({ type: "hangup", id: liveId });
    endCall("Appel terminé");
  });

  $("mute").addEventListener("click", function () {
    var b = $("mute"), on = b.getAttribute("aria-pressed") !== "true";
    b.setAttribute("aria-pressed", on ? "true" : "false");
    if (mic) mic.getAudioTracks().forEach(function (t) { t.enabled = !on; });
  });

  function endCall(text) {
    clearInterval(timerIv);
    if (pc) { pc.close(); pc = null; }
    if (mic) { mic.getTracks().forEach(function (t) { t.stop(); }); mic = null; }
    $("remote").srcObject = null;
    liveId = null;
    $("call-status").textContent = text;
    $("talk-btns").hidden = true;
    $("mute").setAttribute("aria-pressed", "false");
    setTimeout(function () { if (!liveId && !ringingId) { show("home"); loadVoicemail(); } }, 1800);
  }

  // ------------------------------------------------------------- start

  if (navigator.serviceWorker) {
    navigator.serviceWorker.addEventListener("message", function (e) {
      var m = e.data || {};
      if (m.type === "open") {
        params = new URL(m.url).searchParams;
        if (params.get("vm")) loadVoicemail();
        connectWs();
        send({ type: "hello" });
      } else if (m.type === "push" && m.data && m.data.kind === "voicemail") {
        loadVoicemail();
      }
    });
  }

  document.addEventListener("visibilitychange", function () {
    if (document.visibilityState === "visible" && me && me.owner) { connectWs(); send({ type: "hello" }); }
  });

  function boot() {
    return api("me").then(function (r) { return r.json(); }).then(function (m) {
      me = m;
      if (!me.owner) { show("login"); $("code").focus(); return; }
      show("home");
      var ready = navigator.serviceWorker
        ? navigator.serviceWorker.register("sw.js", { scope: "./" }).then(function () { return navigator.serviceWorker.ready; })
        : Promise.reject();
      ready.then(function (r) { reg = r; }, function () {}).then(pushState);
      loadVoicemail();
      connectWs();
    });
  }

  boot();
})();
