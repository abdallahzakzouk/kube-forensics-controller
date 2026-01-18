# Features Deep Dive

This document provides a detailed look at the core features of the Kube Forensics Controller, their implementation, and their limitations.

## Capabilities & Limitations

**What this tool DOES capture:**
*   ✅ **Configuration:** The exact Environment Variables, ConfigMaps, and Secrets mounted at the time of the crash.
*   ✅ **Logs:** The standard output/error logs of the crashed container (preserved in a ConfigMap and optionally exported to S3).
*   ✅ **Networking Context:** The pod is placed in a network-isolated environment to test connectivity safely.
*   ✅ **PVC Data:** Triggers [Volume Snapshots](#3-volume-snapshots-persistence) for Persistent Volume Claims.

**What this tool DOES NOT capture (by default):**
*   ❌ **Filesystem Changes:** Files written to the container's writable layer (e.g., `/tmp`, `/var/run`) are **lost** when the original container dies. The forensic pod starts with a **fresh** filesystem from the image.
*   ❌ **Memory (RAM):** The contents of RAM (variables, encryption keys in memory) are lost.
*   ❌ **Process Tree:** The forensic pod runs `sleep infinity`, not the original process tree.

*Note: Capturing filesystem and memory requires the [Container Checkpointing](#4-container-checkpointing-experimental) feature, which is currently experimental.*

## 1. Smart Deduplication (Rate Limiting)
To prevent "Crash Storms" (where a broken deployment spawns 100s of forensic pods), the controller implements smart deduplication.

*   **Signature:** `SHA256(Namespace + WorkloadName + ContainerName + ExitCode)`
*   **Logic:** Before creating a forensic pod, the controller checks if an existing forensic pod with the same signature was created within the `RateLimitWindow` (default 1h).
*   **Result:** You get exactly **one** forensic snapshot per unique failure type per hour.

## 2. Chain of Custody (Integrity)
Forensic evidence must be trusted.
1.  **Hashing:** When logs are captured, the controller calculates a SHA-256 hash.
2.  **Stamping:** The hash is stored as an immutable annotation `forensic.io/log-sha256` on the forensic pod.
3.  **Verification:** The `kubectl forensic export` command recalculates the hash of the downloaded logs and verifies it against the stamp.

3.  **Volume Snapshots (Persistence)**
If the crashed pod has Persistent Volume Claims (PVCs):
1.  The controller identifies the PVCs.
2.  It creates a `VolumeSnapshot` in the source namespace.
3.  It annotates the forensic pod with the snapshot names (`forensic.io/snapshots`).
*Requirement:* The cluster must support CSI Volume Snapshots and have a default `VolumeSnapshotClass`.

**Next Steps (Roadmap):** Automated "Restore-and-Mount" logic to automatically attach these snapshots to the forensic pod.

## 4. Container Checkpointing (Experimental)
*Requires: `ContainerCheckpoint` feature gate enabled on Kubelet.*

If enabled (`--enable-checkpointing=true`), the controller:
1.  Locates the node of the crashed pod.
2.  Calls the Kubelet API (`POST /checkpoint/...`).
3.  The Kubelet dumps a `.tar` archive of the container memory and disk to `/var/lib/kubelet/checkpoints/` on the node.
4.  The path is annotated on the forensic pod (`forensic.io/checkpoint`).

**Exfiltration Workflow:**
If an S3 Bucket is configured, the controller automatically:
1.  Launches a privileged **Collector Job** pinned to the specific node.
2.  Mounts the checkpoint file.
3.  Calculates the **SHA256 Hash**.
4.  Uploads both the artifact (`checkpoint.tar`) and the hash (`.sha256`) to S3.
5.  Cleans up the file from the node to prevent disk exhaustion.

**Next Steps (Roadmap):** Automated exfiltration of the `.tar` file to S3 via an ephemeral retriever pod.

### On-Demand Trigger (Live Forensics)
Since you cannot checkpoint a crashed (dead) process, this feature is primarily for **Live Forensics** (e.g., investigating a hanging or compromised pod).

**Usage:**
```bash
kubectl annotate pod my-suspicious-pod forensic.io/request-checkpoint=true
```
The controller will detect the annotation, trigger the checkpoint, exfiltrate it to S3 (if configured), and then remove the annotation.

## 5. S3 Log Export
The controller can automatically upload captured logs to S3.
*   **Path:** `s3://<bucket>/<namespace>/<pod>/<timestamp>/crash.log`
*   **Auth:** Uses standard AWS SDK chain (IRSA / Env Vars / Instance Profile).

## 6. Observability Metrics
The controller exposes Prometheus-format metrics on port `8080` at `/metrics`.

| Metric Name | Type | Description | Labels |
|-------------|------|-------------|--------|
| `forensics_crashes_total` | Counter | Total number of crashes detected. | `namespace`, `reason` |
| `forensics_pods_created_total` | Counter | Number of forensic pods successfully created. | `source_namespace` |
| `forensics_pod_creation_errors_total` | Counter | Number of errors during creation workflow. | `source_namespace`, `step` |

**Datadog Users:** These metrics are compatible with the Datadog OpenMetrics integration.

## 7. Universal Toolkit Injection
The controller injects a **Forensic Toolkit** into every cloned pod at `/usr/local/bin/toolkit`.
*   **Content:** A statically linked (`musl`) `busybox` binary providing `sh`, `ls`, `cat`, `nc`, `curl`, etc.
*   **Compatibility:** Works on **distroless** images, Alpine, Debian, and RedHat variants without dependency errors (`glibc` independent).

