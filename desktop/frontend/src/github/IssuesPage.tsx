import { useEffect, useState } from "react";
import Icon from "../Icon";
import NewIssueDialog from "./NewIssueDialog";
import GitHubDetail, { LabelChip } from "./GitHubDetail";
import {
  api,
  errorText,
  type GitHubOverview,
  type GitHubPull,
  type Project,
} from "../api";
import { age } from "../ui/format";

const reviewWords: Record<string, { text: string; tone: string }> = {
  APPROVED: { text: "Approved", tone: "good" },
  CHANGES_REQUESTED: { text: "Changes requested", tone: "bad" },
  REVIEW_REQUIRED: { text: "Review required", tone: "" },
};

/** Check state in words with a matching icon, never color alone. */
function Checks({ pull }: { pull: GitHubPull }) {
  if (pull.checks === "passing")
    return (
      <span className="gh-state good">
        <Icon name="check" size={12} /> Checks pass
      </span>
    );
  if (pull.checks === "failing")
    return (
      <span className="gh-state bad">
        <Icon name="close" size={12} /> Checks failing
      </span>
    );
  if (pull.checks === "running")
    return <span className="gh-state running">Checks running</span>;
  return null;
}

/** The text a task started from an issue begins with. */
export function issueTask(issue: {
  number: number;
  title: string;
  url: string;
  body: string;
}) {
  return [
    `Work on issue #${issue.number}: ${issue.title}`,
    issue.url,
    issue.body.trim(),
  ]
    .filter(Boolean)
    .join("\n\n");
}

/**
 * Open issues and pull requests of a project, read through the GitHub CLI.
 * The page only reads; starting a task from an issue opens New task with the
 * issue's text, and nothing is sent until the user starts it.
 */
export default function IssuesPage({
  projects,
  project,
  onStartTask,
}: {
  projects: Project[];
  /** The project chosen in the top bar, if any. */
  project: string;
  onStartTask: (project: string, text: string) => void;
}) {
  const available = projects.filter((p) => p.available);
  const [chosen, setChosen] = useState(project || available[0]?.path || "");
  const [overview, setOverview] = useState<GitHubOverview>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [tab, setTab] = useState<"issues" | "pulls">("issues");
  const [revision, setRevision] = useState(0);
  const [updated, setUpdated] = useState("");
  const [starting, setStarting] = useState(0);
  // The New issue dialog is open.
  const [writing, setWriting] = useState(false);
  // The issue or pull request read in full beside the list.
  const [selected, setSelected] = useState(0);
  useEffect(() => setSelected(0), [chosen, tab]);
  // Follow the project chosen in the top bar.
  useEffect(() => {
    if (project) setChosen(project);
  }, [project]);
  useEffect(() => {
    if (!chosen) return;
    let active = true;
    setLoading(true);
    setError("");
    api
      .GitHubOverview(chosen)
      .then((next) => {
        if (!active) return;
        setOverview(next);
        setUpdated(
          new Date().toLocaleTimeString([], {
            hour: "2-digit",
            minute: "2-digit",
          }),
        );
      })
      .catch((e) => active && setError(errorText(e)))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, [chosen, revision]);

  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  const matches = (...fields: (string | number)[]) => {
    const text = fields.join(" ").toLowerCase();
    return words.every((word) => text.includes(word));
  };
  const issues = (overview?.issues || []).filter((issue) =>
    matches(issue.number, issue.title, issue.author, ...issue.labels),
  );
  const pulls = (overview?.pulls || []).filter((pull) =>
    matches(pull.number, pull.title, pull.author, pull.branch),
  );
  // Up and Down move through the titles in the list and show each one.
  const moveIn = (e: React.KeyboardEvent<HTMLUListElement>) => {
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
    const titles = [
      ...e.currentTarget.querySelectorAll<HTMLButtonElement>(".gh-title"),
    ];
    const at = titles.indexOf(document.activeElement as HTMLButtonElement);
    if (at < 0) return;
    e.preventDefault();
    const next =
      titles[
        (at + (e.key === "ArrowDown" ? 1 : -1) + titles.length) % titles.length
      ];
    next.focus();
    next.click();
  };
  const open = (link: string) =>
    api.OpenURL(link).catch((e) => setError(errorText(e)));
  async function startTask(number: number) {
    setStarting(number);
    setError("");
    try {
      onStartTask(chosen, issueTask(await api.GitHubIssue(chosen, number)));
    } catch (e) {
      setError(errorText(e));
    } finally {
      setStarting(0);
    }
  }

  if (!available.length)
    return (
      <div className="gh-page">
        <p className="board-empty">
          Add a project folder to see its issues and pull requests.
        </p>
      </div>
    );
  return (
    <div
      className="gh-page"
      role="region"
      aria-label="Issues and pull requests"
    >
      <div className="gh-toolbar">
        <select
          aria-label="Project for issues"
          value={chosen}
          onChange={(e) => setChosen(e.target.value)}
        >
          {available.map((p) => (
            <option key={p.path} value={p.path}>
              {p.name || p.path}
            </option>
          ))}
        </select>
        <label className="history-filter">
          <Icon name="filter" size={15} />
          <input
            aria-label="Filter issues and pull requests"
            placeholder="Filter by title, number, label or author…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
        <button
          className={`icon-button ${loading ? "spinning" : ""}`}
          aria-label="Refresh from GitHub"
          title={updated ? `Updated ${updated}` : "Refresh from GitHub"}
          disabled={loading}
          onClick={() => setRevision((n) => n + 1)}
        >
          <Icon name="refresh" size={16} />
        </button>
        {overview?.url && (
          <button
            className="button secondary"
            onClick={() => void open(overview.url!)}
          >
            {overview.repository} <Icon name="external" size={13} />
          </button>
        )}
        {overview?.repository && (
          <button
            className="button secondary"
            title={`Write an issue and create it in ${overview.repository}`}
            onClick={() => setWriting(true)}
          >
            <Icon name="plus" size={13} /> New issue…
          </button>
        )}
      </div>
      {error && (
        <p className="notice error" role="alert">
          {error}
        </p>
      )}
      {overview?.reason && (
        <p className="board-empty" role="status">
          {overview.reason}
        </p>
      )}
      {overview?.partial && (
        <p className="board-note" role="status">
          Part of this page could not be read. {overview.partial}
        </p>
      )}
      {!overview && loading && (
        <p className="board-empty" role="status">
          Reading from GitHub…
        </p>
      )}
      {overview && !overview.reason && (
        <>
          <div className="history-views" role="group" aria-label="Show">
            <button
              aria-pressed={tab === "issues"}
              onClick={() => setTab("issues")}
            >
              <Icon name="issue" size={14} /> Issues
              <span className="filter-count">{overview.issues.length}</span>
            </button>
            <button
              aria-pressed={tab === "pulls"}
              onClick={() => setTab("pulls")}
            >
              <Icon name="branch" size={14} /> Pull requests
              <span className="filter-count">{overview.pulls.length}</span>
            </button>
          </div>
          <div className={`gh-split ${selected ? "with-detail" : ""}`}>
            <div className="gh-lists">
              {tab === "issues" && (
                <ul
                  className="gh-list"
                  aria-label="Open issues"
                  onKeyDown={moveIn}
                >
                  {issues.map((issue) => (
                    <li
                      key={issue.number}
                      className={selected === issue.number ? "selected" : ""}
                    >
                      <span className="gh-number">#{issue.number}</span>
                      <div className="gh-main">
                        <button
                          className="gh-title"
                          aria-pressed={selected === issue.number}
                          onClick={() => setSelected(issue.number)}
                        >
                          {issue.title}
                        </button>
                        <span className="gh-meta">
                          {issue.labels.map((label) => (
                            <LabelChip
                              key={label}
                              name={label}
                              color={issue.labelColors?.[label]}
                            />
                          ))}
                          <span>
                            {issue.author} · updated {age(issue.updatedAt)}
                          </span>
                        </span>
                      </div>
                      <div className="gh-actions">
                        <button
                          className="button secondary"
                          disabled={!!starting}
                          onClick={() => void startTask(issue.number)}
                        >
                          {starting === issue.number
                            ? "Reading…"
                            : "Start task"}
                        </button>
                        <button
                          className="icon-button"
                          aria-label={`Open issue #${issue.number} on GitHub`}
                          title="Open on GitHub"
                          onClick={() => void open(issue.url)}
                        >
                          <Icon name="external" size={15} />
                        </button>
                      </div>
                    </li>
                  ))}
                  {!issues.length && (
                    <li className="gh-empty">
                      {overview.issues.length
                        ? "No issue matches the filter."
                        : "No open issues."}
                    </li>
                  )}
                </ul>
              )}
              {tab === "pulls" && (
                <ul
                  className="gh-list"
                  aria-label="Open pull requests"
                  onKeyDown={moveIn}
                >
                  {pulls.map((pull) => {
                    const review = reviewWords[pull.review];
                    return (
                      <li
                        key={pull.number}
                        className={selected === pull.number ? "selected" : ""}
                      >
                        <span className="gh-number">#{pull.number}</span>
                        <div className="gh-main">
                          <button
                            className="gh-title"
                            aria-pressed={selected === pull.number}
                            onClick={() => setSelected(pull.number)}
                          >
                            {pull.title}
                          </button>
                          <span className="gh-meta">
                            {pull.isDraft && (
                              <span className="chip outline">Draft</span>
                            )}
                            <Checks pull={pull} />
                            {review && (
                              <span className={`gh-state ${review.tone}`}>
                                {review.text}
                              </span>
                            )}
                            <code title={pull.branch}>{pull.branch}</code>
                            <span>
                              {pull.author} · updated {age(pull.updatedAt)}
                            </span>
                          </span>
                        </div>
                        <div className="gh-actions">
                          <button
                            className="icon-button"
                            aria-label={`Open pull request #${pull.number} on GitHub`}
                            title="Open on GitHub"
                            onClick={() => void open(pull.url)}
                          >
                            <Icon name="external" size={15} />
                          </button>
                        </div>
                      </li>
                    );
                  })}
                  {!pulls.length && (
                    <li className="gh-empty">
                      {overview.pulls.length
                        ? "No pull request matches the filter."
                        : "No open pull requests."}
                    </li>
                  )}
                </ul>
              )}
            </div>
            {selected ? (
              <GitHubDetail
                key={`${tab}-${selected}`}
                project={chosen}
                kind={tab === "issues" ? "issue" : "pull"}
                number={selected}
                starting={starting === selected}
                onStartTask={(number) => void startTask(number)}
                onOpen={(url) => void open(url)}
              />
            ) : (
              <p className="gh-detail-hint">
                Choose a title to read the{" "}
                {tab === "issues" ? "issue" : "pull request"} here.
              </p>
            )}
          </div>
          <p className="git-note">
            Read from GitHub with the GitHub CLI you are signed in to. The only
            change this page makes is an issue you create with New issue.
            {overview.issues.length >= 50 || overview.pulls.length >= 50
              ? " Only the 50 most recent of each are listed."
              : ""}
          </p>
        </>
      )}
      {writing && (
        <NewIssueDialog
          project={chosen}
          onClose={() => setWriting(false)}
          onCreated={() => setRevision((n) => n + 1)}
        />
      )}
    </div>
  );
}
