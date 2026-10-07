// Smoke test of the web dashboard in a real browser engine (Chromium), used
// for the "Tested" browser row in DEPLOYMENT.md (see RELEASING.md, step 5).
//
// It opens the task list, a task's detail page and the service info page,
// checks that each shows the expected content, and fails on browser console
// errors, page errors or failed requests. Screenshots go to <out-dir>.
//
// Setup (once, outside the repository):
//   npm i playwright && npx playwright install chromium
// Usage:
//   node scripts/dashboard-smoke-test.mjs <server-url> <task-id> <out-dir>
// e.g. after submitting the hello-world task from "Testing the server":
//   node scripts/dashboard-smoke-test.mjs http://localhost:8000 <task-id> build/dashboard-test
//
// The Nodes page is not checked: it has a known request loop (DEPLOYMENT.md).

import { mkdirSync } from "node:fs";
import { chromium } from "playwright";

const [base, taskId, outDir] = process.argv.slice(2);
if (!base || !taskId || !outDir) {
  console.error("usage: node scripts/dashboard-smoke-test.mjs <server-url> <task-id> <out-dir>");
  process.exit(2);
}
mkdirSync(outDir, { recursive: true });

const browser = await chromium.launch();
console.log(`Browser: ${browser.browserType().name()} ${browser.version()}`);
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
const problems = [];
page.on("console", (m) => { if (m.type() === "error") problems.push(`console: ${m.text()}`); });
page.on("pageerror", (e) => problems.push(`page error: ${e.message}`));
page.on("requestfailed", (r) => problems.push(`request failed: ${r.url()}`));
page.on("response", (r) => { if (r.status() >= 400) problems.push(`HTTP ${r.status()}: ${r.url()}`); });

const checks = [
  ["task-list", "/tasks", taskId],
  ["task-detail", `/tasks/${taskId}`, "COMPLETE"],
  ["service-info", "/service-info", "Funnel"],
];
let failed = 0;
for (const [name, path, expected] of checks) {
  await page.goto(base + path, { waitUntil: "networkidle" });
  let ok = true;
  try {
    await page.getByText(expected).first().waitFor({ timeout: 10000 });
  } catch {
    ok = false;
    failed++;
  }
  await page.screenshot({ path: `${outDir}/${name}.png`, fullPage: true });
  console.log(`${ok ? "OK  " : "FAIL"} ${path} shows "${expected}"`);
}
await browser.close();

if (problems.length) console.log(`Problems:\n  ${problems.join("\n  ")}`);
else console.log("No console errors, page errors or failed requests.");
process.exit(failed || problems.length ? 1 : 0);
