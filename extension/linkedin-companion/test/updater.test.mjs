// The companion's self-update: which bundles it accepts from HiringPlug, and
// how it writes them into its folder (a fake folder, as the browser gives one).
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {test} from 'node:test';
import vm from 'node:vm';

const context = vm.createContext({});
context.globalThis = context;
vm.runInContext(readFileSync(new URL('../updater.js', import.meta.url), 'utf8'), context);
const {newer, check, isCompanionFolder, write, latest, NAME} = context.hiringplugUpdater;

const manifest = (version, name = NAME) => JSON.stringify({manifest_version: 3, name, version});
const bundle = (version, files = {}) => ({version, files: {'manifest.json': manifest(version), 'worker.js': '// v' + version, ...files}});

/** A folder of files in memory with the browser's folder-handle methods; writes are logged in order. */
function folder(files = {}, log = [], name = 'hiringplug-linkedin-companion', prefix = '') {
  const dirs = {};
  return {
    name, files, log,
    async getFileHandle(n, opts = {}) {
      if (!(n in files) && !opts.create) throw Object.assign(new Error('not found'), {name: 'NotFoundError'});
      return {
        getFile: async () => ({text: async () => files[n]}),
        createWritable: async () => {
          let data = '';
          return {write: async d => { data += d; }, close: async () => { files[n] = data; log.push(prefix + n); }};
        },
      };
    },
    async getDirectoryHandle(n, opts = {}) {
      if (!dirs[n]) { if (!opts.create) throw new Error('not found'); dirs[n] = folder({}, log, n, prefix + n + '/'); }
      return dirs[n];
    },
  };
}

test('knows which version is newer', () => {
  assert.equal(newer('1.4.0', '1.3.9'), true);
  assert.equal(newer('1.10.0', '1.9.0'), true);
  assert.equal(newer('1.4', '1.4.0'), false);
  assert.equal(newer('1.3.0', '1.4.0'), false);
});

test('accepts only this companion, at the version it says, with its own kinds of file', () => {
  assert.deepEqual([...check(bundle('1.4.0'))].sort(), ['manifest.json', 'worker.js']);
  assert.throws(() => check({version: '1.4.0', files: {'manifest.json': manifest('1.4.0', 'Another extension')}}), /not this companion/);
  assert.throws(() => check({version: '1.5.0', files: {'manifest.json': manifest('1.4.0')}}), /not this companion/, 'the version must match its manifest');
  assert.throws(() => check(bundle('1.4.0', {'../escape.js': 'x'})), /unexpected file/);
  assert.throws(() => check(bundle('1.4.0', {'.hidden.js': 'x'})), /unexpected file/);
  assert.throws(() => check(bundle('1.4.0', {'tool.exe': 'x'})), /unexpected file/);
  assert.throws(() => check({version: 'latest', files: {}}), /did not send a companion/);
});

test('writes the files into the folder, its manifest last, making folders as needed', async () => {
  const log = [];
  const dir = folder({'manifest.json': manifest('1.3.0'), 'old.js': 'kept'}, log);
  assert.equal(await isCompanionFolder(dir), true);
  assert.equal(await isCompanionFolder(folder({'manifest.json': manifest('1.0', 'Other')})), false);
  assert.equal(await isCompanionFolder(folder({})), false, 'a folder with no manifest');
  await write(dir, bundle('1.4.0', {'lib/util.js': '// util'}).files);
  assert.equal(log[log.length - 1], 'manifest.json', 'a half-written update never loads as the new version');
  assert.ok(log.includes('lib/util.js'));
  assert.equal(JSON.parse(dir.files['manifest.json']).version, '1.4.0');
  assert.equal(dir.files['worker.js'], '// v1.4.0');
});

test('takes the newest companion among the connected HiringPlugs, ignoring any that fail or send something else', async () => {
  const answers = {
    'https://a.test/linkedin-companion.json': {ok: true, json: async () => bundle('1.4.0')},
    'https://b.test/linkedin-companion.json': {ok: true, json: async () => bundle('1.5.0')},
    'https://c.test/linkedin-companion.json': {ok: true, json: async () => ({version: '9.9.9', files: {'manifest.json': manifest('9.9.9', 'Evil')}})},
    'https://d.test/linkedin-companion.json': {ok: false},
  };
  const asked = [];
  const get = async (url, opts) => { asked.push(opts); if (url.startsWith('https://e.test')) throw new Error('offline'); return answers[url]; };
  const best = await latest(['https://a.test', 'https://b.test', 'https://c.test', 'https://d.test', 'https://e.test'], get);
  assert.equal(best.origin, 'https://b.test');
  assert.equal(best.bundle.version, '1.5.0');
  assert.equal(asked[0].credentials, 'omit', 'no cookies sent');
  assert.equal(await latest([], get), null);
});
