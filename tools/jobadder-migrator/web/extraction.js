'use strict';
let downloadPlanKey = '';
let extraction = {}, extractionHealthy = false, uploadBusy = false;
const readable = text => (text || '').replaceAll('_', ' ');
const formatBytes = bytes => bytes < 1024 ? `${bytes} B` : bytes < 1048576 ? `${(bytes/1024).toFixed(1)} KB` : bytes >= 1073741824 ? `${(bytes / 1073741824).toFixed(1)} GB` : `${(bytes / 1048576).toFixed(1)} MB`;
function renderExtraction() {
    const summary = extraction.summary || {};
    const blocked = !healthy || !extractionHealthy || state.running || state.connecting || state.transferring || busy || uploadBusy || coolingDown();
    $('extract-pause').hidden = !state.running || state.mode !== 'extraction';
    $('extract-start').disabled = blocked || !state.connected;
    $('extract-resume').disabled = blocked || !state.connected || !summary.id;
    $('source-submit').disabled = blocked;
    $('prepare-downloads').disabled = blocked;
    $('export-folder').hidden = !window.showDirectoryPicker;
    $('export-folder').disabled = blocked;
    $('download-manifest').disabled = blocked;
    $('download-part').disabled = blocked;
    $('extraction-status').textContent = readable(summary.status === 'running' && !state.running ? 'paused' : summary.status) || 'Not started';
    const metrics = [['Source IDs indexed', summary.records], ['Native files saved', summary.files], ['Requests saved', summary.collected], ['Known pending', summary.pending], ['Unresolved requests', summary.gaps]];
    $('extraction-metrics').replaceChildren(...metrics.map(([label, value]) => {const card = node('div', ''); card.append(node('strong', (value || 0).toLocaleString()), node('span', label));return card;}));
    $('extraction-current').textContent = (state.mode === 'extraction' ? (state.running ? `${state.current}… ` : `${state.message || ''} `) : '') + (summary.id ? `${formatBytes(summary.bytes || 0)} saved · ${summary.tasks?.toLocaleString() || 0} known requests · completeness unverified${extraction.unlinkedObjects ? ` · ${extraction.unlinkedObjects} unlinked objects need review` : ''}` : 'Extract full source records, notes, linked data and retained native files.');
    $('scope-summary').textContent = '· completeness unverified';
    $('scope-results').replaceChildren(...(extraction.scope || []).map(area => {const tr = document.createElement('tr');tr.title = area.required;tr.append(node('td', area.area), node('td', area.collected.toLocaleString()), node('td', area.gaps.toLocaleString()), node('td', readable(area.status)));return tr;}));
    $('route-results').replaceChildren(...(extraction.routes || []).map(route => {const tr = document.createElement('tr');tr.append(node('td', route.path), node('td', `${route.collected} / ${route.requests}`), node('td', String(route.gaps)), node('td', route.equivalent ? `Bulk source: ${route.equivalent}` : route.policy));return tr;}));
    $('extraction-limits').replaceChildren(...(extraction.limitations || []).map(text => node('li', text)));
    $('source-results').replaceChildren(...(extraction.imports || []).map(source => {const row = node('div', '');row.className = 'source-result';row.append(node('strong', source.name), node('span', `${readable(source.status)} · ${source.records.toLocaleString()} indexed rows · ${formatBytes(source.blob.bytes)}`), node('small', source.message));return row;}));
}
async function refreshExtraction() {
    if (!state.unlocked) return;
    try {extraction = await (await request('/api/extraction')).json();extractionHealthy = true;}
    catch (e) {extractionHealthy = false;error(`${e.message} Extraction results may be stale.`);}
    renderExtraction();
}
$('extract-start').addEventListener('click', () => action(async () => {$('extraction-downloads').hidden = true;await request('/api/extract', {resume:false});await refreshExtraction();}));
$('extract-resume').addEventListener('click', () => action(async () => {$('extraction-downloads').hidden = true;await request('/api/extract', {resume:true});await refreshExtraction();}));
$('source-upload').addEventListener('submit', e => {
    e.preventDefault();
    if (uploadBusy) return;
    const file = $('source-file').files[0]; if (!file) return;
    const query = new URLSearchParams({name:file.name, entity:$('source-entity').value, idField:$('source-id-field').value, source:$('source-kind').value});
    uploadBusy = true; renderExtraction(); error('');
    const xhr = new XMLHttpRequest(); xhr.open('POST', `/api/sources?${query}`);xhr.setRequestHeader('X-Migration-Session',session);xhr.setRequestHeader('Content-Type','application/octet-stream');
    xhr.upload.onprogress = event => {$('upload-progress').textContent = event.lengthComputable ? `${Math.round(event.loaded/event.total*100)}% uploaded · preserving and indexing…` : 'Preserving and indexing…';};
    xhr.onload = async () => {uploadBusy = false;if (xhr.status !== 201) {try {error(JSON.parse(xhr.responseText).error || 'Intake failed.');} catch {error('Intake failed.');}} else {$('source-file').value = '';$('extraction-downloads').hidden = true;}$('upload-progress').textContent = '';await refresh();await refreshExtraction();};
    xhr.onerror = () => {uploadBusy = false;$('upload-progress').textContent = '';error('Upload interrupted. Check the intake list before retrying.');renderExtraction();};
    xhr.send(file);
});
$('prepare-downloads').addEventListener('click', () => action(async () => {
    const plan = await (await request('/api/downloads')).json();downloadPlanKey = plan.plan;
    $('object-part').replaceChildren(...plan.parts.map((part,index) => {const option = node('option', `Part ${index+1} · ${part.files} objects · ${formatBytes(part.bytes)}`);option.value=String(index);return option;}));
    $('download-part').hidden = !plan.parts.length;$('object-part').hidden = !plan.parts.length;$('extraction-downloads').hidden = false;
}));
async function nativeDownload(manifest) {
    await action(async () => {const ticket = await (await request('/api/download-ticket', {manifest,part:Number($('object-part').value || 0),plan:downloadPlanKey})).json();const anchor = document.createElement('a');anchor.href=ticket.url;anchor.download='';anchor.click();});
}
$('download-manifest').addEventListener('click', () => nativeDownload(true));
$('download-part').addEventListener('click', () => nativeDownload(false));
setInterval(refreshExtraction, 8000);
setTimeout(refreshExtraction, 1000);

$('extract-pause').addEventListener('click', () => action(async () => {await request('/api/pause',{});await refreshExtraction();}));

$('export-folder').addEventListener('click', () => action(async () => {
    const parent = await window.showDirectoryPicker({mode:'readwrite'});
    const folder = await parent.getDirectoryHandle(`JobAdder-export-${new Date().toISOString().replaceAll(':','-')}`,{create:true});
    const plan = await (await request('/api/downloads')).json();
    try {for (let index=-1; index<plan.parts.length; index++) {
        $('folder-progress').textContent = index<0 ? 'Saving manifest…' : `Saving object part ${index+1} of ${plan.parts.length}…`;
        const ticket = await (await request('/api/download-ticket',{manifest:index<0,part:Math.max(index,0),plan:plan.plan})).json();
        const response = await fetch(ticket.url,{cache:'no-store'});
        if (!response.ok) throw new Error('Export stopped. A part could not be verified or downloaded; the folder is incomplete.');
        const name = index<0 ? 'jobadder-extraction-manifest.zip' : `jobadder-objects-${plan.parts[index].first}-${plan.parts[index].last}.zip`;
        const file = await folder.getFileHandle(name,{create:true});const output = await file.createWritable();
        try {await response.body.pipeTo(output);} catch(e) {try {await output.abort();} catch {} throw e;}
    }
    } catch(e) {$('folder-progress').textContent = 'Export incomplete. Keep partial files separate and retry a new export.';throw e;}
    $('folder-progress').textContent = 'Export saved. Keep this folder private; completeness still requires source reconciliation.';
}));
