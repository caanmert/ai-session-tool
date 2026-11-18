import { useEffect, useState } from "react";
import { api, errorText, toolLabel, type UsageHistory } from "../api";
import { duration, tokenCount } from "../chat/turnStats";
import { basename } from "../ui/format";

const RANGES = [7, 30] as const;

function dayLabel(day: string, weekday = true) {
  return new Date(`${day}T12:00:00`).toLocaleDateString("en-US", {
    weekday: weekday ? "short" : undefined,
    day: "numeric",
    month: "short",
  });
}

function money(usd: number) {
  return usd > 0 && usd < 0.01 ? "<$0.01" : `$${usd.toFixed(2)}`;
}

/** One line for a set of turns: turns, time, tokens and cost if any. */
export function totalsLine(totals: UsageHistory["total"]) {
  if (!totals.turns) return "No turns";
  const parts = [
    `${totals.turns} ${totals.turns === 1 ? "turn" : "turns"}`,
    duration(totals.ms),
  ];
  if (totals.tokens) parts.push(`${tokenCount(totals.tokens)} tokens`);
  if (totals.costUsd) parts.push(`${money(totals.costUsd)} est.`);
  return parts.join(" · ");
}

function Breakdown({
  title,
  rows,
  label,
}: {
  title: string;
  rows: UsageHistory["projects"];
  label: (key: string) => string;
}) {
  return (
    <table className="usage-table">
      <caption>{title}</caption>
      <thead>
        <tr>
          <th scope="col">
            {title.replace(/^By /, "").replace(/^\w/, (c) => c.toUpperCase())}
          </th>
          <th scope="col">Turns</th>
          <th scope="col">Time</th>
          <th scope="col">Tokens</th>
          <th scope="col">Cost est.</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr key={row.key}>
            <th scope="row" title={row.key}>
              {label(row.key)}
            </th>
            <td>{row.turns}</td>
            <td>{duration(row.ms)}</td>
            <td>{row.tokens ? tokenCount(row.tokens) : "—"}</td>
            <td>{row.costUsd ? money(row.costUsd) : "—"}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/**
 * How much the agents worked in ais, by day, project and agent, added up from
 * the figures saved with each turn. Read-only and local.
 */
export default function UsageHistorySection() {
  const [days, setDays] = useState<(typeof RANGES)[number]>(7);
  const [history, setHistory] = useState<UsageHistory>();
  const [error, setError] = useState("");
  const [hovered, setHovered] = useState<number>();
  useEffect(() => {
    let active = true;
    setError("");
    api
      .UsageHistory(days)
      .then((next) => active && setHistory(next))
      .catch((e) => active && setError(errorText(e)));
    return () => {
      active = false;
    };
  }, [days]);
  const shown = history?.days.length === days ? history : undefined;
  const peak = Math.max(1, ...(shown?.days.map((d) => d.ms) || []));
  const focusDay = hovered === undefined ? undefined : shown?.days[hovered];
  return (
    <section className="usage-history" aria-labelledby="usage-history-title">
      <header>
        <h2 id="usage-history-title">History</h2>
        <div className="history-views" role="group" aria-label="Period">
          {RANGES.map((range) => (
            <button
              key={range}
              type="button"
              aria-pressed={days === range}
              onClick={() => {
                setDays(range);
                setHovered(undefined);
              }}
            >
              {range} days
            </button>
          ))}
        </div>
      </header>
      {error && (
        <p className="notice error" role="alert">
          {error}
        </p>
      )}
      {!shown && !error && <p role="status">Adding up turns…</p>}
      {shown && !shown.total.turns && (
        <p className="usage-reason">
          No turns in the last {days} days. Turns count from when ais started
          recording how long each one took.
        </p>
      )}
      {shown && !!shown.total.turns && (
        <>
          <p className="usage-readout" aria-live="polite">
            <b>{focusDay ? dayLabel(focusDay.day) : `Last ${days} days`}</b>
            {totalsLine(focusDay || shown.total)}
          </p>
          <div
            className="usage-chart"
            role="img"
            aria-label={`Time worked per day over the last ${days} days. The tables below list the figures.`}
            onMouseLeave={() => setHovered(undefined)}
          >
            {shown.days.map((day, i) => (
              <span
                key={day.day}
                className="usage-bar"
                onMouseEnter={() => setHovered(i)}
              >
                <i
                  style={{
                    height: day.ms
                      ? `${Math.max(2, (day.ms / peak) * 100)}%`
                      : 0,
                  }}
                />
              </span>
            ))}
          </div>
          <div className="usage-axis" aria-hidden="true">
            <span>{dayLabel(shown.days[0].day, false)}</span>
            <span>Today</span>
          </div>
          <div className="usage-tables">
            <Breakdown
              title="By project"
              rows={shown.projects}
              label={(key) => basename(key) || key}
            />
            <Breakdown
              title="By agent"
              rows={shown.agents}
              label={(key) => toolLabel(key)}
            />
          </div>
        </>
      )}
      <p className="usage-note">
        Added up from the turns run in ais, from when it started recording how
        long each took; nothing is read from the agents. Tokens are what each
        agent reported, cached input included. Cost is Claude Code&apos;s own
        estimate at API prices, not a charge on a subscription; Codex reports
        none.
      </p>
    </section>
  );
}
