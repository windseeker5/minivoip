# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Rules that come first

1. **Never start, stop or restart the Go server.** Ken runs `task dev` himself. Don't run it in the background
   either, and don't run `templ generate` or `task build` while it runs (see AGENTS.md).
2. **Test your own work, every time, before saying it's done. Never ask Ken to do manual or routine testing.**
   Use every tool it takes:
   - `go vet ./...` and `go test ./...`;
   - the browser test `node tests/e2e/site.mjs` against Ken's running server (it clicks through the real site:
     every link, sign up, line, menu editor, a real call between two browsers, log out; then deletes its test
     account), and extend it whenever you add a page or a flow;
   - screenshots you actually look at (desktop and phone width).

   Browsers run headless only (`playwright-core` with the cached `chromium_headless_shell`), never a visible
   window and never the Playwright MCP. Only ask Ken for what a machine can't do, like his real phone or his ears.
3. **Every feature or bug fix happens on its own branch**, never straight on `main`. When Ken says "ship it", do it
   all yourself with the `gh` command line: push the branch, open a pull request on GitHub, merge it into `main`,
   then switch back to an up-to-date `main` locally. Don't ship (or push) before Ken says so.
4. **Build connected features.** Every new page is linked from the pages around it (landing page, log in,
   dashboard, phone app), and the whole path is tested from the landing page, not just the page in isolation.

@AGENTS.md

AGENTS.md (imported above) holds the project rules and the **Current state** section. Read Current state first and
keep it updated at the end of each session.

## What this is

minivoip: a "Call us" button for small-business websites. The visitor hears the owner's voice menu and can talk to
the owner's phone app (a PWA) or leave a voicemail, with no phone line involved. It started for a friend's unstaffed
B&B and may become a SaaS.

## Where things are

- `docs/architecture.md`: the design and its decisions (stack, call flow, data tables, ports, "growing later").
- `PRODUCT.md` (who it is for, brand rules, Ken's design taste) and `DESIGN.md` (how it looks: type, colours, the landing components). Read both before any screen work.
- `docs/vision.md`: the long-term goal (calls, then meeting room, whiteboard, notes; two plans; web app only). Read before planning features.
- `docs/plan.md`: the build phases, 0 to 6. Work follows this order.
- `docs/research.md`: background, existing projects, competitors, Go component choice.
- The first proof of concept (Python/Flask + coturn "callbox", and a Zadarma evaluation) lives only in git
  commit `5b8d116`, under `custom/` and `zadarma/` (`git show 5b8d116:custom/callbox/app.py`). Reference only;
  the Go code ports its call messages and browser call code. Its secrets were never in git.

## Design rules that span the code

- **One Go program** serves everything: pages, the phone app, `w.js`, the API, call setup (WebSockets), push sending
  and the built-in TURN relay (`pion/turn`). Database is one SQLite file (`modernc.org/sqlite`, pure Go, no cgo).
- **The server never handles audio for web calls.** The voice menu and voicemail recording run in the visitor's
  browser, and the call itself is direct WebRTC between visitor and phone app. The server only passes setup messages.
  Don't add server-side audio for web calls; that is reserved for the later SIP steps.
- **Pages** are templ + shadcn-templ (components are copied into the project) + Tailwind v4 standalone (no Node).
  Plain JavaScript for dynamic bits; htmx only if it clearly helps. Reuse a shadcn-templ component before writing one.
- Secrets (VAPID keys, etc.) go in `.env`, never in git; ship a `.env.example`.
- Container comes last (phase 6). Develop locally first.

## How the code fits together

- `main.go` opens the database and hands everything to `web.Server.Routes`.
- `web/`: every route. `web.go` has login state (session cookie `mv_session`), sign up, log in, log out, and a
  same-site check on form posts; `dashboard.go` has "My line", line creation, the menu editor and the call page;
  `app.go` the owner's app (`/app`, its call list, the Available switch, call history logging).
- `store/`: SQLite (`modernc.org/sqlite`, pure Go). Schema as numbered migrations in `store/store.go` (tracked with
  `PRAGMA user_version`; add a migration, never edit a shipped one) and documented in `docs/data-dictionary.md`.
  Passwords use bcrypt. The dev database is `data/minivoip.db` (gitignored).
- `lines/`: the Line/Option types, menu actions (ring, play, link, voicemail), the spoken menu text and line-name
  rules. `choicePhrase` is mirrored in `assets/js/dashboard.js`.
- `calls/hub.go`: the WebSocket at `/ws?line=&role=visitor|owner`. Only passes messages (call, ringing, ready,
  incoming, accept, offer/answer/ice, hangup, no-answer, busy); one call per line, kept in memory. Only the line's
  logged-in owner may connect as `owner` (`CanAnswer`). It hands the caller a one-time voicemail token on
  "no-answer" or a voicemail choice; `web/voicemail.go` takes the upload (`POST /api/voicemail`) and tells the
  phone app (`NotifyOwners`). Every finished call goes to `OnEnd` (saved in `call_log`, the app's call history);
  `Available` (the app's Available switch, `line.away_until`) sends calls straight to voicemail while away.
- Video, screen sharing and the whiteboard: `assets/js/meet.js` (`MinivoipMeet.Session`) and `assets/css/stage.css`,
  used by `call.js`, `phone.js` and `room.js`. Every connection opens two video lanes (camera, screen) and a data
  channel up front, so turning a camera on is a track swap, never a new offer; the data channel carries who has what
  on and the whiteboard's lines. Nothing new goes through the server for calls.
- Meeting rooms (Premium lines only): `/room/{line}` (`roomPage` in `web/dashboard.go`, `pages/room.templ`, `assets/js/room.js`,
  `assets/css/room.css`) and their socket `/ws/room` (`calls/room.go`, `Rooms`): guests wait until the line's
  logged-in owner opens the room, then everyone connects to everyone (up to `RoomSize`, 4). The owner's apps get a
  "room" message on the call socket with who waits (the app's alert and Room tab), and phones a "room" push
  (`RoomKnock`). Linked from the dashboard and the app's Room tab.
- Pages (`pages/*.templ`) and their scripts (`assets/js/`): `home` + `landing.js` (landing page),
  `account` (sign up, log in), `dashboard` + `dashboard.js` (My line, menu editor with voice recorder: uploads to
  `POST /dashboard/prompt`, `store/prompts.go`), `voicemail` + `voicemail.js`
  (the dashboard's list), `call` + `call.js` (visitor's call window `/call/{line}`) and `app` + `phone.js`
  (the owner's app `/app`, login required: Calls, Voicemail, Menu, Room and Me tabs on a phone; sidebar and a detail
  pane on a computer; the incoming call screen).
- The owner's app has its own plain CSS, `assets/css/app.css` (look C, warm light, DESIGN.md "Owner app"); the
  visitor's call window has `assets/css/callscreen.css` (look A, white, DESIGN.md "Call window") and its own bare
  layout, `layouts.Plain`. The app never links to the dashboard;
  logging in from `/app` uses `AppLogin` (`/login?next=/app`) and stays in the app.
- Checking the app without the server: `node tests/e2e/app.mjs` renders it with sample calls
  (`TestAppPreview` in `pages/app_test.go`) and clicks through it headless with a fake call socket, at phone and
  computer width (screenshots `tests/e2e/shots/app-*.png`). Video, screens and the whiteboard in a call:
  `node tests/e2e/meet.mjs` (the call window and the app connected by a fake call socket in the script, a real
  WebRTC call between two headless browsers, fake camera and screen; `shots/meet-*.png`). The visitor's call window the same way:
  `node tests/e2e/call.mjs` (`TestCallPreview` in `pages/call_test.go`; every step with a fake call socket and a fake
  browser voice, phone, computer and 200% zoom; `shots/call-*.png`). Real calls are only tested by `site.mjs`.
- `push/`: web push (VAPID keys in the `setting` table, `webpush-go`), and Firebase for the Android app
  (`push/fcm.go`: devices with endpoint `fcm:<token>`; the Firebase project `minivoip-push-kd` on Ken's Google account,
  its two files in `data/firebase/`, never in git; `GET /api/android/config` hands the app its settings). The hub's `OnRing` sends "Incoming call"
  every 10 s until answered (30 s max), `OnAnswered` clears it on other devices, `OnMissed` "Missed call", a new
  voicemail "New voicemail". Every push is logged in `push_log` and the phone sends a receipt back
  (`POST /app/push/receipt`), so `push_log` shows what each phone really got and how late. The service worker
  is `assets/js/sw.js`, served at `/sw.js`; the install manifest is `assets/brand/manifest.webmanifest` at
  `/manifest.webmanifest`. The phone app's notification panel is in `phone.js`.
- The Call button for a business's website: `assets/js/w.js`, served at `/w.js` (`main.go`); the dashboard shows
  the line to paste. It reads `GET /api/status/<line>` (`web/status.go`: `here` = Available, a ringing choice, and an
  open app or a push device; `pages.Presence`), and so does the call page (`data-here`, `cw-if-here`/`cw-if-away`).
  On a computer the call page opens in an iframe panel; call.js tells it to close or grow (`postMessage`).
  Tested by `site.mjs` ("website button": a fake business site on another port, a real call from the panel).
- Landing page "Call us now" buttons call a real line: `MINIVOIP_CALL_LINE`, default `ken` (Ken's own account);
  hidden if that line doesn't exist. Tests must never press Call on it: it rings Ken's phone.
- Logos: `line.logo`, files in `data/logos/` (`store/logos.go`), served at `/logo/<name>` (`web/logo.go`). Asked on
  "create your line" (optional), changed on the dashboard (`POST /dashboard/logo`, `/dashboard/logo/remove`).
  `faceBadge` in `pages/call.templ` shows it, else the initials; the stage gets it as `logo` (meet.js). The owner
  frames the picture in a crop window first (a round guide; the logo fills its shape everywhere) (`logoCrop` in `pages/dashboard.templ`, `assets/js/logo.js`, Cropper.js
  1.6.2 copied into `assets/vendor/`, as in minipass); the browser sends a 512 px WebP, so big photos are fine.
- Limits against robots (`web/limits.go`, `Limits`): calls, room knocks, voicemails, wrong passwords and sign ups,
  counted per visitor address in memory (the hub's `AllowCall`/`AllowVoicemail`, the rooms' `AllowKnock`). The address
  is Caddy's `X-Forwarded-For`; visitors on this machine or the home network aren't limited, so `site.mjs` checks
  the limits with made-up addresses in that header. Past the voicemail limit the call page gets "limited".
- Admin: `/admin/promo` (`web/admin.go`, `pages/admin.templ`), admins only (others get 404): users with
  `is_admin` (Ken's account has it), or emails in `MINIVOIP_ADMINS`. Promo codes (each gives a plan) and the
  Accounts list, where Ken moves an account between Basic and Premium.
- Plans: `account.plan` is `basic` or `premium` (`store.Plan`, `store.Billing`; sign up takes the promo code's plan,
  else Basic). `lines.Line.Premium` gates video in calls (`call.templ`, `app.templ` leave out Camera, Screen and
  Whiteboard on Basic), the meeting room (`roomPage` 404s, `Rooms` gets `store.HasRoom`) and the dashboard's and
  app's room panels. Basic owners see a "Premium" tag there; visitors never see an upsell.
- `android/`: the Android app (Kotlin, no Compose; see `android/README.md`): a web view on `/app` (the page finds it
  as `window.MinivoipAndroid` in `phone.js`) plus a native ring (`CallService` gets the Firebase push, `Ring` shows a
  call notification with the ringtone and `RingActivity` over the lock screen; Answer opens `/app?answer=<call>`,
  Decline posts `/app/calls/{id}/decline`). Served to phones at `/download/minivoip.apk` from `data/android/`; its
  version is in `data/android/version`, and the app's Me tab shows the installed version (`MinivoipAndroid.version()`)
  with "Update to …" when a newer one is published.
- `relay/`: built-in TURN relay (pion/turn) on 3478 UDP+TCP, voice on 49160-49200 UDP (the ports the router
  forwards; the old coturn used them). Public address: `MINIVOIP_PUBLIC_IP`, else DNS of `MINIVOIP_TURN_HOST`
  (default voip.dresdell.com). Logins: `GET /api/ice` gives STUN + a 2-hour TURN REST login (secret in the
  `setting` table). `relay/hairpin.go` delivers relay-to-relay voice locally, because the TELUS router rewrites
  hairpin packets. `?relay=1` on `/app` or `/call/...` forces the relay; `window.minivoipCallStats()` shows the
  route and bytes received (the e2e test checks both). A phone needs HTTPS for the microphone.
- Test address: https://voip.dresdell.com points at the dev app (127.0.0.1:8090) through Caddy
  (`~/Documents/DEV/caddy/Caddyfile`, last block). Only up while `task dev` runs.

## Commands

The Go tools live in `~/go/bin` (on PATH via `~/.zshrc`). The project was scaffolded with
`shadcn-templ init --template templ`; `Taskfile.yml` holds the tasks.

- `task dev` (**Ken runs this himself; never start, stop or restart it**, see AGENTS.md): runs the templ, Tailwind and component-bundle watchers. Open http://localhost:7331 (auto-refreshing
  proxy in front of the real app on 8090). Microphone and push work on localhost without HTTPS.
  - The Taskfile looks for the next free port with `nc`, which isn't installed here, so it always picks 8090/7331.
    If those are busy, run `task dev PORT=8091 PROXY_PORT=7332`.
- `shadcn-templ add <component …>`: copy in more components (into `components/`; they are our code to edit).
- `task build`: CSS, JS bundle, `templ generate`, then `bin/app`. Don't run it (or `templ generate`) while `task dev` is running; see AGENTS.md.
- `go test ./...`: tests (`store/` accounts, promo codes, lines; `calls/` call setup and meeting rooms; `pages/` page renders).
  One test: `go test ./store -run TestPromoCodes`.
- `templ` and `shadcn-templ` are also pinned as `tool` lines in `go.mod`; the Taskfile calls them via `go tool`.

Layout: `main.go` (routes), `pages/` and `layouts/` (`.templ` screens), `components/` (shadcn-templ),
`assets/` (CSS, JS and `brand/` logo files, embedded in the binary). `*_templ.go` files are generated from `.templ` files; edit the
`.templ` file, never the generated one.

There is no linter yet.

## Working with Ken

- Plain, short words; no telecom jargon. Explain what happens on whose phone. Use pictures, not ASCII diagrams.
- When Ken is exploring, brainstorm and recommend; build only when told to go.
- Ken does the router and `sudo ufw` himself; give the exact rules to apply.
- Browser checks: headless only, with playwright-core and the cached
  `~/.cache/ms-playwright/chromium_headless_shell-1243`. Never use the Playwright MCP and never launch desktop apps;
  they pop windows on the desktop or block the session.
