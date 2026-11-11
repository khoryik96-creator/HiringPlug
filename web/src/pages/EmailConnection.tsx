import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, type Schemas, unwrap } from "../api/client";
import { mailboxKey, useMailbox } from "../components/MailDeliveryNotice";
import { ErrorMessage } from "../components/ErrorMessage";
import { Field } from "../components/Field";
import { fieldErrors } from "../components/forms";

export function EmailConnection(){
 const client=useQueryClient(),query=useMailbox();
 const [smtp,setSmtp]=useState(false);const [saved,setSaved]=useState(false);
 const connect=useMutation({onMutate:()=>{setOutcome(null);},mutationFn:async(provider:"microsoft"|"google")=>unwrap(await api.POST("/v1/mailbox/authorize",{body:{provider}})),onSuccess:(result)=>{window.location.assign(result.url);}});
 const disconnect=useMutation({mutationFn:async()=>{const result=await api.DELETE("/v1/mailbox");if(!result.response.ok)unwrap(result);},onSuccess:async()=>{setSmtp(false);setOutcome(null);await client.invalidateQueries({queryKey:mailboxKey});}});
 const status=query.data;
 const [outcome,setOutcome]=useState(()=>new URLSearchParams(window.location.hash.split("?")[1]).get("mailbox"));
 return <main><h2>Email connection</h2>
  <ErrorMessage error={query.error ?? connect.error ?? disconnect.error}/>
  {outcome==="connected" && <p role="status">Mailbox connected.</p>}
  {outcome==="failed" && <p role="alert" className="error">Sign-in could not be completed. Connect again; your existing connection was kept.</p>}
  {status && <>
   <section className="panel" aria-label="Current email connection"><h3>Your mailbox</h3>
    {status.connected?<><p><strong>{status.address}</strong> · {status.provider==="microsoft"?"Microsoft":status.provider==="google"?"Google":"SMTP"}</p><p className={status.deliveryMode==="test"?"warning":"muted"}>{status.deliveryMode==="test"?"Local test inbox only; no real email delivery.":"Used for emails you send from this agency."}</p>
      <button type="button" disabled={disconnect.isPending} onClick={()=>{if(window.confirm("Disconnect this mailbox? Emails already queued for it will not be rerouted."))disconnect.mutate();}}>Disconnect</button></>
     :status.serverMode==="test"?<p className="warning">Using a local test inbox. Messages stay on this computer and are not delivered to real email addresses.</p>
     :status.serverMode==="live"?<p>Using the server’s mail account. Connect your mailbox to send from your own address.</p>:<p>No outgoing email connection.</p>}
    {!status.storageReady && <p className="warning">The administrator needs to enable secure mailbox storage before you can connect.</p>}
   </section>
   <section className="panel" aria-label="Connect email"><h3>Connect email</h3>
    <div className="row">
     <button type="button" disabled={!status.microsoftReady || connect.isPending} onClick={()=>{connect.mutate("microsoft");}}>Connect Microsoft</button>
     <button type="button" disabled={!status.googleReady || connect.isPending} onClick={()=>{connect.mutate("google");}}>Connect Google</button>
     <button type="button" disabled={!status.storageReady || connect.isPending} onClick={()=>{setSmtp(true);}}>Other provider (SMTP)</button>
    </div>
    {(!status.microsoftReady || !status.googleReady) && <p className="muted small">Microsoft and Google sign-in become available after the administrator configures the application registrations.</p>}
    {saved && <p role="status">Connection saved.</p>}
    {(smtp || status.provider==="smtp") && <SmtpForm key={status.version} status={status} onSaved={()=>{setSaved(true);setOutcome(null);}} onEdited={()=>{setSaved(false);}}/>}
   </section>
  </>}
 </main>;
}
function SmtpForm({status,onSaved,onEdited}:{status:Schemas["MailboxStatus"];onSaved:()=>void;onEdited:()=>void}){
 const client=useQueryClient();
 const [form,setForm]=useState<Omit<Schemas["SmtpMailboxInput"],"expectedVersion"|"password"> & {password:string}>({address:status.address ?? "",host:status.host ?? "",port:status.port ?? 587,tls:(status.tls==="tls"?"tls":"starttls"),username:status.username ?? "",password:""});
 const save=useMutation({mutationFn:async()=>unwrap(await api.PUT("/v1/mailbox",{body:{...form,expectedVersion:status.version,...(form.password?{password:form.password}:{password:undefined})}})),onSuccess:(result)=>{onSaved();client.setQueryData(mailboxKey,result);}});
 const errors=fieldErrors(save.error);
 const change=(key:"address"|"host"|"username"|"password",value:string)=>{onEdited();setForm((f)=>({...f,[key]:value}));};
 return <form className="form-grid" aria-label="SMTP connection" onChange={onEdited} onSubmit={(e)=>{e.preventDefault();save.mutate();}}>
  <Field label="Email address" error={errors.address}><input disabled={save.isPending} type="email" autoComplete="email" value={form.address} onChange={(e)=>{change("address",e.target.value);}} required/></Field>
  <Field label="SMTP server" error={errors.host}><input disabled={save.isPending} value={form.host} onChange={(e)=>{change("host",e.target.value);}} required/></Field>
  <Field label="Port" error={errors.port}><input disabled={save.isPending} type="number" min={1} max={65535} value={form.port} onChange={(e)=>{setForm({...form,port:Number(e.target.value)});}} required/></Field>
  <Field label="Security" error={errors.tls}><select disabled={save.isPending} value={form.tls} onChange={(e)=>{const tls=e.target.value as "tls"|"starttls";setForm({...form,tls,port:tls==="tls"?465:587});}}><option value="starttls">STARTTLS</option><option value="tls">TLS</option></select></Field>
  <Field label="Username" error={errors.username}><input disabled={save.isPending} autoComplete="username" value={form.username} onChange={(e)=>{change("username",e.target.value);}}/></Field>
  <Field label="Password or app password" error={errors.password}><input disabled={save.isPending} type="password" autoComplete="new-password" value={form.password} onChange={(e)=>{change("password",e.target.value);}}/></Field>
  <p className="muted small full">Use your provider’s SMTP details. Saved passwords are never shown; leave the password blank to keep it for an unchanged connection.</p>
  <div className="row full"><button type="submit" disabled={save.isPending}>Save email connection</button></div>
  <div className="full"><ErrorMessage error={save.error}/></div>
 </form>;
}
