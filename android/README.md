# Micro Android launcher (prototype)

A small Android Home app for the hosted Micro interface at https://micro.mu/home.
Android still runs the phone; Mu runs on the server. No root or bootloader changes.
The native shell provides Home, an alphabetical installed-app drawer, Settings,
a retry screen, file selection and authenticated downloads through Android’s save dialog. The website retains its existing navigation,
Overview and Feed. No new server endpoints, native bridge or copied web UI.

## Build and install

Use JDK 17 and Android SDK 35 (Android Studio can install both). From `android/`:

```sh
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

This is an online launcher, not an offline assistant or a local Mu server. It has
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
