package plugin

import (
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func defaultConfig() Configuration {
	config, _ := resolveConfig(nil, nil, nil)
	return config
}

func TestFormatText(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		config   func(*Configuration)
		input    string
		expected string
	}{
		{
			name:     "indents with tabs by default",
			input:    "if true; then\necho   hi\nfi\n",
			expected: "if true; then\n\techo hi\nfi\n",
		},
		{
			name:     "indents with spaces",
			config:   func(c *Configuration) { c.UseTabs = false; c.IndentWidth = 4 },
			input:    "if true; then\necho hi\nfi\n",
			expected: "if true; then\n    echo hi\nfi\n",
		},
		{
			name:     "binary next line",
			config:   func(c *Configuration) { c.BinaryNextLine = true },
			input:    "foo &&\nbar\n",
			expected: "foo \\\n\t&& bar\n",
		},
		{
			name:     "switch case indent",
			config:   func(c *Configuration) { c.SwitchCaseIndent = true },
			input:    "case $x in\na) echo a ;;\nesac\n",
			expected: "case $x in\n\ta) echo a ;;\nesac\n",
		},
		{
			name:     "space redirects",
			config:   func(c *Configuration) { c.SpaceRedirects = true },
			input:    "echo hi >/dev/null\n",
			expected: "echo hi > /dev/null\n",
		},
		{
			name:     "function next line",
			config:   func(c *Configuration) { c.FunctionNextLine = true },
			input:    "foo() {\necho hi\n}\n",
			expected: "foo()\n{\n\techo hi\n}\n",
		},
		{
			name:     "simplify",
			config:   func(c *Configuration) { c.Simplify = true },
			input:    "echo $(( $a + 1 ))\n",
			expected: "echo $((a + 1))\n",
		},
		{
			name:     "minify",
			config:   func(c *Configuration) { c.Minify = true },
			input:    "# comment\nif [[ \"$a\" == b ]]; then\n  echo hi\nfi\n",
			expected: "if [[ $a == b ]];then\necho hi\nfi\n",
		},
		{
			name:     "keeps comments",
			input:    "echo hi   # comment\n",
			expected: "echo hi # comment\n",
		},
		{
			name:     "bash syntax in a .bash file",
			path:     "/repo/script.bash",
			input:    "if [[ -n $1 ]]; then\necho hi\nfi\n",
			expected: "if [[ -n $1 ]]; then\n\techo hi\nfi\n",
		},
		{
			name:     "zsh syntax in a .zshrc file",
			path:     "/home/me/.zshrc",
			input:    "print   -l ${(U)list}\n",
			expected: "print -l ${(U)list}\n",
		},
		{
			name:     "a bash shebang in a .sh file",
			path:     "/repo/script.sh",
			input:    "#!/usr/bin/env bash\nif [[ -n $1 ]]; then\necho hi\nfi\n",
			expected: "#!/usr/bin/env bash\nif [[ -n $1 ]]; then\n\techo hi\nfi\n",
		},
		{
			name:     "a .sh file without a shebang falls back to bash",
			path:     "/repo/script.sh",
			input:    "a=( 1 2 )\n",
			expected: "a=(1 2)\n",
		},
		{
			name:     "configured dialect",
			path:     "/repo/script.sh",
			config:   func(c *Configuration) { c.Dialect = "zsh" },
			input:    "print   -l ${(U)list}\n",
			expected: "print -l ${(U)list}\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := defaultConfig()
			if test.config != nil {
				test.config(&config)
			}
			path := test.path
			if path == "" {
				path = "/repo/script.sh"
			}
			output, err := FormatText(path, []byte(test.input), config)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if output == nil {
				t.Fatalf("expected a change for %q", test.input)
			}
			if string(output) != test.expected {
				t.Fatalf("got:\n%q\nwant:\n%q", output, test.expected)
			}

			// formatting again shouldn't change anything
			again, err := FormatText(path, output, config)
			if err != nil {
				t.Fatalf("unexpected error when formatting twice: %v", err)
			}
			if again != nil {
				t.Fatalf("formatting twice changed the text:\n%q", again)
			}
		})
	}
}

func TestFormatTextUnchanged(t *testing.T) {
	output, err := FormatText("/repo/script.sh", []byte("echo hi\n"), defaultConfig())
	if err != nil || output != nil {
		t.Fatalf("got %q, %v", output, err)
	}
}

func TestFormatTextErrors(t *testing.T) {
	_, err := FormatText("/repo/script.sh", []byte("if then\n"), defaultConfig())
	if err == nil || err.Error() != "1:1: `if` must be followed by a statement list" {
		t.Fatalf("unexpected error: %v", err)
	}

	// a dialect error says which dialect it parsed as
	_, err = FormatText("/repo/script.sh", []byte("#!/bin/sh\na=(1 2)\n"), defaultConfig())
	if err == nil || err.Error() != "2:3: arrays are a bash/mksh/zsh feature; tried parsing as posix" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveLanguage(t *testing.T) {
	tests := []struct {
		dialect, path, src string
		expected           syntax.LangVariant
	}{
		{"auto", "a.sh", "echo", syntax.LangBash},
		{"auto", "a.sh", "#!/bin/sh\n", syntax.LangPOSIX},
		{"auto", "a.sh", "#!/usr/bin/env zsh\n", syntax.LangZsh},
		{"auto", "a.bash", "#!/bin/sh\n", syntax.LangBash},
		{"auto", "a.mksh", "", syntax.LangMirBSDKorn},
		{"auto", "a.bats", "", syntax.LangBats},
		{"auto", "a.zsh", "", syntax.LangZsh},
		{"auto", "dir/.bashrc", "", syntax.LangBash},
		{"auto", "dir/.zprofile", "", syntax.LangZsh},
		{"auto", "dir/script", "#!/bin/bash\n", syntax.LangBash},
		{"posix", "a.zsh", "#!/usr/bin/env zsh\n", syntax.LangPOSIX},
	}
	for _, test := range tests {
		if got := resolveLanguage(test.dialect, test.path, []byte(test.src)); got != test.expected {
			t.Errorf("resolveLanguage(%q, %q, %q) = %s, want %s", test.dialect, test.path, test.src, got, test.expected)
		}
	}
}
