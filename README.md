# Kube Forensics Controller

![Static Badge](https://img.shields.io/badge/build-successful?style=for-the-badge&logoColor=blue&label=Docker&link=https%3A%2F%2Fhub.docker.com%2Fr%2Famzacdocker%2Fkube-forensics-controller)

[![zread](https://img.shields.io/badge/Ask_Zread-_.svg?style=for-the-badge&color=00b0aa&labelColor=000000&logo=data%3Aimage%2Fsvg%2Bxml%3Bbase64%2CPHN2ZyB3aWR0aD0iMTYiIGhlaWdodD0iMTYiIHZpZXdCb3g9IjAgMCAxNiAxNiIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KPHBhdGggZD0iTTQuOTYxNTYgMS42MDAxSDIuMjQxNTZDMS44ODgxIDEuNjAwMSAxLjYwMTU2IDEuODg2NjQgMS42MDE1NiAyLjI0MDFWNC45NjAxQzEuNjAxNTYgNS4zMTM1NiAxLjg4ODEgNS42MDAxIDIuMjQxNTYgNS42MDAxSDQuOTYxNTZDNS4zMTUwMiA1LjYwMDEgNS42MDE1NiA1LjMxMzU2IDUuNjAxNTYgNC45NjAxVjIuMjQwMUM1LjYwMTU2IDEuODg2NjQgNS4zMTUwMiAxLjYwMDEgNC45NjE1NiAxLjYwMDFaIiBmaWxsPSIjZmZmIi8%2BCjxwYXRoIGQ9Ik00Ljk2MTU2IDEwLjM5OTlIMi4yNDE1NkMxLjg4ODEgMTAuMzk5OSAxLjYwMTU2IDEwLjY4NjQgMS42MDE1NiAxMS4wMzk5VjEzLjc1OTlDMS42MDE1NiAxNC4xMTM0IDEuODg4MSAxNC4zOTk5IDIuMjQxNTYgMTQuMzk5OUg0Ljk2MTU2QzUuMzE1MDIgMTQuMzk5OSA1LjYwMTU2IDE0LjExMzQgNS42MDE1NiAxMy43NTk5VjExLjAzOTlDNS42MDE1NiAxMC42ODY0IDUuMzE1MDIgMTAuMzk5OSA0Ljk2MTU2IDEwLjM5OTlaIiBmaWxsPSIjZmZmIi8%2BCjxwYXRoIGQ9Ik0xMy43NTg0IDEuNjAwMUgxMS4wMzg0QzEwLjY4NSAxLjYwMDEgMTAuMzk4NCAxLjg4NjY0IDEwLjM5ODQgMi4yNDAxVjQuOTYwMUMxMC4zOTg0IDUuMzEzNTYgMTAuNjg1IDUuNjAwMSAxMS4wMzg0IDUuNjAwMUgxMy43NTg0QzE0LjExMTkgNS42MDAxIDE0LjM5ODQgNS4zMTM1NiAxNC4zOTg0IDQuOTYwMVYyLjI0MDFDMTQuMzk4NCAxLjg4NjY0IDE0LjExMTkgMS42MDAxIDEzLjc1ODQgMS42MDAxWiIgZmlsbD0iI2ZmZiIvPgo8cGF0aCBkPSJNNCAxMkwxMiA0TDQgMTJaIiBmaWxsPSIjZmZmIi8%2BCjxwYXRoIGQ9Ik00IDEyTDEyIDQiIHN0cm9rZT0iI2ZmZiIgc3Ryb2tlLXdpZHRoPSIxLjUiIHN0cm9rZS1saW5lY2FwPSJyb3VuZCIvPgo8L3N2Zz4K&logoColor=ffffff)](https://zread.ai/abdallahzakzouk/kube-forensics-controller)

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
3.  **Load:** `make kind-load IMG=controller:v0.2.2`
4.  **Deploy:** `make deploy`
5.  **Crash:** `kubectl apply -f example/crashing-pod.yaml`

*See [Getting Started](docs/getting-started.md) for the full guide.*

## 📦 Installation (Production)

To install the controller on any standard cluster using Helm:

```bash
helm install forensics ./charts/kube-forensics-controller \
  --set image.tag=v0.2.2 \
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
