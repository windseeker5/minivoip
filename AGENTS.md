# minivoip

research and proof of concept about a new way to offer voice assistance without the real plain old telephony line.

## Current state

Keep this short and up to date (status, next action, last update).

- Status: healthy. `main` has calls end to end, the owner's app (look C), the Android app that rings, the landing page in look "Voice" and the visitor's call window (`/call/<line>`) in look A. Nothing waiting on a branch. Plan and progress: `docs/plan.md` "Where we are".
- Next action: step 2 of the call window: the call window as a panel in the corner of the business's website (`w.js`); then the meeting room (video and screen sharing in the call first). Ken re-records his line's greeting and menu (it still says "Hi, you've reached OMT", one choice). Ken should back up `android/release-keystore.jks` + `android/local.properties`.
- Last update: 2026-10-25. Shipped a landing hero fix (PR #6: on a second visit the words didn't follow the voice; `site.mjs` now checks it). Before that, the call window (PR #5) after `go vet`, `go test`, `call.mjs`, `app.mjs` and `site.mjs` all passed. Before that: critique + audit of the old call window (21/40 design, 10/20 code, `.impeccable/critique/`), three propositions (https://claude.ai/artifact/UefEpYx2avVwMxAyRekqLS), Ken picked A. Built in `pages/call.templ`, `assets/css/callscreen.css`, `assets/js/call.js`, new `layouts/plain.templ`; DESIGN.md "Call window", CLAUDE.md updated; new `tests/e2e/call.mjs`, `site.mjs` updated.

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
