# Option 2: Zadarma (provider) — status

Last updated 2026-10-20.

## Where we are

- **2026-10-19:** Ken created a Zadarma account and added **US$10** to the balance. Panel: https://my.zadarma.com/.
  Ken logs in himself; Claude never types his password or card details.
- Nothing is set up inside the account yet: no voice menu, no app login, no Click to Call button, and **no
  phone number ordered**. The website test doesn't need a number.
- Ken doesn't believe the price is that low. The plan is to **prove it with the account's billing history**, not
  their website.
- Ken has a **real IP desk phone** (brand and model not given yet). Zadarma speaks SIP, so it can register as
  extension 100 or a second extension. A call to 0 could then ring the desk phone and the cell app at the same time.

## What Zadarma is

A cloud phone system:
- a real phone number
- a voice menu (IVR) with text-to-speech or uploaded MP3s
- extensions, voicemail
- a phone app for Android and iPhone (a "softphone")
- **Click to Call**, a website button that calls from the browser (WebRTC) into the voice menu

Calls inside Zadarma (website button → menu → app or desk phone) are free; you pay only for the number and for
calls to regular phones.

## Pricing (read from Zadarma's own pages on 2026-10-19, USD, taxes included)

| Item | Price |
|---|---|
| Standard plan | $0/month |
| Phone system + voice menu | $0, **if the balance is topped up (any amount) at least once every 3 months**, otherwise $5/month |
| Click to Call website button | $0 (the widget is free) |
| Quebec number (418, 450, 819, Montreal 438; no 514 listed) | **$2/month paid yearly ($24/yr)**, or $4/month paid monthly, $0 setup |
| Incoming calls to the number | $0 |
| Calls to Canada | $0.006/min, or free within 100 free minutes/month (needs a top-up **every month**) |
| Office / Corporation plans | $39 / $79 a month: call-centre plans, not needed |

**Example** (100 calls/month, 20 press 0 and talk 3 min):
- **$2.00–2.36/month**, about C$3.
- It goes up to about $7/month if nobody tops up for 3 months.

**Not verified yet:**
- the minimum top-up
- card or currency fees
- whether the owner's cell prefix is charged $0.006/min or about $0.10–0.13/min (some Canadian cell numbers are);
  answering in the app is always $0
- iPhone locked-screen behaviour
- what happens after the 7-day phone-system trial

App reviews are mixed: some battery drain, clunky setup, and one report of calls not coming through while the
account was still being verified.

## Next steps (in the panel; menu names may differ slightly)

1. **Extension:** My PBX → Extensions. Extension 100 should exist. Set a password and note its login (like `123456-100`).
2. **App:** install "Zadarma" on the Android and log in as extension 100.
   - Allow notifications and the microphone.
   - Test: call extension 100 with the phone **locked**. Does it ring like a real call?
3. **Voice menu:** My PBX → Incoming calls / Voice menu.
   - Upload our French MP3s from `~/Documents/DEV/minivoip/custom/callbox/static/appel/audio/`: `greeting.mp3` + `menu.mp3`,
     `key1.mp3`, `key2.mp3`.
   - Keys: 1 and 2 play their message, 0 goes to extension 100, no answer goes to voicemail.
4. **Click to Call:** create the button (under My PBX or Integrations).
   - Destination: the voice menu.
   - Allowed site: `omt.dresdell.com`.
   - Ken pastes the code to Claude.
5. **Claude** puts it on a **hidden test page** on omt.dresdell.com. The live "Call me" button stays as it is.
   - Where it goes isn't decided yet: an OMT route, or a static page served by callbox.
6. Run the tests in `test-plan.md` §4 (Z1–Z8).
7. Check the **billing history** after a few days, and after day 7 when the trial ends.
8. Optional: register the IP desk phone.
9. Only if the website test passes: order a Quebec number ($4 for one month, or $24 for the year, plus ID photos;
   verification takes up to 1 business day). Then run Z9–Z13.

## Sources

- https://zadarma.com/en/tariffs/numbers/canada/
- https://zadarma.com/en/tariffs/calls/standart/
- https://zadarma.com/en/services/pbx/
- https://zadarma.com/en/tariffs/plans/
- https://zadarma.com/en/services/click-to-call
- https://zadarma.com/en/support/instructions/android/zadarma/
