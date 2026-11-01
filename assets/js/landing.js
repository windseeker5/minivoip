// Landing page: the light/dark toggle, the Copy button on the code to paste,
// and the "Get Premium" email. The hero's steps move with CSS only.

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
