# Research: calls without a phone line

Last update: 2026-10-21

## In one sentence
Nobody packages "website Call button → voice menu → owner's phone app rings → voicemail" as a small self-hosted service (paid hosted services do exist, see "Competitors" below). The building blocks exist, so we can build it with one small program.

## The idea
```
Visitor on website ──► "Call us" button ──► voice menu (1 = booking, 2 = other)
                                              │
                                              ▼
                          owner's phone app (installed web app) rings
                                              │
                              no answer after 30 s ──► voicemail
```
- No real phone line and no phone number. The "number" is a link, for example `minivoip.example/joe`.
- Real phone lines are **not in the first version**, but the design keeps the door open: later a desk phone or SIP app, then a real number from a cheap SIP provider (Zadarma: Canadian number about US$2–4/month plus per-minute charges; VoIP.ms and Telnyx are similar). See "Growing later" in [architecture.md](architecture.md).

## How WhatsApp does it
| Part | WhatsApp | Can a web app do it? |
|---|---|---|
| Voice | WebRTC-style audio, Opus codec, relay first, then direct | Yes. WebRTC is built into every browser. |
| Knowing who is online | One permanent connection while the app is open | Yes (WebSocket) |
| **Ringing a sleeping phone** | Special iPhone/Android powers (VoIP push + full-screen call screen) | **No.** Only a normal notification. |

That last row is why the PoC felt clunky. It's a platform limit, not a bug.

## What a web app (PWA) can do when the phone is asleep
| Phone | What happens |
|---|---|
| **Android (Chrome)** | Works well. Notification arrives even when the app is closed. We can repeat it every few seconds to act like a ring. Tap → app opens → answer. |
| **iPhone (iOS 16.4+)** | Works **only if the app was added to the home screen**. One "ding", no ringtone, no full-screen call screen. Tap → app opens in a few seconds → answer. |
| **Computer (Chrome/Edge/Firefox)** | Works. Edge is testing real "incoming call" notifications (not usable yet). |

**What this means for the design:**
- The visitor sees "Waking up their phone…" and gets voicemail after about 30 s.
- The owner's app must reconnect fast after the tap.
- **Later fix (optional):** wrap the same app with Capacitor (a tool that packages a web app as a real app) to get WhatsApp-style ringing.
- Push notifications are free with the web standard (VAPID). No Firebase account and no Apple developer account needed.

## What already exists
| Project | What it is | Why it doesn't fit as-is |
|---|---|---|
| [Browser-Phone](https://www.browser-phone.org/) | Web softphone, works as a PWA | Needs a phone system (Asterisk/FreeSWITCH) behind it |
| [MiroTalk](https://github.com/miroslavpejic85) (P2P/SFU/C2C) | Self-hosted web calls, has a click-to-call version | Meeting-style. No push wake-up, no voice menu, no voicemail. Good to borrow ideas from. |
| [pwa-home-call](https://github.com/flancer64/pwa-home-call) | Tiny family-call PWA | Closest in spirit. No voice menu or voicemail. |
| [LiveKit](https://github.com/livekit/livekit) | Media server (Go, Apache-2.0) | Not needed now. Good later for call recording or an AI receptionist. |
| [Fonoster](https://github.com/fonoster/fonoster), [jambonz](https://jambonz.org/) | Open-source alternatives to Twilio | Built for **real phone lines**. Several services. |
| Asterisk, FreeSWITCH, Kamailio | Classic phone systems | Phone-line world. Too heavy for this. |
| Daily, Whereby, Twilio Client, Aircall | Paid services | Either meeting rooms or real phone numbers |

## How it stays small
- **One Go program.** It serves the website, passes call-setup messages, sends pushes and stores files. Database is one SQLite file.
- The voice menu and voicemail run **in the visitor's browser**, and the call itself goes **directly** between visitor and phone. The server never handles audio, and a connected call survives the server going down.
- A relay (TURN, `pion/turn`) is still needed for networks that block direct connections (often 4G).
- Later, for desk phones and real phone lines, the server will have to play the menu and carry the voice for those calls only.
- See [architecture.md](architecture.md).

## Competitors (if this becomes a SaaS)
Quick search on 2026-10-21. Features and prices come from the vendors' own pages, not tested.

| Service | What it does | Price | How close |
|---|---|---|---|
| [Call2](https://natlawreview.com/press-releases/call2-launches-browser-based-business-calling-platform-replacing-traditional) (launched March 2026) | Browser call widget, rings iOS/Android apps, office SIP phones, AI queue, voicemail-to-email | US$9 per licence (3 calls at once, 5 widgets), no per-minute charges | **Closest: almost the same vision** |
| [WebCallHub](https://getwebcallhub.com/) | Browser call button, answer in your browser, AI receptionist when missed, forward to a phone number (paid plans). Voice goes through their server. | Free: 30 min/month, 1 agent | Very close |
| [Zadarma click-to-call](https://zadarma.com/en/services/click-to-call/) | Free WebRTC "Call us" button, answered with Zadarma's softphone or forwarded | Free (needs a Zadarma account) | Close, but tied to their phone system |
| [KrispCall](https://krispcall.com/compare/callpage-alternative/) | Cloud phone system with a website widget and web/mobile apps | From US$12 per user per month | Phone-system first |
| [CallPage](https://www.getapp.com/sales-software/a/callpage/), Callback24 | Callback widgets: the visitor types a number and gets called back | ~US$59–79/month | Different: real phone calls |
| [LiveAgent](https://www.liveagent.com/callpage-alternative/), tawk.to, Aircall, RingCentral | Help desks or phone systems with calling | Per agent per month | Bigger tools for call centres |
| [innovaphone Call Me Button](https://www.innovaphone.com/en/products/software/call-me-button.html), Deltapath, Voiceflex | WebRTC buttons for their own business phone systems | Enterprise | Not for small businesses |

**What this means:** the idea is proven and paid for, so there's a market, but it's no longer empty. Where minivoip could still stand out:
- direct calls (more private, nothing to pay per minute),
- the voice menu with the owner's own recorded prompts plus "open a link" options,
- can be self-hosted,
- bilingual (French/English) for small Quebec/Canadian businesses,
- a very low flat price.

## Web components for Go
Searched GitHub on 2026-10-21 for shadcn-style component libraries for Go:

| Project | Stars | What it is | Verdict |
|---|---|---|---|
| [templ](https://github.com/a-h/templ) | 10.6k | Go's component templates (like Jinja macros, checked by the compiler) | Foundation |
| [shadcn-templ](https://github.com/axadrn/shadcn-templ) (formerly templUI) | 1.8k | shadcn/ui port for templ, made with shadcn's blessing; ~60 components, plain JavaScript, MIT, the CLI copies code into your project | **Chosen.** Only real component library in the top 15 of GitHub's templ topic. 2.0 is beta. |
| [Basecoat](https://github.com/hunvreus/basecoat) | 4.4k | shadcn look as plain CSS classes, any backend (what KDUI uses) | No Go components; you'd write the wrappers yourself |
| [shadcn-htmx](https://github.com/productdevbook/shadcn-htmx) | 70 | 82 components, Go flavour uses `html/template` | Too young |
| [forgeui](https://github.com/xraph/forgeui) | 9 | Built on shadcn-templ | Too young |
| [gowebly](https://github.com/gowebly/gowebly) | 1.2k | Starter generator for Go + htmx | Not a component library |

Porting KDUI (39 Jinja macros on Basecoat) to Go is possible later, but it's not a priority.

## Simpler than Asterisk (for desk phones and real lines later)
| Option | What it is | Fit |
|---|---|---|
| [emiago/sipgo](https://github.com/emiago/sipgo) (BSD-2) + [emiago/diago](https://github.com/emiago/diago) (MPL-2.0) | Go libraries for SIP calls: play audio, touch tones, bridge calls | Smallest: lives inside our program. Young, API still changing; diago's WebRTC support is marked experimental, so browser calls stay plain WebRTC. |
| [livekit/sip](https://github.com/livekit/sip) (Apache-2.0) | SIP bridge for LiveKit, in Go | Good with SIP providers, but needs LiveKit server + Redis. Desk-phone registration not documented. |
| Kamailio + rtpengine | SIP proxy + voice relay | Two C programs, complex config. Doesn't play menus or record. |
| [Janus](https://janus.conf.meetecho.com/) | WebRTC gateway with a SIP plugin | Lighter than Asterisk, but a separate C server. |
| Asterisk, FreeSWITCH | Complete phone exchanges | Do everything, but big. Fallback only. |

## Not confirmed yet
- MiroTalk C2C details (repo `miroslavpejic85/mirotalkc2c`).
- Whether diago can act as a registrar for desk phones (only client-side registration is documented). Decide at step 2.
- Confirmed 2026-10-21: `livekit/sip` is Apache-2.0 and needs LiveKit server + Redis. LiveKit's TURN question is moot (LiveKit not used now).

## Sources
- [webrtcHacks: What's up with WhatsApp and WebRTC](https://webrtchacks.com/whats-up-with-whatsapp-and-webrtc/)
- [Apple Developer Forums: WhatsApp and VoIP push](https://developer.apple.com/forums/thread/130650)
- [WebRTC.ventures: WhatsApp Business Calling API](https://webrtc.ventures/2025/11/how-to-integrate-the-whatsapp-business-calling-api-with-webrtc-to-enable-customer-voice-calls/)
- [What PWA Can Do Today: incoming call notifications](https://whatpwacando.today/incoming-call-notifications/)
- [MagicBell: PWA iOS limitations 2026](https://www.magicbell.com/blog/pwa-ios-limitations-safari-support-complete-guide)
- [LiveKit self-hosting docs](https://docs.livekit.io/transport/self-hosting/)
- [livekit/sip](https://github.com/livekit/sip)
- [shadcn-templ](https://github.com/axadrn/shadcn-templ), [shadcn-templ docs](https://shadcn-templ.com/docs), [templ](https://github.com/a-h/templ), [templ live reload](https://templ.guide/developer-tools/live-reload), [GitHub topic: templ](https://github.com/topics/templ?o=desc&s=stars)
- [Fonoster](https://github.com/fonoster/fonoster)
- [emiago/sipgo](https://github.com/emiago/sipgo), [emiago/diago](https://github.com/emiago/diago), [diago docs](https://emiago.github.io/diago/docs)
- [webrtcHacks: What is a WebRTC gateway](https://webrtchacks.com/webrtc-gw/)
- [jambonz](https://jambonz.org/)
- [Browser Phone](https://www.browser-phone.org/)
- [flancer64/pwa-home-call](https://github.com/flancer64/pwa-home-call)
