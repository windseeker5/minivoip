// Sets the theme before the first paint: the saved choice, otherwise the
// system setting. The toggle in the nav writes the same key. A plain file, not
// inline, so the site's rules can refuse every inline script.
(function () {
  try {
    var t = localStorage.getItem("theme");
    var dark = t ? t === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
    document.documentElement.classList.toggle("dark", dark);
  } catch (e) {}
})();
