import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import OpenSessions from "./OpenSessions";
import type { ChatState } from "../api";

it("groups by project, shows explicit states, supports keyboard switching and guards busy rows", async () => {
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
    const [active, setActive] = useState("one");
    return (
      <OpenSessions
        chats={chats}
        active={active}
        onSelect={setActive}
        onClose={close}
        closing=""
      />
    );
  }
  render(<Tabs />);
  expect(screen.getByRole("heading", { name: "api" })).toBeVisible();
  screen.getByRole("button", { name: /Fix webhook retries Codex/ }).focus();
  await userEvent.keyboard("{End}");
  const review = screen.getByRole("button", {
    name: /Review checkout Claude · Needs your input/,
  });
  expect(review).toHaveFocus();
  expect(review).toHaveAttribute("aria-pressed", "true");
  expect(
    screen.getByRole("button", { name: "Close Review checkout" }),
  ).toBeDisabled();
  await userEvent.keyboard("{ArrowUp}");
  expect(
    screen.getByRole("button", { name: /Fix webhook retries Codex/ }),
  ).toHaveAttribute("aria-pressed", "true");
  await userEvent.keyboard("{ArrowUp}");
  expect(review).toHaveAttribute("aria-pressed", "true");
  await userEvent.click(
    screen.getByRole("button", { name: "Close Fix webhook retries" }),
  );
  expect(close).toHaveBeenCalledWith("one");
});
