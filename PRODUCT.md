# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users
Owners of one-person or very small businesses (freelancers and contractors, B&Bs and small lodgings, trades, small shops), mostly in Quebec and Canada. They already carry a cell phone and don't want to pay for a business phone line just to have a greeting and a voicemail. The first test users are Ken's family and friends who run a business, starting with a friend's unstaffed B&B. Ken himself is the model user: a contractor after retiring from TELUS.

## Product Purpose
minivoip gives a small business a "Call us" button for its website (plus a call link and QR code). The visitor hears the owner's recorded voice menu, then the owner's phone rings through an installable web app; unanswered calls become voicemails with a notification. Success for now: family and friends use it on their real sites, and the project looks complete and credible. It is a test and a learning project (Go), with a possible small paid service later.

## Positioning
It rings the cell phone the owner already has, with the owner's own voice, and needs no phone line, number or SIM. The call goes directly between the visitor's browser and the owner's phone, so there is nothing to pay per minute and nothing recorded. Flat, very low price, made for very small Canadian businesses.

## Operating Context
- Visitors click a button on the owner's website (WordPress, Wix, Squarespace, Shopify) or open the call link from Google Maps, Facebook, an email signature or a QR code on the door.
- Owners set up from a dashboard: record the menu, install the phone app (PWA, no app store), paste one line of code.
- Product family: minipass and minicrm are Ken's other apps.

## Capabilities and Constraints
- Positioning frame: a virtual receptionist for the website that rings the owner's cell. Use receptionist language, never IVR/PBX/VoIP.
- Planned, not built: AI receptionist (answers common questions, sums up messages). Always shown as "Coming soon".
- Voice menu options: ring, play a message, voicemail, open a link, sub-menu. Opening hours.
- Several phones per line; all ring, first to answer wins.
- Voicemail after 30 s without answer, with a push notification.
- Limits to state honestly: no calls from regular phones yet (real number planned later); on iPhone the ring is a notification with sound, not a full-screen ringing call.
- Pricing: free while in beta; later $30/year per line (about $2.50/month). Promo codes make a line free forever and can be revoked.
- Page language: English for now; French later.
- Stack: Go, templ, shadcn-templ components, Tailwind v4, SQLite (see `docs/architecture.md`).

## Brand Commitments
- Logo: the mini-family lockup from minicrm/minipass (colored bars + Montserrat Bold wordmark), wordmark re-outlined as "minivoip" in the same font, size and position. Files: `assets/brand/logo-minivoip.svg` (light) and `logo-minivoip-dark.svg` (white text), one shown per theme; only set the height, never stretch. `assets/brand/icon.png` (bars only) is the favicon. Never redraw or recolor the bars.
- Name: minivoip, always lowercase, one word (like minipass and minicrm). Never "MiniVoIP" or "Mini-VoIP".
- Voice: plain, friendly, honest. Explain what happens on whose phone. No telecom jargon (VoIP, SIP, IVR, WebRTC) in the copy; "VoIP" appears only in the name.
- Visual direction: not decided. Ken rejected the themed concepts (answering machine, message pad) and the generic SaaS look on 2026-10-21. The landing page must name the real features (voice menu, call routing, voicemail, rings your cell, several phones, hours, link/QR, private, low cost).
- Approved landing copy: `docs/landing-copy.md`. Marketing context: `.agents/product-marketing.md`.

## Evidence on Hand
- A working proof of concept ran in October 2026 with no phone line (Python; kept in git commit 5b8d116). The Go version made its first real browser-to-phone-app call on 2026-10-21.
- No customers, testimonials, logos, metrics or photos yet. Do not invent any. The demo business "Joe's B&B" is a labelled example.

## Product Principles
1. Your phone, your voice: the product disappears behind the owner's own phone and greeting.
2. Plain words over telecom words.
3. Honest about limits (iPhone, no real number yet).
4. Small and cheap to run, so it can stay cheap to buy.
