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

  form.addEventListener("change", function (e) {
    if (e.target.matches("[data-action-select]")) update(e.target.closest("[data-choice]"));
  });

  form.addEventListener("click", function (e) {
    if (e.target.closest("[data-add-choice]")) {
      var row = template.content.firstElementChild.cloneNode(true);
      row.querySelector('[name="key"]').value = nextKey();
      list.appendChild(row);
      update(row);
      row.querySelector('[name="label"]').focus();
    } else if (e.target.closest("[data-remove-choice]")) {
      e.target.closest("[data-choice]").remove();
    } else if (e.target.closest("[data-preview]")) {
      preview();
    }
  });

  // Same rule as choicePhrase in lines/lines.go.
  var verbs = ["talk", "speak", "leave", "book", "hear", "reach", "get", "visit", "see", "check", "make", "order", "ask", "call"];
  function choicePhrase(label) {
    var s = label.charAt(0).toLowerCase() + label.slice(1);
    return (verbs.indexOf(s.split(/\s+/)[0]) !== -1 ? "to " : "for ") + s;
  }

  // Reads the greeting and the menu the way callers will hear them.
  function preview() {
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
