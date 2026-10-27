// The logo picker on "create your line" and the dashboard: any picture the
// owner picks opens in a crop window (Cropper.js, assets/vendor), where they
// zoom and drag to frame it in a square. The browser then shrinks it to a
// 512 px WebP (PNG where WebP can't be made; both keep a transparent
// background) and puts that in the form instead, so a 10 MB photo goes up as
// a few KB. Without this script the form still sends the file as it is, and
// the server takes it up to 2 MB.
//
//   <input type="file" data-logo-input>          the file field (data-logo-submit: send the form right away)
//   <img data-logo-preview hidden>               optional, shows the framed logo
//   <p data-logo-error hidden>                   says why a file was refused
//   <dialog data-logo-crop> … </dialog>          the crop window (pages: logoCrop)
(function () {
  var SIZE = 512, MAX_IN = 25 << 20;
  var dlg = document.querySelector("[data-logo-crop]");
  if (!dlg || !window.Cropper || !window.DataTransfer) return;
  var img = dlg.querySelector("[data-logo-crop-img]");
  var cropper = null, input = null, done = false, url = "";

  document.querySelectorAll("[data-logo-input]").forEach(function (el) {
    el.addEventListener("change", function () {
      var f = el.files[0];
      if (!f) return;
      if (!/^image\//.test(f.type) || f.size > MAX_IN) {
        refuse(el, f.size > MAX_IN ? "That picture is too big. Pick one under 25 MB." : "That file isn't a picture. Pick a PNG, JPG or WebP.");
        return;
      }
      open(el, f);
    });
  });

  function errorBox(el) { return el.form && el.form.querySelector("[data-logo-error]"); }

  function refuse(el, text) {
    el.value = "";
    var e = errorBox(el);
    if (e) { e.textContent = text; e.hidden = false; }
    var p = el.form && el.form.querySelector("[data-logo-preview]");
    if (p) p.hidden = true;
  }

  function open(el, file) {
    input = el;
    done = false;
    var e = errorBox(el);
    if (e) e.hidden = true;
    if (url) URL.revokeObjectURL(url);
    url = URL.createObjectURL(file); // freed when the window closes: Cropper reads it again
    img.onerror = function () {
      dlg.close();
      refuse(el, "That picture couldn't be opened. Try a PNG, JPG or WebP.");
    };
    img.onload = function () {
      img.onload = null;
      if (cropper) cropper.destroy();
      cropper = new Cropper(img, {
        aspectRatio: 1, viewMode: 0, dragMode: "move", autoCropArea: 0.9,
        cropBoxMovable: false, cropBoxResizable: false, toggleDragModeOnDblclick: false,
        guides: false, center: false, highlight: false, restore: false,
        ready: fit,
      });
    };
    if (!dlg.open) dlg.showModal();
    img.src = url;
  }

  // fit shows the whole picture inside the square, so a wide logo keeps
  // its ends (the space around it stays transparent).
  function fit() {
    var c = cropper.getContainerData(), side = Math.round(Math.min(c.width, c.height) * 0.86);
    cropper.setCropBoxData({ left: (c.width - side) / 2, top: (c.height - side) / 2, width: side, height: side });
    var d = cropper.getImageData(), k = side / Math.max(d.naturalWidth, d.naturalHeight);
    var w = d.naturalWidth * k, h = d.naturalHeight * k;
    cropper.setCanvasData({ left: (c.width - w) / 2, top: (c.height - h) / 2, width: w, height: h });
  }

  function use() {
    var canvas = cropper.getCroppedCanvas({ width: SIZE, height: SIZE, imageSmoothingEnabled: true, imageSmoothingQuality: "high" });
    canvas.toBlob(function (blob) {
      if (!blob) return refuse(input, "That picture couldn't be used. Try another one.");
      var file = new File([blob], blob.type === "image/webp" ? "logo.webp" : "logo.png", { type: blob.type });
      var dt = new DataTransfer();
      dt.items.add(file);
      input.files = dt.files; // set from a script: no new "change"
      done = true;
      var p = input.form.querySelector("[data-logo-preview]");
      if (p) { p.src = canvas.toDataURL(); p.hidden = false; }
      dlg.close();
      if (input.hasAttribute("data-logo-submit")) input.form.requestSubmit();
    }, "image/webp", 0.9);
  }

  dlg.addEventListener("click", function (e) {
    var b = e.target.closest("[data-crop]");
    if (!b || !cropper) return;
    var a = b.dataset.crop;
    if (a === "in") cropper.zoom(0.1);
    else if (a === "out") cropper.zoom(-0.1);
    else if (a === "fit") fit();
    else if (a === "use") use();
    else if (a === "cancel") { input.value = ""; dlg.close(); }
  });
  // Cancel, Escape or a click outside: nothing chosen.
  dlg.addEventListener("close", function () {
    if (!done && input) input.value = "";
    if (cropper) { cropper.destroy(); cropper = null; }
    if (url) { URL.revokeObjectURL(url); url = ""; }
  });
  dlg.addEventListener("click", function (e) { if (e.target === dlg) dlg.close(); });
})();
