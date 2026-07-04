import { type FormEvent, useEffect, useState } from "react";
import { appsClient, type AppSettingsField, type AppSummary, type AppsPackagePreview } from "../api/apps";
import { bootstrapClient, type BootstrapState } from "../api/bootstrap";

type State =
  | { status: "loading" }
  | { status: "error"; message: string }
  | {
      status: "ready";
      bootstrap: BootstrapState;
      apps: AppSummary[];
      selectedID: string;
      configuringID: string;
      configValues: Record<string, string | boolean>;
      secretValues: Record<string, string>;
      enabled: boolean;
      userAccess: string;
      packageFile: File | null;
      preview: AppsPackagePreview | null;
      message: string;
    };

function appID(app: AppSummary): string {
  return app.app_id || app.id || "";
}

function appName(app: AppSummary): string {
  return app.name || appID(app) || "App";
}

function errorMessage(error: unknown): string {
  return error instanceof Error && error.message ? error.message : "Apps could not be loaded.";
}

function parseObject(value: unknown): Record<string, unknown> {
  if (!value) return {};
  if (typeof value === "string") {
    try {
      const parsed = JSON.parse(value || "{}");
      return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed as Record<string, unknown> : {};
    } catch {
      return {};
    }
  }
  return typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function publicConfig(app: AppSummary): Record<string, unknown> {
  return parseObject(app.public_config_json ?? app.public_config);
}

function secretFieldsSet(app: AppSummary): Record<string, boolean> {
  const raw = parseObject(app.secret_fields_set_json ?? app.secret_fields_set);
  return Object.fromEntries(Object.entries(raw).map(([key, value]) => [key, Boolean(value)]));
}

function settingsFields(app: AppSummary): AppSettingsField[] {
  const raw = parseObject(app.settings_schema_json ?? app.settings_schema);
  const fields = Array.isArray(raw.fields) ? raw.fields : [];
  return fields
    .filter((field): field is AppSettingsField => Boolean(field && typeof field === "object" && "key" in field))
    .slice()
    .sort((a, b) => Number(a.order || 0) - Number(b.order || 0) || a.key.localeCompare(b.key));
}

function fieldValue(config: Record<string, unknown>, field: AppSettingsField): string | boolean {
  const value = Object.prototype.hasOwnProperty.call(config, field.key) ? config[field.key] : field.default;
  if (field.type === "boolean") return Boolean(value);
  return value == null ? "" : String(value);
}

function configFromFields(fields: AppSettingsField[], values: Record<string, string | boolean>): Record<string, unknown> {
  const config: Record<string, unknown> = {};
  for (const field of fields) {
    if (field.secret) continue;
    const value = values[field.key];
    if (field.type === "boolean") {
      config[field.key] = Boolean(value);
    } else if (field.type === "number") {
      const trimmed = String(value || "").trim();
      if (trimmed) config[field.key] = Number(trimmed);
    } else {
      const trimmed = String(value || "").trim();
      if (trimmed) config[field.key] = trimmed;
    }
  }
  return config;
}

export function AppsSettings() {
  const [state, setState] = useState<State>({ status: "loading" });

  async function load(message = "") {
    try {
      const [bootstrap, payload] = await Promise.all([bootstrapClient.load(), appsClient.listApps()]);
      const apps = payload.apps || [];
      setState((current) => ({
        status: "ready",
        bootstrap,
        apps,
        selectedID: current.status === "ready" && apps.some((app) => appID(app) === current.selectedID)
          ? current.selectedID
          : appID(apps[0] || {}),
        configuringID: "",
        configValues: {},
        secretValues: {},
        enabled: false,
        userAccess: "admins_only",
        packageFile: null,
        preview: current.status === "ready" ? current.preview : null,
        message,
      }));
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
        <h1 id="route-title">Apps</h1>
        <p className="loading-state">Loading apps...</p>
      </section>
    );
  }

  if (state.status === "error") {
    return (
      <section className="settings-page" aria-labelledby="route-title">
        <p className="eyebrow">Hank Remote</p>
        <h1 id="route-title">Apps</h1>
        <p className="error-state">{state.message}</p>
      </section>
    );
  }

  const readyState = state;
  const canManage = readyState.bootstrap.permissions.can_manage_apps;
  const selectedApp = readyState.apps.find((app) => appID(app) === readyState.selectedID) || readyState.apps[0] || null;
  const configuringApp = readyState.apps.find((app) => appID(app) === readyState.configuringID) || null;
  const fields = configuringApp ? settingsFields(configuringApp) : [];
  const secretState = configuringApp ? secretFieldsSet(configuringApp) : {};

  function setReady(next: Partial<Extract<State, { status: "ready" }>>) {
    setState((current) => current.status === "ready" ? { ...current, ...next } : current);
  }

  function configure(app: AppSummary) {
    const config = publicConfig(app);
    const values: Record<string, string | boolean> = {};
    for (const field of settingsFields(app)) {
      if (!field.secret) values[field.key] = fieldValue(config, field);
    }
    setReady({
      configuringID: appID(app),
      configValues: values,
      secretValues: {},
      enabled: Boolean(app.enabled),
      userAccess: app.user_access === "home_members" ? "home_members" : "admins_only",
    });
  }

  async function toggle(app: AppSummary) {
    try {
      await appsClient.saveConfig(appID(app), {
        enable: !app.enabled,
        user_access: app.user_access === "home_members" ? "home_members" : "admins_only",
      });
      await load(app.enabled ? "App disabled." : "App enabled.");
    } catch (error) {
      setReady({ message: errorMessage(error) });
    }
  }

  async function saveConfig(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!configuringApp) return;
    const secrets = Object.fromEntries(
      Object.entries(readyState.secretValues)
        .map(([key, value]) => [key, value.trim()])
        .filter(([, value]) => Boolean(value)),
    );
    try {
      await appsClient.saveConfig(appID(configuringApp), {
        public_config: configFromFields(fields, readyState.configValues),
        ...(Object.keys(secrets).length ? { secrets } : {}),
        enable: readyState.enabled,
        user_access: readyState.userAccess,
      });
      await load("App saved.");
    } catch (error) {
      setReady({ message: errorMessage(error) });
    }
  }

  async function previewPackage(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!readyState.packageFile) {
      setReady({ message: "Choose a .hankapp package." });
      return;
    }
    const formData = new FormData();
    formData.set("package", readyState.packageFile, readyState.packageFile.name);
    try {
      setReady({ preview: await appsClient.previewPackage(formData), message: "Package preview ready." });
    } catch (error) {
      setReady({ message: errorMessage(error) });
    }
  }

  async function installPreview() {
    if (!readyState.preview) return;
    try {
      await appsClient.activatePackage({
        staging_id: readyState.preview.staging_id,
        package_sha256: readyState.preview.package_sha256 || "",
        enable: false,
      });
      setReady({ preview: null, packageFile: null });
      await load("App installed.");
    } catch (error) {
      setReady({ message: errorMessage(error) });
    }
  }

  return (
    <section className="settings-page" aria-labelledby="route-title">
      <header className="dashboard-header">
        <div>
          <p className="eyebrow">Hank Remote</p>
          <h1 id="route-title">Apps</h1>
          <p className="meta-line">Import, configure, and enable <code>.hankapp</code> packages.</p>
        </div>
        <span className="status-pill">{readyState.apps.length} app{readyState.apps.length === 1 ? "" : "s"}</span>
      </header>

      {readyState.message ? <p className="notice-state">{readyState.message}</p> : null}

      <section className="settings-panel" aria-label="Installed apps">
        <h2>Installed packages</h2>
        {readyState.apps.length ? (
          <>
            <label>
              <span>Installed app</span>
              <select onChange={(event) => setReady({ selectedID: event.target.value, configuringID: "" })} value={appID(selectedApp || {})}>
                {readyState.apps.map((app) => <option key={appID(app)} value={appID(app)}>{appName(app)} ({appID(app)})</option>)}
              </select>
            </label>
            {selectedApp ? (
              <article className="dashboard-tile">
                <span>{selectedApp.version || "unknown"}</span>
                <strong>{appName(selectedApp)}</strong>
                <small>{selectedApp.status || "unknown"}</small>
                <div className="button-row">
                  <button disabled={!canManage} onClick={() => configure(selectedApp)} type="button">Configure {appName(selectedApp)}</button>
                  <button disabled={!canManage} onClick={() => void toggle(selectedApp)} type="button">
                    {selectedApp.enabled ? "Disable" : "Enable"} {appName(selectedApp)}
                  </button>
                </div>
              </article>
            ) : null}
          </>
        ) : <p className="empty-state">No apps installed.</p>}
      </section>

      <section className="settings-panel" aria-label="Install app">
        <h2>Install app</h2>
        <form className="quick-link-form" onSubmit={previewPackage}>
          <label>
            <span>Package</span>
            <input
              accept=".hankapp,application/zip,application/octet-stream"
              disabled={!canManage}
              onChange={(event) => setReady({ packageFile: event.target.files?.[0] || null })}
              type="file"
            />
          </label>
          <button disabled={!canManage} type="submit">Preview package</button>
        </form>
        {readyState.preview ? (
          <article className="dashboard-tile">
            <span>{readyState.preview.replacing ? "Replace" : "New"}</span>
            <strong>{appName(readyState.preview.app)}</strong>
            <small>{appID(readyState.preview.app)} {readyState.preview.app.version || ""}</small>
            <button disabled={!canManage} onClick={() => void installPreview()} type="button">Install preview</button>
          </article>
        ) : null}
      </section>

      {configuringApp ? (
        <section className="settings-panel" aria-label="App configuration">
          <h2>{appName(configuringApp)} configuration from app.json</h2>
          <div className="kv-list">
            <div><strong>App access</strong><span>{readyState.userAccess === "home_members" ? "Home members" : "Admins only"}</span></div>
            <div><strong>Public config JSON</strong><span>{JSON.stringify(publicConfig(configuringApp))}</span></div>
          </div>
          <form className="quick-link-form" onSubmit={saveConfig}>
            <label className="checkbox-field">
              <input checked={readyState.enabled} disabled={!canManage} onChange={(event) => setReady({ enabled: event.target.checked })} type="checkbox" />
              <span>Enabled</span>
            </label>
            <label>
              <span>App access</span>
              <select aria-label="User access" disabled={!canManage} onChange={(event) => setReady({ userAccess: event.target.value })} value={readyState.userAccess}>
                <option value="admins_only">Admins only</option>
                <option value="home_members">Home members</option>
              </select>
            </label>
            {fields.map((field) => {
              const label = field.label || field.key;
              if (field.secret) {
                const secretKey = field.secret_key || field.key;
                return (
                  <label key={field.key}>
                    <span>{label} · secret{secretState[secretKey] ? " (set)" : ""}</span>
                    <input
                      aria-label={label}
                      disabled={!canManage}
                      onChange={(event) => setReady({ secretValues: { ...readyState.secretValues, [secretKey]: event.target.value } })}
                      type="password"
                      value={readyState.secretValues[secretKey] || ""}
                    />
                  </label>
                );
              }
              if (field.type === "boolean") {
                return (
                  <label className="checkbox-field" key={field.key}>
                    <input
                      checked={Boolean(readyState.configValues[field.key])}
                      disabled={!canManage}
                      onChange={(event) => setReady({ configValues: { ...readyState.configValues, [field.key]: event.target.checked } })}
                      type="checkbox"
                    />
                    <span>{label}</span>
                  </label>
                );
              }
              return (
                <label key={field.key}>
                  <span>{label}</span>
                  <input
                    disabled={!canManage}
                    onChange={(event) => setReady({ configValues: { ...readyState.configValues, [field.key]: event.target.value } })}
                    type={field.type === "number" || field.type === "url" ? field.type : "text"}
                    value={String(readyState.configValues[field.key] || "")}
                  />
                </label>
              );
            })}
            <button aria-label="Save app configuration" disabled={!canManage} type="submit">Save app</button>
          </form>
        </section>
      ) : null}
    </section>
  );
}
