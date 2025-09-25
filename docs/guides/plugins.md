---
title: "Using Plugins"
description: "Extending Volant with specialized, community-built automation workflows."
---

# Using Plugins

While Volant provides a powerful, general-purpose browser automation framework, its true power is unlocked through its extensible plugin system. Plugins are self-contained modules that provide specialized, high-level workflows for specific domains (e.g., e-commerce, social media, security testing).

This architecture allows the core of Volant to remain lean and stable, while enabling a vibrant ecosystem of community-driven innovation.

## Managing Plugins

The `volar` CLI provides a simple set of commands for managing your plugin library.

```bash
# List all currently installed and available plugins
volar plugins list

# Install a new plugin from a git repository
volar plugins install github.com/volant-plugins/casino-stake

# Uninstall a plugin
volar plugins uninstall casino-stake
```

## Using Plugins with Workloads

Plugins are activated through **Workloads**. A workload is a long-lived, managed fleet of one or more microVMs, all running a specific plugin's logic.

**Example: The `casino-stake` Plugin**

1. **Create a Workload Pool:**
  This command creates a fleet of 5 microVMs, each dedicated to the `casino-stake` plugin. Volant automatically handles resource allocation and ensures each VM has the necessary configuration.

  ```bash
  volar workloads create my-stake-farm --plugin casino-stake --count 5
  ```

2. **Execute Plugin-Specific Actions:**
  Once the workload is active, you can use the `volar workloads action` command to execute high-level commands that the plugin exposes. The plugin translates these simple commands into complex browser automation sequences.

  ```bash
  # Claim the daily bonus across all 5 accounts in the pool
  volar workloads action my-stake-farm claim_daily_bonus

  # Check the balance of all accounts
  volar workloads action my-stake-farm check_balance
  ```

3. **Monitor the Workload:**
  You can get a real-time status update for the entire fleet of VMs in your workload.

  ```bash
  volar workloads status my-stake-farm
  ```

## Developing a Plugin

(Note: The full plugin development guide is forthcoming.)
