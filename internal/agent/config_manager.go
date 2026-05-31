package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	agentfiles "github.com/dropfile/hankremote/internal/agent/files"
	agentha "github.com/dropfile/hankremote/internal/agent/homeassistant"
	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
)

var errUnsupportedServiceType = errors.New("unsupported service type")

type configManager struct {
	mu       sync.Mutex
	envPath  string
	ha       *agentha.Client
	files    *agentfiles.Service
	profiles map[string]protocol.ServiceProfileSnapshot
}

func newConfigManager(envPath string, ha *agentha.Client, files *agentfiles.Service) *configManager {
	m := &configManager{
		envPath:  strings.TrimSpace(envPath),
		ha:       ha,
		files:    files,
		profiles: make(map[string]protocol.ServiceProfileSnapshot, 2),
	}
	m.profiles[domain.ServiceTypeHomeAssistant] = m.homeAssistantProfile(0, 0, "")
	m.profiles[domain.ServiceTypeSMB] = m.smbProfile(0, 0, "")
	return m
}

func (m *configManager) Status(_ context.Context, serviceType string) ([]protocol.ServiceProfileSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if serviceType != "" {
		profile, ok := m.profiles[serviceType]
		if !ok {
			return nil, errUnsupportedServiceType
		}
		return []protocol.ServiceProfileSnapshot{profile}, nil
	}

	serviceTypes := make([]string, 0, len(m.profiles))
	for serviceType := range m.profiles {
		serviceTypes = append(serviceTypes, serviceType)
	}
	sort.Strings(serviceTypes)

	profiles := make([]protocol.ServiceProfileSnapshot, 0, len(serviceTypes))
	for _, serviceType := range serviceTypes {
		profiles = append(profiles, m.profiles[serviceType])
	}
	return profiles, nil
}

func (m *configManager) Apply(_ context.Context, request protocol.ConfigApplyRequest) (protocol.ServiceProfileSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch request.ServiceType {
	case domain.ServiceTypeHomeAssistant:
		var public struct {
			BaseURL        string `json:"base_url"`
			TimeoutSeconds int    `json:"timeout_seconds"`
		}
		if len(request.PublicConfig) > 0 {
			if err := json.Unmarshal(request.PublicConfig, &public); err != nil {
				return protocol.ServiceProfileSnapshot{}, err
			}
		}
		var secrets struct {
			Token string `json:"token"`
		}
		if len(request.Secrets) > 0 {
			if err := json.Unmarshal(request.Secrets, &secrets); err != nil {
				return protocol.ServiceProfileSnapshot{}, err
			}
		}
		timeout := time.Duration(public.TimeoutSeconds) * time.Second
		m.ha.ApplyConfig(public.BaseURL, secrets.Token, timeout)
		profile := m.homeAssistantProfile(request.SecretVersion, request.SecretVersion, "")
		m.profiles[request.ServiceType] = profile
		if request.Persist {
			if err := m.persistHomeAssistant(public.BaseURL, secrets.Token, public.TimeoutSeconds); err != nil {
				profile.Status = domain.SyncStatusDegraded
				profile.LastError = err.Error()
				m.profiles[request.ServiceType] = profile
				return profile, err
			}
		}
		return profile, nil

	case domain.ServiceTypeSMB:
		var public smbPublicConfig
		if len(request.PublicConfig) > 0 {
			if err := json.Unmarshal(request.PublicConfig, &public); err != nil {
				return protocol.ServiceProfileSnapshot{}, err
			}
		}
		var secrets smbSecretConfig
		if len(request.Secrets) > 0 {
			if err := json.Unmarshal(request.Secrets, &secrets); err != nil {
				return protocol.ServiceProfileSnapshot{}, err
			}
		}
		configs := smbConfigsFromApply(public, secrets, m.files.SMBConfigs())
		m.files.ApplySMBConfigs(configs)
		profile := m.smbProfile(request.SecretVersion, request.SecretVersion, "")
		m.profiles[request.ServiceType] = profile
		if request.Persist {
			if err := m.persistSMBConfigs(m.files.SMBConfigs()); err != nil {
				profile.Status = domain.SyncStatusDegraded
				profile.LastError = err.Error()
				m.profiles[request.ServiceType] = profile
				return profile, err
			}
		}
		return profile, nil

	default:
		return protocol.ServiceProfileSnapshot{}, errUnsupportedServiceType
	}
}

func (m *configManager) homeAssistantProfile(secretVersion int, appliedVersion int, lastError string) protocol.ServiceProfileSnapshot {
	publicConfig, _ := json.Marshal(m.ha.Snapshot())
	status := domain.SyncStatusHealthy
	if lastError != "" {
		status = domain.SyncStatusDegraded
	}
	return protocol.ServiceProfileSnapshot{
		ServiceType:    domain.ServiceTypeHomeAssistant,
		PublicConfig:   publicConfig,
		SecretVersion:  secretVersion,
		AppliedVersion: appliedVersion,
		Status:         status,
		LastError:      lastError,
		UpdatedAt:      time.Now().UTC(),
	}
}

func (m *configManager) smbProfile(secretVersion int, appliedVersion int, lastError string) protocol.ServiceProfileSnapshot {
	publicConfig, _ := json.Marshal(m.files.Snapshot())
	status := domain.SyncStatusHealthy
	if lastError != "" {
		status = domain.SyncStatusDegraded
	}
	return protocol.ServiceProfileSnapshot{
		ServiceType:    domain.ServiceTypeSMB,
		PublicConfig:   publicConfig,
		SecretVersion:  secretVersion,
		AppliedVersion: appliedVersion,
		Status:         status,
		LastError:      lastError,
		UpdatedAt:      time.Now().UTC(),
	}
}

type smbPublicConfig struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Host        string                  `json:"host"`
	Share       string                  `json:"share"`
	Username    string                  `json:"username"`
	Domain      string                  `json:"domain"`
	Policy      agentfiles.AccessPolicy `json:"policy"`
	Shares      []smbSharePayload       `json:"shares"`
	FileSources []smbSharePayload       `json:"file_sources"`
	Sources     []smbSharePayload       `json:"sources"`
}

type smbSecretConfig struct {
	Password string            `json:"password"`
	Shares   []smbShareSecret  `json:"shares"`
	ByID     map[string]string `json:"by_id"`
	SourceID string            `json:"source_id"`
}

type smbSharePayload struct {
	ID          string                  `json:"id"`
	SourceID    string                  `json:"source_id"`
	Name        string                  `json:"name"`
	Type        string                  `json:"type"`
	Host        string                  `json:"host"`
	Share       string                  `json:"share"`
	Username    string                  `json:"username"`
	Domain      string                  `json:"domain"`
	SMBHost     string                  `json:"smb_host"`
	SMBShare    string                  `json:"smb_share"`
	SMBUsername string                  `json:"smb_username"`
	SMBDomain   string                  `json:"smb_domain"`
	Policy      agentfiles.AccessPolicy `json:"policy"`
}

type smbShareSecret struct {
	ID       string `json:"id"`
	SourceID string `json:"source_id"`
	Password string `json:"password"`
}

func smbConfigsFromApply(public smbPublicConfig, secrets smbSecretConfig, existing []agentfiles.SMBConfig) []agentfiles.SMBConfig {
	publicShares := public.Shares
	if len(publicShares) == 0 {
		publicShares = filterSMBSourcePayloads(public.FileSources)
	}
	if len(publicShares) == 0 {
		publicShares = filterSMBSourcePayloads(public.Sources)
	}

	configs := make([]agentfiles.SMBConfig, 0, len(publicShares))
	for _, share := range publicShares {
		policy := share.Policy
		if !policy.HasRules() {
			policy = public.Policy
		}
		configs = append(configs, agentfiles.SMBConfig{
			ID:       strings.TrimSpace(firstNonEmpty(share.ID, share.SourceID)),
			Name:     strings.TrimSpace(share.Name),
			Host:     strings.TrimSpace(firstNonEmpty(share.Host, share.SMBHost)),
			Share:    strings.TrimSpace(firstNonEmpty(share.Share, share.SMBShare)),
			Username: strings.TrimSpace(firstNonEmpty(share.Username, share.SMBUsername)),
			Domain:   strings.TrimSpace(firstNonEmpty(share.Domain, share.SMBDomain)),
			Policy:   policy,
		})
	}
	if len(configs) == 0 {
		configs = append(configs, agentfiles.SMBConfig{
			ID:       strings.TrimSpace(public.ID),
			Name:     strings.TrimSpace(public.Name),
			Host:     strings.TrimSpace(public.Host),
			Share:    strings.TrimSpace(public.Share),
			Username: strings.TrimSpace(public.Username),
			Domain:   strings.TrimSpace(public.Domain),
			Policy:   public.Policy,
		})
	}

	passwords := map[string]string{}
	for _, cfg := range existing {
		passwords[cfg.ID] = cfg.Password
	}
	if secrets.Password != "" {
		targetID := strings.TrimSpace(secrets.SourceID)
		if targetID == "" && len(configs) == 1 {
			targetID = configs[0].ID
		}
		if targetID != "" {
			passwords[targetID] = secrets.Password
		}
	}
	for id, password := range secrets.ByID {
		if strings.TrimSpace(id) != "" && password != "" {
			passwords[strings.TrimSpace(id)] = password
		}
	}
	for _, secret := range secrets.Shares {
		id := strings.TrimSpace(firstNonEmpty(secret.ID, secret.SourceID))
		if id != "" && secret.Password != "" {
			passwords[id] = secret.Password
		}
	}
	for i := range configs {
		configs[i].Password = passwords[configs[i].ID]
	}
	return configs
}

func filterSMBSourcePayloads(sources []smbSharePayload) []smbSharePayload {
	filtered := make([]smbSharePayload, 0, len(sources))
	for _, source := range sources {
		if source.Type == "" || source.Type == "smb" {
			filtered = append(filtered, source)
		}
	}
	return filtered
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (m *configManager) persistHomeAssistant(baseURL string, token string, timeoutSeconds int) error {
	if m.envPath == "" {
		return nil
	}
	env, err := m.readEnvFile()
	if err != nil {
		return err
	}
	env["HANK_REMOTE_HA_BASE_URL"] = strings.TrimSpace(baseURL)
	env["HANK_REMOTE_HA_TOKEN"] = token
	if timeoutSeconds > 0 {
		env["HANK_REMOTE_HA_TIMEOUT_SECONDS"] = fmt.Sprintf("%d", timeoutSeconds)
	}
	return writeEnvFile(m.envPath, env)
}

func (m *configManager) persistSMB(host string, share string, username string, password string, domainName string) error {
	return m.persistSMBConfigs([]agentfiles.SMBConfig{{
		Host:     host,
		Share:    share,
		Username: username,
		Password: password,
		Domain:   domainName,
	}})
}

func (m *configManager) persistSMBConfigs(configs []agentfiles.SMBConfig) error {
	if m.envPath == "" {
		return nil
	}
	env, err := m.readEnvFile()
	if err != nil {
		return err
	}
	var primary agentfiles.SMBConfig
	if len(configs) > 0 {
		primary = configs[0]
	}
	env["HANK_REMOTE_SMB_HOST"] = strings.TrimSpace(primary.Host)
	env["HANK_REMOTE_SMB_SHARE"] = strings.TrimSpace(primary.Share)
	env["HANK_REMOTE_SMB_USERNAME"] = strings.TrimSpace(primary.Username)
	env["HANK_REMOTE_SMB_PASSWORD"] = primary.Password
	env["HANK_REMOTE_SMB_DOMAIN"] = strings.TrimSpace(primary.Domain)
	type envShare struct {
		ID       string                  `json:"id,omitempty"`
		Name     string                  `json:"name,omitempty"`
		Host     string                  `json:"host"`
		Share    string                  `json:"share"`
		Username string                  `json:"username,omitempty"`
		Password string                  `json:"password,omitempty"`
		Domain   string                  `json:"domain,omitempty"`
		Policy   agentfiles.AccessPolicy `json:"policy,omitempty"`
	}
	shares := make([]envShare, 0, len(configs))
	for _, cfg := range configs {
		if !cfg.Enabled() {
			continue
		}
		shares = append(shares, envShare{
			ID:       strings.TrimSpace(cfg.ID),
			Name:     strings.TrimSpace(cfg.Name),
			Host:     strings.TrimSpace(cfg.Host),
			Share:    strings.TrimSpace(cfg.Share),
			Username: strings.TrimSpace(cfg.Username),
			Password: cfg.Password,
			Domain:   strings.TrimSpace(cfg.Domain),
			Policy:   cfg.Policy,
		})
	}
	if len(shares) > 1 {
		encoded, err := json.Marshal(shares)
		if err != nil {
			return err
		}
		env["HANK_REMOTE_SMB_SHARES_JSON"] = string(encoded)
	} else {
		delete(env, "HANK_REMOTE_SMB_SHARES_JSON")
	}
	return writeEnvFile(m.envPath, env)
}

func (m *configManager) readEnvFile() (map[string]string, error) {
	env := map[string]string{}
	if m.envPath == "" {
		return env, nil
	}
	data, err := os.ReadFile(m.envPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return env, nil
		}
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		env[strings.TrimSpace(key)] = value
	}
	return env, nil
}

func writeEnvFile(path string, env map[string]string) error {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys)+1)
	for _, key := range keys {
		lines = append(lines, key+"="+env[key])
	}
	lines = append(lines, "")
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600)
}
