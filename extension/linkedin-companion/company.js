/*
 * HiringPlug: on a LinkedIn company's About page, what the page itself says
 * of the company: its name, industry, company size, website and
 * headquarters, and its overview. Read in a background tab the companion
 * opened at that exact company page, for a new company in HiringPlug.
 * On a LinkedIn companies search (readSearch), the companies it shows, each
 * with its own LinkedIn page, for a lead whose company is not yet linked.
 * Written in the style of CV Studio's reader: only the page asked for, only
 * what is shown, nothing guessed. Runs in the extension's isolated world; no
 * network.
 */
(() => {
  const compact = value => String(value || '').replace(/\s+/g, ' ').trim();
  const visible = el => !!el && !!el.getClientRects().length && getComputedStyle(el).visibility !== 'hidden';
  const text = el => compact(el?.innerText || '');

  function companyURL(value) {
    try {
      const u = new URL(value);
      const match = u.pathname.match(/^\/company\/([A-Za-z0-9_-]+)(?:\/.*)?$/);
      return u.origin === 'https://www.linkedin.com' && !u.username && !u.password && match ? u.origin + '/company/' + match[1] + '/' : '';
    } catch { return ''; }
  }

  // CV Studio's access rules (vendor/linkedin-companion/reader.js, issue()),
  // learnt over many versions: a checkpoint, sign-in or challenge address, a
  // captcha the recruiter can actually see (not an invisible or parked one),
  // a blocking heading, or a restriction notice within one sentence stops
  // the read, with the reason. It is never worked around.
  const restrictionNotice = /unusual activity|temporarily restricted|security verification|too many requests|you(?:.ve| have) reached[^.!?]{0,120}\b(?:(?:search|commercial)[^.!?]{0,120}\blimit|limit[^.!?]{0,120}\b(?:search|commercial))/i;
  function challengeShown() {
    const nodes = [...document.querySelectorAll('iframe[src*="captcha" i], [data-test-id="captcha-internal"], #captcha-internal')];
    return nodes.some(node => {
      const src = String(node.getAttribute('src') || ''), frame = node.tagName === 'IFRAME';
      if (frame && /[?&]size=invisible(?:&|$)/i.test(src)) return false;
      if (!visible(node)) return false;
      const box = node.getBoundingClientRect?.();
      return !box || (box.width >= 40 && box.height >= 40 && box.bottom > 0 && box.right > 0);
    });
  }
  // Notice text counts only while no About section was recognised: a
  // company's own overview may well say "unusual activity" or "security
  // verification" (CV Studio checks a job ad's page text the same way).
  function issue(pageText) {
    if (/^\/(?:checkpoint|challenge|authwall|login|uas)(?:\/|$)/i.test(location.pathname)) return 'LinkedIn needs you to sign in or complete a check. Nothing was read.';
    if (challengeShown()) return 'LinkedIn needs a verification check. Nothing was read.';
    const blocking = [...document.querySelectorAll('h1,h2,[role="heading"]')].filter(visible)
      .some(node => /^(?:sign in(?: to linkedin)?|security verification|verify your identity|temporarily restricted)$/i.test(text(node)));
    if (blocking || (pageText && restrictionNotice.test(text(document.querySelector('main') || document.body).slice(0, 5000)))) {
      return 'LinkedIn is showing a sign-in, access or usage restriction. Nothing was read.';
    }
    return '';
  }
  // The page's own address for itself, when it gives one, must be this company too.
  function sameCompany(expected) {
    const named = [...document.querySelectorAll('link[rel="canonical"], meta[property="og:url"]')]
      .map(node => companyURL(node.getAttribute('href') || node.getAttribute('content') || '')).filter(Boolean);
    return named.every(u => u.toLowerCase() === expected.toLowerCase());
  }

  // The facts in the About section: "Industry" -> "Truck Transportation"
  // (the first line under each heading; "Company size" has a second line
  // with members on LinkedIn, which is left out).
  function facts(root) {
    const out = {};
    for (const list of root.querySelectorAll('dl')) {
      let label = '';
      for (const el of list.children) {
        if (el.tagName === 'DT') label = text(el).toLowerCase();
        else if (el.tagName === 'DD' && label && !(label in out) && visible(el)) out[label] = text(el);
      }
    }
    return out;
  }

  function readAbout(expectedURL) {
    const main = document.querySelector('main') || document.body;
    const known = facts(main);
    const problem = issue(!Object.keys(known).length);
    if (problem) return {blocked: problem};
    // Another company (or a renamed page) is never read as this one; LinkedIn's
    // addresses ignore case, so neither does the comparison.
    if (companyURL(location.href).toLowerCase() !== expectedURL.toLowerCase() || !/^\/company\/[^/]+\/about\/?$/.test(location.pathname) ||
        !sameCompany(expectedURL)) return {failed: true};
    const heading = [...main.querySelectorAll('h1')].find(visible);
    if (!heading || !Object.keys(known).length) return {loading: true};
    const overview = [...main.querySelectorAll('section')].find(s => /^overview$/i.test(text(s.querySelector('h2'))));
    const about = text(overview?.querySelector('p'));
    const clip = (v, n) => compact(v).slice(0, n);
    return {
      name: clip(text(heading), 200),
      industry: clip(known.industry || known.industries, 200),
      size: clip(known['company size'], 100),
      website: clip(known.website, 300),
      headquarters: clip(known.headquarters, 200),
      about: clip(about, 4000),
    };
  }

  // The visible text of root outside the given cards.
  function pageWords(root, cards) {
    if (!root) return '';
    const parts = [], walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      if (visible(node.parentElement) && !cards.some(card => card.contains(node))) parts.push(node.nodeValue);
    }
    return compact(parts.join(' '));
  }

  // A companies search (Find HR for a lead): the result cards, each with the
  // company's own page, its name and the line under it (industry, place).
  // Links to anything but a company page (a school, a showcase, a person)
  // are left out, and a company shown twice is listed once. Only cards in the
  // search results themselves count, never another list on the page (the
  // side rail's suggestions).
  function readSearch(expectedName) {
    if (location.origin !== 'https://www.linkedin.com') return {failed: true};
    const main = document.querySelector('main');
    const primary = main?.querySelector('[aria-label="Primary content"]');
    const scope = primary || main;
    // Any list counts only inside the results region (the side rail is outside it), as the people search reads.
    const selector = '[componentkey^="SearchResults_"] [role="listitem"], li.reusable-search__result-container, .entity-result' +
      (primary ? ', [role="list"] > [role="listitem"], [role="list"] > * > [role="listitem"]' : '');
    const cards = scope ? [...scope.querySelectorAll(selector)].filter(n => visible(n) && !n.parentElement?.closest(selector)) : [];
    const problem = issue(!cards.length);
    if (problem) return {blocked: problem};
    const keywords = new URLSearchParams(location.search).get('keywords');
    if (!/^\/search\/results\/companies\/?$/.test(location.pathname) || compact(keywords).toLowerCase() !== compact(expectedName).toLowerCase()) {
      return {failed: true};
    }
    if (!scope) return {loading: true};
    const companies = [], seen = new Set();
    for (const card of cards) {
      const legacy = card.querySelector('.entity-result__title-text a[href]');
      const paragraphs = [...card.querySelectorAll('p')].filter(visible);
      const link = legacy || paragraphs[0]?.querySelector('a[href]') || paragraphs[0]?.closest('a[href]');
      const url = companyURL(link?.href || '');
      const name = compact(legacy ? text(legacy) : text(paragraphs[0])).slice(0, 200);
      const subtitle = compact(legacy ? text(card.querySelector('.entity-result__primary-subtitle')) : text(paragraphs[1])).slice(0, 300);
      if (!url || !name || seen.has(url.toLowerCase())) continue;
      seen.add(url.toLowerCase());
      companies.push({url, name, subtitle});
      if (companies.length >= 10) break;
    }
    // LinkedIn saying it found nothing for this name, or that it shows
    // results for another (a spelling it corrected), means no company of this
    // name, whatever cards it shows instead. Only the page's own words count,
    // not what a card says of its company.
    if (/\b(?:no results found|no results|0 results|showing results for)\b/i.test(pageWords(main, cards))) return {companies: [], empty: true};
    // Cards that show no company yet may still be drawing: wait (the worker
    // says, if it stays so, that LinkedIn showed no company page).
    if (!companies.length) return {loading: true, cards: cards.length};
    return {companies, empty: false};
  }

  globalThis.HiringplugCompany = {readAbout, readSearch};
})();
