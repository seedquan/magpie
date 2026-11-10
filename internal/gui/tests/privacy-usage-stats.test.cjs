// Run with Node's test runner and Playwright on the module path; see README.md.
// This fork offers no way to enable third-party usage telemetry.
// Legacy opt-in preferences must not restore its controls in any language.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { watchSettingsReads, settingsDrawn } = require("./reader.cjs");

const assets = path.resolve(__dirname, "../assets");

function serve(lang, settings, saved) {
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings });
    if (url.pathname === "/api/settings") {
      if (req.method() === "POST") { const b = req.postDataJSON(); saved.push(b); Object.assign(settings, b); }
      return json(settings);
    }
    if (url.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], gateway: { running: true, window: true } });
    if (url.pathname === "/api/groups") return json({ groups: [], models: [] });
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
    for (const width of [900, 390]) {
      test(`${engine} ${lang} ${width}: Privacy cannot enable third-party telemetry`, async (t) => {
        const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
        t.after(() => browser.close());
        const errors = [], saved = [];
        const settings = { requestArchive:true, requestArchiveLocal:true, otel:{enabled:true,sessions:true}, theme: "light", lang, tray: "panel", currency: "usd", noStats: false, noUsageStats: false };
        const context = await browser.newContext({ viewport: { width, height: 760 }, reducedMotion: "reduce" });
        const page = await context.newPage();
        page.setDefaultTimeout(5000);
        page.on("pageerror", (e) => errors.push(e.message));
        await page.route("**/*", serve(lang, settings, saved));
        await watchSettingsReads(page);
        await page.goto("http://magpie.test/?view=settings&tab=privacy");
        await settingsDrawn(page);
        const names = await page.locator("#setPage-privacy .row .name").allTextContents();
        const words = await page.evaluate(() => ["Count me as a user", "Share the agents, providers and models I use", "Mask personal data"].map((s) => t(s)));
        assert.ok(!names.includes(words[0]), "daily telemetry can be enabled");
        assert.ok(!names.includes(words[1]), "usage telemetry can be enabled");
        assert.ok(names.includes(words[2]), "privacy settings did not render");
        assert.equal(await page.locator('#setTabs [data-set="sync"], #setTabs [data-set="otel"]').count(), 0);
        assert.equal(await page.locator('#syncList, #otelList, #archiveList').count(), 0);
        for (const tab of ["sync", "otel"]) {
          await page.goto(`http://magpie.test/?view=settings&tab=${tab}`);
          await settingsDrawn(page);
          assert.equal(await page.locator('#setPage-privacy:visible').count(), 1, "removed deep link did not retain the last valid section");
        }
        assert.deepEqual(errors, []);
        await context.close();
      });
    }
  }
}
