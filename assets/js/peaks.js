// A recording's real loudness as n bars from 0 to 1, read once per recording;
// the result's .seconds is its length. Used by the phone app's message player
// and the "What callers hear" players.
(function () {
  var waves = {};
  window.minivoipPeaks = function (url, n) {
    var key = url + ":" + n;
    if (!waves[key]) waves[key] = fetch(url).then(function (r) { return r.arrayBuffer(); }).then(function (buf) {
      var C = window.OfflineAudioContext || window.webkitOfflineAudioContext;
      return new Promise(function (ok, fail) { new C(1, 2, 44100).decodeAudioData(buf, ok, fail); });
    }).then(function (audio) {
      var data = audio.getChannelData(0), step = Math.max(1, Math.floor(data.length / n)), out = [], top = 0;
      for (var i = 0; i < n; i++) {
        var m = 0;
        for (var j = i * step, end = Math.min(data.length, j + step); j < end; j += 4) m = Math.max(m, Math.abs(data[j]));
        out.push(m);
        top = Math.max(top, m);
      }
      var bars = out.map(function (m) { return top ? m / top : 0; });
      bars.seconds = audio.duration;
      return bars;
    });
    return waves[key];
  };
})();
