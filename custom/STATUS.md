# Option 1: custom (homemade) — status

Last updated 2026-10-20. Built 2026-10-19; source code is in `custom/callbox/`.

## How it works, in plain words

- **The call is fake, but the voice is real.** A visitor clicks "Call me" and gets a full-screen call screen.
  - The ringback, the keypad tones and the French voice menu all play inside their browser.
  - Keys 1 and 2 play a message and show a link card.
  - Nothing reaches the server until the visitor presses 0.
- **Pressing 0:**
  - The page asks for the microphone.
  - The server sends a notification (Web Push) to the owner's phone, again every 10 s, for up to 45 s.
- **The owner** has the "Appels OMT" web app installed on their phone's Home Screen. They tap the notification and
  then **Répondre**.
  - The two then talk browser to browser (WebRTC).
  - When a direct route isn't possible, the audio goes through our own relay (coturn).
- **No answer after 45 s:** the visitor leaves a voicemail, which the owner plays in the app.
- **No phone number, no Asterisk, no SIP, $0.** A picture of all this is in `explainer.html`.

## What is built and where

| Piece | Where |
|---|---|
| Server (Flask, WebSocket, push, voicemail, rate limits) | `custom/callbox/app.py` |
| Visitor call screen (any `[data-appel]` link opens it) | `custom/callbox/static/appel/widget.js`, `widget.css`; standalone page `index.html` |
| Owner app (installable web app) | `custom/callbox/static/owner/` (`owner.js`, `sw.js` for notifications, `manifest.webmanifest`) |
| Menu text and links (fr-CA) | `custom/callbox/menu.json` (mounted live; restart `callbox` after editing) |
| French voice prompts (MP3, voice fr-CA-SylvieNeural) | `custom/callbox/static/appel/audio/`, made by `custom/callbox/make_audio.py` |
| Secrets (passcode, VAPID keys, TURN secret) | `custom/callbox/.env` (chmod 600, never commit) |
| Relay config | `custom/callbox/turnserver.conf` (gitignored; template `turnserver.conf.example`) |
| Containers | `callbox` (127.0.0.1:8510) and `coturn` (coturn/coturn:4.7.0, host network) in `~/Documents/DEV/docker-compose.yml` |
| Web routing | `~/Documents/DEV/caddy/Caddyfile`, omt.dresdell.com block: `/appel*` and `/call/*` go to callbox |
| Backups taken before the changes | `docker-compose.yml.bak-before-callbox`, `docker-compose.yml.bak-before-minivoip-move` (2026-10-20 move), `Caddyfile.bak-before-callbox` |
| Headless end-to-end test | `tools/e2e.mjs` (see "Testing" below) |

**On the OMT site** (`~/Documents/DEV/OMT`): these changes are live but **not committed**:
- `templates/home.html` has the "Call me" pill (`.omt-listen-call`, `data-appel`) and loads `/appel/widget.css` and `/appel/widget.js`.
- `static/omt.css` styles the pill.
- `templates/base.html` bumps the cache-bust to 30.9.

`custom/callbox/` is not a git repo yet.

Rebuild after a code change (from `~/Documents/DEV`): `docker compose up -d --build callbox`.
Logs: `docker logs -f callbox`. They show every call, push result (`push ring 1 -> 201 in 156 ms`), and the
browsers' connection reports (`diag …` lines with network type, route and audio bytes received).

## Home network setup (done)

- **Router** (TELUS, 192.168.1.254), port forwards to this PC (192.168.1.89):
  - `callbox-turn`: TCP/UDP 3478
  - `callbox-voice`: UDP 49160–49200
- **Public IP** 75.158.253.160, written into `turnserver.conf` (`external-ip`). If the ISP changes it, update it
  there and restart coturn.
- **ufw:** the commands `sudo ufw allow 3478` and `sudo ufw allow 49160:49200/udp` were given to Ken. It isn't
  confirmed that he ran them. The relay works anyway.
- **Gotcha:** from inside the house, all traffic shows up as coming from 192.168.1.254 (the router's loopback), so
  every home device shares one rate limit. The call limit was raised from 3 to 20 per 10 minutes for that reason.

## Test results so far (Ken's Android as the owner)

| Test | Result |
|---|---|
| Menu, keys 1 / 2 / 0, French voice | Works (desktop Chrome as the visitor) |
| Voicemail when nobody answers | Works (tested headless) |
| Audio through the relay (forced) | Works after the router forwards were added. Before that: no sound. |
| Owner on Wi-Fi | Works |
| Owner on **cellular**, Wi-Fi off | **Works, two-way audio.** Found a direct route; the relay wasn't needed. |
| Notification speed | Push delivered in about 160 ms; second push 10 s later |
| **Locked phone** | **Weak spot.** One short notification "ding", like a Discord message, then nothing. The second push doesn't make a sound again. Tapping the notification opens the app, which then rings, and Ken can answer. |
| iPhone (girlfriend's) | **Not tested yet** |

## Open problem: the locked phone doesn't really ring

A web app can only show a normal notification. Only real phone apps (dialer, WhatsApp, Teams) can ring
non-stop with a full call screen. Ideas, none done yet:

1. **A different notification tag per push on Android.** Today it reuses `call-<id>` with `renotify: true` in
   `sw.js`. With a new tag each time, it would ding every 10 s instead of once.
2. **Android settings** for the Appels notifications: set a ringtone as the sound, turn on "pop on screen", and
   turn off "notification cooldown".
3. **ntfy** (a free, open-source app): its "keep alerting for highest priority" setting rings until you touch the
   phone. It works on Android only; iPhone doesn't allow this for free apps. It could be self-hosted as a third
   container.

This is the main pass/fail point of the test. If a locked phone can't ring properly, the friend's B&B is better
off with Zadarma.

## Testing

- **By hand:**
  - Visitor: a computer or a second phone at https://omt.dresdell.com/ → Call me → 0.
  - Owner: the phone with the app installed, locked, ideally on cellular.
- **Headless:** `tools/e2e.mjs` plays both sides in headless Chrome with fake mic audio. `--relay` forces the relay.
  - Needs `npm i playwright-core` in `tools/`, the cached `~/.cache/ms-playwright/chromium_headless_shell-1243`, and a
    `pass.txt` in `tools/` holding the owner passcode. `pass.txt` is gitignored.
  - Never use the Playwright MCP: it pops windows on Ken's desktop.

## Still to do

- [ ] Fix or accept the locked-phone ring (ideas above).
- [ ] iPhone test: Safari → omt.dresdell.com/call/owner/ → Share → Add to Home Screen → open it → enable notifications. Needs iOS 16.4+.
- [ ] Fill in the scorecard in `test-plan.md` §11.
- [ ] Delete the leftover test voicemail (in the owner app, **Supprimer**).
- [ ] Decide whether to commit the OMT "Call me" button (in the OMT repo). `minivoip` is a local git repo since 2026-10-20 (no remote yet).

## Beyond the B&B: a SaaS for small businesses (ideas only, nothing decided)

The pitch: "a call button on your website, no phone number, no phone system". What would change from the POC:

- **More than one business:**
  - Each business gets its own menu, prompts, owner accounts and voicemail.
  - Today there is one menu, one passcode, and one call at a time.
- **Hosting:** a VPS instead of Ken's home PC, which removes the home IP, router and uptime problems. coturn moves
  with it.
- **The ring problem above** has to be solved first. It's the difference between "nice" and "I'd pay for this".
- **Text-to-speech** so a business can type its own messages. The MP3s are made with edge-tts today.
- **An embed snippet** (one `<script>` tag) for any website, plus billing.
