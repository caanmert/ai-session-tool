import { useEffect, useState } from "react";
import Icon from "../Icon";
import {
  api,
  errorText,
  type GitHubIssueDetail,
  type GitHubPullDetail,
} from "../api";
import { SafeMarkdown } from "../ui/SafeMarkdown";
import { age } from "../ui/format";

/** A GitHub label colour as a dot; anything unexpected shows no colour. */
export const labelColor = (color?: string) =>
  color && /^[0-9a-f]{6}$/i.test(color) ? `#${color}` : "transparent";

export function LabelChip({ name, color }: { name: string; color?: string }) {
  return (
    <span className="chip outline gh-label">
      <i aria-hidden="true" style={{ background: labelColor(color) }} />
      {name}
    </span>
  );
}

const reviewWords: Record<string, string> = {
  APPROVED: "Approved",
  CHANGES_REQUESTED: "Changes requested",
  REVIEW_REQUIRED: "Review required",
};
const checkIcon: Record<string, "check" | "close" | "refresh" | "arrow"> = {
  passing: "check",
  failing: "close",
  running: "refresh",
  skipped: "arrow",
};

/**
 * An issue or pull request read in full: its description, labels, latest
 * comments and, for a pull request, its branch, checks and size. It only
 * reads from GitHub.
 */
export default function GitHubDetail({
  project,
  kind,
  number,
  starting,
  onStartTask,
  onOpen,
}: {
  project: string;
  kind: "issue" | "pull";
  number: number;
  starting: boolean;
  onStartTask: (number: number) => void;
  onOpen: (url: string) => void;
}) {
  const [item, setItem] = useState<GitHubIssueDetail | GitHubPullDetail>();
  const [error, setError] = useState("");
  useEffect(() => {
    let active = true;
    setItem(undefined);
    setError("");
    (kind === "issue"
      ? api.GitHubIssue(project, number)
      : api.GitHubPull(project, number)
    )
      .then((found) => active && setItem(found))
      .catch((e) => active && setError(errorText(e)));
    return () => {
      active = false;
    };
  }, [project, kind, number]);
  const noun = kind === "issue" ? "issue" : "pull request";
  if (error)
    return (
      <section className="gh-detail" aria-label={`${noun} #${number}`}>
        <p className="notice error" role="alert">
          {error}
        </p>
      </section>
    );
  if (!item)
    return (
      <section className="gh-detail" aria-label={`${noun} #${number}`}>
        <p className="board-empty" role="status">
          Reading #{number}…
        </p>
      </section>
    );
  const pull = kind === "pull" ? (item as GitHubPullDetail) : undefined;
  const comments = item.comments || [];
  const total = item.commentCount ?? comments.length;
  return (
    <section
      className="gh-detail"
      aria-label={`${kind === "issue" ? "Issue" : "Pull request"} #${number}`}
    >
      <header>
        <p className="gh-detail-meta">
          <span
            className={`chip status ${item.state === "OPEN" ? "running" : ""}`}
          >
            {item.state
              ? item.state[0] + item.state.slice(1).toLowerCase()
              : "Open"}
          </span>
          #{item.number}
          {item.author && <> · {item.author} opened</>}
          {item.createdAt && <> {age(item.createdAt).toLowerCase()}</>}
          {item.updatedAt && (
            <> · updated {age(item.updatedAt).toLowerCase()}</>
          )}
        </p>
        <h2>{item.title}</h2>
        {!!item.labels?.length && (
          <p className="gh-meta">
            {item.labels.map((label) => (
              <LabelChip
                key={label.name}
                name={label.name}
                color={label.color}
              />
            ))}
          </p>
        )}
        {pull && (
          <div className="gh-pull-facts">
            <span>
              <code>{pull.branch}</code> into <code>{pull.base}</code>
            </span>
            {pull.isDraft && <span className="chip outline">Draft</span>}
            {reviewWords[pull.review] && (
              <span>{reviewWords[pull.review]}</span>
            )}
            <span>
              {pull.changedFiles} {pull.changedFiles === 1 ? "file" : "files"}{" "}
              <b className="added">+{pull.additions}</b>{" "}
              <b className="removed">−{pull.deletions}</b>
            </span>
          </div>
        )}
        <div className="gh-detail-actions">
          {kind === "issue" && (
            <button
              className="button primary"
              disabled={starting}
              onClick={() => onStartTask(item.number)}
            >
              {starting ? "Reading…" : "Start task"}
            </button>
          )}
          <button className="button secondary" onClick={() => onOpen(item.url)}>
            Open on GitHub <Icon name="external" size={13} />
          </button>
        </div>
      </header>
      {pull && pull.checks.length > 0 && (
        <ul className="gh-checks" aria-label="Checks">
          {pull.checks.map((check, i) => (
            <li key={`${check.name}-${i}`} className={check.state}>
              <Icon name={checkIcon[check.state] || "refresh"} size={12} />
              <span>{check.name}</span>
              <small>
                {check.state === "passing"
                  ? "Passed"
                  : check.state === "failing"
                    ? "Failed"
                    : check.state === "skipped"
                      ? "Skipped"
                      : "Running"}
              </small>
            </li>
          ))}
        </ul>
      )}
      <div className="gh-body markdown">
        {item.body.trim() ? (
          <SafeMarkdown text={item.body} imageNote="[Image not loaded]" />
        ) : (
          <p className="gh-empty-body">No description.</p>
        )}
      </div>
      {item.shortened && (
        <p className="git-note">
          The description is long; open it on GitHub to read it all.
        </p>
      )}
      <h3 className="gh-comments-title">
        {total ? `Comments · ${total}` : "No comments"}
      </h3>
      {total > comments.length && (
        <p className="git-note">Showing the latest {comments.length}.</p>
      )}
      <ol className="gh-comments">
        {comments.map((comment, i) => (
          <li key={i}>
            <p className="gh-comment-meta">
              <strong>{comment.author || "someone"}</strong>{" "}
              {comment.createdAt && age(comment.createdAt).toLowerCase()}
            </p>
            <div className="markdown">
              <SafeMarkdown
                text={comment.body}
                imageNote="[Image not loaded]"
              />
            </div>
          </li>
        ))}
      </ol>
    </section>
  );
}
