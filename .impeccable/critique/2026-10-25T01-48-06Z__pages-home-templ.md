---
target: voip.dresdell.com landing page
total_score: 22
max_score: 36
na_heuristics: 7
p0_count: 1
p1_count: 2
target_identity: "file:/home/kdresdell/Documents/DEV/minivoip/pages/home.templ"
target_fingerprint: "sha256:84cabf6f2bff314b5f6dcdf10124a88d14bf38bd411ce9ad8c7dd2ad5dd0535b"
target_path: /home/kdresdell/Documents/DEV/minivoip/pages/home.templ
timestamp: 2026-10-25T01-48-06Z
slug: pages-home-templ
---
# Critique: landing page (pages/home.templ, live https://voip.dresdell.com)

Score 22/36 (Acceptable). n/a: 7.
| # | Heuristic | Score |
|---|---|---|
| 1 | Visibility of status | 3 |
| 2 | Real world match | 2 ("Get my free line" vs "No phone line"; SaaS words) |
| 3 | User control | 3 |
| 4 | Consistency | 2 (three section-head layouts, five radii) |
| 5 | Error prevention | 3 |
| 6 | Recognition | 2 ("How it works" anchors to the wrong section) |
| 7 | Flexibility | n/a |
| 8 | Minimalist design | 2 (beige + white + black blocks, "free" x5) |
| 9 | Error recovery | 2 (Premium mailto dead end) |
| 10 | Help | 3 (honest FAQ) |

Priority issues
- [P0] Hero sells unbuilt Premium (screen share mock, grey skeleton bars, "MA" tile) instead of the real voice menu + owner's phone ringing.
- [P1] Not white: #f6f5f1 ground + white band + beige cards + 7 black shapes. Keep one dark moment (footer).
- [P1] Headline 13 words / 4 lines; copy repeats (free x5, Get my free line x4, one click x3).
- [P2] Demo/SaaS language for B&B and trade owners.
- [P2] Rhythm: 3 head alignments, 5 radii, pills everywhere, nav "How it works" lands on the wrong section.

Detector (URL scan): 7 findings desktop, 6 phone. Real: white on #f46422 badge 3.1:1; FAQ answer ~97ch lines. Debatable: 3 nested-cards (app pictures). False positive: 2 heading-rhythm. 11 of 17 tap targets under 44px on phone. CLI scan of templ files clean.

Options drawn: A Keynote (centered, two real phone screens), B Step by step (website, voice, phone in a row), C One button (the real Call button as hero). Canvas: https://claude.ai/artifact/XWsCHyUp822kvL6EfHgioS
