# HiringPlug LinkedIn Companion

A browser extension (Chrome, Edge and other Chromium browsers, version 116+)
that lets HiringPlug check on LinkedIn whether contacts still work at their
company. It reads LinkedIn **in the recruiter's own browser, with their own
LinkedIn sign-in**: HiringPlug's server never reads LinkedIn.

## Install and connect

1. Open the browser's Extensions page, turn on **Developer mode** and choose
   **Load unpacked**: this folder.
2. Sign in to LinkedIn in that browser.
3. Open HiringPlug, click the companion's toolbar icon and **Connect this
   HiringPlug**. The browser asks to allow that one address; the companion
   then answers only that HiringPlug (and LinkedIn), nowhere else.

On a company's contacts, **Check employment on LinkedIn** then checks each
contact with a LinkedIn profile (the company needs its LinkedIn page set).

## What it does, and does not

- For each contact HiringPlug asks about, it opens their LinkedIn Experience
  page in a background tab, reads the dates of their time at the company's
  exact LinkedIn page (by its link and LinkedIn's number for the company), and
  closes the tab, unless you looked at it (then it is yours and stays open).
- Like CV Studio, the HiringPlug tab and the LinkedIn tabs it opens are kept
  together in a purple "HiringPlug" tab group (in that window only).
- One read at a time, bounded (about 50 seconds at most), stoppable. It stops
  at a sign-in, check or restriction page; it never tries to get past one.
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
- `worker.js`: the background worker: who may ask (connected HiringPlug pages
  only), the tabs it opens and closes, and the checks on what it returns.
- `bridge.js`: runs on the connected HiringPlug page and passes messages.
- `popup.html`, `popup.js`: connect or disconnect a HiringPlug address.
- `test/worker.test.mjs`: the worker with a fake browser
  (`node --test extension/linkedin-companion/test/worker.test.mjs`, part of
  `task test:web`).
