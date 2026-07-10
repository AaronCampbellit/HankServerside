# File Quick Links Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let dashboard Quick Links open Hank File Server folders and preview individual SMB-backed files, including `.html`, through authenticated Hank routes.

**Architecture:** Keep this as a small dashboard/file-preview feature. Quick Links continue storing one `url` string, but now allow same-origin `/dashboard/file-server?...` URLs in addition to external `http` and `https` URLs. File Server deep links read `source_id`, `path`, and `preview=1`; folder links open that folder, while file links open the parent folder and select the file in the existing preview panel.

**Tech Stack:** Go cloud handlers/tests, React/Vite dashboard, existing WebSocket file commands, existing `/v1/home/files/preview` HTTP stream.

## Global Constraints

- Do not accept raw `smb://` URLs.
- Do not expose SMB directly to the internet.
- Do not add a database migration.
- Do not add a new protocol command.
- Keep SMB credentials and local file access inside the home agent.
- HTML previews must render inside a sandboxed iframe.
- If any step requires a new persistence model or cross-agent protocol change, stop and report that this is no longer a quickie feature.

---

## File Structure

- `internal/cloud/quick_links.go`: accept and classify same-origin dashboard Quick Link URLs.
- `internal/cloud/quick_links_test.go`: unit coverage for Quick Link URL normalization.
- `internal/cloud/server.go`: add iframe safety headers for HTML preview responses.
- `internal/cloud/server_test.go`: assert HTML preview response remains inline and carries sandbox safety headers.
- `web/dashboard/src/api/quickLinks.ts`: no type changes expected.
- `web/dashboard/src/dashboard/DashboardHome.tsx`: open internal quick links in-app and external links in a new tab.
- `web/dashboard/src/settings/QuickLinksSettings.tsx`: allow relative dashboard URLs in the URL input.
- `web/dashboard/src/dashboard/FileServerPage.tsx`: build/copy file links and consume `source_id`, `path`, and `preview=1` from the URL.
- `web/dashboard/src/dashboard/FileServerPage.test.tsx`: cover folder deep links, file preview deep links, and copy-link actions.
- `web/dashboard/src/App.test.tsx`: cover internal Quick Links on the dashboard/settings pages.

---

### Task 1: Allow Internal File Server Quick Link URLs

**Files:**
- Modify: `internal/cloud/quick_links.go`
- Create: `internal/cloud/quick_links_test.go`
- Test: `internal/cloud/quick_links_test.go`

**Interfaces:**
- Consumes: `normalizedQuickLink(body quickLinkRequest, existing domain.HomeQuickLink) (domain.HomeQuickLink, error)`
- Produces: `isInternalDashboardQuickLink(rawURL string) bool`

- [ ] **Step 1: Write the failing tests**

Create `internal/cloud/quick_links_test.go`:

```go
package cloud

import (
	"testing"

	"github.com/dropfile/hankremote/internal/domain"
)

func TestNormalizedQuickLinkAllowsDashboardFileServerURL(t *testing.T) {
	enabled := true
	link, err := normalizedQuickLink(quickLinkRequest{
		Title:              "Recipes",
		URL:                "/dashboard/file-server?source_id=media&path=%2FRecipes&preview=1",
		Description:        "Recipe index",
		HealthCheckEnabled: &enabled,
	}, domain.HomeQuickLink{})
	if err != nil {
		t.Fatalf("normalizedQuickLink returned error: %v", err)
	}
	if link.URL != "/dashboard/file-server?source_id=media&path=%2FRecipes&preview=1" {
		t.Fatalf("URL = %q", link.URL)
	}
	if link.HealthCheckEnabled {
		t.Fatalf("internal dashboard links should not keep external health checks enabled")
	}
	if link.Status != domain.QuickLinkStatusDisabled {
		t.Fatalf("Status = %q, want disabled", link.Status)
	}
}

func TestNormalizedQuickLinkRejectsRawSMBURL(t *testing.T) {
	_, err := normalizedQuickLink(quickLinkRequest{
		Title: "Raw SMB",
		URL:   "smb://nas.local/media/index.html",
	}, domain.HomeQuickLink{})
	if err == nil {
		t.Fatal("normalizedQuickLink accepted raw SMB URL")
	}
}

func TestNormalizedQuickLinkRejectsOtherRelativeURL(t *testing.T) {
	_, err := normalizedQuickLink(quickLinkRequest{
		Title: "Unsafe relative",
		URL:   "/v1/home/files/preview?path=%2Findex.html",
	}, domain.HomeQuickLink{})
	if err == nil {
		t.Fatal("normalizedQuickLink accepted non-dashboard relative URL")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/cloud -run 'TestNormalizedQuickLink(AllowsDashboardFileServerURL|RejectsRawSMBURL|RejectsOtherRelativeURL)' -count=1
```

Expected: the dashboard file-server URL test fails because current validation requires `http` or `https`.

- [ ] **Step 3: Implement minimal backend validation**

In `internal/cloud/quick_links.go`, add helpers near `normalizedQuickLink`:

```go
func isInternalDashboardQuickLink(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	if parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil {
		return false
	}
	return parsed.Path == "/dashboard/file-server"
}
```

Then replace the URL validation block in `normalizedQuickLink` with:

```go
internalDashboardLink := isInternalDashboardQuickLink(link.URL)
var parsed *url.URL
if internalDashboardLink {
	parsed, _ = url.Parse(link.URL)
} else {
	var err error
	parsed, err = url.Parse(link.URL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return domain.HomeQuickLink{}, fmt.Errorf("valid http, https, or dashboard file-server url is required")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return domain.HomeQuickLink{}, fmt.Errorf("valid http, https, or dashboard file-server url is required")
	}
	if parsed.User != nil {
		return domain.HomeQuickLink{}, fmt.Errorf("url credentials are not allowed")
	}
}
if link.Title == "" {
	if internalDashboardLink {
		link.Title = "File Server"
	} else {
		link.Title = parsed.Hostname()
	}
}
```

And force health checks off for internal dashboard links before the disabled-status branch:

```go
if internalDashboardLink {
	healthEnabled = false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:

```bash
go test ./internal/cloud -run 'TestNormalizedQuickLink(AllowsDashboardFileServerURL|RejectsRawSMBURL|RejectsOtherRelativeURL)' -count=1
```

Expected: PASS.

---

### Task 2: Make Quick Links Treat Internal URLs As In-App Links

**Files:**
- Modify: `web/dashboard/src/dashboard/DashboardHome.tsx`
- Modify: `web/dashboard/src/settings/QuickLinksSettings.tsx`
- Modify: `web/dashboard/src/App.test.tsx`
- Test: `web/dashboard/src/App.test.tsx`

**Interfaces:**
- Consumes: `HomeQuickLink.url`
- Produces: dashboard links where internal `/dashboard/...` URLs do not use `target="_blank"`

- [ ] **Step 1: Write failing frontend tests**

In the dashboard Quick Links test in `web/dashboard/src/App.test.tsx`, add an internal link to the mocked Quick Link payload:

```ts
{
  id: "ql_4",
  home_id: "home_1",
  title: "Recipes",
  url: "/dashboard/file-server?source_id=media&path=%2FRecipes&preview=1",
  description: "Recipe index",
  sort_order: 3,
  health_check_enabled: false,
  status: "disabled",
  status_code: 0,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  updated_by: "usr_1",
}
```

Add assertions after `const quickLinks = screen.getByRole("region", { name: "Quick links" });`:

```ts
const recipesLink = within(quickLinks).getByRole("link", { name: "Recipes" });
expect(recipesLink).toHaveAttribute("href", "/dashboard/file-server?source_id=media&path=%2FRecipes&preview=1");
expect(recipesLink).not.toHaveAttribute("target");
expect(recipesLink).not.toHaveAttribute("rel");
```

In the Quick Links settings test, change the add-link form URL value to an internal URL and expected body:

```ts
fireEvent.change(screen.getByLabelText("URL"), {
  target: { value: "/dashboard/file-server?source_id=media&path=%2FRecipes&preview=1" },
});
```

Expected body URL:

```ts
url: "/dashboard/file-server?source_id=media&path=%2FRecipes&preview=1",
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
npm --prefix web/dashboard test -- App.test.tsx -t 'dashboard bootstrap and quick links|manages quick links'
```

Expected: settings form rejects the relative URL because the input uses `type="url"`, or dashboard internal link still gets external-link attributes.

- [ ] **Step 3: Implement internal link detection in dashboard**

In `web/dashboard/src/dashboard/DashboardHome.tsx`, add:

```ts
function isExternalURL(url: string): boolean {
  return /^https?:\/\//i.test(url);
}
```

Change the `quickLinkCards` mapping:

```ts
const quickLinkCards = quickLinks?.links.length ? quickLinks.links.map((link) => {
  const external = isExternalURL(link.url);
  return {
    id: link.id,
    title: link.title,
    detail: link.description || link.url,
    href: link.url,
    external,
    status: statusLabel(link.status),
    statusTone: quickLinkStatusTone(link.status),
  };
}) : [];
```

- [ ] **Step 4: Allow relative dashboard URLs in settings form**

In `web/dashboard/src/settings/QuickLinksSettings.tsx`, change the URL input type from:

```tsx
type="url"
```

to:

```tsx
type="text"
inputMode="url"
```

Keep `required` and `maxLength={2048}` unchanged.

- [ ] **Step 5: Run tests to verify they pass**

Run:

```bash
npm --prefix web/dashboard test -- App.test.tsx -t 'dashboard bootstrap and quick links|manages quick links'
```

Expected: PASS.

---

### Task 3: Add File Server Deep Links And Copy Link Actions

**Files:**
- Modify: `web/dashboard/src/dashboard/FileServerPage.tsx`
- Modify: `web/dashboard/src/dashboard/FileServerPage.test.tsx`
- Test: `web/dashboard/src/dashboard/FileServerPage.test.tsx`

**Interfaces:**
- Consumes URL query params: `source_id`, `path`, `preview`
- Produces copied URLs:
  - folder: `/dashboard/file-server?source_id=<id>&path=<folder>`
  - file: `/dashboard/file-server?source_id=<id>&path=<file>&preview=1`

- [ ] **Step 1: Write failing deep-link tests**

In `web/dashboard/src/dashboard/FileServerPage.test.tsx`, add a folder deep-link test:

```ts
it("loads a folder deep link with source id", async () => {
  window.history.pushState({}, "", "/dashboard/file-server?source_id=hankdemo2&path=%2FMedia%2FPhotos");

  render(<FileServerPage />);

  await waitFor(() => expect(fileServerClient.list).toHaveBeenCalledWith("/Media/Photos", "hankdemo2"));
});
```

Add a file preview deep-link test:

```ts
it("opens a file preview deep link from its parent folder", async () => {
  window.history.pushState({}, "", "/dashboard/file-server?source_id=hankdemo2&path=%2FMedia%2FPhotos%2Fsunset-beach.jpg&preview=1");

  render(<FileServerPage />);

  await waitFor(() => expect(fileServerClient.list).toHaveBeenCalledWith("/Media/Photos", "hankdemo2"));
  const preview = await screen.findByRole("complementary", { name: "File preview" });
  expect(within(preview).getByRole("img", { name: "Preview sunset-beach.jpg" })).toHaveAttribute(
    "src",
    "/v1/home/files/preview?source_id=hankdemo2&path=%2FMedia%2FPhotos%2Fsunset-beach.jpg",
  );
});
```

Add a copy-link action test:

```ts
it("copies file and folder dashboard links", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.assign(navigator, { clipboard: { writeText } });
  window.history.pushState({}, "", "/dashboard/file-server");

  render(<FileServerPage />);

  const photosMenu = await screen.findByRole("button", { name: "More actions for Photos" });
  fireEvent.click(photosMenu);
  fireEvent.click(screen.getByRole("menuitem", { name: /Copy link/i }));
  expect(writeText).toHaveBeenCalledWith(expect.stringContaining("/dashboard/file-server?source_id=hankdemo2&path=%2FMedia%2FPhotos"));

  const imageMenu = await screen.findByRole("button", { name: "More actions for sunset-beach.jpg" });
  fireEvent.click(imageMenu);
  fireEvent.click(screen.getByRole("menuitem", { name: /Copy preview link/i }));
  expect(writeText).toHaveBeenCalledWith(expect.stringContaining("/dashboard/file-server?source_id=hankdemo2&path=%2FMedia%2FPhotos%2Fsunset-beach.jpg&preview=1"));
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
npm --prefix web/dashboard test -- FileServerPage.test.tsx -t 'deep link|copies file and folder dashboard links'
```

Expected: deep-link source ID and preview selection fail; copy-link menu item is missing.

- [ ] **Step 3: Implement URL parsing helpers**

In `web/dashboard/src/dashboard/FileServerPage.tsx`, replace `initialPathFromLocation` with:

```ts
function normalizedDashboardPath(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed || trimmed === ".") return "/";
  return trimmed.startsWith("/") ? trimmed : `/${trimmed}`;
}

function parentPathForFile(path: string): string {
  const parts = normalizedDashboardPath(path).split("/").filter(Boolean);
  parts.pop();
  return parts.length ? `/${parts.join("/")}` : "/";
}

function initialLinkFromLocation(): { sourceID: string; folderPath: string; previewPath: string; previewOpen: boolean } {
  const params = new URLSearchParams(window.location.search);
  const sourceID = (params.get("source_id") || "").trim();
  const path = normalizedDashboardPath(params.get("path") || "/");
  const previewOpen = params.get("preview") === "1";
  return {
    sourceID,
    folderPath: previewOpen ? parentPathForFile(path) : path,
    previewPath: previewOpen ? path : "",
    previewOpen,
  };
}

function dashboardFileLink(item: FileMeta, sourceID: string): string {
  const params = new URLSearchParams();
  if (sourceID) params.set("source_id", sourceID);
  params.set("path", item.path);
  if (!item.is_directory) params.set("preview", "1");
  return `${window.location.origin}/dashboard/file-server?${params.toString()}`;
}
```

- [ ] **Step 4: Use deep-link state during initial load**

In `FileServerPage`, initialize from `initialLinkFromLocation()`:

```ts
const initialLink = initialLinkFromLocation();
const [state, setState] = useState<State>({ status: "loading", path: initialLink.folderPath });
const [activeSourceID, setActiveSourceID] = useState(initialLink.sourceID);
```

Update the initial effect:

```ts
useEffect(() => {
  void load(initialLink.folderPath, "", initialLink.sourceID, initialLink.previewPath, initialLink.previewOpen);
  void loadTransferJobs();
  // Initial load only.
  // eslint-disable-next-line react-hooks/exhaustive-deps
}, []);
```

Change `load` signature and preview selection:

```ts
async function load(path = state.path, message = "", sourceID = activeSourceID, requestedPreviewPath = "", requestedPreviewOpen = false) {
```

Inside the successful `setState`, compute:

```ts
const requestedPreview = requestedPreviewPath && items.some((item) => item.path === requestedPreviewPath)
  ? requestedPreviewPath
  : "";
```

Then set:

```ts
previewPath: requestedPreview || (current.status === "ready" && items.some((item) => item.path === current.previewPath) ? current.previewPath : defaultPreview),
previewOpen: requestedPreview ? true : current.status === "ready" ? current.previewOpen : requestedPreviewOpen || true,
```

- [ ] **Step 5: Add copy-link action**

Add a function in `FileServerPage`:

```ts
async function copyDashboardLink(item: FileMeta) {
  const link = dashboardFileLink(item, commandSourceID);
  try {
    await navigator.clipboard.writeText(link);
    showToast(item.is_directory ? "Folder link copied." : "Preview link copied.");
  } catch (error) {
    showToast(errorMessage(error), "error");
  }
}
```

Pass it into `FileActionMenu`:

```tsx
onCopyLink={(item) => void copyDashboardLink(item)}
```

Update `FileActionMenu` props and menu body:

```tsx
onCopyLink,
}: {
  item?: FileMeta;
  onClose: () => void;
  onDownload: (item: FileMeta) => void;
  onDelete: (item: FileMeta) => void;
  onMove: (item: FileMeta) => void;
  onOpen: (item: FileMeta) => void;
  onRename: (item: FileMeta) => void;
  onCopyLink: (item: FileMeta) => void;
}) {
```

Add the menu item after Open:

```tsx
<button role="menuitem" type="button" onClick={() => { onCopyLink(item); onClose(); }}>
  <Icon name="file" />{item.is_directory ? "Copy link" : "Copy preview link"}
</button>
```

- [ ] **Step 6: Run tests to verify they pass**

Run:

```bash
npm --prefix web/dashboard test -- FileServerPage.test.tsx -t 'deep link|copies file and folder dashboard links'
```

Expected: PASS.

---

### Task 4: Render HTML Previews In A Sandboxed Iframe

**Files:**
- Modify: `web/dashboard/src/dashboard/FileServerPage.tsx`
- Modify: `internal/cloud/server.go`
- Modify: `internal/cloud/server_test.go`
- Test: `web/dashboard/src/dashboard/FileServerPage.test.tsx`
- Test: `internal/cloud/server_test.go`

**Interfaces:**
- Consumes: `/v1/home/files/preview?source_id=<id>&path=<file.html>`
- Produces: iframe preview for `.html` and `.htm`

- [ ] **Step 1: Write failing tests**

In `web/dashboard/src/dashboard/FileServerPage.test.tsx`, add:

```ts
it("renders html files in a sandboxed preview iframe", async () => {
  window.history.pushState({}, "", "/dashboard/file-server?source_id=hankdemo2&path=%2FMedia%2FDocs%2Findex.html&preview=1");

  render(<FileServerPage />);

  const preview = await screen.findByRole("complementary", { name: "File preview" });
  const frame = within(preview).getByTitle("Preview index.html");
  expect(frame).toHaveAttribute("src", "/v1/home/files/preview?source_id=hankdemo2&path=%2FMedia%2FDocs%2Findex.html");
  expect(frame).toHaveAttribute("sandbox", "");
});
```

In `internal/cloud/server_test.go`, add a focused assertion to an HTML preview test or create:

```go
func TestFilePreviewHTMLUsesSandboxHeaders(t *testing.T) {
	t.Parallel()

	if got := previewContentSecurityPolicy(previewContentType("docs/index.html")); got != "sandbox" {
		t.Fatalf("html preview CSP = %q, want sandbox", got)
	}
	if got := previewContentSecurityPolicy(previewContentType("docs/readme.pdf")); got != "" {
		t.Fatalf("pdf preview CSP = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
npm --prefix web/dashboard test -- FileServerPage.test.tsx -t 'html files'
go test ./internal/cloud -run 'TestFilePreviewHTMLUsesSandboxHeaders|TestFilePreviewStreamsInlineRangeOverHTTP' -count=1
```

Expected: frontend HTML iframe test fails because only PDFs iframe today; backend sandbox header assertion fails until added.

- [ ] **Step 3: Add HTML file detection and iframe rendering**

In `web/dashboard/src/dashboard/FileServerPage.tsx`, add near `isPDFFile`:

```ts
function isHTMLFile(item: FileMeta): boolean {
  const name = fileName(item).toLowerCase();
  return name.endsWith(".html") || name.endsWith(".htm");
}
```

Change preview rendering:

```tsx
) : isPDFFile(previewItem) || isHTMLFile(previewItem) ? (
  <iframe
    sandbox={isHTMLFile(previewItem) ? "" : undefined}
    src={previewURL(previewItem, commandSourceID)}
    title={`Preview ${fileName(previewItem)}`}
  />
) : (
```

- [ ] **Step 4: Add defensive sandbox header for HTML preview responses**

In `internal/cloud/server.go`, after setting preview response headers in `handleFilePreviewStream`, add:

```go
func previewContentSecurityPolicy(contentType string) string {
	if strings.HasPrefix(contentType, "text/html") {
		return "sandbox"
	}
	return ""
}
```

Then assign the preview content type once inside `handleFilePreviewStream`:

```go
contentType := previewContentType(path)
w.Header().Set("Content-Type", contentType)
if policy := previewContentSecurityPolicy(contentType); policy != "" {
	w.Header().Set("Content-Security-Policy", policy)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run:

```bash
npm --prefix web/dashboard test -- FileServerPage.test.tsx -t 'html files'
go test ./internal/cloud -run 'TestFilePreviewHTMLUsesSandboxHeaders|TestFilePreviewStreamsInlineRangeOverHTTP' -count=1
```

Expected: PASS.

---

### Task 5: Final Verification And Throwaway Gate

**Files:**
- Modify only if earlier tests expose a small missing assertion.

**Interfaces:**
- Consumes all prior tasks.
- Produces a verified quick feature or a clear stop decision.

- [ ] **Step 1: Run targeted backend checks**

Run:

```bash
gofmt -w internal/cloud/quick_links.go internal/cloud/quick_links_test.go internal/cloud/server.go internal/cloud/server_test.go
go test ./internal/cloud -run 'TestNormalizedQuickLink|TestFilePreview' -count=1
```

Expected: PASS.

- [ ] **Step 2: Run targeted dashboard checks**

Run:

```bash
npm --prefix web/dashboard test -- App.test.tsx FileServerPage.test.tsx
```

Expected: PASS.

- [ ] **Step 3: Build dashboard assets**

Run:

```bash
npm --prefix web/dashboard run build
```

Expected: PASS and `internal/cloud/ui/react` assets updated.

- [ ] **Step 4: Run broad cheap checks if the environment supports them**

Run:

```bash
go test ./internal/cloud ./internal/protocol ./internal/agent -count=1
git diff --check
```

Expected: PASS.

- [ ] **Step 5: Stop instead of expanding scope if any of these happen**

Stop and report the feature is no longer a quickie if implementation needs:

- A new database column or migration.
- A new cloud-agent protocol command.
- Raw `smb://` parsing outside the agent.
- Public unauthenticated file-preview links.
- Custom HTML rewriting, proxying subresources, or script permission exceptions.
- A new file permission model beyond existing `files.download` policy checks.
