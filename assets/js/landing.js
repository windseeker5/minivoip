// Landing page: the light/dark toggle and the call-flow demo.

document.querySelectorAll("[data-theme-toggle]").forEach(function (btn) {
  btn.addEventListener("click", function () {
    var dark = !document.documentElement.classList.contains("dark");
    document.documentElement.classList.toggle("dark", dark);
    try {
      localStorage.setItem("theme", dark ? "dark" : "light");
    } catch (e) {}
  });
});

// The demo is a sample call drawn as its path: the website's Call button,
// Joe's greeting, his choices and his cell, linked by wires. The wires on the
// call's path light up. Nothing rings and nothing is recorded.
(function () {
  var flow = document.querySelector("[data-flow]");
  if (!flow) return;
  var svg = flow.querySelector("[data-wires]");
  var SVGNS = "http://www.w3.org/2000/svg";

  // Every wire, from node to node; lit[state] lists the ones on the path.
  var wires = [["site", "greet"], ["greet", "o1"], ["greet", "o2"], ["greet", "o3"], ["greet", "o0"], ["o2", "phone"]];
  var paths = {};
  var lit = {
    idle: ["site-greet", "greet-o2", "o2-phone"],
    greeting: ["site-greet"],
    menu: ["site-greet"],
    book: ["site-greet", "greet-o1"],
    hours: ["site-greet", "greet-o3"],
    ringing: ["site-greet", "greet-o2", "o2-phone"],
  };
  var pick = "o2"; // the choice that led to voicemail: Talk to Joe or Leave a message

  function node(name) {
    return flow.querySelector('[data-node="' + name + '"]');
  }

  // Draw each wire from the right edge of one node to the left edge of the
  // next. Only on wide screens: on a phone the steps stack and CSS joins them.
  function draw() {
    if (getComputedStyle(svg).display === "none") return light();
    var box = flow.getBoundingClientRect();
    svg.querySelector("linearGradient").setAttribute("x2", String(box.width));
    wires.forEach(function (w) {
      var a = node(w[0]).getBoundingClientRect();
      var b = node(w[1]).getBoundingClientRect();
      var x1 = a.right - box.left, y1 = a.top + a.height / 2 - box.top;
      var x2 = b.left - box.left, y2 = b.top + b.height / 2 - box.top;
      var mx = (x1 + x2) / 2;
      var id = w[0] + "-" + w[1];
      if (!paths[id]) {
        paths[id] = document.createElementNS(SVGNS, "path");
        svg.appendChild(paths[id]);
      }
      paths[id].setAttribute("d", "M" + x1 + " " + y1 + " C" + mx + " " + y1 + " " + mx + " " + y2 + " " + x2 + " " + y2);
    });
    light();
  }

  function light() {
    var state = flow.dataset.state;
    var on = lit[state] || ["site-greet", "greet-" + pick].concat(pick === "o2" ? ["o2-phone"] : []);
    Object.keys(paths).forEach(function (id) {
      paths[id].classList.toggle("on", on.indexOf(id) !== -1);
    });
    var chosen = { idle: "o2", book: "o1", hours: "o3", ringing: "o2" }[state] || (lit[state] ? "" : pick);
    flow.querySelectorAll("[data-action][data-node]").forEach(function (b) {
      b.toggleAttribute("data-on", b.dataset.node === chosen);
      b.disabled = state !== "menu";
    });
  }

  var timers = [];
  var recSeconds = 0;

  function later(fn, ms) {
    timers.push(setTimeout(fn, ms));
  }
  function every(fn, ms) {
    timers.push(setInterval(fn, ms));
  }
  function clearTimers() {
    timers.forEach(function (t) {
      clearTimeout(t);
      clearInterval(t);
    });
    timers = [];
  }

  // Show every element whose data-when lists the state, hide the others.
  function go(state) {
    clearTimers();
    flow.dataset.state = state;
    flow.querySelectorAll("[data-when]").forEach(function (el) {
      el.hidden = el.dataset.when.split(" ").indexOf(state) === -1;
    });
    light();
    var first = flow.querySelector('[data-when~="' + state + '"] button:not([hidden])');
    if (first) first.focus({ preventScroll: true });
    else if (state === "menu") node("o1").focus({ preventScroll: true });
    if (enter[state]) enter[state]();
  }

  function clock(s) {
    return Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
  }

  var enter = {
    greeting: function () {
      later(function () { go("menu"); }, 4200);
    },
    ringing: function () {
      var n = 0;
      var out = flow.querySelector("[data-ring-count]");
      out.textContent = "0";
      every(function () { out.textContent = String(++n); }, 1000);
      later(function () { go("voicemail"); }, 5200);
    },
    recording: function () {
      recSeconds = 0;
      var out = flow.querySelector("[data-rec-count]");
      out.textContent = clock(0);
      every(function () { out.textContent = clock(++recSeconds); }, 1000);
    },
    sent: function () {
      flow.querySelector("[data-rec-final]").textContent = clock(Math.max(recSeconds, 1));
    },
  };

  var actions = {
    call: function () { pick = "o2"; go("greeting"); },
    book: function () { go("book"); },
    ring: function () { pick = "o2"; go("ringing"); },
    hours: function () { go("hours"); },
    message: function () { pick = "o0"; go("voicemail"); },
    record: function () { go("recording"); },
    send: function () { go("sent"); },
    reset: function () {
      pick = "o2";
      go("idle");
      flow.querySelector('[data-action="call"]').focus({ preventScroll: true });
    },
  };

  flow.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-action]");
    if (btn && !btn.disabled && actions[btn.dataset.action]) actions[btn.dataset.action]();
  });

  draw();
  addEventListener("resize", draw);
  if (document.fonts) document.fonts.ready.then(draw);
})();
