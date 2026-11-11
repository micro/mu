const { contextBridge, ipcRenderer } = require("electron");
contextBridge.exposeInMainWorld("muDesktop", {
  loadRuntime: () => ipcRenderer.invoke("runtime-load"),
  saveRuntime: (server) => ipcRenderer.invoke("runtime-save", server),
  redirect: () => ipcRenderer.invoke("auth-redirect"),
  authorize: (url) => ipcRenderer.invoke("auth-authorize", url),
  loadSession: () => ipcRenderer.invoke("session-load"),
  sessionPersistent: () => ipcRenderer.invoke("session-persistent"),
  saveSession: (session) => ipcRenderer.invoke("session-save", session),
  onCallback: (callback) => {
    const listener = (_event, result) => callback(result);
    ipcRenderer.on("auth-callback", listener);
    return () => ipcRenderer.removeListener("auth-callback", listener);
  },
});
