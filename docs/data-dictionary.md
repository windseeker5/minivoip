# Data dictionary

Last update: 2026-10-21 · One SQLite file: `data/minivoip.db` (or `MINIVOIP_DB`). Schema and migrations: `store/store.go`. Change this file in the same change as the schema.

## account
One customer (a business). Created at sign up.

| Column | Type | Meaning |
|---|---|---|
| `id` | integer, key | |
| `name` | text | For now the sign-up email; a business name later |
| `promo_code` | text, nullable → `promo_code.code` | The code used at sign up. Set = free forever (while the code isn't revoked) |
| `created_at` | text (UTC `YYYY-MM-DD HH:MM:SS`) | |

## user
A person who logs in. One per account for now (staff accounts later).

| Column | Type | Meaning |
|---|---|---|
| `id` | integer, key | |
| `account_id` | integer → `account.id` (delete cascades) | |
| `email` | text, unique, case-insensitive | Login name |
| `password_hash` | text | bcrypt hash; the password is never stored |
| `is_admin` | integer 0/1 | Can manage promo codes (set it with SQL). Emails in `MINIVOIP_ADMINS` are admins too |
| `created_at` | text (UTC) | |

## session
A login. The token is the `mv_session` cookie (HttpOnly, SameSite=Lax). Lasts 30 days.

| Column | Type | Meaning |
|---|---|---|
| `token` | text, key | 64 random hex characters |
| `user_id` | integer → `user.id` (delete cascades) | |
| `expires_at` | text (UTC) | Ignored after this time |

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
| `say` | text | Spoken when the caller picks it (required for `play`) |
| `url` | text | Web page for `link` (http or https) |
| `say_audio` | text | The owner's recording of `say` (file under `prompts/`), or empty |

## promo_code
Codes that make a line free forever. Admins manage them at `/admin/promo` (create, see who used them, revoke or restore). Revoking also ends "free forever" for the accounts that used the code.

| Column | Type | Meaning |
|---|---|---|
| `code` | text, key, case-insensitive | e.g. `FAMILY` |
| `valid_from` | text (UTC), nullable | Not usable before this time |
| `valid_to` | text (UTC), nullable | Not usable after this time |
| `max_uses` | integer | 0 = unlimited |
| `times_used` | integer | Raised by one at each sign up that uses it |
| `revoked` | integer 0/1 | 1 = no longer accepted |
| `created_at` | text (UTC) | |

## voicemail
A message a caller left. The audio file is `<data dir>/voicemail/<file>` (deleted with the row). Uploads need a one-time token from the call socket (5 minutes), given when nobody answers or when the caller picks a voicemail choice.

| Column | Type | Meaning |
|---|---|---|
| `id` | integer, key | |
| `line_id` | integer → `line.id` (delete cascades; the file stays, see note) | |
| `option` | text | Menu choice the caller picked ("Talk to Joe", "Leave a message") |
| `duration_ms` | integer | Length measured by the caller's browser |
| `file` | text | File name under `voicemail/` (`msg-*.webm`, `.m4a` or `.ogg`) |
| `content_type` | text | e.g. `audio/webm` |
| `listened` | integer 0/1 | 1 once the owner pressed play |
| `created_at` | text (UTC) | |

Note: deleting an account removes its voicemail rows but not the files; the e2e test deletes its own files.

## device
A browser that gets push notifications (usually the owner's phone app). Saved when the owner turns notifications on; deleted when they turn them off or the push service answers 404/410.

| Column | Type | Meaning |
|---|---|---|
| `id` | integer, key | |
| `account_id` | integer → `account.id` (delete cascades) | |
| `endpoint` | text, unique | The push service URL from the browser's `PushSubscription` |
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
Server settings. Keys today: `vapid_public`, `vapid_private` (web push keys, made on first start; never change them or every phone must turn notifications on again) and `turn_secret` (signs the relay logins; made on first start).

| Column | Type | Meaning |
|---|---|---|
| `key` | text, key | |
| `value` | text | |

## Version history
| Version (`PRAGMA user_version`) | Date | Change |
|---|---|---|
| 1 | 2026-10-21 | First schema: `account`, `user`, `session`, `line`, `menu_option`, `promo_code` |
| 2 | 2026-10-21 | `voicemail` table |
| 5 | 2026-10-21 | `user.is_admin` |
| 4 | 2026-10-21 | `device`, `setting` (push notifications) |
| 3 | 2026-10-21 | `line.greeting_audio`, `menu_option.say_audio` (recorded prompts in `<data dir>/prompts/`, public at `/prompt/<name>`, random names; saving the menu deletes recordings it no longer uses) |
| 6 | 2026-10-22 | `push_log` (delivery receipts) |
