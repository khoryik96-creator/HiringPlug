// On its own: fake timers here would leak into the other preview tests.
import { act, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { renderWithClient } from "../../test-utils";
import { TemplatePreview } from "./TemplatePreview";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("template preview while writing", () => {
  it("settles once the writing stops, and does not keep re-drawing", () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const { rerender } = renderWithClient(<TemplatePreview subject="Hi [Full name]" html="<p>Hello</p>" bannerUrls={{}} />);
    rerender(<TemplatePreview subject="Hi [Full name]" html="<p>Hello there</p>" bannerUrls={{}} />); // a key typed
    act(() => { vi.advanceTimersByTime(1000); }); // settled
    const timers = vi.spyOn(globalThis, "setTimeout");
    act(() => { vi.advanceTimersByTime(3000); });
    const waits = timers.mock.calls.filter((c) => c[1] === 300).length;
    timers.mockRestore();
    expect(waits).toBe(0); // nothing more to wait for
  });
});
