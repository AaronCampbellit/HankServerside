(function () {
  function toast(message, options = {}) {
    const target = options.target || document.getElementById("toast");
    if (!target) return;
    target.hidden = false;
    target.textContent = message;
    target.style.background = options.error ? "rgba(142, 45, 28, 0.94)" : "rgba(35, 27, 20, 0.92)";
    clearTimeout(target._hankToastTimeoutID);
    target._hankToastTimeoutID = window.setTimeout(() => {
      target.hidden = true;
    }, options.duration || 3400);
  }

  function setBusy(element, isBusy, label) {
    if (!element) return;
    if (isBusy) {
      if (!element.dataset.idleText) {
        element.dataset.idleText = element.textContent || "";
      }
      element.disabled = true;
      if (label) {
        element.textContent = label;
      }
      element.setAttribute("aria-busy", "true");
      return;
    }
    element.disabled = false;
    element.removeAttribute("aria-busy");
    if (element.dataset.idleText) {
      element.textContent = element.dataset.idleText;
      delete element.dataset.idleText;
    }
  }

  function emptyState(container, message) {
    if (!container) return;
    container.className = "card-list empty-state";
    container.textContent = message;
  }

  window.HankUI = {
    toast,
    setBusy,
    emptyState,
  };
})();
