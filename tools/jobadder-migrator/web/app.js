'use strict';
const $ = id => document.getElementById(id);
const fragment = location.hash.slice(1);
if (fragment) {
    sessionStorage.setItem('migration-session', fragment);
    history.replaceState(null, '', location.pathname);
}
const session = sessionStorage.getItem('migration-session') || '';
let state = {}, busy = false, downloading = false, healthy = false, actionError = '', statusError = '';
const coolingDown = () => state.retryAt && Date.parse(state.retryAt) > Date.now();
const labels = { counted: 'Count reported', definitions: 'Definition response saved', denied: 'Permission denied', unavailable: 'Not available', needs_review: 'Needs review', failed: 'Request failed', reconnect: 'Reconnect required', rate_limited: 'Rate limited', deferred: 'Requires candidate-level discovery' };
function renderError() { const message = statusError || actionError; $('error').textContent = message; $('error').hidden = !message; }
function error(message) { actionError = message || ''; renderError(); }
async function request(path, body) {
    const response = await fetch(path, { method: body === undefined ? 'GET' : 'POST', headers: { 'X-Migration-Session': session, ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) }, body: body === undefined ? undefined : JSON.stringify(body), cache: 'no-store' });
    if (!response.ok) {
        let message = 'Request could not complete.';
        try {
            const data = await response.json();
            message = data.error || message;
        }
        catch {
            if (response.status === 401)
                message = 'Open HiringPlug Migration.url from your workspace to reconnect this browser.';
        }
        throw new Error(message);
    }
    return response;
}
function node(tag, text) { const n = document.createElement(tag); n.textContent = text; return n; }
function render() {
    $('demo').hidden = !state.demo;
    $('unlock-section').hidden = state.unlocked;
    $('workspace').hidden = !state.unlocked;
    $('account').textContent = state.connected ? state.account : 'No account connected.';
    $('callback').textContent = state.callback || '';
    $('connection-state').textContent = state.connected ? 'Connected' : 'Not connected';
    $('connection-controls').hidden = !!state.demo;
    const blocked = !healthy || !!coolingDown();
    $('start').disabled = blocked || !state.connected || state.running || state.connecting || busy;
    $('resume').disabled = blocked || !state.connected || state.running || state.connecting || busy || !state.report?.run;
    $('pause').hidden = !state.running;
    $('run-state').textContent = state.running ? 'Running' : (state.report?.status || 'Not started').replace('_', ' ');
    $('current').textContent = coolingDown() ? `Waiting until ${new Date(state.retryAt).toLocaleString()}. Completed checkpoints are saved.` : (state.current ? `Checking ${state.current}…` : state.message || '');
    const rows = state.report?.results || [];
    $('progress').max = state.resources || 1;
    $('progress').value = rows.length;
    $('report').disabled = downloading || !healthy || !state.report?.run;
    $('originals').disabled = downloading || !healthy || !state.report?.run || state.running;
    for (const id of ['oauth', 'token']) $(id).querySelector('button').disabled = blocked || !!state.running || !!state.connecting || busy;
    $('unlock').querySelector('button').disabled = !healthy || busy;
    $('results').replaceChildren(...rows.map(r => { const tr = document.createElement('tr'); tr.append(node('td', r.label), node('td', r.count == null ? 'Unknown' : r.count.toLocaleString())); const cell = document.createElement('td'), badge = node('span', labels[r.status] || r.status); if (r.count == null)
        badge.className = 'warning'; if(r.evidenceCheckedAt) badge.title = `Saved response from ${new Date(r.evidenceCheckedAt).toLocaleString()}`; cell.append(badge); tr.append(cell, node('td', r.http ? String(r.http) : '—')); return tr; }));
    if (!rows.length) {
        const tr = document.createElement('tr'), td = node('td', state.running ? 'Checking resources…' : 'Start an inventory to check account coverage.');
        td.colSpan = 4;
        tr.append(td);
        $('results').append(tr);
    }
    $('coverage').replaceChildren(...(state.report?.coverage || []).map(t => node('li', t)));
}
async function refresh() {
    try {
        const next = await (await request('/api/status')).json();
        state = next;
        healthy = true;
        statusError = '';
    } catch (e) {
        healthy = false;
        statusError = `${e.message} Last displayed results may be stale.`;
    }
    renderError();
    render();
}
async function action(f) { if (busy)
    return; busy = true; error(''); render(); try {
    await f();
}
catch (e) {
    error(e.message);
}
finally {
    busy = false;
    await refresh();
} }
$('unlock').addEventListener('submit', e => { e.preventDefault(); action(async () => { await request('/api/unlock', { passphrase: $('passphrase').value }); $('passphrase').value = ''; }); });
$('oauth').addEventListener('submit', e => { e.preventDefault(); action(async () => { const body = { clientId: $('client-id').value, clientSecret: $('client-secret').value }; $('client-secret').value = ''; const result = await (await request('/api/oauth', body)).json(); location.assign(result.url); }); });
$('token').addEventListener('submit', e => { e.preventDefault(); action(async () => { const body = { api: $('api').value, token: $('access-token').value }; $('access-token').value = ''; await request('/api/connect', body); }); });
$('start').addEventListener('click', () => action(() => request('/api/start', { resume: false })));
$('resume').addEventListener('click', () => action(() => request('/api/start', { resume: true })));
$('pause').addEventListener('click', () => action(() => request('/api/pause', {})));
async function download(path, name) {
    if(downloading) return;
    downloading = true;
    error('');
    render();
    try {
        const response = await request(path);
        const blob = await response.blob();
        const url = URL.createObjectURL(blob), a = document.createElement('a');
        a.href = url; a.download = name; a.click();
        setTimeout(() => URL.revokeObjectURL(url), 30000);
    } catch(e) {
        error(e.message);
    } finally {
        downloading = false;
        await refresh();
    }
}
$('report').addEventListener('click', () => download('/api/report', 'jobadder-discovery-report.json'));
$('originals').addEventListener('click', () => download('/api/originals.zip', 'jobadder-original-responses.zip'));
refresh();
setInterval(refresh, 1500);
