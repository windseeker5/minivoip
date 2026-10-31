# Landing page copy

Last update: 2026-10-31 · Source of truth for the words on `/` (`pages/home.templ`). Context: [../.agents/product-marketing.md](../.agents/product-marketing.md).
Look: "Voice" (with the Spectrum colour band and the Swiss code example), picked by Ken on 2026-10-25 from ten mocks (DESIGN.md "Landing page"). Changes to the words are noted below.

## The message (Ken, 2026-10-31)
Lead with a real person, one click away: with AI answering everywhere, that's the surprise. The voice menu is the filter, not the headline. The story: **it filters** (the voice menu), **you answer** (phone, or computer with a headset), **you show them** (screen, whiteboard: the wow). AI only takes notes on Premium, later; it never talks in the owner's place. Basic is never sold as free; a promo code makes it free. The page also tests the market with Ken's friends and social network.

## Elevator sentence
minivoip puts a Call button on your website. Your voice menu answers the easy questions. When a customer needs you, your phone and your computer ring, and you can share your screen or draw together in seconds. No phone line, nothing to install.

FR : minivoip ajoute un bouton « Appelez-nous » à votre site. Votre menu vocal répond aux questions simples. Quand un client a besoin de vous, votre téléphone et votre ordinateur sonnent, et vous pouvez partager votre écran ou dessiner ensemble en quelques secondes. Pas de ligne téléphonique, rien à installer.

## Meta
- Title: minivoip: one click, and they're talking to you
- Description: the elevator sentence.

## Hero
- Headline: One click, and they're talking to you.
- Sub: A Call button for your website. It rings your phone and your computer, and your cell number stays private.
- The voice player (a stand-in until the hero animation): minivoip's own line's recorded greeting (play and pause), the wave, "minivoip's own line · the real recording" and its length. Under it, the greeting and the spoken menu written out from the line itself. Without the line: an example menu, labelled "Example of a voice menu", no play button.
- CTA: Get my line · Call us now, it's the real thing (minivoip's own line, `MINIVOIP_CALL_LINE`; without it: See how it works)
- Microcopy: $2 a month · No phone line · Nothing to install

## Then your own phone rings (the colour band)
- Heading: Then your own phone rings.
- Text: Or your computer, when you're at your desk with a headset. You see what they chose before you answer. Busy? They leave a voicemail and you get an alert.
- Picture: the owner's phone ringing, "Someone chose “<the line's first ringing choice>”", and a "New voicemail · From your website · 0:38" alert.

## How it works (`stages`)
- Heading: It filters. You answer. You show them.
- Sub: Most visitors only need an answer. Some need you. A few need to see it. minivoip handles all three in the same call.
- **It filters.** Your voice menu answers the easy questions: hours, prices, directions, or a link to the right page of your site. Recorded in your own voice. (Basic)
- **You answer.** Your phone rings, and so does your computer. Pick up on the go, or at your desk with a headset. Away? They leave a voicemail and you get an alert. (Basic)
- **You show them.** Share your screen in seconds, draw on a whiteboard together, or turn on video. The demo happens while they're still interested. (Premium)

## Plans (`plans`)
- Heading: Start with Basic. Grow into Premium. · Sub: One price a month. No per-minute charges.
- **Basic** · $2 / month · A business line without giving out your cell.
  Call button, call link and QR code · Voice menu in your own voice · Sends callers to the right page of your site · Rings your phone and your computer · Voicemail with an alert. CTA: Get my line
- **Premium** · $10 / month · Talk, show, close the deal.
  Everything in Basic · Share your screen in seconds · Draw together on a whiteboard · Video, and a meeting room for your team · *Soon:* Notes taken for you, with a summary in your app after each call.
  No payments yet, so Premium starts with an email: "Leave your email. I'll set you up and tell you when the call notes are ready." [you@business.com] **Get Premium** → "Thanks! I'll write to you soon to set up Premium." (`POST /premium`, the admin page's "Asked for Premium".)
- Note: Got a promo code? Enter it when you sign up and it's free.
- Basic never gets video, whiteboard or AI (Ken, 2026-10-24). The AI front desk is kept off the page (2026-10-31): the pitch is a real person.

## Maker
“I build software. I wanted people to reach me in one click from any of my websites and hear a real voice. Now my computer rings, I put on my headset, and I show them my screen right away. It's on all my sites.” Ken Dresdell, maker of minipass and minivoip.

## Ready in three steps (`steps`)
- Beside the steps: "Paste this on your site", the code `<a href="<this site>/call/your-business">Call us</a>`, and a picture of the orange Call us button it makes: "That's the button it makes. No website? Share the link, or print it as a QR code for your door."
1. **Record your menu.** Say hello and set the choices, right from your browser.
2. **Get the app.** The Android app rings like a real call. On a computer or an iPhone, use the web app.
3. **Add the button.** Paste one line on your site, or share your call link and QR code.

## Questions (`faqs`)
Only the worries a first visitor really has (Ken, 2026-10-31). Real questions will come in by call; add the ones that keep coming up, drop the ones nobody asks.
- **Can people call me from a regular phone?** Not yet. Calls come from your website, your call link or a QR code. A real phone number is planned as an add-on.
- **Will my phone ring like a normal call?** On Android, yes: the minivoip app rings with your ringtone and shows the call screen, even when your phone is locked. Computers ring while the app is open. On iPhone, for now you get a notification with a sound and answer by tapping it; a ringing iPhone app is next.
- **Does the visitor need to install anything?** No. They click the button and talk from their browser, after allowing the microphone.
- Ending: Another question? Press Call and ask me. [Call] (minivoip's own line, in the call window; without the line: "Another question? Write to me", an email link)

## Final call and footer
- Heading: Let your customers reach a real person.
- Body: $2 a month. Set it up from your browser.
- CTA: Get my line · Call us now
- Footer: minivoip, part of the mini family, with minipass and minicrm. Get the Android app. Contact: kdresdell@minipass.me

## Sign up page
- Title: Get your line · Sub: No card needed for now. Got a promo code? Enter it below.
