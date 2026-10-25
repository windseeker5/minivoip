# Design

How minivoip looks. Product truth lives in PRODUCT.md; the landing wording in docs/landing-copy.md.
First built on the landing page (`pages/home.templ`) on 2026-10-21; the other screens follow it next.

## Direction

Clean, lean and modern, at the craft level of top product sites (Quo, Aircall, Linear). The product explains
itself: the landing hero plays the owner's real recorded voice menu (look "Voice", 2026-10-25). No themed metaphors (answering machines,
key tags, lanterns) and no objects drawn as decoration: Ken rejected both.

## Tokens

- **Type:** Onest (Google Fonts, loaded in `layouts/base.templ`, set as `--font-sans` in `assets/css/globals.css`).
  Headings semibold with tight tracking: hero `text-6xl`/`4rem`, `tracking-[-0.035em]`; section titles
  `2.75rem`, `tracking-[-0.03em]` (the landing page uses Funnel, larger: 88px hero, 60px sections). Body 15 to 18px, `leading-relaxed`, muted colour for supporting text.
- **Colour:** the shadcn neutral tokens in `globals.css` (light and dark). Ink buttons (`primary`), ink pills on the
  landing page.
  Colour is reserved for meaning:
  - the logo's bar colours (orange #f3840f to blue #143495) for the live call path and played voice waves only;
  - orange #f46422 for the caller's chosen option;
  - green for Answer and check marks, red for Decline, recording and hang up.
- **Shape:** cards 14 to 20px radius with a 1px hairline ring (`shadow-[0_0_0_1px_var(--border)]`), a soft
  offset shadow only on the hero picture; buttons 10 to 12px radius (pills on the landing page). One elevation per element (ring + shadow, never border + shadow).
- **Layout:** `max-w-6xl` content, `px-4` gutter. Lists use hairline rows (`border-t`), not grids of icon cards.

## Landing page (chosen 2026-10-25: look "Voice")

Ken rejected the warm look B and the white "Keynote" rebuild (same layout, only recoloured), then three more
directions, and asked for ten. Ten mocks each held to a top site were shown
(https://claude.ai/artifact/8AZNbvGtZWkXWjugauaBTM); he liked 2 Spectrum, 6 Voice, 8 Swiss and 10 Signal, and
chose **6 Voice**, held to ElevenLabs and Spotify, with two borrowed pieces: **Spectrum**'s logo-colour field (held to
Stripe), used once, and **Swiss**'s line of code to paste (held to Vercel). Built in `pages/home.templ`
(`pages/voice.go`, `pages/wave.go`, `assets/js/landing.js`); the words are in `docs/landing-copy.md`.

- **Type:** Funnel Display for headings (`.lp-display`) and Funnel Sans for text, loaded by the page itself (the
  rest of the site keeps Onest). Headlines semibold, tight (`tracking-[-0.04em]`): hero 88px, sections 60px.
- **Colour:** pure white, ink #0c0c0d, muted #66666c, hairlines #e7e7ea, soft grey panels #f4f4f5 (`.lp` tokens in
  `globals.css`). Buttons are ink pills. The logo's colours appear in two places only: the played part of the
  voice wave, and the one gradient field (`.lp-spectrum`, blurred radial blobs from orange #f3840f to blue
  #143495). Peach #ffe4d6 marks the word being said. One dark moment: the closing band (`lp-ink`). Dark mode:
  #0b0b0c ground, the same band and field.
- **Hero (`hero`, `voicePlayer`):** centred headline and sub, then the voice: a 96px ink play button and a
  120-bar wave (`voiceWave`), with "minivoip's own line · the real recording" and its length. landing.js plays
  the line's recorded greeting (`/prompt/<file>`), lights the wave and the words as it goes, and redraws the wave
  from the recording's real shape (decoded in the browser, quiet trimmed). The words are the line's greeting and
  spoken menu (`voiceWords`), so a new recording updates the page. At rest the first 44% looks played.
- **Then your own phone rings (`phoneRings`):** a 32px-radius band with the gradient field; ink text on the
  orange end, the owner's phone (the real app screen, with the line's first ringing choice) and a voicemail alert.
- **Sections:** centred heads (`sectionHead`) for the call grows, plans and questions; plans as one white card
  and one grey card (prices "planned"); the maker's quote large; three steps beside the code to paste
  (`.lp-code`, `your-business` highlighted) and a picture of the orange Call us button it makes.
- **"Call us now"** calls minivoip's own line, in the hero and the closing band. Tests never press it.

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

The landing page moves only when the visitor plays the greeting (the wave and the words follow it). Smooth scrolling to the sections, off under
`prefers-reduced-motion`.
