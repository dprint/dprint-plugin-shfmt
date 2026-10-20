# dprint-plugin-shfmt

[![npm version](https://img.shields.io/npm/v/@dprint/shfmt.svg)](https://www.npmjs.com/package/@dprint/shfmt) [![CI](https://github.com/dprint/dprint-plugin-shfmt/workflows/CI/badge.svg)](https://github.com/dprint/dprint-plugin-shfmt/actions?query=workflow%3ACI)

Wrapper for [shfmt](https://github.com/mvdan/sh) for use as a formatting plugin in [dprint](https://github.com/dprint/dprint). It formats shell scripts (POSIX shell, Bash, mksh, Bats and Zsh) exactly like the shfmt CLI does, since it's shfmt's own Go code compiled to Wasm with [TinyGo](https://tinygo.org).

## Install

[Install](https://dprint.dev/install/) and [setup](https://dprint.dev/setup/) dprint.

Then in your project's directory with a dprint.json file, run:

```shellsession
dprint add shfmt
```

Note: You do not need shfmt installed globally as dprint will run shfmt from the .wasm file in a sandboxed environment.

## Configuration

Works with no configuration by default. To add configuration, specify a `"shfmt"` key in your dprint.json:

```jsonc
{
  "shfmt": {
    "useTabs": false,
    "indentWidth": 2,
    "binaryNextLine": true,
    "switchCaseIndent": true,
  },
  "plugins": [
    // ...etc...
  ],
}
```

The options are shfmt's formatting flags:

| Option             | shfmt flag | Default | Description                                                                              |
| ------------------ | ---------- | ------- | ---------------------------------------------------------------------------------------- |
| `dialect`          | `-ln`      | `auto`  | `auto`, `bash`, `posix`, `mksh`, `bats` or `zsh`.                                        |
| `useTabs`          | `-i 0`     | `true`  | Indent with tabs. Uses the global `useTabs` when not specified.                          |
| `indentWidth`      | `-i`       | `2`     | Spaces per indent when not using tabs. Uses the global `indentWidth` when not specified. |
| `binaryNextLine`   | `-bn`      | `false` | Binary operators like `&&` and `\|` may start a line.                                    |
| `switchCaseIndent` | `-ci`      | `false` | Indent the patterns of a `case` statement.                                               |
| `spaceRedirects`   | `-sr`      | `false` | Follow redirect operators with a space (ex. `> file`).                                   |
| `functionNextLine` | `-fn`      | `false` | Put the opening brace of a function on the next line.                                    |
| `simplify`         | `-s`       | `false` | Simplify the code.                                                                       |
| `minify`           | `-mn`      | `false` | Minify the code, which removes comments. Like `-mn`, it implies `simplify`.              |

`-kp` (keep padding) isn't supported. shfmt deprecated it, and its output with it isn't stable, which dprint reports as an error. shfmt reads its options from EditorConfig files when it's given no flags, but the plugin can't read files, so set them in your dprint config instead.

For an overview of the config, see https://dprint.dev/plugins/shfmt/config/

### Dialect

With the default `dialect` of `auto`, the dialect is picked like `shfmt` does:

1. Shell config files: `.bashrc`, `.bash_profile` and `.bash_logout` are Bash, and `.zshrc`, `.zshenv`, `.zprofile`, `.zlogin` and `.zlogout` are Zsh.
2. The file extension: `.bash`, `.mksh`, `.bats` and `.zsh`. A `.sh` file doesn't count, since its shebang could say Bash.
3. The shebang (ex. `#!/bin/sh` is POSIX shell).
4. Otherwise, Bash.

### Files

The plugin formats the same files shfmt finds when walking a directory: `.sh`, `.bash`, `.mksh`, `.bats` and `.zsh` files, along with the shell config files above. To format scripts without an extension, use dprint's [`shebangs`](https://dprint.dev/config/#shebangs) configuration.

## JS Formatting API

- [JS Formatter](https://github.com/dprint/js-formatter) - Browser/Deno and Node
- [npm package](https://www.npmjs.com/package/@dprint/shfmt)

## Versioning

This repo automatically upgrades to the latest version of shfmt once a day. You can check which version of shfmt is being used by looking at the `mvdan.cc/sh/v3` entry in the go.mod file in this repo:

https://github.com/dprint/dprint-plugin-shfmt/blob/main/go.mod

At the moment, the version of this plugin does not reflect the version of shfmt.

## Development

The plugin is built with [TinyGo](https://tinygo.org), which produces a much smaller module than the Go compiler, and one that has no imports, so it doesn't rely on the dprint CLI providing WASI.

```shellsession
go test ./...
deno run -A ./scripts/build.ts # outputs target/plugin.wasm
```

The build uses a local `tinygo` (which needs binaryen's `wasm-opt`) when there is one, and otherwise the TinyGo Docker image.

## Credits

- [shfmt](https://github.com/mvdan/sh) by Daniel Martí, under the BSD 3-Clause license.
- The plugin interface is adapted from Jake Bailey's [dprint-plugin-gofumpt](https://github.com/jakebailey/dprint-plugin-gofumpt).

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details. See [third_party](third_party) for the licenses of the code it includes.
