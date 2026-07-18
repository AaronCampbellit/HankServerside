import { useEffect, useRef, useState } from "react";
import { appsClient, type AppSummary } from "../api/apps";
import { hankAIClient, type HankAIMessage, type HankAISession } from "../api/hankAI";
import { useConfirmDialog } from "../ui/primitives";

type SlashCommand = { token: string; hint: string };

type State =
  | { status: "loading" }
  | { status: "error"; message: string }
  | {
      status: "ready";
      assistantStatus: Record<string, unknown>;
      sessions: HankAISession[];
      selectedSessionID: string;
      messages: HankAIMessage[];
      draft: string;
      notice: string;
      sending: boolean;
      slashCommands: SlashCommand[];
    };

const BUILT_IN_SLASH_COMMANDS: SlashCommand[] = [
  { token: "/ha", hint: "Control Home Assistant entities" },
  { token: "/files", hint: "Search or browse the file server" },
  { token: "/notes", hint: "Find or append to a note" },
  { token: "/append", hint: "Append text to a note" },
  { token: "/calendar", hint: "Check the calendar" },
  { token: "/docs", hint: "Search project docs" },
  { token: "/status", hint: "Report connector + agent status" },
];

function slashCommandsForApps(apps: AppSummary[]): SlashCommand[] {
  const commands = [...BUILT_IN_SLASH_COMMANDS];
  const seen = new Set(commands.map((command) => command.token.toLowerCase()));
  for (const app of apps) {
    if (!app.enabled) continue;
    for (const slashCommand of app.slash_commands || []) {
      const token = slashCommand.command?.trim();
      if (!token?.startsWith("/") || token.includes(" ")) continue;
      const identity = token.toLowerCase();
      if (seen.has(identity)) continue;
      seen.add(identity);
      commands.push({
        token,
        hint: slashCommand.description?.trim() || `Run ${app.name?.trim() || app.id || app.app_id || "installed app"}`,
      });
    }
  }
  return commands;
}

function errorMessage(error: unknown): string {
  return error instanceof Error && error.message ? error.message : "HankAI could not be loaded.";
}

function messageText(message: HankAIMessage): string {
  return message.text || message.content || "";
}

function sessionTitle(session: HankAISession): string {
  return session.title?.trim() || "New Conversation";
}

export function HankAIPage() {
  const [state, setState] = useState<State>({ status: "loading" });
  const [mobileConversationsOpen, setMobileConversationsOpen] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const dialog = useConfirmDialog();

  async function load() {
    try {
      const [assistantStatus, sessionPayload, appsPayload] = await Promise.all([
        hankAIClient.status(),
        hankAIClient.listSessions(),
        appsClient.listApps().catch(() => ({ apps: [] })),
      ]);
      const sessions = sessionPayload.sessions || [];
      const selectedSessionID = sessions[0]?.id || "";
      const messages = selectedSessionID ? (await hankAIClient.listMessages(selectedSessionID)).messages || [] : [];
      setState({
        status: "ready",
        assistantStatus,
        sessions,
        selectedSessionID,
        messages,
        draft: "",
        notice: "",
        sending: false,
        slashCommands: slashCommandsForApps(appsPayload.apps),
      });
    } catch (error) {
      setState({ status: "error", message: errorMessage(error) });
    }
  }

  useEffect(() => {
    void load();
  }, []);

  if (state.status === "loading") {
    return (
      <section className="dashboard-page" aria-labelledby="route-title">
        <p className="eyebrow">Hank Remote</p>
        <h1 id="route-title">HankAI</h1>
        <p className="loading-state"><span className="spinner" aria-hidden="true" />Loading HankAI…</p>
      </section>
    );
  }

  if (state.status === "error") {
    return (
      <section className="dashboard-page" aria-labelledby="route-title">
        <p className="eyebrow">Hank Remote</p>
        <h1 id="route-title">HankAI</h1>
        <p className="error-state">{state.message}</p>
      </section>
    );
  }

  const readyState = state;

  function setReady(next: Partial<Extract<State, { status: "ready" }>>) {
    setState((current) => current.status === "ready" ? { ...current, ...next } : current);
  }

  async function selectSession(sessionID: string) {
    try {
      const payload = await hankAIClient.listMessages(sessionID);
      setReady({ selectedSessionID: sessionID, messages: payload.messages || [], notice: "" });
      setMobileConversationsOpen(false);
    } catch (error) {
      setReady({ notice: errorMessage(error) });
    }
  }

  async function createSession() {
    try {
      const session = await hankAIClient.createSession();
      setReady({
        sessions: [session, ...readyState.sessions],
        selectedSessionID: session.id,
        messages: [],
        notice: "Conversation created.",
      });
    } catch (error) {
      setReady({ notice: errorMessage(error) });
    }
  }

  async function deleteSession(session: HankAISession) {
    const title = sessionTitle(session);
    const confirmed = await dialog.confirm({
      title: "Delete conversation",
      message: "Delete " + title + "? This cannot be undone.",
      confirmLabel: "Delete",
      tone: "danger",
    });
    if (!confirmed) return;
    try {
      await hankAIClient.deleteSession(session.id);
    } catch (error) {
      setReady({ notice: errorMessage(error) });
      return;
    }
    const sessions = readyState.sessions.filter((candidate) => candidate.id !== session.id);
    const changedSelection = readyState.selectedSessionID === session.id;
    const selectedSessionID = changedSelection ? sessions[0]?.id || "" : readyState.selectedSessionID;
    setReady({
      sessions,
      selectedSessionID,
      messages: changedSelection ? [] : readyState.messages,
      notice: "Conversation deleted.",
    });
    if (!changedSelection || !selectedSessionID) return;
    try {
      const payload = await hankAIClient.listMessages(selectedSessionID);
      setState((current) => current.status === "ready" && current.selectedSessionID === selectedSessionID
        ? { ...current, messages: payload.messages || [] }
        : current);
    } catch (error) {
      setReady({ notice: errorMessage(error) });
    }
  }

  async function sendMessage() {
    const content = readyState.draft.trim();
    if (!content) return;
    let sessionID = readyState.selectedSessionID;
    let sessions = readyState.sessions;
    setReady({ sending: true, notice: "" });
    try {
      if (!sessionID) {
        const session = await hankAIClient.createSession();
        sessionID = session.id;
        sessions = [session, ...sessions];
      }
      const userMessage: HankAIMessage = { role: "user", text: content };
      setReady({ sessions, selectedSessionID: sessionID, draft: "", messages: [...readyState.messages, userMessage] });
      const run = await hankAIClient.sendMessage(sessionID, content);
      setReady({
        messages: run.assistant_message ? [...readyState.messages, userMessage, run.assistant_message] : [...readyState.messages, userMessage],
        notice: run.state === "completed" ? "Completed." : run.state,
        sending: false,
      });
    } catch (error) {
      setReady({ notice: errorMessage(error), sending: false });
    }
  }

  function applyCommand(token: string) {
    setReady({ draft: `${token} ` });
    requestAnimationFrame(() => textareaRef.current?.focus());
  }

  const provider = typeof state.assistantStatus.provider === "string" ? state.assistantStatus.provider : "assistant";
  const ready = state.assistantStatus.ready === true;
  const draftTrimmed = state.draft.trim();
  const showPalette = draftTrimmed.startsWith("/") && !draftTrimmed.includes(" ");
  const paletteMatches = showPalette
    ? state.slashCommands.filter((command) => command.token.toLowerCase().startsWith(draftTrimmed.toLowerCase()))
    : [];

  return (
    <section className="dashboard-page hank-page" aria-labelledby="route-title">
      <header className="dashboard-header">
        <div>
          <p className="eyebrow">Hank Remote</p>
          <h1 id="route-title">HankAI</h1>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 9 }}>
          <button
            className="secondary hank-mobile-conversations-toggle"
            type="button"
            aria-expanded={mobileConversationsOpen}
            aria-label={mobileConversationsOpen ? "Hide conversations" : "Show conversations"}
            onClick={() => setMobileConversationsOpen((open) => !open)}
          >
            Conversations
          </button>
          <span className={`status-pill ${state.sending ? "" : "status-online"}`}>
            <span className={`status-dot ${state.sending ? "warn" : "ok"}`} aria-hidden="true" />
            {state.sending ? "Working…" : "Ready"}
          </span>
          <span className={`status-pill ${ready ? "status-online" : ""}`}>{provider}</span>
        </div>
      </header>

      {state.notice ? <p className="notice-state">{state.notice}</p> : null}

      <div className="hank-layout">
        <section className={`settings-panel conversations-panel${mobileConversationsOpen ? " mobile-conversations-open" : ""}`} aria-label="Conversations">
          <div className="panel-heading">
            <h2>Conversations</h2>
            <button type="button" className="secondary" onClick={() => void createSession()}>New chat</button>
          </div>
          {state.sessions.length ? (
            <div className="notes-list">
              {state.sessions.map((session) => (
                <div className="conversation-row" key={session.id}>
                  <button
                    aria-label={sessionTitle(session)}
                    className={session.id === state.selectedSessionID ? "note-list-button active" : "note-list-button"}
                    onClick={() => void selectSession(session.id)}
                    type="button"
                  >
                    <strong>{sessionTitle(session)}</strong>
                    <span>{session.last_message_at || "No messages yet"}</span>
                  </button>
                  <button className="icon-button danger conversation-delete" type="button" aria-label={"Delete " + sessionTitle(session)} title="Delete conversation" onClick={() => void deleteSession(session)}>×</button>
                </div>
              ))}
            </div>
          ) : (
            <p className="empty-state">No conversations yet.</p>
          )}
        </section>

        <section className="settings-panel chat-panel" aria-label="Conversation">
          <div className="chat-panel-header">
            <div>
              <span className="eyebrow">Conversation</span>
              <h2>Hank</h2>
              <strong>{state.sessions.find((session) => session.id === state.selectedSessionID)?.title || "New conversation"}</strong>
            </div>
            <div className="chat-panel-actions">
              <span className="status-pill status-online">Ready</span>
            </div>
          </div>
          <div className="chat-thread">
            {state.messages.length ? (
              state.messages.map((message, index) => (
                <article className={`chat-msg ${message.role}`} key={message.id || `${message.role}-${index}`}>
                  <div className="chat-msg-role">{message.role === "user" ? "You" : "Hank"}</div>
                  <div className="chat-bubble">{messageText(message)}</div>
                </article>
              ))
            ) : (
              <div className="chat-empty">
                <strong>Start a conversation</strong>
                <span>Ask Hank about your home, files, notes, or devices. Type <code>/</code> for commands.</span>
              </div>
            )}
          </div>

          <form className="chat-composer" onSubmit={(event) => { event.preventDefault(); void sendMessage(); }}>
            {showPalette && paletteMatches.length ? (
              <div className="cmd-palette" role="listbox" aria-label="Slash commands">
                {paletteMatches.map((command) => (
                  <button
                    type="button"
                    role="option"
                    aria-selected={false}
                    key={command.token}
                    className="cmd-item"
                    onMouseDown={(event) => { event.preventDefault(); applyCommand(command.token); }}
                  >
                    <span className="cmd-token">{command.token}</span>
                    <span className="cmd-hint">{command.hint}</span>
                  </button>
                ))}
              </div>
            ) : null}
            <div className="composer-bar">
              <textarea
                ref={textareaRef}
                aria-label="Message"
                placeholder="Ask Hank…  (type / for commands)"
                rows={1}
                value={state.draft}
                onChange={(event) => setReady({ draft: event.target.value })}
                onKeyDown={(event) => {
                  if (event.key === "Enter" && !event.shiftKey) { event.preventDefault(); void sendMessage(); }
                }}
              />
              <button disabled={state.sending} type="submit" className="composer-send">Send</button>
            </div>
          </form>
        </section>
      </div>
    </section>
  );
}
