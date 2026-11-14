import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import WorkspaceTabs from "./WorkspaceTabs";
import type { ChatState } from "./api";

it("uses titles and explicit states, supports keyboard switching and guards busy tabs", async () => {
  const close = vi.fn();
  const chats = [
    {
      id: "one",
      title: "Fix webhook retries",
      tool: "codex",
      project: "/api",
      status: "idle",
    },
    {
      id: "two",
      title: "Review checkout",
      tool: "claude",
      project: "/api",
      status: "waiting",
    },
  ] as ChatState[];
  function Tabs() {
    const [active, setActive] = useState("");
    return (
      <WorkspaceTabs
        chats={chats}
        active={active}
        onSelect={setActive}
        onClose={close}
        closing=""
      />
    );
  }
  render(<Tabs />);
  screen.getByRole("button", { name: "History" }).focus();
  await userEvent.keyboard("{End}");
  expect(
    screen.getByRole("button", { name: /Review checkout Claude/ }),
  ).toHaveFocus();
  expect(screen.getByRole("img", { name: "Needs your input" })).toBeVisible();
  expect(
    screen.getByRole("button", { name: "Close Review checkout" }),
  ).toBeDisabled();
  await userEvent.keyboard("{ArrowLeft}");
  expect(
    screen.getByRole("button", { name: /Fix webhook retries Codex/ }),
  ).toHaveAttribute("aria-pressed", "true");
  await userEvent.click(
    screen.getByRole("button", { name: "Close Fix webhook retries" }),
  );
  expect(close).toHaveBeenCalledWith("one");
});
