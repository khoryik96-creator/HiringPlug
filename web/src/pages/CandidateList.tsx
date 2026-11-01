import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";

/** Search and add candidates. */
export function CandidateList({ canCreate }: { canCreate: boolean }) {
  const [q, setQ] = useState("");
  const list = useQuery({
    queryKey: ["candidates", q],
    queryFn: async () =>
      unwrap(await api.GET("/v1/candidates", { params: { query: q.trim() ? { q: q.trim() } : {} } })),
  });

  return (
    <main>
      <div className="row spread">
        <h2>Candidates</h2>
        <input placeholder="Search name or email" value={q} onChange={(e) => { setQ(e.target.value); }} />
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
                <a href={`#/candidates/${c.id}`}>{c.fullName}</a>
              </td>
              <td>{[c.currentTitle, c.currentCompany].filter(Boolean).join(", ")}</td>
              <td>{c.email}</td>
              <td>{c.workAuthorizationStatus?.replace("_", " ")}</td>
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
  const create = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/v1/candidates", {
          body: {
            firstName: form.firstName,
            lastName: form.lastName,
            email: form.email || null,
            phone: form.phone || null,
          },
        }),
      ),
    onSuccess: (c) => {
      void queryClient.invalidateQueries({ queryKey: ["candidates"] });
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
      <button type="submit" disabled={!form.firstName.trim() || create.isPending}>
        Add candidate
      </button>
      <ErrorMessage error={create.error} />
    </form>
  );
}
