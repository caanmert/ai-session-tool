import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { SafeMarkdown } from "./SafeMarkdown";

test("renders tables, task lists and strikethrough", () => {
  render(
    <SafeMarkdown
      text={[
        "| File | Change |",
        "| --- | --- |",
        "| app.ts | retry |",
        "",
        "- [x] Add retries",
        "- [ ] Update docs",
        "",
        "~~old plan~~",
      ].join("\n")}
    />,
  );
  const table = screen.getByRole("table");
  expect(
    within(table).getByRole("columnheader", { name: "File" }),
  ).toBeVisible();
  expect(within(table).getByRole("cell", { name: "retry" })).toBeVisible();
  // Task boxes show state; they are not checkboxes the reader can toggle.
  expect(screen.getByRole("img", { name: "Done" })).toBeVisible();
  expect(screen.getByRole("img", { name: "Not done" })).toBeVisible();
  expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
  expect(screen.getByText("old plan").tagName).toBe("DEL");
});

test("highlights named languages and copies the code as written", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  const { container } = render(
    <SafeMarkdown
      text={
        '```ts\nconst retries = 3;\nreturn "ok";\n```\n\n```\nplain <b>text</b>\n```'
      }
    />,
  );
  expect(screen.getByText("ts")).toBeVisible();
  expect(container.querySelector(".hljs-keyword")).toHaveTextContent("const");
  expect(container.querySelector(".hljs-string")).toHaveTextContent('"ok"');
  // A block without a language is left alone and never becomes markup.
  expect(screen.getByText("text")).toBeVisible();
  expect(container.querySelector("b")).toBeNull();
  const [copy] = screen.getAllByRole("button", { name: "Copy code" });
  await userEvent.click(copy);
  expect(writeText).toHaveBeenCalledWith('const retries = 3;\nreturn "ok";');
  expect(await within(copy).findByText("Copied")).toBeVisible();
});

test("keeps provider output inert: no links, images, scripts or raw HTML", () => {
  const { container } = render(
    <SafeMarkdown
      text={
        '[docs](https://example.com) https://example.org ![x](https://example.com/a.png)\n\n<script>alert(1)</script><img src="x" onerror="alert(1)">'
      }
    />,
  );
  expect(container.querySelector("a, img, script")).toBeNull();
  expect(screen.getByText("docs")).toHaveClass("transcript-link");
  expect(screen.getByText("[Image omitted]")).toBeVisible();
});
