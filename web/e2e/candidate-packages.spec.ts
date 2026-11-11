// Candidate details and chosen CVs belong to each source, independently of the recipient.
import { mkdirSync, readFileSync } from "node:fs";
import type { Schemas } from "../src/api/client";
import { expect, type APIRequestContext, test } from "@playwright/test";

const lines=readFileSync(process.env.HP_E2E_TOKENS_FILE ?? "", "utf8").split("\n");
const token=lines[lines.findIndex((l)=>l.startsWith("owner "))+1]?.trim();
if(!token)throw new Error("HP_E2E_TOKENS_FILE needs the demo owner's token");
const headers={Authorization:`Bearer ${token}`};
async function create<T>(request:APIRequestContext,path:string,data:unknown):Promise<T>{
 const response=await request.post(path,{headers,data});
 expect(response.ok(),await response.text()).toBe(true);return await response.json() as T;
}
// Small synthetic PDF; no worker is required for selecting attachments.
const pdf=Buffer.from("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF");

test("selects several candidates in contact, candidate and job emails, preserving salary notes",async({page,request})=>{
 const stamp=Date.now();
 const catalogue=await(await request.get("/v1/industries",{headers})).json() as Schemas["IndustryCatalogue"];
 const bank=catalogue.items.find((i)=>i.label==="FSI – Banking");
 if(!bank)throw new Error("Seed Banking subindustry missing");
 const first=await create<Schemas["Candidate"]>(request,"/v1/candidates",{firstName:"Nadia",lastName:`Package ${stamp}`,currentTitle:"QA Lead",email:`nadia.${stamp}@example.test`,industrySelections:{catalogueVersion:catalogue.version,ids:[bank.id],primaryId:bank.id}});
 const second=await create<Schemas["Candidate"]>(request,"/v1/candidates",{firstName:"Daniel",lastName:`Package ${stamp}`,currentTitle:"Platform Engineer"});
 const company=await create<Schemas["Company"]>(request,"/v1/companies",{name:`Synthetic Package Client ${stamp}`});
 const contact=await create<Schemas["Contact"]>(request,"/v1/contacts",{companyId:company.id,firstName:"Aisha",lastName:"Client",email:`aisha.${stamp}@example.test`});
 const job=await create<Schemas["JobOrder"]>(request,"/v1/jobs",{companyId:company.id,contactId:contact.id,title:`QA and Platform ${stamp}`});
 for(const [index,c] of [first,second].entries()){
  const saved=await create<Schemas["ScreeningCallResult"]>(request,`/v1/candidates/${c.id}/screening-calls`,{version:c.version,calledOn:"2026-10-01",compensation:{expectedVersion:0,currentSalary:{amount:String(index===0?16700:21000),currency:"MYR",period:"month",description:index===0?"12k + 3200 allowance + 13.5 months total":"21k monthly"},expectedSalary:{min:String(index===0?19038:25000),currency:"MYR",period:"month",description:"Negotiable"},salaryNotes:index===0?"Original salary discussion — retained":"Separate package notes"}});
  expect(saved.call.salarySnapshot?.salaryNotes).toBeTruthy();
  for(const name of ["Original CV","Alternative CV"]){
   const uploaded=await request.post(`/v1/candidates/${c.id}/documents`,{headers,multipart:{source:"original",file:{name:`${c.firstName} ${name}.pdf`,mimeType:"application/pdf",buffer:Buffer.concat([pdf,Buffer.from(`\n% ${name} ${c.id}`)])}}});
   expect(uploaded.status(),await uploaded.text()).toBe(201);
  }
  await create(request,`/v1/jobs/${job.id}/applications`,{candidateId:c.id});
 }
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 await page.addInitScript((t)=>{if(window===window.top)sessionStorage.setItem("hiringplug.token",t);},token);
 const shot=async(name:string)=>{
  const dir=process.env.HP_UI_SCREENSHOT_DIR;if(!dir)return;mkdirSync(dir,{recursive:true});
  const target=name==="screening-notes-and-industry"?page.getByRole("form",{name:"Screening call",exact:true})
   :name==="salary-call-history"?page.getByRole("region",{name:"Screening calls",exact:true})
   :name==="candidate-salary"||name==="edit-salary"?page.getByRole("region",{name:"Salary",exact:true})
   :name==="job-client-email"?page.getByRole("region",{name:"Email client",exact:true})
   :page.getByRole("form",{name:"Write email",exact:true});
  await target.screenshot({path:`${dir}/${name}.png`,style:".topbar { visibility: hidden !important; }"});
 };
 await page.goto(`/#/candidates/${first.id}/screening`);
 await expect(page.getByLabel("Salary notes",{exact:true})).toHaveValue("Original salary discussion — retained");
 const picker=page.getByRole("region",{name:"Industries and subindustries"});await expect(picker).toContainText("FSI – Banking");
 await page.getByLabel("Salary notes",{exact:true}).fill("Updated discussion: prefers fixed pay");await shot("screening-notes-and-industry");
 await page.getByRole("button",{name:"Save the call",exact:true}).click();await expect(page).toHaveURL(new RegExp(`#/candidates/${first.id}$`));
 await page.getByRole("region",{name:"Salary",exact:true}).scrollIntoViewIfNeeded();await shot("candidate-salary");
 await page.getByRole("button",{name:"Edit salary",exact:true}).click();await expect(page.getByLabel("Salary notes",{exact:true})).toHaveValue("Updated discussion: prefers fixed pay");await shot("edit-salary");await page.getByRole("button",{name:"Cancel",exact:true}).click();
 await page.getByText("Salary history",{exact:true}).click();
 const calls=page.getByRole("region",{name:"Screening calls",exact:true});await calls.locator("details").last().locator("summary").click();await expect(calls).toContainText("Original salary discussion — retained");await calls.scrollIntoViewIfNeeded();await shot("salary-call-history");
 await page.goto(`/#/contacts/${contact.id}`);await page.getByRole("button",{name:"Write email",exact:true}).click();
 const editor=page.getByRole("textbox",{name:"Message",exact:true});await editor.fill("Dear Aisha, please review these candidates.");
 for(const c of [first,second]){await page.getByLabel("Add candidates",{exact:true}).fill(c.fullName);await page.getByRole("button",{name:`${c.fullName} · ${c.currentTitle}`,exact:true}).click();}
 await page.getByRole("checkbox",{name:"Salary notes",exact:true}).check();const choices=page.getByLabel(`Profile to attach for ${second.fullName}`,{exact:true});await expect(choices.locator("option")).toHaveCount(3);await choices.selectOption({index:1});
 await page.getByRole("button",{name:"Insert candidates into the message",exact:true}).click();await expect(editor).toContainText("Updated discussion: prefers fixed pay");await expect(editor).toContainText("Separate package notes");
 await expect(editor.locator('[data-candidate-summary="true"]')).toHaveCount(1);await expect(editor.locator("[data-candidate-id]").first()).not.toContainText("Dear Aisha");
 await page.getByLabel("Subject",{exact:true}).fill("Selected QA and platform candidates");await page.getByRole("button",{name:"Preview email",exact:true}).click();await expect(page.getByLabel("Email preview",{exact:true})).toContainText("Daniel");await shot("contact-email-multiple-candidates");
 await page.getByRole("button",{name:`Move ${second.fullName} up`,exact:true}).click();await expect(editor.locator("[data-candidate-id]").first()).toContainText("Daniel");
 const send=page.waitForResponse(r=>r.url().endsWith("/v1/emails")&&r.request().method()==="POST");await page.getByRole("button",{name:"Send",exact:true}).click();const sent=await send;expect(sent.status(),await sent.text()).toBe(202);const email=await sent.json() as Schemas["Email"];expect(email.attachments).toHaveLength(2);expect(email.contactId).toBe(contact.id);
 await page.goto(`/#/candidates/${first.id}`);await page.getByRole("button",{name:"Write email",exact:true}).click();await page.getByRole("button",{name:"Use this candidate’s details",exact:true}).click();await page.getByLabel("Add candidates",{exact:true}).fill(second.fullName);await page.getByRole("button",{name:`${second.fullName} · ${second.currentTitle}`,exact:true}).click();await page.getByRole("button",{name:"Insert candidates into the message",exact:true}).click();await expect(editor).toContainText(second.fullName);await shot("candidate-email-multiple-candidates");
 await page.goto(`/#/jobs/${job.id}`);await page.getByRole("checkbox",{name:`Choose ${first.fullName} for an email to the client`,exact:true}).check();await page.getByRole("checkbox",{name:`Choose ${second.fullName} for an email to the client`,exact:true}).check();await page.getByRole("button",{name:"Email client about 2 candidates",exact:true}).click();await page.getByRole("button",{name:"Insert candidates into the message",exact:true}).click();await expect(page.getByRole("textbox",{name:"Message",exact:true})).toContainText(first.fullName);await shot("job-client-email");
 expect(errors).toEqual([]);
});
