// Checks the owner's app (/app) without the Go server: renders it with sample
// calls (pages/app_test.go, TestAppPreview), serves those files and the real
// assets, and clicks through it in headless Chromium with a fake call socket
// (tabs, Available switch, voicemail playback, ringing, answer, decline,
// offline, phone and computer layouts). The real calls are in site.mjs.
// Run from the repo root: node tests/e2e/app.mjs
import { createRequire } from "node:module";
import { createServer } from "node:http";
import { execFileSync } from "node:child_process";
import { readFileSync, readdirSync, existsSync, mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { join, extname } from "node:path";
import { homedir, tmpdir } from "node:os";

const REPO = process.cwd();
const SHOTS = "tests/e2e/shots";
mkdirSync(SHOTS, { recursive: true });
const PRE = mkdtempSync(join(tmpdir(), "minivoip-app-"));
execFileSync("go", ["test", "./pages", "-run", "TestAppPreview", "-count=1"], { env: { ...process.env, MINIVOIP_PREVIEW: PRE }, stdio: "inherit" });
const npx = join(homedir(), ".npm/_npx");
const pw = readdirSync(npx).map((d) => join(npx, d, "node_modules/playwright-core")).find(existsSync);
const { chromium } = createRequire(import.meta.url)(pw);
const cache = join(homedir(), ".cache/ms-playwright");
const shell = readdirSync(cache).filter((d) => d.startsWith("chromium_headless_shell-")).sort().pop();

// A one-second tone as the voicemail audio.
function wav() {
  const rate = 8000, n = rate * 3, b = Buffer.alloc(44 + n * 2);
  b.write("RIFF", 0); b.writeUInt32LE(36 + n * 2, 4); b.write("WAVEfmt ", 8); b.writeUInt32LE(16, 16);
  b.writeUInt16LE(1, 20); b.writeUInt16LE(1, 22); b.writeUInt32LE(rate, 24); b.writeUInt32LE(rate * 2, 28);
  b.writeUInt16LE(2, 32); b.writeUInt16LE(16, 34); b.write("data", 36); b.writeUInt32LE(n * 2, 40);
  for (let i = 0; i < n; i++) b.writeInt16LE(Math.round(3000 * Math.sin(i / 8)), 44 + i * 2);
  return b;
}
const types = { ".css": "text/css", ".js": "text/javascript", ".svg": "image/svg+xml", ".png": "image/png", ".webmanifest": "application/manifest+json" };
const hits = [];
const server = createServer((req, res) => {
  const u = new URL(req.url, "http://x");
  hits.push(req.method + " " + u.pathname);
  let body = "";
  req.on("data", (c) => (body += c));
  req.on("end", () => {
    const send = (code, type, data) => { res.writeHead(code, { "Content-Type": type }); res.end(data); };
    const pre = (f) => send(200, "text/html", readFileSync(join(PRE, f)));
    if (u.pathname === "/app") return pre(u.searchParams.has("empty") ? "app-empty.html" : "app.html");
    if (u.pathname === "/app/calls") return pre("calls.html");
    if (u.pathname === "/login") return pre("login.html");
    if (u.pathname === "/app/voicemails") return pre("voicemails.html");
    if (u.pathname === "/app/available") return pre("avail-" + new URLSearchParams(body).get("mode") + ".html");
    if (/^\/voicemail\/\d+\/audio$/.test(u.pathname)) return send(200, "audio/wav", wav());
    if (/^\/voicemail\/\d+\/listened$/.test(u.pathname)) { res.writeHead(204); return res.end(); }
    if (u.pathname === "/api/ice") return send(200, "application/json", '{"iceServers":[]}');
    if (u.pathname === "/sw.js") return send(200, "text/javascript", readFileSync(join(REPO, "assets/js/sw.js")));
    if (u.pathname === "/manifest.webmanifest") return send(200, "application/manifest+json", readFileSync(join(REPO, "assets/brand/manifest.webmanifest")));
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

async function open(w, h, path = "/app") {
  const ctx = await browser.newContext({ viewport: { width: w, height: h }, permissions: ["microphone"] });
  const sock = { sent: [], ws: null, refuse: false };
  await ctx.routeWebSocket(/\/ws/, (ws) => {
    if (sock.refuse) return ws.close();
    sock.ws = ws;
    ws.onMessage((m) => {
      const msg = JSON.parse(m);
      sock.sent.push(msg);
      if (msg.type === "hello") ws.send(JSON.stringify({ type: "ready" }));
    });
  });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (m) => { if (m.type() === "error" && !/favicon|Failed to load resource/.test(m.text())) errors.push(m.text()); });
  await page.goto(BASE + path);
  await page.waitForFunction(() => document.querySelector("[data-phone]").dataset.online === "true");
  return { page, sock };
}
const overflow = (p) => p.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);

// --- phone ---------------------------------------------------------------
{
  const { page, sock } = await open(390, 844);
  check(await overflow(page) <= 0, "phone: no sideways scroll");
  check(await page.isVisible("text=Call while you were away") && await page.isVisible("text=Yesterday"), "phone: calls grouped by day");
  check(await page.isVisible("[data-vm-dot]"), "phone: red dot on Voicemail tab for a new message");
  await page.screenshot({ path: `${SHOTS}/app-phone-calls.png` });

  // Play the new voicemail from the call list.
  await page.click('[data-panel="calls"] [data-voicemail="3"] [data-play]');
  await page.waitForSelector('[data-panel="calls"] [data-voicemail="3"] .ap-play.is-playing');
  check(true, "phone: a voicemail plays in place");
  await page.waitForTimeout(400);
  check(hits.includes("POST /voicemail/3/listened"), "phone: playing marks it listened");
  check(await page.locator('[data-voicemail="3"] [data-new]').count() === 0 && await page.isHidden("[data-vm-dot]"), "phone: New badge and tab dot clear");
  await page.click('[data-panel="calls"] [data-voicemail="3"] [data-play]');

  for (const tab of ["voicemail", "menu", "room", "me"]) {
    await page.click(`[data-tab="${tab}"]`);
    check(await page.isVisible(`[data-panel="${tab}"]`) && (await page.evaluate(() => location.hash)) === "#" + tab, `phone: ${tab} tab opens`);
    await page.screenshot({ path: `${SHOTS}/app-phone-${tab}.png`, fullPage: true });
  }
  check(await page.isVisible("text=Log out"), "phone: Me has Log out");
  check(await page.getAttribute('[data-panel="room"] [data-room-open]', "href") === "https://voip.example.com/room/joesbnb", "phone: the Room tab opens the meeting room");

  // Someone waits in the meeting room: an alert with Join, on every tab.
  check(await page.isHidden("[data-room-alert]"), "phone: no meeting room alert while nobody waits");
  sock.ws.send(JSON.stringify({ type: "room", names: ["Ana Roy"] }));
  await page.waitForSelector("[data-room-alert]:visible");
  check(/Ana Roy is waiting in your meeting room/.test(await page.textContent("[data-room-alert]")), "phone: the app says who waits in the meeting room");
  check(await overflow(page) <= 0, "phone: the meeting room alert fits");
  await page.waitForTimeout(400);
  await page.screenshot({ path: `${SHOTS}/app-phone-room-waiting.png` });
  sock.ws.send(JSON.stringify({ type: "room" }));
  await page.waitForSelector("[data-room-alert]", { state: "hidden" });
  check(true, "phone: the alert goes away when nobody waits");
  check(!/Checking/.test(await page.textContent("[data-push-state]")), "phone: notification state is shown");
  await page.reload();
  await page.waitForFunction(() => document.querySelector("[data-phone]").dataset.online === "true");
  check(await page.isVisible('[data-panel="me"]'), "phone: the tab survives a reload (#me)");
  await page.click('[data-tab="calls"]');

  // Available switch.
  await page.click("[data-avail-btn]");
  await page.screenshot({ path: `${SHOTS}/app-phone-available-menu.png` });
  await page.click('[data-avail-menu] [data-mode="pause"]');
  await page.waitForSelector('[data-avail-state][data-mode="pause"]');
  check(/Paused until/.test(await page.textContent("[data-avail-btn]")), "phone: Pause for 1 hour shows until when");
  check(await overflow(page) <= 0, "phone: paused label fits");
  await page.screenshot({ path: `${SHOTS}/app-phone-paused.png` });
  await page.click("[data-avail-btn]");
  await page.click('[data-avail-menu] [data-mode="on"]');
  await page.waitForSelector('[data-avail-state][data-mode="on"]');

  // Incoming call, declined.
  sock.ws.send(JSON.stringify({ type: "incoming", id: "c1", option: "Book a room" }));
  await page.waitForSelector('[data-action="accept"]:visible');
  await page.waitForTimeout(1200);
  check(/Someone chose\s*“Book a room”/.test(await page.textContent("[data-call-title]")), "phone: ringing says what the caller chose");
  const cd = await page.textContent("[data-countdown]");
  check(/voicemail in 0:2\d/.test(cd), `phone: countdown to voicemail (${cd})`);
  await page.screenshot({ path: `${SHOTS}/app-phone-ringing.png` });
  await page.click('[data-action="decline"]');
  await page.waitForTimeout(200);
  check(sock.sent.some((m) => m.type === "decline" && m.id === "c1") && await page.isHidden("[data-call-screen]"), "phone: Decline tells the server and closes");

  // Incoming call with no choice, answered, then the caller hangs up.
  sock.ws.send(JSON.stringify({ type: "incoming", id: "c2" }));
  await page.waitForSelector('[data-action="accept"]:visible');
  check(/Someone is calling/.test(await page.textContent("[data-call-title]")), "phone: ringing without a choice");
  await page.click('[data-action="accept"]');
  await page.waitForFunction(() => document.querySelector('[data-action="hangup"]').offsetParent !== null);
  check(sock.sent.some((m) => m.type === "accept" && m.id === "c2"), "phone: Answer tells the server");
  await page.screenshot({ path: `${SHOTS}/app-phone-connecting.png` });
  await page.click('[data-action="mute"]');
  check((await page.getAttribute('[data-action="mute"]', "aria-pressed")) === "true", "phone: mute toggles");
  sock.ws.send(JSON.stringify({ type: "hangup", id: "c2" }));
  await page.waitForSelector("[data-call-screen]", { state: "hidden" });
  check(true, "phone: the caller hanging up closes the call screen");

  // The socket drops: banner after a moment.
  sock.refuse = true;
  await sock.ws.close();
  await page.waitForSelector("[data-offline]:visible", { timeout: 6000 });
  check(true, "phone: offline banner when the connection drops");
  await page.screenshot({ path: `${SHOTS}/app-phone-offline.png` });
  sock.refuse = false;
  await page.waitForFunction(() => document.querySelector("[data-phone]").dataset.online === "true", null, { timeout: 8000 });
  check(await page.isHidden("[data-offline]"), "phone: reconnects and hides the banner");
  await page.context().close();
}

// --- small phone, empty -------------------------------------------------
{
  const { page } = await open(360, 740, "/app?empty");
  check(await overflow(page) <= 0, "small phone: no sideways scroll");
  check(await page.isVisible("text=No calls yet"), "empty: friendly empty call list");
  await page.screenshot({ path: `${SHOTS}/app-phone-empty.png` });
  await page.context().close();
}

// --- log in ----------------------------------------------------------------
for (const [w, h, name] of [[390, 844, "phone"], [1280, 800, "desk"]]) {
  const ctx = await browser.newContext({ viewport: { width: w, height: h } });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto(BASE + "/login");
  check(await page.isVisible("[data-app-login]") && await overflow(page) <= 0, `${name}: app log in fits`);
  await page.screenshot({ path: `${SHOTS}/app-${name}-login.png` });
  await ctx.close();
}

// --- computer --------------------------------------------------------------
{
  const { page, sock } = await open(1280, 800);
  check(await overflow(page) <= 0, "computer: no sideways scroll");
  check(await page.isVisible("text=Calls ring here"), "computer: the right side explains ringing");
  await page.screenshot({ path: `${SHOTS}/app-desk-calls.png` });
  await page.click('[data-panel="calls"] [data-voicemail="3"] [data-open]');
  check(await page.isVisible("[data-detail-vm]") && /Voicemail · 0:42/.test(await page.textContent("[data-vm-title]")), "computer: a voicemail opens on the right");
  await page.click("[data-vm-play]");
  await page.waitForTimeout(1300);
  check(await page.evaluate(() => parseFloat(document.querySelector("[data-vm-bar]").style.width) > 0), "computer: the big player plays and shows progress");
  await page.screenshot({ path: `${SHOTS}/app-desk-voicemail.png` });
  await page.click("[data-vm-play]");
  sock.ws.send(JSON.stringify({ type: "incoming", id: "c3", option: "Book a room" }));
  await page.waitForSelector('[data-action="accept"]:visible');
  await page.waitForTimeout(500); // the screen slides in
  const box = await page.locator("[data-call-screen]").boundingBox();
  check(box && box.width === 1280 && box.height === 800, `computer: the call covers the whole window (${JSON.stringify(box)})`);
  await page.screenshot({ path: `${SHOTS}/app-desk-ringing.png` });
  await page.click('[data-action="decline"]');
  await page.evaluate(() => { document.hasFocus = () => false; });
  sock.ws.send(JSON.stringify({ type: "incoming", id: "c4", option: "Book a room" }));
  await page.waitForSelector('[data-action="accept"]:visible');
  await page.waitForFunction(() => /Incoming call/.test(document.title), null, { timeout: 3000 });
  check(true, "computer: behind other windows, the tab title flashes Incoming call");
  await page.click('[data-action="accept"]');
  await page.waitForFunction(() => document.querySelector('[data-action="hangup"]').offsetParent !== null);
  await page.screenshot({ path: `${SHOTS}/app-desk-talking.png` });
  await page.click('[data-action="hangup"]');
  check(sock.sent.some((m) => m.type === "hangup"), "computer: Hang up tells the server");
  await page.click('[data-tab="me"]');
  await page.screenshot({ path: `${SHOTS}/app-desk-me.png` });
  await page.click("[data-avail-btn]");
  await page.screenshot({ path: `${SHOTS}/app-desk-available-menu.png` });
  await page.context().close();
}

for (const e of errors) failures.push("browser error: " + e);
console.log(failures.length ? `\n${failures.length} problem(s):\n- ${failures.join("\n- ")}` : "\nAll good.");
await browser.close();
server.close();
rmSync(PRE, { recursive: true, force: true });
process.exit(failures.length ? 1 : 0);
