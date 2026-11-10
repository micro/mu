# Mu desktop

Mu for Windows, macOS and Linux packages the React app with Electron. No Node.js,
local server setup or Mu installation is required by users. Models and tools run
on the chosen Mu server. The default is micro.mu; Settings selects another server.

The app uses a sandboxed renderer with no Node integration. A per-launch loopback
server serves bundled assets under an unpredictable path. OAuth opens the system
browser and returns through a state-checked, single-use loopback callback. PKCE
is still checked by the runtime. Tokens are retained only with OS-backed encryption;
Linux's plaintext `basic_text` backend is not used. Otherwise sign-in lasts for
the app session. Playback and microphone features depend on Electron/platform support;
there is no desktop background wake-word promise.

## Development

From the repository root with Node.js 24+:

```sh
npm ci --prefix client/web
npm run build --prefix client/web
npm ci --prefix client/desktop
npm run prepare:app --prefix client/desktop
npm start --prefix client/desktop
```

Run `npm test --prefix client/desktop` for the local callback boundary tests.
`node smoke.mjs` from this directory runs the installed Electron app's sign-in and
chat check (Linux needs a graphical display, e.g. Xvfb). It keeps Electron's sandbox.
`npm run package --prefix client/desktop` creates platform installers in `dist/`.

## Releases

The desktop workflow builds Windows x64 NSIS, macOS arm64/x64 DMG, and Linux x64
AppImage/DEB. Release CI attaches supported installers to the same Mu release.
PR builds retain installers and screenshots as Actions artifacts.

macOS consumer downloads require Developer ID signing and notarization. Configure
`DESKTOP_CSC_LINK`, `DESKTOP_CSC_KEY_PASSWORD`, `APPLE_ID`,
`APPLE_APP_SPECIFIC_PASSWORD` and `APPLE_TEAM_ID` as repository secrets. No App Store
submission is involved. Without those credentials, macOS builds remain Actions
artifacts and are not presented as consumer downloads.

Windows signing optionally uses `DESKTOP_WIN_CSC_LINK` and
`DESKTOP_WIN_CSC_KEY_PASSWORD`; unsigned installers may display an unknown-publisher
warning. Linux packages do not require an app store. Updates are currently manual:
download the new installer from Mu. Automatic updates are not implemented.
