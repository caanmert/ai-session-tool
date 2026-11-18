import AgentMark from "../AgentMark";
import Icon from "../Icon";
import { toolLabel, type AgentUsage, type Tool } from "../api";
import { when } from "../ui/format";
import { resetClock, resetText, useNow } from "./limits";
import UsageHistorySection from "./UsageHistory";

function percentText(percent: number) {
  return `${Math.round(percent)}%`;
}

function AgentLimits({ agent, now }: { agent: AgentUsage; now: Date }) {
  const name = toolLabel(agent.tool);
  const credits = agent.resetCredits;
  return (
    <section className="usage-card" aria-label={`${name} limits`}>
      <header>
        <AgentMark tool={agent.tool} small />
        <h2>{name}</h2>
        {agent.plan && <span className="chip outline">{agent.plan} plan</span>}
        {agent.blocked && (
          <span className="chip status error">Limit reached</span>
        )}
      </header>
      {agent.reason ? (
        <p className="usage-reason">{agent.reason}</p>
      ) : (
        <ul className="usage-limits">
          {agent.limits.map((limit) => (
            <li key={limit.label}>
              <div className="usage-limit-head">
                <span>{limit.label}</span>
                <b className={limit.level}>
                  {limit.level === "reached"
                    ? "Used up"
                    : `${percentText(limit.percent)} used`}
                </b>
              </div>
              <div
                className={`usage-meter ${limit.level}`}
                role="meter"
                aria-label={`${name}: ${limit.label}`}
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={Math.round(limit.percent)}
                aria-valuetext={`${percentText(limit.percent)} used`}
              >
                <i
                  style={{
                    width: `${Math.min(100, Math.max(0, limit.percent))}%`,
                  }}
                />
              </div>
              <small>{resetText(limit.resetsAt, now)}</small>
            </li>
          ))}
        </ul>
      )}
      {(credits || agent.extraUsage || !!agent.breakdown?.length) && (
        <div className="usage-extra">
          {agent.breakdown && agent.breakdown.length > 0 && (
            <p>
              This week:{" "}
              {agent.breakdown
                .map(
                  // Keep each name with its share on one line.
                  (share) =>
                    `${share.label}\u00a0${percentText(share.percent)}`,
                )
                .join(" · ")}
            </p>
          )}
          {agent.extraUsage === "off" && (
            <p>
              Extra usage is off, so a used-up limit means waiting for its
              reset.
            </p>
          )}
          {agent.extraUsage === "on" && (
            <p>Extra usage is on: work can continue past a limit.</p>
          )}
          {credits && (
            <p>
              {credits.available} free full{" "}
              {credits.available === 1 ? "reset" : "resets"} available in {name}
              {credits.nextExpiry &&
                `; the first expires ${resetClock(new Date(credits.nextExpiry), now)}`}
              . ais only shows them and never uses one.
            </p>
          )}
        </div>
      )}
    </section>
  );
}

/**
 * Each installed agent's plan limits and when they reset, as the agents
 * report them for the account they are signed in to.
 */
export default function UsagePage({
  agents,
  tools,
  loading,
  error,
  onRefresh,
}: {
  agents?: AgentUsage[];
  tools: Tool[];
  loading: boolean;
  error: string;
  onRefresh: () => void;
}) {
  const now = useNow();
  const installed = (agents || []).filter((agent) =>
    tools.some((tool) => tool.name === agent.tool && tool.available),
  );
  const readAt = installed
    .map((agent) => agent.readAt)
    .sort()
    .at(0);
  if (!tools.some((tool) => tool.available))
    return (
      <div className="usage-page">
        <p className="board-empty">
          Install Claude Code or Codex to see their usage limits.
        </p>
      </div>
    );
  return (
    <div className="usage-page" role="region" aria-label="Usage limits">
      <div className="usage-toolbar">
        <p role="status">
          {loading
            ? "Reading limits…"
            : readAt
              ? `Updated ${when(readAt, { inline: true })}`
              : ""}
        </p>
        <button
          className="button secondary"
          disabled={loading}
          onClick={onRefresh}
        >
          <Icon name="refresh" size={14} /> Refresh
        </button>
      </div>
      {error && (
        <p className="notice error" role="alert">
          {error}
        </p>
      )}
      {!agents && !error && (
        <p className="board-empty" role="status">
          Reading limits…
        </p>
      )}
      <div className="usage-grid">
        {installed.map((agent) => (
          <AgentLimits key={agent.tool} agent={agent} now={now} />
        ))}
      </div>
      <p className="usage-note">
        Read from each agent with its own sign-in, the same figures as Claude
        Code&apos;s /usage and Codex&apos;s /status. Reading starts no model
        turn. ais reads them again just after a limit resets, and every ten
        minutes while it is in front.
      </p>
      <UsageHistorySection />
    </div>
  );
}
