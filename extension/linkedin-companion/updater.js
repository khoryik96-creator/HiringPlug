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

  // What a manifest may reach: an update that asks for more is installed by hand,
  // so the browser shows what it asks for (an unpacked companion would get it silently).
  const ACCESS = ['permissions', 'optional_permissions', 'host_permissions', 'optional_host_permissions',
    'content_scripts', 'externally_connectable', 'web_accessible_resources', 'content_security_policy', 'key'];

  /** The access the new manifest asks for that the running one does not have (empty: none). */
  function widens(next, current) {
    const more = [];
    for (const key of ACCESS) {
      const a = next[key], b = current[key];
      if (a === undefined) continue;
      if (Array.isArray(a) && a.every(x => typeof x === 'string') && (b === undefined || Array.isArray(b))) {
        if (a.some(x => !(b || []).includes(x))) more.push(key);
      } else if (JSON.stringify(a) !== JSON.stringify(b)) {
        more.push(key);
      }
    }
    return more;
  }

  /** The companion's version in the folder chosen (null: not the companion's folder). */
  async function folderVersion(dir) {
    try {
      const file = await (await dir.getFileHandle('manifest.json')).getFile();
      const manifest = JSON.parse(await file.text());
      return manifest.name === NAME && typeof manifest.version === 'string' ? manifest.version : null;
    } catch {
      return null;
    }
  }

  async function handle(dir, path, create) {
    const parts = path.split('/');
    let folder = dir;
    for (const part of parts.slice(0, -1)) folder = await folder.getDirectoryHandle(part, {create});
    return folder.getFileHandle(parts[parts.length - 1], {create});
  }

  async function put(dir, path, text) {
    const out = await (await handle(dir, path, true)).createWritable();
    await out.write(text);
    await out.close();
  }

  /**
   * Writes the files into the folder, the manifest last (a half-written update
   * never loads as the new version). If a write fails, the files already
   * written get their old contents back, so the folder stays one version.
   */
  async function write(dir, files) {
    const order = Object.keys(files).filter(p => p !== 'manifest.json').concat('manifest.json');
    const done = []; // [path, old contents or null when it is new]
    try {
      for (const path of order) {
        let old = null;
        try { old = await (await (await handle(dir, path, false)).getFile()).text(); } catch { /* a new file */ }
        await put(dir, path, files[path]);
        done.push([path, old]);
      }
    } catch (error) {
      for (const [path, old] of done.reverse()) {
        try {
          if (old === null) {
            const parts = path.split('/');
            let folder = dir;
            for (const part of parts.slice(0, -1)) folder = await folder.getDirectoryHandle(part);
            await folder.removeEntry(parts[parts.length - 1]);
          } else {
            await put(dir, path, old);
          }
        } catch { /* best effort: the next update writes it again */ }
      }
      throw new Error('The update could not be written, so nothing was changed: ' + (error && error.message || error));
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

  globalThis.hiringplugUpdater = {NAME, newer, check, widens, folderVersion, write, latest};
})();
