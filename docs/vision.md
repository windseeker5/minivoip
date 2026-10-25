# Long-term vision

Last update: 2026-10-24 · Agreed with Ken. Read this before planning any new feature.

## The goal

One simple place where a small business talks with its customers. Visitors use any browser; the owner uses the same
app pages in a browser, on a computer, or inside a small phone app that rings like a real call. No phone line. If someone who only knows how to use a phone can't use it without
help, it isn't done.

## Our edge: anyone can reach you

Google Chat, Teams and Slack are closed rooms where only coworkers get in. A friend can't call Ken on his TELUS
Google Chat. minivoip is the opposite: one public link or button, and anyone can reach the owner from any browser.
Nothing to install on either side, nothing to deploy for the owner beyond pasting a button. Cheap and simple.

## How it grows

1. **Calls come in.** A customer clicks "Call us" on the website, hears the owner's voice menu, and the owner answers
   on whatever they have: phone, computer or tablet. No answer: voicemail. (Built; works, but confusing to use.)
2. **One app everywhere.** The same web app on the phone and the computer. Every device rings; answer on any of them.
3. **A meeting room.** Each business gets its own room link to share. People click and join with no account and
   nothing to install. Voice, video, screen sharing.
4. **Whiteboard and meeting notes.** Draw together during the meeting. Notes are written up (French, English or
   other languages) and saved for the owner afterwards.
5. **More people.** Several customers or team members in one room.

## Two plans

Named Basic and Premium on 2026-10-24 (before: Voice and Meetings).

| Plan | Price | What's in it |
|---|---|---|
| **Basic** | Really cheap (landing page placeholder: $2 a month) | "Call us" button, voice menu that answers common questions and sends callers to the right page, rings on every device, voicemail. Never video, whiteboard or AI. |
| **Premium** | About $5 a month (placeholder) | Everything in Basic, plus AI front desk (answers from the owner's FAQ), video, screen sharing, whiteboard, a team meeting room, recording and meeting notes |

The pitch (2026-10-24): "minivoip puts a Call button on your website. Your voice menu answers the common
questions, and when a visitor wants more, you talk to them and show them your screen, in one click. No phone line,
nothing to install." Premium's main pain: an interested visitor gets a live demo right away, before they cool off
(Ken wants it for minipass first); team meetings without free-tool time limits are a bonus.

Why the prices differ:
- Basic costs almost nothing to run: the call goes straight between the two browsers.
- Premium costs real money: recordings take storage, notes cost a little per meeting hour, and rooms with three or
  more people need the server to carry the video. The Premium plan has to cover that, probably with a monthly limit
  on note hours.

## Decisions

- **One set of app pages, with small phone apps around them for ringing** (changed 2026-10-24; before: web app
  only). Ken's Pixel 9 test showed a web app's alert arrives in half a second on a locked phone, but silently: Android
  and iPhone only let installed apps ring like a call. So:
  - Computer: the web app (a browser window); full ringing while it's open.
  - Android: a small Kotlin app (`android/`) showing the same `/app` pages, plus a real ring (Firebase push, the
    phone's ringtone, the call screen over the lock screen). Installed from our site for now, Play Store later.
  - iPhone: next, the same idea (Apple's call screen; $99 USD a year Apple account, built on a rented Mac). The family's
    iPhones are the test phones. Until then, the iPhone web app only gets a short alert.
  - Safety net when nobody answers: voicemail, later "leave your number, we call you back".
- **The voice menu is the junk-call filter.** An open door invites junk callers; the menu keeps robots out and is a
  selling point ("only real people get through").
- **Simple first.** Minimalist, easy to understand, intuitive. Today's screens are not; making them simple comes
  before adding features.

## Not decided yet

- Prices for each plan.
- Privacy wording. The landing page's "nothing is recorded" was a placeholder; recording and notes will change it
  (likely "recorded only when you turn notes on").
- How group rooms carry video (a forwarding server is needed from three people on).
- Real prices. The AI front desk runs on Ken's own AI server, so its cost per call is still to be seen.
