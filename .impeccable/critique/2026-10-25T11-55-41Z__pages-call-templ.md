---
target: visitor call window (click to call)
total_score: 21
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 4
target_identity: "file:/home/kdresdell/Documents/DEV/minivoip/pages/call.templ"
target_fingerprint: "sha256:d9aa1214d838dfaf613ff4886edf58a37835caac18b84624ec3186ce6835ab96"
target_path: /home/kdresdell/Documents/DEV/minivoip/pages/call.templ
timestamp: 2026-10-25T11-55-41Z
slug: pages-call-templ
---
# Call window (/call/{line}): critique + audit, 2026-10-25

Method: dual-agent (A: design review · B: detector + browser), independent; code audit in the parent.

In one line: it works and it's familiar, but it's a generic phone screen that doesn't show the business, says "Connected" when nobody picked up, and has a few real bugs. Design 21/40, code 10/20.

## Design health (critique)

| # | Heuristic | Score | Key issue |
|---|---|---|---|
| 1 | Visibility of system status | 2 | "Connected" while only the recorded menu plays; no hint voicemail comes after 30 s; end screen looks like the start |
| 2 | Match system / real world | 3 | Real ringback, plain words; but "Tap the green button" (colour, "tap" on a computer) and "no phone needed" to someone on a phone |
| 3 | User control and freedom | 2 | No way back to the menu, no "start over" for a voicemail, no link back to the business's site |
| 4 | Consistency and standards | 2 | Mute and Keypad show when they do nothing; font, colours and dark theme match neither the white site nor the owner's app |
| 5 | Error prevention | 1 | Mute pressed during the menu shows "muted" but the visitor is heard once the call starts (checked headless) |
| 6 | Recognition rather than recall | 3 | Choices are labelled buttons while the voice plays |
| 7 | Flexibility and efficiency | 3 | Number keys work; a choice can be tapped early; no Esc to hang up |
| 8 | Aesthetic and minimalist design | 2 | Big empty gap; red Hang up loudest on the menu; on a computer a 390px phone floats in black |
| 9 | Error recovery | 2 | "Microphone blocked. Allow it" without how; a failed voicemail upload loses the message |
| 10 | Help and documentation | 1 | Nothing says it's free, that the mic will be asked for, or what happens next |
| **Total** | | **21/40** | **Acceptable** |

## Design specificity verdict

About 30% authored. It copies the standard iOS/Android in-call screen (dark gradient, orange initial "O", round Mute/Keypad/Hang up). Any web phone demo could use it unchanged. The one minivoip idea, readable choices while the voice plays, is undersold as a quiet list under the call buttons. No trace of the business (logo, photo, website) and no trace of minivoip's new look (system font instead of Onest, orange everywhere though DESIGN.md reserves it for the chosen option, always dark while the landing is white and the app warm light).

Detector: 11 warnings on a rendered mirror (the CLI is blind on .templ with absolute stylesheet links): 8 x undersized-ui-text (9px keypad letters, callscreen.css:76), 2 x layout-transition (.cs-head padding :35, .cs-avatar width/height/margin :39), 1 x pulsing-dot (.cs-dot, false positive: live recording indicator). Overlay injected headless (no user-visible tab); a later dark-glow finding came from the overlay's own badge style and was discarded.

## Audit (code)

| # | Dimension | Score | Key finding |
|---|---|---|---|
| 1 | Accessibility | 2 | Spoken content never shown as text; focus lost on every state change; 200% zoom clips Hang up |
| 2 | Performance | 2 | ~593 KB loaded, ~570 KB unused (shadcn bundle 417 KB, output.css 133 KB, htmx CDN, Onest CSS) |
| 3 | Responsive | 2 | Fine on phones; px sizes ignore text-size settings; zoom clips buttons and keypad (overflow hidden) |
| 4 | Theming | 2 | Always dark but theme-color #ffffff, color-scheme light dark, white body on overscroll; hard-coded colours |
| 5 | Implementation integrity | 2 | `.cs button { font: inherit }` cancels .cs-btn 13px and .cs-choice 600; mute bug; link popup outside the gesture |
| **Total** | | **10/20** | **Acceptable** |

## What's working
- Choices as big labelled buttons while the menu plays.
- Familiar phone model: green Call, red Hang up, real ringback, timer, 56-76px targets, good contrast (lowest 5.24:1), live status region.
- Mic asked only when needed, from the tap.

## Priority issues
1. [P1] Mute lies and the call buttons show too early. Mute pressed before the call: aria-pressed=true but track.enabled=true once connected. Keypad dead while ringing/talking. Fix: menu shows choices + quiet Hang up; Mute only with a live mic, applied to new tracks. distill, harden.
2. [P1] Microphone moment unguarded: no pre-prompt line, denial is a dead end. Fix: one line before, per-platform steps + Try again on denial. onboard, harden.
3. [P1] Voicemail has no safety net: no "say your name and number", 2-min limit hidden, Hang up silently sends, no start over, failed upload loses it. clarify, harden.
4. [P1] Spoken-only content: greeting and "play a message" answers never on screen (sound off, noisy place, hard of hearing). Fix: captions as they play. clarify.
5. [P2] Generic look, no business face: orange "O", system font, black void on desktop. Redesign. typeset, colorize, adapt.

## Other findings
- [P1] Link choice: window.open after the spoken line, outside the tap; Safari likely blocks it while status says "Page opened" (code reading, not confirmed headless).
- [P1] 200% zoom at 390: Hang up clipped ~21px; keypad outer columns clipped.
- [P2] Focus falls to body on Call and each state change; choices not announced.
- [P2] ~570 KB unused assets from layouts/base.templ.
- [P2] "Connected" during the menu; end screen equals the start screen.
- [P2] Status uses the Owner field ("Ringing OMT") while the choice says "Talk to Ken".
- [P2] `.cs button { font: inherit }` specificity bug.
- [P3] Speech fixed to en-US; empty menu dead end; 7th choice hidden below the fold on desktop without a hint; 9px keypad letters; layout-property transitions.

## Persona red flags
- Denise, 65, B&B guest on Android: orange "O" not the B&B; mic prompt scares her into Block; doesn't know to leave her number; 12px footer.
- Jordan, first-timer: "Connected" makes him talk to a recording; can't tell if the message went.
- Sam, screen reader: focus lost every step; choices appear unannounced; 2-minute limit unknowable.

## Questions to consider
- Before Ken's phone rings, the menu is answers read aloud. What if it looked like a short readable list, with the call look only once it rings?
- What if this screen were the business's front desk (photo, hours, "Ken usually answers in 30 s") with minivoip as a quiet signature?
- When Ken can't answer, should it also offer to type a number or email?
