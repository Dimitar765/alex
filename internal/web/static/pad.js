// pad.js — gamepad navigation for the desktop shell (Steam Deck friendly).
// D-pad or left stick moves focus across choices, cards, and deck
// actions; A activates; B snaps back to the first choice. Focus is
// re-homed after every htmx swap. Purely additive: without a pad this
// file does nothing.
(function () {
  "use strict";

  var padIndex = -1;

  window.addEventListener("gamepadconnected", function (e) {
    padIndex = e.gamepad.index;
    focusFirst();
  });
  window.addEventListener("gamepaddisconnected", function (e) {
    if (e.gamepad.index === padIndex) {
      padIndex = -1;
    }
  });

  function focusables() {
    var els = document.querySelectorAll(
      "#scene .choices button:not([disabled]), " +
      "#hand .card-btn:not([disabled]), " +
      ".deck-actions button:not([disabled])");
    return Array.prototype.slice.call(els);
  }

  function focusFirst() {
    var f = focusables();
    if (f.length) {
      f[0].focus();
    }
  }

  // After swaps the focused element is gone; re-home when a pad is active
  // and nothing else took focus.
  document.body.addEventListener("htmx:afterSwap", function () {
    if (padIndex >= 0 && document.activeElement === document.body) {
      focusFirst();
    }
  });

  var lastDown = false, lastUp = false, lastA = false, lastB = false, lastY = 0;

  function poll() {
    if (padIndex >= 0 && navigator.getGamepads) {
      var pads = navigator.getGamepads();
      var p = pads[padIndex];
      if (p) {
        var down = p.buttons[13] && p.buttons[13].pressed;
        var up = p.buttons[12] && p.buttons[12].pressed;
        var a = p.buttons[0] && p.buttons[0].pressed;
        var b = p.buttons[1] && p.buttons[1].pressed;
        var stick = p.axes[1] || 0;

        var list = focusables();
        var focus = document.activeElement;
        var idx = focus ? list.indexOf(focus) : -1;
        var move = 0;
        if (down && !lastDown) { move = 1; }
        if (up && !lastUp) { move = -1; }
        if (stick > 0.6 && lastY <= 0.6) { move = 1; }
        if (stick < -0.6 && lastY >= -0.6) { move = -1; }
        if (move !== 0 && list.length) {
          var n = idx < 0 ? 0 : (idx + move + list.length) % list.length;
          list[n].focus();
          list[n].scrollIntoView({ block: "nearest" });
        }
        if (a && !lastA && idx >= 0 && focus) {
          focus.click();
        }
        if (b && !lastB) {
          focusFirst();
        }
        lastDown = down; lastUp = up; lastA = a; lastB = b; lastY = stick;
      }
    }
    requestAnimationFrame(poll);
  }
  requestAnimationFrame(poll);
})();
