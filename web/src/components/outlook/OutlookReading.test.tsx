import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../../api/client";
import { OutlookMail } from "../../pages/OutlookMail";
import { ok, renderWithClient, sentBody } from "../../test-utils";
import { frameDocument } from "./OutlookMessageFrame";
import { folderTree, listDate } from "./OutlookMessageList";
import { initials } from "./OutlookReadingPane";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.location.hash = "";
});

const message = (over: Partial<Schemas["OutlookMessage"]> = {}): Schemas["OutlookMessage"] => ({
  id: "omg_one", folderId: "omf_inbox", conversation: "thread", subject: "Re: Hypies | Profiles for SME Automation", sender: "natasha@hyppies.test",
  senderName: "Leander Natasha", flagged: false, participants: [], receivedAt: "2026-11-19T06:49:00Z", isRead: false, isDraft: false,
  hasAttachments: false, webUrl: "https://outlook.office.com/mail/item", ...over,
});

const folders: Schemas["OutlookMailFolder"][] = [
  { id: "omf_inbox", name: "Inbox", kind: "inbox", parentId: null, unread: 3 },
  { id: "omf_drafts", name: "Drafts", kind: "drafts", parentId: null, unread: 0 },
  { id: "omf_deleted", name: "Deleted Items", kind: "deleteditems", parentId: null, unread: 0 },
  { id: "omf_archive", name: "Archive", kind: "archive", parentId: null, unread: 0 },
  { id: "omf_clients", name: "Clients", kind: "", parentId: null, unread: 0 },
  { id: "omf_acme", name: "Acme", kind: "", parentId: "omf_clients", unread: 1 },
];

const body: Schemas["OutlookMessageBody"] = {
  text: "Hi Samuel & Henk,", truncated: false, html: `<p>Hi <b>Samuel</b> &amp; Henk,</p><img data-hp-src="https://hyppies.test/logo.png" alt="Hyppies">`,
  blockedImages: 1, from: { name: "Leander Natasha", address: "natasha@hyppies.test" },
  to: [{ name: "Samuel", address: "samuel@client.test" }], cc: [{ name: "", address: "henk@client.test" }],
  sentAt: "2026-11-19T06:49:00Z", importance: "high",
};

function serve(items: Schemas["OutlookMessage"][]) {
  vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
    path === "/v1/outlook/mail" ? ok({ connected: true, enabled: true, syncing: false, lastSyncAt: null, error: null, folders, items, nextOffset: null })
      : ok(body),
  ));
  return vi.spyOn(api, "PATCH").mockResolvedValue({ response: new Response(null, { status: 204 }) });
}

describe("Outlook Mail reads like Outlook", () => {
  it("lists a message's attachments once, when its body arrives after them", async () => {
    // Two parts of the pane sharing a key: React draws the attachments twice in a production build.
    const warn = vi.spyOn(console, "error").mockImplementation(() => undefined);
    let arrive: (() => void) | undefined;
    vi.spyOn(api, "GET").mockImplementation((path) => (
      path === "/v1/outlook/mail" ? Promise.resolve(ok({ connected: true, enabled: true, syncing: false, lastSyncAt: null, error: null, folders, items: [message({ hasAttachments: true })], nextOffset: null }))
        : path === "/v1/outlook/messages/{messageId}/attachments" ? Promise.resolve(ok({ items: [{ id: "att", fileName: "Letter.pdf", sizeBytes: 900, downloadable: true, savable: true }], more: false }))
          : new Promise((resolve) => { arrive = () => { resolve(ok(body)); }; })
    ) as never);
    vi.spyOn(api, "PATCH").mockResolvedValue({ response: new Response(null, { status: 204 }) });
    renderWithClient(<OutlookMail />);
    fireEvent.click(await screen.findByRole("button", { name: /Profiles for SME Automation/ }));
    const pane = await screen.findByRole("region", { name: "Outlook message" });
    await within(pane).findByText("Letter.pdf");
    await waitFor(() => { expect(arrive).toBeDefined(); });
    arrive?.();
    await within(pane).findByTitle(/^Message: /);
    expect(within(pane).getAllByRole("region", { name: "Attachments" })).toHaveLength(1);
    expect(within(pane).getAllByText("Letter.pdf")).toHaveLength(1);
    expect(warn.mock.calls.map((c) => String(c[0])).filter((m) => m.includes("same key"))).toEqual([]);
  });

  it("shows who it is from and to, and the message in its own formatting with its pictures", async () => {
    const patch = serve([message()]);
    renderWithClient(<OutlookMail />);
    fireEvent.click(await screen.findByRole("button", { name: /Profiles for SME Automation/ }));
    const pane = await screen.findByRole("region", { name: "Outlook message" });
    expect(await within(pane).findByText("Leander Natasha")).toBeTruthy();
    expect(within(pane).getByText("<natasha@hyppies.test>", { exact: false })).toBeTruthy();
    expect(within(pane).getByText("Samuel")).toBeTruthy();
    expect(within(pane).getByText("henk@client.test")).toBeTruthy();
    expect(within(pane).getByLabelText("High importance")).toBeTruthy();
    const frame = within(pane).getByTitle<HTMLIFrameElement>(/^Message: /);
    expect(frame.getAttribute("sandbox")).not.toContain("allow-scripts");
    expect(frame.getAttribute("srcdoc")).toContain("<b>Samuel</b>");
    expect(frame.getAttribute("srcdoc")).toContain(`src="https://hyppies.test/logo.png"`); // pictures from the web load, as in Outlook
    expect(frame.getAttribute("srcdoc")).toContain(`<meta name="referrer" content="no-referrer">`);
    expect(within(pane).queryByRole("button", { name: "Show pictures" })).toBeNull();
    // Opened: marked read, as Outlook does.
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toEqual({ isRead: true });
    // The Move dropdown is gone; Reply, Reply all and Forward sit beside the sender, named in tooltips.
    expect(within(pane).queryByLabelText("Destination folder")).toBeNull();
    expect(within(within(pane).getByRole("toolbar", { name: "Respond" })).getByRole("button", { name: "Reply all" }).dataset.tip).toBe("Reply all");
  });

  it("deletes into Deleted Items from the command bar and opens the next message", async () => {
    const patch = serve([message({ isRead: true }), message({ id: "omg_two", subject: "Head of AIOps", isRead: true })]);
    renderWithClient(<OutlookMail />);
    fireEvent.click(await screen.findByRole("button", { name: /Profiles for SME Automation/ }));
    const bar = screen.getByRole("toolbar", { name: "Message commands" });
    fireEvent.click(within(bar).getByRole("button", { name: "Delete" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(patch.mock.calls[0]?.[1]).toMatchObject({ params: { path: { messageId: "omg_one" } }, body: { folderId: "omf_deleted" } });
    expect(await screen.findByRole("heading", { name: /Head of AIOps/ })).toBeTruthy();
  });

  it("flags and moves a message beside its date, and marks it unread from the command bar", async () => {
    const patch = serve([message({ isRead: true })]);
    renderWithClient(<OutlookMail />);
    const list = await screen.findByLabelText("Messages");
    fireEvent.click(await within(list).findByRole("button", { name: "Flag message" }));
    await waitFor(() => { expect(sentBody(patch)).toEqual({ flagged: true }); });
    const menu = within(list).getByRole("menu", { name: "Move to folder" });
    expect(within(menu).queryByRole("menuitem", { name: "Inbox" })).toBeNull();
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Acme" }));
    await waitFor(() => { expect(sentBody(patch, 1)).toEqual({ folderId: "omf_acme" }); });
  });

  it("marks a message unread from the command bar, which holds no flag or move", async () => {
    const patch = serve([message({ isRead: true })]);
    renderWithClient(<OutlookMail />);
    fireEvent.click(await screen.findByRole("button", { name: /Profiles for SME Automation/ }));
    const bar = screen.getByRole("toolbar", { name: "Message commands" });
    expect(within(bar).queryByRole("button", { name: "Flag" })).toBeNull();
    expect(within(bar).queryByRole("menu")).toBeNull();
    fireEvent.click(within(bar).getByRole("button", { name: "Mark as unread" }));
    await waitFor(() => { expect(sentBody(patch)).toEqual({ isRead: false }); });
  });

  it("holds back pictures from the web in Junk Email until asked", async () => {
    vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
      path === "/v1/outlook/mail" ? ok({ connected: true, enabled: true, syncing: false, lastSyncAt: null, error: null,
        folders: [...folders, { id: "omf_junk", name: "Junk Email", kind: "junkemail", parentId: null, unread: 0 }],
        items: [message({ folderId: "omf_junk", isRead: true })], nextOffset: null }) : ok(body),
    ));
    renderWithClient(<OutlookMail />);
    fireEvent.click(await screen.findByRole("button", { name: /Profiles for SME Automation/ }));
    const pane = await screen.findByRole("region", { name: "Outlook message" });
    const frame = await within(pane).findByTitle<HTMLIFrameElement>(/^Message: /);
    expect(frame.getAttribute("srcdoc")).not.toContain(` src="https://hyppies.test`);
    fireEvent.click(within(pane).getByRole("button", { name: "Show pictures" }));
    expect(frame.getAttribute("srcdoc")).toContain(`src="https://hyppies.test/logo.png"`);
  });

  it("lists folders with their unread counts, sub-folders under their parent", async () => {
    serve([]);
    renderWithClient(<OutlookMail />);
    const nav = await screen.findByRole("navigation", { name: "Outlook folders" });
    const names = within(nav).getAllByRole("button").map((b) => b.textContent);
    expect(names).toEqual(["Inbox3", "Drafts", "Deleted Items", "Archive", "Clients", "Acme1"]);
    expect(within(nav).getByLabelText("3 unread")).toBeTruthy();
  });
});

describe("helpers", () => {
  it("writes list dates as Outlook does", () => {
    const now = new Date(2026, 10, 20, 15, 0);
    expect(listDate(new Date(2026, 10, 20, 9, 5).toISOString(), now)).toMatch(/09.05/);
    expect(listDate(new Date(2026, 10, 19, 9, 5).toISOString(), now)).toBe("Yesterday");
    expect(listDate(new Date(2026, 10, 17, 9, 5).toISOString(), now)).toMatch(/^Tue/);
    expect(listDate(new Date(2025, 0, 2).toISOString(), now)).toMatch(/2025/);
  });

  it("puts sub-folders under their parent", () => {
    expect(folderTree(folders).map(({ folder, depth }) => `${folder.name}:${String(depth)}`))
      .toEqual(["Inbox:0", "Drafts:0", "Deleted Items:0", "Archive:0", "Clients:0", "Acme:1"]);
  });

  it("names a sender by their initials", () => {
    expect(initials("Leander Natasha", "natasha@hyppies.test")).toBe("LN");
    expect(initials("", "henk.devries@client.test")).toBe("HD");
    expect(initials("Lydia", "")).toBe("L");
  });

  it("makes a frame document that opens links outside and shows web pictures only when asked", () => {
    const html = `<img data-hp-src="https://a.test/x.png"><img data-hp-src="http://a.test/y.png">`;
    expect(frameDocument(html, false)).toContain(`<base target="_blank">`);
    expect(frameDocument(html, false)).not.toContain(` src=`);
    const shown = frameDocument(html, true);
    expect(shown).toContain(`src="https://a.test/x.png"`);
    expect(shown).not.toContain(`src="http://a.test`);
  });
});
