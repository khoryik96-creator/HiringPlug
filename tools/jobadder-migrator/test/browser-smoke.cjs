// Synthetic-only UI check. No JobAdder or HiringPlug credentials are used.
const { chromium, expect } = require('../../../web/node_modules/@playwright/test');
const { spawn } = require('node:child_process');
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const net = require('node:net');
const binary = path.resolve(process.argv[2] || 'dist/HiringPlugMigration');
const screenshots = process.argv[3] && path.resolve(process.argv[3]);
async function port() { return new Promise(resolve => { const s = net.createServer(); s.listen(0, '127.0.0.1', () => { const p = s.address().port; s.close(() => resolve(p)); }); }); }
async function start(demo) {
    const dir = await fs.mkdtemp(path.join(os.tmpdir(), 'hp-discovery-smoke-'));
    const p = await port();
    const proc = spawn(binary, ['--workspace', dir, '--port', String(p), '--no-browser', ...(demo ? ['--demo'] : [])], { stdio: 'ignore' });
    let startupError;
    proc.on('error', () => { startupError = true; });
    const exited = new Promise(resolve => proc.once('close', resolve));
    async function close() {
        if (proc.exitCode === null && !startupError) {
            proc.kill('SIGINT');
            await Promise.race([exited, new Promise(resolve => setTimeout(resolve, 3000))]);
            if (proc.exitCode === null) {
                proc.kill('SIGKILL');
                await exited;
            }
        }
        await fs.rm(dir, { recursive: true, force: true });
    }
    const file = path.join(dir, ...(demo ? ['demo'] : []), 'HiringPlug Migration.url');
    for (let i = 0; i < 100 && !startupError && proc.exitCode === null; i++) {
        try {
            const url = (await fs.readFile(file, 'utf8')).split('URL=')[1].trim();
            return { url, close };
        } catch {
            await new Promise(resolve => setTimeout(resolve, 100));
        }
    }
    await close();
    throw Error('Local discovery tool did not start');
}
(async () => {
    let demo, real, browser;
    try {
        if (screenshots)
            await fs.mkdir(screenshots, { recursive: true });
        demo = await start(true);
        real = await start(false);
        browser = await chromium.launch({ ...(process.env.HP_E2E_CHROMIUM ? { executablePath: process.env.HP_E2E_CHROMIUM } : {}), args: ['--no-sandbox'] });
        const page = await browser.newPage({ viewport: { width: 1280, height: 1000 } }), errors = [];
        page.on('pageerror', e => errors.push(e.message));
        await page.goto(demo.url);
        await expect(page.getByText('DEMO · Synthetic sample counts. No JobAdder account is connected.', { exact: true })).toBeVisible();
        await page.getByRole('button', { name: 'Start new inventory', exact: true }).click();
        await expect(page.getByRole('button', { name: 'Pause', exact: true })).toBeVisible();
        await page.getByRole('button', { name: 'Pause', exact: true }).click();
        await expect(page.getByText('Paused. Completed checkpoints are saved.', { exact: true })).toBeVisible();
        await page.getByRole('button', { name: 'Resume / retry unresolved', exact: true }).click();
        await expect(page.getByText('Inventory finished. Review all coverage gaps before planning a full export.', { exact: true })).toBeVisible({ timeout: 30000 });
        const candidates = page.getByRole('row').filter({ has: page.getByRole('cell', { name: 'Candidates', exact: true }) });
        await expect(candidates).toContainText('80,000');
        const denied = page.getByRole('row').filter({ has: page.getByRole('cell', { name: 'Candidate payroll/billing', exact: true }) });
        await expect(denied).toContainText('Unknown');
        await expect(denied).toContainText('Permission denied');
        const download = page.waitForEvent('download');
        await page.getByRole('button', { name: 'Download count report', exact: true }).click();
        const reportFile = await (await download).path();
        const report = JSON.parse(await fs.readFile(reportFile, 'utf8'));
        if (!report.demo || report.report.status !== 'finished')
            throw Error('Demo report context missing');
        const zipDownload = page.waitForEvent('download');
        await page.getByRole('button', { name: 'Download original responses', exact: true }).click();
        if ((await zipDownload).suggestedFilename() !== 'jobadder-original-responses.zip')
            throw Error('Original response export unavailable');
        if (screenshots)
            await page.screenshot({ path: path.join(screenshots, 'discovery-demo.png'), fullPage: true });
        await page.setViewportSize({ width: 390, height: 900 });
        if (await page.evaluate(() => document.documentElement.scrollWidth > innerWidth))
            throw Error('Mobile layout overflows');
        if (screenshots)
            await page.screenshot({ path: path.join(screenshots, 'discovery-mobile.png'), fullPage: true });
        // A fresh run can briefly have no checkpoints. Do not leave the old
        // run's candidate counts visible while that empty status is displayed.
        await page.route('**/api/status', async route => {
            const original = await route.fetch();
            const data = await original.json();
            data.report.results = [];
            await route.fulfill({ response: original, json: data });
        });
        await expect(page.getByText('Start an inventory to check account coverage.', { exact: true })).toBeVisible();
        await expect(candidates).toHaveCount(0);
        await page.unroute('**/api/status');
        await page.setViewportSize({ width: 1280, height: 1000 });
        await page.goto(real.url);
        await expect(page.getByRole('heading', { name: 'Open your private workspace', exact: true })).toBeVisible();
        if (screenshots)
            await page.screenshot({ path: path.join(screenshots, 'workspace-unlock.png'), fullPage: true });
        await page.getByLabel('Workspace passphrase', { exact: true }).fill('Synthetic browser workspace only');
        await page.getByRole('button', { name: 'Unlock workspace', exact: true }).click();
        await expect(page.getByRole('heading', { name: '1 · Connect JobAdder', exact: true })).toBeVisible();
        await expect(page.getByRole('button', { name: 'Start new inventory', exact: true })).toBeDisabled();
        if (screenshots)
            await page.screenshot({ path: path.join(screenshots, 'jobadder-connect.png'), fullPage: true });
        if (errors.length)
            throw Error('UI runtime error');
        console.log('UI passed: private workspace, demo inventory, pause/resume, denied-access reporting, downloads and mobile layout.');
    }
    finally {
        await browser?.close();
        await demo?.close();
        await real?.close();
    }
})().catch(e => { console.error(String(e.message).replace(/#[a-f0-9]{64}/g, '#[redacted]')); process.exit(1); });
