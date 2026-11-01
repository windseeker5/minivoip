// Landing page: the light/dark toggle, the Copy button on the code to paste,
// the "Get Premium" email, and the voice wave behind the hero's steps. The
// headline and the steps themselves move with CSS only (globals.css).

document.querySelectorAll("[data-theme-toggle]").forEach(function (btn) {
  btn.addEventListener("click", function () {
    var dark = !document.documentElement.classList.contains("dark");
    document.documentElement.classList.toggle("dark", dark);
    try {
      localStorage.setItem("theme", dark ? "dark" : "light");
    } catch (e) {}
  });
});

// "Get Premium": send the email (POST /premium) and thank them in place.
document.querySelectorAll("[data-premium-form]").forEach(function (form) {
  var done = form.querySelector("[data-premium-done]");
  form.addEventListener("submit", function (e) {
    e.preventDefault();
    var say = function (text) {
      done.textContent = text;
      done.classList.remove("hidden");
    };
    fetch(form.action, { method: "POST", body: new FormData(form) })
      .then(function (res) {
        if (res.ok) {
          say("Thanks! I'll write to you soon to set up Premium.");
          form.replaceChildren(done);
          return;
        }
        return res.text().then(function (text) {
          say(text.trim() || "That didn't work. Please try again.");
        });
      })
      .catch(function () {
        say("That didn't work. Please check your connection and try again.");
      });
  });
});

document.querySelectorAll("[data-copy-code]").forEach(function (btn) {
  var code = btn.closest("div").parentNode.querySelector("[data-code]");
  btn.addEventListener("click", function () {
    var done = function () {
      btn.textContent = "Copied";
      setTimeout(function () {
        btn.textContent = "Copy";
      }, 1600);
    };
    navigator.clipboard.writeText(code.textContent).then(done, function () {
      // No clipboard: select the code so the visitor can copy it.
      var range = document.createRange();
      range.selectNodeContents(code);
      var sel = window.getSelection();
      sel.removeAllRanges();
      sel.addRange(range);
    });
  });
});

// The voice wave behind the hero's five steps: bars in the logo's colours,
// quiet under each step's icon. It colours in up to each step as the step
// lights up (the times of .lp-step in globals.css), then keeps moving like a
// voice while it is on screen. With less motion it is drawn once, in colour.
(function () {
  var canvas = document.querySelector("[data-hero-wave]");
  if (!canvas || !canvas.getContext) return;
  var steps = canvas.parentNode;
  var icons = steps.querySelectorAll(".lp-step-ic");
  var ctx = canvas.getContext("2d");
  var still = matchMedia("(prefers-reduced-motion: reduce)").matches;
  var down = matchMedia("(max-width: 767px)");

  // When each step lights up (s), as in globals.css.
  var lit = [1.2, 2.2, 3.5, 4.8, 6.2];
  var stops = [
    [0, [243, 132, 15]],
    [0.3, [244, 100, 34]],
    [0.55, [227, 26, 90]],
    [0.75, [159, 21, 116]],
    [0.88, [90, 38, 146]],
    [1, [20, 52, 149]],
  ];
  function colourAt(u) {
    for (var k = 1; k < stops.length; k++) {
      if (u <= stops[k][0]) {
        var a = stops[k - 1], b = stops[k];
        var f = (u - a[0]) / (b[0] - a[0]);
        return "rgb(" + [0, 1, 2].map(function (c) {
          return Math.round(a[1][c] + (b[1][c] - a[1][c]) * f);
        }).join(",") + ")";
      }
    }
    return "rgb(20,52,149)";
  }
  function smooth(a, b, x) {
    var f = Math.min(1, Math.max(0, (x - a) / (b - a)));
    return f * f * (3 - 2 * f);
  }
  // The same wave on every visit: a seeded random.
  var seed = 29;
  function rand() {
    seed = (seed * 16807) % 2147483647;
    return seed / 2147483647;
  }

  var w, h, len, nodes, bars, gap, t0;
  function measure() {
    var dpr = Math.min(window.devicePixelRatio || 1, 2);
    var box = canvas.getBoundingClientRect();
    w = box.width;
    h = box.height;
    canvas.width = Math.round(w * dpr);
    canvas.height = Math.round(h * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    var vertical = down.matches;
    len = vertical ? h : w;
    gap = vertical ? 6 : 8;
    nodes = [].map.call(icons, function (ic) {
      var r = ic.getBoundingClientRect();
      return vertical ? r.top + r.height / 2 - box.top : r.left + r.width / 2 - box.left;
    });
    // No bars right around an icon; on a phone the steps are close, so the
    // bars come back sooner.
    var quiet = vertical ? 28 : 30;
    var ramp = vertical ? 10 : 40;
    seed = 29;
    bars = [];
    for (var at = gap / 2; at < len; at += gap) {
      var near = Math.min.apply(null, nodes.map(function (n) { return Math.abs(at - n); }));
      bars.push({
        at: at,
        colour: colourAt(at / len),
        size: Math.min(1, Math.max(0.18, 0.55 + 0.25 * Math.sin(at * 0.021 + 1.3) + 0.15 * Math.sin(at * 0.067) + (rand() - 0.5) * 0.5)),
        room: smooth(quiet, quiet + ramp, near),
        phase: rand() * 6.28,
      });
    }
  }

  // How far the colour has run (px) at t seconds: from the start to each
  // step as it lights up, then on to the end.
  function reach(t) {
    var from = 0, start = 0.65;
    for (var k = 0; k <= nodes.length; k++) {
      var to = k < nodes.length ? nodes[k] : len + 150; // past the end, so its rise leaves
      var end = k < nodes.length ? lit[k] + 0.15 : lit[lit.length - 1] + 1.2;
      if (t < start) return from;
      if (t < end) {
        var f = (t - start) / (end - start);
        return from + (to - from) * (f < 0.5 ? 2 * f * f : 1 - Math.pow(-2 * f + 2, 2) / 2);
      }
      from = to;
      start = k + 1 < nodes.length ? lit[k + 1] - 0.55 : end;
    }
    return len + 150;
  }

  function draw(t) {
    var vertical = down.matches;
    var cross = vertical ? w : h;
    var thick = vertical ? 2.5 : 3.5;
    var idle = getComputedStyle(canvas).getPropertyValue("--border").trim() || "#e7e7ea";
    var head = still ? len + 150 : reach(t);
    ctx.clearRect(0, 0, w, h);
    for (var i = 0; i < bars.length; i++) {
      var b = bars[i];
      var on = b.at <= head;
      var live = 0.62 + 0.38 * Math.sin(t * 2.6 - b.at * 0.035 + b.phase) * Math.sin(t * 1.3 + b.phase * 2);
      // The colour's front edge rises like a voice arriving.
      var front = on ? 1 + 0.5 * Math.exp(-Math.pow((head - b.at) / 36, 2)) : 0.3;
      var size = Math.min(cross, b.size * b.room * (still ? 0.8 : live) * front * cross * 0.86);
      if (size < thick * 1.8) continue; // a clear space around each step's icon
      ctx.fillStyle = on ? b.colour : idle;
      ctx.beginPath();
      if (vertical) {
        ctx.roundRect ? ctx.roundRect((w - size) / 2, b.at - thick / 2, size, thick, thick / 2) : ctx.rect((w - size) / 2, b.at - thick / 2, size, thick);
      } else {
        ctx.roundRect ? ctx.roundRect(b.at - thick / 2, (h - size) / 2, thick, size, thick / 2) : ctx.rect(b.at - thick / 2, (h - size) / 2, thick, size);
      }
      ctx.fill();
    }
  }

  // Keep time with the steps' own animation, so the colour meets each step.
  var first = steps.querySelector(".lp-step");
  var anim = first.getAnimations ? first.getAnimations()[0] : null;
  t0 = anim && anim.startTime != null ? anim.startTime : performance.now();

  var shown = true, frame = 0;
  function tick(now) {
    frame = 0;
    draw((now - t0) / 1000);
    if (shown && !document.hidden) frame = requestAnimationFrame(tick);
  }
  function go() {
    if (still) return draw(0);
    if (!frame && shown && !document.hidden) frame = requestAnimationFrame(tick);
  }
  measure();
  draw(0);
  new ResizeObserver(function () {
    measure();
    draw((performance.now() - t0) / 1000);
  }).observe(steps);
  new IntersectionObserver(function (seen) {
    shown = seen[0].isIntersecting;
    go();
  }).observe(canvas);
  document.addEventListener("visibilitychange", go);
  go();
})();
