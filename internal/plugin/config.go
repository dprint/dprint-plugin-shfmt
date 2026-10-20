package plugin

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/buger/jsonparser"
)

// Configuration is the resolved configuration of the plugin. The options are
// the formatting flags of the shfmt CLI.
type Configuration struct {
	// Dialect is the shell dialect to parse files as (-ln).
	Dialect string
	// IndentWidth is the number of spaces to indent with when not using tabs (-i).
	IndentWidth uint32
	// UseTabs indents with tabs, which is shfmt's default (-i 0).
	UseTabs          bool
	BinaryNextLine   bool
	SwitchCaseIndent bool
	SpaceRedirects   bool
	FunctionNextLine bool
	Simplify         bool
	Minify           bool
}

type configDiagnostic struct {
	PropertyName string
	Message      string
}

// dialects are the values of shfmt's -ln flag
var dialects = []string{"auto", "bash", "posix", "mksh", "bats", "zsh"}

// the indent width when neither the plugin nor the global configuration has one
const defaultIndentWidth = 2

type configEntry struct {
	key       string
	value     []byte
	valueType jsonparser.ValueType
}

// resolveConfig resolves the plugin's configuration from its JSON object, the
// JSON object of an override for a single format (which may be nil), and the
// global configuration's JSON object.
func resolveConfig(pluginConfig, overrideConfig, globalConfig []byte) (Configuration, []configDiagnostic) {
	var diagnostics []configDiagnostic
	config := Configuration{
		Dialect:     "auto",
		IndentWidth: defaultIndentWidth,
		UseTabs:     true,
	}

	// the global configuration has already been validated by the cli
	if value, valueType, _, err := jsonparser.Get(globalConfig, "indentWidth"); err == nil {
		if width, ok := parseUint(value, valueType); ok {
			config.IndentWidth = width
		}
	}
	if value, valueType, _, err := jsonparser.Get(globalConfig, "useTabs"); err == nil {
		if useTabs, ok := parseBool(value, valueType); ok {
			config.UseTabs = useTabs
		}
	}

	for _, entry := range mergeEntries(objectEntries(pluginConfig), objectEntries(overrideConfig)) {
		if entry.valueType == jsonparser.Null {
			continue
		}
		var ok bool
		switch entry.key {
		case "dialect":
			var dialect string
			dialect, ok = parseEnum(entry.value, entry.valueType, dialects)
			if ok {
				config.Dialect = dialect
			} else {
				diagnostics = append(diagnostics, configDiagnostic{
					PropertyName: entry.key,
					Message:      fmt.Sprintf("Found invalid value '%s'. Expected one of: %s", entry.value, strings.Join(dialects, ", ")),
				})
				continue
			}
		case "indentWidth":
			config.IndentWidth, ok = parseUint(entry.value, entry.valueType)
		case "useTabs":
			config.UseTabs, ok = parseBool(entry.value, entry.valueType)
		case "binaryNextLine":
			config.BinaryNextLine, ok = parseBool(entry.value, entry.valueType)
		case "switchCaseIndent":
			config.SwitchCaseIndent, ok = parseBool(entry.value, entry.valueType)
		case "spaceRedirects":
			config.SpaceRedirects, ok = parseBool(entry.value, entry.valueType)
		case "functionNextLine":
			config.FunctionNextLine, ok = parseBool(entry.value, entry.valueType)
		case "simplify":
			config.Simplify, ok = parseBool(entry.value, entry.valueType)
		case "minify":
			config.Minify, ok = parseBool(entry.value, entry.valueType)
		default:
			diagnostics = append(diagnostics, configDiagnostic{
				PropertyName: entry.key,
				Message:      "Unknown property in configuration",
			})
			continue
		}
		if !ok {
			diagnostics = append(diagnostics, configDiagnostic{
				PropertyName: entry.key,
				Message:      fmt.Sprintf("Found invalid value '%s'.", entry.value),
			})
		}
	}

	return config, diagnostics
}

// objectEntries gets the entries of a JSON object in order.
func objectEntries(data []byte) []configEntry {
	var entries []configEntry
	if len(data) == 0 {
		return entries
	}
	_ = jsonparser.ObjectEach(data, func(key, value []byte, valueType jsonparser.ValueType, _ int) error {
		entries = append(entries, configEntry{key: string(key), value: value, valueType: valueType})
		return nil
	})
	return entries
}

// mergeEntries overwrites the entries with the overrides, the same way
// dprint-core merges an override configuration into the plugin configuration.
func mergeEntries(entries, overrides []configEntry) []configEntry {
	for _, override := range overrides {
		replaced := false
		for i := range entries {
			if entries[i].key == override.key {
				entries[i] = override
				replaced = true
				break
			}
		}
		if !replaced {
			entries = append(entries, override)
		}
	}
	return entries
}

// the parse functions also accept strings, like the "true" and "4" that
// dprint-core accepts for booleans and numbers

func parseBool(value []byte, valueType jsonparser.ValueType) (bool, bool) {
	switch valueType {
	case jsonparser.Boolean:
		result, err := jsonparser.ParseBoolean(value)
		return result, err == nil
	case jsonparser.String:
		result, err := strconv.ParseBool(string(value))
		return result, err == nil
	}
	return false, false
}

func parseUint(value []byte, valueType jsonparser.ValueType) (uint32, bool) {
	if valueType != jsonparser.Number && valueType != jsonparser.String {
		return 0, false
	}
	result, err := strconv.ParseUint(string(value), 10, 32)
	return uint32(result), err == nil
}

func parseEnum(value []byte, valueType jsonparser.ValueType, options []string) (string, bool) {
	if valueType != jsonparser.String {
		return "", false
	}
	text := string(value)
	for _, option := range options {
		if text == option {
			return text, true
		}
	}
	return "", false
}

func marshalConfig(config Configuration) []byte {
	var data []byte
	data = append(data, `{"dialect":`...)
	data = appendJSONString(data, config.Dialect)
	data = append(data, `,"indentWidth":`...)
	data = strconv.AppendUint(data, uint64(config.IndentWidth), 10)
	data = appendBoolProperty(data, "useTabs", config.UseTabs)
	data = appendBoolProperty(data, "binaryNextLine", config.BinaryNextLine)
	data = appendBoolProperty(data, "switchCaseIndent", config.SwitchCaseIndent)
	data = appendBoolProperty(data, "spaceRedirects", config.SpaceRedirects)
	data = appendBoolProperty(data, "functionNextLine", config.FunctionNextLine)
	data = appendBoolProperty(data, "simplify", config.Simplify)
	data = appendBoolProperty(data, "minify", config.Minify)
	return append(data, '}')
}

func marshalConfigDiagnostics(diagnostics []configDiagnostic) []byte {
	data := []byte{'['}
	for i, diagnostic := range diagnostics {
		if i != 0 {
			data = append(data, ',')
		}
		data = append(data, `{"propertyName":`...)
		data = appendJSONString(data, diagnostic.PropertyName)
		data = append(data, `,"message":`...)
		data = appendJSONString(data, diagnostic.Message)
		data = append(data, '}')
	}
	return append(data, ']')
}

func appendBoolProperty(data []byte, name string, value bool) []byte {
	data = append(data, ',', '"')
	data = append(data, name...)
	data = append(data, '"', ':')
	return strconv.AppendBool(data, value)
}
