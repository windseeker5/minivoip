// Video, screen sharing and the whiteboard, shared by the visitor's call
// window (call.js), the owner's app (phone.js) and the meeting room
// (room.js). Everything goes over the same direct connection as the voice:
// every connection opens two video lanes (camera, screen) and one data
// channel up front, so turning a camera on is a track swap, never a new
// offer. The data channel carries who has what on and the whiteboard's
// strokes. The server never sees any of it.
//
//   var s = MinivoipMeet.Session({ stage: el, me: { name, initials }, mic: stream, onChange: fn,
//                                  camera: track, muted: bool });  // the last two optional
//   var link = s.link(id, pc, { name, initials, onAudio: fn });  // before the offer or the answer
//   link.offer() / link.answer(sdp)  -> the local description's SDP
//   s.camera(), s.screen(), s.board(), s.mute(on), s.drop(id), s.end()
//   MinivoipMeet.cameraError(err, blockedText): why the camera didn't start
(function () {
  var CAMERA = 0, SCREEN = 1;
  var BOARD_W = 1600, BOARD_H = 1000; // the whiteboard's own units (16:10)
  var COLORS = ["#0c0c0d", "#f46422", "#2251d1", "#1f9d55", "#d33a2c"];

  var canShare = !!(navigator.mediaDevices && navigator.mediaDevices.getDisplayMedia) &&
    !/Android|iPhone|iPad|iPod/.test(navigator.userAgent);

  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text) e.textContent = text;
    return e;
  }

  var personIcon = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="8" r="4"></circle><path d="M4 21a8 8 0 0 1 16 0"></path></svg>';
  var mutedIcon = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M9 5a3 3 0 0 1 6 0v5M15 13.5A3 3 0 0 1 9 11V9M19 11a7 7 0 0 1-11.5 5.4M5 11a7 7 0 0 0 .7 3M12 18v3M3 3l18 18"></path></svg>';

  // ------------------------------------------------------------ session

  function Session(o) {
    this.stage = o.stage;
    this.me = o.me || { name: "You", initials: "" };
    this.mic = o.mic || null;
    this.onChange = o.onChange || function () {};
    this.links = {};
    this.cam = o.camera || null; // my camera track (one already on, from a preview)
    this.scr = null;      // my screen track
    this.muted = !!o.muted;
    if (this.mic && this.muted) this.mic.getAudioTracks().forEach(function (t) { t.enabled = false; });
    this.focus = [];      // what fills the stage, newest last: "board", "screen:<link id>"
    this.tiles = {};
    this.boardOpen = false;
    this.wb = new Board(this);
    this.canShare = canShare;
    if (this.stage) this.stage.classList.add("mv-stage");
  }

  // link joins a peer connection to the session. Call it right after making
  // pc, before the offer or the answer.
  Session.prototype.link = function (id, pc, info) {
    var l = new Link(this, id, pc, info || {});
    this.links[id] = l;
    this.render();
    return l;
  };

  // drop forgets a peer (they left or the call ended).
  Session.prototype.drop = function (id) {
    var l = this.links[id];
    if (!l) return;
    delete this.links[id];
    try { l.dc.close(); } catch (e) {}
    this.unfocus("screen:" + id);
    this.render();
  };

  Session.prototype.each = function (fn) {
    for (var id in this.links) fn(this.links[id]);
  };

  Session.prototype.broadcast = function (m) {
    this.each(function (l) { l.send(m); });
  };

  Session.prototype.state = function () {
    return { t: "state", name: this.me.name, initials: this.me.initials || "", camera: !!this.cam, screen: !!this.scr, muted: this.muted };
  };

  Session.prototype.changed = function () {
    this.broadcast(this.state());
    this.render();
  };

  // camera turns my camera on or off (on: true/false, or a toggle). It
  // rejects when the browser refuses the camera.
  Session.prototype.camera = function (on) {
    var s = this;
    if (on === undefined) on = !s.cam;
    if (!on) {
      if (s.cam) { s.cam.stop(); s.cam = null; }
      s.each(function (l) { l.put(CAMERA, null); });
      s.changed();
      return Promise.resolve(false);
    }
    if (s.cam) return Promise.resolve(true);
    return navigator.mediaDevices.getUserMedia({ video: { width: { ideal: 1280 }, height: { ideal: 720 }, facingMode: "user" } }).then(function (st) {
      s.cam = st.getVideoTracks()[0];
      s.each(function (l) { l.put(CAMERA, s.cam); });
      s.changed();
      return true;
    });
  };

  // screen starts or stops showing my screen. The browser asks which
  // screen or window; its own "Stop sharing" ends it too.
  Session.prototype.screen = function (on) {
    var s = this;
    if (on === undefined) on = !s.scr;
    if (!on) {
      if (s.scr) { s.scr.onended = null; s.scr.stop(); s.scr = null; }
      s.each(function (l) { l.put(SCREEN, null); });
      s.changed();
      return Promise.resolve(false);
    }
    if (s.scr) return Promise.resolve(true);
    if (!canShare) return Promise.reject(new Error("This browser can't show its screen."));
    return navigator.mediaDevices.getDisplayMedia({ video: { frameRate: { ideal: 15 } }, audio: false }).then(function (st) {
      s.scr = st.getVideoTracks()[0];
      try { s.scr.contentHint = "detail"; } catch (e) {}
      s.scr.onended = function () { s.screen(false); };
      s.each(function (l) { l.put(SCREEN, s.scr); });
      // Showing my screen hides the board on my side, like on theirs.
      s.unfocus("board");
      s.changed();
      return true;
    });
  };

  // board opens or closes the whiteboard, for everyone.
  Session.prototype.board = function (on) {
    if (on === undefined) on = !this.boardOpen;
    this.openBoard(on);
    this.broadcast({ t: "board", open: on });
  };

  Session.prototype.openBoard = function (on) {
    this.boardOpen = on;
    if (on) this.refocus("board");
    else this.unfocus("board");
    this.render();
  };

  Session.prototype.mute = function (on) {
    this.muted = on;
    if (this.mic) this.mic.getAudioTracks().forEach(function (t) { t.enabled = !on; });
    this.changed();
  };

  Session.prototype.refocus = function (key) {
    this.unfocus(key);
    this.focus.push(key);
  };
  Session.prototype.unfocus = function (key) {
    this.focus = this.focus.filter(function (k) { return k !== key; });
  };

  // end stops my camera and screen and clears the stage.
  Session.prototype.end = function () {
    var s = this;
    Object.keys(s.links).forEach(function (id) { s.drop(id); });
    if (s.cam) { s.cam.stop(); s.cam = null; }
    if (s.scr) { s.scr.onended = null; s.scr.stop(); s.scr = null; }
    s.boardOpen = false;
    s.focus = [];
    s.wb.reset();
    s.render();
    // Leave the stage empty for the next call.
    for (var k in s.tiles) s.tiles[k].video.srcObject = null;
    s.tiles = {};
    if (s.stage) { s.stage.textContent = ""; s.stage.meet = null; }
    s.stage = null;
  };

  // active is true when something needs the stage: a camera, a screen,
  // the whiteboard, or more than two people.
  Session.prototype.active = function () {
    var on = !!(this.cam || this.scr || this.boardOpen), n = 0;
    this.each(function (l) { n++; if (l.showCamera() || l.showScreen()) on = true; });
    return on || n > 1;
  };

  // ------------------------------------------------------------ the stage

  // people is everyone on the stage, me first.
  Session.prototype.people = function () {
    var list = [{ key: "me", self: true, name: this.me.name, initials: this.me.initials, track: this.cam, muted: this.muted }];
    this.each(function (l) {
      list.push({ key: "p:" + l.id, name: l.name, initials: l.initials, track: l.showCamera() ? l.tracks[CAMERA] : null, muted: l.muted, lost: l.lost });
    });
    return list;
  };

  // main is what fills the stage: the newest of the whiteboard and the
  // screens being shown, else nothing (people only).
  Session.prototype.main = function () {
    for (var i = this.focus.length - 1; i >= 0; i--) {
      var k = this.focus[i];
      if (k === "board" && this.boardOpen) return { key: "board" };
      if (k.indexOf("screen:") === 0) {
        var l = this.links[k.slice(7)];
        if (l && l.showScreen()) return { key: k, name: l.name, track: l.tracks[SCREEN] };
      }
    }
    return null;
  };

  Session.prototype.tile = function (key) {
    var t = this.tiles[key];
    if (t) return t;
    t = this.tiles[key] = { box: el("div", "mv-tile"), video: el("video"), face: el("span", "mv-face"), label: el("span", "mv-name"), track: null };
    t.box.dataset.tile = key;
    t.video.autoplay = true;
    t.video.muted = true; // voice plays through its own audio element
    t.video.playsInline = true;
    t.video.setAttribute("playsinline", "");
    t.box.append(t.face, t.video, t.label);
    return t;
  };

  Session.prototype.fill = function (t, p) {
    if (t.track !== p.track) {
      t.track = p.track;
      t.video.srcObject = p.track ? new MediaStream([p.track]) : null;
    }
    t.box.classList.toggle("has-video", !!p.track);
    t.box.classList.toggle("is-self", !!p.self);
    t.box.classList.toggle("is-screen", !!p.screen);
    t.box.classList.toggle("is-lost", !!p.lost);
    if (p.initials) t.face.textContent = p.initials;
    else t.face.innerHTML = personIcon;
    t.label.innerHTML = "";
    if (p.muted) t.label.insertAdjacentHTML("beforeend", mutedIcon);
    t.label.append(p.screen ? p.name + "'s screen" : p.self ? "You" : p.lost ? p.name + " · reconnecting…" : p.name);
  };

  Session.prototype.render = function () {
    var stage = this.stage;
    if (!stage) return;
    var s = this, people = this.people(), main = this.main(), used = {};
    var layout = main ? "present" : people.length <= 2 ? "pair" : "grid";
    if (!stage.firstChild) {
      stage.append(el("div", "mv-main"), el("div", "mv-strip"));
      stage.append(this.sharing = el("div", "mv-sharing"));
      this.sharing.innerHTML = '<span><i></i>You\'re showing your screen</span><button type="button" data-meet="stop-screen">Stop</button>';
    }
    var mainBox = stage.querySelector(".mv-main"), strip = stage.querySelector(".mv-strip");
    stage.dataset.layout = layout;
    var wantMain = [], wantStrip = [];
    if (main && main.key === "board") {
      wantMain.push(this.wb.box);
    } else if (main) {
      var st = this.tile(main.key);
      this.fill(st, { name: main.name, track: main.track, screen: true });
      used[main.key] = true;
      wantMain.push(st.box);
    }
    people.forEach(function (p) {
      var t = s.tile(p.key);
      s.fill(t, p);
      used[p.key] = true;
      if (layout === "pair") {
        // Two people: the other one fills the stage, me in the corner
        // (only with my camera on).
        if (!p.self) wantMain.push(t.box);
        else if (p.track) { t.box.classList.add("is-pip"); wantMain.push(t.box); }
      } else {
        t.box.classList.remove("is-pip");
        (layout === "grid" ? wantMain : wantStrip).push(t.box);
      }
      if (layout !== "pair") t.box.classList.remove("is-pip");
    });
    // In the pair layout with nobody else yet, I fill the stage.
    if (layout === "pair" && people.length === 1) {
      var me = this.tiles.me;
      me.box.classList.remove("is-pip");
      wantMain = [me.box];
    }
    place(mainBox, wantMain);
    place(strip, wantStrip);
    mainBox.dataset.count = String(wantMain.length);
    for (var k in this.tiles) {
      if (!used[k]) { this.tiles[k].box.remove(); this.tiles[k].video.srcObject = null; delete this.tiles[k]; }
    }
    this.sharing.hidden = !this.scr;
    stage.querySelectorAll("video").forEach(function (v) { if (v.srcObject && v.paused) v.play().catch(function () {}); });
    if (main && main.key === "board") this.wb.fit();
    this.onChange(this);
  };

  // place puts exactly the elements in list into box, in order, moving as
  // little as possible (a moved video pauses).
  function place(box, list) {
    Array.prototype.slice.call(box.children).forEach(function (c) { if (list.indexOf(c) === -1) c.remove(); });
    list.forEach(function (c, i) { if (box.children[i] !== c) box.insertBefore(c, box.children[i] || null); });
  }

  // ------------------------------------------------------------ one peer

  function Link(s, id, pc, info) {
    var l = this;
    l.s = s;
    l.id = id;
    l.pc = pc;
    l.name = info.name || "Guest";
    l.initials = info.initials || "";
    l.onAudio = info.onAudio || function () {};
    l.state = {};
    l.tracks = [null, null];
    l.lanes = [];
    l.muted = false;
    l.lost = false;
    if (s.mic) s.mic.getTracks().forEach(function (t) { pc.addTrack(t, s.mic); });
    l.dc = pc.createDataChannel("meet", { negotiated: true, id: 0 });
    l.dc.onopen = function () {
      var hi = s.state();
      hi.t = "hi";
      hi.board = s.boardOpen;
      hi.strokes = s.wb.all();
      l.send(hi);
    };
    l.dc.onmessage = function (e) {
      var m;
      try { m = JSON.parse(e.data); } catch (err) { return; }
      l.receive(m);
    };
    pc.ontrack = function (e) {
      if (e.track.kind === "audio") return l.onAudio(e.streams[0] || new MediaStream([e.track]), e);
      var i = l.videoLanes().indexOf(e.transceiver);
      if (i < 0) return;
      l.tracks[i] = e.track;
      e.track.onunmute = function () { s.render(); };
      s.render();
    };
    pc.addEventListener("connectionstatechange", function () {
      var lost = pc.connectionState === "disconnected" || pc.connectionState === "failed";
      if (lost !== l.lost) { l.lost = lost; s.render(); }
    });
  }

  Link.prototype.videoLanes = function () {
    return this.pc.getTransceivers().filter(function (t) { return t.receiver.track && t.receiver.track.kind === "video"; });
  };

  // offer opens the two video lanes and makes the offer.
  Link.prototype.offer = function () {
    var l = this, pc = l.pc;
    l.lanes = [pc.addTransceiver("video", { direction: "sendrecv" }), pc.addTransceiver("video", { direction: "sendrecv" })];
    l.put(CAMERA, l.s.cam);
    l.put(SCREEN, l.s.scr);
    return pc.createOffer().then(function (o) { return pc.setLocalDescription(o); }).then(function () { return pc.localDescription.sdp; });
  };

  // answer takes the other side's offer and its video lanes, and answers.
  Link.prototype.answer = function (sdp) {
    var l = this, pc = l.pc;
    return pc.setRemoteDescription({ type: "offer", sdp: sdp }).then(function () {
      l.lanes = l.videoLanes().slice(0, 2);
      l.lanes.forEach(function (t) { t.direction = "sendrecv"; });
      l.put(CAMERA, l.s.cam);
      l.put(SCREEN, l.s.scr);
      return pc.createAnswer();
    }).then(function (a) { return pc.setLocalDescription(a); }).then(function () { return pc.localDescription.sdp; });
  };

  Link.prototype.put = function (lane, track) {
    var t = this.lanes[lane];
    if (t) t.sender.replaceTrack(track).catch(function () {});
  };

  Link.prototype.send = function (m) {
    if (this.dc.readyState === "open") {
      try { this.dc.send(JSON.stringify(m)); } catch (e) {}
    }
  };

  Link.prototype.showCamera = function () { return !!(this.state.camera && this.tracks[CAMERA]); };
  Link.prototype.showScreen = function () { return !!(this.state.screen && this.tracks[SCREEN]); };

  Link.prototype.receive = function (m) {
    var s = this.s;
    switch (m.t) {
      case "hi":
        if (m.board && !s.boardOpen) s.openBoard(true);
        if (m.strokes) s.wb.merge(m.strokes);
        // falls through: a hello carries the state too
      case "state":
        var was = !!this.state.screen;
        this.state = m;
        if (m.name) this.name = m.name;
        if (m.initials !== undefined) this.initials = m.initials;
        this.muted = !!m.muted;
        if (m.screen && !was) s.refocus("screen:" + this.id);
        if (!m.screen) s.unfocus("screen:" + this.id);
        s.render();
        break;
      case "board":
        s.openBoard(!!m.open);
        break;
      default:
        s.wb.receive(m);
    }
  };

  // ------------------------------------------------------------ whiteboard

  // Board is the shared whiteboard: strokes in board units, drawn by anyone,
  // sent to everyone as they're drawn.
  function Board(s) {
    var b = this;
    b.s = s;
    b.strokes = [];
    b.byId = {};
    b.color = COLORS[0];
    b.n = 0;
    b.mine = [];
    b.box = el("div", "mv-board");
    b.paper = el("div", "mv-paper");
    b.canvas = el("canvas");
    b.paper.append(b.canvas);
    var tools = el("div", "mv-tools");
    COLORS.forEach(function (c, i) {
      var d = el("button", "mv-color");
      d.type = "button";
      d.style.setProperty("--c", c);
      d.dataset.color = c;
      d.setAttribute("aria-label", ["Black", "Orange", "Blue", "Green", "Red"][i] + " pen");
      d.setAttribute("aria-pressed", String(i === 0));
      tools.append(d);
    });
    tools.append(el("span", "mv-tools-gap"));
    [["undo", "Undo"], ["clear", "Clear"]].forEach(function (x) {
      var btn = el("button", "mv-tool", x[1]);
      btn.type = "button";
      btn.dataset.board = x[0];
      tools.append(btn);
    });
    var close = el("button", "mv-tool is-icon");
    close.type = "button";
    close.dataset.board = "close";
    close.setAttribute("aria-label", "Close the whiteboard");
    close.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"></path></svg>';
    tools.append(close);
    b.box.append(tools, b.paper);
    tools.addEventListener("click", function (e) {
      var t = e.target.closest("button");
      if (!t) return;
      if (t.dataset.color) {
        b.color = t.dataset.color;
        tools.querySelectorAll(".mv-color").forEach(function (x) { x.setAttribute("aria-pressed", String(x === t)); });
      } else if (t.dataset.board === "undo") b.undo();
      else if (t.dataset.board === "clear") { b.reset(); s.broadcast({ t: "clear" }); }
      else if (t.dataset.board === "close") s.board(false);
    });
    b.draw();
    if (window.ResizeObserver) new ResizeObserver(function () { b.fit(); }).observe(b.paper);
  }

  Board.prototype.all = function () {
    return this.strokes.map(function (k) { return { t: "s", id: k.id, c: k.c, w: k.w, p: k.p }; });
  };

  Board.prototype.reset = function () {
    this.strokes = [];
    this.byId = {};
    this.mine = [];
    this.dirty();
  };

  Board.prototype.merge = function (list) {
    var b = this;
    list.forEach(function (m) { b.receive(m); });
  };

  Board.prototype.receive = function (m) {
    if (m.t === "s") {
      var k = this.byId[m.id];
      if (!k) {
        k = this.byId[m.id] = { id: m.id, c: m.c, w: m.w, p: [] };
        this.strokes.push(k);
      }
      if (m.p) k.p = k.p.concat(m.p);
      this.dirty();
    } else if (m.t === "x") {
      this.remove(m.id);
    } else if (m.t === "clear") {
      this.reset();
    }
  };

  Board.prototype.remove = function (id) {
    if (!this.byId[id]) return;
    delete this.byId[id];
    this.strokes = this.strokes.filter(function (k) { return k.id !== id; });
    this.dirty();
  };

  Board.prototype.undo = function () {
    var id = this.mine.pop();
    if (!id) return;
    this.remove(id);
    this.s.broadcast({ t: "x", id: id });
  };

  // fit sizes the canvas to the paper on screen, sharp on every screen.
  Board.prototype.fit = function () {
    var r = this.paper.getBoundingClientRect(), dpr = window.devicePixelRatio || 1;
    if (!r.width) return;
    var w = Math.round(r.width * dpr), h = Math.round(r.height * dpr);
    if (this.canvas.width !== w || this.canvas.height !== h) {
      this.canvas.width = w;
      this.canvas.height = h;
      this.dirty();
    }
  };

  Board.prototype.dirty = function () {
    var b = this;
    if (b.raf) return;
    b.raf = requestAnimationFrame(function () { b.raf = 0; b.paint(); });
  };

  Board.prototype.paint = function () {
    var c = this.canvas, g = c.getContext("2d"), k = c.width / BOARD_W;
    g.clearRect(0, 0, c.width, c.height);
    g.lineCap = "round";
    g.lineJoin = "round";
    this.strokes.forEach(function (s) {
      var p = s.p;
      if (!p.length) return;
      g.strokeStyle = s.c;
      g.fillStyle = s.c;
      g.lineWidth = s.w * k;
      if (p.length < 4) {
        g.beginPath();
        g.arc(p[0] * k, p[1] * k, s.w * k / 2, 0, Math.PI * 2);
        g.fill();
        return;
      }
      g.beginPath();
      g.moveTo(p[0] * k, p[1] * k);
      for (var i = 2; i < p.length - 2; i += 2) {
        var mx = (p[i] + p[i + 2]) / 2, my = (p[i + 1] + p[i + 3]) / 2;
        g.quadraticCurveTo(p[i] * k, p[i + 1] * k, mx * k, my * k);
      }
      g.lineTo(p[p.length - 2] * k, p[p.length - 1] * k);
      g.stroke();
    });
  };

  // draw follows the pen, mouse or finger on the paper and sends the new
  // points every few frames.
  Board.prototype.draw = function () {
    var b = this, cur = null, pending = [], sendT = 0;
    function at(e) {
      var r = b.canvas.getBoundingClientRect();
      return [Math.round((e.clientX - r.left) / r.width * BOARD_W), Math.round((e.clientY - r.top) / r.height * BOARD_H)];
    }
    function flush() {
      sendT = 0;
      if (!cur || !pending.length) return;
      b.s.broadcast({ t: "s", id: cur.id, c: cur.c, w: cur.w, p: pending });
      pending = [];
    }
    b.canvas.addEventListener("pointerdown", function (e) {
      if (e.button > 0) return;
      e.preventDefault();
      b.canvas.setPointerCapture(e.pointerId);
      var p = at(e);
      cur = { id: (b.s.me.name || "me").slice(0, 8) + "-" + Date.now().toString(36) + "-" + (b.n++), c: b.color, w: 6, p: p };
      b.strokes.push(cur);
      b.byId[cur.id] = cur;
      b.mine.push(cur.id);
      pending = p.slice();
      flush();
      b.dirty();
    });
    b.canvas.addEventListener("pointermove", function (e) {
      if (!cur) return;
      var evs = e.getCoalescedEvents ? e.getCoalescedEvents() : [e];
      (evs.length ? evs : [e]).forEach(function (x) {
        var p = at(x);
        cur.p.push(p[0], p[1]);
        pending.push(p[0], p[1]);
      });
      b.dirty();
      if (!sendT) sendT = setTimeout(flush, 40);
    });
    function up() {
      if (!cur) return;
      clearTimeout(sendT);
      flush();
      cur = null;
    }
    b.canvas.addEventListener("pointerup", up);
    b.canvas.addEventListener("pointercancel", up);
  };

  // The stage's own buttons ("Stop" while showing my screen).
  document.addEventListener("click", function (e) {
    var b = e.target.closest("[data-meet]");
    if (!b) return;
    var stage = b.closest(".mv-stage");
    var s = stage && stage.meet;
    if (s && b.dataset.meet === "stop-screen") s.screen(false);
  });
  // cameraError says why the camera didn't start, in plain words; blocked is
  // the page's own "allow it" advice.
  function cameraError(e, blocked) {
    var n = e && e.name;
    if (n === "NotFoundError" || n === "OverconstrainedError") return "No camera found on this device.";
    if (n === "NotReadableError") return "The camera is busy in another app. Close it, then try again.";
    return blocked;
  }
  var make = Session;
  window.MinivoipMeet = {
    cameraError: cameraError,
    Session: function (o) {
      var s = new make(o);
      if (o.stage) o.stage.meet = s;
      return s;
    },
    canShare: canShare,
  };
})();
