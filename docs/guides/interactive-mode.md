---
title: Interactive Mode (Proxy)
description: How to get a live, interactive view of a remote browser.
---

# Interactive Mode: The DevTools Proxy

One of Volant's most powerful features is the ability to get a live, fully interactive "remote desktop" session for any browser running inside a microVM. This is the ultimate tool for debugging, human-in-the-loop automation, and observability.

## How it Works

The `volar browsers proxy` command creates a secure, multi-layered proxy that tunnels the Chrome DevTools Protocol (CDP) from the `headless-shell` process inside the VM all the way to your local machine. For quick integrations, `volar browsers stream` prints the raw WebSocket endpoint so other tools (including AI agents) can connect directly.

## Usage

1.  **Start the Proxy:**
    In your terminal, run the proxy command, specifying the name of a running VM:
```bash
    volar browsers stream my-first-vm
    volar browsers proxy my-first-vm
```
    The CLI will start a local server and print a message:
    `✅ CDP proxy is live. Open http://localhost:9222 in your browser to inspect.`

2.  **Connect Your Local Browser:**
    Open a standard Chrome or Chromium browser on your laptop and navigate to `http://localhost:9222`.

3.  **Inspect the Remote Target:**
    You will see a list of the available browser tabs running inside the remote microVM. Click "inspect" on the one you want to control.

A new Chrome DevTools window will open. This window is a **live, pixel-perfect, and fully interactive stream** of the remote browser. You can click, type, inspect the DOM, and use the full power of the DevTools as if the browser were running locally.