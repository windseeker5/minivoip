// End-to-end test of the whole site, against Ken's running dev server.
// Headless only. Run from the project folder:  node tests/e2e/site.mjs
//
// 1. Logged out: visits every link reachable from the landing page.
// 2. Signs up, creates a line, builds a menu, records prompts (a caller hears
//    them), then visits every link again.
// 3. A real call: phone app online, a visitor calls, the phone answers, hang up;
//    then voicemail (unanswered call and a voicemail choice), listen, delete.
// 4. A stranger can't open the phone app; log out works.
// It deletes its test account at the end. Screenshots: tests/e2e/shots/.
//
// BASE=https://voip.dresdell.com node tests/e2e/site.mjs  tests through Caddy.

import { createRequire } from "node:module";
import { DatabaseSync } from "node:sqlite"; // built into Node 22.5+
import { existsSync, mkdirSync, readdirSync, rmSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join } from "node:path";

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
  await owner.click("[data-add-choice]");
  await rows.nth(3).locator("[name=label]").fill("Leave a message");
  await rows.nth(3).locator("[data-action-select]").selectOption("voicemail");
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

  // Recordings ------------------------------------------------------------
  step = "record greeting";
  await rows.nth(2).locator("[name=url]").fill("https://example.com/");
  const greetingRec = owner.locator('[data-recorder]:has([name="greeting_audio"])');
  async function record(box, ms) {
    await box.locator("[data-rec-start]").click();
    await owner.waitForTimeout(ms);
    await box.locator("[data-rec-stop]").click();
    await box.locator("text=Recorded.").waitFor({ timeout: 10000 });
  }
  await record(greetingRec, 2000);
  await record(rows.nth(1).locator("[data-recorder]"), 1500);
  await owner.click("main button[type=submit]");
  await owner.waitForSelector("text=Saved.");
  const greetingFile = await owner.inputValue('[name="greeting_audio"]');
  const hoursFile = await rows.nth(1).locator('[name="say_audio"]').inputValue();
  check(/^prompt-/.test(greetingFile) && /^prompt-/.test(hoursFile), "recorded greeting and message are saved with the menu");
  const promptRes = await owner.request.get(`${BASE}/prompt/${greetingFile}`);
  check(promptRes.ok() && (await promptRes.body()).length > 1000, `a recording plays back (${(await promptRes.body()).length} bytes)`);
  await owner.screenshot({ path: `${SHOTS}/menu-recorded.png`, fullPage: true });

  step = "caller hears recordings";
  const listener = await newPage(390, 760);
  const heardFiles = [];
  listener.on("request", (r) => { if (r.url().includes("/prompt/")) heardFiles.push(r.url().split("/prompt/")[1]); });
  await listener.goto(BASE + `/call/${LINE}`);
  await listener.click('[data-action="call"]');
  await listener.waitForSelector('[data-key="2"]:visible');
  await listener.click('[data-key="2"]');
  await listener.waitForSelector('[data-action="call"]:visible', { timeout: 15000 });
  check(heardFiles.includes(greetingFile) && heardFiles.includes(hoursFile), "the caller hears the recorded greeting and message");

  step = "back to computer voice";
  await greetingRec.locator("[data-rec-clear]").click();
  await owner.click("main button[type=submit]");
  await owner.waitForSelector("text=Saved.");
  check(await owner.inputValue('[name="greeting_audio"]') === "", "the greeting can go back to the computer voice");
  const gone = await owner.request.get(`${BASE}/prompt/${greetingFile}`);
  check(gone.status() === 404, `the unused recording is deleted (${gone.status()})`);

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

  // Voicemail ---------------------------------------------------------------
  step = "voicemail after no answer";
  await caller.click('[data-action="call"]');
  await caller.waitForSelector('[data-key="1"]:visible');
  await caller.click('[data-key="1"]');
  await owner.waitForSelector('[data-action="decline"]:visible', { timeout: 15000 });
  await owner.click('[data-action="decline"]');
  await caller.waitForSelector('[data-action="send-message"]:visible', { timeout: 20000 });
  check(true, "declining sends the caller to voicemail");
  await caller.waitForTimeout(2500);
  await caller.screenshot({ path: `${SHOTS}/voicemail-recording.png` });
  await caller.click('[data-action="send-message"]');
  await caller.waitForSelector("text=Message sent", { timeout: 10000 });
  check(true, "the caller's message is sent");
  await owner.waitForSelector("[data-voicemail]", { timeout: 10000 });
  check(true, "the phone app shows the new message without reloading");

  step = "voicemail choice";
  await caller.click('[data-action="call"]');
  await caller.waitForSelector('[data-key="4"]:visible');
  await caller.click('[data-key="4"]');
  await caller.waitForSelector('[data-action="send-message"]:visible', { timeout: 20000 });
  await caller.waitForTimeout(1500);
  await caller.click('[data-action="hangup"]');
  await caller.waitForSelector("text=Message sent", { timeout: 10000 });
  check(true, "a 'take a voicemail' choice records, and hanging up sends it");

  step = "voicemail page";
  await owner.goto(BASE + "/dashboard");
  check(await owner.isVisible("text=2 new messages"), "the dashboard counts 2 new messages");
  await owner.click('a[href="/dashboard/voicemail"] >> nth=0');
  await owner.waitForSelector("[data-voicemail]");
  const items = owner.locator("[data-voicemail]");
  check(await items.count() === 2, `the voicemail page lists 2 messages (${await items.count()})`);
  const src = await items.first().locator("audio").getAttribute("src");
  const audioRes = await owner.request.get(BASE + src);
  const audioBytes = (await audioRes.body()).length;
  check(audioRes.ok() && audioBytes > 1000 && /audio/.test(audioRes.headers()["content-type"]), `a message plays back as audio (${audioBytes} bytes, ${audioRes.headers()["content-type"]})`);
  const strangerAudio = await (await newPage()).request.get(BASE + src, { maxRedirects: 0 });
  check(strangerAudio.status() === 303, `a stranger can't download a message (${strangerAudio.status()})`);
  await items.first().locator("audio").evaluate((a) => a.dispatchEvent(new Event("play")));
  await owner.waitForFunction(() => document.querySelectorAll("[data-voicemail] [data-new]").length === 1, null, { timeout: 5000 });
  check(true, "playing a message clears its New badge");
  await owner.screenshot({ path: `${SHOTS}/voicemail.png`, fullPage: true });
  await items.first().locator('button[aria-label="Delete this message"]').click();
  await owner.waitForFunction(() => document.querySelectorAll("[data-voicemail]").length === 1, null, { timeout: 5000 });
  check(true, "deleting a message removes it");

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
  // Delete the test account (cascades to its line, menu, sessions and
  // voicemail rows) and its voicemail files.
  const db = new DatabaseSync(DB);
  db.exec("PRAGMA foreign_keys = ON");
  const files = db.prepare(`SELECT v.file FROM voicemail v JOIN line l ON l.id = v.line_id
    JOIN account a ON a.id = l.account_id WHERE a.name LIKE 'e2e-%@example.com'`).all();
  for (const f of files) rmSync(join(dirname(DB), "voicemail", f.file), { force: true });
  const prompts = db.prepare(`SELECT l.greeting_audio AS f FROM line l JOIN account a ON a.id = l.account_id
      WHERE a.name LIKE 'e2e-%@example.com'
    UNION SELECT o.say_audio FROM menu_option o JOIN line l ON l.id = o.line_id JOIN account a ON a.id = l.account_id
      WHERE a.name LIKE 'e2e-%@example.com'`).all();
  for (const p of prompts) if (p.f) rmSync(join(dirname(DB), "prompts", p.f), { force: true });
  db.prepare("DELETE FROM account WHERE name LIKE 'e2e-%@example.com'").run();
  db.close();
}

for (const e of consoleErrors) failures.push(`browser error: ${e}`);
console.log(failures.length ? `\n${failures.length} problem(s):\n- ${failures.join("\n- ")}` : "\nAll good.");
process.exit(failures.length ? 1 : 0);
