# HankAI Intent Rollout

Status: Superseded by current assistant intent/tool implementation and active eval guidance. Retained only as historical rollout context; it is not the current task tracker.

## Goal

Make HankAI reliable for simple asks and commands inside Hank by routing common requests through deterministic intents before falling back to general model reasoning.

The target behavior is:

- understand short commands such as `add call Patrick to the work note`
- resolve destinations in a predictable order
- ask a clear follow-up when the target is ambiguous
- return tappable result cards for notes, files, calendar events, and Home Assistant entities
- require confirmation for risky or destructive changes

## Routing Principles

HankAI should use typed intent routing for common commands.

For each intent, define:

- user phrases
- required fields
- optional fields
- source permissions required
- entity resolver
- confidence threshold
- confirmation rules
- card/action payload returned to Hank
- regression test phrases

The model can help phrase answers, summarize content, and reason over retrieved context, but the server should own command classification, entity resolution, permissions, confirmation, and writes.

## Shared Entity Resolution

Use the same resolver pattern for notes, files, folders, calendar events, and Home Assistant entities.

Recommended match order:

1. Exact name, title, path, or friendly alias
2. Prefix or title/name contains
3. Token match in title/name
4. Body, content, path, calendar notes, or state attributes
5. Vector or semantic match from the assistant index
6. Recency and usage tie-breakers

When the top two matches are close inside the same tier, HankAI should ask the user to choose from 2-3 tappable options instead of guessing.

## Rollout Phases

### Phase 1: Intent Contract And Test Harness

Add a central intent contract table for:

- intent name
- examples
- extracted slots
- required permissions
- risk level
- confirmation requirement
- expected response shape

Add table-driven tests for every supported phrase. These tests should run without a model call.

Initial files:

- `internal/cloud/assistant_tools.go`
- `internal/cloud/assistant.go`
- `internal/cloud/assistant_workflow_test.go`

### Phase 2: Shared Slot Extraction

Normalize command parsing into reusable slots:

- `action`: find, open, add, append, create, attach, move, delete, check off, turn on
- `object`: note, file, photo, link, event, entity, folder, checklist item
- `payload`: text being added, uploaded attachment IDs, date/time, state value
- `destination`: note title, folder path, calendar, entity name
- `qualifiers`: work, latest, shared, personal, tomorrow, first result, same note

Support references from previous chat turns:

- `that`
- `this`
- `it`
- `the first one`
- `same note`
- `the file I uploaded`
- `the link above`

### Phase 3: Notes Intents

Notes should be the first full rollout target because Notes is the most common HankAI write surface.

Read intents:

- `notes.list`
- `notes.search`
- `notes.open`
- `notes.summarize`
- `notes.find_text`

Write intents:

- `notes.create`
- `notes.append_text`
- `notes.append_link`
- `notes.append_file`
- `notes.append_photo`
- `notes.add_checklist_item`
- `notes.check_item`
- `notes.uncheck_item`
- `notes.rename`
- `notes.move_or_share`

Resolver order for note destinations:

1. exact note title, such as `Work`
2. title contains, such as `Work Projects`
3. note body mentions the target, such as a note discussing work
4. semantic/vector match
5. ask user to choose

Recommended defaults:

- Append plain text as a bullet unless the user says checklist, task, todo, or check off.
- Append links as the URL or markdown link if a title is known.
- Append photos/files as note attachments with a short markdown reference.
- Confirm before creating a new note if a likely existing note is found.
- Ask before editing shared Home notes when the destination is ambiguous.

Regression phrases:

- `add call Patrick to the work note`
- `add this link to work note`
- `put the uploaded photo in receipts`
- `add milk to the grocery list`
- `check off call Patrick in work`
- `create a note called server fixes`
- `summarize my work note`
- `find notes talking about deployment`

### Phase 4: File And SMB Intents

Read intents:

- `files.search`
- `files.open`
- `files.list_folder`
- `files.summarize_location`

Write intents:

- `files.upload`
- `files.move`
- `files.copy`
- `files.rename`
- `files.create_folder`
- `files.delete`

Resolver order:

1. exact configured share or folder alias
2. exact indexed path
3. folder/file name contains
4. path/content semantic match
5. ask user to choose

Confirmation rules:

- Require confirmation for overwrite, move, rename, delete, and cross-folder copy.
- Low-risk upload to a uniquely matched folder can proceed after the assistant displays the target.

Regression phrases:

- `put this PDF in the taxes folder`
- `upload this photo to receipts`
- `find the GitLab docker file`
- `open the 2025 tax folder`
- `rename this file to invoice May`

### Phase 5: Calendar Intents

Read intents:

- `calendar.search`
- `calendar.today`
- `calendar.tomorrow`
- `calendar.week`
- `calendar.find_event`

Write intents:

- `calendar.create_event`
- `calendar.update_event`
- `calendar.cancel_event`

Resolver rules:

- Calendar context must be uploaded before assistant send/resume when calendar is enabled.
- Missing year or ambiguous date should require confirmation.
- Event edits and deletes should always require confirmation.

Regression phrases:

- `what do I have tomorrow`
- `add dentist appointment Friday at 2`
- `move my 3pm meeting to 4`
- `cancel the work meeting tomorrow`
- `what meetings mention Patrick`

### Phase 6: Home Assistant Intents

Read intents:

- `homeassistant.search_entities`
- `homeassistant.entity_status`
- `homeassistant.room_status`
- `homeassistant.list_on`

Write intents:

- `homeassistant.turn_on`
- `homeassistant.turn_off`
- `homeassistant.set_brightness`
- `homeassistant.set_temperature`
- `homeassistant.lock_or_unlock`

Resolver order:

1. exact entity friendly name
2. room plus device type
3. domain plus state attributes
4. semantic match from indexed entity state
5. ask user to choose

Confirmation rules:

- Lights and switches can be low risk if the entity is uniquely matched.
- Locks, garage doors, thermostats, and security-related devices should require confirmation.

Regression phrases:

- `turn off the kitchen lights`
- `set living room lights to 40 percent`
- `what garage entities are open`
- `is the front door locked`
- `turn on the fan in the bedroom`

### Phase 7: Project And Operator Intents

Read intents:

- `project_docs.search`
- `project_docs.answer`
- `server.status`
- `storage.status`
- `backup.status`
- `sync.status`

Operator intents:

- `backup.run`
- `restore.verify`
- `service_profile.check`
- `agent.status`

Confirmation rules:

- Project docs and status are read-only.
- Backup verification can be confirmation-free if it is non-destructive.
- Primary restore, destructive storage operations, and service-profile mutation must require confirmation and role checks.

Regression phrases:

- `how do I deploy HankServerside`
- `what does AGENTS.md say about SMB`
- `show backup status`
- `is the home agent online`
- `run a restore verification`
- `what server changes does the app need`

### Phase 8: Clarification And Cards

Every ambiguous resolver should return structured options:

- note card
- file/folder card
- calendar event card
- Home Assistant entity card
- project document card

Clarification responses should be short:

`I found more than one Work match. Which one should I use?`

Then show options:

- `Work`
- `Work Projects`
- `Personal Reminders`

The next user reply can be:

- `first one`
- `the Work note`
- `personal reminders`
- tap a card

### Phase 9: Safety, Audit, And Rollback

Persist for every write intent:

- prompt
- resolved intent
- extracted slots
- selected target
- confidence tier
- confirmation state
- before and after summary
- resulting note/file/event/entity IDs

Support rollback where practical:

- note appends can store the previous revision
- file moves/renames can store source and destination
- calendar edits can store previous event fields

Deletes and primary restore should stay confirmation-gated and should not run from casual phrasing.

## Intent Inventory To Add

### Notes

- list notes
- search notes
- open note
- summarize note
- create note
- append text
- append link
- append file/photo
- add checklist item
- check off checklist item
- uncheck checklist item
- rename note
- share note
- find text inside notes

### Files

- search files
- search folders
- open folder
- upload file/photo
- move file
- copy file
- rename file
- create folder
- delete file/folder

### Calendar

- today schedule
- tomorrow schedule
- week schedule
- search events
- create event
- update event
- cancel event

### Home Assistant

- search entities
- get entity status
- list devices on/open/unavailable
- turn on/off
- set brightness
- set temperature
- lock/unlock

### Project And Server

- answer project-doc question
- search project docs
- explain app/server contract
- show assistant source/index status
- show sync status
- show agent status
- show storage/backup status
- run non-destructive backup/restore verification

## Product Decisions From Aaron

These decisions replace the initial static-input assumptions. HankAI must expect names, aliases, homes, devices, folders, and user habits to change per user.

### Dynamic Aliases

Do not hard-code global note, file-share, folder, calendar, or Home Assistant aliases as product truth.

Aliases should be learned or resolved dynamically from:

- user-owned note titles and bodies
- shared Home note titles and bodies
- recent successful HankAI resolutions
- service profile names
- indexed SMB paths and folder names
- Home Assistant friendly names, areas, entity IDs, and aliases
- calendar names and recently used calendars

Static examples can stay in tests, but production behavior must be per user and per Home.

### Default Note Write Style

When adding content to an existing note, use the last content type already used in that note.

Resolution order:

1. If the last meaningful line is a checklist item, append as a checklist item.
2. If the last meaningful line is a bullet item, append as a bullet item.
3. If the last meaningful line is a numbered item, append as the next numbered item.
4. If the note is paragraph-style, append as a paragraph.
5. If the note is empty, default to a bullet item unless the user says checklist, todo, or task.

The user can still override this in the prompt:

- `add this as a checklist item`
- `append as a paragraph`
- `make this a todo`

### Calendar Defaults

Add a user-facing default-calendar setting in Hank calendar settings.

HankAI should:

- use the configured default calendar when the prompt does not specify a calendar
- still allow explicit calendar targeting, such as `add this to my work calendar`
- use calendar names and recent calendar usage as dynamic resolver context
- keep date/time ambiguity handling in the assistant confirmation flow

### Confirmation Preference

Default to auto-run when the target is uniquely resolved and the action is not a deletion or removal.

Always require confirmation for:

- delete
- remove
- cancel
- primary restore
- any command that clearly discards data
- any low-confidence or ambiguous destination

Non-delete/non-removal actions can auto-run when uniquely matched, including note appends, note creation, file/photo upload, calendar creation, and Home Assistant changes.

### Regression Phrase Seed List

These real phrases should become assistant workflow tests. Keep spelling variants where useful because users will type them that way.

- `Add drinks to the grocery list`
- `add email patrick to work notes`
- `find pest control to family tasks`
- `find the 2025 tax documents folder`
- `find bagheeras paperwork`
- `add family dinner to the 12th of july`
- `when is Taylors Birthday`
- `how do i verify if PostgreSQL data checksums are enabled for hank`
- `add this picture to work notes`
- `add this picutre to the work folder`
- `pull todo tags list`
- `whats on my calandar for today`
- `find the hank feature note`
- `add link to hank feature note`

## Remaining Product Inputs

These are still useful but should be stored as user/Home settings or learned context, not hard-coded globally.

- default event duration
- whether project/operator actions such as backup verification should be admin-only visible in HankAI
- whether Home Assistant physical-device actions should show a lightweight confirmation even though they are not deletion/removal
- whether HankAI should remember successful target selections as aliases automatically or ask before saving aliases

## Done Criteria

This rollout is done when:

- every supported command has a typed intent contract
- every typed intent has regression phrases
- entity resolution follows exact/title/content/semantic order
- ambiguous matches return tappable clarification options
- risky actions use confirmation summaries
- Hank app can route every result card to the right screen
- `go test ./...` and focused assistant workflow tests pass
- at least one live test covers notes, files, calendar, Home Assistant, and project docs
