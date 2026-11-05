# minivoip

research and proof of concept about a new way to offer voice assistance without the real plain old telephony line.

## Current state

Keep this short and up to date (status, next action, last update).

- Status: Premium notes shipped on `main` (calls, meetings, voicemails: highlights with next actions, then the full transcript; French unless the account picks English; the rest of the site stays in English). Running as a demo with `MINIVOIP_NOTES_LOCAL=1` in `.env`: graphics card and Claude Code login, no keys. `hero-motion` (the rejected 3D film) is kept unmerged until Ken says to delete it.
- Next action: Ken tries it on his real line and installs Android 0.5 for the checks under "Real phones" in `docs/notes-plan.md`; he still has to read the words callers are told (`lines.Notice`) and check the recording rules where he operates. Before real customers: API keys (`.env.example`), since his Max plan is not for serving them. Then from `docs/plan.md`, the container and home server. Parked: the Android app's native ring screen still has the old look C; polish of the admin and sign up looks; the dashboard and app still say Premium "opens after the beta"; the computer voice reads owners' greetings as English even on French accounts; email (for "forgot password" and checking addresses); payments, so people without a code can sign up; "delete my account". Ken should back up `android/release-keystore.jks` + `android/local.properties`, and could check Firebase's public key is restricted in Google's console.
- Last update: 2026-11-04, the demo without keys works: `node tests/e2e/notes-real.mjs` passes a real call and voicemail between headless browsers, with real transcripts and summaries. Not yet on a real phone, Safari, or ElevenLabs and the Claude API. Earlier that day, built Premium notes in a loop from `docs/notes-plan.md` (its Log says what was chosen at each step), migrations 14 to 21 ran on the dev database. Language per account, a background worker with ElevenLabs Scribe and Claude, voicemail notes, the Notes tab, callers told first, calls and meetings recorded in the owner's browser, Android 0.5 (built, not published), the landing page's words. An independent review's findings are fixed. Tested with fake services and two real headless browsers; no real service, real phone or Safari yet. Earlier, 2026-11-03: PR #22, security fixes; Android 0.4 published (0.3 kept as `data/android/minivoip-0.3.apk.bak`); a copy of the database from before migration 12 is `data/minivoip-before-security.db`. The relay call in `site.mjs` failed once in 8 runs on 2026-11-03, cause unknown; if it fails again, suspect the relay limits first.

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
