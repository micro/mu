const { contextBridge, ipcRenderer } = require("electron");
contextBridge.exposeInMainWorld("muDesktop", {
  redirect: () => ipcRenderer.invoke("auth-redirect"),
  authorize: (url) => ipcRenderer.invoke("auth-authorize", url),
  loadSession: () => ipcRenderer.invoke("session-load"),
  saveSession: (session) => ipcRenderer.invoke("session-save", session),
  onCallback: (callback) => {
    const listener = (_event, result) => callback(result);
    ipcRenderer.on("auth-callback", listener);
    return () => ipcRenderer.removeListener("auth-callback", listener);
  },
});
