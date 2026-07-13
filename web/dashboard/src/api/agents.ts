import { HankSocket, type HankSocketEvent } from "../socket/HankSocket";
import { apiClient, type ApiTransport } from "./client";
import { arrayFrom } from "./normalize";

export type AgentMetrics = {
  cpu_load_1m?: number;
  memory_used_bytes?: number;
  memory_total_bytes?: number;
  disk_used_bytes?: number;
  disk_total_bytes?: number;
  uptime_seconds?: number;
  battery_percent?: number;
  battery_charging?: boolean;
};

export type HomeAgentEntry = {
  agent_id: string;
  name?: string;
  status: "online" | "offline" | string;
  agent_type?: "primary" | "worker" | string;
  last_seen_at?: string | null;
  capabilities?: string[];
  metadata?: Record<string, string>;
  metrics?: AgentMetrics;
};

export type AgentsPayload = {
  agents: HomeAgentEntry[];
};

export type AgentAlert = {
  home_id?: string;
  agent_id: string;
  kind: string;
  severity: "info" | "warning" | string;
  summary: string;
  time?: string;
  details?: Record<string, unknown>;
};

export type ShellResult = {
  exit_code: number;
  stdout: string;
  stderr: string;
  truncated: boolean;
};

export type TerminalOpen = { session_id: string; cursor: number; shell?: string };
export type TerminalAttach = { session_id: string; cursor: number; output?: string; exited?: boolean; exit_code?: number };
export type TerminalEvent = {
  session_id: string;
  cursor: number;
  data?: string;
  exit_code?: number;
  reason?: string;
  exited: boolean;
};

export type AgentsSocket = {
  subscribe(topics: string[]): Promise<unknown>;
  sendCommand<T>(command: string, body?: unknown, options?: { timeoutMs?: number; agentID?: string }): Promise<T>;
  onEvent(listener: (event: HankSocketEvent) => void): () => void;
};

export function agentDisplayName(agent: HomeAgentEntry): string {
  if (agent.name && agent.name.trim()) return agent.name;
  return agent.metadata?.hostname || agent.agent_id;
}

export function agentIsPrimary(agent: HomeAgentEntry): boolean {
  return (agent.agent_type || "primary") === "primary";
}

export function agentIsOnline(agent: HomeAgentEntry): boolean {
  return String(agent.status).toLowerCase() === "online";
}

export function agentHasCapability(agent: HomeAgentEntry, capability: string): boolean {
  return Array.isArray(agent.capabilities) && agent.capabilities.includes(capability);
}

export class AgentsClient {
  constructor(
    private readonly api: ApiTransport = apiClient,
    private readonly socket: AgentsSocket = new HankSocket(),
  ) {}

  /** Requires the multi-agent server; older servers 404 (caller shows a notice). */
  async listAgents(): Promise<HomeAgentEntry[]> {
    const payload = await this.api.request<Partial<AgentsPayload>>("/v1/home/agents");
    return arrayFrom<HomeAgentEntry>(payload.agents);
  }

  async subscribeHealth(): Promise<void> {
    await this.socket.subscribe(["agents.health"]);
  }

  onAlert(listener: (alert: AgentAlert) => void): () => void {
    return this.socket.onEvent((event) => {
      if (event.topic !== "agents.health") return;
      const body = decodeEventBody(event.body);
      if (body && typeof body.agent_id === "string") {
        listener(body as unknown as AgentAlert);
      }
    });
  }

  // RMM device commands (admin-only, server-audited). Targeted by agent_id.

  lock(agentID: string): Promise<unknown> {
    return this.socket.sendCommand("host.lock", {}, { agentID });
  }

  restart(agentID: string): Promise<unknown> {
    return this.socket.sendCommand("system.restart", { reason: "requested from Hank dashboard" }, { agentID });
  }

  wakeOnLAN(agentID: string, mac: string, broadcast?: string): Promise<unknown> {
    const body: Record<string, unknown> = { mac };
    if (broadcast) body.broadcast = broadcast;
    return this.socket.sendCommand("wol.send", body, { agentID });
  }

  hostStatus(agentID: string): Promise<{ hostname?: string; platform?: string; os_version?: string; metrics?: AgentMetrics }> {
    return this.socket.sendCommand("host.status", {}, { agentID });
  }

  runShell(agentID: string, command: string, timeoutSeconds = 60): Promise<ShellResult> {
    return this.socket.sendCommand<ShellResult>(
      "shell.exec",
      { command, timeout_seconds: timeoutSeconds },
      { agentID, timeoutMs: (timeoutSeconds + 15) * 1000 },
    );
  }

  openTerminal(agentID: string, sessionID: string, columns: number, rows: number): Promise<TerminalOpen> {
    return this.socket.sendCommand("shell.session.open", { session_id: sessionID, columns, rows }, { agentID });
  }

  subscribeTerminal(sessionID: string): Promise<unknown> {
    return this.socket.subscribe([`shell.session:${sessionID}`]);
  }

  attachTerminal(agentID: string, sessionID: string, afterCursor: number): Promise<TerminalAttach> {
    return this.socket.sendCommand("shell.session.attach", { session_id: sessionID, after_cursor: afterCursor }, { agentID });
  }

  writeTerminal(agentID: string, sessionID: string, data: string): Promise<unknown> {
    return this.socket.sendCommand("shell.session.input", { session_id: sessionID, data }, { agentID });
  }

  resizeTerminal(agentID: string, sessionID: string, columns: number, rows: number): Promise<unknown> {
    return this.socket.sendCommand("shell.session.resize", { session_id: sessionID, columns, rows }, { agentID });
  }

  closeTerminal(agentID: string, sessionID: string): Promise<unknown> {
    return this.socket.sendCommand("shell.session.close", { session_id: sessionID }, { agentID });
  }

  onTerminalEvent(sessionID: string, listener: (event: TerminalEvent) => void): () => void {
    const topic = `shell.session:${sessionID}`;
    return this.socket.onEvent((event) => {
      if (event.topic !== topic || (event.event !== "shell.session.output" && event.event !== "shell.session.exited")) return;
      const body = decodeEventBody(event.body);
      if (!body || body.session_id !== sessionID || typeof body.cursor !== "number") return;
      listener({
        session_id: sessionID,
        cursor: body.cursor,
        data: typeof body.data === "string" ? body.data : undefined,
        exit_code: typeof body.exit_code === "number" ? body.exit_code : undefined,
        reason: typeof body.reason === "string" ? body.reason : undefined,
        exited: event.event === "shell.session.exited",
      });
    });
  }
}

function decodeEventBody(body: unknown): Record<string, unknown> | null {
  if (!body) return null;
  if (typeof body === "string") {
    try {
      return JSON.parse(body) as Record<string, unknown>;
    } catch {
      return null;
    }
  }
  return typeof body === "object" ? (body as Record<string, unknown>) : null;
}

export const agentsClient = new AgentsClient();
