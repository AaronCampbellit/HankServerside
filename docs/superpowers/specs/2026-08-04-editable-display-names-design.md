# Editable Display Names Design

## Goal

Add optional, editable display names for Hank users without changing the
meaning or behavior of email-based identity.

Email remains the only identifier used for password login, invitations,
account recovery, login rate limiting, external-identity matching, and audit
lookups. A display name is presentation metadata only and must never be
accepted by an authentication or recovery endpoint.

## Authorization

- An administrator may edit the display name of any member of the current
  home.
- A user may edit their own display name.
- A non-administrator may not edit another member's display name.
- The target user must be a member of the authenticated user's current home.

The existing authenticated Home Members route owns this behavior because it
already resolves the current home, membership, and administrator role.

## Data Model

Add `users.display_name` through migration `000028_user_display_names`.

- Type: `TEXT`
- Nullability: `NOT NULL`
- Default: empty string
- Maximum length: 80 Unicode code points
- Stored form: leading and trailing whitespace removed
- Uniqueness: none
- Empty value: allowed and means “use email as the presentation fallback”

Existing users receive the empty default. Invitations and account creation do
not require a display name, so their security and compatibility contracts stay
unchanged.

`domain.User` and `domain.HomeMember` expose `display_name`. Store user scans,
user inserts, and member listings read and write the new column. A dedicated
store update method changes only `display_name` and `updated_at`; it never
changes email or authentication state.

## API

Add:

```text
PUT /v1/home/members/{userID}/display-name
Content-Type: application/json

{"display_name":"Aaron Campbell"}
```

Successful responses return the updated `HomeMember`.

Validation and errors:

- Trim leading and trailing whitespace before validation.
- Reject values longer than 80 Unicode code points with HTTP 400.
- Reject malformed JSON with HTTP 400.
- Return HTTP 403 when a non-administrator targets another user.
- Return HTTP 404 when the target is not a member of the current home.
- Allow an empty string to clear the display name.

The route uses the existing cookie/bearer authentication and CSRF enforcement
that protects Home Members writes. A successful update emits the existing
members-changed event and records a `user.display_name_updated` audit event.
The audit event identifies the actor and target by user ID and does not replace
email-based audit identity or log the display-name value.

The authenticated bootstrap payload includes `user.display_name`. Existing
clients remain compatible because the field is additive.

## Dashboard

Extend the People API client with `updateDisplayName(userID, displayName)`.

On Settings > People:

- Show the display name as the member's primary label when present.
- Keep the email visible as the secondary identity.
- Use the display name for avatar initials when present; otherwise use email.
- Show an Edit name action for the current user and, for administrators, every
  home member.
- Use an inline form with Save and Cancel actions.
- Saving refreshes the member list and shows a success or server validation
  message.
- Clearing the field restores the email presentation fallback.

Other dashboard surfaces that already present the signed-in user or Home
Members use the same display-name-first, email-fallback rule. Login, invitation,
password reset, and audit screens continue to label and submit email wherever
identity precision matters.

## Security and Compatibility

- No authentication handler accepts `display_name`.
- `GetUserByEmail`, invitation email matching, login backoff keys, password
  recovery, SSO identity matching, and email audit lookups remain unchanged.
- Display names are not unique and cannot be used to select an account.
- React renders display names as text, so no HTML interpretation is added.
- The update route is home-scoped and enforces self-or-admin authorization
  server-side.
- The migration is additive and reversible; its down migration removes only
  `users.display_name`.

## Verification

Implementation follows test-driven development:

1. Migration and store tests prove default/backfill behavior, round trips,
   clearing, trimming, and length enforcement.
2. Cloud handler tests prove self update, administrator update, cross-member
   denial, non-member denial, audit emission, and unchanged email login.
3. Dashboard API and People UI tests prove display, fallback, edit
   authorization, save, clear, and error behavior.
4. Run formatting, targeted Go and frontend tests, full `go test ./...`,
   `make build`, `make frontend-check`, migration status, schema drift, and
   `git diff --check`.
5. PostgreSQL migration and live behavior are validated on the approved Hank
   demo server before the board card is completed.
