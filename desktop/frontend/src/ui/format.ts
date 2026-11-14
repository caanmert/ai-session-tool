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
