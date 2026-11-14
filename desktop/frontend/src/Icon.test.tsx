import { render } from "@testing-library/react";
import { expect, test } from "vitest";
import Icon, { iconNames } from "./Icon";
import AgentMark from "./AgentMark";

test("every icon draws a decorative shape that follows the text color", () => {
  for (const name of iconNames) {
    const { container, unmount } = render(<Icon name={name} size={14} />);
    const svg = container.querySelector("svg")!;
    expect(svg.getAttribute("aria-hidden")).toBe("true");
    expect(svg.getAttribute("stroke")).toBe("currentColor");
    expect(svg.getAttribute("width")).toBe("14");
    expect(svg.children.length, name).toBeGreaterThan(0);
    unmount();
  }
});

test("the design's icon set is available", () => {
  expect(iconNames).toEqual(
    expect.arrayContaining([
      "bell",
      "pencil",
      "file",
      "alert",
      "stop",
      "handoff",
      "code",
      "settings",
      "filter",
      "paperclip",
      "arrow-up",
    ]),
  );
});

test("agent marks carry the agent as a class so shape and color can differ", () => {
  const { container } = render(
    <>
      <AgentMark tool="claude" />
      <AgentMark tool="codex" small />
    </>,
  );
  const [claude, codex] = container.querySelectorAll(".agent-mark");
  expect(claude.className).toBe("agent-mark claude");
  expect(codex.className).toBe("agent-mark codex small");
  expect(claude.getAttribute("aria-hidden")).toBe("true");
  expect(container.querySelectorAll(".agent-dot")).toHaveLength(2);
});
