// fx.js — presentation effects: a hand-rolled canvas particle layer,
// stat delta floats, scene micro-shakes, shuffle ghost cards, and ending
// ambience. Subtle by design (prestige, not arcade). Everything is gated
// on the FX toggle and prefers-reduced-motion; the game plays fine
// without any of it.
(function () {
  "use strict";

  var reduced = false;
  try {
    reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  } catch (e) { /* matchMedia unavailable */ }
  var enabled = !reduced;
  try {
    enabled = !reduced && localStorage.getItem("fx-off") !== "1";
  } catch (e) { /* storage unavailable */ }

  function ok() { return enabled && !reduced; }

  // --- canvas particle layer --------------------------------------------
  var canvas = document.createElement("canvas");
  canvas.className = "fx-canvas";
  var ctx = canvas.getContext("2d");
  document.body.appendChild(canvas);

  function resize() {
    var dpr = window.devicePixelRatio || 1;
    canvas.width = window.innerWidth * dpr;
    canvas.height = window.innerHeight * dpr;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  }
  window.addEventListener("resize", resize);
  resize();

  var GOLD = "201,162,39";
  var EMBER = "192,91,78";
  var DUST = "154,143,125";
  var particles = [];
  var running = false;
  var last = 0;

  function rand(spread) { return (Math.random() - 0.5) * 2 * (spread || 0); }

  function spawn(x, y, o) {
    particles.push({
      x: x, y: y,
      vx: (o.vx || 0) + rand(o.spread),
      vy: (o.vy || 0) + rand(o.spread),
      ttl: o.ttl || 900,
      age: 0,
      size: o.size || 2.2,
      color: o.color || GOLD
    });
    if (!running) {
      running = true;
      requestAnimationFrame(tick);
    }
  }

  function tick(ts) {
    var dt = Math.min(48, (ts - last) || 16);
    last = ts;
    ctx.clearRect(0, 0, window.innerWidth, window.innerHeight);
    for (var i = particles.length - 1; i >= 0; i--) {
      var p = particles[i];
      p.age += dt;
      if (p.age >= p.ttl) {
        particles.splice(i, 1);
        continue;
      }
      p.x += p.vx * dt / 16;
      p.y += p.vy * dt / 16;
      var k = 1 - p.age / p.ttl;
      ctx.globalAlpha = k * 0.75;
      ctx.fillStyle = "rgb(" + p.color + ")";
      ctx.beginPath();
      ctx.arc(p.x, p.y, p.size * (0.5 + k * 0.5), 0, 6.2832);
      ctx.fill();
    }
    ctx.globalAlpha = 1;
    if (particles.length) {
      requestAnimationFrame(tick);
    } else {
      running = false;
      last = 0;
    }
  }

  // --- bursts -------------------------------------------------------------
  function rect(el) { return el.getBoundingClientRect(); }

  function dust(at, n) {
    for (var i = 0; i < (n || 9); i++) {
      spawn(at.x + Math.random() * at.width,
            at.y + Math.random() * at.height * 0.5, {
        vy: -0.25, spread: 0.35,
        ttl: 700 + Math.random() * 500,
        size: 1.3 + Math.random() * 1.2,
        color: DUST
      });
    }
  }

  function goldRise(at, n) {
    for (var i = 0; i < (n || 7); i++) {
      spawn(at.x + Math.random() * at.width, at.y + at.height, {
        vy: -0.35, spread: 0.12,
        ttl: 2200 + Math.random() * 1400,
        size: 1.5 + Math.random() * 1.6
      });
    }
  }

  function embers(at, n) {
    for (var i = 0; i < (n || 7); i++) {
      spawn(at.x + Math.random() * at.width, at.y, {
        vy: 0.3, spread: 0.1,
        ttl: 2600 + Math.random() * 1600,
        size: 1.3 + Math.random() * 1.4,
        color: EMBER
      });
    }
  }

  function trail(fromEl) {
    var a = rect(fromEl);
    var scene = document.getElementById("scene");
    if (!scene) { return; }
    var b = rect(scene);
    var tx = b.left + b.width / 2;
    var ty = b.top + b.height * 0.3;
    for (var i = 0; i < 6; i++) {
      (function (i) {
        setTimeout(function () {
          var x = a.left + a.width / 2 + rand(18);
          var y = a.top + 10 + rand(10);
          spawn(x, y, { vx: (tx - x) / 26, vy: (ty - y) / 26, ttl: 620, size: 1.8, spread: 0.1 });
        }, i * 28);
      })(i);
    }
  }

  // --- shuffle ghost cards -------------------------------------------------
  function shuffleGhosts(btn) {
    var a = rect(btn);
    for (var i = 0; i < 4; i++) {
      var g = document.createElement("div");
      g.className = "fx-ghost";
      g.style.left = (a.left + a.width / 2 - 18) + "px";
      g.style.top = (a.top - 8) + "px";
      document.body.appendChild(g);
      var ang = (-50 - Math.random() * 30) * Math.PI / 180;
      var dist = 34 + Math.random() * 26;
      void g.offsetWidth; // force layout before the transition
      g.style.transform = "translate(" + (Math.cos(ang) * dist) + "px," +
                          (Math.sin(ang) * dist) + "px) rotate(" +
                          (-24 + Math.random() * 48) + "deg)";
      g.style.opacity = "0";
      setTimeout(function (el) { return function () { el.remove(); }; }(g), 640);
    }
    dust({ x: a.left, y: a.top, width: a.width, height: a.height }, 7);
  }

  // --- stat delta floats -----------------------------------------------------
  function readStats() {
    var scene = document.getElementById("scene");
    if (!scene) { return null; }
    var spans = scene.querySelectorAll(".stats span");
    var vals = [];
    for (var i = 0; i < spans.length; i++) {
      var m = spans[i].textContent.match(/-?\d+/);
      vals.push({ el: spans[i], v: m ? parseInt(m, 10) : 0 });
    }
    return vals;
  }

  function floatDeltas(oldVals, newVals) {
    if (!oldVals || !newVals || oldVals.length !== newVals.length) { return; }
    for (var i = 0; i < newVals.length; i++) {
      var d = newVals[i].v - oldVals[i].v;
      if (!d) { continue; }
      var r = newVals[i].el.getBoundingClientRect();
      var f = document.createElement("span");
      f.className = "stat-float " + (d > 0 ? "up" : "down");
      f.textContent = (d > 0 ? "+" : "\u2212") + Math.abs(d);
      f.style.left = (r.left + r.width / 2 - 10) + "px";
      f.style.top = (r.top - 4) + "px";
      document.body.appendChild(f);
      setTimeout(function (el) { return function () { el.remove(); }; }(f), 950);
    }
  }

  // --- shakes and ambience ---------------------------------------------------
  function shake(el, cls) {
    el.classList.remove(cls);
    void el.offsetWidth;
    el.classList.add(cls);
    el.addEventListener("animationend", function h() {
      el.classList.remove(cls);
      el.removeEventListener("animationend", h);
    });
  }

  function ambience(kind) {
    var scene = document.getElementById("scene");
    if (!scene) { return; }
    var at = rect(scene);
    for (var w = 0; w < 5; w++) {
      (function (w) {
        setTimeout(function () {
          if (kind === "defeat") { embers(at, 6); } else { goldRise(at, 6); }
        }, w * 450);
      })(w);
    }
  }

  function vignette(kind) {
    var v = document.createElement("div");
    v.className = "fx-vignette " + kind;
    document.body.appendChild(v);
  }

  function endingTone(badge) {
    var dark = /defeat|death/.test(badge.className);
    ambience(dark ? "defeat" : "victory");
    vignette(dark ? "dark" : "gold");
  }

  // --- wiring ------------------------------------------------------------------
  var prevStats = null;
  var lastKind = "choice";

  document.addEventListener("submit", function (e) {
    var f = e.target;
    if (!f.classList) { return; }
    if (f.classList.contains("card")) {
      lastKind = "card";
    } else if (f.classList.contains("shuffle-form")) {
      lastKind = "shuffle";
      if (ok()) { shuffleGhosts(f.querySelector("button")); }
    } else if (f.classList.contains("scout-form")) {
      lastKind = "scout";
    } else {
      lastKind = "choice";
    }
  }, true);

  document.body.addEventListener("htmx:confirm", function (e) {
    var elt = e.detail.elt;
    var card = elt && elt.closest ? elt.closest(".card") : null;
    if (card && ok()) { trail(card); }
  });

  document.body.addEventListener("htmx:beforeSwap", function () {
    prevStats = readStats();
  });

  document.body.addEventListener("htmx:afterSwap", function () {
    var scene = document.getElementById("scene");
    if (!scene) { return; }
    if (ok()) {
      floatDeltas(prevStats, readStats());
      var badge = scene.querySelector(".ending-badge");
      if (badge) {
        endingTone(badge);
        return;
      }
      if (scene.hasAttribute("data-fate")) {
        shake(scene, "fate-shake");
        dust(rect(scene), 10);
      } else if (lastKind === "choice") {
        shake(scene, "shake");
      }
    }
    prevStats = null;
  });

  // Full-page renders (refresh, no-JS transitions back to JS) get their
  // ending ambience once on load.
  var badge0 = document.querySelector("#scene .ending-badge");
  if (badge0 && ok()) { endingTone(badge0); }

  // --- FX toggle -----------------------------------------------------------------
  var btn = document.getElementById("fx-toggle");
  function paint() {
    if (btn) { btn.setAttribute("aria-pressed", String(enabled)); }
  }
  if (btn) {
    paint();
    btn.addEventListener("click", function () {
      enabled = !enabled;
      try {
        localStorage.setItem("fx-off", enabled ? "0" : "1");
      } catch (e) { /* storage unavailable */ }
      paint();
    });
  }
})();
