// The recorder for Premium notes: runs in the browser that holds the call, and
// sends what it records up in pieces, so a crash loses little. Nothing here may
// disturb the call: whatever goes wrong, it stops quietly and says so.
//
//   var r = MinivoipRecorder.start({ url: "/app/calls/<id>/recording", voices: [ownerStream, callerStream], onState: fn });
//   r.stop();
//
// Each voice gets a sound channel of its own, so the transcript can tell who is
// who. The server answers the first post with whether to record at all; pieces
// then go to url/0, url/1 and so on, and url/done says the last one is in.
// onState gets "on", "stopped", or the server's reason for not recording.
(function () {
  var SLICE = 30000;     // ms of sound in a piece
  var WAITING = 20;      // pieces held back while the network is down, then it gives up
  var RETRY = 4000;      // ms between tries of a piece
  var BITRATE = 64000;   // plenty for two voices

  function bestType() {
    if (!window.MediaRecorder || !MediaRecorder.isTypeSupported) return "";
    return ["audio/webm;codecs=opus", "audio/mp4", "audio/ogg;codecs=opus", "audio/webm"].filter(function (t) {
      return MediaRecorder.isTypeSupported(t);
    })[0] || "";
  }

  function start(o) {
    var type = bestType(), slice = window.MinivoipRecorderSlice || SLICE;
    var ac, rec, part = 0, queue = [], sending = false, over = false, dead = false, begun = false, retryTimer;

    function state(s) { try { if (o.onState) o.onState(s); } catch (e) {} }

    // Sound stops here, but what was recorded still goes up.
    function stopSound() {
      if (rec && rec.state !== "inactive") { try { rec.stop(); } catch (e) {} }
      if (ac) { ac.close().catch(function () {}); ac = null; }
    }
    function giveUp() {
      if (over) return;
      over = dead = true;
      queue = [];
      stopSound();
      state("stopped");
      finish();
    }
    function finish() {
      if (begun) fetch(o.url + "/done", { method: "POST" }).catch(function () {});
      begun = false;
    }

    function pump() {
      if (sending) return;
      if (!queue.length) { if (over) finish(); return; }
      if (queue.length > WAITING) return giveUp();
      sending = true;
      fetch(o.url + "/" + part, { method: "POST", body: queue[0] }).then(function (r) {
        sending = false;
        if (r.ok) { part++; queue.shift(); return pump(); }
        // The server won't take more: past a limit, or the note is gone.
        if (r.status === 403 || r.status === 404 || r.status === 409 || r.status === 413) return giveUp();
        later();
      }, function () {
        sending = false;
        later();
      });
    }
    // The network or the server is having a moment. At the end of a call there's
    // one more try, then what's in is what the note gets.
    function later() {
      if (over) { queue = []; return finish(); }
      clearTimeout(retryTimer);
      retryTimer = setTimeout(pump, RETRY);
    }

    function begin() {
      try {
        if (!type) throw new Error("this browser can't record");
        ac = new (window.AudioContext || window.webkitAudioContext)();
        if (ac.state === "suspended") ac.resume();
        var out = ac.createMediaStreamDestination();
        out.channelCount = 2;
        var merge = ac.createChannelMerger(2);
        o.voices.forEach(function (stream, i) { ac.createMediaStreamSource(stream).connect(merge, 0, i); });
        merge.connect(out);
        rec = new MediaRecorder(out.stream, { mimeType: type, audioBitsPerSecond: BITRATE });
        rec.ondataavailable = function (e) { if (e.data && e.data.size && !dead) { queue.push(e.data); pump(); } };
        rec.onerror = giveUp;
        rec.start(slice);
        state("on");
      } catch (e) {
        giveUp();
      }
    }

    fetch(o.url + "?type=" + encodeURIComponent(type.split(";")[0]), { method: "POST" }).then(function (r) {
      return r.ok ? r.json() : { record: false, reason: "stopped" };
    }).then(function (a) {
      if (over) return;
      if (!a.record) { over = true; return state(a.reason || ""); }
      begun = true;
      begin();
    }).catch(function () {
      if (!over) { over = true; state("stopped"); }
    });

    return {
      // The call ended: the last piece goes up, then "done".
      stop: function () {
        if (over) return;
        over = true;
        clearTimeout(retryTimer);
        if (rec && rec.state !== "inactive") rec.addEventListener("stop", function () { pump(); });
        stopSound();
        if (!rec) finish();
        state("");
      },
    };
  }

  window.MinivoipRecorder = { start: start };
})();
