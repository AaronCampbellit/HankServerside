# Editable Display Names Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add optional display names that users may edit for themselves and administrators may edit for any current home member, without changing email-based identity.

**Architecture:** Store presentation metadata on `users`, expose it additively through existing user/member payloads, and add one home-scoped member update route with self-or-admin authorization. The People dashboard owns editing; Shell and Dashboard Home consume the additive bootstrap field for display while continuing to show email where identity precision matters.

**Tech Stack:** Go, PostgreSQL migrations, JSON-over-HTTPS, React, TypeScript, Vitest.

## Global Constraints

- Email remains the only login, invitation, recovery, login-rate-limit, external-identity, and audit lookup key.
- Display names are trimmed, non-unique, clearable, and limited to 80 Unicode code points.
- Administrators may edit any current home member; users may edit only themselves.
- A display-name update never changes email, password state, external identities, or sessions.
- Do not commit, push, or deploy without explicit user authorization.

---

### Task 1: Migration and persistence contract

**Files:**
- Create: `internal/migrations/sql/000028_user_display_names.up.sql`
- Create: `internal/migrations/sql/000028_user_display_names.down.sql`
- Modify: `internal/migrations/migrations_test.go`
- Modify: `internal/domain/models.go`
- Modify: `internal/store/store.go`
- Modify: `internal/store/collaboration.go`
- Modify: `internal/store/store_test.go`

**Interfaces:**
- Produces: `domain.User.DisplayName string`
- Produces: `domain.HomeMember.DisplayName string`
- Produces: `(*Store).UpdateUserDisplayName(ctx context.Context, userID, displayName string) error`

- [ ] **Step 1: Write the failing migration test**

Add a migration test that loads migration 28 and requires:

```go
for _, required := range []string{
    "ADD COLUMN IF NOT EXISTS display_name TEXT NOT NULL DEFAULT ''",
    "users_display_name_length_check",
    "char_length(display_name) <= 80",
    "display_name = btrim(display_name)",
} {
    if !strings.Contains(text, required) {
        t.Fatalf("migration 28 missing %q in:\n%s", required, text)
    }
}
```

- [ ] **Step 2: Run the migration test and verify RED**

Run:

```bash
go test ./internal/migrations -run TestUserDisplayNameMigrationDefinesPresentationOnlyField -count=1
```

Expected: failure because migration 28 does not exist.

- [ ] **Step 3: Add the migration**

Use an additive column and database constraints:

```sql
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS display_name TEXT NOT NULL DEFAULT '';

ALTER TABLE users
    ADD CONSTRAINT users_display_name_length_check
    CHECK (char_length(display_name) <= 80);

ALTER TABLE users
    ADD CONSTRAINT users_display_name_trimmed_check
    CHECK (display_name = btrim(display_name));
```

The down migration drops both constraints and then the column.

- [ ] **Step 4: Verify the migration test GREEN**

Run the same focused migration test and require PASS.

- [ ] **Step 5: Write failing store behavior tests**

Add PostgreSQL-backed tests proving:

```go
user := domain.User{
    ID: "usr_display_name", Email: "display@example.com",
    DisplayName: "Original Name", PasswordHash: "hash",
    CreatedAt: now, UpdatedAt: now,
}
mustStore(t, db.CreateUser(ctx, user))
mustStore(t, db.UpdateUserDisplayName(ctx, user.ID, "Updated Name"))

stored, err := db.GetUserByID(ctx, user.ID)
// Assert DisplayName == "Updated Name" and Email == "display@example.com".
```

Also list the home members and assert the same display name appears in the
member payload, then clear it with `UpdateUserDisplayName(..., "")`.

- [ ] **Step 6: Run store tests and verify RED**

Run:

```bash
go test ./internal/store -run 'Test(UserDisplayName|HomeMemberDisplayName)' -count=1
```

Expected: compile or assertion failure because the field and update method do
not exist.

- [ ] **Step 7: Implement domain and store support**

Add `DisplayName string \`json:"display_name"\`` to `User` and `HomeMember`.
Update every user insert/select/scan path and the member list query. Implement
the narrow update:

```go
func (s *Store) UpdateUserDisplayName(ctx context.Context, userID, displayName string) error {
    result, err := s.exec(ctx,
        `UPDATE users SET display_name = ?, updated_at = ? WHERE id = ?`,
        displayName, time.Now().UTC(), userID,
    )
    // Return ErrNotFound when RowsAffected is zero.
}
```

- [ ] **Step 8: Run focused migration and store tests GREEN**

Require both focused commands to pass.

---

### Task 2: Home-scoped API and security boundary

**Files:**
- Modify: `internal/cloud/collaboration_handlers.go`
- Modify: `internal/cloud/collaboration_test.go`
- Modify: `internal/cloud/audit.go`
- Modify: `internal/cloud/server_test.go`

**Interfaces:**
- Consumes: `UpdateUserDisplayName`
- Produces: `PUT /v1/home/members/{userID}/display-name`
- Produces: audit event `user.display_name_updated`

- [ ] **Step 1: Write failing authorization and validation tests**

Create an owner, two members, an outsider, sessions, and one home. Assert:

```go
requestJSON(t, server, "member-token", http.MethodPut,
    "/v1/home/members/usr_member/display-name",
    map[string]any{"display_name": "  Member Name  "}, &updated)
// 200, stored "Member Name"

requestJSON(t, server, "owner-token", http.MethodPut,
    "/v1/home/members/usr_other/display-name",
    map[string]any{"display_name": "Other Name"}, &updated)
// 200

requestJSONStatus(t, server, "member-token", http.MethodPut,
    "/v1/home/members/usr_other/display-name",
    map[string]any{"display_name": "Forbidden"}, http.StatusForbidden)

requestJSONStatus(t, server, "owner-token", http.MethodPut,
    "/v1/home/members/usr_outsider/display-name",
    map[string]any{"display_name": "Missing"}, http.StatusNotFound)
```

Add cases for 81 Unicode code points returning 400, empty-string clearing, and
malformed JSON returning 400.

- [ ] **Step 2: Run the focused cloud test and verify RED**

Run:

```bash
go test ./internal/cloud -run TestHomeMemberDisplayNameAuthorization -count=1
```

Expected: 404 or 405 because the route does not exist.

- [ ] **Step 3: Implement the route**

In the existing Home Members router:

```go
if len(parts) == 3 && parts[0] == "members" &&
    parts[2] == "display-name" && r.Method == http.MethodPut {
    targetUserID := parts[1]
    if targetUserID != membership.UserID && membership.Role != domain.HomeRoleAdmin {
        http.Error(w, "admin role required to rename another member", http.StatusForbidden)
        return true
    }
    if _, err := s.store.GetHomeMembership(r.Context(), home.ID, targetUserID); err != nil {
        // Map ErrNotFound to 404.
    }
    var body struct {
        DisplayName string `json:"display_name"`
    }
    // parseJSON, strings.TrimSpace, utf8.RuneCountInString <= 80.
    // Update, reload from ListHomeMembers or a scoped member lookup, audit,
    // emitMembersChanged, return updated member.
}
```

Audit metadata contains no display name:

```go
s.audit(ctx, "user.display_name_updated", auditSeverityInfo,
    membership.UserID, "", home.ID, requestID, "user", targetUserID, nil)
```

Add helper text: “A member display name was updated.”

- [ ] **Step 4: Add unchanged-email authentication proof**

After a display-name update, assert password login still succeeds using the
original email and that submitting the display name in the `email` field does
not authenticate.

- [ ] **Step 5: Run focused cloud tests GREEN**

Run the display-name and existing login/audit tests. Require PASS.

---

### Task 3: Dashboard client and People editing

**Files:**
- Modify: `web/dashboard/src/api/bootstrap.ts`
- Modify: `web/dashboard/src/api/bootstrap.test.ts`
- Modify: `web/dashboard/src/api/people.ts`
- Modify: `web/dashboard/src/api/people.test.ts`
- Create: `web/dashboard/src/settings/PeopleSettings.test.tsx`
- Modify: `web/dashboard/src/settings/PeopleSettings.tsx`
- Modify: `web/dashboard/src/App.tsx`
- Modify: `web/dashboard/src/ui/Shell.tsx`
- Modify: `web/dashboard/src/ui/Shell.test.tsx`
- Modify: `web/dashboard/src/dashboard/DashboardHome.tsx`
- Modify: relevant dashboard tests
- Modify: `web/dashboard/src/styles.css` only if the existing form classes are insufficient

**Interfaces:**
- Produces: `BootstrapUser.display_name`
- Produces: `HomeMember.display_name`
- Produces: `PeopleClient.updateDisplayName(userID, displayName)`

- [ ] **Step 1: Write failing API client tests**

Add:

```ts
await client.updateDisplayName("usr_member", "Member Name");

expect(calls).toContainEqual({
  path: "/v1/home/members/usr_member/display-name",
  method: "PUT",
  body: { display_name: "Member Name" },
});
```

Extend bootstrap/member fixture types with `display_name`.

- [ ] **Step 2: Run API tests and verify RED**

Run:

```bash
npm --prefix web/dashboard test -- --run src/api/people.test.ts src/api/bootstrap.test.ts
```

Expected: missing method/type failures.

- [ ] **Step 3: Implement additive client types and request**

```ts
updateDisplayName(userID: string, displayName: string) {
  return this.api.request<HomeMember>(
    `/v1/home/members/${encodeURIComponent(userID)}/display-name`,
    { method: "PUT", body: { display_name: displayName } },
  );
}
```

- [ ] **Step 4: Write failing People UI tests**

Mock complete bootstrap and people payloads. Assert:

- display name is the primary label and email remains visible;
- an administrator sees Edit name for every member;
- a member sees Edit name for self only;
- Save sends the target ID and typed value, then shows the updated result;
- clearing restores the email fallback;
- an API validation error is visible and leaves the form editable.

- [ ] **Step 5: Run People UI tests and verify RED**

Run the new `PeopleSettings.test.tsx`; expect missing controls.

- [ ] **Step 6: Implement People editing**

Track one edit draft:

```ts
type NameEdit = { member: HomeMember; value: string };
```

Render the primary label with:

```ts
const memberLabel = member.display_name.trim() || member.email;
```

Show the edit action when:

```ts
const mayEditName = canManage || isSelf;
```

Save through `peopleClient.updateDisplayName`, refresh the People payload, and
keep email visible as the secondary line.

- [ ] **Step 7: Update signed-in presentation surfaces**

Pass `bootstrap.user.display_name` to Shell and use it for the footer label,
while retaining email as the title/secondary identity. Use the display name in
the Dashboard Home greeting and email in “Signed in as” service metadata.

- [ ] **Step 8: Run focused frontend tests GREEN**

Run People API/UI, bootstrap, Shell, App, and Dashboard Home tests. Require PASS.

---

### Task 4: Documentation and complete verification

**Files:**
- Modify: `docs/app-integration/hank-app-home-sync-checklist.md`
- Modify: relevant API/deployment documentation if member payloads are documented there

- [ ] **Step 1: Document the additive contract**

Document `display_name` on user/member payloads and the update route. State
explicitly that email remains the authentication, invitation, recovery,
rate-limit, and audit identity.

- [ ] **Step 2: Run formatting and focused gates**

Run:

```bash
gofmt -w ./cmd ./internal
go test ./internal/migrations ./internal/store ./internal/cloud
npm --prefix web/dashboard test -- --run
git diff --check
```

- [ ] **Step 3: Run full repository gates**

Run:

```bash
go test ./...
make build
make frontend-check
```

Use an explicit timeout for the full Go suite only if the known
`internal/cloud` hang recurs, preserving the package-specific result.

- [ ] **Step 4: Validate PostgreSQL and deployment behavior on the demo server**

After explicit deployment authorization, deploy the exact pushed commit, apply
migration 28, and run:

```bash
make migrate-status
make schema-drift-check
scripts/doctor.sh
```

Prove self update, administrator update, cross-member denial, email login, and
fresh `/readyz` version/assets through the public demo URL.

- [ ] **Step 5: Update the Hank Build card**

Append outcome and verification work logs. Move the card to Done only after
the migration and live API behavior pass; otherwise move completed work to
Review with the exact remaining validation.
