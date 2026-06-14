(function () {
  function normalizeTag(value) {
    return String(value || "")
      .trim()
      .replace(/^#/, "")
      .replace(/\s+/g, "-")
      .replace(/[^A-Za-z0-9_-]/g, "");
  }

  function normalizeURL(value) {
    const trimmed = String(value || "").trim();
    if (!trimmed) {
      return "";
    }
    if (/^[a-z][a-z0-9+.-]*:/i.test(trimmed)) {
      return trimmed;
    }
    return `https://${trimmed}`;
  }

  function isLikelyURL(value) {
    return /^(https?:\/\/|www\.|[A-Za-z0-9-]+\.[A-Za-z]{2,})/.test(String(value || "").trim());
  }

  function cleanLinkToken(value) {
    return String(value || "")
      .trim()
      .replace(/^["'`<([{]+/, "")
      .replace(/[>"'`)\]},.;!?]+$/g, "");
  }

  function orderedListMatch(line) {
    return String(line || "").match(/^(\s*)(\d+)(\.\s+)(.*)$/);
  }

  function mapPositionThroughLineChange(position, oldStart, oldLine, newStart, newLine, oldPrefixLength, newPrefixLength) {
    if (position <= oldStart) {
      return newStart;
    }
    const oldEnd = oldStart + oldLine.length;
    if (position > oldEnd) {
      return newStart + newLine.length;
    }
    const offset = position - oldStart;
    if (offset <= oldPrefixLength) {
      return newStart + Math.min(offset, newPrefixLength);
    }
    return newStart + newPrefixLength + Math.min(offset - oldPrefixLength, newLine.length - newPrefixLength);
  }

  function mapPositionThroughOrderedListChanges(position, changes) {
    let delta = 0;
    for (const change of changes) {
      const oldEnd = change.oldStart + change.oldLine.length;
      if (position < change.oldStart) {
        return position + delta;
      }
      if (position <= oldEnd) {
        return mapPositionThroughLineChange(
          position,
          change.oldStart,
          change.oldLine,
          change.oldStart + delta,
          change.newLine,
          change.oldPrefixLength,
          change.newPrefixLength,
        );
      }
      delta += change.newLine.length - change.oldLine.length;
    }
    return position + delta;
  }

  window.HankProfileNotesEditor = {
    normalizeTag,
    normalizeURL,
    isLikelyURL,
    cleanLinkToken,
    orderedListMatch,
    mapPositionThroughOrderedListChanges,
  };
})();
