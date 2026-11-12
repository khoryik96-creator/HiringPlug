import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { api, type Permission } from "../../api/client";
import { ok, renderWithClient, sentBody } from "../../test-utils";
import { EmailComposer } from "../EmailComposer";
const permissions:Permission[]=["emails:send","candidates:read","documents:read","candidates.compensation:read"];
afterEach(()=>{cleanup();vi.restoreAllMocks();window.localStorage.clear();});
function setup(recipient:"contact"|"candidate"="contact"){
 vi.spyOn(api,"GET").mockImplementation(((path:string,init?:{params?:{path?:{candidateId?:string};query?:{q?:string}}})=>{
  if(path==="/v1/email-templates")return Promise.resolve(ok({items:[],fields:[]}));
  if(path==="/v1/candidates")return Promise.resolve(ok({items:[{id:"cand_one",fullName:"First Person",currentTitle:"Analyst"},{id:"cand_two",fullName:"Second Person",currentTitle:"Engineer"}]}));
  if(path==="/v1/candidates/{candidateId}/documents"){const id=init?.params?.path?.candidateId ?? "";return Promise.resolve(ok({items:[{id:`doc_${id}`,fileName:`${id}.docx`,source:"formatted",createdAt:"2026-11-01T10:00:00Z",pdfStatus:"ready"}]}));}
  return Promise.resolve(ok({items:[]}));
 }) as never);
 const post=vi.spyOn(api,"POST").mockImplementation(((path:string,init:{body:{candidateId?:string}})=>{
  if(path==="/v1/email-previews"){const id=init.body.candidateId;return Promise.resolve(ok({subject:"",bodyHtml:"",missing:[],salary:!!id,clientEmail:null,candidateVersion:3,compensationVersion:id==="cand_one"?4:5,fieldValues:id?{"[Full name]":id==="cand_one"?"First Person":"Second Person","[Current salary]":id==="cand_one"?"RM 10,000":"RM 20,000","[Expected salary]":id==="cand_one"?"RM 12,000":"RM 24,000"}:{}}));}
  return Promise.resolve(ok({id:"eml_sent"}));
 }) as never);
 renderWithClient(<EmailComposer {...(recipient==="contact"?{contactId:"contact_client"}:{candidateId:"cand_recipient"})} defaultTo="client@example.test" permissions={permissions} canManageTemplates={false} onDone={vi.fn()}/>);
 fireEvent.click(screen.getByText("Candidates and CVs (0)"));
 return post;
}
async function add(){fireEvent.change(screen.getByLabelText("Add candidates"),{target:{value:"Person"}});fireEvent.click(await screen.findByRole("button",{name:"First Person · Analyst"}));fireEvent.change(screen.getByLabelText("Add candidates"),{target:{value:"Person"}});fireEvent.click(await screen.findByRole("button",{name:"Second Person · Engineer"}));await waitFor(()=>{expect(screen.getByRole<HTMLButtonElement>("button",{name:"Insert candidates into the message"}).disabled).toBe(false);});}
it("puts multiple candidates with their own salary versions and chosen CVs into a contact email",async()=>{
 const post=setup();await add();fireEvent.change(screen.getByLabelText("Profile to attach for Second Person"),{target:{value:"doc_cand_two:file"}});
 const editor=screen.getByRole("textbox",{name:"Message"});editor.innerHTML="<p>Intro to keep.</p>";fireEvent.input(editor);
 fireEvent.click(screen.getByRole("button",{name:"Insert candidates into the message"}));
 await waitFor(()=>{expect(editor.textContent).toContain("RM 12,000");});expect(editor.textContent).toContain("RM 24,000");
 fireEvent.change(screen.getByLabelText("Subject"),{target:{value:"Candidate profiles"}});fireEvent.click(screen.getByRole("button",{name:"Send"}));
 await waitFor(()=>{expect(post.mock.calls.some((c)=>(c as unknown[])[0]==="/v1/emails")).toBe(true);});const index=post.mock.calls.findIndex((c)=>(c as unknown[])[0]==="/v1/emails");
 expect(sentBody(post,index)).toMatchObject({contactId:"contact_client",candidates:[{candidateId:"cand_one",candidateVersion:3,compensationVersion:4},{candidateId:"cand_two",candidateVersion:3,compensationVersion:5}],cvs:[{documentId:"doc_cand_one",asPdf:true},{documentId:"doc_cand_two",asPdf:false}]});
});
it("updates one managed section and removes deselected candidate details without deleting the introduction",async()=>{
 setup("candidate");await add();const editor=screen.getByRole("textbox",{name:"Message"});editor.innerHTML="<p>Keep my introduction.</p>";fireEvent.input(editor);
 fireEvent.click(screen.getByRole("button",{name:"Insert candidates into the message"}));await waitFor(()=>{expect(editor.textContent).toContain("Second Person");});
 fireEvent.click(screen.getByRole("button",{name:"Insert candidates into the message"}));await waitFor(()=>{expect(editor.querySelectorAll('[data-candidate-summary="true"]').length).toBe(1);});
 const first=editor.querySelector('[data-candidate-id="cand_one"]');if(first)first.insertAdjacentHTML("beforeend","<br>Keep this personal comment.");fireEvent.input(editor);
 const row=screen.getByText("Second Person",{selector:"strong"}).closest("li");if(!row)throw new Error("missing candidate row");fireEvent.click(within(row).getByRole("button",{name:"Remove"}));
 await waitFor(()=>{expect(editor.textContent).not.toContain("Second Person");});expect(editor.textContent).toContain("Keep my introduction.");expect(editor.textContent).toContain("Keep this personal comment.");
 fireEvent.click(screen.getByRole("button",{name:"Insert candidates into the message"}));await waitFor(()=>{expect(editor.textContent).toContain("First Person");});expect(editor.textContent).not.toContain("Second Person");
});
