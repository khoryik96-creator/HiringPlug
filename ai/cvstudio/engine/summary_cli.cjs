'use strict';
// HiringPlug's adapter (new glue, not CV Studio code): CV Studio's "About Him /
// Her" summary helpers from vendor/cvstudio/candidate-summary.js, unchanged,
// for the Python AI worker. That file is browser code: it is loaded in a
// sandbox where the page (document, window, storage) is a stand-in that does
// nothing, and only its pure functions are called. One JSON request on
// stdin, {"op": ...}, one JSON answer on stdout.
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const page = new Proxy(function () {}, { get: () => page, apply: () => page, construct: () => page });
const sandbox = { window: {}, document: page, localStorage: page, sessionStorage: page, console };
vm.createContext(sandbox);
vm.runInContext(fs.readFileSync(path.join(__dirname, 'vendor', 'cvstudio', 'candidate-summary.js'), 'utf8'), sandbox);

// CV Studio's lengths: short 4-5 bullets ("shorter"), normal 6-7, detailed 8-10 ("longer").
const MODIFIER = { short: 'shorter', normal: 'normal', detailed: 'longer' };

// CV Studio's focus (setSummaryFocus/getSummaryFocusPrompt: General, Technology,
// Leadership, Commercial), its own wording; and the recruiter's direction and
// tone, which CV Studio takes as a custom focus. One rule line in the prompt,
// under its other rules (never invent, never pay).
function focusNote(focus, direction) {
  sandbox._summaryFocus = ['technology', 'leadership', 'commercial'].includes(focus) ? focus : 'general';
  const preset = String(sandbox.getSummaryFocusPrompt() || '');
  const own = String(direction || '').replace(/\s+/g, ' ').trim();
  const custom = own ? 'Follow the recruiter\'s direction on what to bring out and the tone, within every rule above: ' + own + '.' : '';
  return [preset, custom].filter(Boolean).join(' ');
}

const ops = {
  // CV Studio's prompt for the summary written while formatting (requestFormattingSummary).
  prompt: (r) => sandbox.cvSummaryPrompt(r.cv, MODIFIER[r.detail] || 'normal', focusNote(r.focus, r.direction)),
  // The AI's answer as CV Studio reads it: bullet lines, at most 20.
  bullets: (r) => Array.from(sandbox.summaryBulletLines(r.text)),
};

let input = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', (chunk) => { input += chunk; });
process.stdin.on('end', () => {
  try {
    const request = JSON.parse(input);
    const op = ops[request.op];
    if (!op) throw new Error('unknown op');
    process.stdout.write(JSON.stringify({ ok: true, value: op(request) }));
  } catch (e) {
    process.stdout.write(JSON.stringify({ ok: false }));
  }
});
