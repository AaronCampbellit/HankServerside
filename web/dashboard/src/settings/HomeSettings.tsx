import { type FormEvent, useEffect, useState } from "react";
import {
  homeClient,
  type AgentPayload,
  type AgentToken,
  type AgentTokensPayload,
  type CreatedAgentToken,
  type Home,
} from "../api/home";

type State =
  | { status: "loading" }
  | { status: "error"; message: string }
  | {
      status: "ready";
      home: Home;
      homeName: string;
      agentPayload: AgentPayload;
      tokens: AgentToken[];
      tokenForm: { agentID: string; name: string; expiresInSeconds: string };
      createdToken: CreatedAgentToken | null;
      message: string;
    };

function errorMessage(error: unknown): string {
  return error instanceof Error && error.message ? error.message : "Home settings could not be loaded.";
}

function agentIDFromHomeName(homeName: string): string {
  const slug = homeName.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
  return slug || "home-agent";
}

function agentNameFromHomeName(homeName: string): string {
  const name = homeName.trim();
  return name ? `${name} Agent` : "Home Agent";
}

function quotedEnv(value: string | undefined): string {
  return `"${String(value || "").replaceAll("\\", "\\\\").replaceAll('"', '\\"')}"`;
}

function agentEnvFile(token: CreatedAgentToken, home: Home): string {
  return [
    "HANK_REMOTE_AGENT_CLOUD_URL=ws://cloud:8080/ws/agent",
    `HANK_REMOTE_AGENT_ID=${quotedEnv(token.agent_id || agentIDFromHomeName(home.name))}`,
    `HANK_REMOTE_AGENT_TOKEN=${quotedEnv(token.token)}`,
    `HANK_REMOTE_AGENT_HOME_NAME=${quotedEnv(home.name || "Home")}`,
    "HANK_REMOTE_AGENT_CONFIG_PATH=/app/.env.agent",
    "",
    "HANK_REMOTE_AGENT_FILES_ROOT=/srv/hank/files",
    "HANK_REMOTE_AGENT_NOTES_ROOT=/srv/hank/notes",
  ].join("\n");
}

function formatDate(value: string | null | undefined): string {
  return value ? new Date(value).toLocaleString() : "Never";
}

export function HomeSettings() {
  const [state, setState] = useState<State>({ status: "loading" });

  async function load(message = "", createdToken: CreatedAgentToken | null = null) {
    try {
      const [home, agentPayload, tokenPayload] = await Promise.all([
        homeClient.getHome(),
        homeClient.getAgent(),
        homeClient.listAgentTokens(),
      ]);
      setState({
        status: "ready",
        home,
        homeName: home.name,
        agentPayload,
        tokens: tokenPayload.tokens || [],
        tokenForm: {
          agentID: agentIDFromHomeName(home.name),
          name: agentNameFromHomeName(home.name),
          expiresInSeconds: "",
        },
        createdToken,
        message,
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
      <section className="settings-page" aria-labelledby="route-title">
        <p className="eyebrow">Hank Remote</p>
        <h1 id="route-title">Home & Connector</h1>
        <p className="loading-state">Loading home settings...</p>
      </section>
    );
  }

  if (state.status === "error") {
    return (
      <section className="settings-page" aria-labelledby="route-title">
        <p className="eyebrow">Hank Remote</p>
        <h1 id="route-title">Home & Connector</h1>
        <p className="error-state">{state.message}</p>
      </section>
    );
  }

  function setReady(next: Partial<Extract<State, { status: "ready" }>>) {
    setState((current) => current.status === "ready" ? { ...current, ...next } : current);
  }

  const readyState = state;

  async function saveHome(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      const home = await homeClient.renameHome(readyState.homeName.trim());
      setReady({
        home,
        homeName: home.name,
        tokenForm: {
          ...readyState.tokenForm,
          agentID: agentIDFromHomeName(home.name),
          name: agentNameFromHomeName(home.name),
        },
        message: "Home updated.",
      });
    } catch (error) {
      setReady({ message: errorMessage(error) });
    }
  }

  async function restartAgent() {
    try {
      const response = await homeClient.restartAgent();
      setReady({ message: response.message || "Connector restart requested." });
    } catch (error) {
      setReady({ message: errorMessage(error) });
    }
  }

  async function createSetupFile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      const token = await homeClient.createAgentToken({
        agent_id: readyState.tokenForm.agentID.trim() || agentIDFromHomeName(readyState.home.name),
        name: readyState.tokenForm.name.trim() || agentNameFromHomeName(readyState.home.name),
        agent_type: "primary",
        expires_in_seconds: Number.parseInt(readyState.tokenForm.expiresInSeconds || "0", 10) || 0,
      });
      const tokenPayload: AgentTokensPayload = await homeClient.listAgentTokens();
      setReady({ tokens: tokenPayload.tokens || [], createdToken: token, message: "Setup file created." });
    } catch (error) {
      setReady({ message: errorMessage(error) });
    }
  }

  async function revokeToken(token: AgentToken) {
    try {
      await homeClient.revokeAgentToken(token.id);
      const tokenPayload = await homeClient.listAgentTokens();
      setReady({ tokens: tokenPayload.tokens || [], message: "Setup file disabled." });
    } catch (error) {
      setReady({ message: errorMessage(error) });
    }
  }

  const agent = readyState.agentPayload.agent;
  const canRestart = readyState.agentPayload.can_restart && agent?.status === "online";

  return (
    <section className="settings-page" aria-labelledby="route-title">
      <header className="dashboard-header">
        <div>
          <p className="eyebrow">Hank Remote</p>
          <h1 id="route-title">Home & Connector</h1>
          <p className="meta-line">Home name, connector setup, and the agent setup file.</p>
        </div>
      </header>

      {readyState.message ? <p className="notice-state">{readyState.message}</p> : null}

      <section className="settings-panel" aria-label="Home name">
        <h2>Home Name</h2>
        <form className="inline-form" onSubmit={saveHome}>
          <label>
            <span>Home name</span>
            <input
              onChange={(event) => setReady({ homeName: event.target.value })}
              type="text"
              value={readyState.homeName}
            />
          </label>
          <label>
            <span>Time zone</span>
            <input readOnly type="text" value={Intl.DateTimeFormat().resolvedOptions().timeZone || "America/Chicago"} />
          </label>
          <button type="submit">Save home name</button>
          <button className="secondary" type="button" onClick={() => setReady({ homeName: readyState.home.name, message: "" })}>
            Discard
          </button>
        </form>
      </section>

      <section className="settings-panel" aria-label="Home connector">
        <div className="panel-heading">
          <h2>Home Connector</h2>
          <span className={`status-pill status-${agent?.status || "offline"}`}>{agent?.status || "offline"}</span>
        </div>
        {agent ? (
          <div className="dashboard-grid">
            <article className="dashboard-tile">
              <span>Name</span>
              <strong>{agent.name}</strong>
            </article>
            <article className="dashboard-tile">
              <span>Connector ID</span>
              <strong>{agent.agent_id}</strong>
            </article>
            <article className="dashboard-tile">
              <span>Last online</span>
              <strong>{formatDate(agent.last_seen_at)}</strong>
              <small>Last heartbeat</small>
            </article>
            <article className="dashboard-tile">
              <span>Agent version</span>
              <strong>{(agent as { version?: string }).version || "v2.4.1"}</strong>
              <small>Connector runtime</small>
            </article>
          </div>
        ) : (
          <p className="empty-state">The home connector has not been set up yet.</p>
        )}
        <div className="form-actions">
          <button disabled={!canRestart} onClick={() => void restartAgent()} type="button">
            Restart connector
          </button>
        </div>
      </section>

      <section className="settings-panel" aria-label="Setup file">
        <h2>Agent setup file</h2>
        <p className="empty-state">Generate a fresh <code>.env.agent</code> for the home connector.</p>
        <form className="quick-link-form" onSubmit={createSetupFile}>
          <label>
            <span>Connector ID</span>
            <input
              onChange={(event) => setReady({ tokenForm: { ...readyState.tokenForm, agentID: event.target.value } })}
              required
              type="text"
              value={readyState.tokenForm.agentID}
            />
          </label>
          <label>
            <span>Connector name</span>
            <input
              onChange={(event) => setReady({ tokenForm: { ...readyState.tokenForm, name: event.target.value } })}
              type="text"
              value={readyState.tokenForm.name}
            />
          </label>
          <label>
            <span>Expires after seconds</span>
            <input
              min={0}
              onChange={(event) => setReady({ tokenForm: { ...readyState.tokenForm, expiresInSeconds: event.target.value } })}
              step={1}
              type="number"
              value={readyState.tokenForm.expiresInSeconds}
            />
          </label>
          <button type="submit">Create setup file</button>
        </form>

        {readyState.createdToken ? (
          <pre className="token-output">{agentEnvFile(readyState.createdToken, readyState.home)}</pre>
        ) : null}

        <div aria-label="Setup files" className="quick-links-list settings-list" role="list">
          {readyState.tokens.length > 0 ? readyState.tokens.map((token) => {
            const revoked = Boolean(token.revoked_at);
            return (
              <article className="quick-link-row" key={token.id} role="listitem">
                <div className="quick-link-copy">
                  <strong>{token.agent_id}</strong>
                  <span>Created: {formatDate(token.created_at)}</span>
                  <small>{revoked ? "Disabled" : `Expires: ${formatDate(token.expires_at)}`}</small>
                </div>
                <div className="row-actions">
                  <button className="danger-link" onClick={() => void revokeToken(token)} type="button">
                    {revoked ? `Remove setup file for ${token.agent_id}` : `Disable setup file for ${token.agent_id}`}
                  </button>
                </div>
              </article>
            );
          }) : (
            <p className="empty-state">No setup files have been created for this home yet.</p>
          )}
        </div>
      </section>
    </section>
  );
}
