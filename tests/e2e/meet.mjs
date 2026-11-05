// Checks video, screen sharing and the whiteboard in a call, without the Go
// server: renders the visitor's call window and the owner's app (TestCallPreview,
// TestAppPreview), and connects two headless browsers through a small fake
// call socket in this script that passes messages like calls/hub.go. The call
// itself is real WebRTC between the two browsers (fake camera, fake screen).
// Notes on: the app records the real call and sends it to a fake upload here.
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

// The fake upload: what the app's recorder sends for call c1.
const rec = { mode: "on", started: 0, parts: [], done: 0, type: "", missed: 0 };
const recReset = (mode) => Object.assign(rec, { mode, started: 0, parts: [], done: 0, type: "", missed: 0 });
const types = { ".css": "text/css", ".js": "text/javascript", ".svg": "image/svg+xml", ".png": "image/png", ".webmanifest": "application/manifest+json" };
const server = createServer((req, res) => {
  const u = new URL(req.url, "http://x");
  const send = (code, type, data) => { res.writeHead(code, { "Content-Type": type }); res.end(data); };
  if (u.pathname === "/call/auberge") return send(200, "text/html", readFileSync(join(PRE, "call.html")));
  if (u.pathname === "/app") return send(200, "text/html", readFileSync(join(PRE, "app.html")));
  if (u.pathname === "/app-notes") return send(200, "text/html", readFileSync(join(PRE, "app-notes.html")));
  if (u.pathname === "/call/notes") return send(200, "text/html", readFileSync(join(PRE, "call-notes.html")));
  if (u.pathname === "/rec.bin") return send(200, "application/octet-stream", Buffer.concat(rec.parts));
  if (u.pathname.startsWith("/app/calls/c1/recording")) {
    const chunks = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => {
      const tail = u.pathname.split("/recording")[1];
      if (tail === "") {
        rec.started++;
        rec.type = u.searchParams.get("type");
        if (rec.mode === "start-error") return send(500, "text/plain", "no");
        if (rec.mode === "limit") return send(200, "application/json", '{"record":false,"reason":"month_limit"}');
        if (rec.type === "") return send(200, "application/json", '{"record":false,"reason":"no_recorder"}');
        return send(200, "application/json", '{"record":true}');
      }
      rec.missed = Math.max(rec.missed, Number(u.searchParams.get("missed") || 0));
      if (tail === "/done") { rec.done++; return send(204, "text/plain", ""); }
      if (rec.mode === "upload-error") return send(500, "text/plain", "no");
      if (rec.mode === "refused" && rec.parts.length >= 1) return send(409, "text/plain", "no");
      // A login that ran out: the server answers with a redirect to the log in page.
      if (rec.mode === "logged-out" && rec.parts.length >= 1) { res.writeHead(303, { Location: "/app" }); return res.end(); }
      rec.parts[Number(tail.slice(1))] = Buffer.concat(chunks);
      send(204, "text/plain", "");
    });
    return;
  }
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
    window.MinivoipRecorderSlice = 1500; // a piece every 1.5 seconds, not 30
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
  // It waits for the owner's tap: a stranger's picture never shows by itself.
  check(await until(() => owner.isVisible('[data-tile="p:c1"] [data-meet="show-video"]')) && !(await stage(owner)).vids.some((x) => x.tile === "p:c1"), `${name}: the visitor's camera waits for the owner's tap`);
  await owner.screenshot({ path: `${SHOTS}/meet-owner-${name}-ask.png` });
  await owner.click('[data-tile="p:c1"] [data-meet="show-video"]');
  const o = await until(async () => { const s = await stage(owner); return s.vids.some((x) => x.tile === "p:c1" && x.w > 0) && s; });
  check(!!o, `${name}: the visitor's camera shows on the owner's screen`);
  check((await stage(visitor)).vids.some((x) => x.tile === "me"), `${name}: the visitor sees themselves in the corner`);

  // The owner shows their screen.
  const shared = await owner.click('[data-action="screen"]').then(() => until(async () => { const x = await stage(visitor); return x.layout === "present" && x.vids.some((y) => y.tile === "screen:owner" && y.w > 0); }), () => false);
  v = await stage(visitor);
  check(shared && v.vids.some((x) => x.tile === "screen:owner" && x.w > 0), `${name}: the owner's screen fills the visitor's stage (${JSON.stringify(v.vids)})`);
  const mine = await until(async () => (await stage(owner)).vids.some((x) => x.tile === "screen:me" && x.w > 0));
  check(mine && await owner.isVisible('.mv-strip [data-tile="screen:me"] .mv-stop'), `${name}: the owner sees their own screen, small, with Stop sharing`);
  await visitor.screenshot({ path: `${SHOTS}/meet-${name}-screen.png` });
  // The whiteboard while a screen shows: it fills both stages, the screen goes small (Ken's test, 2026-10-26).
  await owner.click('[data-action="board"]');
  const both = async (p, small) => (await p.isVisible(".mv-main .mv-board")) && (await p.isVisible(`.mv-strip [data-focus="${small}"]`));
  check(await until(() => both(owner, "screen:me")) && await until(() => both(visitor, "screen:owner")), `${name}: the whiteboard fills both stages, the screen small under it`);
  await owner.screenshot({ path: `${SHOTS}/meet-owner-${name}-sharing.png` });
  await visitor.click('[data-focus="screen:owner"]');
  check(await until(() => visitor.isVisible('.mv-main [data-tile="screen:owner"]')) && await visitor.isVisible('.mv-strip [data-focus="board"]'), `${name}: a click on the small screen swaps it with the whiteboard`);
  await visitor.screenshot({ path: `${SHOTS}/meet-${name}-swap.png` });
  await owner.click('[data-focus="screen:me"]');
  check(await until(() => owner.isVisible('.mv-main [data-tile="screen:me"] .mv-stop')), `${name}: the owner can show their own screen big`);
  await owner.click('[data-action="board"]');
  check(await until(async () => !(await stage(visitor)).board && !(await stage(owner)).board), `${name}: the Whiteboard button closes it for both`);
  await owner.click(".mv-stop");
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

  // Nobody's camera on: the whiteboard gets the whole stage, no empty tiles under it (Ken, 2026-10-27).
  await owner.click('[data-action="camera"]');
  await visitor.click('[data-action="camera"]');
  await visitor.click('[data-action="board"]');
  const bare = async (p) => (await p.isVisible(".mv-main .mv-board")) && (await p.locator(".mv-strip > *").count()) === 0;
  check(await until(() => bare(visitor)) && await until(() => bare(owner)), `${name}: without cameras, nobody's tile shows under the whiteboard`);
  await visitor.mouse.move(5, 5);
  await visitor.waitForTimeout(700);
  await visitor.screenshot({ path: `${SHOTS}/meet-${name}-board-nocam.png` });
  await visitor.click('[data-action="board"]');
  check(await until(async () => !(await stage(owner)).board), `${name}: the whiteboard closes again`);

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

// What the other person's browser sends can't break the stage: junk on the
// data channel is ignored, and the camera button still tells the truth.
{
  const o = await context(1280, 860, "owner");
  await o.goto(BASE + "/app");
  await o.waitForFunction(() => document.querySelector("[data-phone]").dataset.online === "true");
  const v = await context(1280, 800, "visitor");
  await v.goto(BASE + "/call/auberge");
  await v.click('[data-action="call"]:visible');
  await v.waitForSelector('[data-key="1"]:visible');
  await v.click('[data-key="1"]');
  await o.waitForSelector('[data-action="accept"]:visible', { timeout: 15000 });
  await o.click('[data-action="accept"]');
  await v.waitForFunction(() => document.querySelector("[data-call]").dataset.state === "talking", null, { timeout: 15000 });
  await until(() => v.evaluate(() => { const s = document.querySelector("[data-stage]").meet; return s && Object.values(s.links).every((l) => l.dc.readyState === "open"); }));
  await v.evaluate(() => {
    const l = Object.values(document.querySelector("[data-stage]").meet.links)[0];
    l.send({ t: "state", name: { toString: 1, valueOf: 1 }, initials: 7 });
    l.send({ t: "s", id: "bad", c: "#0c0c0d", w: -5, p: [1, 2] });
    l.send({ t: "s", id: "__proto__", c: "javascript:", w: 3, p: ["x"] });
    l.send({ t: "state", name: "x".repeat(5000) });
  });
  await o.waitForTimeout(400);
  const flood = await o.evaluate(() => {
    const wb = document.querySelector("[data-stage]").meet.wb;
    for (let i = 0; i < 100; i++) wb.receive({ t: "s", id: "flood" + i, c: wb.color, w: 3, p: Array(20000).fill(1) });
    const kept = wb.received;
    wb.receive({ t: "clear" });
    return kept;
  });
  check(flood <= 400000, `a flood of whiteboard lines stops at the board's budget (${flood})`);
  const name = await o.evaluate(() => Object.values(document.querySelector("[data-stage]").meet.links)[0].name);
  check(name === "Caller", `the caller can't rename themselves on the owner's screen (${name})`);
  check(await o.evaluate(() => document.querySelector("[data-stage]").meet.wb.strokes.length) === 0, "junk whiteboard strokes are ignored");
  await o.click('[data-action="camera"]');
  check(await until(async () => (await o.getAttribute('[data-action="camera"]', "aria-pressed")) === "true") && await o.isHidden("text=blocked the camera"), "after the junk, the Camera button still tells the truth");
  await v.click('[data-action="hangup"]:visible');
  await o.waitForSelector("[data-call-screen]", { state: "hidden", timeout: 10000 });
  await v.context().close();
  await o.context().close();
}

// Notes on: a real call, recorded in the owner's app ---------------------------
async function notesCall(mode, during, before) {
  recReset(mode);
  const o = await context(1280, 860, "owner");
  if (before) await o.addInitScript(before);
  await o.goto(BASE + "/app-notes");
  await o.waitForFunction(() => document.querySelector("[data-phone]").dataset.online === "true");
  const v = await context(390, 844, "visitor");
  await v.goto(BASE + "/call/notes");
  await v.click('[data-action="call"]:visible');
  await v.waitForSelector('[data-key="1"]:visible');
  await v.click('[data-key="1"]');
  await o.waitForSelector('[data-action="accept"]:visible', { timeout: 15000 });
  check(rec.started === 0, `${mode}: nothing is recorded while it rings`);
  await o.click('[data-action="accept"]');
  await v.waitForFunction(() => document.querySelector("[data-call]").dataset.state === "talking", null, { timeout: 15000 });
  await o.waitForSelector("text=On the line", { timeout: 10000 });
  await during(o, v);
  // The call is still fine, whatever happened to the recording.
  const st = await o.evaluate(() => window.minivoipCallStats());
  check(st && st.bytesIn > 1000 && await o.isVisible('[data-action="hangup"]'), `${mode}: the call goes on (${st && st.bytesIn} bytes)`);
  await v.click('[data-action="hangup"]:visible');
  await o.waitForSelector("[data-call-screen]", { state: "hidden", timeout: 10000 });
  return { o, v };
}
const peaks = (o) => o.evaluate(async () => {
  const b = await (await fetch("/rec.bin")).arrayBuffer();
  const audio = await new OfflineAudioContext(2, 48000, 48000).decodeAudioData(b);
  const level = (data) => { let sum = 0; for (let i = 0; i < data.length; i++) sum += data[i] * data[i]; return Math.sqrt(sum / data.length); };
  const half = Math.floor(audio.length / 2);
  return { channels: audio.numberOfChannels, seconds: audio.duration,
    owner: [level(audio.getChannelData(0).subarray(0, half)), level(audio.getChannelData(0).subarray(half))],
    caller: [level(audio.getChannelData(1).subarray(0, half)), level(audio.getChannelData(1).subarray(half))] };
});
{
  console.log("--- notes on");
  // The owner mutes halfway: their channel goes quiet, and the caller's doesn't.
  const { o, v } = await notesCall("on", async (o) => {
    await o.waitForSelector("[data-rec-note].is-on", { timeout: 5000 });
    check(/Recording for your notes/.test(await o.textContent("[data-rec-note]")), "the owner sees the call is being recorded");
    await o.waitForTimeout(4000);
    check(rec.parts.length >= 2, `pieces go up during the call (${rec.parts.length})`);
    await o.screenshot({ path: `${SHOTS}/meet-owner-recording.png` });
    await o.click('[data-action="mute"]');
    await o.waitForTimeout(5000);
    await o.click('[data-action="mute"]');
  });
  await until(() => rec.done > 0);
  check(rec.done === 1, `the app says the last piece is in (${rec.done})`);
  check(rec.started === 1 && /^audio\/(webm|mp4|ogg)/.test(rec.type), `it starts once, with a recording type (${rec.type})`);
  check(rec.parts.every(Boolean), `no piece is missing (${rec.parts.length})`);
  const p = await peaks(o);
  check(p.channels === 2 && p.seconds > 8, `the pieces make one file: two channels, ${p.seconds.toFixed(1)} s`);
  check(p.owner[0] > 0.001 && p.caller[0] > 0.001, `both voices are in it (owner ${p.owner[0].toFixed(4)}, caller ${p.caller[0].toFixed(4)})`);
  check(p.owner[1] < p.owner[0] / 2 && p.caller[1] > p.caller[0] / 2, `each on its own channel: mute silences only the owner's (owner ${p.owner[1].toFixed(4)}, caller ${p.caller[1].toFixed(4)})`);
  check(await o.isHidden("[data-rec-note]"), "after the call the recording line is gone");
  await v.context().close();
  await o.context().close();
}
for (const [mode, words] of [["limit", "10 hours"], ["start-error", "Recording stopped"], ["upload-error", null], ["refused", null], ["logged-out", null]]) {
  const before = errors.length;
  const { o, v } = await notesCall(mode, async (o) => {
    await o.waitForTimeout(mode === "upload-error" ? 2500 : 4000);
    if (words) check((await o.textContent("[data-rec-note]")).includes(words), `${mode}: the owner is told (${await o.textContent("[data-rec-note]")})`);
    if (mode === "limit") check(rec.parts.length === 0, "limit: nothing is sent");
    if (mode === "refused") check(rec.parts.length === 1 && /stopped/.test(await o.textContent("[data-rec-note]")), `refused: it stops after the server says so (${rec.parts.length})`);
    if (mode === "logged-out") check(rec.parts.length === 1 && /stopped/.test(await o.textContent("[data-rec-note]")), `logged-out: a redirect isn't taken for an upload (${rec.parts.length})`);
  });
  if (mode === "upload-error") check(await until(() => rec.done > 0, 3000) || rec.parts.length === 0, "upload-error: the failed pieces don't hold up the end of the call");
  errors.splice(before);
  await v.context().close();
  await o.context().close();
}
// Other browsers, and what a locked phone does -----------------------------------
const hasMp4 = await owner_probe();
async function owner_probe() {
  const p = await context(800, 600, "owner");
  await p.goto(BASE + "/app");
  const ok = await p.evaluate(() => MediaRecorder.isTypeSupported("audio/mp4"));
  await p.context().close();
  return ok;
}
{
  console.log(`--- other browsers (this one records mp4: ${hasMp4})`);
  // Safari's choice: only mp4 on offer.
  const { o, v } = await notesCall("on", async (o) => {
    await o.waitForFunction(() => document.querySelector("[data-rec-note]").textContent !== "", null, { timeout: 5000 });
    await o.waitForTimeout(3500);
  }, () => { const real = MediaRecorder.isTypeSupported.bind(MediaRecorder); MediaRecorder.isTypeSupported = (t) => t === "audio/mp4" && real(t); });
  await until(() => rec.done > 0);
  if (hasMp4) {
    check(rec.type === "audio/mp4", `with only mp4 on offer, it asks for mp4 (${rec.type})`);
    const first = rec.parts[0];
    check(first && first.subarray(4, 8).toString() === "ftyp", "the pieces start like an mp4 file");
    const p = await peaks(o);
    check(p.channels === 2 && p.owner[0] > 0.001 && p.caller[0] > 0.001, `the mp4 pieces make one file with both voices (${p.channels} channels)`);
  } else check(rec.type === "", `no recording type left, it says so (${rec.type})`);
  await v.context().close();
  await o.context().close();
}
{
  // A browser with no recorder: the owner is told, and the call goes on.
  const { o, v } = await notesCall("on", async (o) => {
    await o.waitForFunction(() => document.querySelector("[data-rec-note]").textContent !== "", null, { timeout: 5000 });
    check(/can't record/.test(await o.textContent("[data-rec-note]")), "no recorder: the owner is told");
    check(rec.type === "" && rec.parts.length === 0, "and nothing is sent");
    await o.waitForTimeout(2000);
  }, () => { delete window.MediaRecorder; });
  await v.context().close();
  await o.context().close();
}
{
  // The page frozen, the microphone cut off, the sound engine stopped: each is
  // counted as missed, and goes up with the pieces.
  const before = () => {
    window.__contexts = [];
    const AC = window.AudioContext;
    window.AudioContext = function (...a) { const c = new AC(...a); window.__contexts.push(c); return c; };
    window.__mics = [];
    const gum = navigator.mediaDevices.getUserMedia.bind(navigator.mediaDevices);
    navigator.mediaDevices.getUserMedia = (c) => gum(c).then((s) => { window.__mics.push(s); return s; });
    window.__recorders = [];
    const MR = window.MediaRecorder;
    window.MediaRecorder = function (...a) { const r = new MR(...a); window.__recorders.push(r); return r; };
    window.MediaRecorder.isTypeSupported = MR.isTypeSupported.bind(MR);
  };
  const { o, v } = await notesCall("on", async (o) => {
    await o.waitForSelector("[data-rec-note].is-on", { timeout: 5000 });
    await o.evaluate(() => { const end = Date.now() + 6500; while (Date.now() < end); });
    await o.waitForTimeout(2500);
    check(rec.missed >= 6 && rec.missed <= 9, `a frozen page counts as missed (${rec.missed} s)`);
    const afterFreeze = rec.missed;
    await o.evaluate(() => window.__mics.at(-1).getAudioTracks()[0].dispatchEvent(new Event("mute")));
    await o.waitForTimeout(3500);
    await o.evaluate(() => window.__mics.at(-1).getAudioTracks()[0].dispatchEvent(new Event("unmute")));
    await o.waitForTimeout(2000);
    check(rec.missed >= afterFreeze + 3 && rec.missed <= afterFreeze + 5, `the phone cutting off the microphone counts too (${rec.missed} s)`);
    const afterMic = rec.missed;
    await o.evaluate(() => window.__contexts.at(-1).suspend());
    await o.waitForTimeout(3500);
    await o.evaluate(() => document.dispatchEvent(new Event("visibilitychange")));
    await o.waitForFunction(() => window.__contexts.at(-1).state === "running", null, { timeout: 3000 });
    check(true, "coming back to the page starts the stopped sound engine again");
    await o.waitForTimeout(2000);
    check(rec.missed >= afterMic + 3 && rec.missed <= afterMic + 5, `so a stopped sound engine counts too (${rec.missed} s)`);
    check(await o.isVisible("[data-rec-note].is-on"), "and the recording goes on");
    // The browser stops the recorder by itself: what's in goes up, and it says so.
    const had = rec.parts.length;
    await o.evaluate(() => window.__recorders.at(-1).stop());
    await o.waitForFunction(() => /Recording stopped/.test(document.querySelector("[data-rec-note]").textContent), null, { timeout: 3000 });
    check(await until(() => rec.done === 1, 3000) && rec.parts.length >= had, "the browser stopping the recorder keeps what's in, and says so");
  }, before);
  check(rec.done === 1, `it doesn't say it's done twice (${rec.done})`);
  await v.context().close();
  await o.context().close();
}
{
  // Notes off: no recorder at all.
  recReset("on");
  const o = await context(1280, 860, "owner");
  await o.goto(BASE + "/app");
  await o.waitForFunction(() => document.querySelector("[data-phone]").dataset.online === "true");
  const v = await context(390, 844, "visitor");
  await v.goto(BASE + "/call/auberge");
  await v.click('[data-action="call"]:visible');
  await v.waitForSelector('[data-key="1"]:visible');
  await v.click('[data-key="1"]');
  await o.waitForSelector('[data-action="accept"]:visible', { timeout: 15000 });
  await o.click('[data-action="accept"]');
  await v.waitForFunction(() => document.querySelector("[data-call]").dataset.state === "talking", null, { timeout: 15000 });
  await o.waitForTimeout(2000);
  check(rec.started === 0 && await o.isHidden("[data-rec-note]"), "notes off: nothing is recorded");
  await v.click('[data-action="hangup"]:visible');
  await v.context().close();
  await o.context().close();
}

check(errors.length === 0, `no page errors${errors.length ? ": " + errors.join(" | ") : ""}`);
await browser.close();
server.close();
rmSync(PRE, { recursive: true, force: true });
console.log(failures.length ? `\n${failures.length} FAILED` : "\nall passed");
process.exit(failures.length ? 1 : 0);
