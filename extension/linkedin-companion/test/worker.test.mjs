// The companion's worker with a fake browser: which pages may use it, the
// reads it makes, the dates it accepts, and the tabs it opens and closes.
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {test} from 'node:test';
import vm from 'node:vm';

const source = readFileSync(new URL('../worker.js', import.meta.url), 'utf8');
const app = 'https://ats.agency.test';
const profile = 'https://www.linkedin.com/in/ana-lim-example';
const company = 'https://www.linkedin.com/company/maju-logistik/';
const op = 'clc_01jbz6w0m0k8q9x2r3s4t5v6w7';

function browser({page = () => ({joined: '2019-04', left: '2025-08', current: false}), origins = [app]} = {}) {
  const listeners = {message: [], activated: [], removed: []};
  const tabs = new Map();
  const store = {local: {hiringplugOrigins: origins}, session: {}};
  let nextId = 10;
  const area = name => ({
    get: async key => ({[key]: store[name][key]}),
    set: async values => { Object.assign(store[name], values); },
  });
  const chrome = {
    runtime: {id: 'companion', getManifest: () => ({version: '1.0.0'}), onMessage: {addListener: f => listeners.message.push(f)}},
    storage: {local: area('local'), session: area('session')},
    tabs: {
      create: async ({url, active}) => { const t = {id: nextId++, url, status: 'complete', active}; tabs.set(t.id, t); return t; },
      get: async id => { const t = tabs.get(id); if (!t) throw Error('No tab with id'); return {...t}; },
      remove: async id => { tabs.delete(id); listeners.removed.forEach(f => f(id)); },
      onActivated: {addListener: f => listeners.activated.push(f)},
      onRemoved: {addListener: f => listeners.removed.push(f)},
    },
    scripting: {
      executeScript: async ({target, files, args}) => {
        if (files) return [{}];
        const tab = tabs.get(target.tabId);
        return [{result: page(tab, args)}];
      },
    },
  };
  const context = vm.createContext({chrome, setTimeout, URL, globalThis: {}});
  context.globalThis = context;
  vm.runInContext(source, context);
  const send = (message, origin = app) => new Promise(resolve => {
    listeners.message[0](message, {id: 'companion', tab: {id: 1}, frameId: 0, origin}, resolve);
  });
  const look = id => { tabs.get(id).active = true; listeners.activated.forEach(f => f({tabId: id})); };
  return {send, tabs, look, context};
}

const check = {action: 'employment', args: {operation_id: op, url: profile, company_url: company, company_id: '123456'}};

test('only the connected HiringPlug may use it', async () => {
  const b = browser();
  assert.match((await b.send(check, 'https://evil.test')).error, /Connect this HiringPlug/);
  assert.equal((await b.send({action: 'status'})).result.version, '1.0.0');
});

test('reads the dates of the time at the company in a background tab, then closes it', async () => {
  let opened = null;
  const b = browser({page: tab => { opened = {url: tab.url, active: tab.active}; return {joined: '2019-04', left: '2025-08', current: false}; }});
  const reply = await b.send(check);
  assert.deepEqual({...reply.result}, {url: profile, company_url: company, company_id: '123456', joined: '2019-04', left: '2025-08', current: false});
  assert.deepEqual(opened, {url: profile + '/details/experience/', active: false});
  assert.equal(b.tabs.size, 0, 'its tab is closed');
});

test('dates that make no sense are refused', async () => {
  for (const dates of [
    {joined: '2024-01', left: '2023-12', current: false}, {joined: '2024', left: '2025', current: true}, {joined: '', left: '', current: true},
    {joined: '2024', left: '', current: false}, {joined: '2024', left: '', current: 'true'}, {joined: '5 yrs', left: '', current: true},
  ]) {
    const b = browser({page: () => dates});
    assert.match((await b.send(check)).error, /could not be confirmed/, JSON.stringify(dates));
    assert.equal(b.tabs.size, 0);
  }
  const unknown = browser({page: () => ({joined: '', left: '', current: null})});
  assert.equal((await unknown.send(check)).result.current, null, 'unclear is an answer, not an error');
});

test('a sign-in page or another profile stops the read', async () => {
  const b = browser({page: () => ({failed: true})});
  assert.match((await b.send(check)).error, /sign in/);
  for (const args of [{...check.args, url: 'https://www.linkedin.com/in/ana/details'}, {...check.args, company_id: 'maju'},
    {...check.args, company_url: 'https://evil.test/company/maju/'}, {...check.args, operation_id: 'x'}]) {
    assert.ok((await browser().send({action: 'employment', args})).error);
  }
});

test('a tab the recruiter looked at is theirs: it is not closed', async () => {
  let b;
  let looked = false;
  b = browser({page: tab => {
    if (!looked) { looked = true; b.look(tab.id); return {loading: true}; }
    return {joined: '2021', left: '', current: true};
  }});
  const reply = await b.send(check);
  assert.equal(reply.result.current, true);
  assert.equal(b.tabs.size, 1, 'left open');
});

test('one read at a time, and Stop stops it', async () => {
  let b;
  b = browser({page: () => ({loading: true})});
  const first = b.send(check);
  await new Promise(r => setTimeout(r, 20));
  assert.match((await b.send(check)).error, /busy/);
  await b.send({action: 'cancel', args: {operation_id: op}});
  assert.match((await first).error, /stopped/);
  assert.equal(b.tabs.size, 0);
});

test("finds LinkedIn's number for the company on its People page", async () => {
  let url = '';
  const b = browser({page: tab => { url = tab.url; return {company_name: 'Maju', company_id: '123456'}; }});
  const reply = await b.send({action: 'companyId', args: {operation_id: 'a'.repeat(32), company_url: company}});
  assert.deepEqual({...reply.result}, {company_url: company, company_id: '123456'});
  assert.equal(url, company + 'people/');
});
