// The recorder for Premium notes: runs in the browser that holds the call, and
// sends what it records up in pieces, so a crash loses little. Nothing here may
// disturb the call: whatever goes wrong, it stops quietly and says so.
//
//   var r = MinivoipRecorder.start({ url: "/app/calls/<id>/recording", voices: { owner: stream, caller: stream }, onState: fn });
//   r.add("guest1", stream); r.remove("guest1"); r.stop();
//
// A call's two voices each get a sound channel of their own, so the transcript
// can tell who is who. A meeting (mixed: true) puts everyone on one, joining and
// leaving as they please. The server answers the first post with whether to record
// at all, and maybe another url; pieces go to url/0, url/1 and so on, and url/done
// says the last one is in. onState gets "on", "stopped", or the server's reason
// for not recording; MinivoipRecorder.show puts that in words.
//
// A locked phone or a hidden page can lose sound: the microphone goes quiet, the
// sound engine stops, or the page freezes. mine names the voice to watch, and each
// post says how many seconds it reckons were missed, for the note to say so.
(function () {
  var SLICE = 30000;     // ms of sound in a piece
  var WAITING = 20;      // pieces held back while the network is down, then it gives up
  var RETRY = 4000;      // ms between tries of a piece
  var BITRATE = 64000;   // plenty for two voices
  var FROZEN = 5000;     // ms between ticks that mean the page was frozen

  function bestType() {
    if (!window.MediaRecorder || !MediaRecorder.isTypeSupported) return "";
    return ["audio/webm;codecs=opus", "audio/mp4", "audio/ogg;codecs=opus", "audio/webm"].filter(function (t) {
      return MediaRecorder.isTypeSupported(t);
    })[0] || "";
  }

  function start(o) {
    var type = bestType(), slice = window.MinivoipRecorderSlice || SLICE;
    var base = o.url, voices = o.voices || {}, sources = {};
    var ac, mix, rec, part = 0, queue = [], sending = false, over = false, dead = false, begun = false, retryTimer;
    var missed = 0, troubleSince = 0, micQuiet = false, lastTick = 0, tickTimer, soundDone = false;

    // ------------------------------------------------- what the recording misses

    function trouble() {
      var on = micQuiet || !!(ac && ac.state !== "running");
      if (on && !troubleSince) troubleSince = Date.now();
      if (!on && troubleSince) { missed += Date.now() - troubleSince; troubleSince = 0; }
    }
    function missedSeconds() {
      return Math.round((missed + (troubleSince ? Date.now() - troubleSince : 0)) / 1000);
    }
    function tick() {
      var now = Date.now();
      if (now - lastTick > FROZEN && !troubleSince) missed += now - lastTick;
      lastTick = now;
    }
    function micQuietIs(on) { return function () { micQuiet = on; trouble(); }; }
    // Back from a locked phone or another app: the sound engine may need a nudge.
    function backAgain() {
      if (ac && !document.hidden && ac.state !== "running" && ac.state !== "closed") ac.resume().catch(function () {});
    }

    function state(s) { try { if (o.onState) o.onState(s); } catch (e) {} }

    // Sound stops here, but what was recorded still goes up.
    function stopSound() {
      clearInterval(tickTimer);
      document.removeEventListener("visibilitychange", backAgain);
      trouble();
      if (rec && rec.state !== "inactive") { try { rec.stop(); } catch (e) {} }
      if (ac) { ac.onstatechange = null; ac.close().catch(function () {}); ac = null; trouble(); }
    }
    function giveUp() {
      if (over) return;
      over = dead = true;
      missed += queue.length * slice;
      queue = [];
      stopSound();
      state("stopped");
      finish();
    }
    // The browser stopped the recorder itself: what's recorded still goes up.
    function ended() {
      if (over) return;
      over = true;
      stopSound();
      state("stopped");
      pump();
    }
    function finish() {
      if (begun) fetch(base + "/done?missed=" + missedSeconds(), { method: "POST" }).catch(function () {});
      begun = false;
    }

    function pump() {
      if (sending) return;
      // The recorder's last piece comes after it's told to stop: "done" waits for it.
      if (!queue.length) { if (over && (soundDone || !rec)) finish(); return; }
      if (queue.length > WAITING) return giveUp();
      sending = true;
      // A login that ran out answers with a redirect, which must not count as sent.
      fetch(base + "/" + part + "?missed=" + missedSeconds(), { method: "POST", body: queue[0], redirect: "manual" }).then(function (r) {
        sending = false;
        if (r.ok) { part++; queue.shift(); return pump(); }
        // The server won't take more: past a limit, or the note is gone.
        if (r.type === "opaqueredirect" || r.status === 403 || r.status === 404 || r.status === 409 || r.status === 413) return giveUp();
        later();
      }, function () {
        sending = false;
        later();
      });
    }
    // The network or the server is having a moment. At the end of a call there's
    // one more try, then what's in is what the note gets.
    function later() {
      if (over) { missed += queue.length * slice; queue = []; return finish(); }
      clearTimeout(retryTimer);
      retryTimer = setTimeout(pump, RETRY);
    }

    // plug connects a voice: a channel of its own on a call, the mix in a meeting.
    function plug(key, i) {
      try {
        var src = ac.createMediaStreamSource(voices[key]);
        src.connect(mix, 0, o.mixed ? 0 : i);
        sources[key] = src;
      } catch (e) {} // one voice missing is no reason to stop
    }
    function unplug(key) {
      try { if (sources[key]) sources[key].disconnect(); } catch (e) {}
      delete sources[key];
      delete voices[key];
    }

    function begin() {
      try {
        if (!type) throw new Error("this browser can't record");
        ac = new (window.AudioContext || window.webkitAudioContext)();
        if (ac.state === "suspended") ac.resume();
        var out = ac.createMediaStreamDestination();
        out.channelCount = o.mixed ? 1 : 2;
        if (o.mixed) mix = out;
        else {
          mix = ac.createChannelMerger(2);
          mix.connect(out);
        }
        Object.keys(voices).forEach(plug);
        rec = new MediaRecorder(out.stream, { mimeType: type, audioBitsPerSecond: BITRATE });
        rec.ondataavailable = function (e) { if (e.data && e.data.size && !dead) { queue.push(e.data); pump(); } };
        rec.onerror = ended;
        rec.onstop = function () { soundDone = true; ended(); pump(); };
        ac.onstatechange = trouble;
        var mine = voices[o.mine];
        if (mine) mine.getAudioTracks().forEach(function (t) {
          t.addEventListener("mute", micQuietIs(true));
          t.addEventListener("unmute", micQuietIs(false));
        });
        document.addEventListener("visibilitychange", backAgain);
        lastTick = Date.now();
        tickTimer = setInterval(tick, 1000);
        rec.start(slice);
        state("on");
      } catch (e) {
        giveUp();
      }
    }

    fetch(o.url + "?type=" + encodeURIComponent(type.split(";")[0]), { method: "POST", redirect: "manual" }).then(function (r) {
      return r.ok ? r.json() : { record: false, reason: "stopped" };
    }).then(function (a) {
      if (over) return;
      if (!a.record) { over = true; return state(a.reason || ""); }
      if (a.url) base = a.url;
      begun = true;
      begin();
    }).catch(function () {
      if (!over) { over = true; state("stopped"); }
    });

    return {
      add: function (key, stream) {
        voices[key] = stream;
        if (ac && !over) plug(key, 0);
      },
      remove: unplug,
      // The call ended: the last piece goes up, then "done".
      stop: function () {
        if (over) return;
        over = true;
        clearTimeout(retryTimer);
        stopSound();
        if (!rec) finish();
        state("");
      },
    };
  }

  // show says in a line under the call what's happening with the recording.
  function show(el, state, what) {
    var not = "so this " + what + " isn't recorded.";
    var words = {
      on: "Recording for your notes",
      not_set_up: "Notes aren't set up yet, " + not,
      no_recorder: "This browser can't record, " + not,
      busy: "Too many recordings are waiting, " + not,
      not_told: "Someone here wasn't told notes are on, " + not,
      month_limit: "This month's 10 hours of notes are used, " + not,
      day_limit: "Too many notes today, " + not,
      stopped: "Recording stopped. Your " + what + " goes on.",
    }[state];
    el.textContent = words || "";
    el.hidden = !words;
    el.classList.toggle("is-on", state === "on");
  }

  window.MinivoipRecorder = { start: start, show: show };
})();
