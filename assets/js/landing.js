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

// The voice wave behind the hero's steps: one faint grey signal behind the
// line (the line carries the colour). There is no sound before the visitor
// taps Call us, so it starts at Call us, when it is pressed, and runs on with
// the line to each next step (the times in globals.css), up to the screen.
// Then it keeps moving like a voice while it is on screen. With less motion it
// is drawn once, in full.
(function () {
  var canvas = document.querySelector("[data-hero-wave]");
  if (!canvas || !canvas.getContext) return;
  var steps = canvas.parentNode;
  var icons = steps.querySelectorAll(".lp-step-ic");
  var ctx = canvas.getContext("2d");
  var still = matchMedia("(prefers-reduced-motion: reduce)").matches;
  var down = matchMedia("(max-width: 767px)");

  // When each step lights up, and when Call us is pressed (s), as in
  // globals.css.
  var lit = [1.2, 2.2, 3.5, 4.8, 6.2];
  var pressed = 2.65;
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

  var w, h, nodes, bars, t0;
  function measure() {
    var dpr = Math.min(window.devicePixelRatio || 1, 2);
    var box = canvas.getBoundingClientRect();
    w = box.width;
    h = box.height;
    canvas.width = Math.round(w * dpr);
    canvas.height = Math.round(h * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    var vertical = down.matches;
    var gap = vertical ? 6 : 8;
    var icon = icons[0].getBoundingClientRect().width;
    nodes = [].map.call(icons, function (ic) {
      var r = ic.getBoundingClientRect();
      return vertical ? r.top + r.height / 2 - box.top : r.left + r.width / 2 - box.left;
    });
    // From Call us to the screen, behind the icons too, so it reads as one
    // signal; under an icon the bars stay within it.
    var from = nodes[1], to = nodes[nodes.length - 1];
    seed = 29;
    bars = [];
    for (var at = from; at <= to; at += gap) {
      var near = Math.min.apply(null, nodes.map(function (n) { return Math.abs(at - n); }));
      bars.push({
        at: at,
        size: Math.min(1, Math.max(0.2, 0.58 + 0.24 * Math.sin(at * 0.019 + 1.3) + 0.14 * Math.sin(at * 0.061) + (rand() - 0.5) * 0.5)),
        cap: (icon - 16) + (vertical ? w : h) * smooth(icon / 2, icon / 2 + 36, near),
        phase: rand() * 6.28,
      });
    }
  }

  // How far the voice has gone (px) at t seconds: nowhere before Call us is
  // pressed, then on to each step as it lights up.
  function reach(t) {
    var from = nodes[1], start = lit[2] - 0.55;
    for (var k = 2; k < nodes.length; k++) {
      var end = lit[k] + 0.15;
      if (t < start) return from;
      if (t < end) {
        var f = (t - start) / (end - start);
        return from + (nodes[k] - from) * (f < 0.5 ? 2 * f * f : 1 - Math.pow(-2 * f + 2, 2) / 2);
      }
      from = nodes[k];
      start = k + 1 < nodes.length ? lit[k + 1] - 0.55 : end;
    }
    return from;
  }

  function draw(t) {
    var vertical = down.matches;
    var cross = vertical ? w : h;
    var thick = vertical ? 2.5 : 3.5;
    var head = still ? Infinity : reach(t);
    var end = nodes[nodes.length - 1];
    ctx.clearRect(0, 0, w, h);
    if (!still && t < pressed) return; // no sound before Call us is pressed
    ctx.fillStyle = getComputedStyle(canvas).getPropertyValue("--wave").trim() || "#ececef";
    for (var i = 0; i < bars.length; i++) {
      var b = bars[i];
      if (b.at > head) break;
      var live = still ? 0.8 : 0.62 + 0.38 * Math.sin(t * 2.6 - b.at * 0.035 + b.phase) * Math.sin(t * 1.3 + b.phase * 2);
      // The front edge rises like a voice arriving, until it reaches the end.
      var front = head < end ? 1 + 0.3 * Math.exp(-Math.pow((head - b.at) / 36, 2)) : 1;
      var size = Math.max(thick, Math.min(b.cap, b.size * live * front * cross * 0.7));
      ctx.beginPath();
      if (vertical) {
        ctx.roundRect ? ctx.roundRect((w - size) / 2, b.at - thick / 2, size, thick, thick / 2) : ctx.rect((w - size) / 2, b.at - thick / 2, size, thick);
      } else {
        ctx.roundRect ? ctx.roundRect(b.at - thick / 2, (h - size) / 2, thick, size, thick / 2) : ctx.rect(b.at - thick / 2, (h - size) / 2, thick, size);
      }
      ctx.fill();
    }
  }

  // Keep time with the steps' own animation, so the voice meets each step.
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
