# Kube Forensics Controller

This is a Kubernetes controller that automatically creates forensic copies of crashed Pods for debugging purposes.

## Features

- **Crash Detection**: Watches for Pods in `Failed` phase or containers with `Error`/`OOMKilled` states.
- **Forensic Cloning**: Creates a copy of the crashed pod in a dedicated `debug-forensics` namespace.
- **State Preservation**: Clones all referenced ConfigMaps and Secrets to the debug namespace.
- **Safe Debugging**:
  - Removes Liveness/Readiness probes.
  - Overrides the command to `sleep infinity` so you can `kubectl exec` into it.
  - Network Isolation: Creates a default-deny Egress NetworkPolicy in `debug-forensics`.
- **Smart Deduplication**: Uses a hash of `(Namespace + Workload + Container)` to identify crash signatures and prevents "crash storms" via configurable rate limiting.
- **Security Control**: Supports opting out of Secret cloning globally or per-pod for sensitive workloads.

## Capabilities & Limitations

**What this tool DOES capture:**
*   ✅ **Configuration:** The exact Environment Variables, ConfigMaps, and Secrets mounted at the time of the crash.
*   ✅ **Logs:** The standard output/error logs of the crashed container (preserved in a ConfigMap).
*   ✅ **Networking Context:** The pod is placed in a network-isolated environment to test connectivity safely.

**What this tool DOES NOT capture (yet):**
*   ❌ **Filesystem Changes:** Files written to the container's writable layer (e.g., `/tmp`, `/var/run`) are **lost** when the original container dies. The forensic pod starts with a **fresh** filesystem from the image.
*   ❌ **Memory (RAM):** The contents of RAM (variables, encryption keys in memory) are lost.
*   ❌ **Process Tree:** The forensic pod runs `sleep infinity`, not the original process tree.

*Note: Capturing filesystem and memory requires the Kubernetes Checkpoint API, which is on our [Roadmap](#future-roadmap).*

## Prerequisites

- **Kubernetes Cluster** (v1.25+) - EKS, GKE, AKS, Kind, Minikube, etc.
- **kubectl** installed and configured.
- **Helm** (optional, for Chart installation).

## Quick Start (Local Development with Kind)

This guide assumes you want to try the controller locally using **Kind** (Kubernetes in Docker).

**Requirements for Quick Start:** Kind, Docker.

1. **Create a Kind cluster** (if you don't have one):
   ```bash
   kind create cluster
   ```

2. **Build the Docker Image**:
   ```bash
   make docker-build
   ```

3. **Load the Image into Kind**:
   ```bash
   make kind-load IMG=controller:v0.1.0
   ```

4. **Deploy the Controller**:
   ```bash
   make deploy
   ```

5. **Verify Deployment**:
   ```bash
   kubectl get pods -l control-plane=controller-manager
   ```

## Installation via Helm (Production)

To install the controller on any standard cluster (EKS, GKE, etc.):

```bash
# 1. Add repo (if hosted) or clone
git clone https://github.com/abdallahzakzouk/kube-forensics-controller.git
cd kube-forensics-controller

# 2. Install Chart
helm install forensics ./charts/kube-forensics-controller \
  --set image.repository=ghcr.io/abdallahzakzouk/kube-forensics-controller \
  --set image.tag=v0.1.0 \
  --set config.enableSecretCloning=true
```

*(Note: You will need to build and push the image to a registry accessible by your cluster, or use the pre-built image if available).*

## Kubectl Plugin

The project includes a `kubectl` plugin to simplify interacting with forensic pods.

**Installation:**
```bash
make plugin
sudo cp bin/kubectl-forensic /usr/local/bin/kubectl-forensic
```

**Usage:**
*   **List Forensic Pods:**
    ```bash
    kubectl forensic list
    ```
*   **Access Shell (Auto-connects to Toolkit):**
    ```bash
    kubectl forensic access <pod-name>
    ```
*   **View Original Crash Logs:**
    ```bash
    kubectl forensic logs <pod-name>
    ```
*   **Export Logs (with Integrity Check):**
    ```bash
    kubectl forensic export <pod-name> > crash.log
    ```
    *This verifies the SHA256 hash of the logs against the signature stored at the time of the crash.*

## Configuration

The controller supports the following command-line flags to customize its behavior:

| Flag | Default | Description |
|------|---------|-------------|
| `--target-namespace` | `debug-forensics` | The namespace where forensic pods and resources will be created. |
| `--forensic-ttl` | `24h` | Duration after which forensic resources are automatically deleted (e.g., `30m`, `1h`, `24h`). |
| `--max-log-size` | `512000` | Maximum size of original logs to capture in bytes (default ~500KB). |
| `--ignore-namespaces` | `kube-system,kube-public` | Comma-separated list of namespaces to ignore crashes in. |
| `--watch-namespaces` | `""` (All) | Comma-separated list of allowed namespaces. If set, only these namespaces are monitored. |
| `--rate-limit-window` | `1h` | Window for deduplicating similar crashes. Only one forensic pod per unique crash signature is created in this window. |
| `--enable-secret-cloning` | `true` | Enable/Disable cloning of secrets. If false, secrets are redacted. |

## Security Features

### Service Account Hardening
The controller automatically disables `AutomountServiceAccountToken` on all forensic pods. This ensures that even if a forensic pod is compromised, it cannot be used to attack the Kubernetes API using the source workload's identity.

### Secret Redaction
You can prevent the controller from copying sensitive Secrets to the forensic namespace in two ways:
1.  **Global Disable:** Start the controller with `--enable-secret-cloning=false`. All secrets will be replaced with dummy `REDACTED` values.
2.  **Per-Pod Opt-Out:** Add the annotation `forensic.io/no-secret-clone: "true"` to your Pod.

### Network Isolation
All forensic pods are automatically restricted by a `default-deny` egress NetworkPolicy.

## Architectural Decisions & Roadmap

### Why Pod Cloning vs Ephemeral Containers?
This project deliberately uses **Pod Cloning** instead of **Ephemeral Containers**. See [Security & Architecture](docs/security.md#architectural-decisions) for the full rationale.

### Future Roadmap
While the controller is production-ready for configuration and log forensics, we are working on the following "Excellence" features:

*   **Automated Checkpoint Exfiltration:** Automatically launching a retriever pod to move Kubelet checkpoints to S3. (See [Checkpointing Docs](docs/features.md#4-container-checkpointing-experimental))
*   **Automated Snapshot Restore:** Automatically mounting Persistent Volume Snapshots into the forensic pod for instant disk inspection. (See [Snapshots Docs](docs/features.md#3-volume-snapshots-persistence))
*   **Multi-Cloud Exporters:** Native support for Azure Blob Storage and Google Cloud Storage. (See [S3 Export Docs](docs/features.md#5-s3-log-export))
*   **Fine-Grained RBAC:** A pre-configured "Namespaced" installation mode for restricted environments.

## Datadog Integration

This controller includes optional built-in support for Datadog observability.

### Metrics
The controller emits custom Prometheus metrics compatible with Datadog's OpenMetrics integration:
*   `forensics_crashes_total`: Counter of detected crashes (tagged by `namespace`, `reason`).
*   `forensics_pods_created_total`: Counter of successfully created forensic pods.
*   `forensics_pod_creation_errors_total`: Counter of failures during creation steps.

To enable metric collection, ensure your Datadog Agent is configured to scrape the controller pod (via annotations or ServiceMonitor).

### Profiling
To enable Datadog Continuous Profiling for performance debugging:
1.  Set the flag `--enable-datadog-profiling=true`.
2.  Ensure the `DD_AGENT_HOST` environment variable is set in the deployment (pointing to the node's IP or agent service).

```yaml
env:
  - name: DD_AGENT_HOST
    valueFrom:
      fieldRef:
        fieldPath: status.hostIP
```

## Production Readiness

This controller is designed for production use, but consider the following before deploying:

### Security Implications
*   **Secret Cloning:** The controller clones **Secrets** referenced by the crashing pod into the `debug-forensics` namespace. This is necessary for the application to run, but it means sensitive data is duplicated.
    *   *Mitigation:* Ensure access to the `debug-forensics` namespace is strictly restricted via RBAC. Only authorized developers should have `get/list` access to Secrets in this namespace.
*   **Network Policies:** The controller automatically creates a `deny-all-egress` NetworkPolicy in the target namespace. This prevents cloned pods from accidentally connecting to production databases or external services.

### Resource Management
*   **Quotas:** It is highly recommended to apply a **ResourceQuota** to the `debug-forensics` namespace to prevent a "crash storm" from filling the cluster with forensic pods.
*   **TTL:** The built-in TTL cleaner ensures forensic pods do not accumulate indefinitely.

### Observability
*   **Logging:** By default, the controller outputs structured JSON logs suitable for aggregators like Fluentd/Elasticsearch.
*   **Metrics:** Prometheus metrics are exposed on `:8080/metrics`.
*   **Probes:** Liveness and Readiness probes are configured on the manager deployment.

## Usage

1. **Deploy a crashing app** (example):
   ```yaml
   apiVersion: v1
   kind: Pod
   metadata:
     name: crash-app
     labels:
       app: crash
   spec:
     containers:
     - name: crash
       image: busybox
       command: ["/bin/sh", "-c", "echo 'I am crashing'; exit 1"]
   ```

2. **Wait for the crash**.

3. **Check the `debug-forensics` namespace**:
   ```bash
   kubectl get pods -n debug-forensics
   ```
   You should see a pod named `crash-app-forensic-<hash>`.

4. **Debug**:
   ```bash
   kubectl exec -it -n debug-forensics <forensic-pod-name> -- /bin/sh
   ```

## Real-World Example: Debugging a "Flaky" Service

Imagine a service that fails to start 50% of the time due to a race condition or external dependency, but eventually succeeds (CrashLoopBackOff -> Running).

**The Problem:**
- **Live Pod is Running:** When you check `kubectl get pods`, the pod is healthy. You can't debug the crash because the process is running fine *now*.
- **Logs are Limited:** Previous container logs might be rotated or insufficient to reproduce the state.
- **Can't Touch Production:** You can't just "restart" the live production pod to see if it fails again without causing downtime.

**The Forensic Solution:**
1. **Deploy the Flaky Service:**
   ```bash
   kubectl apply -f example/flaky-service.yaml
   ```
2. **Controller Detects Failure:** Even though the live pod eventually heals, the controller catches the initial crash events.
3. **Forensic Pod Created:** A snapshot of the *failed environment* is created in `debug-forensics`.

**What can you do with the Forensic Pod?**

*   **View Original Logs:** The logs from the *exact moment of the crash* are preserved in a mounted volume at `/forensics/original-logs`, even if the live pod has since restarted and wiped its local logs.
    ```bash
    cat /forensics/original-logs/crash.log
    ```

**Beyond Logs (Advanced Forensic Capabilities):**
Depending on the complexity of your service, the forensic pod provides several powerful capabilities:

*   **Isolated Reproduction:** Since the forensic pod is a clone with `sleep infinity`, you can manually trigger the original entrypoint (e.g., `./run-app.sh`) inside the container. This allows you to verify if the crash is deterministic or flaky without affecting production traffic.
*   **Toolkit Injection:** For "distroless" or minimal images that lack debugging tools, the controller automatically mounts a **Forensic Toolkit** (`/usr/local/bin/toolkit/`). This gives you access to a shell, `ls`, `cat`, and other essentials in an environment that was previously a "black box".
*   **State & Filesystem Inspection:** Unlike a fresh debug pod, this is a **living snapshot**. You can inspect local socket files, temporary lock files, or cached data exactly as they existed when the crash occurred.
*   **Network Isolation:** All forensic pods are automatically restricted by a `default-deny` egress NetworkPolicy, ensuring that your manual reproduction attempts don't accidentally reach out to production databases or external APIs.

This turns a "mystery intermittent bug" into a reproducible, debuggable environment.

## Development

To run the controller locally against your current kubeconfig context:

```bash
make run
```
# kube-forensics-controller
