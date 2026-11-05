# Design

How minivoip looks. Product truth lives in PRODUCT.md; the landing wording in docs/landing-copy.md.
First built on the landing page (`pages/home.templ`) on 2026-10-21; the other screens follow it next.

## Direction

Clean, lean and modern, at the craft level of top product sites (Quo, Aircall, Linear). The product explains
itself: the landing page is look "Voice" (2026-10-25), and its hero says in one line and five icon steps how a call goes (2026-11-01). No themed metaphors (answering machines,
key tags, lanterns) and no objects drawn as decoration: Ken rejected both.

## Tokens

- **Type:** Onest (served from `assets/fonts/`, loaded in `layouts/base.templ`, set as `--font-sans` in `assets/css/globals.css`).
  Headings semibold with tight tracking: hero `text-6xl`/`4rem`, `tracking-[-0.035em]`; section titles
  `2.75rem`, `tracking-[-0.03em]` (the landing page uses Funnel, larger: 88px hero, 60px sections). Body 15 to 18px, `leading-relaxed`, muted colour for supporting text.
- **Colour:** the shadcn neutral tokens in `globals.css` (light and dark). Ink buttons (`primary`), ink pills on the
  landing page.
  Colour is reserved for meaning:
  - the logo's bar colours (orange #f3840f to blue #143495) for the live call path and played voice waves only;
  - orange #f46422 for the caller's chosen option;
  - green for Answer and check marks, red for Decline, recording and hang up.
- **Focus:** a 2px ink outline, 2px out, on everything the keyboard reaches (the owner app already did this).
- **Shape:** cards 14 to 20px radius with a 1px hairline ring (`shadow-[0_0_0_1px_var(--border)]`), a soft
  offset shadow only on the hero picture; buttons 10 to 12px radius (pills on the landing page). One elevation per element (ring + shadow, never border + shadow).
- **Layout:** `max-w-6xl` content, `px-4` gutter. Lists use hairline rows (`border-t`), not grids of icon cards.

## Landing page (chosen 2026-10-25: look "Voice")

Ken rejected the warm look B and the white "Keynote" rebuild (same layout, only recoloured), then three more
directions, and asked for ten. Ten mocks each held to a top site were shown
(https://claude.ai/artifact/8AZNbvGtZWkXWjugauaBTM); he liked 2 Spectrum, 6 Voice, 8 Swiss and 10 Signal, and
chose **6 Voice**, held to ElevenLabs and Spotify, with two borrowed pieces: **Spectrum**'s logo-colour field (held to
Stripe), used once, and **Swiss**'s line of code to paste (held to Vercel). Built in `pages/home.templ`
(`pages/voice.go`, `assets/js/landing.js`); the words are in `docs/landing-copy.md`.

- **Type:** Funnel Display for headings (`.lp-display`) and Funnel Sans for text, loaded by the page itself (the
  rest of the site keeps Onest). Headlines semibold, tight (`tracking-[-0.04em]`): hero 88px, sections 60px.
- **Colour:** pure white, ink #0c0c0d, muted #66666c, hairlines #e7e7ea, soft grey panels #f4f4f5 (`.lp` tokens in
  `globals.css`). Buttons are ink pills. The logo's colours appear in three places only: the line under the
  hero's "in one click.", the line between the hero's steps, and the one gradient field (`.lp-spectrum`, blurred radial blobs from orange #f3840f to blue
  #143495). Peach #ffe4d6 highlights `your-business` in the code to paste. One dark moment: the closing band (`lp-ink`). Dark mode:
  #0b0b0c ground, the same band and field.
- **Hero (`hero`, `heroSteps`):** centred on a computer, like the steps and the band under it (Ken, 2026-11-01:
  a left headline over centred steps looked unaligned); left-aligned on a phone. Headline 84px on a computer, a
  line in the logo's colours under "in one click.", then sub, buttons (Get my line, and "Call us" whose orange phone rings every 6 s with a soft
  orange ring around the button) and price line, then five steps on one line (`.lp-steps`): 64px round icons with
  a hairline ring, their words under them, joined by a 2px grey line that fills in the logo's colours; behind the
  line, one faint grey voice wave (`--wave`, #ececef; 150px band, drawn on a canvas by `landing.js`) from Call us
  to the screen, passing behind the icons so it reads as one signal (Ken, 2026-11-01: colourful bars were too
  loud); no wave before Call us (there is no sound before the visitor taps) or after the screen; Call us is the orange
  #f46422 circle, Premium an orange outline tag. On a phone the steps stack down the wave, right under the
  headline.
- **Then your own phone rings (`phoneRings`):** a 32px-radius band with the gradient field, shorter than the
  phone: the phone stands out of it by 80px above and below on a computer, below on a phone (Ken, 2026-11-01, for
  depth). Ink text on the orange end, the owner's phone (the real app screen, with the line's first ringing
  choice) and a voicemail alert.
- **Sections:** centred heads (`sectionHead`) for the call grows, plans and questions; plans as one white card
  and one grey card (prices "planned"); the maker's quote large; three steps beside the code to paste
  (`.lp-code`, `your-business` highlighted) and a picture of the orange Call us button it makes.
- **"Call us now"** calls minivoip's own line, in the hero and the closing band. Tests never press it.

## Sign up and log in (polished 2026-11-01)

`accountShell` in `pages/account.templ` (sign up, log in, and step 2 "your business"): the logo and one white card
(hairline ring + soft shadow) on the light grey ground, in the middle of the screen a little above centre on a
computer, at the top on a phone (the keyboard never covers it). Onest, the site's shadcn tokens. Fields are 40px,
as tall as the ink button. Form fields follow the site's theme, not the computer's (`color-scheme` in
`globals.css`), and a field the browser fills in (saved email, password) keeps its own white look: Ken saw Chrome
draw them dark grey-blue on a dark computer with the site set to light. Sign up ends with the language of the notes:
two choices side by side (`languageChoice`, as tall as the fields, an ink outline on the one chosen), Français first
and already chosen. The same choice is on "Language of your notes", from the account menu.

## Dashboard (redesigned 2026-11-01: proposition 2, "menu on the side")

For owners with no tech skills: is my line on, what's my link, where do I put it. Ken picked proposition 2 of three
(https://claude.ai/artifact/9hfo7Bd3dq6488ZtSBcJrF), held to Stripe, but in the site's own type: `.dash` shares
`.lp`'s tokens in `globals.css` (ink, Funnel Sans, headings in Funnel Display); the page is light grey #f6f6f7 (dark
#0b0b0c) with white cards (`card`: 16px radius, a 1px ring, `bg-card`).

- **Frame** (`dashShell`): on a computer a white menu on the left (248px): the logo, Home, What callers hear, Messages
  (orange count of new voicemails), Notes (a "Premium" tag on Basic), Business, the open one shaded; at its foot a "Phone app" card and the account
  button (letter, "Your account", email). Its menu (a native popover) holds the email, Change password, Language of your notes, the phone app, Log out, and
  "minivoip admin" for staff only: minivoip's own admin is never a tab. On a phone a top bar (logo, account) and
  the five tabs fixed at the bottom, like an app. A "Questions?" line closes each page.
- **Home** (`Dashboard`): the title, then a peach band (#fff4ec) with the next setup step and its button, ticked from
  real data (a menu of its own, a phone that gets notifications, a logo; gone when all are done); a card with the
  business's badge, name and status (green dot: calls ring you; amber: away, or messages until the phone is set up)
  over the call link with Copy link (ink) and Try it; then "Share your line" (website, emails, video meetings, each
  row opening a window) beside "Latest calls". Between them, always there, a "What callers hear" card: three grey
  tiles (Welcome, the choices, Voicemail greeting) saying whose voice callers hear, and one Change button.
- **Windows** (`sheet`, a native dialog): centred on a computer, rising from the bottom on a phone; Esc, Close or a
  click outside close them. The website code lives only there, with "1. Copy this code, 2. Paste it" and where to
  paste it on Wix, Squarespace, WordPress and Shopify.
- **Business** (`Business`): one card of label and value rows for the logo, the name (changed here), the call link and the plan.
- Buttons on Home are pills; the messages list keeps its white cards.
- **Links and code to copy show whole** (`shownValue`), the code to paste highlights the line name on peach
  (`--now`). **Copy** (`copyButton`) shows a check and "Copied"; screen readers hear it (`copyStatus`).
- **On a phone** every button, tab and menu item is 44px tall (`tap`). **Keyboard focus, every page:** a 2px outline
  in the ink colour, 2px out (`:focus-visible` in `globals.css`), inside the menus (`.focus-inset`).

## Notes (2026-11-04)

`pages/notes.templ`, `/dashboard/notes`, dashboard only and for owners who aren't technical. **The tab:** one card with the
"Take notes" switch (ink when on; it saves itself, `notes.js`), the month's hours over a thin ink bar, and the language
with a Change link. Below it, hairline rows, newest first: an icon for the kind, Call / Meeting room / Voicemail, the
date, and two lines at most of the summary or of how the note is doing, always in plain words ("Being written", "Notes
aren't set up yet"). **One note,** in the account's language, on one page, in two sections: Highlights with Next actions, then "Full transcript" (who and when on the left,
the words on the right; on a phone one above the other), each its own white card, then Hear the message and Delete. The
summary carries `lang`, so screen readers read French as French. A page with a note being written looks again every 15
seconds. **On Basic** the tab is one card with the Premium tag. A voicemail in Messages has "Read the note".

## What callers hear (2026-11-02)

The owner's one place for everything callers hear; it replaces the "Voice menu" editor. Ken couldn't find where to
change his menu or record his voicemail after the redesign, and picked this from the picture
https://claude.ai/artifact/EisCY92EKymwYNacqeAjCB. `hearForm` in `pages/hear.templ`, `assets/css/hear.css` (plain CSS:
the same form on the dashboard and in the app, in their colours), `assets/js/menu.js`.

- **Three numbered cards, in the order a caller hears them:** 1 Welcome, 2 Choices, 3 Voicemail greeting (when a call
  is missed, while away, and on a "Take a voicemail" choice). The top has "Hear it like a caller".
- **Recording cards** (`voiceCard`): the record button is always in the card's top right corner (an ink pill "Record
  my voice", or a white "Record again"), and a tag says "Your voice" or "Computer voice". With a recording: a player with
  the voice drawn as bars and "Use a computer voice instead". Without: the words to read out.
- **What to say when you record** (peach, on the welcome): the page writes out the greeting and the choices, since a
  recording replaces the spoken list. A red line asks to record again when the choices have changed since.
- **Choices:** short rows (key, name, what it does, Change); a row opens to edit it (`<details>`). A voicemail choice
  has no message of its own: it plays the voicemail greeting.
- **Saving:** a dark bar "You changed something · Save" follows the screen once something changed, above the tabs on a
  phone; "Saved." shows for 4 s. The page never reloads: in the app that would drop the call socket. Leaving with
  unsaved changes asks first.

## Call window (chosen 2026-10-25: look A "the voice in the middle")

What a visitor sees after "Call us": `/call/<line>` (`pages/call.templ`, `assets/css/callscreen.css`,
`assets/js/call.js`). A critique on 2026-10-25 (`.impeccable/critique/`, 21/40) found the old dark phone screen
generic and buggy; Ken picked A from three propositions (https://claude.ai/artifact/UefEpYx2avVwMxAyRekqLS: A the
voice in the middle, B answers first, C like a text conversation).

- **Look:** the landing page's world: white, Funnel Display and Funnel Sans, ink #0c0c0d, muted #66666c, soft grey
  #f4f4f5, ink buttons, red #d33a2c only for hang up and recording. The voice wave lights up in the logo's
  colours; the word being said is on peach #ffe4d6, words still to come grey #76767c. Always light. The page has
  its own bare layout (`layouts.Plain`): no site CSS, htmx or component scripts.
- **The business's face:** its logo (`line.logo`) filling its shape edge to edge (Ken, 2026-10-29: no white plate or ring), else its initials on an
  ink square (`initials`); the same in the meeting room and as the owner's face on the stage (`faceBadge`). Asked
  when the line is made, changed on the dashboard (Ken, 2026-10-27). The owner frames it in a round guide, since
  round places (the stage, the website button) cut the square's corners; transparent parts show white. No business colour or background yet (parked).
- **Phone:** the whole screen. **Computer:** one 400px white panel on the grey ground, the size of the call window it
  becomes on a business's website.
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
- **Notes on (2026-11-04):** a quiet line with the recording's red dot (`noticeLine`, `.cw-notice`) on every step from
  the start to the call and the message, in the account's language; the words are in `lines.Notice`. The same words are
  said first, before the owner's welcome, and written out with it. A choice pressed before the end doesn't skip them.
  Before the tone the shorter message version is said. A menu that only plays messages records nothing and says nothing.
- **Keys:** the number keys pick a choice (not with Ctrl, Alt or Cmd); Escape hangs up.

## Call button on a business's website (chosen 2026-10-27)

`w.js` (`assets/js/w.js`), pasted by the business into its own site; Ken picked it from the mock
https://claude.ai/artifact/GUgtWk5t9fAzWQMFrn9aUK. A pill in the bottom-right corner, in a shadow root so the site's
styles and ours never mix; system fonts (the site's page doesn't load ours).

- **Here** (Available switch on, a choice that rings, and a phone that can ring: an open app or one with push): ink
  pill, the business's logo filling a circle (else initials on white) with a green dot that pulses, a phone icon, "Talk to us
  now". A small white note above: "<Owner> is here. Talk to us now."
- **Away:** white pill with a hairline ring, the logo greyed with a grey dot, a message icon, "Leave us a message";
  the note says "We're away. Our menu answers, or leave a message." The menu still plays (hours, links, answers):
  a ringing choice says "Away now: leave a message" and goes to voicemail.
- The note has an ×, remembered for the visit; it's hidden on phones. The status is read again every minute.
- **Click on another site (security review, 2026-11-02):** on a computer, the call page opens in its own 480px browser
  window, so the microphone permission is minivoip's and not that website's, and the website can't reach it.
- **Click on our own landing page:** the call page opens in a 400px window in the middle of the page (Ken, 2026-10-29), a
  modal dialog: the page behind is dimmed (ink at 45%) and blurred (6px) and doesn't scroll; the window rises in. It
  grows to almost the whole page while a camera, a screen or the whiteboard is on. × closes it (and ends a call);
  Esc or a click on the dimmed page close it too, but never while a call is on. The call page's own back arrow is
  hidden there (the × is in that corner); its "Back to the website" at the end closes it. On a phone, the call page
  takes the whole screen.
- The site's own links can open the same window: any element with `data-minivoip-call`. While one of them is on
  screen, the corner button fades aside, so two call buttons never sit on top of each other (on a phone it covered
  the hero's). The minivoip landing page uses it: our own button in the corner, and its "Call us now" buttons open
  the window (Ken's line).
- **The call page** says it too: under the name a green "<Owner> is here" or a grey "Away right now", and the big
  button reads "Hear our menu" (white, ink ring) when away.

## Owner app (redesigned 2026-11-01: the dashboard's look)

One installable web app for the owner, on the phone and the computer (Chrome "Install", or "Add to Home Screen" on
iPhone): `pages/app.templ`, `assets/css/app.css`, `assets/js/phone.js`; the app's log in screen uses it too. Ken
first picked look C (warm beige, Onest, 2026-10-24), then disliked it; on 2026-11-01 he picked proposition 1 of three
(https://claude.ai/artifact/QUHgeBgYVQ15wUVRSnvCCw: 1 like Stripe, the dashboard's look; 2 Spotify-like dark; 3 a
calm white page), "exactly like proposed", so the app and the dashboard read as one product.

- **Look:** the dashboard's: light grey ground #f6f6f7, white cards (16px radius, a 1px ring #e7e7ea), ink #0c0c0d,
  muted #66666c, Funnel Display for titles and Funnel Sans for text, pill buttons. Peach #ffe4d6 for a new message
  and the caller's choice; orange #f46422 for the count of new messages; green for Available and Answer, amber for a
  pause, red for missed calls, Decline and Hang up. Always light.
- **Plain words on every call:** "New message · 0:42" (then "Message" once played), "You talked · 4:18", "You
  declined", "Missed while you were away", "Missed call"; the second line "Chose “Book a room” · 9:36 PM". The
  dashboard's Latest calls uses the same words (`entryTitle`).
- **Four tabs:** Calls, Messages (orange count of new ones), What callers hear ("Callers hear" on a phone), Room ("Meeting room" on a computer). The
  account (ringing and notifications, install, Log out) opens from the round letter: top right on a phone, the
  business name and email at the foot of the menu on a computer. `#voicemail` still opens Messages.
- **Does my phone ring?** A band on top of Calls (`Availability`): green "You're available. Calls ring on this
  computer and on your phone." ("on this phone" on a phone), amber "Paused until 3:04 PM. Callers can leave a
  message.", white "You're off. Every call goes to a message." On a computer three buttons in the band (Available
  with a check, Pause 1 hour, Off; the current one filled); on a phone "Change" and a menu. While away, calls go
  straight to voicemail.
- **Lists:** one white card, the days (Today, Yesterday) inside it; round icons (peach for a new message, green for
  talked, red for missed); an ink play button on messages.
- **Computer** (900 px and wider): a white menu on the left (248px), then the title and band across the page, the
  list (420px) and on the right the open message or a "Calls ring here" note. The message: when, "New message ·
  0:42", "The caller chose [Book a room]" on peach, a big player with the message's real voice drawn as bars that
  light up in the logo's colours as it plays (click to jump), Delete, and "Callers are asked to say their name and
  phone number in the message." What callers hear (the same form as the dashboard's), Room and the account take the page (680px max).
- **Call coming in** (the whole phone screen, the whole window on a computer): white; "Incoming call from your
  website", the business's face (logo or initials) with two faint rings that pulse, "Someone wants to talk to
  you", "They chose [Book a room]", a bar draining to voicemail with "Goes to voicemail in 0:29", a big green
  Answer pill, a white Decline pill, "If you decline, they can leave a message." On the line: "Talking with a
  website caller", Mute, Camera, Screen, Whiteboard and a red Hang up. If the window is behind others, the app
  also shows an "Incoming call: <business>" alert that brings it to the front, and flashes the tab title (Ken,
  2026-10-24: a corner card was too easy to miss).
- **Notifications** (computer and phone): the title names the business ("Incoming call: Joe's B&B", "Missed call:
  …", "New voicemail: …"), the text is plain ("Someone on your website wants to talk to you."; not the menu
  choice, whose label reads oddly to the owner), and the picture is the business's logo cut into a circle on white
  (the minivoip mark without one), so it stays round where the desktop draws pictures square (Omarchy's mako).
  The browser adds the site's address under the title; a page can't remove it.
- The Android app's own ring screen (`RingActivity`, native) still has look C.

## The stage: cameras, screens, whiteboard (2026-10-25)

One look for the call window, the owner's app and the meeting room (`assets/css/stage.css`, `assets/js/meet.js`):
dark tiles (#18181b, 18px radius) on whatever page holds them; a person without a camera is their initials on a
grey circle; names in a small dark pill at the bottom left, with a red crossed mic when muted. Two people: the other
fills the stage, me in a small corner tile (only with my camera on, mirrored). Three or more: a grid. A screen or the
whiteboard fills the stage (the newest one), with the others small and, under it, only the people whose camera is on
(Ken, 2026-10-27: empty tiles were noise); a click on a
small screen or the whiteboard makes it fill the stage. My own screen shows small as "Your screen" with a red
"Stop sharing" on it, never big unless I click it (a full-size copy of my screen repeats itself); Ken asked for this
after his first test, 2026-10-26. The whiteboard is white paper with a faint dot grid (16:10, it
fits the space like a slide) on a light grey board, five pens (ink, orange, blue, green, red), Undo, Clear and a
close button. Opening or closing it does so for everyone. The whiteboard stays open on my side while I show my screen.

## Meeting room (2026-10-25)

Notes on (2026-11-04): a quiet line with the recording's red dot at the door, while waiting and in the room
(`roomNotice`), said out loud to a guest pressing Join. The host also reads "Recording for your notes", or why not.

`/room/<line>` (`pages/room.templ`, `assets/css/room.css`, `assets/js/room.js`). The call window's world (look A:
white, Funnel, ink, red only for Leave), not a new look.

- **Get ready:** the camera preview (dark, 16:10 on a computer, 4:3 on a phone) with Mic and Camera buttons on it,
  beside the business's initials, "Meeting room", its name, "Your name" and a big ink Join (the owner: "Open the
  room", no name). On a computer, one white card on the grey ground; on a phone, stacked.
- **Waiting:** an orange pulsing dot and "Waiting for <owner> to let you in…", with Leave. Declined, removed or no answer:
  "<owner> didn't let you in.", "<owner> removed you from the room." or "<owner> didn't answer.", with Join again.
- **In the room:** a bar (initials, name, "Meeting room · 3 people", Copy link), "Nobody else is here yet" with the
  link while alone, the stage, then Mute, Camera, Screen, Whiteboard and a red Leave. The host also gets the door, a
  soft grey band above the stage: "Ana is waiting" with Decline and an ink Let in; a People chip in the bar opens the
  list of who is in the room, each with Remove.
- **The owner's app:** a Room tab (the link, Copy, "Open the room", three steps) and, when a guest waits, a dark pill
  on top of every tab, "Ana is waiting in your meeting room · Join".
- **A caller's picture (2026-11-03):** in the owner's app, a caller's camera or screen shows only after a tap: the face stays, with a white "Show their video" pill under it. One tap shows everything for that call.
- **Basic accounts** (2026-10-25): no room and no Camera, Screen or Whiteboard in calls, on either side (the visitor
  never sees an upsell). The owner's Room tab and the dashboard's Meeting room panel show a small ink "Premium" tag
  and one line on what it adds, "You're on Basic. Premium opens after the beta."

## Motion

The landing page has one authored moment, the hero, about 8 s once per page view: the headline's words come in
one after the other out of a blur, its line in the logo's colours wipes in under "in one click.", then each step
comes in (its icon grows out of a blur and sends out one ring in its colour on the line) as the line fills up to
it; when Call us is pressed the faint voice wave starts there and runs on with the line (its front edge rises
like a voice arriving); Call us is pressed
with one ripple, the phone icon shakes, "Hi, it's Marie!" comes in, a stroke draws on the screen icon, then
Premium. The headline and steps are CSS (`globals.css`); the wave is `landing.js`, which takes its time from the
steps' own animation so they meet, keeps moving gently while on screen and stops when off screen or hidden.
The hero's Call us button rings every 6 s (its phone shakes, a soft ring grows around it). Smooth ease-outs only, no bounce. With `prefers-reduced-motion` everything simply shows, on a still wave in
colour. Smooth scrolling to the sections, off under `prefers-reduced-motion`.
