package files

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dropfile/hankremote/internal/protocol"
)

func TestUploadListDownloadAndBlockEscape(t *testing.T) {
	t.Parallel()

	service := New(t.TempDir())
	content := base64.StdEncoding.EncodeToString([]byte("hello"))

	if err := service.Upload(context.Background(), "docs/hello.txt", content); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	items, err := service.List(context.Background(), "docs")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Name != "hello.txt" {
		t.Fatalf("List items = %#v, want one hello.txt item", items)
	}

	downloaded, err := service.Download(context.Background(), "docs/hello.txt")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if downloaded != content {
		t.Fatalf("Download content = %q, want %q", downloaded, content)
	}

	if _, err := service.Download(context.Background(), "../secret.txt"); err == nil {
		t.Fatal("Download escape: expected error")
	}
}

func TestLocalSymlinkEscapeBlocked(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("write outside secret: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside-link")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	service := New(root)
	content := base64.StdEncoding.EncodeToString([]byte("overwrite"))
	if _, err := service.Download(context.Background(), "outside-link/secret.txt"); err == nil {
		t.Fatal("Download through symlink escape succeeded, want error")
	}
	if err := service.Upload(context.Background(), "outside-link/new.txt", content); err == nil {
		t.Fatal("Upload through symlink escape succeeded, want error")
	}
}

func TestLocalSourcePolicyDeniesPrefixesDeleteAndMaxUpload(t *testing.T) {
	t.Parallel()

	allow := true
	deny := false
	service := NewWithConfig(Config{
		Root: t.TempDir(),
		Policy: AccessPolicy{
			Read:            &allow,
			Write:           &allow,
			Delete:          &deny,
			AllowedPrefixes: []string{"/Allowed"},
			BlockedPrefixes: []string{"/Allowed/Private"},
			MaxUploadBytes:  4,
		},
	})

	if err := service.Upload(context.Background(), "/Allowed/ok.txt", base64.StdEncoding.EncodeToString([]byte("1234"))); err != nil {
		t.Fatalf("Upload allowed: %v", err)
	}
	if _, err := service.List(context.Background(), "/Allowed/Private"); err == nil {
		t.Fatal("List blocked prefix succeeded, want error")
	}
	if err := service.Delete(context.Background(), "/Allowed/ok.txt", false); err == nil {
		t.Fatal("Delete with disabled policy succeeded, want error")
	}
	if err := service.Upload(context.Background(), "/Allowed/too-large.txt", base64.StdEncoding.EncodeToString([]byte("12345"))); err == nil {
		t.Fatal("Upload over policy size limit succeeded, want error")
	}
	writer, _, err := service.OpenWriter(context.Background(), "/Allowed/stream.txt", 0)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if _, err := writer.Write([]byte("12345")); err == nil {
		t.Fatal("stream upload over policy size limit succeeded, want error")
	}
	_ = writer.Close()
}

func TestLocalSourcePolicyDefaultAllowsDelete(t *testing.T) {
	t.Parallel()

	service := New(t.TempDir())
	if err := service.Upload(context.Background(), "/ok.txt", base64.StdEncoding.EncodeToString([]byte("ok"))); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if err := service.Delete(context.Background(), "/ok.txt", false); err != nil {
		t.Fatalf("Delete with default policy: %v", err)
	}
}

func TestMoveFailureBeforeDeleteKeepsSourceAndChecksumMismatchFails(t *testing.T) {
	t.Parallel()

	service := New(t.TempDir())
	ctx := context.Background()
	sourcePayload := base64.StdEncoding.EncodeToString([]byte("source"))
	destinationPayload := base64.StdEncoding.EncodeToString([]byte("target"))
	if err := service.Upload(ctx, "/source.txt", sourcePayload); err != nil {
		t.Fatalf("Upload source: %v", err)
	}
	if err := service.Upload(ctx, "/dest.txt", destinationPayload); err != nil {
		t.Fatalf("Upload dest: %v", err)
	}
	if err := service.verifyCopiedFile(ctx, LocalSourceID, LocalSourceID, "/source.txt", "/dest.txt"); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("verifyCopiedFile error = %v, want checksum mismatch", err)
	}
	if err := service.MoveBetweenSources(ctx, LocalSourceID, "missing-source", "/source.txt", "/moved.txt", false); err == nil {
		t.Fatal("MoveBetweenSources to missing source succeeded, want error")
	}
	downloaded, err := service.Download(ctx, "/source.txt")
	if err != nil {
		t.Fatalf("Download source after failed move: %v", err)
	}
	if downloaded != sourcePayload {
		t.Fatalf("source after failed move = %q, want original payload", downloaded)
	}
}

func TestMoveRejectsSameLocationAndNestedDirectoryDestination(t *testing.T) {
	t.Parallel()

	service := New(t.TempDir())
	ctx := context.Background()
	payload := base64.StdEncoding.EncodeToString([]byte("source"))
	if err := service.Upload(ctx, "/folder/source.txt", payload); err != nil {
		t.Fatalf("Upload source: %v", err)
	}
	if err := service.CreateDirectory(ctx, "/folder/subdir"); err != nil {
		t.Fatalf("CreateDirectory: %v", err)
	}

	if err := service.MoveBetweenSources(ctx, LocalSourceID, LocalSourceID, "/folder/source.txt", "/folder/source.txt", false); err == nil || !strings.Contains(err.Error(), "already at destination") {
		t.Fatalf("same-location file move error = %v, want already at destination", err)
	}
	if err := service.MoveBetweenSources(ctx, LocalSourceID, LocalSourceID, "/folder", "/folder/subdir/folder", true); err == nil || !strings.Contains(err.Error(), "inside itself") {
		t.Fatalf("nested directory move error = %v, want inside itself", err)
	}
	downloaded, err := service.Download(ctx, "/folder/source.txt")
	if err != nil {
		t.Fatalf("Download source after rejected move: %v", err)
	}
	if downloaded != payload {
		t.Fatalf("source after rejected move = %q, want original payload", downloaded)
	}
}

func TestRollbackMoveDestinationDeletesCopiedDestination(t *testing.T) {
	t.Parallel()

	service := New(t.TempDir())
	ctx := context.Background()
	payload := base64.StdEncoding.EncodeToString([]byte("copied"))
	if err := service.Upload(ctx, "/dest/copied.txt", payload); err != nil {
		t.Fatalf("Upload copied destination: %v", err)
	}

	if err := service.RollbackMoveDestination(ctx, LocalSourceID, "/dest/copied.txt", false); err != nil {
		t.Fatalf("RollbackMoveDestination: %v", err)
	}
	if _, err := service.Download(ctx, "/dest/copied.txt"); err == nil {
		t.Fatal("destination still downloads after rollback")
	}
	if err := service.RollbackMoveDestination(ctx, LocalSourceID, "/dest/copied.txt", false); err != nil {
		t.Fatalf("RollbackMoveDestination second pass should be idempotent: %v", err)
	}
}

func TestSearchFindsNestedFiles(t *testing.T) {
	t.Parallel()

	service := New(t.TempDir())
	content := base64.StdEncoding.EncodeToString([]byte("hello"))
	if err := service.Upload(context.Background(), "docs/taxes/2025-summary.pdf", content); err != nil {
		t.Fatalf("Upload summary: %v", err)
	}
	if err := service.Upload(context.Background(), "docs/recipes/pasta.txt", content); err != nil {
		t.Fatalf("Upload recipe: %v", err)
	}

	results, err := service.Search(context.Background(), "taxes summary", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search returned no results")
	}
	found := false
	for _, result := range results {
		if result.Path == "docs/taxes/2025-summary.pdf" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Search results = %#v, want docs/taxes/2025-summary.pdf", results)
	}
}

func TestSearchFindsFilesBeyondLegacyTraversalDepth(t *testing.T) {
	t.Parallel()

	service := New(t.TempDir())
	content := base64.StdEncoding.EncodeToString([]byte("hello"))
	path := "one/two/three/four/five/six/needle.txt"
	if err := service.Upload(context.Background(), path, content); err != nil {
		t.Fatalf("Upload nested file: %v", err)
	}

	results, err := service.Search(context.Background(), "needle", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, result := range results {
		if result.Path == path {
			return
		}
	}
	t.Fatalf("Search results = %#v, want %q", results, path)
}

func TestStreamingWriteInvalidatesSearchCacheOnClose(t *testing.T) {
	t.Parallel()

	service := New(t.TempDir())
	ctx := context.Background()
	if err := service.Upload(ctx, "existing.txt", base64.StdEncoding.EncodeToString([]byte("existing"))); err != nil {
		t.Fatalf("Upload existing: %v", err)
	}
	if _, err := service.Search(ctx, "existing", 10); err != nil {
		t.Fatalf("Search existing: %v", err)
	}

	writer, _, err := service.OpenWriter(ctx, "streamed-needle.txt", 0)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if _, err := io.WriteString(writer, "needle"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	results, err := service.Search(ctx, "streamed-needle", 10)
	if err != nil {
		t.Fatalf("Search streamed: %v", err)
	}
	if len(results) != 1 || results[0].Path != "streamed-needle.txt" {
		t.Fatalf("Search results = %#v, want streamed-needle.txt", results)
	}
}

func TestInvalidatedSearchBuildCannotPublishStaleIndex(t *testing.T) {
	t.Parallel()

	service := New(t.TempDir())
	generation := service.searchIndexGeneration(LocalSourceID)
	service.invalidateSearchIndex(LocalSourceID)
	service.storeSearchIndexIfCurrent(LocalSourceID, generation, []protocol.FileItem{{Path: "stale.txt", Name: "stale.txt"}}, time.Now())

	service.searchCacheMu.Lock()
	_, cached := service.searchCache[LocalSourceID]
	service.searchCacheMu.Unlock()
	if cached {
		t.Fatal("stale search index was published after invalidation")
	}
}

func TestSearchDoesNotReturnUnrelatedDirectories(t *testing.T) {
	t.Parallel()

	service := New(t.TempDir())
	content := base64.StdEncoding.EncodeToString([]byte("hello"))
	if err := service.Upload(context.Background(), "docs/taxes/2025-summary.pdf", content); err != nil {
		t.Fatalf("Upload summary: %v", err)
	}
	if err := service.Upload(context.Background(), "docs/recipes/pasta.txt", content); err != nil {
		t.Fatalf("Upload recipe: %v", err)
	}

	results, err := service.Search(context.Background(), "needle-not-present", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("Search returned unrelated results = %#v", results)
	}
}

func TestOpenWriterOffsetZeroTruncatesWithoutStatRequirement(t *testing.T) {
	t.Parallel()

	service := New(t.TempDir())
	writer, size, err := service.OpenWriter(context.Background(), "downloads/movie.mp4.part", 0)
	if err != nil {
		t.Fatalf("OpenWriter create: %v", err)
	}
	if size != 0 {
		t.Fatalf("create size = %d, want 0", size)
	}
	if _, err := io.WriteString(writer, "old-content"); err != nil {
		t.Fatalf("write old content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close old content: %v", err)
	}

	writer, size, err = service.OpenWriter(context.Background(), "downloads/movie.mp4.part", 0)
	if err != nil {
		t.Fatalf("OpenWriter truncate: %v", err)
	}
	if size != 0 {
		t.Fatalf("truncate size = %d, want 0", size)
	}
	if _, err := io.WriteString(writer, "new"); err != nil {
		t.Fatalf("write new content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close new content: %v", err)
	}
	content, err := service.Download(context.Background(), "downloads/movie.mp4.part")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if got := string(mustDecodeBase64(t, content)); got != "new" {
		t.Fatalf("content = %q, want truncated new content", got)
	}
}

func TestOpenRandomWriterWritesAtOffsetsAndTruncates(t *testing.T) {
	t.Parallel()

	service := New(t.TempDir())
	writer, err := service.OpenRandomWriter(context.Background(), "downloads/movie.mp4.part")
	if err != nil {
		t.Fatalf("OpenRandomWriter create: %v", err)
	}
	if _, err := writer.WriteAt([]byte("world"), 6); err != nil {
		t.Fatalf("WriteAt suffix: %v", err)
	}
	if _, err := writer.WriteAt([]byte("hello "), 0); err != nil {
		t.Fatalf("WriteAt prefix: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close random writer: %v", err)
	}

	content, err := service.Download(context.Background(), "downloads/movie.mp4.part")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if got := string(mustDecodeBase64(t, content)); got != "hello world" {
		t.Fatalf("content = %q, want assembled content", got)
	}

	writer, err = service.OpenRandomWriter(context.Background(), "downloads/movie.mp4.part")
	if err != nil {
		t.Fatalf("OpenRandomWriter truncate: %v", err)
	}
	if _, err := writer.WriteAt([]byte("new"), 0); err != nil {
		t.Fatalf("WriteAt new: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close truncated random writer: %v", err)
	}
	content, err = service.Download(context.Background(), "downloads/movie.mp4.part")
	if err != nil {
		t.Fatalf("Download truncated: %v", err)
	}
	if got := string(mustDecodeBase64(t, content)); got != "new" {
		t.Fatalf("content after truncate = %q, want new", got)
	}
}

func mustDecodeBase64(t *testing.T, content string) []byte {
	t.Helper()
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	return decoded
}

func TestLocalSourcesServeMultipleHostFolders(t *testing.T) {
	t.Parallel()

	mediaRoot := t.TempDir()
	docsRoot := t.TempDir()
	service := NewWithConfig(Config{
		LocalSources: []LocalConfig{
			{ID: "media", Name: "Media", Root: mediaRoot},
			{ID: "docs", Name: "Docs", Root: docsRoot},
		},
	})

	if !service.Enabled() {
		t.Fatal("Enabled = false, want true with host folders configured")
	}

	content := base64.StdEncoding.EncodeToString([]byte("hi"))
	if err := service.UploadSource(context.Background(), "media", "a.txt", content); err != nil {
		t.Fatalf("UploadSource media: %v", err)
	}

	// A file written to one host folder must not leak into another.
	items, err := service.ListSource(context.Background(), "docs", "/")
	if err != nil {
		t.Fatalf("ListSource docs: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("docs items = %#v, want empty isolated root", items)
	}

	if _, err := os.Stat(filepath.Join(mediaRoot, "a.txt")); err != nil {
		t.Fatalf("media file not written to its host folder: %v", err)
	}
}

func TestSnapshotIncludesHostFolders(t *testing.T) {
	t.Parallel()

	defaultRoot := t.TempDir()
	denyWrite := false
	service := NewWithConfig(Config{
		Root:         defaultRoot,
		LocalSources: []LocalConfig{{ID: "media", Name: "Media", Root: t.TempDir(), Policy: AccessPolicy{Write: &denyWrite}}},
	})

	snapshot := service.Snapshot()
	if enabled, _ := snapshot["local_root_enabled"].(bool); !enabled {
		t.Fatal("local_root_enabled = false, want true")
	}
	folders, ok := snapshot["folders"].([]localSourceSnapshot)
	if !ok {
		t.Fatalf("folders type = %T, want []localSourceSnapshot", snapshot["folders"])
	}
	if len(folders) != 2 {
		t.Fatalf("folders count = %d, want 2 (default local + media)", len(folders))
	}
	if folders[0].ID != LocalSourceID || folders[0].Root != defaultRoot {
		t.Fatalf("first folder = %#v, want default local root", folders[0])
	}
	if folders[1].ID != "media" {
		t.Fatalf("second folder = %#v, want media", folders[1])
	}
	if folders[1].Policy.Write == nil || *folders[1].Policy.Write {
		t.Fatalf("second folder policy = %#v, want write=false", folders[1].Policy)
	}
}

func TestApplyLocalConfigsReplacesHostFolders(t *testing.T) {
	t.Parallel()

	service := NewWithConfig(Config{LocalSources: []LocalConfig{{ID: "old", Root: t.TempDir()}}})
	newRoot := t.TempDir()
	service.ApplyLocalConfigs([]LocalConfig{{ID: "fresh", Root: newRoot}})

	configs := service.LocalConfigs()
	if len(configs) != 1 || configs[0].ID != "fresh" || configs[0].Root != newRoot {
		t.Fatalf("LocalConfigs = %#v, want single fresh source", configs)
	}
	if _, err := service.sourceForID("old"); err == nil {
		t.Fatal("old source still resolvable after ApplyLocalConfigs, want error")
	}
}

func TestApplyLocalConfigsInvalidatesSearchCache(t *testing.T) {
	t.Parallel()

	oldRoot := t.TempDir()
	newRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(oldRoot, "old.txt"), []byte("old"), 0o600); err != nil {
		t.Fatalf("write old file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(newRoot, "new.txt"), []byte("new"), 0o600); err != nil {
		t.Fatalf("write new file: %v", err)
	}

	service := NewWithConfig(Config{LocalSources: []LocalConfig{{ID: "shared", Root: oldRoot}}})
	if results, err := service.SearchSource(context.Background(), "shared", "old", 10); err != nil || len(results) != 1 {
		t.Fatalf("initial search results = %#v, err = %v", results, err)
	}

	service.ApplyLocalConfigs([]LocalConfig{{ID: "shared", Root: newRoot}})
	results, err := service.SearchSource(context.Background(), "shared", "new", 10)
	if err != nil {
		t.Fatalf("SearchSource after apply: %v", err)
	}
	if len(results) != 1 || results[0].Name != "new.txt" {
		t.Fatalf("search results after apply = %#v, want new.txt from replacement root", results)
	}
}

func TestSMBConfigEnablesService(t *testing.T) {
	t.Parallel()

	service := NewWithConfig(Config{
		Shares: []SMBConfig{
			{Host: "192.168.1.20", Share: "media"},
		},
	})

	if !service.Enabled() {
		t.Fatal("Enabled = false, want true when SMB is configured")
	}
}

func TestMultipleSMBSharesSnapshotAndDefaultSource(t *testing.T) {
	t.Parallel()

	denyDelete := false
	service := NewWithConfig(Config{
		Root: t.TempDir(),
		Shares: []SMBConfig{
			{ID: "media", Name: "Media", Host: "nas.local", Share: "media", Username: "aaron", Password: "secret", Policy: AccessPolicy{Delete: &denyDelete}},
			{ID: "archive", Name: "Archive", Host: "nas.local", Share: "archive", Username: "aaron"},
		},
	})

	snapshot := service.Snapshot()
	if got := snapshot["active_source_id"]; got != "media" {
		t.Fatalf("active_source_id = %v, want media", got)
	}
	sources, ok := snapshot["file_sources"].([]SourceSnapshot)
	if !ok {
		t.Fatalf("file_sources type = %T, want []SourceSnapshot", snapshot["file_sources"])
	}
	if len(sources) != 3 {
		t.Fatalf("source count = %d, want 3 including local", len(sources))
	}
	if sources[0].ID != "media" || !sources[0].SMBPasswordSet {
		t.Fatalf("first source = %#v, want media with saved password", sources[0])
	}
	if sources[0].Policy.Delete == nil || *sources[0].Policy.Delete {
		t.Fatalf("first source policy = %#v, want delete=false", sources[0].Policy)
	}
	if sources[2].ID != LocalSourceID || !sources[2].LocalRootEnabled {
		t.Fatalf("last source = %#v, want local source", sources[2])
	}
}

func TestLocalSourceIDUsesLocalRootWhenSMBConfigured(t *testing.T) {
	t.Parallel()

	service := NewWithConfig(Config{
		Root: t.TempDir(),
		Shares: []SMBConfig{
			{ID: "media", Host: "nas.local", Share: "media"},
		},
	})
	content := base64.StdEncoding.EncodeToString([]byte("local"))
	if err := service.UploadSource(context.Background(), LocalSourceID, "docs/local.txt", content); err != nil {
		t.Fatalf("UploadSource local: %v", err)
	}
	downloaded, err := service.DownloadSource(context.Background(), LocalSourceID, "docs/local.txt")
	if err != nil {
		t.Fatalf("DownloadSource local: %v", err)
	}
	if downloaded != content {
		t.Fatalf("DownloadSource content = %q, want %q", downloaded, content)
	}
}

func TestApplySMBConfigsPreservesExistingPasswordsBySourceID(t *testing.T) {
	t.Parallel()

	service := NewWithConfig(Config{
		Shares: []SMBConfig{
			{ID: "media", Host: "nas.local", Share: "media", Username: "aaron", Password: "old-secret"},
			{ID: "archive", Host: "nas.local", Share: "archive", Username: "aaron", Password: "archive-secret"},
		},
	})

	service.ApplySMBConfigs([]SMBConfig{
		{ID: "media", Host: "nas.local", Share: "media", Username: "aaron"},
		{ID: "backup", Host: "nas.local", Share: "backup", Username: "aaron", Password: "backup-secret"},
	})

	configs := service.SMBConfigs()
	if len(configs) != 2 {
		t.Fatalf("SMB config count = %d, want 2", len(configs))
	}
	passwords := map[string]string{}
	for _, cfg := range configs {
		passwords[cfg.ID] = cfg.Password
	}
	if passwords["media"] != "old-secret" {
		t.Fatalf("media password = %q, want old-secret", passwords["media"])
	}
	if passwords["backup"] != "backup-secret" {
		t.Fatalf("backup password = %q, want backup-secret", passwords["backup"])
	}
	if _, ok := passwords["archive"]; ok {
		t.Fatal("archive share was kept after apply, want removed")
	}
}

func TestNormalizeSMBHostAcceptsWebAndSMBInputs(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"192.0.2.10":                 "192.0.2.10",
		"https://192.0.2.10":         "192.0.2.10",
		"https://192.0.2.10/ui":      "192.0.2.10",
		"https://192.0.2.10:443":     "192.0.2.10",
		"smb://truenas.local/media":  "truenas.local",
		"smb://truenas.local:1445/x": "truenas.local:1445",
		"//truenas.local/media":      "truenas.local",
		`\\truenas.local\media`:      "truenas.local",
	}

	for input, want := range cases {
		if got := NormalizeSMBHost(input); got != want {
			t.Fatalf("NormalizeSMBHost(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSMBAddressDialsNormalizedHostOnPort445(t *testing.T) {
	t.Parallel()

	if got := smbAddress("https://192.0.2.10"); got != "192.0.2.10:445" {
		t.Fatalf("smbAddress() = %q, want %q", got, "192.0.2.10:445")
	}
	if got := smbAddress("smb://truenas.local:1445/share"); got != "truenas.local:1445" {
		t.Fatalf("smbAddress() = %q, want %q", got, "truenas.local:1445")
	}
}

func TestSMBFinishCleanupIgnoresClosedNetworkCleanupError(t *testing.T) {
	t.Parallel()

	err := finishSMBCleanup(nil, fmt.Errorf("close tcp 198.51.100.3:34698->192.0.2.20:445: use of closed network connection"))
	if err != nil {
		t.Fatalf("finishSMBCleanup() = %v, want nil for benign SMB cleanup close", err)
	}
}

func TestSMBFinishCleanupPreservesRealCloseErrors(t *testing.T) {
	t.Parallel()

	fileErr := errors.New("file close failed")
	cleanupErr := fmt.Errorf("close tcp 198.51.100.3:34698->192.0.2.20:445: use of closed network connection")
	if err := finishSMBCleanup(fileErr, cleanupErr); !errors.Is(err, fileErr) {
		t.Fatalf("finishSMBCleanup() = %v, want file close error", err)
	}

	cleanupErr = errors.New("session logoff failed")
	if err := finishSMBCleanup(nil, cleanupErr); !errors.Is(err, cleanupErr) {
		t.Fatalf("finishSMBCleanup() = %v, want cleanup error", err)
	}
}

func TestResolveSharePathUsesShareRoot(t *testing.T) {
	t.Parallel()

	if got := resolveSharePath(""); got != "." {
		t.Fatalf("resolveSharePath(\"\") = %q, want %q", got, ".")
	}
	if got := resolveSharePath("../movies"); got != "movies" {
		t.Fatalf("resolveSharePath(\"../movies\") = %q, want %q", got, "movies")
	}
}
