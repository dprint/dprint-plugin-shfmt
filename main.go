//go:build tinygo

// The Wasm exports of the dprint plugin interface (schema version 4).
// See https://github.com/dprint/dprint/blob/main/docs/wasm-plugin-development.md
package main

import (
	_ "embed"
	"strings"

	"github.com/dprint/dprint-plugin-shfmt/internal/plugin"
)

func main() {}

var instance plugin.Plugin

//go:embed VERSION
var rawVersion string

var version = strings.TrimSpace(rawVersion)

//go:embed LICENSE
var license string

//go:embed third_party/mvdan-sh/LICENSE
var shLicense string

//go:embed third_party/jsonparser/LICENSE
var jsonparserLicense string

//go:wasmexport get_shared_bytes_ptr
func get_shared_bytes_ptr() uint32 {
	return instance.SharedBytesPtr()
}

//go:wasmexport clear_shared_bytes
func clear_shared_bytes(size uint32) uint32 {
	return instance.ClearSharedBytes(size)
}

//go:wasmexport dprint_plugin_version_4
func dprint_plugin_version_4() uint32 {
	return 4
}

//go:wasmexport get_license_text
func get_license_text() uint32 {
	return instance.LicenseText(license +
		"\n\n---\n\nmvdan/sh (shfmt):\n\n" + shLicense +
		"\n\n---\n\nbuger/jsonparser:\n\n" + jsonparserLicense)
}

//go:wasmexport get_plugin_info
func get_plugin_info() uint32 {
	return instance.Info(version)
}

//go:wasmexport register_config
func register_config(configID uint32) {
	instance.RegisterConfig(configID)
}

//go:wasmexport release_config
func release_config(configID uint32) {
	instance.ReleaseConfig(configID)
}

//go:wasmexport get_config_diagnostics
func get_config_diagnostics(configID uint32) uint32 {
	return instance.ConfigDiagnostics(configID)
}

//go:wasmexport get_resolved_config
func get_resolved_config(configID uint32) uint32 {
	return instance.ResolvedConfig(configID)
}

//go:wasmexport get_config_file_matching
func get_config_file_matching(configID uint32) uint32 {
	return instance.ConfigFileMatching(configID)
}

//go:wasmexport check_config_updates
func check_config_updates() uint32 {
	return instance.CheckConfigUpdates()
}

//go:wasmexport set_file_path
func set_file_path() {
	instance.SetFilePath()
}

//go:wasmexport set_override_config
func set_override_config() {
	instance.SetOverrideConfig()
}

//go:wasmexport format
func format(configID uint32) uint32 {
	return instance.Format(configID)
}

//go:wasmexport get_formatted_text
func get_formatted_text() uint32 {
	return instance.FormattedText()
}

//go:wasmexport get_error_text
func get_error_text() uint32 {
	return instance.ErrorText()
}
