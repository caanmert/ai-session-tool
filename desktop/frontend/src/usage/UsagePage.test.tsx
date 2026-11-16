import {
  act,
  render,
  renderHook,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import type { AgentUsage, Backend, Tool } from "../api";
import Sidebar from "../shell/Sidebar";
import UsagePage from "./UsagePage";
import {
  nextReset,
  resetClock,
  resetIn,
  resetText,
  tightest,
  useUsageLimits,
} from "./limits";

const hour = 3_600_000;
const inHours = (hours: number) =>
  new Date(Date.now() + hours * hour).toISOString();
const claude = (): AgentUsage => ({
  tool: "claude",
  plan: "Pro",
  limits: [
    {
      label: "5-hour session",
      window: "session",
      percent: 59,
      resetsAt: inHours(2),
      level: "normal",
    },
    {
      label: "Weekly · all models",
      window: "weekly",
      percent: 88,
      resetsAt: inHours(10),
      level: "warning",
    },
  ],
  extraUsage: "off",
  breakdown: [
    { label: "Claude Code", percent: 95 },
    { label: "Chats", percent: 1 },
  ],
  readAt: new Date().toISOString(),
});
const codex = (): AgentUsage => ({
  tool: "codex",
  plan: "Plus",
  limits: [
    {
      label: "Weekly",
      window: "weekly",
      percent: 100,
      resetsAt: inHours(30),
      level: "reached",
    },
  ],
  blocked: true,
  resetCredits: { available: 3, nextExpiry: inHours(24 * 14) },
  readAt: new Date().toISOString(),
});
const tools: Tool[] = [
  { name: "claude", available: true, path: "/bin/claude" },
  { name: "codex", available: true, path: "/bin/codex" },
];

test("shows each agent's limits, when they reset and what else it reports", async () => {
  const refresh = vi.fn();
  render(
    <UsagePage
      agents={[claude(), codex()]}
      tools={tools}
      loading={false}
      error=""
      onRefresh={refresh}
    />,
  );
  const claudeCard = screen.getByRole("region", { name: "Claude Code limits" });
  expect(within(claudeCard).getByText("Pro plan")).toBeVisible();
  const session = within(claudeCard).getByRole("meter", {
    name: "Claude Code: 5-hour session",
  });
  expect(session).toHaveAttribute("aria-valuenow", "59");
  expect(within(claudeCard).getByText("88% used")).toHaveClass("warning");
  expect(within(claudeCard).getByText(/^Resets in 2 h · /)).toBeVisible();
  expect(
    within(claudeCard).getByText("This week: Claude Code 95% · Chats 1%"),
  ).toBeVisible();
  expect(within(claudeCard).getByText(/Extra usage is off/)).toBeVisible();
  expect(within(claudeCard).queryByText("Limit reached")).toBeNull();

  const codexCard = screen.getByRole("region", { name: "Codex limits" });
  expect(within(codexCard).getByText("Limit reached")).toBeVisible();
  expect(within(codexCard).getByText("Used up")).toBeVisible();
  expect(
    within(codexCard).getByText(
      /3 free full resets available in Codex; the first expires/,
    ),
  ).toHaveTextContent("ais only shows them and never uses one.");

  await userEvent.click(screen.getByRole("button", { name: "Refresh" }));
  expect(refresh).toHaveBeenCalled();
});

test("says why an agent has no limits and leaves out agents not installed", () => {
  render(
    <UsagePage
      agents={[
        {
          tool: "claude",
          limits: [],
          reason:
            "Claude Code reports no plan limits for this sign-in. Limits apply to Claude subscriptions, not to API keys.",
          readAt: new Date().toISOString(),
        },
        { ...codex(), reason: "Codex is not installed.", limits: [] },
      ]}
      tools={[tools[0], { ...tools[1], available: false }]}
      loading
      error=""
      onRefresh={vi.fn()}
    />,
  );
  expect(screen.getByText(/not to API keys/)).toBeVisible();
  expect(screen.queryByRole("region", { name: "Codex limits" })).toBeNull();
  expect(screen.getByRole("button", { name: "Refresh" })).toBeDisabled();
  expect(screen.getAllByText("Reading limits…").length).toBeGreaterThan(0);
});

test("the sidebar sums up each agent and opens the page", async () => {
  const open = vi.fn();
  render(
    <Sidebar
      view="history"
      snapshot={{ sessions: [], projects: [], tools } as never}
      chats={[]}
      activeChat=""
      closingChat=""
      newButton={{ current: null }}
      canStart
      onNew={vi.fn()}
      onHistory={vi.fn()}
      onRuns={vi.fn()}
      onSaved={vi.fn()}
      onSettings={vi.fn()}
      onBoard={vi.fn()}
      onIssues={vi.fn()}
      usage={[claude(), codex()]}
      onUsage={open}
      onSelectChat={vi.fn()}
      onCloseChat={vi.fn()}
    />,
  );
  const claudeGlance = screen.getByRole("button", {
    name: "Claude Code: Session 59%, Week 88%. Open usage limits",
  });
  expect(within(claudeGlance).getByText(/Week 88%/)).toHaveClass("warning");
  expect(
    screen.getByRole("button", {
      name: /^Codex: Limit reached · resets .+\. Open usage limits$/,
    }),
  ).toBeVisible();
  await userEvent.click(claudeGlance);
  await userEvent.click(screen.getByRole("button", { name: "Usage limits" }));
  expect(open).toHaveBeenCalledTimes(2);
});

test("reset times read naturally", () => {
  const now = new Date(2026, 10, 16, 9, 0);
  expect(resetIn(new Date(2026, 10, 16, 9, 23), now)).toBe("23 min");
  expect(resetIn(new Date(2026, 10, 16, 13, 13), now)).toBe("4 h 13 min");
  expect(resetIn(new Date(2026, 10, 16, 11, 0), now)).toBe("2 h");
  expect(resetIn(new Date(2026, 10, 20, 21, 0), now)).toBe("5 days");
  const time = (at: Date) =>
    at.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  const today = new Date(2026, 10, 16, 17, 0);
  expect(resetClock(today, now)).toBe(time(today));
  const tomorrow = new Date(2026, 10, 17, 8, 30);
  expect(resetClock(tomorrow, now)).toBe(`tomorrow ${time(tomorrow)}`);
  expect(resetClock(new Date(2026, 10, 20, 21, 42), now)).toMatch(/^\S+ /);
  expect(resetText(undefined, now)).toBe("Reset time not reported");
  expect(resetText(new Date(2026, 10, 16, 8, 0).toISOString(), now)).toMatch(
    /^Reset at .+ · reading again…$/,
  );
  const agents = [claude(), codex()];
  expect(nextReset(agents, Date.now())).toBe(
    Date.parse(agents[0].limits[0].resetsAt!),
  );
  expect(tightest(claude())?.percent).toBe(88);
});

let backend: Record<string, ReturnType<typeof vi.fn>>;
beforeEach(() => {
  backend = {
    UsageLimits: vi.fn().mockResolvedValue({ agents: [claude(), codex()] }),
  };
  window.go = { main: { App: backend as unknown as Backend } };
});
afterEach(() => {
  vi.useRealTimers();
});

test("reads on start, just after a reset, and every ten minutes", async () => {
  vi.useFakeTimers();
  const soon = {
    ...claude(),
    limits: [
      {
        label: "5-hour session",
        window: "session",
        percent: 99,
        resetsAt: new Date(Date.now() + 60_000).toISOString(),
        level: "warning" as const,
      },
    ],
  };
  backend.UsageLimits.mockResolvedValueOnce({ agents: [soon] });
  const { result } = renderHook(() => useUsageLimits(true));
  await act(async () => {});
  expect(backend.UsageLimits).toHaveBeenCalledWith(false);
  expect(result.current.overview?.agents[0].limits[0].percent).toBe(99);
  // The session resets in a minute: read again, forced, just after.
  await act(async () => {
    await vi.advanceTimersByTimeAsync(60_000 + 20_000);
  });
  expect(backend.UsageLimits).toHaveBeenLastCalledWith(true);
  expect(result.current.overview?.agents[0].limits[0].percent).toBe(59);
  const reads = backend.UsageLimits.mock.calls.length;
  await act(async () => {
    await vi.advanceTimersByTimeAsync(10 * 60_000);
  });
  expect(backend.UsageLimits.mock.calls.length).toBe(reads + 1);
});

test("a failed read is reported and nothing is read while disabled", async () => {
  backend.UsageLimits.mockRejectedValueOnce(
    new Error("conversation manager is starting"),
  );
  const { result, rerender } = renderHook(
    ({ on }: { on: boolean }) => useUsageLimits(on),
    { initialProps: { on: false } },
  );
  await act(async () => {});
  expect(backend.UsageLimits).not.toHaveBeenCalled();
  rerender({ on: true });
  await act(async () => {});
  expect(result.current.error).toContain("conversation manager is starting");
  await act(async () => {
    await result.current.refresh(true);
  });
  expect(result.current.error).toBe("");
  expect(result.current.overview?.agents).toHaveLength(2);
});
