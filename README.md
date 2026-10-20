# minivoip

A simple phone line for a small business that doesn't want to deal with a real phone system. A visitor clicks
**Call** on the website and gets a French voice menu: **1** and **2** give information, and **0** reaches a real
person.

It started on 2026-10-19 as a request from Ken's friend, who runs a B&B with no one at the front desk. The proof of
concept runs on Ken's own site, omt.dresdell.com, about Ken himself rather than the B&B.

## Two options under test

| | Option 1: **custom** (homemade) | Option 2: **zadarma** (provider) |
|---|---|---|
| What it is | Our own code. The web page fakes the phone call; the owner answers in a web app on the phone. | A cloud phone system (Zadarma). A real number, a real voice menu and a phone app. |
| Real phone number | No, website only | Yes (a Quebec number, once ID is checked) |
| Monthly cost | $0 (runs on our server) | About US$2/month, if the number is paid yearly (see `zadarma/STATUS.md`) |
| Status | **Built and working** on omt.dresdell.com. Voice works on Wi-Fi and cellular. | **Account created, US$10 added.** Phone system not set up yet. |
| Big open question | A locked phone only gives **one short notification sound**, not a ring. | Does their app really ring a locked phone like a normal call? Is the pricing real? |
| Details | [`custom/STATUS.md`](custom/STATUS.md) | [`zadarma/STATUS.md`](zadarma/STATUS.md) |

## Where things are

| What | Where |
|---|---|
| Live "Call me" button | https://omt.dresdell.com/ (in the contact line, next to the email) |
| Call screen alone | https://omt.dresdell.com/appel/ |
| Owner app (install on the phone) | https://omt.dresdell.com/call/owner/ (passcode in `custom/callbox/.env`) |
| Custom option source code | `custom/callbox/` (runs from here; see `custom/STATUS.md`) |
| How it works, as a picture | `custom/explainer.html` (also published: https://claude.ai/artifact/WedNiBxF4ZpzFCTRjWpeXo) |
| Original test plans | `custom/test-plan.md`, `zadarma/test-plan.md` |
| Zadarma account | https://my.zadarma.com/ (Ken logs in himself) |

## The idea beyond the B&B

Option 1 could become a small **SaaS for small businesses**: "a call button on your website, no phone number
needed". Notes on what that would take are at the end of `custom/STATUS.md`.
