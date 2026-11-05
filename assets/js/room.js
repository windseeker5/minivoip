// A line's meeting room (/room/<line>, pages/room.templ). The "get ready"
// screen tries the microphone and camera, then the room's socket
// (calls/room.go) says who is there; this browser connects directly to each
// of them, voice, cameras, screens and the whiteboard through meet.js.
// Guests wait until the owner lets them in; the owner's door lists who waits.
(function () {
  var root = document.querySelector("[data-room]");
  if (!root) return;
  var room = JSON.parse(document.getElementById("room").textContent);
  var $ = function (sel) { return root.querySelector(sel); };
  var $$ = function (sel) { return Array.prototype.slice.call(root.querySelectorAll(sel)); };

  var state = "lobby";
  var ws, mic, cam, meet, myId, cfg, myName, muted = false;
  var peers = {}; // id -> { pc, link, audio, name, owner }
  var waiting = []; // the host's door: { id, name }
  var doorOpen = false;

  function show(s) {
    state = s;
    root.dataset.state = s;
    var view;
    $$("[data-show]").forEach(function (el) {
      el.hidden = el.dataset.show.split(" ").indexOf(s) === -1;
      if (!el.hidden && el.tagName === "SECTION") view = el;
    });
    var f = view && view.querySelector("[data-focus]");
    if (f && !view.contains(document.activeElement)) f.focus({ preventScroll: true });
  }
  function status(t) { $("[data-status]").textContent = t; }
  function camNote(t) {
    var n = $("[data-cam-note]");
    n.textContent = t || "";
    n.hidden = !t;
  }

  // initials are a guest's face until their camera is on: "Ana Roy" is "AR".
  function initials(name) {
    return (name || "").split(/\s+/).filter(Boolean).slice(0, 2).map(function (w) { return w[0].toUpperCase(); }).join("");
  }

  // ----------------------------------------------------------- get ready

  var nameBox = $("[data-name]");
  if (nameBox) {
    try { nameBox.value = localStorage.getItem("minivoip-name") || ""; } catch (e) {}
    nameBox.addEventListener("input", function () { previewFace(); });
  }
  function previewFace() {
    if (!room.isOwner) $("[data-preview-face]").textContent = initials(nameBox.value) || "";
  }
  previewFace();

  function preCamera() {
    var b = $('[data-action="pre-camera"]');
    if (cam) {
      cam.stop();
      cam = null;
      preview();
      return;
    }
    navigator.mediaDevices.getUserMedia({ video: { width: { ideal: 1280 }, height: { ideal: 720 }, facingMode: "user" } }).then(function (st) {
      cam = st.getVideoTracks()[0];
      camNote("");
      preview();
    }, function (e) {
      camNote(MinivoipMeet.cameraError(e, "Your browser blocked the camera. Allow it next to the web address, then try again."));
      b.setAttribute("aria-pressed", "false");
    });
  }
  function preview() {
    var v = $("[data-preview-video]");
    v.srcObject = cam ? new MediaStream([cam]) : null;
    $("[data-preview]").classList.toggle("has-video", !!cam);
    $('[data-action="pre-camera"]').setAttribute("aria-pressed", String(!!cam));
  }
  function preMute() {
    muted = !muted;
    $('[data-action="pre-mute"]').setAttribute("aria-pressed", String(muted));
  }

  // ----------------------------------------------------------- joining

  function join() {
    if (state === "joining" || state === "waiting" || state === "in") return;
    myName = room.isOwner ? room.owner : (nameBox.value.trim() || "Guest");
    if (nameBox) { try { localStorage.setItem("minivoip-name", nameBox.value.trim()); } catch (e) {} }
    $("[data-mic-help]").hidden = true;
    show("joining");
    status("Joining…");
    tellGuest();
    var get = navigator.mediaDevices && navigator.mediaDevices.getUserMedia
      ? navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true } })
      : Promise.reject(new Error("no microphone"));
    get.then(function (stream) {
      mic = stream;
      connect();
    }, function () {
      show("lobby");
      $("[data-mic-help]").hidden = false;
      status("Your browser blocked the microphone.");
    });
  }

  // Notes on: a guest hears so at the door, before anything can be recorded.
  function tellGuest() {
    if (!room.notice || !window.speechSynthesis) return;
    speechSynthesis.cancel();
    var u = new SpeechSynthesisUtterance(room.notice);
    u.lang = room.noticeLang;
    speechSynthesis.speak(u);
  }

  function connect() {
    var proto = location.protocol === "https:" ? "wss:" : "ws:";
    var sock = ws = new WebSocket(proto + "//" + location.host + "/ws/room?line=" + encodeURIComponent(room.line));
    sock.onopen = function () { sock.send(JSON.stringify({ type: "join", name: myName, told: !!room.notice })); };
    sock.onmessage = function (e) {
      if (sock !== ws) return;
      var m = JSON.parse(e.data);
      if (m.type === "waiting") {
        show("waiting");
        status("Waiting for " + room.owner + " to let you in.");
      } else if (m.type === "full") {
        stop();
        show("full");
        status("This room is full.");
      } else if (m.type === "denied" || m.type === "removed" || m.type === "wait-over") {
        stop();
        var said = m.type === "denied" ? room.owner + " didn't let you in." : m.type === "removed" ? room.owner + " removed you from the room." : room.owner + " didn't answer. Try again in a little while.";
        $("[data-left-text]").textContent = said;
        show("left");
        status(said);
      } else if (m.type === "door") {
        waiting = m.peers || [];
        door();
      } else if (m.type === "seats-full") {
        doorNote("The room is full: a meeting room fits 4 people.");
      } else if (m.type === "welcome") enter(m);
      else if (m.type === "joined") { doorNote(""); peer(m.peer, false); door(); }
      else if (m.type === "left") { gone(m.id); door(); }
      else if (m.type === "signal") signal(m.from, m.data || {});
    };
    sock.onclose = function () {
      if (sock !== ws) return;
      if (state === "in" || state === "waiting" || state === "joining") {
        stop();
        $("[data-left-text]").textContent = "The connection dropped.";
        show("dropped");
        status("The connection dropped.");
      }
    };
  }

  function enter(m) {
    myId = m.id;
    // The relay login comes with being let in. ?relay=1 forces the relay (for testing).
    cfg = m.ice || { iceServers: [] };
    if (/[?&]relay=1/.test(location.search)) cfg.iceTransportPolicy = "relay";
    meet = MinivoipMeet.Session({ stage: $("[data-stage]"), me: { name: myName, initials: room.isOwner ? room.initials : initials(myName), logo: room.isOwner ? room.logo : "" },
      mic: mic, camera: cam, muted: muted, onChange: changed });
    cam = null; // the session has it now
    preview();
    show("in");
    status("You're in the meeting room.");
    // In the Android app, the microphone stays open if the screen locks now.
    if (window.MinivoipAndroid && MinivoipAndroid.callStarted) MinivoipAndroid.callStarted();
    // I'm the newcomer: I offer a connection to each person already here.
    (m.peers || []).forEach(function (p) { peer(p, true); });
    meet.render();
  }

  // ------------------------------------------------- notes, in the host's browser

  // Notes on: the host's browser records the meeting from the first guest let in,
  // everyone mixed together. With nobody left for a minute, the meeting is over.
  var notes = null, noteVoices = {}, aloneTimer = null;
  var ALONE_MS = 60000;
  function recNote(state) {
    MinivoipRecorder.show($("[data-rec-note]"), state, "meeting");
  }
  function followGuests() {
    if (!room.isOwner || !room.notes || !window.MinivoipRecorder || !mic) return;
    if (!Object.keys(peers).length) {
      if (notes && !aloneTimer) aloneTimer = setTimeout(stopNotes, ALONE_MS);
      return;
    }
    clearTimeout(aloneTimer);
    aloneTimer = null;
    // Their page loaded before notes were on, so it didn't tell them.
    if (Object.keys(peers).some(function (id) { return !peers[id].told; })) {
      stopNotes();
      recNote("not_told");
      return;
    }
    if (!notes) {
      var voices = { me: mic };
      Object.keys(noteVoices).forEach(function (id) { voices[id] = noteVoices[id]; });
      notes = MinivoipRecorder.start({ url: "/room/" + encodeURIComponent(room.line) + "/recording", voices: voices, mixed: true, mine: "me", onState: recNote });
    }
  }
  function stopNotes() {
    clearTimeout(aloneTimer);
    aloneTimer = null;
    if (notes) { notes.stop(); notes = null; }
    if (window.MinivoipRecorder && room.notes && room.isOwner) recNote("");
  }

  function peer(p, offerer) {
    if (!meet || peers[p.id]) return;
    var pc = new RTCPeerConnection(cfg);
    var audio = document.createElement("audio");
    audio.autoplay = true;
    audio.setAttribute("playsinline", "");
    $("[data-audio]").append(audio);
    var link = meet.link(p.id, pc, { name: p.name, initials: p.owner ? room.initials : initials(p.name), logo: p.owner ? room.logo : "", fixedName: true, onAudio: function (stream) {
      audio.srcObject = stream;
      audio.play().catch(function () {});
      noteVoices[p.id] = stream;
      if (notes) notes.add(p.id, stream);
    } });
    peers[p.id] = { pc: pc, link: link, audio: audio, name: p.name, owner: p.owner, told: p.told };
    followGuests();
    pc.onicecandidate = function (e) {
      if (e.candidate) send(p.id, { type: "ice", candidate: e.candidate.toJSON() });
    };
    if (offerer) link.offer().then(function (sdp) { send(p.id, { type: "offer", sdp: sdp }); });
  }

  function signal(from, d) {
    var x = peers[from];
    if (!x) return;
    if (d.type === "offer") x.link.answer(d.sdp).then(function (sdp) { send(from, { type: "answer", sdp: sdp }); });
    else if (d.type === "answer") x.pc.setRemoteDescription({ type: "answer", sdp: d.sdp }).catch(function () {});
    else if (d.type === "ice" && d.candidate) x.pc.addIceCandidate(d.candidate).catch(function () {});
  }

  function gone(id) {
    var x = peers[id];
    if (!x) return;
    delete peers[id];
    delete noteVoices[id];
    if (notes) notes.remove(id);
    followGuests();
    // Whatever the stage does, the connection closes.
    try { if (meet) meet.drop(id); } catch (e) {}
    x.pc.close();
    x.audio.remove();
  }

  // ----------------------------------------------------------- the host's door

  function doorRow(name, buttons) {
    var li = document.createElement("li");
    li.className = "rm-door-row";
    var n = document.createElement("span");
    n.className = "rm-door-name";
    n.textContent = name;
    li.append(n);
    buttons.forEach(function (b) {
      var x = document.createElement("button");
      x.type = "button";
      x.className = "rm-door-btn" + (b.yes ? " is-yes" : "");
      x.dataset.action = b.action;
      x.dataset.to = b.to;
      x.textContent = b.label;
      x.setAttribute("aria-label", b.label + " " + name);
      li.append(x);
    });
    return li;
  }

  // door shows who waits to come in; "People" also lists who to remove.
  function door() {
    var box = $("[data-door]");
    if (!box) return;
    var list = $("[data-door-waiting]"), inRoom = $("[data-door-in]");
    list.replaceChildren.apply(list, waiting.map(function (g) {
      return doorRow(g.name + " is waiting", [{ action: "deny", to: g.id, label: "Decline" }, { action: "admit", to: g.id, label: "Let in", yes: true }]);
    }));
    list.hidden = !waiting.length;
    var guests = Object.keys(peers).filter(function (id) { return !peers[id].owner; });
    inRoom.replaceChildren.apply(inRoom, guests.map(function (id) {
      return doorRow(peers[id].name, [{ action: "remove", to: id, label: "Remove" }]);
    }));
    inRoom.hidden = !doorOpen || !guests.length;
    box.hidden = list.hidden && inRoom.hidden && $("[data-door-note]").hidden;
    $('[data-action="people"]').setAttribute("aria-pressed", String(doorOpen));
    if (waiting.length) status(waiting[0].name + " is waiting to come in.");
  }
  function doorNote(t) {
    var n = $("[data-door-note]");
    if (!n) return;
    n.textContent = t;
    n.hidden = !t;
    door();
  }
  function doorSend(type, to) {
    if (ws && ws.readyState === 1) ws.send(JSON.stringify({ type: type, to: to }));
  }

  function send(to, data) {
    if (ws && ws.readyState === 1) ws.send(JSON.stringify({ type: "signal", to: to, data: data }));
  }

  // changed follows the stage: who is here, and what the buttons say.
  function changed(m) {
    var n = 1 + Object.keys(peers).length;
    $("[data-people]").textContent = "Meeting room · " + (n === 1 ? "just you" : n + " people");
    $("[data-alone]").hidden = n > 1;
    pressed("mute", m.muted);
    pressed("camera", !!m.cam);
    pressed("screen", !!m.scr);
    pressed("board", m.boardOpen);
  }
  function pressed(action, on) {
    $('.rm-controls [data-action="' + action + '"]').setAttribute("aria-pressed", String(on));
  }
  function meetNote(t) {
    var n = $("[data-meet-note]");
    n.textContent = t || "";
    n.hidden = !t;
  }

  // stop ends everything on this side.
  function stop() {
    if (window.MinivoipAndroid && MinivoipAndroid.callStopped) MinivoipAndroid.callStopped();
    stopNotes();
    var sock = ws;
    ws = null;
    if (sock) { sock.onclose = null; sock.close(); }
    Object.keys(peers).forEach(gone);
    stopNotes(); // the guests' going must not start the minute
    if (meet) {
      muted = meet.muted;
      try { meet.end(); } catch (e) {}
      meet = null;
    }
    if (mic) { mic.getTracks().forEach(function (t) { t.stop(); }); mic = null; }
    meetNote("");
    waiting = [];
  }

  function leave() {
    stop();
    $("[data-left-text]").textContent = "You left the room.";
    show("left");
    status("You left the room.");
  }

  function copy(btn) {
    var label = btn.querySelector("span");
    if (!navigator.clipboard) return;
    navigator.clipboard.writeText(room.url).then(function () {
      label.textContent = "Copied";
      setTimeout(function () { label.textContent = "Copy link"; }, 1600);
    }, function () {});
  }

  // For tests and support: each connection's route and what came in.
  window.minivoipRoomStats = function () {
    return Promise.all(Object.keys(peers).map(function (id) {
      return peers[id].pc.getStats().then(function (st) {
        var out = { id: id, route: null, bytesIn: 0, videoIn: 0 };
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
    }));
  };

  if (window.MinivoipMeet && MinivoipMeet.canShare) $("[data-share-btn]").hidden = false;
  show("lobby");

  root.addEventListener("click", function (e) {
    var b = e.target.closest("button");
    if (!b) return;
    switch (b.dataset.action) {
      case "pre-camera": preCamera(); break;
      case "pre-mute": preMute(); break;
      case "join": join(); break;
      case "leave": leave(); break;
      case "copy": copy(b); break;
      case "people": doorOpen = !doorOpen; door(); break;
      case "admit": case "deny": case "remove": doorSend(b.dataset.action, b.dataset.to); break;
      case "mute": if (meet) meet.mute(!meet.muted); break;
      case "camera":
        if (!meet) break;
        meetNote("");
        meet.camera().catch(function (e) { meetNote(MinivoipMeet.cameraError(e, "Your browser blocked the camera. Allow it next to the web address, then try again.")); });
        break;
      case "screen":
        if (!meet) break;
        meetNote("");
        meet.screen().catch(function (err) { if (err && err.name !== "NotAllowedError") meetNote("Your screen couldn't be shown. Try again."); });
        break;
      case "board": if (meet) meet.board(); break;
    }
  });
  if (nameBox) nameBox.addEventListener("keydown", function (e) { if (e.key === "Enter") join(); });
  window.addEventListener("pagehide", stop);
})();
