// A pretend API for the WhatsApp window's tests: templates kept in memory (so
// adding, changing and deleting show in the lists), a candidate's draft with
// their field values, and the signed-in person's permissions.
import { vi } from "vitest";

import { api } from "../api/client";
import { failure, ok } from "../test-utils";

type Template = { id: string; version: number; name: string; body: string; createdAt: string; updatedAt: string };

export const templateFields = [
  { key: "candidate.firstName", label: "First name" },
  { key: "candidate.fullName", label: "Full name" },
  { key: "candidate.nationality", label: "Nationality" },
  { key: "job.title", label: "Job title" },
  { key: "company.name", label: "Client company" },
  { key: "sender.name", label: "Your name" },
];
const values: Record<string, string> = {
  "candidate.firstName": "Siti", "candidate.fullName": "Siti Aminah", "candidate.nationality": "",
  "job.title": "Finance Manager", "company.name": "Contoh Berhad", "sender.name": "Nur Rekrut",
};
export const draftFields = templateFields.map((f) => ({ ...f, value: values[f.key] ?? "" }));

const applications = { items: [{ id: "appl_1", jobId: "job_1", jobTitle: "Finance Manager", companyName: "Contoh Berhad" }], nextCursor: null };
const numbers = [{ label: "Phone", number: "+60123456789", chatNumber: "60123456789" }];

export function mockWhatsAppApi(permissions: string[], { takenName }: { takenName?: string } = {}) {
  const store: Template[] = [{ id: "wtpl_1", version: 1, name: "Introduce a role", body: "Hi {{candidate.firstName}}, {{job.title}}?", createdAt: "", updatedAt: "" }];
  const get = vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
    path === "/v1/me" ? { permissions }
      : path === "/v1/whatsapp-templates" ? { items: [...store], placeholders: templateFields.map((f) => f.key), fields: templateFields }
        : applications,
  ))) as never);
  const post = vi.spyOn(api, "POST").mockImplementation(((path: string, init: { body: Record<string, string> }) => {
    if (path === "/v1/whatsapp-templates") {
      if (init.body.name === takenName) return Promise.resolve(failure("VALIDATION_FAILED", "A WhatsApp template with this name already exists."));
      const t = { id: `wtpl_${String(store.length + 1)}`, version: 1, name: init.body.name ?? "", body: init.body.body ?? "", createdAt: "", updatedAt: "" };
      store.push(t);
      return Promise.resolve(ok(t));
    }
    if (path === "/v1/notes") return Promise.resolve(ok({ id: "note_1" }));
    const chosen = store.find((t) => t.id === init.body.templateId);
    return Promise.resolve(ok({ text: chosen ? `From ${chosen.name}` : "", missing: [], numbers, fields: draftFields }));
  }) as never);
  const put = vi.spyOn(api, "PUT").mockImplementation(((_path: string, init: { params: { path: { templateId: string } }; body: { name: string; body: string } }) => {
    const t = store.find((x) => x.id === init.params.path.templateId);
    if (!t) return Promise.resolve(failure("NOT_FOUND", "Not found.", undefined, 404));
    Object.assign(t, { name: init.body.name, body: init.body.body, version: t.version + 1 });
    return Promise.resolve(ok({ ...t }));
  }) as never);
  const del = vi.spyOn(api, "DELETE").mockImplementation(((_path: string, init: { params: { path: { templateId: string } } }) => {
    store.splice(store.findIndex((x) => x.id === init.params.path.templateId), 1);
    return Promise.resolve({ data: undefined, error: undefined, response: new Response(null, { status: 204 }) });
  }) as never);
  return { get, post, put, del, store };
}

/** jsdom has no modal dialogs: open and close just set `open`. */
export function allowDialogs() {
  if (typeof HTMLDialogElement.prototype.showModal !== "function") {
    Object.defineProperty(HTMLDialogElement.prototype, "showModal", { configurable: true, value(this: HTMLDialogElement) { this.open = true; } });
  }
  if (typeof HTMLDialogElement.prototype.close !== "function") {
    Object.defineProperty(HTMLDialogElement.prototype, "close", { configurable: true, value(this: HTMLDialogElement) { this.open = false; } });
  }
}
