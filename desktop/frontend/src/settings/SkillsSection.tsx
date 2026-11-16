import { useEffect, useState } from "react";
import AgentMark from "../AgentMark";
import Icon from "../Icon";
import {
  api,
  errorText,
  toolLabel,
  type Project,
  type SkillList,
  type Tool,
} from "../api";

/**
 * The skills each installed agent offers in a project, as the agents report
 * them. Reading them starts no model turn. Type "/" in a conversation to use
 * one.
 */
export default function SkillsSection({
  tools,
  projects,
  project,
}: {
  tools: Tool[];
  projects: Project[];
  project: string;
}) {
  const available = projects.filter((p) => p.available);
  const [chosen, setChosen] = useState(project || available[0]?.path || "");
  const [lists, setLists] = useState<Record<string, SkillList>>({});
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const agents = tools.filter((tool) => tool.available);
  const names = agents.map((tool) => tool.name).join();
  useEffect(() => {
    if (!chosen) return;
    let active = true;
    setLists({});
    setError("");
    for (const name of names.split(",").filter(Boolean))
      api
        .AgentSkills(name, chosen, revision > 0)
        .then((list) => active && setLists((all) => ({ ...all, [name]: list })))
        .catch((e) => active && setError(errorText(e)));
    return () => {
      active = false;
    };
  }, [chosen, names, revision]);
  return (
    <section aria-labelledby="settings-skills">
      <div className="settings-heading">
        <h2 id="settings-skills">Skills</h2>
        {available.length > 0 && (
          <div className="skills-tools">
            <select
              aria-label="Project for skills"
              value={chosen}
              onChange={(e) => setChosen(e.target.value)}
            >
              {available.map((p) => (
                <option key={p.path} value={p.path}>
                  {p.name || p.path}
                </option>
              ))}
            </select>
            <button
              className="button secondary"
              onClick={() => setRevision((n) => n + 1)}
            >
              <Icon name="refresh" size={14} /> Read again
            </button>
          </div>
        )}
      </div>
      <p className="setting-note">
        Skills come from each agent&apos;s own settings, plugins and the
        project. Type <kbd>/</kbd> in a conversation to add one to your message;
        nothing runs until you send it.
      </p>
      {!available.length && (
        <p className="setting-note">Add a project folder to see its skills.</p>
      )}
      {error && (
        <p className="notice error" role="alert">
          {error}
        </p>
      )}
      {!!chosen &&
        agents.map((tool) => {
          const list = lists[tool.name];
          return (
            <div
              className="setting-group skills-group"
              key={tool.name}
              role="group"
              aria-label={`${toolLabel(tool.name)} skills`}
            >
              <header>
                <AgentMark tool={tool.name} small />
                <strong>{toolLabel(tool.name)}</strong>
                <small>
                  {!list
                    ? "Reading…"
                    : list.reason
                      ? ""
                      : `${list.skills.length} ${list.skills.length === 1 ? "skill" : "skills"} · use with ${tool.name === "codex" ? "$name" : "/name"}`}
                </small>
              </header>
              {list?.reason && <p className="setting-note">{list.reason}</p>}
              {!!list?.skills.length && (
                <ul className="skills-list">
                  {list.skills.map((skill) => (
                    <li key={skill.name}>
                      <code>{skill.name}</code>
                      <span>{skill.description}</span>
                      {skill.scope && <small>{skill.scope}</small>}
                    </li>
                  ))}
                </ul>
              )}
            </div>
          );
        })}
    </section>
  );
}
