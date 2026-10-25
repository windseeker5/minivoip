// Checks video, screen sharing and the whiteboard in a call, without the Go
// server: renders the visitor's call window and the owner's app (TestCallPreview,
// TestAppPreview), and connects two headless browsers through a small fake
// call socket in this script that passes messages like calls/hub.go. The call
// itself is real WebRTC between the two browsers (fake camera, fake screen).
// The real server's calls are in site.mjs.
// Run from the repo root: node tests/e2e/meet.mjs
import { createRequire } from "node:module";
import { createServer } from "node:http";
import { execFileSync } from "node:child_process";
import { readFileSync, readdirSync, existsSync, mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { join, extname } from "node:path";
import { homedir, tmpdir } from "node:os";

const REPO = process.cwd();
const SHOTS = "tests/e2e/shots";
mkdirSync(SHOTS, { recursive: true });
const PRE = mkdtempSync(join(tmpdir(), "minivoip-meet-"));
for (const t of ["TestCallPreview", "TestAppPreview"]) {
  execFileSync("go", ["test", "./pages", "-run", t, "-count=1"], { env: { ...process.env, MINIVOIP_PREVIEW: PRE }, stdio: "inherit" });
}
const npx = join(homedir(), ".npm/_npx");
const pw = readdirSync(npx).map((d) => join(npx, d, "node_modules/playwright-core")).find(existsSync);
const { chromium } = createRequire(import.meta.url)(pw);
const cache = join(homedir(), ".cache/ms-playwright");
const shell = readdirSync(cache).filter((d) => d.startsWith("chromium_headless_shell-")).sort().pop();

const types = { ".css": "text/css", ".js": "text/javascript", ".svg": "image/svg+xml", ".png": "image/png", ".webmanifest": "application/manifest+json" };
const server = createServer((req, res) => {
  const u = new URL(req.url, "http://x");
  const send = (code, type, data) => { res.writeHead(code, { "Content-Type": type }); res.end(data); };
  if (u.pathname === "/call/auberge") return send(200, "text/html", readFileSync(join(PRE, "call.html")));
  if (u.pathname === "/app") return send(200, "text/html", readFileSync(join(PRE, "app.html")));
  if (u.pathname === "/app/calls") return send(200, "text/html", readFileSync(join(PRE, "calls.html")));
  if (u.pathname === "/api/ice") return send(200, "application/json", '{"iceServers":[]}');
  if (u.pathname === "/sw.js") return send(200, "text/javascript", readFileSync(join(REPO, "assets/js/sw.js")));
  if (u.pathname.startsWith("/assets/")) {
    const f = join(REPO, u.pathname);
    if (existsSync(f)) return send(200, types[extname(f)] || "application/octet-stream", readFileSync(f));
  }
  send(404, "text/plain", "not found");
});
await new Promise((r) => server.listen(0, "127.0.0.1", r));
const BASE = `http://127.0.0.1:${server.address().port}`;

const browser = await chromium.launch({
  executablePath: join(cache, shell, "chrome-headless-shell-linux64/chrome-headless-shell"),
  args: ["--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream", "--autoplay-policy=no-user-gesture-required",
    "--auto-select-desktop-capture-source=Entire screen", "--enable-usermedia-screen-capturing"],
});
const failures = [];
const check = (ok, what) => { console.log((ok ? "ok   " : "FAIL ") + what); if (!ok) failures.push(what); };
const errors = [];

// The fake call socket: one visitor, one phone app, messages passed like
// calls/hub.go does.
const hub = { visitor: null, owner: null };
const to = (ws, m) => ws && ws.send(JSON.stringify(m));
async function context(w, h, role) {
  const ctx = await browser.newContext({ viewport: { width: w, height: h }, permissions: ["microphone", "camera"] });
  await ctx.addInitScript(() => {
    // A fake browser voice, quick, so the menu's hold message ends fast.
    Object.defineProperty(window, "speechSynthesis", { configurable: true, value: {
      speak(u) { setTimeout(() => u.onend && u.onend({}), 200); }, cancel() {},
    } });
  });
  await ctx.routeWebSocket(/\/ws/, (ws) => {
    hub[role] = ws;
    ws.onMessage((raw) => {
      const m = JSON.parse(raw);
      if (role === "owner") {
        if (m.type === "hello") to(ws, { type: "ready" });
        else if (m.type === "accept") to(hub.visitor, { type: "accepted", id: "c1" });
        else if (m.type === "hangup") to(hub.visitor, { type: "hangup", id: "c1" });
        else to(hub.visitor, m);
      } else {
        if (m.type === "call") { to(ws, { type: "ringing", id: "c1" }); to(hub.owner, { type: "incoming", id: "c1", option: m.option }); }
        else if (m.type === "hangup") to(hub.owner, { type: "hangup", id: "c1" });
        else to(hub.owner, { ...m, id: "c1" });
      }
    });
  });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => errors.push(`${role}: ${e.message}`));
  page.on("console", (m) => { if (m.type() === "error" && !/favicon|Failed to load resource|fonts/.test(m.text())) errors.push(`${role}: ${m.text()}`); });
  return page;
}

const stage = (p) => p.evaluate(() => {
  const s = document.querySelector(".mv-stage");
  const m = s && s.meet;
  const vids = [...s.querySelectorAll(".mv-tile.has-video video")].map((v) => ({ tile: v.parentElement.dataset.tile, w: v.videoWidth, playing: !v.paused }));
  return { layout: s.dataset.layout, on: !!(m && m.active()), vids, strokes: m ? m.wb.strokes.length : 0, board: !!(m && m.boardOpen),
    names: [...s.querySelectorAll(".mv-name")].map((n) => n.textContent) };
});
const until = async (fn, ms = 8000) => { const end = Date.now() + ms; let v; while (Date.now() < end) { v = await fn(); if (v) return v; await new Promise((r) => setTimeout(r, 150)); } return v; };
const overflow = (p) => p.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);

// draw drags a line across the whiteboard's paper.
async function draw(p, from, toPt) {
  const box = await p.locator(".mv-paper canvas").boundingBox();
  await p.mouse.move(box.x + box.width * from[0], box.y + box.height * from[1]);
  await p.mouse.down();
  for (let i = 1; i <= 12; i++) {
    await p.mouse.move(box.x + box.width * (from[0] + (toPt[0] - from[0]) * i / 12), box.y + box.height * (from[1] + (toPt[1] - from[1]) * i / 12));
  }
  await p.mouse.up();
}

// One phone app for two calls in a row: the second call reuses its stage.
const owner = await context(1280, 860, "owner");
await owner.goto(BASE + "/app");
await owner.waitForFunction(() => document.querySelector("[data-phone]").dataset.online === "true");
for (const [name, vw, vh] of [["phone", 390, 844], ["desk", 1280, 860]]) {
  console.log(`--- visitor on a ${name}`);
  const visitor = await context(vw, vh, "visitor");
  await visitor.goto(BASE + "/call/auberge");
  await visitor.click('[data-action="call"]:visible');
  await visitor.waitForSelector('[data-key="1"]:visible');
  await visitor.click('[data-key="1"]');
  await owner.waitForSelector('[data-action="accept"]:visible', { timeout: 15000 });
  await owner.click('[data-action="accept"]');
  await visitor.waitForFunction(() => document.querySelector("[data-call]").dataset.state === "talking", null, { timeout: 15000 });
  await owner.waitForSelector("text=On the line", { timeout: 10000 });
  check(true, `${name}: the call connects`);
  check(!(await stage(visitor)).on && !(await stage(owner)).on && !(await visitor.isVisible(".mv-stage")) && !(await owner.isVisible(".mv-stage")),
    `${name}: voice only at first: no stage (${await visitor.isVisible(".mv-stage")} ${await owner.isVisible(".mv-stage")} ${JSON.stringify(await stage(owner))})`);
  const shareShown = await visitor.isVisible('[data-action="screen"]');
  check(name === "desk" ? shareShown : true, `${name}: the visitor's Screen button shows on a computer`);

  // The owner turns the camera on: the visitor sees them.
  await owner.click('[data-action="camera"]');
  let v = await until(async () => { const s = await stage(visitor); return s.vids.some((x) => x.tile === "p:owner" && x.w > 0) && s; });
  check(v && v.on && v.layout === "pair", `${name}: the owner's camera shows on the visitor's screen (${JSON.stringify(v && v.vids)})`);
  check(await visitor.evaluate(() => document.querySelector("[data-call]").dataset.stageOn) === "on", `${name}: the call window opens its stage`);
  const st = await until(async () => { const x = await visitor.evaluate(() => window.minivoipCallStats()); return x && x.videoIn > 5000 && x; }) || await visitor.evaluate(() => window.minivoipCallStats());
  check(st && st.videoIn > 5000 && st.bytesIn > 1000, `${name}: video and voice both flow (${st && st.videoIn} video bytes)`);
  check(await owner.getAttribute('[data-action="camera"]', "aria-pressed") === "true", `${name}: the owner's Camera button shows it's on`);
  await visitor.waitForTimeout(600);
  await visitor.screenshot({ path: `${SHOTS}/meet-${name}-camera.png` });
  await owner.screenshot({ path: `${SHOTS}/meet-owner-${name}-camera.png` });
  check(await overflow(visitor) <= 0, `${name}: no sideways scroll with the stage`);

  // The visitor turns theirs on too.
  await visitor.click('[data-action="camera"]');
  const o = await until(async () => { const s = await stage(owner); return s.vids.some((x) => x.tile === "p:c1" && x.w > 0) && s; });
  check(!!o, `${name}: the visitor's camera shows on the owner's screen`);
  check((await stage(visitor)).vids.some((x) => x.tile === "me"), `${name}: the visitor sees themselves in the corner`);

  // The owner shows their screen.
  const shared = await owner.click('[data-action="screen"]').then(() => until(async () => { const x = await stage(visitor); return x.layout === "present" && x.vids.some((y) => y.tile === "screen:owner" && y.w > 0); }), () => false);
  v = await stage(visitor);
  check(shared && v.vids.some((x) => x.tile === "screen:owner" && x.w > 0), `${name}: the owner's screen fills the visitor's stage (${JSON.stringify(v.vids)})`);
  check(await owner.isVisible("text=You're showing your screen"), `${name}: the owner sees "You're showing your screen"`);
  await visitor.waitForTimeout(800);
  await visitor.screenshot({ path: `${SHOTS}/meet-${name}-screen.png` });
  await owner.screenshot({ path: `${SHOTS}/meet-owner-${name}-sharing.png` });
  await owner.click(".mv-sharing button");
  check(await until(async () => (await stage(visitor)).layout === "pair"), `${name}: Stop ends the screen on both sides`);

  // The visitor opens the whiteboard: it opens for the owner too, and both draw.
  await visitor.click('[data-action="board"]');
  check(await until(async () => (await stage(owner)).board), `${name}: the whiteboard opens on both sides`);
  await draw(visitor, [0.2, 0.3], [0.7, 0.6]);
  check(await until(async () => (await stage(owner)).strokes === 1), `${name}: the visitor's drawing shows on the owner's board`);
  await owner.click('.mv-color[aria-label="Orange pen"]');
  await draw(owner, [0.3, 0.7], [0.8, 0.2]);
  check(await until(async () => (await stage(visitor)).strokes === 2), `${name}: the owner's drawing shows on the visitor's board`);
  await visitor.waitForTimeout(400);
  await visitor.screenshot({ path: `${SHOTS}/meet-${name}-board.png` });
  await owner.screenshot({ path: `${SHOTS}/meet-owner-${name}-board.png` });
  await owner.click('[data-board="undo"]');
  check(await until(async () => (await stage(visitor)).strokes === 1), `${name}: Undo removes the owner's own line for both`);
  await visitor.click('[data-board="clear"]');
  check(await until(async () => (await stage(owner)).strokes === 0), `${name}: Clear empties the board for both`);
  await owner.click('[data-board="close"]');
  check(await until(async () => !(await stage(visitor)).board), `${name}: Close closes the board for both`);

  // Mute shows on the other side.
  await visitor.click('[data-action="mute"]');
  check(await until(async () => (await owner.locator('.mv-tile[data-tile="p:c1"] .mv-name svg').count()) === 1), `${name}: the owner sees the visitor is muted`);

  // Hang up: everything stops.
  await visitor.click('[data-action="hangup"]:visible');
  await owner.waitForSelector("[data-call-screen]", { state: "hidden", timeout: 10000 });
  const live = await owner.evaluate(() => document.querySelectorAll(".mv-tile").length);
  check(live === 0, `${name}: after hanging up, the owner's stage is empty`);
  check(await visitor.evaluate(() => !document.querySelector("[data-call]").dataset.stageOn), `${name}: the call window closes its stage`);
  await visitor.context().close();
}
await owner.context().close();

check(errors.length === 0, `no page errors${errors.length ? ": " + errors.join(" | ") : ""}`);
await browser.close();
server.close();
rmSync(PRE, { recursive: true, force: true });
console.log(failures.length ? `\n${failures.length} FAILED` : "\nall passed");
process.exit(failures.length ? 1 : 0);
