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
		t.Fatal("internal dashboard links should not keep external health checks enabled")
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
