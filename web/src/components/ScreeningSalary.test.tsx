import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { failure, ok, renderWithClient, sentBody } from "../test-utils";
import { ScreeningSalary } from "./ScreeningSalary";
import { formFromCompensation, salaryProblems, type SalaryFormState, toCompensationBody } from "./salaryFormState";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

function Harness({ initial }: { initial?: SalaryFormState } = {}) {
  const [form, set] = useState<SalaryFormState>(() => initial ?? formFromCompensation(null));
  return <><ScreeningSalary form={form} patch={(v) => { set((f) => ({ ...f, ...v })); }} set={(k,v) => { set((f) => ({ ...f, [k]:v })); }} errors={{}} />
    <output data-testid="body">{JSON.stringify(toCompensationBody(form,0))}</output><output data-testid="problems">{JSON.stringify(salaryProblems(form))}</output></>;
}

it("requires review, preserves the wording, and only applies the calculated monthly figures", async () => {
  vi.spyOn(api,"GET").mockResolvedValue(ok({ allowances:true, bonus:true, bonusOnAllowances:false, cents:false }));
  const post = vi.spyOn(api,"POST").mockResolvedValue(ok({ base:"16000.00", allowances:"400.00", bonus:"2000.00", monthly:"18400.00", annualMonths:13.5, hasCurrent:true, reviewRequired:false,
    expectedMin:"20240.00", warnings:[], excluded:["3 months performance", "16% epf"], currentCurrency:"MYR", expectedCurrency:"MYR" }));
  renderWithClient(<Harness initial={{...formFromCompensation(null),salaryNotes:"Original pay notes: confirm next review date."}} />);
  const description="16k monthly + 400 allowance + 13.5th bonus + 3 months performance + 16% epf";
  fireEvent.change(screen.getByLabelText("Current salary breakdown"),{target:{value:description}});
  fireEvent.change(screen.getByLabelText("Expected salary"),{target:{value:"10% lateral"}});
  expect(screen.getByTestId("problems").textContent).toContain("Calculate and review");
  await waitFor(() => { expect(screen.getByRole<HTMLButtonElement>("button",{name:"Calculate"}).disabled).toBe(false); });
  fireEvent.click(screen.getByRole("button",{name:"Calculate"}));
  await screen.findByRole("button",{name:"Use these figures"});
  expect(sentBody(post)).not.toHaveProperty("rules"); // the server uses the company's saved rules
  expect(screen.getByTestId("body").textContent).not.toContain("18400");
  fireEvent.click(screen.getByRole("button",{name:"Use these figures"}));
  expect(screen.getByTestId("body").textContent).toContain('"amount":"18400.00"');
  expect(screen.getByTestId("body").textContent).toContain(description);
  expect(screen.getByLabelText<HTMLTextAreaElement>("Salary notes").value).toBe("Original pay notes: confirm next review date.");
  expect(screen.getByTestId("body").textContent).toContain('"min":"20240.00"');
  expect(screen.getByTestId("problems").textContent).toBe("{}");
  expect(sentBody(post)).toMatchObject({current:description,expected:"10% lateral"});
  fireEvent.change(screen.getByLabelText("Current salary breakdown"),{target:{value:"18k"}});
  expect(screen.queryByRole("button",{name:"Use these figures"})).toBeNull();
  expect(screen.getByTestId("problems").textContent).toContain("Calculate and review");
  fireEvent.change(screen.getByLabelText("Amount"), { target: { value: "18000" } });
  expect(screen.getByTestId("problems").textContent).toContain("Calculate and review");
  fireEvent.change(screen.getByLabelText("From (or the single figure)"), { target: { value: "19800" } });
  expect(screen.getByTestId("problems").textContent).toBe("{}");
  expect(screen.getByTestId("body").textContent).toContain('"amount":"18000"');
  expect(screen.getByTestId("body").textContent).toContain("Original pay notes: confirm next review date.");
});

it.each([{current:"",expected:"22k",hasCurrent:false,expectedMin:"22000.00"},{current:"0",expected:"",hasCurrent:true,expectedMin:undefined}])("supports expected-only and known zero salaries: $current / $expected", async ({current,expected,hasCurrent,expectedMin}) => {
  vi.spyOn(api,"GET").mockResolvedValue(ok({allowances:true,bonus:true,bonusOnAllowances:false,cents:false}));
  vi.spyOn(api,"POST").mockResolvedValue(ok({base:"0",allowances:"0",bonus:"0",monthly:"0.00",annualMonths:12,hasCurrent,reviewRequired:false,expectedMin,warnings:[],excluded:[]}));
  renderWithClient(<Harness />);
  if(current) fireEvent.change(screen.getByLabelText("Current salary breakdown"),{target:{value:current}});
  if(expected) fireEvent.change(screen.getByLabelText("Expected salary"),{target:{value:expected}});
  if(!hasCurrent) {const field=screen.getAllByLabelText<HTMLInputElement>("Currency")[0];if(!field) throw new Error("Missing current currency");fireEvent.change(field,{target:{value:""}}); }
  await waitFor(() => {expect(screen.getByRole<HTMLButtonElement>("button",{name:"Calculate"}).disabled).toBe(false);});
  fireEvent.click(screen.getByRole("button",{name:"Calculate"}));
  const use=await screen.findByRole<HTMLButtonElement>("button",{name:"Use these figures"});
  expect(use.disabled).toBe(false);fireEvent.click(use);
  expect(screen.getByTestId("problems").textContent).toBe("{}");
  expect(screen.getByTestId("body").textContent).toContain(hasCurrent?'"amount":"0.00"':'"currentSalary":null');
  if(expectedMin) expect(screen.getByTestId("body").textContent).toContain(expectedMin);
});

it("discards a preview after the figures or currency change", async () => {
  vi.spyOn(api,"GET").mockResolvedValue(ok({allowances:true,bonus:true,bonusOnAllowances:false,cents:false}));
  vi.spyOn(api,"POST").mockResolvedValue(ok({base:"16000",allowances:"0",bonus:"0",monthly:"16000",annualMonths:12,hasCurrent:true,reviewRequired:false,currentCurrency:"MYR",warnings:[],excluded:[]}));
  renderWithClient(<Harness />);
  fireEvent.change(screen.getByLabelText("Current salary breakdown"),{target:{value:"16k"}});
  await waitFor(() => {expect(screen.getByRole<HTMLButtonElement>("button",{name:"Calculate"}).disabled).toBe(false);});
  fireEvent.click(screen.getByRole("button",{name:"Calculate"}));await screen.findByRole("button",{name:"Use these figures"});
  fireEvent.change(screen.getByLabelText("Amount"), { target: { value: "17000" } });
  expect(screen.queryByRole("button",{name:"Use these figures"})).toBeNull();
  fireEvent.click(screen.getByRole("button",{name:"Calculate"}));
  await screen.findByRole("button",{name:"Use these figures"});
  const currentCurrency=screen.getAllByLabelText<HTMLInputElement>("Currency")[0];
  if(!currentCurrency) throw new Error("Current currency field missing");
  fireEvent.change(currentCurrency,{target:{value:"SGD"}});
  expect(screen.queryByRole("button",{name:"Use these figures"})).toBeNull();
  fireEvent.change(currentCurrency,{target:{value:"MYR"}});
  fireEvent.click(screen.getByRole("button",{name:"Calculate"}));
  await screen.findByRole("button",{name:"Use these figures"});
});


it("does not let an unrelated expected figure approve an unreviewed current salary", () => {
  vi.spyOn(api,"GET").mockResolvedValue(ok({ allowances:true, bonus:true, bonusOnAllowances:false, cents:false }));
  renderWithClient(<Harness />);
  fireEvent.change(screen.getByLabelText("Current salary breakdown"), { target: { value: "25k + AWS" } });
  fireEvent.change(screen.getByLabelText("To (optional)"), { target: { value: "30000" } });
  expect(screen.getByTestId("problems").textContent).toContain("Calculate and review");
});

it("highlights what needs review but still lets the recruiter use the figures", async () => {
  vi.spyOn(api,"POST").mockResolvedValue(ok({ base:"15000",allowances:"500",bonus:"1875",monthly:"17375",annualMonths:13.5,hasCurrent:true,reviewRequired:true,
    expectedMin:"19113",expectedMax:"20850",warnings:["Choose one fixed salary or guaranteed bonus figure, not a range."],excluded:["rsu"] }));
  renderWithClient(<Harness />);
  fireEvent.change(screen.getByLabelText("Current salary breakdown"), { target: { value: "rm15k + 13.5th month + rsu + 500 allowance + 2-4 months bonus" } });
  fireEvent.change(screen.getByLabelText("Expected salary"), { target: { value: "10-20%" } });
  fireEvent.click(screen.getByRole("button",{name:"Calculate"}));
  const button = await screen.findByRole<HTMLButtonElement>("button",{name:"Use these figures"});
  expect(screen.getByText("Choose one fixed salary or guaranteed bonus figure, not a range.").className).toContain("warning");
  expect(button.disabled).toBe(false);
  fireEvent.click(button);
  expect(screen.getByText("Figures applied. Save the call.")).toBeTruthy();
  expect(screen.getByTestId("problems").textContent).toBe("{}");
  expect(screen.getByTestId("body").textContent).toContain('"amount":"17375"');
  expect(screen.getByTestId("body").textContent).toContain('"min":"19113"');
});

it("cannot use a calculation that read no figure at all", async () => {
  vi.spyOn(api,"POST").mockResolvedValue(ok({ base:"0",allowances:"0",bonus:"0",monthly:"0",annualMonths:12,hasCurrent:false,reviewRequired:true,warnings:["Enter a basic salary."],excluded:[] }));
  renderWithClient(<Harness />);
  fireEvent.change(screen.getByLabelText("Current salary breakdown"), { target: { value: "salary unknown" } });
  fireEvent.click(screen.getByRole("button",{name:"Calculate"}));
  const button = await screen.findByRole<HTMLButtonElement>("button",{name:"Use these figures"});
  expect(button.disabled).toBe(true);
  expect(screen.getByRole("button",{name:"Enter figures manually"})).toBeTruthy();
});

it("offers explicit manual review when the scanner cannot resolve a bonus", async () => {
  vi.spyOn(api,"GET").mockResolvedValue(ok({allowances:true,bonus:true,bonusOnAllowances:false,cents:false}));
  vi.spyOn(api,"POST").mockResolvedValue(ok({base:"9000",allowances:"0",bonus:"0",monthly:"9000",annualMonths:12,hasCurrent:true,reviewRequired:true,warnings:["Enter the guaranteed bonus as extra months or total annual months."],excluded:[]}));
  renderWithClient(<Harness initial={{...formFromCompensation(null),currentAmount:"9000",expectedMin:"11000"}} />);
  fireEvent.change(screen.getByLabelText("Current salary breakdown"),{target:{value:"9k + fixed bonus"}});
  await waitFor(() => {expect(screen.getByRole<HTMLButtonElement>("button",{name:"Calculate"}).disabled).toBe(false);});
  fireEvent.click(screen.getByRole("button",{name:"Calculate"}));
  await screen.findByRole("button",{name:"Use these figures"});
  fireEvent.click(screen.getByRole("button",{name:"Enter figures manually"}));
  expect(screen.getByLabelText("Amount")).toBe(document.activeElement);
  fireEvent.click(screen.getByRole("button",{name:"Use entered figures"}));
  expect(screen.getByTestId("problems").textContent).toBe("{}");
  expect(screen.getByTestId("body").textContent).toContain('"amount":"9000"');
  expect(screen.getByText("Figures applied. Save the call.")).toBeTruthy();
});

it("does not confirm empty, invalid or backwards manual figures", () => {
  vi.spyOn(api,"GET").mockResolvedValue(ok({allowances:true,bonus:true,bonusOnAllowances:false,cents:false}));
  renderWithClient(<Harness />);
  fireEvent.change(screen.getByLabelText("Current salary breakdown"),{target:{value:"9k + fixed bonus"}});
  fireEvent.click(screen.getByText("Salary figures and currencies"));
  fireEvent.click(screen.getByRole("button",{name:"Use entered figures"}));
  expect(screen.getByText("Enter a reviewed current amount, or clear its description.")).toBeTruthy();
  expect(screen.getByTestId("problems").textContent).toContain("Calculate and review");
  fireEvent.change(screen.getByLabelText("Amount"),{target:{value:"9000"}});
  fireEvent.change(screen.getByLabelText("From (or the single figure)"),{target:{value:"13000"}});
  fireEvent.change(screen.getByLabelText("To (optional)"),{target:{value:"12000"}});
  fireEvent.click(screen.getByRole("button",{name:"Use entered figures"}));
  expect(screen.getByText("The upper figure must be at least the lower figure.")).toBeTruthy();
});

it.each(["+10%", "10pct", "10 per cent", "10% plus bonus"])("rechecks expected pay after a manual base change for %s", async (expected) => {
  vi.spyOn(api,"GET").mockResolvedValue(ok({allowances:true,bonus:true,bonusOnAllowances:false,cents:false}));
  vi.spyOn(api,"POST").mockResolvedValue(ok({base:"10000",allowances:"0",bonus:"0",monthly:"10000",annualMonths:12,hasCurrent:true,reviewRequired:false,expectedMin:"11000",warnings:[],excluded:[]}));
  renderWithClient(<Harness />);
  fireEvent.change(screen.getByLabelText("Current salary breakdown"),{target:{value:"10k"}});
  fireEvent.change(screen.getByLabelText("Expected salary"),{target:{value:expected}});
  await waitFor(() => {expect(screen.getByRole<HTMLButtonElement>("button",{name:"Calculate"}).disabled).toBe(false);});
  fireEvent.click(screen.getByRole("button",{name:"Calculate"}));
  fireEvent.click(await screen.findByRole("button",{name:"Use these figures"}));
  fireEvent.change(screen.getByLabelText("Amount"),{target:{value:"12000"}});
  expect(screen.getByTestId("problems").textContent).toContain("Calculate and review");
});

it("keeps reviewed manual entry available when the calculation fails", async () => {
  vi.spyOn(api,"POST").mockResolvedValue(failure("SERVICE_UNAVAILABLE","Could not calculate.",undefined,503));
  renderWithClient(<Harness initial={{...formFromCompensation(null),currentAmount:"9000",expectedMin:"11000"}} />);
  fireEvent.click(screen.getByRole("button",{name:"Calculate"}));
  fireEvent.click(await screen.findByRole("button",{name:"Enter figures manually"}));
  fireEvent.click(screen.getByRole("button",{name:"Use entered figures"}));
  expect(screen.getByTestId("problems").textContent).toBe("{}");
  expect(screen.getByText("Figures applied. Save the call.")).toBeTruthy();
});
