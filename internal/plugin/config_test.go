package plugin

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestResolveConfigDefaults(t *testing.T) {
	config, diagnostics := resolveConfig(nil, nil, nil)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	want := Configuration{Dialect: "auto", IndentWidth: 2, UseTabs: true}
	if config != want {
		t.Fatalf("got %#v, want %#v", config, want)
	}
}

func TestResolveConfigGlobal(t *testing.T) {
	config, diagnostics := resolveConfig(nil, nil, []byte(`{"lineWidth":120,"indentWidth":4,"useTabs":false,"newLineKind":null}`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	if config.IndentWidth != 4 || config.UseTabs {
		t.Fatalf("global config not used: %#v", config)
	}

	// the plugin's own config wins over the global config
	config, _ = resolveConfig([]byte(`{"useTabs":true,"indentWidth":8}`), nil, []byte(`{"indentWidth":4,"useTabs":false}`))
	if config.IndentWidth != 8 || !config.UseTabs {
		t.Fatalf("plugin config not used: %#v", config)
	}
}

func TestResolveConfigAllOptions(t *testing.T) {
	config, diagnostics := resolveConfig([]byte(`{
		"dialect": "mksh",
		"indentWidth": 3,
		"useTabs": false,
		"binaryNextLine": true,
		"switchCaseIndent": true,
		"spaceRedirects": true,
		"functionNextLine": true,
		"simplify": true,
		"minify": true
	}`), nil, nil)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	want := Configuration{
		Dialect:          "mksh",
		IndentWidth:      3,
		BinaryNextLine:   true,
		SwitchCaseIndent: true,
		SpaceRedirects:   true,
		FunctionNextLine: true,
		Simplify:         true,
		Minify:           true,
	}
	if config != want {
		t.Fatalf("got %#v, want %#v", config, want)
	}
}

func TestResolveConfigStringValues(t *testing.T) {
	config, diagnostics := resolveConfig([]byte(`{"indentWidth":"4","useTabs":"false"}`), nil, nil)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	if config.IndentWidth != 4 || config.UseTabs {
		t.Fatalf("string values not parsed: %#v", config)
	}
}

func TestResolveConfigOverride(t *testing.T) {
	config, diagnostics := resolveConfig([]byte(`{"binaryNextLine":true,"dialect":"bash"}`), []byte(`{"dialect":"zsh","minify":true}`), nil)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	if config.Dialect != "zsh" || !config.BinaryNextLine || !config.Minify {
		t.Fatalf("override not merged: %#v", config)
	}
}

func TestResolveConfigDiagnostics(t *testing.T) {
	_, diagnostics := resolveConfig([]byte(`{"dialect":"fish","indentWidth":-1,"minify":"yes","unknownProp":1,"useTabs":null}`), nil, nil)
	want := []configDiagnostic{
		{PropertyName: "dialect", Message: "Found invalid value 'fish'. Expected one of: auto, bash, posix, mksh, bats, zsh"},
		{PropertyName: "indentWidth", Message: "Found invalid value '-1'."},
		{PropertyName: "minify", Message: "Found invalid value 'yes'."},
		{PropertyName: "unknownProp", Message: "Unknown property in configuration"},
	}
	if !reflect.DeepEqual(diagnostics, want) {
		t.Fatalf("got %#v, want %#v", diagnostics, want)
	}
}

func TestMarshalConfig(t *testing.T) {
	data := marshalConfig(Configuration{Dialect: "auto", IndentWidth: 2, UseTabs: true, Simplify: true})
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("invalid json %s: %v", data, err)
	}
	if decoded["dialect"] != "auto" || decoded["indentWidth"] != float64(2) || decoded["useTabs"] != true || decoded["simplify"] != true || decoded["minify"] != false {
		t.Fatalf("unexpected config: %s", data)
	}
}

// the schema documents the defaults, so it needs to stay in sync with them
func TestSchemaMatchesDefaults(t *testing.T) {
	data, err := os.ReadFile("../../deployment/schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]struct {
			Default any `json:"default"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	config, _ := resolveConfig(nil, nil, nil)
	var defaults map[string]any
	if err := json.Unmarshal(marshalConfig(config), &defaults); err != nil {
		t.Fatal(err)
	}
	if len(schema.Properties) != len(defaults) {
		t.Fatalf("schema has %d properties, but the config has %d", len(schema.Properties), len(defaults))
	}
	for key, value := range defaults {
		property, ok := schema.Properties[key]
		if !ok {
			t.Errorf("schema is missing %q", key)
		} else if !reflect.DeepEqual(property.Default, value) {
			t.Errorf("default of %q is %v in the schema, but %v in the config", key, property.Default, value)
		}
	}
}
