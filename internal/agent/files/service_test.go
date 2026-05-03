package files

import (
	"context"
	"encoding/base64"
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

func TestResolveSharePathUsesShareRoot(t *testing.T) {
	t.Parallel()

	if got := resolveSharePath(""); got != "." {
		t.Fatalf("resolveSharePath(\"\") = %q, want %q", got, ".")
	}
	if got := resolveSharePath("../movies"); got != "movies" {
		t.Fatalf("resolveSharePath(\"../movies\") = %q, want %q", got, "movies")
	}
}
