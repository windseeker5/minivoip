// A line's meeting room (/room/<line>, pages/room.templ). The "get ready"
// screen tries the microphone and camera, then the room's socket
// (calls/room.go) says who is there; this browser connects directly to each
// of them, voice, cameras, screens and the whiteboard through meet.js.
// Guests wait until the owner opens the room.
(function () {
  var root = document.querySelector("[data-room]");
  if (!root) return;
  var room = JSON.parse(document.getElementById("room").textContent);
  var $ = function (sel) { return root.querySelector(sel); };
  var $$ = function (sel) { return Array.prototype.slice.call(root.querySelectorAll(sel)); };

  var state = "lobby";
  var ws, mic, cam, meet, myId, cfg, myName, muted = false;
  var peers = {}; // id -> { pc, link, audio }

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
    var ice = fetch("/api/ice").then(function (r) { return r.json(); }).catch(function () { return { iceServers: [] }; });
    var get = navigator.mediaDevices && navigator.mediaDevices.getUserMedia
      ? navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true } })
      : Promise.reject(new Error("no microphone"));
    get.then(function (stream) {
      mic = stream;
      return ice;
    }, function () {
      show("lobby");
      $("[data-mic-help]").hidden = false;
      status("Your browser blocked the microphone.");
      return null;
    }).then(function (c) {
      if (!c) return;
      cfg = c;
      // ?relay=1 forces the relay (for testing).
      if (/[?&]relay=1/.test(location.search)) cfg.iceTransportPolicy = "relay";
      connect();
    });
  }

  function connect() {
    var proto = location.protocol === "https:" ? "wss:" : "ws:";
    var sock = ws = new WebSocket(proto + "//" + location.host + "/ws/room?line=" + encodeURIComponent(room.line));
    sock.onopen = function () { sock.send(JSON.stringify({ type: "join", name: myName })); };
    sock.onmessage = function (e) {
      if (sock !== ws) return;
      var m = JSON.parse(e.data);
      if (m.type === "waiting") {
        show("waiting");
        status("Waiting for " + room.owner + " to open the room.");
      } else if (m.type === "full") {
        stop();
        show("full");
        status("This room is full.");
      } else if (m.type === "welcome") enter(m);
      else if (m.type === "joined") peer(m.peer, false);
      else if (m.type === "left") gone(m.id);
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
    meet = MinivoipMeet.Session({ stage: $("[data-stage]"), me: { name: myName, initials: room.isOwner ? room.initials : initials(myName), logo: room.isOwner ? room.logo : "" },
      mic: mic, camera: cam, muted: muted, onChange: changed });
    cam = null; // the session has it now
    preview();
    show("in");
    status("You're in the meeting room.");
    // I'm the newcomer: I offer a connection to each person already here.
    (m.peers || []).forEach(function (p) { peer(p, true); });
    meet.render();
  }

  function peer(p, offerer) {
    if (!meet || peers[p.id]) return;
    var pc = new RTCPeerConnection(cfg);
    var audio = document.createElement("audio");
    audio.autoplay = true;
    audio.setAttribute("playsinline", "");
    $("[data-audio]").append(audio);
    var link = meet.link(p.id, pc, { name: p.name, initials: p.owner ? room.initials : initials(p.name), logo: p.owner ? room.logo : "", onAudio: function (stream) {
      audio.srcObject = stream;
      audio.play().catch(function () {});
    } });
    peers[p.id] = { pc: pc, link: link, audio: audio };
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
    if (meet) meet.drop(id);
    x.pc.close();
    x.audio.remove();
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
    var sock = ws;
    ws = null;
    if (sock) { sock.onclose = null; sock.close(); }
    Object.keys(peers).forEach(gone);
    if (meet) {
      muted = meet.muted;
      meet.end();
      meet = null;
    }
    if (mic) { mic.getTracks().forEach(function (t) { t.stop(); }); mic = null; }
    meetNote("");
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
