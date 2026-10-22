# Long-term vision

Last update: 2026-10-22 · Agreed with Ken. Read this before planning any new feature.

## The goal

One simple place where a small business talks with its customers. It works in any browser (iPhone, Android or a
computer), with no app store and no phone line. If someone who only knows how to use a phone can't use it without
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

| Plan | Price | What's in it |
|---|---|---|
| **Voice** | Really cheap (amount not set yet) | "Call us" button, voice menu that filters calls, rings on every device, one voicemail |
| **Meetings** | About double Voice | Everything in Voice, plus meeting room, whiteboard, screen sharing, recording, meeting notes |

Why the prices differ:
- Voice costs almost nothing to run: the call goes straight between the two browsers.
- Meetings cost real money: recordings take storage, notes cost a little per meeting hour, and rooms with three or
  more people need the server to carry the video. The Meetings plan has to cover that, probably with a monthly limit
  on note hours.

## Decisions

- **Web app only (PWA), no store apps.** One app for iPhone, Android and computers. Store apps would ring better on
  phones, but Apple's $99 USD a year and two separate apps don't fit a $5-a-month product with an unproven market.
  Revisit only once there are paying customers who need it.
- **What a web app can do for ringing, accepted as the limit:**
  - Computer: full ringing (looping ringtone, call window) while the app or a tab is open.
  - Android: good, once the owner sets the app's alerts to a ringtone, allows the lock screen and turns off battery
    saver for it. The app must guide them through that and offer a "Test my ringing" call.
  - iPhone: a short alert sound only; no ringtone and no full call screen (Apple's rule for web apps).
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
- The landing page copy (`docs/landing-copy.md`) and `.agents/product-marketing.md` still describe Voice only;
  update them when the landing page is reworked.
