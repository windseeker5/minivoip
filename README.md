# minivoip

A virtual receptionist for small-business websites, with no phone line.

A visitor clicks **Call us** on your website. They hear your welcome message (typed or recorded in your own voice), pick a choice (1 for bookings, 2 to talk to you…), and your cell phone rings through a small installable web app. If you're busy, they leave a voicemail.

Proof of concept. Part of the mini family, with minipass and minicrm.

## What works

- Sign up, then your line and its call link (`/call/<name>`)
- Voice menu editor: ring your phone, play a message, open a web page, take a voicemail
- Real calls, browser to phone (WebRTC), with a built-in relay (TURN) for phones that can't connect directly
- Voicemail, push notifications that wake a sleeping phone, promo codes with an admin page

## Stack

Go, [templ](https://templ.guide), [shadcn-templ](https://github.com/axadrn/shadcn-templ), Tailwind v4, SQLite (pure Go), [pion/turn](https://github.com/pion/turn), [webpush-go](https://github.com/SherClockHolmes/webpush-go). One program, one database file.

## Run it

Needs Go, `templ`, `task`, `shadcn-templ` and the Tailwind standalone CLI ([docs/plan.md](docs/plan.md), Phase 0).

```bash
task dev        # auto-reload; open http://localhost:7331
go test ./...   # unit tests
node tests/e2e/site.mjs   # whole-site browser test against the running app
```

The microphone needs HTTPS on a real phone; on `localhost` it works as is.

## Docs

[Architecture](docs/architecture.md) · [Build plan](docs/plan.md) · [Data dictionary](docs/data-dictionary.md) · [Research](docs/research.md)
