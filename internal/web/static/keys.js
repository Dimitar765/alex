// Keyboard play: pressing 1-9 takes the corresponding available choice.
// Progressive enhancement — the forms work natively without this script.
(function () {
  "use strict";
  document.body.classList.add("js");

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
})();
