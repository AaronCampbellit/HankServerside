(function () {
  function messageRoleLabel(role) {
    return role === "assistant" ? "Hank" : role;
  }

  function formatMessageTimestamp(value) {
    if (!value) return "";
    const date = new Date(value);
    const now = new Date();
    const isSameDay =
      date.getFullYear() === now.getFullYear() &&
      date.getMonth() === now.getMonth() &&
      date.getDate() === now.getDate();
    return isSameDay ? date.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" }) : date.toLocaleString();
  }

  function formatTraceTime(value) {
    return value ? new Date(value).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }) : "";
  }

  function renderTraceDetails(details = {}, escapeHTML = window.HankUI.escapeHTML) {
    const rows = Object.entries(details || {}).filter(([_, value]) => String(value || "").trim());
    if (!rows.length) return "";
    return `<dl class="hank-log-details">${rows.map(([key, value]) => `
      <div>
        <dt>${escapeHTML(key.replaceAll("_", " "))}</dt>
        <dd>${escapeHTML(value)}</dd>
      </div>
    `).join("")}</dl>`;
  }

  window.HankRenderers = {
    messageRoleLabel,
    formatMessageTimestamp,
    formatTraceTime,
    renderTraceDetails,
  };
})();
