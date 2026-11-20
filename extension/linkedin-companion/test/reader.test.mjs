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
