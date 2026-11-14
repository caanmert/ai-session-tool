export type ToolName = "codex" | "claude";
export type Session = {
  key: string;
  id: string;
  tool: ToolName;
  title: string;
  cwd: string;
  project: string;
  gitBranch?: string;
  updatedAt: string;
  model?: string;
  userTurns: number;
  assistantTurns: number;
  pinned?: boolean;
  tags?: string[];
  live?: { pid: number };
  usage: {
    input: number;
    output: number;
    cacheRead: number;
    cacheWrite: number;
  };
};
export type Project = {
  path: string;
  name: string;
  sessions: number;
  available: boolean;
  recent: boolean;
};
/** Result of asking an installed agent for its version and whether it still
 * accepts the options ais uses. No model is contacted. */
export type AgentHealth = {
  tool: ToolName;
  version: string;
  /** A version known to work, offered when the installed one is not compatible. */
  tested: string;
  problem?: string;
};
export type AnnotationChange = {
  /** An empty title restores the agent's own title. */
  title?: string;
  pinned?: boolean;
  addTags?: string[];
  removeTags?: string[];
};
export type Tool = { name: ToolName; available: boolean; path: string };
export type Snapshot = {
  sessions: Session[];
  projects: Project[];
  tools: Tool[];
  warnings: string[];
};
export type Message = {
  role: "user" | "assistant";
  kind: "text" | "tool_use" | "tool_result" | "thinking";
  text?: string;
  toolName?: string;
  time?: string;
};
export type Preview = { messages: Message[]; truncated: boolean };
export type LiveSession = {
  id: string;
  tool: ToolName;
  project: string;
  key: string;
  exited: boolean;
  exitCode: number;
};
export type TerminalChunk = {
  data: string;
  cursor: number;
  lost: boolean;
  exited: boolean;
  exitCode: number;
};
export type ModelSettings = {
  model: string;
  effort: string;
  configured?: boolean;
};
export type ModelOption = {
  id: string;
  name: string;
  description: string;
  efforts: string[];
  defaultEffort: string;
};
export type ModelCatalog = {
  models: ModelOption[];
  defaultModel: string;
  defaultEffort: string;
};
export type ChatMessage = {
  model?: string;
  id: string;
  role: string;
  kind: string;
  text: string;
  name?: string;
  status?: string;
};
export type ChatQuestion = {
  id: string;
  text: string;
  options: string[];
  secret?: boolean;
};
export type ChatApproval = {
  id: string;
  title: string;
  detail: string;
  questions: ChatQuestion[];
};
export type ReviewFile = { path: string; patch: string };
export type ReviewSnapshot = {
  root: string;
  base: string;
  hash: string;
  diff: string;
  files: ReviewFile[];
  warnings: string[];
};
export type ChangedFile = {
  path: string;
  /** M modified, A added, D deleted, ? untracked */
  status: string;
  added: number;
  removed: number;
  binary?: boolean;
};
export type ChangeSummary = {
  available: boolean;
  reason?: string;
  files: ChangedFile[];
  added: number;
  removed: number;
  truncated?: boolean;
};
export type ReviewFinding = {
  id: string;
  priority: string;
  file: string;
  line: number;
  title: string;
  body: string;
  sent: boolean;
};
export type ReviewState = {
  sourceId: string;
  snapshot: ReviewSnapshot;
  summary: string;
  findings: ReviewFinding[];
  complete: boolean;
};
export type ReviewDraft = {
  settings?: ModelSettings;
  id: string;
  sourceId: string;
  tool: ToolName;
  snapshot: ReviewSnapshot;
  created: string;
};
export type ChatState = {
  settings?: ModelSettings;
  reviewerSettings?: ModelSettings;
  activeModel?: string;
  id: string;
  tool: ToolName;
  project: string;
  sessionId: string;
  sourceKey: string;
  title: string;
  status: string;
  error: string;
  messages: ChatMessage[];
  approvals: ChatApproval[];
  review?: ReviewState;
  updatedAt: string;
  shortened: boolean;
  closed?: boolean;
  version: number;
};
/**
 * `state` is null when nothing changed. When `partial` is set,
 * `state.messages` holds only the messages changed since the caller's version;
 * `first` and `count` describe the complete list. See chatUpdates.ts.
 */
export type ChatUpdate = {
  version: number;
  state: ChatState | null;
  partial?: boolean;
  first?: string;
  count?: number;
};
export type RunTask = {
  prompt: string;
  branch: string;
  path: string;
  window: string;
};
export type Run = {
  id: string;
  tool: ToolName;
  repository: string;
  base: string;
  createdAt: string;
  notifications: boolean;
  tasks: RunTask[];
};
export type RunTaskStatus = {
  task: RunTask;
  state: string;
  pane?: string;
  exitCode?: number;
  signal?: string;
  attention: boolean;
  quiet: boolean;
  currentBranch?: string;
  files: { status: string; path: string }[];
  error?: string;
};
export type RunStatus = {
  id: string;
  base: string;
  tasks: RunTaskStatus[];
  warning?: string;
};
export type RunPatch = { patch: string; truncated: boolean };
export type ChatSummary = {
  id: string;
  tool: ToolName;
  project: string;
  title: string;
  status: string;
  updatedAt: string;
  closed: boolean;
  isReview: boolean;
  complete: boolean;
  findingCount: number;
  sentCount: number;
  fileCount: number;
};
export interface Backend {
  SetAppearance(theme: "light" | "dark"): Promise<void>;
  /** Dock badge with the number of conversations waiting; `alert` bounces the icon. */
  SetAttention(count: number, alert: boolean): Promise<void>;
  AgentHealth(): Promise<AgentHealth[]>;
  /** Opens the conversation's project folder in Finder. */
  RevealProject(id: string): Promise<void>;
  ChatModels(
    id: string,
    reviewer: boolean,
    refresh: boolean,
  ): Promise<ModelCatalog>;
  SetChatSettings(
    id: string,
    settings: ModelSettings,
    reviewer: boolean,
  ): Promise<void>;
  NewChat(project: string, tool: ToolName): Promise<ChatState>;
  ResumeChat(key: string): Promise<ChatState>;
  ListChats(): Promise<ChatState[]>;
  ListSavedChats(): Promise<ChatSummary[]>;
  ReopenChat(id: string): Promise<ChatState>;
  ReadChat(id: string, version: number): Promise<ChatUpdate>;
  SendChat(id: string, text: string): Promise<void>;
  StopChat(id: string): Promise<void>;
  CloseChat(id: string): Promise<void>;
  AnswerChat(
    id: string,
    request: string,
    allow: boolean,
    answers: Record<string, string>,
  ): Promise<void>;
  /** Paths of files the user chose to mention, relative to the project when inside it. */
  PickFiles(id: string): Promise<string[]>;
  PrepareReview(id: string): Promise<ReviewDraft>;
  ChatChanges(id: string): Promise<ChangeSummary>;
  ChatDiff(id: string): Promise<ReviewSnapshot>;
  StartReview(id: string): Promise<ChatState>;
  SendReviewFindings(id: string, findings: string[]): Promise<string>;
  Refresh(): Promise<Snapshot>;
  AnnotateSession(key: string, change: AnnotationChange): Promise<Snapshot>;
  Search(query: string): Promise<string[]>;
  Transcript(key: string): Promise<Preview>;
  PickProject(): Promise<string>;
  NewSession(project: string, tool: ToolName): Promise<LiveSession>;
  ResumeSession(key: string): Promise<LiveSession>;
  LiveSessions(): Promise<LiveSession[]>;
  ReadTerminal(id: string, cursor: number): Promise<TerminalChunk>;
  WriteTerminal(id: string, data: string): Promise<void>;
  ResizeTerminal(id: string, cols: number, rows: number): Promise<void>;
  StopTerminal(id: string): Promise<void>;
  CloseTerminal(id: string): Promise<void>;
  ListRuns(): Promise<Run[]>;
  RunStatus(id: string): Promise<RunStatus>;
  RunDiff(id: string, task: string, file: string): Promise<RunPatch>;
  StartRun(project: string, tool: ToolName, prompts: string[]): Promise<Run>;
  StopRun(id: string): Promise<void>;
  RunAttachCommand(id: string, task: string): Promise<string>;
}
declare global {
  interface Window {
    go?: { main: { App: Backend } };
  }
}
function backend(): Backend {
  const app = window.go?.main.App;
  if (!app)
    throw new Error(
      "The desktop connection is unavailable. Reopen ais to reconnect.",
    );
  return app;
}
export const api: Backend = {
  SetAppearance: async (theme) => backend().SetAppearance(theme),
  SetAttention: async (count, alert) => backend().SetAttention(count, alert),
  AgentHealth: async () => backend().AgentHealth(),
  RevealProject: async (id) => backend().RevealProject(id),
  ChatModels: async (id, reviewer, refresh) =>
    backend().ChatModels(id, reviewer, refresh),
  SetChatSettings: async (id, settings, reviewer) =>
    backend().SetChatSettings(id, settings, reviewer),
  NewChat: async (project, tool) => backend().NewChat(project, tool),
  ResumeChat: async (key) => backend().ResumeChat(key),
  ListChats: async () => backend().ListChats(),
  ListSavedChats: async () => backend().ListSavedChats(),
  ReopenChat: async (id) => backend().ReopenChat(id),
  ReadChat: async (id, version) => backend().ReadChat(id, version),
  SendChat: async (id, text) => backend().SendChat(id, text),
  StopChat: async (id) => backend().StopChat(id),
  CloseChat: async (id) => backend().CloseChat(id),
  AnswerChat: async (id, request, allow, answers) =>
    backend().AnswerChat(id, request, allow, answers),
  PickFiles: async (id) => backend().PickFiles(id),
  PrepareReview: async (id) => backend().PrepareReview(id),
  ChatChanges: async (id) => backend().ChatChanges(id),
  ChatDiff: async (id) => backend().ChatDiff(id),
  StartReview: async (id) => backend().StartReview(id),
  SendReviewFindings: async (id, findings) =>
    backend().SendReviewFindings(id, findings),
  Refresh: async () => backend().Refresh(),
  AnnotateSession: async (key, change) =>
    backend().AnnotateSession(key, change),
  Search: async (q) => backend().Search(q),
  Transcript: async (key) => backend().Transcript(key),
  PickProject: async () => backend().PickProject(),
  NewSession: async (project, tool) => backend().NewSession(project, tool),
  ResumeSession: async (key) => backend().ResumeSession(key),
  LiveSessions: async () => backend().LiveSessions(),
  ReadTerminal: async (id, cursor) => backend().ReadTerminal(id, cursor),
  WriteTerminal: async (id, data) => backend().WriteTerminal(id, data),
  ResizeTerminal: async (id, cols, rows) =>
    backend().ResizeTerminal(id, cols, rows),
  StopTerminal: async (id) => backend().StopTerminal(id),
  CloseTerminal: async (id) => backend().CloseTerminal(id),
  ListRuns: async () => backend().ListRuns(),
  RunStatus: async (id) => backend().RunStatus(id),
  RunDiff: async (id, task, file) => backend().RunDiff(id, task, file),
  StartRun: async (project, tool, prompts) =>
    backend().StartRun(project, tool, prompts),
  StopRun: async (id) => backend().StopRun(id),
  RunAttachCommand: async (id, task) => backend().RunAttachCommand(id, task),
};
export const toolLabel = (tool: string) =>
  tool === "claude" ? "Claude Code" : "Codex";
export function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
