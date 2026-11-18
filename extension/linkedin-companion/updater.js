/*
 * The companion updates itself from the HiringPlug it is connected to (no
 * store, no download): HiringPlug serves /linkedin-companion.json (the
 * version and files built with it); the update page writes those files into
 * the folder the browser loads the companion from (chosen once; the browser
 * asks to allow writing) and reloads it. Only files: nothing fetched is run
 * before the browser loads the folder again, as when updating by hand.
 */
(() => {
  const NAME = 'HiringPlug LinkedIn Companion';
  // The companion's own kinds of file, in folders of plain names (no "..", no hidden files).
  const PATH = /^[a-z0-9_-]+(\/[a-z0-9_-]+)*\.(js|json|html|css|md|txt)$/i;

  /** Whether version a ("1.4.0") is newer than b. */
  function newer(a, b) {
    const x = String(a).split('.').map(Number), y = String(b).split('.').map(Number);
    for (let i = 0; i < Math.max(x.length, y.length); i++) {
      const d = (x[i] || 0) - (y[i] || 0);
      if (d) return d > 0;
    }
    return false;
  }

  /** The bundle's file paths, once it is known to be this companion at its stated version. */
  function check(bundle) {
    if (!bundle || typeof bundle.version !== 'string' || !/^\d+(\.\d+){0,3}$/.test(bundle.version) || !bundle.files || typeof bundle.files !== 'object') {
      throw new Error('This HiringPlug did not send a companion to update to.');
    }
    const paths = Object.keys(bundle.files);
    for (const p of paths) {
      if (!PATH.test(p) || typeof bundle.files[p] !== 'string') throw new Error('The update has an unexpected file: ' + p.slice(0, 80));
    }
    let manifest;
    try { manifest = JSON.parse(bundle.files['manifest.json']); } catch { throw new Error('The update has no manifest.'); }
    if (manifest.name !== NAME || manifest.version !== bundle.version) throw new Error('The update is not this companion.');
    return paths;
  }

  /** Whether the folder chosen is the companion's own (its manifest names it). */
  async function isCompanionFolder(dir) {
    try {
      const file = await (await dir.getFileHandle('manifest.json')).getFile();
      return JSON.parse(await file.text()).name === NAME;
    } catch {
      return false;
    }
  }

  /** Writes the files into the folder; the manifest last, so a half-written update never loads as the new version. */
  async function write(dir, files) {
    const order = Object.keys(files).filter(p => p !== 'manifest.json').concat('manifest.json');
    for (const path of order) {
      const parts = path.split('/');
      let folder = dir;
      for (const part of parts.slice(0, -1)) folder = await folder.getDirectoryHandle(part, {create: true});
      const out = await (await folder.getFileHandle(parts[parts.length - 1], {create: true})).createWritable();
      await out.write(files[path]);
      await out.close();
    }
  }

  /** The newest companion among the connected HiringPlugs, with where it came from (null if none answered). */
  async function latest(origins, get = fetch) {
    let best = null;
    for (const origin of origins) {
      try {
        const res = await get(origin + '/linkedin-companion.json', {cache: 'no-store', credentials: 'omit'});
        if (!res.ok) continue;
        const bundle = await res.json();
        check(bundle);
        if (!best || newer(bundle.version, best.bundle.version)) best = {origin, bundle};
      } catch { /* that HiringPlug has no companion to offer */ }
    }
    return best;
  }

  globalThis.hiringplugUpdater = {NAME, newer, check, isCompanionFolder, write, latest};
})();
