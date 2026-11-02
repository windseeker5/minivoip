# minivoip

research and proof of concept about a new way to offer voice assistance without the real plain old telephony line.

## Current state

Keep this short and up to date (status, next action, last update).

- Status: healthy. `main` has everything up to PR #20: calls, owner's app, Android 0.3, Premium video and room, plans, logos, website Call button, limits, call window, the landing page's message, notifications that name the business (#15), the call window's menu wave (#16), the plain hero with five steps (#17) and its polish (#18), and the redesign (#20): log in polish, the dashboard with a side menu, and the owner's app in the dashboard's look. `hero-motion` (the rejected 3D film) is kept unmerged until Ken says to delete it.
- Next action: from `docs/plan.md`, the container and home server. Parked: the Android app's native ring screen still has the old look C; polish of the admin and sign up looks; the dashboard and app still say Premium "opens after the beta"; AI call notes (on the landing page as "Soon"). Ken should back up `android/release-keystore.jks` + `android/local.properties`.
- Last update: 2026-11-02. Shipped PR #20 (the four stacked branches `login-polish`, `dashboard-fixes`, `dashboard-redesign`, `app-redesign`): the owner's app redone as Ken's pick, proposition 1 of three (https://claude.ai/artifact/QUHgeBgYVQ15wUVRSnvCCw), in the dashboard's look (DESIGN.md "Owner app"): four tabs, the account from the round letter, a green/amber/white Available band, plain words on every call (also in the dashboard's Latest calls), a message player with the real voice drawn as bars, a white incoming call screen. Before it, the dashboard as proposition 2 (menu on the side), the audit's seven fixes and the log in polish. `app.mjs` and `site.mjs` pass.

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
