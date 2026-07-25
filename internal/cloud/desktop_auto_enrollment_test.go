package cloud

import (
	"testing"

	"github.com/dropfile/hankremote/internal/domain"
)

func TestSameDesktopIdentityEnrollmentRequiresSameKeyAndAuthSession(t *testing.T) {
	existing := domain.DesktopIdentity{
		HomeID: "home_1", IdentityType: domain.DesktopIdentityOperatorDevice,
		UserID: "user_1", DeviceID: "browser_1", AuthSessionID: "session_1",
		TrustRootGeneration: 2, Fingerprint: "fingerprint_1", PublicKeySPKI: []byte("public-key-1"),
	}
	if !sameDesktopIdentityEnrollment(existing, existing) {
		t.Fatal("identical enrollment was not reusable")
	}

	differentSession := existing
	differentSession.AuthSessionID = "session_2"
	if sameDesktopIdentityEnrollment(existing, differentSession) {
		t.Fatal("enrollment from another auth session was reused")
	}

	differentKey := existing
	differentKey.PublicKeySPKI = []byte("public-key-2")
	if sameDesktopIdentityEnrollment(existing, differentKey) {
		t.Fatal("enrollment with another public key was reused")
	}

	endpoint := existing
	endpoint.IdentityType = domain.DesktopIdentityEndpoint
	endpoint.UserID = ""
	endpoint.DeviceID = ""
	endpoint.AgentID = "agent_1"
	endpoint.AuthSessionID = ""
	if sameDesktopIdentityEnrollment(endpoint, endpoint) {
		t.Fatal("endpoint renewal was treated as a duplicate browser enrollment")
	}
}
