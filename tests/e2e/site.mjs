// End-to-end test of the whole site, against Ken's running dev server.
// Headless only. Run from the project folder:  node tests/e2e/site.mjs
//
// 1. Logged out: visits every link reachable from the landing page.
// 2. Signs up, creates a line, builds a menu, records prompts (a caller hears
//    them), then visits every link again.
// 3. A real call: phone app online, a visitor calls, the phone answers, hang up;
//    then voicemail (unanswered call and a voicemail choice), listen, delete.
//    Video through the relay. The meeting room: a guest waits, the app says so,
//    the owner opens it, a third person, the whiteboard, leaving.
// 4. Promo codes: admins only; create, sign up with it, used up, revoke.
// 5. A stranger can't open the phone app; log out works.
// It deletes its test account at the end. Screenshots: tests/e2e/shots/.
//
// BASE=https://voip.dresdell.com node tests/e2e/site.mjs  tests through Caddy.

import { createRequire } from "node:module";
import { DatabaseSync } from "node:sqlite"; // built into Node 22.5+
import { existsSync, mkdirSync, readdirSync, rmSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import { createServer } from "node:http";
import { createDecipheriv, createECDH, hkdfSync, randomBytes } from "node:crypto";

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

// A fake push service: the test subscribes it as one of the owner's phones,
// keeps the notifications the server sends, and opens them with the phone's
// keys (RFC 8291) to read what's inside.
const pushes = [];
const pushKeys = createECDH("prime256v1");
pushKeys.generateKeys();
const pushAuth = randomBytes(16);
const pushService = createServer((req, res) => {
  const chunks = [];
  req.on("data", (c) => chunks.push(c));
  req.on("end", () => {
    pushes.push({ at: Date.now(), path: req.url, topic: req.headers.topic, urgency: req.headers.urgency, body: Buffer.concat(chunks) });
    res.writeHead(201).end();
  });
});
function openPush(p) {
  const b = p.body, salt = b.subarray(0, 16), idlen = b[20];
  const serverKey = b.subarray(21, 21 + idlen), sealed = b.subarray(21 + idlen);
  const info = Buffer.concat([Buffer.from("WebPush: info\0"), pushKeys.getPublicKey(), serverKey]);
  const ikm = Buffer.from(hkdfSync("sha256", pushKeys.computeSecret(serverKey), pushAuth, info, 32));
  const key = Buffer.from(hkdfSync("sha256", ikm, salt, Buffer.from("Content-Encoding: aes128gcm\0"), 16));
  const nonce = Buffer.from(hkdfSync("sha256", ikm, salt, Buffer.from("Content-Encoding: nonce\0"), 12));
  const d = createDecipheriv("aes-128-gcm", key, nonce);
  d.setAuthTag(sealed.subarray(sealed.length - 16));
  const plain = Buffer.concat([d.update(sealed.subarray(0, sealed.length - 16)), d.final()]);
  let end = plain.length - 1;
  while (end > 0 && plain[end] === 0) end--; // padding, then the 0x02 delimiter
  return { ...JSON.parse(plain.subarray(0, end).toString()), topic: p.topic, urgency: p.urgency };
}
const pushesSince = (i) => pushes.slice(i).map(openPush);
await new Promise((ok) => pushService.listen(0, "127.0.0.1", ok));
const pushURL = `http://localhost:${pushService.address().port}/sub/${Date.now()}`;
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
  const ctx = await browser.newContext({ viewport: { width, height }, permissions: ["microphone", "camera"] });
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
    if (path.startsWith("/download/")) {
      // A file to download (the Android app), not a page.
      const res = await page.request.get(BASE + path);
      check(res.status() === 200 && (await res.body()).length > 100000, `${label}: ${path} downloads (${res.status()})`);
      continue;
    }
    const res = await page.goto(BASE + path, { waitUntil: "domcontentloaded" });
    check(res && res.status() === 200, `${label}: ${path} loads (${res && res.status()})`);
    const links = await page.$$eval("a[href]", (as) => as.map((a) => a.getAttribute("href")));
    for (const href of links) {
      if (href.startsWith("#")) {
        const id = href.slice(1);
        if (id) check(await page.$(`[id="${id}"]`) !== null, `${label}: ${path} anchor ${href} exists`);
        continue;
      }
      if (href.startsWith("mailto:")) continue;
      let target = href;
      if (/^https?:/.test(href)) {
        const u = new URL(href);
        if (u.origin !== new URL(BASE).origin) continue;
        target = u.pathname + u.search;
      }
      const clean = target.split("#")[0];
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
  // "Call us now" calls a real line (Ken's): open its page, never press Call.
  const callUs = await visitorSite.getAttribute("[data-call-us]", "href").catch(() => null);
  if (callUs) {
    const res = await visitorSite.request.get(BASE + callUs);
    check(res.ok() && (await res.text()).includes('data-action="call"'), `"Call us now" opens a working call page (${callUs})`);
    const calls = await visitorSite.$$eval('a[href^="/call/"]', (as) => as.map((a) => a.getAttribute("href")));
    check(calls.length === 2 && calls.every((h) => h === callUs), `every "Call us now" on the landing page calls minivoip's own line (${calls.join(", ")})`);
  } else {
    check(await visitorSite.isVisible('a[href="#how"]'), "without a real line, the hero offers how it works");
  }
  // The hero plays the line's recorded greeting, and the wave and the words follow it.
  // The crawl above already loaded the page, so this is a second visit: the
  // recording comes from the browser's cache (it once broke the words).
  if (await visitorSite.$("[data-voice-play]")) {
    await visitorSite.click("[data-voice-play]");
    await visitorSite.waitForTimeout(2000);
    const voice = await visitorSite.evaluate(() => ({
      playing: document.querySelector("[data-voice]").classList.contains("playing"),
      at: document.querySelector("[data-voice] audio").currentTime,
      said: document.querySelectorAll("[data-voice-text] span.said").length,
      length: document.querySelector("[data-voice-time]").textContent,
    }));
    check(voice.playing && voice.at > 0.5 && voice.length !== "" && voice.said > 0, `the hero plays the greeting (${voice.at.toFixed(1)} s of ${voice.length}, ${voice.said} words said)`);
    await visitorSite.click("[data-voice-play]");
    check(await visitorSite.evaluate(() => document.querySelector("[data-voice] audio").paused), "pressing again pauses the greeting");
  }
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
  await owner.waitForSelector("[data-business]");
  check(true, "sign up leads to step 2, the business name");

  step = "create line";
  await owner.fill("#business", "E2E Shop & Co");
  check(await owner.textContent("[data-link-preview]") === "e2e-shop-and-co", `the call link is made from the business name (${await owner.textContent("[data-link-preview]")})`);
  await owner.screenshot({ path: `${SHOTS}/new-line.png` });
  await owner.click("[data-link-edit]");
  await owner.fill("#name", LINE);
  check(await owner.textContent("[data-link-preview]") === LINE, "'Change it' edits the call link");
  await owner.fill("#business", "E2E Shop");
  await owner.click("form[data-new-line] button[type=submit]");
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
  check(/press 1 to talk to us/i.test(heard) && /press 2 for opening hours/i.test(heard), `"Hear it" reads the menu: ${heard.trim()}`);
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
  await listener.click('[data-action="call"]:visible');
  await listener.waitForSelector('[data-key="2"]:visible');
  await listener.click('[data-key="2"]');
  await listener.waitForFunction(() => /Anything else/.test(document.querySelector("[data-menu-status]").textContent), null, { timeout: 15000 });
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

  // Notifications ---------------------------------------------------------
  step = "install files";
  const manifest = await owner.request.get(BASE + "/manifest.webmanifest");
  const mf = manifest.ok() ? await manifest.json() : {};
  check(mf.start_url === "/app" && mf.icons && mf.icons.length >= 2, "the phone app's install manifest is served");
  const sw = await owner.request.get(BASE + "/sw.js");
  check(sw.ok() && /javascript/.test(sw.headers()["content-type"]) && (await sw.text()).includes("showNotification"), "the service worker is served");
  step = "push panel";
  await owner.goto(BASE + "/app");
  await owner.waitForFunction(() => !/Checking/.test(document.querySelector("[data-push-state]").textContent), null, { timeout: 10000 });
  check(await owner.evaluate(() => navigator.serviceWorker.getRegistration().then((r) => !!r)), "the phone app registers its service worker");
  // Headless Chromium blocks notifications, so the panel must say so; a real
  // phone shows "Off" and the Turn on button.
  const pushText = (await owner.textContent("[data-push-state]")).trim();
  const permission = await owner.evaluate(() => Notification.permission);
  check(permission === "denied" ? /blocked/.test(pushText) : await owner.isVisible("[data-push-on]"),
    `the notification panel explains this browser's state (${permission}): "${pushText}"`);
  await owner.setViewportSize({ width: 390, height: 844 });
  await owner.screenshot({ path: `${SHOTS}/phone-app.png`, fullPage: true });
  await owner.setViewportSize({ width: 1280, height: 900 });
  step = "subscribe";
  const sub = await owner.request.post(BASE + "/app/push/subscribe", {
    headers: { Origin: BASE },
    data: { endpoint: pushURL, keys: { p256dh: pushKeys.getPublicKey("base64url"), auth: pushAuth.toString("base64url") } },
  });
  check(sub.status() === 204, `a phone's push subscription is saved (${sub.status()})`);
  const test = await (await owner.request.post(BASE + "/app/push/test", { headers: { Origin: BASE } })).json();
  check(test.sent === 1 && pushes.length === 1, `"Send a test" reaches the phone (sent ${test.sent}, service got ${pushes.length})`);

  step = "delivery receipt";
  const testPush = openPush(pushes[0]);
  check(testPush.kind === "test" && testPush.id > 0, `the test notification carries its receipt id (${JSON.stringify(testPush)})`);
  const receipt = (id) => owner.request.post(BASE + "/app/push/receipt", { headers: { Origin: BASE }, data: { id, shownAs: "test", note: "e2e" } });
  check((await receipt(testPush.id)).status() === 204, "the phone's receipt is accepted");
  check((await receipt(testPush.id)).status() === 404, "a receipt counts once");
  const logDB = new DatabaseSync(DB, { readOnly: true });
  const logged = logDB.prepare("SELECT result, shown_as, shown_at FROM push_log WHERE id = ?").get(testPush.id);
  logDB.close();
  check(logged && logged.result === "ok" && logged.shown_as === "test" && logged.shown_at, `the push log has the send and the receipt (${JSON.stringify(logged)})`);

  // 3. A real call --------------------------------------------------------
  step = "call";
  await owner.goto(BASE + "/app");
  await owner.waitForSelector('[data-phone][data-online="true"]', { timeout: 10000 });
  const caller = await newPage(390, 760);
  await caller.goto(BASE + `/call/${LINE}`);
  await caller.click('[data-action="call"]:visible');
  await caller.waitForSelector('[data-key="3"]:visible');
  const ringStart = pushes.length;
  await caller.click('[data-key="1"]');
  await owner.waitForSelector('[data-action="accept"]:visible', { timeout: 15000 });
  check(true, "the phone app rings");
  await caller.screenshot({ path: `${SHOTS}/call-ringing.png` });
  await owner.waitForTimeout(11000);
  const rings = pushesSince(ringStart).filter((n) => n.kind === "ring");
  check(rings.length >= 2, `a sleeping phone gets repeated "Incoming call" notifications (${rings.length} in 11 s)`);
  const r0 = rings[0] || {};
  check(r0.call && r0.topic === "call-" + r0.call && r0.urgency === "high" && r0.until > Date.now(),
    `a ring is urgent, replaceable while waiting, and says when it stops (${JSON.stringify(r0)})`);
  await owner.click('[data-action="accept"]');
  await caller.waitForSelector("text=On the line with E2E Shop", { timeout: 15000 });
  await caller.waitForTimeout(2500);
  const direct = await caller.evaluate(() => window.minivoipCallStats());
  check(direct && direct.bytesIn > 1000, `the call connects and the caller hears the phone (${direct && direct.route}, ${direct && direct.bytesIn} bytes)`);
  await caller.screenshot({ path: `${SHOTS}/call.png` });
  await owner.setViewportSize({ width: 390, height: 844 });
  await owner.screenshot({ path: `${SHOTS}/phone-talking.png` });
  await owner.setViewportSize({ width: 1280, height: 900 });
  const answered = pushesSince(ringStart).filter((n) => n.kind === "answered");
  check(answered.length === 1 && answered[0].call === r0.call, `answering clears the ringing on the owner's other devices (${answered.length} "answered")`);
  const afterAnswer = pushes.length;
  await caller.waitForTimeout(6000);
  check(pushes.length === afterAnswer, `answering stops the notifications (${pushes.length - afterAnswer} more)`);
  await caller.click('[data-action="hangup"]:visible');
  await owner.waitForSelector("[data-call-screen]", { state: "hidden", timeout: 10000 });
  check(true, "hanging up reaches the phone");
  await owner.waitForSelector('[data-panel="calls"] .ap-row[data-kind="answered"]', { timeout: 5000 });
  check(/Talk to us/.test(await owner.textContent('[data-panel="calls"] .ap-row[data-kind="answered"]')), "the answered call shows in the call list, with the caller's choice");

  step = "relay call";
  // Both sides forced through the relay (TURN) at the public address: the
  // path a phone on cellular data needs.
  await owner.goto(BASE + "/app?relay=1");
  await owner.waitForSelector('[data-phone][data-online="true"]', { timeout: 10000 });
  await caller.goto(BASE + `/call/${LINE}?relay=1`);
  await caller.click('[data-action="call"]:visible');
  await caller.waitForSelector('[data-key="1"]:visible');
  await caller.click('[data-key="1"]');
  await owner.waitForSelector('[data-action="accept"]:visible', { timeout: 15000 });
  await owner.click('[data-action="accept"]');
  const relayed = await caller.waitForSelector("text=On the line with E2E Shop", { timeout: 20000 }).then(() => true, () => false);
  check(relayed, "a call connects through the relay (TURN)");
  if (relayed) {
    await caller.waitForTimeout(3000);
    const c = await caller.evaluate(() => window.minivoipCallStats());
    const o = await owner.evaluate(() => window.minivoipCallStats());
    check(c && c.route === "relay>relay" && c.bytesIn > 1000 && o && o.bytesIn > 1000,
      `voice flows both ways through the relay (caller ${c && c.route}, ${c && c.bytesIn} bytes in; phone ${o && o.bytesIn} bytes in)`);
    // Video through the relay too: the owner turns the camera on.
    await owner.click('[data-action="camera"]');
    await caller.waitForTimeout(3000);
    const v = await caller.evaluate(() => window.minivoipCallStats());
    check(v && v.videoIn > 20000 && await caller.isVisible('.mv-tile[data-tile="p:owner"].has-video'),
      `the owner's camera reaches the caller through the relay (${v && v.videoIn} video bytes)`);
    await caller.screenshot({ path: `${SHOTS}/call-video.png` });
    await caller.click('[data-action="hangup"]:visible');
    await owner.waitForSelector("[data-call-screen]", { state: "hidden", timeout: 10000 });
  }
  await owner.goto(BASE + "/app");
  await owner.waitForSelector('[data-phone][data-online="true"]', { timeout: 10000 });
  await caller.goto(BASE + `/call/${LINE}`);

  step = "play option";
  await caller.click('[data-action="call"]:visible');
  await caller.waitForSelector('[data-key="2"]:visible');
  await caller.click('[data-key="2"]');
  await caller.waitForFunction(() => /Anything else/.test(document.querySelector("[data-menu-status]").textContent), null, { timeout: 15000 });
  check(true, "after a 'play a message' answer the menu stays, for another choice");
  await caller.click('[data-action="hangup"]:visible');
  await caller.waitForSelector('[data-action="call"]:visible');

  // Voicemail ---------------------------------------------------------------
  step = "voicemail after no answer";
  await caller.click('[data-action="call"]:visible');
  await caller.waitForSelector('[data-key="1"]:visible');
  await caller.click('[data-key="1"]');
  await owner.waitForSelector('[data-action="decline"]:visible', { timeout: 15000 });
  const beforeDecline = pushes.length;
  await owner.click('[data-action="decline"]');
  await caller.waitForSelector('[data-action="send-message"]:enabled', { timeout: 20000 });
  check(true, "declining sends the caller to voicemail");
  await caller.waitForTimeout(2500);
  await caller.screenshot({ path: `${SHOTS}/voicemail-recording.png` });
  await caller.click('[data-action="send-message"]');
  await caller.waitForSelector("text=Message sent", { timeout: 10000 });
  check(true, "the caller's message is sent");
  await caller.waitForTimeout(1000);
  const afterDecline = pushesSince(beforeDecline).map((n) => n.kind);
  check(afterDecline.includes("missed") && afterDecline.includes("voicemail"), `declining sends "Missed call", the message sends "New voicemail" (${afterDecline.join(", ")})`);
  await owner.waitForSelector("[data-voicemail]", { timeout: 10000 });
  check(true, "the phone app shows the new message without reloading");
  check(await owner.isVisible("[data-vm-dot]"), "the Voicemail tab shows a dot for the new message");

  step = "voicemail choice";
  await caller.click('[data-action="call"]:visible');
  await caller.waitForSelector('[data-key="4"]:visible');
  await caller.click('[data-key="4"]');
  await caller.waitForSelector('[data-action="send-message"]:enabled', { timeout: 20000 });
  await caller.waitForTimeout(1500);
  await caller.click('[data-action="send-message"]');
  await caller.waitForSelector("text=Message sent", { timeout: 10000 });
  check(true, "a 'take a voicemail' choice records, and Send my message sends it");

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

  step = "away";
  await owner.goto(BASE + "/app");
  await owner.waitForSelector('[data-phone][data-online="true"]', { timeout: 10000 });
  await owner.click("[data-avail-btn]");
  await owner.click('[data-avail-menu] [data-mode="off"]');
  await owner.waitForSelector('[data-avail-state][data-mode="off"]');
  await caller.click('[data-action="call"]:visible');
  await caller.waitForSelector('[data-key="1"]:visible');
  await caller.click('[data-key="1"]');
  await caller.waitForSelector('[data-action="send-message"]:enabled', { timeout: 20000 });
  check(await owner.isHidden("[data-call-screen]"), "Off: a call goes straight to voicemail and the app doesn't ring");
  await caller.waitForTimeout(1500);
  await caller.click('[data-action="send-message"]');
  await caller.waitForSelector("text=Message sent", { timeout: 10000 });
  const callDB = new DatabaseSync(DB, { readOnly: true });
  const outcomes = callDB.prepare(`SELECT c.outcome FROM call_log c JOIN line l ON l.id = c.line_id WHERE l.name = ? ORDER BY c.id`).all(LINE).map((r) => r.outcome);
  callDB.close();
  check(outcomes.includes("answered") && outcomes.includes("declined") && outcomes[outcomes.length - 1] === "away",
    `the call history has every call (${outcomes.join(", ")})`);
  await owner.click("[data-avail-btn]");
  await owner.click('[data-avail-menu] [data-mode="on"]');
  await owner.waitForSelector('[data-avail-state][data-mode="on"]');
  check(true, "the Available switch turns back on");

  // Meeting room ----------------------------------------------------------
  step = "meeting room links";
  await owner.goto(BASE + "/dashboard");
  check((await owner.inputValue("#room-link")).endsWith(`/room/${LINE}`), "the dashboard shows the meeting room link");
  await owner.goto(BASE + "/app");
  await owner.waitForSelector('[data-phone][data-online="true"]', { timeout: 10000 });
  await owner.click('[data-tab="room"]');
  check((await owner.textContent("[data-room-url]")).trim().endsWith(`/room/${LINE}`) && await owner.isVisible("[data-room-open]"), "the app's Room tab has the link and Open the room");

  step = "guest waits";
  const roomPushes = pushes.length;
  const guest = await newPage(390, 844);
  await guest.goto(BASE + `/room/${LINE}`);
  check(!(await guest.isVisible("text=You're the host")), "a guest isn't the host");
  await guest.fill("[data-name]", "Ana Roy");
  await guest.click('[data-action="join"]');
  await guest.waitForSelector("text=Waiting for", { timeout: 10000 });
  await guest.screenshot({ path: `${SHOTS}/room-waiting.png` });
  await owner.waitForSelector("[data-room-alert]:visible", { timeout: 5000 });
  check(/Ana Roy is waiting/.test(await owner.textContent("[data-room-alert]")), "the app says who is waiting in the meeting room");
  await owner.waitForTimeout(800);
  check(pushesSince(roomPushes).some((p) => p.kind === "room" && p.url === `/room/${LINE}`), "a sleeping phone gets “Someone is waiting in your meeting room”");

  step = "owner opens the room";
  const [host] = await Promise.all([owner.context().waitForEvent("page"), owner.click("[data-room-alert]")]);
  host.on("pageerror", (e) => consoleErrors.push(`${step}: ${e}`));
  await host.waitForSelector('[data-action="join"]:visible');
  check(await host.isVisible("text=You're the host"), "the owner's room page says they're the host");
  await host.click('.rm-toggle[data-action="pre-camera"]');
  await host.waitForSelector("[data-preview].has-video");
  await host.waitForFunction(() => document.querySelector("[data-preview-video]").videoWidth > 0);
  check(await host.isHidden("text=This room is full") && await host.isHidden("text=You left the room") && await host.isHidden("text=Waiting for"),
    "getting ready shows only its own step");
  await host.waitForTimeout(300);
  await host.screenshot({ path: `${SHOTS}/room-ready.png` });
  await host.click('[data-action="join"]');
  await guest.waitForSelector(".rm-in:visible", { timeout: 10000 });
  await host.waitForSelector(".rm-in:visible", { timeout: 10000 });
  check(true, "opening the room lets the waiting guest in");
  await owner.waitForSelector("[data-room-alert]", { state: "hidden", timeout: 5000 });
  check(true, "the app's waiting alert goes away");
  await guest.waitForTimeout(3000);
  const gs = await guest.evaluate(() => window.minivoipRoomStats());
  check(gs.length === 1 && gs[0].bytesIn > 1000 && gs[0].videoIn > 5000, `the guest hears and sees the owner (${JSON.stringify(gs)})`);

  step = "third person";
  const bo = await newPage(1280, 900);
  await bo.goto(BASE + `/room/${LINE}`);
  await bo.fill("[data-name]", "Bo");
  await bo.click('[data-action="join"]');
  await bo.waitForSelector(".rm-in:visible", { timeout: 10000 });
  await bo.waitForFunction(() => /3 people/.test(document.querySelector("[data-people]").textContent), null, { timeout: 10000 });
  check(/3 people/.test(await host.textContent("[data-people]")), "a third person comes straight in while the owner is there");
  await bo.waitForTimeout(3000);
  const bs = await bo.evaluate(() => window.minivoipRoomStats());
  check(bs.length === 2 && bs.every((x) => x.bytesIn > 1000), `everyone hears everyone (${bs.map((x) => x.bytesIn).join(", ")} bytes)`);
  check(await bo.isVisible('.mv-stage[data-layout="grid"]'), "three people show as a grid");
  await bo.click('.rm-controls [data-action="board"]');
  await host.waitForSelector(".mv-board", { timeout: 5000 });
  await guest.waitForSelector(".mv-board", { timeout: 5000 });
  const paper = await bo.locator(".mv-paper canvas").boundingBox();
  await bo.mouse.move(paper.x + 100, paper.y + 100);
  await bo.mouse.down();
  await bo.mouse.move(paper.x + 300, paper.y + 220, { steps: 10 });
  await bo.mouse.up();
  await host.waitForFunction(() => document.querySelector(".mv-stage").meet.wb.strokes.length === 1, null, { timeout: 5000 });
  await guest.waitForFunction(() => document.querySelector(".mv-stage").meet.wb.strokes.length === 1, null, { timeout: 5000 });
  check(true, "the whiteboard opens for everyone, and a drawing shows for everyone");
  await bo.screenshot({ path: `${SHOTS}/room-board.png` });
  await guest.screenshot({ path: `${SHOTS}/room-phone-board.png` });
  await bo.click('[data-board="close"]');
  await bo.waitForTimeout(500);
  await bo.screenshot({ path: `${SHOTS}/room.png` });
  await bo.click('.rm-controls [data-action="leave"]');
  await host.waitForFunction(() => /2 people/.test(document.querySelector("[data-people]").textContent), null, { timeout: 5000 });
  check(await bo.isVisible("text=You left the room"), "Leave leaves; the others see one less");
  await guest.click('.rm-controls [data-action="leave"]');
  await host.waitForFunction(() => /just you/.test(document.querySelector("[data-people]").textContent), null, { timeout: 5000 });
  await host.click('.rm-controls [data-action="leave"]');
  await host.close();

  // Promo codes (admin) -----------------------------------------------------
  step = "admin only";
  await owner.goto(BASE + "/dashboard");
  check(!(await owner.isVisible('a[href="/admin/promo"]')), "a normal user has no Admin link");
  check((await owner.request.get(BASE + "/admin/promo")).status() === 404, "a normal user can't open the admin page");
  const adminDB = new DatabaseSync(DB);
  adminDB.prepare("UPDATE user SET is_admin = 1 WHERE email = ?").run(EMAIL);
  adminDB.close();
  await owner.reload();
  await owner.click('a[href="/admin/promo"]');
  await owner.waitForSelector("#code");
  check(true, "an admin reaches the promo code page from the dashboard");

  step = "create code";
  const CODE = `E2E-${stamp.toUpperCase()}`;
  await owner.fill("#code", CODE.toLowerCase());
  await owner.fill("#max_uses", "1");
  await owner.click("main button[type=submit]");
  await owner.waitForSelector(`text=${CODE} is ready`);
  check(await owner.isVisible(`[data-promo="${CODE}"] >> text=0 of 1 used`), "the new code is listed, unused");
  await owner.fill("#code", CODE);
  await owner.click("main button[type=submit]");
  check(await owner.isVisible("text=already exists"), "the same code can't be made twice");

  step = "friend signs up with the code";
  const friend = await newPage();
  const FRIEND = `e2e-${stamp}-friend@example.com`;
  await friend.goto(BASE + "/signup");
  await friend.fill("#email", FRIEND);
  await friend.fill("#password", "e2e-password");
  await friend.fill("#promo", CODE.toLowerCase());
  await friend.click("main button[type=submit]");
  await friend.waitForSelector("[data-business]");
  await friend.fill("#business", `e2e ${stamp} friend`);
  await friend.click("form[data-new-line] button[type=submit]");
  await friend.waitForSelector("[data-add-choice]");
  await friend.goto(BASE + "/dashboard");
  check((await friend.textContent("[data-plan]")).includes(`free forever, with the code ${CODE}`), "a friend who used the code is free forever");

  step = "code used up";
  const late = await newPage();
  await late.goto(BASE + "/signup");
  await late.fill("#email", `e2e-${stamp}-late@example.com`);
  await late.fill("#password", "e2e-password");
  await late.fill("#promo", CODE);
  await late.click("main button[type=submit]");
  check(await late.isVisible("text=promo code isn't valid"), "a used-up code is refused at sign up");

  step = "revoke";
  await owner.goto(BASE + "/admin/promo");
  check(await owner.isVisible(`[data-promo="${CODE}"] >> text=1 of 1 used`) && await owner.isVisible(`[data-promo="${CODE}"] >> text=${FRIEND}`), "the admin page shows who used the code");
  await owner.screenshot({ path: `${SHOTS}/admin-promo.png`, fullPage: true });
  await owner.setViewportSize({ width: 390, height: 844 });
  const adminWide = await owner.evaluate(() => document.documentElement.scrollWidth);
  check(adminWide <= 390, `phone width: /admin/promo doesn't scroll sideways (${adminWide}px)`);
  await owner.screenshot({ path: `${SHOTS}/admin-promo-phone.png`, fullPage: true });
  await owner.setViewportSize({ width: 1280, height: 900 });
  await owner.click(`[data-promo="${CODE}"] button[type=submit]`);
  await owner.waitForSelector(`[data-promo="${CODE}"] >> text=Revoked`);
  await friend.reload();
  check((await friend.textContent("[data-plan]")).includes("was revoked"), "revoking a code shows on the friend's dashboard");

  // 4. Strangers and log out ----------------------------------------------
  step = "stranger";
  const stranger = await newPage(390, 844);
  await stranger.goto(BASE + "/app");
  check(new URL(stranger.url()).pathname === "/login" && await stranger.isVisible("[data-app-login]"), "a stranger can't open the phone app, and gets the app's own log in");
  await stranger.screenshot({ path: `${SHOTS}/app-login.png` });

  step = "phone app log in and out";
  const phoneOnly = await newPage(390, 844);
  await phoneOnly.goto(BASE + "/app");
  await phoneOnly.fill("#email", EMAIL);
  await phoneOnly.fill("#password", "e2e-password");
  await phoneOnly.click("button[type=submit]");
  await phoneOnly.waitForSelector('[data-phone][data-online="true"]', { timeout: 10000 });
  check(new URL(phoneOnly.url()).pathname === "/app", "logging in from the phone app stays in the app");
  check(!(await phoneOnly.isVisible('a[href="/dashboard"]')), "the phone app has no dashboard links");
  await phoneOnly.click('[data-tab="me"]');
  await phoneOnly.click("text=Log out");
  await phoneOnly.waitForSelector("[data-app-login]");
  check(true, "logging out of the phone app returns to the app's log in");
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
  pushService.close();
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
  db.prepare("DELETE FROM push_log WHERE line LIKE 'e2e-%'").run();
  db.prepare("DELETE FROM promo_code WHERE code LIKE 'E2E-%'").run();
  db.close();
}

for (const e of consoleErrors) failures.push(`browser error: ${e}`);
console.log(failures.length ? `\n${failures.length} problem(s):\n- ${failures.join("\n- ")}` : "\nAll good.");
process.exit(failures.length ? 1 : 0);
