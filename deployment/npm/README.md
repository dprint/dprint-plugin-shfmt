# @dprint/shfmt

npm distribution of [dprint-plugin-shfmt](https://github.com/dprint/dprint-plugin-shfmt) which is a wrapper plugin for [shfmt](https://github.com/mvdan/sh).

Use this with [@dprint/formatter](https://github.com/dprint/js-formatter) or just use @dprint/formatter and download the [dprint-plugin-shfmt Wasm file](https://github.com/dprint/dprint-plugin-shfmt/releases).

## Example

```ts
import { createFromBuffer } from "@dprint/formatter";
import { getBuffer } from "@dprint/shfmt";

const formatter = createFromBuffer(getBuffer());

console.log(formatter.formatText({
  filePath: "file.sh",
  fileText: "echo   hello\n",
}));
```
