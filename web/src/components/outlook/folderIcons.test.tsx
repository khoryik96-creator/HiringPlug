import { describe, expect, it } from "vitest";

import { folderIconKey } from "./folderIcons";

describe("folder icons, as in Outlook", () => {
  it("go by the folder's kind, then by well-known names, else a folder", () => {
    const icon = (kind: string, name: string) => folderIconKey({ kind, name });
    expect(icon("inbox", "Inbox")).toBe("inbox");
    expect(icon("drafts", "Drafts")).toBe("drafts");
    expect(icon("sentitems", "Sent Items")).toBe("sentitems");
    expect(icon("deleteditems", "Deleted Items")).toBe("deleteditems");
    expect(icon("junkemail", "Junk Email")).toBe("junkemail");
    expect(icon("archive", "Archive")).toBe("archive");
    expect(icon("", "Outbox")).toBe("outbox");
    expect(icon("", "Snoozed")).toBe("snoozed");
    expect(icon("", "Conversation History")).toBe("conversationhistory");
    expect(icon("", "Jobstreet")).toBe("folder"); // their own
    expect(icon("", "To Call")).toBe("folder");
  });
});
