# Zadarma — test project

> **Status: plan only, written 2026-10-19. Not tested.** Setup steps are in `zadarma-quickbook.md`; this file is the test script, the cost log and the pass/fail rule.

## 1. What we're testing

Does Zadarma do exactly what the friend asked (call → French menu → 1 / 2 information → 0 reaches the owner), from a phone number **and** from a button on a web page, for **less than US$5/month all in**?

If yes, it's the no-brainer: no server to maintain. The homemade project (`homemade-test-project.md`) is the $0 alternative.

## 2. Before starting (at home)

| Item | Why |
|---|---|
| Personal email + personal phone | Account sign-up |
| ID or passport (photos of both sides) + home address | Canadian number verification. **Checking can take up to 1 business day**, so the number may not be active tonight. |
| Credit card or PayPal | Prepaid top-up (minimum amount not published; plan ~US$10–20) |
| `omt.dresdell.com` | To host the Click to Call button (needs HTTPS: already OK) |
| Android + iPhone | App and caller tests |

## 3. Plan for tonight

The number may wait for verification, so the test is split in two.

**Part A — tonight, without a number** (quickbook sections 3–5)
1. Sign up, price plan **Standard**, currency USD, region USA/Canada (quickbook §3).
2. Set up the phone system: 1 extension, voice menu (§4 steps 1–5). The phone system is free to try for 7 days.
3. Install the Zadarma app on the Android (and iPhone), log in as extension 100.
4. Create the Click to Call button, point it at the voice menu, paste the code into a test page on `omt.dresdell.com` (§5 B).
5. Run tests Z1–Z8.
6. Order the Quebec number, yearly, upload the ID (§4 step 6).

**Part B — when the number is active** (tomorrow or later)
- Run tests Z9–Z13.

## 4. Test script

| # | Test | Expected | Result |
|---|---|---|---|
| Z1 | Text-to-speech ("Read") offers a French voice | Yes, or upload an MP3 instead | |
| Z2 | Click to Call on `omt.dresdell.com` from **desktop Chrome** | Browser call reaches the French menu | |
| Z3 | Same from **iPhone Safari** and **Android Chrome** | Works; note how the screen looks (does it look like a phone call?) | |
| Z4 | The button gives a **keypad** to press 1 / 2 / 0 | Yes. If not: note it, and point the button at extension 100 instead | |
| Z5 | Key 1 / key 2 | Message plays, returns to the menu (or hangs up) | |
| Z6 | Key 0, Zadarma app on **Android**, phone locked | Rings like a normal call | |
| Z7 | Key 0, Zadarma app on **iPhone**, phone locked | Rings like a normal call | |
| Z8 | App closed / phone offline, key 0 | After ~20 s, forwards to the owner's cell (needs balance) | |
| Z9 | Dial the Quebec number from a regular cell | Same menu as Z2 | |
| Z10 | Key 0 from a regular call → owner's cell | Cell shows the caller's number | |
| Z11 | Two calls at the same time | Note whether the second caller gets through | |
| Z12 | Outbound call from the app | Person called sees the Quebec number | |
| Z13 | Rate lookup for the owner's cell number | $0.005/min group, not "CANADA NPA-1" ($0.104–0.13) | |

## 5. Cost log (fill in from the account's billing history)

| Date | What | Amount (USD) | Recurring? |
|---|---|---|---|
| | Top-up | | No (prepaid balance, not a cost) |
| | Quebec number | | Yearly / monthly |
| | Forwarding minutes during tests | | Per use |
| | Taxes on the receipt | | |

### Monthly cost calculation

Use the same example as the quickbook: 100 calls/month, 20 press 0 and talk 3 minutes.

| Line | Value |
|---|---|
| Number (yearly price ÷ 12, or monthly price) | |
| Phone system | $0 if the balance is topped up at least once every 3 months, otherwise $5 |
| Forwarding to cell: 60 min × rate (or $0 inside Standard's 100 free min, which needs a top-up every month) | |
| "0" answered in the Zadarma app instead of the cell | $0 |
| Taxes (if added on top) | |
| **Total per month** | |

Watch for the traps: the phone system becomes **$5/month** if there's no top-up for 3 months, and the 100 free minutes need a **monthly** top-up. Top-ups are not a cost (the money stays on the balance), but they're a monthly chore for the friend.

## 6. Pass / fail

- **Pass (no-brainer):** Z2, Z5, Z6, Z7, Z9, Z10 work and the monthly total is **< US$5**.
- **Partial:** works by phone number, but the web button can't reach the menu or has no keypad (Z2–Z4). Still a candidate; the website just uses a `tel:` link.
- **Fail:** total ≥ US$5/month for this usage, or the app doesn't ring a locked phone.

## 7. Scorecard (same rows as the homemade test, for the recommendation)

| Criterion | Result |
|---|---|
| Locked iPhone owner gets the call | |
| Locked Android owner gets the call | |
| Guest screen looks like a phone call (web button) | |
| Guest can press 1 / 2 / 0 (web button) | |
| People can dial a regular phone number | Yes (once verified) |
| Setup time (actual) | |
| Monthly cost | |
| Who maintains it | Zadarma (friend tops up the balance) |
| Friend can change messages alone | |
