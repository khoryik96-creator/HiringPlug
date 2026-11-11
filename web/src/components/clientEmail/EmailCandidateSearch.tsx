import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api, unwrap } from "../../api/client";
import { ErrorMessage } from "../ErrorMessage";
import type { Shortlisted } from "./Shortlist";

/** Search the permitted candidate database independently from the email recipients. */
export function EmailCandidateSearch({ people, onAdd }: { people:Shortlisted[];onAdd:(p:Shortlisted)=>void }) {
 const [text,setText]=useState("");const [query,setQuery]=useState("");
 useEffect(()=>{const timer=setTimeout(()=>{setQuery(text.trim());},250);return ()=>{clearTimeout(timer);};},[text]);
 const found=useQuery({queryKey:["email-candidate-search",query],enabled:query.length>1 && people.length<10,
  queryFn:async()=>unwrap(await api.GET("/v1/candidates",{params:{query:{q:query,limit:10}}})).items});
 const rows=found.data?.filter((c)=>!people.some((p)=>p.candidateId===c.id)) ?? [];
 return <div className="email-candidate-search">
  <label className="field">Add candidates<input type="search" value={text} disabled={people.length>=10} onChange={(e)=>{setText(e.target.value);}} /></label>
  {query.length>1 && <div className="picker-results">
   {found.isFetching && <span className="muted">Searching…</span>}
   {!found.isFetching && rows.map((c)=><button type="button" className="link" key={c.id} onClick={()=>{onAdd({candidateId:c.id,name:c.fullName,title:c.currentTitle});setText("");setQuery("");}}>{c.fullName}{c.currentTitle?` · ${c.currentTitle}`:""}</button>)}
   {!found.isFetching && !found.error && rows.length===0 && <span className="muted">No more matching candidates.</span>}
  </div>}
  <ErrorMessage error={found.error}/>
 </div>;
}
