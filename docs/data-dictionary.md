# Data dictionary

Last update: 2026-11-04 · One SQLite file: `data/minivoip.db` (or `MINIVOIP_DB`). Schema and migrations: `store/store.go`. Change this file in the same change as the schema.

## account
One customer (a business). Created at sign up.

| Column | Type | Meaning |
|---|---|---|
| `id` | integer, key | |
| `name` | text | For now the sign-up email; a business name later |
| `promo_code` | text, nullable → `promo_code.code` | The code used at sign up, which is required since 2026-11-02. Set = free forever (while the code isn't revoked) |
| `plan` | text: `basic` or `premium` | What the account gets. Basic: calls, voice menu, voicemail. Premium adds video, screen sharing, the whiteboard and the meeting room (`store.Plan`). From the promo code at sign up, else `basic`; admins switch it on `/admin/promo` |
| `language` | text: `fr` or `en` | What the account's call notes are written in (`store.Language`). Chosen at sign up, French unless English is picked; changed at `/dashboard/language`, from the account menu. The site itself stays in English |
| `created_at` | text (UTC `YYYY-MM-DD HH:MM:SS`) | |

## user
A person who logs in. One per account for now (staff accounts later).

| Column | Type | Meaning |
|---|---|---|
| `id` | integer, key | |
| `account_id` | integer → `account.id` (delete cascades) | |
| `email` | text, unique, case-insensitive | Login name |
| `password_hash` | text | bcrypt hash; the password is never stored |
| `is_admin` | integer 0/1 | Can manage promo codes (set it with SQL) |
| `created_at` | text (UTC) | |

## session
A login. The token is the `mv_session` cookie (`__Host-mv_session` behind HTTPS; HttpOnly, SameSite=Lax); the database keeps only its SHA-256, so a copy of the database can't log anyone in. Lasts 30 days; expired rows are deleted daily (`Store.Cleanup`).

| Column | Type | Meaning |
|---|---|---|
| `token` | text, key | SHA-256 of the cookie's 64 random hex characters |
| `user_id` | integer → `user.id` (delete cascades) | |
| `expires_at` | text (UTC) | Ignored after this time |
| `hashed` | integer 0/1 | 1: `token` is a hash (always, once the app has started on migration 12) |

## line
A phone line: the call link `/call/<name>` and its voice menu. One per account.

| Column | Type | Meaning |
|---|---|---|
| `id` | integer, key | |
| `account_id` | integer → `account.id` (delete cascades) | |
| `name` | text, unique | The call link's last part. Made from the business name (`lines.Suggest`, `-2`… if taken) unless the owner changes it; lowercase letters, digits, dashes, 3–30; can't be changed. Some names are reserved (`lines.CheckName`) |
| `business` | text | Shown on the call page |
| `owner_name` | text | Name callers hear in "Ringing …" and "On the line with …". Since 2026-10-21 it is the business name (sign up no longer asks for a first name) |
| `greeting` | text | Welcome message, read by the browser's voice |
| `greeting_audio` | text | The owner's recorded welcome (file under `prompts/`), or empty. When set, callers hear it instead of `greeting` and the spoken menu |
| `voicemail_text` | text | The voicemail greeting's words: what callers hear before the beep on a missed call, while away and on a `voicemail` choice. Empty = the standard words (`lines.Line.DefaultVoicemail`, which follow the business's name) |
| `voicemail_audio` | text | The owner's recording of the voicemail greeting (file under `prompts/`), or empty. When set, callers hear it instead of the computer voice |
| `away_until` | text (UTC) | The app's Available switch: '' = available; a time = paused until then ("Pause for 1 hour"); `9999-12-31 00:00:00` = Off until turned back on. While away, calls go straight to voicemail and nothing rings |
| `logo` | text | The business's logo (file under `logos/`, PNG, JPG or WebP, 2 MB at most; the browser frames and shrinks it to a 512 px WebP first), or empty. Callers see it instead of the initials: call window, meeting room, the owner's face on the stage. Asked when the line is made, changed on the dashboard |
| `created_at` | text (UTC) | |

## menu_option
One choice in a line's voice menu.

| Column | Type | Meaning |
|---|---|---|
| `id` | integer, key | |
| `line_id` | integer → `line.id` (delete cascades) | |
| `position` | integer | Order in the menu, from 0 |
| `key` | text | The key the caller presses: `0`–`9`, `*` or `#`; unique per line |
| `label` | text | Choice name, e.g. "Bookings"; spoken as "press 1 for bookings" |
| `action` | text: `ring`, `play`, `link`, `voicemail` | What the choice does |
| `say` | text | Spoken when the caller picks it (required for `play`; empty for `voicemail`, which plays the line's voicemail greeting) |
| `url` | text | Web page for `link` (http or https) |
| `say_audio` | text | The owner's recording of `say` (file under `prompts/`), or empty |

## promo_code
Codes that make a line free forever, on a plan. Admins manage them at `/admin/promo` (create, see who used them, revoke or restore). Revoking also ends "free forever" for the accounts that used the code; it doesn't change their plan.

| Column | Type | Meaning |
|---|---|---|
| `code` | text, key, case-insensitive | e.g. `FAMILY` |
| `valid_from` | text (UTC), nullable | Not usable before this time |
| `valid_to` | text (UTC), nullable | Not usable after this time |
| `max_uses` | integer | 0 = unlimited |
| `times_used` | integer | Raised by one at each sign up that uses it |
| `revoked` | integer 0/1 | 1 = no longer accepted |
| `plan` | text: `basic` or `premium` | The plan accounts signing up with it get |
| `created_at` | text (UTC) | |

## voicemail
A message a caller left. The audio file is `<data dir>/voicemail/<file>` (deleted with the row). Uploads need a one-time token from the call socket (5 minutes), given when nobody answers or when the caller picks a voicemail choice.

| Column | Type | Meaning |
|---|---|---|
| `id` | integer, key | |
| `line_id` | integer → `line.id` (delete cascades; the file stays, see note) | |
| `option` | text | Menu choice the caller picked ("Talk to Joe", "Leave a message") |
| `call_id` | text | The unanswered call (`call_log.call_id`) the message follows; '' when the caller picked a voicemail choice |
| `duration_ms` | integer | Length measured by the caller's browser |
| `file` | text | File name under `voicemail/` (`msg-*.webm`, `.m4a` or `.ogg`) |
| `content_type` | text | e.g. `audio/webm` |
| `size` | integer | The file's size in bytes, for the mailbox limits (`Store.MailboxFull`) |
| `listened` | integer 0/1 | 1 once the owner pressed play |
| `created_at` | text (UTC) | |

Note: deleting an account removes its voicemail rows but not the files; the e2e test deletes its own files.

## call_log
The owner's call history: one row per call that reached the line, written by the call hub when the call ends (`calls.Hub.OnEnd`). The app's Calls list shows these rows plus voicemails left from a voicemail choice; a call followed by a voicemail shows as that voicemail.

| Column | Type | Meaning |
|---|---|---|
| `id` | integer, key | |
| `line_id` | integer → `line.id` (delete cascades) | |
| `call_id` | text, unique | The hub's id for the call (also in `push_log.call_id` and `voicemail.call_id`) |
| `option` | text | Menu choice the caller picked to ring the owner |
| `outcome` | text: `answered`, `missed`, `declined`, `away` | `missed` = rang out or the caller hung up first; `declined` = the owner pressed Decline; `away` = the Available switch was off or paused, nothing rang |
| `started_at` | text (UTC) | When it started ringing |
| `answered_at` | text (UTC), null | When the owner answered; null if nobody did |
| `ended_at` | text (UTC) | When it ended |

## device
A browser that gets push notifications (usually the owner's phone app). Saved when the owner turns notifications on; deleted when they turn them off or the push service answers 404/410.

| Column | Type | Meaning |
|---|---|---|
| `id` | integer, key | |
| `account_id` | integer → `account.id` (delete cascades) | |
| `endpoint` | text, unique | The push service URL from the browser's `PushSubscription`, or `fcm:<token>` for the Android app (its Firebase token; `p256dh` and `auth` are then empty) |
| `p256dh` | text | Browser's public key (base64url) |
| `auth` | text | Browser's auth secret (base64url) |
| `user_agent` | text | Which browser/phone, for display |
| `created_at` | text (UTC) | |

## push_log
Delivery receipts for push notifications, to find out whether a phone really rang and how late. The server writes a row for every notification it sends to a device; the phone's service worker (`assets/js/sw.js`) reports back through `POST /app/push/receipt`. Rows older than 14 days are deleted when new ones are written. No foreign keys: a row outlives its device.

| Column | Type | Meaning |
|---|---|---|
| `id` | integer, key | Sent inside the notification so the phone can report it |
| `device_id` | integer | `device.id` it went to |
| `line` | text | `line.name` |
| `call_id` | text | The call it was about; '' for tests and voicemail |
| `kind` | text | `ring`, `answered`, `missed`, `voicemail` or `test` |
| `sent_at` | text (UTC, milliseconds) | When the server sent it |
| `result` | text | What the push service answered: `ok`, or the error |
| `shown_at` | text (UTC, milliseconds), null | When the phone's receipt reached the server (server clock); null if it never did |
| `shown_as` | text | What the phone did: `ring`, `late` (arrived after the ringing stopped, shown as "Missed call"), `answered`, `cleared` (app on screen, ring removed), `missed`, `voicemail`, `test` |
| `phone_note` | text | `app on screen` or `app not on screen` |

## setting
Server settings. Keys today: `vapid_public`, `vapid_private` (web push keys, made on first start; never change them or every phone must turn notifications on again) and `turn_secret` (signs the relay logins; made on first start) and `known_key` (signs the mark of a browser that has logged in; made on first need).

| Column | Type | Meaning |
|---|---|---|
| `key` | text, key | |
| `value` | text | |

## premium_request
Emails left on the landing page's "Get Premium" form (`POST /premium`, at most 3 an hour from one visitor). There are no payments yet: Ken sees the list on `/admin/promo` ("Asked for Premium"), writes back and sets them up.

| Column | Type | Meaning |
|---|---|---|
| `id` | integer, key | |
| `email` | text, unique, case-insensitive | Asking twice keeps the first row |
| `created_at` | text (UTC) | |

## prompt
Who owns each recorded message (files in `<data dir>/prompts/`): only that account can use it in a menu or delete it. At most 40 per account; recordings no menu uses go away after a day.

| Column | Type | Meaning |
|---|---|---|
| `name` | text, key | The file name, also the `line.greeting_audio`, `line.voicemail_audio` and `menu_option.say_audio` value |
| `account_id` | integer → `account.id` (delete cascades) | |
| `created_at` | text (UTC) | |

## note
Premium's written record of a call, a meeting room or a voicemail (`store/notes.go`; written by the `notes` package, one at a time). Kept a year, or until the owner deletes it. Only the account's owner reads it. `transcript` holds what strangers said, so it's always shown as plain text and never given to the summary as instructions.

| Column | Type | Meaning |
|---|---|---|
| `id` | integer, key | |
| `account_id` | integer → `account.id` (delete cascades) | |
| `line_id` | integer → `line.id` (delete cascades) | |
| `kind` | text: `voicemail`, `call` or `room` | What was recorded |
| `status` | text: `waiting`, `working`, `done`, `failed` or `skipped` | `waiting` includes a note between two tries. A server that stops leaves `working`, put back at the next start |
| `reason` | text | Why it `failed` or was `skipped`: `not_set_up` (a key is missing), `not_premium`, `month_limit`, `day_limit` (everyone's day, or a line's 30 voicemail notes in a day), `silent` (nothing said), `no_audio`, `transcribe`, `summarize`. A `summarize` failure still has its transcript |
| `language` | text: `fr` or `en` | What the summary and next actions are written in: the account's at the time. The transcript keeps the words as spoken |
| `audio` | text | The recording, a path under the data folder; empty once transcribed or given up on. A call's or a room's file is deleted then; a voicemail's stays, it's the owner's message |
| `audio_type` | text | Its content type |
| `seconds` | integer | The recording's length: first as told, then as the service measured it |
| `transcript` | text (JSON list of `{speaker, start, text}`) | Who said what, word for word |
| `summary` | text | One page at most (cut at 20,000 characters) |
| `next_actions` | text (JSON list of text) | What the owner has to do (at most 30) |
| `tries` | integer | Tries so far, 4 at most; a stop in the middle counts, so it can't spend money without end |
| `next_try` | text (UTC) | When its turn comes |
| `created_at` | text (UTC `YYYY-MM-DD HH:MM:SS`) | |
| `done_at` | text (UTC), nullable | When it was finished or given up on |
| `voicemail_id` | integer, nullable → `voicemail.id` (delete sets null) | A voicemail's message. Deleting the message deletes the note (`DeleteVoicemail`); a message that expires after 90 days leaves its note |

## note_usage
How much audio each account's notes covered each day (UTC). Apart from `note`, so deleting a note doesn't give its hours back. It sets the month's limit (`store.NoteMonthSeconds`, 10 hours per account) and the day's ceiling across all accounts (`MINIVOIP_NOTES_DAY_HOURS`, default 20). Counted only once a transcript comes back. Kept 62 days.

| Column | Type | Meaning |
|---|---|---|
| `account_id` | integer → `account.id` (delete cascades) | |
| `day` | text (UTC `YYYY-MM-DD`) | Key, with `account_id` |
| `seconds` | integer | |

## Version history
| Version (`PRAGMA user_version`) | Date | Change |
|---|---|---|
| 1 | 2026-10-21 | First schema: `account`, `user`, `session`, `line`, `menu_option`, `promo_code` |
| 2 | 2026-10-21 | `voicemail` table |
| 5 | 2026-10-21 | `user.is_admin` |
| 4 | 2026-10-21 | `device`, `setting` (push notifications) |
| 3 | 2026-10-21 | `line.greeting_audio`, `menu_option.say_audio` (recorded prompts in `<data dir>/prompts/`, public at `/prompt/<name>`, random names; saving the menu deletes recordings it no longer uses) |
| 6 | 2026-10-22 | `push_log` (delivery receipts) |
| 7 | 2026-10-24 | `call_log` (call history), `voicemail.call_id`, `line.away_until` (Available switch) |
| 8 | 2026-10-25 | `account.plan`, `promo_code.plan` (Basic or Premium); admins' accounts start on Premium |
| 9 | 2026-10-27 | `line.logo` (in `<data dir>/logos/`, public at `/logo/<name>`, random names; a new logo or Remove deletes the old file) |
| 10 | 2026-10-31 | `premium_request` (the landing page's "Get Premium" emails) |
| 11 | 2026-11-02 | `line.voicemail_text`, `line.voicemail_audio` (the voicemail greeting; a `voicemail` choice's own message becomes it) |
| 12 | 2026-11-02 | `prompt` (who owns each recording), `session.hashed` (tokens stored as a hash, old ones converted at start) |
| 13 | 2026-11-03 | `voicemail.size` (old messages filled in at start) |
| 14 | 2026-11-04 | `account.language` (`fr` or `en`; existing accounts get French) |
| 15 | 2026-11-04 | `note`, `note_usage` (Premium notes and the hours they used) |
| 16 | 2026-11-04 | `note.voicemail_id` |
