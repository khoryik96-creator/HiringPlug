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

const ops = {
  // CV Studio's prompt for the summary written while formatting (requestFormattingSummary).
  prompt: (r) => sandbox.cvSummaryPrompt(r.cv, sandbox.cvSummaryModifierForPreference(r.detail || 'normal'), ''),
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
