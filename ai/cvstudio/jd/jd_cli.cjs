'use strict';
// HiringPlug's adapter (new glue, not CV Studio code): CV Studio's JD blinding
// functions and prompt, unchanged, for the Python AI worker. It reads one
// JSON request on stdin, {"op": ..., ...}, and writes one JSON answer.
//
// - jd_guardrails.cjs: CV Studio v24.6.584's own local JD helpers (exact
//   function bodies from vendor/cvstudio/blind-jd.js and ai-proxy.js).
// - prompts/JD_PROMPT_BUILDER.js.txt: CV Studio's exact prompt assembly from
//   generateAnonJD (blind-jd.js), run here with the options it reads from its
//   form. The industry instruction and tones below are copied from the same
//   function (blind-jd.js lines 553-566), word for word.
const fs = require('node:fs');
const path = require('node:path');
const guards = require('./jd_guardrails.cjs');

const BUILDER = fs.readFileSync(path.join(__dirname, 'prompts', 'JD_PROMPT_BUILDER.js.txt'), 'utf8');

function industryInstructionOf(industry, companyName) {
  if (industry) {
    return 'Industry/sector: ' + industry + ' — use this directly.';
  } else if (companyName) {
    return 'Company name provided: "' + companyName + '". Determine their primary industry from the JD content and company name.';
  }
  return 'No industry or company provided. Infer the industry from the JD content (tech stack, role type, domain language, regulatory references).';
}

const toneMap = {
  professional:   'formal, professional, third-person where appropriate',
  conversational: 'warm, engaging, first-person plural ("we are looking for…")',
  concise:        'concise and direct — no fluff, bullet-heavy, scannable'
};

function buildPrompt(o) {
  const vars = {
    raw: o.raw || '',
    clientDesc: o.clientDesc || '',
    workMode: o.workMode || '',
    location: o.location || '',
    context: o.context || '',
    redact: o.redact || '',
    whyJoinUs: o.whyJoinUs || '',
    paraphraseInstruction: guards.jdAnonParaphraseStrengthInstruction(String(o.strength || 'medium').toLowerCase()),
    industryInstruction: industryInstructionOf(o.industry || '', o.companyName || ''),
    toneMap: toneMap,
    _anonTone: o.tone || 'professional',
  };
  const names = Object.keys(vars);
  // eslint-disable-next-line no-new-func -- CV Studio's own excerpt, vendored with this file
  const run = new Function(...names, BUILDER + '\nreturn prompt;');
  return run(...names.map((n) => vars[n]));
}

const ops = {
  normalize: (r) => guards.anonNormalizeExtractedTextArtifacts(r.raw),
  prompt: (r) => buildPrompt(r),
  issues: (r) => guards.jdAnonFindParaphraseIssues(r.jd, r.raw),
  needsRetry: (r) => guards.jdAnonNeedsParaphraseRetry(r.jd, r.raw),
  retryInstruction: (r) => guards.jdAnonParaphraseRetryInstruction(r.issues),
  missing: (r) => guards.jdAnonMissingPreservedTechTerms(r.jd, r.raw, r.companyName || ''),
  finalize: (r) => guards.finalizeJD(r.jd, r.raw, r.companyName || ''),
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
    process.stdout.write(JSON.stringify({ ok: false, error: String((e && e.message) || e).slice(0, 300) }));
  }
});
