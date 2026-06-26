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

  function defaultEscapeHTML(value) {
    return String(value || "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function escapeForOptions(value, options = {}) {
    const escapeHTML = typeof options.escapeHTML === "function" ? options.escapeHTML : defaultEscapeHTML;
    return escapeHTML(value);
  }

  function renderInlineLink(label, rawURL, image, options = {}) {
    if (typeof options.renderLink === "function") {
      return options.renderLink(label, rawURL, image);
    }
    return escapeForOptions(label || rawURL, options);
  }

  function renderBareLinks(text, options = {}) {
    const source = String(text || "");
    const barePattern = /\b((?:https?:\/\/|www\.)[^\s<>()]+|[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\.[A-Za-z0-9-]+)+(?:\/[^\s<>()]*)?)/g;
    let html = "";
    let lastIndex = 0;
    for (const match of source.matchAll(barePattern)) {
      const raw = match[1];
      const index = match.index || 0;
      html += escapeForOptions(source.slice(lastIndex, index), options);
      html += renderInlineLink(raw, raw, false, options);
      lastIndex = index + raw.length;
    }
    html += escapeForOptions(source.slice(lastIndex), options);
    return html;
  }

  function renderInlineText(text, options = {}) {
    const source = String(text || "");
    const tokenPattern = /(!?)\[([^\]\n]+)\]\(([^)\s]+)\)|\*\*([^*\n]+)\*\*|\*([^*\n]+)\*|<u>([^<\n]+)<\/u>/gi;
    let html = "";
    let lastIndex = 0;
    for (const match of source.matchAll(tokenPattern)) {
      const index = match.index || 0;
      html += renderBareLinks(source.slice(lastIndex, index), options);
      if (match[2] !== undefined) {
        html += renderInlineLink(match[2], match[3], match[1] === "!", options);
      } else if (match[4] !== undefined) {
        html += `<strong>${renderInlineText(match[4], options)}</strong>`;
      } else if (match[5] !== undefined) {
        html += `<em>${renderInlineText(match[5], options)}</em>`;
      } else if (match[6] !== undefined) {
        html += `<u>${renderInlineText(match[6], options)}</u>`;
      }
      lastIndex = index + match[0].length;
    }
    html += renderBareLinks(source.slice(lastIndex), options);
    return html || "&nbsp;";
  }

  function renderChecklistToggle(lineIndex, checked, options = {}) {
    if (typeof options.checklistToggle === "function") {
      return options.checklistToggle({ lineIndex, checked });
    }
    const pressed = checked ? "true" : "false";
    const title = checked ? "Mark incomplete" : "Mark complete";
    const checkedClass = checked ? " checked" : "";
    return `<button type="button" class="note-check-toggle inline${checkedClass}" data-line-index="${lineIndex}" aria-pressed="${pressed}" title="${title}"><span class="note-check-circle" aria-hidden="true"></span></button>`;
  }

  function renderInlineLine(line, lineIndex, options = {}) {
    const source = String(line || "");
    const checklist = source.match(/^(\s*)((?:[-*]\s+\[)([ xX])(?:\]\s+)|[○●]\s+)(.*)$/);
    if (checklist) {
      const checked = checklist[2].startsWith("●") || (checklist[3] || "").toLowerCase() === "x";
      const text = checklist[4] || "Checklist item";
      return `${escapeForOptions(checklist[1], options)}${renderChecklistToggle(lineIndex, checked, options)}<span class="note-check-text">${renderInlineText(text, options)}</span>`;
    }

    const heading = source.match(/^(\s*)(#{1,6})\s+(.*)$/);
    if (heading) {
      const level = heading[2].length;
      return `${escapeForOptions(heading[1], options)}<span class="note-heading note-heading-${level}">${renderInlineText(heading[3], options)}</span>`;
    }

    const unordered = source.match(/^(\s*)[-*]\s+(.*)$/);
    if (unordered) {
      return `${escapeForOptions(unordered[1], options)}<span class="note-list-marker">-</span> <span class="note-list-text">${renderInlineText(unordered[2], options)}</span>`;
    }

    const ordered = source.match(/^(\s*)(\d+)\.\s+(.*)$/);
    if (ordered) {
      return `${escapeForOptions(ordered[1], options)}<span class="note-list-marker">${escapeForOptions(`${ordered[2]}.`, options)}</span> <span class="note-list-text">${renderInlineText(ordered[3], options)}</span>`;
    }

    return renderInlineText(source, options);
  }

  window.HankProfileNotesEditor = {
    normalizeTag,
    normalizeURL,
    isLikelyURL,
    cleanLinkToken,
    orderedListMatch,
    mapPositionThroughOrderedListChanges,
    renderInlineText,
    renderInlineLine,
  };
})();
