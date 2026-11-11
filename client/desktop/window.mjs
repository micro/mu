// Restore onto a currently connected display, including after undocking.
export function restoreWindow(saved, displays) {
  const primary = displays[0];
  const valid =
    saved &&
    ["x", "y", "width", "height"].every(
      (key) => Number.isFinite(saved[key]) && Math.abs(saved[key]) < 100000,
    ) &&
    saved.width >= 360 &&
    saved.height >= 480;
  const area =
    (valid &&
      displays.find(
        (d) =>
          saved.x < d.x + d.width &&
          saved.x + saved.width > d.x &&
          saved.y >= d.y &&
          saved.y + 30 <= d.y + d.height,
      )) ||
    primary;
  const width = Math.min(valid ? saved.width : 1000, area.width);
  const height = Math.min(valid ? saved.height : 800, area.height);
  return {
    x: Math.round(
      valid
        ? Math.max(area.x, Math.min(saved.x, area.x + area.width - width))
        : area.x + (area.width - width) / 2,
    ),
    y: Math.round(
      valid
        ? Math.max(area.y, Math.min(saved.y, area.y + area.height - height))
        : area.y + (area.height - height) / 2,
    ),
    width: Math.round(width),
    height: Math.round(height),
    maximized: !!(valid && saved.maximized === true),
  };
}

export function menuTemplate(platform, navigate, downloads) {
  const item = (id, label, accelerator, view) => ({
    id,
    label,
    accelerator,
    click: () => navigate(view),
  });
  return [
    ...(platform === "darwin" ? [{ role: "appMenu" }] : []),
    {
      label: "File",
      submenu: [
        item("new-chat", "New chat", "CmdOrCtrl+N", ""),
        item("settings", "Settings…", "CmdOrCtrl+,", "settings"),
        { type: "separator" },
        { role: platform === "darwin" ? "close" : "quit" },
      ],
    },
    { role: "editMenu" },
    {
      label: "Go",
      submenu: [
        item("home", "Home", "CmdOrCtrl+1", "home"),
        item("inbox", "Inbox", "CmdOrCtrl+2", "inbox"),
        item("agents", "Agents", "CmdOrCtrl+3", "agents"),
        item("services", "Services", "CmdOrCtrl+4", "services"),
        item("chats", "All chats", "CmdOrCtrl+5", "chats"),
        item("account", "Account", undefined, "account"),
      ],
    },
    {
      label: "View",
      submenu: [
        { role: "reload" },
        { type: "separator" },
        { role: "resetZoom" },
        { role: "zoomIn" },
        { role: "zoomOut" },
        { type: "separator" },
        { role: "togglefullscreen" },
      ],
    },
    { role: "windowMenu" },
    {
      role: "help",
      submenu: [
        { id: "downloads", label: "Download latest Mu…", click: downloads },
      ],
    },
  ];
}
