import { cleanup,fireEvent,screen,waitFor } from "@testing-library/react";
import { afterEach,expect,it,vi } from "vitest";
import { api,type Schemas } from "../api/client";
import { ok,renderWithClient,sentBody } from "../test-utils";
import { EmailConnection } from "./EmailConnection";
import { MailDeliveryNotice } from "../components/MailDeliveryNotice";

const initial:Schemas["MailboxStatus"]={connected:false,storageReady:true,microsoftReady:false,googleReady:false,serverMode:"test",version:0};
afterEach(()=>{cleanup();vi.restoreAllMocks();window.location.hash="";});

it("makes local test delivery explicit and hides unavailable provider sign-in",async()=>{
 vi.spyOn(api,"GET").mockResolvedValue(ok(initial));renderWithClient(<EmailConnection/>);
 expect(await screen.findByText(/Messages stay on this computer/)).toBeTruthy();
 expect(screen.getByRole<HTMLButtonElement>("button",{name:"Connect Microsoft"}).disabled).toBe(true);
 expect(screen.getByRole<HTMLButtonElement>("button",{name:"Connect Google"}).disabled).toBe(true);
 expect(screen.getByRole<HTMLButtonElement>("button",{name:"Other provider (SMTP)"}).disabled).toBe(false);
});
it("saves SMTP without returning the password and keeps its saved acknowledgement",async()=>{
 const saved:Schemas["MailboxStatus"]={...initial,connected:true,provider:"smtp",address:"sender@agency.test",host:"smtp.example.test",port:587,tls:"starttls",username:"sender",version:1,deliveryMode:"live"};
 vi.spyOn(api,"GET").mockResolvedValue(ok(initial));const put=vi.spyOn(api,"PUT").mockResolvedValue(ok(saved));renderWithClient(<EmailConnection/>);
 fireEvent.click(await screen.findByRole("button",{name:"Other provider (SMTP)"}));
 for(const [label,value] of [["Email address","sender@agency.test"],["SMTP server","smtp.example.test"],["Username","sender"],["Password or app password","synthetic-password"]])fireEvent.change(screen.getByLabelText(label ?? ""),{target:{value}});
 fireEvent.click(screen.getByRole("button",{name:"Save email connection"}));
 expect(await screen.findByText("Connection saved.")).toBeTruthy();expect(sentBody(put)).toMatchObject({expectedVersion:0,address:"sender@agency.test",password:"synthetic-password",tls:"starttls"});
 await waitFor(()=>{expect(screen.getByLabelText<HTMLInputElement>("Password or app password").value).toBe("");});
});
it("shows a connected account without exposing any token inputs",async()=>{
 vi.spyOn(api,"GET").mockResolvedValue(ok({...initial,connected:true,provider:"microsoft",address:"recruiter@another-domain.test",version:1,deliveryMode:"live"}));renderWithClient(<EmailConnection/>);
 expect(await screen.findByText("recruiter@another-domain.test")).toBeTruthy();expect(screen.queryByLabelText("Password or app password")).toBeNull();
});
it("warns in the composer when mail is captured locally",async()=>{
 vi.spyOn(api,"GET").mockResolvedValue(ok(initial));renderWithClient(<MailDeliveryNotice/>);
 expect(await screen.findByText(/These emails will not reach real mailboxes/)).toBeTruthy();
});
