# Premium notes: the loop plan

After every inbound call, meeting room and voicemail on Premium, the owner finds the full transcript, a one-page
summary and the next actions in the dashboard. No email. Written 2026-11-04 with Ken.

Start it with: `/loop read docs/notes-plan.md and do the next step`

## How each pass works

1. Read this file, `AGENTS.md`, and `PRODUCT.md` + `DESIGN.md` if the step has a screen. Stay on branch `call-notes`.
2. Do the **first unticked step** and nothing else. Read the code it touches first.
3. Test it (below). A step isn't done until its tests pass and new behaviour has its own test.
4. Update the docs in the same pass, tick the step here, add one line under "Log" (what's done, anything odd), and
   commit on `call-notes`. Never push. Shipping waits for Ken's "ship it".
5. Say in one or two plain lines what's done and what's next, then schedule the next pass. Say what you're doing
   every few minutes on long work.
6. All ticked: end the loop and report (step 10).

**Fix, don't wait.** Make the sensible choice, note it in the Log, and carry on. Stop the loop only if:
- Ken's server isn't answering on http://127.0.0.1:8090 (browser tests can't run, and he must start it);
- the same test still fails after three real attempts at a fix (report what you tried, with the output);
- something would need a new Go or browser dependency, or a choice that changes what callers are told.

## Rules that always apply

- Never start, stop or restart the server, and never run `templ generate` or `task build`. The watcher rebuilds.
- Tests: `go vet ./...`, `go test ./...`, then `tests/e2e/site.mjs`, plus `app.mjs`, `call.mjs` or `meet.mjs` when
  those screens change. Headless only. Never press Call on Ken's line. Look at the screenshots, phone and desktop.
- Extend `site.mjs` for every new page and flow, reached from the pages around it, and leave no test data behind.
- New database columns are new migrations (next is 14), added to `docs/data-dictionary.md`.
- Security, in every step: only the line's logged-in owner can see or upload anything; check Premium on the server,
  not just on the page; limit sizes and counts; show transcripts as plain text. What callers say is untrusted: it
  can never give the summary instructions or reach anything but its own note.
- Go tests use fake services, so they never spend money or need keys. Talk to the services with plain `net/http`.

## Choices already made

Ken decided:
- Notes are Premium only, and live in the dashboard. No email.
- Each account has French or English, chosen at sign up. French unless the user sets otherwise. Existing accounts
  get French.

Proposed by Claude; Ken can overrule any of these before starting:
- The **summary and next actions** follow the account's language. The **transcript** keeps the words as spoken.
- The language setting doesn't translate the site, which stays in English.
- Transcripts: ElevenLabs Scribe, about $0.22 an hour on its price page in July 2026, with speaker labels and French.
  Check the price again before relying on it. Summaries: Claude (load the `claude-api` skill when writing that code).
  Both sit behind a small interface, so either can be swapped.
- 10 hours of notes per account per month, shown in the dashboard. Past that, recordings aren't taken and the owner
  sees why. A daily ceiling across all accounts protects Ken's bill.
- A "Take notes" switch per account, on by default.
- Call and room audio is deleted once transcribed. Voicemail audio stays as today. Notes last a year, like call
  history, or until the owner deletes them.

Needs Ken: an account and key for each service, in `.env`, then a restart of `task dev`. The loop doesn't wait for
them: without keys, new notes say "Notes aren't set up yet", and everything is tested with the fake services.

## The steps

- [x] **1. Language.** `account.language` (`fr` or `en`). A choice on the sign up form, French already selected; a place
  to change it from the account menu; a store function and tests.
- [x] **2. The notes engine, with no audio source yet.** A `note` table: account, line, kind (voicemail, call, room),
  status (waiting, working, done, failed, skipped), language, transcript, summary, next actions, minutes, dates. A
  background worker that survives restarts, retries a few times and handles one note at a time. The two service
  clients. The monthly and daily limits. Keys through `.env`: add `dotenv` to `Taskfile.yml` and ship `.env.example`.
  The summarizer's prompt treats the transcript as data. Notes are cleaned up after a year.
- [x] **3. Voicemail notes.** A Premium voicemail queues a note after upload; deleting the voicemail deletes it. Very
  short messages are skipped, so silence costs nothing. Robots can't spend Ken's money past the limits.
- [x] **4. The Notes tab in the dashboard.** In the side menu and bottom tabs, Premium only (Basic sees the usual
  "Premium" tag). A list, newest first, and a one-page view: summary, next actions, then the transcript with who said
  what. Waiting, failed and "not set up" states. Hours used this month. The "Take notes" switch. Delete. A voicemail
  in Messages links to its note. Follow `DESIGN.md`, and keep the screenshots for Ken.
- [x] **5. Telling callers.** When notes are on: a written line in the call window and a spoken one before the call, in
  the account's language, also on the voicemail path and at the meeting room's door. Works when the owner recorded their
  own welcome. Nothing is recorded before this has been shown. Update `call.mjs`. Tell Ken in the final report to check
  the rules where he operates: this isn't legal advice.
- [x] **6. Inbound calls.** In the owner's app, once the call is answered: mix the owner's mic and the caller's voice,
  ideally on separate channels so each is labelled, and record in the browser (`MediaRecorder`). Upload a piece every 30
  seconds, so a crash loses little. An endpoint for the line's logged-in owner only, tied to a real call, with size
  limits. The note is queued when the call ends; the audio is deleted once transcribed. Must not disturb the call: if
  recording fails, the call goes on. Change the rule "the server never handles audio" in `CLAUDE.md` and
  `docs/architecture.md`: still no live audio, but recordings are uploaded when notes are on. Test in `meet.mjs`.
- [x] **7. Meeting rooms.** Same recorder in the host's browser, mixing everyone in the room, joining and leaving included.
  Kind `room`, with the people's names. Guests are told at the door. Test with a real room in `site.mjs`.
- [x] **8. Android and other browsers.** Check the recorder's fallbacks in code: Safari records mp4, the Android web view
  must keep recording when the screen locks, or the note says what was missed. Fix what code and headless tests can
  reach, and list what only a real phone can show.
- [x] **9. The words.** `docs/landing-copy.md` first, then the page: notes are no longer "Soon", and the summary is in the
  dashboard, not "in your app". Mention the monthly limit. Update `docs/vision.md`'s privacy line. Check the promises
  on the page still hold now that calls can be recorded.
- [x] **10. Finish.** An independent security and bug review of the whole branch by an agent that hasn't seen the work,
  and fix what it finds. If keys are set, one real French voicemail end to end, and read the result. Three full
  `site.mjs` runs. Update `AGENTS.md` Current state. Then end the loop and report in short plain English: what works,
  screenshots of the Notes tab, what isn't tested, what Ken must do (keys, one real call, a voicemail). Not shipped.

## Real phones

What only a real phone can show, once keys are set and Android 0.5 is on it:

1. **Android, screen locks during a call:** does the caller still hear the owner, does the recording keep both voices, and
   does the note stay silent about missing audio? 0.4 had no service for this, so the owner may have gone silent.
2. **Android, answered from the lock screen:** Android must allow the microphone service from there; if it refuses, the call
   goes on but the note should say what was missed.
3. **Android, the "On a call" notification:** it shows during a call and goes after; a tap returns to the call.
4. **iPhone or Safari on a Mac:** does a call record the caller's voice? Older Safari gave silence for a caller's voice
   in this kind of recording. Do the mp4 pieces make one file ElevenLabs reads?
5. **iPhone, screen locks:** Safari stops the page, so the call itself likely ends, as it does today. The note should say
   what was missed.
6. **Long calls:** a 30-minute call on a phone: battery, heat, every piece up.

## Log

(One line per pass.)

- 2026-11-04, step 1: `account.language` (migration 14, ran on the dev database, existing accounts French), the choice on sign up, `/dashboard/language` from the account menu. A made-up language at sign up becomes French; on the page it is refused. `CreateAccount` keeps its arguments and sign up sets the language right after, so a failure there leaves a working French account.
- 2026-11-04, step 2: `notes/` (worker, Scribe, Claude), `store/notes.go`, migration 15, `.env.example`, Taskfile `dotenv`. Choices: past the day's ceiling a note is skipped, not held for tomorrow; a note past a limit or without keys loses a call's recording; hours live in `note_usage` so deleting a note doesn't give them back; Scribe hears the language itself. Claude is `claude-opus-5-5` (about $0.10 an hour of audio), the default for now; Ken can pick a cheaper one. Price of $0.22 an hour for Scribe v2 seen only on third-party lists, one says $0.27: Ken checks when he gets the key. For step 6: separate channels cost double at Scribe (each billed in full). Nothing has reached the real services yet.
- 2026-11-04, step 3: a Premium voicemail queues its note after upload (migration 16, `note.voicemail_id`), deleting the message deletes it. Choices: under 2 seconds gets no note row at all, so the Notes tab isn't full of hang-ups; a line's voicemails get 30 notes a day, past that the note says so (the message still arrives), so a robot can't use up the owner's hours or the day's ceiling. Once Ken sets keys, each `site.mjs` run spends a few cents on real services (its Premium voicemails). In `site.mjs` the voicemail choice now records 2.5 s, past the 2 second minimum.
- 2026-11-04, step 4: the Notes tab (`web/notes.go`, `pages/notes.templ`, `notes.js`), migration 17 (`account.take_notes`), a fifth tab on the phone, Messages link to their note and back. Choices: delete works on any plan and has no confirm step, like Messages; Basic can't open a saved note but is told they're kept; speakers read "Person 1", "Caller" on a voicemail (step 6 can name them). `site.mjs` writes sample notes straight into the database, since there are no keys. Screenshots for Ken: `tests/e2e/shots/notes*.png`, `note*.png`. The switch's text says callers are told first: step 5 makes that true.
- 2026-11-04, step 5: callers are told (`lines.Notice`) in writing on each step, out loud before the menu and before the tone, at the room's door. Ken should read the exact words: FR "Votre conversation ou votre message sera enregistré et résumé automatiquement pour {business}.", EN "Your conversation or message will be recorded and summarized automatically for {business}." Choices: no consent button, as planned: a caller who doesn't agree hangs up; they don't name the outside services (ElevenLabs, Anthropic); told whenever the switch is on, even without keys or past the limit. The account's language applies, so an English caller to a French account gets French. Odd: the computer voice still reads owners' greetings as en-US even on French accounts, a French greeting sounds wrong; left, since existing accounts default to French with English starter words. **Final report: tell Ken to check the rules where he operates, this isn't legal advice.**
- 2026-11-04, step 6: the app records answered calls (`assets/js/recorder.js`, `web/recording.go`, migration 18), the owner and the caller on separate channels, 30-second pieces, tied to the live call. Choices: separate channels cost double at Scribe (about $0.44 an hour instead of $0.22, so 20 hours a day is up to about $11), for reliable "You"/"Caller" labels; the hours shown count the call once. No keys or past a limit: not recorded at all, and the note says why. Scribe's multi-channel answer isn't verified against the real service: the client takes both documented shapes; step 10 checks if keys are set. Tested with a real WebRTC call in `meet.mjs` and a fake upload server, and the endpoints in Go; a real upload on the real server needs keys to be set, since without them nothing is recorded.
- 2026-11-04, step 7: the host's browser records meetings (mixed on one channel), kind `room`, with who came (`note.people`, migration 19). Choices: recording starts with the first guest let in, not when the host opens the room; with nobody left for a minute it ends, and the next guest starts a new note, so an empty room costs nothing; speakers read "Person 1", since naming them would mean a channel each, at up to 4 times the price. `site.mjs` runs a real room with its real sound and catches the upload in the test, since without keys the server records nothing.
- 2026-11-04, step 8: the recorder handles Safari's mp4 (a real mp4 recording passes in `meet.mjs`), no recorder at all (the note says so), and a stopped recorder (what's in goes up); the note says what it missed (migration 20). Android 0.5 adds a microphone service for locked screens (`CallAlive`), built but not published and not on a phone: Ken publishes it when shipping (`android/README.md`). The service also fixes something that wasn't about notes: on 0.4 a locked screen likely silenced the owner in the call. Worth knowing: "missed" is an estimate from the browser, and over-reports rather than under. What only a phone can show: "Real phones" above.
- 2026-11-04, step 9: `docs/landing-copy.md`, then the page: notes are no longer "Soon", they're in the dashboard, 10 hours a month. The page never said "nothing is recorded" (only `PRODUCT.md` did, now fixed), and "yours stays private" is about the phone number, so it holds. Choice: a new question, "Are calls recorded?", answers it plainly, naming outside services without their names. Ken can drop it, but a business owner will ask. For the final report: existing Premium accounts, Ken's line included, start with notes ON when this ships and keys are set; Ken should tell those owners or turn it off for them.
- 2026-11-04, step 10: an independent review of the whole branch found real problems; all fixed but those below. Fixed: the meter undercounting a mostly silent call (it now charges by size and the service's own length, and the day's ceiling counts what's billed); robots using up an owner's month through voicemails (voicemails have their own budgets of real audio, and cost the month 5 minutes each at most); recording going on after "Take notes" was turned off or the plan changed; the disk cap beaten by many recordings at once (real-time growth, 3 open, 30 starts an hour); a note closing mid-call after 2 minutes without a piece (now 15, or 2 once the call has ended); dropped audio not counted as missed; an expired login swallowing pieces; a caller whose page loaded before notes were on being recorded untold (the server now knows who was told); and smaller ones (deleted notes still summarized, notes stuck on a database error, a piece written twice, a guest landing on the last meeting's note, the rooms' lock held during database work, "done" sent before the last piece, the Android service stopped before it started). Left: the 30-a-day voicemail count can be beaten by a burst, up to the line's 60, but the budgets of real audio still hold; if saving a transcript fails, the retry pays the service again, 4 times at most. Keys weren't set, so no real voicemail was tried: Ken does that.
