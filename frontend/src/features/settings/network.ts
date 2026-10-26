/** The usual reasons a device can't see the server, in the order worth checking. */
export const networkHints = [
  {
    title: "Same network.",
    text: "The device and this computer must be on the same Wi-Fi. A guest network usually can't see the main one.",
  },
  {
    title: "Wi-Fi client isolation.",
    text: "Some routers and mesh systems stop devices talking to each other (\"AP isolation\", \"guest mode\"). The address and QR code won't work either until it is turned off.",
  },
  {
    title: "Blocked discovery.",
    text: "A firewall or router that drops UDP ports 1900 and 5353 hides the server from TVs and from eincast.local. Typing the address still works.",
  },
  {
    title: "The first-run firewall prompt.",
    text: "macOS and Windows ask whether to allow incoming connections the first time. Choose Allow (on Windows, tick Private networks), or nothing can connect.",
  },
] as const;
