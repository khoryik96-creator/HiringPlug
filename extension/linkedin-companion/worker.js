/*
 * HiringPlug LinkedIn companion: in the recruiter's own browser, reads from a
 * contact's LinkedIn profile the dates of their time at one company, for the
 * HiringPlug address the recruiter connected (and no other site). Adapted
 * from CV Studio's LinkedIn companion (its employment check); reader.js is
 * CV Studio's reader, unchanged.
 *
 * - One read at a time. Each opens LinkedIn in a new background tab that the
 *   companion owns, and closes it afterwards, unless the recruiter looked at
 *   it (then it is theirs and stays open). It never touches other tabs.
 * - Bounded: up to 100 half-second polls per read, 8 scrolls, 40 reads of
 *   the page; it stops at a sign-in, check or restriction page.
 * - The identity of the profile and the company is checked before and after
 *   reading; a name or job title alone is never evidence.
 * - No AI, no LinkedIn messages, nothing sent anywhere but back to the page.
 */
const ORIGINS_KEY = 'hiringplugOrigins';
const TABS_KEY = 'hiringplugTabs';
const owned = new Map();  // tab id -> the LinkedIn address it was opened at (ours to close)
const viewed = new Map(); // tabs the recruiter looked at: still readable, never closed by us
let current = null;       // the read in progress: {operation, cancelled}

const operationRe = /^(?:[a-z]{2,8}_[0-9a-hjkmnp-tv-z]{26}|[a-f0-9]{32})$/;
const profileRe = /^https:\/\/www\.linkedin\.com\/in\/[A-Za-z0-9_%~-]+$/;
const companyRe = /^https:\/\/www\.linkedin\.com\/company\/[A-Za-z0-9_-]+\/$/;
const dateRe = /^(?:(?:19|20)\d{2}(?:-(?:0[1-9]|1[0-2]))?)?$/;

function safeError(message) { const e = Error(message); e.hiringplugSafe = true; return e; }
function publicError(e) {
  return {error: e?.hiringplugSafe ? e.message : 'The companion stopped. Check the LinkedIn tab, or reload the companion on the browser Extensions page.'};
}
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));

async function connectedOrigins() {
  const saved = (await chrome.storage.local.get(ORIGINS_KEY))[ORIGINS_KEY];
  return Array.isArray(saved) ? saved.filter(o => typeof o === 'string') : [];
}

async function persist() {
  await chrome.storage.session.set({[TABS_KEY]: {owned: [...owned], viewed: [...viewed]}});
}

// After the service worker was stopped mid-read, its tabs are closed (unless
// looked at): a read never resumes on its own.
const ready = (async () => {
  const saved = (await chrome.storage.session.get(TABS_KEY))[TABS_KEY];
  for (const [id, url] of Array.isArray(saved?.owned) ? saved.owned : []) {
    if (Number.isInteger(id) && typeof url === 'string') owned.set(id, url);
  }
  for (const [id, url] of [...owned]) await closeOwned(id, url);
  await persist();
})().catch(() => { owned.clear(); viewed.clear(); });

function samePage(value, expected) {
  return typeof value === 'string' && !/[?#]/.test(value) && value.replace(/\/$/, '') === expected.replace(/\/$/, '');
}

// The tab, if it is still on the page it was opened at; looking at it makes it the recruiter's.
async function ownedTab(id) {
  const expected = owned.get(id) || viewed.get(id);
  const tab = await chrome.tabs.get(id).catch(() => null);
  const loading = tab?.status === 'loading' && samePage(tab.pendingUrl, expected);
  if (!expected || !tab || (!samePage(tab.url, expected) && !loading) || (tab.pendingUrl && !samePage(tab.pendingUrl, expected))) {
    owned.delete(id); viewed.delete(id); await persist();
    throw safeError('The LinkedIn tab was changed or closed. Nothing was saved; check again.');
  }
  if (tab.active && owned.has(id)) { viewed.set(id, owned.get(id)); owned.delete(id); await persist(); }
  return tab;
}

async function closeOwned(id, expected) {
  try {
    const tab = await chrome.tabs.get(id);
    if (owned.get(id) === expected && !tab.active && (samePage(tab.url, expected) || samePage(tab.pendingUrl, expected))) await chrome.tabs.remove(id);
  } catch { /* closed already */ }
  if (owned.get(id) === expected) owned.delete(id);
  if (viewed.get(id) === expected) viewed.delete(id);
}

chrome.tabs.onActivated.addListener(({tabId}) => {
  void ready.then(async () => { if (owned.has(tabId)) { viewed.set(tabId, owned.get(tabId)); owned.delete(tabId); await persist(); } });
});
chrome.tabs.onRemoved.addListener(tabId => {
  void ready.then(async () => { if (owned.delete(tabId) || viewed.delete(tabId)) await persist(); });
});

async function inject(tabId, func, args, message) {
  try {
    // The reader once per page (it stays until the page changes), then the read.
    const [present] = await chrome.scripting.executeScript({target: {tabId}, func: () => !!globalThis.SourcerReader});
    if (!present?.result) await chrome.scripting.executeScript({target: {tabId}, files: ['reader.js']});
    const [read] = await chrome.scripting.executeScript({target: {tabId}, func, args});
    return read?.result;
  } catch { throw safeError(message); }
}

// Opens url in a background tab of ours and polls read until it answers.
async function withTab(op, url, read, prepare) {
  await ready;
  const tab = await chrome.tabs.create({url, active: false});
  owned.set(tab.id, url); await persist();
  let scrolls = 0, reads = 0;
  try {
    for (let attempt = 0; attempt < 100; attempt++) {
      if (op.cancelled) throw safeError('The check was stopped. Nothing more was saved.');
      const now = await ownedTab(tab.id);
      if (now.status === 'complete') {
        const value = await read(tab.id);
        if (op.cancelled) throw safeError('The check was stopped. Nothing more was saved.');
        if (value?.failed) throw safeError('LinkedIn needs you to sign in, or the profile or company could not be confirmed.');
        if (value && !value.loading) return value;
        if (++reads >= 40) break;
        if (prepare && scrolls < 8 && !viewed.has(tab.id)) { scrolls++; await prepare(tab.id); }
      }
      await pause(500);
    }
    throw safeError('LinkedIn did not finish loading. Check again.');
  } finally {
    await closeOwned(tab.id, url); await persist();
  }
}

// LinkedIn's number for a company, from its People page header.
async function companyId(op, args) {
  if (!companyRe.test(args.company_url || '')) throw safeError("Add the company's LinkedIn page in HiringPlug first.");
  const value = await withTab(op, args.company_url + 'people/', tabId => inject(tabId, expected => {
    try { return globalThis.SourcerReader.readHRCompany(expected, '', true); } catch { return {failed: true}; }
  }, [args.company_url], 'The LinkedIn company page could not be read.'));
  if (!/^[0-9]{1,24}$/.test(value.company_id || '')) throw safeError('LinkedIn showed no single number for this company.');
  return {company_url: args.company_url, company_id: value.company_id};
}

// The dates of the profile's time at the company: current, former or unknown.
async function employment(op, args) {
  if (!profileRe.test(args.url || '') || !companyRe.test(args.company_url || '') || !/^[0-9]{1,24}$/.test(args.company_id || '')) {
    throw safeError('Choose a contact with a LinkedIn profile, at a company with a LinkedIn page.');
  }
  const value = await withTab(op, args.url + '/details/experience/', tabId => inject(tabId, (url, company, id) => {
    try { return globalThis.SourcerReader.readHRJoined(url, company, id); } catch { return {failed: true}; }
  }, [args.url, args.company_url, args.company_id], 'The LinkedIn profile could not be read.'),
  tabId => inject(tabId, url => (document.visibilityState === 'visible' ? false : globalThis.SourcerReader.prepareHRJoined(url)),
    [args.url], 'The Experience section could not be loaded.'));
  const {joined, left, current: now} = value;
  if (!dateRe.test(joined) || !dateRe.test(left) || ![true, false, null].includes(now) ||
      (now === true && (!joined || left)) || (now === false && (!joined || !left)) || (now === null && (joined || left)) ||
      (left && (joined.length === 4 ? joined + '-01' : joined) > (left.length === 4 ? left + '-12' : left))) {
    throw safeError('The employment dates could not be confirmed.');
  }
  return {url: args.url, company_url: args.company_url, company_id: args.company_id, joined, left, current: now};
}

async function handle(message, sender) {
  if (sender.id !== chrome.runtime.id || !sender.tab || sender.frameId !== 0 || !(await connectedOrigins()).includes(sender.origin)) {
    throw safeError('Connect this HiringPlug first: click the companion icon in its tab.');
  }
  const {action, args = {}} = message || {};
  if (action === 'status') return {version: chrome.runtime.getManifest().version, busy: !!current};
  if (action === 'cancel') {
    if (current && current.operation === args.operation_id) current.cancelled = true;
    return true;
  }
  if (action !== 'companyId' && action !== 'employment') throw safeError('The companion does not know this action. Update it.');
  if (!operationRe.test(args.operation_id || '')) throw safeError('Start the check from HiringPlug.');
  if (current) throw safeError('The companion is busy with another check. Wait for it, or stop it.');
  const op = {operation: args.operation_id, cancelled: false};
  current = op;
  try {
    return action === 'companyId' ? await companyId(op, args) : await employment(op, args);
  } finally {
    if (current === op) current = null;
  }
}

chrome.runtime.onMessage.addListener((message, sender, reply) => {
  handle(message, sender).then(result => reply({result}), error => reply(publicError(error)));
  return true;
});

globalThis.hiringplugCompanion = {handle, owned, viewed}; // for tests
