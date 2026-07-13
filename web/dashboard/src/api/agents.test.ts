import { describe, expect, it } from "vitest";
import {
  AgentsClient,
  agentDisplayName,
  agentHasCapability,
  agentIsOnline,
  agentIsPrimary,
  type HomeAgentEntry,
} from "./agents";
import type { HankSocketEvent } from "../socket/HankSocket";

function fakeSocket() {
  const commands: Array<{ command: string; body: unknown; agentID?: string }> = [];
  const subscriptions: string[][] = [];
  const listeners = new Set<(event: HankSocketEvent) => void>();
  const socket = {
    subscribe: async (topics: string[]) => {
      subscriptions.push(topics);
      return {};
    },
    sendCommand: async <T,>(command: string, body: unknown = {}, options: { agentID?: string } = {}) => {
      commands.push({ command, body, agentID: options.agentID });
      return {} as T;
    },
    onEvent: (listener: (event: HankSocketEvent) => void) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
  return { socket, commands, subscriptions, emit: (event: HankSocketEvent) => listeners.forEach((l) => l(event)) };
}

describe("AgentsClient", () => {
  it("lists agents from the plural endpoint", async () => {
    const calls: string[] = [];
    const request = async <T,>(path: string) => {
      calls.push(path);
      return { agents: [{ agent_id: "a1", status: "online" }] } as T;
    };
    const { socket } = fakeSocket();
    const client = new AgentsClient({ request }, socket);
    const agents = await client.listAgents();
    expect(calls).toEqual(["/v1/home/agents"]);
    expect(agents).toHaveLength(1);
  });

  it("targets commands at a specific agent", async () => {
    const request = async <T,>() => ({}) as T;
    const { socket, commands, subscriptions } = fakeSocket();
    const client = new AgentsClient({ request }, socket);

    await client.subscribeHealth();
    await client.lock("mac-1");
    await client.restart("mac-1");
    await client.wakeOnLAN("mac-1", "AA:BB:CC:DD:EE:FF");
    await client.runShell("mac-1", "uptime");

    expect(subscriptions).toEqual([["agents.health"]]);
    expect(commands.map((c) => [c.command, c.agentID])).toEqual([
      ["host.lock", "mac-1"],
      ["system.restart", "mac-1"],
      ["wol.send", "mac-1"],
      ["shell.exec", "mac-1"],
    ]);
    expect(commands[2].body).toEqual({ mac: "AA:BB:CC:DD:EE:FF" });
    expect(commands[3].body).toEqual({ command: "uptime", timeout_seconds: 60 });
  });

  it("decodes agents.health alerts", async () => {
    const request = async <T,>() => ({}) as T;
    const { socket, emit } = fakeSocket();
    const client = new AgentsClient({ request }, socket);
    const received: string[] = [];
    client.onAlert((alert) => received.push(`${alert.agent_id}:${alert.kind}`));

    emit({ topic: "agents.health", event: "agent.offline", body: { agent_id: "mac-1", kind: "agent.offline", severity: "warning", summary: "x" } });
    emit({ topic: "homeassistant.states", event: "x", body: {} });

    expect(received).toEqual(["mac-1:agent.offline"]);
  });
});

describe("agent helpers", () => {
  const worker: HomeAgentEntry = {
    agent_id: "mac-1",
    status: "online",
    agent_type: "worker",
    capabilities: ["files.read", "shell.exec"],
    metadata: { hostname: "studio.local" },
  };

  it("resolves display name, type, online, capabilities", () => {
    expect(agentDisplayName(worker)).toBe("studio.local");
    expect(agentDisplayName({ ...worker, name: "Studio" })).toBe("Studio");
    expect(agentIsPrimary(worker)).toBe(false);
    expect(agentIsPrimary({ ...worker, agent_type: "primary" })).toBe(true);
    expect(agentIsPrimary({ ...worker, agent_type: undefined })).toBe(true);
    expect(agentIsOnline(worker)).toBe(true);
    expect(agentHasCapability(worker, "shell.exec")).toBe(true);
    expect(agentHasCapability(worker, "docker.control")).toBe(false);
  });
});
