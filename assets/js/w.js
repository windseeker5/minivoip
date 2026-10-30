// The Call button a business pastes into its website, just before </body>:
//
//   <script src="https://voip.example/w.js" data-line="joesbnb" async></script>
//
// It puts a button in the bottom-right corner that says whether the owner is
// here ("Talk to us now", their logo with a green dot) or away ("Leave us a
// message", grey), from /api/status/<line> (web/status.go), checked again
// every minute. A click opens the call page from our site: in a panel in the
// corner on a computer (it grows while a camera, a screen or the whiteboard
// is on), the whole screen on a phone. Everything sits in a shadow root, so
// the website's styles and ours never mix. Look: DESIGN.md "Call button".
(function () {
  var me = document.currentScript;
  if (!me || !me.dataset.line || window.minivoipButton) return;
  window.minivoipButton = true;
  var base = new URL(me.src, location.href).origin, lineName = me.dataset.line;
  var PHONE = "(max-width: 639px)", HIDE_KEY = "minivoip-bubble-hidden";
  var p = null, framed = false;

  var host = document.createElement("div");
  host.setAttribute("data-minivoip", lineName);
  var root = host.attachShadow({ mode: "open" });
  root.innerHTML =
    "<style>" +
    ":host{all:initial}" +
    ".mv{position:fixed;z-index:2147483000;right:24px;bottom:24px;display:flex;flex-direction:column;align-items:flex-end;gap:10px;" +
    "font:500 15px/1.3 'Funnel Sans',system-ui,-apple-system,'Segoe UI',sans-serif;color:#0c0c0d;-webkit-font-smoothing:antialiased}" +
    ".mv[hidden],[hidden]{display:none!important}" +
    ".bubble{display:flex;align-items:center;gap:8px;margin:0;padding:8px 8px 8px 14px;border-radius:12px;background:#fff;font-size:14px;max-width:280px;" +
    "box-shadow:0 10px 30px -14px rgba(0,0,0,.35),0 0 0 1px rgba(12,12,13,.06)}" +
    ".x{display:grid;place-items:center;flex:none;width:24px;height:24px;padding:0;border:0;border-radius:50%;background:none;color:#66666c;cursor:pointer;font:inherit}" +
    ".x:hover{background:#f4f4f5}.x svg{width:15px;height:15px}.close svg{width:18px;height:18px}" +
    ".btn{display:flex;align-items:center;gap:12px;height:60px;padding:0 22px 0 8px;border:0;border-radius:999px;background:#0c0c0d;color:#fff;" +
    "font-family:inherit;font-size:17px;font-weight:600;line-height:1;cursor:pointer;box-shadow:0 18px 40px -16px rgba(0,0,0,.55);transition:transform .2s cubic-bezier(.16,1,.3,1)}" +
    ".btn:hover{transform:translateY(-1px)}.btn:active{transform:scale(.98)}" +
    ".btn:focus-visible,.x:focus-visible,.close:focus-visible{outline:2px solid #0c0c0d;outline-offset:3px}" +
    ".face{position:relative;display:grid;place-items:center;flex:none;width:44px;height:44px;border-radius:50%;background:#fff;color:#0c0c0d;font-size:15px;font-weight:600}" +
    ".face img{width:100%;height:100%;object-fit:cover;border-radius:50%}" +
    ".dot{position:absolute;right:-1px;bottom:-1px;width:13px;height:13px;box-sizing:border-box;border-radius:50%;background:#1f9d55;border:2.5px solid #0c0c0d;animation:pulse 1.8s ease-out infinite}" +
    "@keyframes pulse{0%{box-shadow:0 0 0 0 rgba(31,157,85,.55)}70%{box-shadow:0 0 0 9px rgba(31,157,85,0)}100%{box-shadow:0 0 0 0 rgba(31,157,85,0)}}" +
    ".btn svg{width:20px;height:20px;flex:none}" +
    ".away .btn{background:#fff;color:#0c0c0d;box-shadow:0 0 0 1px rgba(12,12,13,.12),0 18px 40px -18px rgba(0,0,0,.35)}" +
    ".away .face{background:#f1efe9;color:#66666c}.away .face img{filter:grayscale(1);opacity:.7}" +
    ".away .dot{background:#c9c9ce;border-color:#fff;animation:none}" +
    ".away .bubble{color:#4a4a50}" +
    ".here .if-away,.away .if-here{display:none}" +
    ".panel{position:relative;width:400px;height:min(680px,calc(100vh - 128px));border-radius:20px;overflow:hidden;background:#fff;" +
    "box-shadow:0 30px 70px -30px rgba(0,0,0,.5),0 0 0 1px rgba(12,12,13,.08);transition:width .3s cubic-bezier(.16,1,.3,1),height .3s cubic-bezier(.16,1,.3,1)}" +
    ".panel iframe{display:block;width:100%;height:100%;border:0}" +
    ".close{position:absolute;z-index:1;top:10px;right:10px;display:grid;place-items:center;width:32px;height:32px;padding:0;border:0;border-radius:50%;" +
    "background:rgba(12,12,13,.06);color:#0c0c0d;cursor:pointer;font:inherit}" +
    ".close:hover{background:rgba(12,12,13,.12)}" +
    ".open .bubble{display:none}" +
    ".big .panel{width:min(1240px,calc(100vw - 48px));height:calc(100vh - 48px)}.big .btn{display:none}" +
    "@media " + PHONE + "{.mv{right:16px;bottom:16px}.bubble{display:none}.btn{height:54px;font-size:16px}}" +
    "@media (prefers-reduced-motion:reduce){.dot{animation:none}.btn,.panel{transition:none}}" +
    "</style>" +
    '<div class="mv here" hidden>' +
    '<div class="panel" hidden><button type="button" class="close" aria-label="Close the call window">' + icon("M6 6l12 12M18 6 6 18") + "</button></div>" +
    '<p class="bubble" role="status"><span class="if-here" data-here-text></span><span class="if-away">We\'re away. Our menu answers, or leave a message.</span>' +
    '<button type="button" class="x" aria-label="Hide this note">' + icon("M6 6l12 12M18 6 6 18") + "</button></p>" +
    '<button type="button" class="btn"><span class="face" aria-hidden="true"><i class="dot"></i></span>' +
    '<span class="if-here">' + icon("M5 4h4l2 5-2.5 1.5a11 11 0 0 0 5 5L15 13l5 2v4a2 2 0 0 1-2 2A16 16 0 0 1 3 6a2 2 0 0 1 2-2") + "</span>" +
    '<span class="if-away">' + icon("M4 5h16v11H8l-4 4zM8 10h8M8 13h5") + "</span>" +
    '<span class="if-here">Talk to us now</span><span class="if-away">Leave us a message</span></button>' +
    "</div>";

  function icon(d) {
    return '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="' + d + '"></path></svg>';
  }

  var box = root.querySelector(".mv"), btn = root.querySelector(".btn"), face = root.querySelector(".face");
  var panel = root.querySelector(".panel"), bubble = root.querySelector(".bubble");

  // show draws the button from the line's status.
  function show(s) {
    p = s;
    box.classList.toggle("here", !!s.here);
    box.classList.toggle("away", !s.here);
    box.dataset.here = String(!!s.here);
    root.querySelector("[data-here-text]").textContent = s.owner + " is here. Talk to us now.";
    btn.setAttribute("aria-label", (s.here ? "Talk to " : "Leave a message for ") + s.business);
    var dot = face.querySelector(".dot"), want = s.logo ? base + s.logo : "";
    var img = face.querySelector("img");
    if (want && (!img || img.getAttribute("src") !== want)) {
      face.textContent = "";
      img = document.createElement("img");
      img.alt = "";
      img.src = want;
      face.append(img, dot);
    } else if (!want) {
      face.textContent = s.initials;
      face.append(dot);
    }
    var hidden = false;
    try { hidden = sessionStorage.getItem(HIDE_KEY) === "1"; } catch (e) {}
    bubble.hidden = hidden;
    box.hidden = false;
  }

  function check() {
    if (document.hidden) return;
    fetch(base + "/api/status/" + encodeURIComponent(lineName), { cache: "no-store" })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (s) { if (s) show(s); })
      .catch(function () {});
  }

  function open() {
    if (!p) return;
    var url = base + p.call;
    // A phone: the call page takes the whole screen; Back returns here.
    if (window.matchMedia(PHONE).matches) { location.href = url; return; }
    if (!framed) {
      var f = document.createElement("iframe");
      f.title = "Call " + p.business;
      f.allow = "microphone; camera; display-capture; autoplay";
      f.src = url;
      panel.append(f);
      framed = true;
    }
    panel.hidden = false;
    box.classList.add("open");
    btn.setAttribute("aria-expanded", "true");
  }

  // close hides the panel and ends whatever was going on in it.
  function close() {
    var f = panel.querySelector("iframe");
    if (f) f.remove();
    framed = false;
    panel.hidden = true;
    box.classList.remove("open", "big");
    btn.setAttribute("aria-expanded", "false");
    btn.focus();
  }

  btn.addEventListener("click", function () { if (panel.hidden) open(); else close(); });
  root.querySelector(".close").addEventListener("click", close);
  root.querySelector(".x").addEventListener("click", function () {
    bubble.hidden = true;
    try { sessionStorage.setItem(HIDE_KEY, "1"); } catch (e) {}
  });
  // The call page in the panel says when to close and when to grow.
  window.addEventListener("message", function (e) {
    if (e.origin !== base || !e.data || !e.data.minivoip) return;
    if (e.data.type === "close") close();
    else if (e.data.type === "stage") box.classList.toggle("big", !!e.data.on);
  });

  function start() {
    document.body.appendChild(host);
    check();
    setInterval(check, 60000);
    document.addEventListener("visibilitychange", check);
  }
  if (document.body) start();
  else document.addEventListener("DOMContentLoaded", start);
})();
