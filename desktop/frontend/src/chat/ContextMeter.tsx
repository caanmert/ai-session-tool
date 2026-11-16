import type { ChatState } from "../api";

const compact = new Intl.NumberFormat("en", {
  notation: "compact",
  maximumFractionDigits: 1,
});
const RADIUS = 6;
const AROUND = 2 * Math.PI * RADIUS;

/**
 * How full the context is, from the agent's last report: percent, tokens and
 * level. Nothing is known until the agent has reported both halves.
 */
export function contextFill(context: ChatState["context"]) {
  if (!context?.window || !context.used) return undefined;
  const share = Math.min(1, context.used / context.window);
  const percent = Math.round(share * 100);
  return {
    share,
    percent,
    level: percent >= 95 ? "full" : percent >= 80 ? "high" : "normal",
    tokens: `${compact.format(context.used)} of ${compact.format(context.window)} tokens`,
  };
}

/** A small ring with the percent of the context used, by the message box. */
export default function ContextMeter({
  context,
}: {
  context: ChatState["context"];
}) {
  const fill = contextFill(context);
  if (!fill) return null;
  const advice =
    fill.level === "normal"
      ? ""
      : " Nearly full: a new conversation starts with an empty context.";
  return (
    <span
      className={`context-meter ${fill.level}`}
      role="meter"
      aria-label="Context used"
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={fill.percent}
      aria-valuetext={`${fill.percent}% of the context used, ${fill.tokens}.${advice}`}
      title={`Context: ${fill.tokens} after the latest response.${advice}`}
    >
      <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
        <circle className="track" cx="8" cy="8" r={RADIUS} />
        <circle
          className="value"
          cx="8"
          cy="8"
          r={RADIUS}
          strokeDasharray={`${fill.share * AROUND} ${AROUND}`}
        />
      </svg>
      {fill.percent}%
    </span>
  );
}
