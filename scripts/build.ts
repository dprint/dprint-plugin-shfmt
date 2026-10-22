/**
 * Builds the plugin to `target/plugin.wasm` with TinyGo.
 *
 * Uses the `tinygo` on the path when there is one, and otherwise the TinyGo
 * Docker image (pass `--docker` to always use it). TinyGo's Wasm target also
 * needs binaryen's `wasm-opt`, which the image comes with.
 */
import { $ } from "automation";
import binaryen from "binaryen";

export const TINYGO_VERSION = "0.43.0";

const rootDirPath = $.path(import.meta.dirname!).parentOrThrow();
const outputPath = rootDirPath.join("target/plugin.wasm");

const tinygoArgs = [
  "build",
  "-target=wasm-unknown",
  // the plugin is single threaded, so it doesn't need goroutines
  "-scheduler=none",
  "-no-debug",
  "-opt=2",
];

if (import.meta.main) {
  await build({ docker: Deno.args.includes("--docker") });
}

export async function build(options: { docker: boolean }) {
  const wasm = options.docker || !(await $.commandExists("tinygo"))
    ? await buildWithDocker()
    : await buildWithLocalTinygo();
  outputPath.parentOrThrow().mkdirSync({ recursive: true });
  outputPath.writeSync(patchWasm(wasm));
  $.log(`Built ${outputPath} (${Math.round(outputPath.statSync()!.size / 1024)} KB)`);
}

async function buildWithLocalTinygo() {
  const tempDir = $.path(await Deno.makeTempDir());
  try {
    const tempPath = tempDir.join("plugin.wasm");
    await $`tinygo ${tinygoArgs} -o ${tempPath}`.cwd(rootDirPath);
    return tempPath.readBytesSync();
  } finally {
    tempDir.removeSync({ recursive: true });
  }
}

async function buildWithDocker() {
  const image = `ghcr.io/tinygo-org/tinygo:${TINYGO_VERSION}`;
  // written to stdout, since a file the container writes is owned by its user
  return await $`docker run --rm -v ${rootDirPath}:/src -w /src -e GOFLAGS=-buildvcs=false ${image} tinygo ${tinygoArgs} -o /dev/stdout`
    .bytes();
}

/**
 * Initializes the Go runtime when the module is instantiated, rather than
 * exporting `_initialize` for the host to call. Older dprint CLIs and
 * `@dprint/formatter` don't call `_initialize`.
 */
function patchWasm(wasm: Uint8Array) {
  const module = binaryen.readBinary(wasm);
  try {
    module.setStart(module.getFunction("_initialize"));
    module.removeExport("_initialize");
    return module.emitBinary();
  } finally {
    module.dispose();
  }
}
