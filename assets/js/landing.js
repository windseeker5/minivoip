// Landing page: the light/dark toggle and the fake demo call.

document.querySelectorAll("[data-theme-toggle]").forEach(function (btn) {
  btn.addEventListener("click", function () {
    var dark = !document.documentElement.classList.contains("dark");
    document.documentElement.classList.toggle("dark", dark);
    try {
      localStorage.setItem("theme", dark ? "dark" : "light");
    } catch (e) {}
  });
});

(function () {
  var demo = document.querySelector("[data-demo]");
  if (!demo) return;

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
    demo.dataset.state = state;
    demo.querySelectorAll("[data-when]").forEach(function (el) {
      el.hidden = el.dataset.when.split(" ").indexOf(state) === -1;
    });
    var first = demo.querySelector('[data-when~="' + state + '"] button:not([hidden])');
    if (first && state !== "idle") first.focus({ preventScroll: true });
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
      var out = demo.querySelector("[data-ring-count]");
      out.textContent = "0";
      every(function () { out.textContent = String(++n); }, 1000);
      later(function () { go("voicemail"); }, 5200);
    },
    recording: function () {
      recSeconds = 0;
      var out = demo.querySelector("[data-rec-count]");
      out.textContent = clock(0);
      every(function () { out.textContent = clock(++recSeconds); }, 1000);
    },
    sent: function () {
      demo.querySelector("[data-rec-final]").textContent = clock(Math.max(recSeconds, 1));
    },
  };

  var actions = {
    call: function () { go("greeting"); },
    book: function () { go("book"); },
    ring: function () { go("ringing"); },
    hours: function () { go("hours"); },
    record: function () { go("recording"); },
    send: function () { go("sent"); },
    reset: function () {
      go("idle");
      demo.querySelector('[data-action="call"]').focus({ preventScroll: true });
    },
  };

  demo.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-action]");
    if (btn && actions[btn.dataset.action]) actions[btn.dataset.action]();
  });
})();
