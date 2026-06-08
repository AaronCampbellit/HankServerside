# Invite Signup And Password Reset Design

Date: 2026-06-08

Status: Approved design, pending implementation plan.

## Context

Hank Remote already has first-admin registration, login, sessions, home
memberships, and home invitations. First-admin registration creates the
singleton home and is disabled after setup. Existing home invitations create a
one-time token and require the invitee to already be signed in with the matching
email before accepting.

The goal is to make member onboarding and password recovery usable without
weakening the current single-home, self-hosted security model.

## Goals

- Let admins invite a new person without creating or knowing that person's
  permanent password.
- Let invitees create their own account from an invite code and set their
  permanent password during invite setup.
- Let admins reset a member password with a temporary password.
- Let admins choose whether the member must change that temporary password on
  first login after reset.
- Revoke existing sessions whenever an admin or CLI password reset happens.
- Add a break-glass CLI reset path for admin recovery.
- Keep raw passwords, invite tokens, and reset tokens out of logs and durable
  storage.

## Non-Goals

- No email or SMS delivery in the first implementation.
- No multi-home or SaaS invitation model.
- No direct SMB, Home Assistant, file, note, or media credential sharing.
- No admin-created permanent password during normal invite signup.

## Product Decisions

The first implementation uses manual plain-text sharing for invite codes and
temporary reset passwords. Admins can copy the generated invite code, a join
link, or a generated temporary password from the dashboard. Those values are
shown once and are never stored raw.

Email and SMS can be added later as optional delivery transports for invite or
reset links. They should not become the source of truth for credentials.

## Invite Flow

An admin creates an invite from the People page. The cloud validates admin home
membership, creates a pending invitation scoped to the singleton home, target
email, role, token hash, and expiration, then returns the raw token once.

The dashboard shows:

- Target email.
- Role.
- Expiration.
- Invite code.
- Join URL.
- Copy controls for the invite code and join URL.

The join URL should avoid sending the token to the server as a query parameter
when practical. A fragment URL such as `/join#token=<invite-token>` lets browser
JavaScript read the token and submit it in a JSON body, which reduces accidental
token exposure in reverse proxy logs. The plain invite code path remains
available for manual paste.

## Invite Signup Flow

The join page is public enough to display a signup form, but all invite details
come from token-backed API calls.

The invitee opens the join URL or pastes the invite code. The page calls a
public invite preview endpoint with the token in the request body. The preview
returns only limited information needed to confirm the invite:

- Email.
- Role.
- Expiration.
- Whether the invite is expired or already accepted.

If the invite email does not already have a user account, the page lets the
invitee set their permanent password. Signup submits token, email, and password.
The server validates:

- Token hash matches a pending invitation.
- Invitation is not expired.
- Invitation is not accepted.
- Submitted email exactly matches the invitation email after normalization.
- No existing user already owns that email.
- Password meets the current password rules.

On success, the server creates the user, accepts the invitation, creates the
home membership, creates a normal session, sets the session cookie, and returns
the same session response shape used by login.

If the email already has a user account, the page directs the person to sign in
and use the existing authenticated invite-accept route. That preserves the
current requirement that existing users prove ownership of the account before
joining the home.

## Admin Password Reset Flow

The People page adds a reset action for other members. The reset dialog includes:

- Temporary password input.
- Generate button.
- Require password change on next login checkbox, default checked.
- Confirmation text that existing sessions will be revoked.

Submitting the reset requires home admin role. The server hashes the temporary
password, updates the target user password metadata, revokes every active
session for that user, removes APNS devices tied to those sessions where
applicable, and writes an audit event.

The raw temporary password is never logged and never stored. If the server
generates the password, the response includes it once so the admin can share it
manually. If the admin supplies a temporary password, the response does not echo
it back.

The UI does not expose reset for the signed-in admin's own account. Users should
change their own password through a change-password flow, and break-glass
recovery should use the CLI.

## Forced Password Change

The users table gains a `password_change_required` flag. Login still verifies
the submitted password and creates a session, but a user with this flag set is
limited to:

- `GET /v1/me`
- `POST /v1/auth/change-password`
- `POST /v1/auth/logout`
- the password-change page and required static assets

Other dashboard, home, file, notes, assistant, settings, websocket, and admin
routes return a clear `password_change_required` error until the password is
changed.

Changing the password requires the current password and a new valid password.
On success, the server writes the new password hash, clears
`password_change_required`, records `password_changed_at`, revokes any other
sessions for that user, keeps or refreshes the current session, and returns the
normal authenticated state.

## CLI Break-Glass Reset

Add a cloud CLI command:

```bash
hank-remote-cloud users reset-password --email user@example.com --force-change
```

Default behavior generates a strong temporary password and prints it once. The
command stores only the bcrypt hash, sets password reset metadata, revokes all
active sessions, and writes an audit event with actor `cli`.

Operator-supplied passwords are accepted only through stdin:

```bash
printf '%s' "$TEMP_PASSWORD" | hank-remote-cloud users reset-password --email user@example.com --stdin --force-change
```

The command also supports `--admin-only` so recovery scripts can refuse to reset
non-admin accounts.

## Data Model

Add a versioned migration for these `users` columns:

- `password_change_required BOOLEAN NOT NULL DEFAULT FALSE`
- `password_changed_at TIMESTAMP NULL`
- `password_reset_at TIMESTAMP NULL`
- `password_reset_by TEXT NOT NULL DEFAULT ''`

Store changes:

- Extend the domain user model and user scanners.
- Keep existing user creation compatible by defaulting the new fields.
- Add a transactional password update helper.
- Add user-wide session revocation.
- Add APNS cleanup for sessions revoked by password reset.

## API Surface

New or changed routes:

- `POST /v1/home/members/invitations`
  - Existing admin route.
  - Response gains `join_url`.
- `POST /v1/auth/invitations/preview`
  - Public token-backed preview with token in JSON body.
- `POST /v1/auth/invitations/signup`
  - Public token-backed account creation and invitation acceptance.
- `PUT /v1/home/members/{userID}/password`
  - Admin reset route.
  - Body includes temporary password or generate request and
    `password_change_required`.
- `POST /v1/auth/change-password`
  - Authenticated route for required or voluntary password changes.

Existing route kept:

- `POST /v1/home/invitations/accept`
  - Authenticated acceptance for users who already have an account.

## Security Rules

- Registration after first setup remains disabled except through a valid invite.
- Invite signup is token-scoped and email-scoped.
- Admin password reset always revokes the target user's existing sessions.
- Raw passwords and raw invite tokens are never stored.
- Raw passwords, tokens, reset codes, cookies, and authorization headers are
  never logged or included in audit metadata.
- Browser write routes keep same-origin and CSRF protections.
- Forced-password-change users cannot access home data, settings, file
  operations, notes, assistant routes, or app websockets before changing the
  password.
- Password reset audit events identify the actor and target without secret
  values.

## Future Delivery Integrations

Email and SMS delivery should be added behind a small notification interface
after the core flows are implemented and tested. Reasonable provider options:

- Resend or SendGrid for transactional email.
- Twilio Messaging for SMS.
- Twilio Verify for verification-code based flows if the product later needs
  phone or email possession checks.

Those integrations should send invite links or reset links/codes, not reusable
passwords.

## Tests

Add tests for:

- Invite preview with valid, expired, accepted, unknown, and email-mismatch
  cases.
- Invite signup creates user, membership, and session.
- Invite signup rejects existing users and invalid passwords.
- Existing signed-in user can still accept a matching invite.
- Admin reset rejects non-admin callers.
- Admin reset updates password hash and metadata.
- Admin reset with force-change limits routes until password change.
- Admin reset without force-change permits normal login.
- Admin reset revokes existing sessions and APNS devices tied to those sessions.
- Admin reset never logs or audits raw password values.
- CLI reset generates a password, supports stdin, supports `--admin-only`, and
  revokes sessions.
- Last admin remains recoverable through CLI even if dashboard access is lost.

## Validation

Implementation validation should include:

```bash
gofmt -w ./cmd ./internal
go build ./...
go test ./...
make migrate-status
make schema-drift-check
```

If local database checks are unavailable, the implementation report must say so
and name the remaining migration risk.
