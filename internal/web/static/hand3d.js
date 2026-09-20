// hand3d.js — the 3D card table (three.js). The DOM hand remains the
// source of truth — server-rendered, swapped by htmx, focusable for
// keyboard and gamepad — and this layer mirrors it as WebGL card meshes:
// fanned on a table with a deck stack, hover tilt, deal-ins, play arcs,
// and a shuffle riffle. Without WebGL or JavaScript, the DOM hand simply
// stays visible.
(function () {
  "use strict";
  if (!window.THREE) { return; }

  var probe = document.createElement("canvas");
  var gl = probe.getContext("webgl") || probe.getContext("experimental-webgl");
  if (!gl) { return; }

  var host = document.querySelector(".table-side");
  if (!host || !document.getElementById("hand")) { return; }

  var reduced = false;
  try {
    reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  } catch (e) { /* matchMedia unavailable */ }

  host.classList.add("hand-3d-host");

  // --- renderer, scene, camera -------------------------------------------
  var renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true });
  renderer.setPixelRatio(window.devicePixelRatio || 1);
  var canvas = renderer.domElement;
  canvas.className = "hand-3d-canvas";
  host.insertBefore(canvas, host.firstChild);

  var scene = new THREE.Scene();
  var camera = new THREE.PerspectiveCamera(34, 1, 0.1, 100);
  var camBase = { x: 0, y: 2.4, z: 6.3 };
  camera.position.set(camBase.x, camBase.y, camBase.z);
  camera.lookAt(0, -0.2, 0);

  scene.add(new THREE.AmbientLight(0xfff2dd, 0.75));
  var key = new THREE.DirectionalLight(0xffe8c0, 0.85);
  key.position.set(2.5, 4, 3);
  scene.add(key);

  var table = new THREE.Mesh(
    new THREE.PlaneGeometry(30, 30),
    new THREE.MeshLambertMaterial({ color: 0x14100c })
  );
  table.rotation.x = -Math.PI / 2;
  table.position.y = -1.35;
  scene.add(table);

  // --- card textures -------------------------------------------------------
  var GOLD = "#c9a227";
  var CARD_W = 1.7, CARD_H = 2.38;
  var textureCache = {};
  var emblemCache = {}; // card id -> decoded 320x320 canvas
  var artDoc = null;

  function emblemURI(id) {
    if (!artDoc) { return null; }
    var sym = artDoc.getElementById("art_" + id);
    if (!sym) { return null; }
    var svg = '<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64" viewBox="0 0 64 64">' +
      sym.innerHTML
        .replace(/currentColor/g, GOLD)
        .replace(/stroke-width="2.5"/g, 'stroke-width="4"') + "</svg>";
    return "data:image/svg+xml;charset=utf-8," + encodeURIComponent(svg);
  }

  // loadEmblem decodes the emblem once so paintFront can draw it
  // synchronously from the cache; cb fires when repainting is safe.
  function loadEmblem(id, cb) {
    if (emblemCache[id]) { cb(); return; }
    var uri = emblemURI(id);
    if (!uri) { cb(); return; }
    var img = new Image();
    img.onload = function () {
      var c = document.createElement("canvas");
      c.width = 320; c.height = 320;
      c.getContext("2d").drawImage(img, 0, 0, 320, 320);
      emblemCache[id] = c;
      cb();
    };
    img.onerror = cb;
    img.src = uri;
  }

  function loadEmblems(list, done) {
    fetch("/static/art.svg")
      .then(function (r) { return r.text(); })
      .then(function (text) {
        artDoc = new DOMParser().parseFromString(text, "image/svg+xml");
        var pending = list.length;
        if (!pending) { done(); return; }
        list.forEach(function (id) {
          loadEmblem(id, function () {
            if (--pending === 0) { done(); }
          });
        });
      })
      .catch(done);
  }

  function roundedCard(ctx, x, y, w, h, r) {
    ctx.beginPath();
    ctx.moveTo(x + r, y);
    ctx.arcTo(x + w, y, x + w, y + h, r);
    ctx.arcTo(x + w, y + h, x, y + h, r);
    ctx.arcTo(x, y + h, x, y, r);
    ctx.arcTo(x, y, x + w, y, r);
    ctx.closePath();
  }

  function paintFront(card) {
    var W = 512, H = 716;
    var c = document.createElement("canvas");
    c.width = W; c.height = H;
    var ctx = c.getContext("2d");
    var g = ctx.createLinearGradient(0, 0, 0, H);
    g.addColorStop(0, "#282219");
    g.addColorStop(1, "#191410");
    ctx.fillStyle = g;
    roundedCard(ctx, 0, 0, W, H, 28);
    ctx.fill();
    ctx.strokeStyle = "#4a3f2e";
    ctx.lineWidth = 6;
    ctx.stroke();
    ctx.strokeStyle = "rgba(201,162,39,0.55)";
    ctx.lineWidth = 2;
    roundedCard(ctx, 16, 16, W - 32, H - 32, 18);
    ctx.stroke();

    // Art panel
    ctx.save();
    roundedCard(ctx, 36, 36, W - 72, 400, 16);
    ctx.clip();
    var pg = ctx.createRadialGradient(W / 2, 60, 30, W / 2, 240, 460);
    pg.addColorStop(0, "#241f18");
    pg.addColorStop(1, "#120e0a");
    ctx.fillStyle = pg;
    ctx.fillRect(36, 36, W - 72, 400);
    if (emblemCache[card.id]) {
      ctx.globalAlpha = 0.95;
      ctx.drawImage(emblemCache[card.id], 86, 56, 340, 340);
      ctx.globalAlpha = 1;
    }

    ctx.restore();

    // Name
    ctx.fillStyle = GOLD;
    ctx.font = "600 44px Georgia, serif";
    ctx.textAlign = "center";
    var size = 44;
    while (ctx.measureText(card.name).width > W - 90 && size > 22) {
      size -= 2;
      ctx.font = "600 " + size + "px Georgia, serif";
    }
    ctx.fillText(card.name, W / 2, 540);

    // Flavor
    ctx.fillStyle = "#9a8f7d";
    ctx.font = "italic 26px Georgia, serif";
    wrapText(ctx, card.text, W / 2, 596, W - 110, 34);

    // Cost chip
    if (card.cost > 0) {
      ctx.beginPath();
      ctx.arc(W - 78, 78, 44, 0, 6.2832);
      ctx.strokeStyle = GOLD;
      ctx.lineWidth = 4;
      ctx.stroke();
      ctx.fillStyle = GOLD;
      ctx.font = "600 48px ui-monospace, Menlo, monospace";
      ctx.fillText(String(card.cost), W - 78, 95);
    }
    return c;
  }

  function paintBack() {
    var W = 512, H = 716;
    var c = document.createElement("canvas");
    c.width = W; c.height = H;
    var ctx = c.getContext("2d");
    var g = ctx.createLinearGradient(0, 0, 0, H);
    g.addColorStop(0, "#221c15");
    g.addColorStop(1, "#151009");
    ctx.fillStyle = g;
    roundedCard(ctx, 0, 0, W, H, 28);
    ctx.fill();
    ctx.strokeStyle = "#4a3f2e";
    ctx.lineWidth = 6;
    ctx.stroke();
    // Meander suggestion: interlocking key pattern border.
    ctx.strokeStyle = "rgba(201,162,39,0.4)";
    ctx.lineWidth = 3;
    var step = 42;
    for (var x = 30; x < W - 30; x += step) {
      ctx.strokeRect(x, 30, step / 2, step / 2);
      ctx.strokeRect(x, H - 30 - step / 2, step / 2, step / 2);
    }
    for (var y = 72; y < H - 72; y += step) {
      ctx.strokeRect(30, y, step / 2, step / 2);
      ctx.strokeRect(W - 30 - step / 2, y, step / 2, step / 2);
    }
    ctx.fillStyle = "rgba(201,162,39,0.85)";
    ctx.font = "600 300px Georgia, serif";
    ctx.textAlign = "center";
    ctx.fillText("\u0391", W / 2, H / 2 + 105);
    return c;
  }

  function wrapText(ctx, text, cx, y, maxW, lineH) {
    var words = String(text).split(" ");
    var line = "";
    var yy = y;
    for (var i = 0; i < words.length; i++) {
      var test = line ? line + " " + words[i] : words[i];
      if (ctx.measureText(test).width > maxW && line) {
        ctx.fillText(line, cx, yy);
        line = words[i];
        yy += lineH;
        if (yy > 660) { break; }
      } else {
        line = test;
      }
    }
    if (line && yy <= 660) {
      ctx.fillText(line, cx, yy);
    }
  }

  function tex(canvas) {
    var t = new THREE.CanvasTexture(canvas);
    t.anisotropy = 4;
    t.encoding = THREE.sRGBEncoding;
    return t;
  }

  var backMaterial;
  var edgeMaterial;

  function cardMaterials(card, mesh) {
    if (!textureCache[card.id]) {
      textureCache[card.id] = tex(paintFront(card));
      if (!emblemCache[card.id]) {
        loadEmblem(card.id, function () {
          textureCache[card.id] = tex(paintFront(card));
          if (mesh && Array.isArray(mesh.material)) {
            mesh.material[4].map = textureCache[card.id];
            mesh.material[4].needsUpdate = true;
          }
        });
      }
    }
    return [
      edgeMaterial, edgeMaterial, edgeMaterial, edgeMaterial,
      new THREE.MeshLambertMaterial({ map: textureCache[card.id] }),
      backMaterial
    ];
  }

  // --- meshes ---------------------------------------------------------------
  var geometry = new THREE.BoxGeometry(CARD_W, CARD_H, 0.04);

  function makeCard(card) {
    var m = new THREE.Mesh(geometry);
    m.material = cardMaterials(card, m);
    m.userData = card;
    scene.add(m);
    return m;
  }

  // Deck stack (visual only; counts come from data-deck).
  var deckGroup = new THREE.Group();
  deckGroup.position.set(2.55, 0.6, -1.7);
  deckGroup.rotation.x = -0.18;
  scene.add(deckGroup);
  var deckCards = [];

  function syncDeck(count) {
    var n = Math.max(0, Math.min(count, 8));
    while (deckCards.length < n) {
      var d = new THREE.Mesh(geometry, backMaterial);
      d.scale.setScalar(0.82);
      var i = deckCards.length;
      d.position.set((i % 2) * 0.02, i * 0.045, ((i % 3) - 1) * 0.015);
      d.rotation.y = (Math.random() - 0.5) * 0.08;
      deckGroup.add(d);
      deckCards.push(d);
    }
    while (deckCards.length > n) {
      deckGroup.remove(deckCards.pop());
    }
  }

  // --- tween engine -----------------------------------------------------------
  var tweens = [];
  function getter(obj, prop, axis) {
    if (prop === "rotation") { return obj.rotation[axis || "z"]; }
    if (prop === "x" || prop === "y" || prop === "z") { return obj.position[prop]; }
    return obj[prop];
  }
  function setter(obj, prop, v, axis) {
    if (prop === "rotation") { obj.rotation[axis || "z"] = v; return; }
    if (prop === "x" || prop === "y" || prop === "z") { obj.position[prop] = v; return; }
    obj[prop] = v;
  }
  function tween(obj, to, dur, opts) {
    opts = opts || {};
    var from = {};
    for (var k in to) {
      from[k] = getter(obj, k, opts.axis);
    }
    tweens.push({
      obj: obj, from: from, to: to, axis: opts.axis,
      t: -(opts.delay || 0), dur: reduced ? 1 : dur,
      ease: opts.ease || easeOut,
      onDone: opts.onDone
    });
  }
  function easeOut(t) { return 1 - Math.pow(1 - t, 3); }

  function stepTweens(dt) {
    for (var i = tweens.length - 1; i >= 0; i--) {
      var tw = tweens[i];
      tw.t += dt;
      var k = tw.t <= 0 ? 0 : tw.ease(Math.min(1, tw.t / tw.dur));
      for (var p in tw.to) {
        var v = tw.from[p] + (tw.to[p] - tw.from[p]) * k;
        if (p === "opacity") {
          var mats = Array.isArray(tw.obj.material) ? tw.obj.material : [tw.obj.material];
          mats.forEach(setOpacity(v));
        } else {
          setter(tw.obj, p, v, tw.axis);
        }
      }
      if (tw.t >= tw.dur) {
        tweens.splice(i, 1);
        if (tw.onDone) { tw.onDone(); }
      }
    }
  }
  function setOpacity(v) {
    return function (mtl) {
      mtl.transparent = true;
      mtl.opacity = v;
    };
  }
  function killTweens(obj) {
    for (var i = tweens.length - 1; i >= 0; i--) {
      if (tweens[i].obj === obj) { tweens.splice(i, 1); }
    }
  }

  // --- hand layout and sync ---------------------------------------------------
  var meshes = []; // ordered like the DOM hand

  function fanSlot(i, n) {
    var t = n === 1 ? 0 : (i - (n - 1) / 2);
    return {
      x: t * 1.3,
      y: -Math.abs(t) * 0.16 - 0.1,
      z: 0.6 - Math.abs(t) * 0.1,
      rz: -t * 0.085
    };
  }

  function relayout() {
    var n = meshes.length;
    meshes.forEach(function (m, i) {
      var s = fanSlot(i, n);
      killTweens(m);
      tween(m, { x: s.x, y: s.y, z: s.z }, 0.3);
      tween(m, { rotation: s.rz }, 0.3, { axis: "z" });
    });
  }

  function allMaterials(m) {
    return Array.isArray(m.material) ? m.material : [m.material];
  }

  function removeMesh(m, played) {
    killTweens(m);
    var target = played
      ? { x: 2.6, y: 1.6, z: 3.2 }
      : { x: m.position.x, y: -2.6, z: 0 };
    tween(m, target, 0.34);
    if (played) {
      tween(m, { rotation: -0.9 }, 0.34, { axis: "x" });
    }
    tween(m, { opacity: 0 }, 0.3, {
      onDone: function () { scene.remove(m); }
    });
  }

  function readDOM() {
    var section = document.getElementById("hand");
    var out = [];
    if (!section) { return out; }
    var forms = section.querySelectorAll("form.card");
    for (var i = 0; i < forms.length; i++) {
      var btn = forms[i].querySelector("button");
      var nameEl = forms[i].querySelector(".card-name");
      out.push({
        id: forms[i].querySelector('input[name="card"]').value,
        name: nameEl ? nameEl.textContent.trim().split(" \u00b7")[0] : "?",
        text: (forms[i].querySelector(".card-text") || {}).textContent || "",
        cost: btn && btn.disabled ? -1 : (Number((forms[i].querySelector(".card-cost") || {}).textContent) || 0),
        playable: !!(btn && !btn.disabled),
        form: forms[i]
      });
    }
    return out;
  }

  var pendingPlay = null;

  function sync() {
    var dom = readDOM();
    var byId = {};
    dom.forEach(function (c) { byId[c.id] = c; });

    // Remove meshes whose cards left the hand.
    var kept = [];
    meshes.forEach(function (m) {
      if (byId[m.userData.id]) {
        m.userData = byId[m.userData.id]; // refresh playability
        kept.push(m);
      } else {
        removeMesh(m, m.userData.id === pendingPlay);
      }
    });
    meshes = kept;
    pendingPlay = null;

    // Deal new cards in from the deck stack.
    var toAdd = dom.filter(function (c) {
      return !meshes.some(function (m) { return m.userData.id === c.id; });
    });
    toAdd.forEach(function (c, i) {
      var m = makeCard(c);
      m.position.set(deckGroup.position.x - 0.4, 0.2 + i * 0.05, -1.2);
      m.rotation.set(-0.3, -0.45, 0.1);
      meshes.push(m);
      // Deal: staggered flight from the deck into the fan.
      var slot = fanSlot(meshes.indexOf(m), dom.length);
      tween(m, { x: slot.x, y: slot.y, z: slot.z }, 0.42, { delay: 0.07 * i });
      tween(m, { rotation: 0, }, 0.42, { axis: "x", delay: 0.07 * i });
      tween(m, { rotation: 0 }, 0.42, { axis: "y", delay: 0.07 * i });
      tween(m, { rotation: slot.rz }, 0.42, { axis: "z", delay: 0.07 * i });
    });

    // Restore order to match the DOM; settled cards re-fan, freshly
    // dealt ones are already flying to their slots.
    meshes.sort(function (a, b) {
      var ia = dom.findIndex(function (c) { return c.id === a.userData.id; });
      var ib = dom.findIndex(function (c) { return c.id === b.userData.id; });
      return ia - ib;
    });
    var dealt = {};
    toAdd.forEach(function (c) { dealt[c.id] = true; });
    meshes.forEach(function (m, i) {
      if (dealt[m.userData.id]) { return; }
      var s = fanSlot(i, meshes.length);
      killTweens(m);
      tween(m, { x: s.x, y: s.y, z: s.z }, 0.3);
      tween(m, { rotation: s.rz }, 0.3, { axis: "z" });
    });

    var section = document.getElementById("hand");
    syncDeck(section ? Number(section.getAttribute("data-deck") || 0) : 0);
    applyTint();
  }

  // Unaffordable cards sit darker; focus mirrors from the DOM buttons.
  function applyTint(focusedId) {
    meshes.forEach(function (m) {
      var target = focusedId && m.userData.id === focusedId ? 1.22 : (m.userData.playable ? 1.0 : 0.62);
      m.scale.setScalar(0.96 * target);
    });
  }

  // --- interaction ---------------------------------------------------------
  var ray = new THREE.Raycaster();
  var pointer = new THREE.Vector2(0, 0);
  var hovered = null;

  function pick(ev) {
    var r = canvas.getBoundingClientRect();
    pointer.x = ((ev.clientX - r.left) / r.width) * 2 - 1;
    pointer.y = -((ev.clientY - r.top) / r.height) * 2 + 1;
    ray.setFromCamera(pointer, camera);
    var hits = ray.intersectObjects(meshes);
    return hits.length ? hits[0].object : null;
  }

  canvas.addEventListener("pointermove", function (ev) {
    var m = pick(ev);
    if (m !== hovered) {
      if (hovered && meshes.indexOf(hovered) >= 0) {
        var s = fanSlot(meshes.indexOf(hovered), meshes.length);
        tween(hovered, { y: s.y, z: s.z }, 0.2);
      }
      hovered = m;
      if (hovered && hovered.userData.playable) {
        tween(hovered, { y: 0.55, z: 1.55 }, 0.2);
        canvas.style.cursor = "pointer";
      } else {
        canvas.style.cursor = "";
      }
      applyTint();
    }
    // Subtle camera parallax.
    camera.position.x = camBase.x + pointer.x * 0.22;
    camera.position.y = camBase.y + pointer.y * 0.12;
    camera.lookAt(0, -0.2, 0);
  });

  canvas.addEventListener("pointerdown", function (ev) {
    var m = pick(ev);
    if (!m) { return; }
    if (!m.userData.playable) {
      // Refused: a small head-shake.
      killTweens(m);
      tween(m, { x: m.position.x - 0.14 }, 0.07, {
        onDone: function () {
          tween(m, { x: m.position.x }, 0.07, {
            onDone: function () { relayout(false); }
          });
        }
      });
      return;
    }
    pendingPlay = m.userData.id;
    meshes = meshes.filter(function (x) { return x !== m; });
    removeMesh(m, true);
    relayout(false);
    if (m.userData.form) {
      m.userData.form.requestSubmit();
    }
  });

  // Mirror DOM focus (keyboard / gamepad) onto the meshes.
  document.addEventListener("focusin", function (ev) {
    var form = ev.target.closest ? ev.target.closest("form.card") : null;
    if (!form) { return; }
    var id = form.querySelector('input[name="card"]');
    applyTint(id && id.value);
    var idx = meshes.findIndex(function (m) { return m.userData.id === (id && id.value); });
    if (idx >= 0) {
      var s = fanSlot(idx, meshes.length);
      tween(meshes[idx], { y: 0.55, z: 1.55 }, 0.2);
      void s;
    }
  });
  document.addEventListener("focusout", function () {
    applyTint();
  });

  // --- shuffle riffle and scout pulse --------------------------------------
  document.addEventListener("submit", function (ev) {
    var f = ev.target;
    if (!f.classList) { return; }
    if (f.classList.contains("shuffle-form")) { riffle(); }
    if (f.classList.contains("scout-form")) { scoutPulse(); }
  }, true);

  function riffle() {
    if (reduced) { return; }
    for (var i = 0; i < 6; i++) {
      (function (i) {
        var d = new THREE.Mesh(geometry, backMaterial);
        d.scale.setScalar(0.82);
        d.position.copy(deckGroup.position);
        d.position.y += 0.2;
        scene.add(d);
        tween(d, {
          x: deckGroup.position.x + (Math.random() - 0.5) * 3,
          y: 0.9 + Math.random() * 0.8,
          z: deckGroup.position.z + (Math.random() - 0.5) * 2
        }, 0.4);
        tween(d, { rotation: (Math.random() - 0.5) * 2.4 }, 0.4, { axis: "y" });
        tween(d, { opacity: 0 }, 0.36, { onDone: function () { scene.remove(d); } });
      })(i);
    }
    deckCards.forEach(function (d, i) {
      tween(d, { rotation: (Math.random() - 0.5) * 0.5 }, 0.3, { axis: "y", onDone: null });
      void i;
    });
  }

  function scoutPulse() {
    var top = deckCards[deckCards.length - 1];
    if (!top) { return; }
    tween(top, { y: top.position.y + 0.22 }, 0.18, {
      onDone: function () { tween(top, { y: top.position.y }, 0.25); }
    });
  }

  // --- resize and loop -------------------------------------------------------
  function resize() {
    var w = host.clientWidth;
    var h = document.getElementById("hand") ? document.getElementById("hand").offsetHeight : 336;
    if (w < 10 || h < 10) { return; }
    renderer.setSize(w, h, false);
    canvas.style.width = w + "px";
    canvas.style.height = h + "px";
    camera.aspect = w / h;
    camera.updateProjectionMatrix();
  }
  if (window.ResizeObserver) {
    new ResizeObserver(resize).observe(host);
  }
  window.addEventListener("resize", resize);

  var last = 0;
  function loop(ts) {
    var dt = Math.min(50, ts - last || 16);
    last = ts;
    stepTweens(dt);
    renderer.render(scene, camera);
    requestAnimationFrame(loop);
  }

  // --- boot ---------------------------------------------------------------------
  var initial = readDOM();
  loadEmblems(initial.map(function (c) { return c.id; }), function () {
    document.documentElement.setAttribute("data-emblems",
      "ready:" + Object.keys(emblemCache).length + ":" + (textureCache.bucephalus ? "tex" : "notex"));
    backMaterial = new THREE.MeshLambertMaterial({ map: tex(paintBack()) });
    edgeMaterial = new THREE.MeshLambertMaterial({ color: 0x2b241c });
    resize();
    sync();
    requestAnimationFrame(loop);
  });

  document.body.addEventListener("htmx:afterSwap", function () {
    if (document.querySelector(".table-side")) {
      sync();
    }
  });

})();
