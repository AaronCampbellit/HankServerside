package apps

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/dropfile/hankremote/internal/protocol"
)

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
var slashCommandPattern = regexp.MustCompile(`^/[A-Za-z][A-Za-z0-9_-]*$`)

const networkPermissionConfiguredBaseURL = "configured_base_url"

type Manifest struct {
	SchemaVersion string      `json:"schema_version"`
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Version       string      `json:"version"`
	Publisher     string      `json:"publisher,omitempty"`
	Description   string      `json:"description,omitempty"`
	Runtime       Runtime     `json:"runtime"`
	Assistant     Assistant   `json:"assistant,omitempty"`
	Commands      []Command   `json:"commands"`
	Config        Config      `json:"config,omitempty"`
	Permissions   Permissions `json:"permissions,omitempty"`
}

type Runtime struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type Assistant struct {
	SlashCommands []SlashCommand `json:"slash_commands,omitempty"`
}

type SlashCommand struct {
	Command     string `json:"command"`
	CommandID   string `json:"command_id"`
	Description string `json:"description,omitempty"`
}

type Command struct {
	ID             string `json:"id"`
	Mode           string `json:"mode"`
	InputSchema    string `json:"input_schema,omitempty"`
	OutputSchema   string `json:"output_schema,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	AdminOnly      bool   `json:"admin_only,omitempty"`
}

type Config struct {
	Schema       string   `json:"schema,omitempty"`
	SecretFields []string `json:"secret_fields,omitempty"`
}

type Permissions struct {
	Network []NetworkPermission `json:"network,omitempty"`
	Files   []json.RawMessage   `json:"files,omitempty"`
	Events  []json.RawMessage   `json:"events,omitempty"`
}

type NetworkPermission struct {
	Kind  string `json:"kind"`
	Field string `json:"field,omitempty"`
}

func ValidateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != protocol.AppSchemaVersion {
		return fmt.Errorf("schema_version must be %q", protocol.AppSchemaVersion)
	}
	if !identifierPattern.MatchString(manifest.ID) {
		return fmt.Errorf("app id %q must match %s", manifest.ID, identifierPattern.String())
	}
	if manifest.Runtime.Type != protocol.AppRuntimeStdio {
		return fmt.Errorf("runtime type %q is not supported", manifest.Runtime.Type)
	}
	if err := validatePackagePath("runtime command", manifest.Runtime.Command, true); err != nil {
		return err
	}
	if len(manifest.Commands) == 0 {
		return fmt.Errorf("at least one command is required")
	}

	commandIDs := make(map[string]struct{}, len(manifest.Commands))
	for i, command := range manifest.Commands {
		if !identifierPattern.MatchString(command.ID) {
			return fmt.Errorf("command id %q must match %s", command.ID, identifierPattern.String())
		}
		if _, ok := commandIDs[command.ID]; ok {
			return fmt.Errorf("duplicate command id %q", command.ID)
		}
		commandIDs[command.ID] = struct{}{}
		if err := validatePackagePath("schema path", command.InputSchema, false); err != nil {
			return fmt.Errorf("command %d input %w", i, err)
		}
		if err := validatePackagePath("schema path", command.OutputSchema, false); err != nil {
			return fmt.Errorf("command %d output %w", i, err)
		}
	}

	slashCommands := make(map[string]struct{}, len(manifest.Assistant.SlashCommands))
	for i, slashCommand := range manifest.Assistant.SlashCommands {
		if !slashCommandPattern.MatchString(slashCommand.Command) {
			return fmt.Errorf("slash command %q must match %s", slashCommand.Command, slashCommandPattern.String())
		}
		if _, ok := slashCommands[slashCommand.Command]; ok {
			return fmt.Errorf("duplicate slash command %q", slashCommand.Command)
		}
		slashCommands[slashCommand.Command] = struct{}{}
		if _, ok := commandIDs[slashCommand.CommandID]; !ok {
			return fmt.Errorf("slash command %d references unknown command id %q", i, slashCommand.CommandID)
		}
	}

	if err := validatePackagePath("schema path", manifest.Config.Schema, false); err != nil {
		return fmt.Errorf("config %w", err)
	}
	for i, permission := range manifest.Permissions.Network {
		if permission.Kind != networkPermissionConfiguredBaseURL {
			return fmt.Errorf("network permission %d kind %q is not supported", i, permission.Kind)
		}
	}
	if len(manifest.Permissions.Files) > 0 {
		return fmt.Errorf("file permission entries are not supported")
	}
	if len(manifest.Permissions.Events) > 0 {
		return fmt.Errorf("event permission entries are not supported")
	}

	return nil
}

func validatePackagePath(label string, value string, required bool) error {
	if value == "" {
		if required {
			return fmt.Errorf("%s is required", label)
		}
		return nil
	}
	if _, ok := cleanPackagePath(value); !ok {
		return fmt.Errorf("%s %q must be a relative clean path inside the app package", label, value)
	}
	return nil
}

func cleanPackagePath(value string) (string, bool) {
	if value == "" || strings.Contains(value, "\x00") || strings.Contains(value, "\\") {
		return "", false
	}
	if path.IsAbs(value) || hasWindowsVolume(value) {
		return "", false
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", false
		}
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned != value {
		return "", false
	}
	return cleaned, true
}

func hasWindowsVolume(value string) bool {
	return len(value) >= 2 && value[1] == ':' &&
		((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z'))
}

func referencedSchemaPaths(manifest Manifest) []string {
	paths := make([]string, 0, 1+len(manifest.Commands)*2)
	if manifest.Config.Schema != "" {
		paths = append(paths, manifest.Config.Schema)
	}
	for _, command := range manifest.Commands {
		if command.InputSchema != "" {
			paths = append(paths, command.InputSchema)
		}
		if command.OutputSchema != "" {
			paths = append(paths, command.OutputSchema)
		}
	}
	return paths
}
