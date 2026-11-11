import { fieldErrors } from "../forms";

/** Address lists use one control; the contract reports an invalid address as to.0. */
export function emailFieldErrors(error:unknown):Record<string,string> {
 const fields=fieldErrors(error);
 for(const [path,message] of Object.entries(fields)) {
  const root=/^(to|cc|bcc)\.\d+$/.exec(path)?.[1];
  if(root)fields[root]=fields[root]?`${fields[root]}; ${message}`:message;
 }
 return fields;
}
