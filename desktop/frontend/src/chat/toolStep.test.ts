import { expect, test } from "vitest";
import type { ChatMessage } from "../api";
import { describeToolStep, shortPath, splitToolText } from "./toolStep";

const step = (name: string, input: unknown, result = ""): ChatMessage => ({
  id: "t",
  role: "assistant",
  kind: "tool",
  name,
  status: "completed",
  text:
    (typeof input === "string" ? input : JSON.stringify(input, null, 2)) +
    (result ? "\n\n" + result : ""),
});

test("splits recorded input from the result, and keeps partial input as text", () => {
  expect(
    splitToolText('{\n  "command": "npm test"\n}\n\nPASS\n\n2 passed'),
  ).toEqual({ input: { command: "npm test" }, output: "PASS\n\n2 passed" });
  expect(splitToolText("{}")).toEqual({ input: {}, output: "" });
  // Input still streaming in.
  expect(splitToolText('{"command": "npm te')).toEqual({
    output: '{"command": "npm te',
  });
  // Codex command output is plain text.
  expect(splitToolText("PASS checkout")).toEqual({ output: "PASS checkout" });
});

test("shows paths relative to the project", () => {
  expect(shortPath("/work/app/src/a.ts", "/work/app")).toBe("src/a.ts");
  expect(shortPath("/work/app-two/a.ts", "/work/app")).toBe(
    "/work/app-two/a.ts",
  );
  expect(shortPath("/etc/hosts", "")).toBe("/etc/hosts");
});

test("describes reads, commands and searches by what they touched", () => {
  expect(
    describeToolStep(
      step("Read", { file_path: "/work/app/src/a.ts" }, "1 a"),
      "/work/app",
    ),
  ).toMatchObject({
    verb: "Read",
    target: "src/a.ts",
    icon: "file",
    output: "1 a",
  });
  expect(
    describeToolStep(
      step(
        "Bash",
        { command: "npm test auth", description: "Run tests" },
        "FAIL",
      ),
    ),
  ).toMatchObject({ verb: "Run", target: "npm test auth", output: "FAIL" });
  expect(
    describeToolStep(
      step("Grep", { pattern: "jwt.sign", path: "/work/app/src" }),
      "/work/app",
    ),
  ).toMatchObject({ verb: "Search", target: "jwt.sign  in  src" });
  // Codex records the command line as the step's name and its output as text.
  expect(
    describeToolStep({
      ...step("npm test -- checkout", "3 passed"),
      text: "3 passed",
    }),
  ).toMatchObject({
    verb: "Run",
    target: "npm test -- checkout",
    output: "3 passed",
  });
  expect(describeToolStep(step("TodoWrite", { todos: [] }))).toMatchObject({
    verb: "TodoWrite",
    icon: "code",
  });
});

test("turns edits into removed and added lines", () => {
  const edit = describeToolStep(
    step(
      "Edit",
      {
        file_path: "/work/app/src/keys.ts",
        old_string: "return env.SECRET;\n",
        new_string: "const key = store.current();\nreturn key.secret;\n",
      },
      "The file was updated.",
    ),
    "/work/app",
  );
  expect(edit).toMatchObject({
    verb: "Edit",
    target: "src/keys.ts",
    icon: "pencil",
  });
  expect(edit.diff).toEqual([
    { kind: "del", text: "return env.SECRET;" },
    { kind: "add", text: "const key = store.current();" },
    { kind: "add", text: "return key.secret;" },
  ]);
  expect(edit.output).toBe("The file was updated.");

  const multi = describeToolStep(
    step("MultiEdit", {
      file_path: "a.ts",
      edits: [
        { old_string: "a", new_string: "b" },
        { old_string: "c", new_string: "" },
      ],
    }),
  );
  expect(multi.diff).toEqual([
    { kind: "del", text: "a" },
    { kind: "add", text: "b" },
    { kind: "del", text: "c" },
  ]);

  const write = describeToolStep(
    step("Write", { file_path: "new.ts", content: "one\ntwo\n" }),
  );
  expect(write).toMatchObject({ verb: "Write", target: "new.ts" });
  expect(write.diff).toEqual([
    { kind: "add", text: "one" },
    { kind: "add", text: "two" },
  ]);
});

test("caps very long edits and says how much was left out", () => {
  const content = Array.from({ length: 450 }, (_, i) => `line ${i}`).join("\n");
  const write = describeToolStep(
    step("Write", { file_path: "big.ts", content }),
  );
  expect(write.diff).toHaveLength(400);
  expect(write.diffOmitted).toBe(50);
});

test("an edit whose input is still streaming shows the raw text and no diff", () => {
  const partial = describeToolStep({
    ...step("Edit", ""),
    text: '{"file_path": "/work/app/src/ke',
    status: "running",
  });
  expect(partial.diff).toBeUndefined();
  expect(partial.output).toBe('{"file_path": "/work/app/src/ke');
});

test("a Codex web search reads as a web step with its query", () => {
  const step = describeToolStep({
    id: "w",
    role: "assistant",
    kind: "tool",
    name: "webSearch",
    status: "completed",
    text: JSON.stringify(
      { type: "webSearch", id: "ws_1", query: "codex web_search modes" },
      null,
      2,
    ),
  });
  expect([step.verb, step.target]).toEqual(["Web", "codex web_search modes"]);
});

test("reads a compaction as one step with its note", () => {
  const view = describeToolStep(
    step("Conversation compacted", "The agent summarized the conversation."),
  );
  expect([view.icon, view.verb, view.target, view.output]).toEqual([
    "gauge",
    "Compacted",
    "the context",
    "The agent summarized the conversation.",
  ]);
});
