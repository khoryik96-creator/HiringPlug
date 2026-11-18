import { describe, expect, it } from "vitest";

import { ApiError, throwFor, unwrap } from "./client";

describe("unwrap", () => {
  it("returns the data, accepts a 204 with none (a delete), and throws the API's errors", () => {
    expect(unwrap({ data: { id: "x" }, response: new Response(null, { status: 200 }) })).toEqual({ id: "x" });
    expect(unwrap({ response: new Response(null, { status: 204 }) })).toBeUndefined();
    expect(() => unwrap({ error: { error: { code: "NOT_FOUND", message: "That document does not exist.", requestId: "req_1" } }, response: new Response(null, { status: 404 }) }))
      .toThrow(ApiError);
    expect(() => unwrap({ response: new Response(null, { status: 500 }) })).toThrow("Unexpected response 500");
  });
});

describe("throwFor", () => {
  it("throws the API's error, or what the status says when the body is not the API's (a proxy's page)", async () => {
    const api = new Response(JSON.stringify({ error: { code: "NOT_FOUND", message: "That document does not exist.", requestId: "req_1" } }), { status: 404 });
    await expect(throwFor(api)).rejects.toThrow(ApiError);
    await expect(throwFor(new Response("<html>Too large</html>", { status: 413 }))).rejects.toThrow("The file is too large to upload.");
    await expect(throwFor(new Response("<html>Bad gateway</html>", { status: 502 }))).rejects.toThrow("The server answered 502");
  });
});
