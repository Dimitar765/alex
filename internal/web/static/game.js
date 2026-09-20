// Presentation layer: keyboard play, card play-out animation, and
// synthesized sound effects. Progressive enhancement throughout — the
// game is fully playable without this script.
(function () {
  "use strict";
  document.body.classList.add("js");

  // --- Keyboard play -----------------------------------------------------
  document.addEventListener("keydown", function (e) {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key < "1" || e.key > "9") return;
    var buttons = document.querySelectorAll("#scene .choices button:not([disabled])");
    var btn = buttons[Number(e.key) - 1];
    if (btn) {
      e.preventDefault();
      btn.click();
    }
  });

  // --- Card play-out: hold the request while the card lifts away --------
  document.body.addEventListener("htmx:confirm", function (e) {
    var elt = e.detail.elt;
    var card = elt && elt.closest ? elt.closest(".card") : null;
    if (card && !card.classList.contains("card-played")) {
      e.preventDefault();
      card.classList.add("card-played");
      sfx.play("card");
      setTimeout(function () {
        e.detail.issueRequest(true);
      }, 170);
    }
  });

  // --- Sound effects: a tiny WebAudio synth, no audio files --------------
  var ctx = null;
  var muted = false;
  try {
    muted = localStorage.getItem("sfx-muted") === "1";
  } catch (e) { /* storage unavailable */ }

  function audio() {
    if (!ctx) {
      var AC = window.AudioContext || window.webkitAudioContext;
      if (!AC) return null;
      ctx = new AC();
    }
    if (ctx.state === "suspended") ctx.resume();
    return ctx;
  }

  function tone(freq, dur, opts) {
    var c = audio();
    if (!c) return;
    opts = opts || {};
    var osc = c.createOscillator();
    var g = c.createGain();
    osc.type = opts.type || "sine";
    osc.frequency.setValueAtTime(freq, c.currentTime);
    if (opts.slideTo) {
      osc.frequency.exponentialRampToValueAtTime(opts.slideTo, c.currentTime + dur);
    }
    g.gain.setValueAtTime(opts.gain || 0.1, c.currentTime);
    g.gain.exponentialRampToValueAtTime(0.0001, c.currentTime + dur);
    osc.connect(g).connect(c.destination);
    osc.start();
    osc.stop(c.currentTime + dur + 0.02);
  }

  function noise(dur, freq) {
    var c = audio();
    if (!c) return;
    var len = Math.max(1, Math.floor(dur * c.sampleRate));
    var buf = c.createBuffer(1, len, c.sampleRate);
    var data = buf.getChannelData(0);
    for (var i = 0; i < len; i++) {
      data[i] = (Math.random() * 2 - 1) * (1 - i / len);
    }
    var src = c.createBufferSource();
    src.buffer = buf;
    var bp = c.createBiquadFilter();
    bp.type = "bandpass";
    bp.frequency.value = freq || 1600;
    var g = c.createGain();
    g.gain.value = 0.16;
    src.connect(bp).connect(g).connect(c.destination);
    src.start();
  }

  window.sfx = {
    play: function (name) {
      if (muted) return;
      switch (name) {
        case "card":
          noise(0.09, 1600);
          tone(196, 0.08, { type: "triangle", gain: 0.05 });
          break;
        case "choice":
          tone(392, 0.06, { type: "triangle", gain: 0.06 });
          setTimeout(function () { tone(523, 0.09, { type: "triangle", gain: 0.05 }); }, 55);
          break;
        case "shuffle":
          noise(0.12, 900);
          setTimeout(function () { noise(0.12, 1200); }, 90);
          break;
        case "error":
          tone(147, 0.22, { type: "sawtooth", gain: 0.05, slideTo: 98 });
          break;
        case "victory":
          [523, 659, 784].forEach(function (f, i) {
            setTimeout(function () { tone(f, 0.4, { type: "sine", gain: 0.07 }); }, i * 110);
          });
          break;
        case "defeat":
          tone(196, 0.7, { type: "triangle", gain: 0.07, slideTo: 110 });
          break;
      }
    },
    toggle: function () {
      muted = !muted;
      try {
        localStorage.setItem("sfx-muted", muted ? "1" : "0");
      } catch (e) { /* storage unavailable */ }
      return muted;
    }
  };

  // Choice takes: soft two-note tick; the shuffle gets a double swish.
  document.addEventListener("submit", function (e) {
    var form = e.target;
    if (!form.classList || form.classList.contains("card")) return;
    if (form.action && form.action.indexOf("/game/action") !== -1) {
      sfx.play(form.classList.contains("shuffle-form") ? "shuffle" : "choice");
    }
  });

  // After each swap: error thud, or the ending fanfare/dirge.
  document.body.addEventListener("htmx:afterSwap", function () {
    var log = document.querySelector("#log li");
    if (log && log.textContent.indexOf("Error:") === 0) {
      sfx.play("error");
      return;
    }
    var badge = document.querySelector("#scene .ending-badge");
    if (badge && /defeat|death/.test(badge.className)) {
      sfx.play("defeat");
    } else if (badge) {
      sfx.play("victory");
    }
  });

  // --- Mute toggle --------------------------------------------------------
  var btn = document.getElementById("sfx-toggle");
  if (btn) {
    btn.setAttribute("aria-pressed", String(!muted));
    btn.addEventListener("click", function () {
      btn.setAttribute("aria-pressed", String(!sfx.toggle()));
    });
  }
})();
