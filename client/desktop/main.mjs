import {
  app,
  BrowserWindow,
  ipcMain,
  shell,
  safeStorage,
  dialog,
} from "electron";
import { readFile, writeFile, rm, mkdir } from "node:fs/promises";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { serve, secureOrigin } from "./server.mjs";
const directory = dirname(fileURLToPath(import.meta.url));
let window, server;
app.enableSandbox();
if (!app.requestSingleInstanceLock()) app.quit();
else {
  app.on("second-instance", () => {
    window?.show();
    window?.focus();
  });
  app
    .whenReady()
    .then(async () => {
      server = await serve(join(directory, "web"), (result) => {
        window?.webContents.send("auth-callback", result);
        window?.show();
        window?.focus();
      });
      const trusted = (event) => {
        if (
          !window ||
          event.sender !== window.webContents ||
          event.senderFrame !== window.webContents.mainFrame ||
          !event.senderFrame.url.startsWith(server.url)
        )
          throw Error("Untrusted caller");
      };
      const vault = join(app.getPath("userData"), "session.enc");
      const encrypted = () =>
        safeStorage.isEncryptionAvailable() &&
        (process.platform !== "linux" ||
          safeStorage.getSelectedStorageBackend() !== "basic_text");
      const validSession = (value) => {
        if (
          !value ||
          typeof value.token !== "string" ||
          !value.token ||
          value.token.length > 16384 ||
          value.cookie ||
          !Number.isFinite(value.expires) ||
          value.expires <= Date.now() ||
          value.expires > Date.now() + 86400000
        )
          throw Error("Invalid session");
        return {
          server: secureOrigin(value.server),
          token: value.token,
          expires: value.expires,
        };
      };
      ipcMain.handle("auth-redirect", (event) => {
        trusted(event);
        return server.callback;
      });
      ipcMain.handle("auth-authorize", async (event, url) => {
        trusted(event);
        await shell.openExternal(server.authorize(url));
      });
      ipcMain.handle("session-save", async (event, value) => {
        trusted(event);
        if (value === null) {
          await rm(vault, { force: true });
          return;
        }
        const session = validSession(value);
        if (!encrypted()) return;
        await mkdir(dirname(vault), { recursive: true });
        await writeFile(
          vault,
          safeStorage.encryptString(JSON.stringify(session)),
          { mode: 0o600 },
        );
      });
      ipcMain.handle("session-load", async (event) => {
        trusted(event);
        if (!encrypted()) return null;
        try {
          return validSession(
            JSON.parse(safeStorage.decryptString(await readFile(vault))),
          );
        } catch {
          return null;
        }
      });
      function open() {
        window = new BrowserWindow({
          width: 1000,
          height: 800,
          minWidth: 360,
          minHeight: 480,
          title: "Mu",
          autoHideMenuBar: true,
          webPreferences: {
            preload: join(directory, "preload.cjs"),
            nodeIntegration: false,
            contextIsolation: true,
            sandbox: true,
            webSecurity: true,
          },
        });
        const external = (url) => {
          try {
            const u = new URL(url);
            if (u.protocol === "https:" && !u.username && !u.password)
              shell.openExternal(u.href).catch(() => {});
          } catch {}
        };
        window.webContents.setWindowOpenHandler(({ url }) => {
          external(url);
          return { action: "deny" };
        });
        window.webContents.on("will-navigate", (event, url) => {
          if (!url.startsWith(server.url)) {
            event.preventDefault();
            external(url);
          }
        });
        window.webContents.session.setPermissionCheckHandler(
          (contents, permission, origin) =>
            contents === window.webContents &&
            origin === new URL(server.url).origin &&
            permission === "media",
        );
        window.webContents.session.setPermissionRequestHandler(
          async (contents, permission, callback, details) => {
            if (
              contents !== window.webContents ||
              permission !== "media" ||
              !details.requestingUrl?.startsWith(server.url) ||
              details.mediaTypes?.some((t) => t !== "audio")
            ) {
              callback(false);
              return;
            }
            const answer = await dialog.showMessageBox(window, {
              type: "question",
              message: "Allow Mu to use your microphone?",
              buttons: ["Cancel", "Allow"],
              defaultId: 0,
              cancelId: 0,
            });
            callback(answer.response === 1);
          },
        );
        window.on("closed", () => {
          window = null;
        });
        window.loadURL(server.url);
      }
      open();
      app.on("activate", () => {
        if (!window) open();
      });
    })
    .catch((error) => {
      dialog.showErrorBox("Mu could not start", error.message);
      app.quit();
    });
  app.on("window-all-closed", () => {
    if (process.platform !== "darwin") app.quit();
  });
  app.on("before-quit", () => server?.close());
}
