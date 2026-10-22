# minivoip

research and proof of concept about a new way to offer voice assistance without the real plain old telephony line.

## Current state

Keep this short and up to date (status, next action, last update).

- Status: Accounts and menus work: sign up (with promo code), log in, create a line, edit the voice menu (typed text, computer voice), call page and phone app use each user's own line. Calls work on one Wi-Fi. Tests pass (`go test ./...`).
- Next action: Ken picks the next feature: voicemail, recorded greetings, waking the phone (push), or the promo-code admin page. Run `node tests/e2e/site.mjs` after every change.
- Last update: 2026-10-21. Built the landing page and logo; ported the POC's call setup to Go (`calls/`, `lines/`, `pages/call.templ`, `assets/js/call.js`, `phone.js`, new dependency `github.com/coder/websocket`). Removed the old POC containers, Caddy route and OMT "Call me" button. `OLD-ARCHIVE/` was deleted outside this session; the POC stays in git commit 5b8d116. Committed on branch `go-version` (a7b8724). Added https://voip.dresdell.com (Caddy block, DNS by Ken) pointing at the dev app; test call through it passed. Added the database (`store/`, `docs/data-dictionary.md`), sign up/log in (`web/`), line creation and menu editor; the phone app now needs the line owner's login. New dependencies: `modernc.org/sqlite`, `golang.org/x/crypto`. Linked all pages (Log in / My dashboard on the landing page, way back from the phone app), added CLAUDE.md rules 1-3 (don't run the server, test your own work, build connected features) and the whole-site browser test `tests/e2e/site.mjs` (passes locally and through voip.dresdell.com).

## Rules

### Workflow

#### Never start, stop or restart the Go server

- Ken runs the dev server (`task dev`) himself, in his own terminal. Never start it, stop it, restart it or run it in the background, not even "just to check".
- To check a change, ask Ken to look at http://localhost:7331, or run `go test ./...` / `go vet ./...`, which compile without starting a server.
- If something needs a restart, say so and let Ken do it.
- Never run `templ generate` (or `task build`) while Ken's `task dev` is running: it overwrites the watcher's dev-mode files and the page shows "templ: failed to render template" until the next real change to each `.templ` file makes the watcher regenerate it (no restart needed). Check code with `go test ./...` and `go vet ./...` instead.

#### Update docs in the same change

- When you change code, config, a query, a schema or behaviour, update the documentation that describes it (README, data dictionary, design doc, AGENTS.md) in the same change — never "later".
- When you add a column or field, update the data dictionary and its version-history table.
- If a doc you rely on turns out to be wrong, fix it as part of the work and mention it.
- Documentation-only edits are verified by re-reading the file.

#### Keep Current state up to date

At the end of every work session (or when the user wraps up), update the `## Current state` section of this project's AGENTS.md:
- **Status** — one or two lines on where things stand (healthy, blocked, in progress).
- **Next action** — the single next concrete step.
- **Last update** — today's date plus what changed (last PR, last major operation, open issues).
Keep it short and factual; replace stale lines rather than appending a log. Read this section first at the start of each session and never assume a blocker listed there has been resolved without checking.

### Style

#### Reuse before writing new code

- Before writing a utility, component or script, search the repo (and any shared component library named in AGENTS.md) for one that already does the job, and use it.
- Never hand-roll a parallel version of something that exists. If nothing fits, say so explicitly before writing new code and keep it minimal.
- Touch only files in scope for the task; leave deprecated or off-limits areas listed in AGENTS.md alone.
- Don't add new dependencies, build steps or frameworks without asking.
<!-- adna-kit:end -->
