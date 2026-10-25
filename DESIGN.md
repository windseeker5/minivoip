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

## Call window (chosen 2026-10-25: look A "the voice in the middle")

What a visitor sees after "Call us": `/call/<line>` (`pages/call.templ`, `assets/css/callscreen.css`,
`assets/js/call.js`). A critique on 2026-10-25 (`.impeccable/critique/`, 21/40) found the old dark phone screen
generic and buggy; Ken picked A from three propositions (https://claude.ai/artifact/UefEpYx2avVwMxAyRekqLS: A the
voice in the middle, B answers first, C like a text conversation).

- **Look:** the landing page's world: white, Funnel Display and Funnel Sans, ink #0c0c0d, muted #66666c, soft grey
  #f4f4f5, ink buttons, red #d33a2c only for hang up and recording. The voice wave lights up in the logo's
  colours; the word being said is on peach #ffe4d6, words still to come grey #76767c. Always light. The page has
  its own bare layout (`layouts.Plain`): no site CSS, htmx or component scripts.
- **The business's face:** its initials on an ink square (`initials`), its logo later. No business colour yet: the
  mocks' green was only an example of one.
- **Phone:** the whole screen. **Computer:** one 400px white panel on the grey ground, the size of the corner panel it
  will become on a business's website.
- **Steps, one view each:** Start (name, big ink "Call us" button, a line that the browser will ask for the
  microphone) → the menu (wave, the words written out as they're said, the choices as big buttons that light up
  when the voice names them, a quiet Hang up; a "play" answer is written out and the menu stays) → Ringing (rings
  around the owner's initial, "You chose: …", the countdown to voicemail, "Leave a message now", red Hang up) →
  On the line (timer, a wave that moves with the owner's voice, Mute and Hang up: Mute exists only here) → Leave a
  message (the prompt written out, asks for a name and phone number, time "0:12 of 2:00", Send my message, Start
  over) → the end (a check for "Message sent", an "Open the page" button for a link choice, "Try sending again"
  when the upload fails, "Back to the website" when a page sent the visitor here, "Call again").
- **Microphone blocked:** a red-tinted note under the choices says how to allow it, with Try again.
- **Video, screen and whiteboard (2026-10-25):** on the line, five round buttons: Mute, Camera, Screen (computers
  only), Whiteboard, Hang up. While a camera, a screen or the whiteboard is on, the stage (below) takes the place of
  the voice wave, the owner's name and the timer move to a small header, and on a computer the window grows to
  almost the whole screen.
- **Keys:** the number keys pick a choice (not with Ctrl, Alt or Cmd); Escape hangs up.

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

## The stage: cameras, screens, whiteboard (2026-10-25)

One look for the call window, the owner's app and the meeting room (`assets/css/stage.css`, `assets/js/meet.js`):
dark tiles (#18181b, 18px radius) on whatever page holds them; a person without a camera is their initials on a
grey circle; names in a small dark pill at the bottom left, with a red crossed mic when muted. Two people: the other
fills the stage, me in a small corner tile (only with my camera on, mirrored). Three or more: a grid. A screen or the
whiteboard fills the stage, with everyone in a strip under it; showing my own screen shows "You're showing your
screen · Stop" on top instead of the screen itself. The whiteboard is white paper with a faint dot grid (16:10, it
fits the space like a slide) on a light grey board, five pens (ink, orange, blue, green, red), Undo, Clear and a
close button. Opening or closing it does so for everyone.

## Meeting room (2026-10-25)

`/room/<line>` (`pages/room.templ`, `assets/css/room.css`, `assets/js/room.js`). The call window's world (look A:
white, Funnel, ink, red only for Leave), not a new look.

- **Get ready:** the camera preview (dark, 16:10 on a computer, 4:3 on a phone) with Mic and Camera buttons on it,
  beside the business's initials, "Meeting room", its name, "Your name" and a big ink Join (the owner: "Open the
  room", no name). On a computer, one white card on the grey ground; on a phone, stacked.
- **Waiting:** an orange pulsing dot and "Waiting for <owner> to open the room…", with Leave.
- **In the room:** a bar (initials, name, "Meeting room · 3 people", Copy link), "Nobody else is here yet" with the
  link while alone, the stage, then Mute, Camera, Screen, Whiteboard and a red Leave.
- **The owner's app:** a Room tab (the link, Copy, "Open the room", three steps) and, when a guest waits, a dark pill
  on top of every tab, "Ana is waiting in your meeting room · Join".

## Motion

The landing page moves only when the visitor plays the greeting (the wave and the words follow it). Smooth scrolling to the sections, off under
`prefers-reduced-motion`.
