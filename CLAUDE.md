# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

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

## How a call works in the code

- `lines/`: the lines and their voice menus (one test line, `joesbnb`, in code until Phase 2 moves it to SQLite).
- `calls/hub.go`: the WebSocket at `/ws?line=&role=visitor|owner`. It only passes messages (call, ringing,
  incoming, accept, offer/answer/ice, hangup, no-answer, busy); one call per line at a time, kept in memory.
  `calls/hub_test.go` covers answered, unanswered and busy calls.
- `/call/{line}` (`pages/call.templ` + `assets/js/call.js`): the visitor's call window. Plays the menu with the
  browser's voice; a "ring" choice asks for the microphone, opens the socket and makes the WebRTC offer.
- `/app` (`pages/call.templ` `Phone` + `assets/js/phone.js`): the owner's phone app. Stays connected, rings,
  answers. No push or login yet, so the page must be open.
- No STUN/TURN yet (`iceServers: []`): works on one machine or one Wi-Fi. A phone needs HTTPS for the microphone.

## Commands

The Go tools live in `~/go/bin` (on PATH via `~/.zshrc`). The project was scaffolded with
`shadcn-templ init --template templ`; `Taskfile.yml` holds the tasks.

- `task dev` (**Ken runs this himself; never start, stop or restart it**, see AGENTS.md): runs the templ, Tailwind and component-bundle watchers. Open http://localhost:7331 (auto-refreshing
  proxy in front of the real app on 8090). Microphone and push work on localhost without HTTPS.
  - The Taskfile looks for the next free port with `nc`, which isn't installed here, so it always picks 8090/7331.
    If those are busy, run `task dev PORT=8091 PROXY_PORT=7332`.
- `shadcn-templ add <component …>`: copy in more components (into `components/`; they are our code to edit).
- `task build`: CSS, JS bundle, `templ generate`, then `bin/app`. Don't run it (or `templ generate`) while `task dev` is running; see AGENTS.md.
- `go test ./...`: tests (`pages/home_test.go` checks the landing page renders). `go test ./pages -run TestHomeRenders` for one.
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
