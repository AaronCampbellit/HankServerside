# File Server Worker and Host Sources Design

Date: 2026-07-13

## Goal

Present every usable file target in the HankServerside File Server dashboard:

- SMB shares configured on the primary home agent
- host folders configured on the primary home agent
- shared folders exposed by online HankAgent worker devices

Browsing, searching, previews, uploads, downloads, and mutations must route to
the agent that owns the selected target.

## Current Failure

The primary agent already validates, creates when requested, persists, restores,
and serves configured host folders. Its SMB service snapshot includes both SMB
and local sources. The dashboard currently discards records whose type is local,
so those working sources never appear.

Worker agents expose their shared folders as directories under a virtual root.
They do not publish an SMB service profile. The dashboard does not load the
home's worker agents and its file client does not send an `agent_id`, so it
cannot discover or route to worker folders even though the cloud already
supports targeted commands, previews, and transfers.

## Source Model

The dashboard will use one unified picker. Each entry carries:

- a stable UI key composed from agent and source identity
- the owning `agent_id` (blank for the primary agent's compatibility route)
- the agent-local `source_id` (blank for a worker virtual root)
- a display name and descriptive detail
- whether the entry represents a primary SMB source, primary host folder, or
  worker shared-folder root

Primary sources come from all compatible SMB profile arrays, including
`sources`, `file_sources`, `shares`, and `folders`. Local records are retained.
Duplicate source IDs are collapsed.

Online workers advertising file access appear as one picker entry per device.
Selecting a worker lists its virtual root with a blank `source_id`; the worker's
shared folders then appear as normal top-level directories. Offline workers and
workers without file capabilities are not shown as usable targets.

## Routing

The selected entry's `agent_id` and `source_id` travel together through every
operation:

- WebSocket commands: list, search, stat, create directory, rename, move, and
  delete
- HTTP transfer setup: upload and download
- preview URLs, including ranged media previews

Changing targets resets the path to root, clears selection/search/preview
state, and reloads from the new owner.

Moves may target another source only when both sources belong to the same
agent. Cross-agent moves are not offered because the existing `files.move`
contract performs a move inside one agent and cannot safely move data between
devices. Upload, download, rename, delete, and folder creation retain their
existing permission and policy behavior.

## Error Handling and Compatibility

If agent discovery fails, primary sources still load from the existing service
profile route. If the profile route fails but worker discovery succeeds, worker
targets remain available. A selected target that disconnects reports the
existing target-offline error without silently rerouting to the primary agent.

Older single-agent deployments continue using a blank `agent_id`. Existing SMB
source IDs and deep links remain valid. Deep links may add an optional
`agent_id`; links without it retain primary-agent behavior.

## Security and Data Impact

No new public route, credential exposure, or filesystem permission is added.
Existing authenticated home membership checks, file policies, path containment,
transfer leases, and agent routing checks remain authoritative. Host absolute
paths may be shown only where the existing profile already exposes them to the
authenticated dashboard.

There is no database schema change.

## Verification

Regression tests will prove:

- primary local/host sources are retained and selectable
- online file-capable workers appear and browse their virtual root
- worker commands, previews, uploads, and downloads carry the selected
  `agent_id`
- changing device/source clears stale browsing state
- move destinations are limited to sources owned by the active agent
- existing primary SMB behavior remains compatible

Focused dashboard tests and the full dashboard check/build will run. Relevant
Go tests will confirm the existing targeted cloud transfer/preview routing and
host-folder configuration path remain green.
