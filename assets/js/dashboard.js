// Dashboard: copy the call link, and the menu editor's add/remove choice
// rows, web-address field and "Hear it" preview.

document.querySelectorAll("[data-copy]").forEach(function (btn) {
  btn.addEventListener("click", function () {
    var field = document.getElementById(btn.dataset.copy);
    var label = btn.lastChild;
    navigator.clipboard.writeText(field.value).then(function () {
      label.textContent = " Copied";
      setTimeout(function () { label.textContent = " Copy"; }, 1500);
    }, function () { field.select(); });
  });
});

// Step 2 of sign up: show the call link made from the business name, the
// same way lines.Suggest makes it on the server.
(function () {
  var form = document.querySelector("[data-new-line]");
  if (!form) return;
  var business = form.querySelector("[data-business]");
  var preview = form.querySelector("[data-link-preview]");
  var field = form.querySelector("[data-link-field]");
  var name = form.querySelector('[name="name"]');
  var accents = { "é": "e", "è": "e", "ê": "e", "à": "a", "ç": "c", "ô": "o", "î": "i", "û": "u" };

  function suggest(text) {
    var s = text.toLowerCase().replace(/['’]/g, "").replace(/&/g, "and").replace(/[éèêàçôîû]/g, function (c) { return accents[c]; });
    s = s.replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 30).replace(/-+$/, "");
    while (s.length < 3) s = (s + "-line").replace(/^-+/, "");
    return s;
  }

  business.addEventListener("input", function () {
    if (field.hidden) preview.textContent = business.value.trim() ? suggest(business.value) : "your-business";
  });
  name.addEventListener("input", function () {
    preview.textContent = name.value.trim() || "your-business";
  });
  form.querySelector("[data-link-edit]").addEventListener("click", function (e) {
    e.target.hidden = true;
    field.hidden = false;
    name.value = preview.textContent === "your-business" ? "" : preview.textContent;
    name.focus();
  });
})();

(function () {
  var form = document.querySelector("[data-menu-form]");
  if (!form) return;
  var list = form.querySelector("[data-choices]");
  var template = form.querySelector("[data-choice-template]");

  var sayExamples = {
    ring: "Please hold, I'm ringing you.",
    play: "We're open every day from 9 to 5.",
    link: "Opening our booking page.",
    voicemail: "Please leave your name and number after the beep.",
  };
  var sayLabels = {
    ring: "Message before ringing (optional)",
    play: "Message to play",
    link: "Message before opening the page (optional)",
    voicemail: "Message before the beep (optional)",
  };

  // Show the web address only for "Open a web page", and name the message box.
  function update(row) {
    var action = row.querySelector("[data-action-select]").value;
    row.querySelector("[data-url-field]").hidden = action !== "link";
    row.querySelector("[data-say-label]").textContent = sayLabels[action] || "Message";
    row.querySelector('[name="say"]').placeholder = sayExamples[action] || "";
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

  // --------------------------------------------------------- recordings
  // Each [data-recorder] records the owner's voice, uploads it right away and
  // keeps its name in the hidden field until the menu is saved.
  var MAX_PROMPT_MS = 60000;
  var active = null; // the recorder in use: one at a time

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
      var iv = setInterval(function () {
        var s = Math.floor((Date.now() - began) / 1000);
        timer.textContent = Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
      }, 250);
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
    var form = new FormData();
    form.append("audio", blob, "prompt" + (/mp4/.test(blob.type) ? ".m4a" : /ogg/.test(blob.type) ? ".ogg" : ".webm"));
    fetch("/dashboard/prompt", { method: "POST", body: form }).then(function (r) {
      if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
      return r.json();
    }).then(function (j) {
      box.querySelector("[data-rec-value]").value = j.name;
      var a = box.querySelector("[data-rec-audio]");
      a.src = "/prompt/" + j.name;
      a.hidden = false;
      box.querySelector("[data-rec-clear]").hidden = false;
      box.querySelector("[data-rec-start-label]").textContent = "Record again";
      status.textContent = "Recorded. Save the menu to use it.";
    }).catch(function (err) {
      status.textContent = (err.message || "The recording couldn't be saved.").trim();
    });
  }

  function recClear(box) {
    box.querySelector("[data-rec-value]").value = "";
    var a = box.querySelector("[data-rec-audio]");
    a.pause();
    a.removeAttribute("src");
    a.hidden = true;
    box.querySelector("[data-rec-clear]").hidden = true;
    box.querySelector("[data-rec-start-label]").textContent = "Record my voice";
    box.querySelector("[data-rec-status]").textContent = "The computer voice will read the text. Save the menu to keep this.";
  }

  form.addEventListener("change", function (e) {
    if (e.target.matches("[data-action-select]")) update(e.target.closest("[data-choice]"));
  });

  form.addEventListener("click", function (e) {
    if (e.target.closest("[data-add-choice]")) {
      var row = template.content.firstElementChild.cloneNode(true);
      row.querySelector('[name="key"]').value = nextKey();
      row.querySelector('[name="say_audio"]').value = "";
      list.appendChild(row);
      update(row);
      row.querySelector('[name="label"]').focus();
    } else if (e.target.closest("[data-remove-choice]")) {
      e.target.closest("[data-choice]").remove();
    } else if (e.target.closest("[data-preview]")) {
      preview();
    } else if (e.target.closest("[data-rec-start]")) {
      recStart(e.target.closest("[data-recorder]"));
    } else if (e.target.closest("[data-rec-stop]")) {
      if (active && active.state === "recording") active.stop();
    } else if (e.target.closest("[data-rec-clear]")) {
      recClear(e.target.closest("[data-recorder]"));
    }
  });

  // Same rule as choicePhrase in lines/lines.go.
  var verbs = ["talk", "speak", "leave", "book", "hear", "reach", "get", "visit", "see", "check", "make", "order", "ask", "call"];
  function choicePhrase(label) {
    var s = label.charAt(0).toLowerCase() + label.slice(1);
    return (verbs.indexOf(s.split(/\s+/)[0]) !== -1 ? "to " : "for ") + s;
  }

  // Reads the greeting and the menu the way callers will hear them: a
  // recorded greeting replaces both.
  function preview() {
    var recorded = form.querySelector('[name="greeting_audio"]').value;
    if (recorded) {
      form.querySelector("[data-preview-text]").textContent = "Playing your recorded welcome message.";
      var a = form.querySelector('[name="greeting_audio"]').closest("[data-recorder]").querySelector("[data-rec-audio]");
      a.currentTime = 0;
      a.play();
      return;
    }
    var parts = [];
    list.querySelectorAll("[data-choice]").forEach(function (row) {
      var key = row.querySelector('[name="key"]').value.trim();
      var label = row.querySelector('[name="label"]').value.trim();
      if (key && label) parts.push("press " + key + " " + choicePhrase(label));
    });
    var menu = parts.join(", ");
    var text = form.querySelector('[name="greeting"]').value.trim() + " " +
      (menu ? menu.charAt(0).toUpperCase() + menu.slice(1) + "." : "");
    form.querySelector("[data-preview-text]").textContent = text;
    if (!window.speechSynthesis) return;
    speechSynthesis.cancel();
    var u = new SpeechSynthesisUtterance(text);
    u.lang = "en-US";
    speechSynthesis.speak(u);
  }
})();
