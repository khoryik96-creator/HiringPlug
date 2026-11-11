import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "../api/client";
import { ErrorMessage } from "./ErrorMessage";

export const mailboxKey=["mailbox-connection"] as const;
export const useMailbox=()=>useQuery({queryKey:mailboxKey,queryFn:async()=>unwrap(await api.GET("/v1/mailbox")),staleTime:30_000});

export function MailDeliveryNotice(){
 const query=useMailbox(),connection=query.data;
 return <div className="full">
  <ErrorMessage error={query.error}/>
  {connection?.connected && connection.deliveryMode==="test"?<p className="warning small">This connection uses a local test inbox. It will not deliver to real mailboxes. <a href="#/email-connection">Email connection</a></p>
   :connection?.connected?<p className="muted small">Sending from {connection.address}. <a href="#/email-connection">Email connection</a></p>
   :connection?.serverMode==="test"?<p className="warning small" role="status">Local test inbox only. These emails will not reach real mailboxes. <a href="#/email-connection">Connect email</a></p>
   :connection?.serverMode==="unconfigured"?<p className="warning small">Email delivery is not configured. <a href="#/email-connection">Connect email</a></p>
   :connection?.serverMode==="live"?<p className="muted small">Sending through the server’s mail account. <a href="#/email-connection">Use your own mailbox</a></p>:null}
 </div>;
}
