---
title: Your First VM
description: A step-by-step tutorial to launch your first microVM and run a browser task.
---

# Your First VM: A Quick Start Tutorial

This guide will walk you through launching your first microVM, interacting with it, and running a simple browser automation task.

**Prerequisites:** You must have successfully run `volar setup`.

### 1. Launch the Interactive TUI

The easiest way to interact with Volant is through the TUI. Open your terminal and simply run:

```bash
volar
```

You should see the full-screen TUI, with an empty list of VMs.

### 2. Create Your First MicroVM

In the command input at the bottom of the TUI, type the following command and press Enter:

```
vms create my-first-vm
```

You will see the orchestrator spring to life. In the log pane, you'll see events as the VM is created, and within seconds, `my-first-vm` will appear in the VM list with a `(running)` status and an assigned IP address.

### 3. Spawn a Browser Context

The `volary` inside the VM can manage multiple, isolated browser contexts. Let's create one. Type:

```
browsers spawn my-first-vm main-context
```

This sends a command through the control plane to the agent, telling it to start a new `headless-shell` instance.

### 4. Submit an Automation Task

Now for the magic. We'll submit a task to navigate to a website and take a screenshot. First, create a file named `task.json`:

```json
{
  "actions": [
    { "type": "browser.navigateTo", "params": { "url": "https://volant.example" } },
    { "type": "browser.screenshot", "params": { "path": "/tmp/volant.png" } }
  ]
}
```

Now, submit this task from the TUI:

```
tasks submit my-first-vm ./task.json
```

### 5. View the Result

The task will execute inside the microVM. To see the result, you need to retrieve the screenshot. You can see the agent's logs in the TUI to confirm the task was completed.

### 6. Clean Up

Once you're done, you can destroy the VM:

```
vms destroy my-first-vm
```

You will see the VM disappear from the list in real-time.

Congratulations! you've successfully orchestrated your first secure, microVM-based browser automation workload.