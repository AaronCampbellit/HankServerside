(function () {
  const imageExtensions = new Set(["apng", "avif", "bmp", "gif", "heic", "heif", "jpeg", "jpg", "png", "svg", "tif", "tiff", "webp"]);
  const textExtensions = new Set(["cfg", "conf", "css", "csv", "go", "js", "json", "log", "md", "ps1", "py", "rb", "rs", "sh", "sql", "swift", "toml", "ts", "txt", "xml", "yaml", "yml"]);

  function extensionForName(name) {
    const lowerName = String(name || "").toLowerCase();
    const index = lowerName.lastIndexOf(".");
    return index > 0 ? lowerName.slice(index + 1) : "";
  }

  function fileTypeLabel(name, isDirectory = false) {
    if (isDirectory) return "Folder";
    const extension = extensionForName(name);
    return extension ? extension.toUpperCase() : "File";
  }

  function iconLabel(name, isDirectory = false) {
    if (isDirectory) return "";
    const extension = extensionForName(name);
    if (!extension) return "FILE";
    if (imageExtensions.has(extension)) return "IMG";
    if (extension === "pdf") return "PDF";
    if (textExtensions.has(extension)) return "TXT";
    return extension.slice(0, 4).toUpperCase();
  }

  window.HankFileServerPreview = {
    extensionForName,
    fileTypeLabel,
    iconLabel,
  };
})();
