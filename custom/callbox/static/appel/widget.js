/* Callbox guest screen: a phone call in the browser.
 *
 * Any element with [data-appel] opens a full-screen call. The voice menu runs here, from MP3s in ./audio/ (the
 * browser's own voice if one is missing); keys 1 and 2 answer locally. Key 0 asks for the microphone, then the
 * server (/call/ws) rings the owner. Answered: WebRTC audio. Not answered: a voicemail recorded and uploaded.
 * The tap that opens the call is what lets the page play sound, so everything starts from that click.
 */
(function () {
  "use strict";

  var SCRIPT = document.currentScript;
  var ROOT = new URL(".", SCRIPT ? SCRIPT.src : location.href);      // .../appel/
  var API = new URL("/call/api/", ROOT);
  var WS_URL = new URL("/call/ws?role=guest", ROOT);
  WS_URL.protocol = WS_URL.protocol === "https:" ? "wss:" : "ws:";

  var RECORD_MAX_S = 60;
  var MENU_REPEATS = 2;          // replay the menu this many times, then say goodbye
  var MENU_IDLE_MS = 12000;

  var menu = null, menuReady = null;
  var ac = null, buffers = {}, speaking = null;
  var el = {}, built = false;
  var phase = "idle", seq = 0;
  var timerStart = 0, timerIv = 0;
  var ws = null, pc = null, mic = null, callId = null, iceConfig = null, iceReady = null;
  var stopRing = null, recorder = null, recordIv = 0, wakeLock = null, lastFocus = null, historyPushed = false;

  // ------------------------------------------------------------- sound

  function audio() {
    if (!ac) ac = new (window.AudioContext || window.webkitAudioContext)();
    if (ac.state !== "running") ac.resume();
    return ac;
  }

  function loadMenu() {
    if (menuReady) return menuReady;
    menuReady = fetch(new URL("menu", API)).then(function (r) { return r.json(); }).then(function (m) {
      menu = m;
      var keys = Object.keys(m.prompts);
      return Promise.all(keys.map(function (k) {
        return fetch(new URL("audio/" + k + ".mp3", ROOT))
          .then(function (r) { if (!r.ok) throw 0; return r.arrayBuffer(); })
          .then(function (b) { return new Promise(function (ok, no) { audio().decodeAudioData(b, ok, no); }); })
          .then(function (buf) { buffers[k] = buf; })
          .catch(function () {});   // no MP3: the browser voice reads the text instead
      }));
    }).catch(function () { menuReady = null; throw new Error("menu"); });
    return menuReady;
  }

  function hush() {
    if (speaking) { speaking(); speaking = null; }
    if (window.speechSynthesis) speechSynthesis.cancel();
  }

  // Plays one prompt; resolves when it ends or is cut off by another prompt or a key.
  function say(key) {
    hush();
    return new Promise(function (done) {
      var finished = false;
      function finish() { if (!finished) { finished = true; done(); } }
      if (buffers[key]) {
        var src = audio().createBufferSource();
        src.buffer = buffers[key];
        src.connect(ac.destination);
        src.onended = finish;
        src.start();
        speaking = function () { try { src.onended = null; src.stop(); } catch (e) {} finish(); };
      } else if (window.speechSynthesis && menu && menu.prompts[key]) {
        var u = new SpeechSynthesisUtterance(menu.prompts[key]);
        u.lang = menu.lang || "fr-CA";
        var v = speechSynthesis.getVoices().filter(function (x) { return x.lang.replace("_", "-") === u.lang; })[0]
          || speechSynthesis.getVoices().filter(function (x) { return /^fr/.test(x.lang); })[0];
        if (v) u.voice = v;
        u.onend = u.onerror = finish;
        speechSynthesis.speak(u);
        speaking = finish;
      } else {
        finish();
      }
    });
  }

  function tone(freqs, ms, level, at) {
    var ctx = audio(), t = (at || ctx.currentTime), g = ctx.createGain();
    g.gain.setValueAtTime(0, t);
    g.gain.linearRampToValueAtTime(level, t + 0.01);
    g.gain.setValueAtTime(level, t + ms / 1000 - 0.02);
    g.gain.linearRampToValueAtTime(0, t + ms / 1000);
    g.connect(ctx.destination);
    freqs.forEach(function (f) {
      var o = ctx.createOscillator();
      o.frequency.value = f;
      o.connect(g);
      o.start(t);
      o.stop(t + ms / 1000);
    });
  }

  var DTMF = { "1": [697, 1209], "2": [697, 1336], "3": [697, 1477], "4": [770, 1209], "5": [770, 1336],
    "6": [770, 1477], "7": [852, 1209], "8": [852, 1336], "9": [852, 1477], "*": [941, 1209], "0": [941, 1336],
    "#": [941, 1477] };

  // North American ringback: 440 + 480 Hz, 2 s on, 4 s off. Returns a stop function.
  function ringback(once) {
    var stopped = false, iv = 0;
    function burst() { if (!stopped) tone([440, 480], 2000, 0.09); }
    burst();
    if (!once) iv = setInterval(burst, 6000);
    return function () { stopped = true; clearInterval(iv); };
  }

  function beep() {
    tone([1000], 450, 0.12);
    return sleep(600);
  }

  function sleep(ms) { return new Promise(function (r) { setTimeout(r, ms); }); }

  // ------------------------------------------------------------- screen

  var ICONS = {
    mute: '<path d="M12 3a3 3 0 0 0-3 3v5a3 3 0 0 0 6 0V6a3 3 0 0 0-3-3Z"/><path d="M19 11a7 7 0 0 1-14 0M12 18v3"/>',
    muted: '<path d="M15 9.3V6a3 3 0 0 0-5.7-1.3M9 9v2a3 3 0 0 0 5.1 2.1M19 11a7 7 0 0 1-1.2 3.9M5 11a7 7 0 0 0 11.4 5.4M12 18v3M3 3l18 18"/>',
    pad: '<circle cx="6" cy="5" r="1.2"/><circle cx="12" cy="5" r="1.2"/><circle cx="18" cy="5" r="1.2"/><circle cx="6" cy="11" r="1.2"/><circle cx="12" cy="11" r="1.2"/><circle cx="18" cy="11" r="1.2"/><circle cx="6" cy="17" r="1.2"/><circle cx="12" cy="17" r="1.2"/><circle cx="18" cy="17" r="1.2"/><circle cx="12" cy="22" r="1.2"/>',
    hangup: '<path d="M3.6 14.6c4.6-4.1 12.2-4.1 16.8 0 .6.5.7 1.4.2 2l-1.3 1.6c-.4.5-1.1.6-1.7.4l-2.6-1a1.4 1.4 0 0 1-.9-1.3v-1.7a10 10 0 0 0-4.2 0v1.7c0 .6-.4 1.1-.9 1.3l-2.6 1c-.6.2-1.3.1-1.7-.4l-1.3-1.6c-.5-.6-.4-1.5.2-2Z"/>',
    call: '<path d="M5 3.5h3l1.6 4-2 1.3a11 11 0 0 0 7.6 7.6l1.3-2 4 1.6v3a2 2 0 0 1-2 2A16.5 16.5 0 0 1 3 5.5a2 2 0 0 1 2-2Z"/>',
    link: '<path d="M7 17 17 7M8 7h9v9"/>',
    rec: '<circle cx="12" cy="12" r="6"/>'
  };

  function svg(name, fill) {
    return '<svg viewBox="0 0 24 24" aria-hidden="true" ' + (fill ? 'fill="currentColor"' :
      'fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"') +
      ">" + ICONS[name] + "</svg>";
  }

  var LETTERS = { "2": "ABC", "3": "DEF", "4": "GHI", "5": "JKL", "6": "MNO", "7": "PQRS", "8": "TUV", "9": "WXYZ",
    "0": "+" };

  function build() {
    if (built) return;
    built = true;
    var root = document.createElement("div");
    root.className = "cbx";
    root.hidden = true;
    root.setAttribute("role", "dialog");
    root.setAttribute("aria-modal", "true");
    root.setAttribute("aria-label", "Appel");
    var keys = ["1", "2", "3", "4", "5", "6", "7", "8", "9", "*", "0", "#"].map(function (k) {
      return '<button type="button" class="cbx-key" data-key="' + k + '"><b>' + k + "</b><small>" +
        (LETTERS[k] || "&nbsp;") + "</small></button>";
    }).join("");
    root.innerHTML =
      '<div class="cbx-screen">' +
        '<div class="cbx-head">' +
          '<div class="cbx-avatar" aria-hidden="true"></div>' +
          '<h2 class="cbx-name"></h2>' +
          '<p class="cbx-status" role="status" aria-live="polite"></p>' +
          '<p class="cbx-timer" aria-hidden="true">00:00</p>' +
        "</div>" +
        '<div class="cbx-mid">' +
          '<div class="cbx-cards"></div>' +
          '<div class="cbx-pad" role="group" aria-label="Clavier" hidden>' + keys + "</div>" +
          '<div class="cbx-rec" hidden><span class="cbx-dot"></span><span class="cbx-rec-t">0:00</span>' +
            '<button type="button" class="cbx-send" data-act="send">Envoyer le message</button></div>' +
        "</div>" +
        '<div class="cbx-bar">' +
          '<button type="button" class="cbx-btn" data-act="mute" aria-pressed="false">' + svg("mute") +
            "<span>Muet</span></button>" +
          '<button type="button" class="cbx-btn" data-act="pad" aria-pressed="false">' + svg("pad", true) +
            "<span>Clavier</span></button>" +
          '<button type="button" class="cbx-btn cbx-end" data-act="hangup">' + svg("hangup", true) +
            "<span>Raccrocher</span></button>" +
        "</div>" +
        '<div class="cbx-after" hidden>' +
          '<button type="button" class="cbx-btn cbx-again" data-act="again">' + svg("call", true) +
            "<span>Rappeler</span></button>" +
          '<button type="button" class="cbx-close" data-act="close">Fermer</button>' +
        "</div>" +
        '<audio class="cbx-remote" autoplay playsinline></audio>' +
      "</div>";
    document.body.appendChild(root);
    el.root = root;
    ["avatar", "name", "status", "timer", "cards", "pad", "rec", "bar", "after"].forEach(function (k) {
      el[k] = root.querySelector(".cbx-" + k);
    });
    el.recT = root.querySelector(".cbx-rec-t");
    el.remote = root.querySelector(".cbx-remote");
    el.mute = root.querySelector('[data-act="mute"]');
    el.padBtn = root.querySelector('[data-act="pad"]');

    root.addEventListener("click", function (e) {
      var key = e.target.closest("[data-key]");
      if (key) return press(key.getAttribute("data-key"));
      var act = e.target.closest("[data-act]");
      if (!act) return;
      var a = act.getAttribute("data-act");
      if (a === "hangup") hangup();
      else if (a === "pad") showPad(el.pad.hidden);
      else if (a === "mute") toggleMute();
      else if (a === "send") stopRecording();
      else if (a === "again") start();
      else if (a === "close") close();
    });
    document.addEventListener("keydown", function (e) {
      if (el.root.hidden) return;
      if (DTMF[e.key] && !e.ctrlKey && !e.metaKey && !e.altKey) press(e.key);
      else if (e.key === "Escape") phase === "ended" ? close() : hangup();
    });
    window.addEventListener("popstate", function () {
      if (!el.root.hidden) { historyPushed = false; if (phase !== "ended") hangup(); close(); }
    });
  }

  function setStatus(t) { el.status.textContent = t; }

  function showPad(on) {
    el.pad.hidden = !on;
    el.padBtn.setAttribute("aria-pressed", on ? "true" : "false");
    el.root.classList.toggle("has-pad", on);
  }

  function card(k) {
    var c = menu && menu.cards && menu.cards[k];
    if (!c) return;
    var exists = el.cards.querySelector('[data-card="' + k + '"]');
    if (exists) exists.remove();
    var a = document.createElement("a");
    a.className = "cbx-card";
    a.setAttribute("data-card", k);
    a.href = c.url;
    if (/^https?:/.test(c.url)) { a.target = "_blank"; a.rel = "noopener"; }
    a.innerHTML = "<span></span>" + svg("link");
    a.firstChild.textContent = c.label;
    a.addEventListener("click", function () {
      // An in-page link (#tricks) ends the call and lands there.
      if (c.url.charAt(0) === "/" && phase !== "talking") { if (phase !== "ended") hangup(); close(); }
    });
    el.cards.prepend(a);
    el.cards.hidden = false;
  }

  function startTimer() {
    clearInterval(timerIv);
    timerStart = Date.now();
    el.timer.textContent = "00:00";
    timerIv = setInterval(function () { el.timer.textContent = clock(Date.now() - timerStart, true); }, 500);
  }

  function clock(ms, pad) {
    var s = Math.floor(ms / 1000), m = Math.floor(s / 60);
    return (pad && m < 10 ? "0" : "") + m + ":" + (s % 60 < 10 ? "0" : "") + (s % 60);
  }

  // ------------------------------------------------------------- the call

  function open() {
    build();
    lastFocus = document.activeElement;
    el.root.hidden = false;
    document.documentElement.classList.add("cbx-open");
    if (!historyPushed) {   // the phone's Back button hangs up instead of leaving the page
      history.pushState({ cbx: 1 }, "");
      historyPushed = true;
    }
    start();
  }

  function start() {
    var s = ++seq;
    phase = "dialing";
    audio();
    loadMenu().catch(function () {});
    el.root.classList.remove("is-ended", "is-talking");
    el.after.hidden = true;
    el.bar.hidden = false;
    el.cards.innerHTML = "";
    el.rec.hidden = true;
    showPad(false);
    el.timer.textContent = "";
    el.mute.setAttribute("aria-pressed", "false");
    el.mute.innerHTML = svg("mute") + "<span>Muet</span>";
    el.name.textContent = (menu && menu.name) || "Ken Dresdell";
    el.avatar.textContent = (menu && menu.initials) || "KD";
    setStatus("Appel en cours…");
    el.root.querySelector('[data-act="hangup"]').focus();
    if ("wakeLock" in navigator) navigator.wakeLock.request("screen").then(function (l) { wakeLock = l; }, function () {});

    stopRing = ringback(true);
    Promise.all([sleep(2600), (menuReady || Promise.reject()).catch(function () {})]).then(function () {
      if (s !== seq) return;
      if (!menu) { setStatus("Ligne indisponible"); return end(); }
      el.name.textContent = menu.name;
      el.avatar.textContent = menu.initials;
      phase = "menu";
      setStatus("En ligne");
      startTimer();
      showPad(true);
      return say("greeting").then(function () { if (s === seq) menuLoop(s, 0); });
    });
  }

  function menuLoop(s, round) {
    say("menu").then(function () {
      if (s !== seq) return;
      return sleep(MENU_IDLE_MS).then(function () {
        if (s !== seq) return;
        if (round + 1 < MENU_REPEATS) return menuLoop(s, round + 1);
        return say("goodbye").then(function () { if (s === seq) end(); });
      });
    });
  }

  function press(k) {
    if (phase === "idle" || phase === "ended") return;
    tone(DTMF[k], 160, 0.08);
    if (phase !== "menu") return;
    var s = ++seq;
    hush();
    if (k === "1" || k === "2") {
      card(k);
      say("key" + k).then(function () { return sleep(600); }).then(function () { if (s === seq) menuLoop(s, 0); });
    } else if (k === "0") {
      transfer(s);
    } else {
      say("invalid").then(function () { if (s === seq) menuLoop(s, 0); });
    }
  }

  function transfer(s) {
    phase = "transfer";
    setStatus("Transfert…");
    var spoken = say("transfer");
    // Asked from the key press itself: some browsers only show the microphone prompt during a tap.
    var gum = navigator.mediaDevices && navigator.mediaDevices.getUserMedia
      ? navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true } })
      : Promise.reject(new Error("no mic"));
    iceReady = fetch(new URL("turn", API)).then(function (r) { return r.json(); }).then(function (c) { iceConfig = c; });
    gum.then(function (stream) {
      mic = stream;
      audio();   // iPhone pauses the audio context when the microphone starts
      return spoken;
    }).then(function () {
      if (s !== seq) return;
      dial(s);
    }, function () {
      if (s !== seq) return;
      phase = "menu";
      setStatus("Micro refusé");
      card("2");
      spoken.then(function () { return say("mic"); }).then(function () { if (s === seq) menuLoop(s, 0); });
    });
  }

  function dial(s) {
    phase = "ringing";
    setStatus("Sonnerie…");
    showPad(false);
    stopRing = ringback(false);
    ws = new WebSocket(WS_URL);
    ws.onopen = function () { send({ type: "call" }); };
    ws.onmessage = function (e) {
      var m = JSON.parse(e.data);
      if (s !== seq) return;
      if (m.type === "ringing") callId = m.id;
      else if (m.type === "accepted") connect(s);
      else if (m.type === "answer" && pc) pc.setRemoteDescription({ type: "answer", sdp: m.sdp });
      else if (m.type === "ice" && pc && m.candidate) pc.addIceCandidate(m.candidate).catch(function () {});
      else if (m.type === "no-answer") voicemail(s, m.token);
      else if (m.type === "hangup") end();
      else if (m.type === "error") { stopRing(); setStatus("Trop d'appels, réessayez plus tard"); end(true); }
    };
    ws.onclose = function () {
      if (s === seq && (phase === "ringing" || phase === "connecting" || phase === "talking")) {
        setStatus("Ligne coupée");
        end(true);
      }
    };
  }

  function send(m) {
    if (callId && !m.id) m.id = callId;
    if (ws && ws.readyState === 1) ws.send(JSON.stringify(m));
  }

  function connect(s) {
    phase = "connecting";
    stopRing();
    setStatus("Connexion…");
    iceReady.catch(function () {}).then(function () {
      if (s !== seq) return;
      pc = new RTCPeerConnection(iceConfig || {});
      mic.getTracks().forEach(function (t) { pc.addTrack(t, mic); });
      cands = {};
      pc.ontrack = function (e) {
        el.remote.srcObject = e.streams[0];
        el.remote.play().catch(function (err) { report("play blocked: " + err.name); });
      };
      pc.onicecandidate = function (e) {
        if (!e.candidate) return;
        cands[e.candidate.type] = (cands[e.candidate.type] || 0) + 1;
        send({ type: "ice", candidate: e.candidate.toJSON() });
      };
      pc.oniceconnectionstatechange = function () { if (pc) report("ice " + pc.iceConnectionState); };
      pc.onconnectionstatechange = function () {
        if (s !== seq || !pc) return;
        report("conn " + pc.connectionState);
        if (pc.connectionState === "connected" && phase !== "talking") {
          setTimeout(function () { report("5 s later"); }, 5000);
          phase = "talking";
          el.root.classList.add("is-talking");
          setStatus("En ligne avec " + (menu.owner_first_name || "Ken"));
          startTimer();
        } else if (pc.connectionState === "failed") {
          setStatus("Connexion impossible");
          send({ type: "hangup" });
          end(true);
        }
      };
      return pc.createOffer().then(function (o) { return pc.setLocalDescription(o); }).then(function () {
        send({ type: "offer", sdp: pc.localDescription.sdp });
      });
    });
  }

  // Tells the server how the audio connection is doing (state, route, bytes heard); it logs it for debugging.
  var cands = {};
  function report(what) {
    var p = pc, info = { what: what, gathered: cands, net: (navigator.connection || {}).type };
    if (!p) return send({ type: "diag", info: info });
    p.getStats().then(function (st) {
      st.forEach(function (x) {
        if (x.type === "candidate-pair" && x.nominated && x.state === "succeeded") {
          var l = st.get(x.localCandidateId), r = st.get(x.remoteCandidateId);
          info.route = (l && l.candidateType) + ">" + (r && r.candidateType);
        }
        if (x.type === "inbound-rtp" && x.kind === "audio") info.heardBytes = x.bytesReceived;
      });
      send({ type: "diag", info: info });
    });
  }

  function voicemail(s, token) {
    phase = "voicemail";
    stopRing();
    setStatus("Messagerie");
    if (ws) { ws.onclose = null; ws.close(); ws = null; }
    say("voicemail").then(function () { return s === seq && beep(); }).then(function () {
      if (s !== seq || !mic) return;
      var type = ["audio/webm;codecs=opus", "audio/mp4", "audio/ogg;codecs=opus", "audio/webm"].filter(function (t) {
        return window.MediaRecorder && MediaRecorder.isTypeSupported && MediaRecorder.isTypeSupported(t);
      })[0];
      try { recorder = new MediaRecorder(mic, type ? { mimeType: type } : undefined); }
      catch (e) { setStatus("Enregistrement impossible"); return end(true); }
      var chunks = [], began = Date.now();
      recorder.ondataavailable = function (e) { if (e.data.size) chunks.push(e.data); };
      recorder.onstop = function () {
        clearInterval(recordIv);
        el.rec.hidden = true;
        var blob = new Blob(chunks, { type: (recorder.mimeType || type || "audio/webm").split(";")[0] });
        recorder = null;
        setStatus("Envoi du message…");
        var form = new FormData();
        form.append("token", token);
        form.append("audio", blob, "message." + (/mp4/.test(blob.type) ? "m4a" : /ogg/.test(blob.type) ? "ogg" : "webm"));
        fetch(new URL("voicemail", API), { method: "POST", headers: { "X-Call": "1" }, body: form })
          .then(function (r) { if (!r.ok) throw 0; setStatus("Message envoyé"); return say("thanks"); })
          .catch(function () { setStatus("Le message n'a pas pu être envoyé"); })
          .then(function () { end(true); });
      };
      recorder.start(1000);
      setStatus("Enregistrement en cours");
      el.rec.hidden = false;
      el.recT.textContent = "0:00 / 1:00";
      recordIv = setInterval(function () {
        var ms = Date.now() - began;
        el.recT.textContent = clock(ms) + " / 1:00";
        if (ms >= RECORD_MAX_S * 1000) stopRecording();
      }, 250);
    });
  }

  function stopRecording() {
    if (recorder && recorder.state !== "inactive") recorder.stop();
  }

  function toggleMute() {
    var on = el.mute.getAttribute("aria-pressed") !== "true";
    el.mute.setAttribute("aria-pressed", on ? "true" : "false");
    el.mute.innerHTML = svg(on ? "muted" : "mute") + "<span>" + (on ? "Muet activé" : "Muet") + "</span>";
    if (mic) mic.getAudioTracks().forEach(function (t) { t.enabled = !on; });
  }

  function hangup() {
    if (phase === "voicemail" && recorder) return stopRecording();   // hanging up sends the message, like a phone
    send({ type: "hangup" });
    end();
  }

  // Tears everything down and shows the end screen. keepStatus leaves an error message on screen.
  function end(keepStatus) {
    if (phase === "ended") return;
    seq++;
    var talked = timerStart && phase !== "dialing" ? Date.now() - timerStart : 0;
    phase = "ended";
    hush();
    if (stopRing) stopRing();
    clearInterval(timerIv);
    clearInterval(recordIv);
    if (recorder && recorder.state !== "inactive") { recorder.onstop = null; recorder.stop(); }
    recorder = null;
    if (pc) { pc.close(); pc = null; }
    if (mic) { mic.getTracks().forEach(function (t) { t.stop(); }); mic = null; }
    if (ws) { ws.onclose = null; ws.close(); ws = null; }
    callId = null;
    el.remote.srcObject = null;
    if (wakeLock) { wakeLock.release().catch(function () {}); wakeLock = null; }
    if (ac) tone([480, 620], 250, 0.06);
    el.root.classList.remove("is-talking");
    el.root.classList.add("is-ended");
    if (!keepStatus) setStatus("Appel terminé");
    el.timer.textContent = talked ? clock(talked, true) : "";
    timerStart = 0;
    showPad(false);
    el.rec.hidden = true;
    el.bar.hidden = true;
    el.after.hidden = false;
    card("2");
    card("1");
    el.root.querySelector('[data-act="again"]').focus();
  }

  function close() {
    if (phase !== "ended") { send({ type: "hangup" }); end(); }
    phase = "idle";
    el.root.hidden = true;
    document.documentElement.classList.remove("cbx-open");
    if (historyPushed) { historyPushed = false; history.back(); }
    if (lastFocus && lastFocus.focus) lastFocus.focus();
    document.dispatchEvent(new CustomEvent("cbx:closed"));
  }

  document.addEventListener("click", function (e) {
    var trigger = e.target.closest && e.target.closest("[data-appel]");
    if (!trigger || e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey) return;
    if (!window.RTCPeerConnection || !window.fetch || !window.Promise) return;   // the link's own page takes over
    e.preventDefault();
    open();
  });

  window.Callbox = { open: open };
})();
