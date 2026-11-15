/*
 * Runs on the HiringPlug address the recruiter connected (no other site):
 * passes the page's requests to the companion and its answers back, to this
 * page only. The page asks with window.postMessage(
 *   {source: 'hiringplug-app', type: 'call', id, action, args}, location.origin).
 */
(() => {
  if (globalThis.hiringplugBridge) return;
  globalThis.hiringplugBridge = true;
  const send = message => window.postMessage({source: 'hiringplug-companion', ...message}, location.origin);
  const announce = () => send({type: 'ready', version: chrome.runtime.getManifest().version});
  window.addEventListener('message', event => {
    const m = event.data;
    if (event.source !== window || event.origin !== location.origin || !m || m.source !== 'hiringplug-app') return;
    if (m.type === 'hello') { announce(); return; }
    if (m.type !== 'call' || typeof m.id !== 'string' || m.id.length > 64 || typeof m.action !== 'string') return;
    chrome.runtime.sendMessage({action: m.action, args: m.args}).then(
      reply => { send({type: 'reply', id: m.id, ...reply}); },
      () => { send({type: 'reply', id: m.id, error: 'The companion was reloaded. Refresh this page.'}); },
    );
  });
  announce();
})();
