# minivoip

research and proof of concept about a new way to offer voice assistance without the real plain old telephony line.

## Current state

Keep this short and up to date (status, next action, last update).

- Status: Android app works (2026-10-24, not committed): Ken's locked Pixel 9 rang with his ringtone and the call screen within 1 s, answered, voice both ways. `android/` (Kotlin web view on `/app` + native ring through Firebase, project `minivoip-push-kd`, files in `data/firebase/`), served at `/download/minivoip.apk`, installed over USB (adb). Computer app works too. `go test`, `app.mjs`, `site.mjs` pass.
- Next action: commit; then iPhone (same idea: Apple call screen, $99/yr Apple account, rented Mac; family iPhones to test). Still pending: landing page demo phone redesign, security (anyone who knows `/call/<line>` can call), back up `android/release-keystore.jks` + `android/local.properties`.
- Last update: 2026-10-24. Pixel 9 test: web push arrived in 0.5 s while locked but silent; decided with Ken: native shells (Kotlin Android now, iPhone next), same app pages inside (`docs/vision.md`). Built `android/`, `push/fcm.go` (FCM HTTP v1, stdlib JWT), `GET /api/android/config`, `POST /app/calls/{id}/decline` (hub `Decline`), `/download/minivoip.apk`; Android signing key in `android/` (gitignored, back it up). Computer test passed on the second try.

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
