// Voicemail lists (dashboard and phone app): playing a message marks it
// played. Works on lists that phone.js swaps in later too.
document.addEventListener("play", function (e) {
  var item = e.target.closest && e.target.closest("[data-voicemail]");
  if (!item) return;
  var badge = item.querySelector("[data-new]");
  if (!badge) return;
  fetch("/voicemail/" + item.dataset.voicemail + "/listened", { method: "POST" }).then(function (r) {
    if (r.ok) badge.remove();
  });
}, true);
