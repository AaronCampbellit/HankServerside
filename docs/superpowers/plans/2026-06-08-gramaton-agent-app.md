# Gramaton Agent App Follow-Up Plan

Date: 2026-06-08

Status: Approved handoff scope, pending implementation.

## Goal

Extract the optional `/gramaton` media search and download workflow into a first-party installable `.hankapp` package after the Hermes runtime path is stable. This plan is a handoff stub for the next implementation pass; it does not implement Gramaton.

## Scope Boundaries

- Keep Hank auth, assistant shell, files, notes, Home Assistant, dashboard, storage, backup, and restore flows built into HankServerside.
- Keep cloud responsibilities generic: app install/config/invoke, event relay, and app status. Do not add Gramaton-specific cloud routes unless they are compatibility shims during migration.
- Keep local media credentials, network access, SMB destination access, and file writes inside the home agent app runtime.
- Preserve the compiled Gramaton/media fallback until package-backed search, plan, download, status, cancel, jobs, and progress events are validated end to end.

## Preserved Requirements

- source-aware destination selection
- login and destination validation before saving settings
- ranged download verification with fallback to single-stream download
- `media.downloads` event publishing
- download status, cancel, and jobs commands
- no writes outside selected SMB source/destination policy

## Implementation Notes

- Extend the app runtime before extraction if Gramaton needs persistent jobs, cancellation, or event publication that Hermes did not require.
- Model Gramaton package permissions explicitly: configured network origins for media search/download APIs, selected SMB destination source IDs, and `media.downloads` event publish access.
- Keep the assistant `/gramaton` command behavior compatible with existing media cards and download confirmation flows while the package path is adopted.
- Add package-level tests for search, destination validation, ranged-download fallback, job lifecycle, cancellation, and path containment before removing compiled fallback code.
