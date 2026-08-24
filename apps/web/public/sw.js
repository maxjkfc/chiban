// Service Worker for Chiban PWA & Web Push

self.addEventListener("install", (event) => {
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener("push", (event) => {
  if (!event.data) {
    return;
  }

  let data = {};
  try {
    data = event.data.json();
  } catch (err) {
    data = { title: "吃伴", body: event.data.text() };
  }

  const title = data.title || "吃伴";
  const options = {
    body: data.body || "您有一則新訊息",
    icon: data.icon || "/icons/icon-192.png",
    badge: "/icons/icon-192.png",
    tag: data.tag || data.group_id || "chiban-notification",
    renotify: true,
    data: {
      url: data.url || "/",
      group_id: data.group_id,
    },
  };

  event.waitUntil(self.registration.showNotification(title, options));
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();

  const targetUrl = (event.notification.data && event.notification.data.url) || "/";

  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((clientList) => {
      // If a window is already open, focus it and navigate
      for (const client of clientList) {
        if ("focus" in client) {
          client.focus();
          if ("navigate" in client) {
            return client.navigate(targetUrl);
          }
          return;
        }
      }
      // If no window is open, open a new one
      if (self.clients.openWindow) {
        return self.clients.openWindow(targetUrl);
      }
    })
  );
});
