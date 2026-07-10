# Notes Background Autosave Design

Notes will save 750 milliseconds after the latest editor change. Each new change resets the timer so typing is not interrupted and requests are not sent per keystroke.

The existing serialized save queue remains the only persistence path. If an autosave becomes due while another save is running, only the latest editor snapshot is queued. Save responses update revision metadata without replacing newer local content.

Pending edits are flushed before selecting or creating another note. The existing Save button remains available for immediate manual saves. The passive status pill continues to show Unsaved, Saving, or Saved; background success does not show a toast, while failures leave the note marked Unsaved and show the existing error feedback.

Regression coverage must prove debounce behavior, latest-snapshot behavior during an in-flight save, and flushing before navigation.
