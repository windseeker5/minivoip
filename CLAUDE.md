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
3. **Build connected features.** Every new page is linked from the pages around it (landing page, log in,
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
  same-site check on form posts; `dashboard.go` has "My line", line creation, the menu editor, the phone app and
  the call page.
- `store/`: SQLite (`modernc.org/sqlite`, pure Go). Schema as numbered migrations in `store/store.go` (tracked with
  `PRAGMA user_version`; add a migration, never edit a shipped one) and documented in `docs/data-dictionary.md`.
  Passwords use bcrypt. The dev database is `data/minivoip.db` (gitignored).
- `lines/`: the Line/Option types, menu actions (ring, play, link, voicemail), the spoken menu text and line-name
  rules. `choicePhrase` is mirrored in `assets/js/dashboard.js`.
- `calls/hub.go`: the WebSocket at `/ws?line=&role=visitor|owner`. Only passes messages (call, ringing, ready,
  incoming, accept, offer/answer/ice, hangup, no-answer, busy); one call per line, kept in memory. Only the line's
  logged-in owner may connect as `owner` (`CanAnswer`).
- Pages (`pages/*.templ`) and their scripts (`assets/js/`): `home` + `landing.js` (landing page),
  `account` (sign up, log in), `dashboard` + `dashboard.js` (My line, menu editor), `call` + `call.js` (visitor's
  call window `/call/{line}`) and `Phone` + `phone.js` (phone app `/app`, login required, must stay open: no push yet).
- No STUN/TURN yet (`iceServers: []`): calls work on one machine or one Wi-Fi. A phone needs HTTPS for the microphone.
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
- `go test ./...`: tests (`store/` accounts, promo codes, lines; `calls/` call setup; `pages/` landing page renders).
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
