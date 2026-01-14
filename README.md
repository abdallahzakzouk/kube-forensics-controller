# Kube Forensics Controller

**Stop Chasing Ghosts in Your Kubernetes Cluster.**

The **Kube Forensics Controller** is a Kubernetes Operator that automatically captures the state of crashed pods ("Crime Scenes") before they are deleted or rescheduled. It creates a forensic clone in a quarantined sandbox, allowing you to debug intermittent failures ("Heisenbugs") safely and efficiently.

## 🚀 Key Features

*   **Automated Forensics:** Instantly clones crashed pods (Failed/Error/OOMKilled) into a [quarantined sandbox](docs/security.md#3-network-isolation).
*   **Evidence Preservation:** Captures logs, configuration, and triggers [Volume Snapshots](docs/features.md#3-volume-snapshots-persistence) for PVCs.
*   **Toolkit Injection:** Automatically injects debugging tools (shell, curl, nc) into [distroless containers](docs/features.md).
*   **Production Safe:** Implements [Smart Rate Limiting](docs/features.md#1-smart-deduplication-rate-limiting) to prevent crash storms and [Secret Redaction](docs/security.md#2-secret-redaction) for security.
*   **Chain of Custody:** Hashes logs with SHA-256 for [integrity verification](docs/features.md#2-chain-of-custody-integrity).

## 📚 Documentation Index

| Topic | Description |
| :--- | :--- |
| [**Getting Started**](docs/getting-started.md) | Run locally with Kind in 5 minutes. |
| [**Installation Guide**](docs/installation.md) | Helm charts and manifests for EKS/GKE/AKS. |
| [**Configuration Reference**](docs/configuration.md) | Full list of Flags, Environment Variables, and Annotations. |
| [**Security & Architecture**](docs/security.md) | Hardening, Redaction, and Design Decisions. |
| [**Features Deep Dive**](docs/features.md) | Snapshots, Checkpointing, and S3 details. |
| [**CLI Plugin Manual**](docs/cli-plugin.md) | Usage of the `kubectl-forensic` tool. |

## ⚡ Quick Start (Local Development)

**Prerequisites:** Docker, Kind, kubectl.

1.  **Create Cluster:** `kind create cluster`
2.  **Build:** `make docker-build`
3.  **Load:** `make kind-load IMG=controller:v0.2.0`
4.  **Deploy:** `make deploy`
5.  **Crash:** `kubectl apply -f example/crashing-pod.yaml`

*See [Getting Started](docs/getting-started.md) for the full guide.*

## 📦 Installation (Production)

To install the controller on any standard cluster using Helm:

```bash
helm install forensics ./charts/kube-forensics-controller \
  --set image.tag=v0.2.0 \
  --set config.enableSecretCloning=true
```

*See [Installation Guide](docs/installation.md) for S3 and RBAC configuration.*

## 🛠️ Kubectl Plugin

Simplify your investigation with our CLI plugin:

```bash
make plugin
sudo cp bin/kubectl-forensic /usr/local/bin/kubectl-forensic

kubectl forensic list
kubectl forensic access <pod-name>
```

*See [CLI Plugin Docs](docs/cli-plugin.md).*

## License
MIT License. See [LICENSE](LICENSE).
