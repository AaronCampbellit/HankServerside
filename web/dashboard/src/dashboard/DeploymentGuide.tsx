const serverSettings = [
  [".env.cloud", "Private server file for database, cloud, backup, and secret settings."],
  ["Address", "Default server address is port 18080."],
  ["Database", "Postgres stays private inside Docker."],
  ["Backups", "Encrypted backup settings live in .env.cloud."],
  ["Login Lifetime", "How long app sessions stay signed in."],
  ["Request Timeout", "How long the server waits for the home connector."],
];

const connectorSettings = [
  ["Server Link", "The connector uses this to reach Hank Remote."],
  ["Connector ID", "The name used to recognize this connector."],
  ["Setup Token", "The private token created by the dashboard."],
  ["Home Name", "The friendly name shown in the dashboard."],
];

const firstChecks = [
  "Server running returns OK.",
  "Database ready returns OK.",
  "First account becomes admin.",
  "Later registration is blocked.",
  "The home connector shows online.",
  "Storage backup and restore test pass.",
  "The Hank app can log in.",
  "Files, notes, or Home Assistant load from the app.",
];

export function DeploymentGuide() {
  return (
    <section className="dashboard-page deployment-page" aria-labelledby="route-title">
      <header className="dashboard-header">
        <div>
          <p className="eyebrow">Hank Remote</p>
          <h1 id="route-title">Setup Guide</h1>
        </div>
        <span className="status-pill status-online">Live Server</span>
      </header>

      <section className="settings-panel" aria-labelledby="deployment-picture-title">
        <h2 id="deployment-picture-title">Simple Picture</h2>
        <p className="notice-state">Hank app -&gt; Hank Remote server -&gt; Home connector</p>
        <p className="empty-state">The app talks to the server. The home connector keeps local Home Assistant, files, and notes reachable.</p>
      </section>

      <section className="settings-panel" aria-labelledby="deployment-bootstrap-title">
        <h2 id="deployment-bootstrap-title">Recommended Bootstrap</h2>
        <div className="dashboard-grid">
          <article className="dashboard-tile">
            <strong>scripts/bootstrap-first-run.sh</strong>
            <span>Creates `.env.cloud`, starts Postgres, runs migrations, starts cloud/db-ops, and checks health.</span>
          </article>
          <article className="dashboard-tile">
            <strong>scripts/doctor.sh</strong>
            <span>Run after bootstrap and later updates to check the deployment.</span>
          </article>
        </div>
      </section>

      <section className="settings-panel" aria-labelledby="deployment-server-title">
        <h2 id="deployment-server-title">Server Settings</h2>
        <div className="dashboard-grid">
          {serverSettings.map(([label, description]) => (
            <article className="dashboard-tile" key={label}>
              <strong>{label}</strong>
              <span>{description}</span>
            </article>
          ))}
        </div>
      </section>

      <section className="settings-panel" aria-labelledby="deployment-connector-title">
        <h2 id="deployment-connector-title">Home Connector Settings</h2>
        <div className="dashboard-grid">
          {connectorSettings.map(([label, description]) => (
            <article className="dashboard-tile" key={label}>
              <strong>{label}</strong>
              <span>{description}</span>
            </article>
          ))}
        </div>
      </section>

      <section className="settings-panel" aria-labelledby="deployment-checks-title">
        <h2 id="deployment-checks-title">First Checks</h2>
        <ol className="settings-list">
          {firstChecks.map((check) => (
            <li className="dashboard-tile" key={check}>{check}</li>
          ))}
        </ol>
      </section>
    </section>
  );
}
