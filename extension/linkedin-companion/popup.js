// Connects the HiringPlug in the current tab: asks the browser for access to
// that one address, and runs bridge.js there (now and on every visit).
const ORIGINS_KEY = 'hiringplugOrigins';
const status = document.getElementById('status');
const scriptId = origin => 'bridge-' + Array.from(new TextEncoder().encode(origin), b => b.toString(16).padStart(2, '0')).join('').slice(0, 120);

// Match patterns cannot hold a port: the pattern covers the host, and the
// companion still answers only the exact origin (with its port) connected.
const pattern = origin => { const u = new URL(origin); return u.protocol + '//' + u.hostname + '/*'; };

async function origins() {
  const saved = (await chrome.storage.local.get(ORIGINS_KEY))[ORIGINS_KEY];
  return Array.isArray(saved) ? saved : [];
}

async function show() {
  const list = document.getElementById('origins');
  list.replaceChildren();
  for (const origin of await origins()) {
    const item = document.createElement('li');
    item.textContent = origin;
    const remove = document.createElement('button');
    remove.type = 'button'; remove.className = 'link'; remove.textContent = 'Disconnect';
    remove.addEventListener('click', async () => {
      await chrome.scripting.unregisterContentScripts({ids: [scriptId(origin)]}).catch(() => {});
      await chrome.permissions.remove({origins: [pattern(origin)]}).catch(() => {});
      await chrome.storage.local.set({[ORIGINS_KEY]: (await origins()).filter(o => o !== origin)});
      await show();
    });
    item.append(remove);
    list.append(item);
  }
}

document.getElementById('connect').addEventListener('click', async () => {
  status.textContent = '';
  const [tab] = await chrome.tabs.query({active: true, currentWindow: true});
  let origin = '';
  try {
    const url = new URL(tab?.url || '');
    if (url.protocol === 'https:' || ['localhost', '127.0.0.1'].includes(url.hostname)) origin = url.origin;
  } catch { /* not a web page */ }
  if (!origin || origin === 'https://www.linkedin.com') { status.textContent = 'Open HiringPlug in this tab first.'; return; }
  if (!await chrome.permissions.request({origins: [pattern(origin)]})) { status.textContent = 'Not connected: access was not allowed.'; return; }
  await chrome.scripting.unregisterContentScripts({ids: [scriptId(origin)]}).catch(() => {});
  await chrome.scripting.registerContentScripts([{id: scriptId(origin), matches: [pattern(origin)], js: ['bridge.js'], runAt: 'document_idle'}]);
  await chrome.scripting.executeScript({target: {tabId: tab.id}, files: ['bridge.js']});
  const all = await origins();
  if (!all.includes(origin)) await chrome.storage.local.set({[ORIGINS_KEY]: [...all, origin]});
  status.textContent = 'Connected. Go back to HiringPlug.';
  await show();
});

void show();
