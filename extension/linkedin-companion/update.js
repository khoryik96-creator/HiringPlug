// The update page: finds the newest companion among the connected
// HyPlugs, writes it into the companion's folder (remembered once
// chosen) and reloads the companion (see updater.js).
const {newer, check, widens, folderVersion, write, latest} = globalThis.hiringplugUpdater;
const $ = id => document.getElementById(id);
const say = (text, problem = false) => { $('status').textContent = text; $('status').className = problem ? 'problem' : ''; };
const installed = chrome.runtime.getManifest().version;

// The chosen folder, kept in this browser (a folder handle can be stored, not its path).
function db() {
  return new Promise((resolve, reject) => {
    const open = indexedDB.open('hiringplug-companion', 1);
    open.onupgradeneeded = () => open.result.createObjectStore('folder');
    open.onsuccess = () => resolve(open.result);
    open.onerror = () => reject(open.error);
  });
}
async function stored(value) {
  const d = await db();
  return new Promise((resolve, reject) => {
    const t = d.transaction('folder', value === undefined ? 'readonly' : 'readwrite');
    const req = value === undefined ? t.objectStore('folder').get('dir') : t.objectStore('folder').put(value, 'dir');
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

let offer = null;

async function showFolder() {
  const dir = await stored().catch(() => null);
  $('folder').textContent = dir ? `Folder: ${dir.name} (chosen).` : 'Folder: not chosen yet.';
  $('choose').textContent = dir ? 'Choose another folder' : "Choose the companion's folder";
  return dir;
}

/** The folder must be the copy the browser runs: the companion's, at the version running now. */
async function checkFolder(dir) {
  const version = await folderVersion(dir);
  if (version === null) throw new Error('That folder is not the companion: choose the folder with its manifest.json (the one you loaded).');
  if (version !== installed) {
    throw new Error(`That folder holds version ${version}, but the browser runs ${installed}: choose the folder it loads the companion from (the extensions page shows it under the companion's Details, "Loaded from").`);
  }
}

async function choose() {
  if (!window.showDirectoryPicker) throw new Error('This browser cannot edit a folder from a page: update it by hand (download it again from HyPlug).');
  const dir = await window.showDirectoryPicker({id: 'hiringplug-companion', mode: 'readwrite'});
  await checkFolder(dir);
  await stored(dir);
  await showFolder();
  return dir;
}

$('choose').addEventListener('click', () => {
  choose().then(() => { say('Folder chosen.'); }, e => { if (e.name !== 'AbortError') say(e.message, true); });
});

$('update').addEventListener('click', async () => {
  if (!offer) return;
  $('update').disabled = true;
  try {
    const dir = (await stored().catch(() => null)) || await choose();
    if (await dir.requestPermission({mode: 'readwrite'}) !== 'granted') throw new Error('Editing the folder was not allowed.');
    await checkFolder(dir);
    check(offer.bundle);
    const wider = widens(JSON.parse(offer.bundle.files['manifest.json']), chrome.runtime.getManifest());
    if (wider.length) throw new Error(`This update asks for more access (${wider.join(', ')}): download it from HyPlug and load it by hand, so the browser shows what it asks for.`);
    say(`Writing version ${offer.bundle.version}…`);
    await write(dir, offer.bundle.files);
    say(`Updated to ${offer.bundle.version}. The companion restarts now: refresh your HyPlug tabs.`);
    setTimeout(() => chrome.runtime.reload(), 800); // the browser loads the folder again (this tab closes)
  } catch (e) {
    if (e.name !== 'AbortError') say(e.message || String(e), true);
    $('update').disabled = false;
  }
});

(async () => {
  await showFolder();
  const origins = (await chrome.storage.local.get('hiringplugOrigins')).hiringplugOrigins || [];
  if (!origins.length) { $('versions').textContent = `Version ${installed}. Connect a HyPlug first (the companion's icon on its tab).`; return; }
  const found = await latest(origins);
  if (!found) { $('versions').textContent = `Version ${installed}. The connected HyPlug did not answer: try again later.`; return; }
  if (!newer(found.bundle.version, installed)) { $('versions').textContent = `Version ${installed}: up to date.`; return; }
  const wider = widens(JSON.parse(found.bundle.files['manifest.json']), chrome.runtime.getManifest());
  if (wider.length) {
    $('versions').textContent = `Version ${found.bundle.version} is ready at ${found.origin}, but it asks for more access (${wider.join(', ')}): download it there and load it by hand this time, so the browser shows what it asks for.`;
    return;
  }
  offer = found;
  $('versions').textContent = `Version ${installed} installed; ${found.bundle.version} is ready at ${found.origin}.`;
  $('update').textContent = `Update to ${found.bundle.version}`;
  $('update').disabled = false;
})();
