import { useState } from "react";
export interface DeviceApprovalDetails { home: string; subject: string; platform: string; fingerprint: string; capabilities: string[]; expiry: string; signer: string; changed?: boolean }
export function DesktopDeviceApproval({ details, onApprove }: { details: DeviceApprovalDetails; onApprove(): void }) {
  const [confirmation,setConfirmation]=useState("");
  return <section className="settings-panel" aria-label="Desktop device approval">
    <h3>{details.changed ? "Identity changed — blocked" : "Approve Remote Desktop identity"}</h3>
    <dl><dt>Home</dt><dd>{details.home}</dd><dt>Device or agent</dt><dd>{details.subject}</dd><dt>Platform</dt><dd>{details.platform}</dd><dt>Fingerprint</dt><dd><code>{details.fingerprint}</code></dd><dt>Capabilities</dt><dd>{details.capabilities.join(", ")}</dd><dt>Expires</dt><dd>{details.expiry}</dd><dt>Signer</dt><dd>{details.signer}</dd></dl>
    {details.changed&&<label>Type the changed fingerprint to replace the blocked identity<input aria-label="Confirm changed fingerprint" value={confirmation} onChange={event=>setConfirmation(event.target.value)}/></label>}
    <button type="button" disabled={details.changed&&confirmation!==details.fingerprint} onClick={onApprove}>Compare fingerprint and approve</button>
  </section>;
}
