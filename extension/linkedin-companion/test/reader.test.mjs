// The reader on an Experience page laid out as LinkedIn draws it (a
// synthetic profile), in a simulated browser: the dates of the time at a
// company, by its LinkedIn page or, for a role linked to no page, by its
// exact name.
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {createRequire} from 'node:module';
import {test} from 'node:test';

const {JSDOM} = createRequire(new URL('../../../web/package.json', import.meta.url))('jsdom');
const source = readFileSync(new URL('../reader.js', import.meta.url), 'utf8');
const profile = 'https://www.linkedin.com/in/aisha-example';
const time = 'https://www.linkedin.com/company/time-dotcom/';

const role = (title, company, dates, link) => `
  <div componentkey="entity-collection-item-${Math.random().toString(36).slice(2)}">
    ${link ? `<a href="${link}"><p>${title}</p><p>${company}</p><p>${dates}</p></a>` : `<p>${title}</p><p>${company}</p><p>${dates}</p>`}
    <p>What the role was about.</p>
  </div>`;

function page(cards) {
  const dom = new JSDOM(`<main><section componentkey="com.linkedin.ExperienceDetailsSection">
    <div componentkey="profileExperienceDetails_top_anchor"></div><h2>Experience</h2>${cards.join('')}</section></main>`,
  {url: profile + '/details/experience/', runScripts: 'outside-only'});
  const {window} = dom;
  window.Element.prototype.getClientRects = function () { return [{}]; }; // all of it shown
  Object.defineProperty(window.HTMLElement.prototype, 'innerText', {get() { return this.textContent; }});
  window.eval(source);
  return window.SourcerReader;
}

// As the profile in question shows: the current role and the one before it typed in, not linked to a company page.
const typedIn = () => page([
  role('Senior Manager, Performance, Rewards and HR Operations', 'Payments Network Malaysia · Full-time', 'Aug 2017 - Present · 9 yrs 4 mos'),
  role('Senior Manager, HR, Legal & Secretarial', 'Malaysian Electronic Payment System · Full-time', 'Sep 2002 - Aug 2017 · 15 yrs'),
  role('Executive, HR', 'TIME dotCom Berhad', 'Jan 1996 - Aug 2002 · 6 yrs 8 mos', time),
]);

test('a role linked to no LinkedIn page matches the company by its exact name, said so', () => {
  const reader = typedIn();
  assert.deepEqual({...reader.readHRJoined(profile, 'https://www.linkedin.com/company/paynet/', '555', ['Payments Network Malaysia Sdn Bhd', 'PayNet'])},
    {joined: '2017-08', left: '', current: true, by_name: true});
});

test('never by a similar name, and a role linked to the page needs no name', () => {
  const reader = typedIn();
  assert.deepEqual({...reader.readHRJoined(profile, 'https://www.linkedin.com/company/paynet/', '555', ['PayNet', 'Payments Network'])}, {absent: true});
  assert.deepEqual({...reader.readHRJoined(profile, 'https://www.linkedin.com/company/paynet/', '555', [])}, {absent: true});
  assert.deepEqual({...reader.readHRJoined(profile, time, '777', ['Payments Network Malaysia'])}, {joined: '1996-01', left: '2002-08', current: false},
    'their role at the page wins over a name');
});

test("a company's name is compared without case, punctuation or legal suffixes", () => {
  const {companyKey} = page([]);
  assert.equal(companyKey('Payments Network Malaysia Sdn. Bhd.'), 'payments network malaysia');
  assert.equal(companyKey('TIME dotCom Berhad'), companyKey('Time Dotcom'));
  assert.equal(companyKey('AT&T Inc.'), 'at and t');
  assert.notEqual(companyKey('Payments Network'), companyKey('Payments Network Malaysia'));
});

test('a role still drawing its dates is waited for, by name too', () => {
  const reader = page([role('HR Manager', 'Payments Network Malaysia · Full-time', '')]);
  assert.equal(reader.readHRJoined(profile, 'https://www.linkedin.com/company/paynet/', '555', ['Payments Network Malaysia']).wait, 'dates');
});

// HiringPlug's company reader (company.js) on a LinkedIn companies search, laid
// out as LinkedIn draws its result cards (invented companies).
const companySource = readFileSync(new URL('../company.js', import.meta.url), 'utf8');
const searchURL = name => 'https://www.linkedin.com/search/results/companies/?keywords=' + encodeURIComponent(name) + '&origin=SWITCH_SEARCH_VERTICAL';

const companyCard = (link, name, subtitle) => `
  <div role="listitem"><a href="${link}"><img alt=""></a>
    <div><p><a href="${link}">${name}</a></p><p>${subtitle}</p><p>1K followers</p></div></div>`;

const results = cards => `<div componentkey="SearchResults_Companies"><div role="list">${cards.join('')}</div></div>`;

function companies(cards, url = searchURL('Contoh Logistik')) {
  return searchPage(`<div aria-label="Primary content">${results(cards)}</div>`, url);
}

function searchPage(inMain, url = searchURL('Contoh Logistik')) {
  const dom = new JSDOM(`<main>${inMain}</main>`, {url, runScripts: 'outside-only'});
  const {window} = dom;
  window.Element.prototype.getClientRects = function () { return [{}]; };
  Object.defineProperty(window.HTMLElement.prototype, 'innerText', {get() { return this.textContent; }});
  window.eval(companySource);
  return window.HiringplugCompany;
}

test('a companies search is read into its companies, each once, with its own LinkedIn page', () => {
  const reader = companies([
    companyCard('https://www.linkedin.com/company/contoh-logistik/?trk=search_result', 'Contoh  Logistik Sdn Bhd', 'Truck Transportation •\n Shah Alam, Selangor'),
    companyCard('https://www.linkedin.com/school/universiti-contoh/', 'Universiti Contoh', 'Higher Education • Kuala Lumpur'),
    companyCard('https://www.linkedin.com/company/Contoh-Logistik/life/', 'Contoh Logistik Sdn Bhd', 'Truck Transportation'),
    companyCard('https://www.linkedin.com/company/contoh-freight-services/', 'Contoh Freight Services', 'Freight and Package Transportation • Klang'),
  ]);
  assert.deepEqual(JSON.parse(JSON.stringify(reader.readSearch('Contoh Logistik'))), {companies: [
    {url: 'https://www.linkedin.com/company/contoh-logistik/', name: 'Contoh Logistik Sdn Bhd', subtitle: 'Truck Transportation • Shah Alam, Selangor'},
    {url: 'https://www.linkedin.com/company/contoh-freight-services/', name: 'Contoh Freight Services', subtitle: 'Freight and Package Transportation • Klang'},
  ], empty: false});
});

test('a companies search with no results, still drawing, for another name, or at a sign-in page', () => {
  const none = companies([]);
  assert.deepEqual({...none.readSearch('Contoh Logistik')}, {loading: true, cards: 0}, 'still drawing');
  const empty = new JSDOM(`<main><h2>No results found</h2></main>`, {url: searchURL('Kedai Kopi Contoh'), runScripts: 'outside-only'});
  empty.window.Element.prototype.getClientRects = function () { return [{}]; };
  Object.defineProperty(empty.window.HTMLElement.prototype, 'innerText', {get() { return this.textContent; }});
  empty.window.eval(companySource);
  assert.deepEqual(JSON.parse(JSON.stringify(empty.window.HiringplugCompany.readSearch('Kedai Kopi Contoh'))), {companies: [], empty: true});
  const card = companyCard('https://www.linkedin.com/company/contoh-logistik/', 'Contoh Logistik Sdn Bhd', 'Truck Transportation');
  assert.deepEqual({...companies([card]).readSearch('Contoh Freight')}, {failed: true}, 'a search for another name is not this one');
  assert.deepEqual({...companies([card], 'https://www.linkedin.com/search/results/people/?keywords=Contoh%20Logistik').readSearch('Contoh Logistik')}, {failed: true});
  assert.match(companies([], 'https://www.linkedin.com/checkpoint/challenge/').readSearch('Contoh Logistik').blocked, /sign in or complete a check/);
});

test("another list on the page (the side rail's suggestions) is never read as a result", () => {
  const suggestion = companyCard('https://www.linkedin.com/company/syarikat-lain/', 'Syarikat Lain Bhd', 'Retail • Penang');
  const rail = `<aside><h2>Pages people also viewed</h2><div role="list">${suggestion}</div></aside>`;
  const found = companyCard('https://www.linkedin.com/company/contoh-logistik/', 'Contoh Logistik Sdn Bhd', 'Truck Transportation');
  assert.deepEqual(JSON.parse(JSON.stringify(searchPage(results([found]) + rail).readSearch('Contoh Logistik'))), {companies: [
    {url: 'https://www.linkedin.com/company/contoh-logistik/', name: 'Contoh Logistik Sdn Bhd', subtitle: 'Truck Transportation'},
  ], empty: false}, 'no Primary content region: only the results');
  assert.deepEqual({...searchPage(rail).readSearch('Contoh Logistik')}, {loading: true, cards: 0}, 'results not drawn yet: the rail is no answer');
});

test('a page of results for another spelling, or saying it found none, is no company of this name', () => {
  const card = companyCard('https://www.linkedin.com/company/contoh-logistik/', 'Contoh Logistik Sdn Bhd', 'Truck Transportation');
  const url = searchURL('Contoh Logistk');
  const corrected = '<p>No results found for Contoh Logistk.</p><p>Showing results for <a href="#">Contoh Logistik</a></p>';
  assert.deepEqual(JSON.parse(JSON.stringify(searchPage(`<div aria-label="Primary content">${corrected}${results([card])}</div>`, url).readSearch('Contoh Logistk'))),
    {companies: [], empty: true});
  assert.deepEqual(JSON.parse(JSON.stringify(searchPage(`<p>Showing results for <b>Contoh Logistik</b></p>${results([card])}`, url).readSearch('Contoh Logistk'))),
    {companies: [], empty: true}, 'the notice alone');
  const says = companyCard('https://www.linkedin.com/company/contoh-logistik/', 'Contoh Logistik Sdn Bhd', 'Never no results for your freight');
  assert.equal(companies([says]).readSearch('Contoh Logistik').companies.length, 1, "a card's own words are not LinkedIn's notice");
});
