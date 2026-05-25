package files

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
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

func TestNormalizeSMBHostAcceptsWebAndSMBInputs(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"192.168.86.138":             "192.168.86.138",
		"https://192.168.86.138":     "192.168.86.138",
		"https://192.168.86.138/ui":  "192.168.86.138",
		"https://192.168.86.138:443": "192.168.86.138",
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

	if got := smbAddress("https://192.168.86.138"); got != "192.168.86.138:445" {
		t.Fatalf("smbAddress() = %q, want %q", got, "192.168.86.138:445")
	}
	if got := smbAddress("smb://truenas.local:1445/share"); got != "truenas.local:1445" {
		t.Fatalf("smbAddress() = %q, want %q", got, "truenas.local:1445")
	}
}

func TestSMBFinishCleanupIgnoresClosedNetworkCleanupError(t *testing.T) {
	t.Parallel()

	err := finishSMBCleanup(nil, fmt.Errorf("close tcp 192.168.48.3:34698->192.168.86.137:445: use of closed network connection"))
	if err != nil {
		t.Fatalf("finishSMBCleanup() = %v, want nil for benign SMB cleanup close", err)
	}
}

func TestSMBFinishCleanupPreservesRealCloseErrors(t *testing.T) {
	t.Parallel()

	fileErr := errors.New("file close failed")
	cleanupErr := fmt.Errorf("close tcp 192.168.48.3:34698->192.168.86.137:445: use of closed network connection")
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
