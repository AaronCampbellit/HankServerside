package domain

import "time"

const (
	DesktopTrustAlgorithm = "ECDSA_P256_SHA256"

	DesktopIdentityOperatorDevice = "operator_device"
	DesktopIdentityEndpoint       = "endpoint"
)

type DesktopTrustRoot struct {
	HomeID           string
	Generation       int
	Algorithm        string
	PublicKeySPKI    []byte
	Fingerprint      string
	RecoveryEnvelope []byte
	CreatedAt        time.Time
	RotatedAt        *time.Time
}

type DesktopIdentity struct {
	ID                  string
	HomeID              string
	IdentityType        string
	UserID              string
	DeviceID            string
	AgentID             string
	PublicKeySPKI       []byte
	Certificate         []byte
	Fingerprint         string
	Capabilities        []string
	TrustRootGeneration int
	CreatedAt           time.Time
	ExpiresAt           time.Time
	RevokedAt           *time.Time
	RevocationReason    string
}

type DesktopSession struct {
	ID                       string
	HomeID                   string
	AgentID                  string
	OperatorUserID           string
	OperatorDeviceIdentityID string
	RequestedPermissions     []string
	EffectivePermissions     []string
	State                    string
	KeyEpoch                 uint32
	RequestedAt              time.Time
	JoinExpiresAt            time.Time
	HardExpiresAt            time.Time
	ActiveAt                 *time.Time
	ReconnectExpiresAt       *time.Time
	TerminatedAt             *time.Time
	TerminationReason        string
	SourceIPHash             string
	SourceUserAgentHash      string
	BrowserToAgentBytes      int64
	AgentToBrowserBytes      int64
}

type DesktopJoinCredential struct {
	ID             string
	SessionID      string
	Side           string
	CredentialHash []byte
	KeyEpoch       uint32
	CreatedAt      time.Time
	ExpiresAt      time.Time
	ConsumedAt     *time.Time
	RevokedAt      *time.Time
}

type DesktopSessionEvent struct {
	SessionID    string
	Sequence     int64
	EventType    string
	ActorType    string
	ActorID      string
	OccurredAt   time.Time
	Severity     string
	ReasonCode   string
	MetadataJSON string
}
