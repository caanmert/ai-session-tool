import type { KeyboardEvent, RefObject } from "react";
import Icon from "../Icon";
import type { Snapshot } from "../api";

export default function TopBar({
  snapshot,
  project,
  busy,
  query,
  searchInput,
  onProject,
  onAddProject,
  onQuery,
  onSearchKey,
}: {
  snapshot?: Snapshot;
  project: string;
  busy: boolean;
  query: string;
  searchInput: RefObject<HTMLInputElement | null>;
  onProject: (path: string) => void;
  onAddProject: () => void;
  onQuery: (query: string) => void;
  onSearchKey: (event: KeyboardEvent<HTMLInputElement>) => void;
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
      <label className="search">
        <Icon name="search" size={15} />
        <input
          ref={searchInput}
          aria-label="Search sessions"
          placeholder="Search conversations, projects, or tags…"
          value={query}
          onChange={(e) => onQuery(e.target.value)}
          onKeyDown={onSearchKey}
        />
        {query ? (
          <button
            className="icon-button"
            aria-label="Clear search"
            onClick={() => {
              onQuery("");
              searchInput.current?.focus();
            }}
          >
            <Icon name="close" size={14} />
          </button>
        ) : (
          <kbd>⌘K</kbd>
        )}
      </label>
    </header>
  );
}
