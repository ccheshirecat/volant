# Deployments guide

Ground truth: internal/server/orchestrator/orchestrator.go (deployments: CreateDeployment, reconcileDeployment, buildDeployment, ScaleDeployment, DeleteDeployment).

Deployments manage a named group of identical VM replicas from a single config. The orchestrator keeps actual VMs in sync with desired replicas and configuration.

## Concepts

- Deployment name: user-chosen identifier
- DesiredReplicas: target number of VMs
- ReadyReplicas: number of VMs currently running
- Config: the vmconfig.Config used for new/updated replicas

## Creating a deployment

```bash
cat > web.json <<EOF
{ "plugin": "caddy", "resources": { "cpu_cores": 2, "memory_mb": 512 } }
EOF
volar deployments create web --config web.json --replicas 3
```

The orchestrator normalizes config (infers runtime from plugin or manifest if missing) and persists it with the group. It launches missing VMs using the normalized config.

## Scaling

```bash
volar deployments scale web 5
```

- If current < desired: create new VMs with unique names (web-1, web-2, …) filling gaps.
- If current > desired: delete highest-indexed VMs first.

## Naming and gaps

Replica names follow <name>-<index>. Missing indices are filled when scaling up. The reconcile loop ensures consistent state after failures or manual deletions.

## Updating config

Currently, deployments store the config used for creating new replicas. To change CPU/memory or other fields of an existing VM, use per-VM config update + optional restart:

```bash
volar vms scale web-3 --cpu 4 --restart
```

## Deletion

```bash
volar deployments delete web
```

All member VMs are stopped and deleted, the group record is removed, and any cloud-init seed files are cleaned up.
