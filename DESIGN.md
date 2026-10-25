# Design

How minivoip looks. Product truth lives in PRODUCT.md; the landing wording in docs/landing-copy.md.
First built on the landing page (`pages/home.templ`) on 2026-10-21; the other screens follow it next.

## Direction

Clean, lean and modern, at the craft level of top product sites (Quo, Aircall, Linear). The product explains
itself: the landing hero shows both ends of a real minivoip call. The landing page is white and pure, Apple-like, never glassy. No themed metaphors (answering machines,
key tags, lanterns) and no objects drawn as decoration: Ken rejected both.

## Tokens

- **Type:** Onest (Google Fonts, loaded in `layouts/base.templ`, set as `--font-sans` in `assets/css/globals.css`).
  Headings semibold with tight tracking: hero `text-6xl`/`4rem`, `tracking-[-0.035em]`; section titles
  `2.75rem`, `tracking-[-0.03em]` (the landing page goes larger: 76px hero, 56px sections). Body 15 to 18px, `leading-relaxed`, muted colour for supporting text.
- **Colour:** the shadcn neutral tokens in `globals.css` (light and dark). Ink buttons (`primary`), except on the
  landing page, where actions are blue (see below).
  Colour is reserved for meaning:
  - the logo's bar colours (orange #f3840f to blue #143495) for the live call path and played voice waves only;
  - orange #f46422 for the caller's chosen option;
  - green for Answer and check marks, red for Decline, recording and hang up.
- **Shape:** cards 14 to 20px radius with a 1px hairline ring (`shadow-[0_0_0_1px_var(--border)]`), a soft
  offset shadow only on the hero picture; buttons 10 to 12px radius (pills on the landing page). One elevation per element (ring + shadow, never border + shadow).
- **Layout:** `max-w-6xl` content, `px-4` gutter. Lists use hairline rows (`border-t`), not grids of icon cards.

## Landing page (built 2026-10-25: option A "Keynote")

Ken asked for "clean, lean and modern, white and pure, like Apple but not the glassy thing" (2026-10-24). He was
shown three white options (canvas https://claude.ai/artifact/XWsCHyUp822kvL6EfHgioS: A Keynote, B Step by step,
C One button) and skipped the choice, so A, the recommended one, was built. It replaces look B (warm beige, split
hero with a screen-share call). Built in `pages/home.templ`; the words are in `docs/landing-copy.md`.

- **Look:** pure white ground, one cool grey band colour #f5f5f7 (`--band`, the "call grows" and quote sections),
  ink #1d1d1f, muted #6e6e73, soft grey #86868b only for "in one click." in the headline. One blue #1f4fd1
  (`--primary`) for actions only: buttons and the "Call us now ›" text link; status labels ("Coming soon") stay
  muted. One dark moment: the closing band with the footer (`lp-ink`). Dark mode: #0b0b0d ground, #141416 bands,
  blue #5b86ff. The `.lp` wrapper in `globals.css` sets these as shadcn tokens, so the components follow.
- **Hero:** centred headline (76px desktop, `tracking-[-0.035em]`), one-line sub, blue pill "Get my free line" and
  the "Call us now ›" link, one microcopy line. Under it `callPair`: one call from both ends, two 300x600 phone
  screens copied from the real product (left the visitor's call window, always dark, hearing the voice menu with
  "1 Book a demo" chosen; right the owner's app ringing, "Someone chose “Book a demo”"). Nothing in it is a
  control. On a phone the pair shrinks (`zoom: 0.68`) and the owner's screen overlaps the visitor's.
- **Sections:** every head is centred (`sectionHead`). The call grows (`stages`: They ask. / They talk. / They
  see., hairline columns on the band, the plan under each), two plans as plain white cards with a hairline ring
  (prices "/ month after beta, planned"), the maker's quote large and centred on the band, three steps (numeral
  inline in the heading), four questions, then the closing band.
- **"Call us now"** calls minivoip's own line (Ken's), in the hero and the closing band. Tests never press it.
- **Voice wave** (`waveform` + `pages/wave.go`): bars drawn server-side from a seed; played bars take the logo
  colours, the rest the border colour (#3a3b40 on the dark call screen).

## Owner app (chosen 2026-10-24: look C)

One installable web app for the owner, on the phone and the computer (Chrome "Install", or "Add to Home Screen" on
iPhone). Ken picked look C from three mocks (`.impeccable/mocks/decision/app-looks/index.html`,
https://claude.ai/artifact/UWo6GNqvbGgihPSJEDXbab). Built 2026-10-24: `pages/app.templ`, `assets/css/app.css`,
`assets/js/phone.js`; the app's log in screen uses it too.

- **Look:** warm light. Ground #f6f5f1, white rounded groups (16px radius, 1px ring #e9e7e1), ink #1a1a17,
  muted #6b6a64, Onest. Green for Answer, red for missed and Decline, orange for the caller's chosen option.
- **Phone:** tabs at the bottom (Calls, Voicemail, Menu, Me); "Available" pill top right; calls grouped by day
  (Today, Yesterday), each row says which choice the caller pressed.
- **Call coming in (Android, full screen):** says it in words ("Someone wants to book a room"), shows what else
  rings and when voicemail starts, one big Answer button, then Decline and To voicemail.
- **Computer** (900 px and wider): a wide window like Google Chat: tabs and the Available switch on the left, the
  list in the middle, the open voicemail on the right (otherwise a "Calls ring here" note). An incoming call covers the whole window
  (Ken, 2026-10-24: a corner card was too easy to miss); if the window is behind others, the app also shows an
  "Incoming call, click to answer" alert that brings it to the front, and flashes the tab title.
- **Available switch:** Available / Pause for 1 hour / Off. Green, amber, grey dot. While away, calls go straight
  to voicemail.

## Motion

The landing page is still (2026-10-25, option A). Smooth scrolling to the sections, off under
`prefers-reduced-motion`.
