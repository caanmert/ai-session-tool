import type { Session } from "../api";

const compact = new Intl.NumberFormat("en", {
  notation: "compact",
  maximumFractionDigits: 1,
});

/** All tokens of a session, including cached ones, in compact form. */
export function sessionTokens(session: Session) {
  const u = session.usage;
  return compact.format(u.input + u.output + u.cacheRead + u.cacheWrite);
}

/** "$0.84", "<$0.01", or "$12.40+" when part of the usage has no price. */
export function sessionCost(session: Session) {
  if (session.cost === undefined || session.cost === null) return "";
  const amount =
    session.cost > 0 && session.cost < 0.01
      ? "<$0.01"
      : "$" + session.cost.toFixed(2);
  return session.costPartial ? amount + "+" : amount;
}

const clock = (d: Date) =>
  d.toLocaleTimeString("en-US", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });

/** Calendar days from now's day to d's day: -1 is yesterday. */
function dayOffset(d: Date, now: Date) {
  const start = (x: Date) =>
    new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  return Math.round((start(d) - start(now)) / 86_400_000);
}

/**
 * Every date in the app, one way: "Just now", "12m ago" and "3h ago" for the
 * last hours; "Today 09:30", "Yesterday 18:00", "Tomorrow 09:00" or
 * "Tue 18:00" within a week; "Nov 4" (and "Nov 4, 2025" in another year)
 * beyond. With inline set, a leading word is lower case, for use mid-sentence.
 */
export function when(
  value: string | Date,
  { now = new Date(), inline = false }: { now?: Date; inline?: boolean } = {},
) {
  const d = typeof value === "string" ? new Date(value) : value;
  if (Number.isNaN(d.getTime())) return "";
  const lower = (text: string) =>
    inline
      ? text.replace(/^(Just|Today|Yesterday|Tomorrow)/, (w) => w.toLowerCase())
      : text;
  const ago = now.getTime() - d.getTime();
  if (ago >= 0 && ago < 60_000) return lower("Just now");
  if (ago >= 0 && ago < 3_600_000) return `${Math.floor(ago / 60_000)}m ago`;
  if (ago >= 0 && ago < 12 * 3_600_000)
    return `${Math.floor(ago / 3_600_000)}h ago`;
  const days = dayOffset(d, now);
  if (days === 0) return lower(`Today ${clock(d)}`);
  if (days === -1) return lower(`Yesterday ${clock(d)}`);
  if (days === 1) return lower(`Tomorrow ${clock(d)}`);
  if (Math.abs(days) < 7)
    return `${d.toLocaleDateString("en-US", { weekday: "short" })} ${clock(d)}`;
  return d.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    ...(d.getFullYear() === now.getFullYear() ? {} : { year: "numeric" }),
  });
}

/** How long ago a time was, in words: the same as when(). */
export function age(date: string) {
  return when(date);
}

/** "1 prompt", "2 prompts"; many defaults to one + "s". */
export function plural(count: number, one: string, many = `${one}s`) {
  return `${count} ${count === 1 ? one : many}`;
}

/** The last part of a path. */
export function basename(path: string) {
  return path.split("/").filter(Boolean).at(-1) || path;
}
