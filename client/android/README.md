# Mu for Android

A focused native client for your Mu account: text conversations, offline
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

Tap **Speak**, dictate, review the transcription and tap **Send**. For direct
voice requests, enable **Hands-free “Hey Micro” while open** and include your
request after the wake phrase. This submits recognised requests automatically,
except during playback as described below. Turning the screen off or leaving
the app stops listening and disables the switch. Nothing starts at boot.

Vosk transcribes locally using the bundled English model. No microphone audio
is uploaded. Only submitted text goes to your Mu server and its configured AI
provider. Spoken replies use an installed offline English Android TTS voice;
if none is installed, text still works. Long replies are spoken in bounded
chunks and remain fully readable on screen. Resuming a paused readout restarts
the current chunk.
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
Debug APK: `app/build/outputs/apk/debug/mu-android-preview.apk`.

PRs and main builds run Android unit tests, lint and APK compilation separately
from Go builds. Download **mu-android-preview** from the Android Actions run
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
With signing configured, releases verify the APK signature before attaching
`mu-android.apk`. Without signing secrets, no Android release APK is published;
CI still builds an installable debug preview.

This does not publish to Google Play or create a new release tag automatically.

## Dependencies and licences

Vosk Android 0.3.75 and the small US English 0.15 model: Apache 2.0,
[Alpha Cephei](https://alphacephei.com/vosk/models).
JNA 5.18.1: LGPL 2.1 or Apache 2.0.
Markwon 4.6.2: Apache 2.0.
Model attribution/README is packaged with the model.

## Hands-free service use

With **Hands-free “Hey Micro” while open** enabled, say the wake phrase and
your request together, for example “Hey Micro read the headlines” or “Hey Micro
play K-pop demon hunters”. The text is submitted to your existing agent, using
its normal tools and permissions. Turning the switch off restores review-before-
send voice input. Leaving the app still disables the microphone.

The agent API now returns its existing typed result objects as well as text.
Android displays those results and opens selected YouTube videos in a visible
player. Explicit play/watch requests may start the selected video automatically;
provider restrictions can still require tapping Play. The player provides
Play, Pause, Speak, Close and Open in YouTube controls.

Say “Hey Micro pause”, “resume”, “next”, “previous”, “read first”, “read that”,
or “back”. Include the wake phrase for each command when not already dictating.
Next/previous moves through returned results or through a long spoken response.
Long briefs are spoken in chunks instead of being cut off.

While audio is playing, only local playback controls are executed automatically.
Other recognised requests are shown for confirmation, so a video or spoken
article cannot issue account actions through the microphone. Close playback to
make a new hands-free service request. General service requests still go through
the agent: this is not a second tool runner or a native screen for every service.

YouTube requires a visible player: “audio only” explains that restriction rather
than hiding its video. Headlines, briefs and other text answers can be spoken
without video. No YouTube downloads, extracted audio or background playback.


Agent entry: tap the agent name to choose from the account-scoped catalogue. The saved name and description are shown directly; instructions are never downloaded by the client. The last selection and separate thread IDs are encrypted alongside account state. Manual server/agent entry is under Account → Advanced.

Android visitors can use “Open in Android” on an agent chat page. Links use `micro-agent://open?server=<HTTPS origin>&agent=<agent ID>`. Links never contain credentials, names, descriptions, instructions or messages, and do not grant access to private agents. A different server requires confirmation and a new sign-in. Shared private-agent links identify the agent by ID rather than selecting a same-named agent in another account.

Account → Add agent to home screen requests a launcher shortcut; Account → Share agent link shares an explicit app link. These require the app to be installed. Normal HTTPS agent links remain web links; verified Android App Links require a release signing certificate and domain association and are not configured here.

The installed app and APKs are named Mu; Micro remains the general-purpose agent. Choose an agent → Create agent opens the existing web builder for instructions, permissions and tools.

### Phone assistant

After signing in and allowing microphone access, choose **Account → Use Mu as
phone assistant**, then select Mu in Android's assistant settings. Invoke the
phone's supported assistant button/gesture to speak directly to the selected
agent. The exact shortcut and lock-screen availability depend on the device.
This does not enable background wake-word detection.

A locked phone opens a distinct **Public questions** mode. It can answer public
questions and look up weather for a named town/city. It does not use saved
location, personal agent instructions, conversation history, memory or private
tools. Personal requests require unlocking. Public requests still use the
signed-in account's credits, use the configured model provider, and require
connectivity. No screen contents or screenshots are requested by the assistant.

Unlock opens the full app; an unlocked invocation resumes the selected agent's
conversation. Closing or stopping silences playback and stops listening; an
already submitted request may still complete on the server. Agent-to-agent
messaging, shared-agent participation and screen-off wake words are separate
features, not part of this integration.
