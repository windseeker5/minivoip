// Checks the visitor's call window (/call/<line>) without the Go server:
// renders it with sample lines (pages/call_test.go, TestCallPreview), serves
// those files and the real assets, and clicks through every step in headless
// Chromium with a fake call socket: the menu written out, a "play" answer, a
// page to open, ringing, "Leave a message now", recording, start over, send,
// a failed send and its retry, no answer, microphone blocked, phone, computer
// and a phone zoomed to 200%. Nothing reaches a real phone. The real calls
// are in site.mjs.
// Run from the repo root: node tests/e2e/call.mjs
import { createRequire } from "node:module";
import { createServer } from "node:http";
import { execFileSync } from "node:child_process";
import { readFileSync, readdirSync, existsSync, mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { join, extname } from "node:path";
import { homedir, tmpdir } from "node:os";

const REPO = process.cwd();
const SHOTS = "tests/e2e/shots";
mkdirSync(SHOTS, { recursive: true });
const PRE = mkdtempSync(join(tmpdir(), "minivoip-call-"));
execFileSync("go", ["test", "./pages", "-run", "TestCallPreview", "-count=1"], { env: { ...process.env, MINIVOIP_PREVIEW: PRE }, stdio: "inherit" });
const npx = join(homedir(), ".npm/_npx");
const pw = readdirSync(npx).map((d) => join(npx, d, "node_modules/playwright-core")).find(existsSync);
const { chromium } = createRequire(import.meta.url)(pw);
const cache = join(homedir(), ".cache/ms-playwright");
const shell = readdirSync(cache).filter((d) => d.startsWith("chromium_headless_shell-")).sort().pop();

const types = { ".css": "text/css", ".js": "text/javascript", ".svg": "image/svg+xml", ".png": "image/png" };
const pages = { "/call/auberge": "call.html", "/call/omt": "call-one.html", "/call/many": "call-many.html" };
const uploads = [];
let vmStatus = 204;
const server = createServer((req, res) => {
  const u = new URL(req.url, "http://x");
  const chunks = [];
  req.on("data", (c) => chunks.push(c));
  req.on("end", () => {
    const send = (code, type, data) => { res.writeHead(code, { "Content-Type": type }); res.end(data); };
    if (pages[u.pathname]) return send(200, "text/html", readFileSync(join(PRE, pages[u.pathname])));
    if (u.pathname === "/api/voicemail") {
      uploads.push({ status: vmStatus, bytes: Buffer.concat(chunks).length });
      res.writeHead(vmStatus);
      return res.end();
    }
    if (u.pathname === "/api/ice") return send(200, "application/json", '{"iceServers":[]}');
    if (u.pathname.startsWith("/assets/")) {
      const f = join(REPO, u.pathname);
      if (existsSync(f)) return send(200, types[extname(f)] || "application/octet-stream", readFileSync(f));
    }
    send(404, "text/plain", "not found");
  });
});
await new Promise((r) => server.listen(0, "127.0.0.1", r));
const BASE = `http://127.0.0.1:${server.address().port}`;

const browser = await chromium.launch({
  executablePath: join(cache, shell, "chrome-headless-shell-linux64/chrome-headless-shell"),
  args: ["--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream", "--autoplay-policy=no-user-gesture-required"],
});
const failures = [];
const check = (ok, what) => { console.log((ok ? "ok   " : "FAIL ") + what); if (!ok) failures.push(what); };
const errors = [];

// open loads a call window with a fake call socket: "call" gets "ringing",
// "voicemail" gets a voicemail pass; the test sends the rest.
async function open(w, h, path, opts = {}) {
  const ctx = await browser.newContext({ viewport: { width: w, height: h }, deviceScaleFactor: opts.scale || 1, permissions: opts.noMic ? [] : ["microphone"] });
  // A fake browser voice, 30 ms a letter with word boundaries, so the timing
  // is the same on every machine.
  await ctx.addInitScript(() => {
    let cur = null;
    Object.defineProperty(window, "speechSynthesis", { configurable: true, value: {
      speak(u) {
        cur = u;
        if (u.onstart) u.onstart({});
        for (const m of u.text.matchAll(/\S+/g)) setTimeout(() => { if (cur === u && u.onboundary) u.onboundary({ charIndex: m.index }); }, m.index * 30);
        setTimeout(() => { if (cur === u) { cur = null; if (u.onend) u.onend({}); } }, u.text.length * 30 + 100);
      },
      cancel() { if (cur) { const u = cur; cur = null; if (u.onerror) u.onerror({}); } },
    } });
  });
  if (opts.noMic) await ctx.addInitScript(() => { navigator.mediaDevices.getUserMedia = () => Promise.reject(new DOMException("denied", "NotAllowedError")); });
  const sock = { sent: [], ws: null };
  await ctx.routeWebSocket(/\/ws/, (ws) => {
    sock.ws = ws;
    ws.onMessage((m) => {
      const msg = JSON.parse(m);
      sock.sent.push(msg);
      if (msg.type === "call") ws.send(JSON.stringify({ type: "ringing", id: "c1" }));
      if (msg.type === "voicemail") ws.send(JSON.stringify({ type: "voicemail", token: "t1" }));
    });
  });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (m) => { if (m.type() === "error" && !/favicon|Failed to load resource|fonts/.test(m.text())) errors.push(m.text()); });
  await page.goto(BASE + path, { referer: opts.referer });
  return { page, sock, ctx };
}
const settle = (p) => p.waitForTimeout(600); // the view fades in
const until = async (ok, ms = 3000) => { for (const end = Date.now() + ms; Date.now() < end; await new Promise((r) => setTimeout(r, 50))) if (ok()) return true; return ok(); };
const state = (p) => p.evaluate(() => document.querySelector("[data-call]").dataset.state);
const waitState = (p, s, timeout = 20000) => p.waitForFunction((s) => document.querySelector("[data-call]").dataset.state === s, s, { timeout });
const overflow = (p) => p.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
const visibleMute = (p) => p.locator('[data-action="mute"]').isVisible();
// The bounding boxes of the visible buttons that stick out of the window.
const clipped = (p) => p.evaluate(() => [...document.querySelectorAll("button, a")].filter((b) => b.offsetParent).filter((b) => {
  const r = b.getBoundingClientRect();
  return r.left < -1 || r.right > window.innerWidth + 1;
}).map((b) => b.textContent.trim() || b.getAttribute("aria-label")));

for (const [name, w, h] of [["phone", 390, 844], ["desk", 1280, 860]]) {
  const { page, sock, ctx } = await open(w, h, "/call/auberge", { referer: "https://auberge.example/rooms" });
  check(await overflow(page) <= 0, `${name}: no sideways scroll`);
  check(await page.getAttribute('[data-show="idle"] [data-back]', "href") === "https://auberge.example/rooms", `${name}: "Back to the website" goes to the page the visitor came from`);
  check(await page.isVisible("text=your browser will ask to use your microphone"), `${name}: the start says the microphone will be asked for`);
  await settle(page);
  await page.screenshot({ path: `${SHOTS}/call-${name}-1-start.png` });

  await page.click('[data-action="call"]:visible');
  await waitState(page, "menu");
  await page.waitForTimeout(1500);
  const words = await page.locator("[data-words] span").count();
  check(words > 10, `${name}: the menu's words are written out (${words} words)`);
  check(await page.locator("[data-words] span.said").count() > 0, `${name}: words light up as the voice goes`);
  check(!(await visibleMute(page)), `${name}: no Mute before the call`);
  check(!(await page.isVisible("text=Connected")), `${name}: the menu doesn't claim someone answered`);
  await settle(page);
  await page.screenshot({ path: `${SHOTS}/call-${name}-2-menu.png` });

  // A "play" choice: its answer is written out, then the menu stays.
  await page.click('[data-key="2"]');
  check(/Hours and directions/.test(await page.textContent("[data-menu-status]")), `${name}: the chosen answer's name shows`);
  check(/chemin du Lac/.test(await page.textContent("[data-words]")), `${name}: the answer is written out`);
  await page.waitForFunction(() => /Anything else/.test(document.querySelector("[data-menu-status]").textContent), null, { timeout: 15000 });
  check(await state(page) === "menu", `${name}: after an answer, the menu stays for another choice`);
  await settle(page);
  await page.screenshot({ path: `${SHOTS}/call-${name}-3-answer.png` });

  // Ring: hold message, then ringing with the countdown.
  await page.click('[data-key="1"]');
  await waitState(page, "ringing");
  check(await until(() => sock.sent.some((m) => m.type === "call" && m.option === "Book a room")), `${name}: the call is placed with the choice`);
  check(/^0:(30|29|28)$/.test(await page.textContent("[data-ring-count]")), `${name}: the countdown to voicemail shows (${await page.textContent("[data-ring-count]")})`);
  check(/You chose: Book a room/.test(await page.textContent("[data-ring-choice]")), `${name}: ringing says what was chosen`);
  check(!(await visibleMute(page)), `${name}: no Mute while ringing`);
  await page.waitForTimeout(600);
  await settle(page);
  await page.screenshot({ path: `${SHOTS}/call-${name}-4-ringing.png` });

  // Leave a message now: hang up the ringing, get a voicemail pass.
  await page.click('[data-action="leave-message"]');
  await waitState(page, "voicemail");
  const after = sock.sent.slice(-2).map((m) => m.type).join(",");
  check(after === "hangup,voicemail", `${name}: "Leave a message now" stops the ringing and asks for voicemail (${after})`);
  check(/name and phone number/.test(await page.textContent("[data-vm-words]")), `${name}: the voicemail asks for a name and number`);
  check(await page.isDisabled('[data-action="send-message"]'), `${name}: Send waits for the recording`);
  await waitState(page, "recording", 15000);
  await page.waitForTimeout(2200);
  check(/^0:0[12] of 2:00$/.test(await page.textContent("[data-rec-time]")), `${name}: the recording shows its time and the 2 minute limit (${await page.textContent("[data-rec-time]")})`);
  await settle(page);
  await page.screenshot({ path: `${SHOTS}/call-${name}-5-recording.png` });
  await page.click('[data-action="restart-message"]');
  await page.waitForTimeout(300);
  check(await state(page) === "recording" && /^0:00/.test(await page.textContent("[data-rec-time]")), `${name}: Start over records again from 0:00`);
  await page.waitForTimeout(1200);
  await page.click('[data-action="send-message"]');
  await waitState(page, "ended", 10000);
  check(/Message sent to Marie/.test(await page.textContent("[data-end-title]")), `${name}: the end says the message was sent`);
  check(uploads.length > 0 && uploads[uploads.length - 1].status === 204, `${name}: the message was uploaded`);
  check(await page.isVisible('[data-show="ended"] [data-back]'), `${name}: the end offers "Back to the website"`);
  await settle(page);
  await page.screenshot({ path: `${SHOTS}/call-${name}-6-sent.png` });

  // A page to open: a real link, so no popup blocker stops it.
  await page.click('[data-action="call"]:visible');
  await waitState(page, "menu");
  await page.click('[data-key="3"]');
  await waitState(page, "ended", 15000);
  check(await page.getAttribute("[data-end-link]", "href") === "https://example.com/rooms" && await page.isVisible("[data-end-link]"), `${name}: a link choice ends with an "Open the page" button`);
  await settle(page);
  await page.screenshot({ path: `${SHOTS}/call-${name}-7-link.png` });

  // No answer: straight to voicemail; a failed send keeps the message.
  await page.click('[data-action="call"]:visible');
  await waitState(page, "menu");
  await page.click('[data-key="1"]');
  await waitState(page, "ringing");
  sock.ws.send(JSON.stringify({ type: "no-answer", id: "c1", token: "t2" }));
  await waitState(page, "voicemail");
  check(/can't answer right now/.test(await page.textContent("[data-vm-words]")), `${name}: no answer says so and offers a message`);
  await waitState(page, "recording", 15000);
  await page.waitForTimeout(1200);
  vmStatus = 500;
  await page.click('[data-action="send-message"]');
  await waitState(page, "ended", 10000);
  check(await page.isVisible('[data-action="retry-send"]') && /wasn't sent/.test(await page.textContent("[data-end-title]")), `${name}: a failed send says so and offers to try again`);
  await settle(page);
  await page.screenshot({ path: `${SHOTS}/call-${name}-8-unsent.png` });
  vmStatus = 204;
  await page.click('[data-action="retry-send"]');
  await page.waitForFunction(() => /Message sent/.test(document.querySelector("[data-end-title]").textContent), null, { timeout: 10000 });
  check(true, `${name}: "Try sending again" sends the same message`);

  // Answered: connecting, then the talking view with Mute.
  await page.click('[data-action="call"]:visible');
  await waitState(page, "menu");
  await page.click('[data-key="1"]');
  await waitState(page, "ringing");
  sock.ws.send(JSON.stringify({ type: "accepted", id: "c1" }));
  await waitState(page, "connecting");
  check(/Connecting/.test(await page.textContent("[data-ring-title]")) && !(await page.isVisible("[data-ring-box]")), `${name}: answering shows "Connecting…"`);
  // The fake socket can't carry a real connection: show the talking view as call.js does.
  await page.evaluate(() => {
    const r = document.querySelector("[data-call]");
    r.dataset.state = "talking";
    r.querySelectorAll("[data-show]").forEach((e) => (e.hidden = e.dataset.show.split(" ").indexOf("talking") === -1));
    r.querySelector("[data-timer]").textContent = "2:14";
  });
  check(await visibleMute(page), `${name}: Mute shows during the call`);
  await page.waitForTimeout(500);
  await settle(page);
  await page.screenshot({ path: `${SHOTS}/call-${name}-9-talking.png` });
  await page.keyboard.press("Escape");
  await waitState(page, "ended");
  check(sock.sent[sock.sent.length - 1].type === "hangup", `${name}: Escape hangs up`);
  await settle(page);
  await page.screenshot({ path: `${SHOTS}/call-${name}-10-ended.png` });

  // Ctrl+2 is the browser's (switch tab), not a choice.
  await page.click('[data-action="call"]:visible');
  await waitState(page, "menu");
  await page.keyboard.press("Control+2");
  await page.waitForTimeout(200);
  check(!/Hours/.test(await page.textContent("[data-menu-status]")), `${name}: Ctrl+2 doesn't pick a choice`);
  await page.keyboard.press("2");
  check(/Hours and directions/.test(await page.textContent("[data-menu-status]")), `${name}: the 2 key picks choice 2`);
  check(await overflow(page) <= 0, `${name}: no sideways scroll on the menu`);
  await ctx.close();
}

// Microphone blocked: how to allow it, and Try again.
{
  const { page, ctx } = await open(390, 844, "/call/auberge", { noMic: true });
  await page.click('[data-action="call"]:visible');
  await waitState(page, "menu");
  await page.click('[data-key="1"]');
  await page.waitForSelector("[data-mic-help]:visible", { timeout: 5000 });
  check(await state(page) === "menu" && await page.isVisible('[data-action="retry-mic"]'), "phone: a blocked microphone says how to allow it, with Try again");
  await settle(page);
  await page.screenshot({ path: `${SHOTS}/call-phone-mic-blocked.png` });
  await ctx.close();
}

// Without a referrer (QR code, typed link): no Back button.
{
  const { page, ctx } = await open(390, 844, "/call/omt");
  check(!(await page.isVisible("[data-back]")), "phone: no Back button when nothing sent the visitor here");
  check(await page.isVisible("text=Live call") === false, "phone: the start doesn't show choices");
  await page.click('[data-action="call"]:visible');
  await waitState(page, "menu");
  await page.waitForTimeout(800);
  await settle(page);
  await page.screenshot({ path: `${SHOTS}/call-phone-one-choice.png` });
  await ctx.close();
}

// A long name and seven choices; a phone zoomed to 200% (195 px wide).
for (const [name, w, h, scale] of [["many", 390, 844, 1], ["zoom", 195, 422, 2]]) {
  const { page, ctx } = await open(w, h, name === "zoom" ? "/call/auberge" : "/call/many", { scale });
  check(await overflow(page) <= 0 && !(await clipped(page)).length, `${name}: start fits (${await clipped(page)})`);
  await page.click('[data-action="call"]:visible');
  await waitState(page, "menu");
  await page.waitForTimeout(600);
  check(await overflow(page) <= 0 && !(await clipped(page)).length, `${name}: menu fits, nothing cut off (${await clipped(page)})`);
  await settle(page);
  await page.screenshot({ path: `${SHOTS}/call-${name}-menu.png`, fullPage: true });
  await page.evaluate(() => {
    const r = document.querySelector("[data-call]");
    r.dataset.state = "talking";
    r.querySelectorAll("[data-show]").forEach((e) => (e.hidden = e.dataset.show.split(" ").indexOf("talking") === -1));
  });
  check(!(await clipped(page)).length, `${name}: Mute and Hang up fit (${await clipped(page)})`);
  await settle(page);
  await page.screenshot({ path: `${SHOTS}/call-${name}-talking.png`, fullPage: true });
  await ctx.close();
}

for (const e of errors) failures.push("browser error: " + e);
console.log(failures.length ? `\n${failures.length} problem(s):\n- ${failures.join("\n- ")}` : "\nAll good.");
await browser.close();
server.close();
rmSync(PRE, { recursive: true, force: true });
process.exit(failures.length ? 1 : 0);
