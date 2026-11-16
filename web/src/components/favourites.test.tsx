import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { failure, ok, sentBody } from "../test-utils";
import { preferencesKey } from "./candidateColumns";
import { countryGroups, useFavourites } from "./favourites";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

function withClient(client = new QueryClient({ defaultOptions: { queries: { retry: false } } })) {
  return ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("favourites", () => {
  it("builds a second star on the first even if an older list comes back meanwhile", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ favourites: { itSkill: [] } })); // the server, before either save
    let finish: (value: unknown) => void = () => undefined;
    const put = vi.spyOn(api, "PUT").mockImplementation(() => new Promise((resolve) => { finish = resolve; }) as never);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useFavourites(), { wrapper: withClient(client) });
    await waitFor(() => { expect(result.current.loaded).toBe(true); });
    act(() => { result.current.of("itSkill").toggle("Java"); });
    await waitFor(() => { expect(put).toHaveBeenCalledTimes(1); });
    // Something else refetches the preferences while Java is being saved: the old list comes back.
    act(() => { client.setQueryData(preferencesKey, { favourites: { itSkill: [] } }); });
    act(() => { result.current.of("itSkill").toggle("Python"); });
    await act(async () => { finish(ok({ favourites: { itSkill: ["Java"] } })); await Promise.resolve(); });
    await waitFor(() => { expect(put).toHaveBeenCalledTimes(2); });
    expect(sentBody(put, 1), "Java is not lost").toEqual({ favourites: { itSkill: ["Java", "Python"] } });
  });

  it("says when a star could not be saved", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ favourites: {} }));
    vi.spyOn(api, "PUT").mockResolvedValue(failure("INTERNAL", "down", undefined, 500));
    const { result } = renderHook(() => useFavourites(), { wrapper: withClient() });
    await waitFor(() => { expect(result.current.of("country").ready).toBe(true); });
    act(() => { result.current.of("country").toggle("MY"); });
    await waitFor(() => { expect(result.current.of("country").failed).toBe(true); });
  });

  it("puts the person's countries first, then every other country by name", () => {
    const g = countryGroups({ list: ["SG"], has: (v) => v === "SG", toggle: () => undefined, ready: true, failed: false });
    expect(g.top).toEqual(["SG"]);
    expect(g.rest[0]).toBe("AF"); // Afghanistan
    expect(g.rest).not.toContain("SG");
    expect(countryGroups(undefined, "ZZ").rest.at(-1), "a stored code that is not a country is still offered").toBe("ZZ");
  });
});
