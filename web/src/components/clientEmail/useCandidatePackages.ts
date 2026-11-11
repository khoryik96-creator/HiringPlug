import { useState } from "react";
import type { Schemas } from "../../api/client";
import type { Shortlisted } from "./Shortlist";

export type Insertion = { id: number; html: string; managed?: boolean; removeIds?:string[]; order?:string[] };
export const hasCandidateDetails = (html: string) => html.includes('data-candidate-summary="true"');

/** Own one replaceable candidate section while preserving the rest of the writer's message. */
export function useCandidatePackages(initial: Shortlisted[] = []) {
 const [people,setPeople]=useState(initial);
 const [inserting,setInserting]=useState<Insertion>();
 const [notice,setNotice]=useState("");
 const clear = () => {setNotice("Update candidate details in the message to apply the refreshed fields.");};
 const change = (next: Shortlisted[]) => {
  const removed=people.filter((p)=>!next.some((n)=>n.candidateId===p.candidateId)).map((p)=>p.candidateId);
  const order=next.map((p)=>p.candidateId);
  if(JSON.stringify(order)!==JSON.stringify(people.map((p)=>p.candidateId))) {
   setInserting((v)=>({id:(v?.id ?? 0)+1,html:"",managed:true,removeIds:removed,order}));
   setNotice("Selection updated. Insert or update candidate details in the message.");
  }
  setPeople(next);
 };
 const insert = (html:string,snapshot:Shortlisted[]) => {
  setPeople(snapshot);setNotice("");
  setInserting((v)=>({id:(v?.id ?? 0)+1,html:`<div data-candidate-summary="true">${html}</div>`,managed:true}));
 };
 const references=(html:string):Schemas["EmailCandidate"][]=>people.map((p)=>({candidateId:p.candidateId,...(p.applicationId?{applicationId:p.applicationId}:{}),...(hasCandidateDetails(html)?{
  ...(p.candidateVersion!==undefined?{candidateVersion:p.candidateVersion}:{}),...(p.compensationVersion!==undefined?{compensationVersion:p.compensationVersion}:{})
 }: {})}));
 const insertLoose=(html:string)=>{setInserting((v)=>({id:(v?.id ?? 0)+1,html}));};
 return {people,change,insert,clear,inserting,notice,references,insertLoose};
}
