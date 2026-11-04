// The Notes tab: the switch saves itself, and a page with a note being written
// looks again until it's done.
(function () {
  var form = document.querySelector("[data-notes-switch]");
  if (form) {
    form.querySelector('input[type="checkbox"]').addEventListener("change", function () {
      form.submit();
    });
  }
  if (document.querySelector("[data-note-busy]")) {
    setTimeout(function () { location.reload(); }, 15000);
  }
})();
