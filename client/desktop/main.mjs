import {
  app,
  BrowserWindow,
  ipcMain,
  shell,
  safeStorage,
  dialog,
  Menu,
  screen,
} from "electron";
import { readFile, writeFile, rm, mkdir } from "node:fs/promises";
import { writeFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { serve, secureOrigin } from "./server.mjs";
import { restoreWindow, menuTemplate } from "./window.mjs";
const directory = dirname(fileURLToPath(import.meta.url));
let window, server;
let showWindow = () => {};
app.enableSandbox();
if (!app.requestSingleInstanceLock()) app.quit();
else {
  app.on("second-instance", () => {
    showWindow();
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
      const preference = join(app.getPath("userData"), "runtime.json");
      const geometry = join(app.getPath("userData"), "window.json");
      let savedWindow;
      try {
        savedWindow = JSON.parse(await readFile(geometry, "utf8"));
      } catch {}
      ipcMain.handle("runtime-load", async (event) => {
        trusted(event);
        try {
          return secureOrigin(JSON.parse(await readFile(preference, "utf8")));
        } catch {
          return null;
        }
      });
      ipcMain.handle("runtime-save", async (event, value) => {
        trusted(event);
        const origin = secureOrigin(value);
        await mkdir(dirname(preference), { recursive: true });
        await writeFile(preference, JSON.stringify(origin), { mode: 0o600 });
      });
      const encrypted = () =>
        safeStorage.isEncryptionAvailable() &&
        (process.platform !== "linux" ||
          safeStorage.getSelectedStorageBackend() !== "basic_text");
      let memorySession = null;
      ipcMain.handle("session-persistent", (event) => {
        trusted(event);
        return encrypted();
      });
      const validSession = (value) => {
        if (
          !value ||
          typeof value.token !== "string" ||
          !value.token ||
          value.token.length > 16384 ||
          value.cookie ||
          !Number.isFinite(value.expires) ||
          (value.expires <= Date.now() &&
            !(
              typeof value.refresh === "string" &&
              value.refresh &&
              typeof value.client === "string" &&
              value.client
            )) ||
          value.expires > Date.now() + 86400000
        )
          throw Error("Invalid session");
        return {
          server: secureOrigin(value.server),
          token: value.token,
          expires: value.expires,
          client:
            typeof value.client === "string" && value.client.length <= 256
              ? value.client
              : undefined,
          refresh:
            typeof value.refresh === "string" && value.refresh.length <= 16384
              ? value.refresh
              : undefined,
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
          memorySession = null;
          await rm(vault, { force: true });
          return;
        }
        const session = validSession(value);
        memorySession = session;
        if (!encrypted()) {
          // Do not restore an older account if encryption becomes available
          // again after this temporary sign-in.
          await rm(vault, { force: true });
          return;
        }
        await mkdir(dirname(vault), { recursive: true });
        await writeFile(
          vault,
          safeStorage.encryptString(JSON.stringify(session)),
          { mode: 0o600 },
        );
      });
      ipcMain.handle("session-load", async (event) => {
        trusted(event);
        if (!encrypted()) {
          try {
            return memorySession ? validSession(memorySession) : null;
          } catch {
            return null;
          }
        }
        try {
          return validSession(
            JSON.parse(safeStorage.decryptString(await readFile(vault))),
          );
        } catch {
          return null;
        }
      });
      function open(view = "") {
        const primary = screen.getPrimaryDisplay();
        const displays = [
          primary,
          ...screen.getAllDisplays().filter((d) => d.id !== primary.id),
        ];
        const { maximized, ...bounds } = restoreWindow(
          savedWindow,
          displays.map((d) => d.workArea),
        );
        window = new BrowserWindow({
          ...bounds,
          minWidth: 360,
          minHeight: 480,
          title: "Mu",
          autoHideMenuBar: false,
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
        window.on("close", () => {
          savedWindow = {
            ...window.getNormalBounds(),
            maximized: window.isMaximized(),
          };
          // Synchronous because the process may quit immediately after close.
          try {
            writeFileSync(geometry, JSON.stringify(savedWindow), {
              mode: 0o600,
            });
          } catch {}
        });
        window.on("closed", () => {
          window = null;
        });
        window.loadURL(server.url + (view ? "?view=" + view : ""));
        if (maximized) window.maximize();
      }
      showWindow = () => {
        if (!window) open();
        if (window.isMinimized()) window.restore();
        window.show();
        window.focus();
      };
      Menu.setApplicationMenu(
        Menu.buildFromTemplate(
          menuTemplate(
            process.platform,
            (view) => {
              if (!window) {
                open(view);
                return;
              }
              showWindow();
              window.loadURL(server.url + (view ? "?view=" + view : ""));
            },
            () => {
              shell.openExternal("https://micro.mu/download").catch(() => {});
            },
          ),
        ),
      );
      open();
      app.on("activate", () => {
        showWindow();
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
