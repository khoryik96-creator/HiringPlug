import { expect,it } from "vitest";
import { FormInvalid } from "../forms";
import { emailFieldErrors } from "./emailFieldErrors";

it("maps invalid addresses to their shared input without losing the actual field path",()=>{
 const errors=emailFieldErrors(new FormInvalid({"to.0":"Invalid recipient","cc.2":"Invalid CC",subject:"Required"}));
 expect(errors).toMatchObject({to:"Invalid recipient",cc:"Invalid CC",subject:"Required","to.0":"Invalid recipient"});
 expect(errors.bcc).toBeUndefined();
});
