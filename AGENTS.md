# minivoip

research and proof of concept about a new way to offer voice assistance without the real plain old telephony line.

## Current state

Keep this short and up to date (status, next action, last update).

- Status: healthy. `main` has everything up to PR #18: calls, owner's app, Android 0.3, Premium video and room, plans, logos, website Call button, limits, call window, the landing page's message, notifications that name the business (#15), the call window's menu wave (#16) the plain hero with five steps (#17) and its polish (#18). `hero-motion` (the rejected 3D film) is kept unmerged until Ken says to delete it.
- Next action: from `docs/plan.md`, the container and home server. Parked: the app's UI redesign (Ken doesn't like it yet), polish of the admin, sign up and button looks; the dashboard and app still say Premium "opens after the beta"; AI call notes (on the landing page as "Soon"). Ken should back up `android/release-keystore.jks` + `android/local.properties`.
- Last update: 2026-11-01. Branch `dashboard-fixes` (on top of `login-polish`, neither shipped): the dashboard audit's seven fixes (links and code shown whole, Copy with a check and a spoken "Copied", one Add my logo button, 44px buttons on a phone, a clear focus outline on every page, no htmx or component bundle on the dashboard pages, Cropper loaded on the first pick); `site.mjs` checks each. Before that, branch `login-polish`: the log in and sign up card sits in the middle on a computer, fields 40px, and saved emails and passwords no longer show on dark grey-blue (`color-scheme` + autofill rule in `globals.css`); `site.mjs` checks both. Before that: PR #18 merged (hero polish): the hero is centred on a computer, its headline comes in word by word with a line in the logo's colours under "in one click.", the steps come in one by one along a line that fills in the logo's colours, with a faint grey voice wave behind it from Call us to the screen (canvas in `landing.js`), the hero's button is just "Call us" and rings every 6 s, and the orange band is shorter so the phone stands out of it above and below. DESIGN.md updated; `site.mjs` checks the wave. Earlier today: PRs #15, #16, #17 merged.

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
