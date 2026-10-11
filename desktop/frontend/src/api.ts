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
export type ChatUpdate = { version: number; state: ChatState | null };
export interface Backend {
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
  PrepareReview(id: string): Promise<ReviewDraft>;
  StartReview(id: string): Promise<ChatState>;
  SendReviewFindings(id: string, findings: string[]): Promise<string>;
  Refresh(): Promise<Snapshot>;
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
  ChatModels: async (id, reviewer, refresh) =>
    backend().ChatModels(id, reviewer, refresh),
  SetChatSettings: async (id, settings, reviewer) =>
    backend().SetChatSettings(id, settings, reviewer),
  NewChat: async (project, tool) => backend().NewChat(project, tool),
  ResumeChat: async (key) => backend().ResumeChat(key),
  ListChats: async () => backend().ListChats(),
  ReadChat: async (id, version) => backend().ReadChat(id, version),
  SendChat: async (id, text) => backend().SendChat(id, text),
  StopChat: async (id) => backend().StopChat(id),
  CloseChat: async (id) => backend().CloseChat(id),
  AnswerChat: async (id, request, allow, answers) =>
    backend().AnswerChat(id, request, allow, answers),
  PrepareReview: async (id) => backend().PrepareReview(id),
  StartReview: async (id) => backend().StartReview(id),
  SendReviewFindings: async (id, findings) =>
    backend().SendReviewFindings(id, findings),
  Refresh: async () => backend().Refresh(),
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
};
export const toolLabel = (tool: string) =>
  tool === "claude" ? "Claude Code" : "Codex";
export function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
