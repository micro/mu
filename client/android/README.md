# Micro for Android

A focused native client for your Micro account: text conversations, offline
English speech input, local spoken replies, and an optional “Hey Micro” listener
while the app is visible. No dashboard, service catalogue, or embedded web app.

## Account and conversation

Tap **Connection**, keep `https://micro.mu` or enter your own HTTPS Mu origin,
and sign in. Your browser handles login, signup and consent. After allowing access, tap **Open Micro** to return to the app. Existing browser
sessions can be reused. Choose Assistant API / agent access and allow actions.
The app uses S256 PKCE and validates a single-use, expiring state; it never sees
your password. Credentials are encrypted using Android Keystore and excluded
from backups. Debug and release apps have distinct callback schemes.

Your account's existing tools, plan, credits and conversations are used.
Follow-up messages retain the returned thread. **New chat** starts a fresh one.
Switching accounts/server clears the current conversation and draft.
Earlier messages remain in the web Inbox; this first client does not yet browse
all saved conversations. **Sign out** removes local access; revoke the grant
under Account → Tokens to invalidate it on the server.

Mu currently issues 24-hour OAuth tokens with no refresh flow. When a token
expires or is revoked, sign in again. The client does not silently resend
requests. If a request times out, check the web Inbox before retrying.

The server must include support for the Android OAuth callback allowlist.
Older Mu versions reject this registration rather than accepting an unsafe
fallback.

## Voice

Tap **Speak**, dictate, review the transcription and tap **Send**. Enable the
“Hey Micro” switch to use that phrase instead of tapping Speak, pause for the
Listening prompt, then dictate. Turning the screen off or leaving the app stops
listening and disables the switch. Nothing starts at boot.

Vosk transcribes locally using the bundled English model. No microphone audio
is uploaded. Only submitted text goes to your Mu server and its configured AI
provider. Spoken replies use an installed offline English Android TTS voice;
if none is installed, text still works. Very long replies are spoken up to the
Android engine's per-utterance limit and remain fully readable on screen.
Wake recognition is experimental and uses CPU speech recognition, not a
low-power hardware wake detector. It needs real-device accuracy/battery testing.

This is not yet a lock-screen or background “Hey Micro” assistant.
The app advertises Android's assist action so it can be selected on compatible
devices, but does not claim the privileged always-on hotword APIs.

## Build

Use Java 17, Gradle **8.13**, Android SDK 36, and:

```sh
gradle -p client/android testDebugUnitTest lintDebug assembleDebug
```

Open `client/android` in Android Studio if preferred. The first build downloads
the pinned Vosk model and checks SHA-256 before packaging. Android 8+ is required.
Debug APK: `app/build/outputs/apk/debug/micro-android-preview.apk`.

PRs and main builds run Android unit tests, lint and APK compilation separately
from Go builds. Download **micro-android-preview** from the Android Actions run
for a debug-signed installable preview. Debug keys on hosted runners are not
stable; uninstall a previous preview if Android reports a signature conflict.

## Releases and signing

The existing Release workflow also builds Android. APK version/name follows
`VERSION` / the release tag, using `major * 1000000 + minor * 1000 + patch + 1`
for the Android version code (each component 0–999).

Configure these repository Actions secrets once for installable release APKs:

- `ANDROID_KEYSTORE_BASE64`: base64-encoded release keystore.
- `ANDROID_STORE_PASSWORD`
- `ANDROID_KEY_ALIAS`
- `ANDROID_KEY_PASSWORD`

Keep and back up the signing key outside this repository; subsequent upgrades
must use the same key. No key is generated or committed by this project.
With signing configured, releases attach `micro-android.apk`. Without it,
they attach explicitly named `micro-android-unsigned.apk`, which must be signed
before installation; CI still builds an installable debug preview.

This does not publish to Google Play or create a new release tag automatically.

## Dependencies and licences

Vosk Android 0.3.75 and the small US English 0.15 model: Apache 2.0,
[Alpha Cephei](https://alphacephei.com/vosk/models).
JNA 5.18.1: LGPL 2.1 or Apache 2.0.
Markwon 4.6.2: Apache 2.0.
Model attribution/README is packaged with the model.
