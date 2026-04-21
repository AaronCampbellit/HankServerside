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
		var public struct {
			Host     string `json:"host"`
			Share    string `json:"share"`
			Username string `json:"username"`
			Domain   string `json:"domain"`
		}
		if len(request.PublicConfig) > 0 {
			if err := json.Unmarshal(request.PublicConfig, &public); err != nil {
				return protocol.ServiceProfileSnapshot{}, err
			}
		}
		var secrets struct {
			Password string `json:"password"`
		}
		if len(request.Secrets) > 0 {
			if err := json.Unmarshal(request.Secrets, &secrets); err != nil {
				return protocol.ServiceProfileSnapshot{}, err
			}
		}
		m.files.ApplySMBConfig(agentfiles.SMBConfig{
			Host:     public.Host,
			Share:    public.Share,
			Username: public.Username,
			Password: secrets.Password,
			Domain:   public.Domain,
		})
		profile := m.smbProfile(request.SecretVersion, request.SecretVersion, "")
		m.profiles[request.ServiceType] = profile
		if request.Persist {
			if err := m.persistSMB(public.Host, public.Share, public.Username, secrets.Password, public.Domain); err != nil {
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
	if m.envPath == "" {
		return nil
	}
	env, err := m.readEnvFile()
	if err != nil {
		return err
	}
	env["HANK_REMOTE_SMB_HOST"] = strings.TrimSpace(host)
	env["HANK_REMOTE_SMB_SHARE"] = strings.TrimSpace(share)
	env["HANK_REMOTE_SMB_USERNAME"] = strings.TrimSpace(username)
	env["HANK_REMOTE_SMB_PASSWORD"] = password
	env["HANK_REMOTE_SMB_DOMAIN"] = strings.TrimSpace(domainName)
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
