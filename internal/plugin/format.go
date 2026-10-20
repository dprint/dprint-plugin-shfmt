package plugin

import (
	"bytes"
	"path"
	"strings"

	"mvdan.cc/sh/v3/fileutil"
	"mvdan.cc/sh/v3/syntax"
)

// FileExtensions are the extensions shfmt formats when walking directories.
var FileExtensions = []string{"sh", "bash", "mksh", "bats", "zsh"}

// FileNames are the shell config files shfmt detects the dialect of.
var FileNames = []string{
	".bashrc",
	".bash_profile",
	".bash_logout",
	".zshenv",
	".zprofile",
	".zshrc",
	".zlogin",
	".zlogout",
}

// FormatText formats the shell script the same way shfmt formats it with the
// flags that match the configuration. It returns nil when the text is
// unchanged.
func FormatText(filePath string, src []byte, config Configuration) ([]byte, error) {
	lang := resolveLanguage(config.Dialect, filePath, src)
	parser := syntax.NewParser(syntax.KeepComments(true), syntax.Variant(lang))
	// the errors of dialect specific syntax already say which dialect was tried
	file, err := parser.Parse(bytes.NewReader(src), "")
	if err != nil {
		return nil, err
	}
	// shfmt simplifies right after parsing, and its -mn implies -s
	if config.Simplify || config.Minify {
		syntax.Simplify(file)
	}

	indent := uint(config.IndentWidth)
	if config.UseTabs {
		indent = 0
	}
	printer := syntax.NewPrinter(
		syntax.Minify(config.Minify),
		syntax.Indent(indent),
		syntax.BinaryNextLine(config.BinaryNextLine),
		syntax.SwitchCaseIndent(config.SwitchCaseIndent),
		syntax.SpaceRedirects(config.SpaceRedirects),
		syntax.FunctionNextLine(config.FunctionNextLine),
	)
	var buf bytes.Buffer
	if err := printer.Print(&buf, file); err != nil {
		return nil, err
	}
	if bytes.Equal(buf.Bytes(), src) {
		return nil, nil
	}
	return buf.Bytes(), nil
}

// resolveLanguage picks the dialect like `shfmt --filename <path>` does: the
// configured one, then the file name, then the shebang, falling back to bash.
func resolveLanguage(dialect, filePath string, src []byte) syntax.LangVariant {
	lang := syntax.LangAuto
	if err := lang.Set(dialect); err == nil && lang != syntax.LangAuto {
		return lang
	}
	if lang = langFromFilename(filePath); lang != syntax.LangAuto {
		return lang
	}
	if err := lang.Set(fileutil.Shebang(src)); err != nil {
		lang = syntax.LangBash
	}
	return lang
}

// langFromFilename is the same as the function of that name in shfmt's main.go.
func langFromFilename(name string) syntax.LangVariant {
	// shell config files typically have no extension nor shebang
	switch strings.TrimPrefix(path.Base(name), ".") {
	case "bash_profile", "bashrc", "bash_logout":
		return syntax.LangBash
	case "zshenv", "zprofile", "zshrc", "zlogin", "zlogout":
		return syntax.LangZsh
	}

	lang := syntax.LangAuto // fallback when none is found
	if ext := strings.TrimPrefix(path.Ext(name), "."); ext != "sh" {
		// ".sh" doesn't mean it's POSIX Shell for sure, since a shebang in the
		// file contents could say Bash or some other POSIX-like shell
		lang.Set(ext)
	}
	return lang
}
