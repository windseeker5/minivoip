// Dashboard: the Home tab's windows, copy buttons, the logo button and step 2
// of sign up. "What callers hear" has its own script, menu.js.

// Logging out: this browser stops getting the business's notifications.
document.querySelectorAll('form[action="/logout"]').forEach(function (form) {
  form.addEventListener("submit", function (e) {
    if (!navigator.serviceWorker) return;
    e.preventDefault();
    var done = function () { form.submit(); };
    var giveUp = new Promise(function (ok) { setTimeout(ok, 2000); });
    Promise.race([giveUp, navigator.serviceWorker.getRegistration().then(function (reg) {
      return reg && reg.pushManager.getSubscription();
    }).then(function (sub) {
      return sub && fetch("/app/push/unsubscribe", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ endpoint: sub.endpoint }) });
    })]).then(done, done);
  });
});

// The Home tab's windows (website code, email line, meeting room, phone set
// up) open from data-open; a click on the dimmed page around one closes it.
document.querySelectorAll("[data-open]").forEach(function (btn) {
  btn.addEventListener("click", function () { document.getElementById(btn.dataset.open).showModal(); });
});
document.querySelectorAll("dialog[data-sheet]").forEach(function (d) {
  d.addEventListener("click", function (e) { if (e.target === d) d.close(); });
});

// Copy shows a check mark for a moment, and the status line tells screen
// readers. If the browser refuses, the text is selected to copy by hand.
document.querySelectorAll("[data-copy]").forEach(function (btn) {
  var timer;
  var label = btn.querySelector("[data-copy-label]").textContent;
  btn.addEventListener("click", function () {
    var field = document.getElementById(btn.dataset.copy);
    var status = document.querySelector("[data-copy-status]");
    navigator.clipboard.writeText(field.dataset.value).then(function () {
      show(true);
      if (status) status.textContent = "Copied";
      clearTimeout(timer);
      timer = setTimeout(function () { show(false); if (status) status.textContent = ""; }, 1800);
    }, function () { getSelection().selectAllChildren(field); });
  });
  function show(done) {
    btn.querySelector("[data-copy-icon]").toggleAttribute("hidden", done);
    btn.querySelector("[data-copied-icon]").toggleAttribute("hidden", !done);
    btn.querySelector("[data-copy-label]").textContent = done ? "Copied" : label;
  }
});

// The logo button opens the hidden file field (logo.js takes it from there).
document.querySelectorAll("[data-logo-pick]").forEach(function (btn) {
  btn.addEventListener("click", function () { document.getElementById(btn.dataset.logoPick).click(); });
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
