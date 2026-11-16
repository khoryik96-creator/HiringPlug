/*
 * HiringPlug: on a LinkedIn company's About page, what the page itself says
 * of the company: its name, industry, company size, website and
 * headquarters, and its overview. Read in a background tab the companion
 * opened at that exact company page, for a new company in HiringPlug.
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
      return !box || !box.width || (box.width >= 40 && box.height >= 40 && box.bottom > 0 && box.right > 0);
    });
  }
  function issue() {
    if (/^\/(?:checkpoint|challenge|authwall|login|uas)(?:\/|$)/i.test(location.pathname)) return 'LinkedIn needs you to sign in or complete a check. Nothing was read.';
    if (challengeShown()) return 'LinkedIn needs a verification check. Nothing was read.';
    const blocking = [...document.querySelectorAll('h1,h2,[role="heading"]')].filter(visible)
      .some(node => /^(?:sign in(?: to linkedin)?|security verification|verify your identity|temporarily restricted)$/i.test(text(node)));
    if (blocking || restrictionNotice.test(text(document.querySelector('main') || document.body).slice(0, 5000))) {
      return 'LinkedIn is showing a sign-in, access or usage restriction. Nothing was read.';
    }
    return '';
  }
  // The page's own address for itself, when it gives one, must be this company too.
  function sameCompany(expected) {
    const named = [...document.querySelectorAll('link[rel="canonical"], meta[property="og:url"]')]
      .map(node => companyURL(node.getAttribute('href') || node.getAttribute('content') || '')).filter(Boolean);
    return named.every(u => u === expected);
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
    const problem = issue();
    if (problem) return {blocked: problem};
    // Another company (or a renamed page) is never read as this one.
    if (companyURL(location.href) !== expectedURL || !/^\/company\/[^/]+\/about\/?$/.test(location.pathname) || !sameCompany(expectedURL)) return {failed: true};
    const main = document.querySelector('main') || document.body;
    const heading = [...main.querySelectorAll('h1')].find(visible);
    const known = facts(main);
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

  globalThis.HiringplugCompany = {readAbout};
})();
