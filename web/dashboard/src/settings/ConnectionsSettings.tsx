import { type FormEvent, useEffect, useState } from "react";
import { bootstrapClient, type BootstrapState } from "../api/bootstrap";
import { connectionsClient, type ServiceProfile } from "../api/connections";

type SMBShare = {
  id: string;
  name: string;
  host: string;
  share: string;
  domain: string;
  username: string;
  password_set?: boolean;
};

type HostFolder = {
  id: string;
  name: string;
  root: string;
  create: boolean;
};

type State =
  | { status: "loading" }
  | { status: "error"; message: string }
  | {
      status: "ready";
      bootstrap: BootstrapState;
      profiles: ServiceProfile[];
      ha: { baseURL: string; timeoutSeconds: string; token: string; persist: boolean };
      smb: SMBShare & { password: string; persist: boolean };
      smbSources: Array<Record<string, unknown>>;
      folders: HostFolder[];
      message: string;
    };

function parseConfig(profile: ServiceProfile | undefined): Record<string, unknown> {
  if (!profile?.public_config_json) return {};
  try {
    const parsed = JSON.parse(profile.public_config_json);
    return parsed && typeof parsed === "object" ? parsed as Record<string, unknown> : {};
  } catch {
    return {};
  }
}

function profileByType(profiles: ServiceProfile[], type: string): ServiceProfile | undefined {
  return profiles.find((profile) => profile.service_type === type);
}

function firstString(...values: unknown[]): string {
  for (const value of values) {
    if (typeof value === "string" && value.trim()) return value;
  }
  return "";
}

function cleanID(value: string): string {
  return value.trim().toLowerCase().replace(/[^a-z0-9_.-]+/g, "-").replace(/^-+|-+$/g, "");
}

function normalizeHost(value: string): string {
  let host = value.trim();
  host = host.replace(/^[a-z][a-z0-9+.-]*:\/\//i, "");
  host = host.replace(/^\\\\+/, "");
  host = host.replace(/^\/+/, "");
  return (host.split(/[/?#]/)[0] || host).trim();
}

function smbSourceRecords(profile: ServiceProfile | undefined): Array<Record<string, unknown>> {
  const config = parseConfig(profile);
  for (const candidate of [config.shares, config.file_sources, config.sources]) {
    if (!Array.isArray(candidate)) continue;
    const sources = candidate.flatMap((entry) => {
      if (!entry || typeof entry !== "object") return [];
      const record = entry as Record<string, unknown>;
      if (record.type && record.type !== "smb") return [];
      return [{ ...record }];
    });
    if (sources.length > 0) return sources;
  }
  return firstString(config.host, config.smb_host, config.share, config.smb_share) ? [{ ...config }] : [];
}

function firstSMBShare(profile: ServiceProfile | undefined): SMBShare {
  const config = parseConfig(profile);
  const first = smbSourceRecords(profile)[0];
  const fallback = first || config;
  const share = firstString(fallback.share, fallback.smb_share);
  const id = cleanID(firstString(fallback.id, fallback.source_id, fallback.name, share, "smb")) || "smb";
  return {
    id,
    name: firstString(fallback.name, share, "SMB Share"),
    host: firstString(fallback.host, fallback.smb_host),
    share,
    domain: firstString(fallback.domain, fallback.smb_domain),
    username: firstString(fallback.username, fallback.smb_username),
    password_set: Boolean(fallback.password_set || fallback.smb_password_set),
  };
}

function hostFolders(profile: ServiceProfile | undefined): HostFolder[] {
  const config = parseConfig(profile);
  const folders = Array.isArray(config.folders) ? config.folders : [];
  const parsed = folders.flatMap((entry, index) => {
    if (!entry || typeof entry !== "object") return [];
    const record = entry as Record<string, unknown>;
    const root = firstString(record.root, record.path);
    if (!root) return [];
    const id = cleanID(firstString(record.id, record.source_id, record.name, `local-${index + 1}`)) || `local-${index + 1}`;
    return [{
      id,
      name: firstString(record.name, id),
      root,
      create: false,
    }];
  });
  if (parsed.length > 0) return parsed;

  const legacyRoot = firstString(config.root);
  return legacyRoot ? [{ id: "local", name: "Home connector files", root: legacyRoot, create: false }] : [];
}

function publicConfigForSMB(share: SMBShare, existingSources: Array<Record<string, unknown>>): Record<string, unknown> {
  const publicShare = {
    ...(existingSources[0] || {}),
    id: share.id,
    name: share.name,
    host: share.host,
    share: share.share,
    domain: share.domain,
    username: share.username,
  };
  const hasShare = Boolean(share.host && share.share);
  return {
    active_source_id: hasShare ? share.id : "",
    host: share.host,
    share: share.share,
    domain: share.domain,
    username: share.username,
    shares: hasShare ? [publicShare, ...existingSources.slice(1)] : [],
  };
}

function publicConfigForFolders(folders: HostFolder[]): Record<string, unknown> {
  const publicFolders = folders.flatMap((folder, index) => {
    const root = folder.root.trim();
    if (!root) return [];
    const id = cleanID(folder.id || folder.name || `local-${index + 1}`) || `local-${index + 1}`;
    return [{
      id,
      name: folder.name.trim() || id,
      root,
      create: folder.create,
    }];
  });
  return { folders: publicFolders };
}

function errorMessage(error: unknown): string {
  return error instanceof Error && error.message ? error.message : "Connections could not be loaded.";
}

export function ConnectionsSettings() {
  const [state, setState] = useState<State>({ status: "loading" });

  async function load(message = "") {
    try {
      const [bootstrap, payload] = await Promise.all([bootstrapClient.load(), connectionsClient.listProfiles()]);
      const profiles = payload.profiles || [];
      const haConfig = parseConfig(profileByType(profiles, "homeassistant"));
      const smbProfile = profileByType(profiles, "smb");
      const smbShare = firstSMBShare(smbProfile);
      setState({
        status: "ready",
        bootstrap,
        profiles,
        ha: {
          baseURL: firstString(haConfig.base_url, haConfig.url),
          timeoutSeconds: String(Number(haConfig.timeout_seconds || 10) || 10),
          token: "",
          persist: true,
        },
        smb: { ...smbShare, password: "", persist: true },
        smbSources: smbSourceRecords(smbProfile),
        folders: hostFolders(smbProfile),
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
        <h1 id="route-title">Connections</h1>
        <p className="loading-state">Loading connections...</p>
      </section>
    );
  }

  if (state.status === "error") {
    return (
      <section className="settings-page" aria-labelledby="route-title">
        <p className="eyebrow">Hank Remote</p>
        <h1 id="route-title">Connections</h1>
        <p className="error-state">{state.message}</p>
      </section>
    );
  }

  const readyState = state;
  const canManage = readyState.bootstrap.permissions.can_manage_settings;

  function setReady(next: Partial<Extract<State, { status: "ready" }>>) {
    setState((current) => current.status === "ready" ? { ...current, ...next } : current);
  }

  async function saveHA(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      const input = {
        public_config: {
          base_url: readyState.ha.baseURL.trim().replace(/\/+$/, ""),
          timeout_seconds: Number.parseInt(readyState.ha.timeoutSeconds, 10) || 10,
        },
        persist: readyState.ha.persist,
        ...(readyState.ha.token.trim() ? { secrets: { token: readyState.ha.token.trim() } } : {}),
      };
      await connectionsClient.saveProfile("homeassistant", input);
      await load("Home Assistant settings saved.");
    } catch (error) {
      setReady({ message: errorMessage(error) });
    }
  }

  async function saveSMB(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      const host = normalizeHost(readyState.smb.host);
      const shareID = cleanID(readyState.smb.id || readyState.smb.name || readyState.smb.share || host || "smb") || "smb";
      const share: SMBShare = {
        id: shareID,
        name: readyState.smb.name.trim() || readyState.smb.share.trim() || shareID,
        host,
        share: readyState.smb.share.trim(),
        domain: readyState.smb.domain.trim(),
        username: readyState.smb.username.trim(),
      };
      const input = {
        public_config: publicConfigForSMB(share, readyState.smbSources),
        persist: readyState.smb.persist,
        ...(readyState.smb.password ? { secrets: { shares: [{ id: share.id, password: readyState.smb.password }] } } : {}),
      };
      await connectionsClient.saveProfile("smb", input);
      await load("File server share saved.");
    } catch (error) {
      setReady({ message: errorMessage(error) });
    }
  }

  function updateFolder(index: number, next: Partial<HostFolder>) {
    setReady({ folders: readyState.folders.map((folder, folderIndex) => folderIndex === index ? { ...folder, ...next } : folder) });
  }

  function addFolder() {
    const index = readyState.folders.length + 1;
    setReady({ folders: [...readyState.folders, { id: `local-${index}`, name: "", root: "", create: false }] });
  }

  function removeFolder(index: number) {
    setReady({ folders: readyState.folders.filter((_, folderIndex) => folderIndex !== index) });
  }

  async function saveFolders(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      await connectionsClient.saveProfile("smb", {
        public_config: publicConfigForFolders(readyState.folders),
        persist: readyState.smb.persist,
      });
      await load("Host folders saved.");
    } catch (error) {
      setReady({ message: errorMessage(error) });
    }
  }

  return (
    <section className="settings-page" aria-labelledby="route-title">
      <header className="dashboard-header">
        <div>
          <p className="eyebrow">Hank Remote</p>
          <h1 id="route-title">Connections</h1>
          <p className="meta-line">Credentials stay on the home agent - the cloud never sees them.</p>
        </div>
        <span className="status-pill">{canManage ? "Admin" : "View Only"}</span>
      </header>

      {readyState.message ? <p className="notice-state">{readyState.message}</p> : null}

      <section className="settings-panel" aria-label="Connection status">
        <h2>Connection Status</h2>
        <div className="dashboard-grid">
          {readyState.profiles.length > 0 ? readyState.profiles.map((profile) => (
            <article className="dashboard-tile" key={profile.service_type}>
              <span>{profile.service_type}</span>
              <strong>{profile.status || "unknown"}</strong>
              <small>Version {profile.applied_version}</small>
            </article>
          )) : (
            <p className="empty-state">No connections saved for this home yet.</p>
          )}
        </div>
      </section>

      <section className="settings-panel" aria-label="Home Assistant">
        <div className="panel-heading">
          <h2>Home Assistant</h2>
          <span className="status-pill status-online">Connected</span>
        </div>
        <form className="quick-link-form" onSubmit={saveHA}>
          <label>
            <span>Base URL</span>
            <input
              aria-label="Home Assistant address"
              disabled={!canManage}
              onChange={(event) => setReady({ ha: { ...readyState.ha, baseURL: event.target.value } })}
              type="url"
              value={readyState.ha.baseURL}
            />
          </label>
          <label>
            <span>Request timeout</span>
            <input
              disabled={!canManage}
              min={1}
              onChange={(event) => setReady({ ha: { ...readyState.ha, timeoutSeconds: event.target.value } })}
              type="number"
              value={readyState.ha.timeoutSeconds}
            />
          </label>
          <label>
            <span>Long-lived access token</span>
            <input
              aria-label="Home Assistant token"
              disabled={!canManage}
              onChange={(event) => setReady({ ha: { ...readyState.ha, token: event.target.value } })}
              type="password"
              value={readyState.ha.token}
            />
          </label>
          <label className="checkbox-field">
            <input
              checked={readyState.ha.persist}
              disabled={!canManage}
              onChange={(event) => setReady({ ha: { ...readyState.ha, persist: event.target.checked } })}
              type="checkbox"
            />
            <span>Save on home connector</span>
          </label>
          <div className="button-row">
            <button disabled={!canManage} type="submit">Save Home Assistant</button>
            <button className="secondary" disabled={!canManage} type="button">Test</button>
          </div>
        </form>
      </section>

      <section className="settings-panel" aria-label="File Server">
        <div className="panel-heading">
          <h2>SMB Shares</h2>
          <button className="secondary" disabled={!canManage} type="button">Add share</button>
        </div>
        {readyState.smb.name ? <p className="notice-state">{readyState.smb.name}</p> : null}
        <form className="quick-link-form" onSubmit={saveSMB}>
          <label>
            <span>Share label</span>
            <input
              disabled={!canManage}
              onChange={(event) => setReady({ smb: { ...readyState.smb, name: event.target.value } })}
              type="text"
              value={readyState.smb.name}
            />
          </label>
          <label>
            <span>Server address</span>
            <input
              disabled={!canManage}
              onChange={(event) => setReady({ smb: { ...readyState.smb, host: event.target.value } })}
              type="text"
              value={readyState.smb.host}
            />
          </label>
          <label>
            <span>Share name</span>
            <input
              disabled={!canManage}
              onChange={(event) => setReady({ smb: { ...readyState.smb, share: event.target.value } })}
              type="text"
              value={readyState.smb.share}
            />
          </label>
          <label>
            <span>Domain</span>
            <input
              disabled={!canManage}
              onChange={(event) => setReady({ smb: { ...readyState.smb, domain: event.target.value } })}
              type="text"
              value={readyState.smb.domain}
            />
          </label>
          <label>
            <span>Username</span>
            <input
              disabled={!canManage}
              onChange={(event) => setReady({ smb: { ...readyState.smb, username: event.target.value } })}
              type="text"
              value={readyState.smb.username}
            />
          </label>
          <label>
            <span>SMB password</span>
            <input
              disabled={!canManage}
              onChange={(event) => setReady({ smb: { ...readyState.smb, password: event.target.value } })}
              type="password"
              value={readyState.smb.password}
            />
          </label>
          <label className="checkbox-field">
            <input
              checked={readyState.smb.persist}
              disabled={!canManage}
              onChange={(event) => setReady({ smb: { ...readyState.smb, persist: event.target.checked } })}
              type="checkbox"
            />
            <span>Save on home connector</span>
          </label>
          <button disabled={!canManage} type="submit">Save File Server</button>
        </form>
      </section>

      <section className="settings-panel" aria-label="Host Folders">
        <div className="panel-heading">
          <div>
            <h2>Host Folders</h2>
            <p className="meta-line">Expose selected directories from the home connector as File Server sources.</p>
          </div>
          <button className="secondary" disabled={!canManage} onClick={addFolder} type="button">Add folder</button>
        </div>
        <form className="quick-link-form" onSubmit={saveFolders}>
          {readyState.folders.length ? readyState.folders.map((folder, index) => (
            <fieldset className="quick-link-form" key={`${folder.id}-${index}`}>
              <label>
                <span>Folder label</span>
                <input
                  aria-label="Host folder label"
                  disabled={!canManage}
                  onChange={(event) => updateFolder(index, { name: event.target.value })}
                  type="text"
                  value={folder.name}
                />
              </label>
              <label>
                <span>Absolute path</span>
                <input
                  aria-label="Host folder path"
                  disabled={!canManage}
                  onChange={(event) => updateFolder(index, { root: event.target.value })}
                  placeholder="/srv/media"
                  type="text"
                  value={folder.root}
                />
              </label>
              <label className="checkbox-field">
                <input
                  aria-label="Create folder if it does not exist"
                  checked={folder.create}
                  disabled={!canManage}
                  onChange={(event) => updateFolder(index, { create: event.target.checked })}
                  type="checkbox"
                />
                <span>Create folder if it does not exist</span>
              </label>
              <button
                className="secondary"
                disabled={!canManage}
                onClick={() => removeFolder(index)}
                type="button"
              >Remove {folder.name || `folder ${index + 1}`}</button>
            </fieldset>
          )) : <p className="empty-state">No host folders configured.</p>}
          <button disabled={!canManage} type="submit">Save Host Folders</button>
        </form>
      </section>
    </section>
  );
}
