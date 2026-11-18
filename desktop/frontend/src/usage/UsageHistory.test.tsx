import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import type { Backend, UsageHistory } from "../api";
import UsageHistorySection, { totalsLine } from "./UsageHistory";

const none = { turns: 0, ms: 0, tokens: 0, costUsd: 0 };
function history(days: number): UsageHistory {
  const list = Array.from({ length: days }, (_, i) => ({
    ...none,
    day: `2026-11-${String(19 - days + i).padStart(2, "0")}`,
  }));
  list[days - 1] = {
    ...list[days - 1],
    turns: 2,
    ms: 180_000,
    tokens: 5_000,
    costUsd: 0.5,
  };
  list[days - 2] = { ...list[days - 2], turns: 1, ms: 30_000, tokens: 500 };
  return {
    days: list,
    projects: [
      { key: "/work/shop", turns: 2, ms: 150_000, tokens: 4_000, costUsd: 0 },
      { key: "/work/site", turns: 1, ms: 60_000, tokens: 1_500, costUsd: 0.5 },
    ],
    agents: [
      { key: "codex", turns: 2, ms: 150_000, tokens: 4_000, costUsd: 0 },
      { key: "claude", turns: 1, ms: 60_000, tokens: 1_500, costUsd: 0.5 },
    ],
    total: { turns: 3, ms: 210_000, tokens: 5_500, costUsd: 0.5 },
  };
}

let backend: Record<string, ReturnType<typeof vi.fn>>;
beforeEach(() => {
  backend = {
    UsageHistory: vi.fn(async (days: number) => history(days)),
  };
  window.go = { main: { App: backend as unknown as Backend } };
});

test("adds turns up in one line", () => {
  expect(totalsLine(none)).toBe("No turns");
  expect(totalsLine({ turns: 1, ms: 9_000, tokens: 0, costUsd: 0 })).toBe(
    "1 turn · 9s",
  );
  expect(
    totalsLine({ turns: 3, ms: 210_000, tokens: 5_500, costUsd: 0.004 }),
  ).toBe("3 turns · 3m 30s · 5.5k tokens · <$0.01 est.");
});

test("shows the week by day, project and agent, and switches to 30 days", async () => {
  render(<UsageHistorySection />);
  expect(
    await screen.findByText("3 turns · 3m 30s · 5.5k tokens · $0.50 est."),
  ).toBeVisible();
  expect(backend.UsageHistory).toHaveBeenCalledWith(7);
  const bars = document.querySelectorAll(".usage-bar");
  expect(bars).toHaveLength(7);
  // The busiest day fills the chart; a day without turns draws no bar.
  expect((bars[6].firstChild as HTMLElement).style.height).toBe("100%");
  expect((bars[0].firstChild as HTMLElement).style.height).toBe("0px");
  // Hovering a day reads it out.
  fireEvent.mouseEnter(bars[5]);
  expect(screen.getByText("Tue, Nov 17")).toBeVisible();
  expect(screen.getByText("1 turn · 30s · 500 tokens")).toBeVisible();
  const projects = screen.getByRole("table", { name: "By project" });
  expect(
    within(projects).getByRole("rowheader", { name: "shop" }),
  ).toHaveAttribute("title", "/work/shop");
  const agents = screen.getByRole("table", { name: "By agent" });
  expect(
    within(agents)
      .getAllByRole("rowheader")
      .map((r) => r.textContent),
  ).toEqual(["Codex", "Claude Code"]);
  await userEvent.click(screen.getByRole("button", { name: "30 days" }));
  expect(backend.UsageHistory).toHaveBeenLastCalledWith(30);
  expect(await screen.findByText("Last 30 days")).toBeVisible();
  expect(document.querySelectorAll(".usage-bar")).toHaveLength(30);
});

test("says when there is nothing to add up, or the read failed", async () => {
  backend.UsageHistory = vi.fn().mockResolvedValue({
    ...history(7),
    total: none,
  });
  const view = render(<UsageHistorySection />);
  expect(await screen.findByText(/No turns in the last 7 days/)).toBeVisible();
  view.unmount();
  backend.UsageHistory = vi.fn().mockRejectedValue(new Error("not ready"));
  render(<UsageHistorySection />);
  expect(await screen.findByRole("alert")).toHaveTextContent("not ready");
});
