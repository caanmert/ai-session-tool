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
  /** USD at API list prices; absent when the usage cannot be priced. A
   * subscription is not billed this way. */
  cost?: number;
  /** Some usage has no known price and is left out of `cost`. */
  costPartial?: boolean;
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
export type AgentSkill = {
  name: string;
  description: string;
  /** The arguments it takes, when the agent says. */
  hint?: string;
  /** Where it comes from: repo, user, system (Codex). */
  scope?: string;
  path?: string;
};
export type SkillList = {
  tool: ToolName;
  skills: AgentSkill[];
  reason?: string;
};
export type GitHubLabel = { name: string; color: string; description: string };
/** Where a new issue would go. When reason is set, none can be created. */
export type GitHubIssueForm = {
  reason?: string;
  repository?: string;
  url?: string;
  labels: GitHubLabel[];
  /** Labels could not be read; an issue can still be created without them. */
  partial?: string;
};
export type GitHubIssueCreated = { number: number; url: string };
export type UsageLimit = {
  label: string;
  /** "session" or "weekly", when the agent says which. */
  window?: string;
  percent: number;
  resetsAt?: string;
  level: "normal" | "warning" | "reached";
};
export type AgentUsage = {
  tool: ToolName;
  plan?: string;
  limits: UsageLimit[];
  /** The agent says ordinary usage is not allowed right now. */
  blocked?: boolean;
  extraUsage?: "on" | "off";
  /** Where this week's usage went, when the agent reports it. */
  breakdown?: { label: string; percent: number }[];
  /** Free full resets Codex offers. Shown only; never used by ais. */
  resetCredits?: { available: number; nextExpiry?: string };
  /** Why no limits could be read. */
  reason?: string;
  readAt: string;
};
export type UsageOverview = { agents: AgentUsage[] };
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
/** An image attached to a message: its name and bytes in base64. */
export type ChatImage = { name: string; data: string };
export type ChatMessage = {
  /** Names of images attached to a user message. */
  images?: string[];
  /** When the message first arrived (RFC 3339); absent on older saved messages. */
  at?: string;
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
/** A wider grant the agent itself offered for a request. */
export type ApprovalOption = { id: string; label: string; detail?: string };
export type ChatApproval = {
  id: string;
  title: string;
  detail: string;
  questions: ChatQuestion[];
  /** Offered next to "Allow once"; absent when the agent offers nothing wider. */
  options?: ApprovalOption[];
  /** The proposed plan, when the agent asks to leave plan mode. */
  plan?: string;
};
/** A permission mode the conversation's agent supports. */
export type ModeOption = { id: string; label: string; detail: string };
export type ReviewFile = { path: string; patch: string };
export type ReviewSnapshot = {
  root: string;
  base: string;
  hash: string;
  diff: string;
  files: ReviewFile[];
  warnings: string[];
  /** Every changed file, shown or not. */
  total?: number;
  /** A view that leaves changed files out; the warnings say which. */
  partial?: boolean;
};
export type ChangedFile = {
  path: string;
  /** M modified, A added, D deleted, ? untracked */
  status: string;
  added: number;
  removed: number;
  binary?: boolean;
};
export type CommitPlan = {
  branch: string;
  defaultBranch?: string;
  /** On the branch pull requests go into; a new branch is suggested. */
  onDefault: boolean;
  files: ChangedFile[];
  added: number;
  removed: number;
  truncated?: boolean;
  message: string;
  newBranch: string;
};
export type CommitRequest = {
  message: string;
  files: string[];
  newBranch?: string;
};
export type CommitResult = {
  commit: string;
  branch: string;
  subject: string;
  files: number;
};
export type PullRequestInfo = {
  number: number;
  url: string;
  title?: string;
  isDraft?: boolean;
};
/** When `reason` is set nothing can be done, and it says why. */
export type PullRequestPlan = {
  reason?: string;
  repository?: string;
  remote?: string;
  branch?: string;
  base?: string;
  commits: string[];
  unpushed: number;
  uncommitted: number;
  existing?: PullRequestInfo;
  title?: string;
  body?: string;
};
export type PullRequestRequest = {
  title: string;
  body: string;
  draft: boolean;
};
export type PullRequestResult = {
  number: number;
  url: string;
  created: boolean;
  pushed: number;
};
export type GitHubIssue = {
  number: number;
  title: string;
  url: string;
  author: string;
  labels: string[];
  /** A label's GitHub colour, by name. */
  labelColors?: Record<string, string>;
  updatedAt: string;
};
export type GitHubPull = {
  number: number;
  title: string;
  url: string;
  author: string;
  branch: string;
  isDraft: boolean;
  updatedAt: string;
  /** APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED or "". */
  review: string;
  /** passing, failing, running or "". */
  checks: string;
};
/** When `reason` is set nothing could be read, and it says why. */
export type GitHubOverview = {
  reason?: string;
  repository?: string;
  url?: string;
  issues: GitHubIssue[];
  pulls: GitHubPull[];
  /** One of the two lists could not be read. */
  partial?: string;
};
/** The copy of a project a conversation works in: a worktree on its own branch. */
export type WorkCopy = {
  project: string;
  base: string;
  branch: string;
  worktree: string;
  brought?: string;
};
/** A reusable message; {project}, {branch} and {changed files} are filled in. */
export type SavedPrompt = { id: string; name: string; text: string };
/** A conversation continued with the other agent, and its drafted first message. */
export type HandOff = { chat: ChatState; prompt: string };
/** A command the project offers, such as a package script. */
export type ProjectCommand = {
  label: string;
  command: string;
  /** Where it was found, such as package.json or Makefile. */
  source: string;
};
/** The latest command run beside a conversation. */
export type ProjectRunState = {
  command: string;
  running: boolean;
  exitCode: number;
  output: string;
  shortened?: boolean;
  startedAt: string;
  endedAt?: string;
  stopped?: boolean;
};
/** An instructions file an agent reads for a project. */
export type InstructionFile = {
  agent: string;
  /** "project" (editable here) or "user" (shown, edited outside ais). */
  scope: string;
  name: string;
  path: string;
  exists: boolean;
  size: number;
  editable: boolean;
};
export type InstructionText = {
  name: string;
  text: string;
  exists: boolean;
  /** Changes with the content, so a save can tell the file changed meanwhile. */
  version: string;
};
/** A conversation whose text matches a search. */
export type MessageHit = {
  /** "chat" for a conversation in the app, "session" for a saved agent session. */
  kind: "chat" | "session";
  chatId?: string;
  messageId?: string;
  key?: string;
  title?: string;
  tool?: string;
  /** The matching passage, with matches between "\u0002" and "\u0003". */
  snippet: string;
};
export type Checkpoint = {
  id: string;
  /** The message it belongs to: the one that started a turn, or a restore's note. */
  messageId: string;
  commit: string;
  at: string;
  /** "turn" for before a turn, "restore" for before a restore or discard. */
  kind: string;
};
/** One file a restore changes back: "restore" earlier content or "remove" a new file. */
export type RestoreChange = { path: string; change: "restore" | "remove" };
export type GitHubComment = { author: string; body: string; createdAt: string };
export type GitHubIssueDetail = {
  number: number;
  title: string;
  body: string;
  url: string;
  state?: string;
  author?: string;
  labels?: { name: string; color: string }[];
  createdAt?: string;
  updatedAt?: string;
  /** The latest comments; commentCount says how many there are. */
  comments?: GitHubComment[];
  commentCount?: number;
  /** The description was cut to fit. */
  shortened?: boolean;
};
export type GitHubPullDetail = GitHubIssueDetail & {
  branch: string;
  base: string;
  isDraft: boolean;
  review: string;
  /** Each check, with state passing, failing, running or skipped. */
  checks: { name: string; state: string }[];
  additions: number;
  deletions: number;
  changedFiles: number;
};
export type ComparisonSide = {
  tool: ToolName;
  chatId: string;
  worktree: string;
  branch: string;
};
export type Comparison = {
  id: string;
  project: string;
  base: string;
  prompt: string;
  createdAt: string;
  sides: ComparisonSide[];
};
export type ComparisonStart = {
  comparison: Comparison;
  chats: ChatState[];
  /** A side could not start; its conversation is open to try again. */
  problem?: string;
};
export type ComparisonSideView = ComparisonSide & {
  status: string;
  changes: ChangeSummary;
};
export type ComparisonView = {
  id: string;
  project: string;
  base: string;
  prompt: string;
  sides: ComparisonSideView[];
};
export type ChangeSummary = {
  available: boolean;
  reason?: string;
  files: ChangedFile[];
  added: number;
  removed: number;
  truncated?: boolean;
  /** The checked-out branch; absent when HEAD is detached. */
  branch?: string;
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
  /** Codex's web search here: "" for Codex's own setting, or disabled, cached or live. */
  webSearch?: string;
  /** The files before each turn and each restore, so a turn can be undone. */
  checkpoints?: Checkpoint[];
  /** How full the context is after the latest response, as the agent reported it. */
  context?: { used: number; window?: number };
  /** Set on both conversations of a comparison. */
  comparison?: string;
  /** Permission mode for the next message; "" is the agent's normal asking. */
  mode?: string;
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
  /** The conversation this one continues, with the other agent. */
  handedFrom?: string;
  /** Conversations that continue this one with the other agent. */
  handedTo?: string[];
  /** Set when the conversation works in its own copy of the project. */
  copy?: WorkCopy;
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
  /** Posts a notification banner. macOS asks for permission the first time. */
  /** Posts a banner; clicking it opens the conversation chatId names. */
  Notify(title: string, body: string, chatId: string): Promise<void>;
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
  SendChatWithImages(
    id: string,
    text: string,
    images: ChatImage[],
  ): Promise<void>;
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
  /** Files in the conversation's project matching a query, best first. */
  ProjectFiles(id: string, query: string): Promise<string[]>;
  ChatModes(id: string): Promise<ModeOption[]>;
  SetChatMode(id: string, mode: string): Promise<void>;
  /** Allow a request using a wider grant the agent offered for it. */
  AnswerChatOption(id: string, request: string, option: string): Promise<void>;
  PrepareReview(id: string): Promise<ReviewDraft>;
  ChatChanges(id: string): Promise<ChangeSummary>;
  CommitPlan(id: string): Promise<CommitPlan>;
  /** A local commit of the chosen files. Nothing is pushed. */
  Commit(id: string, request: CommitRequest): Promise<CommitResult>;
  PullRequestPlan(id: string): Promise<PullRequestPlan>;
  CreatePullRequest(
    id: string,
    request: PullRequestRequest,
  ): Promise<PullRequestResult>;
  /** Opens an https link in the default browser. */
  OpenURL(link: string): Promise<void>;
  /** The same task for both agents, each in its own worktree and branch. */
  StartComparison(project: string, text: string): Promise<ComparisonStart>;
  ComparisonOverview(id: string): Promise<ComparisonView>;
  ComparisonDiff(id: string, tool: string): Promise<ReviewSnapshot>;
  /** Copies one side's changes into the project. Nothing is committed. */
  UseComparisonSide(id: string, tool: string): Promise<string[]>;
  /** Deletes both worktrees and branches, and closes both conversations. */
  RemoveComparison(id: string): Promise<void>;
  /** Each agent's plan limits. A read from the last minute is reused unless refresh is set. */
  UsageLimits(refresh: boolean): Promise<UsageOverview>;
  /** An agent's skills in a project. Starts no model turn. */
  AgentSkills(
    tool: string,
    project: string,
    refresh: boolean,
  ): Promise<SkillList>;
  /** Codex's web search for a conversation, from its next message. */
  SetChatWebSearch(id: string, mode: string): Promise<void>;
  /** Reads where a new issue would go and its labels. Only reads. */
  GitHubIssueForm(project: string): Promise<GitHubIssueForm>;
  /** Creates an issue through the GitHub CLI, after the user confirms. */
  CreateGitHubIssue(
    project: string,
    title: string,
    body: string,
    labels: string[],
  ): Promise<GitHubIssueCreated>;
  /** A project's open issues and pull requests, read with the GitHub CLI. */
  GitHubOverview(project: string): Promise<GitHubOverview>;
  GitHubIssue(project: string, number: number): Promise<GitHubIssueDetail>;
  /** Starts a conversation in a new copy (Git worktree) of the project. */
  NewChatInCopy(project: string, tool: ToolName): Promise<ChatState>;
  /** What bringing a copy back would change. Only reads. */
  CopyChanges(chatId: string): Promise<ReviewSnapshot>;
  /** Applies a copy's changes to the project, uncommitted; all or none. */
  BringBackCopy(chatId: string): Promise<string[]>;
  /** Deletes a copy and closes its conversation; refused with changes not
   * brought back unless discard is set. */
  RemoveCopy(chatId: string, discard: boolean): Promise<void>;
  /** The saved prompts, or a few starters until the list is first saved. */
  SavedPrompts(): Promise<SavedPrompt[]>;
  SetSavedPrompts(prompts: SavedPrompt[]): Promise<void>;
  /** What changed since a checkpoint, such as a message's turn. Only reads. */
  TurnDiff(chatId: string, checkpointId: string): Promise<ReviewSnapshot>;
  /** Opens a conversation with the other agent in the same project and
   * drafts its first message from this one. Sends nothing. */
  HandOffChat(chatId: string): Promise<HandOff>;
  /** The commands the conversation's project offers. Only reads. */
  ProjectCommands(chatId: string): Promise<ProjectCommand[]>;
  /** Runs a command the user chose in the project folder. */
  StartProjectCommand(
    chatId: string,
    command: string,
  ): Promise<ProjectRunState>;
  ProjectRun(chatId: string): Promise<ProjectRunState>;
  StopProjectCommand(chatId: string): Promise<void>;
  /** The instructions files each agent reads for a project. */
  ProjectInstructions(project: string): Promise<InstructionFile[]>;
  ReadInstruction(project: string, name: string): Promise<InstructionText>;
  /** Saves a project's instructions file; refused if it changed on disk. */
  WriteInstruction(
    project: string,
    name: string,
    text: string,
    version: string,
  ): Promise<InstructionText>;
  RevealInstruction(project: string, name: string): Promise<void>;
  /** Conversations whose text matches, with the passage. Only reads. */
  SearchMessages(query: string): Promise<MessageHit[]>;
  /** The files restoring a checkpoint would change. Changes nothing. */
  RestorePlan(chatId: string, checkpointId: string): Promise<RestoreChange[]>;
  /** Puts files back as they were at a checkpoint, after the user confirmed. */
  RestoreCheckpoint(
    chatId: string,
    checkpointId: string,
  ): Promise<RestoreChange[]>;
  /** Undoes the uncommitted changes to one file, after the user confirmed. */
  DiscardFile(chatId: string, path: string): Promise<void>;
  /** One pull request in full. Only reads. */
  GitHubPull(project: string, number: number): Promise<GitHubPullDetail>;
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
    /** The desktop shell's event channel; absent outside the app. */
    runtime?: {
      EventsOn?: (
        name: string,
        callback: (...data: unknown[]) => void,
      ) => () => void;
    };
  }
}

/** Calls back with a conversation's ID when its notification is clicked. */
export function onNotificationOpen(callback: (chatId: string) => void) {
  const off = window.runtime?.EventsOn?.("notification:open", (chatId) => {
    if (typeof chatId === "string" && chatId) callback(chatId);
  });
  return () => off?.();
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
  Notify: async (title, body, chatId) => backend().Notify(title, body, chatId),
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
  SendChatWithImages: async (id, text, images) =>
    backend().SendChatWithImages(id, text, images),
  StopChat: async (id) => backend().StopChat(id),
  CloseChat: async (id) => backend().CloseChat(id),
  AnswerChat: async (id, request, allow, answers) =>
    backend().AnswerChat(id, request, allow, answers),
  PickFiles: async (id) => backend().PickFiles(id),
  ProjectFiles: async (id, query) => backend().ProjectFiles(id, query),
  ChatModes: async (id) => backend().ChatModes(id),
  SetChatMode: async (id, mode) => backend().SetChatMode(id, mode),
  AnswerChatOption: async (id, request, option) =>
    backend().AnswerChatOption(id, request, option),
  PrepareReview: async (id) => backend().PrepareReview(id),
  ChatChanges: async (id) => backend().ChatChanges(id),
  CommitPlan: async (id) => backend().CommitPlan(id),
  Commit: async (id, request) => backend().Commit(id, request),
  PullRequestPlan: async (id) => backend().PullRequestPlan(id),
  CreatePullRequest: async (id, request) =>
    backend().CreatePullRequest(id, request),
  OpenURL: async (link) => backend().OpenURL(link),
  StartComparison: async (project, text) =>
    backend().StartComparison(project, text),
  ComparisonOverview: async (id) => backend().ComparisonOverview(id),
  ComparisonDiff: async (id, tool) => backend().ComparisonDiff(id, tool),
  UseComparisonSide: async (id, tool) => backend().UseComparisonSide(id, tool),
  RemoveComparison: async (id) => backend().RemoveComparison(id),
  UsageLimits: async (refresh) => backend().UsageLimits(refresh),
  GitHubIssueForm: async (project) => backend().GitHubIssueForm(project),
  AgentSkills: async (tool, project, refresh) =>
    backend().AgentSkills(tool, project, refresh),
  SetChatWebSearch: async (id, mode) => backend().SetChatWebSearch(id, mode),
  CreateGitHubIssue: async (project, title, body, labels) =>
    backend().CreateGitHubIssue(project, title, body, labels),
  GitHubOverview: async (project) => backend().GitHubOverview(project),
  GitHubIssue: async (project, number) =>
    backend().GitHubIssue(project, number),
  GitHubPull: async (project, number) => backend().GitHubPull(project, number),
  SearchMessages: async (query) => backend().SearchMessages(query),
  HandOffChat: async (chatId) => backend().HandOffChat(chatId),
  NewChatInCopy: async (project, tool) =>
    backend().NewChatInCopy(project, tool),
  CopyChanges: async (chatId) => backend().CopyChanges(chatId),
  BringBackCopy: async (chatId) => backend().BringBackCopy(chatId),
  RemoveCopy: async (chatId, discard) => backend().RemoveCopy(chatId, discard),
  SavedPrompts: async () => backend().SavedPrompts(),
  SetSavedPrompts: async (prompts) => backend().SetSavedPrompts(prompts),
  TurnDiff: async (chatId, checkpointId) =>
    backend().TurnDiff(chatId, checkpointId),
  ProjectCommands: async (chatId) => backend().ProjectCommands(chatId),
  StartProjectCommand: async (chatId, command) =>
    backend().StartProjectCommand(chatId, command),
  ProjectRun: async (chatId) => backend().ProjectRun(chatId),
  StopProjectCommand: async (chatId) => backend().StopProjectCommand(chatId),
  ProjectInstructions: async (project) =>
    backend().ProjectInstructions(project),
  ReadInstruction: async (project, name) =>
    backend().ReadInstruction(project, name),
  WriteInstruction: async (project, name, text, version) =>
    backend().WriteInstruction(project, name, text, version),
  RevealInstruction: async (project, name) =>
    backend().RevealInstruction(project, name),
  RestorePlan: async (chatId, checkpointId) =>
    backend().RestorePlan(chatId, checkpointId),
  RestoreCheckpoint: async (chatId, checkpointId) =>
    backend().RestoreCheckpoint(chatId, checkpointId),
  DiscardFile: async (chatId, path) => backend().DiscardFile(chatId, path),
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
