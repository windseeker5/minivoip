# minivoip

research and proof of concept about a new way to offer voice assistance without the real plain old telephony line.

## Current state

Keep this short and up to date (status, next action, last update).

- Status: healthy, everything on `main` (PR #8, merged 2026-10-27): calls end to end, the owner's app (look C), the Android app that rings (0.3), the landing page (look "Voice"), the call window (look A), Premium video / screen sharing / whiteboard and the meeting room, Basic and Premium plans, business logos with a crop window, and the website Call button (`w.js`, here or away). Plan and progress: `docs/plan.md` "Where we are".
- Next action: Ken tries the website Call button on a real page and gives feedback (he parked cosmetic polish). Then, from `docs/plan.md`: security (call limits, robots), then the container and home server. Open: the landing page still says Premium is "coming soon"; no payments yet. Ken should back up `android/release-keystore.jks` + `android/local.properties`.
- Last update: 2026-10-27. Shipped PR #8 (stack of premium-video, plans, stage-fixes, call-button). Migrations 8 (plans) and 9 (`line.logo`) already ran on the dev database. Ken's line `ken` is Premium and admin, with his logo. All tests pass: `go vet`, `go test`, `site.mjs`, `meet.mjs`, `call.mjs`, `app.mjs`.

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
