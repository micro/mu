# Micro Android assistant (prototype)

The app includes an **Assistant** button that runs Mu's existing `agent/local`
loop inside the Android process through a Go mobile binding. It does not call the
hosted Micro agent, require a Micro login, or start an HTTP server on the phone.
Home still opens the existing hosted interface; Apps and Settings work offline.

## Try the local assistant

Open **Assistant → Model**. Enter an OpenAI-compatible HTTPS base URL (with or
without `/v1`), model name and provider key. Messages go directly to that provider;
provider charges are separate from Micro credits. The key is held in memory only
until the assistant screen closes. Endpoint/model preferences are saved, but keys
are never written to preferences, conversation files or Activity saved state.

Ask **“Which apps can I open on this phone?”**. The model can call `device_apps`,
which uses Android's package manager to list launchable apps in the current
profile. The list goes to the chosen provider only when the tool runs. This first
tool is read-only: it cannot launch apps, inspect their contents or change settings.

Close the app, force-stop it, reopen Assistant, and ask a follow-up after entering
the key again. Successful exchanges remain in private app storage. The full
transcript is retained; only the recent context is sent to the model. A failed or
interrupted request is not automatically replayed. Android backup is disabled.
Uninstalling or clearing app data deletes the transcript. A corrupt file is
reported without overwriting it. Viewing history needs no network; generating an
answer still does. This is local **agent execution**, not an on-device language
model, background scheduler, feed sync or a full hosted Mu instance.

## Build and install

Use Go matching the root go.mod, JDK 17, Android SDK 35 and NDK 27.2.12479018.
Build the Go binding first, from `android/runtime/`:

```sh
go test ./...
go install golang.org/x/mobile/cmd/gomobile
go install golang.org/x/mobile/cmd/gobind
export ANDROID_NDK_HOME="$ANDROID_HOME/ndk/27.2.12479018"
mkdir -p ../app/libs
gomobile bind -target=android/arm64,android/arm,android/amd64 -androidapi 26 -o ../app/libs/mu.aar .
cd ..
./gradlew testDebugUnitTest lintDebug assembleDebug
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

Alternatively open this directory in Android Studio and Run. Android 8.0 or later
is required. The Android launcher GitHub Actions workflow publishes a debug APK
artifact on pushes and pull requests touching this project. Debug builds are for
trying the launcher, not Play Store distribution; signing keys may differ between
CI runs, requiring uninstall/reinstall (which clears the launcher's session).

Open Micro normally first. Sign in using your Micro username/password. Google
OAuth and passkeys are not integrated into this prototype: if your account uses
Google, use a normal browser to set a password under Account, then sign in here.
Browser cookies are separate; browser login does not sign the launcher in.
WebView cookies persist across launches until the server session expires or you
log out. This app never stores your password itself.

Then choose **Settings → Set default home app → Micro**. Home returns to Micro;
Back navigates page history or closes the app drawer. Apps and Android Settings
remain available without a network connection. To revert, open Android Settings →
Apps → Default apps → Home app and select your previous launcher. No kiosk lock.

## Scope and checks

The hosted Home view remains online. The local assistant is an explicit prototype; it has
no native push notifications, microphone/camera integration, work-profile drawer,
widgets or Google sign-in session handoff yet. External links open in other apps.
Only the exact HTTPS Micro origin stays in the main WebView. Cleartext traffic,
file/content access, mixed content, third-party cookies and arbitrary intent URLs
are disabled; invalid TLS certificates use WebView's default rejection.

Before using it as your daily launcher, test on a spare device:

- Sign in, close/reopen and reboot; confirm session persistence and Home selection.
- Open Apps, launch Phone/Camera/Settings, return Home, then switch launcher back.
- Open a chat and use the keyboard, Back, rotation and file upload.
- Follow external links and a new-window link; confirm they leave the WebView.
- Disconnect networking, reload, use Apps/Settings, reconnect and Retry.
- Try Google login; confirm the password setup explanation appears.

Unit tests cover URL boundaries and blocked schemes; build and lint run in CI.
