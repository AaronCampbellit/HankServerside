package repo_checks

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestActiveDocsDoNotReferenceRemovedLegacyPaths(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	activeDocs := []string{filepath.Join(root, "README.md")}
	err := filepath.WalkDir(filepath.Join(root, "docs"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if filepath.Base(path) == "archive" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" || filepath.Base(path) == "legacy-code-audit.md" {
			return nil
		}
		activeDocs = append(activeDocs, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk docs: %v", err)
	}

	forbiddenText := []string{
		"/dashboard/home-users",
		"/dashboard/service-profiles",
		"/dashboard/sync-status",
		"/dashboard/storage",
		"/dashboard/assistant-settings",
		"/dashboard/accept-invitation",
		"/v1/oauth/openai/callback",
	}
	forbiddenPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\bHANK_REMOTE_SMB_HOST\b`),
		regexp.MustCompile(`\bHANK_REMOTE_SMB_SHARE\b`),
		regexp.MustCompile(`\bHANK_REMOTE_SMB_USERNAME\b`),
		regexp.MustCompile(`\bHANK_REMOTE_SMB_PASSWORD\b`),
		regexp.MustCompile(`\bHANK_REMOTE_SMB_DOMAIN\b`),
	}
	for _, path := range activeDocs {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s read: %v", path, err)
		}
		body := string(data)
		for _, value := range forbiddenText {
			if strings.Contains(body, value) {
				t.Fatalf("%s references removed legacy value %q", path, value)
			}
		}
		for _, pattern := range forbiddenPatterns {
			if pattern.MatchString(body) {
				t.Fatalf("%s references removed legacy env key %q", path, pattern.String())
			}
		}
	}
}

func TestPhaseDocsAreIndexedAsArchive(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	indexPath := filepath.Join(root, "docs", "project-knowledge-index.md")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read project knowledge index: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "archive/phases") {
		t.Fatal("project knowledge index should point phase docs at archive/phases")
	}
	for _, stale := range []string{
		"](phase-1-",
		"](phase-2-",
		"](phase-3-",
		"](phase-4-",
		"](phase-5-",
		"](phase-6-",
	} {
		if strings.Contains(body, stale) {
			t.Fatalf("project knowledge index still presents phase doc as active link %q", stale)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
