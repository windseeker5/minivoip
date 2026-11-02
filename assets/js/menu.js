// "What callers hear" (pages/hear.templ), on the dashboard and in the phone
// app: recordings, the choices' rows, "Hear it" and saving without leaving the
// page (in the app, leaving the page would drop the call socket).
(function () {
  var form = document.querySelector("[data-menu-form]");
  if (!form) return;
  var list = form.querySelector("[data-choices]");
  var template = form.querySelector("[data-choice-template]");
  var changed = false;

  var sayExamples = {
    ring: "Please hold, I'm ringing you.",
    play: "We're open every day from 9 to 5.",
    link: "Opening our booking page.",
  };
  var sayLabels = {
    ring: "Message before ringing (optional)",
    play: "Message to play",
    link: "Message before opening the page (optional)",
  };

  // Show the web address only for "Open a web page", the message box except
  // for a voicemail (it plays the voicemail greeting), and sum the row up.
  function update(row) {
    var select = row.querySelector("[data-action-select]"), action = select.value;
    var url = row.querySelector('[name="url"]').value.trim();
    row.querySelector("[data-url-field]").hidden = action !== "link";
    row.querySelector("[data-say-box]").hidden = action === "voicemail";
    row.querySelector("[data-vm-note]").hidden = action !== "voicemail";
    row.querySelector("[data-say-label]").textContent = sayLabels[action] || "Message";
    row.querySelector('[name="say"]').placeholder = sayExamples[action] || "";
    row.querySelector("[data-sum-key]").textContent = row.querySelector('[name="key"]').value.trim();
    row.querySelector("[data-sum-label]").textContent = row.querySelector('[name="label"]').value.trim() || "New choice";
    row.querySelector("[data-sum-words]").textContent = action === "link" && url ? "Opens " + url : select.selectedOptions[0].dataset.words;
  }

  // The next free key: 1 to 9, then 0.
  function nextKey() {
    var used = {};
    list.querySelectorAll('[name="key"]').forEach(function (i) { used[i.value] = true; });
    var keys = "1234567890".split("");
    for (var i = 0; i < keys.length; i++) if (!used[keys[i]]) return keys[i];
    return "";
  }

  list.querySelectorAll("[data-choice]").forEach(update);

  // --------------------------------------------------------------- saving
  var saveBar = form.querySelector("[data-save]");
  var saved = form.querySelector("[data-saved]");
  var error = form.querySelector("[data-menu-error]");

  var savedTimer;

  function markChanged() {
    changed = true;
    saveBar.hidden = false;
    saved.hidden = true;
    clearTimeout(savedTimer);
  }

  form.addEventListener("submit", function (e) {
    e.preventDefault();
    var btn = form.querySelector("[data-save-button]");
    btn.disabled = true;
    error.hidden = true;
    // getAttribute: form.action would find the choices' "action" fields.
    fetch(form.getAttribute("action"), { method: "POST", body: new URLSearchParams(new FormData(form)) }).then(function (r) {
      if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
      changed = false;
      saveBar.hidden = true;
      saved.hidden = false;
      savedTimer = setTimeout(function () { saved.hidden = true; }, 4000);
      form.querySelectorAll("[data-rec-status]").forEach(function (s) { s.textContent = ""; });
    }).catch(function (err) {
      error.textContent = (err.message || "").trim() || "It couldn't be saved. Try again.";
      error.hidden = false;
      // The problem may be in a folded choice.
      list.querySelectorAll("[data-choice]").forEach(function (row) { row.open = true; });
    }).finally(function () { btn.disabled = false; });
  });

  window.addEventListener("beforeunload", function (e) {
    if (changed) { e.preventDefault(); e.returnValue = ""; }
  });

  // ----------------------------------------------------------- recordings
  // Each [data-recorder] records the owner's voice, uploads it right away and
  // keeps its name in the hidden field until the page is saved.
  var MAX_PROMPT_MS = 60000;
  var active = null; // the recorder in use: one at a time
  var playing = null; // the player in use: one at a time

  function recStart(box) {
    if (active) return;
    var status = box.querySelector("[data-rec-status]");
    if (!navigator.mediaDevices || !window.MediaRecorder) { status.textContent = "This browser can't record."; return; }
    navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true } }).then(function (stream) {
      var type = ["audio/webm;codecs=opus", "audio/mp4", "audio/ogg;codecs=opus", "audio/webm"].filter(function (t) {
        return MediaRecorder.isTypeSupported && MediaRecorder.isTypeSupported(t);
      })[0];
      var rec = new MediaRecorder(stream, type ? { mimeType: type } : undefined);
      var chunks = [], began = Date.now();
      var timer = box.querySelector("[data-rec-timer]");
      var iv = setInterval(function () { timer.textContent = mmss((Date.now() - began) / 1000); }, 250);
      var limit = setTimeout(function () { if (rec.state === "recording") rec.stop(); }, MAX_PROMPT_MS);
      rec.ondataavailable = function (e) { if (e.data.size) chunks.push(e.data); };
      rec.onstop = function () {
        clearInterval(iv);
        clearTimeout(limit);
        stream.getTracks().forEach(function (t) { t.stop(); });
        box.querySelector("[data-rec-stop]").hidden = true;
        box.querySelector("[data-rec-start]").hidden = false;
        active = null;
        upload(box, new Blob(chunks, { type: (rec.mimeType || type || "audio/webm").split(";")[0] }));
      };
      active = rec;
      rec.start(500);
      timer.textContent = "0:00";
      status.textContent = "";
      box.querySelector("[data-rec-start]").hidden = true;
      box.querySelector("[data-rec-stop]").hidden = false;
      box.querySelector("[data-rec-stop]").focus();
    }, function () {
      status.textContent = "Microphone blocked: allow it to record.";
    });
  }

  function upload(box, blob) {
    var status = box.querySelector("[data-rec-status]");
    status.textContent = "Saving the recording…";
    var body = new FormData();
    body.append("audio", blob, "prompt" + (/mp4/.test(blob.type) ? ".m4a" : /ogg/.test(blob.type) ? ".ogg" : ".webm"));
    fetch("/dashboard/prompt", { method: "POST", body: body }).then(function (r) {
      if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
      return r.json();
    }).then(function (j) {
      box.querySelector("[data-rec-value]").value = j.name;
      box.querySelector("[data-rec-audio]").src = "/prompt/" + j.name;
      voiced(box, true);
      drawPlayer(box.querySelector("[data-rec-player]"));
      if (box.querySelector('[name="greeting_audio"]')) { recordedWords = welcomeWords(); checkStale(); }
      status.textContent = "Recorded. Save to use it.";
      markChanged();
    }).catch(function (err) {
      status.textContent = (err.message || "The recording couldn't be saved.").trim();
    });
  }

  function recClear(box) {
    box.querySelector("[data-rec-value]").value = "";
    var a = box.querySelector("[data-rec-audio]");
    a.pause();
    a.removeAttribute("src");
    voiced(box, false);
    if (box.querySelector('[name="greeting_audio"]')) checkStale();
    box.querySelector("[data-rec-status]").textContent = "The computer voice will read your words. Save to keep this.";
    markChanged();
  }

  // voiced shows a card with a recording (the player) or without (the words).
  function voiced(box, on) {
    box.querySelector("[data-rec-player]").hidden = !on;
    box.querySelector("[data-rec-text]").hidden = on;
    box.querySelector("[data-rec-clear]").hidden = !on;
    box.querySelector("[data-rec-start-label]").textContent = on ? "Record again" : "Record my voice";
    var start = box.querySelector("[data-rec-start]");
    start.classList.toggle("is-ink", !on && start.hasAttribute("data-strong"));
    var chip = box.querySelector("[data-rec-chip]");
    if (chip) {
      chip.textContent = on ? "Your voice" : "Computer voice";
      chip.classList.toggle("is-you", on);
    }
  }

  // -------------------------------------------------------------- players
  var BARS = 64;

  function mmss(s) {
    s = Math.floor(s);
    return Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
  }

  // drawPlayer draws a recording's real voice; flat bars until it's read (or
  // if this browser can't read it).
  function drawPlayer(player) {
    var audio = player.querySelector("[data-rec-audio]"), box = player.querySelector("[data-rec-bars]");
    var url = audio.getAttribute("src");
    if (!url) return;
    function bars(h) {
      box.textContent = "";
      h.forEach(function (x) {
        var b = document.createElement("i");
        b.style.height = Math.round(Math.max(0.12, x) * 100) + "%";
        box.append(b);
      });
    }
    bars(new Array(BARS).fill(0.3));
    window.minivoipPeaks(url, BARS).then(function (h) {
      if (audio.getAttribute("src") !== url) return;
      bars(h);
      player.querySelector("[data-rec-time]").textContent = mmss(h.seconds);
    }, function () {});
  }

  // Read a recording only once it's on screen: in the phone app this tab is
  // often never opened.
  var watcher = window.IntersectionObserver && new IntersectionObserver(function (seen) {
    seen.forEach(function (e) {
      if (e.isIntersecting) { watcher.unobserve(e.target); drawPlayer(e.target); }
    });
  });
  form.querySelectorAll("[data-rec-player]").forEach(function (p) {
    if (watcher) watcher.observe(p); else drawPlayer(p);
  });

  function toggle(player) {
    var audio = player.querySelector("[data-rec-audio]");
    if (playing && playing !== audio) playing.pause();
    speak("");
    if (audio.paused) { playing = audio; audio.play().catch(function () {}); } else audio.pause();
  }

  // The media events don't bubble, so they're caught on the way down.
  ["play", "pause", "ended"].forEach(function (name) {
    form.addEventListener(name, function (e) {
      var player = e.target.closest("[data-rec-player]");
      if (!player) return;
      var on = name === "play";
      player.querySelector("[data-rec-icon-play]").toggleAttribute("hidden", on);
      player.querySelector("[data-rec-icon-pause]").toggleAttribute("hidden", !on);
      player.querySelector("[data-rec-play]").setAttribute("aria-label", on ? "Pause" : "Play");
      if (name === "ended") e.target.currentTime = 0;
    }, true);
  });
  form.addEventListener("timeupdate", function (e) {
    var player = e.target.closest("[data-rec-player]");
    if (!player) return;
    var d = e.target.duration, f = d > 0 && isFinite(d) ? e.target.currentTime / d : 0;
    var bars = player.querySelector("[data-rec-bars]").children;
    for (var i = 0; i < bars.length; i++) bars[i].classList.toggle("is-played", (i + 0.5) / bars.length <= f);
  }, true);

  // ----------------------------------------------------- words and preview
  var script = form.querySelector("[data-script]");
  var stale = form.querySelector("[data-script-stale]");
  var greeting = form.querySelector('[name="greeting"]');

  // Same rule as choicePhrase in lines/lines.go.
  var verbs = ["talk", "speak", "leave", "book", "hear", "reach", "get", "visit", "see", "check", "make", "order", "ask", "call"];
  function choicePhrase(label) {
    var s = label.charAt(0).toLowerCase() + label.slice(1);
    return (verbs.indexOf(s.split(/\s+/)[0]) !== -1 ? "to " : "for ") + s;
  }

  // The greeting and the spoken menu, as lines.WelcomeWords says them.
  function welcomeWords() {
    var parts = [];
    list.querySelectorAll("[data-choice]").forEach(function (row) {
      var key = row.querySelector('[name="key"]').value.trim();
      var label = row.querySelector('[name="label"]').value.trim();
      if (key && label) parts.push("press " + key + " " + choicePhrase(label));
    });
    var menu = parts.join(", ");
    return (greeting.value.trim() + " " + (menu ? menu.charAt(0).toUpperCase() + menu.slice(1) + "." : "")).trim();
  }

  // A recorded welcome says the choices as they were when it was recorded:
  // warn when they've changed since.
  var recordedWords = welcomeWords();
  function checkStale() {
    stale.hidden = !form.querySelector('[name="greeting_audio"]').value || welcomeWords() === recordedWords;
  }

  function choicesChanged() {
    script.textContent = welcomeWords();
    checkStale();
  }

  function speak(text) {
    if (!window.speechSynthesis) return;
    speechSynthesis.cancel();
    if (!text) return;
    var u = new SpeechSynthesisUtterance(text);
    u.lang = "en-US";
    speechSynthesis.speak(u);
  }

  // Plays the welcome the way callers will hear it: a recording replaces both
  // the greeting and the menu.
  function preview() {
    var note = form.querySelector("[data-preview-text]");
    var box = greeting.closest("[data-recorder]");
    if (box.querySelector('[name="greeting_audio"]').value) {
      note.textContent = "Playing your recorded welcome message.";
      var a = box.querySelector("[data-rec-audio]");
      a.currentTime = 0;
      if (a.paused) toggle(box.querySelector("[data-rec-player]"));
      return;
    }
    if (playing) playing.pause();
    note.textContent = welcomeWords();
    speak(note.textContent);
  }

  // ---------------------------------------------------------------- events
  form.addEventListener("input", function (e) {
    markChanged();
    var row = e.target.closest("[data-choice]");
    if (row) update(row);
    if (e.target === greeting) script.textContent = welcomeWords();
    else if (e.target.matches('[name="key"], [name="label"]')) choicesChanged();
  });

  form.addEventListener("change", function (e) {
    if (e.target.matches("[data-action-select]")) update(e.target.closest("[data-choice]"));
  });

  form.addEventListener("click", function (e) {
    if (e.target.closest("[data-add-choice]")) {
      var row = template.content.firstElementChild.cloneNode(true);
      row.querySelector('[name="key"]').value = nextKey();
      row.querySelector('[name="say_audio"]').value = "";
      row.open = true;
      list.appendChild(row);
      update(row);
      row.querySelector('[name="label"]').focus();
      markChanged();
    } else if (e.target.closest("[data-remove-choice]")) {
      e.target.closest("[data-choice]").remove();
      markChanged();
      choicesChanged();
    } else if (e.target.closest("[data-preview]")) {
      preview();
    } else if (e.target.closest("[data-speak]")) {
      if (playing) playing.pause();
      speak(e.target.closest("[data-rec-text]").querySelector("textarea").value);
    } else if (e.target.closest("[data-rec-play]")) {
      toggle(e.target.closest("[data-rec-player]"));
    } else if (e.target.closest("[data-rec-start]")) {
      recStart(e.target.closest("[data-recorder]"));
    } else if (e.target.closest("[data-rec-stop]")) {
      if (active && active.state === "recording") active.stop();
    } else if (e.target.closest("[data-rec-clear]")) {
      recClear(e.target.closest("[data-recorder]"));
    }
  });
})();
