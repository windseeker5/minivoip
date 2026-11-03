// End-to-end test of the whole site, against Ken's running dev server.
// Headless only. Run from the project folder:  node tests/e2e/site.mjs
//
// 1. Logged out: visits every link reachable from the landing page.
// 2. Signs up, creates a line, builds a menu, records prompts (a caller hears
//    them), then visits every link again.
// 3. A real call: phone app online, a visitor calls, the phone answers, hang up;
//    then voicemail (unanswered call and a voicemail choice), listen, delete.
//    A new account is Basic (voice only, no meeting room); then Premium:
//    video through the relay. The meeting room: a guest waits, the app says so,
//    the owner opens it, a third person, the whiteboard, leaving.
// 4. Promo codes and plans: admins only; a Premium code, sign up with it, used
//    up, revoke, switch an account between Basic and Premium.
// 5. A stranger can't open the phone app; log out works. Security: no account
//    without a promo code, the host lets each guest into the meeting room,
//    Change password, a made-up recording type is refused.
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
const PREMIUM_EMAIL = `e2e-${stamp}-premium@example.com`;
// No account without a promo code: the test's own, removed at the end.
const JOIN_CODE = `E2E-JOIN-${stamp.toUpperCase()}`;
{
  const db = new DatabaseSync(DB);
  db.prepare("INSERT INTO promo_code (code, plan) VALUES (?, 'basic')").run(JOIN_CODE);
  db.close();
}
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

// until retries fn until it gives something truthy, or ms run out.
const until = async (fn, ms = 8000) => { const end = Date.now() + ms; let v; while (Date.now() < end) { v = await fn(); if (v) return v; await new Promise((r) => setTimeout(r, 200)); } return v; };

// setAvail flips the phone app's Available switch: three buttons on a
// computer, Change and a menu on a phone.
async function setAvail(page, mode) {
  if (await page.isVisible("[data-avail-seg]")) await page.click(`[data-avail-seg] [data-mode="${mode}"]`);
  else {
    await page.click("[data-avail-btn]");
    await page.click(`[data-avail-menu] [data-mode="${mode}"]`);
  }
  await page.waitForSelector(`[data-avail-state][data-mode="${mode}"]`);
}

async function newPage(width = 1280, height = 900) {
  const ctx = await browser.newContext({ viewport: { width, height }, permissions: ["microphone", "camera"] });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => consoleErrors.push(`${step}: ${e}`));
  page.on("console", (m) => {
    // The dev proxy's live-reload stream drops when the app restarts; not ours.
    // A form sent back with an error answers 422 on purpose. An app whose login
    // ended is refused its socket on purpose, then goes to the login screen.
    if (m.type() === "error" && !/_templ\/reload|status of 422|role=owner.*code: 403|frame-ancestors|Refused to display/.test(m.text())) consoleErrors.push(`${step}: ${m.text()}`);
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
  // A dark computer with the site set to light: Chrome drew saved emails and
  // passwords on dark grey-blue. The fields follow the site's theme, and a
  // field the browser fills in stays white.
  {
    step = "log in";
    const ctx = await browser.newContext({ viewport: { width: 1600, height: 1000 }, colorScheme: "dark" });
    await ctx.addInitScript(() => localStorage.setItem("theme", "light"));
    const p = await ctx.newPage();
    await p.goto(BASE + "/login");
    await p.fill("#email", "someone@example.com");
    const cdp = await ctx.newCDPSession(p);
    await cdp.send("DOM.enable"); await cdp.send("CSS.enable");
    const { root } = await cdp.send("DOM.getDocument");
    const { nodeId } = await cdp.send("DOM.querySelector", { nodeId: root.nodeId, selector: "#email" });
    await cdp.send("CSS.forcePseudoState", { nodeId, forcedPseudoClasses: ["autofill"] });
    const look = await p.evaluate(() => {
      const r = document.querySelector("#email").getBoundingClientRect(), card = document.querySelector("main > div").getBoundingClientRect();
      return { scheme: getComputedStyle(document.documentElement).colorScheme, filled: getComputedStyle(document.querySelector("#email")).boxShadow, mid: (card.top + card.bottom) / 2, h: r.height };
    });
    check(look.scheme === "light" && /^oklch\(1 0 0\) 0px 0px 0px 100px inset/.test(look.filled), `the log in fields stay white when the browser fills them in (${look.scheme}, ${look.filled.slice(0, 40)})`);
    check(look.mid > 350 && look.mid < 550 && look.h === 40, `on a computer the log in card sits in the middle, fields as tall as the button (${Math.round(look.mid)}, ${look.h})`);
    await ctx.close();
  }
  step = "landing";
  await visitorSite.goto(BASE + "/");
  await visitorSite.screenshot({ path: `${SHOTS}/landing.png` });
  // "Call us now" calls a real line (Ken's): open its page, never press Call.
  const callUs = await visitorSite.getAttribute("[data-call-us]", "href").catch(() => null);
  if (callUs) {
    const res = await visitorSite.request.get(BASE + callUs);
    check(res.ok() && (await res.text()).includes('data-action="call"'), `"Call us now" opens a working call page (${callUs})`);
    const calls = await visitorSite.$$eval('a[href^="/call/"]', (as) => as.map((a) => a.getAttribute("href")));
    check(calls.length === 3 && calls.every((h) => h === callUs), `every "Call us now" on the landing page (and the questions' "ask me") calls minivoip's own line (${calls.join(", ")})`);
    // On a computer it opens our own call window over the page (w.js). Only
    // the call page loads in it; Call is never pressed.
    await until(async () => visitorSite.evaluate(() => { const m = document.querySelector("[data-minivoip]")?.shadowRoot?.querySelector(".mv"); return m && !m.hidden; }));
    await visitorSite.click("[data-call-us]");
    const landingCall = await until(async () => visitorSite.frames().find((f) => f.url().endsWith(callUs)), 10000);
    check(!!landingCall && visitorSite.context().pages().length === 1, `"Call us now" opens the call window over the landing page, not a new tab`);
    if (landingCall) await landingCall.waitForSelector('[data-action="call"]:visible');
    await visitorSite.waitForTimeout(500);
    await visitorSite.screenshot({ path: `${SHOTS}/landing-call.png` });
    await visitorSite.click(".close");
    check(await until(async () => visitorSite.frames().length === 1), "× closes it");
    await visitorSite.click("[data-call-us]");
    await until(async () => visitorSite.frames().find((f) => f.url().endsWith(callUs)), 10000);
    const landingWindow = await visitorSite.evaluate(() => {
      const d = document.querySelector("[data-minivoip]").shadowRoot.querySelector(".panel");
      return { modal: d.matches(":modal"), blur: getComputedStyle(d, "::backdrop").backdropFilter, scroll: document.documentElement.style.overflow };
    });
    check(landingWindow.modal && /blur/.test(landingWindow.blur) && landingWindow.scroll === "hidden", `the window is modal: the page behind is dimmed, blurred and doesn't scroll (${JSON.stringify(landingWindow)})`);
    await visitorSite.locator(".close").focus();
    await visitorSite.keyboard.press("Escape");
    check(await until(async () => visitorSite.frames().length === 1), "Esc closes it when no call is on");
  } else {
    check(await visitorSite.isVisible('a[href="#how"]'), "without a real line, the hero offers how it works");
  }
  // The message (docs/landing-copy.md, 2026-10-31): a real person in one
  // click, Basic $2 and Premium $10, nothing "free" but a promo code.
  const landingText = await visitorSite.textContent("body");
  check(landingText.includes("Let people call you from your website, in one click.") && landingText.includes("It filters. You answer. You show them."), "the landing page says what it does in one line");
  check(landingText.includes("$10") && !/free while in beta|free line/i.test(landingText), "Premium is $10 and Basic isn't sold as free");
  // "Get Premium": no payments yet, the email goes on the admin page's list.
  step = "get premium";
  await visitorSite.fill("#premium-email", PREMIUM_EMAIL);
  await visitorSite.click("[data-premium-form] button[type=submit]");
  await visitorSite.waitForSelector("[data-premium-done]:visible");
  check((await visitorSite.textContent("[data-premium-done]")).includes("Thanks") && !(await visitorSite.isVisible("#premium-email")) && !(await visitorSite.isVisible("[data-premium-form] button")), `"Get Premium" thanks the visitor in place`);
  {
    const db = new DatabaseSync(DB);
    const row = db.prepare("SELECT email FROM premium_request WHERE email = ?").get(PREMIUM_EMAIL);
    db.close();
    check(!!row, "the Premium email is saved");
  }
  await visitorSite.locator("#pricing").screenshot({ path: `${SHOTS}/landing-pricing.png` });
  await visitorSite.setViewportSize({ width: 390, height: 844 });
  await visitorSite.goto(BASE + "/");
  const landingWide = await visitorSite.evaluate(() => document.documentElement.scrollWidth);
  check(landingWide <= 390, `phone width: the landing page doesn't scroll sideways (${landingWide}px)`);
  await visitorSite.screenshot({ path: `${SHOTS}/landing-phone.png`, fullPage: true });
  await visitorSite.setViewportSize({ width: 1280, height: 900 });
  await visitorSite.goto(BASE + "/");
  step = "landing";
  // The hero's five steps: all shown once their animation is over (8 s), the
  // line between them filled, and the faint voice wave drawn behind it.
  await visitorSite.waitForTimeout(8500);
  const steps = await visitorSite.evaluate(() => [...document.querySelectorAll("[data-hero-steps] .lp-step")].map((s) => getComputedStyle(s).opacity));
  check(steps.length === 5 && steps.every((o) => o === "1"), `the hero's five steps all show (${steps.join(", ")})`);
  const wave = await visitorSite.evaluate(() => {
    const c = document.querySelector("[data-hero-wave]");
    const px = c.getContext("2d").getImageData(0, 0, c.width, c.height).data;
    let drawn = 0;
    for (let i = 3; i < px.length; i += 4) if (px[i] > 200) drawn++;
    const line = document.querySelector("[data-hero-steps] .lp-steps-line > i");
    return { drawn, filled: line.getAnimations().every((a) => a.playState === "finished") && getComputedStyle(line).clipPath };
  });
  check(wave.drawn > 1000 && wave.filled === "inset(0px)", `the line between the steps is filled (${wave.filled}) and the voice wave is drawn (${wave.drawn} pixels)`);
  await visitorSite.screenshot({ path: `${SHOTS}/landing-hero.png` });
  await visitorSite.goto(BASE + "/dashboard");
  check(new URL(visitorSite.url()).pathname === "/login", "dashboard sends logged-out visitors to log in");

  // 2. Sign up, line, menu ------------------------------------------------
  const owner = await newPage();
  step = "sign up";
  await owner.goto(BASE + "/");
  await owner.click('nav a[href="/signup"]');
  check(await owner.getAttribute("#promo", "required") !== null && await owner.isVisible("text=You need a promo code"), "the sign up page says a promo code is needed");
  await owner.screenshot({ path: `${SHOTS}/signup.png` });
  const noCode = await owner.request.post(BASE + "/signup", { form: { email: EMAIL, password: "e2e-password" }, maxRedirects: 0 });
  check(noCode.status() === 422 && (await noCode.text()).includes("Enter your promo code"), "no promo code, no account");
  await owner.fill("#email", EMAIL);
  await owner.fill("#password", "e2e-password");
  await owner.fill("#promo", JOIN_CODE);
  await owner.click("main button[type=submit]");
  await owner.waitForSelector("[data-business]");
  check(true, "sign up with a code leads to step 2, the business name");

  step = "create line";
  await owner.fill("#business", "E2E Shop & Co");
  check(await owner.textContent("[data-link-preview]") === "e2e-shop-and-co", `the call link is made from the business name (${await owner.textContent("[data-link-preview]")})`);
  await owner.screenshot({ path: `${SHOTS}/new-line.png` });
  await owner.click("[data-link-edit]");
  await owner.fill("#name", LINE);
  check(await owner.textContent("[data-link-preview]") === LINE, "'Change it' edits the call link");
  await owner.fill("#business", "E2E Shop");
  // The logo, asked here too. A file that isn't a picture: the page refuses it, and so does the server.
  await owner.setInputFiles("#logo", { name: "logo.txt", mimeType: "text/plain", buffer: Buffer.from("not a picture") });
  check(await owner.isVisible("[data-logo-error]") && (await owner.textContent("[data-logo-error]")).includes("isn't a picture") && await owner.inputValue("#logo") === "",
    "a file that isn't a picture is refused right away");
  const sent = await owner.request.post(BASE + "/dashboard/line", { multipart: { business: "E2E Shop", name: LINE, logo: { name: "logo.png", mimeType: "image/png", buffer: Buffer.from("not a picture") } } });
  check(sent.status() === 422 && (await sent.text()).includes("PNG, JPG or WebP"), "the server refuses it too, without making the line");
  // A big photo (about 10 MB): framed in the crop window, sent as a small square.
  const big = Buffer.from(await owner.evaluate(() => {
    const c = Object.assign(document.createElement("canvas"), { width: 2400, height: 1500 }), g = c.getContext("2d"), d = g.createImageData(2400, 1500);
    for (let i = 0; i < d.data.length; i++) d.data[i] = i % 4 === 3 ? 255 : Math.random() * 255;
    g.putImageData(d, 0, 0);
    g.fillStyle = "#f46422"; g.fillRect(700, 400, 1000, 700);
    return c.toDataURL("image/png").split(",")[1];
  }), "base64");
  await owner.setInputFiles("#logo", { name: "photo.png", mimeType: "image/png", buffer: big });
  await owner.waitForSelector("[data-logo-crop] .cropper-crop-box", { timeout: 10000 });
  await owner.click('[data-crop="in"]');
  await owner.screenshot({ path: `${SHOTS}/logo-crop.png` });
  await owner.click('[data-crop="use"]');
  await owner.waitForSelector("[data-logo-preview]:visible");
  const framed = await owner.evaluate(() => { const f = document.querySelector("#logo").files[0]; return f && { size: f.size, type: f.type }; });
  check(framed && framed.size < 600000 && /image\/(webp|png)/.test(framed.type) && !(await owner.locator("[data-logo-crop]").evaluate((d) => d.open)),
    `a ${(big.length / 1e6).toFixed(1)} MB photo goes up framed and small (${framed && framed.type}, ${framed && Math.round(framed.size / 1000)} KB)`);
  await owner.click("form[data-new-line] button[type=submit]");
  await owner.waitForSelector("[data-add-choice]");
  check(true, "creating the line opens the menu editor");

  step = "menu editor";
  const rows = owner.locator("[data-choice]");
  const openRow = (row) => row.evaluate((d) => { d.open = true; });
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
  check(/press 3 for our website/i.test(await owner.textContent("[data-script]")), "the words to say when recording follow the choices");
  check(/plays your voicemail greeting/i.test(await rows.nth(3).locator("[data-sum-words]").textContent()) && await rows.nth(3).locator("[data-say-box]").isHidden(), "a voicemail choice uses the voicemail greeting, with no message of its own");
  await owner.click("main button[type=submit]");
  await owner.waitForSelector("text=Saved.");
  check(await owner.isHidden("[data-save]"), "menu saves without leaving the page");
  await owner.reload();
  check(await rows.count() === 4 && await rows.nth(1).locator("[name=say]").isHidden() && await rows.nth(1).locator("summary").isVisible(), "after a reload the saved choices are folded into short rows");
  check(/Opening hours/.test(await rows.nth(1).locator("[data-sum-label]").textContent()) && /Plays a message/.test(await rows.nth(1).locator("[data-sum-words]").textContent()), "a folded row says what the choice does");
  check(await owner.isVisible('[name="greeting_audio"] ~ .hr-head [data-rec-start]') && await owner.isVisible('[name="voicemail_audio"] ~ .hr-head [data-rec-start]'), "the welcome and the voicemail greeting each have a record button in sight");
  await owner.screenshot({ path: `${SHOTS}/menu.png`, fullPage: true });

  step = "menu errors";
  await openRow(rows.nth(2));
  await rows.nth(2).locator("[name=url]").fill("");
  await owner.click("main button[type=submit]");
  await owner.waitForSelector("text=needs a web address");
  check(true, "the page explains a missing web address");

  // Recordings ------------------------------------------------------------
  step = "record greeting";
  await rows.nth(2).locator("[name=url]").fill("https://example.com/");
  const greetingRec = owner.locator('[data-recorder]:has([name="greeting_audio"])');
  const voicemailRec = owner.locator('[data-recorder]:has([name="voicemail_audio"])');
  async function record(box, ms) {
    await box.locator("[data-rec-start]").click();
    await owner.waitForTimeout(ms);
    await box.locator("[data-rec-stop]").click();
    await box.locator("text=Recorded.").waitFor({ timeout: 10000 });
  }
  await record(greetingRec, 2000);
  await openRow(rows.nth(1));
  await record(rows.nth(1).locator("[data-recorder]"), 1500);
  await record(voicemailRec, 1500);
  await owner.click("main button[type=submit]");
  await owner.waitForSelector("text=Saved.");
  const greetingFile = await owner.inputValue('[name="greeting_audio"]');
  const hoursFile = await rows.nth(1).locator('[name="say_audio"]').inputValue();
  const voicemailFile = await owner.inputValue('[name="voicemail_audio"]');
  check(/^prompt-/.test(greetingFile) && /^prompt-/.test(hoursFile) && /^prompt-/.test(voicemailFile), "recorded greeting, message and voicemail greeting are saved");
  check((await greetingRec.locator("[data-rec-chip]").textContent()) === "Your voice" && await greetingRec.locator("[data-rec-text]").isHidden() && await greetingRec.locator("[data-rec-player]").isVisible(), "a recorded card says Your voice and shows its player");
  await greetingRec.locator("[data-rec-bars] i").first().waitFor({ state: "attached", timeout: 5000 });
  await greetingRec.locator("[data-rec-play]").click();
  await greetingRec.locator('[data-rec-play][aria-label="Pause"]').waitFor({ timeout: 5000 });
  await greetingRec.locator("[data-rec-play]").click();
  check(true, "the player plays a recording");
  await openRow(rows.nth(0));
  await rows.nth(0).locator("[name=label]").fill("Talk to reception");
  check(await owner.isVisible("[data-script-stale]"), "changing a choice says the recorded welcome must be recorded again");
  await rows.nth(0).locator("[name=label]").fill("Talk to us");
  await owner.click("main button[type=submit]");
  await owner.waitForSelector("text=Saved.");
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

  // The voicemail greeting: a caller who presses "Leave a message" hears the
  // owner's recording, then their own words once the recording is gone.
  step = "voicemail greeting";
  const vmHeard = [];
  const vmCaller = await newPage(390, 760);
  vmCaller.on("request", (r) => { if (r.url().includes("/prompt/")) vmHeard.push(r.url().split("/prompt/")[1]); });
  await vmCaller.goto(BASE + `/call/${LINE}`);
  await vmCaller.click('[data-action="call"]:visible');
  await vmCaller.waitForSelector('[data-key="4"]:visible');
  await vmCaller.click('[data-key="4"]');
  await vmCaller.waitForSelector("[data-vm-words]:visible");
  await vmCaller.waitForFunction((f) => performance.getEntriesByType("resource").some((e) => e.name.includes(f)), voicemailFile, { timeout: 10000 });
  check(vmHeard.includes(voicemailFile), "a caller leaving a message hears the recorded voicemail greeting");
  await vmCaller.context().close();
  await voicemailRec.locator("[data-rec-clear]").click();
  await owner.fill("#voicemail-text", "Sorry we missed you. Leave your number after the beep.");
  await owner.click("main button[type=submit]");
  await owner.waitForSelector("text=Saved.");
  check((await owner.request.get(`${BASE}/prompt/${voicemailFile}`)).status() === 404, "the unused voicemail recording is deleted");
  await owner.reload();
  check((await voicemailRec.locator("[data-rec-chip]").textContent()) === "Computer voice" && await owner.inputValue("#voicemail-text") === "Sorry we missed you. Leave your number after the beep.", "the voicemail greeting's own words are kept");
  const vmWords = await newPage(390, 760);
  await vmWords.goto(BASE + `/call/${LINE}`);
  await vmWords.click('[data-action="call"]:visible');
  await vmWords.waitForSelector('[data-key="4"]:visible');
  await vmWords.click('[data-key="4"]');
  await vmWords.waitForSelector("[data-vm-words]:visible");
  check(/Sorry we missed you/.test(await vmWords.textContent("[data-vm-words]")), "the caller gets the owner's voicemail words");
  await vmWords.context().close();

  const pagesIn = await crawl(owner, "/", "logged in");
  for (const p of ["/dashboard", "/dashboard/menu", "/dashboard/voicemail", "/dashboard/business", "/app", `/call/${LINE}`]) check(pagesIn.has(p), `logged in: ${p} is reachable by links`);
  step = "dashboard";
  await owner.goto(BASE + "/dashboard");
  await owner.screenshot({ path: `${SHOTS}/dashboard.png`, fullPage: true });
  const loaded = (p, sel) => p.evaluate((s) => [...document.querySelectorAll(s)].filter((i) => i.complete && i.naturalWidth > 0).length, sel);
  check(await loaded(owner, "[data-logo-badge] img") === 1, "the dashboard shows the logo given at sign up");
  check(/Welcome[\s\S]*Computer voice[\s\S]*4 choices[\s\S]*Voicemail greeting[\s\S]*Computer voice/.test(await owner.textContent("[data-hear-card]")), "Home says what callers hear, and whose voice");
  await owner.click('[data-hear-card] a:has-text("Change")');
  await owner.waitForSelector("[data-menu-form]");
  check(new URL(owner.url()).pathname === "/dashboard/menu" && await owner.isVisible('aside a[aria-current="page"]:has-text("What callers hear")'), "Home's Change opens What callers hear");
  await owner.goto(BASE + "/dashboard");
  // Get set up ticks what the line really has: a menu of its own and a logo, but no phone yet.
  const band = (await owner.textContent("[data-setup-band]")).trim();
  check(band.startsWith("One step left: make your phone ring."), `the setup band shows the one step left: a menu of its own and a logo are done, the phone isn't (${band.slice(0, 60)})`);
  check((await owner.getAttribute("[data-line-status]", "data-on")) === null && (await owner.textContent("[data-line-status]")).includes("until your phone is set up"), "the line's status says calls go to messages until the phone is set up");
  await owner.click('[data-setup-band] [data-open="dlg-phone"]');
  check(await owner.isVisible("#dlg-phone") && (await owner.getAttribute("#app-link", "data-value")).endsWith("/app"), "Set up my phone opens the steps with the phone app's address");
  await owner.click('#dlg-phone button[aria-label="Close"]');
  check(!(await owner.isVisible("#dlg-phone")), "Close shuts the window");
  // The website code shows only when asked for.
  check(!(await owner.isVisible("#button-code")), "the website code is hidden until asked for");
  await owner.click('[data-open="dlg-website"]');
  check(await owner.isVisible("#button-code"), "Add a Call button opens the code to paste");
  await owner.mouse.click(5, 5);
  check(!(await owner.isVisible("#dlg-website")), "a click outside closes the window");
  // A new account with a Basic code is Basic: no meeting room.
  await owner.click('[data-open="dlg-room"]');
  check(await owner.isVisible("[data-room-locked]") && (await owner.locator("#room-link").count()) === 0, "a Basic dashboard says the meeting room comes with Premium");
  await owner.keyboard.press("Escape");
  await owner.goto(BASE + "/dashboard/business");
  check((await owner.textContent("[data-plan]")).includes("Plan: Basic"), "a new account is on Basic");
  await owner.fill("[data-business-form] #business", "E2E Shop Two");
  await owner.click("[data-business-form] button[type=submit]");
  await owner.waitForSelector("text=Callers see the new name");
  check(await owner.inputValue("[data-business-form] #business") === "E2E Shop Two", "the Business tab changes the business's name");
  await owner.fill("[data-business-form] #business", "E2E Shop");
  await owner.click("[data-business-form] button[type=submit]");
  await owner.waitForSelector("text=Callers see the new name");
  check((await owner.request.get(BASE + `/room/${LINE}`)).status() === 404, "a Basic line has no meeting room page");
  const phoneSize = await newPage(390, 844);
  await phoneSize.context().addCookies(await owner.context().cookies());
  for (const p of ["/", "/dashboard", "/dashboard/menu", "/dashboard/voicemail", "/dashboard/business", "/app", `/call/${LINE}`]) {
    step = `phone width ${p}`;
    await phoneSize.goto(BASE + p);
    const wide = await phoneSize.evaluate(() => document.documentElement.scrollWidth);
    check(wide <= 390, `phone width: ${p} doesn't scroll sideways (${wide}px)`);
  }
  await phoneSize.goto(BASE + "/dashboard");
  check(await phoneSize.isVisible("[data-setup-band] >> text=One step left: make your phone ring."), "on a phone the next setup step sits at the top");
  const tabs = await phoneSize.$$eval('nav[aria-label="Dashboard"]', (ns) => ns.filter((n) => n.offsetWidth).map((n) => { const r = n.getBoundingClientRect(); return [n.children.length, Math.round(r.bottom), getComputedStyle(n).position]; }));
  check(JSON.stringify(tabs) === '[[4,844,"fixed"]]', `on a phone the four tabs sit at the bottom of the screen (${JSON.stringify(tabs)})`);
  await phoneSize.click('[data-open="dlg-website"]');
  const sheet = await phoneSize.locator("#dlg-website").boundingBox();
  check(sheet && sheet.width === 390 && Math.round(sheet.y + sheet.height) === 844, `on a phone the windows rise from the bottom (${JSON.stringify(sheet)})`);
  await phoneSize.screenshot({ path: `${SHOTS}/dashboard-website-phone.png` });
  await phoneSize.goto(BASE + "/dashboard/menu");
  await phoneSize.screenshot({ path: `${SHOTS}/menu-phone.png`, fullPage: true });
  // The phone app changes what callers hear itself: no trip to a computer.
  step = "app: what callers hear";
  await phoneSize.goto(BASE + "/app");
  await phoneSize.click('[role="tab"][data-tab="menu"]');
  await phoneSize.waitForSelector('[data-panel="menu"] [data-menu-form]');
  check(await phoneSize.isVisible('[data-panel="menu"] [name="voicemail_audio"] ~ .hr-head [data-rec-start]'), "the phone app can record the voicemail greeting");
  const appRows = phoneSize.locator('[data-panel="menu"] [data-choice]');
  await openRow(appRows.nth(1));
  await appRows.nth(1).locator("[name=label]").fill("Our hours");
  await phoneSize.waitForSelector('[data-panel="menu"] [data-save]:visible');
  await phoneSize.screenshot({ path: `${SHOTS}/app-hear-phone.png`, fullPage: true });
  const saveBox = await phoneSize.locator('[data-panel="menu"] [data-save]').boundingBox();
  const tabBox = await phoneSize.locator(".ap-tabs").boundingBox();
  check(saveBox && tabBox && saveBox.y + saveBox.height <= tabBox.y, `Save sits above the tabs (${Math.round(saveBox.y + saveBox.height)} <= ${Math.round(tabBox.y)})`);
  await phoneSize.click('[data-panel="menu"] [data-save-button]');
  await phoneSize.waitForSelector('[data-panel="menu"] >> text=Saved.');
  check(new URL(phoneSize.url()).pathname === "/app", "saving in the app stays in the app");
  await phoneSize.reload();
  await phoneSize.click('[role="tab"][data-tab="menu"]');
  check(/Our hours/.test(await appRows.nth(1).locator("[data-sum-label]").textContent()), "a change made in the app is saved");
  await phoneSize.click('[data-panel="menu"] button[data-tab="messages"]');
  check(await phoneSize.isVisible('[data-panel="messages"]'), "the card's Messages link opens the app's Messages");
  await phoneSize.click('[role="tab"][data-tab="menu"]');
  await openRow(appRows.nth(1));
  await appRows.nth(1).locator("[name=label]").fill("Opening hours");
  await phoneSize.click('[data-panel="menu"] [data-save-button]');
  await phoneSize.waitForSelector('[data-panel="menu"] >> text=Saved.');

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
  // The notification's picture is the business's logo cut round (sw.js), so
  // desktops that draw it square still show a circle. Headless Chromium can't
  // show notifications, so the worker's own code runs in a page.
  {
    const logo = await owner.getAttribute("[data-phone]", "data-me-logo");
    // bypassCSP: this test runs the worker's text as code, which pages may not do.
    const p = await (await browser.newContext({ viewport: { width: 800, height: 600 }, bypassCSP: true })).newPage();
    p.on("pageerror", (e) => consoleErrors.push(`${step}: ${e}`));
    await p.goto(BASE + "/login");
    const icons = await p.evaluate(async (logo) => {
      const shown = [];
      window.registration = { showNotification: (title, o) => { shown.push(o.icon); return Promise.resolve(); } };
      // The worker falls back to the plain picture after a second, and headless Chromium can take that long to draw.
      const worker = (await (await fetch("/sw.js")).text()).replace("ok(url); }, 1000)", "ok(url); }, 8000)");
      const show = new Function(worker + "; return show;")();
      await show({ icon: logo });
      await show({ icon: "/manifest.webmanifest" }); // not a picture
      const alpha = async (url) => {
        const img = new Image();
        img.src = url;
        await img.decode();
        const g = Object.assign(document.createElement("canvas"), { width: img.width, height: img.height }).getContext("2d");
        g.drawImage(img, 0, 0);
        return [g.getImageData(2, 2, 1, 1).data[3], g.getImageData(img.width / 2, img.height / 2, 1, 1).data[3]];
      };
      return Promise.all(shown.map(async (u) => ({ data: u.startsWith("data:image/png"), alpha: await alpha(u) })));
    }, logo);
    await p.context().close();
    const round = (i) => i && i.data && i.alpha[0] === 0 && i.alpha[1] === 255;
    check(/^\/logo\//.test(logo) && round(icons[0]), `a notification shows the logo round, see-through corners (${JSON.stringify(icons[0])})`);
    check(round(icons[1]), `a logo that can't be read gives the round minivoip mark (${JSON.stringify(icons[1])})`);
  }
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
  await caller.waitForLoadState("load");
  check(await loaded(caller, '[data-show="idle"] .cw-badge.has-logo img') === 1, "the caller sees the business's logo, not its initials");
  await caller.screenshot({ path: `${SHOTS}/call-logo.png` });
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
  check(r0.title === "Incoming call: E2E Shop" && r0.body === "Someone on your website wants to talk to you." && /^\/logo\/logo-/.test(r0.icon),
    `a ring names the business and carries its logo (${r0.title} / ${r0.body} / ${r0.icon})`);
  await owner.click('[data-action="accept"]');
  await caller.waitForSelector("text=On the line with E2E Shop", { timeout: 15000 });
  await caller.waitForTimeout(2500);
  const direct = await caller.evaluate(() => window.minivoipCallStats());
  check(direct && direct.bytesIn > 1000, `the call connects and the caller hears the phone (${direct && direct.route}, ${direct && direct.bytesIn} bytes)`);
  await caller.screenshot({ path: `${SHOTS}/call.png` });
  check((await caller.locator('[data-action="camera"], [data-action="board"]').count()) === 0 && (await owner.locator('[data-call-screen] [data-action="camera"]').count()) === 0,
    "a Basic line's call is voice only: no Camera or Whiteboard on either side");
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

  step = "basic room tab";
  await owner.click('[data-tab="room"]');
  check(await owner.isVisible("[data-room-locked]") && (await owner.locator("[data-room-open]").count()) === 0, "a Basic app's Room tab says what Premium adds");
  await owner.click('[data-tab="calls"]');

  // From here the account is Premium (what the admin's "Move to Premium"
  // does; the admin part below clicks that button for another account).
  const planDB = new DatabaseSync(DB);
  planDB.prepare("UPDATE account SET plan = 'premium' WHERE name = ?").run(EMAIL);
  planDB.close();

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
  check((await owner.textContent("[data-vm-count]")).trim() === "1", "the Messages tab counts the new message");

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
  check((await owner.textContent("[data-tab-count]")).startsWith("2"), "the Messages tab counts 2 new messages");
  check((await owner.textContent("[data-latest]")).includes("Message"), "Latest calls lists the messages");
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
  await setAvail(owner, "off");
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
  await setAvail(owner, "on");
  check(true, "the Available switch turns back on");

  // The Call button on a business's own website (w.js), pasted from the dashboard ----
  step = "website button";
  const dashHTML = await (await owner.request.get(BASE + "/dashboard")).text();
  const pasted = (dashHTML.match(/id="button-code"[^>]*value="([^"]*)"/) || dashHTML.match(/value="([^"]*w\.js[^"]*)"/) || [])[1] || "";
  const snippet = pasted.replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&#34;|&quot;/g, '"').replace(/&amp;/g, "&");
  check(snippet.includes(`/w.js" data-line="${LINE}"`), `the dashboard gives the line to paste (${snippet})`);
  const bizSite = createServer((req, res) => {
    res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    res.end(`<!doctype html><html><head><meta name="viewport" content="width=device-width"><title>E2E Shop</title>
      <style>body{margin:0;font:16px Georgia,serif;background:#f7f4ee;color:#23302a}h1{font-size:56px;margin:120px 72px 0}button{all:unset}</style></head>
      <body><h1>E2E Shop, a business website</h1><p style="margin:16px 72px">Its own styles must not touch the button.</p>
      <p style="margin:1600px 72px 400px"><a id="own-link" href="${BASE}/call/${LINE}" data-minivoip-call>Call us</a></p>${snippet}</body></html>`);
  });
  await new Promise((r) => bizSite.listen(0, "127.0.0.1", r));
  const BIZ = `http://127.0.0.1:${bizSite.address().port}/`;
  const biz = await newPage(1280, 860);
  const btnState = (p) => p.evaluate(() => { const m = document.querySelector("[data-minivoip]")?.shadowRoot?.querySelector(".mv"); return m && !m.hidden ? m.dataset.here : null; });
  await biz.goto(BIZ);
  check(await until(async () => (await btnState(biz)) === "true") && (await biz.locator(".btn").textContent()).includes("Talk to us now"),
    'the button on the website says "Talk to us now" while the owner is available');
  await biz.screenshot({ path: `${SHOTS}/button-here.png` });
  await setAvail(owner, "off");
  await biz.reload();
  check(await until(async () => (await btnState(biz)) === "false") && (await biz.locator(".btn").textContent()).includes("Leave us a message"),
    'the button says "Leave us a message" while the owner is off');
  await biz.screenshot({ path: `${SHOTS}/button-away.png` });
  const awayPage = await newPage(390, 760);
  await awayPage.goto(BASE + `/call/${LINE}`);
  check(await awayPage.isVisible("text=Away right now") && await awayPage.isVisible("text=Hear our menu"), "the call page says the owner is away, and the menu still answers");
  await awayPage.screenshot({ path: `${SHOTS}/call-away.png` });
  await setAvail(owner, "on");
  check(await until(async () => (await awayPage.evaluate(() => document.querySelector("[data-call]").dataset.here)) === "true", 40000),
    "an open call page sees the owner come back, without a reload");
  await awayPage.context().close();
  // A computer, on another site: the call opens in its own window, so the
  // microphone permission is ours and never the website's, and a real call
  // works in it. The site can't reach that window.
  await biz.reload();
  await until(async () => (await btnState(biz)) === "true");
  const [panel] = await Promise.all([biz.context().waitForEvent("page"), biz.click(".btn")]);
  panel.on("pageerror", (e) => consoleErrors.push(`${step}: ${e}`));
  await panel.waitForLoadState();
  check(panel.url().includes(`/call/${LINE}`) && biz.frames().length === 1, "a click opens the call page in its own window, not inside the website");
  check(await panel.evaluate(() => window.opener === null), "the website has no hold on that window");
  await panel.waitForSelector('[data-action="call"]:visible');
  await panel.screenshot({ path: `${SHOTS}/button-window.png` });
  await panel.click('[data-action="call"]:visible');
  await panel.waitForSelector('[data-key="1"]:visible');
  await panel.click('[data-key="1"]');
  await owner.waitForSelector('[data-action="accept"]:visible', { timeout: 15000 });
  await owner.click('[data-action="accept"]');
  check(await until(async () => (await panel.evaluate(() => document.querySelector("[data-call]").dataset.state)) === "talking", 15000),
    "a call from the website's button connects");
  await panel.click('[data-action="hangup"]:visible');
  await owner.waitForSelector("[data-call-screen]", { state: "hidden", timeout: 10000 });
  await panel.close();
  // Another site can't put the call window in a frame of its own.
  await biz.evaluate((src) => { const f = document.createElement("iframe"); f.id = "steal"; f.src = src; document.body.append(f); }, `${BASE}/call/${LINE}`);
  await biz.waitForTimeout(800);
  const stolen = biz.frames().find((f) => f !== biz.mainFrame());
  check(!stolen || !(await stolen.url()).includes("/call/") || !(await stolen.$("[data-call]").catch(() => null)), "another site can't frame the call window");
  await biz.evaluate(() => document.getElementById("steal").remove());
  // The site's own link opens it too, and the corner button steps aside
  // while that link is on screen.
  const aside = () => biz.evaluate(() => document.querySelector("[data-minivoip]").shadowRoot.querySelector(".mv").classList.contains("aside"));
  check(!(await aside()), "the corner button shows while the site's own call link is off screen");
  await biz.locator("#own-link").scrollIntoViewIfNeeded();
  check(await until(async () => (await aside()) && !(await biz.locator(".btn").isVisible())), "and steps aside while the link is on screen");
  const [ownWindow] = await Promise.all([biz.context().waitForEvent("page"), biz.click("#own-link")]);
  await ownWindow.waitForLoadState();
  check(ownWindow.url().includes(`/call/${LINE}`), "the site's own Call us link opens the same window");
  await ownWindow.close();
  await biz.evaluate(() => window.scrollTo(0, 0));
  check(await until(async () => !(await aside())), "the corner button comes back once the link is scrolled away");
  await biz.context().close();
  // A phone: the call page takes the whole screen.
  const bizPhone = await newPage(390, 844);
  await bizPhone.goto(BIZ);
  await until(async () => (await btnState(bizPhone)) === "true");
  await bizPhone.screenshot({ path: `${SHOTS}/button-phone.png` });
  await bizPhone.click(".btn");
  await bizPhone.waitForURL(new RegExp(`/call/${LINE}`));
  check(true, "on a phone, the button opens the call page full screen");
  await bizPhone.context().close();
  bizSite.close();

  // Meeting room ----------------------------------------------------------
  step = "meeting room links";
  await owner.goto(BASE + "/dashboard/business");
  check((await owner.textContent("[data-plan]")).includes("Plan: Premium"), "the dashboard says the account is on Premium");
  await owner.goto(BASE + "/dashboard");
  await owner.click('[data-open="dlg-room"]');
  check(await owner.isVisible("#room-link") && (await owner.getAttribute("#room-link", "data-value")).endsWith(`/room/${LINE}`), "the dashboard shows the meeting room link");
  await owner.keyboard.press("Escape");
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
  await host.waitForSelector(".rm-in:visible", { timeout: 10000 });
  await host.waitForSelector('[data-door] >> text=Ana Roy is waiting', { timeout: 5000 });
  await host.waitForTimeout(500);
  check(await guest.isVisible("text=Waiting for") && await guest.isHidden(".rm-in"), "opening the room doesn't let anyone in: the guest still waits");
  await host.screenshot({ path: `${SHOTS}/room-door.png` });
  await host.click('[data-door] [data-action="admit"]');
  await guest.waitForSelector(".rm-in:visible", { timeout: 10000 });
  check(true, "the host lets the guest in");
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
  await bo.waitForSelector("text=Waiting for", { timeout: 10000 });
  await host.waitForSelector("[data-door] >> text=Bo is waiting", { timeout: 5000 });
  check(await bo.isHidden(".rm-in"), "a third person waits too, while the owner is there");
  await host.click('[data-door] [data-action="admit"]');
  await bo.waitForSelector(".rm-in:visible", { timeout: 10000 });
  await bo.waitForFunction(() => /3 people/.test(document.querySelector("[data-people]").textContent), null, { timeout: 10000 });
  check(/3 people/.test(await host.textContent("[data-people]")), "and comes in when the host says yes");
  check(await guest.locator('[data-action="people"], [data-door]').count() === 0, "a guest has no door and can't remove anyone");
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

  step = "host declines and removes";
  const eve = await newPage();
  await eve.goto(BASE + `/room/${LINE}`);
  await eve.fill("[data-name]", "Eve");
  await eve.click('[data-action="join"]');
  await host.waitForSelector("[data-door] >> text=Eve is waiting", { timeout: 5000 });
  await host.click('[data-door] [data-action="deny"]');
  await eve.waitForSelector("text=didn't let you in", { timeout: 5000 });
  check(/2 people/.test(await host.textContent("[data-people]")), "the host declines someone: they never come in");
  await eve.click('[data-show="left dropped"] [data-action="join"]');
  await host.waitForSelector("[data-door] >> text=Eve is waiting", { timeout: 5000 });
  await host.click('[data-door] [data-action="admit"]');
  await eve.waitForSelector(".rm-in:visible", { timeout: 10000 });
  await host.waitForFunction(() => /3 people/.test(document.querySelector("[data-people]").textContent), null, { timeout: 5000 });
  await host.click('[data-action="people"]');
  await host.waitForSelector("[data-door-in] >> text=Eve");
  await host.screenshot({ path: `${SHOTS}/room-people.png` });
  await host.setViewportSize({ width: 390, height: 844 });
  check(await host.evaluate(() => document.documentElement.scrollWidth) <= 390, "phone width: the host's list doesn't scroll sideways");
  await host.screenshot({ path: `${SHOTS}/room-people-phone.png` });
  await host.setViewportSize({ width: 1280, height: 900 });
  await host.click('[data-door-in] [aria-label="Remove Eve"]');
  await eve.waitForSelector("text=removed you from the room", { timeout: 5000 });
  await host.waitForFunction(() => /2 people/.test(document.querySelector("[data-people]").textContent), null, { timeout: 5000 });
  check(await eve.isHidden(".rm-in"), "the host removes someone: they're out, and the others see one less");
  await eve.screenshot({ path: `${SHOTS}/room-removed.png` });
  await eve.context().close();
  await guest.click('.rm-controls [data-action="leave"]');
  await host.waitForFunction(() => /just you/.test(document.querySelector("[data-people]").textContent), null, { timeout: 5000 });
  await host.click('.rm-controls [data-action="leave"]');
  await host.close();

  // Promo codes (admin) -----------------------------------------------------
  step = "admin only";
  await owner.goto(BASE + "/dashboard");
  check((await owner.locator('a[href="/admin/promo"]').count()) === 0, "a normal user has no Admin link");
  check((await owner.request.get(BASE + "/admin/promo")).status() === 404, "a normal user can't open the admin page");
  const adminDB = new DatabaseSync(DB);
  adminDB.prepare("UPDATE user SET is_admin = 1 WHERE email = ?").run(EMAIL);
  adminDB.close();
  await owner.reload();
  check(!(await owner.isVisible('a[href="/admin/promo"]')), "minivoip's admin isn't among a customer's tabs");
  await owner.click("[data-account-button]:visible");
  await owner.click('#account-menu a[href="/admin/promo"]');
  await owner.waitForSelector("#code");
  check(true, "an admin reaches the promo code page from the dashboard");

  step = "create code";
  const CODE = `E2E-${stamp.toUpperCase()}`;
  await owner.fill("#code", CODE.toLowerCase());
  await owner.selectOption("#plan", "premium");
  await owner.fill("#max_uses", "1");
  await owner.click("main button[type=submit]");
  await owner.waitForSelector(`text=${CODE} is ready`);
  check(await owner.isVisible(`[data-promo="${CODE}"] >> text=0 of 1 used`), "the new code is listed, unused");
  check(await owner.isVisible(`[data-promo="${CODE}"] [data-plan-badge="premium"]`), "the code says it gives Premium");
  await owner.fill("#code", CODE);
  await owner.click("main button[type=submit]");
  check(await owner.waitForSelector("text=already exists", { timeout: 5000 }).then(() => true, () => false), "the same code can't be made twice");

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
  await friend.goto(BASE + "/dashboard/business");
  check((await friend.textContent("[data-plan]")).includes(`Plan: Premium, free forever with the code ${CODE}`), "a friend who used the code gets its plan, Premium, free forever");
  check((await (await friend.request.get(BASE + "/dashboard")).text()).includes('id="room-link"'), "the friend's Premium dashboard has a meeting room");

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
  check(await owner.isVisible(`[data-premium-request="${PREMIUM_EMAIL}"]`), `the admin page lists who asked for Premium on the landing page`);
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

  step = "switch plan";
  const row = `[data-account="${FRIEND}"]`;
  check(await owner.isVisible(`${row} [data-plan-badge="premium"]`) && await owner.isVisible(`${row} >> text=${CODE} (revoked)`), "the Accounts list shows the friend on Premium, with their code");
  await owner.click(`${row} button[type=submit]`);
  await owner.waitForSelector(`text=${FRIEND} is on Basic now.`);
  check(await owner.isVisible(`${row} [data-plan-badge="basic"]`), "Move to Basic switches the friend's plan");
  await friend.reload();
  check((await friend.textContent("[data-plan]")).includes("Plan: Basic") && (await (await friend.request.get(BASE + "/dashboard")).text()).includes("data-room-locked"), "the friend's dashboard is Basic now: no meeting room");
  await owner.click(`${row} button[type=submit]`);
  await owner.waitForSelector(`text=${FRIEND} is on Premium now.`);
  check(await owner.isVisible(`${row} [data-plan-badge="premium"]`), "and Move to Premium switches it back");

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
  step = "dashboard";
  await owner.goto(BASE + "/dashboard");
  {
    // Only the page's own scripts: no htmx, component bundle or crop tool until a logo is picked.
    const extra = await owner.evaluate(() => performance.getEntriesByType("resource").map((e) => e.name).filter((n) => /htmx|shadcn-templ-|cropper/.test(n)));
    check(extra.length === 0, `the dashboard loads no scripts it doesn't use (${extra.join(", ")})`);
    // The link and the code to paste show whole, and Copy says it worked.
    await owner.click('[data-open="dlg-website"]');
    const whole = await owner.evaluate(() => ["call-link", "button-code"].map((id) => { const e = document.getElementById(id); return e.dataset.value.endsWith(e.textContent.trim()) && e.scrollWidth <= e.clientWidth; }));
    check(whole.every(Boolean), `the call link and the code to paste show whole (${whole})`);
    await owner.context().grantPermissions(["clipboard-read", "clipboard-write"], { origin: BASE });
    await owner.click('[data-copy="button-code"]');
    await owner.waitForFunction(() => document.querySelector("[data-copy-status]").textContent === "Copied", null, { timeout: 3000 }).catch(() => {});
    const copied = await owner.evaluate(async () => [document.querySelector("[data-copy-status]").textContent, !document.querySelector('[data-copy="button-code"] [data-copied-icon]').hidden, await navigator.clipboard.readText()]);
    check(copied[0] === "Copied" && copied[1] && copied[2].includes("/w.js") && copied[2].includes(`data-line="${LINE}"`), `Copy puts the code on the clipboard, shows a check and tells screen readers (${copied[0]}, ${copied[1]})`);
    await owner.keyboard.press("Escape");
    // The keyboard's place: a 2px outline, drawn inside the menu's scrolling strip.
    await owner.focus('aside nav a[href="/dashboard/menu"]');
    await owner.keyboard.press("Shift+Tab"); await owner.keyboard.press("Tab");
    const ring = await owner.evaluate(() => { const c = getComputedStyle(document.activeElement); return [document.activeElement.getAttribute("href"), c.outlineStyle, c.outlineWidth, c.outlineOffset]; });
    check(ring.join() === "/dashboard/menu,solid,2px,-2px", `the keyboard's place shows as a clear outline in the menu (${ring})`);
    // Thumb-sized buttons on a phone.
    await owner.setViewportSize({ width: 390, height: 844 });
    await owner.waitForFunction(() => innerWidth === 390);
    await owner.waitForTimeout(400); // the buttons' height animates (transition-all)
    const short = await owner.evaluate(() => [...document.querySelectorAll("main [data-slot=button], header button, nav a")].filter((e) => e.offsetParent && e.getBoundingClientRect().height < 44).map((e) => e.textContent.trim()));
    check(short.length === 0, `on a phone the dashboard's buttons are 44px tall (${short.join(", ")})`);
    await owner.screenshot({ path: `${SHOTS}/dashboard-phone.png`, fullPage: true });
    await owner.setViewportSize({ width: 1280, height: 900 });
  }
  step = "logo";
  await owner.goto(BASE + "/dashboard/business");
  await owner.click('form[action="/dashboard/logo/remove"] button');
  await owner.waitForSelector("text=Callers see your initials instead");
  check((await owner.locator("[data-logo-badge]").count()) === 0, "Remove the logo goes back to the initials");
  const pickBox = await owner.locator("#logo-file").boundingBox();
  check(pickBox.width <= 1 && (await owner.textContent("[data-logo-pick]")).includes("Add my logo"), `the logo is one Add my logo button, without the browser's file box (${pickBox.width}px)`);
  // Cancel in the crop window: nothing is sent.
  const [chooser] = await Promise.all([owner.waitForEvent("filechooser"), owner.click("[data-logo-pick]")]);
  await chooser.setFiles("assets/brand/apple-touch-icon.png");
  await owner.waitForSelector("[data-logo-crop] .cropper-crop-box", { timeout: 10000 });
  await owner.click('[data-crop="cancel"]');
  const afterCancel = [await owner.inputValue("#logo-file"), await owner.locator("[data-logo-badge]").count(), owner.url()];
  check(afterCancel[0] === "" && afterCancel[1] === 0, `Cancel in the crop window leaves the logo as it was (${JSON.stringify(afterCancel)})`);
  // Pick, frame, Use this logo: it goes up right away.
  await owner.setInputFiles("#logo-file", "assets/brand/apple-touch-icon.png");
  await owner.waitForSelector("[data-logo-crop] .cropper-crop-box", { timeout: 10000 });
  await owner.click('[data-crop="fit"]');
  await owner.click('[data-crop="use"]');
  await owner.waitForSelector("text=Callers see your new logo right away");
  check(await loaded(owner, "[data-logo-badge] img") === 1, "Add my logo puts a new one on the dashboard");
  await owner.screenshot({ path: `${SHOTS}/dashboard-logo.png` });
  await owner.click('form[action="/dashboard/logo/remove"] button');
  await owner.waitForSelector("text=Callers see your initials instead");
  step = "sign up while logged in";
  await owner.goto(BASE + "/signup");
  check((await owner.textContent("[data-signed-in]")).includes(EMAIL), "the sign up page says who is logged in, instead of jumping to the dashboard");
  await owner.click("text=Log out and create another account");
  await owner.waitForSelector("form[action='/signup'] #email");
  check(new URL(owner.url()).pathname === "/signup", "Log out and create another account opens a fresh sign up");
  await owner.goto(BASE + "/login");
  await owner.fill("#email", EMAIL);
  await owner.fill("#password", "e2e-password");
  await owner.click("main button[type=submit]");
  await owner.waitForURL(/\/dashboard/);

  // Limits (web/limits.go). Visitors on this machine aren't limited, so each
  // check comes from a made-up visitor address in X-Forwarded-For, as Caddy
  // would send it.
  step = "limits";
  {
    // A different network each: IPv6 visitors are counted by their /64.
    const visitorAddr = () => `2001:db8:${randomBytes(6).toString("hex").match(/.{4}/g).join(":")}::1`;
    const as = async (addr) => {
      const ctx = await browser.newContext({ extraHTTPHeaders: { "X-Forwarded-For": addr } });
      return { ctx, page: await ctx.newPage() };
    };
    // Log in: 5 wrong passwords, then that visitor waits, even with the right one.
    const guesser = await as(visitorAddr());
    const logIn = (who, password) => who.ctx.request.post(BASE + "/login", { form: { email: EMAIL, password }, maxRedirects: 0 });
    const wrong = [];
    for (let i = 0; i < 5; i++) wrong.push((await logIn(guesser, "not-the-password")).status());
    const blocked = await logIn(guesser, "e2e-password");
    check(wrong.every((s) => s === 422) && blocked.status() === 429 && (await blocked.text()).includes("Too many wrong passwords"),
      `after 5 wrong passwords that visitor waits 15 minutes (${wrong.join(",")} then ${blocked.status()})`);
    const someoneElse = await as(visitorAddr());
    check((await logIn(someoneElse, "e2e-password")).status() === 303, "another visitor still logs in");
    // Sign up: 3 new accounts an hour from one visitor.
    const maker = await as(visitorAddr());
    const made = [];
    for (let i = 1; i <= 4; i++) {
      made.push((await maker.ctx.request.post(BASE + "/signup", { form: { email: `e2e-${stamp}-limit${i}@example.com`, password: "e2e-password", promo: JOIN_CODE }, maxRedirects: 0 })).status());
    }
    check(made.join(",") === "303,303,303,429", `a 4th account in an hour from one visitor is refused (${made.join(",")})`);
    // Guessing promo codes: 10 refused tries an hour from one visitor.
    const codeGuesser = await as(visitorAddr());
    const guesses = [];
    for (let i = 1; i <= 11; i++) {
      guesses.push((await codeGuesser.ctx.request.post(BASE + "/signup", { form: { email: `e2e-${stamp}-guess@example.com`, password: "e2e-password", promo: `E2E-GUESS-${i}` }, maxRedirects: 0 })).status());
    }
    check(guesses.slice(0, 10).every((s) => s === 422) && guesses[10] === 429, `promo codes can't be guessed: the 11th try is refused (${guesses[10]})`);
    await codeGuesser.ctx.close();
    // "Get Premium": 3 emails an hour from one visitor.
    const asker = await as(visitorAddr());
    const asked = [];
    for (let i = 1; i <= 4; i++) {
      asked.push((await asker.ctx.request.post(BASE + "/premium", { form: { email: `e2e-${stamp}-ask${i}@example.com` }, maxRedirects: 0 })).status());
    }
    check(asked.join(",") === "303,303,303,429", `a 4th "Get Premium" in an hour from one visitor is refused (${asked.join(",")})`);
    // Calls: 3 rings in 10 minutes, then voicemail without ringing.
    const ringer = await as(visitorAddr());
    await ringer.page.goto(BASE + "/login");
    const ask = (m) => ringer.page.evaluate(({ line, m }) => new Promise((ok, fail) => {
      let answer;
      const ws = new WebSocket(location.origin.replace(/^http/, "ws") + "/ws?role=visitor&line=" + line);
      ws.onopen = () => ws.send(JSON.stringify(m));
      ws.onmessage = (e) => {
        answer = JSON.parse(e.data);
        if (answer.type === "ringing") ws.send(JSON.stringify({ type: "hangup", id: answer.id }));
        ws.close();
      };
      ws.onclose = () => (answer ? ok(answer) : fail(new Error("the socket closed without an answer")));
      setTimeout(() => fail(new Error("no answer")), 5000);
    }), { line: LINE, m });
    const rings = [];
    for (let i = 0; i < 4; i++) rings.push((await ask({ type: "call", option: "Limits" })).type);
    check(rings.join(",") === "ringing,ringing,ringing,no-answer", `a 4th call in 10 minutes goes to voicemail without ringing (${rings.join(",")})`);
    // Messages: 3 an hour from one visitor. Four passes first, then four uploads.
    const passes = [];
    for (let i = 0; i < 4; i++) passes.push((await ask({ type: "voicemail", option: "Limits" })).token);
    const sent = [];
    for (const token of passes) {
      sent.push((await ringer.ctx.request.post(BASE + "/api/voicemail", { headers: { "X-Voicemail-Pass": token }, multipart: { duration_ms: "1000", audio: { name: "m.webm", mimeType: "audio/webm", buffer: Buffer.from("e2e message") } } })).status());
    }
    check(sent.join(",") === "204,204,204,429", `a 4th message in an hour is refused (${sent.join(",")})`);
    // A recording of a made-up type is refused: it could open as a web page.
    const hostile = await as(visitorAddr());
    await hostile.page.goto(BASE + "/login");
    const hostilePass = await hostile.page.evaluate((line) => new Promise((ok) => {
      const ws = new WebSocket(location.origin.replace(/^http/, "ws") + "/ws?role=visitor&line=" + line);
      ws.onopen = () => ws.send(JSON.stringify({ type: "voicemail" }));
      ws.onmessage = (e) => { ok(JSON.parse(e.data).token); ws.close(); };
    }), LINE);
    const hostileUp = await hostile.ctx.request.post(BASE + "/api/voicemail", { headers: { "X-Voicemail-Pass": hostilePass }, multipart: { duration_ms: "1000", audio: { name: "m", mimeType: "audio/x,text/html", buffer: Buffer.from("<script>alert(1)</script>") } } });
    check(hostileUp.status() === 415, `a recording of a made-up type is refused (${hostileUp.status()})`);
    await hostile.ctx.close();
    const more = await ask({ type: "voicemail", option: "Limits" });
    check(more.type === "limited" && !more.token, "and the call page is told to try later, with no voicemail pass");
    for (const v of [guesser, someoneElse, maker, ringer]) await v.ctx.close();
  }

  // Change password, from the account menu: the other devices log out.
  step = "change password";
  const otherDevice = await newPage();
  await otherDevice.goto(BASE + "/login");
  await otherDevice.fill("#email", EMAIL);
  await otherDevice.fill("#password", "e2e-password");
  await otherDevice.click("main button[type=submit]");
  await otherDevice.waitForURL(/\/dashboard/);
  await owner.goto(BASE + "/dashboard");
  await owner.click("[data-account-button]:visible");
  await owner.click('#account-menu a[href="/dashboard/password"]');
  await owner.waitForSelector("#current");
  await owner.fill("#current", "not-the-password");
  await owner.fill("#new", "e2e-new-password");
  await owner.click("main form[action='/dashboard/password'] button[type=submit]");
  await owner.waitForSelector("text=current password isn't right");
  check(true, "a wrong current password is refused");
  await owner.fill("#current", "e2e-password");
  await owner.fill("#new", "e2e-new-password");
  await owner.screenshot({ path: `${SHOTS}/password.png`, fullPage: true });
  await owner.click("main form[action='/dashboard/password'] button[type=submit]");
  await owner.waitForSelector("[data-password-saved]");
  check(true, "the password changes, and this device stays logged in");
  await otherDevice.goto(BASE + "/dashboard");
  check(new URL(otherDevice.url()).pathname === "/login", "the other device is logged out");
  await otherDevice.context().close();
  // An app left open on a device that logged out goes to the login screen.
  {
    const phone = await newPage(390, 844);
    await phone.goto(BASE + "/login?next=/app");
    await phone.fill("#email", EMAIL);
    await phone.fill("#password", "e2e-new-password");
    await phone.click("main button[type=submit]");
    await phone.waitForSelector("[data-phone]");
    await phone.waitForTimeout(500);
    const thisLogin = await phone.context().cookies();
    const outside = await newPage();
    await outside.context().addCookies(thisLogin);
    await outside.goto(BASE + "/dashboard");
    await outside.click("[data-account-button]:visible");
    await outside.click("#account-menu button[type=submit]");
    await phone.waitForURL(/\/login\?next=\/app/, { timeout: 15000 });
    check(true, "logging out closes that login's open app, which goes to the login screen");
    await phone.context().close();
    await outside.context().close();
  }

  step = "security headers";
  const headers = (await owner.request.get(BASE + "/dashboard")).headers();
  check(headers["x-content-type-options"] === "nosniff" && headers["x-frame-options"] === "DENY" && /frame-ancestors 'none'/.test(headers["content-security-policy"] || ""), "pages carry the browser's security rules");
  check((await owner.request.get(BASE + `/call/${LINE}`)).headers()["x-frame-options"] === "SAMEORIGIN", "the call window only sits in our own pages");
  check(/script-src 'self';/.test(headers["content-security-policy"] || ""), "scripts only come from this site, none inline");
  const anonICE = await (await browser.newContext()).request.get(BASE + "/api/ice");
  check(!(await anonICE.text()).includes("credential"), "a stranger gets no relay login");
  check((await (await owner.request.get(BASE + "/api/ice")).text()).includes("credential"), "a logged-in owner does");

  step = "log out";
  await owner.goto(BASE + "/dashboard");
  await owner.click("[data-account-button]:visible");
  await owner.click("#account-menu button[type=submit]");
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
    UNION SELECT l.voicemail_audio FROM line l JOIN account a ON a.id = l.account_id
      WHERE a.name LIKE 'e2e-%@example.com'
    UNION SELECT o.say_audio FROM menu_option o JOIN line l ON l.id = o.line_id JOIN account a ON a.id = l.account_id
      WHERE a.name LIKE 'e2e-%@example.com'`).all();
  for (const p of prompts) if (p.f) rmSync(join(dirname(DB), "prompts", p.f), { force: true });
  const logos = db.prepare("SELECT l.logo AS f FROM line l JOIN account a ON a.id = l.account_id WHERE a.name LIKE 'e2e-%@example.com'").all();
  for (const p of logos) if (p.f) rmSync(join(dirname(DB), "logos", p.f), { force: true });
  db.prepare("DELETE FROM account WHERE name LIKE 'e2e-%@example.com'").run();
  db.prepare("DELETE FROM push_log WHERE line LIKE 'e2e-%'").run();
  db.prepare("UPDATE account SET promo_code = NULL WHERE promo_code LIKE 'E2E-%'").run();
  db.prepare("DELETE FROM promo_code WHERE code LIKE 'E2E-%'").run();
  db.prepare("DELETE FROM premium_request WHERE email LIKE 'e2e-%@example.com'").run();
  db.close();
}

for (const e of consoleErrors) failures.push(`browser error: ${e}`);
console.log(failures.length ? `\n${failures.length} problem(s):\n- ${failures.join("\n- ")}` : "\nAll good.");
process.exit(failures.length ? 1 : 0);
