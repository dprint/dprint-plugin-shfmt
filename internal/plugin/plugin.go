// Package plugin implements the state behind the exports of dprint's Wasm
// plugin interface (schema version 4).
//
// See https://github.com/dprint/dprint/blob/main/docs/wasm-plugin-development.md
package plugin

import (
	"strings"
	"unsafe"

	"github.com/buger/jsonparser"
)

const (
	formatNoChange uint32 = iota
	formatChanged
	formatError
)

// Plugin holds the plugin's state between calls from the host.
type Plugin struct {
	sharedBytes    []byte
	configs        map[uint32]*registeredConfig
	overrideConfig []byte
	filePath       string
	formattedText  []byte
	errorText      string
}

type registeredConfig struct {
	plugin []byte
	global []byte
	// resolved lazily, since the host may only need some of the configs
	resolved    *Configuration
	diagnostics []configDiagnostic
}

func (p *Plugin) SharedBytesPtr() uint32 {
	if len(p.sharedBytes) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&p.sharedBytes[0])))
}

func (p *Plugin) ClearSharedBytes(size uint32) uint32 {
	if size == 0 {
		p.sharedBytes = nil
		return 0
	}
	p.sharedBytes = make([]byte, size)
	return uint32(uintptr(unsafe.Pointer(&p.sharedBytes[0])))
}

// Info stores the plugin info and returns its length.
func (p *Plugin) Info(version string) uint32 {
	var data []byte
	data = append(data, `{"name":"dprint-plugin-shfmt","version":`...)
	data = appendJSONString(data, version)
	data = append(data, `,"configKey":"shfmt","helpUrl":"https://dprint.dev/plugins/shfmt","configSchemaUrl":`...)
	data = appendJSONString(data, "https://plugins.dprint.dev/dprint/dprint-plugin-shfmt/"+version+"/schema.json")
	data = append(data, `,"updateUrl":"https://plugins.dprint.dev/dprint/dprint-plugin-shfmt/latest.json"}`...)
	return p.setSharedBytes(data)
}

func (p *Plugin) LicenseText(text string) uint32 {
	return p.setSharedBytes([]byte(text))
}

// RegisterConfig stores the `{ "plugin": ..., "global": ... }` configuration
// in the shared bytes under the config id.
func (p *Plugin) RegisterConfig(configID uint32) {
	data := p.takeFromSharedBytes()
	config := &registeredConfig{}
	if value, _, _, err := jsonparser.Get(data, "plugin"); err == nil {
		config.plugin = value
	}
	if value, _, _, err := jsonparser.Get(data, "global"); err == nil {
		config.global = value
	}
	if p.configs == nil {
		p.configs = make(map[uint32]*registeredConfig)
	}
	p.configs[configID] = config
}

func (p *Plugin) ReleaseConfig(configID uint32) {
	delete(p.configs, configID)
}

func (p *Plugin) ConfigDiagnostics(configID uint32) uint32 {
	_, diagnostics := p.resolvedConfig(configID)
	return p.setSharedBytes(marshalConfigDiagnostics(diagnostics))
}

func (p *Plugin) ResolvedConfig(configID uint32) uint32 {
	config, _ := p.resolvedConfig(configID)
	return p.setSharedBytes(marshalConfig(config))
}

func (p *Plugin) ConfigFileMatching(configID uint32) uint32 {
	var data []byte
	data = append(data, `{"fileExtensions":`...)
	data = appendJSONStringArray(data, FileExtensions)
	data = append(data, `,"fileNames":`...)
	data = appendJSONStringArray(data, FileNames)
	data = append(data, `,"additive":false}`...)
	return p.setSharedBytes(data)
}

// CheckConfigUpdates reports that no configuration changes are needed after an
// upgrade of the plugin.
func (p *Plugin) CheckConfigUpdates() uint32 {
	_ = p.takeFromSharedBytes()
	return p.setSharedBytes([]byte(`{"kind":"ok","data":[]}`))
}

// SetOverrideConfig stores configuration that only applies to the next format.
func (p *Plugin) SetOverrideConfig() {
	p.overrideConfig = p.takeFromSharedBytes()
}

func (p *Plugin) SetFilePath() {
	// windows paths have backslashes, which the path package doesn't handle
	p.filePath = strings.ReplaceAll(string(p.takeFromSharedBytes()), "\\", "/")
}

// Format formats the file text in the shared bytes with the config, returning
// whether it changed or errored.
func (p *Plugin) Format(configID uint32) uint32 {
	src := p.takeFromSharedBytes()
	filePath := p.filePath
	overrideConfig := p.overrideConfig
	p.filePath = ""
	p.overrideConfig = nil

	var config Configuration
	if len(overrideConfig) == 0 {
		config, _ = p.resolvedConfig(configID)
	} else {
		registered := p.registeredConfig(configID)
		config, _ = resolveConfig(registered.plugin, overrideConfig, registered.global)
	}

	output, err := FormatText(filePath, src, config)
	if err != nil {
		p.errorText = err.Error()
		return formatError
	}
	if output == nil {
		return formatNoChange
	}
	p.formattedText = output
	return formatChanged
}

func (p *Plugin) FormattedText() uint32 {
	result := p.formattedText
	p.formattedText = nil
	return p.setSharedBytes(result)
}

func (p *Plugin) ErrorText() uint32 {
	result := p.errorText
	p.errorText = ""
	return p.setSharedBytes([]byte(result))
}

func (p *Plugin) resolvedConfig(configID uint32) (Configuration, []configDiagnostic) {
	registered := p.registeredConfig(configID)
	if registered.resolved == nil {
		config, diagnostics := resolveConfig(registered.plugin, nil, registered.global)
		registered.resolved = &config
		registered.diagnostics = diagnostics
	}
	return *registered.resolved, registered.diagnostics
}

// registeredConfig gets the config for the id, which falls back to the
// defaults for an id the host never registered.
func (p *Plugin) registeredConfig(configID uint32) *registeredConfig {
	if config, ok := p.configs[configID]; ok {
		return config
	}
	return &registeredConfig{}
}

func (p *Plugin) setSharedBytes(data []byte) uint32 {
	p.sharedBytes = data
	return uint32(len(data))
}

func (p *Plugin) takeFromSharedBytes() []byte {
	result := p.sharedBytes
	p.sharedBytes = nil
	return result
}
