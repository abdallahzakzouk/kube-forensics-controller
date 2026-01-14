# Configuration Guide

The Kube Forensics Controller is highly configurable via command-line flags passed to the binary.

If using Helm, these maps to `config.*` values in `values.yaml`.

## Controller Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--target-namespace` | `debug-forensics` | The namespace where forensic pods and cloned resources will be created. |
| `--forensic-ttl` | `24h` | Duration after which forensic resources are automatically deleted (e.g., `30m`, `1h`, `24h`). |
| `--max-log-size` | `512000` | Maximum size of original logs to capture in bytes (default ~500KB). |
| `--ignore-namespaces` | `kube-system,kube-public` | Comma-separated list of namespaces to ignore crashes in. |
| `--watch-namespaces` | `""` (All) | Comma-separated list of allowed namespaces. If set, only these namespaces are monitored. |
| `--rate-limit-window` | `1h` | Window for deduplicating similar crashes. Only one forensic pod per unique crash signature is created in this window. |
| `--enable-secret-cloning` | `true` | Enable/Disable cloning of secrets. If `false`, secrets are redacted. |
| `--enable-checkpointing` | `false` | Enable experimental Container Checkpointing (requires Kubelet feature gate). |
| `--collector-image` | `...:v0.1.0` | Image used for the forensic collector job (defaults to controller image). |
| `--s3-bucket` | `""` | S3 Bucket name for exporting forensic artifacts (logs). |
| `--s3-region` | `us-east-1` | AWS Region for S3. |
| `--enable-datadog-profiling` | `false` | Enable Datadog Continuous Profiling (requires `DD_AGENT_HOST` env var). |
| `--datadog-service-name` | `kube-forensics-controller` | Service name for Datadog tagging. |
| `--zap-devel` | `false` | Enable development logging (human-readable text). Defaults to structured JSON for production. |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `AWS_ACCESS_KEY_ID` | AWS Credentials for S3 Export (if not using IRSA). |
| `AWS_SECRET_ACCESS_KEY` | AWS Credentials for S3 Export (if not using IRSA). |
| `DD_AGENT_HOST` | Host IP of the Datadog Agent (required if profiling is enabled). |

## Annotations

You can control behavior per-pod using annotations on your workloads:

| Annotation | Value | Description |
|------------|-------|-------------|
| `forensic.io/no-secret-clone` | `"true"` | Prevents cloning secrets for this specific pod, even if global cloning is enabled. |
| `forensic.io/hold` | `"true"` | **On Forensic Pod:** Prevents TTL cleanup. Keeps the forensic pod indefinitely. |
