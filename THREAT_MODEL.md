# Threat Model & Security Architecture

This document outlines the security model, potential threats, and mitigations implemented in the Kube Forensics Controller.

## Architecture Overview

The controller operates as a Kubernetes Operator. It watches for Pod events, clones crashed pods into a dedicated namespace (`debug-forensics`), and facilitates debugging.

### Trusted Boundaries
1.  **Controller Manager**: Runs with high privileges (ClusterRole) to manage pods across namespaces.
2.  **Forensic Namespace**: A semi-trusted zone where forensic pods live.
3.  **User/Developer**: Users with `kubectl` access to the forensic namespace.

## Asset Identification

| Asset | Description | Value |
| :--- | :--- | :--- |
| **Production Secrets** | Database passwords, API keys used by workloads. | Critical |
| **Production Data** | Data accessible via Pod connections. | Critical |
| **K8s API Access** | Service Account tokens. | Critical |
| **Forensic Evidence** | Logs and filesystem state of crashed pods. | High |

## Threat Analysis (STRIDE)

### 1. Spoofing / Tampering
**Threat:** An attacker compromises a forensic pod and modifies the logs to hide evidence of an attack.
*   **Mitigation (Chain of Custody):** The controller calculates a **SHA256 Hash** of the logs immediately upon crash detection and stores it in the `forensic.io/log-sha256` annotation. The `kubectl forensic export` command verifies this hash before exporting.

### 2. Information Disclosure
**Threat:** A developer gains access to `debug-forensics` and views highly sensitive production secrets (e.g., Payment Keys) cloned from a crashed pod.
*   **Mitigation (Secret Redaction):**
    *   **Global Flag:** `--enable-secret-cloning=false` replaces all secret values with `REDACTED`.
    *   **Per-Pod Annotation:** Workloads can be annotated with `forensic.io/no-secret-clone: "true"` to opt-out of cloning.
    *   **RBAC:** Access to the `debug-forensics` namespace should be restricted.

### 3. Denial of Service (DoS)
**Threat:** A Deployment enters a crash loop (100 replicas crashing every second), causing the controller to flood the cluster with thousands of forensic pods, exhausting resources.
*   **Mitigation (Rate Limiting):** The controller calculates a "Crash Signature" (Hash of Namespace + Workload + Container + ExitCode). It enforces a **Rate Limit Window** (default 1h), creating only *one* forensic pod per signature per window.
*   **Mitigation (TTL):** Forensic pods are automatically deleted after a configurable TTL (default 24h).

### 4. Elevation of Privilege
**Threat:** An attacker uses the shell inside a forensic pod to leverage the *original workload's* Service Account (SA) to attack the Kubernetes API.
*   **Mitigation (SA Hardening):** The controller explicitly sets `AutomountServiceAccountToken: false` on all forensic pods. The forensic pod has **no** API access identity.

### 5. Lateral Movement
**Threat:** An attacker uses the forensic pod as a jump host to scan the internal network or connect to production databases.
*   **Mitigation (Network Isolation):** The controller automatically creates a `Default Deny Egress` NetworkPolicy in the `debug-forensics` namespace, blocking all outgoing connections.

## Security Configuration Checklist

For high-security environments, we recommend the following configuration:

- [ ] Run with `--enable-secret-cloning=false` (Redaction enabled).
- [ ] Use `--watch-namespaces` to restrict the controller's scope to non-system namespaces.
- [ ] Apply strict RBAC to the `debug-forensics` namespace (only specific SREs should have access).
- [ ] Ensure NetworkPolicies are enforced by the CNI plugin.

## Vulnerability Reporting

Please do not report security vulnerabilities via public GitHub issues.
Contact the maintainer directly at: `abdallah.m.zakzouk@gmail.com`
