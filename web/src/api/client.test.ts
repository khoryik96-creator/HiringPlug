import { describe, expect, it } from "vitest";

import { ApiError, unwrap } from "./client";

describe("unwrap", () => {
  it("returns the data, accepts a 204 with none (a delete), and throws the API's errors", () => {
    expect(unwrap({ data: { id: "x" }, response: new Response(null, { status: 200 }) })).toEqual({ id: "x" });
    expect(unwrap({ response: new Response(null, { status: 204 }) })).toBeUndefined();
    expect(() => unwrap({ error: { error: { code: "NOT_FOUND", message: "That document does not exist.", requestId: "req_1" } }, response: new Response(null, { status: 404 }) }))
      .toThrow(ApiError);
    expect(() => unwrap({ response: new Response(null, { status: 500 }) })).toThrow("Unexpected response 500");
  });
});
