import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import vm from "node:vm";

const __dirname = dirname(fileURLToPath(import.meta.url));

const context = {
  window: {},
};
vm.createContext(context);
vm.runInContext(readFileSync(join(__dirname, "profile-notes-editor.js"), "utf8"), context);

const editor = context.window.HankProfileNotesEditor;
const escapeHTML = (value) => String(value || "")
  .replace(/&/g, "&amp;")
  .replace(/</g, "&lt;")
  .replace(/>/g, "&gt;")
  .replace(/"/g, "&quot;");
const renderLink = (label, rawURL, image = false) => `${image ? "image" : "link"}:${escapeHTML(label)}:${escapeHTML(rawURL)}`;
const checklistToggle = ({ lineIndex, checked }) => `<button data-line-index="${lineIndex}" data-checked="${checked}"></button>`;

assert.equal(
  editor.renderInlineText("Use **bold**, *italic*, and <u>underlined</u> text.", { escapeHTML, renderLink }),
  "Use <strong>bold</strong>, <em>italic</em>, and <u>underlined</u> text.",
);

assert.equal(
  editor.renderInlineText("Keep [Hank](https://hank.local) and ![shot](hank-note-attachment://att_1).", { escapeHTML, renderLink }),
  "Keep link:Hank:https://hank.local and image:shot:hank-note-attachment://att_1.",
);

assert.equal(
  editor.renderInlineLine("## Roadmap", 0, { escapeHTML, renderLink, checklistToggle }),
  '<span class="note-heading note-heading-2">Roadmap</span>',
);

assert.equal(
  editor.renderInlineLine("- ship notes tools", 1, { escapeHTML, renderLink, checklistToggle }),
  '<span class="note-list-marker">-</span> <span class="note-list-text">ship notes tools</span>',
);

assert.equal(
  editor.renderInlineLine("1. verify renderer", 2, { escapeHTML, renderLink, checklistToggle }),
  '<span class="note-list-marker">1.</span> <span class="note-list-text">verify renderer</span>',
);

assert.equal(
  editor.renderInlineLine("○ call Patrick", 3, { escapeHTML, renderLink, checklistToggle }),
  '<button data-line-index="3" data-checked="false"></button><span class="note-check-text">call Patrick</span>',
);
