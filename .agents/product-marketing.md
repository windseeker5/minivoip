# Product Marketing Context

**Document version:** v6
**Last updated:** 2026-10-24

Draft from `docs/architecture.md`, `docs/research.md` and Ken's own words. Items marked **OPEN** still need Ken's answer.

## Product Overview
**One-liner (elevator sentence, agreed 2026-10-24):** minivoip puts a Call button on your website. Your voice menu answers the common questions, and when a visitor wants more, you talk to them and show them your screen, in one click. No phone line, nothing to install.
**What it does:** A visitor clicks "Call us" on your website (or a call link or QR code). They hear your recorded menu ("press 1 for bookings…"), then your phone rings through a small app installed from the browser. If you don't answer, they leave a voicemail and you get a notification.
**Product category:** Website call button / virtual business line (people search for "business phone line for small business", "call button for website", "business voicemail without a phone line").
**Product type:** Hosted web service (small SaaS), self-hosted on Ken's server.
**Business model (2026-10-31):** two plans: **Basic** $2/month ("A business line without giving out your cell": button, voice menu, rings your phone and computer, voicemail; never video, whiteboard or AI) and **Premium** $10/month ("Talk, show, close the deal": plus screen sharing, whiteboard, video, team room; soon, AI call notes with a summary in the app). No payments yet: sign up works without a card, and "Get Premium" leaves an email for Ken. No per-minute charges (calls go directly between the two devices, so they cost almost nothing to carry). Promo codes make a line free forever (Ken gives them to family and friends and can revoke them). For now the landing page is a test; pricing is there so the project looks complete.

## Target Audience
**Target companies:** Premium: solo founders and small teams who sell by showing (Ken with minipass is customer number one). Basic: one-person or very small businesses: freelancers and contractors (like Ken after retiring from TELUS), B&Bs and small lodgings, trades, small shops, local services. Mostly Quebec and Canada at first.
**Decision-makers:** The owner, who is also the person who answers the phone.
**Primary use case:** Look like a real business on the phone (menu, voicemail, a "Call us" button) without paying for a second line or giving out a personal cell number.
**Jobs to be done:**
- Let website visitors reach me by voice in one click, without showing my personal number.
- Send callers to the right place on their own (booking link, opening hours, voicemail) so I'm not interrupted for simple questions.
- Never miss a lead: if I can't answer, they leave a message and I'm notified.
**Use cases:**
- A B&B with nobody at the desk: guests press 1 for bookings (opens the booking site), 2 to reach the owner.
- A freelancer's portfolio site: "Call me" rings their cell, voicemail when they're with a client.
- A call link or QR code on a Google Maps listing, Facebook page, email signature or front door.

## Personas
Not B2B; one buyer who is also the user (the owner).

## Problems & Pain Points
**Core problem:** "They already have a cell phone but they don't want a business phone line just for a voicemail." (Ken)
**Why alternatives fall short:**
- A second business line or a VoIP number costs every month, plus setup, for something that mostly takes messages.
- Putting your personal cell number on your website means spam and calls at any hour.
- A contact form isn't a conversation; many customers just want to talk.
- Business phone systems (RingCentral, Aircall) are built for call centres, too big and priced per user.
**What it costs them:** A monthly bill for a line they barely use, or missed customers when they can't pick up.
**Emotional tension:** Wanting to look professional while being a one-person business; not wanting their private number everywhere; fear of missing the one call that mattered.

## Competitive Landscape
**Direct:** Call2 (browser call widget, rings phone apps, US$9/licence) and WebCallHub (call button, free tier 30 min/month) — closest; voice usually goes through their servers, English-first, priced per licence or minute.
**Secondary:** A second line or VoIP number (TELUS/Fizz business line, VoIP.ms, Zadarma) — a real number, but a monthly bill, setup and per-minute charges just to have a voicemail.
**Indirect:** Putting your personal cell on the website, a contact form, or WhatsApp/Messenger links — free, but no menu, no voicemail separation, privacy lost.

## Differentiation
**Key differentiators:**
- Rings the phone you already have; no new line, no SIM, no number to buy.
- Your own recorded voice menu, including "open a link" options (e.g. booking site).
- Calls go directly between the visitor and your phone: private, and nothing to pay per minute.
- Bilingual French/English, made for small Quebec/Canadian businesses.
- One line of code for your website, or just a link/QR code if you can't edit the site.
- Very low flat price: $3/month, billed yearly.
**How we do it differently:** Everything works in the browser: the visitor needs no app, the owner installs a web app with no app store.
**Why that's better:** Set up in minutes, costs almost nothing, and your private number stays private.
**Why customers choose us:** Simple, cheap, local, personal (Ken sets it up for you if needed).

## Objections
| Objection | Response |
|-----------|----------|
| "People can't call me from a normal phone." | True for now: it's for website visitors, links and QR codes. A real phone number is planned as an add-on later. |
| "Will my iPhone ring like a real call?" | On iPhone it's a notification with a sound, not a full ringing screen (an Apple limit). Android and computers ring repeatedly. Voicemail catches what you miss. |
| "I'm not technical." | Paste one line into your site, or just share the link. Ken can set it up with you. |

**Anti-persona:** Call centres, teams needing call queues and recording, anyone who must have a real phone number today, businesses whose customers only call from landlines.

## Switching Dynamics
**Push:** Paying for a line just for voicemail; personal number exposed; missed calls.
**Pull:** Free/cheap, looks professional, own voice menu, works with the phone you have.
**Habit:** "Just call my cell" works well enough; customers are used to phone numbers.
**Anxiety:** Will it really ring? Will callers find it weird to call from a website? Is it reliable?

## Customer Language
**How they describe the problem:**
- "I don't want a business phone line just for a voicemail." (Ken, about his own situation)
**How they describe us:** None yet; gather from the first family-and-friends users.
**Page language:** English for now (French later).
**Core frame:** the virtual receptionist (Ken, 2026-10-21): owners understand "a receptionist that answers and sends calls to the right person or online process", not IVR/PBX/VoIP.
**Lead message (2026-10-31):** a real person, one click away ("One click, and they're talking to you."), in a world of AI answering machines. The story: it filters (voice menu), you answer (phone or computer with a headset), you show them (screen, whiteboard). AI only takes notes; it never talks in the owner's place.
**Roadmap to mention:** AI call notes with a summary after each call, labelled "Soon". The AI front desk stays off the page for now.
**Words to use:** receptionist, answers, sends the call, the right person, the right page, call button, your phone, your voice, voicemail, no phone line, link, QR code.
**Words to avoid:** VoIP, WebRTC, SIP, PBX, IVR, TURN, codec, "platform" (jargon that loses the audience).
**Glossary:**
| Term | Meaning |
|------|---------|
| Line | Your call link and menu, e.g. `minivoip.example/joesbnb` |
| Call button | The "Call us" button added to your website |
| Phone app | The web app installed on your phone that rings |
| Voice menu | Your recorded greeting and choices (press 1, 2…) |

## Brand Voice
| Attribute | Means | Doesn't mean | Example line |
|-----------|-------|--------------|--------------|
| Plain | Short words, what happens on whose phone | Dumbed down or cute | "Your phone rings. That's it." (draft) |
| Friendly | Neighbourly, small-business to small-business | Salesy hype | **OPEN:** needs a real line |
| Honest | Says the limits (iPhone, no real number yet) | Over-promising | "On iPhone, it's a notification, not a ringing screen." |

**Tone by context:**
- Support: patient, step by step.
- Launch: warm, personal ("I built this for my friend's B&B").
- Errors: calm, says what to do next.
- Sales: no pressure; free to try.
- Social: not planned yet.
**Terms we own:** minivoip (fits Ken's family of apps: minipass, minicrm). The name is the only place "VoIP" appears; the copy itself stays jargon-free.
**Never use (and why):** Telecom jargon (VoIP, SIP, IVR): the audience got lost in it during testing.

## Proof Points
**Metrics:** None yet.
**Customers:** First test: a friend's B&B (**OPEN:** can we name it?).
**Testimonials:** None yet.
**Value themes:**
| Theme | Proof |
|-------|-------|
| No phone line needed | The proof of concept ran with no line (Oct 2026) |
| Private and free per minute | Calls go directly between the two devices |

## Goals
**Business goal:** Test with family and friends, then offer it to small businesses as a side business after Ken retires from TELUS.
**Conversion action:** Sign up / ask for a free line (for the test: "Try it free").
**Current metrics:** None (pre-launch).

## Learnings
*Decisions and tested lessons. Newest first. Keep it to about 20 active entries.*
| Date | Learning | Evidence | Applies to | Status |
|------|----------|----------|------------|--------|
| 2026-10-24 | "Virtual receptionist" alone isn't the killer feature; the gap between "I'm interested" and "I'm talking to a real person" is. Basic shortens it, Premium removes it (live demo in one click). | Brainstorm with Ken; he picked the demo pain over team meetings | Landing page, all copy | active |
| 2026-10-21 | Explain in plain words what happens on whose phone; telecom jargon loses people. | Ken got lost in jargon during the proof of concept | All copy | active |

## Changelog
*Newest first. One line per revision: what changed and why.*
- v6 (2026-10-24) — New elevator sentence; two plans Basic and Premium with their pains; Premium audience is solo founders who sell by showing; landing page rebuilt (look B).
- v5 (2026-10-21) — Repositioned around the virtual receptionist (Ken's words); features rewritten as receptionist tasks; AI receptionist added as "Coming soon".
- v4 (2026-10-21) — Name is always lowercase: minivoip (like minipass, minicrm).
- v3 (2026-10-21) — Pricing set: free in beta, then $3/month billed yearly; promo codes = free forever, revocable.
- v2 (2026-10-21) — Name set to minivoip (family with minipass, minicrm); landing page in English for now.
- v1 (2026-10-21) — Initial context, drafted from the design docs and Ken's description of the target customer.
