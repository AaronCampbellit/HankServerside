import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import {
  agentDisplayName,
  agentHasCapability,
  agentIsOnline,
  agentIsPrimary,
  agentsClient,
  type AgentAlert,
  type AgentMetrics,
  type HomeAgentEntry,
} from "../api/agents";
import { terminalStore } from "../api/terminalStore";
import { bootstrapClient } from "../api/bootstrap";
import { homeClient, type AgentToken, type CreatedAgentToken } from "../api/home";
import { useConfirmDialog, useToast } from "../ui/primitives";
import { DesktopReadinessCard } from "../desktop/DesktopReadinessCard";
import {desktopAuditClient,type DesktopAgentReadiness} from "../api/desktopAudit";

type PageState =
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "unsupported" }
  | {
      status: "ready";
      isAdmin: boolean;
      agents: HomeAgentEntry[];
      tokens: AgentToken[];
      alerts: AgentAlert[];
    };

function errorMessage(error: unknown): string {
  return error instanceof Error && error.message ? error.message : "Agents could not be loaded.";
}

function formatBytes(bytes: number | undefined): string {
  if (!bytes || bytes <= 0) return "—";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(value >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`;
}

function percentOf(used: number | undefined, total: number | undefined): number | null {
  if (!used || !total || total <= 0) return null;
  return Math.round((used / total) * 100);
}

function formatUptime(seconds: number | undefined): string {
  if (!seconds || seconds <= 0) return "—";
  const days = Math.floor(seconds / 86400);
  if (days > 0) return `${days}d ${Math.floor((seconds % 86400) / 3600)}h`;
  const hours = Math.floor(seconds / 3600);
  if (hours > 0) return `${hours}h ${Math.floor((seconds % 3600) / 60)}m`;
  return `${Math.floor(seconds / 60)}m`;
}

function relativeTime(value: string | null | undefined): string {
  if (!value) return "never";
  const then = new Date(value).getTime();
  if (Number.isNaN(then)) return "unknown";
  const diff = Date.now() - then;
  const minutes = Math.round(diff / 60000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.round(hours / 24)}d ago`;
}

function agentKindLabel(agent: HomeAgentEntry): string {
  return agentIsPrimary(agent) ? "Home agent" : "Worker";
}

function MetricStat({ label, value, tone }: { label: string; value: string; tone?: "warn" | "bad" }) {
  return (
    <div className={`agent-stat${tone ? ` agent-stat-${tone}` : ""}`}>
      <strong>{value}</strong>
      <span>{label}</span>
    </div>
  );
}

function MetricRow({ metrics }: { metrics: AgentMetrics | undefined }) {
  if (!metrics) return <span className="agent-tile-idle">no metrics</span>;
  const ram = percentOf(metrics.memory_used_bytes, metrics.memory_total_bytes);
  const disk = percentOf(metrics.disk_used_bytes, metrics.disk_total_bytes);
  return (
    <div className="agent-tile-metrics">
      {typeof metrics.cpu_load_1m === "number" ? <span>CPU {metrics.cpu_load_1m.toFixed(2)}</span> : null}
      {ram !== null ? <span>RAM {ram}%</span> : null}
      {disk !== null ? <span className={disk >= 90 ? "agent-metric-bad" : undefined}>Disk {disk}%</span> : null}
      {typeof metrics.battery_percent === "number" ? (
        <span>{metrics.battery_charging ? "⚡" : "Batt"} {metrics.battery_percent}%</span>
      ) : null}
    </div>
  );
}

function AgentCard({ agent, onOpen }: { agent: HomeAgentEntry; onOpen: () => void }) {
  const online = agentIsOnline(agent);
  return (
    <button type="button" className="agent-card" onClick={onOpen}>
      <div className="agent-card-top">
        <span className={`agent-avatar ${agentIsPrimary(agent) ? "is-primary" : "is-worker"}`} aria-hidden="true">
          {agentIsPrimary(agent) ? "⌂" : "▤"}
        </span>
        <div className="agent-card-identity">
          <strong>{agentDisplayName(agent)}</strong>
          <span>{agentKindLabel(agent)}</span>
        </div>
        <span className={`status-pill ${online ? "status-online" : "status-offline"}`}>
          {online ? "Online" : "Offline"}
        </span>
      </div>
      {online ? (
        <MetricRow metrics={agent.metrics} />
      ) : (
        <span className="agent-tile-idle">last seen {relativeTime(agent.last_seen_at)}</span>
      )}
    </button>
  );
}

function ShellConsole({ agent }: { agent: HomeAgentEntry }) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const writtenRef = useRef(0);
  const snapshot = useSyncExternalStore(
    (listener) => terminalStore.subscribe(agent.agent_id, listener),
    () => terminalStore.snapshot(agent.agent_id),
  );

  useEffect(() => {
    const host = hostRef.current;
    if (!host || navigator.userAgent.toLowerCase().includes("jsdom")) return;
    const terminal = new Terminal({
      cursorBlink: true,
      convertEol: false,
      fontFamily: "var(--font-mono)",
      fontSize: 12,
      scrollback: 5000,
      theme: { background: "#0b1118", foreground: "#d7e0ea", cursor: "#57a6ff", selectionBackground: "#2c5c88" },
    });
    const fit = new FitAddon();
    terminal.loadAddon(fit);
    terminal.open(host);
    fit.fit();
    terminalRef.current = terminal;
    fitRef.current = fit;
    const input = terminal.onData((data) => { void terminalStore.write(agent.agent_id, data); });
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(() => {
      fit.fit();
      void terminalStore.resize(agent.agent_id, terminal.cols, terminal.rows);
    });
    observer?.observe(host);
    return () => { observer?.disconnect(); input.dispose(); terminal.dispose(); terminalRef.current = null; fitRef.current = null; writtenRef.current = 0; };
  }, [agent.agent_id]);

  useEffect(() => {
    const terminal = terminalRef.current;
    if (!terminal) return;
    if (snapshot.output.length < writtenRef.current) { terminal.clear(); writtenRef.current = 0; }
    const next = snapshot.output.slice(writtenRef.current);
    if (next) terminal.write(next);
    writtenRef.current = snapshot.output.length;
  }, [snapshot.output]);

  useEffect(() => {
    if (!snapshot.sessionID || snapshot.status === "closed" || snapshot.status === "exited") return;
    void terminalStore.attach(agent.agent_id);
    const timer = window.setInterval(() => { void terminalStore.attach(agent.agent_id); }, 10_000);
    return () => window.clearInterval(timer);
  }, [agent.agent_id, snapshot.sessionID]);

  async function start() {
    const terminal = terminalRef.current;
    await terminalStore.open(agent.agent_id, terminal?.cols || 100, terminal?.rows || 30);
    terminal?.focus();
  }

  return (
    <div className="agent-shell">
      <div className="agent-shell-head">
        <div><h3>Live shell</h3><span className="agent-shell-badge">{snapshot.status} · audited · admin only</span></div>
        <div className="agent-shell-actions">
          {snapshot.status === "closed" || snapshot.status === "exited" || snapshot.status === "error" ? (
            <button type="button" className="secondary" onClick={() => void start()}>New terminal</button>
          ) : (
            <button type="button" className="danger" onClick={() => void terminalStore.close(agent.agent_id)}>Close</button>
          )}
        </div>
      </div>
      <div className="agent-shell-terminal" ref={hostRef} aria-label={`Live terminal on ${agentDisplayName(agent)}`} />
      {snapshot.error ? <p className="agent-shell-error">{snapshot.error}</p> : null}
    </div>
  );
}

function AgentDetail({
  agent,
  isAdmin,
  onBack,
  onAction,
}: {
  agent: HomeAgentEntry;
  isAdmin: boolean;
  onBack: () => void;
  onAction: (kind: "lock" | "restart" | "wake", agent: HomeAgentEntry) => void;
}) {
  const online = agentIsOnline(agent);
  const [desktopReadiness,setDesktopReadiness]=useState<DesktopAgentReadiness|null>(null);
  useEffect(()=>{let live=true;desktopAuditClient.readiness(agent.agent_id).then(value=>{if(live)setDesktopReadiness(value)}).catch(()=>{if(live)setDesktopReadiness(null)});return()=>{live=false}},[agent.agent_id]);
  const metrics = agent.metrics;
  const ram = percentOf(metrics?.memory_used_bytes, metrics?.memory_total_bytes);
  const disk = percentOf(metrics?.disk_used_bytes, metrics?.disk_total_bytes);

  return (
    <section className="agent-detail">
      <div className="agent-detail-head">
        <button type="button" className="secondary agent-back" onClick={onBack}>← All devices</button>
        <div className="agent-detail-title">
          <span className={`agent-avatar ${agentIsPrimary(agent) ? "is-primary" : "is-worker"}`} aria-hidden="true">
            {agentIsPrimary(agent) ? "⌂" : "▤"}
          </span>
          <div>
            <strong>{agentDisplayName(agent)}</strong>
            <span>{agent.metadata?.os_version || agentKindLabel(agent)}</span>
          </div>
        </div>
        <span className={`status-pill ${online ? "status-online" : "status-offline"}`}>
          {online ? "Online" : `Offline · ${relativeTime(agent.last_seen_at)}`}
        </span>
      </div>

      <div className="agent-stat-grid">
        {typeof metrics?.cpu_load_1m === "number" ? <MetricStat label="CPU load (1m)" value={metrics.cpu_load_1m.toFixed(2)} /> : null}
        {ram !== null ? (
          <MetricStat
            label={`Memory (${formatBytes(metrics?.memory_used_bytes)} / ${formatBytes(metrics?.memory_total_bytes)})`}
            value={`${ram}%`}
            tone={ram >= 90 ? "warn" : undefined}
          />
        ) : null}
        {disk !== null ? (
          <MetricStat
            label={`Disk (${formatBytes(metrics?.disk_used_bytes)} / ${formatBytes(metrics?.disk_total_bytes)})`}
            value={`${disk}%`}
            tone={disk >= 90 ? "bad" : undefined}
          />
        ) : null}
        {typeof metrics?.battery_percent === "number" ? (
          <MetricStat label={metrics.battery_charging ? "Battery (charging)" : "Battery"} value={`${metrics.battery_percent}%`} />
        ) : null}
        {typeof metrics?.uptime_seconds === "number" ? <MetricStat label="Uptime" value={formatUptime(metrics.uptime_seconds)} /> : null}
      </div>

      <div className="agent-detail-columns">
        <DesktopReadinessCard readiness={desktopReadiness} />
        <div className="agent-info-card">
          <h3>Details</h3>
          <dl className="agent-info-list">
            <div><dt>Agent ID</dt><dd>{agent.agent_id}</dd></div>
            <div><dt>Type</dt><dd>{agentKindLabel(agent)}</dd></div>
            <div><dt>Status</dt><dd>{online ? "Online" : "Offline"}</dd></div>
            <div><dt>Last seen</dt><dd>{relativeTime(agent.last_seen_at)}</dd></div>
            {agent.metadata?.hostname ? <div><dt>Hostname</dt><dd>{agent.metadata.hostname}</dd></div> : null}
            {agent.metadata?.platform ? <div><dt>Platform</dt><dd>{agent.metadata.platform}</dd></div> : null}
            {agent.metadata?.app_version ? <div><dt>Agent version</dt><dd>{agent.metadata.app_version}</dd></div> : null}
          </dl>
          {agent.capabilities && agent.capabilities.length ? (
            <div className="agent-capabilities">
              {agent.capabilities.map((capability) => (
                <span className="agent-capability" key={capability}>{capability}</span>
              ))}
            </div>
          ) : null}
        </div>

        <div className="agent-info-card">
          <h3>Actions</h3>
          {isAdmin ? (
            <div className="agent-actions">
			  {online && agentHasCapability(agent, "desktop.session.open") && agentHasCapability(agent, "desktop.view") ? (
				<a className="button" href={`/dashboard/agents/${encodeURIComponent(agent.agent_id)}/desktop`}>Open Remote Desktop</a>
			  ) : null}
              {agentHasCapability(agent, "host.lock") ? (
                <button type="button" className="secondary" disabled={!online} onClick={() => onAction("lock", agent)}>Lock screen</button>
              ) : null}
              {agentHasCapability(agent, "wol.send") ? (
                <button type="button" className="secondary" disabled={!online} onClick={() => onAction("wake", agent)}>Wake device…</button>
              ) : null}
              <button type="button" className="danger" disabled={!online} onClick={() => onAction("restart", agent)}>Restart agent</button>
            </div>
          ) : (
            <p className="empty-state">Device actions require an admin account.</p>
          )}
          {!agentHasCapability(agent, "shell.session.open") ? (
            <p className="agent-hint">Remote shell is disabled on this device. Enable shell commands in that agent's settings to allow live terminals.</p>
          ) : null}
        </div>
      </div>

      {isAdmin && agentHasCapability(agent, "shell.session.open") ? <ShellConsole agent={agent} /> : null}
    </section>
  );
}

function TokenSection({
  tokens,
  onCreate,
  onRevoke,
}: {
  tokens: AgentToken[];
  onCreate: (agentID: string, name: string) => Promise<CreatedAgentToken | null>;
  onRevoke: (token: AgentToken) => void;
}) {
  const [agentID, setAgentID] = useState("");
  const [name, setName] = useState("");
  const [created, setCreated] = useState<CreatedAgentToken | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit() {
    if (!agentID.trim() || busy) return;
    setBusy(true);
    try {
      const result = await onCreate(agentID.trim(), name.trim() || agentID.trim());
      if (result) {
        setCreated(result);
        setAgentID("");
        setName("");
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="agent-panel">
      <div className="agent-panel-head">
        <h2>Enrollment tokens</h2>
        <span className="meta-line">Admin only · one token per device</span>
      </div>
      <form
        className="agent-token-form"
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        <label>
          <span>Agent ID</span>
          <input value={agentID} placeholder="mac-studio" autoComplete="off" onChange={(event) => setAgentID(event.target.value)} />
        </label>
        <label>
          <span>Display name</span>
          <input value={name} placeholder="Mac Studio" autoComplete="off" onChange={(event) => setName(event.target.value)} />
        </label>
        <button type="submit" disabled={busy || !agentID.trim()}>{busy ? "Creating…" : "Create token"}</button>
      </form>

      {created ? (
        <div className="agent-token-created">
          <p>Token for <strong>{created.agent_id}</strong> — copy it now, it won't be shown again:</p>
          <code className="agent-token-value">{created.token}</code>
        </div>
      ) : null}

      {tokens.length ? (
        <table className="agent-token-table">
          <thead>
            <tr><th scope="col">Agent</th><th scope="col">Created</th><th scope="col">Status</th><th scope="col" /></tr>
          </thead>
          <tbody>
            {tokens.map((token) => (
              <tr key={token.id}>
                <td>{token.agent_id}</td>
                <td>{relativeTime(token.created_at)}</td>
                <td>{token.revoked_at ? <span className="status-pill status-offline">Revoked</span> : <span className="status-pill status-online">Active</span>}</td>
                <td className="agent-token-actions">
                  {!token.revoked_at ? (
                    <button type="button" className="danger" onClick={() => onRevoke(token)}>Revoke</button>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <p className="empty-state">No enrollment tokens yet.</p>
      )}
    </section>
  );
}

export function AgentsPage() {
  const [state, setState] = useState<PageState>({ status: "loading" });
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const { showToast } = useToast();
  const { confirm, prompt } = useConfirmDialog();

  const refresh = useCallback(async (): Promise<HomeAgentEntry[] | null> => {
    try {
      const agents = await agentsClient.listAgents();
      setState((current) =>
        current.status === "ready"
          ? { ...current, agents }
          : current,
      );
      return agents;
    } catch {
      return null;
    }
  }, []);

  useEffect(() => {
    let active = true;
    (async () => {
      try {
        const bootstrap = await bootstrapClient.load();
        const isAdmin = Boolean(bootstrap.permissions?.is_admin);
        const agents = await agentsClient.listAgents();
        const tokens = isAdmin ? await homeClient.listAgentTokens().then((payload) => payload.tokens).catch(() => []) : [];
        if (!active) return;
        setState({ status: "ready", isAdmin, agents, tokens, alerts: [] });
        await agentsClient.subscribeHealth();
      } catch (error) {
        if (!active) return;
        if (error instanceof Error && /not found|404/i.test(error.message)) {
          setState({ status: "unsupported" });
        } else {
          setState({ status: "error", message: errorMessage(error) });
        }
      }
    })();

    const unsubscribe = agentsClient.onAlert((alert) => {
      if (!active) return;
      setState((current) => (current.status === "ready" ? { ...current, alerts: [alert, ...current.alerts].slice(0, 25) } : current));
      showToast(alert.summary, alert.severity === "info" ? "neutral" : "error");
      void refresh();
    });

    const timer = window.setInterval(() => void refresh(), 15000);
    return () => {
      active = false;
      unsubscribe();
      window.clearInterval(timer);
    };
  }, [refresh, showToast]);

  const ready = state.status === "ready" ? state : null;
  const selected = useMemo(
    () => (ready && selectedID ? ready.agents.find((agent) => agent.agent_id === selectedID) ?? null : null),
    [ready, selectedID],
  );

  async function performAction(kind: "lock" | "restart" | "wake", agent: HomeAgentEntry) {
    try {
      if (kind === "lock") {
        await agentsClient.lock(agent.agent_id);
        showToast(`Locked ${agentDisplayName(agent)}`);
      } else if (kind === "restart") {
        const ok = await confirm({
          title: `Restart ${agentDisplayName(agent)}?`,
          message: "The agent process will restart and briefly disconnect.",
          confirmLabel: "Restart",
          tone: "danger",
        });
        if (!ok) return;
        await agentsClient.restart(agent.agent_id);
        showToast(`Restart requested for ${agentDisplayName(agent)}`);
      } else if (kind === "wake") {
        const mac = await prompt({
          title: "Wake a device",
          message: `${agentDisplayName(agent)} will broadcast a wake-on-LAN packet to this MAC address.`,
          placeholder: "AA:BB:CC:DD:EE:FF",
          confirmLabel: "Send wake packet",
        });
        if (!mac) return;
        await agentsClient.wakeOnLAN(agent.agent_id, mac);
        showToast(`Sent wake packet from ${agentDisplayName(agent)}`);
      }
    } catch (error) {
      showToast(errorMessage(error), "error");
    }
  }

  async function createToken(agentID: string, name: string): Promise<CreatedAgentToken | null> {
    try {
      const token = await homeClient.createAgentToken({ agent_id: agentID, name, agent_type: "worker", expires_in_seconds: 0 });
      const tokens = await homeClient.listAgentTokens().then((payload) => payload.tokens).catch(() => []);
      setState((current) => (current.status === "ready" ? { ...current, tokens } : current));
      showToast(`Token created for ${agentID}`);
      return token;
    } catch (error) {
      showToast(errorMessage(error), "error");
      return null;
    }
  }

  async function revokeToken(token: AgentToken) {
    const ok = await confirm({
      title: `Revoke ${token.agent_id}?`,
      message: "That device will be disconnected and can no longer authenticate.",
      confirmLabel: "Revoke",
      tone: "danger",
    });
    if (!ok) return;
    try {
      await homeClient.revokeAgentToken(token.id);
      const tokens = await homeClient.listAgentTokens().then((payload) => payload.tokens).catch(() => []);
      setState((current) => (current.status === "ready" ? { ...current, tokens } : current));
      showToast(`Revoked ${token.agent_id}`);
    } catch (error) {
      showToast(errorMessage(error), "error");
    }
  }

  if (state.status === "loading") {
    return (
      <section className="dashboard-page agents-page" aria-labelledby="route-title">
        <p className="eyebrow">Hank Remote</p>
        <h1 id="route-title">Agents</h1>
        <p className="loading-state"><span className="spinner" aria-hidden="true" />Loading agents…</p>
      </section>
    );
  }

  if (state.status === "unsupported") {
    return (
      <section className="dashboard-page agents-page" aria-labelledby="route-title">
        <p className="eyebrow">Hank Remote</p>
        <h1 id="route-title">Agents</h1>
        <p className="notice-state">This server doesn't support multiple agents yet. Deploy the multi-agent update to manage devices here.</p>
      </section>
    );
  }

  if (state.status === "error") {
    return (
      <section className="dashboard-page agents-page" aria-labelledby="route-title">
        <p className="eyebrow">Hank Remote</p>
        <h1 id="route-title">Agents</h1>
        <p className="error-state">{state.message}</p>
      </section>
    );
  }

  const onlineCount = state.agents.filter(agentIsOnline).length;

  return (
    <section className="dashboard-page agents-page" aria-labelledby="route-title">
      <header className="dashboard-header">
        <div>
          <p className="eyebrow">Hank Remote</p>
          <h1 id="route-title">Agents</h1>
          <p className="meta-line">{state.agents.length} device{state.agents.length === 1 ? "" : "s"} · {onlineCount} online</p>
        </div>
        <div className="settings-actions">
          <button type="button" className="secondary" onClick={() => void refresh()}>Refresh</button>
        </div>
      </header>

      {state.alerts.length ? (
        <section className="agent-alerts" aria-label="Recent alerts">
          {state.alerts.slice(0, 4).map((alert, index) => (
            <div className={`agent-alert agent-alert-${alert.severity}`} key={`${alert.agent_id}-${alert.time}-${index}`}>
              <span className="agent-alert-dot" aria-hidden="true" />
              <span className="agent-alert-body">
                <strong>{state.agents.find((agent) => agent.agent_id === alert.agent_id)?.name || alert.agent_id}</strong>
                {" "}{alert.summary}
              </span>
              <span className="agent-alert-time">{relativeTime(alert.time)}</span>
            </div>
          ))}
        </section>
      ) : null}

      {selected ? (
        <AgentDetail agent={selected} isAdmin={state.isAdmin} onBack={() => setSelectedID(null)} onAction={(kind, agent) => void performAction(kind, agent)} />
      ) : state.agents.length ? (
        <div className="agent-grid">
          {state.agents.map((agent) => (
            <AgentCard agent={agent} key={agent.agent_id} onOpen={() => setSelectedID(agent.agent_id)} />
          ))}
        </div>
      ) : (
        <p className="empty-state">No agents are registered yet. Create an enrollment token below to connect a device.</p>
      )}

      {state.isAdmin && !selected ? (
        <TokenSection tokens={state.tokens} onCreate={createToken} onRevoke={(token) => void revokeToken(token)} />
      ) : null}
    </section>
  );
}
