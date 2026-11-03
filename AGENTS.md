# minivoip

research and proof of concept about a new way to offer voice assistance without the real plain old telephony line.

## Current state

Keep this short and up to date (status, next action, last update).

- Status: healthy. `main` has everything up to PR #22: calls, owner's app, Android 0.4, Premium video and room, plans, logos, website Call button, limits, call window, the landing page's message, notifications that name the business (#15), the call window's menu wave (#16), the plain hero with five steps (#17) and its polish (#18), and the redesign (#20): log in polish, the dashboard with a side menu, the owner's app in the dashboard's look, and "What callers hear" (#21). `hero-motion` (the rejected 3D film) is kept unmerged until Ken says to delete it. The security fixes are in (#22).
- Next action: Ken installs Android 0.4 and makes one real call on his phone (the lock screen fix isn't tested on a real phone). Then from `docs/plan.md`, the container and home server. Parked: the Android app's native ring screen still has the old look C; polish of the admin and sign up looks; the dashboard and app still say Premium "opens after the beta"; AI call notes (on the landing page as "Soon"); email (for "forgot password" and checking addresses); payments, so people without a code can sign up; "delete my account". Ken should back up `android/release-keystore.jks` + `android/local.properties`, and could check Firebase's public key is restricted in Google's console.
- Last update: 2026-11-03, shipped as PR #22, Android 0.4 published (0.3 kept as `data/android/minivoip-0.3.apk.bak`). Security review and fixes (`docs/architecture.md` "Security rules"): the relay crash (library upgrade), voicemail takeover, requests into the home network, relay quotas, room door (host lets each guest in), no sign up without a promo code, and the rest. Migration 12 ran on the dev database; a copy from before it is `data/minivoip-before-security.db`. `/call/ken` stays short on purpose. Then three line-by-line reviews found more (limits beaten by bursts, voicemail filling the disk, the relay free for strangers, a locked Android phone opening the app, and others), fixed in a second round. On other sites the Call button now opens its own window. A third round closed what was left: fonts on our own site, `__Host-` login cookies (everyone logs in once more), a browser that logged in before can't be locked out, and a caller's video waits for the owner's tap. An independent review then found IPv6 networks beating the limits, and smaller ones; all fixed. Migration 13 (`voicemail.size`) ran on the dev database. Real phones, a full call through Caddy and other browsers aren't tested. The relay call in `site.mjs` failed once in 8 runs on 2026-11-03, cause unknown; if it fails again, suspect the new relay limits first.

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
