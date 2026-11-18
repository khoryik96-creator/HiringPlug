// The update page: finds the newest companion among the connected
// HiringPlugs, writes it into the companion's folder (remembered once
// chosen) and reloads the companion (see updater.js).
const {newer, check, isCompanionFolder, write, latest} = globalThis.hiringplugUpdater;
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

async function choose() {
  if (!window.showDirectoryPicker) throw new Error('This browser cannot edit a folder from a page: update it by hand (download it again from HiringPlug).');
  const dir = await window.showDirectoryPicker({id: 'hiringplug-companion', mode: 'readwrite'});
  if (!await isCompanionFolder(dir)) throw new Error('That folder is not the companion: choose the folder with its manifest.json (the one you loaded).');
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
    if (!await isCompanionFolder(dir)) throw new Error('The folder chosen is no longer the companion: choose it again.');
    check(offer.bundle);
    say(`Writing version ${offer.bundle.version}…`);
    await write(dir, offer.bundle.files);
    say(`Updated to ${offer.bundle.version}. The companion restarts now: refresh your HiringPlug tabs.`);
    setTimeout(() => chrome.runtime.reload(), 800); // the browser loads the folder again (this tab closes)
  } catch (e) {
    if (e.name !== 'AbortError') say(e.message || String(e), true);
    $('update').disabled = false;
  }
});

(async () => {
  await showFolder();
  const origins = (await chrome.storage.local.get('hiringplugOrigins')).hiringplugOrigins || [];
  if (!origins.length) { $('versions').textContent = `Version ${installed}. Connect a HiringPlug first (the companion's icon on its tab).`; return; }
  const found = await latest(origins);
  if (!found) { $('versions').textContent = `Version ${installed}. The connected HiringPlug did not answer: try again later.`; return; }
  if (!newer(found.bundle.version, installed)) { $('versions').textContent = `Version ${installed}: up to date.`; return; }
  offer = found;
  $('versions').textContent = `Version ${installed} installed; ${found.bundle.version} is ready at ${found.origin}.`;
  $('update').textContent = `Update to ${found.bundle.version}`;
  $('update').disabled = false;
})();
