# Setup Package

`internal/setup` contains the host bootstrap logic used by `volar setup`. It is intentionally internal to ensure the CLI remains the single entry point.

## Behaviour Summary
- Validates privileges (requires root unless in `--dry-run`).
- Ensures runtime/log directories exist (default `~/.volant/run` and `~/.volant/logs`).
- Ensures required binaries (`ip`, `iptables`, `cloud-hypervisor`) are present.
- Creates/configures Linux bridge (default `vbr0`), assigns `VOLANT_HOST_IP`/CIDR, and brings interface up.
- Enables IPv4 forwarding and installs NAT + FORWARD rules (idempotent).
- Optionally writes a systemd unit when `ServicePath`/`BinaryPath` are provided.
- Captures executed commands (or, in dry-run, the commands that would be executed).

## TODO
- Detect existing bridge configuration to avoid recreating if already present.
- Support removal/rollback (`volar setup --uninstall`).
- Add structured diagnostics for each check and eventual logging integration.
- Converge on user-visible output format shared with CLI.

