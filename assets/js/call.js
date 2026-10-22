// Visitor's call window (/call/<line>). Plays the menu in the browser, then
// for a "ring" choice opens the call socket and a direct WebRTC call to the
// owner's phone app. Ported from the Python proof of concept's widget.js.
(function () {
  var root = document.querySelector("[data-call]");
  if (!root) return;
  var line = JSON.parse(document.getElementById("line").textContent);
  var $ = function (sel) { return root.querySelector(sel); };

  var phase = "idle"; // idle, menu, ringing, connecting, talking, ended
  var seq = 0;        // bumps on every new call or hang-up, so old callbacks stop
  var ws, pc, mic, callId, stopRing, timerIv, ac;

  function show(p) {
    phase = p;
    root.querySelectorAll("[data-show]").forEach(function (el) {
      el.hidden = el.dataset.show.split(" ").indexOf(p) === -1;
    });
  }
  function status(t) { $("[data-status]").textContent = t; }

  // ----------------------------------------------------------- sound

  function audio() {
    ac = ac || new (window.AudioContext || window.webkitAudioContext)();
    if (ac.state === "suspended") ac.resume();
    return ac;
  }

  // say speaks with the browser's voice. Resolves when done, or after a
  // time based on the text's length if the browser never says it finished.
  function say(text) {
    return new Promise(function (done) {
      var finished = false;
      function finish() { if (!finished) { finished = true; done(); } }
      setTimeout(finish, 1500 + text.length * 75);
      if (!window.speechSynthesis) return;
      speechSynthesis.cancel();
      var u = new SpeechSynthesisUtterance(text);
      u.lang = "en-US";
      u.onend = finish;
      u.onerror = finish;
      speechSynthesis.speak(u);
    });
  }
  function hush() { if (window.speechSynthesis) speechSynthesis.cancel(); }

  function tone(freqs, ms, level) {
    var a = audio(), t = a.currentTime, g = a.createGain();
    g.gain.value = level;
    g.connect(a.destination);
    freqs.forEach(function (f) {
      var o = a.createOscillator();
      o.frequency.value = f;
      o.connect(g);
      o.start(t);
      o.stop(t + ms / 1000);
    });
  }

  // North American ringback: 440 + 480 Hz, 2 s on, 4 s off.
  function ringback() {
    var stopped = false;
    function burst() { if (!stopped) tone([440, 480], 2000, 0.08); }
    burst();
    var iv = setInterval(burst, 6000);
    return function () { stopped = true; clearInterval(iv); };
  }

  function startTimer() {
    var start = Date.now();
    clearInterval(timerIv);
    timerIv = setInterval(function () {
      var s = Math.floor((Date.now() - start) / 1000);
      $("[data-timer]").textContent = Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
    }, 500);
  }

  // ----------------------------------------------------------- the call

  function start() {
    var s = ++seq;
    audio();
    show("menu");
    status("Connected");
    $("[data-timer]").textContent = "";
    say(line.greeting).then(function () {
      if (s === seq && phase === "menu" && line.menu) return say(line.menu);
    });
  }

  function press(key) {
    if (phase !== "menu") return;
    var o = line.options.filter(function (x) { return x.key === key; })[0];
    if (!o) return;
    var s = ++seq;
    tone([697, 1209], 150, 0.1);
    hush();
    if (o.action === "link") {
      status("Opening the page…");
      say(o.say).then(function () { if (s === seq) { end("Page opened"); window.open(o.url, "_blank"); } });
    } else if (o.action === "play") {
      status(o.label);
      say(o.say).then(function () { if (s === seq) end("Call ended"); });
    } else if (o.action === "ring") {
      ring(s, o);
    } else if (o.action === "voicemail") {
      status("Voicemail");
      say(o.say || "Voicemail is coming soon. Please call again later.").then(function () {
        if (s === seq) end("Voicemail is coming soon");
      });
    }
  }

  function ring(s, o) {
    status("Please hold…");
    var spoken = say(o.say || "Please hold.");
    // Asked from the key press itself: some browsers only show the microphone prompt during a tap.
    var gum = navigator.mediaDevices && navigator.mediaDevices.getUserMedia
      ? navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true } })
      : Promise.reject(new Error("no microphone"));
    gum.then(function (stream) {
      mic = stream;
      audio(); // iPhone pauses the audio context when the microphone starts
      return spoken;
    }).then(function () {
      if (s === seq) dial(s, o);
    }, function () {
      if (s !== seq) return;
      status("Microphone blocked. Allow it to talk to " + line.owner + ".");
      show("menu");
    });
  }

  function dial(s, o) {
    show("ringing");
    status("Ringing " + line.owner + "…");
    stopRing = ringback();
    var proto = location.protocol === "https:" ? "wss:" : "ws:";
    ws = new WebSocket(proto + "//" + location.host + "/ws?role=visitor&line=" + encodeURIComponent(line.line));
    ws.onopen = function () { send({ type: "call", option: o.label }); };
    ws.onmessage = function (e) {
      var m = JSON.parse(e.data);
      if (s !== seq) return;
      if (m.type === "ringing") callId = m.id;
      else if (m.type === "accepted") connect(s);
      else if (m.type === "answer" && pc) pc.setRemoteDescription({ type: "answer", sdp: m.sdp });
      else if (m.type === "ice" && pc && m.candidate) pc.addIceCandidate(m.candidate).catch(function () {});
      else if (m.type === "no-answer") end(line.owner + " can't answer right now. Voicemail is coming soon.");
      else if (m.type === "busy") end(line.owner + " is on another call. Try again soon.");
      else if (m.type === "hangup") end("Call ended");
    };
    ws.onclose = function () {
      if (s === seq && (phase === "ringing" || phase === "connecting" || phase === "talking")) end("Line dropped");
    };
  }

  function send(m) {
    if (callId && !m.id) m.id = callId;
    if (ws && ws.readyState === 1) ws.send(JSON.stringify(m));
  }

  function connect(s) {
    show("connecting");
    if (stopRing) stopRing();
    status("Connecting…");
    pc = new RTCPeerConnection({ iceServers: [] });
    mic.getTracks().forEach(function (t) { pc.addTrack(t, mic); });
    pc.ontrack = function (e) {
      var el = $("[data-remote]");
      el.srcObject = e.streams[0];
      el.play().catch(function () {});
    };
    pc.onicecandidate = function (e) {
      if (e.candidate) send({ type: "ice", candidate: e.candidate.toJSON() });
    };
    pc.onconnectionstatechange = function () {
      if (s !== seq || !pc) return;
      if (pc.connectionState === "connected" && phase !== "talking") {
        show("talking");
        status("On the line with " + line.owner);
        startTimer();
      } else if (pc.connectionState === "failed") {
        send({ type: "hangup" });
        end("Couldn't connect the call");
      }
    };
    pc.createOffer().then(function (offer) { return pc.setLocalDescription(offer); }).then(function () {
      send({ type: "offer", sdp: pc.localDescription.sdp });
    });
  }

  function end(why) {
    seq++;
    hush();
    if (stopRing) { stopRing(); stopRing = null; }
    clearInterval(timerIv);
    if (ws) { ws.onclose = null; ws.close(); ws = null; }
    if (pc) { pc.close(); pc = null; }
    if (mic) { mic.getTracks().forEach(function (t) { t.stop(); }); mic = null; }
    callId = null;
    status(why);
    show("ended");
  }

  root.addEventListener("click", function (e) {
    var b = e.target.closest("button");
    if (!b) return;
    if (b.dataset.key) press(b.dataset.key);
    else if (b.dataset.action === "call") start();
    else if (b.dataset.action === "hangup") { send({ type: "hangup" }); end("Call ended"); }
  });
  document.addEventListener("keydown", function (e) {
    if (/^[0-9*#]$/.test(e.key)) press(e.key);
  });
})();
