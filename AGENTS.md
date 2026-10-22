# minivoip

research and proof of concept about a new way to offer voice assistance without the real plain old telephony line.

## Current state

Keep this short and up to date (status, next action, last update).

- Status: Working end to end; go test + `tests/e2e/site.mjs` all green (2026-10-22, with the new push checks). Real-phone finding: with Ken's Android locked, Google accepts the push alerts but holds them until the phone is unlocked (3+ min late, `push_log` rows 4-8): Android battery saving keeps Chrome asleep. App on screen: alerts arrive in under 0.5 s. Our side works (late ring shown as "Missed call", queued rings collapsed).
- Next action: Ken sets Settings → Apps → Chrome (and minivoip, if listed) → Battery → Unrestricted, locks the phone 2 min, calls `/call/ken` and lets it ring out; then read `push_log` (`sqlite3 -readonly data/minivoip.db "select * from push_log where line='ken' order by id"`). Ask the phone brand (Samsung/Xiaomi have extra "sleeping apps" lists). If that fixes it, it becomes step 1 of the "Set up your ringing" guide + "Test my ringing" call (vision step 1-2, `docs/vision.md`). Still pending: landing page review, design pass on other screens, security (anyone who knows `/call/<line>` can call).
- Last update: 2026-10-22. Long-term vision in `docs/vision.md` (web app only, meeting room, whiteboard, notes, Voice and Meetings plans). Ringing fixes: rings every 10 s (was 5), ring pushes wait up to a minute and collapse (`Topic`), late ring shows "Missed call", answering clears the ring on other devices (`OnAnswered`), app ringtone unlocked on first tap, delivery receipts (`push_log`, migration 6, `POST /app/push/receipt`). `go mod tidy` added missing go.sum checksums (templ tool wouldn't start). Nothing committed yet. 2026-10-21: new landing page, TURN relay, phone-style screens. Public repo: https://github.com/windseeker5/minivoip.

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
