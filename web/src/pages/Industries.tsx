import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";
import { Field } from "../components/Field";
import { fieldErrors } from "../components/forms";
import { categoryMatches, industryKey, useIndustries, type Category } from "../components/IndustryPicker";
import { filledIn } from "../requiredFields";

type Edit = { item?: Category; parentId?: string };
/** Shared catalogue administration; server checks usage and versions in one transaction. */
export function Industries() {
  const query = useIndustries();
  const [search, setSearch] = useState("");
  const [retired, setRetired] = useState(false);
  const [edit, setEdit] = useState<Edit | null>(null);
  const [remove, setRemove] = useState<Category | null>(null);
  const data = query.data;
  const items = data?.items ?? [];
  const visible = items.filter((i) => retired || !i.retired);
  const roots = visible.filter((i) => !i.parentId && (categoryMatches(i, search) || visible.some((c) => c.parentId === i.id && categoryMatches(c, search))));
  return <main><div className="row spread"><h2>Industries &amp; subindustries</h2><button type="button" onClick={() => { setRemove(null); setEdit({}); }}>+ Add industry</button></div>
    <ErrorMessage error={query.error} />
    {data && <section className="panel" aria-label="Industry catalogue"><div className="row spread"><h3>Your taxonomy</h3><label className="row"><input type="checkbox" checked={retired} onChange={(e) => { setRetired(e.target.checked); }} />Show retired</label></div>
      <label className="field"><span>Search catalogue</span><input type="search" value={search} onChange={(e) => { setSearch(e.target.value); }} /></label>
      {edit && <EditCategory key={edit.item?.id ?? `new-${edit.parentId ?? "root"}`} edit={edit} catalogue={data} onDone={() => { setEdit(null); }} />}
      {remove && <RemoveCategory key={remove.id} item={remove} catalogue={data} onDone={() => { setRemove(null); }} />}
      <div className="industry-admin-wrap"><table className="industry-admin"><thead><tr><th>Industry / subindustry</th><th>In use</th><th>Manage</th></tr></thead><tbody>
        {roots.flatMap((root) => [root, ...visible.filter((c) => c.parentId === root.id && (categoryMatches(root, search) || categoryMatches(c, search)))].map((i) => <tr key={i.id} className={i.parentId ? "" : "parent-row"}>
          <td className={i.parentId ? "child-name" : ""}>{i.label}{i.retired && <small> · Retired</small>}</td><td>{i.candidateCount} profiles<br />{i.companyCount} companies</td><td><div className="row">
            {!i.parentId && !i.retired && <button type="button" className="secondary" onClick={() => { setRemove(null); setEdit({ parentId: i.id }); }}>+ Subindustry</button>}
            <button type="button" className="secondary" onClick={() => { setRemove(null); setEdit({ item: i, parentId: i.parentId }); }}>{i.parentId ? "Move / edit" : "Edit"}</button>
            <button type="button" className="secondary danger" onClick={() => { setEdit(null); setRemove(i); }}>Delete</button>
          </div></td></tr>))}
      </tbody></table></div>
      {roots.length === 0 && <p className="muted">No matching industries.</p>}
    </section>}
  </main>;
}
function useSaved(onDone: () => void) {
  const client = useQueryClient();
  return async (data: Schemas["IndustryCatalogue"]) => {
    client.setQueryData(industryKey, data);
    // Renames and moves update saved summaries and make open editors stale.
    await Promise.all([client.invalidateQueries({ queryKey: ["candidate"] }), client.invalidateQueries({ queryKey: ["company"] }), client.invalidateQueries({ queryKey: ["candidates"] }), client.invalidateQueries({ queryKey: ["companies"] })]);
    onDone();
  };
}
function EditCategory({ edit, catalogue, onDone }: { edit: Edit; catalogue: Schemas["IndustryCatalogue"]; onDone: () => void }) {
  const [label, setLabel] = useState(edit.item?.label ?? "");
  const [parent, setParent] = useState(edit.parentId ?? "");
  const [aliases, setAliases] = useState(edit.item?.aliases.join("\n") ?? "");
  const [retired, setRetired] = useState(edit.item?.retired ?? false);
  const [version] = useState(catalogue.version);
  const saved = useSaved(onDone);
  const save = useMutation({ mutationFn: async () => {
    const body: Schemas["IndustryEdit"] = { catalogueVersion: version, label, aliases: aliases.split("\n").map((s) => s.trim()).filter(Boolean), retired, ...(parent ? { parentId: parent } : {}) };
    return edit.item ? unwrap(await api.PATCH("/v1/industries/{industryId}", { params: { path: { industryId: edit.item.id } }, body })) : unwrap(await api.POST("/v1/industries", { body }));
  }, onSuccess: saved });
  const errors = fieldErrors(save.error);
  const hasChildren = catalogue.items.some((i) => i.parentId === edit.item?.id);
  return <form className="industry-dialog form-grid" aria-label="Edit category" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
    <h3 className="full">{edit.item ? "Edit category" : parent ? "Add subindustry" : "Add industry"}</h3>
    <Field label="Name" error={errors.label}><input value={label} maxLength={100} required onChange={(e) => { setLabel(e.target.value); }} /></Field>
    <Field label="Main industry" error={errors.parentId}><select value={parent} disabled={hasChildren} onChange={(e) => { setParent(e.target.value); }}>
      <option value="">Main industry</option>{catalogue.items.filter((i) => !i.parentId && !i.retired && i.id !== edit.item?.id).map((i) => <option value={i.id} key={i.id}>{i.label}</option>)}
    </select></Field>
    <div className="full"><Field label="Search aliases (one per line)" error={errors.aliases}><textarea rows={3} value={aliases} onChange={(e) => { setAliases(e.target.value); }} /></Field></div>
    {edit.item && <><label className="row full"><input type="checkbox" checked={retired} onChange={(e) => { setRetired(e.target.checked); }} />Retired</label>
      <p className="muted full">{edit.item.candidateCount} profiles and {edit.item.companyCount} companies keep their links. Moving changes their grouping.</p></>}
    <div className="row full"><button type="submit" disabled={save.isPending}>Save category</button><button type="button" className="link" disabled={save.isPending} onClick={onDone}>Cancel</button></div>
    <ErrorMessage error={save.error} />
  </form>;
}
function RemoveCategory({ item, catalogue, onDone }: { item: Category; catalogue: Schemas["IndustryCatalogue"]; onDone: () => void }) {
  const [replacement, setReplacement] = useState("");
  const replacementField = useRef<HTMLSelectElement>(null);
  const [version] = useState(catalogue.version);
  const saved = useSaved(onDone);
  const used = item.candidateCount + item.companyCount > 0;
  const children = catalogue.items.filter((i) => i.parentId === item.id);
  const remove = useMutation({ mutationFn: async (action: "retire" | "delete") => unwrap(await api.POST("/v1/industries/{industryId}/remove", {
    params: { path: { industryId: item.id } }, body: { catalogueVersion: version, action, ...(replacement ? { replacementId: replacement } : {}) },
  })), onSuccess: saved });
  return <section className="industry-dialog" aria-label="Delete category"><h3>Delete {item.label}?</h3>
    <p>{item.candidateCount} profiles · {item.companyCount} companies{children.length ? ` · ${children.length} subindustries` : ""}</p>
    {used && <p>Retire to keep existing selections, or choose where to move them before deletion.</p>}
    <Field label="Replacement" error={fieldErrors(remove.error).replacementId}><select ref={replacementField} required={used} value={replacement} onChange={(e) => { setReplacement(e.target.value); }}>
      <option value="">Choose replacement</option>{catalogue.items.filter((i) => !i.retired && !catalogue.items.find((p) => p.id === i.parentId)?.retired && i.id !== item.id && i.parentId !== item.id).map((i) => <option value={i.id} key={i.id}>{i.parentId ? `${catalogue.items.find((p) => p.id === i.parentId)?.label ?? ""} / ` : ""}{i.label}</option>)}
    </select></Field>
    <div className="row"><button type="button" className="secondary" disabled={remove.isPending} onClick={() => { remove.mutate("retire"); }}>Retire</button>
      <button type="button" className="danger" disabled={remove.isPending} onClick={() => { if (filledIn(replacementField.current)) remove.mutate("delete"); }}>{used ? "Reassign and delete" : "Delete category"}</button>
      <button type="button" className="link" disabled={remove.isPending} onClick={onDone}>Cancel</button></div>
    <ErrorMessage error={remove.error} />
  </section>;
}
