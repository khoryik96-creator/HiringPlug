import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { ScreeningSalary } from "./ScreeningSalary";
import { formFromCompensation, salaryProblems, type SalaryFormState, toCompensationBody } from "./salaryFormState";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

function Harness() {
  const [form, set] = useState<SalaryFormState>(() => formFromCompensation(null));
  return <><ScreeningSalary form={form} patch={(v) => { set((f) => ({ ...f, ...v })); }} set={(k,v) => { set((f) => ({ ...f, [k]:v })); }} errors={{}} />
    <output data-testid="body">{JSON.stringify(toCompensationBody(form,0))}</output><output data-testid="problems">{JSON.stringify(salaryProblems(form))}</output></>;
}

it("requires review, preserves the wording, and only applies the calculated monthly figures", async () => {
  vi.spyOn(api,"GET").mockResolvedValue(ok({ allowances:true, bonus:true, bonusOnAllowances:false, cents:false }));
  const post = vi.spyOn(api,"POST").mockResolvedValue(ok({ base:"16000.00", allowances:"400.00", bonus:"2000.00", monthly:"18400.00", annualMonths:13.5,
    expectedMin:"20240.00", warnings:[], excluded:["3 months performance", "16% epf"], currentCurrency:"MYR", expectedCurrency:"MYR" }));
  renderWithClient(<Harness />);
  const description="16k monthly + 400 allowance + 13.5th bonus + 3 months performance + 16% epf";
  fireEvent.change(screen.getByLabelText("Current salary breakdown"),{target:{value:description}});
  fireEvent.change(screen.getByLabelText("Expected salary"),{target:{value:"10% lateral"}});
  expect(screen.getByTestId("problems").textContent).toContain("Calculate and review");
  await waitFor(() => { expect(screen.getByRole<HTMLButtonElement>("button",{name:"Calculate"}).disabled).toBe(false); });
  fireEvent.click(screen.getByRole("button",{name:"Calculate"}));
  await screen.findByRole("button",{name:"Use these figures"});
  expect(screen.getByTestId("body").textContent).not.toContain("18400");
  fireEvent.click(screen.getByRole("button",{name:"Use these figures"}));
  expect(screen.getByTestId("body").textContent).toContain('"amount":"18400.00"');
  expect(screen.getByTestId("body").textContent).toContain(description);
  expect(screen.getByTestId("body").textContent).toContain('"min":"20240.00"');
  expect(screen.getByTestId("problems").textContent).toBe("{}");
  expect(sentBody(post)).toMatchObject({current:description,expected:"10% lateral"});
  fireEvent.change(screen.getByLabelText("Current salary breakdown"),{target:{value:"18k"}});
  expect(screen.queryByRole("button",{name:"Use these figures"})).toBeNull();
  expect(screen.getByTestId("problems").textContent).toContain("Calculate and review");
  fireEvent.change(screen.getByLabelText("Amount"), { target: { value: "18000" } });
  expect(screen.getByTestId("problems").textContent).toBe("{}");
  expect(screen.getByTestId("body").textContent).toContain('"amount":"18000"');
});
