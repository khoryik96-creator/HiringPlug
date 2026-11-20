/*
 * HiringPlug LinkedIn companion: in the recruiter's own browser, reads from a
 * contact's LinkedIn profile the dates of their time at one company, from
 * a company's About page its name, industry, size and website, and pages of
 * a people search at a company (Find HR), and a person's name and current
 * roles (a new contact), for the
 * HiringPlug address the recruiter connected (and no other site). Adapted
 * from CV Studio's LinkedIn companion (its employment check); reader.js is
 * CV Studio's reader, unchanged.
 *
 * - One read at a time. Each opens LinkedIn in a new background tab that the
 *   companion owns, and closes it afterwards, unless the recruiter looked at
 *   it (then it is theirs and stays open). It never touches other tabs.
 * - Like CV Studio, it puts the HiringPlug tab that asked and the tabs it
 *   opens in one "HiringPlug" tab group (in that window). Grouping is only
 *   for looks: it gives no authority, and a check never fails because of it.
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

// A people search is the same while its filters and page are (LinkedIn adds
// tracking options such as origin to the address as it loads).
function searchKey(value) {
  try {
    const url = new URL(value);
    if (url.origin !== 'https://www.linkedin.com' || url.pathname.replace(/\/$/, '') !== '/search/results/people') return '';
    for (const name of ['origin', 'lipi', 'trk', 'sid', 'skipRedirect', 'spellCorrectionEnabled', 'prioritizeMessage']) url.searchParams.delete(name);
    url.searchParams.sort(); url.hash = '';
    return url.pathname.replace(/\/$/, '') + '?' + url.searchParams.toString();
  } catch { return ''; }
}

function samePage(value, expected) {
  if (typeof value !== 'string') return false;
  if (searchKey(expected)) return searchKey(value) === searchKey(expected);
  return !/[?#]/.test(value) && value.replace(/\/$/, '') === expected.replace(/\/$/, '');
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

// Puts a tab we opened in the asking HiringPlug tab's group (or a new
// "HiringPlug" group with it), in the same window only.
async function group(tabId, sourceId) {
  if (typeof chrome.tabs.group !== 'function' || typeof chrome.tabGroups?.update !== 'function') return;
  try {
    const tab = await chrome.tabs.get(tabId);
    let anchor = null;
    try { anchor = await chrome.tabs.get(sourceId); } catch { /* the HiringPlug tab was closed */ }
    if (anchor && anchor.windowId !== tab.windowId) anchor = null;
    if (anchor && Number.isInteger(anchor.groupId) && anchor.groupId >= 0) {
      if (tab.groupId !== anchor.groupId) await chrome.tabs.group({tabIds: [tabId], groupId: anchor.groupId});
      return;
    }
    const groupId = await chrome.tabs.group({tabIds: anchor ? [anchor.id, tabId] : [tabId], createProperties: {windowId: tab.windowId}});
    await chrome.tabGroups.update(groupId, {title: 'HiringPlug', color: 'purple'});
  } catch { /* only for looks */ }
}

// Opens url in a background tab of ours and polls read until it answers;
// timedOut names what never finished (read's last answer).
async function withTab(op, url, read, prepare, timedOut = () => 'LinkedIn did not finish loading. Check again.') {
  await ready;
  const source = op.source;
  const tab = await chrome.tabs.create({url, active: false, ...(source ? {windowId: source.windowId, openerTabId: source.id} : {})});
  owned.set(tab.id, url); await persist();
  if (source) await group(tab.id, source.id);
  let scrolls = 0, reads = 0, last;
  try {
    for (let attempt = 0; attempt < 100; attempt++) {
      if (op.cancelled) throw safeError('The check was stopped. Nothing more was saved.');
      const now = await ownedTab(tab.id);
      if (now.status === 'complete') {
        const value = await read(tab.id);
        last = value;
        if (op.cancelled) throw safeError('The check was stopped. Nothing more was saved.');
        if (typeof value?.blocked === 'string') throw safeError(value.blocked.slice(0, 200));
        if (value?.failed) throw safeError('LinkedIn needs you to sign in, or the profile or company could not be confirmed.');
        if (value && !value.loading) return value;
        if (++reads >= 40) break;
        if (prepare && scrolls < 8 && !viewed.has(tab.id)) { scrolls++; await prepare(tab.id); }
      }
      await pause(500);
    }
    throw safeError(timedOut(last));
  } finally {
    await closeOwned(tab.id, url); await persist();
  }
}

// The roles shown as current at other companies (roles.js); none if unreadable.
async function currentRoles(tabId, url, leftCompany) {
  try {
    const [present] = await chrome.scripting.executeScript({target: {tabId}, func: () => !!globalThis.HiringplugRoles});
    if (!present?.result) await chrome.scripting.executeScript({target: {tabId}, files: ['roles.js']});
    const [read] = await chrome.scripting.executeScript({target: {tabId}, func: (u, c) => {
      try { return globalThis.HiringplugRoles.readCurrentRoles(u, c); } catch { return []; }
    }, args: [url, leftCompany]});
    const text = (v, n) => typeof v === 'string' && v.length <= n;
    return (Array.isArray(read?.result) ? read.result : []).filter(r => r && companyRe.test(r.company_url) && text(r.company_name, 200) && r.company_name &&
      text(r.title, 200) && typeof r.since === 'string' && /^(?:(?:19|20)\d{2}(?:-(?:0[1-9]|1[0-2]))?)?$/.test(r.since)).slice(0, 3)
      .map(r => ({company_url: r.company_url, company_name: r.company_name, title: r.title, since: r.since}));
  } catch { return []; }
}

// LinkedIn's number for a company, from its People page header.
async function companyId(op, args) {
  if (!companyRe.test(args.company_url || '')) throw safeError("Add the company's LinkedIn page in HiringPlug first.");
  const value = await withTab(op, args.company_url + 'people/', tabId => inject(tabId, expected => {
    try { return globalThis.SourcerReader.readHRCompany(expected, '', true); } catch { return {failed: true}; }
  }, [args.company_url], 'The LinkedIn company page could not be read.'));
  if (!/^[0-9]{1,24}$/.test(value.company_id || '')) throw safeError('LinkedIn showed no single number for this company.');
  const name = typeof value.company_name === 'string' ? value.company_name.replace(/\s+/g, ' ').trim().slice(0, 200) : '';
  return {company_url: args.company_url, company_id: value.company_id, company_name: name};
}

// A person's profile, for a new contact: their name (profile page) and the
// roles shown as current, each with its company's exact LinkedIn page and
// since when (Experience page). Two background tabs, one after the other.
async function profile(op, args) {
  if (!profileRe.test(args.url || '')) throw safeError('Enter their LinkedIn profile (linkedin.com/in/…) first.');
  const top = await withTab(op, args.url + '/', tabId => inject(tabId, url => {
    try { return globalThis.SourcerReader.readProfile(url); } catch { return {failed: true}; }
  }, [args.url], 'The LinkedIn profile could not be read.'));
  const clip = (v, n) => (typeof v === 'string' ? v.replace(/\s+/g, ' ').trim().slice(0, n) : '');
  const name = clip(top.name, 150);
  if (!name) throw safeError('LinkedIn showed no name on this profile.');
  if (op.cancelled) throw safeError('The check was stopped. Nothing more was saved.');
  let roles = [];
  await withTab(op, args.url + '/details/experience/', async tabId => {
    roles = await currentRoles(tabId, args.url, '');
    // The Experience page can draw its roles after the page loads: wait a little for them.
    return roles.length || ++op.rolePolls > 12 ? {roles} : {loading: true};
  });
  return {url: args.url, name, roles};
}

// One page of a people search at a company (Find HR): the people whose title
// matches a keyword, with their profile link, title and location. The page is
// its own address (&page=N), so a page that failed is simply read again. The
// filters LinkedIn shows must name the company and the country.
async function peoplePage(op, args) {
  const text = (v, n) => typeof v === 'string' && v.trim() && v.length <= n;
  if (!/^[0-9]{1,24}$/.test(args.company_id || '') || !text(args.company_name, 200) || !text(args.query, 300) || !text(args.keywords, 800) ||
      !Number.isInteger(args.page) || args.page < 1 || args.page > 10 ||
      (args.geo_id !== undefined && (!/^[0-9]{1,24}$/.test(args.geo_id) || !text(args.country, 100)))) {
    throw safeError('Start Find HR from the company in HiringPlug.');
  }
  const url = new URL('https://www.linkedin.com/search/results/people/');
  url.searchParams.set('currentCompany', JSON.stringify([args.company_id]));
  if (args.geo_id) url.searchParams.set('geoUrn', JSON.stringify([args.geo_id]));
  url.searchParams.set('keywords', args.query);
  url.searchParams.set('origin', 'FACETED_SEARCH');
  if (args.page > 1) url.searchParams.set('page', String(args.page));
  let waiting = '';
  let value;
  try {
    value = await withTab(op, url.href, async tabId => {
      const read = await inject(tabId, (company, country, keywords) => {
        try { return globalThis.SourcerReader.readHRSearch(company, country, keywords); } catch { return {failed: true}; }
      }, [args.company_name, args.country || '', args.keywords], 'The LinkedIn results could not be read.');
      waiting = read?.wait_stage || '';
      return read;
    });
  } catch (e) {
    if (waiting === 'company_filter') throw safeError(`LinkedIn did not show the company filter as ${args.company_name}. Check the company's LinkedIn page.`);
    if (waiting === 'country_filter') throw safeError(`LinkedIn did not show the location filter as ${args.country}.`);
    throw e;
  }
  const clip = (v, n) => (typeof v === 'string' ? v.replace(/\s+/g, ' ').trim().slice(0, n) : '');
  const people = (Array.isArray(value.people) ? value.people : []).slice(0, 40)
    .filter(p => p && profileRe.test(p.url) && clip(p.name, 200))
    .map(p => ({url: p.url, name: clip(p.name, 200), title: clip(p.title, 500), location: clip(p.location, 240)}));
  return {people, has_next: value.hasNext === true && !value.empty};
}

// The dates of the profile's time at the company: current, former or unknown.
async function employment(op, args) {
  if (!profileRe.test(args.url || '') || !companyRe.test(args.company_url || '') || !/^[0-9]{1,24}$/.test(args.company_id || '')) {
    throw safeError('Choose a contact with a LinkedIn profile, at a company with a LinkedIn page.');
  }
  let moves = [];
  let absent = 0;
  const value = await withTab(op, args.url + '/details/experience/', async tabId => {
    const read = await inject(tabId, (url, company, id) => {
      try { return globalThis.SourcerReader.readHRJoined(url, company, id); } catch { return {failed: true}; }
    }, [args.url, args.company_url, args.company_id], 'The LinkedIn profile could not be read.');
    // Every role shown with its dates, none at the company's page: twice in a row, so not a page still drawing.
    if (read?.absent === true) {
      if (++absent >= 2) throw safeError("Their LinkedIn Experience shows no role at this company's LinkedIn page. Check the company's LinkedIn page in HiringPlug (it must be the page their role links to), or update the contact by hand.");
      return {loading: true};
    }
    absent = 0;
    // Their time there ended: where they work now, from the same page (best effort).
    if (read && !read.loading && !read.failed && read.current === false) moves = await currentRoles(tabId, args.url, args.company_url);
    return read;
  },
  tabId => inject(tabId, url => (document.visibilityState === 'visible' ? false : globalThis.SourcerReader.prepareHRJoined(url)),
    [args.url], 'The Experience section could not be loaded.'),
  last => ({
    section: 'LinkedIn did not show their Experience section. Open the profile once to check it loads, then check again.',
    roles: 'LinkedIn did not finish showing their roles. Check again.',
    dates: "LinkedIn did not show the dates of their role at this company. Check again.",
  })[last?.wait] || 'LinkedIn did not finish loading. Check again.');
  const {joined, left, current: now} = value;
  if (!dateRe.test(joined) || !dateRe.test(left) || ![true, false, null].includes(now) ||
      (now === true && (!joined || left)) || (now === false && (!joined || !left)) || (now === null && (joined || left)) ||
      (left && (joined.length === 4 ? joined + '-01' : joined) > (left.length === 4 ? left + '-12' : left))) {
    throw safeError('The employment dates could not be confirmed.');
  }
  return {url: args.url, company_url: args.company_url, company_id: args.company_id, joined, left, current: now, ...(now === false ? {moved_to: moves} : {})};
}

// What a company's About page says of it, for a new company in HiringPlug.
async function companyAbout(op, args) {
  if (!companyRe.test(args.company_url || '')) throw safeError("Enter the company's LinkedIn page (linkedin.com/company/…) first.");
  const value = await withTab(op, args.company_url + 'about/', async tabId => {
    try {
      const [present] = await chrome.scripting.executeScript({target: {tabId}, func: () => !!globalThis.HiringplugCompany});
      if (!present?.result) await chrome.scripting.executeScript({target: {tabId}, files: ['company.js']});
      const [read] = await chrome.scripting.executeScript({target: {tabId}, func: u => {
        try { return globalThis.HiringplugCompany.readAbout(u); } catch { return {failed: true}; }
      }, args: [args.company_url]});
      return read?.result;
    } catch { throw safeError('The LinkedIn company page could not be read.'); }
  });
  const text = (v, n) => (typeof v === 'string' ? v.replace(/\s+/g, ' ').trim().slice(0, n) : '');
  const name = text(value.name, 200);
  if (!name) throw safeError('LinkedIn showed no name for this company.');
  return {
    company_url: args.company_url, name, industry: text(value.industry, 200), size: text(value.size, 100),
    website: /^https?:\/\/\S+$|^[a-z0-9.-]+\.[a-z]{2,}(\/\S*)?$/i.test(text(value.website, 300)) ? text(value.website, 300) : '',
    headquarters: text(value.headquarters, 200), about: text(value.about, 4000),
  };
}

async function handle(message, sender) {
  if (sender.id !== chrome.runtime.id || !sender.tab || sender.frameId !== 0 || !(await connectedOrigins()).includes(sender.origin)) {
    throw safeError('Connect this HiringPlug first: click the companion icon in its tab.');
  }
  const {action, args = {}} = message || {};
  if (action === 'status') return {version: chrome.runtime.getManifest().version, busy: !!current};
  if (action === 'openUpdater') { // HiringPlug's "Update the companion": the update page, beside its tab
    await chrome.tabs.create({url: chrome.runtime.getURL('update.html'), windowId: sender.tab.windowId, index: sender.tab.index + 1});
    return true;
  }
  if (action === 'cancel') {
    if (current && current.operation === args.operation_id) current.cancelled = true;
    return true;
  }
  if (!['companyId', 'employment', 'companyAbout', 'peoplePage', 'profile'].includes(action)) throw safeError('The companion does not know this action. Update it.');
  if (!operationRe.test(args.operation_id || '')) throw safeError('Start the check from HiringPlug.');
  if (current) throw safeError('The companion is busy with another check. Wait for it, or stop it.');
  const op = {operation: args.operation_id, cancelled: false, rolePolls: 0, source: {id: sender.tab.id, windowId: sender.tab.windowId}};
  current = op;
  try {
    if (action === 'companyAbout') return await companyAbout(op, args);
    if (action === 'peoplePage') return await peoplePage(op, args);
    if (action === 'profile') return await profile(op, args);
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
