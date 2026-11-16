import { useCallback, useEffect, useRef, useState } from "react";
import { api, errorText, type AgentUsage, type UsageOverview } from "../api";

/** How often limits are read again while the app is in front. */
export const USAGE_REFRESH_MS = 10 * 60_000;
/** Wait this long after a reset before reading, so the agent has caught up. */
const AFTER_RESET_MS = 20_000;
// setTimeout can't wait longer than this.
const LONGEST_TIMER = 2 ** 31 - 1;

/**
 * Each agent's plan limits, shared by the sidebar and the Usage limits page.
 * They are read on start, every ten minutes while the app is in front, when
 * it comes back to the front after a while, and just after a limit resets.
 */
export function useUsageLimits(enabled: boolean) {
  const [overview, setOverview] = useState<UsageOverview>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const lastRead = useRef(0);
  const reading = useRef(false);
  const refresh = useCallback(async (force = false) => {
    if (reading.current) return;
    reading.current = true;
    setLoading(true);
    try {
      const next = await api.UsageLimits(force);
      setOverview(next);
      setError("");
    } catch (e) {
      setError(errorText(e));
    } finally {
      lastRead.current = Date.now();
      reading.current = false;
      setLoading(false);
    }
  }, []);
  useEffect(() => {
    if (!enabled) return;
    void refresh();
    const timer = setInterval(() => {
      if (document.visibilityState === "visible") void refresh();
    }, USAGE_REFRESH_MS);
    const back = () => {
      if (
        document.visibilityState === "visible" &&
        Date.now() - lastRead.current > USAGE_REFRESH_MS / 2
      )
        void refresh();
    };
    document.addEventListener("visibilitychange", back);
    return () => {
      clearInterval(timer);
      document.removeEventListener("visibilitychange", back);
    };
  }, [enabled, refresh]);
  // Read again just after the next limit resets.
  useEffect(() => {
    const next = nextReset(overview?.agents || [], Date.now());
    if (!next) return;
    const delay = next - Date.now() + AFTER_RESET_MS;
    if (delay > LONGEST_TIMER) return;
    const timer = setTimeout(() => void refresh(true), delay);
    return () => clearTimeout(timer);
  }, [overview, refresh]);
  return { overview, loading, error, refresh };
}

/** The earliest reset still ahead, in milliseconds since 1970. */
export function nextReset(agents: AgentUsage[], now: number) {
  let next = 0;
  for (const agent of agents)
    for (const limit of agent.limits) {
      const at = Date.parse(limit.resetsAt || "");
      if (at > now && (!next || at < next)) next = at;
    }
  return next;
}

/** The limit closest to running out, for the sidebar. */
export function tightest(agent: AgentUsage) {
  return agent.limits.reduce<AgentUsage["limits"][number] | undefined>(
    (worst, limit) => (!worst || limit.percent > worst.percent ? limit : worst),
    undefined,
  );
}

function sameDay(a: Date, b: Date) {
  return a.toDateString() === b.toDateString();
}

/** A reset time in local time: "14:30", "tomorrow 09:00" or "Fri 23:42". */
export function resetClock(at: Date, now: Date) {
  const time = at.toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
  });
  if (sameDay(at, now)) return time;
  const tomorrow = new Date(now);
  tomorrow.setDate(now.getDate() + 1);
  if (sameDay(at, tomorrow)) return `tomorrow ${time}`;
  const days = (at.getTime() - now.getTime()) / 86_400_000;
  const day = at.toLocaleDateString(
    [],
    days < 6
      ? { weekday: "short" }
      : { weekday: "short", day: "numeric", month: "short" },
  );
  return `${day} ${time}`;
}

/** How long until a reset: "23 min", "4 h 13 min" or "4 days". */
export function resetIn(at: Date, now: Date) {
  const minutes = Math.max(
    1,
    Math.round((at.getTime() - now.getTime()) / 60_000),
  );
  if (minutes < 60) return `${minutes} min`;
  if (minutes < 48 * 60) {
    const hours = Math.floor(minutes / 60);
    const rest = minutes % 60;
    return rest ? `${hours} h ${rest} min` : `${hours} h`;
  }
  return `${Math.round(minutes / 1440)} days`;
}

/** The words under a limit about its reset. */
export function resetText(resetsAt: string | undefined, now: Date) {
  const at = new Date(resetsAt || "");
  if (!resetsAt || Number.isNaN(at.getTime())) return "Reset time not reported";
  if (at.getTime() <= now.getTime())
    return `Reset at ${resetClock(at, now)} · reading again…`;
  return `Resets in ${resetIn(at, now)} · ${resetClock(at, now)}`;
}

/** The current time, updated every interval, for countdowns. */
export function useNow(interval = 30_000) {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = setInterval(() => setNow(new Date()), interval);
    return () => clearInterval(timer);
  }, [interval]);
  return now;
}
