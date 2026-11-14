import Icon from "../Icon";
import type { Snapshot } from "../api";

export default function TopBar({
  snapshot,
  project,
  busy,
  onProject,
  onAddProject,
  onSearch,
}: {
  snapshot?: Snapshot;
  project: string;
  busy: boolean;
  onProject: (path: string) => void;
  onAddProject: () => void;
  onSearch: () => void;
}) {
  return (
    <header className="top-bar">
      <div className="top-brand">
        <span className="brand-symbol" aria-hidden="true">
          <Icon name="terminal" size={15} />
        </span>
        <strong>ais</strong>
      </div>
      <div className="project-switcher">
        <Icon name="folder" size={15} />
        <select
          aria-label="Project"
          value={project}
          onChange={(e) => onProject(e.target.value)}
        >
          <option value="">All projects</option>
          {snapshot?.projects.map((p) => (
            <option key={p.path} value={p.path} title={p.path}>
              {p.name || p.path}
              {p.available ? "" : " (folder unavailable)"}
            </option>
          ))}
        </select>
        <button
          className="icon-button"
          aria-label="Add project folder"
          title="Add project folder"
          onClick={onAddProject}
          disabled={busy}
        >
          <Icon name="plus" size={15} />
        </button>
      </div>
      <button className="search" aria-haspopup="dialog" onClick={onSearch}>
        <Icon name="search" size={15} />
        <span>Search conversations, projects and actions…</span>
        <kbd>⌘K</kbd>
      </button>
    </header>
  );
}
