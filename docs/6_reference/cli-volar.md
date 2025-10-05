# volar CLI reference

Source: internal/cli/standard.

## Global flag
- --api, -a: base URL for volantd (default from VOLANT_API_BASE or http://127.0.0.1:7777)

## Commands

- version — print CLI version
- vms — manage microVMs
  - list — list VMs
  - get <name> — show details
  - create <name> [flags] — create a VM
    - --plugin <name>
    - --runtime <type>
    - --cpu <n>
    - --memory <mb>
    - --kernel-cmdline <extra>
    - --config <path to JSON>
    - --api-host <host> / --api-port <port>
    - --device <pci> (repeatable)
    - --device-allowlist <pattern> (repeatable)
  - delete <name>
  - start <name>
  - stop <name>
  - restart <name>
  - scale <name> [--cpu N] [--memory MB] [--restart] | for deployments: --replicas N
  - config
    - get <name> [--raw] [--output file]
    - set <name> --file <path>
    - history <name> [--limit N]
  - console <name> [--socket <path>] — attach to serial socket
  - operations <vm> — list operations from the VM’s plugin OpenAPI
  - call <vm> <operation-id> [--query k=v] [--body '{}'] [--body-file file] [--timeout 60s]

- plugins — manage engine plugins
  - list
  - show <name>
  - enable <name>
  - disable <name>
  - install [--manifest <file> | --url <http(s)>] (positional arg allowed)
  - remove <name>

- deployments — manage VM groups
  - list
  - create <name> --config <file> [--replicas N]
  - get <name> [--output file]
  - delete <name>
  - scale <name> <replicas>

- setup — configure host networking and service (Linux)
  - Flags: --bridge, --subnet, --host-ip, --dry-run, --runtime-dir, --log-dir,
           --service-file, --work-dir, --bzimage, --vmlinux
