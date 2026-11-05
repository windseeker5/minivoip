// Premium notes for real, against Ken's running dev server, when its .env can write
// them (keys, or MINIVOIP_NOTES_LOCAL=1). Headless only:  node tests/e2e/notes-real.mjs
//
// Two browsers whose microphones play made-up French voices: a real call and a real
// voicemail, written down and summarized by the real services (this spends a little),
// read back from the Notes tab. It never rings Ken's line, and deletes its test account.
// Screenshots: tests/e2e/shots/real-*.png.

import { createRequire } from "node:module";
import { DatabaseSync } from "node:sqlite";
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readdirSync, rmSync } from "node:fs";
import { homedir, tmpdir } from "node:os";
import { dirname, join } from "node:path";

const BASE = process.env.BASE || "http://localhost:7331";
const SHOTS = "tests/e2e/shots";
const DB = process.env.MINIVOIP_DB || "data/minivoip.db";
mkdirSync(SHOTS, { recursive: true });

const npx = join(homedir(), ".npm/_npx");
const pw = process.env.PWC || readdirSync(npx).map((d) => join(npx, d, "node_modules/playwright-core")).find(existsSync);
const cache = join(homedir(), ".cache/ms-playwright");
const shell = readdirSync(cache).filter((d) => d.startsWith("chromium_headless_shell-")).sort().pop();
const { chromium } = createRequire(import.meta.url)(pw);

// The voices: words nobody really said, in the 16-bit sound Chromium's fake microphone plays.
const voices = mkdtempSync(join(tmpdir(), "minivoip-voices-"));
function voice(name, who, text) {
  const mp3 = join(voices, name + ".mp3"), wav = join(voices, name + ".wav");
  execFileSync("uv", ["run", "-q", "--no-project", "--with", "edge-tts", "edge-tts", "--voice", who, "--text", text, "--write-media", mp3], { stdio: "ignore" });
  execFileSync("ffmpeg", ["-y", "-loglevel", "error", "-i", mp3, "-af", "apad=pad_dur=2", "-ac", "1", "-ar", "48000", "-c:a", "pcm_s16le", wav]);
  return wav;
}
const callerVoice = voice("caller", "fr-CA-SylvieNeural", "Bonjour, ici Marie Tremblay. Je voudrais réserver la chambre bleue du douze au quatorze décembre, pour deux personnes. Pouvez-vous me rappeler au quatre un huit, cinq cinq cinq, zéro un neuf neuf? Merci et bonne journée.");
const ownerVoice = voice("owner", "fr-CA-AntoineNeural", "Oui bonjour, ici l'auberge du Lac. La chambre bleue est libre à ces dates-là. Je vous envoie la confirmation par courriel ce soir.");

const open = (mic) => chromium.launch({
  executablePath: join(cache, shell, "chrome-headless-shell-linux64/chrome-headless-shell"),
  args: ["--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream", `--use-file-for-fake-audio-capture=${mic}`, "--autoplay-policy=no-user-gesture-required"],
});
const page = async (b) => (await b.newContext({ viewport: { width: 1280, height: 900 }, permissions: ["microphone", "camera"] })).newPage();

const stamp = Date.now().toString(36);
const EMAIL = `e2e-${stamp}@example.com`, LINE = `e2e-${stamp}`, CODE = `E2E-REAL-${stamp.toUpperCase()}`;
const failures = [];
const check = (ok, what) => { console.log(`${ok ? "ok  " : "FAIL"} ${what}`); if (!ok) failures.push(what); };
const notes = (where = "1 = 1") => {
  const db = new DatabaseSync(DB, { readOnly: true });
  const rows = db.prepare(`SELECT n.* FROM note n JOIN line l ON l.id = n.line_id WHERE l.name = ? AND ${where} ORDER BY n.id`).all(LINE);
  db.close();
  return rows;
};
async function finished(kind) {
  for (let i = 0; i < 300; i++) {
    const n = notes(`n.kind = '${kind}'`)[0];
    if (n && !["waiting", "working"].includes(n.status)) return n;
    await new Promise((r) => setTimeout(r, 1000));
  }
  return notes(`n.kind = '${kind}'`)[0];
}
function show(n) {
  console.log(`\n--- ${n.kind}: ${n.status} ${n.reason || ""} (${n.seconds} s)`);
  for (const l of JSON.parse(n.transcript || "[]")) console.log(`  ${l.speaker}: ${l.text}`);
  console.log(`  summary: ${n.summary}\n  next: ${n.next_actions}\n`);
}

const ownerBrowser = await open(ownerVoice), callerBrowser = await open(callerVoice);
{
  const db = new DatabaseSync(DB);
  db.prepare("INSERT INTO promo_code (code, plan) VALUES (?, 'premium')").run(CODE);
  db.close();
}
try {
  const owner = await page(ownerBrowser), caller = await page(callerBrowser);
  await owner.goto(BASE + "/signup");
  await owner.fill("#email", EMAIL);
  await owner.fill("#password", "e2e-password");
  await owner.fill("#promo", CODE);
  await owner.click("main button[type=submit]");
  await owner.waitForSelector("[data-business]");
  await owner.fill("#business", "Auberge du Lac");
  await owner.click("[data-link-edit]");
  await owner.fill("#name", LINE);
  await owner.click("main button[type=submit]");
  await owner.waitForURL(/dashboard/);
  await owner.goto(BASE + "/app");
  await owner.waitForSelector('[data-phone][data-online="true"]', { timeout: 10000 });

  // A real call, long enough for each voice to play out.
  await caller.goto(BASE + `/call/${LINE}`);
  await caller.click('[data-action="call"]:visible');
  await caller.waitForSelector('[data-key="1"]:visible', { timeout: 60000 });
  await caller.click('[data-key="1"]');
  await owner.waitForSelector('[data-action="accept"]:visible', { timeout: 30000 });
  await owner.click('[data-action="accept"]');
  await caller.waitForSelector("text=On the line with Auberge du Lac", { timeout: 20000 });
  await caller.waitForTimeout(32000);
  await caller.click('[data-action="hangup"]:visible');
  await owner.waitForSelector("[data-call-screen]", { state: "hidden", timeout: 10000 });
  const call = await finished("call");
  show(call);
  check(call?.status === "done", `the call has its note (${call?.status} ${call?.reason})`);
  const said = JSON.parse(call?.transcript || "[]");
  check(said.some((l) => l.speaker === "caller" && /Tremblay/i.test(l.text)), "the caller's words are under the caller");
  check(said.some((l) => l.speaker === "owner" && /auberge|chambre/i.test(l.text)), "the owner's words are under the owner");
  check(/Tremblay/.test(call?.summary) && /\b(a|est|elle|chambre)\b/i.test(call?.summary), "the summary is in French and names the caller");
  check(call?.audio === "", "the call's recording is gone");

  // A real voicemail: the owner declines.
  await caller.waitForSelector('[data-action="call"]:visible');
  await caller.click('[data-action="call"]:visible');
  await caller.waitForSelector('[data-key="1"]:visible', { timeout: 60000 });
  await caller.click('[data-key="1"]');
  await owner.waitForSelector('[data-action="decline"]:visible', { timeout: 30000 });
  await owner.click('[data-action="decline"]');
  await caller.waitForSelector('[data-action="send-message"]:enabled', { timeout: 60000 });
  await caller.waitForTimeout(20000);
  await caller.click('[data-action="send-message"]');
  await caller.waitForSelector("text=Message sent", { timeout: 15000 });
  const message = await finished("voicemail");
  show(message);
  check(message?.status === "done" && /Tremblay/.test(message?.summary), `the voicemail has its note (${message?.status} ${message?.reason})`);

  // What the owner sees.
  await owner.goto(BASE + "/dashboard/notes");
  await owner.waitForSelector("[data-notes]");
  await owner.screenshot({ path: `${SHOTS}/real-notes.png`, fullPage: true });
  await owner.goto(BASE + `/dashboard/notes/${call?.id}`);
  check(/Tremblay/.test(await owner.textContent("[data-note-summary]")), "the Notes tab shows the summary");
  check(await owner.locator("[data-note-actions] li").count() > 0, "and the next actions");
  check((await owner.textContent("[data-note]")).includes("Faits saillants") && (await owner.textContent("[data-note]")).includes("Transcription complète"), "in French, in two sections");
  check(/Tremblay/.test(await owner.textContent("[data-note-transcript]")), "and the transcript");
  await owner.screenshot({ path: `${SHOTS}/real-note.png`, fullPage: true });
  await owner.setViewportSize({ width: 390, height: 844 });
  await owner.screenshot({ path: `${SHOTS}/real-note-phone.png`, fullPage: true });
} catch (e) {
  failures.push(String(e));
  console.log(String(e));
} finally {
  await ownerBrowser.close();
  await callerBrowser.close();
  rmSync(voices, { recursive: true, force: true });
  const db = new DatabaseSync(DB);
  db.exec("PRAGMA foreign_keys = ON");
  const owned = "IN (SELECT l.id FROM line l JOIN account a ON a.id = l.account_id WHERE a.name = ?)";
  for (const v of db.prepare(`SELECT file FROM voicemail WHERE line_id ${owned}`).all(EMAIL)) rmSync(join(dirname(DB), "voicemail", v.file), { force: true });
  for (const n of db.prepare(`SELECT audio FROM note WHERE line_id ${owned} AND audio LIKE 'notes/%'`).all(EMAIL)) rmSync(join(dirname(DB), n.audio), { force: true });
  db.prepare("DELETE FROM account WHERE name = ?").run(EMAIL);
  db.prepare("DELETE FROM push_log WHERE line = ?").run(LINE);
  db.prepare("UPDATE account SET promo_code = NULL WHERE promo_code = ?").run(CODE);
  db.prepare("DELETE FROM promo_code WHERE code = ?").run(CODE);
  db.close();
}
console.log(failures.length ? `\n${failures.length} problem(s):\n- ${failures.join("\n- ")}` : "\nAll good.");
process.exit(failures.length ? 1 : 0);
