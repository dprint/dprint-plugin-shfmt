// @ts-check
const assert = require("assert");
const createFromBuffer = require("@dprint/formatter").createFromBuffer;
const getBuffer = require("./index").getBuffer;

const formatter = createFromBuffer(getBuffer());

assert.strictEqual(
  formatter.formatText({
    filePath: "file.sh",
    fileText: "if [[ -n $1 ]]; then\necho   hi\nfi\n",
  }),
  "if [[ -n $1 ]]; then\n\techo hi\nfi\n",
);

// deeply nested code doesn't overflow the stack
const depth = 200;
let nestedExpected = "";
for (let i = 0; i < depth; i++) {
  nestedExpected += `${"\t".repeat(i)}if true; then\n`;
}
nestedExpected += `${"\t".repeat(depth)}echo hi\n`;
for (let i = depth - 1; i >= 0; i--) {
  nestedExpected += `${"\t".repeat(i)}fi\n`;
}
assert.strictEqual(
  formatter.formatText({
    filePath: "file.sh",
    fileText: `${"if true; then\n".repeat(depth)}echo hi\n${"fi\n".repeat(depth)}`,
  }),
  nestedExpected,
);

// uses the global config and the plugin's own config
formatter.setConfig({ indentWidth: 2, useTabs: false }, { binaryNextLine: true });
assert.strictEqual(
  formatter.formatText({
    filePath: "file.sh",
    fileText: "if true; then\nfoo &&\nbar\nfi\n",
  }),
  "if true; then\n  foo \\\n    && bar\nfi\n",
);

assert.throws(
  () => formatter.formatText({ filePath: "file.sh", fileText: "if then\n" }),
  /`if` must be followed by a statement list/,
);
