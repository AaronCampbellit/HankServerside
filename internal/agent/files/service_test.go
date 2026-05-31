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

func TestSMBConfigEnablesService(t *testing.T) {
	t.Parallel()

	service := NewWithConfig(Config{
		SMB: SMBConfig{
			Host:  "192.168.1.20",
			Share: "media",
		},
	})

	if !service.Enabled() {
		t.Fatal("Enabled = false, want true when SMB is configured")
	}
}

func TestMultipleSMBSharesSnapshotAndDefaultSource(t *testing.T) {
	t.Parallel()

	service := NewWithConfig(Config{
		Root: t.TempDir(),
		Shares: []SMBConfig{
			{ID: "media", Name: "Media", Host: "nas.local", Share: "media", Username: "aaron", Password: "secret"},
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
		"192.0.2.10":             "192.0.2.10",
		"https://192.0.2.10":     "192.0.2.10",
		"https://192.0.2.10/ui":  "192.0.2.10",
		"https://192.0.2.10:443": "192.0.2.10",
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
