package apps

import (
	"bytes"
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
const filePermissionConfiguredSource = "configured_source"

var allowedSettingsFieldTypes = map[string]struct{}{
	"boolean":  {},
	"number":   {},
	"password": {},
	"path":     {},
	"select":   {},
	"text":     {},
	"url":      {},
}

var allowedSettingsFieldSources = map[string]struct{}{
	"file_sources": {},
}

var reservedSlashCommands = map[string]struct{}{
	"/append":   {},
	"/calendar": {},
	"/docs":     {},
	"/files":    {},
	"/ha":       {},
	"/notes":    {},
	"/status":   {},
}

type SettingsSchema = protocol.AppSettingsSchema
type SettingsField = protocol.AppSettingsField
type SettingsOption = protocol.AppSettingsOption

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
	Schema       string         `json:"schema,omitempty"`
	SecretFields []string       `json:"secret_fields,omitempty"`
	Settings     SettingsSchema `json:"settings_schema,omitempty"`
}

type Permissions struct {
	Network []NetworkPermission `json:"network,omitempty"`
	Files   []FilePermission    `json:"files,omitempty"`
	Events  []json.RawMessage   `json:"events,omitempty"`
}

type NetworkPermission struct {
	Kind  string `json:"kind"`
	Field string `json:"field,omitempty"`
}

type FilePermission struct {
	Kind  string `json:"kind"`
	Field string `json:"field,omitempty"`
	Label string `json:"label,omitempty"`
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
		if _, ok := reservedSlashCommands[strings.ToLower(slashCommand.Command)]; ok {
			return fmt.Errorf("reserved slash command %q is owned by HankAI", slashCommand.Command)
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
	for i, field := range manifest.Config.Settings.Fields {
		if !identifierPattern.MatchString(field.Key) {
			return fmt.Errorf("settings field %d key %q must match %s", i, field.Key, identifierPattern.String())
		}
		if _, ok := allowedSettingsFieldTypes[field.Type]; !ok {
			return fmt.Errorf("settings field %d type %q is not supported", i, field.Type)
		}
		if field.Secret && field.SecretKey != "" && !identifierPattern.MatchString(field.SecretKey) {
			return fmt.Errorf("settings field %d secret_key %q must match %s", i, field.SecretKey, identifierPattern.String())
		}
		if field.Source != "" {
			if _, ok := allowedSettingsFieldSources[field.Source]; !ok {
				return fmt.Errorf("settings field %d source %q is not supported", i, field.Source)
			}
			if field.Type != "select" {
				return fmt.Errorf("settings field %d source requires select type", i)
			}
		}
		if err := validateSettingsDefault(i, field); err != nil {
			return err
		}
		if field.Type != "select" && len(field.Options) > 0 {
			return fmt.Errorf("settings field %d options require select type", i)
		}
		optionValues := make(map[string]struct{}, len(field.Options))
		for optionIndex, option := range field.Options {
			normalizedOption, err := normalizeSettingsScalarJSON(option.Value)
			if err != nil {
				return fmt.Errorf("settings field %d option %d value %w", i, optionIndex, err)
			}
			optionValues[normalizedOption] = struct{}{}
		}
		if field.Type == "select" && len(optionValues) > 0 && len(bytes.TrimSpace(field.Default)) > 0 {
			normalizedDefault, err := normalizeSettingsScalarJSON(field.Default)
			if err != nil {
				return fmt.Errorf("settings field %d default %w", i, err)
			}
			if _, ok := optionValues[normalizedDefault]; !ok {
				return fmt.Errorf("settings field %d default must match one of the static options", i)
			}
		}
	}
	for i, permission := range manifest.Permissions.Network {
		if permission.Kind != networkPermissionConfiguredBaseURL {
			return fmt.Errorf("network permission %d kind %q is not supported", i, permission.Kind)
		}
	}
	for i, permission := range manifest.Permissions.Files {
		if permission.Kind != filePermissionConfiguredSource {
			return fmt.Errorf("file permission %d kind %q is not supported", i, permission.Kind)
		}
		if !identifierPattern.MatchString(permission.Field) {
			return fmt.Errorf("file permission %d field %q must match %s", i, permission.Field, identifierPattern.String())
		}
	}
	if len(manifest.Permissions.Events) > 0 {
		return fmt.Errorf("event permission entries are not supported")
	}

	return nil
}

func validateSettingsDefault(fieldIndex int, field SettingsField) error {
	if len(bytes.TrimSpace(field.Default)) == 0 {
		return nil
	}
	kind, err := settingsJSONKind(field.Default)
	if err != nil {
		return fmt.Errorf("settings field %d default %w", fieldIndex, err)
	}
	switch field.Type {
	case "boolean":
		if kind != "boolean" {
			return fmt.Errorf("settings field %d default must be a boolean", fieldIndex)
		}
	case "number":
		if kind != "number" {
			return fmt.Errorf("settings field %d default must be a number", fieldIndex)
		}
	case "password", "path", "text", "url":
		if kind != "string" {
			return fmt.Errorf("settings field %d default must be a string", fieldIndex)
		}
	case "select":
		if kind != "boolean" && kind != "number" && kind != "string" {
			return fmt.Errorf("settings field %d default must be a string, number, or boolean", fieldIndex)
		}
	}
	return nil
}

func normalizeSettingsScalarJSON(raw json.RawMessage) (string, error) {
	kind, err := settingsJSONKind(raw)
	if err != nil {
		return "", err
	}
	if kind != "boolean" && kind != "number" && kind != "string" {
		return "", fmt.Errorf("must be a string, number, or boolean")
	}
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, bytes.TrimSpace(raw)); err != nil {
		return "", fmt.Errorf("must be valid JSON")
	}
	return compacted.String(), nil
}

func settingsJSONKind(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "", fmt.Errorf("is required")
	}
	if !json.Valid(trimmed) {
		return "", fmt.Errorf("must be valid JSON")
	}
	var value interface{}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("must be valid JSON")
	}
	switch value.(type) {
	case nil:
		return "null", nil
	case bool:
		return "boolean", nil
	case json.Number:
		return "number", nil
	case string:
		return "string", nil
	case []interface{}:
		return "array", nil
	case map[string]interface{}:
		return "object", nil
	default:
		return "unknown", nil
	}
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
