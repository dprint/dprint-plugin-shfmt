package plugin

import (
	"encoding/json"
	"strings"
	"testing"
)

// write mimics the host writing data into the plugin's shared bytes
func write(p *Plugin, data string) {
	p.ClearSharedBytes(uint32(len(data)))
	copy(p.sharedBytes, data)
}

func TestInfo(t *testing.T) {
	var p Plugin
	if size := p.Info("1.2.3"); size != uint32(len(p.sharedBytes)) {
		t.Fatalf("Info() = %d, want %d", size, len(p.sharedBytes))
	}
	var info struct {
		Name            string `json:"name"`
		Version         string `json:"version"`
		ConfigKey       string `json:"configKey"`
		HelpURL         string `json:"helpUrl"`
		ConfigSchemaURL string `json:"configSchemaUrl"`
		UpdateURL       string `json:"updateUrl"`
	}
	if err := json.Unmarshal(p.takeFromSharedBytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Name != "dprint-plugin-shfmt" ||
		info.Version != "1.2.3" ||
		info.ConfigKey != "shfmt" ||
		info.ConfigSchemaURL != "https://plugins.dprint.dev/dprint/dprint-plugin-shfmt/1.2.3/schema.json" ||
		info.UpdateURL != "https://plugins.dprint.dev/dprint/dprint-plugin-shfmt/latest.json" {
		t.Fatalf("unexpected plugin info: %#v", info)
	}
}

func TestConfigFileMatching(t *testing.T) {
	var p Plugin
	p.ConfigFileMatching(1)
	var matching struct {
		FileExtensions []string `json:"fileExtensions"`
		FileNames      []string `json:"fileNames"`
		Additive       bool     `json:"additive"`
	}
	if err := json.Unmarshal(p.takeFromSharedBytes(), &matching); err != nil {
		t.Fatal(err)
	}
	if strings.Join(matching.FileExtensions, ",") != "sh,bash,mksh,bats,zsh" || len(matching.FileNames) != 8 || matching.Additive {
		t.Fatalf("unexpected file matching: %#v", matching)
	}
}

func TestConfigsByID(t *testing.T) {
	var p Plugin
	write(&p, `{"plugin":{"useTabs":false},"global":{"indentWidth":4}}`)
	p.RegisterConfig(1)
	write(&p, `{"plugin":{"unknown":true},"global":{}}`)
	p.RegisterConfig(2)

	p.ResolvedConfig(1)
	var resolved map[string]any
	if err := json.Unmarshal(p.takeFromSharedBytes(), &resolved); err != nil {
		t.Fatal(err)
	}
	if resolved["useTabs"] != false || resolved["indentWidth"] != float64(4) {
		t.Fatalf("unexpected resolved config: %#v", resolved)
	}

	p.ConfigDiagnostics(1)
	if diagnostics := string(p.takeFromSharedBytes()); diagnostics != "[]" {
		t.Fatalf("unexpected diagnostics for config 1: %s", diagnostics)
	}
	p.ConfigDiagnostics(2)
	if diagnostics := string(p.takeFromSharedBytes()); diagnostics != `[{"propertyName":"unknown","message":"Unknown property in configuration"}]` {
		t.Fatalf("unexpected diagnostics for config 2: %s", diagnostics)
	}

	// formatting uses the config of the id
	write(&p, "/repo/script.sh")
	p.SetFilePath()
	write(&p, "if true; then\necho hi\nfi\n")
	if result := p.Format(1); result != formatChanged {
		t.Fatalf("Format() = %d, want %d", result, formatChanged)
	}
	p.FormattedText()
	if formatted := string(p.takeFromSharedBytes()); formatted != "if true; then\n    echo hi\nfi\n" {
		t.Fatalf("unexpected formatted text: %q", formatted)
	}

	p.ReleaseConfig(1)
	if _, ok := p.configs[1]; ok {
		t.Fatal("config 1 was not released")
	}
}

func TestFormat(t *testing.T) {
	var p Plugin
	write(&p, `{"plugin":{},"global":{}}`)
	p.RegisterConfig(1)

	// windows paths are normalized
	write(&p, `C:\repo\.zshrc`)
	p.SetFilePath()
	write(&p, "print   -l ${(U)list}\n")
	if result := p.Format(1); result != formatChanged {
		t.Fatalf("Format() = %d, want %d", result, formatChanged)
	}
	p.FormattedText()
	if formatted := string(p.takeFromSharedBytes()); formatted != "print -l ${(U)list}\n" {
		t.Fatalf("unexpected formatted text: %q", formatted)
	}
	p.FormattedText()
	if formatted := p.takeFromSharedBytes(); formatted != nil {
		t.Fatalf("FormattedText() did not clear the result: %q", formatted)
	}

	write(&p, "/repo/script.sh")
	p.SetFilePath()
	write(&p, "echo hi\n")
	if result := p.Format(1); result != formatNoChange {
		t.Fatalf("Format() = %d, want %d", result, formatNoChange)
	}

	write(&p, "/repo/script.sh")
	p.SetFilePath()
	write(&p, "if then\n")
	if result := p.Format(1); result != formatError {
		t.Fatalf("Format() = %d, want %d", result, formatError)
	}
	p.ErrorText()
	if errorText := string(p.takeFromSharedBytes()); errorText != "1:1: `if` must be followed by a statement list" {
		t.Fatalf("unexpected error text: %q", errorText)
	}
}

func TestFormatWithOverrideConfig(t *testing.T) {
	var p Plugin
	write(&p, `{"plugin":{"useTabs":false},"global":{}}`)
	p.RegisterConfig(1)

	write(&p, `{"indentWidth":3}`)
	p.SetOverrideConfig()
	write(&p, "/repo/script.sh")
	p.SetFilePath()
	write(&p, "if true; then\necho hi\nfi\n")
	if result := p.Format(1); result != formatChanged {
		t.Fatalf("Format() = %d, want %d", result, formatChanged)
	}
	p.FormattedText()
	if formatted := string(p.takeFromSharedBytes()); formatted != "if true; then\n   echo hi\nfi\n" {
		t.Fatalf("override not applied: %q", formatted)
	}

	// the override only applies to the one format
	write(&p, "/repo/script.sh")
	p.SetFilePath()
	write(&p, "if true; then\necho hi\nfi\n")
	p.Format(1)
	p.FormattedText()
	if formatted := string(p.takeFromSharedBytes()); formatted != "if true; then\n  echo hi\nfi\n" {
		t.Fatalf("override was kept: %q", formatted)
	}
}

func TestFormatRange(t *testing.T) {
	var p Plugin
	write(&p, `{"plugin":{"useTabs":false},"global":{}}`)
	p.RegisterConfig(1)

	write(&p, `{"indentWidth":3}`)
	p.SetOverrideConfig()
	write(&p, "/repo/script.sh")
	p.SetFilePath()
	write(&p, "if true; then\necho hi\nfi\n")
	if result := p.FormatRange(1, 0, 5); result != formatNoChange {
		t.Fatalf("FormatRange() = %d, want %d", result, formatNoChange)
	}
	if p.sharedBytes != nil || p.filePath != "" || p.overrideConfig != nil {
		t.Fatal("FormatRange() did not clear the state of the format")
	}

	// the override sent with the range doesn't apply to the next format
	write(&p, "/repo/script.sh")
	p.SetFilePath()
	write(&p, "if true; then\necho hi\nfi\n")
	p.Format(1)
	p.FormattedText()
	if formatted := string(p.takeFromSharedBytes()); formatted != "if true; then\n  echo hi\nfi\n" {
		t.Fatalf("override was kept: %q", formatted)
	}
}

func TestCheckConfigUpdates(t *testing.T) {
	var p Plugin
	write(&p, `{"oldVersion":"0.1.0","config":{}}`)
	p.CheckConfigUpdates()
	if result := string(p.takeFromSharedBytes()); result != `{"kind":"ok","data":[]}` {
		t.Fatalf("unexpected result: %s", result)
	}
}
