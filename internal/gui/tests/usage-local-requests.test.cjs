// Local Requests remain readable after archive and CSV controls are removed.
// Every API and asset is served in-process; no user configuration is accessed.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { inView } = require("./reader.cjs");

const assets = process.env.MAGPIE_ASSETS || path.resolve(__dirname, "../assets");
const row = { t: new Date().toISOString(), agent: "codex", agentName: "Codex", icon: "codex-color", provider: "relay", providerName: "Relay", req: "local-request-model", model: "local-request-model", in: 100, out: 10, ms: 900, status: 200, cost: 0.01, priced: true };

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(`${engine}: local Requests load, fit and filter without export controls`, async (t) => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    t.after(() => browser.close());
    for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
      await t.test(lang, async () => {
        const context = await browser.newContext({ viewport: { width: 1180, height: 760 }, reducedMotion: "reduce" });
        const page = await context.newPage();
        const errors = [], writes = [], requests = [];
        let release;
        const held = new Promise((resolve) => { release = resolve; });
        t.after(() => { release(); return context.close(); });
        page.setDefaultTimeout(5000);
        page.on("pageerror", (e) => errors.push(e.message));
        await page.route("**/*", async (route) => {
          const req = route.request(), url = new URL(req.url());
          const json = (data) => route.fulfill({ json: data });
          if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:${JSON.stringify(lang)},theme:"light",web:false};` });
          if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
          if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light", requestArchive: true, requestArchiveLocal: true }, fx: { rate: 7.2 } });
          if (url.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], models: [], gateway: { running: false } });
          if (url.pathname === "/api/usage/requests") {
            requests.push(url.searchParams.get("q") || "");
            await held;
            const rows = url.searchParams.get("q") ? [] : [row];
            return json({ period: url.searchParams.get("period"), rows, offset: 0, total: rows.length, calls: rows.length, input: rows.length * 100, output: rows.length * 10, cost: rows.length * 0.01, agents: [], providers: [], series: [] });
          }
          if (url.pathname === "/api/usage/heatmap") return json({ from: "", to: "", days: [] });
          if (url.pathname === "/api/usage/quotas") return json([]);
          if (url.pathname === "/api/usage") return json({ calls: 1, input: 100, output: 10, cost: 0.01, series: [], agents: [], models: [] });
          if (url.pathname === "/api/gateway/trace") return json({ mine: false, seq: 0, routes: [], totals: { requests: 0, rerouted: 0, errors: 0 }, now: new Date().toISOString() });
          if (url.pathname === "/api/groups") return json({ groups: [], models: [] });
          if (url.pathname.startsWith("/api/")) {
            if (req.method() !== "GET") writes.push(url.pathname);
            return json({});
          }
          const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
          const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
          await route.fulfill({ body: await fs.readFile(file), contentType });
        });
        await page.goto("http://magpie.test/");
        await page.locator('[data-view="usage"]').first().click();
        await page.locator(":is(#usageTab .opt, .nav-s):visible").nth(1).click();
        await page.locator("#ledWrap .led-sk").first().waitFor();
        assert.equal(await page.locator("#ledSum").innerText(), "", "loading keeps an empty local summary");
        release();
        await page.locator("#ledWrap .led tbody tr").first().waitFor();
        const summary = await page.evaluate(() => t("{n} request", { n: "1" }));
        assert.ok((await page.locator("#ledSum").innerText()).includes(summary), "the request count is visible");
        assert.ok((await page.locator("#ledWrap").innerText()).includes(row.model), "local request rows render");
        await page.setViewportSize({ width: 560, height: 760 });
        await page.locator("#ledTight").waitFor({ state: "attached" });
        assert.equal(await page.locator("#ledTight").evaluate((e) => e.parentElement.className), "led-head", "the responsive hint has its summary container");
        await inView(page, page.locator("#ledQ"));
        await page.locator("#ledQ").fill("missing-local-model");
        await page.locator("#ledWrap .led-none").waitFor();
        assert.ok(requests.includes("missing-local-model"), "search reads filtered local requests");
        const empty = await page.evaluate(() => t("No requests match these filters."));
        assert.equal(await page.locator("#ledWrap .led-none").innerText(), empty);
        assert.equal(await page.locator("#ledExport, #ledArchive, #ledArchiveFetch").count(), 0, "removed export and archive controls stay absent");
        assert.deepEqual(writes, [], "viewing requests sends no mutation");
        assert.deepEqual(errors, [], "loading, rendering and responsive fitting throw no errors");
        await context.close();
      });
    }
  });
}
