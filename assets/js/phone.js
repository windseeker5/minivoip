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
      $("[data-dot]").className = "size-2 rounded-full bg-emerald-600";
      $("[data-conn]").textContent = "Online";
      send({ type: "hello" });
    };
    ws.onclose = function () {
      $("[data-dot]").className = "size-2 rounded-full bg-muted-foreground";
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
    $("[data-status]").textContent = "Connecting…";
    navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true } })
      .then(function (stream) {
        mic = stream;
        if (ringingId !== id) { endCall(); return; }
        ringingId = null;
        liveId = id;
        show("connecting");
        pc = new RTCPeerConnection({ iceServers: [] });
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
    if (pc) { pc.close(); pc = null; }
    if (mic) { mic.getTracks().forEach(function (t) { t.stop(); }); mic = null; }
    liveId = null;
    show("ready");
  }

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
    }
  });

  connectWs();
})();
