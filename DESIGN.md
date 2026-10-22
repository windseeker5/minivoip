# Design

How minivoip looks. Product truth lives in PRODUCT.md; the landing wording in docs/landing-copy.md.
First built on the landing page (`pages/home.templ`) on 2026-10-21; the other screens follow it next.

## Direction

Clean, lean and modern, at the craft level of top product sites (Quo, Aircall, Linear). The product explains
itself: the hero is a working diagram of one call, from the website's Call button to the owner's cell. No themed
metaphors (answering machines, key tags, lanterns) and no objects drawn as decoration: Ken rejected both.

## Tokens

- **Type:** Onest (Google Fonts, loaded in `layouts/base.templ`, set as `--font-sans` in `assets/css/globals.css`).
  Headings semibold with tight tracking: hero `text-6xl`/`4rem`, `tracking-[-0.035em]`; section titles
  `2.75rem`, `tracking-[-0.03em]`. Body 15 to 18px, `leading-relaxed`, muted colour for supporting text.
- **Colour:** the shadcn neutral tokens in `globals.css` (light and dark). Ink buttons (`primary`).
  Colour is reserved for meaning:
  - the logo's bar colours (orange #f3840f to blue #143495) for the live call path and played voice waves only;
  - orange #f46422 for the caller's chosen option;
  - green for Answer and check marks, red for Decline, recording and hang up.
- **Shape:** cards 14px radius with a 1px hairline ring and a soft offset shadow (`.flow-node`); buttons 10 to
  12px radius; pills only for small controls. One elevation per element (ring + shadow, never border + shadow).
- **Layout:** `max-w-6xl` content, `px-4` gutter. Lists use hairline rows (`border-t`), not grids of icon cards.

## Components (landing page)

- **Call diagram** (`callFlow`, `.flow-*` in globals.css, `landing.js`): dotted canvas, four steps (site, greeting,
  choices, phone) joined by SVG wires drawn from the nodes' positions; the wires on the call's path light up in
  the logo colours. On phones the steps stack and short gradient wires join them. It is also the demo: pressing
  Call us walks the states idle → greeting → menu → book / hours / ringing → voicemail → recording → sent.
- **Phone** (`joesPhone`, `setupPhone`, `.lp-phone`): an Android-style frame (the iPhone rings with a notification
  instead, so never show an iPhone ringing full screen). Size it with `zoom`.
- **Voice wave** (`waveform` + `pages/wave.go`): bars drawn server-side from a seed; played bars take the logo
  colours, the rest the border colour.

## Motion

One authored moment: the demo call. Fades in each state, animates the wave while a voice speaks and pulses the
Answer button while ringing. All of it stops under `prefers-reduced-motion`.
