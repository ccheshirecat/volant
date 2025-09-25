---
title: Introduction
description: Volant. Underestimated. Unstoppable.
---

# VOLANT

**Volant is a microVM-based browser automation framework built on a single, defiant premise: the old way is broken.**

We reject the fragile, process-based paradigm of traditional tools. We believe that true security and statefulness are not features to be bolted on; they are the **unbreakable foundation** upon which all modern automation must be built.

We provide this foundation through kernel-isolated microVMs. This is not an incremental improvement. It is a fundamental architectural shift.

---

## A Flawed Foundation

The entire landscape of browser automation is a graveyard of hacks built on a flawed foundation. Tools like Puppeteer and Playwright run the browser as a simple process, making them fundamentally insecure, easy to detect, and a nightmare to manage at scale. The industry has been engaged in a constant, unwinnable arms race.

**We are not here to build a better weapon for that race. We are here to end it.**

Volant solves these problems at the architectural level. We don't patch the browser; we give every browser its own pristine, disposable operating system.

| Feature             | The Old Way (Puppeteer/Playwright) | The Volant Way                                |
| ------------------- | :--------------------------------: | :-----------------------------------------------: |
| **Isolation**       |         Process Sandbox          | **Kernel-Level Isolation (via microVM)**         |
| **State Persistence** |        Manual (Cookies)          | **Full System Snapshots (True Statefulness)**      |
| **Stealth**         |      Constant Cat-and-Mouse      | **Pristine, Forensically-Perfect Environments**  |
| **Scalability**     |       Complex & Brittle          | **Natively Orchestrated & Built to Scale**       |

---

## Simplicity by Design

Virtualization sounds complex. We've made it simple. Volant is an aggressively **opinionated platform** that abstracts away the entire infrastructure layer.

The `volar setup` command transforms any Linux machine into a private browser cloud in minutes. Our "God Mode" TUI provides a beautiful, interactive command center. The complexity is still there—we just handled it for you.

This is not a tool for infrastructure experts. This is a weapon for builders. Power users are free to look under the hood and customize everything, but our default is a single, powerful promise: **it just works.**

---

## Who is this for?

Volant is for anyone who is tired of the old compromises and requires a professional-grade, reliable, and secure platform for their web-based workloads.

- **Automation Engineers** building high-stakes, mission-critical workflows.
- **Security Researchers** who need a truly sandboxed environment for analysis.
- **AI Developers** looking for a secure, stateful, and observable execution layer for their autonomous agents.
- **Anyone** who believes their LLM deserves a better, safer way to interact with the web.

---

## Where's The Hype?

The name would be ironic if we couldn't back it up.

Volant is **AI-native** from the ground up. The control plane speaks **Model Context Protocol (MCP)** and streams **AG-UI events** out of the box.

This isn't a feature bolted on to chase a trend. Our entire architecture—the secure isolation, the stateful persistence, the simple API—was designed to be the perfect physical execution layer for AI. We built the platform that we, and our agents, wanted to use.

**Hype is the product. MicroVMs are the truth.**

> **Repo**: [`github.com/ccheshirecat/volant`](https://github.com/ccheshirecat/volant)