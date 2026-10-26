# minivoip

research and proof of concept about a new way to offer voice assistance without the real plain old telephony line.

## Current state

Keep this short and up to date (status, next action, last update).

- Status: healthy. `main` has calls end to end, the owner's app (look C), the Android app that rings, the landing page in look "Voice" and the visitor's call window in look A. Not shipped yet, stacked: `premium-video` (video, screen sharing, whiteboard in calls; a meeting room per line, up to 4 people) on top of it `plans` (every account Basic or Premium; those features are Premium only; promo codes give a plan; the admin page switches plans), and on top of that `stage-fixes` (fixes from Ken's first test of the stage). Plan and progress: `docs/plan.md` "Where we are".
- Next action: Ken tries the stage fixes, then gives his next feedback (starting a call from the web); then "ship it": push `stage-fixes` (it contains `plans` and `premium-video`), PR, merge. The Android app 0.3 (camera allowed, shows its version in the Me tab) is already served from `data/android/minivoip.apk` (`data/android/version` says 0.3; 0.1 kept as `minivoip-0.1.apk.bak`); Ken installs it on his Pixel. Open: the landing page still says Premium is "coming soon". After that: the corner panel (`w.js`). Ken should back up `android/release-keystore.jks` + `android/local.properties`.
- Last update: 2026-10-26. Branch `stage-fixes` (Ken's first test): my own screen shows small as "Your screen" with "Stop sharing"; the whiteboard no longer hides on my side while I share; a click on a small screen or the whiteboard swaps it in. Before that, 2026-10-25: Branch `plans`: migration 8 (`account.plan`, `promo_code.plan`; admins start on Premium, already run on the dev database, backup in `data/minivoip-before-plans.db`), `store.Billing/SetPlan/Accounts/HasRoom`, `lines.Line.Premium`, admin Accounts list, Basic gating in call window, app, dashboard and room. Ken deleted his old account and signed up again (line `ken`, made Premium and admin by SQL; backup `data/minivoip-before-reset.db`). Camera errors now say "No camera found" when there is none (his computer has no webcam). `go vet`, `go test`, `meet.mjs`, `call.mjs`, `app.mjs` and `site.mjs` all pass. Before that: `premium-video` (meet.js, rooms).

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
