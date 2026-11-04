# minivoip Android app

The owner's app for Android: the same `/app` pages as the browser, in a web view, plus what a web page can't do:
ring like a phone call (ringtone until answered, call screen over the lock screen). See `docs/vision.md`.

- `CallService`: Firebase messages from the server (`push/fcm.go`), same JSON as the web push.
- `Ring`: the call notification (channel "Incoming calls", the phone's ringtone, repeating) and missed-call notes.
- `RingActivity`: the ring screen. Answer opens `/app?answer=<call>` with a secret pass only this app knows, and
  `phone.js` answers (only inside the app: a browser ignores `answer`); Decline posts
  `/app/calls/{id}/decline`.
- `MainActivity`: the web view, microphone and camera (video calls, the meeting room), and `window.MinivoipAndroid`
  (start, settings, clearRing, callEnded, callStarted, callStopped, takeAnswer). Only our own pages get the microphone and camera. Over the lock
  screen the page shows only the call, and Back leaves.
- `CallAlive`: a service with a quiet "On a call" notification while a call or a meeting is on (`callStarted`,
  `callStopped`): without it, Android cuts off the microphone when the screen locks, in the call and in the notes.
- `Server`: posts with the web view's login cookie; starts Firebase with the settings from `GET /api/android/config`,
  so no `google-services.json` is built in.

## Build and publish

```
cd android
ANDROID_HOME=~/Android/Sdk JAVA_HOME=/opt/android-studio/jbr ./gradlew assembleRelease
cp app/build/outputs/apk/release/app-release.apk ../data/android/minivoip.apk
echo 0.3 > ../data/android/version   # the versionName you just built
```

Phones download it at `/download/minivoip.apk` (the app's account page, "Get the Android app"). Bump `versionCode` and
`versionName` in `app/build.gradle.kts` for each new build, and write the new `versionName` to
`data/android/version`: the app tells the page its version (`MinivoipAndroid.version()`, from 0.3), and the account page
shows it, with "Update to …" when the published one is newer.

**Signing key:** `android/release-keystore.jks`, passwords in `android/local.properties` (both gitignored). Back them
up: without that key, phones can't update the app and must uninstall it first. The server address is `SERVER` in
`local.properties` (default https://voip.dresdell.com).
