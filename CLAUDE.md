# CLAUDE.md

minivoip: a simple website phone line for small businesses (call button → French voice menu → 1/2 info, 0 reaches
a person). It started 2026-10-19 for Ken's friend's unstaffed B&B. The proof of concept runs on omt.dresdell.com,
about Ken himself.

Read `README.md` first, then the status file of the option being worked on:
- `custom/STATUS.md`: our own build (Web Push + WebRTC + coturn, $0). Built and working; the open problem is that
  a locked phone gives one short ding, not a ring. It could become a SaaS.
- `zadarma/STATUS.md`: the paid provider. Ken has an account with US$10 on it; nothing is set up inside yet.

## Layout

- **The custom option's code is `custom/callbox/`** (moved here from `~/Documents/DEV/callbox/` on 2026-10-20).
  - It runs as the `callbox` and `coturn` services in `~/Documents/DEV/docker-compose.yml` (build and volume paths
    point into this folder), behind Caddy (`~/Documents/DEV/caddy/Caddyfile`).
  - Rebuild from `~/Documents/DEV`: `docker compose up -d --build callbox`. Renaming or moving `custom/callbox/`
    breaks the compose file, so update its paths in the same step.
- Ken's goal: turn this into a **one-container SaaS** that lets small businesses add a one-button mini IVR to their
  website, linked to a phone app.
- The "Call me" button lives in the OMT repo (`~/Documents/DEV/OMT`, `templates/home.html`). It isn't committed;
  that repo has its own CLAUDE.md and rules.
- `custom/test-plan.md` and `zadarma/test-plan.md` are the original test plans with the scorecards to fill in.
- `custom/explainer.html` is the visual "how it works" page (published at
  https://claude.ai/artifact/WedNiBxF4ZpzFCTRjWpeXo).

## Rules that matter here

- **Secrets:**
  - They live in `custom/callbox/.env` and `custom/callbox/turnserver.conf`. Never copy them into docs or commit them.
  - Ken logs into Zadarma and pays himself. Never type his passwords or card details.
- **Ken's network work:** he does the router (TELUS, 192.168.1.254) and `sudo ufw` himself. Give him the exact rules.
- **Browser checks:** headless only, using playwright-core with the cached
  `~/.cache/ms-playwright/chromium_headless_shell-1243`. Never use the Playwright MCP and never launch desktop apps;
  they pop windows on his desktop or block the session.
- **Talking to Ken:**
  - He got lost in telecom jargon. Explain in plain, short words: what happens on whose phone.
  - Use pictures, not ASCII diagrams.
  - When he's exploring, brainstorm and recommend; build only when he says go.
