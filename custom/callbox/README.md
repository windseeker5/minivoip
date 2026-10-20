# Callbox

A "phone call" button for a website, at $0: the guest gets a call screen with a French voice menu (1 and 2 give
information, 0 reaches a person). Pressing 0 sends Web Push notifications to the owner's phone (an installed PWA);
the owner answers and both talk over WebRTC. No answer in 45 s: the guest leaves a voicemail.

Proof of concept on `omt.dresdell.com` (the "Call me" button in the contact line) for a friend's B&B phone line.
Test plan and status: `../test-plan.md`, `../STATUS.md` (this lives in `~/Documents/DEV/minivoip/custom/callbox/`).

## Pieces

| Where | What |
|---|---|
| `app.py` | Flask: WebSocket `/call/ws`, push, TURN credentials, voicemail, serves both pages |
| `static/appel/` | Guest call screen. `widget.js` + `widget.css` turn any `[data-appel]` link into a call; `index.html` is the standalone page |
| `static/owner/` | Owner PWA at `/call/owner/` (passcode, notifications, answer, voicemail) |
| `menu.json` | Menu text, card links, names. Mounted live: edit, run `make_audio.py`, `docker restart callbox` |
| `static/appel/audio/` | The prompts as MP3 (fr-CA neural voice). Rebuild the image after regenerating them |
| `turnserver.conf` | coturn (gitignored, holds the TURN secret; see `.example`) |
| `.env` | Secrets: passcode, VAPID keys, TURN secret (gitignored) |

Runs as `callbox` (127.0.0.1:8510) and `coturn` (host network) in `~/Documents/DEV/docker-compose.yml`; Caddy sends
`/appel*` and `/call/*` on omt.dresdell.com to callbox.

    docker compose up -d --build callbox      # after code changes, from ~/Documents/DEV
    uv run --no-project --with edge-tts python make_audio.py   # after changing a prompt

## Network

Router forwards to this machine: 3478 UDP+TCP, 49160-49200 UDP. Firewall (ufw) allows the same.
coturn's `external-ip` holds the home public IP; update it if the ISP changes it.
