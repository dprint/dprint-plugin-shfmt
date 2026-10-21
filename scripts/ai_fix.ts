/**
 * Reconciles this plugin with a new shfmt release using OpenAI Codex, in two
 * stages that each run Codex in a session of its own:
 *
 *   1. a Codex agentic session edits the source and runs the checks until they
 *      pass (fixing breakage and/or wiring up new formatter options), then
 *   2. a SEPARATE reviewer session reviews the result with Codex, so it can
 *      investigate (read the upstream shfmt source, grep the repo). It is
 *      instructed to review only and not modify anything. If it finds blocking
 *      issues they are fed back to the stage-1 Codex for another pass, bounded
 *      by `REVIEW_MAX_ROUNDS`. If it still isn't approved, this throws so the
 *      workflow fails and nothing is published.
 *
 * Two situations call this:
 *   - a patch bump that failed the checks (fix the breakage), and
 *   - any minor bump (review for new/renamed/removed options even when it
 *     still compiles).
 *
 * Both stages use the default model of the Codex CLI, which is installed at
 * its latest version when it is missing, so they move to a new model when
 * Codex does. Set `CODEX_MODEL` or `REVIEW_MODEL` to pin the model of a stage.
 *
 * Codex must NOT commit or push -- `update.ts` captures the working tree
 * changes into the existing shfmt bump commit afterwards.
 */
import { $ } from "automation";

export interface AiFixOptions {
  /** true for a patch bump, false for a minor/major bump. */
  isPatchBump: boolean;
  /** shfmt version currently in go.mod (before the bump). */
  fromVersion: string;
  /** shfmt version being upgraded to. */
  toVersion: string;
  /** Whether the checks (vet + test + wasm build) already passed. */
  checksPassed: boolean;
  /** Combined output of the failing checks (empty when `checksPassed`). */
  checkOutput: string;
}

/** Max number of times the reviewer may send changes back to Codex. */
const REVIEW_MAX_ROUNDS = 2;

export async function aiFixShfmtUpdate(options: AiFixOptions): Promise<void> {
  const apiKey = requireApiKey();
  await ensureCodexInstalled();
  await codexLogin(apiKey);

  // stage 1: let Codex reconcile the update.
  await runCodex(buildFixPrompt(options));

  // stage 2: independent second-model review, with a bounded refix loop.
  for (let round = 1;; round++) {
    const review = await reviewChanges(options);
    logReview(review);
    if (review.approved) {
      return;
    }
    if (round > REVIEW_MAX_ROUNDS) {
      const blocking = review.issues.filter((i) => i.severity === "blocking");
      throw new Error(
        `AI reviewer did not approve after ${REVIEW_MAX_ROUNDS} refix round(s). Blocking issues:\n`
          + blocking.map((i) => `  - ${i.description}`).join("\n"),
      );
    }
    $.logStep(`Reviewer requested changes — Codex refix round ${round}...`);
    await runCodex(buildRefixPrompt(review));
  }
}

// stage 1: Codex ---------------------------------------------------------------

async function runCodex(prompt: string): Promise<void> {
  const args = ["exec", "--dangerously-bypass-approvals-and-sandbox", "--skip-git-repo-check"];
  const model = Deno.env.get("CODEX_MODEL");
  if (model) {
    args.push("--model", model);
  }
  args.push(prompt);

  $.logStep("Running Codex...");
  await $`codex ${args}`;
}

function buildFixPrompt(options: AiFixOptions): string {
  const { isPatchBump, fromVersion, toVersion, checksPassed, checkOutput } = options;

  const situation = checksPassed
    ? `shfmt was upgraded from ${fromVersion} to ${toVersion} (a ${
      isPatchBump ? "patch" : "minor"
    } bump). The project already compiles and the checks pass, but a new shfmt version may have ADDED, RENAMED, or REMOVED formatter options that should be surfaced by this plugin.`
    : `shfmt was upgraded from ${fromVersion} to ${toVersion} and the checks fail (the project no longer builds, \`go vet\` or \`go test\` fails, or the TinyGo Wasm build fails). This is usually because the API of mvdan.cc/sh/v3's \`syntax\` or \`fileutil\` packages (\`NewParser\`, \`NewPrinter\`, the \`PrinterOption\`s, \`LangVariant\`, \`Simplify\`, or \`fileutil.Shebang\`) changed, or because the module now needs a Go version that TinyGo doesn't support yet, or because it now formats some code differently.`;

  const failureOutput = checkOutput.trim().length > 0
    ? [
      ``,
      `The checks currently fail with the output below. Use it as your starting point instead of re-running the checks just to rediscover the errors:`,
      "```",
      truncateHead(checkOutput.trim()),
      "```",
    ]
    : [];

  return [
    `You are updating "dprint-plugin-shfmt", a dprint plugin written in Go and compiled to Wasm with TinyGo, that wraps the \`mvdan.cc/sh/v3\` Go module (shfmt) to format shell scripts.`,
    ``,
    situation,
    ...failureOutput,
    ``,
    `Your goal: make the checks pass AND keep this plugin's configuration surface in sync with the formatting flags of the shfmt CLI (\`cmd/shfmt/main.go\`). Do NOT commit or push; only edit files in the working tree.`,
    ``,
    describeWiring(),
    ``,
    `To see exactly what changed in shfmt, inspect the actual dependency source that Go already downloaded, e.g.:`,
    `  find \"$(go env GOMODCACHE)/mvdan.cc/sh\" -maxdepth 1 -type d -name 'v3@v*'`,
    `then read its \`cmd/shfmt/main.go\` (the CLI flags, \`langFromFilename\`, and how it builds the parser and printer) and \`syntax/printer.go\` (the \`PrinterOption\`s), and diff them against the directory of the previous version if it is there too. Compare that against \`FormatText\` in \`internal/plugin/format.go\`.`,
    ``,
    `Rules:`,
    `1. If an option was RENAMED or REMOVED in the formatting flags of the shfmt CLI (\`cmd/shfmt/main.go\`), update the mapping in \`FormatText\` in \`internal/plugin/format.go\` (and remove/rename the corresponding plugin config in the other files if it no longer exists upstream).`,
    `2. If an option was ADDED to the formatting flags of the shfmt CLI (\`cmd/shfmt/main.go\`), expose it as a new plugin config option across ALL of: \`internal/plugin/config.go\` (\`Configuration\`, \`resolveConfig\` and \`marshalConfig\`), \`internal/plugin/format.go\`, and \`deployment/schema.json\`, AND add a test case to \`internal/plugin/format_test.go\` that exercises it. Use the default of the defaults of the shfmt CLI and match the existing naming conventions (Go exported fields, camelCase dprint keys). The same goes for a value added to an upstream enum that this plugin mirrors in its own enum.`,
    `3. The plugin has to format exactly like the shfmt CLI does, so it mirrors the CLI rather than the library: only expose flags of the CLI (not library-only options such as \`SingleLine\`), keep \`resolveLanguage\` and \`langFromFilename\` in sync with how the CLI picks the dialect, and keep \`minify\` implying \`simplify\` like \`-mn\` implies \`-s\`. \`keepPadding\` (\`-kp\`) is deliberately not exposed because shfmt's output with it isn't stable, which dprint reports as an error, so do NOT add it.`,
    `4. If a test fails only because shfmt intentionally formats some code differently now, update that test's expected output and explain what changed in your final message. Never change an expected output to hide a bug in this plugin.`,
    `5. Do NOT edit \`README.md\`. Its config documentation is maintained separately, so leave it untouched.`,
    `6. Preserve the existing code style and run \`gofmt -w .\`. New comments start lowercase unless multiple sentences.`,
    `7. When done, ALL of these must pass (the TinyGo Wasm build is what actually ships, and TinyGo can reject code the Go compiler accepts) — iterate until they are all clean:`,
    `     go vet ./...`,
    `     go test ./...`,
    `     deno run -A ./scripts/build.ts`,
    `8. Do not change the plugin's own version in the VERSION file, do not run git commit, and do not push.`,
    ``,
    `The tests in \`internal/plugin/format_test.go\` are a table of inputs and expected outputs for each option. Add or update cases for options you add or rename, delete the cases of any removed option, and run \`go test ./...\` to confirm they pass. \`internal/plugin/config_test.go\` also checks that the defaults in \`deployment/schema.json\` match the resolved configuration.`,
  ].join("\n");
}

function buildRefixPrompt(review: ReviewResult): string {
  return [
    `An independent reviewer examined your changes to dprint-plugin-shfmt and found issues that must be fixed. Address every blocking issue below, keeping \`go test ./...\` green. Do not commit or push.`,
    ``,
    `Reviewer summary: ${review.summary}`,
    ``,
    `Issues:`,
    ...review.issues.map((i) => `  - [${i.severity}] ${i.description}`),
    ``,
    describeWiring(),
  ].join("\n");
}

function describeWiring(): string {
  return [
    `How the plugin is wired (keep all of these consistent with each other):`,
    `- \`internal/plugin/format.go\` -> \`FormatText\` parses with \`syntax.NewParser\`, simplifies, and prints with \`syntax.NewPrinter\` like the shfmt CLI does, and \`resolveLanguage\` picks the dialect like \`shfmt --filename\` does.`,
    `- \`internal/plugin/config.go\` -> the plugin's own \`Configuration\` struct, \`resolveConfig\` (which reads each dprint config key, camelCase, falling back to the global \`indentWidth\` and \`useTabs\`), and \`marshalConfig\`.`,
    `- \`deployment/schema.json\` -> the JSON schema of config options shown to users.`,
    `- \`internal/plugin/format_test.go\` -> the tests that exercise each config option.`,
  ].join("\n");
}

// keep the tail of long check output -- the errors and the "could not compile"
// summary are at the end, which is the most useful part for the AI.
function truncateHead(text: string, max = 20_000): string {
  return text.length > max ? `... (truncated)\n${text.slice(text.length - max)}` : text;
}

// stage 2: independent reviewer ------------------------------------------------

interface ReviewResult {
  approved: boolean;
  summary: string;
  issues: ReviewIssue[];
}

interface ReviewIssue {
  severity: "blocking" | "nit";
  description: string;
}

// The reviewer runs Codex too so it can actually investigate (read the
// shfmt source Go downloaded, grep the repo, verify option names against
// the real upstream API). Its verdict is captured as JSON via --output-last-message.
//
// NOTE: we deliberately do NOT use `--sandbox read-only` here. That makes Codex
// wrap commands in bubblewrap, which cannot set up its network namespace on the
// GitHub Actions runner ("bwrap: loopback: Failed RTM_NEWADDR: Operation not
// permitted"), so every command the reviewer runs fails before executing. We
// use the same no-sandbox mode as the fixer and enforce read-only via the
// prompt instead. (CI is itself a throwaway VM, and the final check gate in
// update.ts re-verifies whatever ends up in the working tree.)
async function reviewChanges(options: AiFixOptions): Promise<ReviewResult> {
  // the reviewer starts without the context of the fixer, which is what makes
  // it a second opinion. It uses the default model of Codex unless one is set.
  const model = Deno.env.get("REVIEW_MODEL");

  // record intent-to-add so any files Codex created also show in `git diff`.
  await $`git add -N .`.quiet();

  const outputFile = await Deno.makeTempFile({ prefix: "codex-review-", suffix: ".json" });
  try {
    $.logStep(`Reviewing changes with Codex (${model ?? "default model"})...`);
    const args = ["exec", "--skip-git-repo-check", "--dangerously-bypass-approvals-and-sandbox"];
    if (model) {
      args.push("--model", model);
    }
    args.push("--output-last-message", outputFile, buildReviewPrompt(options));
    await $`codex ${args}`;
    return parseReview(await Deno.readTextFile(outputFile));
  } finally {
    await Deno.remove(outputFile).catch(() => {});
  }
}

function buildReviewPrompt(options: AiFixOptions): string {
  return [
    `You are an independent reviewer for dprint-plugin-shfmt, a dprint plugin that wraps the mvdan.cc/sh/v3 Go module (shfmt) to format shell scripts. shfmt was just upgraded from ${options.fromVersion} to ${options.toVersion} and another AI edited this plugin to reconcile it. Review the UNCOMMITTED working-tree changes.`,
    `IMPORTANT: this is REVIEW ONLY. Read any files and run read-only commands (git diff, cat, grep, find, etc.) to investigate, but you MUST NOT edit, create, or delete any files, and MUST NOT run git commit or git push. Leave the working tree exactly as you found it.`,
    ``,
    `Investigate as needed:`,
    `- Run \`git --no-pager diff\` (and \`git status\`) to see exactly what changed, including any new files.`,
    `- Verify against the REAL shfmt ${options.toVersion} API by reading the source Go downloaded:`,
    `    find \"$(go env GOMODCACHE)/mvdan.cc/sh\" -maxdepth 1 -type d -name 'v3@v*'`,
    `  then read its \`cmd/shfmt/main.go\` (the CLI flags, \`langFromFilename\`, and how it builds the parser and printer) and \`syntax/printer.go\` (the \`PrinterOption\`s).`,
    `- Read the plugin files to confirm they are consistent with each other and with upstream.`,
    ``,
    describeWiring(),
    ``,
    `Verify specifically:`,
    `- Every option set in \`FormatText\` in \`internal/plugin/format.go\` still exists in the formatting flags of the shfmt CLI (\`cmd/shfmt/main.go\`) with that exact name (catch removed/renamed options).`,
    `- Any option newly added upstream is exposed through ALL layers: config.go, format.go, and deployment/schema.json. A partial addition is a blocking issue.`,
    `- Every config option added or renamed in this change has a test case in \`internal/plugin/format_test.go\` exercising it (and any removed option's cases are deleted). A missing test is a blocking issue.`,
    `- Naming conventions are consistent (Go exported fields, camelCase dprint keys, matching the schema), and defaults match the defaults of the shfmt CLI.`,
    `- The plugin has to format exactly like the shfmt CLI does, so it mirrors the CLI rather than the library: only expose flags of the CLI (not library-only options such as \`SingleLine\`), keep \`resolveLanguage\` and \`langFromFilename\` in sync with how the CLI picks the dialect, and keep \`minify\` implying \`simplify\` like \`-mn\` implies \`-s\`. \`keepPadding\` (\`-kp\`) is deliberately not exposed because shfmt's output with it isn't stable, which dprint reports as an error, so do NOT add it.`,
    `- Any changed expected output in the tests is explained by an intentional formatting change in shfmt ${options.toVersion}, not by a bug in this plugin. An unexplained change is a blocking issue.`,
    `- README.md was NOT modified (its documentation is maintained separately); flag any README.md change as a blocking issue.`,
    `- No obvious correctness bugs, and code style matches the surrounding code.`,
    ``,
    `When done, your FINAL message must be ONLY a JSON object (no markdown code fences, no extra prose) of exactly this shape:`,
    `{"approved": true|false, "summary": "one sentence", "issues": [{"severity": "blocking"|"nit", "description": "..."}]}`,
    `Set approved=false if there is any blocking issue; nits alone do not block.`,
  ].join("\n");
}

function parseReview(message: string): ReviewResult {
  const review = JSON.parse(extractJsonObject(message)) as ReviewResult;
  if (typeof review.approved !== "boolean" || !Array.isArray(review.issues)) {
    throw new Error(`Codex review returned malformed JSON:\n${message}`);
  }
  return review;
}

// Codex is instructed to emit only JSON, but tolerate an accidental code fence
// or surrounding prose by extracting the outermost { ... } object.
function extractJsonObject(text: string): string {
  const start = text.indexOf("{");
  const end = text.lastIndexOf("}");
  if (start === -1 || end === -1 || end < start) {
    throw new Error(`Could not find a JSON object in Codex review output:\n${text}`);
  }
  return text.slice(start, end + 1);
}

function logReview(review: ReviewResult): void {
  $.log(`Review: ${review.approved ? "approved" : "changes requested"} — ${review.summary}`);
  for (const issue of review.issues) {
    $.logLight(`  [${issue.severity}] ${issue.description}`);
  }
}

// setup ------------------------------------------------------------------------

function requireApiKey(): string {
  const apiKey = Deno.env.get("OPENAI_API_KEY");
  if (!apiKey) {
    throw new Error("OPENAI_API_KEY is not set. It is required to run the AI fixer and reviewer.");
  }
  return apiKey;
}

async function ensureCodexInstalled(): Promise<void> {
  const found = await $`codex --version`.noThrow().quiet();
  if (found.code === 0) {
    return;
  }
  $.logStep("Installing OpenAI Codex CLI...");
  await $`npm install -g @openai/codex`;
}

// `codex exec` does not read OPENAI_API_KEY on its own -- it needs credentials
// stored via `codex login` first, otherwise its requests go out with no auth
// header and the API returns 401. The key is piped over stdin so it never
// appears in the process arguments.
async function codexLogin(apiKey: string): Promise<void> {
  $.logStep("Authenticating Codex with the OpenAI API key...");
  await $`codex login --with-api-key`.stdinText(apiKey);
}
