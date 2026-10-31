// Landing page: the light/dark toggle, the hero's voice player, the Copy
// button on the code to paste, and the "Get Premium" email.

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

// The voice player plays minivoip's recorded greeting. The wave and the
// words below it follow the recording; the wave takes the recording's real
// shape when the browser can read it, and the words move only while the
// voice speaks.
(function () {
  var box = document.querySelector("[data-voice]");
  var audio = box && box.querySelector("audio");
  var btn = box && box.querySelector("[data-voice-play]");
  if (!audio || !btn) return;
  var wave = box.querySelector(".lp-wave");
  var bars = Array.prototype.slice.call(wave.querySelectorAll("i"));
  var words = Array.prototype.slice.call(box.querySelectorAll("[data-voice-text] span"));
  var head = box.querySelector(".lp-head");
  var at = box.querySelector("[data-voice-at]");
  var time = box.querySelector("[data-voice-time]");
  var length = 0;
  var from = 0; // where the voice starts and ends in the recording (s)
  var to = 0;
  var voiced = null; // for each bar: how much of the speaking is done by it (0 to 1)

  // The logo's bar colours, orange to blue (pages/wave.go brandStops).
  var stops = [[243, 132, 15], [244, 100, 34], [227, 26, 90], [159, 21, 116], [90, 38, 146], [20, 52, 149]];
  function brand(t) {
    var p = Math.min(t, 0.9999) * (stops.length - 1);
    var i = Math.floor(p);
    var f = p - i;
    return "rgb(" + stops[i].map(function (v, k) {
      return Math.round(v + (stops[i + 1][k] - v) * f);
    }).join(",") + ")";
  }
  function clock(s) {
    s = Math.max(0, Math.round(s));
    return Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
  }

  // Share of the words that word i ends at, weighted by letters.
  var ends = [];
  var letters = words.reduce(function (n, w) {
    return n + w.textContent.trim().length;
  }, 0);
  words.reduce(function (n, w) {
    n += w.textContent.trim().length;
    ends.push(n / letters);
    return n;
  }, 0);

  // Progress p (0 to 1) through the voice.
  function progress() {
    var span = to - from || length;
    return Math.max(0, Math.min(1, (audio.currentTime - from) / span));
  }
  function show(p) {
    var lit = Math.round(p * bars.length);
    bars.forEach(function (b, i) {
      var on = i < lit;
      b.classList.toggle("on", on);
      if (on) b.style.setProperty("--c", brand(i / Math.max(lit, 1)));
    });
    var spoken = voiced ? (lit > 0 ? voiced[lit - 1] : 0) : p;
    var now = ends.findIndex(function (e) {
      return e > spoken;
    });
    words.forEach(function (w, i) {
      w.classList.toggle("said", now === -1 || i < now);
      w.classList.toggle("now", i === now && p > 0 && p < 1);
    });
    head.style.left = p * 100 + "%";
    if (at) at.textContent = clock(audio.currentTime);
  }
  function learn() {
    if (!isFinite(audio.duration) || audio.duration <= 0) return false;
    length = audio.duration;
    if (!to) to = length;
    if (time) time.textContent = clock(length);
    if (at && !box.classList.contains("heard")) at.textContent = clock(from + 0.44 * (to - from));
    return true;
  }

  function measure() {
    if (learn()) return;
    // Recordings made in a browser often don't say how long they are: jump
    // to the end once to find out, then come back (to where the voice
    // starts, if the visitor already pressed play).
    audio.addEventListener("durationchange", function found() {
      if (!learn()) return;
      audio.removeEventListener("durationchange", found);
      audio.currentTime = box.classList.contains("heard") ? from : 0;
    });
    audio.currentTime = 1e9;
  }
  // On a second visit the recording comes from the browser's cache and is
  // often ready before this script runs, so "loadedmetadata" never comes.
  if (audio.readyState >= 1) measure();
  else audio.addEventListener("loadedmetadata", measure);
  audio.addEventListener("timeupdate", function () {
    if (length && box.classList.contains("heard")) show(progress());
  });
  audio.addEventListener("ended", function () {
    if (!box.classList.contains("playing")) return;
    box.classList.remove("playing");
    btn.setAttribute("aria-label", "Play minivoip's greeting");
    show(1);
  });
  function play() {
    box.classList.add("playing", "heard");
    btn.setAttribute("aria-label", "Pause");
    audio.play().catch(function () {
      box.classList.remove("playing");
    });
  }
  btn.addEventListener("click", function () {
    if (box.classList.contains("playing")) {
      audio.pause();
      box.classList.remove("playing");
      btn.setAttribute("aria-label", "Play minivoip's greeting");
      return;
    }
    if (audio.ended || !box.classList.contains("heard")) {
      audio.currentTime = from;
      show(0);
    }
    play();
  });
  // Click the wave to jump to that moment.
  wave.addEventListener("click", function (e) {
    if (!length) return;
    var r = wave.getBoundingClientRect();
    var p = Math.max(0, Math.min(1, (e.clientX - r.left) / r.width));
    audio.currentTime = from + p * ((to - from) || length);
    box.classList.add("heard");
    show(p);
    if (!box.classList.contains("playing")) play();
  });

  // The recording's real shape: the loudest moment in each slice of time.
  var Offline = window.OfflineAudioContext || window.webkitOfflineAudioContext;
  if (!Offline || !window.fetch) return;
  fetch(audio.currentSrc || audio.src)
    .then(function (r) {
      return r.arrayBuffer();
    })
    .then(function (data) {
      return new Promise(function (ok, fail) {
        new Offline(1, 1, 44100).decodeAudioData(data, ok, fail);
      });
    })
    .then(function (sound) {
      var samples = sound.getChannelData(0);
      // Skip the quiet before and after the voice.
      var loud = 0;
      for (var k = 0; k < samples.length; k++) loud = Math.max(loud, Math.abs(samples[k]));
      var a = 0;
      var b = samples.length - 1;
      while (a < b && Math.abs(samples[a]) < loud * 0.06) a++;
      while (b > a && Math.abs(samples[b]) < loud * 0.06) b--;
      var size = Math.floor((b - a) / bars.length);
      if (!size) return;
      var peaks = bars.map(function (_, i) {
        var top = 0;
        for (var j = a + i * size; j < a + (i + 1) * size; j++) top = Math.max(top, Math.abs(samples[j]));
        return top;
      });
      var loudest = Math.max.apply(null, peaks) || 1;
      bars.forEach(function (bar, i) {
        bar.style.height = Math.max(8, Math.round(100 * Math.pow(peaks[i] / loudest, 0.6))) + "%";
      });
      // The words move only through the bars where the voice speaks.
      var speaking = peaks.map(function (v) {
        return v > loudest * 0.15 ? 1 : 0;
      });
      var total = speaking.reduce(function (n, v) {
        return n + v;
      }, 0) || 1;
      var sum = 0;
      voiced = speaking.map(function (v) {
        sum += v;
        return sum / total;
      });
      from = a / sound.sampleRate;
      to = b / sound.sampleRate;
      if (length) learn();
      // Pressed play before this was known: skip the quiet start.
      if (box.classList.contains("playing") && audio.currentTime < from) audio.currentTime = from;
    })
    .catch(function () {});
})();
