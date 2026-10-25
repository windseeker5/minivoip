// Visitor's call window (/call/<line>), look A (DESIGN.md "Call window").
// Plays the menu in the browser and writes its words out as they're said,
// then for a "ring" choice opens the call socket and a direct WebRTC call to
// the owner's phone app. Ported from the Python proof of concept's widget.js.
(function () {
  var root = document.querySelector("[data-call]");
  if (!root) return;
  var line = JSON.parse(document.getElementById("line").textContent);
  var $ = function (sel) { return root.querySelector(sel); };
  var $$ = function (sel) { return Array.prototype.slice.call(root.querySelectorAll(sel)); };

  var phase = "idle"; // idle, menu, ringing, connecting, talking, voicemail, recording, sending, ended
  var seq = 0;        // bumps on every new call or hang-up, so old callbacks stop
  var ws, pc, mic, callId, choice, stopRing, timerIv, countIv, ac, recorder, vmToken, stopMeter, unsent;
  var MAX_MESSAGE_MS = 120000;
  var waveBars = $$("[data-wave] i"), liveBars = $$("[data-live] i"), recBars = $$("[data-rec-wave] i");

  // "Back to the website" returns to the page that sent the visitor here.
  if (document.referrer) {
    $$("[data-back]").forEach(function (a) { a.href = document.referrer; a.hidden = false; });
  }

  // show switches to the view for call state p. Keyboard and screen reader
  // users land on the new view's title.
  function show(p) {
    phase = p;
    root.dataset.state = p;
    var view;
    $$("[data-show]").forEach(function (el) {
      el.hidden = el.dataset.show.split(" ").indexOf(p) === -1;
      if (!el.hidden) view = el;
    });
    var f = view && view.querySelector("[data-focus]");
    if (f && !view.contains(document.activeElement)) f.focus({ preventScroll: true });
  }
  function status(t) { $("[data-status]").textContent = t; }
  function menuStatus(t) { $("[data-menu-status]").textContent = t; }
  function clock(s) { return Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0"); }

  // ----------------------------------------------------------- the words

  // writeOut puts text in el, one span per word, and returns a function that
  // lights the words up to p (0 to 1) like the landing page: said, now, next.
  function writeOut(el, text, onWord) {
    el.textContent = "";
    var total = 0;
    var spans = text.split(/\s+/).filter(Boolean).map(function (w) {
      var s = document.createElement("span");
      s.textContent = w + " ";
      el.appendChild(s);
      total += w.length;
      return s;
    });
    var n = 0;
    var ends = spans.map(function (s) { n += s.textContent.length - 1; return n / (total || 1); });
    return function (p) {
      var now = p >= 1 ? -1 : ends.findIndex(function (e) { return p < e; });
      spans.forEach(function (s, i) { s.className = now === -1 || i < now ? "said" : i === now ? "now" : ""; });
      if (onWord) onWord(now === -1 ? spans.length : now, spans);
    };
  }

  // While the menu says "press 1 to …", choice 1 lights up.
  function lightChoice(now, spans) {
    var key = "";
    for (var i = Math.min(now, spans.length - 1); i > 0 && now < spans.length; i--) {
      var w = spans[i].textContent.replace(/[^0-9*#]/g, "");
      if (w && /^press$/i.test(spans[i - 1].textContent.trim())) { key = w; break; }
    }
    $$(".cw-choice").forEach(function (b) { b.classList.toggle("is-now", b.dataset.key === key); });
  }

  function lightWave(p) {
    waveBars.forEach(function (b, i) { b.classList.toggle("on", i < p * waveBars.length); });
  }

  // meter moves bars with a stream's loudness. Returns a stop function.
  function meter(stream, bars) {
    try {
      var src = audio().createMediaStreamSource(stream), an = ac.createAnalyser(), raf;
      an.fftSize = 512;
      src.connect(an);
      var data = new Uint8Array(an.fftSize);
      (function tick() {
        an.getByteTimeDomainData(data);
        var sum = 0;
        for (var i = 0; i < data.length; i++) { var v = (data[i] - 128) / 128; sum += v * v; }
        var level = Math.min(1, Math.sqrt(sum / data.length) * 5), t = Date.now() / 170;
        bars.forEach(function (b, i) {
          b.style.transform = "scaleY(" + (0.3 + 0.7 * level * (0.55 + 0.45 * Math.sin(t + i * 0.9))).toFixed(3) + ")";
        });
        raf = requestAnimationFrame(tick);
      })();
      return function () {
        cancelAnimationFrame(raf);
        try { src.disconnect(); } catch (e) {}
        bars.forEach(function (b) { b.style.transform = ""; });
      };
    } catch (e) {
      return function () {};
    }
  }

  // ----------------------------------------------------------- sound

  function audio() {
    ac = ac || new (window.AudioContext || window.webkitAudioContext)();
    if (ac.state === "suspended") ac.resume();
    return ac;
  }

  // say plays the owner's recording when there is one, else speaks the text
  // with the browser's voice. onp gets the progress (0 to 1) as it goes.
  // Resolves when done, or after a time limit if the browser never says it
  // finished.
  var playing = null, voiceId = 0;
  function say(text, recording, onp) {
    var id = ++voiceId;
    function tick(q) { if (id === voiceId && onp) onp(Math.min(1, q)); }
    if (recording) {
      return new Promise(function (done) {
        var finished = false, iv;
        function finish() { if (!finished) { finished = true; clearInterval(iv); if (playing === a) playing = null; tick(1); done(); } }
        var a = new Audio(recording);
        playing = a;
        iv = setInterval(function () { if (a.duration) tick(a.currentTime / a.duration); }, 100);
        a.onended = finish;
        a.onerror = function () {
          if (finished) return;
          finished = true;
          clearInterval(iv);
          playing = null;
          say(text, "", onp).then(done);
        };
        a.onloadedmetadata = function () {
          if (isFinite(a.duration)) setTimeout(finish, a.duration * 1000 + 1500);
        };
        setTimeout(finish, 65000);
        a.play().catch(a.onerror);
      });
    }
    return new Promise(function (done) {
      var finished = false, began = Date.now(), est = 400 + text.length * 62, at = -1, iv;
      function finish() { if (!finished) { finished = true; clearInterval(iv); tick(1); done(); } }
      setTimeout(finish, 1500 + text.length * 75);
      iv = setInterval(function () { tick(at >= 0 ? at : Math.min(0.97, (Date.now() - began) / est)); }, 100);
      if (!window.speechSynthesis) return;
      speechSynthesis.cancel();
      var u = new SpeechSynthesisUtterance(text);
      u.lang = "en-US";
      u.onstart = function () { began = Date.now(); };
      u.onboundary = function (e) { if (typeof e.charIndex === "number") at = e.charIndex / text.length; };
      u.onend = finish;
      u.onerror = finish;
      speechSynthesis.speak(u);
    });
  }
  function hush() {
    voiceId++;
    if (window.speechSynthesis) speechSynthesis.cancel();
    if (playing) { playing.onended = null; playing.onerror = null; playing.pause(); playing = null; }
  }

  // play says parts one after the other, with one progress across them all.
  function play(s, parts, onp) {
    var total = parts.reduce(function (n, x) { return n + x.text.length; }, 0) || 1, before = 0;
    return parts.reduce(function (chain, part) {
      return chain.then(function () {
        if (s !== seq) return;
        var from = before / total, share = part.text.length / total;
        before += part.text.length;
        return say(part.text, part.audio, function (q) { onp(from + share * q); });
      });
    }, Promise.resolve());
  }

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
    var start = Date.now(), el = $("[data-timer]");
    clearInterval(timerIv);
    el.textContent = "0:00";
    timerIv = setInterval(function () { el.textContent = clock(Math.floor((Date.now() - start) / 1000)); }, 500);
  }

  // ----------------------------------------------------------- the menu

  function start() {
    var s = ++seq;
    audio();
    choice = null;
    $$(".cw-choice").forEach(function (b) { b.classList.remove("is-now", "is-chosen"); });
    $("[data-mic-help]").hidden = true;
    show("menu");
    menuStatus("Playing the menu");
    status("Playing " + line.business + "'s menu. You can pick a choice at any time.");
    var text = [line.greeting, line.menu].filter(Boolean).join(" ");
    var lit = writeOut($("[data-words]"), text, lightChoice);
    var parts = line.greetingAudio ? [{ text: text, audio: line.greetingAudio }]
      : [{ text: line.greeting }, { text: line.menu }].filter(function (x) { return x.text; });
    play(s, parts, function (p) { if (s === seq) { lit(p); lightWave(p); } }).then(function () {
      if (s === seq && phase === "menu") menuStatus("Pick a choice");
    });
  }

  function press(key) {
    if (phase !== "menu") return;
    var o = line.options.filter(function (x) { return x.key === key; })[0];
    if (!o) return;
    var s = ++seq;
    choice = o;
    tone([697, 1209], 150, 0.1);
    hush();
    $("[data-mic-help]").hidden = true;
    $$(".cw-choice").forEach(function (b) {
      b.classList.remove("is-now");
      b.classList.toggle("is-chosen", b.dataset.key === key);
    });
    menuStatus(o.label);
    if (o.action === "ring") return ring(s, o);
    if (o.action === "voicemail") return askVoicemail(s, o);
    // "play" and "link": say the owner's words, written out.
    status(o.label + ". " + (o.say || ""));
    var lit = writeOut($("[data-words]"), o.say || "");
    say(o.say || "", o.sayAudio, function (p) { if (s === seq) { lit(p); lightWave(p); } }).then(function () {
      if (s !== seq) return;
      if (o.action === "link") return end("link", o.label, "", o.url);
      $$(".cw-choice").forEach(function (b) { b.classList.remove("is-chosen"); });
      menuStatus("Anything else?");
    });
  }

  function getMic() {
    return navigator.mediaDevices && navigator.mediaDevices.getUserMedia
      ? navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true } })
      : Promise.reject(new Error("no microphone"));
  }

  function micBlocked() {
    hush();
    menuStatus("Microphone blocked");
    $$(".cw-choice").forEach(function (b) { b.classList.remove("is-chosen"); });
    $("[data-mic-help]").hidden = false;
    status("Your browser blocked the microphone, so " + line.owner + " can't hear you.");
  }

  // ----------------------------------------------------------- the call

  function ring(s, o) {
    menuStatus("Please hold…");
    var hold = o.say || "Please hold.";
    var lit = writeOut($("[data-words]"), hold);
    var spoken = say(hold, o.sayAudio, function (p) { if (s === seq) { lit(p); lightWave(p); } });
    // Asked from the key press itself: some browsers only show the microphone prompt during a tap.
    getMic().then(function (stream) {
      if (s !== seq) { stream.getTracks().forEach(function (t) { t.stop(); }); return; }
      mic = stream;
      audio(); // iPhone pauses the audio context when the microphone starts
      return spoken.then(function () { if (s === seq) dial(s, o); });
    }, function () {
      if (s === seq) micBlocked();
    });
  }

  function dial(s, o) {
    show("ringing");
    $("[data-ring-title]").textContent = "Ringing " + line.owner + "…";
    $("[data-ring-choice]").textContent = "You chose: " + o.label;
    $("[data-ring-box]").hidden = false;
    status("Ringing " + line.owner + "…");
    stopRing = ringback();
    countdown(line.ringSeconds || 30);
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
      else if (m.type === "no-answer") voicemail(s, m.token, line.owner + " can't answer right now. After the tone, please say your name and phone number.");
      else if (m.type === "voicemail") voicemail(s, m.token, "After the tone, please say your name and phone number.");
      else if (m.type === "busy") end("ended", line.owner + " is on another call", "Please try again in a few minutes.");
      else if (m.type === "hangup") end("ended", "Call ended", "Thanks for calling " + line.business + ".");
    };
    ws.onclose = function () {
      if (s === seq && (phase === "ringing" || phase === "connecting" || phase === "talking")) {
        end("ended", "The call dropped", "The connection was lost. You can call again.");
      }
    };
  }

  // countdown shows how long until the visitor can leave a message.
  function countdown(n) {
    var left = n, el = $("[data-ring-count]");
    clearInterval(countIv);
    el.textContent = clock(left);
    countIv = setInterval(function () { left = Math.max(0, left - 1); el.textContent = clock(left); }, 1000);
  }

  // "Leave a message now": stop ringing and ask for a voicemail pass on the
  // same socket.
  function leaveNow() {
    if (phase !== "ringing" || !ws || !choice) return;
    if (stopRing) { stopRing(); stopRing = null; }
    clearInterval(countIv);
    $("[data-ring-title]").textContent = "One moment…";
    send({ type: "hangup" });
    callId = null;
    send({ type: "voicemail", option: choice.label });
  }

  // A "take a voicemail" choice: get the microphone and a voicemail pass.
  function askVoicemail(s, o) {
    getMic().then(function (stream) {
      if (s !== seq) { stream.getTracks().forEach(function (t) { t.stop(); }); return; }
      mic = stream;
      audio();
      var proto = location.protocol === "https:" ? "wss:" : "ws:";
      ws = new WebSocket(proto + "//" + location.host + "/ws?role=visitor&line=" + encodeURIComponent(line.line));
      ws.onopen = function () { send({ type: "voicemail", option: o.label }); };
      ws.onmessage = function (e) {
        var m = JSON.parse(e.data);
        if (s === seq && m.type === "voicemail") voicemail(s, m.token, o.say || "After the tone, please say your name and phone number.", o.sayAudio);
      };
    }, function () {
      if (s === seq) micBlocked();
    });
  }

  // voicemail plays the prompt, beeps, then records up to two minutes.
  function voicemail(s, token, prompt, recording) {
    if (stopRing) { stopRing(); stopRing = null; }
    clearInterval(countIv);
    if (ws) { ws.onclose = null; ws.close(); ws = null; }
    if (!mic || !window.MediaRecorder) return end("ended", line.owner + " can't answer right now", "Please call again later.");
    vmToken = token;
    show("voicemail");
    recState("Recording starts after the tone", clock(MAX_MESSAGE_MS / 1000));
    $("[data-rec-bar]").style.width = "0";
    recBars.forEach(function (b) { b.classList.remove("on"); });
    buttons(false);
    status(prompt);
    var lit = writeOut($("[data-vm-words]"), prompt);
    say(prompt, recording, function (p) { if (s === seq) lit(p); }).then(function () {
      if (s === seq) record(s);
    });
  }

  function record(s) {
    tone([1000], 400, 0.12);
    var type = ["audio/webm;codecs=opus", "audio/mp4", "audio/ogg;codecs=opus", "audio/webm"].filter(function (t) {
      return MediaRecorder.isTypeSupported && MediaRecorder.isTypeSupported(t);
    })[0];
    try { recorder = new MediaRecorder(mic, type ? { mimeType: type } : undefined); }
    catch (e) { return end("ended", "Couldn't record a message", "This browser can't record. Please call again from another browser."); }
    var rec = recorder, chunks = [], began = Date.now();
    rec.ondataavailable = function (e) { if (e.data.size) chunks.push(e.data); };
    rec.onstop = function () {
      if (recorder !== rec) return; // started over, or hung up
      recorder = null;
      clearInterval(timerIv);
      if (stopMeter) { stopMeter(); stopMeter = null; }
      var blob = new Blob(chunks, { type: (rec.mimeType || type || "audio/webm").split(";")[0] });
      upload(s, vmToken, blob, Date.now() - began);
    };
    rec.start(1000);
    show("recording");
    buttons(true);
    status("Recording. Say your name and phone number, then send your message.");
    recBars.forEach(function (b) { b.classList.add("on"); });
    stopMeter = meter(mic, recBars);
    clearInterval(timerIv);
    function tick() {
      var ms = Date.now() - began;
      recState("Recording", clock(Math.floor(ms / 1000)) + " of 2:00");
      $("[data-rec-bar]").style.width = Math.min(100, ms / MAX_MESSAGE_MS * 100) + "%";
      if (ms >= MAX_MESSAGE_MS && rec.state === "recording") rec.stop();
    }
    tick();
    timerIv = setInterval(tick, 500);
  }

  // Start over: drop what was recorded and record again.
  function restart() {
    if (phase !== "recording" || !recorder) return;
    var rec = recorder;
    recorder = null;
    rec.stop();
    clearInterval(timerIv);
    if (stopMeter) { stopMeter(); stopMeter = null; }
    record(seq);
  }

  function recState(what, time) {
    $("[data-rec-state]").textContent = what;
    $("[data-rec-time]").textContent = time;
  }
  function buttons(on) {
    $('[data-action="send-message"]').disabled = !on;
    $('[data-action="restart-message"]').disabled = !on;
  }

  function upload(s, token, blob, ms) {
    show("sending");
    buttons(false);
    recState("Sending your message…", "");
    status("Sending your message…");
    var form = new FormData();
    form.append("token", token);
    form.append("duration_ms", String(ms));
    form.append("audio", blob, "message" + (/mp4/.test(blob.type) ? ".m4a" : /ogg/.test(blob.type) ? ".ogg" : ".webm"));
    fetch("/api/voicemail", { method: "POST", body: form }).then(function (r) {
      if (!r.ok) throw new Error(r.status);
      if (s === seq) end("sent", "Message sent to " + line.owner, "Thanks for calling " + line.business + ".");
    }).catch(function () {
      if (s !== seq) return;
      unsent = { token: token, blob: blob, ms: ms };
      end("unsent", "Your message wasn't sent", "Check your connection, then try again.");
    });
  }

  function send(m) {
    if (callId && !m.id) m.id = callId;
    if (ws && ws.readyState === 1) ws.send(JSON.stringify(m));
  }

  // iceConfig gets STUN and a relay login from the server; ?relay=1 forces
  // the relay (for testing).
  function iceConfig() {
    return fetch("/api/ice").then(function (r) { return r.json(); }).catch(function () { return { iceServers: [] }; })
      .then(function (c) {
        if (/[?&]relay=1/.test(location.search)) c.iceTransportPolicy = "relay";
        return c;
      });
  }

  function connect(s) {
    show("connecting");
    if (stopRing) { stopRing(); stopRing = null; }
    clearInterval(countIv);
    $("[data-ring-title]").textContent = "Connecting…";
    $("[data-ring-box]").hidden = true;
    status("Connecting…");
    iceConfig().then(function (cfg) { if (s === seq) offer(s, cfg); });
  }

  function offer(s, cfg) {
    var remote;
    pc = new RTCPeerConnection(cfg);
    mic.getTracks().forEach(function (t) { pc.addTrack(t, mic); });
    pc.ontrack = function (e) {
      var el = $("[data-remote]");
      remote = e.streams[0];
      el.srcObject = remote;
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
        if (remote) stopMeter = meter(remote, liveBars);
      } else if (pc.connectionState === "failed") {
        send({ type: "hangup" });
        end("ended", "Couldn't connect the call", "Please call again. If it keeps happening, try another network.");
      }
    };
    pc.createOffer().then(function (offer) { return pc.setLocalDescription(offer); }).then(function () {
      send({ type: "offer", sdp: pc.localDescription.sdp });
    });
  }

  function mute() {
    var b = $('[data-action="mute"]');
    var on = b.getAttribute("aria-pressed") !== "true";
    b.setAttribute("aria-pressed", String(on));
    b.lastElementChild.textContent = on ? "Muted" : "Mute";
    if (mic) mic.getTracks().forEach(function (t) { t.enabled = !on; });
  }

  function hangup() {
    send({ type: "hangup" });
    end("ended", "Call ended", "Thanks for calling " + line.business + ".");
  }

  // end stops everything and shows the last screen: kind is "ended", "sent",
  // "link" (url is the page to open) or "unsent" (the message can be resent).
  function end(kind, title, text, url) {
    seq++;
    if (recorder && recorder.state === "recording") { var rec = recorder; recorder = null; rec.stop(); }
    recorder = null;
    hush();
    if (stopRing) { stopRing(); stopRing = null; }
    if (stopMeter) { stopMeter(); stopMeter = null; }
    clearInterval(timerIv);
    clearInterval(countIv);
    if (ws) { ws.onclose = null; ws.close(); ws = null; }
    if (pc) { pc.close(); pc = null; }
    if (mic) { mic.getTracks().forEach(function (t) { t.stop(); }); mic = null; }
    callId = null;
    if (kind !== "unsent") unsent = null;
    var m = $('[data-action="mute"]');
    m.setAttribute("aria-pressed", "false");
    m.lastElementChild.textContent = "Mute";
    $("[data-end-kind]").dataset.endKind = kind;
    $("[data-end-title]").textContent = title;
    $("[data-end-text]").textContent = text || "";
    var a = $("[data-end-link]");
    a.hidden = !url;
    if (url) a.href = url;
    $('[data-action="retry-send"]').hidden = kind !== "unsent";
    status(title + ". " + (text || ""));
    show("ended");
  }

  function retrySend() {
    if (!unsent) return;
    var u = unsent;
    unsent = null;
    upload(++seq, u.token, u.blob, u.ms);
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
    var b = e.target.closest("button");
    if (!b || b.disabled) return;
    if (b.dataset.key) return press(b.dataset.key);
    switch (b.dataset.action) {
      case "call": start(); break;
      case "mute": mute(); break;
      case "hangup": hangup(); break;
      case "leave-message": leaveNow(); break;
      case "retry-mic": if (choice) press(choice.key); break;
      case "send-message": if (recorder && recorder.state === "recording") recorder.stop(); break;
      case "restart-message": restart(); break;
      case "retry-send": retrySend(); break;
    }
  });
  document.addEventListener("keydown", function (e) {
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    if (/^[0-9*#]$/.test(e.key)) press(e.key);
    else if (e.key === "Escape" && /^(menu|ringing|connecting|talking)$/.test(phase)) hangup();
  });
})();
