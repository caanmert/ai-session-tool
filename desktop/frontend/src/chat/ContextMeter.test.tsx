import { render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import type { ChatState } from "../api";
import ContextMeter, { contextFill } from "./ContextMeter";
import DetailsPanel from "./DetailsPanel";

test("the fill is known only once both halves are reported", () => {
  expect(contextFill(undefined)).toBeUndefined();
  expect(contextFill({ used: 9000 })).toBeUndefined();
  expect(contextFill({ used: 0, window: 200000 })).toBeUndefined();
  expect(contextFill({ used: 90000, window: 200000 })).toMatchObject({
    percent: 45,
    level: "normal",
    tokens: "90K of 200K tokens",
  });
  expect(contextFill({ used: 158000, window: 200000 })?.level).toBe("normal");
  expect(contextFill({ used: 160000, window: 200000 })?.level).toBe("high");
  expect(contextFill({ used: 190000, window: 200000 })?.level).toBe("full");
  // A count past the window, as after a long tool result, stays at 100%.
  expect(contextFill({ used: 260000, window: 200000 })?.percent).toBe(100);
});

test("the message box shows the percent, with the tokens and advice in words", () => {
  const { rerender } = render(
    <ContextMeter context={{ used: 168000, window: 200000 }} />,
  );
  const meter = screen.getByRole("meter", { name: "Context used" });
  expect(meter).toHaveTextContent("84%");
  expect(meter).toHaveAttribute("aria-valuenow", "84");
  expect(meter).toHaveClass("high");
  expect(meter.getAttribute("aria-valuetext")).toBe(
    "84% of the context used, 168K of 200K tokens. Nearly full: a new conversation starts with an empty context.",
  );
  rerender(<ContextMeter context={{ used: 1200 }} />);
  expect(screen.queryByRole("meter")).toBeNull();
});

test("Session details list the context next to the tokens", () => {
  const chat = {
    id: "chat",
    tool: "codex",
    project: "/work/app",
    title: "Retries",
    status: "idle",
    messages: [],
    context: { used: 50000, window: 272000 },
  } as unknown as ChatState;
  render(
    <DetailsPanel
      chat={chat}
      summary={{ available: true, files: [], added: 0, removed: 0 }}
      error=""
      drawer={false}
      onRefresh={vi.fn()}
      onClose={vi.fn()}
    />,
  );
  expect(screen.getByText("Context")).toBeVisible();
  expect(screen.getByText("18% used")).toBeVisible();
  expect(screen.getByText("18% used").closest("div")).toHaveAttribute(
    "title",
    "50K of 272K tokens after the latest response",
  );
});
