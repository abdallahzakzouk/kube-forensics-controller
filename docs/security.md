# Security & Architecture

This document details the security model, architectural decisions, and hardening measures implemented in the controller.

## Threat Model
See [THREAT_MODEL.md](../THREAT_MODEL.md) for a detailed STRIDE analysis.

## Security Features

### 1. Service Account Hardening
The controller automatically sets `AutomountServiceAccountToken: false` on all forensic pods.
**Why:** This ensures that even if an attacker compromises a forensic pod, they cannot use the original workload's Service Account identity to attack the Kubernetes API.

### 2. Secret Redaction
Cloning secrets is risky. We provide two layers of defense:
1.  **Global Disable:** Start the controller with `--enable-secret-cloning=false`. All secrets in the forensic namespace will be replaced with dummy `REDACTED` values.
2.  **Per-Pod Opt-Out:** Add `forensic.io/no-secret-clone: "true"` to your Pod.

### 3. Network Isolation
The controller creates a **Default Deny Egress** NetworkPolicy in the `debug-forensics` namespace.
**Why:** To prevent a compromised forensic pod (or a developer debugging it) from accidentally connecting to production databases or external C2 servers.

### 4. Capability Dropping
The controller explicitly drops dangerous capabilities (`NET_ADMIN`, `SYS_ADMIN`, `SYS_PTRACE`) from the forensic pod spec.

### 5. Collector Job Security (Checkpointing)
**Note:** Enabling `--enable-checkpointing` introduces higher privileges.
To exfiltrate checkpoint archives, the controller launches a temporary **Collector Job**.
*   **Privilege:** This job runs as **root** with `privileged: true` to access the node's filesystem.
*   **HostPath:** It mounts `/var/lib/kubelet/checkpoints` (read-only for the directory, specific file access).
*   **Mitigation:** The job is short-lived (TTL 5 mins), pinned to a specific node, and runs only when a crash is detected.

## Architectural Decisions

### Pod Cloning vs. Ephemeral Containers
We chose **Pod Cloning** over `kubectl debug` (Ephemeral Containers) for forensic analysis.

| Feature | Pod Cloning (Our Approach) | Ephemeral Containers |
| :--- | :--- | :--- |
| **Post-Mortem Analysis** | ✅ Captures state even after the original pod dies. | ❌ Requires a *running* target pod. |
| **Isolation** | ✅ Runs in a separate, quarantined namespace. | ❌ Runs in the same namespace (risk of side effects). |
| **Reproducibility** | ✅ Can restart the app process from scratch (`entrypoint`). | ❌ Attaches to an existing process tree. |
| **Resource Usage** | ✅ Separate resource quota. | ❌ Consumes resources of the target node/pod. |

### Limitations (The "Fresh Container" Issue)
Because we clone the pod spec:
*   **Filesystem:** The forensic pod starts with a **fresh** writable layer from the image. Files written to `/tmp` in the crashed pod are **lost**.
*   **Memory:** RAM contents are **lost**.

*Mitigation:* We support **Volume Snapshots** to capture persistent data (PVCs) and **Checkpoint API** (experimental) to capture state dumps.
