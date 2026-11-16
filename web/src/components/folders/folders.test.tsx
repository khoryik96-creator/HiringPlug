import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type ContactSummary } from "../../api/client";
import { ContactList } from "../../pages/ContactList";
import { ok, renderWithClient, testFieldSettings } from "../../test-utils";
import { AddToFolder } from "./AddToFolder";
import { FolderHead } from "./FolderHead";
import type { Folder } from "./useFolders";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

const ME = "usr_01jbz6w0m0k8q9x2r3s4t5v6w7";
const AINA = "usr_01jbz6w0m0k8q9x2r3s4t5v6a1";
const BEN = "usr_01jbz6w0m0k8q9x2r3s4t5v6b2";
const users = { items: [
  { id: ME, name: "Demo Owner", role: "owner", canInterview: true },
  { id: AINA, name: "Aina Ismail", role: "recruiter", canInterview: true },
  { id: BEN, name: "Ben Lim", role: "sourcer", canInterview: false },
] };
const folder = (over: Partial<Folder>): Folder => ({
  id: "fld_01jbz6w0m0k8q9x2r3s4t5v6f1", kind: "contact", name: "HR in KL", ownerId: ME, mine: true, sharedWith: [], itemCount: 2,
  version: 1, createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z", ...over,
});
const contact = (id: string, fullName: string): ContactSummary => ({
  id, fullName, title: "HR Manager", companyId: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7", companyName: "Maju", email: null, emailCheckStatus: null,
  mailboxStatus: null, mailboxCheckedAt: null, linkedinEmployment: null, phone: null, mobile: null, status: "active", role: "hr", updatedAt: "2026-11-01T00:00:00Z",
});
const MEI = contact("cont_01jbz6w0m0k8q9x2r3s4t5v6c1", "Mei Tan");
const ALI = contact("cont_01jbz6w0m0k8q9x2r3s4t5v6c2", "Ali Hassan");

const gone = () => ({ data: undefined, error: undefined, response: new Response(null, { status: 204 }) }) as never;

function serve(folders: Folder[] | (() => Folder[])) {
  const listed: Record<string, unknown>[] = [];
  vi.spyOn(api, "GET").mockImplementation(((path: string, init?: { params?: { query?: Record<string, unknown> } }) => {
    if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
    if (path === "/v1/folders") return Promise.resolve(ok({ items: typeof folders === "function" ? folders() : folders }));
    if (path === "/v1/users") return Promise.resolve(ok(users));
    const query = init?.params?.query ?? {};
    listed.push(query);
    return Promise.resolve(ok({ nextCursor: null, items: query.folder ? [MEI] : [MEI, ALI] }));
  }) as never);
  return listed;
}

describe("folders on a list", () => {
  it("lists the person's folders and those shared with them, and opens one", async () => {
    const listed = serve([folder({}), folder({ id: "fld_01jbz6w0m0k8q9x2r3s4t5v6f2", name: "Q4 clients", mine: false, ownerId: AINA, itemCount: 8 })]);
    vi.spyOn(api, "POST").mockResolvedValue(ok({ items: [] }));
    renderWithClient(<ContactList canCreate={false} canFolders meId={ME} />);
    const panel = await screen.findByRole("navigation", { name: "Folders" });
    expect(within(panel).getByRole("button", { name: "All contacts" }).getAttribute("aria-current")).toBe("true");
    expect((await within(panel).findByRole("button", { name: /HR in KL/ })).textContent).toContain("2");
    await within(panel).findByText("from Aina Ismail");

    fireEvent.click(within(panel).getByRole("button", { name: /HR in KL/ }));
    await waitFor(() => { expect(listed.at(-1)).toEqual({ limit: 100, folder: "fld_01jbz6w0m0k8q9x2r3s4t5v6f1" }); });
    expect(await screen.findByRole("heading", { name: "HR in KL" })).toBeTruthy();
    expect(screen.getByText(/2 contacts · yours · only you see it/)).toBeTruthy();
    expect(window.localStorage.getItem("hiringplug.folder.contact")).toBe("fld_01jbz6w0m0k8q9x2r3s4t5v6f1"); // opened again next time
  });

  it("chooses rows to put in a folder, without email for those who may not send it", async () => {
    serve([folder({})]);
    vi.spyOn(api, "POST").mockResolvedValue(ok({ items: [] }));
    renderWithClient(<ContactList canCreate={false} canFolders meId={ME} />);
    const row = (await screen.findByRole("link", { name: "Ali Hassan" })).closest("tr");
    if (!row) throw new Error("row");
    fireEvent.click(within(row).getByText("HR Manager"));
    expect(await screen.findByText("1 chosen")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Add to folder ▾" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Email each separately" })).toBeNull();
    expect(row.querySelector("td")?.getAttribute("draggable")).toBe("true"); // and dragged onto a folder
  });

  it("unchooses rows once they are taken out of the folder, or another folder opens", async () => {
    window.localStorage.setItem("hiringplug.folder.contact", "fld_01jbz6w0m0k8q9x2r3s4t5v6f1");
    serve([folder({}), folder({ id: "fld_01jbz6w0m0k8q9x2r3s4t5v6f2", name: "Q4 clients" })]);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(folder({ itemCount: 1 })));
    renderWithClient(<ContactList canCreate={false} canFolders meId={ME} />);
    const row = (await screen.findByRole("link", { name: "Mei Tan" })).closest("tr");
    if (!row) throw new Error("row");
    fireEvent.click(within(row).getByText("HR Manager"));
    fireEvent.click(await screen.findByRole("button", { name: "Remove from this folder" }));
    await waitFor(() => {
      expect(post).toHaveBeenCalledWith("/v1/folders/{folderId}/items/remove", { params: { path: { folderId: "fld_01jbz6w0m0k8q9x2r3s4t5v6f1" } }, body: { itemIds: [MEI.id] } });
    });
    await waitFor(() => { expect(screen.queryByText("1 chosen")).toBeNull(); });

    fireEvent.click(within(row).getByText("HR Manager"));
    expect(await screen.findByText("1 chosen")).toBeTruthy();
    fireEvent.click(within(screen.getByRole("navigation", { name: "Folders" })).getByRole("button", { name: /Q4 clients/ }));
    await waitFor(() => { expect(screen.queryByText("1 chosen")).toBeNull(); });
  });

  it("unchooses rows unticked from the open folder in Add to folder", async () => {
    window.localStorage.setItem("hiringplug.folder.contact", "fld_01jbz6w0m0k8q9x2r3s4t5v6f1");
    serve([folder({})]);
    const post = vi.spyOn(api, "POST").mockImplementation(((path: string) => Promise.resolve(path === "/v1/folders/memberships"
      ? ok({ items: [{ folderId: "fld_01jbz6w0m0k8q9x2r3s4t5v6f1", itemIds: [MEI.id] }] })
      : ok(folder({ itemCount: 0 })))) as never);
    renderWithClient(<ContactList canCreate={false} canFolders meId={ME} />);
    const row = (await screen.findByRole("link", { name: "Mei Tan" })).closest("tr");
    if (!row) throw new Error("row");
    fireEvent.click(within(row).getByText("HR Manager"));
    fireEvent.click(await screen.findByRole("button", { name: "Add to folder ▾" }));
    const open = await screen.findByRole<HTMLInputElement>("checkbox", { name: /HR in KL/ });
    await waitFor(() => { expect(open.checked).toBe(true); });
    fireEvent.click(open);
    await waitFor(() => {
      expect(post).toHaveBeenCalledWith("/v1/folders/{folderId}/items/remove", { params: { path: { folderId: "fld_01jbz6w0m0k8q9x2r3s4t5v6f1" } }, body: { itemIds: [MEI.id] } });
    });
    await waitFor(() => { expect(screen.queryByText("1 chosen")).toBeNull(); });
  });

  it("asks which folders hold the rows shown in groups, not again for rows already asked about", async () => {
    const many = Array.from({ length: 60 }, (_, i) => contact(`cont_${String(i).padStart(26, "0")}`, `Person ${String(i)}`));
    vi.spyOn(api, "GET").mockImplementation(((path: string, init?: { params?: { query?: Record<string, unknown> } }) => {
      if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
      if (path === "/v1/folders") return Promise.resolve(ok({ items: [folder({})] }));
      if (path === "/v1/users") return Promise.resolve(ok(users));
      return Promise.resolve(ok(init?.params?.query?.cursor
        ? { nextCursor: null, items: many.slice(50) }
        : { nextCursor: "cont_00000000000000000000000049", items: many.slice(0, 50) }));
    }) as never);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ items: [] }));
    renderWithClient(<ContactList canCreate={false} canFolders meId={ME} />);
    await screen.findByRole("link", { name: "Person 0" });
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(1); });
    fireEvent.click(screen.getByRole("button", { name: "Show more" }));
    await screen.findByRole("link", { name: "Person 59" });
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(2); });
    const asked = post.mock.calls.map((c) => (c[1] as { body: { itemIds: string[] } }).body.itemIds.length);
    expect(asked).toEqual([50, 10]); // the second page only
  });

  it("refuses more than 500 dropped on a folder at once", async () => {
    serve([folder({})]);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ items: [] }));
    renderWithClient(<ContactList canCreate={false} canFolders meId={ME} />);
    const target = await within(await screen.findByRole("navigation", { name: "Folders" })).findByRole("button", { name: /HR in KL/ });
    const many = Array.from({ length: 501 }, (_, i) => `cont_${String(i).padStart(26, "0")}`);
    const dataTransfer = { types: ["application/x-hiringplug-records"], getData: () => JSON.stringify(many), dropEffect: "" };
    fireEvent.drop(target, { dataTransfer });
    expect((await screen.findByRole("alert")).textContent).toContain("At most 500 at a time");
    expect(post).not.toHaveBeenCalledWith("/v1/folders/{folderId}/items", expect.anything());
  });

  it("makes a new folder and opens it", async () => {
    const made = folder({ id: "fld_01jbz6w0m0k8q9x2r3s4t5v6f9", name: "Warm leads", itemCount: 0 });
    const folders: Folder[] = [];
    serve(() => folders);
    const post = vi.spyOn(api, "POST").mockImplementation((() => { folders.push(made); return Promise.resolve(ok(made)); }) as never);
    renderWithClient(<ContactList canCreate={false} canFolders meId={ME} />);
    const panel = await screen.findByRole("navigation", { name: "Folders" });
    expect(await within(panel).findByText(/make a folder, then drag rows onto it/)).toBeTruthy();
    fireEvent.click(within(panel).getByRole("button", { name: "+ New folder" }));
    fireEvent.change(within(panel).getByLabelText("New folder name"), { target: { value: "  Warm leads " } });
    fireEvent.click(within(panel).getByRole("button", { name: "Create" }));
    await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/folders", { body: { kind: "contact", name: "Warm leads" } }); });
    expect(await screen.findByRole("heading", { name: "Warm leads" })).toBeTruthy();
    expect(window.localStorage.getItem("hiringplug.folder.contact")).toBe("fld_01jbz6w0m0k8q9x2r3s4t5v6f9");
  });

  it("shows everything when a remembered folder is gone", async () => {
    window.localStorage.setItem("hiringplug.folder.contact", "fld_01jbz6w0m0k8q9x2r3s4t5v6zz");
    const listed = serve([folder({})]);
    renderWithClient(<ContactList canCreate={false} canFolders meId={ME} />);
    expect(await screen.findByRole("link", { name: "Ali Hassan" })).toBeTruthy();
    expect(listed).toEqual([{ limit: 100 }]); // never asked for the folder that is gone
    await waitFor(() => { expect(window.localStorage.getItem("hiringplug.folder.contact")).toBeNull(); });
  });
});

describe("an open folder", () => {
  it("lets its owner rename, share and delete it", async () => {
    serve([]);
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(folder({ name: "HR in Penang", version: 2 })));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(folder({ sharedWith: [AINA] })));
    const del = vi.spyOn(api, "DELETE").mockImplementation(((path: string) => Promise.resolve(path === "/v1/folders/{folderId}" ? gone() : ok({ folder: folder({}) }))) as never);
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    const onGone = vi.fn();
    renderWithClient(<FolderHead kind="contact" folder={folder({ sharedWith: [BEN] })} meId={ME} onGone={onGone} />);

    fireEvent.click(screen.getByRole("button", { name: "Rename" }));
    fireEvent.change(screen.getByLabelText("Folder name"), { target: { value: "HR in Penang" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => {
      expect(patch).toHaveBeenCalledWith("/v1/folders/{folderId}", { params: { path: { folderId: "fld_01jbz6w0m0k8q9x2r3s4t5v6f1" } }, body: { name: "HR in Penang", version: 1 } });
    });

    fireEvent.click(screen.getByRole("button", { name: "Share" }));
    const dialog = screen.getByRole("dialog", { name: "Share HR in KL" });
    await within(dialog).findByText("Ben Lim");
    expect(within(dialog).getByText("Demo Owner (you)")).toBeTruthy();
    const options = within(dialog).getAllByRole("option").map((o) => o.textContent);
    expect(options).toEqual(["Choose someone of your agency", "Aina Ismail"]); // not the owner, not Ben again
    fireEvent.change(within(dialog).getByLabelText("Someone to share with"), { target: { value: AINA } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Share" }));
    await waitFor(() => {
      expect(put).toHaveBeenCalledWith("/v1/folders/{folderId}/shares/{userId}", { params: { path: { folderId: "fld_01jbz6w0m0k8q9x2r3s4t5v6f1", userId: AINA } } });
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "Remove Ben Lim" }));
    await waitFor(() => {
      expect(del).toHaveBeenCalledWith("/v1/folders/{folderId}/shares/{userId}", { params: { path: { folderId: "fld_01jbz6w0m0k8q9x2r3s4t5v6f1", userId: BEN } } });
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "Done" }));

    fireEvent.click(screen.getByRole("button", { name: "Delete folder" }));
    expect(confirm).toHaveBeenCalledWith('Delete the folder "HR in KL"? The contacts in it stay as they are.');
    await waitFor(() => { expect(onGone).toHaveBeenCalled(); });
    expect(del).toHaveBeenCalledWith("/v1/folders/{folderId}", { params: { path: { folderId: "fld_01jbz6w0m0k8q9x2r3s4t5v6f1" } } });
  });

  it("lets those it is shared with only leave it", async () => {
    serve([]);
    const del = vi.spyOn(api, "DELETE").mockResolvedValue(ok({ folder: null }));
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const onGone = vi.fn();
    renderWithClient(<FolderHead kind="contact" folder={folder({ mine: false, ownerId: AINA, sharedWith: [] })} meId={ME} onGone={onGone} />);
    expect(await screen.findByText(/from Aina Ismail · you can add and remove/)).toBeTruthy();
    for (const name of ["Share", "Rename", "Delete folder"]) expect(screen.queryByRole("button", { name })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Leave folder" }));
    await waitFor(() => { expect(onGone).toHaveBeenCalled(); });
    expect(del).toHaveBeenCalledWith("/v1/folders/{folderId}/shares/{userId}", { params: { path: { folderId: "fld_01jbz6w0m0k8q9x2r3s4t5v6f1", userId: ME } } });
  });
});

describe("Add to folder", () => {
  const two = ["cont_01jbz6w0m0k8q9x2r3s4t5v6c1", "cont_01jbz6w0m0k8q9x2r3s4t5v6c2"];
  function serveMenu() {
    serve([folder({}), folder({ id: "fld_01jbz6w0m0k8q9x2r3s4t5v6f2", name: "Q4 clients", itemCount: 0 }), folder({ id: "fld_01jbz6w0m0k8q9x2r3s4t5v6f3", name: "Both", itemCount: 2 })]);
    return vi.spyOn(api, "POST").mockImplementation(((path: string) => {
      if (path === "/v1/folders/memberships") {
        return Promise.resolve(ok({ items: [
          { folderId: "fld_01jbz6w0m0k8q9x2r3s4t5v6f1", itemIds: [two[0]] },
          { folderId: "fld_01jbz6w0m0k8q9x2r3s4t5v6f3", itemIds: two },
        ] }));
      }
      if (path === "/v1/folders") return Promise.resolve(ok(folder({ id: "fld_01jbz6w0m0k8q9x2r3s4t5v6f4", name: "Warm", itemCount: 0 })));
      return Promise.resolve(ok(folder({})));
    }) as never);
  }

  it("ticks the folders that hold all the chosen, half-ticks those holding some", async () => {
    const post = serveMenu();
    renderWithClient(<AddToFolder kind="contact" ids={two} />);
    fireEvent.click(screen.getByRole("button", { name: "Add to folder ▾" }));
    const both = await screen.findByRole<HTMLInputElement>("checkbox", { name: /Both/ });
    await waitFor(() => { expect(both.checked).toBe(true); });
    const some = screen.getByRole<HTMLInputElement>("checkbox", { name: /HR in KL/ });
    expect(some.checked).toBe(false);
    expect(some.indeterminate).toBe(true);

    fireEvent.click(some);
    await waitFor(() => {
      expect(post).toHaveBeenCalledWith("/v1/folders/{folderId}/items", { params: { path: { folderId: "fld_01jbz6w0m0k8q9x2r3s4t5v6f1" } }, body: { itemIds: two } });
    });
    fireEvent.click(both);
    await waitFor(() => {
      expect(post).toHaveBeenCalledWith("/v1/folders/{folderId}/items/remove", { params: { path: { folderId: "fld_01jbz6w0m0k8q9x2r3s4t5v6f3" } }, body: { itemIds: two } });
    });
  });

  it("makes a new folder with the chosen in it", async () => {
    const post = serveMenu();
    renderWithClient(<AddToFolder kind="contact" ids={two} />);
    fireEvent.click(screen.getByRole("button", { name: "Add to folder ▾" }));
    fireEvent.change(screen.getByLabelText("Find or name a new folder"), { target: { value: "Warm" } });
    expect(screen.queryByRole("checkbox", { name: /HR in KL/ })).toBeNull(); // the list narrows to the name typed
    fireEvent.click(screen.getByRole("button", { name: "+ New folder “Warm”" }));
    await waitFor(() => {
      expect(post).toHaveBeenCalledWith("/v1/folders/{folderId}/items", { params: { path: { folderId: "fld_01jbz6w0m0k8q9x2r3s4t5v6f4" } }, body: { itemIds: two } });
    });
    expect(post).toHaveBeenCalledWith("/v1/folders", { body: { kind: "contact", name: "Warm" } });
  });
});
