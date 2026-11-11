import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { SalaryCalculationSettings } from "./SalaryCalculationSettings";

afterEach(() => {cleanup();vi.restoreAllMocks();});

it("keeps a visible saved acknowledgement after changed rules are cached", async () => {
  vi.spyOn(api,"GET").mockResolvedValue(ok({allowances:true,bonus:true,bonusOnAllowances:false,cents:false}));
  vi.spyOn(api,"PUT").mockResolvedValue(ok({allowances:true,bonus:false,bonusOnAllowances:false,cents:false}));
  renderWithClient(<SalaryCalculationSettings />);
  const checkbox = await screen.findByLabelText("Include guaranteed bonus");
  fireEvent.click(checkbox);
  fireEvent.click(screen.getByRole("button",{name:"Save calculation rules"}));
  await waitFor(() => {expect(checkbox.isConnected).toBe(false);});
  expect(screen.getByRole("status").textContent).toBe("Saved");
});

it("cannot change controls while a save is pending", async () => {
  vi.spyOn(api,"GET").mockResolvedValue(ok({allowances:true,bonus:true,bonusOnAllowances:false,cents:false}));
  vi.spyOn(api,"PUT").mockImplementation(() => new Promise(() => {}));
  renderWithClient(<SalaryCalculationSettings />);
  const checkbox = await screen.findByLabelText<HTMLInputElement>("Include guaranteed bonus");
  fireEvent.click(checkbox);
  fireEvent.click(screen.getByRole("button",{name:"Save calculation rules"}));
  await waitFor(() => {expect(checkbox.disabled).toBe(true);});
});

it("saves the email salary preference without changing calculation components",async()=>{
 vi.spyOn(api,"GET").mockResolvedValue(ok({allowances:true,bonus:true,bonusOnAllowances:false,cents:false}));
 const put=vi.spyOn(api,"PUT").mockResolvedValue(ok({allowances:true,bonus:true,bonusOnAllowances:false,cents:false,emailSalaryDisplay:"breakdown"}));
 renderWithClient(<SalaryCalculationSettings/>);
 fireEvent.change(await screen.findByLabelText("Salary in emails"),{target:{value:"breakdown"}});
 fireEvent.click(screen.getByRole("button",{name:"Save calculation rules"}));
 await waitFor(()=>{expect(put).toHaveBeenCalled();});expect(sentBody(put)).toMatchObject({emailSalaryDisplay:"breakdown",allowances:true,bonus:true});
});
