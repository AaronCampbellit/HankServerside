import { useEffect, useMemo, useState } from "react";
import {
  entityDomain,
  entityName,
  homeAssistantClient,
  normalizeDashboardEntityIDs,
  type HomeAssistantEntity,
  type UserProfile,
} from "../api/homeAssistant";
import type { HomeAgent } from "../api/home";

type State =
  | { status: "loading" }
  | { status: "error"; message: string }
  | {
      status: "ready";
      agent: HomeAgent | null;
      profile: UserProfile;
      dashboardEntityIDs: string[];
      states: HomeAssistantEntity[];
      query: string;
      message: string;
    };

function searchableText(entity: HomeAssistantEntity): string {
  const attrs = entity.attributes || {};
  return [
    entity.entity_id,
    entity.state,
    attrs.friendly_name,
    attrs.device_class,
    attrs.area_id,
    attrs.unit_of_measurement,
  ].filter(Boolean).join(" ").toLowerCase();
}

export function canToggle(entityID: string): boolean {
  return ["light", "switch", "fan", "input_boolean"].includes(entityDomain(entityID));
}

function numberAttribute(entity: HomeAssistantEntity, key: string): number | null {
  const value = entity.attributes?.[key];
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

function titleState(state: string): string {
  return state.replace(/_/g, " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

export function dashboardStatus(entity: HomeAssistantEntity): string {
  const domain = entityDomain(entity.entity_id);
  const state = String(entity.state || "unknown");
  const unit = typeof entity.attributes?.unit_of_measurement === "string" ? entity.attributes.unit_of_measurement : "";
  if (domain === "light") {
    const brightness = numberAttribute(entity, "brightness");
    const percent = brightness === null ? "" : ` · ${Math.round((brightness / 255) * 100)}%`;
    return state === "on" ? `On${percent}` : titleState(state);
  }
  if (domain === "climate") {
    const current = numberAttribute(entity, "current_temperature");
    return current === null ? titleState(state) : `${current}${unit || "°"} · ${state}`;
  }
  if (domain === "alarm_control_panel") {
    return titleState(state).replace("Armed Home", "Armed · home");
  }
  if (domain === "media_player") {
    return titleState(state);
  }
  if (unit) return `${state}${unit}`;
  return titleState(state);
}

export function tileTone(entity: HomeAssistantEntity): "on" | "ok" | "warn" | "muted" {
  const domain = entityDomain(entity.entity_id);
  const state = String(entity.state || "").toLowerCase();
  if (["on", "playing"].includes(state)) return "on";
  if (["locked", "closed", "clear", "armed_home"].includes(state)) return "ok";
  if (domain === "climate") return "warn";
  return "muted";
}

export function EntityIcon({ entityID }: { entityID: string }) {
  const domain = entityDomain(entityID);
  const path =
    domain === "light" ? "M9 21h6m-3-4v4M7 13a5 5 0 1 1 10 0c0 2-2 3-2.5 4h-5C9 16 7 15 7 13z" :
    domain === "climate" ? "M12 14a3 3 0 1 0 0 6 3 3 0 0 0 0-6zm0 0V4a2 2 0 0 1 4 0v10.5" :
    domain === "lock" ? "M7 11V8a5 5 0 0 1 10 0v3m-9 0h8v9H8z" :
    domain === "fan" ? "M12 12c2-4 6-5 7-2 1 2-1 4-4 3m-3-1c-4-2-5-6-2-7 2-1 4 1 3 4m-1 3c-2 4-6 5-7 2-1-2 1-4 4-3" :
    domain === "media_player" ? "M5 7h14v10H5zM9 20h6M10 11l5 3-5 3z" :
    domain === "alarm_control_panel" ? "M12 3l7 4v5c0 4-3 7-7 9-4-2-7-5-7-9V7zM9.5 12l1.6 1.6L15 10" :
    domain === "sensor" ? "M12 21a7 7 0 0 0 7-7c0-5-7-12-7-12S5 9 5 14a7 7 0 0 0 7 7z" :
    "M5 12h14";
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d={path} />
    </svg>
  );
}

function PlusIcon() {
  return (
    <svg className="ha-action-icon" viewBox="0 0 24 24" aria-hidden="true">
      <path d="M12 5v14M5 12h14" />
    </svg>
  );
}

function upsertEntity(states: HomeAssistantEntity[], next: HomeAssistantEntity): HomeAssistantEntity[] {
  const index = states.findIndex((entity) => entity.entity_id === next.entity_id);
  const nextStates = index === -1 ? [...states, next] : states.map((entity, current) => current === index ? { ...entity, ...next } : entity);
  return nextStates.sort((left, right) => entityName(left).localeCompare(entityName(right)));
}

function errorMessage(error: unknown): string {
  return error instanceof Error && error.message ? error.message : "Home Assistant could not be loaded.";
}

function initialQueryFromLocation(): string {
  return new URLSearchParams(window.location.search).get("query") || "";
}

export function EntityCard({
  entity,
  saved,
  dashboardTile,
  onToggle,
  onDashboard,
}: {
  entity: HomeAssistantEntity;
  saved: boolean;
  dashboardTile?: boolean;
  onToggle: (entity: HomeAssistantEntity) => void;
  onDashboard: (entity: HomeAssistantEntity) => void;
}) {
  const name = entityName(entity);
  const domain = entityDomain(entity.entity_id);
  const unit = entity.attributes?.unit_of_measurement;
  const serviceLabel = entity.state === "on" ? "Turn off" : "Turn on";
  const toggleable = canToggle(entity.entity_id);
  const switchedOn = entity.state === "on";
  if (dashboardTile) {
    const tone = tileTone(entity);
    return (
      <article className={`ha-dashboard-tile tone-${tone}`}>
        <div className="ha-dashboard-tile-top">
          <span className={`ha-entity-icon tone-${tone}`} aria-hidden="true">
            <EntityIcon entityID={entity.entity_id} />
          </span>
          {toggleable ? (
            <button
              className="ha-switch"
              type="button"
              role="switch"
              aria-checked={switchedOn}
              aria-label={`Toggle ${name}`}
              title={`Toggle ${name}`}
              onClick={() => onToggle(entity)}
            >
              <span aria-hidden="true" />
            </button>
          ) : null}
        </div>
        <strong>{name}</strong>
        <span className="ha-dashboard-status">{dashboardStatus(entity)}</span>
      </article>
    );
  }

  return (
    <article className="dashboard-tile ha-entity-tile">
      <div className="tile-heading">
        <div>
          <strong>{name}</strong>
          <span className="tile-meta">{entity.entity_id}</span>
        </div>
        <span className="status-pill">{entity.state}</span>
      </div>
      <div className="ha-entity-reading">
        <strong>{entity.state}{typeof unit === "string" && unit ? unit : ""}</strong>
        <span>{domain}{typeof unit === "string" && unit ? ` / ${unit}` : ""}</span>
      </div>
      <div className="row-actions">
        {toggleable && !dashboardTile ? (
          <button type="button" onClick={() => onToggle(entity)}>{serviceLabel} {name}</button>
        ) : null}
        <button className="secondary" type="button" onClick={() => onDashboard(entity)}>
          {dashboardTile ? "Remove tile" : saved ? "Saved" : "Add tile"}
        </button>
      </div>
    </article>
  );
}

function EntityTable({
  entities,
  dashboardEntityIDs,
  onToggle,
  onDashboard,
}: {
  entities: HomeAssistantEntity[];
  dashboardEntityIDs: string[];
  onToggle: (entity: HomeAssistantEntity) => void;
  onDashboard: (entity: HomeAssistantEntity) => void;
}) {
  return (
    <div className="ha-entities-table-wrap">
      <table className="ha-entities-table" aria-label="All Home Assistant entities">
        <thead>
          <tr>
            <th scope="col">Entity</th>
            <th scope="col">Domain</th>
            <th scope="col">State</th>
            <th scope="col">Tile</th>
          </tr>
        </thead>
        <tbody>
          {entities.map((entity) => {
            const name = entityName(entity);
            const domain = entityDomain(entity.entity_id);
            const saved = dashboardEntityIDs.includes(entity.entity_id);
            const toggleable = canToggle(entity.entity_id);
            const serviceLabel = entity.state === "on" ? "Turn off" : "Turn on";
            return (
              <tr key={entity.entity_id}>
                <td>
                  <strong>{name}</strong>
                  <span>{entity.entity_id}</span>
                </td>
                <td>{domain}</td>
                <td>
                  {toggleable ? (
                    <span className="ha-state-control">
                      <button
                        className="ha-switch ha-table-switch"
                        type="button"
                        role="switch"
                        aria-checked={entity.state === "on"}
                        aria-label={`${serviceLabel} ${name}`}
                        onClick={() => onToggle(entity)}
                      >
                        <span aria-hidden="true" />
                      </button>
                      <span className="ha-state-label">{entity.state}</span>
                    </span>
                  ) : (
                    <span className="status-pill">{entity.state}</span>
                  )}
                </td>
                <td>
                  <button
                    className="secondary"
                    type="button"
                    aria-label={saved ? `Remove ${name} from dashboard` : `Add ${name} to dashboard`}
                    onClick={() => onDashboard(entity)}
                  >
                    {saved ? "Saved" : "Add"}
                  </button>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

export function HomeAssistantPage() {
  const [state, setState] = useState<State>({ status: "loading" });

  useEffect(() => {
    let active = true;
    homeAssistantClient
      .load()
      .then((payload) => {
        if (!active) return;
        setState({
          status: "ready",
          agent: payload.agent,
          profile: payload.profile,
          dashboardEntityIDs: payload.dashboardEntityIDs,
          states: payload.states,
          query: initialQueryFromLocation(),
          message: "",
        });
      })
      .catch((error) => {
        if (active) setState({ status: "error", message: errorMessage(error) });
      });
    const unsubscribe = homeAssistantClient.onStateChanged((entity) => {
      if (!active) return;
      setState((current) => current.status === "ready" ? { ...current, states: upsertEntity(current.states, entity) } : current);
    });
    return () => {
      active = false;
      unsubscribe();
    };
  }, []);

  const ready = state.status === "ready" ? state : null;
  const visibleEntities = useMemo(() => {
    if (!ready) return [];
    const query = ready.query.trim().toLowerCase();
    if (!query) return ready.states.slice(0, 80);
    return ready.states.filter((entity) => searchableText(entity).includes(query)).slice(0, 80);
  }, [ready]);

  if (state.status === "loading") {
    return (
      <section className="dashboard-page home-assistant-page" aria-labelledby="route-title">
        <p className="eyebrow">Hank Remote</p>
        <h1 id="route-title">Home Assistant</h1>
        <p className="loading-state"><span className="spinner" aria-hidden="true" />Loading Home Assistant...</p>
      </section>
    );
  }

  if (state.status === "error") {
    return (
      <section className="dashboard-page home-assistant-page" aria-labelledby="route-title">
        <p className="eyebrow">Hank Remote</p>
        <h1 id="route-title">Home Assistant</h1>
        <p className="error-state">{state.message}</p>
      </section>
    );
  }

  const readyState = state;

  function setReady(next: Partial<Extract<State, { status: "ready" }>>) {
    setState((current) => current.status === "ready" ? { ...current, ...next } : current);
  }

  async function toggleDashboardEntity(entity: HomeAssistantEntity) {
    const saved = readyState.dashboardEntityIDs.includes(entity.entity_id);
    const nextIDs = saved
      ? readyState.dashboardEntityIDs.filter((entityID: string) => entityID !== entity.entity_id)
      : [...readyState.dashboardEntityIDs, entity.entity_id];
    setReady({ dashboardEntityIDs: nextIDs, message: "" });
    try {
      const profile = await homeAssistantClient.saveDashboardTiles(readyState.profile.revision, readyState.profile.settings, nextIDs);
      setReady({
        profile,
        dashboardEntityIDs: normalizeDashboardEntityIDs(profile.settings),
        message: "Dashboard tiles saved.",
      });
    } catch (error) {
      setReady({ dashboardEntityIDs: readyState.dashboardEntityIDs, message: errorMessage(error) });
    }
  }

  async function toggleEntity(entity: HomeAssistantEntity) {
    const domain = entityDomain(entity.entity_id);
    const service = entity.state === "on" ? "turn_off" : "turn_on";
    try {
      await homeAssistantClient.callService(entity.entity_id, domain, service);
      setReady({ message: `${entityName(entity)} ${service.replace("_", " ")} sent.` });
    } catch (error) {
      setReady({ message: errorMessage(error) });
    }
  }

  const dashboardEntities = state.dashboardEntityIDs
    .map((entityID) => state.states.find((entity) => entity.entity_id === entityID))
    .filter((entity): entity is HomeAssistantEntity => Boolean(entity));
  const agentOnline = state.agent?.status?.toLowerCase() === "online";

  return (
    <section className="dashboard-page home-assistant-page" aria-labelledby="route-title">
      <header className="dashboard-header">
        <div>
          <p className="eyebrow">Hank Remote</p>
          <h1 id="route-title">Home Assistant</h1>
          <p className="meta-line">{state.states.length} entities · {dashboardEntities.length} on your dashboard</p>
        </div>
        <div className="settings-actions">
          <button
            className="secondary"
            type="button"
            onClick={() => document.getElementById("ha-entity-search")?.focus()}
          >
            <PlusIcon />
            Add tile
          </button>
          <span className={`status-pill ${agentOnline ? "status-online" : "status-offline"}`}>
            {agentOnline ? "Online" : state.agent ? "Offline" : "Not Set Up"}
          </span>
        </div>
      </header>

      {state.message ? <p className="notice-state">{state.message}</p> : null}

      <section className="ha-dashboard-panel" aria-labelledby="ha-dashboard-title">
        <div className="ha-section-heading">
          <h2 id="ha-dashboard-title">Your dashboard</h2>
        </div>
        {dashboardEntities.length ? (
          <div className="ha-dashboard-grid">
            {dashboardEntities.map((entity) => (
              <EntityCard
                dashboardTile
                entity={entity}
                key={entity.entity_id}
                onDashboard={(nextEntity) => void toggleDashboardEntity(nextEntity)}
                onToggle={(nextEntity) => void toggleEntity(nextEntity)}
                saved
              />
            ))}
          </div>
        ) : (
          <p className="empty-state">Add entities from the list below.</p>
        )}
      </section>

      <section className="ha-entities-panel" aria-labelledby="ha-entities-title">
        <div className="ha-entities-toolbar">
          <h2 id="ha-entities-title">All entities</h2>
          <form>
            <label className="ha-search-field">
              <span className="visually-hidden">Search entities</span>
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <circle cx="11" cy="11" r="7" />
                <path d="m20 20-3-3" />
              </svg>
            <input
              id="ha-entity-search"
              autoComplete="off"
              placeholder="Search entities"
              type="search"
              value={state.query}
              onChange={(event) => setReady({ query: event.target.value })}
            />
            </label>
          </form>
        </div>
        {visibleEntities.length ? (
          <EntityTable
            dashboardEntityIDs={state.dashboardEntityIDs}
            entities={visibleEntities}
            onDashboard={(nextEntity) => void toggleDashboardEntity(nextEntity)}
            onToggle={(nextEntity) => void toggleEntity(nextEntity)}
          />
        ) : (
          <p className="empty-state">No matching entities.</p>
        )}
      </section>
    </section>
  );
}
