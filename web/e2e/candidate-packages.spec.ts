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
  const saved=await create<Schemas["ScreeningCallResult"]>(request,`/v1/candidates/${c.id}/screening-calls`,{version:c.version,calledOn:"2026-10-01",compensation:{expectedVersion:0,currentSalary:{amount:String(index===0?16700:21000),currency:"MYR",period:"month",description:index===0?"12k + 3200 alowance + 13.5 months total + perf bonus + epf13%":"21k monthly"},expectedSalary:{min:String(index===0?19038:25000),currency:"MYR",period:"month",description:"Negotiable"},salaryNotes:index===0?"Original salary discussion — retained":"Separate package notes"}});
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
   :(name==="job-client-email" || name==="invalid-fields")?page.getByRole("region",{name:"Email client",exact:true})
   :page.getByRole("form",{name:"Write email",exact:true});
  await target.screenshot({path:`${dir}/${name}.png`,style:".topbar { visibility: hidden !important; }"});
 };
 await page.goto("/#/settings");
 await page.getByRole("combobox",{name:"Salary in emails",exact:true}).selectOption("breakdown");await page.getByRole("button",{name:"Save calculation rules",exact:true}).click();await expect(page.getByRole("status").filter({hasText:"Saved"})).toBeVisible();
 if(process.env.HP_UI_SCREENSHOT_DIR)await page.getByRole("region",{name:"Salary calculation",exact:true}).screenshot({path:`${process.env.HP_UI_SCREENSHOT_DIR}/settings.png`,style:".topbar {visibility:hidden !important;}"});
 await page.goto(`/#/candidates/${first.id}/screening`);
 await expect(page.getByLabel("Salary notes",{exact:true})).toHaveValue("Original salary discussion — retained");
 const picker=page.getByRole("region",{name:"Industries and subindustries"});await expect(picker).toContainText("FSI – Banking");
 await page.getByLabel("Salary notes",{exact:true}).fill("Updated discussion: prefers fixed pay");await shot("screening-notes-and-industry");
 await page.getByRole("button",{name:"Save the call",exact:true}).click();await expect(page).toHaveURL(new RegExp(`#/candidates/${first.id}$`));
 await page.getByRole("tab",{name:"Salary",exact:true}).click();await page.getByRole("region",{name:"Salary",exact:true}).scrollIntoViewIfNeeded();await shot("candidate-salary");
 await page.getByRole("button",{name:"Edit salary",exact:true}).click();await expect(page.getByLabel("Salary notes",{exact:true})).toHaveValue("Updated discussion: prefers fixed pay");await shot("edit-salary");await page.getByRole("button",{name:"Cancel",exact:true}).click();
 await page.getByRole("tab",{name:"Activity",exact:true}).click();
 const calls=page.getByRole("region",{name:"Screening calls",exact:true});await calls.locator("details").last().locator("summary").click();await expect(calls).toContainText("Original salary discussion — retained");await calls.scrollIntoViewIfNeeded();await shot("salary-call-history");
 await page.goto(`/#/contacts/${contact.id}`);await page.getByRole("button",{name:"Email",exact:true}).click();
 await page.getByText("Candidates and CVs (0)",{exact:true}).click();
 const editor=page.getByRole("textbox",{name:"Message",exact:true});await editor.fill("Dear Aisha, please review these candidates.");
 for(const c of [first,second]){await page.getByLabel("Add candidates",{exact:true}).fill(c.fullName);await page.getByRole("button",{name:`${c.fullName} · ${c.currentTitle}`,exact:true}).click();}
 await page.getByRole("checkbox",{name:"Salary notes",exact:true}).check();const choices=page.getByLabel(`Profile to attach for ${second.fullName}`,{exact:true});await expect(choices.locator("option")).toHaveCount(3);await choices.selectOption({index:1});
 await page.getByRole("button",{name:"Insert candidates into the message",exact:true}).click();await expect(editor).toContainText("Updated discussion: prefers fixed pay");await expect(editor).toContainText("Separate package notes");await expect(editor).toContainText("RM 3,200 fixed allowance");await expect(editor).toContainText("RM 12,000");await expect(editor).toContainText("13.5th month fixed/contractual bonus");await expect(editor).toContainText("performance bonus");await expect(editor).toContainText("13% EPF");
 await expect(editor.locator('[data-candidate-summary="true"]')).toHaveCount(1);await expect(editor.locator("[data-candidate-id]").first()).not.toContainText("Dear Aisha");
 await page.getByLabel("Subject",{exact:true}).fill("Selected QA and platform candidates");await page.getByRole("button",{name:"Preview email",exact:true}).click();await expect(page.getByLabel("Email preview",{exact:true})).toContainText("Daniel");await shot("contact-email-multiple-candidates");
 await page.getByRole("button",{name:`Move ${second.fullName} up`,exact:true}).click();await expect(editor.locator("[data-candidate-id]").first()).toContainText("Daniel");
 const send=page.waitForResponse(r=>r.url().endsWith("/v1/emails")&&r.request().method()==="POST");await page.getByRole("button",{name:"Send",exact:true}).click();const sent=await send;expect(sent.status(),await sent.text()).toBe(202);const email=await sent.json() as Schemas["Email"];expect(email.attachments).toHaveLength(2);expect(email.contactId).toBe(contact.id);
 await page.goto(`/#/candidates/${first.id}`);await page.getByRole("button",{name:"Email",exact:true}).click();await page.getByText("Candidates and CVs (0)",{exact:true}).click();await page.getByRole("button",{name:"Use this candidate’s details",exact:true}).click();await page.getByLabel("Add candidates",{exact:true}).fill(second.fullName);await page.getByRole("button",{name:`${second.fullName} · ${second.currentTitle}`,exact:true}).click();await page.getByRole("button",{name:"Insert candidates into the message",exact:true}).click();await expect(editor).toContainText(second.fullName);await shot("candidate-email-multiple-candidates");
 await page.goto(`/#/jobs/${job.id}`);
 const details=page.getByLabel("Job details",{exact:true});await expect(details).not.toHaveAttribute("open");await details.locator("summary").click();await expect(details).toHaveAttribute("open");await details.locator("summary").click();
 if(process.env.HP_UI_SCREENSHOT_DIR)await details.screenshot({path:`${process.env.HP_UI_SCREENSHOT_DIR}/collapsed-job-details.png`,style:".topbar {visibility:hidden !important;}"});
 await page.getByRole("button",{name:"Select all Applied candidates",exact:true}).click();await page.getByRole("button",{name:"Unselect all Applied candidates",exact:true}).click();await page.getByRole("button",{name:"Select all Applied candidates",exact:true}).click();
 if(process.env.HP_UI_SCREENSHOT_DIR)await page.getByRole("region",{name:"Pipeline",exact:true}).screenshot({path:`${process.env.HP_UI_SCREENSHOT_DIR}/bulk-selection.png`,style:".topbar {visibility:hidden !important;}"});await page.getByRole("button",{name:"Email client about 2 candidates",exact:true}).click();await page.getByText("Candidates and CVs (2)",{exact:true}).click();await page.getByRole("button",{name:"Insert candidates into the message",exact:true}).click();await expect(page.getByRole("textbox",{name:"Message",exact:true})).toContainText(first.fullName);await shot("job-client-email");
 await page.getByRole("button",{name:"One table",exact:true}).click();await page.getByRole("button",{name:"Insert candidates into the message",exact:true}).click();
 const table=page.getByRole("textbox",{name:"Message",exact:true}).locator("table");await expect(table).toContainText("RM 12,000 + RM 3,200 fixed allowance + 13.5th month fixed/contractual bonus + performance bonus + 13% EPF per month");await expect(table.locator("tbody tr").first().locator("td").first()).toHaveText(first.fullName);
 await expect(table.locator("td").filter({has:page.locator("[data-missing-detail]")}).first()).toHaveCSS("outline-color","rgb(220, 38, 38)");
 const invalid=page.waitForResponse(r=>r.url().endsWith("/v1/emails")&&r.request().method()==="POST");await page.getByRole("button",{name:"Send email",exact:true}).click();await invalid;
 await expect(page.getByLabel("Subject",{exact:true})).toHaveAttribute("aria-invalid","true");
 await page.getByLabel("Subject",{exact:true}).fill("Candidate introduction");await page.getByLabel("To",{exact:true}).fill("invalid-address");
 const badTo=page.waitForResponse(r=>r.url().endsWith("/v1/emails")&&r.request().method()==="POST");await page.getByRole("button",{name:"Send email",exact:true}).click();await badTo;await expect(page.getByLabel("To",{exact:true})).toHaveAttribute("aria-invalid","true");await shot("invalid-fields");
 await request.put("/v1/settings/salary-calculation",{headers,data:{allowances:true,bonus:true,bonusOnAllowances:false,cents:false,emailSalaryDisplay:"monthly"}});
 expect(errors).toEqual([]);
});
