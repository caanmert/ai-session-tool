import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import type { Backend } from "../api";
import Settings from "./Settings";

beforeEach(() => {
  window.go = {
    main: {
      App: {
        SavedPrompts: vi.fn().mockResolvedValue([]),
        Schedules: vi.fn().mockResolvedValue([]),
        AgentSkills: vi.fn().mockResolvedValue([]),
      } as unknown as Backend,
    },
  };
});

const props = {
  appearance: {
    choice: "system",
    onChange: vi.fn(),
  } as never,
  tools: [],
  health: [],
  checking: false,
  onCheck: vi.fn(),
};

test("an index jumps to each section and marks the one chosen", async () => {
  const scrolled: string[] = [];
  Element.prototype.scrollIntoView = vi.fn(function (this: Element) {
    scrolled.push(this.querySelector("h2")?.textContent || "");
  });
  const view = render(<Settings {...props} />);
  const index = screen.getByRole("navigation", { name: "Settings sections" });
  expect(
    within(index)
      .getAllByRole("link")
      .map((a) => a.textContent),
  ).toEqual([
    "Look",
    "Notifications",
    "Agents",
    "Skills",
    "Saved prompts",
    "Scheduled tasks",
    "Keyboard shortcuts",
    "Your data",
  ]);
  await userEvent.click(
    within(index).getByRole("link", { name: "Scheduled tasks" }),
  );
  expect(scrolled.at(-1)).toBe("Scheduled tasks");
  expect(
    within(index).getByRole("link", { name: "Scheduled tasks" }),
  ).toHaveAttribute("aria-current", "true");
  // Opened from ⌘K on a section.
  view.rerender(
    <Settings {...props} focusSection={{ id: "shortcuts", at: 1 }} />,
  );
  expect(scrolled.at(-1)).toBe("Keyboard shortcuts");
  expect(
    within(index).getByRole("link", { name: "Keyboard shortcuts" }),
  ).toHaveAttribute("aria-current", "true");
});
