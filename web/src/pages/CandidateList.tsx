import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap } from "../api/client";
import { candidatesKey } from "../api/keys";
import { CandidateExtras, extrasBody } from "../components/MandatoryExtras";
import { DuplicateEmail } from "../components/DuplicateEmail";
import { ErrorMessage } from "../components/ErrorMessage";
import { Chips } from "../components/LabelInput";
import { useUsedValues } from "../components/useUsedValues";
import { COLUMNS, ColumnPicker, DEFAULT_COLUMNS, useCandidateColumns, WaBar, WaLegend } from "../components/candidateColumns";
import { hashParam } from "../hashQuery";

/** Search and add candidates. */
export function CandidateList({ canCreate }: { canCreate: boolean }) {
  const [q, setQ] = useState(() => hashParam("q")); // a search from the dashboard
  const [tag, setTag] = useState("");
  const [skill, setSkill] = useState("");
  const [mine, setMine] = useState(false);
  const used = useUsedValues();
  const cols = useCandidateColumns();
  const list = useQuery({
    queryKey: [...candidatesKey, q, tag, skill, mine],
    queryFn: async () => {
      const query: { q?: string; tag?: string; skill?: string; owner?: string } = {};
      if (q.trim()) query.q = q.trim();
      if (tag) query.tag = tag;
      if (skill) query.skill = skill;
      if (mine) query.owner = "me";
      return unwrap(await api.GET("/v1/candidates", { params: { query } }));
    },
  });

  return (
    <main className={cols.columns.length > DEFAULT_COLUMNS.length ? "wide" : undefined}>
      <div className="row spread">
        <h2>Candidates</h2>
        <div className="row">
          <input aria-label="Search candidates" placeholder="Search" value={q} onChange={(e) => { setQ(e.target.value); }} />
          <label className="small"><input type="checkbox" checked={mine} onChange={(e) => { setMine(e.target.checked); }} /> Mine</label>
          <select aria-label="Skill" value={skill} onChange={(e) => { setSkill(e.target.value); }}>
            <option value="">Any skill</option>
            {used.skills.map((s) => <option key={s} value={s}>{s}</option>)}
          </select>
          <select aria-label="Tag" value={tag} onChange={(e) => { setTag(e.target.value); }}>
            <option value="">Any tag</option>
            {used.tags.map((t) => <option key={t} value={t}>{t}</option>)}
          </select>
          <ColumnPicker columns={cols.columns} chosen={cols.chosen} onSave={(c) => { cols.save.mutate(c); }} />
        </div>
      </div>
      {canCreate && <AddCandidate />}
      <ErrorMessage error={list.error ?? cols.save.error} />
      <WaLegend />
      <div className="table-scroll">
        <table className="candidates-table">
          <thead>
            <tr>
              <th>Name</th>
              {cols.columns.map((col) => <th key={col}>{COLUMNS[col].header}</th>)}
            </tr>
          </thead>
          <tbody>
            {list.data?.items.map((c) => (
              <tr key={c.id}>
                <td className="name-cell">
                  <WaBar c={c} />
                  <a href={`#/candidates/${c.id}`}>{c.fullName}</a>{" "}
                  {!cols.columns.includes("tags") && c.tags.length > 0 && <Chips labels={c.tags} />}
                </td>
                {cols.columns.map((col) => <td key={col} className={`col-${col}`}>{COLUMNS[col].cell(c, cols.look)}</td>)}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </main>
  );
}

function AddCandidate() {
  const queryClient = useQueryClient();
  const [form, setForm] = useState({ firstName: "", lastName: "", email: "", phone: "" });
  const [extras, setExtras] = useState<Record<string, string>>({});
  const create = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/v1/candidates", {
          body: {
            firstName: form.firstName,
            lastName: form.lastName,
            email: form.email || null,
            phone: form.phone || null,
            ...extrasBody(extras),
          },
        }),
      ),
    onSuccess: (c) => {
      void queryClient.invalidateQueries({ queryKey: candidatesKey });
      window.location.hash = `#/candidates/${c.id}`;
    },
  });
  const field = (name: keyof typeof form, label: string) => (
    <input
      aria-label={label}
      placeholder={label}
      value={form[name]}
      onChange={(e) => { setForm({ ...form, [name]: e.target.value }); }}
    />
  );
  return (
    <form
      className="row panel"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate();
      }}
    >
      {field("firstName", "First name")}
      {field("lastName", "Last name")}
      {field("email", "Email")}
      {field("phone", "Phone")}
      <CandidateExtras values={extras} onChange={setExtras} />
      <button type="submit" disabled={!form.firstName.trim() || create.isPending}>
        Add candidate
      </button>
      <ErrorMessage error={create.error}><DuplicateEmail error={create.error} /></ErrorMessage>
    </form>
  );
}
