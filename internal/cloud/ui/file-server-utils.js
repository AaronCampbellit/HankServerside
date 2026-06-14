(function () {
  function formatDate(value) {
    return value ? new Date(value).toLocaleString() : "Never";
  }

  function formatBytes(value) {
    const bytes = Number(value || 0);
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
  }

  function filenameFromContentDisposition(header) {
    const value = String(header || "");
    const filenameExt = value.match(/filename\*=UTF-8''([^;]+)/i);
    if (filenameExt) {
      try {
        return decodeURIComponent(filenameExt[1].trim());
      } catch (_) {
        return filenameExt[1].trim();
      }
    }
    const filename = value.match(/filename=(?:"([^"]+)"|([^;]+))/i);
    return filename ? (filename[1] || filename[2] || "").trim() : "";
  }

  function normalizePath(value) {
    const trimmed = String(value || "").trim();
    if (!trimmed || trimmed === "/") return "/";
    const normalized = `/${trimmed.replace(/^\/+/, "").replace(/\/+/g, "/")}`;
    return normalized.endsWith("/") && normalized !== "/" ? normalized.slice(0, -1) : normalized;
  }

  function joinPath(base, child) {
    const normalizedBase = normalizePath(base);
    const normalizedChild = String(child || "").trim().replace(/^\/+/, "");
    if (!normalizedChild) return normalizedBase;
    return normalizedBase === "/" ? `/${normalizedChild}` : `${normalizedBase}/${normalizedChild}`;
  }

  window.HankFileServerUtils = {
    formatDate,
    formatBytes,
    filenameFromContentDisposition,
    normalizePath,
    joinPath,
  };
})();
