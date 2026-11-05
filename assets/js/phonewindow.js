// "Phone app" in the dashboard's menus opens the window with the app's QR code, on
// every dashboard page. Without this script they're plain links to the web app.
(function () {
  var win = document.getElementById("dlg-phone");
  if (!win) return;
  document.querySelectorAll("[data-phone-window]").forEach(function (link) {
    link.addEventListener("click", function (e) {
      e.preventDefault();
      var menu = link.closest("[popover]");
      if (menu) menu.hidePopover();
      win.showModal();
    });
  });
  win.addEventListener("click", function (e) { if (e.target === win) win.close(); });
})();
