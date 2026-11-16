# HiringPlug LinkedIn Companion

A browser extension (Chrome, Edge and other Chromium browsers, version 116+)
that lets HiringPlug check on LinkedIn whether contacts still work at their
company, and fill a new company in from its LinkedIn page. It reads LinkedIn **in the recruiter's own browser, with their own
LinkedIn sign-in**: HiringPlug's server never reads LinkedIn.

## Install and connect

1. In HiringPlug, **Settings › LinkedIn companion › Download the companion**
   (a zip of this folder, built with the web app) and unzip it. Open the
   browser's Extensions page, turn on **Developer mode** and choose **Load
   unpacked**: the unzipped folder (or this folder, from a checkout).
2. Sign in to LinkedIn in that browser.
3. Open HiringPlug, click the companion's toolbar icon and **Connect this
   HiringPlug**. The browser asks to allow that one address; the companion
   then answers only that HiringPlug (and LinkedIn), nowhere else.

On a company's contacts, **Check employment on LinkedIn** then checks each
contact with a LinkedIn profile (the company needs its LinkedIn page set).
Adding a company with its LinkedIn page, **Fill in from LinkedIn** reads that
page's About tab: the name, industry, company size, website, headquarters and
overview (what is already typed stays; the AI later picks the closest of the
agency's industries).

## What it does, and does not

- For each contact HiringPlug asks about, it opens their LinkedIn Experience
  page in a background tab, reads the dates of their time at the company's
  exact LinkedIn page (by its link and LinkedIn's number for the company), and
  closes the tab, unless you looked at it (then it is yours and stays open).
- Like CV Studio, the HiringPlug tab and the LinkedIn tabs it opens are kept
  together in a purple "HiringPlug" tab group (in that window only).
- One read at a time, bounded (about 50 seconds at most), stoppable. It stops
  at a sign-in, check or restriction page; it never tries to get past one.
  The company read follows CV Studio's access rules (its reader's issue()):
  a checkpoint, sign-in or challenge address, a captcha the recruiter can
  see (not an invisible one), a blocking heading such as "Security
  verification", or a restriction notice within one sentence stops it, with
  the reason; a page that names itself as another company is never read.
- No AI, no LinkedIn messages, no browsing of its own; it sends the dates only
  back to the HiringPlug page that asked.
- The dates are what the profile shows, not proof: a hidden or collapsed role
  is "unclear", never "left".

## Files

- `reader.js`: CV Studio's LinkedIn reader (v24.6.585), unchanged
  (`readHRJoined`, `readHRCompany`): what is read and how dates are judged.
- `roles.js`: HiringPlug's own small reader (in the same style): when a
  contact's time at the company ended, the roles shown as current at other
  companies, with each company's LinkedIn page: where they work now.
- `company.js`: HiringPlug's reader of a company's About page (same style),
  with CV Studio's access rules.
- `worker.js`: the background worker: who may ask (connected HiringPlug pages
  only), the tabs it opens and closes, and the checks on what it returns.
- `bridge.js`: runs on the connected HiringPlug page and passes messages.
- `popup.html`, `popup.js`: connect or disconnect a HiringPlug address.
- `test/worker.test.mjs`: the worker with a fake browser
  (`node --test extension/linkedin-companion/test/worker.test.mjs`, part of
  `task test:web`).
