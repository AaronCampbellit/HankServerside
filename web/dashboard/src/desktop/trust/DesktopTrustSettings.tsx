import { useEffect, useState } from "react";
import type { DesktopTrustSnapshot } from "../../api/desktop";
import { DesktopTrustClient, desktopTrustClient } from "../../api/desktopTrust";
import { IndexedDBDesktopIdentityStore } from "../identityStore";
import { decodeBase64URL, encodeBase64URL } from "../base64url";
import { DesktopDeviceApproval } from "./DesktopDeviceApproval";
import { createDesktopRecoveryOperator, createDesktopTrustBootstrap, createDesktopTrustReset, createDesktopTrustRotation, decryptRecoveryRoot, reviewDesktopApprovalRequest, signDesktopApproval, signDesktopRecoveryEnrollment, type ReviewedDesktopApproval } from "./trustCeremony";

export function DesktopTrustSettings({ client = desktopTrustClient, homeID = "", userID = "" }: { client?: DesktopTrustClient; homeID?: string; userID?: string }) {
  const [snapshot, setSnapshot] = useState<DesktopTrustSnapshot | null>(null), [message, setMessage] = useState(""), [code, setCode] = useState(""), [typedCode, setTypedCode] = useState(""), [offlineCode, setOfflineCode] = useState(""), [resetText, setResetText] = useState("");
  const [approvalText,setApprovalText]=useState(""), [approval,setApproval]=useState<ReviewedDesktopApproval|null>(null);
  const reload = () => client.snapshot().then(setSnapshot).catch(error => setMessage(error instanceof Error ? error.message : "Desktop trust unavailable"));
  useEffect(() => { void reload(); }, []);
  async function bootstrap() {
    const store = new IndexedDBDesktopIdentityStore(); let ceremony: Awaited<ReturnType<typeof createDesktopTrustBootstrap>> | undefined, committed = false;
    try {
      const deviceID = `browser-${crypto.randomUUID()}`; ceremony = await createDesktopTrustBootstrap({ homeID, userID, deviceID, store });
      await client.bootstrap(ceremony.body); committed = true; localStorage.setItem("hank-desktop-device-id", deviceID); setCode(ceremony.recoveryCode); setMessage("Save this recovery code now. It will not be shown again."); await reload();
    } catch (error) { if (!committed) await ceremony?.cleanup(); setMessage(error instanceof Error ? error.message : "Desktop trust setup failed"); }
  }
  async function revoke(identity: DesktopTrustSnapshot["identities"][number]) {
    if (identity.identity_type === "endpoint") await client.revokeEndpoint(identity.agent_id);
    else { await client.revokeOperator(identity.device_id); await new IndexedDBDesktopIdentityStore().remove(identity.device_id); }
    await reload();
  }
  async function rotate() {
    const store=new IndexedDBDesktopIdentityStore(); let ceremony: Awaited<ReturnType<typeof createDesktopTrustRotation>>|undefined, committed=false;
    try { const root = snapshot?.root; if (!root?.recovery_envelope) throw new Error("desktop_recovery_envelope_unavailable"); const oldKey = await decryptRecoveryRoot(offlineCode, homeID, root.generation, root.recovery_envelope), deviceID=`browser-${crypto.randomUUID()}`; ceremony = await createDesktopTrustRotation({homeID,userID,deviceID,store}, root.generation, oldKey); await client.rotate(ceremony.body); committed=true; await store.removeMany([`root:${homeID}:${root.generation}`, ...(snapshot?.identities.filter(value=>value.identity_type==="operator_device" && value.device_id!==deviceID).map(value=>value.device_id) ?? [])]); localStorage.setItem("hank-desktop-device-id", deviceID); setOfflineCode(""); setCode(ceremony.recoveryCode); setMessage("Trust rotated. Save the new recovery code; endpoints must re-enroll."); await reload(); } catch(error) { if(!committed) await ceremony?.cleanup(); setMessage(error instanceof Error ? error.message : "Rotation failed"); }
  }
  async function recover() {
    const store=new IndexedDBDesktopIdentityStore(); let recovered: Awaited<ReturnType<typeof createDesktopRecoveryOperator>>|undefined, committed=false;
    try { const root=snapshot?.root; if(!root?.recovery_envelope) throw new Error("desktop_recovery_envelope_unavailable"); const key=await decryptRecoveryRoot(offlineCode,homeID,root.generation,root.recovery_envelope), deviceID=`browser-${crypto.randomUUID()}`; recovered=await createDesktopRecoveryOperator({homeID,userID,deviceID,store},root.generation,key); const challenge=await client.recoveryChallenge(root.generation,recovered.operator), signature=await signDesktopRecoveryEnrollment(key,homeID,root.generation,recovered,deviceID,decodeBase64URL(challenge.challenge)); await client.recover({generation:root.generation,operator:recovered.operator,challenge:challenge.challenge,root_signature:signature}); committed=true; localStorage.setItem("hank-desktop-device-id",deviceID); setOfflineCode(""); setMessage("New non-exportable operator identity enrolled through offline recovery."); await reload(); } catch(error) { if(!committed) await recovered?.cleanup(); setMessage(error instanceof Error ? error.message : "Recovery failed"); }
  }
  async function reset() {
    const store=new IndexedDBDesktopIdentityStore(); let ceremony: Awaited<ReturnType<typeof createDesktopTrustReset>>|undefined, committed=false;
    try { const oldGeneration=snapshot?.root?.generation ?? 0, generation=oldGeneration+1, deviceID=`browser-${crypto.randomUUID()}`; ceremony=await createDesktopTrustReset({homeID,userID,deviceID,store},generation); await client.reset(ceremony.body); committed=true; await store.removeMany([`root:${homeID}:${oldGeneration}`, ...(snapshot?.identities.filter(value=>value.identity_type==="operator_device" && value.device_id!==deviceID).map(value=>value.device_id) ?? [])]); localStorage.setItem("hank-desktop-device-id", deviceID); setResetText(""); setCode(ceremony.recoveryCode); setMessage("Desktop trust reset. All old identities and live sessions were revoked; save the new recovery code."); await reload(); } catch(error) { if(!committed) await ceremony?.cleanup(); setMessage(error instanceof Error ? error.message : "Reset failed"); }
  }
  async function createOperatorApprovalRequest() {
    try {
      let deviceID=localStorage.getItem("hank-desktop-device-id")||""; if(!deviceID){deviceID=`browser-${crypto.randomUUID()}`;localStorage.setItem("hank-desktop-device-id",deviceID)}
      const store=new IndexedDBDesktopIdentityStore(), existingSPKI=await store.getPublicSPKI(deviceID), spki=existingSPKI??(await store.create(deviceID)).spki, now=new Date(), expires=new Date(now.getTime()+2*365*24*60*60*1000);
      const request={identity_type:"operator_device",identity_id:`dop_${crypto.randomUUID().replaceAll("-","")}`,device_id:deviceID,public_key_spki:encodeBase64URL(spki),capabilities:["operator.approve","endpoint.approve","trust.recover","trust.rotate"],created_at:now.toISOString(),expires_at:expires.toISOString(),platform:"browser"};
      setApprovalText(JSON.stringify(request)); setApproval(null); setMessage("Copy this request to an already approved administrator browser for fingerprint review and approval.");
    } catch(error){setMessage(error instanceof Error?error.message:"Approval request failed")}
  }
  async function reviewApproval() { try { setApproval(await reviewDesktopApprovalRequest(JSON.parse(approvalText))); setMessage("Compare the fingerprint out of band before approval."); } catch(error){setApproval(null);setMessage(error instanceof Error?error.message:"Approval request invalid")} }
  async function approveReviewed() {
    if(!approval||!snapshot?.root) return;
    try {
      const deviceID=localStorage.getItem("hank-desktop-device-id")||"", key=await new IndexedDBDesktopIdentityStore().get(deviceID), signer=snapshot.identities.find(value=>value.identity_type==="operator_device"&&value.device_id===deviceID&&!value.revoked_at);
      if(!key||!signer) throw new Error("desktop_approved_signer_unavailable");
      const body=await signDesktopApproval(approval,key.privateKey,homeID,userID,snapshot.root.generation);
      const replacementConfirmation=changed?{confirmation:"replace changed desktop identity"}:{};
      if(approval.request.identity_type==="endpoint") await client.approveEndpoint(approval.request.agent_id!,{...body,...replacementConfirmation}); else await client.approveOperator({...body,...replacementConfirmation});
      setApproval(null); setApprovalText(""); setMessage("Remote Desktop identity approved."); await reload();
    } catch(error){setMessage(error instanceof Error?error.message:"Desktop identity approval failed")}
  }
  const approvalSubject=approval?.request.device_id||approval?.request.agent_id||"", changed=Boolean(approval&&snapshot?.identities.some(value=>!value.revoked_at&&value.identity_type===approval.request.identity_type&&(value.device_id===approvalSubject||value.agent_id===approvalSubject)&&value.fingerprint!==approval.fingerprint));
  const currentDeviceID=localStorage.getItem("hank-desktop-device-id")||"", currentDeviceApproved=Boolean(snapshot?.identities.some(value=>value.identity_type==="operator_device"&&value.device_id===currentDeviceID&&!value.revoked_at));
  return <section className="settings-panel" aria-label="Remote Desktop trust">
    <div className="panel-heading"><h2>Remote Desktop trust</h2><span className="status-pill">{snapshot?.configured ? `Generation ${snapshot.root?.generation}` : "Not configured"}</span></div>
    <p>Password reset does not recover or replace Remote Desktop cryptographic trust.</p>{message && <p className="notice-state">{message}</p>}
    {!snapshot?.configured && <button type="button" disabled={!homeID || !userID} onClick={() => void bootstrap()}>Create desktop trust</button>}
    {code && <section aria-label="One-time recovery code"><h3>One-time recovery code</h3><code>{code}</code><div className="button-row"><button type="button" onClick={() => void navigator.clipboard.writeText(code)}>Copy</button><button type="button" onClick={() => window.print()}>Print</button></div><label>Type the full code to confirm<input aria-label="Confirm recovery code" value={typedCode} onChange={event=>setTypedCode(event.target.value)}/></label><button type="button" disabled={typedCode !== code} onClick={()=>{setCode("");setTypedCode("");setMessage("Recovery code confirmed and cleared from this page.");}}>I saved the code</button></section>}
    {snapshot?.root && <p>Root fingerprint: <code>{snapshot.root.fingerprint}</code></p>}
    <div className="card-list">{snapshot?.identities.map(identity => <article className="dashboard-tile" key={identity.identity_id}><span>{identity.identity_type}</span><strong>{identity.device_id || identity.agent_id}</strong><code>{identity.fingerprint}</code><small>{identity.capabilities.join(", ")}</small>{!identity.revoked_at && <button type="button" onClick={()=>void revoke(identity)}>Revoke identity</button>}</article>)}</div>
    {snapshot?.configured&&<section aria-label="Approve Remote Desktop identity"><h3>Approve a device or endpoint</h3><p>Transfer the metadata-only enrollment request to an approved administrator browser, compare its fingerprint out of band, then sign it locally.</p><button type="button" disabled={currentDeviceApproved} onClick={()=>void createOperatorApprovalRequest()}>Create this browser approval request</button><label>Approval request<textarea aria-label="Desktop approval request" value={approvalText} onChange={event=>{setApprovalText(event.target.value);setApproval(null)}}/></label><button type="button" disabled={!approvalText} onClick={()=>void reviewApproval()}>Review approval request</button>{approval&&<DesktopDeviceApproval details={{home:homeID,subject:approvalSubject,platform:approval.request.platform||"unknown",fingerprint:approval.fingerprint,capabilities:approval.request.capabilities,expiry:approval.request.expires_at,signer:currentDeviceID||"approved administrator",changed}} onApprove={()=>void approveReviewed()}/>}</section>}
    {snapshot?.configured && <section aria-label="Rotate or reset trust"><h3>Recovery, rotation, and reset</h3><p>Recovery and rotation require the offline recovery code and complete locally; the code and root key are never sent as plaintext.</p><label>Offline recovery code<input aria-label="Offline recovery code" type="password" autoComplete="off" value={offlineCode} onChange={event=>setOfflineCode(event.target.value)}/></label><div className="button-row"><button type="button" disabled={!offlineCode} onClick={()=>void recover()}>Enroll recovered operator</button><button type="button" disabled={!offlineCode} onClick={()=>void rotate()}>Rotate root and recovery code</button></div><p>Reset revokes every operator, endpoint, and live session. Every endpoint must re-enroll.</p><label>Reset confirmation<input aria-label="Reset confirmation" value={resetText} onChange={event=>setResetText(event.target.value)}/></label><button type="button" disabled={resetText !== "reset desktop trust"} onClick={()=>void reset()}>Reset desktop trust</button></section>}
  </section>;
}
