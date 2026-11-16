import Icon from "../Icon";

/** A section heading that folds the section away. */
export default function SectionToggle({
  label,
  title,
  folded,
  count,
  needs = 0,
  onToggle,
}: {
  label: string;
  title?: string;
  folded: boolean;
  /** Shown while folded: how many items the section holds. */
  count: number;
  /** Shown while folded: how many of them need you. */
  needs?: number;
  onToggle: () => void;
}) {
  const hidden = folded
    ? `, folded, ${count}${needs ? `, ${needs} need you` : ""}`
    : "";
  return (
    <h2 className="section-label section-head">
      <button
        className="section-toggle"
        aria-expanded={!folded}
        aria-label={label + hidden}
        title={title ? `${title}\n${folded ? "Show" : "Fold"}` : undefined}
        onClick={onToggle}
      >
        <Icon name="chevron" size={11} />
        <span>{label}</span>
        {folded && <span className="section-count">{count}</span>}
        {folded && needs > 0 && (
          <span className="section-needs">
            <Icon name="alert" size={11} />
            {needs}
          </span>
        )}
      </button>
    </h2>
  );
}
