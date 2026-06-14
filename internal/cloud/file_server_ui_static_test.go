package cloud

import (
	"os"
	"strings"
	"testing"
)

func TestFileServerDownloadUsesBrowserNavigation(t *testing.T) {
	data, err := os.ReadFile("ui/file-server.js")
	if err != nil {
		t.Fatalf("read file-server.js: %v", err)
	}
	body := string(data)
	start := strings.Index(body, "async function downloadFile(")
	if start < 0 {
		t.Fatal("downloadFile function not found")
	}
	end := strings.Index(body[start:], "\nasync function downloadSelected(")
	if end < 0 {
		t.Fatal("downloadSelected function not found after downloadFile")
	}
	downloadFile := body[start : start+end]
	if strings.Contains(downloadFile, "fetchFileBlob(") || strings.Contains(downloadFile, "URL.createObjectURL(") {
		t.Fatal("downloadFile must use browser navigation to the transfer URL instead of fetching the full file into a Blob")
	}
	if !strings.Contains(downloadFile, "startBrowserDownload(") {
		t.Fatal("downloadFile must trigger a browser-native download")
	}
}
