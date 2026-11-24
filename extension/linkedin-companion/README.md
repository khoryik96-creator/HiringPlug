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

## Update

From 1.4.0 the companion updates itself from the HiringPlug it is connected
to: **Settings › LinkedIn companion › Update the companion** (shown when the
HiringPlug has a newer one), or **Check for an update** under its icon, opens
its update page (`update.html`). The first time, choose the folder it was
loaded from and allow editing it; the browser remembers that folder. The page
fetches `/linkedin-companion.json` from the connected HiringPlug (its version
and files, built with the web app like the zip), writes them into the folder
(manifest last) and reloads the companion: no download, no extensions page.
It only writes files; the browser runs them after it loads the folder again,
as when updating by hand. It accepts only this companion (its manifest name)
at the version it states, with the companion's own kinds of file, and only an
update that asks for no more access than the running one (one that does is
installed by hand, so the browser shows what it asks for). The folder must be
the copy the browser runs (its version is the running one); a write that fails
puts the old files back.

On a company's contacts, **Check employment on LinkedIn** then checks each
contact with a LinkedIn profile (the company needs its LinkedIn page set).
A company's **Find HR** tab reads pages of a LinkedIn people search at that
company (one page per background tab, filtered to the country chosen; the
filters LinkedIn shows must name the company and the country) and, for the
people chosen, their joining dates. On a new contact, **Fill in from
LinkedIn** reads their name (profile page) and current roles (Experience page).
Adding a company with its LinkedIn page, **Fill in from LinkedIn** reads that
page's About tab: the name, industry, company size, website, headquarters and
overview (what is already typed stays; the AI later picks the closest of the
agency's industries).
On the Leads page, **Find HR** for a lead whose company is not in HiringPlug
yet (from 1.6.0) runs a LinkedIn companies search for the company's name and
lists up to five companies it shows, each with its own LinkedIn page
(`linkedin.com/company/…`), its name and the line under it; links to anything
else (a school, a showcase page, a person) are left out.

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
- `company.js`: HiringPlug's reader of a company's About page and of a
  companies search (same style), with CV Studio's access rules.
- `worker.js`: the background worker: who may ask (connected HiringPlug pages
  only), the tabs it opens and closes, and the checks on what it returns.
- `bridge.js`: runs on the connected HiringPlug page and passes messages.
- `popup.html`, `popup.js`: connect or disconnect a HiringPlug address; check for an update.
- `update.html`, `update.js`, `updater.js`: the self-update (see Update).
- `test/worker.test.mjs`: the worker with a fake browser,
  `test/updater.test.mjs`: the update with a fake folder, and
  `test/reader.test.mjs`: the readers on pages laid out as LinkedIn draws
  them (all with `node --test`, part of `task test:web`).
