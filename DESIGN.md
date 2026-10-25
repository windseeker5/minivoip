# Design

How minivoip looks. Product truth lives in PRODUCT.md; the landing wording in docs/landing-copy.md.
First built on the landing page (`pages/home.templ`) on 2026-10-21; the other screens follow it next.

## Direction

Clean, lean and modern, at the craft level of top product sites (Quo, Aircall, Linear). The product explains
itself: the landing hero shows a real minivoip call with a shared screen. No themed metaphors (answering machines,
key tags, lanterns) and no objects drawn as decoration: Ken rejected both.

## Tokens

- **Type:** Onest (Google Fonts, loaded in `layouts/base.templ`, set as `--font-sans` in `assets/css/globals.css`).
  Headings semibold with tight tracking: hero `text-6xl`/`4rem`, `tracking-[-0.035em]`; section titles
  `2.75rem`, `tracking-[-0.03em]`. Body 15 to 18px, `leading-relaxed`, muted colour for supporting text.
- **Colour:** the shadcn neutral tokens in `globals.css` (light and dark). Ink buttons (`primary`).
  Colour is reserved for meaning:
  - the logo's bar colours (orange #f3840f to blue #143495) for the live call path and played voice waves only;
  - orange #f46422 for the caller's chosen option;
  - green for Answer and check marks, red for Decline, recording and hang up.
- **Shape:** cards 14 to 20px radius with a 1px hairline ring (`shadow-[0_0_0_1px_var(--border)]`), a soft
  offset shadow only on the hero picture; buttons 10 to 12px radius (pills on the landing page). One elevation per element (ring + shadow, never border + shadow).
- **Layout:** `max-w-6xl` content, `px-4` gutter. Lists use hairline rows (`border-t`), not grids of icon cards.

## Landing page (chosen 2026-10-24: option B "The call grows")

Ken picked B from three options (canvas https://claude.ai/artifact/KZH1jv7iRV2u99cKPYKvya; he hesitated with C, a
dark stage). Built in `pages/home.templ`; the words are in `docs/landing-copy.md`.

- **Look:** warm light like the owner app (ground #f6f5f1, white cards, ink #1a1a17, muted #6b6a64); in dark mode
  a near-black stage (#0b0b0d, cards #141418). The `.lp` wrapper in `globals.css` sets the shadcn tokens for the
  page, so the components follow. Inverted ink blocks (the "They see" card, the Premium card, the footer band)
  mark Premium and the last call to action.
- **Hero:** headline left, a picture of a Premium call on the right (`callWindow`: screen shared, small video
  tile, controls), with a caption saying it's coming soon. Nothing in it is a control.
- **The call grows** (`callGrows`, `stages`): three cards, They ask / They talk / They see, each with a small
  piece of the real screens (`stageSnippet`) and the plan that has it.
- **Plans** (`plans`, `planCard`), the maker's quote (`maker`), three steps, four questions, then the ink footer
  band with the last call to action.
- **"Call us now"** calls minivoip's own line (Ken's), in the hero and the footer band. The Joe's B&B demo is gone.
- **Voice wave** (`waveform` + `pages/wave.go`): bars drawn server-side from a seed; played bars take the logo
  colours, the rest the border colour.

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

The landing page is still (2026-10-24, option B). Smooth scrolling to the sections, off under
`prefers-reduced-motion`.
