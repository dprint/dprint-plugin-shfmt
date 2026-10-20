/**
 * This script checks for any shfmt (mvdan.cc/sh) updates and then
 * automatically publishes a new version of the plugin if so.
 */
import { $, semver } from "automation";
import { aiFixShfmtUpdate } from "./ai_fix.ts";
import { publishRelease } from "./publish_release.ts";

const modulePath = "mvdan.cc/sh/v3";
const rootDirPath = $.path(import.meta.dirname!).parentOrThrow();
const goModPath = rootDirPath.join("go.mod");
const currentVersion = getGoModVersion();

$.logStep("Getting latest version...");
const latestVersion = await getLatestVersion();
if (semver.compare(semver.parse(currentVersion), semver.parse(latestVersion)) >= 0) {
  $.log("No new update found. Exiting.");
  Deno.exit(0);
}

$.log(`Found new version: ${currentVersion} -> ${latestVersion}`);
const isPatchBump = semver.parse(currentVersion).major === semver.parse(latestVersion).major
  && semver.parse(currentVersion).minor === semver.parse(latestVersion).minor;
$.logStep("Updating go.mod...");
// this also raises the go directive when the new version needs a newer Go
await $`go get ${modulePath}@v${latestVersion}`.cwd(rootDirPath);
await $`go mod tidy`.cwd(rootDirPath);

// Verify the update. A clean patch bump publishes exactly as before. A minor
// bump always gets an AI review (shfmt may have added flags without breaking
// the build), and a patch bump that fails the checks gets an AI fix attempt.
$.logStep("Running checks (vet + test + wasm build)...");
const checks = await runChecks();

if (!isPatchBump || !checks.passed) {
  if (checks.passed) {
    $.logStep("Minor shfmt update — running AI review for new/changed options...");
  } else {
    $.logStep(`${isPatchBump ? "Patch" : "Minor"} update failed the checks — running AI fix...`);
  }
  await aiFixShfmtUpdate({
    isPatchBump,
    fromVersion: currentVersion,
    toVersion: latestVersion,
    checksPassed: checks.passed,
    // hand the failing output to the AI so it can go straight to fixing
    // instead of re-running the checks just to rediscover the errors.
    checkOutput: checks.output,
  });

  // the AI must leave the project in a passing state, otherwise fail the
  // workflow (nothing gets published and the maintainer is notified).
  $.logStep("Re-running checks after AI changes...");
  await assertChecks();
}

if (Deno.args.includes("--skip-publish")) {
  Deno.exit(0);
}

$.logStep("Committing shfmt version bump commit...");
await $`git add .`;
const message = `${isPatchBump ? "fix" : "feat"}: update to shfmt ${latestVersion}`;
await $`git commit -m ${message}`;

await publishRelease(isPatchBump ? "patch" : "minor");

// the checks that must pass before publishing. The wasm build is included
// because that is what actually ships, and TinyGo can fail on code that the
// Go compiler accepts. `inheritPiped` + `captureCombined` streams the output to
// the CI log live while also capturing it so a failure can be handed to the AI.
async function runChecks(): Promise<{ passed: boolean; output: string }> {
  const results = [];
  for (const command of checkCommands()) {
    results.push(await capture(command()));
  }
  const failures = results.filter((r) => r.code !== 0);
  return {
    passed: failures.length === 0,
    output: failures.map((r) => r.combined).join("\n\n"),
  };
}

function capture(command: ReturnType<typeof $>) {
  return command
    .stdout("inheritPiped")
    .stderr("inheritPiped")
    .captureCombined()
    .noThrow();
}

// same checks as `runChecks`, but throws on the first failure so the workflow
// aborts before anything is committed, tagged, or published.
async function assertChecks(): Promise<void> {
  for (const command of checkCommands()) {
    await command();
  }
}

// a function (not a top-level const) so it is hoisted -- `runChecks` is called
// from top-level code above before a const declared here would be initialized.
function checkCommands() {
  return [
    () => $`go vet ./...`.cwd(rootDirPath),
    () => $`go test ./...`.cwd(rootDirPath),
    () => $`deno run -A ./scripts/build.ts`.cwd(rootDirPath),
  ];
}

function getGoModVersion() {
  const match = goModPath.readTextSync().match(/^\s*mvdan\.cc\/sh\/v3 v(\S+)$/m);
  const version = match?.[1];
  if (version == null) {
    throw new Error(`Could not find ${modulePath} version in go.mod.`);
  }
  $.logLight(`Found ${modulePath} version in go.mod:`, version);
  return version;
}

async function getLatestVersion() {
  const data = await $.request(`https://proxy.golang.org/${modulePath}/@latest`)
    .json<{ Version?: string }>();
  const version = data.Version?.replace(/^v/, "");
  if (version == null) {
    throw new Error(`Could not find the latest version of ${modulePath} on the Go module proxy.`);
  }
  $.logLight(`Latest ${modulePath} version:`, version);
  return version;
}
