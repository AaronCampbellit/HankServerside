(function () {
  function escapeHTML(value) {
    return String(value ?? "")
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;")
      .replaceAll("'", "&#39;");
  }

  function showToast(element, message, isError = false, timeoutMs = 3400) {
    if (!element) return;
    element.hidden = false;
    element.textContent = message;
    element.style.background = isError ? "rgba(142, 45, 28, 0.94)" : "rgba(35, 27, 20, 0.92)";
    clearTimeout(element._hankToastTimeoutID);
    element._hankToastTimeoutID = window.setTimeout(() => {
      element.hidden = true;
    }, timeoutMs);
  }

  function copyTextFallback(text) {
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.setAttribute("readonly", "readonly");
    textarea.style.position = "fixed";
    textarea.style.left = "-9999px";
    textarea.style.top = "0";
    document.body.appendChild(textarea);
    textarea.focus();
    textarea.select();

    let copied = false;
    try {
      copied = document.execCommand("copy");
    } catch (_) {
      copied = false;
    }
    if (copied) {
      textarea.remove();
    } else {
      window.setTimeout(() => textarea.remove(), 15000);
    }
    return copied;
  }

  async function copyText(text) {
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(text);
        return true;
      }
    } catch (_) {
    }
    return copyTextFallback(text);
  }

  function setBusy(button, busy, busyText = "Working") {
    if (!button) return;
    if (busy) {
      button.dataset.idleText = button.textContent;
      button.textContent = busyText;
      button.disabled = true;
      return;
    }
    button.textContent = button.dataset.idleText || button.textContent;
    button.disabled = false;
    delete button.dataset.idleText;
  }

  window.HankUI = {
    escapeHTML,
    showToast,
    copyText,
    setBusy,
  };
})();
