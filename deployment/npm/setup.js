// @ts-check
const fs = require("fs");
const path = require("path");
const args = process.argv.slice(2);
const rootDir = path.join(__dirname, "../..");
fs.copyFileSync(path.join(rootDir, "target/plugin.wasm"), path.join(__dirname, "plugin.wasm"));
// the license has to be distributed along with the plugin
fs.copyFileSync(path.join(rootDir, "LICENSE"), path.join(__dirname, "LICENSE"));

if (args.length > 0) {
  // update the version based on the first argument
  const packageJsonPath = path.join(__dirname, "package.json");
  const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, "utf8"));
  if (args[0] === "sync-version") {
    packageJson.version = fs.readFileSync(path.join(rootDir, "VERSION"), "utf8").trim();
  } else {
    packageJson.version = args[0];
  }
  fs.writeFileSync(packageJsonPath, JSON.stringify(packageJson, undefined, 2) + "\n");
}
