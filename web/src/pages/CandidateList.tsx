import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap } from "../api/client";
import { candidatesKey } from "../api/keys";
import { CandidateExtras, extrasBody } from "../components/MandatoryExtras";
import { DuplicateEmail } from "../components/DuplicateEmail";
import { ErrorMessage } from "../components/ErrorMessage";
import { Chips } from "../components/LabelInput";
import { useUsedValues } from "../components/useUsedValues";
import { WORK_AUTH_STATUS } from "../labels";

/** Search and add candidates. */
export function CandidateList({ canCreate }: { canCreate: boolean }) {
  const [q, setQ] = useState("");
  const [tag, setTag] = useState("");
  const [skill, setSkill] = useState("");
  const [mine, setMine] = useState(false);
  const used = useUsedValues();
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
    <main>
      <div className="row spread">
        <h2>Candidates</h2>
        <div className="row">
          <input aria-label="Search candidates" placeholder="Search name or email" value={q} onChange={(e) => { setQ(e.target.value); }} />
          <label className="small"><input type="checkbox" checked={mine} onChange={(e) => { setMine(e.target.checked); }} /> Mine</label>
          <select aria-label="Skill" value={skill} onChange={(e) => { setSkill(e.target.value); }}>
            <option value="">Any skill</option>
            {used.skills.map((s) => <option key={s} value={s}>{s}</option>)}
          </select>
          <select aria-label="Tag" value={tag} onChange={(e) => { setTag(e.target.value); }}>
            <option value="">Any tag</option>
            {used.tags.map((t) => <option key={t} value={t}>{t}</option>)}
          </select>
        </div>
      </div>
      {canCreate && <AddCandidate />}
      <ErrorMessage error={list.error} />
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Current role</th>
            <th>Email</th>
            <th>Work authorization</th>
          </tr>
        </thead>
        <tbody>
          {list.data?.items.map((c) => (
            <tr key={c.id}>
              <td>
                <a href={`#/candidates/${c.id}`}>{c.fullName}</a> {c.tags.length > 0 && <Chips labels={c.tags} />}
              </td>
              <td>{[c.currentTitle, c.currentCompany].filter(Boolean).join(", ")}</td>
              <td>{c.email}</td>
              <td>{c.workAuthorizationStatus ? WORK_AUTH_STATUS[c.workAuthorizationStatus] : ""}</td>
            </tr>
          ))}
        </tbody>
      </table>
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
