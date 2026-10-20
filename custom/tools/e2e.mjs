import { chromium } from "playwright-core";
import fs from "fs";
const EXE = process.env.HOME + "/.cache/ms-playwright/chromium_headless_shell-1243/chrome-headless-shell-linux64/chrome-headless-shell";
const BASE = "https://omt.dresdell.com";
const PASS = fs.readFileSync("pass.txt", "utf8").trim();
const RELAY = process.argv.includes("--relay");
const shots = "shots"; fs.mkdirSync(shots, { recursive: true });
const browser = await chromium.launch({ executablePath: EXE, args: ["--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream", "--autoplay-policy=no-user-gesture-required"] });
const forceRelay = `(() => { const P = window.RTCPeerConnection; window.RTCPeerConnection = function (c) { c = Object.assign({}, c, { iceTransportPolicy: "relay" }); const pc = new P(c); window.__pc = pc; return pc; }; window.RTCPeerConnection.prototype = P.prototype; })();`;
const track = `(() => { const P = window.RTCPeerConnection; window.RTCPeerConnection = function (c) { const pc = new P(c); window.__pc = pc; return pc; }; window.RTCPeerConnection.prototype = P.prototype; })();`;
const log = (...a) => console.log(new Date().toISOString().slice(11, 19), ...a);

const owner = await browser.newContext({ viewport: { width: 390, height: 844 }, permissions: ["microphone"] });
await owner.addInitScript(RELAY ? forceRelay : track);
const op = await owner.newPage();
op.on("console", (m) => m.type() === "error" && log("owner console:", m.text()));
await op.goto(BASE + "/call/owner/");
await op.fill("#code", PASS);
await op.click("button[type=submit]");
await op.waitForSelector("#dot.is-on", { timeout: 10000 });
log("owner ready:", await op.textContent("#conn"));
await op.screenshot({ path: `${shots}/owner-home.png` });

const guest = await browser.newContext({ viewport: { width: 390, height: 844 }, permissions: ["microphone"], hasTouch: true, isMobile: true });
await guest.addInitScript(RELAY ? forceRelay : track);
const gp = await guest.newPage();
gp.on("console", (m) => m.type() === "error" && log("guest console:", m.text()));
await gp.goto(BASE + "/#work");
await gp.locator(".omt-listen").scrollIntoViewIfNeeded();
await gp.waitForFunction(() => !document.querySelector(".omt-listen").classList.contains("is-waiting"), null, { timeout: 15000 });
await gp.waitForTimeout(1500);
await gp.locator(".omt-listen").screenshot({ path: `${shots}/contact-mobile.png` });
const status = () => gp.textContent(".cbx-status");

await gp.click(".omt-listen-call");
await gp.waitForTimeout(400);
await gp.screenshot({ path: `${shots}/guest-dialing.png` });
await gp.waitForFunction(() => document.querySelector(".cbx-status").textContent === "En ligne", null, { timeout: 10000 });
log("guest:", await status());
await gp.waitForTimeout(1500);
await gp.screenshot({ path: `${shots}/guest-menu.png` });
await gp.click('[data-key="1"]');
await gp.waitForSelector('[data-card="1"]');
await gp.waitForTimeout(600);
await gp.screenshot({ path: `${shots}/guest-key1.png` });

await gp.click('[data-key="0"]');
const t0 = Date.now();
await op.waitForSelector("#call:not([hidden])", { timeout: 20000 });
log("owner sees incoming after", Date.now() - t0, "ms; guest:", await status());
await gp.screenshot({ path: `${shots}/guest-ringing.png` });
await op.screenshot({ path: `${shots}/owner-incoming.png` });
await op.click("#accept");
await gp.waitForFunction(() => /En ligne avec/.test(document.querySelector(".cbx-status").textContent), null, { timeout: 20000 });
log("connected after", Date.now() - t0, "ms; guest:", await status(), "| owner:", await op.textContent("#call-status"));
await gp.waitForTimeout(3000);
const stats = async (p) => p.evaluate(async () => {
  const s = await window.__pc.getStats(); let r = { in: 0, out: 0, pair: "" };
  s.forEach((x) => { if (x.type === "inbound-rtp" && x.kind === "audio") r.in = x.bytesReceived; if (x.type === "outbound-rtp" && x.kind === "audio") r.out = x.bytesSent;
    if (x.type === "candidate-pair" && x.nominated && x.state === "succeeded") { const l = s.get(x.localCandidateId), rm = s.get(x.remoteCandidateId); r.pair = l.candidateType + "->" + rm.candidateType; } });
  return r; });
log("guest audio", JSON.stringify(await stats(gp)), "| owner audio", JSON.stringify(await stats(op)));
await gp.screenshot({ path: `${shots}/guest-talking.png` });
await op.screenshot({ path: `${shots}/owner-talking.png` });
await gp.click('[data-act="hangup"]');
await op.waitForSelector("#home:not([hidden])", { timeout: 10000 });
log("hung up; guest:", await status(), "| owner back home");
await gp.screenshot({ path: `${shots}/guest-ended.png` });

if (!RELAY) {
  // Voicemail: owner declines
  await gp.click('[data-act="again"]');
  await gp.waitForFunction(() => document.querySelector(".cbx-status").textContent === "En ligne", null, { timeout: 10000 });
  await gp.click('[data-key="0"]');
  await op.waitForSelector("#call:not([hidden])", { timeout: 20000 });
  await op.click("#decline");
  await gp.waitForFunction(() => /Enregistrement/.test(document.querySelector(".cbx-status").textContent), null, { timeout: 20000 });
  await gp.waitForTimeout(3000);
  await gp.screenshot({ path: `${shots}/guest-voicemail.png` });
  await gp.click('[data-act="send"]');
  await gp.waitForFunction(() => /Message envoyé|pas pu/.test(document.querySelector(".cbx-status").textContent), null, { timeout: 10000 });
  log("voicemail:", await status());
  await op.waitForTimeout(1500);
  await op.reload();
  await op.waitForSelector(".vm", { timeout: 5000 });
  log("owner voicemails:", await op.locator(".vm").count());
  await op.screenshot({ path: `${shots}/owner-voicemail.png` });
}
await browser.close();
