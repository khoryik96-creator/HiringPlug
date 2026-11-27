import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { DatabaseSync } from "node:sqlite";
import { test } from "node:test";

import { emailResult } from "./identity.mjs";
import { digest, seal } from "./secrets.mjs";
import { handle } from "./worker.mjs";

const person = {
  url: "https://www.linkedin.com/in/sample-person",
  name: "Sample Person",
  company: "Example Company",
  company_url: "https://www.linkedin.com/company/example/",
  company_id: "123",
};
const contact = {
  linkedin_url: person.url,
  email: "person@example.test",
  email_status: "verified",
};
const record = (org) => ({ person: { ...contact, organization: org } });

test("a conflicting or absent employer link cannot be overridden by its name", () => {
  for (const linkedin_url of [
    null,
    "",
    "https://www.linkedin.com/company/different",
    "http://www.linkedin.com/company/456",
  ]) {
    assert.equal(emailResult(record({ linkedin_url, name: person.company }), person).state, "mismatch");
  }
  for (const linkedin_url of [
    person.company_url,
    "http://www.linkedin.com/company/Example",
    "https://www.linkedin.com/company/123/",
  ]) {
    assert.equal(emailResult(record({ linkedin_url, name: "Former spelling" }), person).email, contact.email);
  }
  const saved = {
    ...record({ linkedin_url: person.company_url }).person,
    id: "person",
    email: null,
    organization_id: "right",
  };
  const wrong = { person_id: "person", organization_name: person.company, email: contact.email };
  assert.equal(emailResult({ person: { ...saved, contact: wrong } }, person).reason, "contact_unconfirmed");
  assert.equal(
    emailResult({ person: { ...saved, contact: { ...wrong, organization_id: "right" } } }, person).state,
    "found",
  );
});

test("a company saved by its LinkedIn number is matched by Apollo's number for it", () => {
  const numbered = { ...person, company_url: "https://www.linkedin.com/company/123/" };
  const named = { linkedin_url: "http://www.linkedin.com/company/example-bhd", name: person.company };
  assert.equal(emailResult(record({ ...named, linkedin_uid: "123" }), numbered).email, contact.email);
  assert.equal(emailResult(record({ ...named, linkedin_uid: 123 }), numbered).email, contact.email);
  for (const linkedin_uid of [undefined, null, "", "456", "12 3", 456]) {
    assert.equal(emailResult(record({ ...named, linkedin_uid }), numbered).state, "mismatch", String(linkedin_uid));
  }
  assert.equal(emailResult(record({ linkedin_url: null, linkedin_uid: "123" }), person).email, contact.email, "the number alone");
});

function fixture() {
  const sql = new DatabaseSync(":memory:");
  sql.exec("PRAGMA foreign_keys=ON");
  sql.exec(readFileSync(new URL("./schema.sql", import.meta.url), "utf8"));
  let failing = null; // a statement that fails once, as D1 sometimes does
  const env = {
    PUBLIC_ORIGIN: "https://apollo.test.workers.dev",
    APOLLO_CLIENT_ID: "test-client",
    APOLLO_CLIENT_SECRET: "test-secret",
    TOKEN_KEY: "01".repeat(32),
    DB: {
      prepare(query) {
        if (failing && query.includes(failing)) {
          failing = null;
          throw Error("storage");
        }
        return {
          bind(...args) {
            const run = () => sql.prepare(query).run(...args);
            return {
              run: () => ({ meta: { changes: run().changes } }),
              first: () => sql.prepare(query).get(...args) ?? null,
            };
          },
        };
      },
      async batch(statements) {
        sql.exec("BEGIN");
        try {
          for (const stmt of statements) await stmt.run();
          sql.exec("COMMIT");
        } catch (error) {
          sql.exec("ROLLBACK");
          throw error;
        }
      },
    },
  };
  const id = "11".repeat(32);
  const secret = "22".repeat(32);
  async function connect() {
    const proof = await digest(secret);
    const credentials = await seal(env, { access_token: "fictional", expires: 10_000 }, id);
    sql.prepare("INSERT INTO flows VALUES (?, ?, 'connected', 5, 0, 'test')").run(id, proof);
    sql
      .prepare("INSERT INTO connections(id,proof,cipher,expires) VALUES (?,?,?,10000)")
      .run(id, proof, credentials);
  }
  let paid = 0,
    matchStatus = 200;
  function fetcher() {
    paid++;
    return Response.json(record({ linkedin_url: person.company_url }), { status: matchStatus });
  }
  function post(route, body, proof = secret, now = 100) {
    return handle(
      new Request(env.PUBLIC_ORIGIN + route, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ connection_id: id, secret: proof, ...body }),
      }),
      env,
      fetcher,
      now,
    );
  }
  const op = (digit) => ({ operation_id: digit.repeat(64), confirmed: true, person });
  return {
    sql,
    env,
    id,
    connect,
    post,
    op,
    paid: () => paid,
    setMatchStatus: (next) => {
      matchStatus = next;
    },
    failOnce: (statement) => {
      failing = statement;
    },
  };
}

test("the versioned broker distinguishes a preflight limit from an attempted match", async (t) => {
  const f = fixture();
  t.after(() => f.sql.close());
  await f.connect();
  f.sql.prepare("UPDATE connections SET busy=1").run();
  const busy = await (await f.post("/reveal-v2", f.op("a"))).json();
  assert.deepEqual(busy, { state: "limited", not_dispatched: true });
  const recovered = await (await f.post("/lookup-status-v2", f.op("a"))).json();
  assert.deepEqual(recovered, busy);
  assert.equal(f.paid(), 0);

  f.sql.prepare("UPDATE connections SET busy=0").run();
  f.setMatchStatus(429);
  const attempted = await (await f.post("/reveal-v2", f.op("b"))).json();
  assert.deepEqual(attempted, { state: "limited", reason: "rate_limited" });
  assert.equal(f.paid(), 1);
  // Which refusal it was, so the person knows what to check in Apollo.
  for (const [status, reason, digit] of [[402, "no_credits", "d"], [403, "plan_not_allowed", "e"]]) {
    f.setMatchStatus(status);
    assert.deepEqual(await (await f.post("/reveal-v2", f.op(digit))).json(), { state: "limited", reason });
  }
  assert.equal(f.paid(), 3);

  f.setMatchStatus(200);
  const found = await (await f.post("/reveal-v2", f.op("c"))).json();
  assert.equal(found.identity_checked, true);
  assert.equal(found.email, contact.email);
  assert.equal((await (await f.post("/lookup-status-v2", f.op("c"))).json()).email, contact.email);
  assert.equal(f.paid(), 4);
});

test("revocation uses the proof even when an OAuth flow has expired", async (t) => {
  const f = fixture();
  t.after(() => f.sql.close());
  await f.connect();
  assert.equal((await f.post("/disconnect-v2", {}, "bb".repeat(32))).status, 200);
  assert.equal(f.sql.prepare("SELECT count(*) AS n FROM connections").get().n, 1);
  const result = await (await f.post("/disconnect-v2", {})).json();
  assert.deepEqual(result, { protocol: 2, connected: false });
  assert.equal(f.sql.prepare("SELECT count(*) AS n FROM connections").get().n, 0);
  assert.equal(f.sql.prepare("SELECT status FROM flows WHERE id=?").get(f.id).status, "cancelled");
  assert.deepEqual(await (await f.post("/disconnect-v2", {})).json(), result);
  assert.deepEqual(await (await f.post("/reveal-v2", f.op("d"))).json(), {
    state: "auth_required",
    not_dispatched: true,
  });
  assert.equal(f.paid(), 0);
});

test("a lookup that never released the connection stops blocking it when its lease ends", async (t) => {
  const f = fixture();
  t.after(() => f.sql.close());
  await f.connect();
  f.sql.prepare("UPDATE connections SET busy=100").run(); // claimed at 100, then cancelled
  const held = await (await f.post("/reveal-v2", f.op("a"), undefined, 150)).json();
  assert.deepEqual(held, { state: "limited", not_dispatched: true });
  const found = await (await f.post("/reveal-v2", f.op("b"), undefined, 300)).json();
  assert.equal(found.email, contact.email);
  assert.equal(f.sql.prepare("SELECT busy FROM connections").get().busy, 0);
  assert.equal(f.paid(), 1);
});

test("a storage failure before Apollo is called can be confirmed again", async (t) => {
  const f = fixture();
  t.after(() => f.sql.close());
  await f.connect();
  f.failOnce("SET busy=?");
  const failed = await (await f.post("/reveal-v2", f.op("a"))).json();
  assert.deepEqual(failed, { state: "limited", not_dispatched: true }, "not a reconnect");
  assert.deepEqual(await (await f.post("/lookup-status-v2", f.op("a"))).json(), failed);
  assert.equal(f.paid(), 0);
  assert.equal((await (await f.post("/reveal-v2", f.op("b"))).json()).email, contact.email);
});

test("a failed Apollo sign-in says which step failed, never the code or token", async () => {
  const { sql, env } = fixture();
  const token = (over = {}) => ({
    access_token: "fictional-access",
    refresh_token: "fictional-refresh",
    expires_in: 3600,
    scope: "read_user_profile app_scopes people_match",
    ...over,
  });
  let n = 0;
  async function signIn(query, apollo) {
    const id = (++n).toString(16).padStart(2, "0").repeat(32);
    sql.prepare("INSERT INTO flows VALUES (?, 'proof', 'pending', 1000, 0, 'test')").run(id);
    const url = env.PUBLIC_ORIGIN + "/callback?" + query.replace("STATE", id);
    const response = await handle(new Request(url), env, apollo, 100);
    return response.text();
  }
  const answers = (tokenReply, profileReply = { status: 200, body: { user: { id: "u1", name: "Sample Person" } } }) =>
    (url) =>
      url.endsWith("/oauth/token")
        ? Response.json(tokenReply.body ?? {}, { status: tokenReply.status })
        : Response.json(profileReply.body ?? {}, { status: profileReply.status });

  assert.match(await signIn("error=access_denied&state=STATE", answers({ status: 200 })), /declined \(Apollo said: access_denied\)/);
  assert.match(await signIn("code=c&state=STATE", answers({ status: 401 })), /refused this broker's app details \(Apollo answered 401\)/);
  const scope = await signIn("code=c&state=STATE", answers({ status: 200, body: token({ scope: "read_user_profile app_scopes" }) }));
  assert.match(scope, /did not grant email lookup\. Granted: read_user_profile, app_scopes\. Needed: read_user_profile and people_match/);
  assert.match(await signIn("code=c&state=STATE", answers({ status: 200, body: token({ refresh_token: undefined }) })), /incomplete/);
  assert.match(
    await signIn("code=c&state=STATE", answers({ status: 200, body: token() }, { status: 403 })),
    /did not return your profile \(Apollo answered 403\)/,
  );
  assert.match(await signIn("code=c&state=STATE", answers({ status: 200, body: token() })), /^Apollo connected/);
  assert.match(await signIn("code=c&state=" + "ff".repeat(32), answers({ status: 200 })), /expired or was already used/);
  assert.doesNotMatch(scope, /fictional|test-secret|code=/);
});
