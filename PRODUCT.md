# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

(The Android app in `android/` is a web view on `/app` plus a native ring; the design language stays the web one.)

## Users
In Ken's words (2026-10-25): people who are not technical but want to serve their customers with quality and a human voice. Small businesses first. Two packages: a really cheap one (voice and voicemail) and the full one (AI assistant, voice menu, video call, meeting room, whiteboard, screen sharing).

Owners of one-person or very small businesses (freelancers and contractors, B&Bs and small lodgings, trades, small shops), mostly in Quebec and Canada. They already carry a cell phone and don't want to pay for a business phone line just to have a greeting and a voicemail. The first test users are Ken's family and friends who run a business, starting with a friend's unstaffed B&B. Ken himself is the model user: a contractor after retiring from TELUS.

## Vision
Long-term goal (agreed 2026-10-22): one simple place where a small business talks with its customers, in any browser, with no app store and no phone line. Calls first, then one app on every device, a meeting room, whiteboard and meeting notes, then several people per room. Edge: anyone can reach you (unlike closed team tools such as Google Chat). Two plans: Basic (really cheap) and Premium (calls plus AI front desk, video, screen sharing, whiteboard, team room). One set of web app pages, with small phone apps around them for ringing (Android done, iPhone later). Details: `docs/vision.md`.

## Product Purpose
minivoip gives a small business a "Call us" button for its website (plus a call link and QR code). The visitor hears the owner's recorded voice menu, then the owner's phone rings through an installable web app; unanswered calls become voicemails with a notification. Success for now: family and friends use it on their real sites, and the project looks complete and credible. It is a test and a learning project (Go), with a possible small paid service later.

## Positioning
Elevator sentence (agreed 2026-10-24): "minivoip puts a Call button on your website. Your voice menu answers the common questions, and when a visitor wants more, you talk to them and show them your screen, in one click. No phone line, nothing to install." Basic: "Your website answers the phone for you." Premium: "From a question to a live demo, in one click." Ken is customer number one (minipass and his other businesses).

It rings the cell phone the owner already has, with the owner's own voice, and needs no phone line, number or SIM. The call goes directly between the visitor's browser and the owner's phone, so there is nothing to pay per minute and nothing recorded. Flat, very low price, made for very small Canadian businesses.

## Operating Context
- Visitors click a button on the owner's website (WordPress, Wix, Squarespace, Shopify) or open the call link from Google Maps, Facebook, an email signature or a QR code on the door.
- Owners set up from a dashboard: record the menu, install the phone app (PWA, no app store), paste one line of code.
- Product family: minipass and minicrm are Ken's other apps.

## Capabilities and Constraints
- Positioning frame: a virtual receptionist for the website that rings the owner's cell. Use receptionist language, never IVR/PBX/VoIP.
- Built 2026-10-25 (Premium, for now free for everyone like the rest of the beta): video, screen sharing and a shared whiteboard in a call, and a meeting room per line (up to 4 people, guests wait until the owner opens it). Planned, not built: the AI front desk answering from the owner's FAQ, recording and meeting notes. The landing page still shows Premium as "Coming soon" until Ken decides how to present it.
- Voice menu options: ring, play a message, voicemail, open a link, sub-menu. Opening hours.
- Several phones per line; all ring, first to answer wins.
- Voicemail after 30 s without answer, with a push notification.
- Limits to state honestly: no calls from regular phones yet (real number planned later); on iPhone the ring is a notification with sound, not a full-screen ringing call.
- Pricing: free while in beta. Later two plans, Basic and Premium; the landing page shows $2 and $5 a month as placeholders (`docs/vision.md`). Basic never gets video, whiteboard or AI. Promo codes make a line free forever and can be revoked.
- Page language: English for now; French later.
- Stack: Go, templ, shadcn-templ components, Tailwind v4, SQLite (see `docs/architecture.md`).

## Brand Commitments
- Logo: the mini-family lockup from minicrm/minipass (colored bars + Montserrat Bold wordmark), wordmark re-outlined as "minivoip" in the same font, size and position. Files: `assets/brand/logo-minivoip.svg` (light) and `logo-minivoip-dark.svg` (white text), one shown per theme; only set the height, never stretch. `assets/brand/icon.png` (bars only) is the favicon. Never redraw or recolor the bars.
- Name: minivoip, always lowercase, one word (like minipass and minicrm). Never "MiniVoIP" or "Mini-VoIP".
- Voice: plain, friendly, honest. Explain what happens on whose phone. No telecom jargon (VoIP, SIP, IVR, WebRTC) in the copy; "VoIP" appears only in the name.
- Visual direction: the landing page is look "Voice" (Ken, 2026-10-25): clean white, Funnel type, the owner's real recorded voice as the hero, the logo's colours used once (DESIGN.md). Ken disliked the earlier font (Onest) and styles on the landing page; a redesign must change the font and the layout, not just colours. Standing preference: clean, lean, modern and premium, at the craft level of top product sites (Ken: "a $10,000 website"). No themed metaphors and no hand-drawn or cartoon shapes: on 2026-10-21 he rejected them twice (answering machine and message pad; then hotel key tags, paper lanterns and a bright-green pill style, "looks like a kid designed that"). Round-2 options (voice-led light, dark call-path diagram, warm photo with serif, category-standard dashboard) are in `.impeccable/mocks/decision/round-2/`. The landing page must name the real features (voice menu, call routing, voicemail, rings your cell, several phones, hours, link/QR, private, low cost).
- Approved landing copy: `docs/landing-copy.md`. Marketing context: `.agents/product-marketing.md`.

## Evidence on Hand
- A working proof of concept ran in October 2026 with no phone line (Python; kept in git commit 5b8d116). The Go version made its first real browser-to-phone-app call on 2026-10-21.
- No customers, testimonials, logos, metrics or photos yet. Do not invent any. Any example business on a page (a sample B&B, a sample name) must be labelled as an example; the Joe's B&B demo was removed on 2026-10-24.

## Product Principles
1. Your phone, your voice: the product disappears behind the owner's own phone and greeting.
2. Plain words over telecom words.
3. Honest about limits (iPhone, no real number yet).
4. Small and cheap to run, so it can stay cheap to buy.
5. Dead simple: if someone who only knows how to use a phone can't use it without help, it isn't done.
