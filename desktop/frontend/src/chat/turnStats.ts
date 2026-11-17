import type { TurnStats } from "../api";

/** A duration as "45s", "2m 14s" or "1h 05m". */
export function duration(ms: number) {
  const seconds = Math.max(0, Math.round(ms / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60)
    return `${minutes}m ${String(seconds % 60).padStart(2, "0")}s`;
  return `${Math.floor(minutes / 60)}h ${String(minutes % 60).padStart(2, "0")}m`;
}

/** A token count as "820", "4.3k", "48k" or "1.2M". */
export function tokenCount(tokens: number) {
  if (tokens < 1000) return String(tokens);
  if (tokens < 1_000_000)
    return `${(tokens / 1000).toFixed(tokens < 10_000 ? 1 : 0).replace(/\.0$/, "")}k`;
  return `${(tokens / 1_000_000).toFixed(1).replace(/\.0$/, "")}M`;
}

/** The line that ends a turn, and its longer hover text. */
export function turnSummary(turn: TurnStats, agent: string) {
  const parts = [`Worked ${duration(turn.ms)}`];
  if (turn.tokens) parts.push(`${tokenCount(turn.tokens)} tokens`);
  const detail = [
    `This turn ran for ${duration(turn.ms)}.`,
    turn.tokens
      ? `${agent} reported ${turn.tokens.toLocaleString("en-US")} tokens, cached input included.`
      : `${agent} reported no token use for it.`,
  ];
  if (turn.costUsd)
    detail.push(
      `${agent} estimates $${turn.costUsd.toFixed(2)} at API prices; on a subscription it is not charged.`,
    );
  return { text: parts.join(" · "), detail: detail.join(" ") };
}
