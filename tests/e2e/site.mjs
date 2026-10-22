// End-to-end test of the whole site, against Ken's running dev server.
// Headless only. Run from the project folder:  node tests/e2e/site.mjs
//
// 1. Logged out: visits every link reachable from the landing page.
// 2. Signs up, creates a line, builds a menu, then visits every link again.
// 3. A real call: phone app online, a visitor calls, the phone answers, hang up.
// 4. A stranger can't open the phone app; log out works.
// It deletes its test account at the end. Screenshots: tests/e2e/shots/.
//
// BASE=https://voip.dresdell.com node tests/e2e/site.mjs  tests through Caddy.

import { createRequire } from "node:module";
import { DatabaseSync } from "node:sqlite"; // built into Node 22.5+
import { existsSync, mkdirSync, readdirSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

const BASE = process.env.BASE || "http://localhost:7331";
const SHOTS = "tests/e2e/shots";
const DB = process.env.MINIVOIP_DB || "data/minivoip.db";
mkdirSync(SHOTS, { recursive: true });

// playwright-core from the npx cache, and the cached headless Chromium.
function findPlaywright() {
  if (process.env.PWC) return process.env.PWC;
  const npx = join(homedir(), ".npm/_npx");
  for (const d of readdirSync(npx)) {
    const p = join(npx, d, "node_modules/playwright-core");
    if (existsSync(p)) return p;
  }
  throw new Error("playwright-core not found; set PWC=/path/to/playwright-core");
}
function findChromium() {
  const cache = join(homedir(), ".cache/ms-playwright");
  const dir = readdirSync(cache).filter((d) => d.startsWith("chromium_headless_shell-")).sort().pop();
  return join(cache, dir, "chrome-headless-shell-linux64/chrome-headless-shell");
}
const { chromium } = createRequire(import.meta.url)(findPlaywright());

const stamp = Date.now().toString(36);
const EMAIL = `e2e-${stamp}@example.com`;
const LINE = `e2e-${stamp}`;
const failures = [];
const consoleErrors = [];
let step = "";

function check(ok, what) {
  console.log(`${ok ? "ok  " : "FAIL"} ${what}`);
  if (!ok) failures.push(what);
}

const browser = await chromium.launch({
  executablePath: findChromium(),
  args: ["--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream", "--autoplay-policy=no-user-gesture-required"],
});

async function newPage(width = 1280, height = 900) {
  const ctx = await browser.newContext({ viewport: { width, height }, permissions: ["microphone"] });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => consoleErrors.push(`${step}: ${e}`));
  page.on("console", (m) => {
    // The dev proxy's live-reload stream drops when the app restarts; not ours.
    // A form sent back with an error answers 422 on purpose.
    if (m.type() === "error" && !/_templ\/reload|status of 422/.test(m.text())) consoleErrors.push(`${step}: ${m.text()}`);
  });
  return page;
}

// crawl visits every same-site link reachable from start (not /logout, not /call
// pages of other lines) and checks each answers 200 and anchors exist.
async function crawl(page, start, label) {
  const seen = new Set();
  const queue = [start];
  while (queue.length) {
    const path = queue.shift();
    if (seen.has(path)) continue;
    seen.add(path);
    step = `${label} ${path}`;
    const res = await page.goto(BASE + path, { waitUntil: "domcontentloaded" });
    check(res && res.status() === 200, `${label}: ${path} loads (${res && res.status()})`);
    const links = await page.$$eval("a[href]", (as) => as.map((a) => a.getAttribute("href")));
    for (const href of links) {
      if (href.startsWith("#")) {
        const id = href.slice(1);
        if (id) check(await page.$(`[id="${id}"]`) !== null, `${label}: ${path} anchor ${href} exists`);
        continue;
      }
      if (href.startsWith("mailto:") || /^https?:/.test(href)) continue;
      const clean = href.split("#")[0];
      if (clean && !seen.has(clean)) queue.push(clean);
    }
  }
  return seen;
}

try {
  // 1. Logged out ------------------------------------------------------
  const visitorSite = await newPage();
  const pagesOut = await crawl(visitorSite, "/", "logged out");
  check(pagesOut.has("/signup") && pagesOut.has("/login"), "landing page links to sign up and log in");
  step = "landing";
  await visitorSite.goto(BASE + "/");
  await visitorSite.screenshot({ path: `${SHOTS}/landing.png` });
  await visitorSite.goto(BASE + "/dashboard");
  check(new URL(visitorSite.url()).pathname === "/login", "dashboard sends logged-out visitors to log in");

  // 2. Sign up, line, menu ------------------------------------------------
  const owner = await newPage();
  step = "sign up";
  await owner.goto(BASE + "/");
  await owner.click('nav a[href="/signup"]');
  await owner.fill("#email", EMAIL);
  await owner.fill("#password", "e2e-password");
  await owner.click("main button[type=submit]");
  await owner.waitForSelector("#name");
  check(true, "sign up leads to 'Create your line'");

  step = "create line";
  await owner.fill("#name", LINE);
  await owner.fill("#business", "E2E Shop");
  await owner.fill("#owner", "Ken");
  await owner.click("main button[type=submit]");
  await owner.waitForSelector("[data-add-choice]");
  check(true, "creating the line opens the menu editor");

  step = "menu editor";
  const rows = owner.locator("[data-choice]");
  await owner.click("[data-add-choice]");
  await rows.nth(1).locator("[name=label]").fill("Opening hours");
  await rows.nth(1).locator("[data-action-select]").selectOption("play");
  await rows.nth(1).locator("[name=say]").fill("We're open 9 to 5.");
  await owner.click("[data-add-choice]");
  await rows.nth(2).locator("[name=label]").fill("Our website");
  await rows.nth(2).locator("[data-action-select]").selectOption("link");
  await rows.nth(2).locator("[name=url]").fill("https://example.com/");
  await owner.click("[data-preview]");
  const heard = await owner.textContent("[data-preview-text]");
  check(/press 1 to talk to Ken/i.test(heard) && /press 2 for opening hours/i.test(heard), `"Hear it" reads the menu: ${heard.trim()}`);
  await owner.click("main button[type=submit]");
  await owner.waitForSelector("text=Saved.");
  check(true, "menu saves");
  await owner.screenshot({ path: `${SHOTS}/menu.png`, fullPage: true });

  step = "menu errors";
  await rows.nth(2).locator("[name=url]").fill("");
  await owner.click("main button[type=submit]");
  check(await owner.isVisible("text=needs a web address"), "menu editor explains a missing web address");

  const pagesIn = await crawl(owner, "/", "logged in");
  for (const p of ["/dashboard", "/dashboard/menu", "/app", `/call/${LINE}`]) check(pagesIn.has(p), `logged in: ${p} is reachable by links`);
  step = "dashboard";
  await owner.goto(BASE + "/dashboard");
  await owner.screenshot({ path: `${SHOTS}/dashboard.png`, fullPage: true });
  const phoneSize = await newPage(390, 844);
  await phoneSize.context().addCookies(await owner.context().cookies());
  for (const p of ["/", "/dashboard", "/dashboard/menu", "/app", `/call/${LINE}`]) {
    step = `phone width ${p}`;
    await phoneSize.goto(BASE + p);
    const wide = await phoneSize.evaluate(() => document.documentElement.scrollWidth);
    check(wide <= 390, `phone width: ${p} doesn't scroll sideways (${wide}px)`);
  }
  await phoneSize.goto(BASE + "/dashboard/menu");
  await phoneSize.screenshot({ path: `${SHOTS}/menu-phone.png`, fullPage: true });

  // 3. A real call --------------------------------------------------------
  step = "call";
  await owner.goto(BASE + "/app");
  await owner.waitForSelector("text=Online", { timeout: 10000 });
  const caller = await newPage(390, 760);
  await caller.goto(BASE + `/call/${LINE}`);
  await caller.click('[data-action="call"]');
  await caller.waitForSelector('[data-key="3"]:visible');
  await caller.click('[data-key="1"]');
  await owner.waitForSelector('[data-action="accept"]:visible', { timeout: 15000 });
  check(true, "the phone app rings");
  await owner.click('[data-action="accept"]');
  await caller.waitForSelector("text=On the line with Ken", { timeout: 15000 });
  check(true, "the call connects");
  await caller.screenshot({ path: `${SHOTS}/call.png` });
  await caller.click('[data-action="hangup"]');
  await owner.waitForSelector("text=Ready for calls", { timeout: 10000 });
  check(true, "hanging up reaches the phone");

  step = "play option";
  await caller.click('[data-action="call"]');
  await caller.waitForSelector('[data-key="2"]:visible');
  await caller.click('[data-key="2"]');
  await caller.waitForSelector('[data-action="call"]:visible', { timeout: 15000 });
  check(true, "a 'play a message' choice ends the call by itself");

  // 4. Strangers and log out ----------------------------------------------
  step = "stranger";
  const stranger = await newPage();
  await stranger.goto(BASE + "/app");
  check(new URL(stranger.url()).pathname === "/login", "a stranger can't open the phone app");
  step = "log out";
  await owner.goto(BASE + "/dashboard");
  await owner.click("header button[type=submit]");
  await owner.waitForSelector('nav a[href="/login"]');
  check(true, "log out returns to the landing page, logged out");
} catch (err) {
  failures.push(`${step}: ${err.message.split("\n")[0]}`);
  console.log(`FAIL ${step}: ${err.message.split("\n")[0]}`);
} finally {
  await browser.close();
  // Delete the test account (cascades to its line, menu and sessions).
  const db = new DatabaseSync(DB);
  db.exec("PRAGMA foreign_keys = ON");
  db.prepare("DELETE FROM account WHERE name LIKE 'e2e-%@example.com'").run();
  db.close();
}

for (const e of consoleErrors) failures.push(`browser error: ${e}`);
console.log(failures.length ? `\n${failures.length} problem(s):\n- ${failures.join("\n- ")}` : "\nAll good.");
process.exit(failures.length ? 1 : 0);
