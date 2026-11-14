/*
 * HiringPlug: on a LinkedIn profile's Experience page, the roles shown as
 * current ("… – Present") at other companies, each with its company's exact
 * LinkedIn page: where a contact who left works now. Read in the same
 * background tab, right after CV Studio's reader (reader.js) has read their
 * time at the contact's company. Written in the style of that reader: only
 * the Experience section, only roles with one company page link, dates as
 * precise as shown and never computed from durations. Runs in the
 * extension's isolated world; no network.
 */
(() => {
  const compact = value => String(value || '').replace(/\s+/g, ' ').trim();
  const visible = el => !!el && !!el.getClientRects().length && getComputedStyle(el).visibility !== 'hidden';
  const text = el => compact(el?.innerText || '');
  const months = ['january', 'february', 'march', 'april', 'may', 'june', 'july', 'august', 'september', 'october', 'november', 'december'];

  function companyURL(value) {
    try {
      const u = new URL(value);
      const match = u.pathname.match(/^\/company\/([A-Za-z0-9_-]+)(?:\/.*)?$/);
      return u.origin === 'https://www.linkedin.com' && !u.username && !u.password && match ? u.origin + '/company/' + match[1] + '/' : '';
    } catch { return ''; }
  }
  function date(value) {
    const match = compact(value).match(/^(?:([A-Za-z]+) )?((?:19|20)\d{2})$/);
    if (!match) return null;
    const month = match[1] ? months.findIndex(name => [name, name.slice(0, 3), name === 'september' ? 'sept' : name].includes(match[1].toLowerCase())) + 1 : 0;
    if (match[1] && !month) return null;
    return {value: match[2] + (month ? '-' + String(month).padStart(2, '0') : ''), order: Number(match[2]) * 12 + (month || 1)};
  }
  // "Sep 2025 - Present · 3 mos": a current role and when it started.
  function currentSince(words) {
    const match = compact(words).match(/^((?:[A-Za-z]+ )?(?:19|20)\d{2})\s*[-–—]\s*Present(?:\s*·.*)?$/i);
    return match ? date(match[1]) : null;
  }
  const isRange = words => /^((?:[A-Za-z]+ )?(?:19|20)\d{2})\s*[-–—]\s*(Present|(?:[A-Za-z]+ )?(?:19|20)\d{2})(?:\s*·.*)?$/i.test(compact(words));

  function readCurrentRoles(expectedURL, leftCompany) {
    const page = new URL(location.href);
    let expected;
    try { expected = new URL(expectedURL); } catch { throw Error('The profile changed.'); }
    if (page.origin !== 'https://www.linkedin.com' || page.pathname.replace(/\/$/, '') !== expected.pathname.replace(/\/$/, '') + '/details/experience') {
      throw Error('The profile changed.');
    }
    const main = document.querySelector('main');
    if (!main) return [];
    const scopes = [...main.querySelectorAll('[componentkey$="ExperienceDetailsSection"]')].filter(visible);
    const scope = scopes.length === 1 ? scopes[0] : main.querySelector('#experience')?.closest('section');
    if (!scope) return [];
    const selector = '[componentkey^="entity-collection-item-"], li.artdeco-list__item';
    const cards = [...scope.querySelectorAll(selector)].filter(n => visible(n) && !n.parentElement?.closest?.(selector));
    const roles = [];
    for (const card of cards) {
      const links = [...card.querySelectorAll('a[href*="/company/"]')].filter(visible);
      const pages = [...new Set(links.map(a => companyURL(a.href)).filter(Boolean))];
      if (pages.length !== 1 || pages[0] === leftCompany) continue; // no page, several, or the company they left
      const lines = [...new Set([...card.querySelectorAll('p, span[aria-hidden="true"], .pvs-entity__caption-wrapper, time')]
        .filter(visible).map(text).filter(Boolean))];
      const starts = lines.map(currentSince).filter(Boolean);
      if (!starts.length) continue; // not a current role
      const since = starts.reduce((a, b) => (b.order < a.order ? b : a));
      // The company's name: its link's label or logo, else the line naming it ("Acme Bhd · Full-time").
      const logo = links.map(a => a.querySelector('img[alt]')?.getAttribute('alt') || '').find(Boolean) || '';
      let name = compact(links.map(a => a.getAttribute('aria-label') || '').find(Boolean) || logo.replace(/\s*logo$/i, ''));
      if (!name) name = (lines.find(l => / · /.test(l) && !isRange(l)) || '').split(' · ')[0];
      if (!name) continue;
      const title = lines.find(l => l !== name && !isRange(l) && !l.startsWith(name + ' · ') && !/ · /.test(l)) || '';
      roles.push({company_url: pages[0], company_name: name.slice(0, 200), title: title.slice(0, 200), since: since.value});
      if (roles.length === 3) break;
    }
    return roles;
  }
  globalThis.HiringplugRoles = {readCurrentRoles};
})();
